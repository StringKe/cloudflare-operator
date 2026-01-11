// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RedirectRuleState represents the state of the redirect rule
// +kubebuilder:validation:Enum=Pending;Syncing;Ready;Error
type RedirectRuleState string

const (
	// RedirectRuleStatePending means the rule is waiting to be synced
	RedirectRuleStatePending RedirectRuleState = "Pending"
	// RedirectRuleStateSyncing means the rule is being synced
	RedirectRuleStateSyncing RedirectRuleState = "Syncing"
	// RedirectRuleStateReady means the rule is synced and ready
	RedirectRuleStateReady RedirectRuleState = "Ready"
	// RedirectRuleStateError means there was an error with the rule
	RedirectRuleStateError RedirectRuleState = "Error"
)

// RedirectStatusCode represents valid HTTP redirect status codes
// +kubebuilder:validation:Enum=301;302;307;308
type RedirectStatusCode int

const (
	// RedirectStatusMovedPermanently (301) - Permanent redirect
	RedirectStatusMovedPermanently RedirectStatusCode = 301
	// RedirectStatusFound (302) - Temporary redirect (commonly used)
	RedirectStatusFound RedirectStatusCode = 302
	// RedirectStatusTemporaryRedirect (307) - Temporary redirect, preserve method
	RedirectStatusTemporaryRedirect RedirectStatusCode = 307
	// RedirectStatusPermanentRedirect (308) - Permanent redirect, preserve method
	RedirectStatusPermanentRedirect RedirectStatusCode = 308
)

// RedirectTarget defines the redirect destination
type RedirectTarget struct {
	// URL is a static target URL
	// Example: https://example.com/new-path
	// +kubebuilder:validation:Optional
	URL string `json:"url,omitempty"`

	// Expression is a dynamic expression for the target URL
	// Example: concat("https://", http.host, "/new", http.request.uri.path)
	// +kubebuilder:validation:Optional
	Expression string `json:"expression,omitempty"`
}

// RedirectRuleDefinition defines a single redirect rule
type RedirectRuleDefinition struct {
	// Name is a human-readable name for the rule
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Expression is the filter expression (Cloudflare Rules language)
	// Example: (http.request.uri.path eq "/old-path")
	// +kubebuilder:validation:Required
	Expression string `json:"expression"`

	// Enabled controls whether the rule is active
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Target defines where to redirect
	// +kubebuilder:validation:Required
	Target RedirectTarget `json:"target"`

	// StatusCode is the HTTP redirect status code
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=302
	StatusCode RedirectStatusCode `json:"statusCode,omitempty"`

	// PreserveQueryString keeps the original query string in the redirect
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	PreserveQueryString bool `json:"preserveQueryString,omitempty"`
}

// WildcardRedirectRule defines a wildcard-based redirect rule
// This provides a simpler syntax for common redirect patterns
type WildcardRedirectRule struct {
	// Name is a human-readable name for the rule
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// SourceURL is the wildcard URL pattern to match
	// Use * as wildcard. Example: https://example.com/blog/*
	// +kubebuilder:validation:Required
	SourceURL string `json:"sourceUrl"`

	// TargetURL is the redirect destination
	// Use ${1}, ${2} etc. for wildcard replacements
	// Example: https://example.com/articles/${1}
	// +kubebuilder:validation:Required
	TargetURL string `json:"targetUrl"`

	// Enabled controls whether the rule is active
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// StatusCode is the HTTP redirect status code
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=301
	StatusCode RedirectStatusCode `json:"statusCode,omitempty"`

	// PreserveQueryString keeps the original query string in the redirect
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	PreserveQueryString bool `json:"preserveQueryString,omitempty"`

	// IncludeSubdomains applies the redirect to subdomains
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	IncludeSubdomains bool `json:"includeSubdomains,omitempty"`

	// SubpathMatching enables matching of subpaths
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	SubpathMatching bool `json:"subpathMatching,omitempty"`
}

// RedirectRuleSpec defines the desired state of RedirectRule
type RedirectRuleSpec struct {
	// Zone is the zone name (domain) to apply rules to
	// +kubebuilder:validation:Required
	Zone string `json:"zone"`

	// Description is a human-readable description of the redirect rules
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`

	// Rules are expression-based redirect rules
	// Use this for complex redirect logic
	// +kubebuilder:validation:Optional
	Rules []RedirectRuleDefinition `json:"rules,omitempty"`

	// WildcardRules are wildcard-based redirect rules
	// Use this for simpler pattern-based redirects
	// +kubebuilder:validation:Optional
	WildcardRules []WildcardRedirectRule `json:"wildcardRules,omitempty"`

	// CredentialsRef references a CloudflareCredentials resource
	// If not specified, the default CloudflareCredentials will be used
	// +kubebuilder:validation:Optional
	CredentialsRef *CredentialsReference `json:"credentialsRef,omitempty"`
}

// RedirectRuleStatus defines the observed state of RedirectRule
type RedirectRuleStatus struct {
	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// State represents the current state of the rule
	// +optional
	State RedirectRuleState `json:"state,omitempty"`

	// RulesetID is the Cloudflare ruleset ID
	// +optional
	RulesetID string `json:"rulesetId,omitempty"`

	// ZoneID is the Cloudflare zone ID
	// +optional
	ZoneID string `json:"zoneId,omitempty"`

	// RuleCount is the total number of redirect rules
	// +optional
	RuleCount int `json:"ruleCount,omitempty"`

	// Message provides additional information about the current state
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cfredirect;redirectrule
// +kubebuilder:printcolumn:name="Zone",type=string,JSONPath=`.spec.zone`
// +kubebuilder:printcolumn:name="Rules",type=integer,JSONPath=`.status.ruleCount`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// RedirectRule manages Cloudflare Redirect Rules (Single Redirects).
// Redirect Rules allow you to create URL redirects with static or dynamic targets.
//
// Two syntaxes are supported:
// - Rules: Expression-based rules for complex redirect logic
// - WildcardRules: Wildcard pattern rules for simpler use cases
//
// This is a simplified interface over ZoneRuleset for redirect use cases.
type RedirectRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedirectRuleSpec   `json:"spec,omitempty"`
	Status RedirectRuleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RedirectRuleList contains a list of RedirectRule
type RedirectRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedirectRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RedirectRule{}, &RedirectRuleList{})
}
