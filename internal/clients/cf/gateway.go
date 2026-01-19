// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"context"
	"time"

	"github.com/cloudflare/cloudflare-go"
)

// parseDuration parses a duration string like "5m", "1h", "24h".
func parseDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

// GatewayRuleParams contains parameters for a Gateway Rule.
type GatewayRuleParams struct {
	Name          string
	Description   string
	Precedence    int
	Enabled       bool
	Action        string
	Filters       []cloudflare.TeamsFilterType
	Traffic       string
	Identity      string
	DevicePosture string
	RuleSettings  *GatewayRuleSettingsParams
	Schedule      *GatewayRuleScheduleParams
	Expiration    *GatewayRuleExpirationParams
}

// GatewayRuleSettingsParams contains settings for a Gateway Rule.
type GatewayRuleSettingsParams struct {
	BlockPageEnabled                *bool
	BlockReason                     string
	OverrideIPs                     []string
	OverrideHost                    string
	L4Override                      *GatewayL4OverrideParams
	BISOAdminControls               *GatewayBISOAdminControlsParams
	CheckSession                    *GatewayCheckSessionParams
	AddHeaders                      map[string]string
	InsecureDisableDNSSECValidation *bool
	Egress                          *GatewayEgressParams
	PayloadLog                      *GatewayPayloadLogParams
	UntrustedCertAction             string
	AuditSSH                        *GatewayAuditSSHParams
	ResolveDNSInternally            *GatewayResolveDNSInternallyParams
	ResolveDNSThroughCloudflare     *bool
	DNSResolvers                    *GatewayDNSResolversParams
	NotificationSettings            *GatewayNotificationSettingsParams
	AllowChildBypass                *bool
	BypassParentRule                *bool
	IgnoreCNAMECategoryMatches      *bool
	IPCategories                    *bool
	IPIndicatorFeeds                *bool
	Quarantine                      *GatewayQuarantineParams
}

// GatewayL4OverrideParams for L4 override settings.
type GatewayL4OverrideParams struct {
	IP   string
	Port int
}

// GatewayBISOAdminControlsParams for browser isolation controls.
type GatewayBISOAdminControlsParams struct {
	DisablePrinting             *bool
	DisableCopyPaste            *bool
	DisableDownload             *bool
	DisableUpload               *bool
	DisableKeyboard             *bool
	DisableClipboardRedirection *bool
}

// GatewayCheckSessionParams for session check settings.
type GatewayCheckSessionParams struct {
	Enforce  bool
	Duration string
}

// GatewayEgressParams for egress settings.
type GatewayEgressParams struct {
	IPv4         string
	IPv6         string
	IPv4Fallback string
}

// GatewayPayloadLogParams for payload logging.
type GatewayPayloadLogParams struct {
	Enabled bool
}

// GatewayAuditSSHParams for SSH audit settings.
type GatewayAuditSSHParams struct {
	CommandLogging bool
}

// GatewayResolveDNSInternallyParams for internal DNS resolution.
type GatewayResolveDNSInternallyParams struct {
	ViewID   string
	Fallback string // "none", "public_dns", etc.
}

// GatewayDNSResolversParams for custom DNS resolvers.
type GatewayDNSResolversParams struct {
	IPv4 []GatewayDNSResolverEntryParams
	IPv6 []GatewayDNSResolverEntryParams
}

// GatewayDNSResolverEntryParams for a single DNS resolver.
type GatewayDNSResolverEntryParams struct {
	IP                         string
	Port                       int
	VNetID                     string
	RouteThroughPrivateNetwork *bool
}

// GatewayNotificationSettingsParams for notification settings.
type GatewayNotificationSettingsParams struct {
	Enabled    bool
	Message    string
	SupportURL string
}

// GatewayQuarantineParams for quarantine settings.
type GatewayQuarantineParams struct {
	FileTypes []string
}

// GatewayRuleScheduleParams for rule scheduling.
type GatewayRuleScheduleParams struct {
	TimeZone string
	Mon      string
	Tue      string
	Wed      string
	Thu      string
	Fri      string
	Sat      string
	Sun      string
}

// GatewayRuleExpirationParams for rule expiration.
type GatewayRuleExpirationParams struct {
	ExpiresAt string
	Duration  string
}

// GatewayRuleResult contains the result of a Gateway Rule operation.
type GatewayRuleResult struct {
	ID          string
	Name        string
	Description string
	Precedence  int
	Enabled     bool
	Action      string
}

