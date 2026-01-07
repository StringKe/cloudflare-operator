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

// NetworkRouteSpec defines the desired state of NetworkRoute
type NetworkRouteSpec struct {
	// Network is the CIDR notation for the IP range to route (e.g., "10.0.0.0/8").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([0-9]{1,3}\.){3}[0-9]{1,3}/[0-9]{1,2}$`
	Network string `json:"network"`

	// TunnelRef references the Tunnel or ClusterTunnel that will handle this route.
	// +kubebuilder:validation:Required
	TunnelRef TunnelRef `json:"tunnelRef"`

	// VirtualNetworkRef references the VirtualNetwork for this route.
	// If not specified, the default Virtual Network will be used.
	// +kubebuilder:validation:Optional
	VirtualNetworkRef *VirtualNetworkRef `json:"virtualNetworkRef,omitempty"`

	// Comment is an optional description for the route.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=500
	Comment string `json:"comment,omitempty"`

	// Cloudflare contains the Cloudflare API credentials and account information.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`
}

// TunnelRef references a Tunnel or ClusterTunnel resource.
type TunnelRef struct {
	// Kind is the type of tunnel resource (Tunnel or ClusterTunnel).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Tunnel;ClusterTunnel
	// +kubebuilder:default=ClusterTunnel
	Kind string `json:"kind"`

	// Name is the name of the Tunnel or ClusterTunnel resource.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the Tunnel resource.
	// Only applicable when Kind is Tunnel. Ignored for ClusterTunnel.
	// +kubebuilder:validation:Optional
	Namespace string `json:"namespace,omitempty"`
}

// VirtualNetworkRef references a VirtualNetwork resource.
type VirtualNetworkRef struct {
	// Name is the name of the VirtualNetwork resource.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// NetworkRouteStatus defines the observed state of NetworkRoute
type NetworkRouteStatus struct {
	// Network is the CIDR from the route in Cloudflare.
	// +kubebuilder:validation:Optional
	Network string `json:"network,omitempty"`

	// TunnelID is the Cloudflare Tunnel ID this route points to.
	// +kubebuilder:validation:Optional
	TunnelID string `json:"tunnelId,omitempty"`

	// TunnelName is the name of the Tunnel in Cloudflare.
	// +kubebuilder:validation:Optional
	TunnelName string `json:"tunnelName,omitempty"`

	// VirtualNetworkID is the Cloudflare Virtual Network ID.
	// +kubebuilder:validation:Optional
	VirtualNetworkID string `json:"virtualNetworkId,omitempty"`

	// AccountID is the Cloudflare Account ID.
	// +kubebuilder:validation:Optional
	AccountID string `json:"accountId,omitempty"`

	// State indicates the current state of the route.
	// +kubebuilder:validation:Optional
	State string `json:"state,omitempty"`

	// Conditions represent the latest available observations of the NetworkRoute's state.
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
// +kubebuilder:resource:scope=Cluster,shortName=netroute
// +kubebuilder:printcolumn:name="Network",type=string,JSONPath=`.spec.network`
// +kubebuilder:printcolumn:name="TunnelID",type=string,JSONPath=`.status.tunnelId`
// +kubebuilder:printcolumn:name="VNetID",type=string,JSONPath=`.status.virtualNetworkId`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NetworkRoute is the Schema for the networkroutes API.
// A NetworkRoute defines a CIDR range to be routed through a Cloudflare Tunnel,
// enabling private network access via WARP clients.
type NetworkRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkRouteSpec   `json:"spec,omitempty"`
	Status NetworkRouteStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NetworkRouteList contains a list of NetworkRoute
type NetworkRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkRoute `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkRoute{}, &NetworkRouteList{})
}
