// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package networkroute provides types and service for NetworkRoute configuration management.
package networkroute

import (
	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// NetworkRouteConfig represents a NetworkRoute configuration.
// Each NetworkRoute K8s resource contributes one NetworkRouteConfig to its SyncState.
type NetworkRouteConfig struct {
	// Network is the CIDR notation for the route (e.g., "10.0.0.0/8")
	Network string `json:"network"`
	// TunnelID is the Cloudflare Tunnel ID to route traffic through
	TunnelID string `json:"tunnelId"`
	// TunnelName is the Cloudflare Tunnel name (for display purposes)
	TunnelName string `json:"tunnelName,omitempty"`
	// VirtualNetworkID is the Cloudflare Virtual Network ID
	VirtualNetworkID string `json:"virtualNetworkId,omitempty"`
	// Comment is a user-provided comment for the route
	Comment string `json:"comment,omitempty"`
}

// RegisterOptions contains options for registering a NetworkRoute configuration.
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
	// Config contains the NetworkRoute configuration
	Config NetworkRouteConfig
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
}

// SyncResult contains the result of a successful NetworkRoute sync operation.
type SyncResult struct {
	// Network is the CIDR notation for the route
	Network string
	// TunnelID is the Cloudflare Tunnel ID
	TunnelID string
	// TunnelName is the Cloudflare Tunnel name
	TunnelName string
	// VirtualNetworkID is the Cloudflare Virtual Network ID
	VirtualNetworkID string
}
