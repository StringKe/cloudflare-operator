// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:revive // cognitive-complexity: table-driven test with many test cases
func TestConvertRuleToSDK(t *testing.T) {
	tests := []struct {
		name     string
		rule     AccessGroupRuleParams
		wantKeys []string
		validate func(t *testing.T, result map[string]interface{})
	}{
		{
			name: "email rule",
			rule: AccessGroupRuleParams{
				Email: &AccessGroupEmailRuleParams{Email: "test@example.com"},
			},
			wantKeys: []string{"email"},
			validate: func(t *testing.T, result map[string]interface{}) {
				email := result["email"].(map[string]string)
				assert.Equal(t, "test@example.com", email["email"])
			},
		},
		{
			name: "email domain rule",
			rule: AccessGroupRuleParams{
				EmailDomain: &AccessGroupEmailDomainRuleParams{Domain: "example.com"},
			},
			wantKeys: []string{"email_domain"},
			validate: func(t *testing.T, result map[string]interface{}) {
				domain := result["email_domain"].(map[string]string)
				assert.Equal(t, "example.com", domain["domain"])
			},
		},
		{
			name: "email list rule",
			rule: AccessGroupRuleParams{
				EmailList: &AccessGroupEmailListRuleParams{ID: "list-123"},
			},
			wantKeys: []string{"email_list"},
			validate: func(t *testing.T, result map[string]interface{}) {
				list := result["email_list"].(map[string]string)
				assert.Equal(t, "list-123", list["id"])
			},
		},
		{
			name: "everyone rule",
			rule: AccessGroupRuleParams{
				Everyone: true,
			},
			wantKeys: []string{"everyone"},
			validate: func(t *testing.T, result map[string]interface{}) {
				_, ok := result["everyone"]
				assert.True(t, ok)
			},
		},
		{
			name: "IP ranges rule",
			rule: AccessGroupRuleParams{
				IPRanges: &AccessGroupIPRangesRuleParams{IP: []string{"192.168.1.0/24", "10.0.0.0/8"}},
			},
			wantKeys: []string{"ip"},
			validate: func(t *testing.T, result map[string]interface{}) {
				ip := result["ip"].(map[string]string)
				assert.Equal(t, "192.168.1.0/24", ip["ip"])
			},
		},
		{
			name: "IP list rule",
			rule: AccessGroupRuleParams{
				IPList: &AccessGroupIPListRuleParams{ID: "ip-list-456"},
			},
			wantKeys: []string{"ip_list"},
			validate: func(t *testing.T, result map[string]interface{}) {
				list := result["ip_list"].(map[string]string)
				assert.Equal(t, "ip-list-456", list["id"])
			},
		},
		{
			name: "country rule",
			rule: AccessGroupRuleParams{
				Country: &AccessGroupCountryRuleParams{Country: []string{"US", "CA"}},
			},
			wantKeys: []string{"geo"},
			validate: func(t *testing.T, result map[string]interface{}) {
				geo := result["geo"].(map[string]string)
				assert.Equal(t, "US", geo["country_code"])
			},
		},
		{
			name: "group rule",
			rule: AccessGroupRuleParams{
				Group: &AccessGroupGroupRuleParams{ID: "group-789"},
			},
			wantKeys: []string{"group"},
			validate: func(t *testing.T, result map[string]interface{}) {
				group := result["group"].(map[string]string)
				assert.Equal(t, "group-789", group["id"])
			},
		},
		{
			name: "service token rule",
			rule: AccessGroupRuleParams{
				ServiceToken: &AccessGroupServiceTokenRuleParams{TokenID: "token-abc"},
			},
			wantKeys: []string{"service_token"},
			validate: func(t *testing.T, result map[string]interface{}) {
				token := result["service_token"].(map[string]string)
				assert.Equal(t, "token-abc", token["token_id"])
			},
		},
		{
			name: "any valid service token rule",
			rule: AccessGroupRuleParams{
				AnyValidServiceToken: true,
			},
			wantKeys: []string{"any_valid_service_token"},
			validate: func(t *testing.T, result map[string]interface{}) {
				_, ok := result["any_valid_service_token"]
				assert.True(t, ok)
			},
		},
		{
			name: "certificate rule",
			rule: AccessGroupRuleParams{
				Certificate: true,
			},
			wantKeys: []string{"certificate"},
			validate: func(t *testing.T, result map[string]interface{}) {
				_, ok := result["certificate"]
				assert.True(t, ok)
			},
		},
		{
			name: "common name rule",
			rule: AccessGroupRuleParams{
				CommonName: &AccessGroupCommonNameRuleParams{CommonName: "example.com"},
			},
			wantKeys: []string{"common_name"},
			validate: func(t *testing.T, result map[string]interface{}) {
				cn := result["common_name"].(map[string]string)
				assert.Equal(t, "example.com", cn["common_name"])
			},
		},
		{
			name: "device posture rule",
			rule: AccessGroupRuleParams{
				DevicePosture: &AccessGroupDevicePostureRuleParams{IntegrationUID: "int-123"},
			},
			wantKeys: []string{"device_posture"},
			validate: func(t *testing.T, result map[string]interface{}) {
				dp := result["device_posture"].(map[string]string)
				assert.Equal(t, "int-123", dp["integration_uid"])
			},
		},
		{
			name: "GSuite rule",
			rule: AccessGroupRuleParams{
				GSuite: &AccessGroupGSuiteRuleParams{
					Email:              "admin@gsuite.example.com",
					IdentityProviderID: "idp-gsuite",
				},
			},
			wantKeys: []string{"gsuite"},
			validate: func(t *testing.T, result map[string]interface{}) {
				gs := result["gsuite"].(map[string]interface{})
				assert.Equal(t, "admin@gsuite.example.com", gs["email"])
				assert.Equal(t, "idp-gsuite", gs["identity_provider_id"])
			},
		},
		{
			name: "GitHub rule with teams",
			rule: AccessGroupRuleParams{
				GitHub: &AccessGroupGitHubRuleParams{
					Name:               "my-org",
					Teams:              []string{"team-a", "team-b"},
					IdentityProviderID: "idp-github",
				},
			},
			wantKeys: []string{"github_organization"},
			validate: func(t *testing.T, result map[string]interface{}) {
				gh := result["github_organization"].(map[string]interface{})
				assert.Equal(t, "my-org", gh["name"])
				assert.Equal(t, "idp-github", gh["identity_provider_id"])
				assert.Equal(t, []string{"team-a", "team-b"}, gh["teams"])
			},
		},
		{
			name: "GitHub rule without teams",
			rule: AccessGroupRuleParams{
				GitHub: &AccessGroupGitHubRuleParams{
					Name:               "my-org",
					IdentityProviderID: "idp-github",
				},
			},
			wantKeys: []string{"github_organization"},
			validate: func(t *testing.T, result map[string]interface{}) {
				gh := result["github_organization"].(map[string]interface{})
				assert.Equal(t, "my-org", gh["name"])
				_, hasTeams := gh["teams"]
				assert.False(t, hasTeams)
			},
		},
		{
			name: "Azure AD rule",
			rule: AccessGroupRuleParams{
				Azure: &AccessGroupAzureRuleParams{
					ID:                 "azure-group-id",
					IdentityProviderID: "idp-azure",
				},
			},
			wantKeys: []string{"azure_ad"},
			validate: func(t *testing.T, result map[string]interface{}) {
				az := result["azure_ad"].(map[string]interface{})
				assert.Equal(t, "azure-group-id", az["id"])
				assert.Equal(t, "idp-azure", az["identity_provider_id"])
			},
		},
		{
			name: "Okta rule",
			rule: AccessGroupRuleParams{
				Okta: &AccessGroupOktaRuleParams{
					Name:               "okta-group",
					IdentityProviderID: "idp-okta",
				},
			},
			wantKeys: []string{"okta"},
			validate: func(t *testing.T, result map[string]interface{}) {
				ok := result["okta"].(map[string]interface{})
				assert.Equal(t, "okta-group", ok["name"])
				assert.Equal(t, "idp-okta", ok["identity_provider_id"])
			},
		},
		{
			name: "OIDC rule",
			rule: AccessGroupRuleParams{
				OIDC: &AccessGroupOIDCRuleParams{
					ClaimName:          "groups",
					ClaimValue:         "admins",
					IdentityProviderID: "idp-oidc",
				},
			},
			wantKeys: []string{"oidc"},
			validate: func(t *testing.T, result map[string]interface{}) {
				oidc := result["oidc"].(map[string]interface{})
				assert.Equal(t, "groups", oidc["claim_name"])
				assert.Equal(t, "admins", oidc["claim_value"])
				assert.Equal(t, "idp-oidc", oidc["identity_provider_id"])
			},
		},
		{
			name: "SAML rule",
			rule: AccessGroupRuleParams{
				SAML: &AccessGroupSAMLRuleParams{
					AttributeName:      "department",
					AttributeValue:     "engineering",
					IdentityProviderID: "idp-saml",
				},
			},
			wantKeys: []string{"saml"},
			validate: func(t *testing.T, result map[string]interface{}) {
				saml := result["saml"].(map[string]interface{})
				assert.Equal(t, "department", saml["attribute_name"])
				assert.Equal(t, "engineering", saml["attribute_value"])
				assert.Equal(t, "idp-saml", saml["identity_provider_id"])
			},
		},
		{
			name: "auth method rule",
			rule: AccessGroupRuleParams{
				AuthMethod: &AccessGroupAuthMethodRuleParams{AuthMethod: "mfa"},
			},
			wantKeys: []string{"auth_method"},
			validate: func(t *testing.T, result map[string]interface{}) {
				am := result["auth_method"].(map[string]string)
				assert.Equal(t, "mfa", am["auth_method"])
			},
		},
		{
			name: "auth context rule",
			rule: AccessGroupRuleParams{
				AuthContext: &AccessGroupAuthContextRuleParams{
					ID:                 "ac-id",
					AcID:               "ac-id-123",
					IdentityProviderID: "idp-ac",
				},
			},
			wantKeys: []string{"auth_context"},
			validate: func(t *testing.T, result map[string]interface{}) {
				ac := result["auth_context"].(map[string]interface{})
				assert.Equal(t, "ac-id", ac["id"])
				assert.Equal(t, "ac-id-123", ac["ac_id"])
				assert.Equal(t, "idp-ac", ac["identity_provider_id"])
			},
		},
		{
			name: "login method rule",
			rule: AccessGroupRuleParams{
				LoginMethod: &AccessGroupLoginMethodRuleParams{ID: "login-method-id"},
			},
			wantKeys: []string{"login_method"},
			validate: func(t *testing.T, result map[string]interface{}) {
				lm := result["login_method"].(map[string]string)
				assert.Equal(t, "login-method-id", lm["id"])
			},
		},
		{
			name: "external evaluation rule",
			rule: AccessGroupRuleParams{
				ExternalEvaluation: &AccessGroupExternalEvaluationRuleParams{
					EvaluateURL: "https://eval.example.com",
					KeysURL:     "https://keys.example.com",
				},
			},
			wantKeys: []string{"external_evaluation"},
			validate: func(t *testing.T, result map[string]interface{}) {
				ee := result["external_evaluation"].(map[string]string)
				assert.Equal(t, "https://eval.example.com", ee["evaluate_url"])
				assert.Equal(t, "https://keys.example.com", ee["keys_url"])
			},
		},
		{
			name: "empty rule",
			rule: AccessGroupRuleParams{},
			validate: func(t *testing.T, result map[string]interface{}) {
				assert.Empty(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertRuleToSDK(tt.rule)

			for _, key := range tt.wantKeys {
				assert.Contains(t, result, key, "Result should contain key %s", key)
			}

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestConvertRulesToSDK(t *testing.T) {
	tests := []struct {
		name      string
		rules     []AccessGroupRuleParams
		wantCount int
	}{
		{
			name:      "nil rules",
			rules:     nil,
			wantCount: 0,
		},
		{
			name:      "empty rules",
			rules:     []AccessGroupRuleParams{},
			wantCount: 0,
		},
		{
			name: "single rule",
			rules: []AccessGroupRuleParams{
				{Email: &AccessGroupEmailRuleParams{Email: "test@example.com"}},
			},
			wantCount: 1,
		},
		{
			name: "multiple rules",
			rules: []AccessGroupRuleParams{
				{Email: &AccessGroupEmailRuleParams{Email: "test@example.com"}},
				{EmailDomain: &AccessGroupEmailDomainRuleParams{Domain: "example.com"}},
				{Everyone: true},
			},
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertRulesToSDK(tt.rules)

			if tt.wantCount == 0 {
				assert.Nil(t, result)
			} else {
				assert.Len(t, result, tt.wantCount)
			}
		})
	}
}

func TestBuildGroupIncludeRule(t *testing.T) {
	groupID := "group-test-123"

	result := BuildGroupIncludeRule(groupID)

	assert.NotNil(t, result.Group)
	assert.Equal(t, groupID, result.Group.ID)

	// Ensure other fields are nil/empty
	assert.Nil(t, result.Email)
	assert.Nil(t, result.EmailDomain)
	assert.Nil(t, result.EmailList)
	assert.Nil(t, result.IPRanges)
	assert.Nil(t, result.IPList)
	assert.Nil(t, result.Country)
	assert.False(t, result.Everyone)
}

func TestDevicePostureMatchParams(t *testing.T) {
	tests := []struct {
		name   string
		params DevicePostureMatchParams
	}{
		{
			name: "platform match",
			params: DevicePostureMatchParams{
				Platform: "windows",
			},
		},
		{
			name: "full match params",
			params: DevicePostureMatchParams{
				Platform: "macos",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.params.Platform)
		})
	}
}

func TestDevicePostureInputParams(t *testing.T) {
	tests := []struct {
		name   string
		params DevicePostureInputParams
	}{
		{
			name: "disk encryption check",
			params: DevicePostureInputParams{
				ID:               "disk-enc-check",
				CheckDisks:       []string{"C:", "D:"},
				RequireAll:       boolPtrAccess(true),
				Enabled:          boolPtrAccess(true),
				Exists:           boolPtrAccess(true),
				Path:             "/path/to/check",
				Thumbprint:       "abc123",
				Sha256:           "sha256hash",
				Running:          boolPtrAccess(true),
				Version:          "1.0.0",
				Operator:         ">=",
				Domain:           "example.com",
				ComplianceStatus: "compliant",
				ConnectionID:     "conn-123",
				LastSeen:         "2024-01-01",
				EidLastSeen:      "2024-01-01",
				OS:               "Windows 11",
				OSDistroName:     "Microsoft",
				OSDistroRevision: "22H2",
				Overall:          "healthy",
				SensorConfig:     "standard",
				State:            "active",
				VersionOperator:  ">=",
				CountOperator:    ">=",
				IssueCount:       intPtrAccess(0),
				CertificateID:    "cert-123",
				CommonName:       "example.com",
				ActiveThreats:    intPtrAccess(0),
				NetworkStatus:    "connected",
				Infected:         boolPtrAccess(false),
				IsActive:         boolPtrAccess(true),
				TotalScore:       intPtrAccess(100),
				RiskLevel:        "low",
				Score:            intPtrAccess(100),
				ScoreOperator:    ">=",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.params.ID)
		})
	}
}

// Helper functions for access tests
func boolPtrAccess(b bool) *bool {
	return &b
}

func intPtrAccess(i int) *int {
	return &i
}

func TestAccessGroupRuleParamsAllFields(t *testing.T) {
	// Test that all rule types can be properly instantiated
	rule := AccessGroupRuleParams{
		Email:                &AccessGroupEmailRuleParams{Email: "test@example.com"},
		EmailDomain:          &AccessGroupEmailDomainRuleParams{Domain: "example.com"},
		EmailList:            &AccessGroupEmailListRuleParams{ID: "list-1"},
		Everyone:             true,
		IPRanges:             &AccessGroupIPRangesRuleParams{IP: []string{"10.0.0.0/8"}},
		IPList:               &AccessGroupIPListRuleParams{ID: "ip-list-1"},
		Country:              &AccessGroupCountryRuleParams{Country: []string{"US"}},
		Group:                &AccessGroupGroupRuleParams{ID: "group-1"},
		ServiceToken:         &AccessGroupServiceTokenRuleParams{TokenID: "token-1"},
		AnyValidServiceToken: true,
		Certificate:          true,
		CommonName:           &AccessGroupCommonNameRuleParams{CommonName: "cn.example.com"},
		DevicePosture:        &AccessGroupDevicePostureRuleParams{IntegrationUID: "int-1"},
		GSuite: &AccessGroupGSuiteRuleParams{
			Email:              "admin@gsuite.com",
			IdentityProviderID: "idp-1",
		},
		GitHub: &AccessGroupGitHubRuleParams{
			Name:               "org",
			Teams:              []string{"team"},
			IdentityProviderID: "idp-2",
		},
		Azure: &AccessGroupAzureRuleParams{
			ID:                 "az-group",
			IdentityProviderID: "idp-3",
		},
		Okta: &AccessGroupOktaRuleParams{
			Name:               "okta-group",
			IdentityProviderID: "idp-4",
		},
		OIDC: &AccessGroupOIDCRuleParams{
			ClaimName:          "claim",
			ClaimValue:         "value",
			IdentityProviderID: "idp-5",
		},
		SAML: &AccessGroupSAMLRuleParams{
			AttributeName:      "attr",
			AttributeValue:     "val",
			IdentityProviderID: "idp-6",
		},
		AuthMethod: &AccessGroupAuthMethodRuleParams{AuthMethod: "mfa"},
		AuthContext: &AccessGroupAuthContextRuleParams{
			ID:                 "ac-id",
			AcID:               "ac-id-2",
			IdentityProviderID: "idp-7",
		},
		LoginMethod: &AccessGroupLoginMethodRuleParams{ID: "lm-1"},
		ExternalEvaluation: &AccessGroupExternalEvaluationRuleParams{
			EvaluateURL: "https://eval.example.com",
			KeysURL:     "https://keys.example.com",
		},
	}

	result := convertRuleToSDK(rule)
	require.NotEmpty(t, result)

	// Should have all non-nil rule types in the result
	expectedKeys := []string{
		"email", "email_domain", "email_list", "everyone",
		"ip", "ip_list", "geo", "group",
		"service_token", "any_valid_service_token", "certificate", "common_name",
		"device_posture", "gsuite", "github_organization", "azure_ad",
		"okta", "oidc", "saml", "auth_method", "auth_context",
		"login_method", "external_evaluation",
	}

	for _, key := range expectedKeys {
		assert.Contains(t, result, key, "Result should contain key %s", key)
	}
}

func TestAccessGroupParamsConversion(t *testing.T) {
	params := AccessGroupParams{
		Name: "test-group",
		Include: []AccessGroupRuleParams{
			{Email: &AccessGroupEmailRuleParams{Email: "include@example.com"}},
		},
		Exclude: []AccessGroupRuleParams{
			{EmailDomain: &AccessGroupEmailDomainRuleParams{Domain: "exclude.com"}},
		},
		Require: []AccessGroupRuleParams{
			{Everyone: true},
		},
	}

	assert.Equal(t, "test-group", params.Name)
	assert.Len(t, params.Include, 1)
	assert.Len(t, params.Exclude, 1)
	assert.Len(t, params.Require, 1)
}
