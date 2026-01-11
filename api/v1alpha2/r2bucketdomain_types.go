// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// R2BucketDomainState represents the state of the R2 bucket domain
// +kubebuilder:validation:Enum=Pending;Initializing;Active;Error
type R2BucketDomainState string

const (
	// R2BucketDomainStatePending means the domain is waiting to be configured
	R2BucketDomainStatePending R2BucketDomainState = "Pending"
	// R2BucketDomainStateInitializing means the domain is being configured
	R2BucketDomainStateInitializing R2BucketDomainState = "Initializing"
	// R2BucketDomainStateActive means the domain is active and serving content
	R2BucketDomainStateActive R2BucketDomainState = "Active"
	// R2BucketDomainStateError means there was an error configuring the domain
	R2BucketDomainStateError R2BucketDomainState = "Error"
)

// R2BucketDomainMinTLS represents the minimum TLS version
// +kubebuilder:validation:Enum="1.0";"1.1";"1.2";"1.3"
type R2BucketDomainMinTLS string

const (
	// R2BucketDomainMinTLS10 is TLS 1.0
	R2BucketDomainMinTLS10 R2BucketDomainMinTLS = "1.0"
	// R2BucketDomainMinTLS11 is TLS 1.1
	R2BucketDomainMinTLS11 R2BucketDomainMinTLS = "1.1"
	// R2BucketDomainMinTLS12 is TLS 1.2
	R2BucketDomainMinTLS12 R2BucketDomainMinTLS = "1.2"
	// R2BucketDomainMinTLS13 is TLS 1.3
	R2BucketDomainMinTLS13 R2BucketDomainMinTLS = "1.3"
)

// R2BucketDomainSpec defines the desired state of R2BucketDomain
type R2BucketDomainSpec struct {
	// BucketName is the name of the R2 bucket to attach the domain to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=3
	BucketName string `json:"bucketName"`

	// Domain is the custom domain name to attach to the bucket
	// The domain must belong to a zone in the same Cloudflare account
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`
	Domain string `json:"domain"`

	// ZoneID is the Cloudflare zone ID for the domain
	// If not specified, it will be looked up automatically
	// +kubebuilder:validation:Optional
	ZoneID string `json:"zoneId,omitempty"`

	// MinTLS sets the minimum TLS version for the custom domain
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="1.2"
	MinTLS R2BucketDomainMinTLS `json:"minTls,omitempty"`

	// EnablePublicAccess enables public access to the bucket via this domain
	// When true, the bucket contents can be accessed without authentication
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	EnablePublicAccess bool `json:"enablePublicAccess,omitempty"`

	// CredentialsRef references a CloudflareCredentials resource
	// If not specified, the default CloudflareCredentials will be used
	// +kubebuilder:validation:Optional
	CredentialsRef *CredentialsReference `json:"credentialsRef,omitempty"`
}

// R2BucketDomainStatus defines the observed state of R2BucketDomain
type R2BucketDomainStatus struct {
	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// State represents the current state of the domain
	// +optional
	State R2BucketDomainState `json:"state,omitempty"`

	// Message provides additional information about the current state
	// +optional
	Message string `json:"message,omitempty"`

	// DomainID is the Cloudflare domain configuration ID
	// +optional
	DomainID string `json:"domainId,omitempty"`

	// ZoneID is the resolved zone ID for the domain
	// +optional
	ZoneID string `json:"zoneId,omitempty"`

	// Enabled indicates if the domain is enabled
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// MinTLS is the configured minimum TLS version
	// +optional
	MinTLS string `json:"minTls,omitempty"`

	// PublicAccessEnabled indicates if public access is enabled
	// +optional
	PublicAccessEnabled bool `json:"publicAccessEnabled,omitempty"`

	// URL is the full URL to access the bucket via this domain
	// +optional
	URL string `json:"url,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cfr2d;r2domain
// +kubebuilder:printcolumn:name="Bucket",type=string,JSONPath=`.spec.bucketName`
// +kubebuilder:printcolumn:name="Domain",type=string,JSONPath=`.spec.domain`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Public",type=boolean,JSONPath=`.status.publicAccessEnabled`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// R2BucketDomain is the Schema for the r2bucketdomains API
// It configures a custom domain for an R2 storage bucket
type R2BucketDomain struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   R2BucketDomainSpec   `json:"spec,omitempty"`
	Status R2BucketDomainStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// R2BucketDomainList contains a list of R2BucketDomain
type R2BucketDomainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []R2BucketDomain `json:"items"`
}

func init() {
	SchemeBuilder.Register(&R2BucketDomain{}, &R2BucketDomainList{})
}
