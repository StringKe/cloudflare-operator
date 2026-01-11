// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ZoneRulesetState represents the state of the ruleset
// +kubebuilder:validation:Enum=Pending;Syncing;Ready;Error
type ZoneRulesetState string

const (
	// ZoneRulesetStatePending means the ruleset is waiting to be synced
	ZoneRulesetStatePending ZoneRulesetState = "Pending"
	// ZoneRulesetStateSyncing means the ruleset is being synced
	ZoneRulesetStateSyncing ZoneRulesetState = "Syncing"
	// ZoneRulesetStateReady means the ruleset is synced and ready
	ZoneRulesetStateReady ZoneRulesetState = "Ready"
	// ZoneRulesetStateError means there was an error with the ruleset
	ZoneRulesetStateError ZoneRulesetState = "Error"
)

// RulesetPhase represents the phase/entry point of the ruleset
// +kubebuilder:validation:Enum=http_request_transform;http_request_late_transform;http_request_origin;http_request_redirect;http_request_dynamic_redirect;http_request_cache_settings;http_config_settings;http_custom_errors;http_response_headers_transform;http_response_compression;http_ratelimit;http_request_firewall_custom;http_request_firewall_managed;http_response_firewall_managed
type RulesetPhase string

const (
	// RulesetPhaseHTTPRequestTransform is for URL Rewrite Rules (transform requests)
	RulesetPhaseHTTPRequestTransform RulesetPhase = "http_request_transform"
	// RulesetPhaseHTTPRequestLateTransform is for HTTP Request Header Modification Rules
	RulesetPhaseHTTPRequestLateTransform RulesetPhase = "http_request_late_transform"
	// RulesetPhaseHTTPRequestOrigin is for Origin Rules
	RulesetPhaseHTTPRequestOrigin RulesetPhase = "http_request_origin"
	// RulesetPhaseHTTPRequestRedirect is for Single Redirects
	RulesetPhaseHTTPRequestRedirect RulesetPhase = "http_request_redirect"
	// RulesetPhaseHTTPRequestDynamicRedirect is for Dynamic Redirects / Bulk Redirects
	RulesetPhaseHTTPRequestDynamicRedirect RulesetPhase = "http_request_dynamic_redirect"
	// RulesetPhaseHTTPRequestCacheSettings is for Cache Rules
	RulesetPhaseHTTPRequestCacheSettings RulesetPhase = "http_request_cache_settings"
	// RulesetPhaseHTTPConfigSettings is for Configuration Rules
	RulesetPhaseHTTPConfigSettings RulesetPhase = "http_config_settings"
	// RulesetPhaseHTTPCustomErrors is for Custom Error Responses
	RulesetPhaseHTTPCustomErrors RulesetPhase = "http_custom_errors"
	// RulesetPhaseHTTPResponseHeadersTransform is for HTTP Response Header Modification Rules
	RulesetPhaseHTTPResponseHeadersTransform RulesetPhase = "http_response_headers_transform"
	// RulesetPhaseHTTPResponseCompression is for Compression Rules
	RulesetPhaseHTTPResponseCompression RulesetPhase = "http_response_compression"
	// RulesetPhaseHTTPRateLimit is for Rate Limiting Rules
	RulesetPhaseHTTPRateLimit RulesetPhase = "http_ratelimit"
	// RulesetPhaseHTTPRequestFirewallCustom is for Custom Firewall Rules (WAF)
	RulesetPhaseHTTPRequestFirewallCustom RulesetPhase = "http_request_firewall_custom"
	// RulesetPhaseHTTPRequestFirewallManaged is for Managed Firewall Rules (WAF)
	RulesetPhaseHTTPRequestFirewallManaged RulesetPhase = "http_request_firewall_managed"
	// RulesetPhaseHTTPResponseFirewallManaged is for Response Firewall Rules
	RulesetPhaseHTTPResponseFirewallManaged RulesetPhase = "http_response_firewall_managed"
)

// RulesetRuleAction represents the action to take when a rule matches
// +kubebuilder:validation:Enum=block;challenge;js_challenge;managed_challenge;log;skip;rewrite;redirect;route;score;execute;set_config;set_cache_settings;serve_error;compress_response
type RulesetRuleAction string

