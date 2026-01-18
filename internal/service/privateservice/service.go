// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package privateservice provides the PrivateServiceService for managing Cloudflare PrivateService configuration.
package privateservice

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
	// ResourceType is the SyncState resource type for PrivateService
	ResourceType = v1alpha2.SyncResourcePrivateService

	// PriorityPrivateService is the default priority for PrivateService configuration
	PriorityPrivateService = 100
)

// Service handles PrivateService configuration registration.
// It implements the ConfigService interface for PrivateService resources.
type Service struct {
	*service.BaseService
}

// NewService creates a new PrivateServiceService
func NewService(c client.Client) *Service {
	return &Service{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers a PrivateService configuration to SyncState.
// Each PrivateService K8s resource has its own SyncState, keyed by a generated ID
// based on the network CIDR and virtual network ID.
//
//nolint:revive // cognitive complexity is acceptable for SyncState lookup logic
func (s *Service) Register(ctx context.Context, opts RegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"accountId", opts.AccountID,
		"source", opts.Source.String(),
		"network", opts.Config.Network,
	)
	logger.V(1).Info("Registering PrivateService configuration")

	// Try to find existing SyncState in this order:
	// 1. By pending-{namespace}-{name} (for resources being created)
	// 2. By RouteNetwork (for resources that were already synced)
	// This prevents creating duplicate SyncStates when RouteNetwork changes
	var syncState *v1alpha2.CloudflareSyncState
	var err error

	// First, try to find by pending ID (primary lookup for resources in creation)
	pendingID := fmt.Sprintf("pending-%s-%s", opts.Source.Namespace, opts.Source.Name)
	syncState, err = s.GetSyncState(ctx, ResourceType, pendingID)
	if err != nil {
		return fmt.Errorf("lookup syncstate by pending ID: %w", err)
	}

	// If not found by pending ID and RouteNetwork is known, try by RouteNetwork-based ID
	if syncState == nil && opts.RouteNetwork != "" {
		routeBasedID := sanitizeNetworkForName(opts.RouteNetwork)
		if opts.VirtualNetworkID != "" && opts.VirtualNetworkID != "default" {
			routeBasedID = fmt.Sprintf("%s-%s", routeBasedID, opts.VirtualNetworkID[:min(8, len(opts.VirtualNetworkID))])
		}
		syncState, err = s.GetSyncState(ctx, ResourceType, routeBasedID)
		if err != nil {
			return fmt.Errorf("lookup syncstate by route ID: %w", err)
		}
	}

	// If still not found, create a new SyncState
	if syncState == nil {
		syncStateID := pendingID
		if opts.RouteNetwork != "" {
			syncStateID = sanitizeNetworkForName(opts.RouteNetwork)
			if opts.VirtualNetworkID != "" && opts.VirtualNetworkID != "default" {
				syncStateID = fmt.Sprintf("%s-%s", syncStateID, opts.VirtualNetworkID[:min(8, len(opts.VirtualNetworkID))])
			}
		}
		syncState, err = s.GetOrCreateSyncState(
			ctx,
			ResourceType,
			syncStateID,
			opts.AccountID,
			"", // PrivateService doesn't have a zone ID
			opts.CredentialsRef,
		)
		if err != nil {
			return fmt.Errorf("get/create syncstate for PrivateService: %w", err)
		}
	}

	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityPrivateService); err != nil {
		return fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("PrivateService configuration registered successfully",
		"syncState", syncState.Name,
		"network", opts.Config.Network)
	return nil
}

// Unregister removes a PrivateService's configuration from the SyncState.
// This is called when the PrivateService K8s resource is deleted.
//
//nolint:revive // cognitive complexity is acceptable for unregistration with fallback logic
func (s *Service) Unregister(ctx context.Context, network, virtualNetworkID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"network", network,
		"virtualNetworkId", virtualNetworkID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering PrivateService from SyncState")

	// Build possible SyncState IDs to search
	syncStateIDs := []string{
		// Pending placeholder (namespace-scoped)
		fmt.Sprintf("pending-%s-%s", source.Namespace, source.Name),
	}

	// Add network-based IDs if network is known
	if network != "" {
		sanitizedNetwork := sanitizeNetworkForName(network)
		if virtualNetworkID != "" && len(virtualNetworkID) >= 8 {
			// With virtual network ID suffix
			syncStateIDs = append([]string{
				fmt.Sprintf("%s-%s", sanitizedNetwork, virtualNetworkID[:min(8, len(virtualNetworkID))]),
			}, syncStateIDs...)
		}
		// Just the network
		syncStateIDs = append([]string{sanitizedNetwork}, syncStateIDs...)
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

		logger.Info("PrivateService unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// GetSyncStatus returns the sync status for a PrivateService.
//
//nolint:revive // cognitive complexity acceptable for status lookup logic
func (s *Service) GetSyncStatus(ctx context.Context, source service.Source, knownNetwork, virtualNetworkID string) (*SyncStatus, error) {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"knownNetwork", knownNetwork,
	)

	// Build possible SyncState IDs to search
	var syncStateIDs []string

	// Add pending placeholder first (since PrivateService is namespaced)
	syncStateIDs = append(syncStateIDs, fmt.Sprintf("pending-%s-%s", source.Namespace, source.Name))

	// Add network-based IDs if network is known
	if knownNetwork != "" {
		sanitizedNetwork := sanitizeNetworkForName(knownNetwork)
		if virtualNetworkID != "" && len(virtualNetworkID) >= 8 {
			syncStateIDs = append(syncStateIDs,
				fmt.Sprintf("%s-%s", sanitizedNetwork, virtualNetworkID[:min(8, len(virtualNetworkID))]))
		}
		syncStateIDs = append(syncStateIDs, sanitizedNetwork)
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

		// Check if synced
		isSynced := syncState.Status.SyncStatus == v1alpha2.SyncStatusSynced
		network := syncState.Spec.CloudflareID

		// If the CloudflareID starts with "pending-", it's not a real ID
		if strings.HasPrefix(network, "pending-") {
			network = ""
		}

		return &SyncStatus{
			IsSynced:    isSynced,
			Network:     network,
			AccountID:   syncState.Spec.AccountID,
			SyncStateID: syncState.Name,
		}, nil
	}

	return nil, nil
}

// UpdateRouteID updates the SyncState to use the actual network CIDR as the ID
// after the route is created. This migrates from the pending placeholder.
func (s *Service) UpdateRouteID(ctx context.Context, source service.Source, network, virtualNetworkID string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"network", network,
	)

	pendingID := fmt.Sprintf("pending-%s-%s", source.Namespace, source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceType, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		// No pending state, might already be using real ID
		logger.V(1).Info("No pending SyncState found, PrivateService may already use actual ID")
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
