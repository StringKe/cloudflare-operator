// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package ruleset provides services for managing Cloudflare Ruleset configurations.
//
//nolint:revive // max-public-structs is acceptable for comprehensive Ruleset API types
package ruleset

import (
	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// Resource Types for SyncState - use constants from v1alpha2
const (
	// ResourceTypeZoneRuleset is the SyncState resource type for ZoneRuleset
	ResourceTypeZoneRuleset = v1alpha2.SyncResourceZoneRuleset
	// ResourceTypeTransformRule is the SyncState resource type for TransformRule
	ResourceTypeTransformRule = v1alpha2.SyncResourceTransformRule
	// ResourceTypeRedirectRule is the SyncState resource type for RedirectRule
	ResourceTypeRedirectRule = v1alpha2.SyncResourceRedirectRule

	// Priority constants
	PriorityZoneRuleset   = 100
	PriorityTransformRule = 100
	PriorityRedirectRule  = 100
)

// ZoneRulesetConfig contains the configuration for a zone ruleset.
type ZoneRulesetConfig struct {
	// Zone is the domain name
	Zone string `json:"zone"`
	// Phase is the ruleset phase (e.g., http_request_firewall_custom)
	Phase string `json:"phase"`
	// Description is an optional ruleset description
	Description string `json:"description,omitempty"`
	// Rules is the list of ruleset rules
	Rules []RulesetRuleConfig `json:"rules"`
}

// RulesetRuleConfig contains a single ruleset rule configuration.
type RulesetRuleConfig struct {
	// Action is the rule action (e.g., block, challenge, skip, execute)
	Action string `json:"action"`
	// Expression is the wirefilter expression
	Expression string `json:"expression"`
	// Description is the rule description
	Description string `json:"description,omitempty"`
	// Enabled indicates if the rule is enabled
	Enabled bool `json:"enabled"`
	// Ref is an optional reference string
	Ref string `json:"ref,omitempty"`
	// ActionParameters contains action-specific parameters
	ActionParameters *v1alpha2.RulesetRuleActionParameters `json:"actionParameters,omitempty"`
	// RateLimit contains rate limiting configuration
	RateLimit *v1alpha2.RulesetRuleRateLimit `json:"rateLimit,omitempty"`
}

// TransformRuleConfig contains the configuration for transform rules.
type TransformRuleConfig struct {
	// Zone is the domain name
	Zone string `json:"zone"`
	// Type is the transform rule type (url_rewrite, request_header, response_header)
	Type string `json:"type"`
	// Description is an optional ruleset description
	Description string `json:"description,omitempty"`
	// Rules is the list of transform rules
	Rules []TransformRuleDefinitionConfig `json:"rules"`
}

// TransformRuleDefinitionConfig contains a single transform rule definition.
type TransformRuleDefinitionConfig struct {
	// Name is the rule name
	Name string `json:"name"`
	// Expression is the wirefilter expression
	Expression string `json:"expression"`
	// Enabled indicates if the rule is enabled
	Enabled bool `json:"enabled"`
	// URLRewrite contains URL rewrite configuration
	URLRewrite *v1alpha2.URLRewriteConfig `json:"urlRewrite,omitempty"`
	// Headers contains header modification rules
	Headers []v1alpha2.HeaderModification `json:"headers,omitempty"`
}

// RedirectRuleConfig contains the configuration for redirect rules.
type RedirectRuleConfig struct {
	// Zone is the domain name
	Zone string `json:"zone"`
	// Description is an optional ruleset description
	Description string `json:"description,omitempty"`
	// Rules is the list of expression-based redirect rules
	Rules []RedirectRuleDefinitionConfig `json:"rules,omitempty"`
	// WildcardRules is the list of wildcard-based redirect rules
	WildcardRules []WildcardRedirectRuleConfig `json:"wildcardRules,omitempty"`
}

// RedirectRuleDefinitionConfig contains a single redirect rule definition.
type RedirectRuleDefinitionConfig struct {
	// Name is the rule name
	Name string `json:"name"`
	// Expression is the wirefilter expression
	Expression string `json:"expression"`
	// Enabled indicates if the rule is enabled
	Enabled bool `json:"enabled"`
	// Target contains the redirect target
	Target RedirectTargetConfig `json:"target"`
	// StatusCode is the HTTP redirect status code (301, 302, 303, 307, 308)
	StatusCode int `json:"statusCode,omitempty"`
	// PreserveQueryString indicates whether to preserve query string
	PreserveQueryString bool `json:"preserveQueryString,omitempty"`
}

// RedirectTargetConfig contains redirect target configuration.
type RedirectTargetConfig struct {
	// URL is a static target URL
	URL string `json:"url,omitempty"`
	// Expression is a dynamic target URL expression
	Expression string `json:"expression,omitempty"`
}

// WildcardRedirectRuleConfig contains wildcard redirect rule configuration.
type WildcardRedirectRuleConfig struct {
	// Name is the rule name
	Name string `json:"name"`
	// Enabled indicates if the rule is enabled
	Enabled bool `json:"enabled"`
	// SourceURL is the source URL pattern with wildcards
	SourceURL string `json:"sourceUrl"`
	// TargetURL is the target URL pattern
	TargetURL string `json:"targetUrl"`
	// StatusCode is the HTTP redirect status code
	StatusCode int `json:"statusCode,omitempty"`
	// PreserveQueryString indicates whether to preserve query string
	PreserveQueryString bool `json:"preserveQueryString,omitempty"`
}

// ZoneRulesetRegisterOptions contains options for registering a ZoneRuleset.
type ZoneRulesetRegisterOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// ZoneID is the Cloudflare zone ID
	ZoneID string
	// RulesetID is the existing ruleset ID (empty for new)
	RulesetID string
	// Source is the K8s resource source
	Source service.Source
	// Config is the ruleset configuration
	Config ZoneRulesetConfig
	// CredentialsRef references the CloudflareCredentials resource
	CredentialsRef v1alpha2.CredentialsReference
}

