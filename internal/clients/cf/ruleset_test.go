// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"testing"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"github.com/stretchr/testify/assert"
)

func TestRulesetResult(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name   string
		result RulesetResult
	}{
		{
			name: "full result",
			result: RulesetResult{
				ID:          "ruleset-123",
				Name:        "Test Ruleset",
				Description: "A test ruleset",
				Kind:        "zone",
				Phase:       "http_request_firewall_custom",
				Version:     "1",
				LastUpdated: now,
				Rules: []cloudflare.RulesetRule{
					{
						ID:          "rule-1",
						Action:      "block",
						Expression:  "(cf.client.bot)",
						Description: "Block bots",
					},
				},
			},
		},
		{
			name: "minimal result",
			result: RulesetResult{
				ID:    "ruleset-456",
				Phase: "http_request_transform",
			},
		},
		{
			name: "result with multiple rules",
			result: RulesetResult{
				ID:          "ruleset-789",
				Name:        "Multi-rule Ruleset",
				Phase:       "http_request_firewall_custom",
				Version:     "5",
				LastUpdated: now,
				Rules: []cloudflare.RulesetRule{
					{
						ID:         "rule-1",
						Action:     "block",
						Expression: "(cf.client.bot)",
					},
					{
						ID:         "rule-2",
						Action:     "challenge",
						Expression: "(cf.threat_score > 30)",
					},
					{
						ID:         "rule-3",
						Action:     "skip",
						Expression: "(ip.src in $whitelist)",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.result.ID)
			assert.NotEmpty(t, tt.result.Phase)
		})
	}
}

func TestRulesetResultFields(t *testing.T) {
	now := time.Now()

	result := RulesetResult{
		ID:          "test-id",
		Name:        "Test Name",
		Description: "Test Description",
		Kind:        "zone",
		Phase:       "http_request_firewall_custom",
		Version:     "3",
		LastUpdated: now,
		Rules: []cloudflare.RulesetRule{
			{
				ID:          "rule-id",
				Action:      "block",
				Expression:  "true",
				Description: "Block all",
			},
		},
	}

	assert.Equal(t, "test-id", result.ID)
	assert.Equal(t, "Test Name", result.Name)
	assert.Equal(t, "Test Description", result.Description)
	assert.Equal(t, "zone", result.Kind)
	assert.Equal(t, "http_request_firewall_custom", result.Phase)
	assert.Equal(t, "3", result.Version)
	assert.Equal(t, now, result.LastUpdated)
	assert.Len(t, result.Rules, 1)
	assert.Equal(t, "rule-id", result.Rules[0].ID)
	assert.Equal(t, "block", result.Rules[0].Action)
}

func TestRulesetPhases(t *testing.T) {
	// Test common ruleset phases
	phases := []struct {
		phase       string
		description string
	}{
		{
			phase:       "http_request_firewall_custom",
			description: "Custom firewall rules",
		},
		{
			phase:       "http_request_transform",
			description: "URL transformation",
		},
		{
			phase:       "http_response_headers_transform",
			description: "Response header modification",
		},
		{
			phase:       "http_request_dynamic_redirect",
			description: "Dynamic redirects",
		},
		{
			phase:       "http_ratelimit",
			description: "Rate limiting",
		},
		{
			phase:       "http_request_cache_settings",
			description: "Cache settings",
		},
		{
			phase:       "http_config_settings",
			description: "Configuration settings",
		},
	}

	for _, p := range phases {
		t.Run(p.phase, func(t *testing.T) {
			result := RulesetResult{
				ID:    "test",
				Phase: p.phase,
			}
			assert.Equal(t, p.phase, result.Phase)
		})
	}
}

func TestRulesetActions(t *testing.T) {
	// Test common ruleset actions
	actions := []struct {
		action      string
		description string
	}{
		{action: "block", description: "Block request"},
		{action: "challenge", description: "JS challenge"},
		{action: "managed_challenge", description: "Managed challenge"},
		{action: "skip", description: "Skip rules"},
		{action: "log", description: "Log only"},
		{action: "rewrite", description: "Rewrite URL/headers"},
		{action: "redirect", description: "Redirect request"},
		{action: "route", description: "Route to origin"},
		{action: "score", description: "Assign score"},
		{action: "execute", description: "Execute ruleset"},
		{action: "set_cache_settings", description: "Set cache settings"},
		{action: "set_config", description: "Set configuration"},
	}

	for _, a := range actions {
		t.Run(a.action, func(t *testing.T) {
			rule := cloudflare.RulesetRule{
				ID:     "test",
				Action: a.action,
			}
			assert.Equal(t, a.action, rule.Action)
		})
	}
}

