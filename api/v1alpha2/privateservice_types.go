// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrivateServiceSpec defines the desired state of PrivateService
type PrivateServiceSpec struct {
	// ServiceRef references the Kubernetes Service to expose privately.
	// The Service must be in the same namespace as the PrivateService.
	// +kubebuilder:validation:Required
	ServiceRef ServiceRef `json:"serviceRef"`

	// TunnelRef references the Tunnel or ClusterTunnel that will handle this private service.
	// +kubebuilder:validation:Required
	TunnelRef TunnelRef `json:"tunnelRef"`

	// VirtualNetworkRef references the VirtualNetwork for this private service.
	// If not specified, the default Virtual Network will be used.
	// +kubebuilder:validation:Optional
	VirtualNetworkRef *VirtualNetworkRef `json:"virtualNetworkRef,omitempty"`

	// Protocol specifies the protocol to use for the private service.
	//
	// Deprecated: Protocol handling is automatic in cloudflared. Tunnel Routes operate at the
	// network layer (IP CIDR) and cloudflared handles protocol detection automatically.
	// This field is kept for documentation purposes only and is not passed to the Cloudflare API.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=tcp;udp
	// +kubebuilder:default=tcp
	Protocol string `json:"protocol,omitempty"`

	// Comment is an optional description for the private service.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=500
	Comment string `json:"comment,omitempty"`

	// Cloudflare contains the Cloudflare API credentials and account information.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`
}

// ServiceRef references a Kubernetes Service.
type ServiceRef struct {
	// Name is the name of the Service.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Port is the port of the Service to expose.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
}

// PrivateServiceStatus defines the observed state of PrivateService
type PrivateServiceStatus struct {
	// Network is the CIDR that was created for this private service.
	// +kubebuilder:validation:Optional
	Network string `json:"network,omitempty"`

	// ServiceIP is the ClusterIP of the referenced Service.
	// +kubebuilder:validation:Optional
	ServiceIP string `json:"serviceIP,omitempty"`

	// TunnelID is the Cloudflare Tunnel ID this service routes through.
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

	// State indicates the current state of the private service.
	// +kubebuilder:validation:Optional
	State string `json:"state,omitempty"`

	// Conditions represent the latest available observations of the PrivateService's state.
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
// +kubebuilder:resource:shortName=psvc
// +kubebuilder:printcolumn:name="Service",type=string,JSONPath=`.spec.serviceRef.name`
// +kubebuilder:printcolumn:name="Port",type=integer,JSONPath=`.spec.serviceRef.port`
// +kubebuilder:printcolumn:name="Network",type=string,JSONPath=`.status.network`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PrivateService is the Schema for the privateservices API.
// A PrivateService exposes a Kubernetes Service privately through a Cloudflare Tunnel,
// making it accessible only to authenticated WARP clients.
type PrivateService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PrivateServiceSpec   `json:"spec,omitempty"`
	Status PrivateServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PrivateServiceList contains a list of PrivateService
type PrivateServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PrivateService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PrivateService{}, &PrivateServiceList{})
}
