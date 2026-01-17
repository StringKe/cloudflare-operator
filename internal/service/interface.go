// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package service provides the Core Service layer for the unified sync architecture.
// Core Services are responsible for:
// - Receiving configuration from Resource Controllers
// - Validating business rules
// - Creating/updating CloudflareSyncState CRDs
// - Handling resource dependencies
package service

import (
	"context"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// Common status state constants used across services.
const (
	// StateReady indicates the resource is synced and ready.
	StateReady = "Ready"
)

// Source identifies the Kubernetes resource that contributes configuration.
// This is used to track ownership and enable proper cleanup when resources are deleted.
type Source struct {
	// Kind is the resource kind (e.g., "Tunnel", "Ingress", "TunnelBinding")
	Kind string `json:"kind"`
	// Namespace is the resource namespace (empty for cluster-scoped resources)
	Namespace string `json:"namespace,omitempty"`
	// Name is the resource name
	Name string `json:"name"`
}

// String returns a string representation of the source in the format "Kind/[Namespace/]Name"
func (s Source) String() string {
	if s.Namespace == "" {
		return s.Kind + "/" + s.Name
	}
	return s.Kind + "/" + s.Namespace + "/" + s.Name
}

// ToReference converts Source to v1alpha2.SourceReference
func (s Source) ToReference() v1alpha2.SourceReference {
	return v1alpha2.SourceReference{
		Kind:      s.Kind,
		Namespace: s.Namespace,
		Name:      s.Name,
	}
}

// FromReference creates a Source from v1alpha2.SourceReference
func FromReference(ref v1alpha2.SourceReference) Source {
	return Source{
		Kind:      ref.Kind,
		Namespace: ref.Namespace,
		Name:      ref.Name,
	}
}

// RegisterOptions contains options for registering configuration to a SyncState
type RegisterOptions struct {
	// ResourceType is the type of Cloudflare resource
	ResourceType v1alpha2.SyncResourceType
	// CloudflareID is the Cloudflare resource identifier
	CloudflareID string
	// AccountID is the Cloudflare account ID
	AccountID string
	// ZoneID is the Cloudflare zone ID (optional)
	ZoneID string
	// Source identifies the contributing K8s resource
	Source Source
	// Config is the configuration to register (will be JSON serialized)
	Config interface{}
	// Priority determines conflict resolution (lower = higher priority)
	Priority int
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
}

// UnregisterOptions contains options for unregistering configuration from a SyncState
type UnregisterOptions struct {
	// ResourceType is the type of Cloudflare resource
	ResourceType v1alpha2.SyncResourceType
	// CloudflareID is the Cloudflare resource identifier
	CloudflareID string
	// Source identifies the K8s resource to unregister
	Source Source
}

// ConfigService is the interface that all Core Services must implement.
// Each service type (Tunnel, DNS, Access, etc.) implements this interface
// to handle configuration registration and unregistration.
type ConfigService interface {
	// Register adds or updates configuration from a source.
	// This creates or updates the corresponding CloudflareSyncState CRD.
	Register(ctx context.Context, opts RegisterOptions) error

	// Unregister removes configuration from a source.
	// If no sources remain, the CloudflareSyncState CRD is deleted.
	Unregister(ctx context.Context, opts UnregisterOptions) error
}

// Priority constants for different source types
const (
	// PriorityTunnel is the priority for Tunnel/ClusterTunnel settings (highest)
	PriorityTunnel = 10
	// PriorityBinding is the priority for TunnelBinding rules
	PriorityBinding = 50
	// PriorityDefault is the default priority for other sources
	PriorityDefault = 100
)