const (
	// RulesetRuleActionBlock blocks the request
	RulesetRuleActionBlock RulesetRuleAction = "block"
	// RulesetRuleActionChallenge presents a CAPTCHA challenge
	RulesetRuleActionChallenge RulesetRuleAction = "challenge"
	// RulesetRuleActionJSChallenge presents a JavaScript challenge
	RulesetRuleActionJSChallenge RulesetRuleAction = "js_challenge"
	// RulesetRuleActionManagedChallenge presents a managed challenge
	RulesetRuleActionManagedChallenge RulesetRuleAction = "managed_challenge"
	// RulesetRuleActionLog logs the request
	RulesetRuleActionLog RulesetRuleAction = "log"
	// RulesetRuleActionSkip skips remaining rules
	RulesetRuleActionSkip RulesetRuleAction = "skip"
	// RulesetRuleActionRewrite rewrites the request
	RulesetRuleActionRewrite RulesetRuleAction = "rewrite"
	// RulesetRuleActionRedirect redirects the request
	RulesetRuleActionRedirect RulesetRuleAction = "redirect"
	// RulesetRuleActionRoute routes the request
	RulesetRuleActionRoute RulesetRuleAction = "route"
	// RulesetRuleActionScore scores the request
	RulesetRuleActionScore RulesetRuleAction = "score"
	// RulesetRuleActionExecute executes another ruleset
	RulesetRuleActionExecute RulesetRuleAction = "execute"
	// RulesetRuleActionSetConfig sets configuration
	RulesetRuleActionSetConfig RulesetRuleAction = "set_config"
	// RulesetRuleActionSetCacheSettings sets cache settings
	RulesetRuleActionSetCacheSettings RulesetRuleAction = "set_cache_settings"
	// RulesetRuleActionServeError serves an error page
	RulesetRuleActionServeError RulesetRuleAction = "serve_error"
	// RulesetRuleActionCompressResponse compresses the response
	RulesetRuleActionCompressResponse RulesetRuleAction = "compress_response"
)