// TransformRuleRegisterOptions contains options for registering a TransformRule.
type TransformRuleRegisterOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// ZoneID is the Cloudflare zone ID
	ZoneID string
	// RulesetID is the existing ruleset ID (empty for new)
	RulesetID string
	// Source is the K8s resource source
	Source service.Source
	// Config is the transform rule configuration
	Config TransformRuleConfig
	// CredentialsRef references the CloudflareCredentials resource
	CredentialsRef v1alpha2.CredentialsReference
}

// RedirectRuleRegisterOptions contains options for registering a RedirectRule.
type RedirectRuleRegisterOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// ZoneID is the Cloudflare zone ID
	ZoneID string
	// RulesetID is the existing ruleset ID (empty for new)
	RulesetID string
	// Source is the K8s resource source
	Source service.Source
	// Config is the redirect rule configuration
	Config RedirectRuleConfig
	// CredentialsRef references the CloudflareCredentials resource
	CredentialsRef v1alpha2.CredentialsReference
}

// SyncResult contains the result of a sync operation.
type SyncResult struct {
	// ID is the Cloudflare resource ID
	ID string
	// AccountID is the Cloudflare account ID
	AccountID string
}

// ZoneRulesetSyncResult contains ZoneRuleset-specific sync result.
type ZoneRulesetSyncResult struct {
	SyncResult
	// RulesetID is the actual ruleset ID
	RulesetID string
	// RulesetVersion is the ruleset version
	RulesetVersion string
	// ZoneID is the zone ID
	ZoneID string
	// RuleCount is the number of rules
	RuleCount int
}

// TransformRuleSyncResult contains TransformRule-specific sync result.
type TransformRuleSyncResult struct {
	SyncResult
	// RulesetID is the actual ruleset ID
	RulesetID string
	// ZoneID is the zone ID
	ZoneID string
	// RuleCount is the number of rules
	RuleCount int
}

// RedirectRuleSyncResult contains RedirectRule-specific sync result.
type RedirectRuleSyncResult struct {
	SyncResult
	// RulesetID is the actual ruleset ID
	RulesetID string
	// ZoneID is the zone ID
	ZoneID string
	// RuleCount is the number of rules
	RuleCount int
}
