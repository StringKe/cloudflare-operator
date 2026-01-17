// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package networkroute provides the NetworkRouteService for managing Cloudflare NetworkRoute configuration.
package networkroute

import (
	"context"
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

const (
	// ResourceType is the SyncState resource type for NetworkRoute
	ResourceType = v1alpha2.SyncResourceNetworkRoute

	// PriorityNetworkRoute is the default priority for NetworkRoute configuration
	PriorityNetworkRoute = 100
)

// Service handles NetworkRoute configuration registration.
// It implements the ConfigService interface for NetworkRoute resources.
type Service struct {
	*service.BaseService
}

// NewService creates a new NetworkRouteService
func NewService(c client.Client) *Service {
	return &Service{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers a NetworkRoute configuration to SyncState.
// Each NetworkRoute K8s resource has its own SyncState, keyed by a generated ID
// based on the network CIDR and virtual network ID.
func (s *Service) Register(ctx context.Context, opts RegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"accountId", opts.AccountID,
		"source", opts.Source.String(),
		"network", opts.Config.Network,
	)
	logger.V(1).Info("Registering NetworkRoute configuration")

	// Generate SyncState ID:
	// - If RouteNetwork is known (existing route), use a sanitized version
	// - Otherwise, use a placeholder based on source (name only since NetworkRoute is cluster-scoped)
	syncStateID := opts.RouteNetwork
	if syncStateID == "" {
		syncStateID = fmt.Sprintf("pending-%s", opts.Source.Name)
	} else {
		// Sanitize network CIDR for use as name (replace / with -)
		syncStateID = sanitizeNetworkForName(syncStateID)
		// Include virtual network ID if specified to make it unique
		if opts.VirtualNetworkID != "" && opts.VirtualNetworkID != "default" {
			syncStateID = fmt.Sprintf("%s-%s", syncStateID, opts.VirtualNetworkID[:8])
		}
	}

	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceType,
		syncStateID,
		opts.AccountID,
		"", // NetworkRoute doesn't have a zone ID
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get/create syncstate for NetworkRoute: %w", err)
	}

	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityNetworkRoute); err != nil {
		return fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("NetworkRoute configuration registered successfully",
		"syncState", syncState.Name,
		"network", opts.Config.Network)
	return nil
}

// Unregister removes a NetworkRoute's configuration from the SyncState.
// This is called when the NetworkRoute K8s resource is deleted.
//
//nolint:revive // cognitive complexity is acceptable for unregistration with fallback logic
func (s *Service) Unregister(ctx context.Context, network, virtualNetworkID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"network", network,
		"virtualNetworkId", virtualNetworkID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering NetworkRoute from SyncState")

	// Build possible SyncState IDs to search
	sanitizedNetwork := sanitizeNetworkForName(network)
	syncStateIDs := []string{
		// With virtual network ID suffix
		fmt.Sprintf("%s-%s", sanitizedNetwork, virtualNetworkID[:min(8, len(virtualNetworkID))]),
		// Just the network
		sanitizedNetwork,
		// Pending placeholder
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceType, id)
		if err != nil {
			logger.V(1).Info("Error getting SyncState", "syncStateId", id, "error", err)
			continue
		}
		if syncState == nil {
			continue
		}

		if err := s.RemoveSource(ctx, syncState, source); err != nil {
			logger.Error(err, "Failed to remove source from SyncState", "syncStateId", id)
			continue
		}

		logger.Info("NetworkRoute unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// UpdateRouteID updates the SyncState to use the actual network CIDR as the ID
// after the route is created. This migrates from the pending placeholder.
func (s *Service) UpdateRouteID(ctx context.Context, source service.Source, network, virtualNetworkID string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"network", network,
	)

	pendingID := fmt.Sprintf("pending-%s", source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceType, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		// No pending state, might already be using real ID
		logger.V(1).Info("No pending SyncState found, NetworkRoute may already use actual ID")
		return nil
	}

	// Build new ID from network and virtual network ID
	newID := sanitizeNetworkForName(network)
	if virtualNetworkID != "" && virtualNetworkID != "default" {
		newID = fmt.Sprintf("%s-%s", newID, virtualNetworkID[:min(8, len(virtualNetworkID))])
	}

	// Update the SyncState name to use the real route ID
	// This requires creating a new SyncState with the correct ID and deleting the old one
	newSyncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceType,
		newID,
		pendingSyncState.Spec.AccountID,
		pendingSyncState.Spec.ZoneID,
		pendingSyncState.Spec.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("create new syncstate with route ID: %w", err)
	}

	// Copy sources from pending to new
	newSyncState.Spec.Sources = pendingSyncState.Spec.Sources
	if err := s.Client.Update(ctx, newSyncState); err != nil {
		return fmt.Errorf("update new syncstate with sources: %w", err)
	}

	// Delete the pending SyncState
	if err := s.Client.Delete(ctx, pendingSyncState); err != nil {
		logger.Error(err, "Failed to delete pending SyncState", "pendingId", pendingID)
		// Non-fatal - the pending state will be orphaned but won't cause issues
	}

	logger.Info("Updated SyncState to use actual route ID",
		"oldId", pendingID,
		"newId", newID)
	return nil
}

// sanitizeNetworkForName converts a CIDR notation to a valid Kubernetes name.
// e.g., "10.0.0.0/8" becomes "10-0-0-0-8"
func sanitizeNetworkForName(network string) string {
	// Replace slashes and dots with hyphens
	result := strings.ReplaceAll(network, "/", "-")
	result = strings.ReplaceAll(result, ".", "-")
	return result
}
