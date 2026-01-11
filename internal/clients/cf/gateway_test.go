// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"testing"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertRuleSettingsToSDK(t *testing.T) {
	tests := []struct {
		name     string
		params   *GatewayRuleSettingsParams
		validate func(t *testing.T, result cloudflare.TeamsRuleSettings)
	}{
		{
			name:   "nil params",
			params: nil,
			validate: func(t *testing.T, result cloudflare.TeamsRuleSettings) {
				// nil params should return empty struct
				assert.False(t, result.BlockPageEnabled)
			},
		},
		{
			name: "block page settings",
			params: &GatewayRuleSettingsParams{
				BlockPageEnabled: boolPtrGateway(true),
				BlockReason:      "Policy violation",
			},
			validate: func(t *testing.T, result cloudflare.TeamsRuleSettings) {
				assert.True(t, result.BlockPageEnabled)
				assert.Equal(t, "Policy violation", result.BlockReason)
			},
		},
		{
			name: "override settings",
			params: &GatewayRuleSettingsParams{
				OverrideIPs:  []string{"1.1.1.1", "8.8.8.8"},
				OverrideHost: "override.example.com",
			},
			validate: func(t *testing.T, result cloudflare.TeamsRuleSettings) {
				assert.Equal(t, []string{"1.1.1.1", "8.8.8.8"}, result.OverrideIPs)
				assert.Equal(t, "override.example.com", result.OverrideHost)
			},
		},
		{
			name: "L4 override",
			params: &GatewayRuleSettingsParams{
				L4Override: &GatewayL4OverrideParams{
					IP:   "10.0.0.1",
					Port: 8080,
				},
			},
			validate: func(t *testing.T, result cloudflare.TeamsRuleSettings) {
				require.NotNil(t, result.L4Override)
				assert.Equal(t, "10.0.0.1", result.L4Override.IP)
				assert.Equal(t, 8080, result.L4Override.Port)
			},
		},
		{
			name: "BISO admin controls",
			params: &GatewayRuleSettingsParams{
				BISOAdminControls: &GatewayBISOAdminControlsParams{
					DisablePrinting:             boolPtrGateway(true),
					DisableCopyPaste:            boolPtrGateway(true),
					DisableDownload:             boolPtrGateway(true),
					DisableUpload:               boolPtrGateway(true),
					DisableKeyboard:             boolPtrGateway(true),
					DisableClipboardRedirection: boolPtrGateway(true),
				},
			},
			validate: func(t *testing.T, result cloudflare.TeamsRuleSettings) {
				require.NotNil(t, result.BISOAdminControls)
				assert.True(t, result.BISOAdminControls.DisablePrinting)
				assert.True(t, result.BISOAdminControls.DisableCopyPaste)
				assert.True(t, result.BISOAdminControls.DisableDownload)
				assert.True(t, result.BISOAdminControls.DisableUpload)
				assert.True(t, result.BISOAdminControls.DisableKeyboard)
				assert.True(t, result.BISOAdminControls.DisableClipboardRedirection)
			},
		},
		{
			name: "check session",
			params: &GatewayRuleSettingsParams{
				CheckSession: &GatewayCheckSessionParams{
					Enforce:  true,
					Duration: "1h",
				},
			},
			validate: func(t *testing.T, result cloudflare.TeamsRuleSettings) {
				require.NotNil(t, result.CheckSession)
				assert.True(t, result.CheckSession.Enforce)
				assert.Equal(t, time.Hour, result.CheckSession.Duration.Duration)
			},
		},
		{
			name: "add headers",
			params: &GatewayRuleSettingsParams{
				AddHeaders: map[string]string{
					"X-Custom-Header": "value1",
					"X-Another":       "value2",
				},
			},
			validate: func(t *testing.T, result cloudflare.TeamsRuleSettings) {
				require.NotNil(t, result.AddHeaders)
				assert.Contains(t, result.AddHeaders.Get("X-Custom-Header"), "value1")
				assert.Contains(t, result.AddHeaders.Get("X-Another"), "value2")
			},
		},
		{
			name: "DNSSEC validation disabled",
			params: &GatewayRuleSettingsParams{
				InsecureDisableDNSSECValidation: boolPtrGateway(true),
			},
			validate: func(t *testing.T, result cloudflare.TeamsRuleSettings) {
				assert.True(t, result.InsecureDisableDNSSECValidation)
			},
		},
		{
			name: "egress settings",
			params: &GatewayRuleSettingsParams{
				Egress: &GatewayEgressParams{
					IPv4:         "203.0.113.1",
					IPv6:         "2001:db8::1/32",
					IPv4Fallback: "192.0.2.1",
				},
			},
			validate: func(t *testing.T, result cloudflare.TeamsRuleSettings) {
				require.NotNil(t, result.EgressSettings)
				assert.Equal(t, "203.0.113.1", result.EgressSettings.Ipv4)
				assert.Equal(t, "2001:db8::1/32", result.EgressSettings.Ipv6Range)
				assert.Equal(t, "192.0.2.1", result.EgressSettings.Ipv4Fallback)
			},
		},
		{
			name: "payload log",
			params: &GatewayRuleSettingsParams{
				PayloadLog: &GatewayPayloadLogParams{
					Enabled: true,
				},
			},
			validate: func(t *testing.T, result cloudflare.TeamsRuleSettings) {
				require.NotNil(t, result.PayloadLog)
				assert.True(t, result.PayloadLog.Enabled)
			},
		},
		{
			name: "untrusted cert action",
			params: &GatewayRuleSettingsParams{
				UntrustedCertAction: "block",
			},
			validate: func(t *testing.T, result cloudflare.TeamsRuleSettings) {
				require.NotNil(t, result.UntrustedCertSettings)
				assert.Equal(t, cloudflare.TeamsGatewayUntrustedCertAction("block"), result.UntrustedCertSettings.Action)
			},
		},
		{
			name: "audit SSH",
			params: &GatewayRuleSettingsParams{
				AuditSSH: &GatewayAuditSSHParams{
					CommandLogging: true,
				},
			},
			validate: func(t *testing.T, result cloudflare.TeamsRuleSettings) {
				require.NotNil(t, result.AuditSSH)
				assert.True(t, result.AuditSSH.CommandLogging)
			},
		},
		{
			name: "resolve DNS internally with fallback",
			params: &GatewayRuleSettingsParams{
				ResolveDNSInternally: &GatewayResolveDNSInternallyParams{
					ViewID:   "view-123",
					Fallback: "public_dns",
				},
			},
			validate: func(t *testing.T, result cloudflare.TeamsRuleSettings) {
				require.NotNil(t, result.ResolveDnsInternallySettings)
				assert.Equal(t, "view-123", result.ResolveDnsInternallySettings.ViewID)
				assert.Equal(t, cloudflare.TeamsResolveDnsInternallyFallbackStrategy("public_dns"), result.ResolveDnsInternallySettings.Fallback)
			},
		},
		{
			name: "resolve DNS through Cloudflare",
			params: &GatewayRuleSettingsParams{
				ResolveDNSThroughCloudflare: boolPtrGateway(true),
			},
			validate: func(t *testing.T, result cloudflare.TeamsRuleSettings) {
				require.NotNil(t, result.ResolveDnsThroughCloudflare)
				assert.True(t, *result.ResolveDnsThroughCloudflare)
			},
		},
		{
			name: "DNS resolvers",
			params: &GatewayRuleSettingsParams{
				DNSResolvers: &GatewayDNSResolversParams{
					IPv4: []GatewayDNSResolverEntryParams{
						{IP: "1.1.1.1", Port: 53, VNetID: "vnet-1", RouteThroughPrivateNetwork: boolPtrGateway(true)},
					},
					IPv6: []GatewayDNSResolverEntryParams{
						{IP: "2606:4700:4700::1111", Port: 53},
					},
				},
			},
			validate: func(t *testing.T, result cloudflare.TeamsRuleSettings) {
				require.NotNil(t, result.DnsResolverSettings)
				require.Len(t, result.DnsResolverSettings.V4Resolvers, 1)
				assert.Equal(t, "1.1.1.1", result.DnsResolverSettings.V4Resolvers[0].IP)
				assert.Equal(t, 53, *result.DnsResolverSettings.V4Resolvers[0].Port)
				assert.Equal(t, "vnet-1", result.DnsResolverSettings.V4Resolvers[0].VnetID)
				require.NotNil(t, result.DnsResolverSettings.V4Resolvers[0].RouteThroughPrivateNetwork)
				assert.True(t, *result.DnsResolverSettings.V4Resolvers[0].RouteThroughPrivateNetwork)

				require.Len(t, result.DnsResolverSettings.V6Resolvers, 1)
				assert.Equal(t, "2606:4700:4700::1111", result.DnsResolverSettings.V6Resolvers[0].IP)
			},
		},
		{
			name: "notification settings",
			params: &GatewayRuleSettingsParams{
				NotificationSettings: &GatewayNotificationSettingsParams{
					Enabled:    true,
					Message:    "Access blocked",
					SupportURL: "https://support.example.com",
				},
			},
			validate: func(t *testing.T, result cloudflare.TeamsRuleSettings) {
				require.NotNil(t, result.NotificationSettings)
				require.NotNil(t, result.NotificationSettings.Enabled)
				assert.True(t, *result.NotificationSettings.Enabled)
				assert.Equal(t, "Access blocked", result.NotificationSettings.Message)
				assert.Equal(t, "https://support.example.com", result.NotificationSettings.SupportURL)
			},
		},
		{
			name: "bypass settings",
			params: &GatewayRuleSettingsParams{
				AllowChildBypass:           boolPtrGateway(true),
				BypassParentRule:           boolPtrGateway(true),
				IgnoreCNAMECategoryMatches: boolPtrGateway(true),
			},
			validate: func(t *testing.T, result cloudflare.TeamsRuleSettings) {
				require.NotNil(t, result.AllowChildBypass)
				assert.True(t, *result.AllowChildBypass)
				require.NotNil(t, result.BypassParentRule)
				assert.True(t, *result.BypassParentRule)
				require.NotNil(t, result.IgnoreCNAMECategoryMatches)
				assert.True(t, *result.IgnoreCNAMECategoryMatches)
			},
		},
		{
			name: "IP categories",
			params: &GatewayRuleSettingsParams{
				IPCategories: boolPtrGateway(true),
			},
			validate: func(t *testing.T, result cloudflare.TeamsRuleSettings) {
				assert.True(t, result.IPCategories)
			},
		},
		{
			name: "quarantine",
			params: &GatewayRuleSettingsParams{
				Quarantine: &GatewayQuarantineParams{
					FileTypes: []string{"exe", "dll", "bat"},
				},
			},
			validate: func(t *testing.T, result cloudflare.TeamsRuleSettings) {
				require.NotNil(t, result.Quarantine)
				assert.Equal(t, []string{"exe", "dll", "bat"}, result.Quarantine.FileTypes)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertRuleSettingsToSDK(tt.params)
			tt.validate(t, result)
		})
	}
}

