// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"context"

	"github.com/cloudflare/cloudflare-go"
)

// DNS record type constants.
const (
	DNSRecordTypeA     = "A"
	DNSRecordTypeAAAA  = "AAAA"
	DNSRecordTypeCNAME = "CNAME"
)

// DNSRecordParams contains parameters for creating/updating a DNS record.
type DNSRecordParams struct {
	Name     string
	Type     string
	Content  string
	TTL      int
	Proxied  bool
	Priority *int
	Comment  string
	Tags     []string
	Data     *DNSRecordDataParams
}

// DNSRecordDataParams contains structured data for special DNS record types.
type DNSRecordDataParams struct {
	// For SRV records
	Service string
	Proto   string
	Weight  int
	Port    int
	Target  string

	// For CAA records
	Flags int
	Tag   string
	Value string

	// For CERT/SSHFP/TLSA records
	Algorithm    int
	Certificate  string
	KeyTag       int
	Usage        int
	Selector     int
	MatchingType int

	// For LOC records
	LatDegrees    int
	LatMinutes    int
	LatSeconds    string
	LatDirection  string
	LongDegrees   int
	LongMinutes   int
	LongSeconds   string
	LongDirection string
	Altitude      string
	Size          string
	PrecisionHorz string
	PrecisionVert string

	// For URI records
	ContentURI string
}

