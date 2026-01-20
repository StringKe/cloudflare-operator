// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PagesDomainState represents the state of the Pages domain
// +kubebuilder:validation:Enum=Pending;Verifying;Active;Moved;Deleting;Error
type PagesDomainState string

const (
	// PagesDomainStatePending means the domain is waiting to be added
	PagesDomainStatePending PagesDomainState = "Pending"
	// PagesDomainStateVerifying means the domain is being verified
	PagesDomainStateVerifying PagesDomainState = "Verifying"
	// PagesDomainStateActive means the domain is active and serving traffic
	PagesDomainStateActive PagesDomainState = "Active"
	// PagesDomainStateMoved means the domain has been moved
	PagesDomainStateMoved PagesDomainState = "Moved"
	// PagesDomainStateDeleting means the domain is being deleted
	PagesDomainStateDeleting PagesDomainState = "Deleting"
	// PagesDomainStateError means there was an error with the domain
	PagesDomainStateError PagesDomainState = "Error"
)

// PagesProjectRef references a PagesProject resource.
type PagesProjectRef struct {
	// Name is the K8s PagesProject resource name.
	// The controller will look up the Cloudflare project name from this resource.
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`

	// CloudflareID is the Cloudflare project name (same as ID for Pages).
	// Use this to reference a project that is not managed by the operator.
	// +kubebuilder:validation:Optional
	CloudflareID string `json:"cloudflareId,omitempty"`

	// CloudflareName is an alias for CloudflareID (same as ID for Pages).
	// Provided for clarity and consistency.
	// +kubebuilder:validation:Optional
	CloudflareName string `json:"cloudflareName,omitempty"`
}

// PagesDomainSpec defines the desired state of PagesDomain
type PagesDomainSpec struct {
	// Domain is the custom domain name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9][a-zA-Z0-9-\.]*[a-zA-Z0-9]$`
	Domain string `json:"domain"`

	// ProjectRef references the PagesProject.
	// Either Name or CloudflareID/CloudflareName must be specified.
	// +kubebuilder:validation:Required
	ProjectRef PagesProjectRef `json:"projectRef"`

	// Cloudflare contains Cloudflare-specific configuration.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`

	// AutoConfigureDNS controls whether to automatically configure DNS records
	// when the domain's zone is in the same Cloudflare account.
	// When true (default), Cloudflare will automatically create CNAME records
	// pointing to the Pages project.
	// When false, DNS must be configured manually (useful for external DNS providers
	// or when you want more control over DNS configuration).
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	AutoConfigureDNS *bool `json:"autoConfigureDNS,omitempty"`

	// ZoneID is the Cloudflare Zone ID for DNS auto-configuration.
	// This is optional. If not provided and AutoConfigureDNS is enabled,
	// the operator will automatically query the zone from Cloudflare API
	// based on the domain name.
	// Providing ZoneID explicitly can improve performance by avoiding API lookups.
	// +kubebuilder:validation:Optional
	ZoneID string `json:"zoneID,omitempty"`

	// DeletionPolicy specifies what happens when the Kubernetes resource is deleted.
	// Delete: The custom domain will be removed from the Pages project.
	// Orphan: The custom domain will be left in Cloudflare.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=Delete;Orphan
	// +kubebuilder:default=Delete
	DeletionPolicy string `json:"deletionPolicy,omitempty"`
}

// PagesDomainStatus defines the observed state of PagesDomain
type PagesDomainStatus struct {
	// DomainID is the Cloudflare domain identifier.
	// +kubebuilder:validation:Optional
	DomainID string `json:"domainId,omitempty"`

	// ProjectName is the Cloudflare project name.
	// +kubebuilder:validation:Optional
	ProjectName string `json:"projectName,omitempty"`

	// AccountID is the Cloudflare account ID.
	// +kubebuilder:validation:Optional
	AccountID string `json:"accountId,omitempty"`

	// ZoneID is the zone ID if the domain is in the same Cloudflare account.
	// +kubebuilder:validation:Optional
	ZoneID string `json:"zoneId,omitempty"`

	// Status is the domain status from Cloudflare (active, pending, moved, etc.).
	// +kubebuilder:validation:Optional
	Status string `json:"status,omitempty"`

	// ValidationMethod is the validation method (txt, http, cname).
	// +kubebuilder:validation:Optional
	ValidationMethod string `json:"validationMethod,omitempty"`

	// ValidationStatus is the validation status.
	// +kubebuilder:validation:Optional
	ValidationStatus string `json:"validationStatus,omitempty"`

	// CertificateAuthority is the Certificate Authority used for SSL.
	// +kubebuilder:validation:Optional
	CertificateAuthority string `json:"certificateAuthority,omitempty"`

	// VerificationData contains DNS verification records if needed.
	// +kubebuilder:validation:Optional
	VerificationData *PagesDomainVerificationData `json:"verificationData,omitempty"`

	// State is the current state of the domain.
	// +kubebuilder:validation:Optional
	State PagesDomainState `json:"state,omitempty"`

	// Conditions represent the latest available observations.
	// +kubebuilder:validation:Optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the last observed generation.
	// +kubebuilder:validation:Optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Message provides additional information about the current state.
	// +kubebuilder:validation:Optional
	Message string `json:"message,omitempty"`
}

// PagesDomainVerificationData contains DNS verification information.
type PagesDomainVerificationData struct {
	// RecordType is the DNS record type required (TXT, CNAME, etc.).
	// +kubebuilder:validation:Optional
	RecordType string `json:"recordType,omitempty"`

	// RecordName is the DNS record name to create.
	// +kubebuilder:validation:Optional
	RecordName string `json:"recordName,omitempty"`

	// RecordValue is the DNS record value to set.
	// +kubebuilder:validation:Optional
	RecordValue string `json:"recordValue,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cfpdom;pagesdomain
// +kubebuilder:printcolumn:name="Domain",type=string,JSONPath=`.spec.domain`
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.status.projectName`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PagesDomain manages a custom domain for a Cloudflare Pages project.
// Custom domains allow you to serve your Pages project from your own domain
// instead of the default *.pages.dev subdomain.
//
// The controller adds and manages custom domains for Pages projects,
// including SSL certificate provisioning and DNS validation.
type PagesDomain struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PagesDomainSpec   `json:"spec,omitempty"`
	Status PagesDomainStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PagesDomainList contains a list of PagesDomain
type PagesDomainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PagesDomain `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PagesDomain{}, &PagesDomainList{})
}
