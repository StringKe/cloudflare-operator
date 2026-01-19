// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package access

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

func TestResourceTypeConstants(t *testing.T) {
	assert.Equal(t, v1alpha2.SyncResourceType("AccessApplication"), ResourceTypeAccessApplication)
	assert.Equal(t, v1alpha2.SyncResourceType("AccessGroup"), ResourceTypeAccessGroup)
	assert.Equal(t, v1alpha2.SyncResourceType("AccessServiceToken"), ResourceTypeAccessServiceToken)
	assert.Equal(t, v1alpha2.SyncResourceType("AccessIdentityProvider"), ResourceTypeAccessIdentityProvider)
}

func TestPriorityConstants(t *testing.T) {
	assert.Equal(t, 100, PriorityAccessApplication)
	assert.Equal(t, 100, PriorityAccessGroup)
	assert.Equal(t, 100, PriorityAccessServiceToken)
	assert.Equal(t, 100, PriorityAccessIdentityProvider)
}

func TestAccessApplicationConfig(t *testing.T) {
	config := AccessApplicationConfig{
		Name:                   "test-app",
		Domain:                 "app.example.com",
		SelfHostedDomains:      []string{"app1.example.com", "app2.example.com"},
		DomainType:             "public",
		Type:                   "self_hosted",
		SessionDuration:        "24h",
		AllowedIdps:            []string{"idp-1", "idp-2"},
		AutoRedirectToIdentity: true,
		LogoURL:                "https://example.com/logo.png",
		Tags:                   []string{"production", "web"},
	}

	assert.Equal(t, "test-app", config.Name)
	assert.Equal(t, "app.example.com", config.Domain)
	assert.Len(t, config.SelfHostedDomains, 2)
	assert.Equal(t, "public", config.DomainType)
	assert.Equal(t, "self_hosted", config.Type)
	assert.Equal(t, "24h", config.SessionDuration)
	assert.Len(t, config.AllowedIdps, 2)
	assert.True(t, config.AutoRedirectToIdentity)
}

func TestAccessApplicationConfigWithPolicies(t *testing.T) {
	config := AccessApplicationConfig{
		Name:   "test-app",
		Domain: "app.example.com",
		Type:   "self_hosted",
		Policies: []AccessPolicyConfig{
			{
				GroupID:    "group-1",
				GroupName:  "Admin Group",
				Decision:   "allow",
				Precedence: 1,
			},
			{
				GroupID:    "group-2",
				GroupName:  "User Group",
				Decision:   "allow",
				Precedence: 2,
			},
		},
	}

	assert.Len(t, config.Policies, 2)
	assert.Equal(t, "group-1", config.Policies[0].GroupID)
	assert.Equal(t, "allow", config.Policies[0].Decision)
	assert.Equal(t, 1, config.Policies[0].Precedence)
}

func TestAccessPolicyConfig(t *testing.T) {
	policies := []AccessPolicyConfig{
		{
			GroupID:         "group-allow",
			GroupName:       "Allow Group",
			Decision:        "allow",
			Precedence:      1,
			PolicyName:      "Allow Policy",
			SessionDuration: "12h",
		},
		{
			GroupID:    "group-deny",
			Decision:   "deny",
			Precedence: 2,
		},
		{
			GroupID:    "group-bypass",
			Decision:   "bypass",
			Precedence: 3,
		},
		{
			GroupID:    "group-non-identity",
			Decision:   "non_identity",
			Precedence: 4,
		},
	}

	assert.Equal(t, "allow", policies[0].Decision)
	assert.Equal(t, "deny", policies[1].Decision)
	assert.Equal(t, "bypass", policies[2].Decision)
	assert.Equal(t, "non_identity", policies[3].Decision)
}

func TestAccessGroupConfig(t *testing.T) {
	isDefault := true
	config := AccessGroupConfig{
		Name: "test-group",
		Include: []v1alpha2.AccessGroupRule{
			{
				Email: &v1alpha2.AccessGroupEmailRule{Email: "user@example.com"},
			},
		},
		Exclude: []v1alpha2.AccessGroupRule{
			{
				EmailDomain: &v1alpha2.AccessGroupEmailDomainRule{Domain: "external.com"},
			},
		},
		Require: []v1alpha2.AccessGroupRule{
			{
				AuthMethod: &v1alpha2.AccessGroupAuthMethodRule{AuthMethod: "mfa"},
			},
		},
		IsDefault: &isDefault,
	}

	assert.Equal(t, "test-group", config.Name)
	assert.Len(t, config.Include, 1)
	assert.Len(t, config.Exclude, 1)
	assert.Len(t, config.Require, 1)
	assert.True(t, *config.IsDefault)
}

