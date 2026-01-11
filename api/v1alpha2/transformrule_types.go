// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TransformRuleState represents the state of the transform rule
// +kubebuilder:validation:Enum=Pending;Syncing;Ready;Error
type TransformRuleState string

const (
	// TransformRuleStatePending means the rule is waiting to be synced
	TransformRuleStatePending TransformRuleState = "Pending"
	// TransformRuleStateSyncing means the rule is being synced
	TransformRuleStateSyncing TransformRuleState = "Syncing"
	// TransformRuleStateReady means the rule is synced and ready
	TransformRuleStateReady TransformRuleState = "Ready"
	// TransformRuleStateError means there was an error with the rule
	TransformRuleStateError TransformRuleState = "Error"
)

// TransformRuleType represents the type of transform rule
// +kubebuilder:validation:Enum=url_rewrite;request_header;response_header
type TransformRuleType string

const (
	// TransformRuleTypeURLRewrite rewrites the URL path and/or query string
	TransformRuleTypeURLRewrite TransformRuleType = "url_rewrite"
	// TransformRuleTypeRequestHeader modifies HTTP request headers
	TransformRuleTypeRequestHeader TransformRuleType = "request_header"
	// TransformRuleTypeResponseHeader modifies HTTP response headers
	TransformRuleTypeResponseHeader TransformRuleType = "response_header"
)

// HeaderOperation represents the operation to perform on a header
// +kubebuilder:validation:Enum=set;add;remove
type HeaderOperation string

const (
	// HeaderOperationSet sets the header value (overwrites if exists)
	HeaderOperationSet HeaderOperation = "set"
	// HeaderOperationAdd adds a value to the header (preserves existing)
	HeaderOperationAdd HeaderOperation = "add"
	// HeaderOperationRemove removes the header
	HeaderOperationRemove HeaderOperation = "remove"
)

// URLRewriteConfig defines URL rewrite configuration
type URLRewriteConfig struct {
	// Path is the new path configuration
	// +kubebuilder:validation:Optional
	Path *RewriteValue `json:"path,omitempty"`

	// Query is the new query string configuration
	// +kubebuilder:validation:Optional
	Query *RewriteValue `json:"query,omitempty"`
}

// RewriteValue defines a rewrite value (static or dynamic)
type RewriteValue struct {
	// Static is a literal value
	// +kubebuilder:validation:Optional
	Static string `json:"static,omitempty"`

	// Expression is a dynamic expression using Cloudflare Rules language
	// Example: concat("/api/v2", http.request.uri.path)
	// +kubebuilder:validation:Optional
	Expression string `json:"expression,omitempty"`
}

// HeaderModification defines a header modification
type HeaderModification struct {
	// Name is the header name
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Operation is the operation to perform
	// +kubebuilder:validation:Required
	Operation HeaderOperation `json:"operation"`

	// Value is the static header value (for set/add operations)
	// +kubebuilder:validation:Optional
	Value string `json:"value,omitempty"`

	// Expression is a dynamic expression for the value
	// Example: ip.geoip.country
	// +kubebuilder:validation:Optional
	Expression string `json:"expression,omitempty"`
}

// TransformRuleDefinition defines a single transform rule
type TransformRuleDefinition struct {
	// Name is a human-readable name for the rule
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Expression is the filter expression (Cloudflare Rules language)
	// Example: (http.host eq "example.com")
	// +kubebuilder:validation:Required
	Expression string `json:"expression"`

	// Enabled controls whether the rule is active
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// URLRewrite contains URL rewrite configuration
	// Only used when type is url_rewrite
	// +kubebuilder:validation:Optional
	URLRewrite *URLRewriteConfig `json:"urlRewrite,omitempty"`

	// Headers contains header modification configuration
	// Only used when type is request_header or response_header
	// +kubebuilder:validation:Optional
	Headers []HeaderModification `json:"headers,omitempty"`
}

// TransformRuleSpec defines the desired state of TransformRule
type TransformRuleSpec struct {
	// Zone is the zone name (domain) to apply rules to
	// +kubebuilder:validation:Required
	Zone string `json:"zone"`

	// Type is the type of transform rule
	// +kubebuilder:validation:Required
	Type TransformRuleType `json:"type"`

	// Description is a human-readable description of the ruleset
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`

	// Rules are the transform rules
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Rules []TransformRuleDefinition `json:"rules"`

	// CredentialsRef references a CloudflareCredentials resource
	// If not specified, the default CloudflareCredentials will be used
	// +kubebuilder:validation:Optional
	CredentialsRef *CredentialsReference `json:"credentialsRef,omitempty"`
}

// TransformRuleStatus defines the observed state of TransformRule
type TransformRuleStatus struct {
	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// State represents the current state of the rule
	// +optional
	State TransformRuleState `json:"state,omitempty"`

	// RulesetID is the Cloudflare ruleset ID
	// +optional
	RulesetID string `json:"rulesetId,omitempty"`

	// ZoneID is the Cloudflare zone ID
	// +optional
	ZoneID string `json:"zoneId,omitempty"`

	// RuleCount is the number of rules
	// +optional
	RuleCount int `json:"ruleCount,omitempty"`

	// Message provides additional information about the current state
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cftransform;transformrule
// +kubebuilder:printcolumn:name="Zone",type=string,JSONPath=`.spec.zone`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Rules",type=integer,JSONPath=`.status.ruleCount`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// TransformRule manages Cloudflare Transform Rules.
// Transform Rules allow you to modify HTTP requests and responses:
// - URL Rewrites: Change the URL path and/or query string
// - Request Headers: Add, modify, or remove HTTP request headers
// - Response Headers: Add, modify, or remove HTTP response headers
//
// This is a simplified interface over ZoneRuleset for common transform use cases.
type TransformRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TransformRuleSpec   `json:"spec,omitempty"`
	Status TransformRuleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TransformRuleList contains a list of TransformRule
type TransformRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TransformRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TransformRule{}, &TransformRuleList{})
}
