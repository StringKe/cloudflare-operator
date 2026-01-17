// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

func TestResourceTypeConstants(t *testing.T) {
	assert.Equal(t, v1alpha2.SyncResourceGatewayRule, ResourceTypeGatewayRule)
	assert.Equal(t, v1alpha2.SyncResourceGatewayList, ResourceTypeGatewayList)
	assert.Equal(t, v1alpha2.SyncResourceGatewayConfiguration, ResourceTypeGatewayConfiguration)
}

func TestPriorityConstants(t *testing.T) {
	assert.Equal(t, 100, PriorityGatewayRule)
	assert.Equal(t, 100, PriorityGatewayList)
	assert.Equal(t, 100, PriorityGatewayConfiguration)
}

func TestGatewayRuleConfig(t *testing.T) {
	config := GatewayRuleConfig{
		Name:        "block-malware",
		Description: "Block known malware domains",
		TrafficType: "dns",
		Action:      "block",
		Priority:    100,
		Enabled:     true,
		Filters: []GatewayRuleFilter{
			{
				Type:       "dns",
				Expression: "any(dns.domains[*] in $malware_list)",
			},
		},
	}

	assert.Equal(t, "block-malware", config.Name)
	assert.Equal(t, "dns", config.TrafficType)
	assert.Equal(t, "block", config.Action)
	assert.Equal(t, 100, config.Priority)
	assert.True(t, config.Enabled)
	assert.Len(t, config.Filters, 1)
}

func TestGatewayRuleFilter(t *testing.T) {
	filter := GatewayRuleFilter{
		Type:       "http",
		Expression: "http.request.uri.path contains \"/admin\"",
	}

	assert.Equal(t, "http", filter.Type)
	assert.Contains(t, filter.Expression, "/admin")
}

func TestGatewayRuleSettings(t *testing.T) {
	blockPageEnabled := true
	disableDNSSEC := false

	settings := GatewayRuleSettings{
		BlockPageEnabled:                &blockPageEnabled,
		BlockReason:                     "Access denied by policy",
		OverrideHost:                    "safe.example.com",
		OverrideIPs:                     []string{"10.0.0.1", "10.0.0.2"},
		InsecureDisableDNSSECValidation: &disableDNSSEC,
		AddHeaders: map[string]string{
			"X-Custom-Header": "value",
		},
	}

	assert.True(t, *settings.BlockPageEnabled)
	assert.Equal(t, "Access denied by policy", settings.BlockReason)
	assert.Equal(t, "safe.example.com", settings.OverrideHost)
	assert.Len(t, settings.OverrideIPs, 2)
	assert.False(t, *settings.InsecureDisableDNSSECValidation)
	assert.NotEmpty(t, settings.AddHeaders)
}

func TestBISOAdminControls(t *testing.T) {
	controls := BISOAdminControls{
		DisablePrinting:          boolPtr(true),
		DisableCopyPaste:         boolPtr(true),
		DisableDownload:          boolPtr(true),
		DisableUpload:            boolPtr(false),
		DisableKeyboard:          boolPtr(false),
		DisableClipboardRedirect: boolPtr(true),
	}

	assert.True(t, *controls.DisablePrinting)
	assert.True(t, *controls.DisableCopyPaste)
	assert.True(t, *controls.DisableDownload)
	assert.False(t, *controls.DisableUpload)
	assert.False(t, *controls.DisableKeyboard)
	assert.True(t, *controls.DisableClipboardRedirect)
}

func TestCheckSessionSettings(t *testing.T) {
	settings := CheckSessionSettings{
		Enforce:  true,
		Duration: "24h",
	}

	assert.True(t, settings.Enforce)
	assert.Equal(t, "24h", settings.Duration)
}

func TestL4OverrideSettings(t *testing.T) {
	settings := L4OverrideSettings{
		IP:   "10.0.0.1",
		Port: 8080,
	}

	assert.Equal(t, "10.0.0.1", settings.IP)
	assert.Equal(t, 8080, settings.Port)
}

func TestNotificationSettings(t *testing.T) {
	settings := NotificationSettings{
		Enabled:    true,
		Message:    "Access blocked by IT policy",
		SupportURL: "https://support.example.com",
	}

	assert.True(t, settings.Enabled)
	assert.Equal(t, "Access blocked by IT policy", settings.Message)
	assert.Equal(t, "https://support.example.com", settings.SupportURL)
}

func TestPayloadLogSettings(t *testing.T) {
	settings := PayloadLogSettings{
		Enabled: true,
	}

	assert.True(t, settings.Enabled)
}

func TestAuditSSHSettings(t *testing.T) {
	settings := AuditSSHSettings{
		CommandLogging: true,
	}

	assert.True(t, settings.CommandLogging)
}

func TestUntrustedCertSettings(t *testing.T) {
	actions := []string{"block", "warn", "pass"}

	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			settings := UntrustedCertSettings{
				Action: action,
			}
			assert.Equal(t, action, settings.Action)
		})
	}
}

func TestEgressSettings(t *testing.T) {
	settings := EgressSettings{
		Ipv4:         "192.0.2.1",
		Ipv6:         "2001:db8::1",
		Ipv4Fallback: "192.0.2.2",
	}

	assert.Equal(t, "192.0.2.1", settings.Ipv4)
	assert.Equal(t, "2001:db8::1", settings.Ipv6)
	assert.Equal(t, "192.0.2.2", settings.Ipv4Fallback)
}

