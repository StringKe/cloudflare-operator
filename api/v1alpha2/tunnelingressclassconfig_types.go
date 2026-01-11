// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DNSManagementMode defines how DNS records are managed for Ingresses
// +kubebuilder:validation:Enum=Automatic;Manual;DNSRecord
type DNSManagementMode string

const (
	// DNSManagementAutomatic - Controller creates CNAME records directly via Cloudflare API
	DNSManagementAutomatic DNSManagementMode = "Automatic"

	// DNSManagementManual - User manages DNS records externally (compatible with external-dns)
	DNSManagementManual DNSManagementMode = "Manual"

	// DNSManagementDNSRecord - Controller creates DNSRecord CRDs for each hostname
	DNSManagementDNSRecord DNSManagementMode = "DNSRecord"
)

// TunnelReference references a Tunnel or ClusterTunnel resource
type TunnelReference struct {
	// Kind is the tunnel resource kind: Tunnel or ClusterTunnel
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Tunnel;ClusterTunnel
	Kind string `json:"kind"`

	// Name is the name of the Tunnel/ClusterTunnel resource
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the Tunnel (only for Kind=Tunnel).
	// If not specified for Kind=Tunnel, defaults to the TunnelIngressClassConfig's namespace.
	// +kubebuilder:validation:Optional
	Namespace string `json:"namespace,omitempty"`
}

// OriginRequestSpec defines origin request configuration for backend connections
type OriginRequestSpec struct {
	// NoTLSVerify disables TLS verification for HTTPS origins
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	NoTLSVerify bool `json:"noTlsVerify,omitempty"`

	// HTTP2Origin enables HTTP/2 to origin (origin must be HTTPS)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	HTTP2Origin bool `json:"http2Origin,omitempty"`

	// ConnectTimeout for establishing connection to origin (e.g., "30s")
	// +kubebuilder:validation:Optional
	ConnectTimeout string `json:"connectTimeout,omitempty"`

	// TLSTimeout for TLS handshake with origin (e.g., "10s")
	// +kubebuilder:validation:Optional
	TLSTimeout string `json:"tlsTimeout,omitempty"`

	// KeepAliveTimeout for idle connections to origin (e.g., "90s")
	// +kubebuilder:validation:Optional
	KeepAliveTimeout string `json:"keepAliveTimeout,omitempty"`

	// KeepAliveConnections is the maximum number of idle connections to keep open
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	KeepAliveConnections *int `json:"keepAliveConnections,omitempty"`

	// CAPool is the name of a Secret containing CA certificate (tls.crt) for backend verification
	// +kubebuilder:validation:Optional
	CAPool string `json:"caPool,omitempty"`

	// OriginServerName overrides the hostname used for TLS verification
	// +kubebuilder:validation:Optional
	OriginServerName string `json:"originServerName,omitempty"`

	// HTTPHostHeader overrides the Host header sent to origin
	// +kubebuilder:validation:Optional
	HTTPHostHeader string `json:"httpHostHeader,omitempty"`

	// ProxyAddress for bastion/SOCKS mode
	// +kubebuilder:validation:Optional
	ProxyAddress string `json:"proxyAddress,omitempty"`

	// ProxyPort for bastion/SOCKS mode
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	ProxyPort *uint16 `json:"proxyPort,omitempty"`

	// ProxyType specifies the proxy type: "" (none) or "socks"
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum="";socks
	ProxyType string `json:"proxyType,omitempty"`

	// DisableChunkedEncoding disables chunked transfer encoding for HTTP requests
	// +kubebuilder:validation:Optional
	DisableChunkedEncoding *bool `json:"disableChunkedEncoding,omitempty"`

	// BastionMode enables bastion mode for the tunnel
	// +kubebuilder:validation:Optional
	BastionMode *bool `json:"bastionMode,omitempty"`
}

// ProtocolType defines the backend protocol type
// +kubebuilder:validation:Enum=http;https;tcp;udp;ssh;rdp;smb;bastion;wss;ws
type ProtocolType string

const (
	ProtocolHTTP    ProtocolType = "http"
	ProtocolHTTPS   ProtocolType = "https"
	ProtocolTCP     ProtocolType = "tcp"
	ProtocolUDP     ProtocolType = "udp"
	ProtocolSSH     ProtocolType = "ssh"
	ProtocolRDP     ProtocolType = "rdp"
	ProtocolSMB     ProtocolType = "smb"
	ProtocolBastion ProtocolType = "bastion"
	ProtocolWSS     ProtocolType = "wss"
	ProtocolWS      ProtocolType = "ws"
)