// convertRuleSettingsToSDK converts GatewayRuleSettingsParams to cloudflare.TeamsRuleSettings.
//
//nolint:gocyclo,revive // cyclomatic/cognitive complexity is acceptable for this conversion
func convertRuleSettingsToSDK(params *GatewayRuleSettingsParams) cloudflare.TeamsRuleSettings {
	if params == nil {
		return cloudflare.TeamsRuleSettings{}
	}

	settings := cloudflare.TeamsRuleSettings{
		BlockReason:  params.BlockReason,
		OverrideIPs:  params.OverrideIPs,
		OverrideHost: params.OverrideHost,
	}

	if params.BlockPageEnabled != nil {
		settings.BlockPageEnabled = *params.BlockPageEnabled
	}
	if params.InsecureDisableDNSSECValidation != nil {
		settings.InsecureDisableDNSSECValidation = *params.InsecureDisableDNSSECValidation
	}
	if params.L4Override != nil {
		settings.L4Override = &cloudflare.TeamsL4OverrideSettings{
			IP:   params.L4Override.IP,
			Port: params.L4Override.Port,
		}
	}
	if params.BISOAdminControls != nil {
		biso := &cloudflare.TeamsBISOAdminControlSettings{}
		if params.BISOAdminControls.DisablePrinting != nil {
			biso.DisablePrinting = *params.BISOAdminControls.DisablePrinting
		}
		if params.BISOAdminControls.DisableCopyPaste != nil {
			biso.DisableCopyPaste = *params.BISOAdminControls.DisableCopyPaste
		}
		if params.BISOAdminControls.DisableDownload != nil {
			biso.DisableDownload = *params.BISOAdminControls.DisableDownload
		}
		if params.BISOAdminControls.DisableUpload != nil {
			biso.DisableUpload = *params.BISOAdminControls.DisableUpload
		}
		if params.BISOAdminControls.DisableKeyboard != nil {
			biso.DisableKeyboard = *params.BISOAdminControls.DisableKeyboard
		}
		if params.BISOAdminControls.DisableClipboardRedirection != nil {
			biso.DisableClipboardRedirection = *params.BISOAdminControls.DisableClipboardRedirection
		}
		settings.BISOAdminControls = biso
	}
	if params.CheckSession != nil {
		settings.CheckSession = &cloudflare.TeamsCheckSessionSettings{
			Enforce:  params.CheckSession.Enforce,
			Duration: cloudflare.Duration{Duration: parseDuration(params.CheckSession.Duration)},
		}
	}
	if params.AddHeaders != nil {
		settings.AddHeaders = make(map[string][]string)
		for k, v := range params.AddHeaders {
			settings.AddHeaders[k] = []string{v}
		}
	}
	if params.Egress != nil {
		settings.EgressSettings = &cloudflare.EgressSettings{
			Ipv4:         params.Egress.IPv4,
			Ipv6Range:    params.Egress.IPv6,
			Ipv4Fallback: params.Egress.IPv4Fallback,
		}
	}
	if params.PayloadLog != nil {
		settings.PayloadLog = &cloudflare.TeamsDlpPayloadLogSettings{
			Enabled: params.PayloadLog.Enabled,
		}
	}
	if params.UntrustedCertAction != "" {
		settings.UntrustedCertSettings = &cloudflare.UntrustedCertSettings{
			Action: cloudflare.TeamsGatewayUntrustedCertAction(params.UntrustedCertAction),
		}
	}
	if params.AuditSSH != nil {
		settings.AuditSSH = &cloudflare.AuditSSHRuleSettings{
			CommandLogging: params.AuditSSH.CommandLogging,
		}
	}
	if params.NotificationSettings != nil {
		settings.NotificationSettings = &cloudflare.TeamsNotificationSettings{
			Enabled:    &params.NotificationSettings.Enabled,
			Message:    params.NotificationSettings.Message,
			SupportURL: params.NotificationSettings.SupportURL,
		}
	}
	if params.AllowChildBypass != nil {
		settings.AllowChildBypass = params.AllowChildBypass
	}
	if params.BypassParentRule != nil {
		settings.BypassParentRule = params.BypassParentRule
	}
	if params.IgnoreCNAMECategoryMatches != nil {
		settings.IgnoreCNAMECategoryMatches = params.IgnoreCNAMECategoryMatches
	}
	if params.IPCategories != nil {
		settings.IPCategories = *params.IPCategories
	}
	// Note: IPIndicatorFeeds is not supported in cloudflare-go SDK v0.116.0
	// The field exists in CRD but cannot be passed to API until SDK is updated
	if params.ResolveDNSThroughCloudflare != nil {
		settings.ResolveDnsThroughCloudflare = params.ResolveDNSThroughCloudflare
	}
	if params.Quarantine != nil {
		settings.Quarantine = &cloudflare.TeamsQuarantine{
			FileTypes: params.Quarantine.FileTypes,
		}
	}
	if params.DNSResolvers != nil {
		settings.DnsResolverSettings = &cloudflare.TeamsDnsResolverSettings{}
		if len(params.DNSResolvers.IPv4) > 0 {
			for _, r := range params.DNSResolvers.IPv4 {
				addr := cloudflare.TeamsDnsResolverAddress{
					IP:                         r.IP,
					VnetID:                     r.VNetID,
					RouteThroughPrivateNetwork: r.RouteThroughPrivateNetwork,
				}
				if r.Port != 0 {
					port := r.Port
					addr.Port = &port
				}
				settings.DnsResolverSettings.V4Resolvers = append(settings.DnsResolverSettings.V4Resolvers,
					cloudflare.TeamsDnsResolverAddressV4{TeamsDnsResolverAddress: addr})
			}
		}
		if len(params.DNSResolvers.IPv6) > 0 {
			for _, r := range params.DNSResolvers.IPv6 {
				addr := cloudflare.TeamsDnsResolverAddress{
					IP:                         r.IP,
					VnetID:                     r.VNetID,
					RouteThroughPrivateNetwork: r.RouteThroughPrivateNetwork,
				}
				if r.Port != 0 {
					port := r.Port
					addr.Port = &port
				}
				settings.DnsResolverSettings.V6Resolvers = append(settings.DnsResolverSettings.V6Resolvers,
					cloudflare.TeamsDnsResolverAddressV6{TeamsDnsResolverAddress: addr})
			}
		}
	}
	if params.ResolveDNSInternally != nil {
		settings.ResolveDnsInternallySettings = &cloudflare.TeamsResolveDnsInternallySettings{
			ViewID:   params.ResolveDNSInternally.ViewID,
			Fallback: cloudflare.TeamsResolveDnsInternallyFallbackStrategy(params.ResolveDNSInternally.Fallback),
		}
	}

	return settings
}

