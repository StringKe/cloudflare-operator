// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package networkroute

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

func TestNetworkRouteConfig(t *testing.T) {
	config := NetworkRouteConfig{
		Network:          "10.0.0.0/8",
		TunnelID:         "tunnel-123",
		TunnelName:       "production-tunnel",
		VirtualNetworkID: "vnet-456",
		Comment:          "Route for internal services",
	}

	assert.Equal(t, "10.0.0.0/8", config.Network)
	assert.Equal(t, "tunnel-123", config.TunnelID)
	assert.Equal(t, "production-tunnel", config.TunnelName)
	assert.Equal(t, "vnet-456", config.VirtualNetworkID)
	assert.Equal(t, "Route for internal services", config.Comment)
}

func TestNetworkRouteConfigMinimal(t *testing.T) {
	config := NetworkRouteConfig{
		Network:  "192.168.0.0/16",
		TunnelID: "tunnel-789",
	}

	assert.Equal(t, "192.168.0.0/16", config.Network)
	assert.Equal(t, "tunnel-789", config.TunnelID)
	assert.Empty(t, config.TunnelName)
	assert.Empty(t, config.VirtualNetworkID)
	assert.Empty(t, config.Comment)
}

func TestRegisterOptions(t *testing.T) {
	opts := RegisterOptions{
		AccountID:        "account-123",
		RouteNetwork:     "10.0.0.0/8",
		VirtualNetworkID: "vnet-456",
		Source: service.Source{
			Kind:      "NetworkRoute",
			Namespace: "",
			Name:      "internal-route",
		},
		Config: NetworkRouteConfig{
			Network:          "10.0.0.0/8",
			TunnelID:         "tunnel-123",
			TunnelName:       "main-tunnel",
			VirtualNetworkID: "vnet-456",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "10.0.0.0/8", opts.RouteNetwork)
	assert.Equal(t, "vnet-456", opts.VirtualNetworkID)
	assert.Equal(t, "NetworkRoute", opts.Source.Kind)
	assert.Equal(t, "internal-route", opts.Source.Name)
}

func TestRegisterOptionsNewRoute(t *testing.T) {
	opts := RegisterOptions{
		AccountID:    "account-123",
		RouteNetwork: "", // Will be set after creation
		Source: service.Source{
			Kind: "NetworkRoute",
			Name: "new-route",
		},
		Config: NetworkRouteConfig{
			Network:  "172.16.0.0/12",
			TunnelID: "tunnel-new",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	assert.Empty(t, opts.RouteNetwork)
	assert.Equal(t, "172.16.0.0/12", opts.Config.Network)
}

func TestSyncResult(t *testing.T) {
	result := SyncResult{
		Network:          "10.0.0.0/8",
		TunnelID:         "tunnel-123",
		TunnelName:       "main-tunnel",
		VirtualNetworkID: "vnet-456",
	}

	assert.Equal(t, "10.0.0.0/8", result.Network)
	assert.Equal(t, "tunnel-123", result.TunnelID)
	assert.Equal(t, "main-tunnel", result.TunnelName)
	assert.Equal(t, "vnet-456", result.VirtualNetworkID)
}

func TestNetworkRouteConfigCIDRFormats(t *testing.T) {
	tests := []struct {
		name    string
		network string
	}{
		{
			name:    "class A private",
			network: "10.0.0.0/8",
		},
		{
			name:    "class B private",
			network: "172.16.0.0/12",
		},
		{
			name:    "class C private",
			network: "192.168.0.0/16",
		},
		{
			name:    "single host",
			network: "10.1.2.3/32",
		},
		{
			name:    "small subnet",
			network: "10.10.10.0/24",
		},
		{
			name:    "IPv6 network",
			network: "fd00::/8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NetworkRouteConfig{
				Network:  tt.network,
				TunnelID: "tunnel-test",
			}
			assert.Equal(t, tt.network, config.Network)
		})
	}
}

func TestSyncResultEmpty(t *testing.T) {
	result := SyncResult{}

	assert.Empty(t, result.Network)
	assert.Empty(t, result.TunnelID)
	assert.Empty(t, result.TunnelName)
	assert.Empty(t, result.VirtualNetworkID)
}
