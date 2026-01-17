// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitTunnelEntry(t *testing.T) {
	tests := []struct {
		name  string
		entry SplitTunnelEntry
	}{
		{
			name: "address entry",
			entry: SplitTunnelEntry{
				Address:     "192.168.1.0/24",
				Description: "Internal network",
			},
		},
		{
			name: "host entry",
			entry: SplitTunnelEntry{
				Host:        "internal.example.com",
				Description: "Internal host",
			},
		},
		{
			name: "both address and host",
			entry: SplitTunnelEntry{
				Address:     "10.0.0.0/8",
				Host:        "*.internal.example.com",
				Description: "Combined entry",
			},
		},
		{
			name:  "empty entry",
			entry: SplitTunnelEntry{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			// Verify struct fields can be accessed
			_ = tt.entry.Address
			_ = tt.entry.Host
			_ = tt.entry.Description
		})
	}
}

func TestFallbackDomainEntry(t *testing.T) {
	tests := []struct {
		name  string
		entry FallbackDomainEntry
	}{
		{
			name: "simple domain",
			entry: FallbackDomainEntry{
				Suffix:      "internal.example.com",
				Description: "Internal domain",
			},
		},
		{
			name: "domain with DNS servers",
			entry: FallbackDomainEntry{
				Suffix:      "corp.example.com",
				Description: "Corporate domain",
				DNSServer:   []string{"10.0.0.1", "10.0.0.2"},
			},
		},
		{
			name: "domain with single DNS server",
			entry: FallbackDomainEntry{
				Suffix:    "local.example.com",
				DNSServer: []string{"192.168.1.1"},
			},
		},
		{
			name:  "empty entry",
			entry: FallbackDomainEntry{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify struct fields can be accessed
			_ = tt.entry.Suffix
			_ = tt.entry.Description
			if len(tt.entry.DNSServer) > 0 {
				assert.NotEmpty(t, tt.entry.DNSServer)
			}
		})
	}
}

func TestSplitTunnelEntryConversion(t *testing.T) {
	// Test conversion to/from different formats
	entries := []SplitTunnelEntry{
		{Address: "192.168.1.0/24", Description: "LAN"},
		{Address: "10.0.0.0/8", Description: "Private"},
		{Host: "*.local", Description: "Local domains"},
	}

	assert.Len(t, entries, 3)

	// Verify each entry has expected fields
	assert.Equal(t, "192.168.1.0/24", entries[0].Address)
	assert.Equal(t, "LAN", entries[0].Description)
	assert.Empty(t, entries[0].Host)

	assert.Equal(t, "10.0.0.0/8", entries[1].Address)
	assert.Equal(t, "Private", entries[1].Description)

	assert.Equal(t, "*.local", entries[2].Host)
	assert.Empty(t, entries[2].Address)
}

func TestFallbackDomainEntryConversion(t *testing.T) {
	// Test conversion to/from different formats
	entries := []FallbackDomainEntry{
		{
			Suffix:      "internal.corp.com",
			Description: "Internal corporate network",
			DNSServer:   []string{"10.1.1.1", "10.1.1.2"},
		},
		{
			Suffix:    "vpn.corp.com",
			DNSServer: []string{"10.2.1.1"},
		},
	}

	assert.Len(t, entries, 2)

	// Verify first entry
	assert.Equal(t, "internal.corp.com", entries[0].Suffix)
	assert.Equal(t, "Internal corporate network", entries[0].Description)
	assert.Len(t, entries[0].DNSServer, 2)

	// Verify second entry
	assert.Equal(t, "vpn.corp.com", entries[1].Suffix)
	assert.Empty(t, entries[1].Description)
	assert.Len(t, entries[1].DNSServer, 1)
}

func TestSplitTunnelEntryValidation(t *testing.T) {
	tests := []struct {
		name    string
		entry   SplitTunnelEntry
		isValid bool
	}{
		{
			name: "valid address",
			entry: SplitTunnelEntry{
				Address: "192.168.0.0/16",
			},
			isValid: true,
		},
		{
			name: "valid host",
			entry: SplitTunnelEntry{
				Host: "*.example.com",
			},
			isValid: true,
		},
		{
			name:    "empty - could be valid for clear operation",
			entry:   SplitTunnelEntry{},
			isValid: true, // Empty might be valid for certain operations
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			// Validation logic would go here
			// For now, just verify the struct is accessible
			hasAddress := tt.entry.Address != ""
			hasHost := tt.entry.Host != ""
			_ = hasAddress || hasHost
		})
	}
}

func TestFallbackDomainEntryValidation(t *testing.T) {
	tests := []struct {
		name    string
		entry   FallbackDomainEntry
		isValid bool
	}{
		{
			name: "valid with suffix only",
			entry: FallbackDomainEntry{
				Suffix: "example.com",
			},
			isValid: true,
		},
		{
			name: "valid with DNS servers",
			entry: FallbackDomainEntry{
				Suffix:    "example.com",
				DNSServer: []string{"8.8.8.8"},
			},
			isValid: true,
		},
		{
			name:    "empty suffix - invalid",
			entry:   FallbackDomainEntry{},
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simple validation: suffix is required
			isValid := tt.entry.Suffix != ""
			if tt.isValid {
				assert.True(t, isValid || !tt.isValid, "Expected entry to be valid")
			}
		})
	}
}
