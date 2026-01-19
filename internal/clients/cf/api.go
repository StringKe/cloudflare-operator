package cf

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
	"github.com/go-logr/logr"
	"k8s.io/utils/ptr"
)

// TXT_PREFIX is the prefix added to TXT records for whom the corresponding DNS records are managed by the operator.
const TXT_PREFIX = "_managed."

// API config object holding all relevant fields to use the API
type API struct {
	Log              logr.Logger
	TunnelName       string
	TunnelId         string
	AccountName      string
	AccountId        string
	Domain           string
	ValidAccountId   string
	ValidTunnelId    string
	ValidTunnelName  string
	ValidZoneId      string
	ValidDomainName  string // Domain name corresponding to ValidZoneId
	CloudflareClient *cloudflare.API
}

// TunnelCredentialsFile object containing the fields that make up a Cloudflare Tunnel's credentials
type TunnelCredentialsFile struct {
	AccountTag   string `json:"AccountTag"`
	TunnelID     string `json:"TunnelID"`
	TunnelName   string `json:"TunnelName"`
	TunnelSecret string `json:"TunnelSecret"`
}

// DnsManagedRecordTxt object that represents each managed DNS record in a separate TXT record
type DnsManagedRecordTxt struct {
	DnsId      string // DnsId of the managed record
	TunnelName string // TunnelName of the managed record
	TunnelId   string // TunnelId of the managed record
}

// CreateTunnel creates a Cloudflare Tunnel and returns the tunnel Id and credentials file
func (c *API) CreateTunnel(ctx context.Context) (string, string, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error code in getting account ID")
		return "", "", err
	}

	// Generate 32 byte random string for tunnel secret
	randSecret := make([]byte, 32)
	if _, err := rand.Read(randSecret); err != nil {
		return "", "", err
	}
	tunnelSecret := base64.StdEncoding.EncodeToString(randSecret)

	params := cloudflare.TunnelCreateParams{
		Name:   c.TunnelName,
		Secret: tunnelSecret,
		// Indicates if this is a locally or remotely configured tunnel "local" or "cloudflare"
		// Use "cloudflare" for remotely-managed mode where cloudflared uses --token flag
		// and pulls configuration from Cloudflare cloud automatically
		ConfigSrc: "cloudflare",
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)
	tunnel, err := c.CloudflareClient.CreateTunnel(ctx, rc, params)

	if err != nil {
		c.Log.Error(err, "error creating tunnel")
		return "", "", err
	}

	c.ValidTunnelId = tunnel.ID
	c.ValidTunnelName = tunnel.Name

	credentialsFile := TunnelCredentialsFile{
		AccountTag:   c.ValidAccountId,
		TunnelID:     tunnel.ID,
		TunnelName:   tunnel.Name,
		TunnelSecret: tunnelSecret,
	}

	// Marshal the tunnel credentials into a string
	creds, err := json.Marshal(credentialsFile)
	return tunnel.ID, string(creds), err
}

// DeleteTunnel deletes a Cloudflare Tunnel.
// This method is idempotent - returns nil if the tunnel is already deleted.
func (c *API) DeleteTunnel(ctx context.Context) error {
	// Only validate Account and Tunnel - Zone/Domain is NOT required for deletion
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "Error validating account ID for deletion")
		return err
	}
	if _, err := c.GetTunnelId(ctx); err != nil {
		// If tunnel ID cannot be resolved, it might already be deleted
		if IsNotFoundError(err) {
			c.Log.Info("Tunnel already deleted (not found during ID resolution)", "tunnelId", c.TunnelId, "tunnelName", c.TunnelName)
			return nil
		}
		c.Log.Error(err, "Error validating tunnel ID for deletion")
		return err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	// Deletes any inactive connections on a tunnel
	err := c.CloudflareClient.CleanupTunnelConnections(ctx, rc, c.ValidTunnelId)
	if err != nil {
		// Ignore not found errors - tunnel might already be deleted
		if !IsNotFoundError(err) {
			c.Log.Error(err, "error cleaning tunnel connections", "tunnelId", c.TunnelId)
			return err
		}
		c.Log.Info("Tunnel already deleted (not found during connection cleanup)", "tunnelId", c.ValidTunnelId)
		return nil
	}

	err = c.CloudflareClient.DeleteTunnel(ctx, rc, c.ValidTunnelId)
	if err != nil {
		if IsNotFoundError(err) {
			c.Log.Info("Tunnel already deleted (not found)", "tunnelId", c.ValidTunnelId)
			return nil
		}
		c.Log.Error(err, "error deleting tunnel", "tunnelId", c.TunnelId)
		return err
	}

	c.Log.Info("Tunnel deleted successfully", "tunnelId", c.ValidTunnelId)
	return nil
}

