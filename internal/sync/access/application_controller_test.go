// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package access

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	accesssvc "github.com/StringKe/cloudflare-operator/internal/service/access"
)

// TestGetPolicyName tests policy name generation for different scenarios.
func TestGetPolicyName(t *testing.T) {
	controller := &ApplicationController{}

	tests := []struct {
		name     string
		policy   accesssvc.AccessPolicyConfig
		expected string
	}{
		{
			name: "explicit policy name",
			policy: accesssvc.AccessPolicyConfig{
				PolicyName: "My Custom Policy",
				GroupName:  "Admin Group",
				Decision:   "allow",
			},
			expected: "My Custom Policy",
		},
		{
			name: "group name with decision",
			policy: accesssvc.AccessPolicyConfig{
				GroupName: "Admin Group",
				Decision:  "allow",
			},
			expected: "Admin Group - allow",
		},
		{
			name: "inline policy with precedence",
			policy: accesssvc.AccessPolicyConfig{
				Decision:   "deny",
				Precedence: 5,
				Include: []v1alpha2.AccessGroupRule{
					{Everyone: true},
				},
			},
			expected: "Inline Policy 5 - deny",
		},
		{
			name: "fallback policy name",
			policy: accesssvc.AccessPolicyConfig{
				Decision:   "bypass",
				Precedence: 3,
			},
			expected: "Policy 3 - bypass",
		},
		{
			name: "inline policy with allow decision",
			policy: accesssvc.AccessPolicyConfig{
				Decision:   "allow",
				Precedence: 1,
				Include: []v1alpha2.AccessGroupRule{
					{
						EmailDomain: &v1alpha2.AccessGroupEmailDomainRule{
							Domain: "example.com",
						},
					},
				},
			},
			expected: "Inline Policy 1 - allow",
		},
		{
			name: "inline policy with non_identity decision",
			policy: accesssvc.AccessPolicyConfig{
				Decision:   "non_identity",
				Precedence: 10,
				Include: []v1alpha2.AccessGroupRule{
					{AnyValidServiceToken: true},
				},
			},
			expected: "Inline Policy 10 - non_identity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.getPolicyName(tt.policy)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConvertGroupRules tests the conversion of AccessGroupRule to AccessGroupRuleParams.
func TestConvertGroupRules(t *testing.T) {
	tests := []struct {
		name     string
		rules    []v1alpha2.AccessGroupRule
		expected int
		verify   func(t *testing.T, params []cf.AccessGroupRuleParams)
	}{
		{
			name:     "empty rules",
			rules:    nil,
			expected: 0,
		},
		{
			name: "single email rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					Email: &v1alpha2.AccessGroupEmailRule{
						Email: "admin@example.com",
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].Email)
				assert.Equal(t, "admin@example.com", params[0].Email.Email)
			},
		},
		{
			name: "email domain rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					EmailDomain: &v1alpha2.AccessGroupEmailDomainRule{
						Domain: "example.com",
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].EmailDomain)
				assert.Equal(t, "example.com", params[0].EmailDomain.Domain)
			},
		},
		{
			name: "email list rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					EmailList: &v1alpha2.AccessGroupEmailListRule{
						ID: "email-list-123",
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].EmailList)
				assert.Equal(t, "email-list-123", params[0].EmailList.ID)
			},
		},
		{
			name: "everyone rule",
			rules: []v1alpha2.AccessGroupRule{
				{Everyone: true},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.True(t, params[0].Everyone)
			},
		},
		{
			name: "IP ranges rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					IPRanges: &v1alpha2.AccessGroupIPRangesRule{
						IP: []string{"10.0.0.0/8", "192.168.0.0/16"},
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].IPRanges)
				assert.Equal(t, []string{"10.0.0.0/8", "192.168.0.0/16"}, params[0].IPRanges.IP)
			},
		},
		{
			name: "IP list rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					IPList: &v1alpha2.AccessGroupIPListRule{
						ID: "ip-list-456",
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].IPList)
				assert.Equal(t, "ip-list-456", params[0].IPList.ID)
			},
		},
		{
			name: "country rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					Country: &v1alpha2.AccessGroupCountryRule{
						Country: []string{"US"},
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].Country)
				assert.Equal(t, []string{"US"}, params[0].Country.Country)
			},
		},
		{
			name: "group rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					Group: &v1alpha2.AccessGroupGroupRule{
						ID: "nested-group-id",
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].Group)
				assert.Equal(t, "nested-group-id", params[0].Group.ID)
			},
		},
		{
			name: "service token rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					ServiceToken: &v1alpha2.AccessGroupServiceTokenRule{
						TokenID: "token-123",
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].ServiceToken)
				assert.Equal(t, "token-123", params[0].ServiceToken.TokenID)
			},
		},
		{
			name: "any valid service token rule",
			rules: []v1alpha2.AccessGroupRule{
				{AnyValidServiceToken: true},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.True(t, params[0].AnyValidServiceToken)
			},
		},
		{
			name: "certificate rule",
			rules: []v1alpha2.AccessGroupRule{
				{Certificate: true},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.True(t, params[0].Certificate)
			},
		},
		{
			name: "common name rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					CommonName: &v1alpha2.AccessGroupCommonNameRule{
						CommonName: "client.example.com",
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].CommonName)
				assert.Equal(t, "client.example.com", params[0].CommonName.CommonName)
			},
		},
		{
			name: "device posture rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					DevicePosture: &v1alpha2.AccessGroupDevicePostureRule{
						IntegrationUID: "posture-uid-123",
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].DevicePosture)
				assert.Equal(t, "posture-uid-123", params[0].DevicePosture.IntegrationUID)
			},
		},
		{
			name: "GSuite rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					GSuite: &v1alpha2.AccessGroupGSuiteRule{
						Email:              "group@gsuite.com",
						IdentityProviderID: "gsuite-idp-id",
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].GSuite)
				assert.Equal(t, "group@gsuite.com", params[0].GSuite.Email)
				assert.Equal(t, "gsuite-idp-id", params[0].GSuite.IdentityProviderID)
			},
		},
		{
			name: "GitHub rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					GitHub: &v1alpha2.AccessGroupGitHubRule{
						Name:               "myorg",
						Teams:              []string{"team1", "team2"},
						IdentityProviderID: "github-idp-id",
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].GitHub)
				assert.Equal(t, "myorg", params[0].GitHub.Name)
				assert.Equal(t, []string{"team1", "team2"}, params[0].GitHub.Teams)
				assert.Equal(t, "github-idp-id", params[0].GitHub.IdentityProviderID)
			},
		},
		{
			name: "Azure AD rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					Azure: &v1alpha2.AccessGroupAzureRule{
						ID:                 "azure-group-id",
						IdentityProviderID: "azure-idp-id",
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].Azure)
				assert.Equal(t, "azure-group-id", params[0].Azure.ID)
				assert.Equal(t, "azure-idp-id", params[0].Azure.IdentityProviderID)
			},
		},
		{
			name: "Okta rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					Okta: &v1alpha2.AccessGroupOktaRule{
						Name:               "okta-group-name",
						IdentityProviderID: "okta-idp-id",
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].Okta)
				assert.Equal(t, "okta-group-name", params[0].Okta.Name)
				assert.Equal(t, "okta-idp-id", params[0].Okta.IdentityProviderID)
			},
		},
		{
			name: "OIDC claim rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					OIDC: &v1alpha2.AccessGroupOIDCRule{
						ClaimName:          "groups",
						ClaimValue:         "admin",
						IdentityProviderID: "oidc-idp-id",
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].OIDC)
				assert.Equal(t, "groups", params[0].OIDC.ClaimName)
				assert.Equal(t, "admin", params[0].OIDC.ClaimValue)
				assert.Equal(t, "oidc-idp-id", params[0].OIDC.IdentityProviderID)
			},
		},
		{
			name: "SAML attribute rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					SAML: &v1alpha2.AccessGroupSAMLRule{
						AttributeName:      "department",
						AttributeValue:     "engineering",
						IdentityProviderID: "saml-idp-id",
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].SAML)
				assert.Equal(t, "department", params[0].SAML.AttributeName)
				assert.Equal(t, "engineering", params[0].SAML.AttributeValue)
				assert.Equal(t, "saml-idp-id", params[0].SAML.IdentityProviderID)
			},
		},
		{
			name: "auth method rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					AuthMethod: &v1alpha2.AccessGroupAuthMethodRule{
						AuthMethod: "mfa",
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].AuthMethod)
				assert.Equal(t, "mfa", params[0].AuthMethod.AuthMethod)
			},
		},
		{
			name: "auth context rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					AuthContext: &v1alpha2.AccessGroupAuthContextRule{
						ID:                 "auth-context-id",
						AcID:               "ac-idp-id",
						IdentityProviderID: "auth-idp-id",
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].AuthContext)
				assert.Equal(t, "auth-context-id", params[0].AuthContext.ID)
				assert.Equal(t, "ac-idp-id", params[0].AuthContext.AcID)
				assert.Equal(t, "auth-idp-id", params[0].AuthContext.IdentityProviderID)
			},
		},
		{
			name: "login method rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					LoginMethod: &v1alpha2.AccessGroupLoginMethodRule{
						ID: "login-method-id",
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].LoginMethod)
				assert.Equal(t, "login-method-id", params[0].LoginMethod.ID)
			},
		},
		{
			name: "external evaluation rule",
			rules: []v1alpha2.AccessGroupRule{
				{
					ExternalEvaluation: &v1alpha2.AccessGroupExternalEvaluationRule{
						EvaluateURL: "https://eval.example.com/check",
						KeysURL:     "https://eval.example.com/.well-known/jwks.json",
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.NotNil(t, params[0].ExternalEvaluation)
				assert.Equal(t, "https://eval.example.com/check", params[0].ExternalEvaluation.EvaluateURL)
				assert.Equal(t, "https://eval.example.com/.well-known/jwks.json", params[0].ExternalEvaluation.KeysURL)
			},
		},
		{
			name: "multiple rules",
			rules: []v1alpha2.AccessGroupRule{
				{Everyone: true},
				{
					Email: &v1alpha2.AccessGroupEmailRule{
						Email: "admin@example.com",
					},
				},
				{
					EmailDomain: &v1alpha2.AccessGroupEmailDomainRule{
						Domain: "example.com",
					},
				},
			},
			expected: 3,
			verify: func(t *testing.T, params []cf.AccessGroupRuleParams) {
				assert.True(t, params[0].Everyone)
				assert.NotNil(t, params[1].Email)
				assert.NotNil(t, params[2].EmailDomain)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertGroupRules(tt.rules)
			assert.Len(t, result, tt.expected)
			if tt.verify != nil {
				tt.verify(t, result)
			}
		})
	}
}

