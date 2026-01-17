// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TunnelBindingSubject defines the subject TunnelBinding connects to the Tunnel
type TunnelBindingSubject struct {
	// Kind can be Service
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:="Service"
	Kind string `json:"kind"`
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// +kubebuilder:validation:Optional
	Spec TunnelBindingSubjectSpec `json:"spec"`
}

type TunnelBindingSubjectSpec struct {
	// Fqdn specifies the DNS name to access this service from.
	// Defaults to the service.metadata.name + tunnel.spec.domain.
	// If specifying this, make sure to use the same domain that the tunnel belongs to.
	// This is not validated and used as provided
	// +kubebuilder:validation:Optional
	Fqdn string `json:"fqdn,omitempty"`

	// Protocol specifies the protocol for the service. Should be one of http, https, tcp, udp, ssh or rdp.
	// Defaults to http, with the exceptions of https for 443, smb for 139 and 445, rdp for 3389 and ssh for 22 if the service has a TCP port.
	// The only available option for a UDP port is udp, which is default.
	// +kubebuilder:validation:Optional
	Protocol string `json:"protocol,omitempty"`

	// Path specifies a regular expression for to match on the request for http/https services
	// If a rule does not specify a path, all paths will be matched.
	// +kubebuilder:validation:Optional
	Path string `json:"path,omitempty"`

	// Target specified where the tunnel should proxy to.
	// Defaults to the form of <protocol>://<service.metadata.name>.<service.metadata.namespace>.svc:<port>
	// +kubebuilder:validation:Optional
	Target string `json:"target,omitempty"`

	// CaPool trusts the CA certificate referenced by the key in the secret specified in tunnel.spec.originCaPool.
	// tls.crt is trusted globally and does not need to be specified. Only useful if the protocol is HTTPS.
	// +kubebuilder:validation:Optional
	CaPool string `json:"caPool,omitempty"`

	// NoTlsVerify disables TLS verification for this service.
	// Only useful if the protocol is HTTPS.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=false
	NoTlsVerify bool `json:"noTlsVerify"`

	// HTTP2Origin makes the service attempt to connect to origin using HTTP2.
	// Origin must be configured as https.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=false
	HTTP2Origin bool `json:"http2Origin"`

	// cloudflared starts a proxy server to translate HTTP traffic into TCP when proxying, for example, SSH or RDP.

	// ProxyAddress configures the listen address for that proxy
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:="127.0.0.1"
	// +kubebuilder:validation:Pattern="((^((([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])\\.){3}([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5]))$)|(^(([0-9a-fA-F]{1,4}:){7,7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:))$))"
	ProxyAddress string `json:"proxyAddress,omitempty"`

	// ProxyPort configures the listen port for that proxy
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=0
	// +kubebuilder:validation:Minimum:=0
	// +kubebuilder:validation:Maximum:=65535
	ProxyPort uint `json:"proxyPort,omitempty"`

	// ProxyType configures the proxy type.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=""
	// +kubebuilder:validation:Enum:="";"socks"
	ProxyType string `json:"proxyType,omitempty"`
}

// TunnelRef defines the Tunnel TunnelBinding connects to
type TunnelRef struct {
	// Kind can be Tunnel or ClusterTunnel
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum:="ClusterTunnel";"Tunnel"
	Kind string `json:"kind"`
	// Name of the tunnel resource
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// +kubebuilder:validation:Optional
	// DisableDNSUpdates disables the DNS updates on Cloudflare, just managing the configs. Assumes the DNS entries are manually added.
	DisableDNSUpdates bool `json:"disableDNSUpdates"`
}

// ServiceInfo stores the Hostname and Target for each service
type ServiceInfo struct {
	// FQDN of the service
	Hostname string `json:"hostname"`
	// Target for cloudflared
	Target string `json:"target"`
}

// TunnelBindingStatus defines the observed state of TunnelBinding
type TunnelBindingStatus struct {
	// To show on the kubectl cli
	Hostnames string        `json:"hostnames"`
	Services  []ServiceInfo `json:"services"`

	// SyncedHostnames contains the hostnames last synced to Cloudflare Tunnel configuration.
	// Used for read-merge-write to track owned hostnames and avoid overwriting
	// rules from other controllers (Tunnel, Ingress, Gateway).
	// +kubebuilder:validation:Optional
	SyncedHostnames []string `json:"syncedHostnames,omitempty"`

	// ConfigVersion is the tunnel configuration version after last sync
	// +kubebuilder:validation:Optional
	ConfigVersion int `json:"configVersion,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="FQDNs",type=string,JSONPath=`.status.hostnames`
// +kubebuilder:deprecatedversion:warning="TunnelBinding is deprecated. Please migrate to Ingress with TunnelIngressClassConfig or Gateway API."

// TunnelBinding is the Schema for the tunnelbindings API
//
// Deprecated: TunnelBinding is deprecated and will be removed in a future release.
// Please migrate to one of the following alternatives:
// - Ingress with TunnelIngressClassConfig for HTTP/HTTPS services
// - Gateway API (HTTPRoute, TCPRoute, UDPRoute) with TunnelGatewayClassConfig for advanced routing
type TunnelBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Subjects  []TunnelBindingSubject `json:"subjects"`
	TunnelRef TunnelRef              `json:"tunnelRef"`
	Status    TunnelBindingStatus    `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TunnelBindingList contains a list of TunnelBinding
type TunnelBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TunnelBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TunnelBinding{}, &TunnelBindingList{})
}