func TestRulesetRuleExpressions(t *testing.T) {
	// Test common rule expressions
	expressions := []struct {
		name       string
		expression string
		valid      bool
	}{
		{
			name:       "bot detection",
			expression: "(cf.client.bot)",
			valid:      true,
		},
		{
			name:       "threat score",
			expression: "(cf.threat_score > 50)",
			valid:      true,
		},
		{
			name:       "country block",
			expression: "(ip.geoip.country eq \"XX\")",
			valid:      true,
		},
		{
			name:       "IP in list",
			expression: "(ip.src in $blocklist)",
			valid:      true,
		},
		{
			name:       "path match",
			expression: "(http.request.uri.path matches \"^/api/.*\")",
			valid:      true,
		},
		{
			name:       "host match",
			expression: "(http.host eq \"example.com\")",
			valid:      true,
		},
		{
			name:       "method check",
			expression: "(http.request.method eq \"POST\")",
			valid:      true,
		},
		{
			name:       "combined expression",
			expression: "(cf.client.bot) and (cf.threat_score > 30) and not (ip.src in $whitelist)",
			valid:      true,
		},
		{
			name:       "always true",
			expression: "true",
			valid:      true,
		},
	}

	for _, e := range expressions {
		t.Run(e.name, func(t *testing.T) {
			rule := cloudflare.RulesetRule{
				ID:         "test",
				Expression: e.expression,
			}
			assert.NotEmpty(t, rule.Expression)
		})
	}
}

func TestRulesetResultWithActionParameters(t *testing.T) {
	// Test rules with action parameters
	tests := []struct {
		name   string
		rule   cloudflare.RulesetRule
		verify func(t *testing.T, rule cloudflare.RulesetRule)
	}{
		{
			name: "redirect action",
			rule: cloudflare.RulesetRule{
				ID:          "redirect-rule",
				Action:      "redirect",
				Expression:  "(http.host eq \"old.example.com\")",
				Description: "Redirect to new domain",
			},
			verify: func(t *testing.T, rule cloudflare.RulesetRule) {
				assert.Equal(t, "redirect", rule.Action)
			},
		},
		{
			name: "rewrite action",
			rule: cloudflare.RulesetRule{
				ID:          "rewrite-rule",
				Action:      "rewrite",
				Expression:  "(http.request.uri.path matches \"^/old/(.*)$\")",
				Description: "Rewrite old paths",
			},
			verify: func(t *testing.T, rule cloudflare.RulesetRule) {
				assert.Equal(t, "rewrite", rule.Action)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.verify(t, tt.rule)
		})
	}
}

func TestRulesetKinds(t *testing.T) {
	kinds := []struct {
		kind        string
		description string
	}{
		{kind: "zone", description: "Zone-level ruleset"},
		{kind: "custom", description: "Custom ruleset"},
		{kind: "managed", description: "Managed ruleset"},
		{kind: "root", description: "Root ruleset"},
	}

	for _, k := range kinds {
		t.Run(k.kind, func(t *testing.T) {
			result := RulesetResult{
				ID:   "test",
				Kind: k.kind,
			}
			assert.Equal(t, k.kind, result.Kind)
		})
	}
}

func TestRulesetVersioning(t *testing.T) {
	// Test version string handling
	versions := []string{"1", "5", "10", "100", "1000"}

	for _, v := range versions {
		t.Run("version_"+v, func(t *testing.T) {
			result := RulesetResult{
				ID:      "test",
				Version: v,
			}
			assert.Equal(t, v, result.Version)
		})
	}
}

func TestEmptyRulesetResult(t *testing.T) {
	result := RulesetResult{}

	assert.Empty(t, result.ID)
	assert.Empty(t, result.Name)
	assert.Empty(t, result.Description)
	assert.Empty(t, result.Kind)
	assert.Empty(t, result.Phase)
	assert.Empty(t, result.Version)
	assert.True(t, result.LastUpdated.IsZero())
	assert.Nil(t, result.Rules)
}
