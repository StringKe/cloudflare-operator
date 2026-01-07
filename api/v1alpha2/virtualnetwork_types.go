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

// VirtualNetworkSpec defines the desired state of VirtualNetwork
type VirtualNetworkSpec struct {
	// Name of the Virtual Network in Cloudflare.
	// If not specified, the Kubernetes resource name will be used.
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`

	// Comment is an optional description for the Virtual Network.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=500
	Comment string `json:"comment,omitempty"`

	// IsDefaultNetwork marks this Virtual Network as the default for the account.
	// Only one Virtual Network can be the default.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	IsDefaultNetwork bool `json:"isDefaultNetwork,omitempty"`

	// Cloudflare contains the Cloudflare API credentials and account information.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`
}

// VirtualNetworkStatus defines the observed state of VirtualNetwork
type VirtualNetworkStatus struct {
	// VirtualNetworkId is the Cloudflare ID of the Virtual Network.
	// +kubebuilder:validation:Optional
	VirtualNetworkId string `json:"virtualNetworkId,omitempty"`

	// AccountId is the Cloudflare Account ID.
	// +kubebuilder:validation:Optional
	AccountId string `json:"accountId,omitempty"`

	// State indicates the current state of the Virtual Network (active, deleted, etc.).
	// +kubebuilder:validation:Optional
	State string `json:"state,omitempty"`

	// IsDefault indicates whether this is the default Virtual Network for the account.
	// +kubebuilder:validation:Optional
	IsDefault bool `json:"isDefault,omitempty"`

	// Conditions represent the latest available observations of the VirtualNetwork's state.
	// +kubebuilder:validation:Optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +kubebuilder:validation:Optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=vnet
// +kubebuilder:printcolumn:name="VNetID",type=string,JSONPath=`.status.virtualNetworkId`
// +kubebuilder:printcolumn:name="Default",type=boolean,JSONPath=`.status.isDefault`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// VirtualNetwork is the Schema for the virtualnetworks API.
// A VirtualNetwork represents a Cloudflare Zero Trust Virtual Network,
// which provides isolated private network address spaces for routing traffic
// through Cloudflare Tunnels.
type VirtualNetwork struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualNetworkSpec   `json:"spec,omitempty"`
	Status VirtualNetworkStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VirtualNetworkList contains a list of VirtualNetwork
type VirtualNetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualNetwork `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VirtualNetwork{}, &VirtualNetworkList{})
}

// GetVirtualNetworkName returns the name to use in Cloudflare.
// Uses spec.name if specified, otherwise falls back to metadata.name.
func (v *VirtualNetwork) GetVirtualNetworkName() string {
	if v.Spec.Name != "" {
		return v.Spec.Name
	}
	return v.Name
}