func TestConvertScheduleToSDK(t *testing.T) {
	tests := []struct {
		name     string
		params   *GatewayRuleScheduleParams
		validate func(t *testing.T, result *cloudflare.TeamsRuleSchedule)
	}{
		{
			name:   "nil params",
			params: nil,
			validate: func(t *testing.T, result *cloudflare.TeamsRuleSchedule) {
				assert.Nil(t, result)
			},
		},
		{
			name: "full schedule",
			params: &GatewayRuleScheduleParams{
				TimeZone: "America/New_York",
				Mon:      "09:00-17:00",
				Tue:      "09:00-17:00",
				Wed:      "09:00-17:00",
				Thu:      "09:00-17:00",
				Fri:      "09:00-17:00",
				Sat:      "",
				Sun:      "",
			},
			validate: func(t *testing.T, result *cloudflare.TeamsRuleSchedule) {
				require.NotNil(t, result)
				assert.Equal(t, "America/New_York", result.TimeZone)
				assert.Equal(t, cloudflare.TeamsScheduleTimes("09:00-17:00"), result.Monday)
				assert.Equal(t, cloudflare.TeamsScheduleTimes("09:00-17:00"), result.Tuesday)
				assert.Equal(t, cloudflare.TeamsScheduleTimes("09:00-17:00"), result.Wednesday)
				assert.Equal(t, cloudflare.TeamsScheduleTimes("09:00-17:00"), result.Thursday)
				assert.Equal(t, cloudflare.TeamsScheduleTimes("09:00-17:00"), result.Friday)
				assert.Empty(t, result.Saturday)
				assert.Empty(t, result.Sunday)
			},
		},
		{
			name: "weekend only",
			params: &GatewayRuleScheduleParams{
				TimeZone: "UTC",
				Sat:      "00:00-23:59",
				Sun:      "00:00-23:59",
			},
			validate: func(t *testing.T, result *cloudflare.TeamsRuleSchedule) {
				require.NotNil(t, result)
				assert.Equal(t, cloudflare.TeamsScheduleTimes("00:00-23:59"), result.Saturday)
				assert.Equal(t, cloudflare.TeamsScheduleTimes("00:00-23:59"), result.Sunday)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertScheduleToSDK(tt.params)
			tt.validate(t, result)
		})
	}
}