// convertDataToMap converts DNSRecordDataParams to map[string]interface{} for SDK.
//
//nolint:revive // cognitive complexity is acceptable for this conversion
func convertDataToMap(data *DNSRecordDataParams) map[string]interface{} {
	if data == nil {
		return nil
	}

	result := make(map[string]interface{})

	// SRV fields
	if data.Service != "" {
		result["service"] = data.Service
	}
	if data.Proto != "" {
		result["proto"] = data.Proto
	}
	if data.Weight != 0 {
		result["weight"] = data.Weight
	}
	if data.Port != 0 {
		result["port"] = data.Port
	}
	if data.Target != "" {
		result["target"] = data.Target
	}

	// CAA fields
	if data.Flags != 0 {
		result["flags"] = data.Flags
	}
	if data.Tag != "" {
		result["tag"] = data.Tag
	}
	if data.Value != "" {
		result["value"] = data.Value
	}

	// CERT/SSHFP/TLSA fields
	if data.Algorithm != 0 {
		result["algorithm"] = data.Algorithm
	}
	if data.Certificate != "" {
		result["certificate"] = data.Certificate
	}
	if data.KeyTag != 0 {
		result["key_tag"] = data.KeyTag
	}
	if data.Usage != 0 {
		result["usage"] = data.Usage
	}
	if data.Selector != 0 {
		result["selector"] = data.Selector
	}
	if data.MatchingType != 0 {
		result["matching_type"] = data.MatchingType
	}

	// LOC fields
	if data.LatDegrees != 0 {
		result["lat_degrees"] = data.LatDegrees
	}
	if data.LatMinutes != 0 {
		result["lat_minutes"] = data.LatMinutes
	}
	if data.LatSeconds != "" {
		result["lat_seconds"] = data.LatSeconds
	}
	if data.LatDirection != "" {
		result["lat_direction"] = data.LatDirection
	}
	if data.LongDegrees != 0 {
		result["long_degrees"] = data.LongDegrees
	}
	if data.LongMinutes != 0 {
		result["long_minutes"] = data.LongMinutes
	}
	if data.LongSeconds != "" {
		result["long_seconds"] = data.LongSeconds
	}
	if data.LongDirection != "" {
		result["long_direction"] = data.LongDirection
	}
	if data.Altitude != "" {
		result["altitude"] = data.Altitude
	}
	if data.Size != "" {
		result["size"] = data.Size
	}
	if data.PrecisionHorz != "" {
		result["precision_horz"] = data.PrecisionHorz
	}
	if data.PrecisionVert != "" {
		result["precision_vert"] = data.PrecisionVert
	}

	// URI fields
	if data.ContentURI != "" {
		result["content"] = data.ContentURI
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// DNSRecordResult contains the result of a DNS record operation.
type DNSRecordResult struct {
	ID      string
	ZoneID  string
	Name    string
	Type    string
	Content string
	TTL     int
	Proxied bool
}

// CreateDNSRecord creates a new DNS record.
func (c *API) CreateDNSRecord(ctx context.Context, params DNSRecordParams) (*DNSRecordResult, error) {
	if _, err := c.GetZoneId(ctx); err != nil {
		c.Log.Error(err, "error getting zone ID")
		return nil, err
	}

	rc := cloudflare.ZoneIdentifier(c.ValidZoneId)

	createParams := cloudflare.CreateDNSRecordParams{
		Name:    params.Name,
		Type:    params.Type,
		Content: params.Content,
		TTL:     params.TTL,
		Comment: params.Comment,
	}

	if params.Type == DNSRecordTypeA || params.Type == DNSRecordTypeAAAA || params.Type == DNSRecordTypeCNAME {
		createParams.Proxied = &params.Proxied
	}

	if params.Priority != nil {
		priority := uint16(*params.Priority)
		createParams.Priority = &priority
	}

	if params.Data != nil {
		createParams.Data = convertDataToMap(params.Data)
	}

	record, err := c.CloudflareClient.CreateDNSRecord(ctx, rc, createParams)
	if err != nil {
		c.Log.Error(err, "error creating DNS record", "name", params.Name)
		return nil, err
	}

	c.Log.Info("DNS Record created", "id", record.ID, "name", record.Name)

	proxied := false
	if record.Proxied != nil {
		proxied = *record.Proxied
	}

	return &DNSRecordResult{
		ID:      record.ID,
		ZoneID:  c.ValidZoneId,
		Name:    record.Name,
		Type:    record.Type,
		Content: record.Content,
		TTL:     record.TTL,
		Proxied: proxied,
	}, nil
}

// GetDNSRecord retrieves a DNS record by ID.
func (c *API) GetDNSRecord(ctx context.Context, zoneID, recordID string) (*DNSRecordResult, error) {
	rc := cloudflare.ZoneIdentifier(zoneID)

	record, err := c.CloudflareClient.GetDNSRecord(ctx, rc, recordID)
	if err != nil {
		c.Log.Error(err, "error getting DNS record", "id", recordID)
		return nil, err
	}

	proxied := false
	if record.Proxied != nil {
		proxied = *record.Proxied
	}

	return &DNSRecordResult{
		ID:      record.ID,
		ZoneID:  zoneID,
		Name:    record.Name,
		Type:    record.Type,
		Content: record.Content,
		TTL:     record.TTL,
		Proxied: proxied,
	}, nil
}

// UpdateDNSRecord updates an existing DNS record.
func (c *API) UpdateDNSRecord(ctx context.Context, zoneID, recordID string, params DNSRecordParams) (*DNSRecordResult, error) {
	rc := cloudflare.ZoneIdentifier(zoneID)

	updateParams := cloudflare.UpdateDNSRecordParams{
		ID:      recordID,
		Name:    params.Name,
		Type:    params.Type,
		Content: params.Content,
		TTL:     params.TTL,
		Comment: &params.Comment,
	}

	if params.Type == DNSRecordTypeA || params.Type == DNSRecordTypeAAAA || params.Type == DNSRecordTypeCNAME {
		updateParams.Proxied = &params.Proxied
	}

	if params.Priority != nil {
		priority := uint16(*params.Priority)
		updateParams.Priority = &priority
	}

	if params.Data != nil {
		updateParams.Data = convertDataToMap(params.Data)
	}

	record, err := c.CloudflareClient.UpdateDNSRecord(ctx, rc, updateParams)
	if err != nil {
		c.Log.Error(err, "error updating DNS record", "id", recordID)
		return nil, err
	}

	c.Log.Info("DNS Record updated", "id", record.ID, "name", record.Name)

	proxied := false
	if record.Proxied != nil {
		proxied = *record.Proxied
	}

	return &DNSRecordResult{
		ID:      record.ID,
		ZoneID:  zoneID,
		Name:    record.Name,
		Type:    record.Type,
		Content: record.Content,
		TTL:     record.TTL,
		Proxied: proxied,
	}, nil
}

// DeleteDNSRecord deletes a DNS record.
// This method is idempotent - returns nil if the record is already deleted.
func (c *API) DeleteDNSRecord(ctx context.Context, zoneID, recordID string) error {
	rc := cloudflare.ZoneIdentifier(zoneID)

	err := c.CloudflareClient.DeleteDNSRecord(ctx, rc, recordID)
	if err != nil {
		if IsNotFoundError(err) {
			c.Log.Info("DNS Record already deleted (not found)", "id", recordID)
			return nil
		}
		c.Log.Error(err, "error deleting DNS record", "id", recordID)
		return err
	}

	c.Log.Info("DNS Record deleted", "id", recordID)
	return nil
}

// InZone DNS Operations
// These methods allow specifying Zone ID directly instead of relying on c.ValidZoneId.
// This enables multi-zone support via DomainResolver.

// GetDNSCNameIDInZone returns the ID of the CNAME record for the given fqdn in the specified zone.
// Returns empty string and nil error if the record does not exist (this is not an error condition).
// Returns empty string and error if there was an actual API error or multiple records found.
func (c *API) GetDNSCNameIDInZone(ctx context.Context, zoneID, fqdn string) (string, error) {
	rc := cloudflare.ZoneIdentifier(zoneID)
	params := cloudflare.ListDNSRecordsParams{
		Type: "CNAME",
		Name: fqdn,
	}
	records, _, err := c.CloudflareClient.ListDNSRecords(ctx, rc, params)
	if err != nil {
		c.Log.Error(err, "error listing DNS records in zone", "zoneId", zoneID, "fqdn", fqdn)
		return "", err
	}

	switch len(records) {
	case 0:
		c.Log.V(1).Info("no DNS record found for fqdn in zone", "zoneId", zoneID, "fqdn", fqdn)
		return "", nil
	case 1:
		return records[0].ID, nil
	default:
		err := ErrMultipleResourcesFound
		c.Log.Error(err, "multiple CNAME records found for fqdn in zone", "zoneId", zoneID, "fqdn", fqdn, "count", len(records))
		return "", err
	}
}

// GetDNSRecordIDInZone returns the ID of a DNS record of the given type for the fqdn in the specified zone.
// Returns empty string and nil error if the record does not exist.
func (c *API) GetDNSRecordIDInZone(ctx context.Context, zoneID, fqdn, recordType string) (string, error) {
	rc := cloudflare.ZoneIdentifier(zoneID)
	params := cloudflare.ListDNSRecordsParams{
		Type: recordType,
		Name: fqdn,
	}
	records, _, err := c.CloudflareClient.ListDNSRecords(ctx, rc, params)
	if err != nil {
		c.Log.Error(err, "error listing DNS records in zone", "zoneId", zoneID, "fqdn", fqdn, "type", recordType)
		return "", err
	}

	switch len(records) {
	case 0:
		c.Log.V(1).Info("no DNS record found for fqdn in zone", "zoneId", zoneID, "fqdn", fqdn, "type", recordType)
		return "", nil
	case 1:
		return records[0].ID, nil
	default:
		err := ErrMultipleResourcesFound
		c.Log.Error(err, "multiple records found for fqdn in zone", "zoneId", zoneID, "fqdn", fqdn, "type", recordType, "count", len(records))
		return "", err
	}
}

// CreateDNSRecordInZone creates a new DNS record in the specified zone.
func (c *API) CreateDNSRecordInZone(ctx context.Context, zoneID string, params DNSRecordParams) (*DNSRecordResult, error) {
	rc := cloudflare.ZoneIdentifier(zoneID)

	createParams := cloudflare.CreateDNSRecordParams{
		Name:    params.Name,
		Type:    params.Type,
		Content: params.Content,
		TTL:     params.TTL,
		Comment: params.Comment,
	}

	if params.Type == DNSRecordTypeA || params.Type == DNSRecordTypeAAAA || params.Type == DNSRecordTypeCNAME {
		createParams.Proxied = &params.Proxied
	}

	if params.Priority != nil {
		priority := uint16(*params.Priority)
		createParams.Priority = &priority
	}

	if params.Data != nil {
		createParams.Data = convertDataToMap(params.Data)
	}

	if len(params.Tags) > 0 {
		createParams.Tags = params.Tags
	}

	record, err := c.CloudflareClient.CreateDNSRecord(ctx, rc, createParams)
	if err != nil {
		c.Log.Error(err, "error creating DNS record in zone", "zoneId", zoneID, "name", params.Name)
		return nil, err
	}

	c.Log.Info("DNS Record created in zone", "zoneId", zoneID, "id", record.ID, "name", record.Name)

	proxied := false
	if record.Proxied != nil {
		proxied = *record.Proxied
	}

	return &DNSRecordResult{
		ID:      record.ID,
		ZoneID:  zoneID,
		Name:    record.Name,
		Type:    record.Type,
		Content: record.Content,
		TTL:     record.TTL,
		Proxied: proxied,
	}, nil
}

// UpdateDNSRecordInZone updates an existing DNS record in the specified zone.
func (c *API) UpdateDNSRecordInZone(ctx context.Context, zoneID, recordID string, params DNSRecordParams) (*DNSRecordResult, error) {
	rc := cloudflare.ZoneIdentifier(zoneID)

	updateParams := cloudflare.UpdateDNSRecordParams{
		ID:      recordID,
		Name:    params.Name,
		Type:    params.Type,
		Content: params.Content,
		TTL:     params.TTL,
		Comment: &params.Comment,
	}

	if params.Type == DNSRecordTypeA || params.Type == DNSRecordTypeAAAA || params.Type == DNSRecordTypeCNAME {
		updateParams.Proxied = &params.Proxied
	}

	if params.Priority != nil {
		priority := uint16(*params.Priority)
		updateParams.Priority = &priority
	}

	if params.Data != nil {
		updateParams.Data = convertDataToMap(params.Data)
	}

	if len(params.Tags) > 0 {
		updateParams.Tags = params.Tags
	}

	record, err := c.CloudflareClient.UpdateDNSRecord(ctx, rc, updateParams)
	if err != nil {
		c.Log.Error(err, "error updating DNS record in zone", "zoneId", zoneID, "id", recordID)
		return nil, err
	}

	c.Log.Info("DNS Record updated in zone", "zoneId", zoneID, "id", record.ID, "name", record.Name)

	proxied := false
	if record.Proxied != nil {
		proxied = *record.Proxied
	}

	return &DNSRecordResult{
		ID:      record.ID,
		ZoneID:  zoneID,
		Name:    record.Name,
		Type:    record.Type,
		Content: record.Content,
		TTL:     record.TTL,
		Proxied: proxied,
	}, nil
}

// DeleteDNSRecordInZone deletes a DNS record in the specified zone.
// This method is idempotent - returns nil if the record is already deleted.
func (c *API) DeleteDNSRecordInZone(ctx context.Context, zoneID, recordID string) error {
	rc := cloudflare.ZoneIdentifier(zoneID)

	err := c.CloudflareClient.DeleteDNSRecord(ctx, rc, recordID)
	if err != nil {
		if IsNotFoundError(err) {
			c.Log.Info("DNS Record already deleted (not found)", "zoneId", zoneID, "id", recordID)
			return nil
		}
		c.Log.Error(err, "error deleting DNS record in zone", "zoneId", zoneID, "id", recordID)
		return err
	}

	c.Log.Info("DNS Record deleted from zone", "zoneId", zoneID, "id", recordID)
	return nil
}

// InsertOrUpdateCNameInZone upserts DNS CNAME record for the given FQDN to point to the tunnel in the specified zone.
// If tunnelID is empty, it uses c.ValidTunnelId.
func (c *API) InsertOrUpdateCNameInZone(ctx context.Context, zoneID, fqdn, dnsID, tunnelID string, proxied bool) (string, error) {
	if tunnelID == "" {
		tunnelID = c.ValidTunnelId
	}
	if tunnelID == "" {
		return "", ErrInvalidTunnelID
	}

	rc := cloudflare.ZoneIdentifier(zoneID)
	target := tunnelID + ".cfargotunnel.com"

	if dnsID != "" {
		c.Log.Info("Updating existing CNAME record in zone", "zoneId", zoneID, "fqdn", fqdn, "dnsID", dnsID)
		updateParams := cloudflare.UpdateDNSRecordParams{
			ID:      dnsID,
			Type:    "CNAME",
			Name:    fqdn,
			Content: target,
			Comment: stringPtr("Managed by cloudflare-operator"),
			TTL:     1,
			Proxied: &proxied,
		}
		_, err := c.CloudflareClient.UpdateDNSRecord(ctx, rc, updateParams)
		if err != nil {
			c.Log.Error(err, "error updating DNS record in zone", "zoneId", zoneID, "fqdn", fqdn)
			return "", err
		}
		c.Log.Info("DNS record updated in zone", "zoneId", zoneID, "fqdn", fqdn)
		return dnsID, nil
	}

	c.Log.Info("Creating CNAME record in zone", "zoneId", zoneID, "fqdn", fqdn)
	createParams := cloudflare.CreateDNSRecordParams{
		Type:    "CNAME",
		Name:    fqdn,
		Content: target,
		Comment: "Managed by cloudflare-operator",
		TTL:     1,
		Proxied: &proxied,
	}
	resp, err := c.CloudflareClient.CreateDNSRecord(ctx, rc, createParams)
	if err != nil {
		c.Log.Error(err, "error creating DNS record in zone", "zoneId", zoneID, "fqdn", fqdn)
		return "", err
	}
	c.Log.Info("DNS record created in zone", "zoneId", zoneID, "fqdn", fqdn)
	return resp.ID, nil
}

// stringPtr returns a pointer to a string
func stringPtr(s string) *string {
	return &s
}
