// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

//nolint:revive // max-public-structs is acceptable for lifecycle operation types
package warp

import (
	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// ConnectorAction defines the action to perform on a WARP connector
type ConnectorAction string

const (
	// ConnectorActionCreate creates a new WARP connector
	ConnectorActionCreate ConnectorAction = "create"
	// ConnectorActionDelete deletes an existing WARP connector
	ConnectorActionDelete ConnectorAction = "delete"
	// ConnectorActionUpdate updates routes for an existing WARP connector
	ConnectorActionUpdate ConnectorAction = "update"
)

// ConnectorLifecycleConfig represents the configuration for a WARP connector lifecycle operation
type ConnectorLifecycleConfig struct {
	// Action is the lifecycle operation to perform
	Action ConnectorAction `json:"action"`

	// ConnectorName is the name of the WARP connector
	ConnectorName string `json:"connectorName"`

	// ConnectorID is the existing WARP connector ID (for update/delete)
	ConnectorID string `json:"connectorId,omitempty"`

	// TunnelID is the tunnel ID associated with the WARP connector (for update/delete)
	TunnelID string `json:"tunnelId,omitempty"`

	// VirtualNetworkID is the Cloudflare VirtualNetwork ID for routes
	VirtualNetworkID string `json:"virtualNetworkId,omitempty"`

	// Routes are the routes to configure for the connector
	Routes []RouteConfig `json:"routes,omitempty"`
}

// RouteConfig represents a route configuration for WARP connector
type RouteConfig struct {
	// Network is the CIDR for the route
	Network string `json:"network"`
	// Comment is an optional comment for the route
	Comment string `json:"comment,omitempty"`
}

// ConnectorLifecycleResult contains the result of a WARP connector lifecycle operation
type ConnectorLifecycleResult struct {
	// ConnectorID is the Cloudflare WARP connector ID
	ConnectorID string `json:"connectorId"`

	// TunnelID is the tunnel ID associated with the connector
	TunnelID string `json:"tunnelId"`

	// TunnelToken is the token used by cloudflared to authenticate
	TunnelToken string `json:"tunnelToken,omitempty"`

	// RoutesConfigured is the number of routes successfully configured
	RoutesConfigured int `json:"routesConfigured,omitempty"`
}

// CreateConnectorOptions contains options for creating a WARP connector
type CreateConnectorOptions struct {
	// ConnectorName is the name of the connector to create
	ConnectorName string
	// AccountID is the Cloudflare account ID
	AccountID string
	// VirtualNetworkID is the Cloudflare VirtualNetwork ID for routes
	VirtualNetworkID string
	// Routes are the routes to configure
	Routes []RouteConfig
	// Source identifies the K8s resource
	Source service.Source
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
}

// DeleteConnectorOptions contains options for deleting a WARP connector
type DeleteConnectorOptions struct {
	// ConnectorID is the ID of the connector to delete
	ConnectorID string
	// ConnectorName is the name of the connector (for SyncState naming)
	ConnectorName string
	// TunnelID is the tunnel ID for route deletion
	TunnelID string
	// VirtualNetworkID is the VirtualNetwork ID for route deletion
	VirtualNetworkID string
	// AccountID is the Cloudflare account ID
	AccountID string
	// Routes are the routes to delete
	Routes []RouteConfig
	// Source identifies the K8s resource
	Source service.Source
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
}

// UpdateConnectorOptions contains options for updating WARP connector routes
type UpdateConnectorOptions struct {
	// ConnectorID is the ID of the connector
	ConnectorID string
	// ConnectorName is the name of the connector
	ConnectorName string
	// TunnelID is the tunnel ID for routes
	TunnelID string
	// VirtualNetworkID is the VirtualNetwork ID for routes
	VirtualNetworkID string
	// AccountID is the Cloudflare account ID
	AccountID string
	// Routes are the routes to configure
	Routes []RouteConfig
	// Source identifies the K8s resource
	Source service.Source
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
}

// Result data keys for WARPConnector SyncState
const (
	ResultKeyConnectorID      = "connectorId"
	ResultKeyTunnelID         = "tunnelId"
	ResultKeyTunnelToken      = "tunnelToken"
	ResultKeyRoutesConfigured = "routesConfigured"
)

// ConnectorResourceType is the SyncState resource type for WARP connector lifecycle
const ConnectorResourceType = v1alpha2.SyncResourceWARPConnector
