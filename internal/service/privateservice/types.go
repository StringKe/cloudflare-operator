// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package privateservice provides types and service for PrivateService configuration management.
package privateservice

import (
	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// PrivateServiceConfig represents a PrivateService configuration.
// Each PrivateService K8s resource contributes one PrivateServiceConfig to its SyncState.
type PrivateServiceConfig struct {
	// Network is the CIDR notation for the route (e.g., "10.96.0.1/32")
	// Derived from the referenced Service's ClusterIP
	Network string `json:"network"`
	// TunnelID is the Cloudflare Tunnel ID to route traffic through
	TunnelID string `json:"tunnelId"`
	// TunnelName is the Cloudflare Tunnel name (for display purposes)
	TunnelName string `json:"tunnelName,omitempty"`
	// VirtualNetworkID is the Cloudflare Virtual Network ID
	VirtualNetworkID string `json:"virtualNetworkId,omitempty"`
	// ServiceIP is the ClusterIP of the referenced K8s Service
	ServiceIP string `json:"serviceIP"`
	// Comment is a user-provided comment for the route
	Comment string `json:"comment,omitempty"`
}

// RegisterOptions contains options for registering a PrivateService configuration.
type RegisterOptions struct {
	// AccountID is the Cloudflare Account ID
	AccountID string
	// RouteNetwork is the network CIDR that serves as the route identifier
	// Used as the CloudflareID in SyncState
	// For new routes, a placeholder is used until the route is created
	RouteNetwork string
	// VirtualNetworkID is the Virtual Network ID for this route
	VirtualNetworkID string
	// Source identifies the K8s resource contributing this configuration
	Source service.Source
	// Config contains the PrivateService configuration
	Config PrivateServiceConfig
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
}

// SyncResult contains the result of a successful PrivateService sync operation.
type SyncResult struct {
	// Network is the CIDR notation for the route
	Network string
	// TunnelID is the Cloudflare Tunnel ID
	TunnelID string
	// TunnelName is the Cloudflare Tunnel name
	TunnelName string
	// VirtualNetworkID is the Cloudflare Virtual Network ID
	VirtualNetworkID string
	// ServiceIP is the ClusterIP of the referenced K8s Service
	ServiceIP string
}

// SyncStatus represents the sync status of a PrivateService.
type SyncStatus struct {
	// IsSynced indicates whether the PrivateService has been synced to Cloudflare
	IsSynced bool
	// Network is the CIDR notation for the route
	Network string
	// AccountID is the Cloudflare Account ID
	AccountID string
	// SyncStateID is the name of the SyncState resource
	SyncStateID string
}
