// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package gatewayrule

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

//nolint:revive // cognitive-complexity: table-driven test with many test cases
func TestBuildRuleSettings(t *testing.T) {
	reconciler := &GatewayRuleReconciler{}

	tests := []struct {
		name     string
		settings *networkingv1alpha2.GatewayRuleSettings
		wantNil  bool
		validate func(t *testing.T, result interface{})
	}{
		{
			name:     "nil settings returns nil",
			settings: nil,
			wantNil:  true,
		},
		{
			name: "basic settings",
			settings: &networkingv1alpha2.GatewayRuleSettings{
				BlockPageEnabled: boolPtr(true),
				BlockReason:      "Policy violation",
			},
			validate: func(t *testing.T, result interface{}) {
				require.NotNil(t, result)
			},
		},
		{
			name: "override settings",
			settings: &networkingv1alpha2.GatewayRuleSettings{
				OverrideIPs:  []string{"1.1.1.1", "8.8.8.8"},
				OverrideHost: "override.example.com",
			},
			validate: func(t *testing.T, result interface{}) {
				require.NotNil(t, result)
			},
		},
		{
			name: "L4 override",
			settings: &networkingv1alpha2.GatewayRuleSettings{
				L4Override: &networkingv1alpha2.L4OverrideSettings{
					IP:   "10.0.0.1",
					Port: 8080,
				},
			},
			validate: func(t *testing.T, result interface{}) {
				require.NotNil(t, result)
			},
		},
		{
			name: "BISO admin controls",
			settings: &networkingv1alpha2.GatewayRuleSettings{
				BISOAdminControls: &networkingv1alpha2.BISOAdminControls{
					DisablePrinting:             boolPtr(true),
					DisableCopyPaste:            boolPtr(true),
					DisableDownload:             boolPtr(true),
					DisableUpload:               boolPtr(true),
					DisableKeyboard:             boolPtr(true),
					DisableClipboardRedirection: boolPtr(true),
				},
			},
			validate: func(t *testing.T, result interface{}) {
				require.NotNil(t, result)
			},
		},
		{
			name: "check session",
			settings: &networkingv1alpha2.GatewayRuleSettings{
				CheckSession: &networkingv1alpha2.SessionSettings{
					Enforce:  true,
					Duration: "1h",
				},
			},
			validate: func(t *testing.T, result interface{}) {
				require.NotNil(t, result)
			},
		},
		{
			name: "add headers",
			settings: &networkingv1alpha2.GatewayRuleSettings{
				AddHeaders: map[string]string{
					"X-Custom-Header": "value",
				},
			},
			validate: func(t *testing.T, result interface{}) {
				require.NotNil(t, result)
			},
		},
		{
			name: "egress settings",
			settings: &networkingv1alpha2.GatewayRuleSettings{
				Egress: &networkingv1alpha2.EgressSettings{
					IPv4:         "203.0.113.1",
					IPv6:         "2001:db8::1/32",
					IPv4Fallback: "192.0.2.1",
				},
			},
			validate: func(t *testing.T, result interface{}) {
				require.NotNil(t, result)
			},
		},
		{
			name: "payload log",
			settings: &networkingv1alpha2.GatewayRuleSettings{
				PayloadLog: &networkingv1alpha2.PayloadLogSettings{
					Enabled: true,
				},
			},
			validate: func(t *testing.T, result interface{}) {
				require.NotNil(t, result)
			},
		},
		{
			name: "audit SSH",
			settings: &networkingv1alpha2.GatewayRuleSettings{
				AuditSSH: &networkingv1alpha2.AuditSSHSettings{
					CommandLogging: true,
				},
			},
			validate: func(t *testing.T, result interface{}) {
				require.NotNil(t, result)
			},
		},
		{
			name: "DNS resolvers",
			settings: &networkingv1alpha2.GatewayRuleSettings{
				DNSResolvers: &networkingv1alpha2.DNSResolversSettings{
					IPv4: []networkingv1alpha2.DNSResolverEntry{
						{IP: "1.1.1.1", Port: 53, VNetID: "vnet-1", RouteThroughPrivateNetwork: boolPtr(true)},
					},
					IPv6: []networkingv1alpha2.DNSResolverEntry{
						{IP: "2606:4700:4700::1111", Port: 53},
					},
				},
			},
			validate: func(t *testing.T, result interface{}) {
				require.NotNil(t, result)
			},
		},
		{
			name: "notification settings",
			settings: &networkingv1alpha2.GatewayRuleSettings{
				NotificationSettings: &networkingv1alpha2.NotificationSettings{
					Enabled:    true,
					Message:    "Access blocked",
					SupportURL: "https://support.example.com",
				},
			},
			validate: func(t *testing.T, result interface{}) {
				require.NotNil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reconciler.buildRuleSettings(tt.settings)

			if tt.wantNil {
				assert.Nil(t, result)
				return
			}

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

// Helper function
func boolPtr(b bool) *bool {
	return &b
}
