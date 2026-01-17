// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package virtualnetwork

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

func TestVirtualNetworkConfig(t *testing.T) {
	config := VirtualNetworkConfig{
		Name:             "my-virtual-network",
		Comment:          "Production virtual network",
		IsDefaultNetwork: true,
	}

	assert.Equal(t, "my-virtual-network", config.Name)
	assert.Equal(t, "Production virtual network", config.Comment)
	assert.True(t, config.IsDefaultNetwork)
}

func TestVirtualNetworkConfigDefaults(t *testing.T) {
	config := VirtualNetworkConfig{
		Name: "simple-network",
	}

	assert.Equal(t, "simple-network", config.Name)
	assert.Empty(t, config.Comment)
	assert.False(t, config.IsDefaultNetwork)
}

func TestRegisterOptions(t *testing.T) {
	opts := RegisterOptions{
		AccountID:        "account-123",
		VirtualNetworkID: "vnet-456",
		Source: service.Source{
			Kind:      "VirtualNetwork",
			Namespace: "",
			Name:      "my-vnet",
		},
		Config: VirtualNetworkConfig{
			Name:             "Production VNet",
			Comment:          "For production workloads",
			IsDefaultNetwork: false,
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "vnet-456", opts.VirtualNetworkID)
	assert.Equal(t, "VirtualNetwork", opts.Source.Kind)
	assert.Empty(t, opts.Source.Namespace)
	assert.Equal(t, "my-vnet", opts.Source.Name)
	assert.Equal(t, "Production VNet", opts.Config.Name)
}

func TestRegisterOptionsNewNetwork(t *testing.T) {
	opts := RegisterOptions{
		AccountID:        "account-123",
		VirtualNetworkID: "", // Empty for new network
		Source: service.Source{
			Kind: "VirtualNetwork",
			Name: "new-vnet",
		},
		Config: VirtualNetworkConfig{
			Name: "New Virtual Network",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	assert.Empty(t, opts.VirtualNetworkID)
	assert.Equal(t, "New Virtual Network", opts.Config.Name)
}

func TestSyncResult(t *testing.T) {
	result := SyncResult{
		VirtualNetworkID: "vnet-123",
		Name:             "my-network",
		IsDefault:        true,
	}

	assert.Equal(t, "vnet-123", result.VirtualNetworkID)
	assert.Equal(t, "my-network", result.Name)
	assert.True(t, result.IsDefault)
}

func TestSyncResultNonDefault(t *testing.T) {
	result := SyncResult{
		VirtualNetworkID: "vnet-456",
		Name:             "secondary-network",
		IsDefault:        false,
	}

	assert.Equal(t, "vnet-456", result.VirtualNetworkID)
	assert.Equal(t, "secondary-network", result.Name)
	assert.False(t, result.IsDefault)
}

func TestVirtualNetworkConfigVariants(t *testing.T) {
	tests := []struct {
		name      string
		config    VirtualNetworkConfig
		isDefault bool
	}{
		{
			name: "default network",
			config: VirtualNetworkConfig{
				Name:             "default",
				IsDefaultNetwork: true,
			},
			isDefault: true,
		},
		{
			name: "production network",
			config: VirtualNetworkConfig{
				Name:             "production",
				Comment:          "Production workloads",
				IsDefaultNetwork: false,
			},
			isDefault: false,
		},
		{
			name: "development network",
			config: VirtualNetworkConfig{
				Name:             "development",
				Comment:          "Development and testing",
				IsDefaultNetwork: false,
			},
			isDefault: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.isDefault, tt.config.IsDefaultNetwork)
			assert.NotEmpty(t, tt.config.Name)
		})
	}
}