// convertScheduleToSDK converts GatewayRuleScheduleParams to cloudflare.TeamsRuleSchedule.
func convertScheduleToSDK(params *GatewayRuleScheduleParams) *cloudflare.TeamsRuleSchedule {
	if params == nil {
		return nil
	}
	return &cloudflare.TeamsRuleSchedule{
		TimeZone:  params.TimeZone,
		Monday:    cloudflare.TeamsScheduleTimes(params.Mon),
		Tuesday:   cloudflare.TeamsScheduleTimes(params.Tue),
		Wednesday: cloudflare.TeamsScheduleTimes(params.Wed),
		Thursday:  cloudflare.TeamsScheduleTimes(params.Thu),
		Friday:    cloudflare.TeamsScheduleTimes(params.Fri),
		Saturday:  cloudflare.TeamsScheduleTimes(params.Sat),
		Sunday:    cloudflare.TeamsScheduleTimes(params.Sun),
	}
}

// convertExpirationToSDK converts GatewayRuleExpirationParams to cloudflare.TeamsRuleExpiration.
func convertExpirationToSDK(params *GatewayRuleExpirationParams) *cloudflare.TeamsRuleExpiration {
	if params == nil {
		return nil
	}
	result := &cloudflare.TeamsRuleExpiration{}
	if params.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, params.ExpiresAt)
		if err == nil {
			result.ExpiresAt = &t
		}
	}
	return result
}

// CreateGatewayRule creates a new Gateway Rule.
func (c *API) CreateGatewayRule(ctx context.Context, params GatewayRuleParams) (*GatewayRuleResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rule := cloudflare.TeamsRule{
		Name:          params.Name,
		Description:   params.Description,
		Precedence:    uint64(params.Precedence),
		Enabled:       params.Enabled,
		Action:        cloudflare.TeamsGatewayAction(params.Action),
		Filters:       params.Filters,
		Traffic:       params.Traffic,
		Identity:      params.Identity,
		DevicePosture: params.DevicePosture,
		RuleSettings:  convertRuleSettingsToSDK(params.RuleSettings),
		Schedule:      convertScheduleToSDK(params.Schedule),
		Expiration:    convertExpirationToSDK(params.Expiration),
	}

	result, err := c.CloudflareClient.TeamsCreateRule(ctx, c.ValidAccountId, rule)
	if err != nil {
		c.Log.Error(err, "error creating gateway rule", "name", params.Name)
		return nil, err
	}

	c.Log.Info("Gateway Rule created", "id", result.ID, "name", result.Name)

	return &GatewayRuleResult{
		ID:          result.ID,
		Name:        result.Name,
		Description: result.Description,
		Precedence:  int(result.Precedence),
		Enabled:     result.Enabled,
		Action:      string(result.Action),
	}, nil
}

