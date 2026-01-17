// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package device

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

func TestResourceTypeConstants(t *testing.T) {
	assert.Equal(t, v1alpha2.SyncResourceDevicePostureRule, ResourceTypeDevicePostureRule)
	assert.Equal(t, v1alpha2.SyncResourceDeviceSettingsPolicy, ResourceTypeDeviceSettingsPolicy)
}

func TestPriorityConstants(t *testing.T) {
	assert.Equal(t, 100, PriorityDevicePostureRule)
	assert.Equal(t, 100, PriorityDeviceSettingsPolicy)
}

func TestDevicePostureRuleConfig(t *testing.T) {
	config := DevicePostureRuleConfig{
		Name:        "serial-number-check",
		Type:        "serial_number",
		Description: "Check for authorized serial numbers",
		Schedule:    "1h",
		Match: []DevicePostureMatch{
			{Platform: "windows"},
			{Platform: "mac"},
		},
	}

	assert.Equal(t, "serial-number-check", config.Name)
	assert.Equal(t, "serial_number", config.Type)
	assert.Equal(t, "Check for authorized serial numbers", config.Description)
	assert.Equal(t, "1h", config.Schedule)
	assert.Len(t, config.Match, 2)
}

func TestDevicePostureMatch(t *testing.T) {
	platforms := []string{"windows", "mac", "linux", "ios", "android"}

	for _, platform := range platforms {
		t.Run(platform, func(t *testing.T) {
			match := DevicePostureMatch{
				Platform: platform,
			}
			assert.Equal(t, platform, match.Platform)
		})
	}
}

func TestDevicePostureInput(t *testing.T) {
	exists := true
	running := true
	requireAll := false

	input := DevicePostureInput{
		ID:         "rule-123",
		Path:       "/usr/local/bin/app",
		Exists:     &exists,
		Sha256:     "abc123def456",
		Running:    &running,
		RequireAll: &requireAll,
		Version:    "1.0.0",
		Operator:   ">=",
	}

	assert.Equal(t, "rule-123", input.ID)
	assert.Equal(t, "/usr/local/bin/app", input.Path)
	assert.True(t, *input.Exists)
	assert.Equal(t, "abc123def456", input.Sha256)
	assert.True(t, *input.Running)
	assert.False(t, *input.RequireAll)
	assert.Equal(t, "1.0.0", input.Version)
	assert.Equal(t, ">=", input.Operator)
}

func TestDevicePostureInputAdvanced(t *testing.T) {
	activeThreats := 0
	infected := false
	score := 80
	issueCount := 5

	input := DevicePostureInput{
		ComplianceStatus: "compliant",
		ActiveThreats:    &activeThreats,
		Infected:         &infected,
		RiskLevel:        "low",
		Score:            &score,
		IssueCount:       &issueCount,
		State:            "active",
	}

	assert.Equal(t, "compliant", input.ComplianceStatus)
	assert.Equal(t, 0, *input.ActiveThreats)
	assert.False(t, *input.Infected)
	assert.Equal(t, "low", input.RiskLevel)
	assert.Equal(t, 80, *input.Score)
	assert.Equal(t, 5, *input.IssueCount)
}

func TestDevicePostureInputCertificate(t *testing.T) {
	checkPrivateKey := true

	input := DevicePostureInput{
		CertificateID:    "cert-123",
		CommonName:       "Example Corp",
		Cn:               "example.com",
		CheckPrivateKey:  &checkPrivateKey,
		ExtendedKeyUsage: []string{"serverAuth", "clientAuth"},
	}

	assert.Equal(t, "cert-123", input.CertificateID)
	assert.Equal(t, "Example Corp", input.CommonName)
	assert.Equal(t, "example.com", input.Cn)
	assert.True(t, *input.CheckPrivateKey)
	assert.Len(t, input.ExtendedKeyUsage, 2)
}

func TestDeviceSettingsPolicyConfig(t *testing.T) {
	config := DeviceSettingsPolicyConfig{
		SplitTunnelMode: "exclude",
		SplitTunnelExclude: []SplitTunnelEntry{
			{Address: "10.0.0.0/8", Description: "Internal network"},
		},
		SplitTunnelInclude: []SplitTunnelEntry{},
		FallbackDomains: []FallbackDomainEntry{
			{Suffix: "internal.example.com", DNSServer: []string{"10.0.0.53"}},
		},
	}

	assert.Equal(t, "exclude", config.SplitTunnelMode)
	assert.Len(t, config.SplitTunnelExclude, 1)
	assert.Empty(t, config.SplitTunnelInclude)
	assert.Len(t, config.FallbackDomains, 1)
}

