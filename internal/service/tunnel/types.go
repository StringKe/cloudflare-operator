// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package tunnel provides types and service for Tunnel configuration management.
//
//nolint:revive // max-public-structs is acceptable for this type-heavy package
package tunnel

import (
	"time"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// IngressRule represents a single tunnel ingress rule.
// This is the configuration contributed by Ingress, TunnelBinding, or Gateway controllers.
type IngressRule struct {
	// Hostname is the public hostname for this rule (e.g., "app.example.com")
	Hostname string `json:"hostname,omitempty"`
	// Path is the URL path to match (e.g., "/api/*")
	Path string `json:"path,omitempty"`
	// Service is the backend service URL (e.g., "http://svc.ns.svc:80")
	Service string `json:"service"`
	// OriginRequest contains optional origin request configuration
	OriginRequest *OriginRequestConfig `json:"originRequest,omitempty"`
}

// OriginRequestConfig contains origin request settings.
// These settings control how cloudflared connects to the backend service.
type OriginRequestConfig struct {
	// ConnectTimeout is the timeout for establishing a connection to origin
	ConnectTimeout *time.Duration `json:"connectTimeout,omitempty"`
	// TLSTimeout is the timeout for TLS handshake with origin
	TLSTimeout *time.Duration `json:"tlsTimeout,omitempty"`
	// TCPKeepAlive is the TCP keepalive interval
	TCPKeepAlive *time.Duration `json:"tcpKeepAlive,omitempty"`
	// NoHappyEyeballs disables Happy Eyeballs for IPv4/v6 fallback
	NoHappyEyeballs *bool `json:"noHappyEyeballs,omitempty"`
	// KeepAliveConnections is the max number of idle connections to keep open
	KeepAliveConnections *int `json:"keepAliveConnections,omitempty"`
	// KeepAliveTimeout is the timeout for idle connections
	KeepAliveTimeout *time.Duration `json:"keepAliveTimeout,omitempty"`
	// HTTPHostHeader overrides the Host header sent to origin
	HTTPHostHeader *string `json:"httpHostHeader,omitempty"`
	// OriginServerName overrides the hostname for TLS verification
	OriginServerName *string `json:"originServerName,omitempty"`
	// CAPool is the path to CA certificates for origin verification
	CAPool *string `json:"caPool,omitempty"`
	// NoTLSVerify disables TLS certificate verification for origin
	NoTLSVerify *bool `json:"noTlsVerify,omitempty"`
	// HTTP2Origin enables HTTP/2 to origin (requires HTTPS)
	HTTP2Origin *bool `json:"http2Origin,omitempty"`
	// DisableChunkedEncoding disables chunked transfer encoding
	DisableChunkedEncoding *bool `json:"disableChunkedEncoding,omitempty"`
	// BastionMode enables bastion/jump host mode
	BastionMode *bool `json:"bastionMode,omitempty"`
	// ProxyAddress is the address for SOCKS proxy
	ProxyAddress *string `json:"proxyAddress,omitempty"`
	// ProxyPort is the port for SOCKS proxy
	ProxyPort *uint `json:"proxyPort,omitempty"`
	// ProxyType is the proxy type (e.g., "socks")
	ProxyType *string `json:"proxyType,omitempty"`
}

// TunnelSettings contains tunnel-level settings.
// These are provided by Tunnel/ClusterTunnel controllers and have highest priority.
type TunnelSettings struct {
	// WarpRouting controls whether WARP routing is enabled
	WarpRouting *WarpRoutingConfig `json:"warpRouting,omitempty"`
	// FallbackTarget is the service URL for unmatched requests (e.g., "http_status:404")
	FallbackTarget string `json:"fallbackTarget,omitempty"`
	// GlobalOriginRequest contains global origin request settings
	GlobalOriginRequest *OriginRequestConfig `json:"globalOriginRequest,omitempty"`
}

// WarpRoutingConfig controls WARP routing settings.
type WarpRoutingConfig struct {
	// Enabled controls whether WARP routing is enabled
	Enabled bool `json:"enabled"`
}

// TunnelConfig represents the complete configuration from a single source.
// Each K8s resource (Tunnel, Ingress, TunnelBinding, Gateway) contributes
// a TunnelConfig to the SyncState.
type TunnelConfig struct {
	// Settings contains tunnel-level settings (only from Tunnel/ClusterTunnel)
	Settings *TunnelSettings `json:"settings,omitempty"`
	// Rules contains ingress rules (from Ingress, TunnelBinding, Gateway)
	Rules []IngressRule `json:"rules,omitempty"`
}

// RegisterSettingsOptions contains options for registering tunnel settings.
type RegisterSettingsOptions struct {
	// TunnelID is the Cloudflare tunnel ID
	TunnelID string
	// AccountID is the Cloudflare account ID
	AccountID string
	// Source identifies the K8s resource contributing this configuration
	Source service.Source
	// Settings contains the tunnel settings
	Settings TunnelSettings
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
}

// RegisterRulesOptions contains options for registering ingress rules.
type RegisterRulesOptions struct {
	// TunnelID is the Cloudflare tunnel ID
	TunnelID string
	// AccountID is the Cloudflare account ID
	AccountID string
	// Source identifies the K8s resource contributing this configuration
	Source service.Source
	// Rules contains the ingress rules
	Rules []IngressRule
	// Priority determines conflict resolution (lower = higher priority)
	Priority int
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
}