func TestAccessServiceTokenConfig(t *testing.T) {
	config := AccessServiceTokenConfig{
		Name:     "test-token",
		Duration: "8760h",
		SecretRef: &SecretReference{
			Name:      "token-secret",
			Namespace: "default",
		},
	}

	assert.Equal(t, "test-token", config.Name)
	assert.Equal(t, "8760h", config.Duration)
	assert.NotNil(t, config.SecretRef)
	assert.Equal(t, "token-secret", config.SecretRef.Name)
	assert.Equal(t, "default", config.SecretRef.Namespace)
}

func TestSecretReference(t *testing.T) {
	tests := []struct {
		name     string
		ref      SecretReference
		wantName string
		wantNs   string
	}{
		{
			name:     "with namespace",
			ref:      SecretReference{Name: "secret-1", Namespace: "default"},
			wantName: "secret-1",
			wantNs:   "default",
		},
		{
			name:     "without namespace",
			ref:      SecretReference{Name: "secret-2"},
			wantName: "secret-2",
			wantNs:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantName, tt.ref.Name)
			assert.Equal(t, tt.wantNs, tt.ref.Namespace)
		})
	}
}

func TestAccessIdentityProviderConfig(t *testing.T) {
	config := AccessIdentityProviderConfig{
		Name: "test-idp",
		Type: "google",
		Config: &v1alpha2.IdentityProviderConfig{
			ClientID: "client-id",
		},
	}

	assert.Equal(t, "test-idp", config.Name)
	assert.Equal(t, "google", config.Type)
	assert.NotNil(t, config.Config)
	assert.Equal(t, "client-id", config.Config.ClientID)
}

func TestAccessApplicationRegisterOptions(t *testing.T) {
	opts := AccessApplicationRegisterOptions{
		AccountID:     "account-123",
		ApplicationID: "app-456",
		Source: service.Source{
			Kind:      "AccessApplication",
			Namespace: "default",
			Name:      "my-app",
		},
		Config: AccessApplicationConfig{
			Name:   "My App",
			Domain: "app.example.com",
			Type:   "self_hosted",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "creds"},
	}

	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "app-456", opts.ApplicationID)
	assert.Equal(t, "AccessApplication", opts.Source.Kind)
	assert.Equal(t, "My App", opts.Config.Name)
}

func TestAccessGroupRegisterOptions(t *testing.T) {
	opts := AccessGroupRegisterOptions{
		AccountID: "account-123",
		GroupID:   "group-456",
		Source: service.Source{
			Kind: "AccessGroup",
			Name: "my-group",
		},
		Config: AccessGroupConfig{
			Name: "My Group",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "creds"},
	}

	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "group-456", opts.GroupID)
	assert.Equal(t, "My Group", opts.Config.Name)
}

func TestAccessServiceTokenRegisterOptions(t *testing.T) {
	opts := AccessServiceTokenRegisterOptions{
		AccountID: "account-123",
		TokenID:   "token-456",
		Source: service.Source{
			Kind: "AccessServiceToken",
			Name: "my-token",
		},
		Config: AccessServiceTokenConfig{
			Name:     "My Token",
			Duration: "8760h",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "creds"},
	}

	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "token-456", opts.TokenID)
	assert.Equal(t, "My Token", opts.Config.Name)
}

func TestAccessIdentityProviderRegisterOptions(t *testing.T) {
	opts := AccessIdentityProviderRegisterOptions{
		AccountID:  "account-123",
		ProviderID: "idp-456",
		Source: service.Source{
			Kind: "AccessIdentityProvider",
			Name: "my-idp",
		},
		Config: AccessIdentityProviderConfig{
			Name: "My IdP",
			Type: "okta",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "creds"},
	}

	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "idp-456", opts.ProviderID)
	assert.Equal(t, "My IdP", opts.Config.Name)
	assert.Equal(t, "okta", opts.Config.Type)
}

func TestSyncResult(t *testing.T) {
	result := SyncResult{
		ID:        "resource-123",
		AccountID: "account-456",
	}

	assert.Equal(t, "resource-123", result.ID)
	assert.Equal(t, "account-456", result.AccountID)
}

