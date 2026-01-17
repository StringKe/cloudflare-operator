// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package ruleset

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

func TestResourceTypeConstants(t *testing.T) {
	assert.Equal(t, v1alpha2.SyncResourceZoneRuleset, ResourceTypeZoneRuleset)
	assert.Equal(t, v1alpha2.SyncResourceTransformRule, ResourceTypeTransformRule)
	assert.Equal(t, v1alpha2.SyncResourceRedirectRule, ResourceTypeRedirectRule)
}

func TestPriorityConstants(t *testing.T) {
	assert.Equal(t, 100, PriorityZoneRuleset)
	assert.Equal(t, 100, PriorityTransformRule)
	assert.Equal(t, 100, PriorityRedirectRule)
}

func TestZoneRulesetConfig(t *testing.T) {
	config := ZoneRulesetConfig{
		Zone:        "example.com",
		Phase:       "http_request_firewall_custom",
		Description: "Custom WAF rules",
		Rules: []RulesetRuleConfig{
			{
				Action:      "block",
				Expression:  "ip.src in {1.2.3.4}",
				Description: "Block bad IPs",
				Enabled:     true,
			},
		},
	}

	assert.Equal(t, "example.com", config.Zone)
	assert.Equal(t, "http_request_firewall_custom", config.Phase)
	assert.Len(t, config.Rules, 1)
}

func TestRulesetRuleConfig(t *testing.T) {
	rule := RulesetRuleConfig{
		Action:      "challenge",
		Expression:  "cf.threat_score > 50",
		Description: "Challenge high threat score",
		Enabled:     true,
		Ref:         "rule-ref-123",
	}

	assert.Equal(t, "challenge", rule.Action)
	assert.Equal(t, "cf.threat_score > 50", rule.Expression)
	assert.Equal(t, "Challenge high threat score", rule.Description)
	assert.True(t, rule.Enabled)
	assert.Equal(t, "rule-ref-123", rule.Ref)
}

func TestRulesetRuleActions(t *testing.T) {
	actions := []string{
		"block", "challenge", "js_challenge", "managed_challenge",
		"skip", "execute", "log", "rewrite",
	}

	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			rule := RulesetRuleConfig{
				Action:     action,
				Expression: "true",
			}
			assert.Equal(t, action, rule.Action)
		})
	}
}

func TestTransformRuleConfig(t *testing.T) {
	config := TransformRuleConfig{
		Zone:        "example.com",
		Type:        "url_rewrite",
		Description: "URL rewrite rules",
		Rules: []TransformRuleDefinitionConfig{
			{
				Name:       "rewrite-api",
				Expression: "http.request.uri.path starts_with \"/api/v1\"",
				Enabled:    true,
			},
		},
	}

	assert.Equal(t, "example.com", config.Zone)
	assert.Equal(t, "url_rewrite", config.Type)
	assert.Len(t, config.Rules, 1)
}

func TestTransformRuleTypes(t *testing.T) {
	types := []string{"url_rewrite", "request_header", "response_header"}

	for _, ruleType := range types {
		t.Run(ruleType, func(t *testing.T) {
			config := TransformRuleConfig{
				Zone: "example.com",
				Type: ruleType,
			}
			assert.Equal(t, ruleType, config.Type)
		})
	}
}

func TestTransformRuleDefinitionConfig(t *testing.T) {
	rule := TransformRuleDefinitionConfig{
		Name:       "transform-rule",
		Expression: "http.host eq \"example.com\"",
		Enabled:    true,
		URLRewrite: &v1alpha2.URLRewriteConfig{
			Path: &v1alpha2.RewriteValue{
				Static: "/new-path",
			},
		},
	}

	assert.Equal(t, "transform-rule", rule.Name)
	assert.True(t, rule.Enabled)
	assert.NotNil(t, rule.URLRewrite)
}

func TestTransformRuleDefinitionConfigWithHeaders(t *testing.T) {
	rule := TransformRuleDefinitionConfig{
		Name:       "header-rule",
		Expression: "true",
		Enabled:    true,
		Headers: []v1alpha2.HeaderModification{
			{
				Name:      "X-Custom-Header",
				Operation: v1alpha2.HeaderOperationSet,
				Value:     "custom-value",
			},
			{
				Name:      "X-Remove-Header",
				Operation: v1alpha2.HeaderOperationRemove,
			},
		},
	}

	assert.Len(t, rule.Headers, 2)
	assert.Equal(t, "X-Custom-Header", rule.Headers[0].Name)
	assert.Equal(t, v1alpha2.HeaderOperationSet, rule.Headers[0].Operation)
}

