// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package virtualnetwork provides the VirtualNetworkService for managing Cloudflare VirtualNetwork configuration.
package virtualnetwork

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
	// ResourceType is the SyncState resource type for VirtualNetwork
	ResourceType = v1alpha2.SyncResourceVirtualNetwork

	// PriorityVirtualNetwork is the default priority for VirtualNetwork configuration
	PriorityVirtualNetwork = 100
)

// Service handles VirtualNetwork configuration registration.
// It implements the ConfigService interface for VirtualNetwork resources.
type Service struct {
	*service.BaseService
}

// NewService creates a new VirtualNetworkService
func NewService(c client.Client) *Service {
	return &Service{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers a VirtualNetwork configuration to SyncState.
// Each VirtualNetwork K8s resource has its own SyncState, keyed by a generated ID
// (namespace/name) until the Cloudflare VirtualNetwork ID is known.
//
//nolint:revive // cognitive complexity is acceptable for SyncState lookup logic
func (s *Service) Register(ctx context.Context, opts RegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"accountId", opts.AccountID,
		"source", opts.Source.String(),
		"vnetName", opts.Config.Name,
	)
	logger.V(1).Info("Registering VirtualNetwork configuration")

	// Try to find existing SyncState in this order:
	// 1. By pending-{source.Name} (for resources being created)
	// 2. By VirtualNetworkID (for resources that were already synced)
	// This prevents creating duplicate SyncStates when VirtualNetworkID changes
	var syncState *v1alpha2.CloudflareSyncState
	var err error

	// First, try to find by pending ID (primary lookup for resources in creation)
	pendingID := fmt.Sprintf("pending-%s", opts.Source.Name)
	syncState, err = s.GetSyncState(ctx, ResourceType, pendingID)
	if err != nil {
		return fmt.Errorf("lookup syncstate by pending ID: %w", err)
	}

	// If not found by pending ID and VirtualNetworkID is known, try by VirtualNetworkID
	if syncState == nil && opts.VirtualNetworkID != "" {
		syncState, err = s.GetSyncState(ctx, ResourceType, opts.VirtualNetworkID)
		if err != nil {
			return fmt.Errorf("lookup syncstate by vnet ID: %w", err)
		}
	}

	// If still not found, create a new SyncState
	if syncState == nil {
		syncStateID := pendingID
		if opts.VirtualNetworkID != "" {
			syncStateID = opts.VirtualNetworkID
		}
		syncState, err = s.GetOrCreateSyncState(
			ctx,
			ResourceType,
			syncStateID,
			opts.AccountID,
			"", // VirtualNetwork doesn't have a zone ID
			opts.CredentialsRef,
		)
		if err != nil {
			return fmt.Errorf("get/create syncstate for VirtualNetwork: %w", err)
		}
	}

	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityVirtualNetwork); err != nil {
		return fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("VirtualNetwork configuration registered successfully",
		"syncState", syncState.Name,
		"vnetName", opts.Config.Name)
	return nil
}

// Unregister removes a VirtualNetwork's configuration from the SyncState.
// This is called when the VirtualNetwork K8s resource is deleted.
//
//nolint:revive // cognitive complexity is acceptable for unregistration with fallback logic
func (s *Service) Unregister(ctx context.Context, vnetID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"vnetId", vnetID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering VirtualNetwork from SyncState")

	// Try both possible SyncState IDs
	// 1. The actual VirtualNetwork ID (if network was created)
	// 2. The pending placeholder (if network was never created)
	syncStateIDs := []string{
		vnetID,
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

		logger.Info("VirtualNetwork unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// GetSyncStatus returns the sync status for a VirtualNetwork.
//
//nolint:revive // cognitive complexity acceptable for status lookup logic
func (s *Service) GetSyncStatus(ctx context.Context, source service.Source, knownVNetID string) (*SyncStatus, error) {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"knownVNetId", knownVNetID,
	)

	// Try both possible SyncState IDs
	syncStateIDs := []string{
		knownVNetID,
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

		// Check if synced
		isSynced := syncState.Status.SyncStatus == v1alpha2.SyncStatusSynced
		vnetID := syncState.Spec.CloudflareID

		// If the CloudflareID starts with "pending-", it's not a real ID
		if strings.HasPrefix(vnetID, "pending-") {
			vnetID = ""
		}

		return &SyncStatus{
			IsSynced:         isSynced,
			VirtualNetworkID: vnetID,
			AccountID:        syncState.Spec.AccountID,
			SyncStateID:      syncState.Name,
		}, nil
	}

	return nil, nil
}

// UpdateVirtualNetworkID updates the SyncState to use the actual Cloudflare VirtualNetwork ID
// after the network is created. This migrates from the pending placeholder.
func (s *Service) UpdateVirtualNetworkID(ctx context.Context, source service.Source, newVNetID string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"newVNetId", newVNetID,
	)

	pendingID := fmt.Sprintf("pending-%s", source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceType, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		// No pending state, might already be using real ID
		logger.V(1).Info("No pending SyncState found, VirtualNetwork may already use actual ID")
		return nil
	}

	// Update the SyncState name to use the real VirtualNetwork ID
	// This requires creating a new SyncState with the correct ID and deleting the old one
	newSyncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceType,
		newVNetID,
		pendingSyncState.Spec.AccountID,
		pendingSyncState.Spec.ZoneID,
		pendingSyncState.Spec.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("create new syncstate with VirtualNetwork ID: %w", err)
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

	logger.Info("Updated SyncState to use actual VirtualNetwork ID",
		"oldId", pendingID,
		"newId", newVNetID)
	return nil
}
