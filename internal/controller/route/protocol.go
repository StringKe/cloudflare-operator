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
	case gatewayv1.HTTPProtocolType:
		return ProtocolHTTP
	case gatewayv1.HTTPSProtocolType:
		return ProtocolHTTPS
	case gatewayv1.TCPProtocolType:
		return ProtocolTCP
	case gatewayv1.UDPProtocolType:
		return ProtocolUDP
	case gatewayv1.TLSProtocolType:
		return ProtocolHTTPS
	default:
		return ProtocolHTTP
	}
}
