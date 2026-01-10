// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package route

import (
	"testing"

	"github.com/stretchr/testify/assert"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestProtocolConstants(t *testing.T) {
	assert.Equal(t, "http", ProtocolHTTP)
	assert.Equal(t, "https", ProtocolHTTPS)
	assert.Equal(t, "tcp", ProtocolTCP)
	assert.Equal(t, "udp", ProtocolUDP)
	assert.Equal(t, "ssh", ProtocolSSH)
	assert.Equal(t, "rdp", ProtocolRDP)
	assert.Equal(t, "smb", ProtocolSMB)
}

func TestInferProtocolFromPort(t *testing.T) {
	tests := []struct {
		name string
		port string
		want string
	}{
		{
			name: "port 443 - https",
			port: "443",
			want: ProtocolHTTPS,
		},
		{
			name: "port 22 - ssh",
			port: "22",
			want: ProtocolSSH,
		},
		{
			name: "port 3389 - rdp",
			port: "3389",
			want: ProtocolRDP,
		},
		{
			name: "port 139 - smb",
			port: "139",
			want: ProtocolSMB,
		},
		{
			name: "port 445 - smb",
			port: "445",
			want: ProtocolSMB,
		},
		{
			name: "port 80 - http (default)",
			port: "80",
			want: ProtocolHTTP,
		},
		{
			name: "port 8080 - http (default)",
			port: "8080",
			want: ProtocolHTTP,
		},
		{
			name: "unknown port - http (default)",
			port: "9999",
			want: ProtocolHTTP,
		},
		{
			name: "empty port - http (default)",
			port: "",
			want: ProtocolHTTP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferProtocolFromPort(tt.port)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestInferProtocolFromPortNumber(t *testing.T) {
	tests := []struct {
		name string
		port int32
		want string
	}{
		{
			name: "port 443 - https",
			port: 443,
			want: ProtocolHTTPS,
		},
		{
			name: "port 22 - ssh",
			port: 22,
			want: ProtocolSSH,
		},
		{
			name: "port 3389 - rdp",
			port: 3389,
			want: ProtocolRDP,
		},
		{
			name: "port 139 - smb",
			port: 139,
			want: ProtocolSMB,
		},
		{
			name: "port 445 - smb",
			port: 445,
			want: ProtocolSMB,
		},
		{
			name: "port 80 - http",
			port: 80,
			want: ProtocolHTTP,
		},
		{
			name: "unknown port - http",
			port: 9999,
			want: ProtocolHTTP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferProtocolFromPortNumber(tt.port)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildServiceURL(t *testing.T) {
	tests := []struct {
		name        string
		protocol    string
		serviceName string
		namespace   string
		port        string
		want        string
	}{
		{
			name:        "http service",
			protocol:    ProtocolHTTP,
			serviceName: "my-service",
			namespace:   "default",
			port:        "80",
			want:        "http://my-service.default.svc:80",
		},
		{
			name:        "https service",
			protocol:    ProtocolHTTPS,
			serviceName: "secure-service",
			namespace:   "production",
			port:        "443",
			want:        "https://secure-service.production.svc:443",
		},
		{
			name:        "tcp service",
			protocol:    ProtocolTCP,
			serviceName: "tcp-service",
			namespace:   "network",
			port:        "9000",
			want:        "tcp://tcp-service.network.svc:9000",
		},
		{
			name:        "ssh service",
			protocol:    ProtocolSSH,
			serviceName: "bastion",
			namespace:   "infra",
			port:        "22",
			want:        "ssh://bastion.infra.svc:22",
		},
		{
			name:        "rdp service",
			protocol:    ProtocolRDP,
			serviceName: "windows-host",
			namespace:   "windows",
			port:        "3389",
			want:        "rdp://windows-host.windows.svc:3389",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildServiceURL(tt.protocol, tt.serviceName, tt.namespace, tt.port)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildServiceURLFromPort(t *testing.T) {
	tests := []struct {
		name        string
		protocol    string
		serviceName string
		namespace   string
		port        int32
		want        string
	}{
		{
			name:        "http with int port",
			protocol:    ProtocolHTTP,
			serviceName: "api",
			namespace:   "default",
			port:        8080,
			want:        "http://api.default.svc:8080",
		},
		{
			name:        "https with int port",
			protocol:    ProtocolHTTPS,
			serviceName: "secure",
			namespace:   "prod",
			port:        443,
			want:        "https://secure.prod.svc:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildServiceURLFromPort(tt.protocol, tt.serviceName, tt.namespace, tt.port)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProtocolFromGatewayProtocol(t *testing.T) {
	tests := []struct {
		name     string
		protocol gatewayv1.ProtocolType
		want     string
	}{
		{
			name:     "HTTP protocol",
			protocol: gatewayv1.HTTPProtocolType,
			want:     ProtocolHTTP,
		},
		{
			name:     "HTTPS protocol",
			protocol: gatewayv1.HTTPSProtocolType,
			want:     ProtocolHTTPS,
		},
		{
			name:     "TLS protocol - maps to https",
			protocol: gatewayv1.TLSProtocolType,
			want:     ProtocolHTTPS,
		},
		{
			name:     "TCP protocol",
			protocol: gatewayv1.TCPProtocolType,
			want:     ProtocolTCP,
		},
		{
			name:     "UDP protocol",
			protocol: gatewayv1.UDPProtocolType,
			want:     ProtocolUDP,
		},
		{
			name:     "unknown protocol - defaults to http",
			protocol: gatewayv1.ProtocolType("UNKNOWN"),
			want:     ProtocolHTTP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProtocolFromGatewayProtocol(tt.protocol)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestInferProtocolConsistency verifies that string and int port versions are consistent.
func TestInferProtocolConsistency(t *testing.T) {
	ports := []struct {
		strPort string
		intPort int32
	}{
		{"22", 22},
		{"80", 80},
		{"443", 443},
		{"3389", 3389},
		{"139", 139},
		{"445", 445},
		{"8080", 8080},
	}

	for _, p := range ports {
		t.Run(p.strPort, func(t *testing.T) {
			fromStr := InferProtocolFromPort(p.strPort)
			fromInt := InferProtocolFromPortNumber(p.intPort)
			assert.Equal(t, fromStr, fromInt,
				"String and int port should produce same protocol")
		})
	}
}