func TestDNSResolverSettings(t *testing.T) {
	settings := DNSResolverSettings{
		Ipv4: []DNSResolverAddress{
			{IP: "1.1.1.1", Port: 53},
			{IP: "8.8.8.8", Port: 53},
		},
		Ipv6: []DNSResolverAddress{
			{IP: "2606:4700:4700::1111", Port: 53},
		},
	}

	assert.Len(t, settings.Ipv4, 2)
	assert.Len(t, settings.Ipv6, 1)
	assert.Equal(t, "1.1.1.1", settings.Ipv4[0].IP)
}

func TestGatewayListConfig(t *testing.T) {
	listTypes := []struct {
		name     string
		listType string
		items    []string
	}{
		{
			name:     "domain list",
			listType: "DOMAIN",
			items:    []string{"example.com", "test.com"},
		},
		{
			name:     "IP list",
			listType: "IP",
			items:    []string{"10.0.0.1", "10.0.0.2"},
		},
		{
			name:     "URL list",
			listType: "URL",
			items:    []string{"https://example.com/path"},
		},
		{
			name:     "serial number list",
			listType: "SERIAL",
			items:    []string{"ABC123", "XYZ789"},
		},
	}

	for _, tt := range listTypes {
		t.Run(tt.name, func(t *testing.T) {
			config := GatewayListConfig{
				Name:        "my-list",
				Description: "Test list",
				Type:        tt.listType,
				Items:       tt.items,
			}

			assert.Equal(t, tt.listType, config.Type)
			assert.Equal(t, tt.items, config.Items)
		})
	}
}

func TestGatewayConfigurationConfig(t *testing.T) {
	config := GatewayConfigurationConfig{
		TLSDecrypt: &TLSDecryptSettings{
			Enabled: true,
		},
		ActivityLog: &ActivityLogSettings{
			Enabled: true,
		},
		AntiVirus: &AntiVirusSettings{
			EnabledDownloadPhase: true,
			EnabledUploadPhase:   true,
			FailClosed:           false,
		},
		BlockPage: &BlockPageSettings{
			Enabled:         true,
			HeaderText:      "Access Blocked",
			FooterText:      "Contact IT for assistance",
			BackgroundColor: "#FF0000",
		},
		BrowserIsolation: &BrowserIsolationSettings{
			URLBrowserIsolationEnabled: true,
			NonIdentityEnabled:         false,
		},
		FIPS: &FIPSSettings{
			TLS: true,
		},
	}

	assert.True(t, config.TLSDecrypt.Enabled)
	assert.True(t, config.ActivityLog.Enabled)
	assert.True(t, config.AntiVirus.EnabledDownloadPhase)
	assert.True(t, config.BlockPage.Enabled)
	assert.True(t, config.BrowserIsolation.URLBrowserIsolationEnabled)
	assert.True(t, config.FIPS.TLS)
}

func TestBodyScanningSettings(t *testing.T) {
	modes := []string{"deep", "shallow"}

	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			settings := BodyScanningSettings{
				InspectionMode: mode,
			}
			assert.Equal(t, mode, settings.InspectionMode)
		})
	}
}

func TestProtocolDetectionSettings(t *testing.T) {
	settings := ProtocolDetectionSettings{
		Enabled: true,
	}

	assert.True(t, settings.Enabled)
}

func TestCustomCertificateSettings(t *testing.T) {
	settings := CustomCertificateSettings{
		Enabled: true,
		ID:      "cert-123",
	}

	assert.True(t, settings.Enabled)
	assert.Equal(t, "cert-123", settings.ID)
}

func TestGatewayRuleRegisterOptions(t *testing.T) {
	opts := GatewayRuleRegisterOptions{
		AccountID: "account-123",
		RuleID:    "rule-456",
		Source: service.Source{
			Kind: "GatewayRule",
			Name: "my-rule",
		},
		Config: GatewayRuleConfig{
			Name:    "Test Rule",
			Action:  "block",
			Enabled: true,
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "rule-456", opts.RuleID)
	assert.Equal(t, "GatewayRule", opts.Source.Kind)
}

func TestGatewayListRegisterOptions(t *testing.T) {
	opts := GatewayListRegisterOptions{
		AccountID: "account-123",
		ListID:    "list-456",
		Source: service.Source{
			Kind: "GatewayList",
			Name: "my-list",
		},
		Config: GatewayListConfig{
			Name: "Test List",
			Type: "DOMAIN",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "list-456", opts.ListID)
}

func TestGatewayConfigurationRegisterOptions(t *testing.T) {
	opts := GatewayConfigurationRegisterOptions{
		AccountID: "account-123",
		Source: service.Source{
			Kind: "GatewayConfiguration",
			Name: "my-config",
		},
		Config:         GatewayConfigurationConfig{},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "GatewayConfiguration", opts.Source.Kind)
}

func TestGatewayRuleSyncResult(t *testing.T) {
	result := GatewayRuleSyncResult{
		RuleID:    "rule-123",
		AccountID: "account-456",
	}

	assert.Equal(t, "rule-123", result.RuleID)
	assert.Equal(t, "account-456", result.AccountID)
}

func TestGatewayListSyncResult(t *testing.T) {
	result := GatewayListSyncResult{
		ListID:    "list-123",
		AccountID: "account-456",
		ItemCount: 25,
	}

	assert.Equal(t, "list-123", result.ListID)
	assert.Equal(t, "account-456", result.AccountID)
	assert.Equal(t, 25, result.ItemCount)
}

func TestGatewayConfigurationSyncResult(t *testing.T) {
	result := GatewayConfigurationSyncResult{
		AccountID: "account-123",
	}

	assert.Equal(t, "account-123", result.AccountID)
}

// Helper function
func boolPtr(b bool) *bool {
	return &b
}
