// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CloudflareDomainState represents the state of the CloudflareDomain
// +kubebuilder:validation:Enum=Pending;Verifying;Ready;Error
type CloudflareDomainState string

const (
	// CloudflareDomainStatePending means the domain is waiting to be verified
	CloudflareDomainStatePending CloudflareDomainState = "Pending"
	// CloudflareDomainStateVerifying means the domain is being verified with Cloudflare API
	CloudflareDomainStateVerifying CloudflareDomainState = "Verifying"
	// CloudflareDomainStateReady means the domain has been verified and is ready to use
	CloudflareDomainStateReady CloudflareDomainState = "Ready"
	// CloudflareDomainStateError means there was an error verifying the domain
	CloudflareDomainStateError CloudflareDomainState = "Error"
)

// CredentialsReference references a CloudflareCredentials resource
type CredentialsReference struct {
	// Name of the CloudflareCredentials resource
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// CloudflareDomainSpec defines the desired state of CloudflareDomain
type CloudflareDomainSpec struct {
	// Domain is the domain name (e.g., "example.com")
	// This should be the apex domain registered in Cloudflare
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*$`
	Domain string `json:"domain"`

	// CredentialsRef references a CloudflareCredentials resource for API access.
	// If not specified, the default CloudflareCredentials will be used.
	// +kubebuilder:validation:Optional
	CredentialsRef *CredentialsReference `json:"credentialsRef,omitempty"`

	// IsDefault marks this domain as the default for resources that don't specify a domain.
	// Only one CloudflareDomain can be marked as default.
	// When multiple hostnames need zone lookup, the longest suffix match is used.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=false
	IsDefault bool `json:"isDefault,omitempty"`

	// ZoneID allows manual specification of the Cloudflare Zone ID.
	// If provided, the controller will skip zone lookup and use this value directly.
	// This is useful for advanced scenarios or when automatic lookup fails.
	// +kubebuilder:validation:Optional
	ZoneID string `json:"zoneId,omitempty"`
}

// CloudflareDomainStatus defines the observed state of CloudflareDomain
type CloudflareDomainStatus struct {
	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// State represents the current state of the domain
	// +optional
	State CloudflareDomainState `json:"state,omitempty"`

	// ZoneID is the Cloudflare Zone ID for this domain
	// +optional
	ZoneID string `json:"zoneId,omitempty"`

	// ZoneName is the zone name as returned by Cloudflare API
	// +optional
	ZoneName string `json:"zoneName,omitempty"`

	// AccountID is the Cloudflare Account ID associated with this zone
	// +optional
	AccountID string `json:"accountId,omitempty"`

	// NameServers are the Cloudflare name servers for this zone
	// +optional
	NameServers []string `json:"nameServers,omitempty"`

	// ZoneStatus is the status of the zone in Cloudflare (active, pending, etc.)
	// +optional
	ZoneStatus string `json:"zoneStatus,omitempty"`

	// LastVerifiedTime is the last time the zone was verified with Cloudflare API
	// +optional
	LastVerifiedTime *metav1.Time `json:"lastVerifiedTime,omitempty"`

	// Message provides additional information about the current state
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cfdomain;cfdom
// +kubebuilder:printcolumn:name="Domain",type=string,JSONPath=`.spec.domain`
// +kubebuilder:printcolumn:name="Zone ID",type=string,JSONPath=`.status.zoneId`
// +kubebuilder:printcolumn:name="Default",type=boolean,JSONPath=`.spec.isDefault`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// CloudflareDomain represents a domain managed in Cloudflare.
// It provides zone information (Zone ID) for DNS operations across all CRDs.
// The controller verifies the domain exists in Cloudflare and caches the Zone ID.
//
// DomainResolver uses CloudflareDomain resources to match hostnames to zones:
// - Exact match: hostname equals domain
// - Suffix match: hostname ends with ".domain" (longest suffix wins)
//
// Example: For hostname "api.staging.example.com":
// - CloudflareDomain "example.com" matches (suffix)
// - CloudflareDomain "staging.example.com" matches better (longer suffix)
type CloudflareDomain struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudflareDomainSpec   `json:"spec,omitempty"`
	Status CloudflareDomainStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudflareDomainList contains a list of CloudflareDomain
type CloudflareDomainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudflareDomain `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudflareDomain{}, &CloudflareDomainList{})
}