// TestAccessPolicyConfigHasInlineRules tests the HasInlineRules method.
func TestAccessPolicyConfigHasInlineRules(t *testing.T) {
	tests := []struct {
		name     string
		config   accesssvc.AccessPolicyConfig
		expected bool
	}{
		{
			name:     "empty config",
			config:   accesssvc.AccessPolicyConfig{},
			expected: false,
		},
		{
			name: "config with only GroupID",
			config: accesssvc.AccessPolicyConfig{
				GroupID: "group-123",
			},
			expected: false,
		},
		{
			name: "config with Include rules",
			config: accesssvc.AccessPolicyConfig{
				Include: []v1alpha2.AccessGroupRule{
					{Everyone: true},
				},
			},
			expected: true,
		},
		{
			name: "config with Exclude rules only",
			config: accesssvc.AccessPolicyConfig{
				Exclude: []v1alpha2.AccessGroupRule{
					{Email: &v1alpha2.AccessGroupEmailRule{Email: "blocked@example.com"}},
				},
			},
			expected: true,
		},
		{
			name: "config with Require rules only",
			config: accesssvc.AccessPolicyConfig{
				Require: []v1alpha2.AccessGroupRule{
					{Certificate: true},
				},
			},
			expected: true,
		},
		{
			name: "config with all rule types",
			config: accesssvc.AccessPolicyConfig{
				Include: []v1alpha2.AccessGroupRule{{Everyone: true}},
				Exclude: []v1alpha2.AccessGroupRule{{Email: &v1alpha2.AccessGroupEmailRule{Email: "blocked@example.com"}}},
				Require: []v1alpha2.AccessGroupRule{{Certificate: true}},
			},
			expected: true,
		},
		{
			name: "config with empty rule slices",
			config: accesssvc.AccessPolicyConfig{
				Include: []v1alpha2.AccessGroupRule{},
				Exclude: []v1alpha2.AccessGroupRule{},
				Require: []v1alpha2.AccessGroupRule{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.HasInlineRules()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestAccessPolicyConfigHasGroupReference tests the HasGroupReference method.
func TestAccessPolicyConfigHasGroupReference(t *testing.T) {
	tests := []struct {
		name     string
		config   accesssvc.AccessPolicyConfig
		expected bool
	}{
		{
			name:     "empty config",
			config:   accesssvc.AccessPolicyConfig{},
			expected: false,
		},
		{
			name: "config with GroupID",
			config: accesssvc.AccessPolicyConfig{
				GroupID: "group-123",
			},
			expected: true,
		},
		{
			name: "config with CloudflareGroupID",
			config: accesssvc.AccessPolicyConfig{
				CloudflareGroupID: "cf-group-123",
			},
			expected: true,
		},
		{
			name: "config with CloudflareGroupName",
			config: accesssvc.AccessPolicyConfig{
				CloudflareGroupName: "My Group",
			},
			expected: true,
		},
		{
			name: "config with K8sAccessGroupName",
			config: accesssvc.AccessPolicyConfig{
				K8sAccessGroupName: "k8s-group",
			},
			expected: true,
		},
		{
			name: "config with only inline rules",
			config: accesssvc.AccessPolicyConfig{
				Include: []v1alpha2.AccessGroupRule{
					{Everyone: true},
				},
			},
			expected: false,
		},
		{
			name: "config with both inline rules and group reference",
			config: accesssvc.AccessPolicyConfig{
				GroupID: "group-123",
				Include: []v1alpha2.AccessGroupRule{
					{Everyone: true},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.HasGroupReference()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestNilIfEmpty tests the nilIfEmpty helper function.
func TestNilIfEmpty(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectNil bool
	}{
		{
			name:      "empty string",
			input:     "",
			expectNil: true,
		},
		{
			name:      "non-empty string",
			input:     "24h",
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nilIfEmpty(tt.input)
			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.input, *result)
			}
		})
	}
}

// TestBoolPtr tests the boolPtr helper function.
func TestBoolPtr(t *testing.T) {
	tests := []struct {
		name      string
		input     bool
		expectNil bool
	}{
		{
			name:      "false value",
			input:     false,
			expectNil: true,
		},
		{
			name:      "true value",
			input:     true,
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := boolPtr(tt.input)
			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.True(t, *result)
			}
		})
	}
}

// categorizePolicies separates policies into inline and group reference modes.
func categorizePolicies(policies []accesssvc.AccessPolicyConfig) (inline, groupRef []accesssvc.AccessPolicyConfig) {
	for _, policy := range policies {
		if policy.HasInlineRules() {
			inline = append(inline, policy)
		} else {
			groupRef = append(groupRef, policy)
		}
	}
	return inline, groupRef
}

// TestSyncPoliciesModeDetection tests that the sync controller correctly detects inline vs group reference mode.
func TestSyncPoliciesModeDetection(t *testing.T) {
	t.Run("empty policies", func(t *testing.T) {
		inline, groupRef := categorizePolicies(nil)
		assert.Empty(t, inline)
		assert.Empty(t, groupRef)
	})

	t.Run("all inline policies", func(t *testing.T) {
		policies := []accesssvc.AccessPolicyConfig{
			{Decision: "allow", Precedence: 1, Include: []v1alpha2.AccessGroupRule{{Everyone: true}}},
			{
				Decision:   "deny",
				Precedence: 2,
				Include:    []v1alpha2.AccessGroupRule{{Email: &v1alpha2.AccessGroupEmailRule{Email: "test@example.com"}}},
			},
		}
		inline, groupRef := categorizePolicies(policies)
		assert.Len(t, inline, 2)
		assert.Empty(t, groupRef)
	})

	t.Run("all group reference policies", func(t *testing.T) {
		policies := []accesssvc.AccessPolicyConfig{
			{Decision: "allow", Precedence: 1, CloudflareGroupID: "cf-group-1"},
			{Decision: "allow", Precedence: 2, GroupID: "group-2"},
		}
		inline, groupRef := categorizePolicies(policies)
		assert.Empty(t, inline)
		assert.Len(t, groupRef, 2)
	})

	t.Run("mixed policies", func(t *testing.T) {
		policies := []accesssvc.AccessPolicyConfig{
			{Decision: "allow", Precedence: 1, Include: []v1alpha2.AccessGroupRule{{Everyone: true}}},
			{Decision: "allow", Precedence: 2, CloudflareGroupID: "cf-group-1"},
			{
				Decision:   "deny",
				Precedence: 3,
				Exclude:    []v1alpha2.AccessGroupRule{{Country: &v1alpha2.AccessGroupCountryRule{Country: []string{"CN"}}}},
			},
			{Decision: "allow", Precedence: 4, CloudflareGroupName: "Admin Group"},
		}
		inline, groupRef := categorizePolicies(policies)
		assert.Len(t, inline, 2)
		assert.Len(t, groupRef, 2)
	})
}

// TestBuildPolicyParams tests the policy params building for both modes.
func TestBuildPolicyParams(t *testing.T) {
	controller := &ApplicationController{}

	tests := []struct {
		name   string
		policy accesssvc.AccessPolicyConfig
		verify func(t *testing.T, params cf.AccessPolicyParams)
	}{
		{
			name: "inline rules mode - email domain",
			policy: accesssvc.AccessPolicyConfig{
				Decision:        "allow",
				Precedence:      1,
				PolicyName:      "Email Domain Policy",
				SessionDuration: "24h",
				Include: []v1alpha2.AccessGroupRule{
					{
						EmailDomain: &v1alpha2.AccessGroupEmailDomainRule{
							Domain: "example.com",
						},
					},
				},
			},
			verify: func(t *testing.T, params cf.AccessPolicyParams) {
				assert.Equal(t, "allow", params.Decision)
				assert.Equal(t, 1, params.Precedence)
				assert.Equal(t, "Email Domain Policy", params.Name)
				assert.NotNil(t, params.SessionDuration)
				assert.Equal(t, "24h", *params.SessionDuration)
				assert.Len(t, params.Include, 1)
				assert.NotNil(t, params.Include[0].EmailDomain)
				assert.Equal(t, "example.com", params.Include[0].EmailDomain.Domain)
			},
		},
		{
			name: "inline rules mode - all rule sections",
			policy: accesssvc.AccessPolicyConfig{
				Decision:   "allow",
				Precedence: 2,
				Include: []v1alpha2.AccessGroupRule{
					{Everyone: true},
				},
				Exclude: []v1alpha2.AccessGroupRule{
					{
						Email: &v1alpha2.AccessGroupEmailRule{
							Email: "blocked@example.com",
						},
					},
				},
				Require: []v1alpha2.AccessGroupRule{
					{Certificate: true},
				},
			},
			verify: func(t *testing.T, params cf.AccessPolicyParams) {
				assert.Len(t, params.Include, 1)
				assert.True(t, params.Include[0].Everyone)
				assert.Len(t, params.Exclude, 1)
				assert.NotNil(t, params.Exclude[0].Email)
				assert.Len(t, params.Require, 1)
				assert.True(t, params.Require[0].Certificate)
			},
		},
		{
			name: "group reference mode",
			policy: accesssvc.AccessPolicyConfig{
				Decision:   "allow",
				Precedence: 3,
				GroupID:    "group-123",
				GroupName:  "Admin Group",
			},
			verify: func(t *testing.T, params cf.AccessPolicyParams) {
				assert.Len(t, params.Include, 1)
				assert.NotNil(t, params.Include[0].Group)
				assert.Equal(t, "group-123", params.Include[0].Group.ID)
				assert.Equal(t, "Admin Group - allow", params.Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policyName := controller.getPolicyName(tt.policy)
			params := cf.AccessPolicyParams{
				ApplicationID:   "app-123",
				Name:            policyName,
				Decision:        tt.policy.Decision,
				Precedence:      tt.policy.Precedence,
				SessionDuration: nilIfEmpty(tt.policy.SessionDuration),
			}

			// Build rules based on mode
			if tt.policy.HasInlineRules() {
				params.Include = convertGroupRules(tt.policy.Include)
				params.Exclude = convertGroupRules(tt.policy.Exclude)
				params.Require = convertGroupRules(tt.policy.Require)
			} else if tt.policy.GroupID != "" {
				params.Include = []cf.AccessGroupRuleParams{cf.BuildGroupIncludeRule(tt.policy.GroupID)}
			}

			tt.verify(t, params)
		})
	}
}