// RulesetRule defines a single rule in the ruleset
type RulesetRule struct {
	// Description is a human-readable description of the rule
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`

	// Expression is the filter expression (Cloudflare Rules language)
	// +kubebuilder:validation:Required
	Expression string `json:"expression"`

	// Action is the action to take when the expression matches
	// +kubebuilder:validation:Required
	Action RulesetRuleAction `json:"action"`

	// ActionParameters contains parameters for the action
	// +kubebuilder:validation:Optional
	ActionParameters *RulesetRuleActionParameters `json:"actionParameters,omitempty"`

	// Enabled controls whether the rule is active
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Ref is a reference ID for the rule (for ordering)
	// +kubebuilder:validation:Optional
	Ref string `json:"ref,omitempty"`

	// RateLimit defines rate limiting parameters (for http_ratelimit phase)
	// +kubebuilder:validation:Optional
	RateLimit *RulesetRuleRateLimit `json:"rateLimit,omitempty"`
}

// RulesetRuleActionParameters contains parameters for rule actions
type RulesetRuleActionParameters struct {
	// URI contains URL rewrite parameters
	// +kubebuilder:validation:Optional
	URI *RulesetURIRewrite `json:"uri,omitempty"`

	// Headers contains header modification parameters
	// +kubebuilder:validation:Optional
	Headers map[string]RulesetHeaderAction `json:"headers,omitempty"`

	// Redirect contains redirect parameters
	// +kubebuilder:validation:Optional
	Redirect *RulesetRedirect `json:"redirect,omitempty"`

	// Origin contains origin override parameters
	// +kubebuilder:validation:Optional
	Origin *RulesetOrigin `json:"origin,omitempty"`

	// Cache contains cache settings
	// +kubebuilder:validation:Optional
	Cache *RulesetCacheSettings `json:"cache,omitempty"`

	// Products lists products to skip (for skip action)
	// +kubebuilder:validation:Optional
	Products []string `json:"products,omitempty"`

	// Ruleset is the ID of ruleset to execute (for execute action)
	// +kubebuilder:validation:Optional
	Ruleset string `json:"ruleset,omitempty"`

	// Phases lists phases to skip (for skip action)
	// +kubebuilder:validation:Optional
	Phases []string `json:"phases,omitempty"`

	// Rules lists rule IDs to skip (for skip action)
	// +kubebuilder:validation:Optional
	Rules map[string][]string `json:"rules,omitempty"`

	// Response contains custom error response parameters
	// +kubebuilder:validation:Optional
	Response *RulesetCustomResponse `json:"response,omitempty"`

	// Algorithms contains compression algorithms
	// +kubebuilder:validation:Optional
	Algorithms []RulesetCompressionAlgorithm `json:"algorithms,omitempty"`
}

// RulesetURIRewrite defines URL rewrite parameters
type RulesetURIRewrite struct {
	// Path is the new path (can use expressions)
	// +kubebuilder:validation:Optional
	Path *RulesetRewriteValue `json:"path,omitempty"`

	// Query is the new query string (can use expressions)
	// +kubebuilder:validation:Optional
	Query *RulesetRewriteValue `json:"query,omitempty"`
}

// RulesetRewriteValue defines a rewrite value
type RulesetRewriteValue struct {
	// Value is a static value
	// +kubebuilder:validation:Optional
	Value string `json:"value,omitempty"`

	// Expression is a dynamic expression
	// +kubebuilder:validation:Optional
	Expression string `json:"expression,omitempty"`
}

// RulesetHeaderAction defines a header modification action
type RulesetHeaderAction struct {
	// Operation is the header operation (set, add, remove)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=set;add;remove
	Operation string `json:"operation"`

	// Value is the header value (for set/add operations)
	// +kubebuilder:validation:Optional
	Value string `json:"value,omitempty"`

	// Expression is a dynamic expression for the value
	// +kubebuilder:validation:Optional
	Expression string `json:"expression,omitempty"`
}

// RulesetRedirect defines redirect parameters
type RulesetRedirect struct {
	// SourceURL is the URL pattern to match
	// +kubebuilder:validation:Optional
	SourceURL string `json:"sourceUrl,omitempty"`

	// TargetURL is the redirect destination
	// +kubebuilder:validation:Optional
	TargetURL *RulesetRewriteValue `json:"targetUrl,omitempty"`

	// StatusCode is the HTTP status code (301, 302, 307, 308)
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=301;302;307;308
	StatusCode int `json:"statusCode,omitempty"`

	// PreserveQueryString preserves the original query string
	// +kubebuilder:validation:Optional
	PreserveQueryString bool `json:"preserveQueryString,omitempty"`

	// IncludeSubdomains applies to subdomains
	// +kubebuilder:validation:Optional
	IncludeSubdomains bool `json:"includeSubdomains,omitempty"`

	// SubpathMatching enables subpath matching
	// +kubebuilder:validation:Optional
	SubpathMatching bool `json:"subpathMatching,omitempty"`
}

// RulesetOrigin defines origin override parameters
type RulesetOrigin struct {
	// Host overrides the Host header
	// +kubebuilder:validation:Optional
	Host string `json:"host,omitempty"`

	// Port overrides the port
	// +kubebuilder:validation:Optional
	Port int `json:"port,omitempty"`
}

// RulesetCacheSettings defines cache settings
type RulesetCacheSettings struct {
	// Cache enables or disables caching
	// +kubebuilder:validation:Optional
	Cache *bool `json:"cache,omitempty"`

	// EdgeTTL sets the edge cache TTL
	// +kubebuilder:validation:Optional
	EdgeTTL *RulesetCacheTTL `json:"edgeTtl,omitempty"`

	// BrowserTTL sets the browser cache TTL
	// +kubebuilder:validation:Optional
	BrowserTTL *RulesetCacheTTL `json:"browserTtl,omitempty"`

	// CacheKey customizes the cache key
	// +kubebuilder:validation:Optional
	CacheKey *RulesetCacheKey `json:"cacheKey,omitempty"`

	// RespectStrongETags respects strong ETags
	// +kubebuilder:validation:Optional
	RespectStrongETags *bool `json:"respectStrongEtags,omitempty"`

	// OriginErrorPagePassthru passes through origin error pages
	// +kubebuilder:validation:Optional
	OriginErrorPagePassthru *bool `json:"originErrorPagePassthru,omitempty"`
}

// RulesetCacheTTL defines cache TTL settings
type RulesetCacheTTL struct {
	// Mode is the TTL mode (respect_origin, bypass_by_default, override_origin)
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=respect_origin;bypass_by_default;override_origin
	Mode string `json:"mode,omitempty"`

	// Default is the default TTL in seconds
	// +kubebuilder:validation:Optional
	Default *int `json:"default,omitempty"`

	// StatusCodeTTL sets TTL based on status codes
	// +kubebuilder:validation:Optional
	StatusCodeTTL []RulesetStatusCodeTTL `json:"statusCodeTtl,omitempty"`
}

// RulesetStatusCodeTTL defines TTL for specific status codes
type RulesetStatusCodeTTL struct {
	// StatusCodeRange is the status code range (e.g., "200-299")
	// +kubebuilder:validation:Optional
	StatusCodeRange *RulesetStatusCodeRange `json:"statusCodeRange,omitempty"`

	// StatusCodeValue is a single status code
	// +kubebuilder:validation:Optional
	StatusCodeValue *int `json:"statusCodeValue,omitempty"`

	// Value is the TTL value in seconds
	// +kubebuilder:validation:Required
	Value int `json:"value"`
}

// RulesetStatusCodeRange defines a range of status codes
type RulesetStatusCodeRange struct {
	// From is the start of the range
	From int `json:"from"`
	// To is the end of the range
	To int `json:"to"`
}

// RulesetCacheKey defines cache key customization
type RulesetCacheKey struct {
	// IgnoreQueryStringsOrder ignores query string order
	// +kubebuilder:validation:Optional
	IgnoreQueryStringsOrder *bool `json:"ignoreQueryStringsOrder,omitempty"`

	// CacheDeceptionArmor enables cache deception armor
	// +kubebuilder:validation:Optional
	CacheDeceptionArmor *bool `json:"cacheDeceptionArmor,omitempty"`

	// QueryString customizes query string handling
	// +kubebuilder:validation:Optional
	QueryString *RulesetQueryStringCacheKey `json:"queryString,omitempty"`

	// Header customizes header-based cache key
	// +kubebuilder:validation:Optional
	Header *RulesetHeaderCacheKey `json:"header,omitempty"`

	// Cookie customizes cookie-based cache key
	// +kubebuilder:validation:Optional
	Cookie *RulesetCookieCacheKey `json:"cookie,omitempty"`

	// User customizes user-based cache key
	// +kubebuilder:validation:Optional
	User *RulesetUserCacheKey `json:"user,omitempty"`

	// Host customizes host-based cache key
	// +kubebuilder:validation:Optional
	Host *RulesetHostCacheKey `json:"host,omitempty"`
}

// RulesetQueryStringCacheKey defines query string cache key settings
type RulesetQueryStringCacheKey struct {
	// Exclude excludes query parameters
	// +kubebuilder:validation:Optional
	Exclude *RulesetQueryStringList `json:"exclude,omitempty"`

	// Include includes query parameters
	// +kubebuilder:validation:Optional
	Include *RulesetQueryStringList `json:"include,omitempty"`
}

// RulesetQueryStringList defines a list of query parameters
type RulesetQueryStringList struct {
	// List is a list of query parameter names
	// +kubebuilder:validation:Optional
	List []string `json:"list,omitempty"`

	// All includes/excludes all query parameters
	// +kubebuilder:validation:Optional
	All *bool `json:"all,omitempty"`
}

// RulesetHeaderCacheKey defines header-based cache key settings
type RulesetHeaderCacheKey struct {
	// Include includes headers
	// +kubebuilder:validation:Optional
	Include []string `json:"include,omitempty"`

	// CheckPresence checks for header presence
	// +kubebuilder:validation:Optional
	CheckPresence []string `json:"checkPresence,omitempty"`

	// ExcludeOrigin excludes origin headers
	// +kubebuilder:validation:Optional
	ExcludeOrigin *bool `json:"excludeOrigin,omitempty"`
}

// RulesetCookieCacheKey defines cookie-based cache key settings
type RulesetCookieCacheKey struct {
	// Include includes cookies
	// +kubebuilder:validation:Optional
	Include []string `json:"include,omitempty"`

	// CheckPresence checks for cookie presence
	// +kubebuilder:validation:Optional
	CheckPresence []string `json:"checkPresence,omitempty"`
}

// RulesetUserCacheKey defines user-based cache key settings
type RulesetUserCacheKey struct {
	// DeviceType includes device type
	// +kubebuilder:validation:Optional
	DeviceType *bool `json:"deviceType,omitempty"`

	// Geo includes geolocation
	// +kubebuilder:validation:Optional
	Geo *bool `json:"geo,omitempty"`

	// Lang includes language
	// +kubebuilder:validation:Optional
	Lang *bool `json:"lang,omitempty"`
}

// RulesetHostCacheKey defines host-based cache key settings
type RulesetHostCacheKey struct {
	// Resolved uses the resolved host
	// +kubebuilder:validation:Optional
	Resolved *bool `json:"resolved,omitempty"`
}

// RulesetCustomResponse defines custom error response
type RulesetCustomResponse struct {
	// StatusCode is the HTTP status code
	// +kubebuilder:validation:Optional
	StatusCode int `json:"statusCode,omitempty"`

	// ContentType is the response content type
	// +kubebuilder:validation:Optional
	ContentType string `json:"contentType,omitempty"`

	// Content is the response body
	// +kubebuilder:validation:Optional
	Content string `json:"content,omitempty"`
}

// RulesetCompressionAlgorithm defines a compression algorithm
type RulesetCompressionAlgorithm struct {
	// Name is the algorithm name (gzip, brotli, auto, none)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=gzip;brotli;auto;none
	Name string `json:"name"`
}

// RulesetRuleRateLimit defines rate limiting parameters
type RulesetRuleRateLimit struct {
	// Characteristics defines what to count for rate limiting
	// +kubebuilder:validation:Optional
	Characteristics []string `json:"characteristics,omitempty"`

	// Period is the period in seconds
	// +kubebuilder:validation:Optional
	Period int `json:"period,omitempty"`

	// RequestsPerPeriod is the request limit
	// +kubebuilder:validation:Optional
	RequestsPerPeriod int `json:"requestsPerPeriod,omitempty"`

	// MitigationTimeout is the block duration in seconds
	// +kubebuilder:validation:Optional
	MitigationTimeout int `json:"mitigationTimeout,omitempty"`

	// CountingExpression is the expression for counting
	// +kubebuilder:validation:Optional
	CountingExpression string `json:"countingExpression,omitempty"`

	// RequestsToOrigin counts only requests to origin
	// +kubebuilder:validation:Optional
	RequestsToOrigin *bool `json:"requestsToOrigin,omitempty"`

	// ScorePerPeriod is the score limit (for complexity-based limiting)
	// +kubebuilder:validation:Optional
	ScorePerPeriod int `json:"scorePerPeriod,omitempty"`

	// ScoreResponseHeaderName is the header for score reporting
	// +kubebuilder:validation:Optional
	ScoreResponseHeaderName string `json:"scoreResponseHeaderName,omitempty"`
}

// ZoneRulesetSpec defines the desired state of ZoneRuleset
type ZoneRulesetSpec struct {
	// Zone is the zone name (domain) to apply rules to
	// +kubebuilder:validation:Required
	Zone string `json:"zone"`

	// Phase is the ruleset phase/entry point
	// +kubebuilder:validation:Required
	Phase RulesetPhase `json:"phase"`

	// Description is a human-readable description of the ruleset
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`

	// Rules are the rules in this ruleset
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Rules []RulesetRule `json:"rules"`

	// CredentialsRef references a CloudflareCredentials resource
	// If not specified, the default CloudflareCredentials will be used
	// +kubebuilder:validation:Optional
	CredentialsRef *CredentialsReference `json:"credentialsRef,omitempty"`
}

