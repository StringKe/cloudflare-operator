// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CloudflareAuthType defines the authentication method for Cloudflare API
// +kubebuilder:validation:Enum=apiToken;globalAPIKey
type CloudflareAuthType string

const (
	// AuthTypeAPIToken uses a scoped API Token for authentication
	AuthTypeAPIToken CloudflareAuthType = "apiToken"
	// AuthTypeGlobalAPIKey uses Global API Key + Email for authentication
	AuthTypeGlobalAPIKey CloudflareAuthType = "globalAPIKey"
)

// SecretReference contains information about the secret location
type SecretReference struct {
	// Name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the secret. Defaults to "cloudflare-operator-system"
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:="cloudflare-operator-system"
	Namespace string `json:"namespace,omitempty"`

	// Key in the secret for API Token (used when authType is apiToken)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:="CLOUDFLARE_API_TOKEN"
	APITokenKey string `json:"apiTokenKey,omitempty"`

	// Key in the secret for Global API Key (used when authType is globalAPIKey)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:="CLOUDFLARE_API_KEY"
	APIKeyKey string `json:"apiKeyKey,omitempty"`

	// Key in the secret for Email (used when authType is globalAPIKey)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:="CLOUDFLARE_EMAIL"
	EmailKey string `json:"emailKey,omitempty"`
}

// CloudflareCredentialsSpec defines the desired state of CloudflareCredentials
type CloudflareCredentialsSpec struct {
	// AccountID is the Cloudflare Account ID
	// +kubebuilder:validation:Required
	AccountID string `json:"accountId"`

	// AccountName is an optional human-readable account name (for reference only)
	// +kubebuilder:validation:Optional
	AccountName string `json:"accountName,omitempty"`

	// AuthType specifies the authentication method
	// +kubebuilder:validation:Required
	// +kubebuilder:default:="apiToken"
	AuthType CloudflareAuthType `json:"authType"`

	// SecretRef references the secret containing the API credentials
	// +kubebuilder:validation:Required
	SecretRef SecretReference `json:"secretRef"`

	// DefaultDomain is the default domain for resources using these credentials
	// +kubebuilder:validation:Optional
	DefaultDomain string `json:"defaultDomain,omitempty"`

	// IsDefault marks this as the default credentials for resources that don't specify credentials
	// Only one CloudflareCredentials can be marked as default
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=false
	IsDefault bool `json:"isDefault,omitempty"`
}

// CloudflareCredentialsStatus defines the observed state of CloudflareCredentials
type CloudflareCredentialsStatus struct {
	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// State represents the current state of the credentials
	// +optional
	State string `json:"state,omitempty"`

	// Validated indicates whether the credentials have been validated
	// +optional
	Validated bool `json:"validated,omitempty"`

	// LastValidatedTime is the last time credentials were validated
	// +optional
	LastValidatedTime *metav1.Time `json:"lastValidatedTime,omitempty"`

	// AccountName is the account name retrieved from Cloudflare API
	// +optional
	AccountName string `json:"accountName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cfcreds
// +kubebuilder:printcolumn:name="Account ID",type=string,JSONPath=`.spec.accountId`
// +kubebuilder:printcolumn:name="Auth Type",type=string,JSONPath=`.spec.authType`
// +kubebuilder:printcolumn:name="Default",type=boolean,JSONPath=`.spec.isDefault`
// +kubebuilder:printcolumn:name="Validated",type=boolean,JSONPath=`.status.validated`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// CloudflareCredentials is the Schema for global Cloudflare API credentials
type CloudflareCredentials struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudflareCredentialsSpec   `json:"spec,omitempty"`
	Status CloudflareCredentialsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudflareCredentialsList contains a list of CloudflareCredentials
type CloudflareCredentialsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudflareCredentials `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudflareCredentials{}, &CloudflareCredentialsList{})
}