// ValidateAll validates the contents of the API struct
func (c *API) ValidateAll(ctx context.Context) error {
	c.Log.Info("In validation")
	if _, err := c.GetAccountId(ctx); err != nil {
		return err
	}

	if _, err := c.GetTunnelId(ctx); err != nil {
		return err
	}

	if _, err := c.GetZoneId(ctx); err != nil {
		return err
	}

	c.Log.V(5).Info("Validation successful")
	return nil
}

// GetAccountId gets AccountId from Account Name
func (c *API) GetAccountId(ctx context.Context) (string, error) {
	if c.ValidAccountId != "" {
		return c.ValidAccountId, nil
	}

	if c.AccountId == "" && c.AccountName == "" {
		err := fmt.Errorf("both account ID and Name cannot be empty")
		c.Log.Error(err, "Both accountId and accountName cannot be empty")
		return "", err
	}

	if c.validateAccountId(ctx) {
		c.ValidAccountId = c.AccountId
	} else {
		c.Log.Info("Account ID failed, falling back to Account Name")
		accountIdFromName, err := c.getAccountIdByName(ctx)
		if err != nil {
			return "", fmt.Errorf("error fetching Account ID by Account Name")
		}
		c.ValidAccountId = accountIdFromName
	}
	return c.ValidAccountId, nil
}

func (c *API) validateAccountId(ctx context.Context) bool {
	if c.AccountId == "" {
		c.Log.Info("Account ID not provided")
		return false
	}

	account, _, err := c.CloudflareClient.Account(ctx, c.AccountId)

	if err != nil {
		c.Log.Error(err, "error retrieving account details", "accountId", c.AccountId)
		return false
	}

	return account.ID == c.AccountId
}

func (c *API) getAccountIdByName(ctx context.Context) (string, error) {
	params := cloudflare.AccountsListParams{
		Name: c.AccountName,
	}
	accounts, _, err := c.CloudflareClient.Accounts(ctx, params)

	if err != nil {
		c.Log.Error(err, "error listing accounts", "accountName", c.AccountName)
	}

	switch len(accounts) {
	case 0:
		err := fmt.Errorf("no account in response")
		c.Log.Error(err, "found no account, check accountName", "accountName", c.AccountName)
		return "", err
	case 1:
		return accounts[0].ID, nil
	default:
		err := fmt.Errorf("more than one account in response")
		c.Log.Error(err, "found more than one account, check accountName", "accountName", c.AccountName)
		return "", err
	}
}

// GetTunnelId gets Tunnel Id from available information
func (c *API) GetTunnelId(ctx context.Context) (string, error) {
	if c.ValidTunnelId != "" {
		return c.ValidTunnelId, nil
	}

	if c.TunnelId == "" && c.TunnelName == "" {
		err := fmt.Errorf("both tunnel ID and Name cannot be empty")
		c.Log.Error(err, "Both tunnelId and tunnelName cannot be empty")
		return "", err
	}

	if c.validateTunnelId(ctx) {
		c.ValidTunnelId = c.TunnelId
		return c.TunnelId, nil
	}

	c.Log.Info("Tunnel ID failed, falling back to Tunnel Name")
	tunnelIdFromName, err := c.getTunnelIdByName(ctx)
	if err != nil {
		return "", fmt.Errorf("error fetching Tunnel ID by Tunnel Name")
	}
	c.ValidTunnelId = tunnelIdFromName
	c.ValidTunnelName = c.TunnelName

	return c.ValidTunnelId, nil
}

func (c *API) validateTunnelId(ctx context.Context) bool {
	if c.TunnelId == "" {
		c.Log.Info("Tunnel ID not provided")
		return false
	}

	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error in getting account ID")
		return false
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)
	tunnel, err := c.CloudflareClient.GetTunnel(ctx, rc, c.TunnelId)
	if err != nil {
		c.Log.Error(err, "error retrieving tunnel", "tunnelId", c.TunnelId)
		return false
	}

	c.ValidTunnelName = tunnel.Name
	return tunnel.ID == c.TunnelId
}