func TestAccessApplicationSyncResult(t *testing.T) {
	result := AccessApplicationSyncResult{
		SyncResult: SyncResult{
			ID:        "app-123",
			AccountID: "account-456",
		},
		AUD:               "aud-789",
		Domain:            "app.example.com",
		SelfHostedDomains: []string{"app1.example.com", "app2.example.com"},
		SaasAppClientID:   "saas-client-id",
		ResolvedPolicies: []v1alpha2.ResolvedPolicyStatus{
			{
				PolicyID:   "policy-1",
				GroupID:    "group-1",
				GroupName:  "Admin",
				Decision:   "allow",
				Precedence: 1,
			},
		},
	}

	assert.Equal(t, "app-123", result.ID)
	assert.Equal(t, "aud-789", result.AUD)
	assert.Equal(t, "app.example.com", result.Domain)
	assert.Len(t, result.SelfHostedDomains, 2)
	assert.Len(t, result.ResolvedPolicies, 1)
}

func TestAccessServiceTokenSyncResult(t *testing.T) {
	result := AccessServiceTokenSyncResult{
		SyncResult: SyncResult{
			ID:        "token-123",
			AccountID: "account-456",
		},
		ClientID:            "client-id",
		ClientSecret:        "client-secret",
		ExpiresAt:           "2025-01-01T00:00:00Z",
		CreatedAt:           "2024-01-01T00:00:00Z",
		UpdatedAt:           "2024-06-01T00:00:00Z",
		LastSeenAt:          "2024-12-01T00:00:00Z",
		ClientSecretVersion: "v1",
	}

	assert.Equal(t, "token-123", result.ID)
	assert.Equal(t, "client-id", result.ClientID)
	assert.Equal(t, "client-secret", result.ClientSecret)
	assert.NotEmpty(t, result.ExpiresAt)
}

func TestAccessIdentityProviderSyncResult(t *testing.T) {
	result := AccessIdentityProviderSyncResult{
		SyncResult: SyncResult{
			ID:        "idp-123",
			AccountID: "account-456",
		},
	}

	assert.Equal(t, "idp-123", result.ID)
	assert.Equal(t, "account-456", result.AccountID)
}