//nolint:revive // cognitive complexity unavoidable: table-driven tests require comprehensive test cases
func TestSplitTunnelEntry(t *testing.T) {
	tests := []struct {
		name       string
		entry      SplitTunnelEntry
		hasAddress bool
		hasHost    bool
	}{
		{
			name: "by address",
			entry: SplitTunnelEntry{
				Address:     "192.168.0.0/16",
				Description: "Local network",
			},
			hasAddress: true,
			hasHost:    false,
		},
		{
			name: "by host",
			entry: SplitTunnelEntry{
				Host:        "*.local",
				Description: "Local domains",
			},
			hasAddress: false,
			hasHost:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.hasAddress {
				assert.NotEmpty(t, tt.entry.Address)
			}
			if tt.hasHost {
				assert.NotEmpty(t, tt.entry.Host)
			}
		})
	}
}

func TestFallbackDomainEntry(t *testing.T) {
	entry := FallbackDomainEntry{
		Suffix:      "corp.example.com",
		Description: "Corporate domain",
		DNSServer:   []string{"10.0.0.53", "10.0.0.54"},
	}

	assert.Equal(t, "corp.example.com", entry.Suffix)
	assert.Equal(t, "Corporate domain", entry.Description)
	assert.Len(t, entry.DNSServer, 2)
}

func TestDevicePostureRuleRegisterOptions(t *testing.T) {
	opts := DevicePostureRuleRegisterOptions{
		AccountID: "account-123",
		RuleID:    "rule-456",
		Source: service.Source{
			Kind: "DevicePostureRule",
			Name: "my-rule",
		},
		Config: DevicePostureRuleConfig{
			Name: "My Posture Rule",
			Type: "file",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "rule-456", opts.RuleID)
	assert.Equal(t, "DevicePostureRule", opts.Source.Kind)
	assert.Equal(t, "My Posture Rule", opts.Config.Name)
}

func TestDeviceSettingsPolicyRegisterOptions(t *testing.T) {
	opts := DeviceSettingsPolicyRegisterOptions{
		AccountID: "account-123",
		Source: service.Source{
			Kind: "DeviceSettingsPolicy",
			Name: "my-policy",
		},
		Config: DeviceSettingsPolicyConfig{
			SplitTunnelMode: "include",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "DeviceSettingsPolicy", opts.Source.Kind)
	assert.Equal(t, "include", opts.Config.SplitTunnelMode)
}

func TestDevicePostureRuleSyncResult(t *testing.T) {
	result := DevicePostureRuleSyncResult{
		RuleID:    "rule-123",
		AccountID: "account-456",
	}

	assert.Equal(t, "rule-123", result.RuleID)
	assert.Equal(t, "account-456", result.AccountID)
}

func TestDeviceSettingsPolicySyncResult(t *testing.T) {
	result := DeviceSettingsPolicySyncResult{
		AccountID:                "account-123",
		SplitTunnelExcludeCount:  5,
		SplitTunnelIncludeCount:  3,
		FallbackDomainsCount:     2,
		AutoPopulatedRoutesCount: 10,
	}

	assert.Equal(t, "account-123", result.AccountID)
	assert.Equal(t, 5, result.SplitTunnelExcludeCount)
	assert.Equal(t, 3, result.SplitTunnelIncludeCount)
	assert.Equal(t, 2, result.FallbackDomainsCount)
	assert.Equal(t, 10, result.AutoPopulatedRoutesCount)
}

func TestAutoPopulatedRoutes(t *testing.T) {
	config := DeviceSettingsPolicyConfig{
		SplitTunnelMode: "include",
		SplitTunnelInclude: []SplitTunnelEntry{
			{Address: "10.0.0.0/8", Description: "User defined"},
		},
		AutoPopulatedRoutes: []SplitTunnelEntry{
			{Address: "192.168.1.0/24", Description: "From NetworkRoute: route-1"},
			{Address: "172.16.0.0/12", Description: "From NetworkRoute: route-2"},
		},
	}

	assert.Len(t, config.SplitTunnelInclude, 1)
	assert.Len(t, config.AutoPopulatedRoutes, 2)
}