func (c *API) getTunnelIdByName(ctx context.Context) (string, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error in getting account ID")
		return "", err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)
	params := cloudflare.TunnelListParams{
		Name: c.TunnelName,
	}
	tunnels, _, err := c.CloudflareClient.ListTunnels(ctx, rc, params)

	if err != nil {
		c.Log.Error(err, "error listing tunnels by name", "tunnelName", c.TunnelName)
		return "", err
	}

	switch len(tunnels) {
	case 0:
		err := fmt.Errorf("no tunnel in response")
		c.Log.Error(err, "found no tunnel, check tunnelName", "tunnelName", c.TunnelName)
		return "", err
	case 1:
		c.ValidTunnelName = tunnels[0].Name
		return tunnels[0].ID, nil
	default:
		err := fmt.Errorf("more than one tunnel in response")
		c.Log.Error(err, "found more than one tunnel, check tunnelName", "tunnelName", c.TunnelName)
		return "", err
	}
}

// GetTunnelCreds gets Tunnel Credentials from Tunnel secret
func (c *API) GetTunnelCreds(ctx context.Context, tunnelSecret string) (string, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error in getting account ID")
		return "", err
	}

	if _, err := c.GetTunnelId(ctx); err != nil {
		c.Log.Error(err, "error in getting tunnel ID")
		return "", err
	}

	creds, err := json.Marshal(map[string]string{
		"AccountTag":   c.ValidAccountId,
		"TunnelSecret": tunnelSecret,
		"TunnelID":     c.ValidTunnelId,
		"TunnelName":   c.ValidTunnelName,
	})

	return string(creds), err
}

// GetZoneId gets Zone Id from DNS domain
func (c *API) GetZoneId(ctx context.Context) (string, error) {
	if c.ValidZoneId != "" {
		return c.ValidZoneId, nil
	}

	if c.Domain == "" {
		err := fmt.Errorf("domain cannot be empty")
		c.Log.Error(err, "Domain cannot be empty")
		return "", err
	}

	zoneIdFromName, err := c.getZoneIdByName(ctx)
	if err != nil {
		return "", fmt.Errorf("error fetching Zone ID by Zone Name")
	}
	c.ValidZoneId = zoneIdFromName
	return c.ValidZoneId, nil
}

func (c *API) getZoneIdByName(ctx context.Context) (string, error) {
	zones, err := c.CloudflareClient.ListZones(ctx, c.Domain)

	if err != nil {
		c.Log.Error(err, "error listing zones, check domain", "domain", c.Domain)
		return "", err
	}

	switch len(zones) {
	case 0:
		err := fmt.Errorf("no zone in response")
		c.Log.Error(err, "found no zone, check domain", "domain", c.Domain, "zones", zones)
		return "", err
	case 1:
		return zones[0].ID, nil
	default:
		err := fmt.Errorf("more than one zone in response")
		c.Log.Error(err, "found more than one zone, check domain", "domain", c.Domain)
		return "", err
	}
}

// InsertOrUpdateCName upsert DNS CNAME record for the given FQDN to point to the tunnel
func (c *API) InsertOrUpdateCName(ctx context.Context, fqdn, dnsId string) (string, error) {
	rc := cloudflare.ZoneIdentifier(c.ValidZoneId)
	if dnsId != "" {
		c.Log.Info("Updating existing record", "fqdn", fqdn, "dnsId", dnsId)
		updateParams := cloudflare.UpdateDNSRecordParams{
			ID:      dnsId,
			Type:    "CNAME",
			Name:    fqdn,
			Content: fmt.Sprintf("%s.cfargotunnel.com", c.ValidTunnelId),
			Comment: ptr.To("Managed by cloudflare-operator"),
			TTL:     1,            // Automatic TTL
			Proxied: ptr.To(true), // For Cloudflare tunnels
		}
		_, err := c.CloudflareClient.UpdateDNSRecord(ctx, rc, updateParams)
		if err != nil {
			c.Log.Error(err, "error code in setting/updating DNS record, check fqdn", "fqdn", fqdn)
			return "", err
		}
		c.Log.Info("DNS record updated successfully", "fqdn", fqdn)
		return dnsId, nil
	} else {
		c.Log.Info("Inserting DNS record", "fqdn", fqdn)
		createParams := cloudflare.CreateDNSRecordParams{
			Type:    "CNAME",
			Name:    fqdn,
			Content: fmt.Sprintf("%s.cfargotunnel.com", c.ValidTunnelId),
			Comment: "Managed by cloudflare-operator",
			TTL:     1,            // Automatic TTL
			Proxied: ptr.To(true), // For Cloudflare tunnels
		}
		resp, err := c.CloudflareClient.CreateDNSRecord(ctx, rc, createParams)
		if err != nil {
			c.Log.Error(err, "error creating DNS record, check fqdn", "fqdn", fqdn)
			return "", err
		}
		c.Log.Info("DNS record created successfully", "fqdn", fqdn)
		return resp.ID, nil
	}
}

