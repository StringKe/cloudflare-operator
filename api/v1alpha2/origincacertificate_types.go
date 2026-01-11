// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OriginCACertificateState represents the state of the certificate
// +kubebuilder:validation:Enum=Pending;Issuing;Ready;Renewing;Error;Revoked
type OriginCACertificateState string

const (
	// OriginCACertificateStatePending means the certificate is waiting to be issued
	OriginCACertificateStatePending OriginCACertificateState = "Pending"
	// OriginCACertificateStateIssuing means the certificate is being issued
	OriginCACertificateStateIssuing OriginCACertificateState = "Issuing"
	// OriginCACertificateStateReady means the certificate is issued and ready
	OriginCACertificateStateReady OriginCACertificateState = "Ready"
	// OriginCACertificateStateRenewing means the certificate is being renewed
	OriginCACertificateStateRenewing OriginCACertificateState = "Renewing"
	// OriginCACertificateStateError means there was an error with the certificate
	OriginCACertificateStateError OriginCACertificateState = "Error"
	// OriginCACertificateStateRevoked means the certificate has been revoked
	OriginCACertificateStateRevoked OriginCACertificateState = "Revoked"
)

// CertificateRequestType represents the type of certificate to request
// +kubebuilder:validation:Enum=origin-rsa;origin-ecc
type CertificateRequestType string

const (
	// CertificateRequestTypeOriginRSA requests an RSA certificate
	CertificateRequestTypeOriginRSA CertificateRequestType = "origin-rsa"
	// CertificateRequestTypeOriginECC requests an ECC certificate
	CertificateRequestTypeOriginECC CertificateRequestType = "origin-ecc"
)

// CertificateValidity represents the validity period of the certificate in days
// +kubebuilder:validation:Enum=7;30;90;365;730;1095;5475
type CertificateValidity int

const (
	// CertificateValidity7Days is 7 days validity
	CertificateValidity7Days CertificateValidity = 7
	// CertificateValidity30Days is 30 days validity
	CertificateValidity30Days CertificateValidity = 30
	// CertificateValidity90Days is 90 days validity
	CertificateValidity90Days CertificateValidity = 90
	// CertificateValidity1Year is 365 days validity
	CertificateValidity1Year CertificateValidity = 365
	// CertificateValidity2Years is 730 days validity
	CertificateValidity2Years CertificateValidity = 730
	// CertificateValidity3Years is 1095 days validity
	CertificateValidity3Years CertificateValidity = 1095
	// CertificateValidity15Years is 5475 days validity (maximum)
	CertificateValidity15Years CertificateValidity = 5475
)

// SecretSyncConfig configures how the certificate is synced to a Kubernetes Secret
type SecretSyncConfig struct {
	// Enabled enables syncing the certificate to a Kubernetes Secret
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// SecretName is the name of the Secret to create/update
	// If not specified, defaults to the OriginCACertificate name
	// +kubebuilder:validation:Optional
	SecretName string `json:"secretName,omitempty"`

	// Namespace is the namespace for the Secret
	// If not specified, defaults to the OriginCACertificate's namespace (for namespaced)
	// or "cloudflare-operator-system" (for cluster-scoped)
	// +kubebuilder:validation:Optional
	Namespace string `json:"namespace,omitempty"`

	// CertManagerCompatible creates the Secret in cert-manager compatible format
	// When true, uses "tls.crt" and "tls.key" keys with kubernetes.io/tls type
	// When false, uses "certificate" and "private-key" keys with Opaque type
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	CertManagerCompatible bool `json:"certManagerCompatible,omitempty"`

	// IncludeCA includes the Cloudflare Origin CA root certificate in the Secret
	// This is useful for clients that need to verify the certificate chain
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	IncludeCA bool `json:"includeCA,omitempty"`
}

// PrivateKeySpec configures how the private key is handled
type PrivateKeySpec struct {
	// Algorithm specifies the private key algorithm
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=RSA;ECDSA
	// +kubebuilder:default=RSA
	Algorithm string `json:"algorithm,omitempty"`

	// Size specifies the key size in bits (for RSA) or curve (for ECDSA)
	// For RSA: 2048, 4096. For ECDSA: 256, 384
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=2048
	Size int `json:"size,omitempty"`

	// SecretRef references an existing Secret containing the private key
	// If specified, the controller will use this key instead of generating one
	// The Secret must contain a "private-key" or "tls.key" key
	// +kubebuilder:validation:Optional
	SecretRef *SecretKeyReference `json:"secretRef,omitempty"`
}

