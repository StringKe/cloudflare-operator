// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package gateway

//nolint:dupl // Similar patterns across resource types are intentional

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// GatewayListService manages GatewayList configurations via CloudflareSyncState.
type GatewayListService struct {
	*service.BaseService
}

// NewGatewayListService creates a new GatewayList service.
func NewGatewayListService(c client.Client) *GatewayListService {
	return &GatewayListService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers a GatewayList configuration with the SyncState.
func (s *GatewayListService) Register(ctx context.Context, opts GatewayListRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"listName", opts.Config.Name,
		"source", opts.Source.String(),
	)
	logger.V(1).Info("Registering GatewayList configuration")

	// Generate SyncState ID
	syncStateID := opts.ListID
	if syncStateID == "" {
		syncStateID = fmt.Sprintf("pending-%s", opts.Source.Name)
	}

	// Get or create SyncState
	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeGatewayList,
		syncStateID,
		opts.AccountID,
		"", // No zone ID for Gateway resources
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get or create syncstate: %w", err)
	}

	// Update source in SyncState
	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityGatewayList); err != nil {
		return fmt.Errorf("update source: %w", err)
	}

	logger.Info("GatewayList configuration registered", "syncStateId", syncState.Name)
	return nil
}

// Unregister removes a configuration from the SyncState.
//
//nolint:revive // cognitive complexity is acceptable for state lookup logic
func (s *GatewayListService) Unregister(ctx context.Context, listID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"listId", listID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering GatewayList from SyncState")

	// Try multiple possible SyncState IDs
	syncStateIDs := []string{
		listID,
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypeGatewayList, id)
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

		logger.Info("GatewayList unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// UpdateListID updates the SyncState to use the actual list ID.
func (s *GatewayListService) UpdateListID(ctx context.Context, source service.Source, listID, accountID string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"listId", listID,
	)

	pendingID := fmt.Sprintf("pending-%s", source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceTypeGatewayList, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		logger.V(1).Info("No pending SyncState found, GatewayList may already use actual ID")
		return nil
	}

	// Create new SyncState with the actual list ID
	newSyncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeGatewayList,
		listID,
		accountID,
		"",
		pendingSyncState.Spec.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("create new syncstate with list ID: %w", err)
	}

	// Copy sources from pending to new
	newSyncState.Spec.Sources = pendingSyncState.Spec.Sources
	newSyncState.Spec.CloudflareID = listID
	if err := s.Client.Update(ctx, newSyncState); err != nil {
		return fmt.Errorf("update new syncstate with sources: %w", err)
	}

	// Delete the pending SyncState
	if err := s.Client.Delete(ctx, pendingSyncState); err != nil {
		logger.Error(err, "Failed to delete pending SyncState", "pendingId", pendingID)
	}

	logger.Info("Updated SyncState to use actual list ID",
		"oldId", pendingID,
		"newId", listID)
	return nil
}

// UpdateStatus updates the K8s GatewayList resource status based on sync result.
func (s *GatewayListService) UpdateStatus(
	ctx context.Context,
	list *v1alpha2.GatewayList,
	result *GatewayListSyncResult,
) error {
	list.Status.State = service.StateReady
	list.Status.ListID = result.ListID
	list.Status.AccountID = result.AccountID
	list.Status.ItemCount = result.ItemCount
	list.Status.ObservedGeneration = list.Generation

	return s.Client.Status().Update(ctx, list)
}