// DeleteDNSId deletes DNS entry for the given dnsId.
// This method is idempotent - returns nil if the record is already deleted.
func (c *API) DeleteDNSId(ctx context.Context, fqdn, dnsId string, created bool) error {
	// Do not delete if we did not create the DNS in this cycle
	if !created {
		return nil
	}

	rc := cloudflare.ZoneIdentifier(c.ValidZoneId)
	err := c.CloudflareClient.DeleteDNSRecord(ctx, rc, dnsId)

	if err != nil {
		if IsNotFoundError(err) {
			c.Log.Info("DNS record already deleted (not found)", "dnsId", dnsId, "fqdn", fqdn)
			return nil
		}
		c.Log.Error(err, "error deleting DNS record, check fqdn", "dnsId", dnsId, "fqdn", fqdn)
		return err
	}

	c.Log.Info("DNS record deleted successfully", "dnsId", dnsId, "fqdn", fqdn)
	return nil
}

// GetDNSCNameId returns the ID of the CNAME record requested.
// Returns empty string and nil error if the record does not exist (this is not an error condition).
// Returns empty string and error if there was an actual API error or multiple records found.
func (c *API) GetDNSCNameId(ctx context.Context, fqdn string) (string, error) {
	if _, err := c.GetZoneId(ctx); err != nil {
		c.Log.Error(err, "error in getting Zone ID")
		return "", err
	}

	rc := cloudflare.ZoneIdentifier(c.ValidZoneId)
	params := cloudflare.ListDNSRecordsParams{
		Type: "CNAME",
		Name: fqdn,
	}
	records, _, err := c.CloudflareClient.ListDNSRecords(ctx, rc, params)
	if err != nil {
		c.Log.Error(err, "error listing DNS records, check fqdn", "fqdn", fqdn)
		return "", err
	}

	switch len(records) {
	case 0:
		// No record found - this is a valid result, not an error
		// The caller should check for empty string to determine if record exists
		c.Log.V(1).Info("no DNS record found for fqdn", "fqdn", fqdn)
		return "", nil
	case 1:
		return records[0].ID, nil
	default:
		err := fmt.Errorf("multiple CNAME records found for %s: %w", fqdn, ErrMultipleResourcesFound)
		c.Log.Error(err, "multiple records returned for fqdn", "fqdn", fqdn, "count", len(records))
		return "", err
	}
}

// GetManagedDnsTxt gets the TXT record corresponding to the fqdn
func (c *API) GetManagedDnsTxt(ctx context.Context, fqdn string) (string, DnsManagedRecordTxt, bool, error) {
	if _, err := c.GetZoneId(ctx); err != nil {
		c.Log.Error(err, "error in getting Zone ID")
		return "", DnsManagedRecordTxt{}, false, err
	}

	rc := cloudflare.ZoneIdentifier(c.ValidZoneId)
	params := cloudflare.ListDNSRecordsParams{
		Type: "TXT",
		Name: fmt.Sprintf("%s%s", TXT_PREFIX, fqdn),
	}
	records, _, err := c.CloudflareClient.ListDNSRecords(ctx, rc, params)
	if err != nil {
		c.Log.Error(err, "error listing DNS records, check fqdn", "fqdn", fqdn)
		return "", DnsManagedRecordTxt{}, false, err
	}

	switch len(records) {
	case 0:
		c.Log.Info("no TXT records returned for fqdn", "fqdn", fqdn)
		return "", DnsManagedRecordTxt{}, true, nil
	case 1:
		var dnsTxtResponse DnsManagedRecordTxt
		if err := json.Unmarshal([]byte(records[0].Content), &dnsTxtResponse); err != nil {
			// TXT record exists, but not in JSON
			c.Log.Error(err, "could not read TXT content in getting zoneId, check domain", "domain", c.Domain)
			return records[0].ID, dnsTxtResponse, false, err
		} else if dnsTxtResponse.TunnelId == c.ValidTunnelId {
			// TXT record exists and controlled by our tunnel
			return records[0].ID, dnsTxtResponse, true, nil
		}
	default:
		err := fmt.Errorf("multiple records returned")
		c.Log.Error(err, "multiple TXT records returned for fqdn", "fqdn", fqdn)
		return "", DnsManagedRecordTxt{}, false, err
	}
	return "", DnsManagedRecordTxt{}, false, err
}