// SecretKeyReference references a specific key in a Secret
type SecretKeyReference struct {
	// Name of the Secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the Secret
	// +kubebuilder:validation:Optional
	Namespace string `json:"namespace,omitempty"`

	// Key is the key in the Secret data
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=tls.key
	Key string `json:"key,omitempty"`
}

// RenewalConfig configures automatic certificate renewal
type RenewalConfig struct {
	// Enabled enables automatic renewal
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// RenewBeforeDays specifies how many days before expiration to renew
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=365
	// +kubebuilder:default=30
	RenewBeforeDays int `json:"renewBeforeDays,omitempty"`
}

// OriginCACertificateSpec defines the desired state of OriginCACertificate
type OriginCACertificateSpec struct {
	// Hostnames are the domain names the certificate should be valid for
	// Supports wildcards (e.g., "*.example.com")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Hostnames []string `json:"hostnames"`

	// RequestType specifies the certificate type (RSA or ECC)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=origin-rsa
	RequestType CertificateRequestType `json:"requestType,omitempty"`

	// Validity specifies the certificate validity period in days
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=5475
	Validity CertificateValidity `json:"validity,omitempty"`

	// CredentialsRef references a CloudflareCredentials resource
	// If not specified, the default CloudflareCredentials will be used
	// +kubebuilder:validation:Optional
	CredentialsRef *CredentialsReference `json:"credentialsRef,omitempty"`

	// PrivateKey configures the private key generation or reference
	// +kubebuilder:validation:Optional
	PrivateKey *PrivateKeySpec `json:"privateKey,omitempty"`

	// SecretSync configures syncing the certificate to a Kubernetes Secret
	// +kubebuilder:validation:Optional
	SecretSync *SecretSyncConfig `json:"secretSync,omitempty"`

	// Renewal configures automatic certificate renewal
	// +kubebuilder:validation:Optional
	Renewal *RenewalConfig `json:"renewal,omitempty"`

	// CSR is an optional Certificate Signing Request
	// If provided, the controller will use this CSR instead of generating one
	// +kubebuilder:validation:Optional
	CSR string `json:"csr,omitempty"`
}

// OriginCACertificateStatus defines the observed state of OriginCACertificate
type OriginCACertificateStatus struct {
	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// State represents the current state of the certificate
	// +optional
	State OriginCACertificateState `json:"state,omitempty"`

	// CertificateID is the Cloudflare certificate ID
	// +optional
	CertificateID string `json:"certificateId,omitempty"`

	// Certificate is the PEM-encoded certificate (public key)
	// +optional
	Certificate string `json:"certificate,omitempty"`

	// ExpiresAt is the certificate expiration time
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`

	// IssuedAt is the time the certificate was issued
	// +optional
	IssuedAt *metav1.Time `json:"issuedAt,omitempty"`

	// RevokedAt is the time the certificate was revoked (if revoked)
	// +optional
	RevokedAt *metav1.Time `json:"revokedAt,omitempty"`

	// RenewalTime is the next scheduled renewal time
	// +optional
	RenewalTime *metav1.Time `json:"renewalTime,omitempty"`

	// SecretName is the name of the synced Secret
	// +optional
	SecretName string `json:"secretName,omitempty"`

	// SecretNamespace is the namespace of the synced Secret
	// +optional
	SecretNamespace string `json:"secretNamespace,omitempty"`

	// Message provides additional information about the current state
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cforiginca;cfoca
// +kubebuilder:printcolumn:name="Hostnames",type=string,JSONPath=`.spec.hostnames[0]`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Expires",type=date,JSONPath=`.status.expiresAt`
// +kubebuilder:printcolumn:name="Secret",type=string,JSONPath=`.status.secretName`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// OriginCACertificate manages Cloudflare Origin CA certificates.
// These certificates are trusted by Cloudflare's edge servers and can be used
// for SSL/TLS encryption between Cloudflare and your origin server.
//
// The controller can optionally sync the certificate to a Kubernetes Secret
// in cert-manager compatible format for use with Ingress or other TLS consumers.
type OriginCACertificate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OriginCACertificateSpec   `json:"spec,omitempty"`
	Status OriginCACertificateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OriginCACertificateList contains a list of OriginCACertificate
type OriginCACertificateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OriginCACertificate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OriginCACertificate{}, &OriginCACertificateList{})
}
