// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/credentials"
)

func TestTunnelProtocolConstants(t *testing.T) {
	tests := []struct {
		protocol string
		expected bool
	}{
		{protocol: "http", expected: true},
		{protocol: "https", expected: true},
		{protocol: "rdp", expected: true},
		{protocol: "smb", expected: true},
		{protocol: "ssh", expected: true},
		{protocol: "tcp", expected: true},
		{protocol: "udp", expected: true},
		{protocol: "invalid", expected: false},
		{protocol: "HTTP", expected: false},  // Case sensitive
		{protocol: "HTTPS", expected: false}, // Case sensitive
		{protocol: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.protocol, func(t *testing.T) {
			result := tunnelValidProtoMap[tt.protocol]
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTunnelLabelConstants(t *testing.T) {
	// Verify all label constants are defined correctly
	assert.Equal(t, "cloudflare-operator.io/tunnel", tunnelLabel)
	assert.Equal(t, "cloudflare-operator.io/is-cluster-tunnel", isClusterTunnelLabel)
	assert.Equal(t, "cloudflare-operator.io/id", tunnelIdLabel)
	assert.Equal(t, "cloudflare-operator.io/name", tunnelNameLabel)
	assert.Equal(t, "cloudflare-operator.io/kind", tunnelKindLabel)
	assert.Equal(t, "cloudflare-operator.io/app", tunnelAppLabel)
	assert.Equal(t, "cloudflare-operator.io/domain", tunnelDomainLabel)
	assert.Equal(t, "cloudflare-operator.io/finalizer", tunnelFinalizer)
}

func TestTunnelAnnotationConstants(t *testing.T) {
	assert.Equal(t, "cloudflare-operator.io/previous-hostnames", tunnelPreviousHostnamesAnnotation)
}

func TestSecretFinalizerPrefix(t *testing.T) {
	assert.Equal(t, "cloudflare-operator.io/secret-finalizer-", secretFinalizerPrefix)

	// Test creating a finalizer name for a specific tunnel
	tunnelName := "my-tunnel"
	finalizerName := secretFinalizerPrefix + tunnelName
	assert.Equal(t, "cloudflare-operator.io/secret-finalizer-my-tunnel", finalizerName)
}

func TestCreateCloudflareClientFromCreds(t *testing.T) {
	tests := []struct {
		name        string
		creds       *credentials.Credentials
		expectError bool
		errorMsg    string
	}{
		{
			name: "API Token auth",
			creds: &credentials.Credentials{
				AuthType: networkingv1alpha2.AuthTypeAPIToken,
				APIToken: "test-token",
			},
			expectError: false,
		},
		{
			name: "Global API Key auth",
			creds: &credentials.Credentials{
				AuthType: networkingv1alpha2.AuthTypeGlobalAPIKey,
				APIKey:   "test-key",
				Email:    "test@example.com",
			},
			expectError: false,
		},
		{
			name: "Fallback to API Token",
			creds: &credentials.Credentials{
				AuthType: "", // Empty, should fallback
				APIToken: "test-token",
			},
			expectError: false,
		},
		{
			name: "Fallback to Global API Key",
			creds: &credentials.Credentials{
				AuthType: "", // Empty, should fallback
				APIKey:   "test-key",
				Email:    "test@example.com",
			},
			expectError: false,
		},
		{
			name: "No credentials - should fail",
			creds: &credentials.Credentials{
				AuthType: "",
			},
			expectError: true,
			errorMsg:    "no valid API credentials found",
		},
		{
			name: "API Key without Email - should fail in fallback",
			creds: &credentials.Credentials{
				AuthType: "",
				APIKey:   "test-key",
				// Missing Email
			},
			expectError: true,
			errorMsg:    "no valid API credentials found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := CreateCloudflareClientFromCreds(tt.creds)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}

func TestTunnelValidProtoMapCompleteness(t *testing.T) {
	// Ensure all expected protocols are in the map
	expectedProtocols := []string{
		tunnelProtoHTTP,
		tunnelProtoHTTPS,
		tunnelProtoRDP,
		tunnelProtoSMB,
		tunnelProtoSSH,
		tunnelProtoTCP,
		tunnelProtoUDP,
	}

	for _, proto := range expectedProtocols {
		t.Run(proto, func(t *testing.T) {
			assert.True(t, tunnelValidProtoMap[proto], "Protocol %s should be valid", proto)
		})
	}

	// Verify the total count
	assert.Len(t, tunnelValidProtoMap, len(expectedProtocols))
}

func TestProtocolConstantValues(t *testing.T) {
	// Verify the actual string values match expected values
	assert.Equal(t, "http", tunnelProtoHTTP)
	assert.Equal(t, "https", tunnelProtoHTTPS)
	assert.Equal(t, "rdp", tunnelProtoRDP)
	assert.Equal(t, "smb", tunnelProtoSMB)
	assert.Equal(t, "ssh", tunnelProtoSSH)
	assert.Equal(t, "tcp", tunnelProtoTCP)
	assert.Equal(t, "udp", tunnelProtoUDP)
}

func TestLabelFormat(t *testing.T) {
	// All labels should follow Kubernetes naming conventions
	labels := []string{
		tunnelLabel,
		isClusterTunnelLabel,
		tunnelIdLabel,
		tunnelNameLabel,
		tunnelKindLabel,
		tunnelAppLabel,
		tunnelDomainLabel,
	}

	for _, label := range labels {
		t.Run(label, func(t *testing.T) {
			// Should contain the domain prefix
			assert.Contains(t, label, "cloudflare-operator.io/")
			// Should not be empty after prefix
			assert.True(t, len(label) > len("cloudflare-operator.io/"))
		})
	}
}