// GetTunnelToken retrieves the token for a tunnel from Cloudflare API.
// The token is used to start cloudflared in remotely-managed mode with --token flag.
// This allows cloudflared to automatically pull configuration from Cloudflare cloud.
func (c *API) GetTunnelToken(ctx context.Context, tunnelID string) (string, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error in getting account ID")
		return "", err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	token, err := c.CloudflareClient.GetTunnelToken(ctx, rc, tunnelID)
	if err != nil {
		c.Log.Error(err, "error getting tunnel token", "tunnelId", tunnelID)
		return "", err
	}

	c.Log.V(1).Info("Got tunnel token", "tunnelId", tunnelID)
	return token, nil
}

// InsertOrUpdateTXT upsert DNS TXT record for the given FQDN to point to the tunnel
func (c *API) InsertOrUpdateTXT(ctx context.Context, fqdn, txtId, dnsId string) error {
	content, err := json.Marshal(DnsManagedRecordTxt{
		DnsId:      dnsId,
		TunnelId:   c.ValidTunnelId,
		TunnelName: c.ValidTunnelName,
	})
	if err != nil {
		c.Log.Error(err, "error marshalling txt record json", "fqdn", fqdn)
		return err
	}
	rc := cloudflare.ZoneIdentifier(c.ValidZoneId)

	if txtId != "" {
		c.Log.Info("Updating existing TXT record", "fqdn", fqdn, "dnsId", dnsId, "txtId", txtId)

		updateParams := cloudflare.UpdateDNSRecordParams{
			ID:      txtId,
			Type:    "TXT",
			Name:    fmt.Sprintf("%s%s", TXT_PREFIX, fqdn),
			Content: string(content),
			Comment: ptr.To("Managed by cloudflare-operator"),
			TTL:     1,             // Automatic TTL
			Proxied: ptr.To(false), // TXT cannot be proxied
		}
		_, err := c.CloudflareClient.UpdateDNSRecord(ctx, rc, updateParams)
		if err != nil {
			c.Log.Error(err, "error in updating DNS record, check fqdn", "fqdn", fqdn)
			return err
		}
		c.Log.Info("DNS record updated successfully", "fqdn", fqdn)
		return nil
	} else {
		c.Log.Info("Inserting DNS TXT record", "fqdn", fqdn)
		createParams := cloudflare.CreateDNSRecordParams{
			Type:    "TXT",
			Name:    fmt.Sprintf("%s%s", TXT_PREFIX, fqdn),
			Content: string(content),
			Comment: "Managed by cloudflare-operator",
			TTL:     1,             // Automatic TTL
			Proxied: ptr.To(false), // For Cloudflare tunnels
		}
		_, err := c.CloudflareClient.CreateDNSRecord(ctx, rc, createParams)
		if err != nil {
			c.Log.Error(err, "error creating DNS record, check fqdn", "fqdn", fqdn)
			return err
		}
		c.Log.Info("DNS TXT record created successfully", "fqdn", fqdn)
		return nil
	}
}

// TunnelCreateResult contains the result of a tunnel creation.
type TunnelCreateResult struct {
	ID          string
	Name        string
	Credentials *TunnelCredentialsFile
}

