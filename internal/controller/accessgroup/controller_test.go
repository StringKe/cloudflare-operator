// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package accessgroup

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

func TestBuildGroupRules(t *testing.T) {
	reconciler := &AccessGroupReconciler{}

	tests := []struct {
		name     string
		rules    []networkingv1alpha2.AccessGroupRule
		wantNil  bool
		validate func(t *testing.T, result []interface{})
	}{
		{
			name:    "nil rules returns nil",
			rules:   nil,
			wantNil: true,
		},
		{
			name:    "empty rules returns nil",
			rules:   []networkingv1alpha2.AccessGroupRule{},
			wantNil: true,
		},
		{
			name: "email rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					Email: &networkingv1alpha2.AccessGroupEmailRule{
						Email: "user@example.com",
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "email domain rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					EmailDomain: &networkingv1alpha2.AccessGroupEmailDomainRule{
						Domain: "example.com",
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "email list rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					EmailList: &networkingv1alpha2.AccessGroupEmailListRule{
						ID: "list-123",
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "IP ranges rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					IPRanges: &networkingv1alpha2.AccessGroupIPRangesRule{
						IP: []string{"192.168.1.0/24", "10.0.0.0/8"},
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "IP ranges rule with empty IP skipped",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					IPRanges: &networkingv1alpha2.AccessGroupIPRangesRule{
						IP: []string{},
					},
				},
			},
			wantNil: false,
			validate: func(t *testing.T, result []interface{}) {
				// Empty IP ranges should not add a rule
				assert.Empty(t, result)
			},
		},
		{
			name: "IP list rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					IPList: &networkingv1alpha2.AccessGroupIPListRule{
						ID: "iplist-456",
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "everyone rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					Everyone: true,
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "group rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					Group: &networkingv1alpha2.AccessGroupGroupRule{
						ID: "group-789",
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "any valid service token rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					AnyValidServiceToken: true,
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "service token rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					ServiceToken: &networkingv1alpha2.AccessGroupServiceTokenRule{
						TokenID: "token-abc",
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "external evaluation rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					ExternalEvaluation: &networkingv1alpha2.AccessGroupExternalEvaluationRule{
						EvaluateURL: "https://eval.example.com/check",
						KeysURL:     "https://eval.example.com/keys",
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "country rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					Country: &networkingv1alpha2.AccessGroupCountryRule{
						Country: []string{"US", "CA", "GB"},
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "country rule with empty list skipped",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					Country: &networkingv1alpha2.AccessGroupCountryRule{
						Country: []string{},
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				assert.Empty(t, result)
			},
		},
		{
			name: "device posture rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					DevicePosture: &networkingv1alpha2.AccessGroupDevicePostureRule{
						IntegrationUID: "posture-xyz",
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "common name rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					CommonName: &networkingv1alpha2.AccessGroupCommonNameRule{
						CommonName: "*.example.com",
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "certificate rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					Certificate: true,
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "SAML rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					SAML: &networkingv1alpha2.AccessGroupSAMLRule{
						AttributeName:      "groups",
						AttributeValue:     "admins",
						IdentityProviderID: "saml-idp-123",
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "OIDC rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					OIDC: &networkingv1alpha2.AccessGroupOIDCRule{
						ClaimName:          "email",
						ClaimValue:         "admin@example.com",
						IdentityProviderID: "oidc-idp-456",
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "GSuite rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					GSuite: &networkingv1alpha2.AccessGroupGSuiteRule{
						Email:              "admin@company.com",
						IdentityProviderID: "gsuite-idp-789",
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "Azure rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					Azure: &networkingv1alpha2.AccessGroupAzureRule{
						ID:                 "azure-group-id",
						IdentityProviderID: "azure-idp-abc",
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "GitHub rule with teams",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					GitHub: &networkingv1alpha2.AccessGroupGitHubRule{
						Name:               "myorg",
						Teams:              []string{"team-a", "team-b"},
						IdentityProviderID: "github-idp-def",
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "Okta rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					Okta: &networkingv1alpha2.AccessGroupOktaRule{
						Name:               "okta-group",
						IdentityProviderID: "okta-idp-ghi",
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "auth method rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					AuthMethod: &networkingv1alpha2.AccessGroupAuthMethodRule{
						AuthMethod: "mfa",
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "auth context rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					AuthContext: &networkingv1alpha2.AccessGroupAuthContextRule{
						ID:                 "auth-ctx-123",
						IdentityProviderID: "idp-456",
						AcID:               "ac-id-789",
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "login method rule",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					LoginMethod: &networkingv1alpha2.AccessGroupLoginMethodRule{
						ID: "login-method-id",
					},
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 1)
			},
		},
		{
			name: "multiple rules",
			rules: []networkingv1alpha2.AccessGroupRule{
				{
					Email: &networkingv1alpha2.AccessGroupEmailRule{
						Email: "user1@example.com",
					},
				},
				{
					EmailDomain: &networkingv1alpha2.AccessGroupEmailDomainRule{
						Domain: "example.org",
					},
				},
				{
					Everyone: true,
				},
			},
			validate: func(t *testing.T, result []interface{}) {
				require.Len(t, result, 3)
			},
		},
		{
			name: "rule with no valid fields is skipped",
			rules: []networkingv1alpha2.AccessGroupRule{
				{}, // Empty rule
			},
			validate: func(t *testing.T, result []interface{}) {
				assert.Empty(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reconciler.buildGroupRules(tt.rules)

			if tt.wantNil {
				assert.Nil(t, result)
				return
			}

			if tt.validate != nil {
				// Convert to []interface{} for validation
				var resultInterface []interface{}
				for _, r := range result {
					resultInterface = append(resultInterface, r)
				}
				tt.validate(t, resultInterface)
			}
		})
	}
}
