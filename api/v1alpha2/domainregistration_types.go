// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DomainRegistrationState represents the state of the domain registration
// +kubebuilder:validation:Enum=Pending;Syncing;Active;TransferPending;Expired;Error
type DomainRegistrationState string

const (
	// DomainRegistrationStatePending means the domain is waiting to be synced
	DomainRegistrationStatePending DomainRegistrationState = "Pending"
	// DomainRegistrationStateSyncing means the domain settings are being synced
	DomainRegistrationStateSyncing DomainRegistrationState = "Syncing"
	// DomainRegistrationStateActive means the domain is registered and active
	DomainRegistrationStateActive DomainRegistrationState = "Active"
	// DomainRegistrationStateTransferPending means a transfer is in progress
	DomainRegistrationStateTransferPending DomainRegistrationState = "TransferPending"
	// DomainRegistrationStateExpired means the domain has expired
	DomainRegistrationStateExpired DomainRegistrationState = "Expired"
	// DomainRegistrationStateError means there was an error with the domain
	DomainRegistrationStateError DomainRegistrationState = "Error"
)

// RegistrantContact contains the registrant contact information
type RegistrantContact struct {
	// FirstName is the registrant's first name
	// +kubebuilder:validation:Required
	FirstName string `json:"firstName"`

	// LastName is the registrant's last name
	// +kubebuilder:validation:Required
	LastName string `json:"lastName"`

	// Organization is the registrant's organization (optional)
	// +kubebuilder:validation:Optional
	Organization string `json:"organization,omitempty"`

	// Address is the street address
	// +kubebuilder:validation:Required
	Address string `json:"address"`

	// Address2 is the secondary address line (optional)
	// +kubebuilder:validation:Optional
	Address2 string `json:"address2,omitempty"`

	// City is the city
	// +kubebuilder:validation:Required
	City string `json:"city"`

	// State is the state/province
	// +kubebuilder:validation:Required
	State string `json:"state"`

	// Zip is the postal/zip code
	// +kubebuilder:validation:Required
	Zip string `json:"zip"`

	// Country is the two-letter country code (ISO 3166-1 alpha-2)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[A-Z]{2}$`
	Country string `json:"country"`

	// Phone is the phone number in E.164 format
	// +kubebuilder:validation:Required
	Phone string `json:"phone"`

	// Email is the contact email address
	// +kubebuilder:validation:Required
	Email string `json:"email"`

	// Fax is the fax number (optional)
	// +kubebuilder:validation:Optional
	Fax string `json:"fax,omitempty"`
}

// DomainConfiguration contains domain configuration settings
type DomainConfiguration struct {
	// AutoRenew enables automatic domain renewal
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	AutoRenew bool `json:"autoRenew,omitempty"`

	// Privacy enables WHOIS privacy protection
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Privacy bool `json:"privacy,omitempty"`

	// Locked prevents unauthorized transfers
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Locked bool `json:"locked,omitempty"`

	// NameServers specifies custom nameservers (optional)
	// If not specified, Cloudflare nameservers will be used
	// +kubebuilder:validation:Optional
	NameServers []string `json:"nameServers,omitempty"`
}

// DomainRegistrationSpec defines the desired state of DomainRegistration
type DomainRegistrationSpec struct {
	// DomainName is the domain name to manage
	// This domain must already be registered with Cloudflare Registrar
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9][a-zA-Z0-9-]{0,61}[a-zA-Z0-9]\.[a-zA-Z]{2,}$`
	DomainName string `json:"domainName"`

	// Configuration contains domain settings
	// +kubebuilder:validation:Optional
	Configuration *DomainConfiguration `json:"configuration,omitempty"`

	// RegistrantContact contains the registrant contact information
	// If not specified, existing contact information will be preserved
	// +kubebuilder:validation:Optional
	RegistrantContact *RegistrantContact `json:"registrantContact,omitempty"`

	// CredentialsRef references a CloudflareCredentials resource
	// If not specified, the default CloudflareCredentials will be used
	// +kubebuilder:validation:Optional
	CredentialsRef *CredentialsReference `json:"credentialsRef,omitempty"`
}

// DomainRegistrationStatus defines the observed state of DomainRegistration
type DomainRegistrationStatus struct {
	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// State represents the current state of the domain
	// +optional
	State DomainRegistrationState `json:"state,omitempty"`

	// DomainID is the Cloudflare domain ID
	// +optional
	DomainID string `json:"domainId,omitempty"`

	// CurrentRegistrar is the current registrar name
	// +optional
	CurrentRegistrar string `json:"currentRegistrar,omitempty"`

	// RegistryStatuses contains the registry status codes
	// +optional
	RegistryStatuses string `json:"registryStatuses,omitempty"`

	// ExpiresAt is when the domain registration expires
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`

	// CreatedAt is when the domain was registered
	// +optional
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// AutoRenew indicates if auto-renewal is enabled
	// +optional
	AutoRenew bool `json:"autoRenew,omitempty"`

	// Privacy indicates if WHOIS privacy is enabled
	// +optional
	Privacy bool `json:"privacy,omitempty"`

	// Locked indicates if the domain is locked
	// +optional
	Locked bool `json:"locked,omitempty"`

	// TransferInStatus contains transfer status if applicable
	// +optional
	TransferInStatus string `json:"transferInStatus,omitempty"`

	// Message provides additional information about the current state
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cfdomreg;domainreg
// +kubebuilder:printcolumn:name="Domain",type=string,JSONPath=`.spec.domainName`
// +kubebuilder:printcolumn:name="Registrar",type=string,JSONPath=`.status.currentRegistrar`
// +kubebuilder:printcolumn:name="Expires",type=date,JSONPath=`.status.expiresAt`
// +kubebuilder:printcolumn:name="AutoRenew",type=boolean,JSONPath=`.status.autoRenew`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DomainRegistration manages Cloudflare Registrar domain settings.
// This CRD allows you to configure settings for domains registered with
// Cloudflare Registrar, including auto-renewal, WHOIS privacy, and
// transfer lock settings.
//
// Note: This CRD manages existing domains registered with Cloudflare.
// Domain registration itself must be done through the Cloudflare dashboard
// or API directly due to payment and verification requirements.
//
// Enterprise Feature: Some advanced features like registry lock require
// an Enterprise plan.
type DomainRegistration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DomainRegistrationSpec   `json:"spec,omitempty"`
	Status DomainRegistrationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DomainRegistrationList contains a list of DomainRegistration
type DomainRegistrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DomainRegistration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DomainRegistration{}, &DomainRegistrationList{})
}
