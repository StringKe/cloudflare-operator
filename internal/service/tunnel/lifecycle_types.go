// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

//nolint:revive // max-public-structs is acceptable for lifecycle operation types
package tunnel

import (
	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// LifecycleAction defines the action to perform on a tunnel
type LifecycleAction string

const (
	// LifecycleActionCreate creates a new tunnel
	LifecycleActionCreate LifecycleAction = "create"
	// LifecycleActionDelete deletes an existing tunnel
	LifecycleActionDelete LifecycleAction = "delete"
	// LifecycleActionAdopt adopts an existing tunnel
	LifecycleActionAdopt LifecycleAction = "adopt"
)

// LifecycleConfig represents the configuration for a tunnel lifecycle operation
type LifecycleConfig struct {
	// Action is the lifecycle operation to perform
	Action LifecycleAction `json:"action"`

	// TunnelName is the name of the tunnel (required for create/adopt)
	TunnelName string `json:"tunnelName,omitempty"`

	// TunnelID is the existing tunnel ID (required for delete/adopt)
	TunnelID string `json:"tunnelId,omitempty"`

	// ConfigSrc specifies the configuration source (local/cloudflare)
	// If "cloudflare", the tunnel uses remotely managed config
	ConfigSrc string `json:"configSrc,omitempty"`

	// ExistingTunnelID is the tunnel ID to adopt (for adopt action)
	ExistingTunnelID string `json:"existingTunnelId,omitempty"`
}

// LifecycleResult contains the result of a tunnel lifecycle operation
type LifecycleResult struct {
	// TunnelID is the Cloudflare tunnel ID
	TunnelID string `json:"tunnelId"`

	// TunnelName is the tunnel name
	TunnelName string `json:"tunnelName"`

	// TunnelToken is the token used by cloudflared to authenticate
	TunnelToken string `json:"tunnelToken,omitempty"`

	// Credentials is the base64-encoded tunnel credentials JSON
	Credentials string `json:"credentials,omitempty"`

	// AccountTag is the Cloudflare account tag (from credentials)
	AccountTag string `json:"accountTag,omitempty"`
}

// CreateTunnelOptions contains options for creating a tunnel
type CreateTunnelOptions struct {
	// TunnelName is the name of the tunnel to create
	TunnelName string
	// AccountID is the Cloudflare account ID
	AccountID string
	// ConfigSrc specifies the configuration source
	ConfigSrc string
	// Source identifies the K8s resource
	Source service.Source
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
}

// DeleteTunnelOptions contains options for deleting a tunnel
type DeleteTunnelOptions struct {
	// TunnelID is the ID of the tunnel to delete
	TunnelID string
	// TunnelName is the name of the tunnel (for SyncState naming)
	TunnelName string
	// AccountID is the Cloudflare account ID
	AccountID string
	// Source identifies the K8s resource
	Source service.Source
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
	// CleanupRoutes indicates whether to delete associated routes
	CleanupRoutes bool
}

// AdoptTunnelOptions contains options for adopting an existing tunnel
type AdoptTunnelOptions struct {
	// TunnelID is the ID of the tunnel to adopt
	TunnelID string
	// TunnelName is the expected tunnel name
	TunnelName string
	// AccountID is the Cloudflare account ID
	AccountID string
	// Source identifies the K8s resource
	Source service.Source
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
}

// Result data keys for TunnelLifecycle SyncState
const (
	ResultKeyTunnelID    = "tunnelId"
	ResultKeyTunnelName  = "tunnelName"
	ResultKeyTunnelToken = "tunnelToken"
	ResultKeyCredentials = "credentials"
	ResultKeyAccountTag  = "accountTag"
)
