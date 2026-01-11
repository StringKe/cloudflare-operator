// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package ingress implements the Kubernetes Ingress Controller for Cloudflare Tunnels.
// It watches Ingress resources with the cloudflare-tunnel IngressClass and configures
// the tunnel to route traffic to the appropriate backends.
package ingress

import (
	"strconv"
	"time"
)

// Annotation prefix for Cloudflare-specific annotations
const AnnotationPrefix = "cloudflare.com/"

// Protocol annotations
const (
	// AnnotationProtocol specifies the backend protocol: http, https, tcp, udp, ssh, rdp, smb, wss, ws
	// Can be used on both Ingress and Service resources.
	// Priority: Ingress annotation > Service annotation > appProtocol > port name > default
	AnnotationProtocol = AnnotationPrefix + "protocol"

	// AnnotationProtocolPrefix is the prefix for port-specific protocol annotations.
	// Usage: cloudflare.com/protocol-{port} = http|https|...
	// Example: cloudflare.com/protocol-9091 = http
	AnnotationProtocolPrefix = AnnotationPrefix + "protocol-"

	// AnnotationNoTLSVerify disables TLS verification for HTTPS origins ("true" or "false")
	AnnotationNoTLSVerify = AnnotationPrefix + "no-tls-verify"

	// AnnotationHTTP2Origin enables HTTP/2 to origin ("true" or "false")
	AnnotationHTTP2Origin = AnnotationPrefix + "http2-origin"

	// AnnotationCAPool specifies the Secret name containing CA certificate for backend verification
	AnnotationCAPool = AnnotationPrefix + "ca-pool"
)

// Proxy annotations (for bastion/SOCKS mode)
const (
	// AnnotationProxyAddress specifies the proxy address for bastion mode
	AnnotationProxyAddress = AnnotationPrefix + "proxy-address"

	// AnnotationProxyPort specifies the proxy port for bastion mode
	AnnotationProxyPort = AnnotationPrefix + "proxy-port"

	// AnnotationProxyType specifies the proxy type: "" (none) or "socks"
	AnnotationProxyType = AnnotationPrefix + "proxy-type"
)

// Connection settings
const (
	// AnnotationConnectTimeout specifies connection timeout (e.g., "30s")
	AnnotationConnectTimeout = AnnotationPrefix + "connect-timeout"

	// AnnotationTLSTimeout specifies TLS handshake timeout (e.g., "10s")
	AnnotationTLSTimeout = AnnotationPrefix + "tls-timeout"

	// AnnotationKeepAliveTimeout specifies keep-alive timeout (e.g., "90s")
	AnnotationKeepAliveTimeout = AnnotationPrefix + "keep-alive-timeout"

	// AnnotationKeepAliveConnections specifies max idle connections
	AnnotationKeepAliveConnections = AnnotationPrefix + "keep-alive-connections"
)

// Origin header settings
const (
	// AnnotationOriginServerName overrides the hostname used for TLS verification
	AnnotationOriginServerName = AnnotationPrefix + "origin-server-name"

	// AnnotationHTTPHostHeader overrides the Host header sent to origin
	AnnotationHTTPHostHeader = AnnotationPrefix + "http-host-header"
)

// DNS annotations
const (
	// AnnotationDisableDNS disables DNS record creation for this Ingress ("true" to disable)
	AnnotationDisableDNS = AnnotationPrefix + "disable-dns"

	// AnnotationDNSProxied controls whether DNS is proxied through Cloudflare ("true" or "false")
	AnnotationDNSProxied = AnnotationPrefix + "dns-proxied"
)

// Advanced settings
const (
	// AnnotationDisableChunkedEncoding disables chunked transfer encoding
	AnnotationDisableChunkedEncoding = AnnotationPrefix + "disable-chunked-encoding"

	// AnnotationBastionMode enables bastion mode
	AnnotationBastionMode = AnnotationPrefix + "bastion-mode"
)

// AnnotationParser helps parse annotation values
type AnnotationParser struct {
	annotations map[string]string
}

// NewAnnotationParser creates a new annotation parser
func NewAnnotationParser(annotations map[string]string) *AnnotationParser {
	if annotations == nil {
		annotations = make(map[string]string)
	}
	return &AnnotationParser{annotations: annotations}
}

// GetString returns the string value of an annotation
func (p *AnnotationParser) GetString(key string) (string, bool) {
	v, ok := p.annotations[key]
	return v, ok
}

// GetBool returns the boolean value of an annotation and whether it was found
// nolint:revive // (value, ok) pattern is standard Go idiom
func (p *AnnotationParser) GetBool(key string) (bool, bool) {
	v, ok := p.annotations[key]
	if !ok {
		return false, false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, false
	}
	return b, true
}

// GetBoolPtr returns a pointer to the boolean value of an annotation
func (p *AnnotationParser) GetBoolPtr(key string) *bool {
	if b, ok := p.GetBool(key); ok {
		return &b
	}
	return nil
}

// GetInt returns the integer value of an annotation
func (p *AnnotationParser) GetInt(key string) (int, bool) {
	v, ok := p.annotations[key]
	if !ok {
		return 0, false
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return i, true
}

// GetUint16 returns the uint16 value of an annotation
func (p *AnnotationParser) GetUint16(key string) (uint16, bool) {
	v, ok := p.annotations[key]
	if !ok {
		return 0, false
	}
	i, err := strconv.ParseUint(v, 10, 16)
	if err != nil {
		return 0, false
	}
	return uint16(i), true
}

// GetDuration returns the duration value of an annotation
func (p *AnnotationParser) GetDuration(key string) (time.Duration, bool) {
	v, ok := p.annotations[key]
	if !ok {
		return 0, false
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, false
	}
	return d, true
}
