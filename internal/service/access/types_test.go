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