// GetGatewayRule retrieves a Gateway Rule by ID.
func (c *API) GetGatewayRule(ctx context.Context, ruleID string) (*GatewayRuleResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rule, err := c.CloudflareClient.TeamsRule(ctx, c.ValidAccountId, ruleID)
	if err != nil {
		c.Log.Error(err, "error getting gateway rule", "id", ruleID)
		return nil, err
	}

	return &GatewayRuleResult{
		ID:          rule.ID,
		Name:        rule.Name,
		Description: rule.Description,
		Precedence:  int(rule.Precedence),
		Enabled:     rule.Enabled,
		Action:      string(rule.Action),
	}, nil
}

// UpdateGatewayRule updates an existing Gateway Rule.
func (c *API) UpdateGatewayRule(ctx context.Context, ruleID string, params GatewayRuleParams) (*GatewayRuleResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rule := cloudflare.TeamsRule{
		ID:            ruleID,
		Name:          params.Name,
		Description:   params.Description,
		Precedence:    uint64(params.Precedence),
		Enabled:       params.Enabled,
		Action:        cloudflare.TeamsGatewayAction(params.Action),
		Filters:       params.Filters,
		Traffic:       params.Traffic,
		Identity:      params.Identity,
		DevicePosture: params.DevicePosture,
		RuleSettings:  convertRuleSettingsToSDK(params.RuleSettings),
		Schedule:      convertScheduleToSDK(params.Schedule),
		Expiration:    convertExpirationToSDK(params.Expiration),
	}

	result, err := c.CloudflareClient.TeamsUpdateRule(ctx, c.ValidAccountId, ruleID, rule)
	if err != nil {
		c.Log.Error(err, "error updating gateway rule", "id", ruleID)
		return nil, err
	}

	c.Log.Info("Gateway Rule updated", "id", result.ID, "name", result.Name)

	return &GatewayRuleResult{
		ID:          result.ID,
		Name:        result.Name,
		Description: result.Description,
		Precedence:  int(result.Precedence),
		Enabled:     result.Enabled,
		Action:      string(result.Action),
	}, nil
}

// DeleteGatewayRule deletes a Gateway Rule.
// This method is idempotent - returns nil if the rule is already deleted.
func (c *API) DeleteGatewayRule(ctx context.Context, ruleID string) error {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	err := c.CloudflareClient.TeamsDeleteRule(ctx, c.ValidAccountId, ruleID)
	if err != nil {
		if IsNotFoundError(err) {
			c.Log.Info("Gateway Rule already deleted (not found)", "id", ruleID)
			return nil
		}
		c.Log.Error(err, "error deleting gateway rule", "id", ruleID)
		return err
	}

	c.Log.Info("Gateway Rule deleted", "id", ruleID)
	return nil
}

// GatewayListParams contains parameters for a Gateway List.
type GatewayListParams struct {
	Name        string
	Description string
	Type        string // SERIAL, URL, DOMAIN, EMAIL, IP
	Items       []GatewayListItem
}

// GatewayListItem represents an item in a Gateway List.
type GatewayListItem struct {
	Value       string
	Description string
}

// GatewayListResult contains the result of a Gateway List operation.
type GatewayListResult struct {
	ID          string
	Name        string
	Description string
	Type        string
	Count       int
	AccountID   string
}

