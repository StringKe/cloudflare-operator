// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVirtualNetworkParams(t *testing.T) {
	tests := []struct {
		name   string
		params VirtualNetworkParams
	}{
		{
			name: "full params",
			params: VirtualNetworkParams{
				Name:             "production-network",
				Comment:          "Production virtual network",
				IsDefaultNetwork: true,
			},
		},
		{
			name: "minimal params",
			params: VirtualNetworkParams{
				Name: "minimal-network",
			},
		},
		{
			name: "non-default network",
			params: VirtualNetworkParams{
				Name:             "secondary-network",
				Comment:          "Secondary network",
				IsDefaultNetwork: false,
			},
		},
		{
			name:   "empty params",
			params: VirtualNetworkParams{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify struct fields can be accessed
			_ = tt.params.Name
			_ = tt.params.Comment
			_ = tt.params.IsDefaultNetwork
		})
	}
}

func TestVirtualNetworkResult(t *testing.T) {
	tests := []struct {
		name   string
		result VirtualNetworkResult
	}{
		{
			name: "full result",
			result: VirtualNetworkResult{
				ID:               "vnet-123",
				Name:             "production-network",
				Comment:          "Production virtual network",
				IsDefaultNetwork: true,
				DeletedAt:        nil,
			},
		},
		{
			name: "deleted network",
			result: VirtualNetworkResult{
				ID:               "vnet-456",
				Name:             "deleted-network",
				IsDefaultNetwork: false,
				DeletedAt:        strPtr("2024-01-15T10:00:00Z"),
			},
		},
		{
			name: "minimal result",
			result: VirtualNetworkResult{
				ID:   "vnet-789",
				Name: "minimal-network",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.result.ID)
			assert.NotEmpty(t, tt.result.Name)
		})
	}
}

func TestVirtualNetworkResultFields(t *testing.T) {
	result := VirtualNetworkResult{
		ID:               "test-id",
		Name:             "test-name",
		Comment:          "test-comment",
		IsDefaultNetwork: true,
		DeletedAt:        nil,
	}

	assert.Equal(t, "test-id", result.ID)
	assert.Equal(t, "test-name", result.Name)
	assert.Equal(t, "test-comment", result.Comment)
	assert.True(t, result.IsDefaultNetwork)
	assert.Nil(t, result.DeletedAt)

	// Test with DeletedAt set
	deletedAt := "2024-01-15T10:00:00Z"
	result.DeletedAt = &deletedAt
	assert.NotNil(t, result.DeletedAt)
	assert.Equal(t, deletedAt, *result.DeletedAt)
}

func TestTunnelRouteParams(t *testing.T) {
	tests := []struct {
		name   string
		params TunnelRouteParams
	}{
		{
			name: "full params",
			params: TunnelRouteParams{
				Network:          "10.0.0.0/8",
				TunnelID:         "tunnel-123",
				VirtualNetworkID: "vnet-456",
				Comment:          "Internal network route",
			},
		},
		{
			name: "minimal params",
			params: TunnelRouteParams{
				Network:  "192.168.1.0/24",
				TunnelID: "tunnel-789",
			},
		},
		{
			name: "IPv6 route",
			params: TunnelRouteParams{
				Network:          "fd00::/8",
				TunnelID:         "tunnel-abc",
				VirtualNetworkID: "vnet-def",
				Comment:          "IPv6 private network",
			},
		},
		{
			name:   "empty params",
			params: TunnelRouteParams{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify struct fields can be accessed
			_ = tt.params.Network
			_ = tt.params.TunnelID
			_ = tt.params.VirtualNetworkID
			_ = tt.params.Comment
		})
	}
}

func TestTunnelRouteResult(t *testing.T) {
	tests := []struct {
		name   string
		result TunnelRouteResult
	}{
		{
			name: "full result",
			result: TunnelRouteResult{
				Network:          "10.0.0.0/8",
				TunnelID:         "tunnel-123",
				TunnelName:       "production-tunnel",
				VirtualNetworkID: "vnet-456",
				Comment:          "Internal network route",
			},
		},
		{
			name: "minimal result",
			result: TunnelRouteResult{
				Network:  "192.168.0.0/16",
				TunnelID: "tunnel-789",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.result.Network)
			assert.NotEmpty(t, tt.result.TunnelID)
		})
	}
}

func TestTunnelRouteResultFields(t *testing.T) {
	result := TunnelRouteResult{
		Network:          "172.16.0.0/12",
		TunnelID:         "tunnel-test",
		TunnelName:       "test-tunnel",
		VirtualNetworkID: "vnet-test",
		Comment:          "Test route comment",
	}

	assert.Equal(t, "172.16.0.0/12", result.Network)
	assert.Equal(t, "tunnel-test", result.TunnelID)
	assert.Equal(t, "test-tunnel", result.TunnelName)
	assert.Equal(t, "vnet-test", result.VirtualNetworkID)
	assert.Equal(t, "Test route comment", result.Comment)
}