func TestConvertExpirationToSDK(t *testing.T) {
	tests := []struct {
		name     string
		params   *GatewayRuleExpirationParams
		validate func(t *testing.T, result *cloudflare.TeamsRuleExpiration)
	}{
		{
			name:   "nil params",
			params: nil,
			validate: func(t *testing.T, result *cloudflare.TeamsRuleExpiration) {
				assert.Nil(t, result)
			},
		},
		{
			name: "expiration with expires_at",
			params: &GatewayRuleExpirationParams{
				ExpiresAt: "2025-12-31T23:59:59Z",
			},
			validate: func(t *testing.T, result *cloudflare.TeamsRuleExpiration) {
				require.NotNil(t, result)
				require.NotNil(t, result.ExpiresAt)
				assert.Equal(t, 2025, result.ExpiresAt.Year())
				assert.Equal(t, time.December, result.ExpiresAt.Month())
				assert.Equal(t, 31, result.ExpiresAt.Day())
			},
		},
		{
			name: "invalid expires_at format",
			params: &GatewayRuleExpirationParams{
				ExpiresAt: "invalid-date",
			},
			validate: func(t *testing.T, result *cloudflare.TeamsRuleExpiration) {
				require.NotNil(t, result)
				assert.Nil(t, result.ExpiresAt)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertExpirationToSDK(tt.params)
			tt.validate(t, result)
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{"empty string", "", 0},
		{"1 hour", "1h", time.Hour},
		{"30 minutes", "30m", 30 * time.Minute},
		{"1 hour 30 minutes", "1h30m", 90 * time.Minute},
		{"24 hours", "24h", 24 * time.Hour},
		{"invalid format", "invalid", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDuration(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGatewayRuleParamsAllFields(t *testing.T) {
	params := GatewayRuleParams{
		Name:          "test-rule",
		Description:   "Test gateway rule",
		Precedence:    100,
		Enabled:       true,
		Action:        "block",
		DevicePosture: "any",
		Traffic:       "http.host == 'example.com'",
		Identity:      "any",
		RuleSettings: &GatewayRuleSettingsParams{
			BlockPageEnabled: boolPtrGateway(true),
			BlockReason:      "Test block",
		},
		Schedule: &GatewayRuleScheduleParams{
			TimeZone: "UTC",
			Mon:      "09:00-17:00",
		},
		Expiration: &GatewayRuleExpirationParams{
			ExpiresAt: "2025-12-31T23:59:59Z",
		},
	}

	assert.Equal(t, "test-rule", params.Name)
	assert.Equal(t, "Test gateway rule", params.Description)
	assert.Equal(t, 100, params.Precedence)
	assert.True(t, params.Enabled)
	assert.Equal(t, "block", params.Action)
	assert.NotNil(t, params.RuleSettings)
	assert.NotNil(t, params.Schedule)
	assert.NotNil(t, params.Expiration)
}

func TestGatewayL4OverrideParams(t *testing.T) {
	params := GatewayL4OverrideParams{
		IP:   "192.168.1.1",
		Port: 443,
	}

	assert.Equal(t, "192.168.1.1", params.IP)
	assert.Equal(t, 443, params.Port)
}

func TestGatewayDNSResolversParams(t *testing.T) {
	params := GatewayDNSResolversParams{
		IPv4: []GatewayDNSResolverEntryParams{
			{IP: "1.1.1.1", Port: 53},
			{IP: "8.8.8.8", Port: 53, VNetID: "vnet-1"},
		},
		IPv6: []GatewayDNSResolverEntryParams{
			{IP: "2606:4700:4700::1111", Port: 53},
		},
	}

	assert.Len(t, params.IPv4, 2)
	assert.Len(t, params.IPv6, 1)
	assert.Equal(t, "vnet-1", params.IPv4[1].VNetID)
}

// Helper function for gateway tests
func boolPtrGateway(b bool) *bool {
	return &b
}