// TunnelIngressClassConfigSpec defines the desired state of TunnelIngressClassConfig
type TunnelIngressClassConfigSpec struct {
	// TunnelRef references the Tunnel or ClusterTunnel to use for this IngressClass
	// +kubebuilder:validation:Required
	TunnelRef TunnelReference `json:"tunnelRef"`

	// DefaultProtocol specifies the default backend protocol when not specified by
	// Ingress annotation, Service annotation, or Service port appProtocol.
	// Protocol detection priority (highest to lowest):
	// 1. Ingress annotation: cloudflare.com/protocol
	// 2. Ingress annotation: cloudflare.com/protocol-{port} (port-specific)
	// 3. Service annotation: cloudflare.com/protocol
	// 4. Service port appProtocol field (Kubernetes native)
	// 5. Service port name (http, https, grpc, h2c, etc.)
	// 6. This defaultProtocol field
	// 7. Port number inference (443→https, 22→ssh, others→http)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=http
	DefaultProtocol ProtocolType `json:"defaultProtocol,omitempty"`

	// DefaultOriginRequest provides default origin request settings for all Ingresses
	// using this IngressClass. Can be overridden per-Ingress via annotations.
	// +kubebuilder:validation:Optional
	DefaultOriginRequest *OriginRequestSpec `json:"defaultOriginRequest,omitempty"`

	// DNSManagement controls how DNS records are managed for Ingress hostnames.
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

	// WatchNamespaces limits which namespaces the controller watches for Ingresses.
	// If empty, watches all namespaces.
	// +kubebuilder:validation:Optional
	WatchNamespaces []string `json:"watchNamespaces,omitempty"`
}

// TunnelIngressClassConfigStatus defines the observed state of TunnelIngressClassConfig
type TunnelIngressClassConfigStatus struct {
	// TunnelID is the resolved Cloudflare Tunnel ID
	// +kubebuilder:validation:Optional
	TunnelID string `json:"tunnelId,omitempty"`

	// TunnelName is the resolved Cloudflare Tunnel name
	// +kubebuilder:validation:Optional
	TunnelName string `json:"tunnelName,omitempty"`

	// IngressCount is the number of Ingresses using this configuration
	// +kubebuilder:validation:Optional
	IngressCount int `json:"ingressCount,omitempty"`

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
// +kubebuilder:resource:scope=Namespaced,shortName=ticc
// +kubebuilder:printcolumn:name="Tunnel",type=string,JSONPath=`.spec.tunnelRef.name`
// +kubebuilder:printcolumn:name="Kind",type=string,JSONPath=`.spec.tunnelRef.kind`
// +kubebuilder:printcolumn:name="DNS",type=string,JSONPath=`.spec.dnsManagement`
// +kubebuilder:printcolumn:name="Ingresses",type=integer,JSONPath=`.status.ingressCount`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// TunnelIngressClassConfig provides IngressClass parameters for Cloudflare Tunnel Ingress Controller.
// This resource links an IngressClass to a specific Tunnel or ClusterTunnel and configures
// how the Ingress Controller handles DNS records and origin connections.
type TunnelIngressClassConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TunnelIngressClassConfigSpec   `json:"spec,omitempty"`
	Status TunnelIngressClassConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TunnelIngressClassConfigList contains a list of TunnelIngressClassConfig
type TunnelIngressClassConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TunnelIngressClassConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TunnelIngressClassConfig{}, &TunnelIngressClassConfigList{})
}

// GetTunnelNamespace returns the namespace of the referenced tunnel.
// For ClusterTunnel, returns empty string.
// For Tunnel, returns the specified namespace or the config's namespace.
func (c *TunnelIngressClassConfig) GetTunnelNamespace() string {
	if c.Spec.TunnelRef.Kind == "ClusterTunnel" {
		return ""
	}
	if c.Spec.TunnelRef.Namespace != "" {
		return c.Spec.TunnelRef.Namespace
	}
	return c.Namespace
}

// IsDNSProxied returns whether DNS records should be proxied through Cloudflare.
func (c *TunnelIngressClassConfig) IsDNSProxied() bool {
	if c.Spec.DNSProxied == nil {
		return true // default to true
	}
	return *c.Spec.DNSProxied
}