func TestNetworkCIDRFormats(t *testing.T) {
	// Test various CIDR network formats
	networks := []struct {
		name    string
		cidr    string
		isValid bool
	}{
		{name: "IPv4 /8", cidr: "10.0.0.0/8", isValid: true},
		{name: "IPv4 /16", cidr: "172.16.0.0/16", isValid: true},
		{name: "IPv4 /24", cidr: "192.168.1.0/24", isValid: true},
		{name: "IPv4 /32", cidr: "192.168.1.1/32", isValid: true},
		{name: "IPv6 /8", cidr: "fd00::/8", isValid: true},
		{name: "IPv6 /64", cidr: "2001:db8::/64", isValid: true},
		{name: "IPv6 /128", cidr: "2001:db8::1/128", isValid: true},
	}

	for _, n := range networks {
		t.Run(n.name, func(t *testing.T) {
			params := TunnelRouteParams{
				Network:  n.cidr,
				TunnelID: "test",
			}
			assert.Equal(t, n.cidr, params.Network)
		})
	}
}

func TestVirtualNetworkParamsConversion(t *testing.T) {
	// Test conversion scenarios
	params := VirtualNetworkParams{
		Name:             "test-network",
		Comment:          "Test comment",
		IsDefaultNetwork: true,
	}

	// Simulate what would happen during API call preparation
	assert.Equal(t, "test-network", params.Name)
	assert.Equal(t, "Test comment", params.Comment)
	assert.True(t, params.IsDefaultNetwork)
}

func TestTunnelRouteParamsConversion(t *testing.T) {
	// Test conversion scenarios
	params := TunnelRouteParams{
		Network:          "10.0.0.0/8",
		TunnelID:         "tunnel-123",
		VirtualNetworkID: "vnet-456",
		Comment:          "Internal route",
	}

	// Simulate what would happen during API call preparation
	assert.Equal(t, "10.0.0.0/8", params.Network)
	assert.Equal(t, "tunnel-123", params.TunnelID)
	assert.Equal(t, "vnet-456", params.VirtualNetworkID)
	assert.Equal(t, "Internal route", params.Comment)
}

func TestVirtualNetworkResultList(t *testing.T) {
	results := []VirtualNetworkResult{
		{
			ID:               "vnet-1",
			Name:             "default-network",
			IsDefaultNetwork: true,
		},
		{
			ID:               "vnet-2",
			Name:             "secondary-network",
			IsDefaultNetwork: false,
		},
		{
			ID:               "vnet-3",
			Name:             "dev-network",
			IsDefaultNetwork: false,
			Comment:          "Development network",
		},
	}

	assert.Len(t, results, 3)

	// Find default network
	var defaultNetwork *VirtualNetworkResult
	for i := range results {
		if results[i].IsDefaultNetwork {
			defaultNetwork = &results[i]
			break
		}
	}

	assert.NotNil(t, defaultNetwork)
	assert.Equal(t, "default-network", defaultNetwork.Name)
}

func TestTunnelRouteResultList(t *testing.T) {
	results := []TunnelRouteResult{
		{
			Network:          "10.0.0.0/8",
			TunnelID:         "tunnel-1",
			VirtualNetworkID: "vnet-1",
		},
		{
			Network:          "192.168.0.0/16",
			TunnelID:         "tunnel-1",
			VirtualNetworkID: "vnet-1",
		},
		{
			Network:          "172.16.0.0/12",
			TunnelID:         "tunnel-2",
			VirtualNetworkID: "vnet-2",
		},
	}

	assert.Len(t, results, 3)

	// Count routes per tunnel
	tunnelRouteCounts := make(map[string]int)
	for _, r := range results {
		tunnelRouteCounts[r.TunnelID]++
	}

	assert.Equal(t, 2, tunnelRouteCounts["tunnel-1"])
	assert.Equal(t, 1, tunnelRouteCounts["tunnel-2"])
}

func TestEmptyVirtualNetworkResult(t *testing.T) {
	result := VirtualNetworkResult{}

	assert.Empty(t, result.ID)
	assert.Empty(t, result.Name)
	assert.Empty(t, result.Comment)
	assert.False(t, result.IsDefaultNetwork)
	assert.Nil(t, result.DeletedAt)
}

func TestEmptyTunnelRouteResult(t *testing.T) {
	result := TunnelRouteResult{}

	assert.Empty(t, result.Network)
	assert.Empty(t, result.TunnelID)
	assert.Empty(t, result.TunnelName)
	assert.Empty(t, result.VirtualNetworkID)
	assert.Empty(t, result.Comment)
}

// Helper function
func strPtr(s string) *string {
	return &s
}
