// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package virtualnetwork provides types and service for VirtualNetwork configuration management.
package virtualnetwork

import (
	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// VirtualNetworkConfig represents a VirtualNetwork configuration.
// Each VirtualNetwork K8s resource contributes one VirtualNetworkConfig to its SyncState.
type VirtualNetworkConfig struct {
	// Name is the virtual network name
	Name string `json:"name"`
	// Comment is a user-provided comment for the virtual network
	Comment string `json:"comment,omitempty"`
	// IsDefaultNetwork indicates if this is the default virtual network
	IsDefaultNetwork bool `json:"isDefaultNetwork,omitempty"`
}

// RegisterOptions contains options for registering a VirtualNetwork configuration.
type RegisterOptions struct {
	// AccountID is the Cloudflare Account ID
	AccountID string
	// VirtualNetworkID is the Cloudflare VirtualNetwork ID (if already created)
	// Used as the CloudflareID in SyncState for existing networks
	// For new networks, a placeholder is used until the network is created
	VirtualNetworkID string
	// Source identifies the K8s resource contributing this configuration
	Source service.Source
	// Config contains the VirtualNetwork configuration
	Config VirtualNetworkConfig
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
}

// SyncResult contains the result of a successful VirtualNetwork sync operation.
type SyncResult struct {
	// VirtualNetworkID is the Cloudflare VirtualNetwork ID after sync
	VirtualNetworkID string
	// Name is the virtual network name
	Name string
	// IsDefault indicates if this is the default virtual network
	IsDefault bool
}