func TestRedirectRuleConfig(t *testing.T) {
	config := RedirectRuleConfig{
		Zone:        "example.com",
		Description: "Redirect rules",
		Rules: []RedirectRuleDefinitionConfig{
			{
				Name:       "redirect-old",
				Expression: "http.request.uri.path eq \"/old\"",
				Enabled:    true,
				Target: RedirectTargetConfig{
					URL: "https://example.com/new",
				},
				StatusCode: 301,
			},
		},
	}

	assert.Equal(t, "example.com", config.Zone)
	assert.Len(t, config.Rules, 1)
	assert.Equal(t, 301, config.Rules[0].StatusCode)
}

func TestRedirectRuleDefinitionConfig(t *testing.T) {
	rule := RedirectRuleDefinitionConfig{
		Name:                "permanent-redirect",
		Expression:          "http.request.uri.path starts_with \"/legacy\"",
		Enabled:             true,
		Target:              RedirectTargetConfig{URL: "https://new.example.com"},
		StatusCode:          301,
		PreserveQueryString: true,
	}

	assert.Equal(t, "permanent-redirect", rule.Name)
	assert.Equal(t, 301, rule.StatusCode)
	assert.True(t, rule.PreserveQueryString)
}

func TestRedirectStatusCodes(t *testing.T) {
	codes := []int{301, 302, 303, 307, 308}

	for _, code := range codes {
		t.Run(string(rune(code)), func(t *testing.T) {
			rule := RedirectRuleDefinitionConfig{
				Name:       "redirect",
				Expression: "true",
				Target:     RedirectTargetConfig{URL: "https://example.com"},
				StatusCode: code,
			}
			assert.Equal(t, code, rule.StatusCode)
		})
	}
}

//nolint:revive // cognitive complexity unavoidable: table-driven tests require comprehensive test cases
func TestRedirectTargetConfig(t *testing.T) {
	tests := []struct {
		name   string
		target RedirectTargetConfig
	}{
		{
			name: "static URL",
			target: RedirectTargetConfig{
				URL: "https://example.com/new-path",
			},
		},
		{
			name: "dynamic expression",
			target: RedirectTargetConfig{
				Expression: "concat(\"https://example.com\", http.request.uri.path)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.target.URL != "" {
				assert.NotEmpty(t, tt.target.URL)
			}
			if tt.target.Expression != "" {
				assert.NotEmpty(t, tt.target.Expression)
			}
		})
	}
}

func TestWildcardRedirectRuleConfig(t *testing.T) {
	rule := WildcardRedirectRuleConfig{
		Name:                "wildcard-redirect",
		Enabled:             true,
		SourceURL:           "https://old.example.com/*",
		TargetURL:           "https://new.example.com/$1",
		StatusCode:          301,
		PreserveQueryString: true,
	}

	assert.Equal(t, "wildcard-redirect", rule.Name)
	assert.True(t, rule.Enabled)
	assert.Contains(t, rule.SourceURL, "*")
	assert.Contains(t, rule.TargetURL, "$1")
	assert.Equal(t, 301, rule.StatusCode)
}

func TestRedirectRuleConfigWithWildcards(t *testing.T) {
	config := RedirectRuleConfig{
		Zone:        "example.com",
		Description: "Mixed redirect rules",
		Rules: []RedirectRuleDefinitionConfig{
			{
				Name:       "expression-redirect",
				Expression: "http.request.uri.path eq \"/old\"",
				Enabled:    true,
				Target:     RedirectTargetConfig{URL: "https://example.com/new"},
			},
		},
		WildcardRules: []WildcardRedirectRuleConfig{
			{
				Name:      "wildcard-redirect",
				Enabled:   true,
				SourceURL: "https://old.example.com/*",
				TargetURL: "https://new.example.com/$1",
			},
		},
	}

	assert.Len(t, config.Rules, 1)
	assert.Len(t, config.WildcardRules, 1)
}

