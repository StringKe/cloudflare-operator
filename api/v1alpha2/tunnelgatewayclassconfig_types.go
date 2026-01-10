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

// TunnelGatewayClassConfigSpec defines the desired state of TunnelGatewayClassConfig
type TunnelGatewayClassConfigSpec struct {
	// TunnelRef references the Tunnel or ClusterTunnel to use for this GatewayClass
	// +kubebuilder:validation:Required
	TunnelRef TunnelReference `json:"tunnelRef"`

	// DefaultOriginRequest provides default origin request settings for all Routes
	// using this GatewayClass. Can be overridden per-Route via annotations.
	// +kubebuilder:validation:Optional
	DefaultOriginRequest *OriginRequestSpec `json:"defaultOriginRequest,omitempty"`

	// DNSManagement controls how DNS records are managed for Route hostnames.
	// - Automatic: Controller creates CNAME records directly via Cloudflare API
	// - Manual: User manages DNS records externally (compatible with external-dns)
	// - DNSRecord: Controller creates DNSRecord CRDs for each hostname
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=Automatic
	DNSManagement DNSManagementMode `json:"dnsManagement,omitempty"`

	// DNSProxied controls whether DNS records are proxied through Cloudflare.
	// Only applies when DNSManagement is Automatic or DNSRecord.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	DNSProxied *bool `json:"dnsProxied,omitempty"`

	// WatchNamespaces limits which namespaces the controller watches for Routes.
	// If empty, watches all namespaces.
	// +kubebuilder:validation:Optional
	WatchNamespaces []string `json:"watchNamespaces,omitempty"`

	// FallbackTarget is the default target for unmatched requests.
	// Defaults to "http_status:404" if not specified.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="http_status:404"
	FallbackTarget string `json:"fallbackTarget,omitempty"`
}

// TunnelGatewayClassConfigStatus defines the observed state of TunnelGatewayClassConfig
type TunnelGatewayClassConfigStatus struct {
	// TunnelID is the resolved Cloudflare Tunnel ID
	// +kubebuilder:validation:Optional
	TunnelID string `json:"tunnelId,omitempty"`

	// TunnelName is the resolved Cloudflare Tunnel name
	// +kubebuilder:validation:Optional
	TunnelName string `json:"tunnelName,omitempty"`

	// GatewayCount is the number of Gateways using this configuration
	// +kubebuilder:validation:Optional
	GatewayCount int `json:"gatewayCount,omitempty"`

	// RouteCount is the total number of Routes across all Gateways
	// +kubebuilder:validation:Optional
	RouteCount int `json:"routeCount,omitempty"`

	// State represents the current state of the configuration
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=pending;active;error
	State string `json:"state,omitempty"`

	// Conditions represent the latest available observations
	// +kubebuilder:validation:Optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed
	// +kubebuilder:validation:Optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=tgcc
// +kubebuilder:printcolumn:name="Tunnel",type=string,JSONPath=`.spec.tunnelRef.name`
// +kubebuilder:printcolumn:name="Kind",type=string,JSONPath=`.spec.tunnelRef.kind`
// +kubebuilder:printcolumn:name="DNS",type=string,JSONPath=`.spec.dnsManagement`
// +kubebuilder:printcolumn:name="Gateways",type=integer,JSONPath=`.status.gatewayCount`
// +kubebuilder:printcolumn:name="Routes",type=integer,JSONPath=`.status.routeCount`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// TunnelGatewayClassConfig provides GatewayClass parameters for Cloudflare Tunnel Gateway Controller.
// This resource links a GatewayClass to a specific Tunnel or ClusterTunnel and configures
// how the Gateway Controller handles DNS records and origin connections.
//
// Example usage:
//
//	apiVersion: networking.cloudflare-operator.io/v1alpha2
//	kind: TunnelGatewayClassConfig
//	metadata:
//	  name: cloudflare-tunnel
//	spec:
//	  tunnelRef:
//	    kind: ClusterTunnel
//	    name: production-tunnel
//	  dnsManagement: Automatic
//	  dnsProxied: true
type TunnelGatewayClassConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TunnelGatewayClassConfigSpec   `json:"spec,omitempty"`
	Status TunnelGatewayClassConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TunnelGatewayClassConfigList contains a list of TunnelGatewayClassConfig
type TunnelGatewayClassConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TunnelGatewayClassConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TunnelGatewayClassConfig{}, &TunnelGatewayClassConfigList{})
}

// GetTunnelNamespace returns the namespace where the Tunnel resource is located.
// For ClusterTunnel, this returns empty string (cluster-scoped).
// For Tunnel, this returns the namespace from TunnelRef or falls back to config's namespace.
func (c *TunnelGatewayClassConfig) GetTunnelNamespace() string {
	if c.Spec.TunnelRef.Kind == "ClusterTunnel" {
		return ""
	}
	if c.Spec.TunnelRef.Namespace != "" {
		return c.Spec.TunnelRef.Namespace
	}
	return c.Namespace
}

// IsDNSProxied returns whether DNS records should be proxied through Cloudflare.
func (c *TunnelGatewayClassConfig) IsDNSProxied() bool {
	if c.Spec.DNSProxied == nil {
		return true // Default to proxied
	}
	return *c.Spec.DNSProxied
}
