/*
Copyright 2025 Adyanth H.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccessServiceTokenSpec defines the desired state of AccessServiceToken
type AccessServiceTokenSpec struct {
	// Name of the Service Token in Cloudflare.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name,omitempty"`

	// Duration is the validity duration (e.g., "8760h" for 1 year, "forever").
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="8760h"
	Duration string `json:"duration,omitempty"`

	// SecretRef is where to store the generated token credentials.
	// +kubebuilder:validation:Required
	SecretRef ServiceTokenSecretRef `json:"secretRef"`

	// Cloudflare contains the Cloudflare API credentials.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`
}

// ServiceTokenSecretRef defines where to store token credentials.
type ServiceTokenSecretRef struct {
	// Name is the name of the Secret to create/update.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace for the Secret.
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// ClientIDKey is the key for the Client ID.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="CF_ACCESS_CLIENT_ID"
	ClientIDKey string `json:"clientIdKey,omitempty"`

	// ClientSecretKey is the key for the Client Secret.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="CF_ACCESS_CLIENT_SECRET"
	ClientSecretKey string `json:"clientSecretKey,omitempty"`
}

// AccessServiceTokenStatus defines the observed state
type AccessServiceTokenStatus struct {
	// TokenID is the Cloudflare Service Token ID.
	// +kubebuilder:validation:Optional
	TokenID string `json:"tokenId,omitempty"`

	// ClientID is the Service Token Client ID.
	// +kubebuilder:validation:Optional
	ClientID string `json:"clientId,omitempty"`

	// AccountID is the Cloudflare Account ID.
	// +kubebuilder:validation:Optional
	AccountID string `json:"accountId,omitempty"`

	// ExpiresAt is when the token expires.
	// +kubebuilder:validation:Optional
	ExpiresAt string `json:"expiresAt,omitempty"`

	// SecretName is the name of the Secret containing credentials.
	// +kubebuilder:validation:Optional
	SecretName string `json:"secretName,omitempty"`

	// State indicates the current state.
	// +kubebuilder:validation:Optional
	State string `json:"state,omitempty"`

	// Conditions represent the latest available observations.
	// +kubebuilder:validation:Optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed.
	// +kubebuilder:validation:Optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=accesstoken
// +kubebuilder:printcolumn:name="TokenID",type=string,JSONPath=`.status.tokenId`
// +kubebuilder:printcolumn:name="ClientID",type=string,JSONPath=`.status.clientId`
// +kubebuilder:printcolumn:name="ExpiresAt",type=string,JSONPath=`.status.expiresAt`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AccessServiceToken is the Schema for the accessservicetokens API.
type AccessServiceToken struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccessServiceTokenSpec   `json:"spec,omitempty"`
	Status AccessServiceTokenStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AccessServiceTokenList contains a list of AccessServiceToken
type AccessServiceTokenList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccessServiceToken `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AccessServiceToken{}, &AccessServiceTokenList{})
}

// GetTokenName returns the name to use in Cloudflare.
func (a *AccessServiceToken) GetTokenName() string {
	if a.Spec.Name != "" {
		return a.Spec.Name
	}
	return a.Name
}