func TestZoneRulesetRegisterOptions(t *testing.T) {
	opts := ZoneRulesetRegisterOptions{
		AccountID: "account-123",
		ZoneID:    "zone-456",
		RulesetID: "ruleset-789",
		Source: service.Source{
			Kind:      "ZoneRuleset",
			Namespace: "default",
			Name:      "my-ruleset",
		},
		Config: ZoneRulesetConfig{
			Zone:  "example.com",
			Phase: "http_request_firewall_custom",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "zone-456", opts.ZoneID)
	assert.Equal(t, "ruleset-789", opts.RulesetID)
	assert.Equal(t, "ZoneRuleset", opts.Source.Kind)
}

func TestTransformRuleRegisterOptions(t *testing.T) {
	opts := TransformRuleRegisterOptions{
		AccountID: "account-123",
		ZoneID:    "zone-456",
		RulesetID: "ruleset-789",
		Source: service.Source{
			Kind:      "TransformRule",
			Namespace: "default",
			Name:      "my-transform",
		},
		Config: TransformRuleConfig{
			Zone: "example.com",
			Type: "url_rewrite",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "TransformRule", opts.Source.Kind)
	assert.Equal(t, "url_rewrite", opts.Config.Type)
}

func TestRedirectRuleRegisterOptions(t *testing.T) {
	opts := RedirectRuleRegisterOptions{
		AccountID: "account-123",
		ZoneID:    "zone-456",
		RulesetID: "ruleset-789",
		Source: service.Source{
			Kind:      "RedirectRule",
			Namespace: "default",
			Name:      "my-redirect",
		},
		Config: RedirectRuleConfig{
			Zone: "example.com",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "RedirectRule", opts.Source.Kind)
}

func TestSyncResult(t *testing.T) {
	result := SyncResult{
		ID:        "resource-123",
		AccountID: "account-456",
	}

	assert.Equal(t, "resource-123", result.ID)
	assert.Equal(t, "account-456", result.AccountID)
}

func TestZoneRulesetSyncResult(t *testing.T) {
	result := ZoneRulesetSyncResult{
		SyncResult: SyncResult{
			ID:        "ruleset-123",
			AccountID: "account-456",
		},
		RulesetID:      "ruleset-123",
		RulesetVersion: "v1",
		ZoneID:         "zone-789",
		RuleCount:      5,
	}

	assert.Equal(t, "ruleset-123", result.RulesetID)
	assert.Equal(t, "v1", result.RulesetVersion)
	assert.Equal(t, "zone-789", result.ZoneID)
	assert.Equal(t, 5, result.RuleCount)
}

func TestTransformRuleSyncResult(t *testing.T) {
	result := TransformRuleSyncResult{
		SyncResult: SyncResult{
			ID:        "transform-123",
			AccountID: "account-456",
		},
		RulesetID: "ruleset-789",
		ZoneID:    "zone-abc",
		RuleCount: 3,
	}

	assert.Equal(t, "transform-123", result.ID)
	assert.Equal(t, "ruleset-789", result.RulesetID)
	assert.Equal(t, 3, result.RuleCount)
}

func TestRedirectRuleSyncResult(t *testing.T) {
	result := RedirectRuleSyncResult{
		SyncResult: SyncResult{
			ID:        "redirect-123",
			AccountID: "account-456",
		},
		RulesetID: "ruleset-789",
		ZoneID:    "zone-abc",
		RuleCount: 10,
	}

	assert.Equal(t, "redirect-123", result.ID)
	assert.Equal(t, "ruleset-789", result.RulesetID)
	assert.Equal(t, 10, result.RuleCount)
}

func TestRulesetPhases(t *testing.T) {
	phases := []string{
		"http_request_firewall_custom",
		"http_request_firewall_managed",
		"http_ratelimit",
		"http_request_transform",
		"http_response_transform",
		"http_response_headers_transform",
		"http_request_cache_settings",
		"http_request_origin",
		"http_request_redirect",
	}

	for _, phase := range phases {
		t.Run(phase, func(t *testing.T) {
			config := ZoneRulesetConfig{
				Zone:  "example.com",
				Phase: phase,
			}
			assert.Equal(t, phase, config.Phase)
		})
	}
}