// ZoneRulesetStatus defines the observed state of ZoneRuleset
type ZoneRulesetStatus struct {
	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// State represents the current state of the ruleset
	// +optional
	State ZoneRulesetState `json:"state,omitempty"`

	// RulesetID is the Cloudflare ruleset ID
	// +optional
	RulesetID string `json:"rulesetId,omitempty"`

	// RulesetVersion is the current ruleset version
	// +optional
	RulesetVersion string `json:"rulesetVersion,omitempty"`

	// ZoneID is the Cloudflare zone ID
	// +optional
	ZoneID string `json:"zoneId,omitempty"`

	// RuleCount is the number of rules in the ruleset
	// +optional
	RuleCount int `json:"ruleCount,omitempty"`

	// LastUpdated is the last time the ruleset was updated
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// Message provides additional information about the current state
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cfzr;zoneruleset
// +kubebuilder:printcolumn:name="Zone",type=string,JSONPath=`.spec.zone`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.spec.phase`
// +kubebuilder:printcolumn:name="Rules",type=integer,JSONPath=`.status.ruleCount`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ZoneRuleset manages Cloudflare zone rulesets.
// Rulesets are the backbone of Cloudflare Rules (Transform Rules, Redirect Rules,
// Cache Rules, Configuration Rules, WAF Custom Rules, etc.).
//
// Each ZoneRuleset manages rules for a specific phase (entry point) in the
// request processing pipeline.
type ZoneRuleset struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZoneRulesetSpec   `json:"spec,omitempty"`
	Status ZoneRulesetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ZoneRulesetList contains a list of ZoneRuleset
type ZoneRulesetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZoneRuleset `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ZoneRuleset{}, &ZoneRulesetList{})
}
