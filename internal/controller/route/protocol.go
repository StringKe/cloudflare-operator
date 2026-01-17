// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package route

import (
	"fmt"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// Protocol constants
const (
	ProtocolHTTP  = "http"
	ProtocolHTTPS = "https"
	ProtocolTCP   = "tcp"
	ProtocolUDP   = "udp"
	ProtocolSSH   = "ssh"
	ProtocolRDP   = "rdp"
	ProtocolSMB   = "smb"
)

// InferProtocolFromPort determines the protocol based on port number.
// This is used when no explicit protocol annotation is provided.
func InferProtocolFromPort(port string) string {
	switch port {
	case "443":
		return ProtocolHTTPS
	case "22":
		return ProtocolSSH
	case "3389":
		return ProtocolRDP
	case "139", "445":
		return ProtocolSMB
	default:
		return ProtocolHTTP
	}
}

// InferProtocolFromPortNumber determines the protocol based on numeric port.
func InferProtocolFromPortNumber(port int32) string {
	return InferProtocolFromPort(fmt.Sprintf("%d", port))
}

// BuildServiceURL constructs a service URL for cloudflared ingress rules.
// Parameters:
//   - protocol: the protocol (http, https, tcp, udp, ssh, rdp, smb)
//   - serviceName: the Kubernetes service name
//   - namespace: the Kubernetes namespace
//   - port: the port number as string
func BuildServiceURL(protocol, serviceName, namespace, port string) string {
	return fmt.Sprintf("%s://%s.%s.svc:%s", protocol, serviceName, namespace, port)
}

// BuildServiceURLFromPort constructs a service URL using a numeric port.
func BuildServiceURLFromPort(protocol, serviceName, namespace string, port int32) string {
	return BuildServiceURL(protocol, serviceName, namespace, fmt.Sprintf("%d", port))
}

// ProtocolFromGatewayProtocol converts Gateway API ProtocolType to cloudflared protocol.
func ProtocolFromGatewayProtocol(protocol gatewayv1.ProtocolType) string {
	switch protocol {
	case gatewayv1.HTTPSProtocolType, gatewayv1.TLSProtocolType:
		// Both HTTPS and TLS map to https protocol in Cloudflare
		return ProtocolHTTPS
	case gatewayv1.TCPProtocolType:
		return ProtocolTCP
	case gatewayv1.UDPProtocolType:
		return ProtocolUDP
	default: // includes HTTPProtocolType
		return ProtocolHTTP
	}
}