// TestAccessPolicyConfigHasInlineRules tests the HasInlineRules method
func TestAccessPolicyConfigHasInlineRules(t *testing.T) {
	tests := []struct {
		name     string
		config   AccessPolicyConfig
		expected bool
	}{
		{
			name: "no rules - empty config",
			config: AccessPolicyConfig{
				Decision:   "allow",
				Precedence: 1,
			},
			expected: false,
		},
		{
			name: "group reference mode only",
			config: AccessPolicyConfig{
				GroupID:    "group-123",
				Decision:   "allow",
				Precedence: 1,
			},
			expected: false,
		},
		{
			name: "include rules only",
			config: AccessPolicyConfig{
				Decision:   "allow",
				Precedence: 1,
				Include: []v1alpha2.AccessGroupRule{
					{Email: &v1alpha2.AccessGroupEmailRule{Email: "user@example.com"}},
				},
			},
			expected: true,
		},
		{
			name: "exclude rules only",
			config: AccessPolicyConfig{
				Decision:   "allow",
				Precedence: 1,
				Exclude: []v1alpha2.AccessGroupRule{
					{EmailDomain: &v1alpha2.AccessGroupEmailDomainRule{Domain: "external.com"}},
				},
			},
			expected: true,
		},
		{
			name: "require rules only",
			config: AccessPolicyConfig{
				Decision:   "allow",
				Precedence: 1,
				Require: []v1alpha2.AccessGroupRule{
					{AuthMethod: &v1alpha2.AccessGroupAuthMethodRule{AuthMethod: "mfa"}},
				},
			},
			expected: true,
		},
		{
			name: "all inline rules",
			config: AccessPolicyConfig{
				Decision:   "allow",
				Precedence: 1,
				Include: []v1alpha2.AccessGroupRule{
					{Email: &v1alpha2.AccessGroupEmailRule{Email: "user@example.com"}},
				},
				Exclude: []v1alpha2.AccessGroupRule{
					{IPRanges: &v1alpha2.AccessGroupIPRangesRule{IP: []string{"10.0.0.0/8"}}},
				},
				Require: []v1alpha2.AccessGroupRule{
					{Certificate: true},
				},
			},
			expected: true,
		},
		{
			name: "inline rules with group reference (inline takes precedence)",
			config: AccessPolicyConfig{
				GroupID:    "group-123",
				Decision:   "allow",
				Precedence: 1,
				Include: []v1alpha2.AccessGroupRule{
					{Email: &v1alpha2.AccessGroupEmailRule{Email: "user@example.com"}},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.HasInlineRules()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestAccessPolicyConfigHasGroupReference tests the HasGroupReference method
func TestAccessPolicyConfigHasGroupReference(t *testing.T) {
	tests := []struct {
		name     string
		config   AccessPolicyConfig
		expected bool
	}{
		{
			name: "no reference - empty config",
			config: AccessPolicyConfig{
				Decision:   "allow",
				Precedence: 1,
			},
			expected: false,
		},
		{
			name: "GroupID reference",
			config: AccessPolicyConfig{
				GroupID:    "group-123",
				Decision:   "allow",
				Precedence: 1,
			},
			expected: true,
		},
		{
			name: "CloudflareGroupID reference",
			config: AccessPolicyConfig{
				CloudflareGroupID: "cf-group-456",
				Decision:          "allow",
				Precedence:        1,
			},
			expected: true,
		},
		{
			name: "CloudflareGroupName reference",
			config: AccessPolicyConfig{
				CloudflareGroupName: "My Cloudflare Group",
				Decision:            "allow",
				Precedence:          1,
			},
			expected: true,
		},
		{
			name: "K8sAccessGroupName reference",
			config: AccessPolicyConfig{
				K8sAccessGroupName: "my-k8s-group",
				Decision:           "allow",
				Precedence:         1,
			},
			expected: true,
		},
		{
			name: "inline rules only (no group reference)",
			config: AccessPolicyConfig{
				Decision:   "allow",
				Precedence: 1,
				Include: []v1alpha2.AccessGroupRule{
					{Email: &v1alpha2.AccessGroupEmailRule{Email: "user@example.com"}},
				},
			},
			expected: false,
		},
		{
			name: "both inline rules and group reference",
			config: AccessPolicyConfig{
				GroupID:    "group-123",
				Decision:   "allow",
				Precedence: 1,
				Include: []v1alpha2.AccessGroupRule{
					{Email: &v1alpha2.AccessGroupEmailRule{Email: "user@example.com"}},
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

// TestAccessPolicyConfigWithInlineRules tests AccessPolicyConfig with inline rules
func TestAccessPolicyConfigWithInlineRules(t *testing.T) {
	config := AccessPolicyConfig{
		Decision:        "allow",
		Precedence:      1,
		PolicyName:      "Allow Engineers",
		SessionDuration: "8h",
		Include: []v1alpha2.AccessGroupRule{
			{Email: &v1alpha2.AccessGroupEmailRule{Email: "admin@example.com"}},
			{EmailDomain: &v1alpha2.AccessGroupEmailDomainRule{Domain: "example.com"}},
			{Group: &v1alpha2.AccessGroupGroupRule{ID: "idp-group-123"}},
		},
		Exclude: []v1alpha2.AccessGroupRule{
			{IPRanges: &v1alpha2.AccessGroupIPRangesRule{IP: []string{"10.0.0.0/8"}}},
			{Country: &v1alpha2.AccessGroupCountryRule{Country: []string{"CN"}}},
		},
		Require: []v1alpha2.AccessGroupRule{
			{AuthMethod: &v1alpha2.AccessGroupAuthMethodRule{AuthMethod: "mfa"}},
			{Certificate: true},
		},
	}

	assert.Equal(t, "allow", config.Decision)
	assert.Equal(t, 1, config.Precedence)
	assert.Equal(t, "Allow Engineers", config.PolicyName)
	assert.Equal(t, "8h", config.SessionDuration)

	// Verify include rules
	assert.Len(t, config.Include, 3)
	assert.NotNil(t, config.Include[0].Email)
	assert.Equal(t, "admin@example.com", config.Include[0].Email.Email)
	assert.NotNil(t, config.Include[1].EmailDomain)
	assert.Equal(t, "example.com", config.Include[1].EmailDomain.Domain)
	assert.NotNil(t, config.Include[2].Group)
	assert.Equal(t, "idp-group-123", config.Include[2].Group.ID)

	// Verify exclude rules
	assert.Len(t, config.Exclude, 2)
	assert.NotNil(t, config.Exclude[0].IPRanges)
	assert.Equal(t, []string{"10.0.0.0/8"}, config.Exclude[0].IPRanges.IP)
	assert.NotNil(t, config.Exclude[1].Country)
	assert.Equal(t, []string{"CN"}, config.Exclude[1].Country.Country)

	// Verify require rules
	assert.Len(t, config.Require, 2)
	assert.NotNil(t, config.Require[0].AuthMethod)
	assert.Equal(t, "mfa", config.Require[0].AuthMethod.AuthMethod)
	assert.True(t, config.Require[1].Certificate)

	// Verify mode detection
	assert.True(t, config.HasInlineRules())
	assert.False(t, config.HasGroupReference())
}

// TestAccessPolicyConfigAllRuleTypes tests all supported rule types in inline mode
//
//nolint:revive // Test verifying all 23 rule types naturally has high cognitive complexity
func TestAccessPolicyConfigAllRuleTypes(t *testing.T) {
	config := AccessPolicyConfig{
		Decision:   "allow",
		Precedence: 1,
		Include: []v1alpha2.AccessGroupRule{
			// Basic rules
			{Email: &v1alpha2.AccessGroupEmailRule{Email: "user@example.com"}},
			{EmailDomain: &v1alpha2.AccessGroupEmailDomainRule{Domain: "example.com"}},
			{EmailList: &v1alpha2.AccessGroupEmailListRule{ID: "list-123"}},
			{Everyone: true},
			{IPRanges: &v1alpha2.AccessGroupIPRangesRule{IP: []string{"192.168.1.0/24"}}},
			{IPList: &v1alpha2.AccessGroupIPListRule{ID: "iplist-456"}},
			{Country: &v1alpha2.AccessGroupCountryRule{Country: []string{"US"}}},
			{Group: &v1alpha2.AccessGroupGroupRule{ID: "group-789"}},
			// Service tokens
			{ServiceToken: &v1alpha2.AccessGroupServiceTokenRule{TokenID: "token-abc"}},
			{AnyValidServiceToken: true},
			// Certificates
			{Certificate: true},
			{CommonName: &v1alpha2.AccessGroupCommonNameRule{CommonName: "*.example.com"}},
			// Device posture
			{DevicePosture: &v1alpha2.AccessGroupDevicePostureRule{IntegrationUID: "posture-123"}},
			// Identity providers
			{GSuite: &v1alpha2.AccessGroupGSuiteRule{Email: "user@gsuite.com", IdentityProviderID: "gsuite-idp"}},
			{GitHub: &v1alpha2.AccessGroupGitHubRule{Name: "org-name", IdentityProviderID: "github-idp"}},
			{Azure: &v1alpha2.AccessGroupAzureRule{ID: "azure-group-id", IdentityProviderID: "azure-idp"}},
			{Okta: &v1alpha2.AccessGroupOktaRule{Name: "okta-group", IdentityProviderID: "okta-idp"}},
			{OIDC: &v1alpha2.AccessGroupOIDCRule{ClaimName: "groups", ClaimValue: "engineers", IdentityProviderID: "oidc-idp"}},
			{SAML: &v1alpha2.AccessGroupSAMLRule{AttributeName: "role", AttributeValue: "admin", IdentityProviderID: "saml-idp"}},
			// Auth methods
			{AuthMethod: &v1alpha2.AccessGroupAuthMethodRule{AuthMethod: "mfa"}},
			{AuthContext: &v1alpha2.AccessGroupAuthContextRule{ID: "context-123", AcID: "acid-456", IdentityProviderID: "azure-idp"}},
			{LoginMethod: &v1alpha2.AccessGroupLoginMethodRule{ID: "login-method-789"}},
			// External evaluation
			{ExternalEvaluation: &v1alpha2.AccessGroupExternalEvaluationRule{
				EvaluateURL: "https://eval.example.com/check",
				KeysURL:     "https://eval.example.com/keys",
			}},
		},
	}

	assert.True(t, config.HasInlineRules())
	assert.Len(t, config.Include, 23)

	// Verify each rule type is present
	ruleTypes := make(map[string]bool)
	for _, rule := range config.Include {
		if rule.Email != nil {
			ruleTypes["email"] = true
		}
		if rule.EmailDomain != nil {
			ruleTypes["emailDomain"] = true
		}
		if rule.EmailList != nil {
			ruleTypes["emailList"] = true
		}
		if rule.Everyone {
			ruleTypes["everyone"] = true
		}
		if rule.IPRanges != nil {
			ruleTypes["ipRanges"] = true
		}
		if rule.IPList != nil {
			ruleTypes["ipList"] = true
		}
		if rule.Country != nil {
			ruleTypes["country"] = true
		}
		if rule.Group != nil {
			ruleTypes["group"] = true
		}
		if rule.ServiceToken != nil {
			ruleTypes["serviceToken"] = true
		}
		if rule.AnyValidServiceToken {
			ruleTypes["anyValidServiceToken"] = true
		}
		if rule.Certificate {
			ruleTypes["certificate"] = true
		}
		if rule.CommonName != nil {
			ruleTypes["commonName"] = true
		}
		if rule.DevicePosture != nil {
			ruleTypes["devicePosture"] = true
		}
		if rule.GSuite != nil {
			ruleTypes["gsuite"] = true
		}
		if rule.GitHub != nil {
			ruleTypes["github"] = true
		}
		if rule.Azure != nil {
			ruleTypes["azure"] = true
		}
		if rule.Okta != nil {
			ruleTypes["okta"] = true
		}
		if rule.OIDC != nil {
			ruleTypes["oidc"] = true
		}
		if rule.SAML != nil {
			ruleTypes["saml"] = true
		}
		if rule.AuthMethod != nil {
			ruleTypes["authMethod"] = true
		}
		if rule.AuthContext != nil {
			ruleTypes["authContext"] = true
		}
		if rule.LoginMethod != nil {
			ruleTypes["loginMethod"] = true
		}
		if rule.ExternalEvaluation != nil {
			ruleTypes["externalEvaluation"] = true
		}
	}

	// Verify all 23 rule types are present
	assert.Len(t, ruleTypes, 23)
}

// TestAccessApplicationConfigWithInlinePolicies tests the full AccessApplicationConfig with inline policies
func TestAccessApplicationConfigWithInlinePolicies(t *testing.T) {
	config := AccessApplicationConfig{
		Name:   "secure-app",
		Domain: "secure.example.com",
		Type:   "self_hosted",
		Policies: []AccessPolicyConfig{
			{
				Decision:   "allow",
				Precedence: 1,
				PolicyName: "Allow Admins",
				Include: []v1alpha2.AccessGroupRule{
					{Email: &v1alpha2.AccessGroupEmailRule{Email: "admin@example.com"}},
				},
				Require: []v1alpha2.AccessGroupRule{
					{AuthMethod: &v1alpha2.AccessGroupAuthMethodRule{AuthMethod: "mfa"}},
				},
			},
			{
				Decision:   "allow",
				Precedence: 2,
				PolicyName: "Allow Employees",
				Include: []v1alpha2.AccessGroupRule{
					{EmailDomain: &v1alpha2.AccessGroupEmailDomainRule{Domain: "example.com"}},
				},
				Exclude: []v1alpha2.AccessGroupRule{
					{IPRanges: &v1alpha2.AccessGroupIPRangesRule{IP: []string{"10.0.0.0/8"}}},
				},
			},
			{
				// Legacy group reference mode
				GroupID:    "external-group-123",
				Decision:   "allow",
				Precedence: 3,
			},
		},
	}

	assert.Equal(t, "secure-app", config.Name)
	assert.Len(t, config.Policies, 3)

	// First policy - inline with require
	assert.True(t, config.Policies[0].HasInlineRules())
	assert.False(t, config.Policies[0].HasGroupReference())
	assert.Equal(t, "Allow Admins", config.Policies[0].PolicyName)
	assert.Len(t, config.Policies[0].Include, 1)
	assert.Len(t, config.Policies[0].Require, 1)

	// Second policy - inline with exclude
	assert.True(t, config.Policies[1].HasInlineRules())
	assert.Equal(t, "Allow Employees", config.Policies[1].PolicyName)
	assert.Len(t, config.Policies[1].Include, 1)
	assert.Len(t, config.Policies[1].Exclude, 1)

	// Third policy - group reference mode
	assert.False(t, config.Policies[2].HasInlineRules())
	assert.True(t, config.Policies[2].HasGroupReference())
	assert.Equal(t, "external-group-123", config.Policies[2].GroupID)
}