// CreateGatewayList creates a new Gateway List.
func (c *API) CreateGatewayList(ctx context.Context, params GatewayListParams) (*GatewayListResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	// Convert items to TeamsListItems with descriptions
	items := make([]cloudflare.TeamsListItem, len(params.Items))
	for i, item := range params.Items {
		items[i] = cloudflare.TeamsListItem{
			Value:       item.Value,
			Description: item.Description,
		}
	}

	createParams := cloudflare.CreateTeamsListParams{
		Name:        params.Name,
		Description: params.Description,
		Type:        params.Type,
		Items:       items,
	}

	result, err := c.CloudflareClient.CreateTeamsList(ctx, cloudflare.AccountIdentifier(c.ValidAccountId), createParams)
	if err != nil {
		c.Log.Error(err, "error creating gateway list", "name", params.Name)
		return nil, err
	}

	c.Log.Info("Gateway List created", "id", result.ID, "name", result.Name)

	return &GatewayListResult{
		ID:          result.ID,
		Name:        result.Name,
		Description: result.Description,
		Type:        result.Type,
		Count:       int(result.Count),
		AccountID:   c.ValidAccountId,
	}, nil
}

// GetGatewayList retrieves a Gateway List by ID.
func (c *API) GetGatewayList(ctx context.Context, listID string) (*GatewayListResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	list, err := c.CloudflareClient.GetTeamsList(ctx, cloudflare.AccountIdentifier(c.ValidAccountId), listID)
	if err != nil {
		c.Log.Error(err, "error getting gateway list", "id", listID)
		return nil, err
	}

	return &GatewayListResult{
		ID:          list.ID,
		Name:        list.Name,
		Description: list.Description,
		Type:        list.Type,
		Count:       int(list.Count),
		AccountID:   c.ValidAccountId,
	}, nil
}

// UpdateGatewayList updates an existing Gateway List.
func (c *API) UpdateGatewayList(ctx context.Context, listID string, params GatewayListParams) (*GatewayListResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	updateParams := cloudflare.UpdateTeamsListParams{
		ID:          listID,
		Name:        params.Name,
		Description: params.Description,
	}

	result, err := c.CloudflareClient.UpdateTeamsList(ctx, cloudflare.AccountIdentifier(c.ValidAccountId), updateParams)
	if err != nil {
		c.Log.Error(err, "error updating gateway list", "id", listID)
		return nil, err
	}

	c.Log.Info("Gateway List updated", "id", result.ID, "name", result.Name)

	return &GatewayListResult{
		ID:          result.ID,
		Name:        result.Name,
		Description: result.Description,
		Type:        result.Type,
		Count:       int(result.Count),
		AccountID:   c.ValidAccountId,
	}, nil
}

// DeleteGatewayList deletes a Gateway List.
// This method is idempotent - returns nil if the list is already deleted.
func (c *API) DeleteGatewayList(ctx context.Context, listID string) error {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	err := c.CloudflareClient.DeleteTeamsList(ctx, cloudflare.AccountIdentifier(c.ValidAccountId), listID)
	if err != nil {
		if IsNotFoundError(err) {
			c.Log.Info("Gateway List already deleted (not found)", "id", listID)
			return nil
		}
		c.Log.Error(err, "error deleting gateway list", "id", listID)
		return err
	}

	c.Log.Info("Gateway List deleted", "id", listID)
	return nil
}

// ListGatewayRulesByName finds a Gateway Rule by name.
// Returns nil if no rule with the given name is found.
func (c *API) ListGatewayRulesByName(ctx context.Context, name string) (*GatewayRuleResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rules, err := c.CloudflareClient.TeamsRules(ctx, c.ValidAccountId)
	if err != nil {
		c.Log.Error(err, "error listing gateway rules")
		return nil, err
	}

	for _, rule := range rules {
		if rule.Name == name {
			return &GatewayRuleResult{
				ID:          rule.ID,
				Name:        rule.Name,
				Description: rule.Description,
				Precedence:  int(rule.Precedence),
				Enabled:     rule.Enabled,
				Action:      string(rule.Action),
			}, nil
		}
	}

	return nil, nil // Not found, return nil without error
}

// ListGatewayListsByName finds a Gateway List by name.
// Returns nil if no list with the given name is found.
func (c *API) ListGatewayListsByName(ctx context.Context, name string) (*GatewayListResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	lists, _, err := c.CloudflareClient.ListTeamsLists(ctx, cloudflare.AccountIdentifier(c.ValidAccountId), cloudflare.ListTeamListsParams{})
	if err != nil {
		c.Log.Error(err, "error listing gateway lists")
		return nil, err
	}

	for _, list := range lists {
		if list.Name == name {
			return &GatewayListResult{
				ID:          list.ID,
				Name:        list.Name,
				Description: list.Description,
				Type:        list.Type,
				Count:       int(list.Count),
				AccountID:   c.ValidAccountId,
			}, nil
		}
	}

	return nil, nil // Not found, return nil without error
}