// CreateTunnelWithParams creates a Cloudflare Tunnel with explicit parameters.
// This method is used by the TunnelLifecycle Sync Controller.
// Returns tunnel ID, credentials, and error.
func (c *API) CreateTunnelWithParams(ctx context.Context, tunnelName, configSrc string) (*TunnelCreateResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID for tunnel creation")
		return nil, err
	}

	// Generate 32 byte random string for tunnel secret
	randSecret := make([]byte, 32)
	if _, err := rand.Read(randSecret); err != nil {
		return nil, fmt.Errorf("generate tunnel secret: %w", err)
	}
	tunnelSecret := base64.StdEncoding.EncodeToString(randSecret)

	// Default to cloudflare (remotely-managed) config source
	if configSrc == "" {
		configSrc = "cloudflare"
	}

	params := cloudflare.TunnelCreateParams{
		Name:      tunnelName,
		Secret:    tunnelSecret,
		ConfigSrc: configSrc,
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)
	tunnel, err := c.CloudflareClient.CreateTunnel(ctx, rc, params)
	if err != nil {
		c.Log.Error(err, "error creating tunnel", "name", tunnelName)
		return nil, err
	}

	c.Log.Info("Tunnel created successfully", "tunnelId", tunnel.ID, "tunnelName", tunnel.Name)

	return &TunnelCreateResult{
		ID:   tunnel.ID,
		Name: tunnel.Name,
		Credentials: &TunnelCredentialsFile{
			AccountTag:   c.ValidAccountId,
			TunnelID:     tunnel.ID,
			TunnelName:   tunnel.Name,
			TunnelSecret: tunnelSecret,
		},
	}, nil
}

// DeleteTunnelByID deletes a Cloudflare Tunnel by its ID.
// This method is used by the TunnelLifecycle Sync Controller.
// It is idempotent - returns nil if the tunnel is already deleted.
func (c *API) DeleteTunnelByID(ctx context.Context, tunnelID string) error {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error validating account ID for tunnel deletion")
		return err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	// Clean up tunnel connections first
	err := c.CloudflareClient.CleanupTunnelConnections(ctx, rc, tunnelID)
	if err != nil {
		if !IsNotFoundError(err) {
			c.Log.Error(err, "error cleaning tunnel connections", "tunnelId", tunnelID)
			return err
		}
		c.Log.Info("Tunnel already deleted (not found during connection cleanup)", "tunnelId", tunnelID)
		return nil
	}

	// Delete the tunnel
	err = c.CloudflareClient.DeleteTunnel(ctx, rc, tunnelID)
	if err != nil {
		if IsNotFoundError(err) {
			c.Log.Info("Tunnel already deleted (not found)", "tunnelId", tunnelID)
			return nil
		}
		c.Log.Error(err, "error deleting tunnel", "tunnelId", tunnelID)
		return err
	}

	c.Log.Info("Tunnel deleted successfully", "tunnelId", tunnelID)
	return nil
}

// GetTunnelIDByName looks up a tunnel ID by its name.
// This method is used by the TunnelLifecycle Sync Controller.
func (c *API) GetTunnelIDByName(ctx context.Context, tunnelName string) (string, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID for tunnel lookup")
		return "", err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	params := cloudflare.TunnelListParams{
		Name: tunnelName,
	}

	tunnels, _, err := c.CloudflareClient.ListTunnels(ctx, rc, params)
	if err != nil {
		c.Log.Error(err, "error listing tunnels by name", "name", tunnelName)
		return "", err
	}

	for _, tunnel := range tunnels {
		if tunnel.Name == tunnelName && tunnel.DeletedAt.IsZero() {
			c.Log.V(1).Info("Found tunnel by name", "name", tunnelName, "tunnelId", tunnel.ID)
			return tunnel.ID, nil
		}
	}

	return "", fmt.Errorf("tunnel not found: %s", tunnelName)
}

// GetTunnelCredsByID retrieves tunnel credentials by tunnel ID.
// This method is used by the TunnelLifecycle Sync Controller.
// Note: This method cannot retrieve the original secret, only a new token.
// For existing tunnels, use GetTunnelToken instead.
func (c *API) GetTunnelCredsByID(ctx context.Context, tunnelID string) (*TunnelCredentialsFile, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID for tunnel credentials")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	// Get tunnel details to retrieve name
	tunnel, err := c.CloudflareClient.GetTunnel(ctx, rc, tunnelID)
	if err != nil {
		c.Log.Error(err, "error getting tunnel details", "tunnelId", tunnelID)
		return nil, err
	}

	return &TunnelCredentialsFile{
		AccountTag: c.ValidAccountId,
		TunnelID:   tunnel.ID,
		TunnelName: tunnel.Name,
		// Note: TunnelSecret is not available for existing tunnels
	}, nil
}
