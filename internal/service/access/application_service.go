// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package access

//nolint:dupl // Similar patterns across resource types are intentional

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/internal/service"
)

// ApplicationService handles AccessApplication configuration registration.
type ApplicationService struct {
	*service.BaseService
}

// NewApplicationService creates a new AccessApplication service.
func NewApplicationService(c client.Client) *ApplicationService {
	return &ApplicationService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers an AccessApplication configuration to SyncState.
func (s *ApplicationService) Register(ctx context.Context, opts AccessApplicationRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"accountId", opts.AccountID,
		"source", opts.Source.String(),
		"name", opts.Config.Name,
		"domain", opts.Config.Domain,
	)
	logger.V(1).Info("Registering AccessApplication configuration")

	// Generate SyncState ID:
	// - If ApplicationID is known (existing app), use it
	// - Otherwise, use a placeholder based on source
	syncStateID := opts.ApplicationID
	if syncStateID == "" {
		syncStateID = fmt.Sprintf("pending-%s", opts.Source.Name)
	}

	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeAccessApplication,
		syncStateID,
		opts.AccountID,
		"", // AccessApplication doesn't use zone ID
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get/create syncstate for AccessApplication: %w", err)
	}

	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityAccessApplication); err != nil {
		return fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("AccessApplication configuration registered successfully",
		"syncState", syncState.Name)
	return nil
}

// Unregister removes a configuration from the SyncState.
//
//nolint:revive // cognitive complexity is acceptable for state lookup logic
func (s *ApplicationService) Unregister(ctx context.Context, applicationID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"applicationId", applicationID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering AccessApplication from SyncState")

	// Try multiple possible SyncState IDs
	syncStateIDs := []string{
		applicationID,
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypeAccessApplication, id)
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

		logger.Info("AccessApplication unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// UpdateApplicationID updates the SyncState to use the actual application ID
// after the application is created.
func (s *ApplicationService) UpdateApplicationID(ctx context.Context, source service.Source, applicationID string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"applicationId", applicationID,
	)

	pendingID := fmt.Sprintf("pending-%s", source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceTypeAccessApplication, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		logger.V(1).Info("No pending SyncState found, AccessApplication may already use actual ID")
		return nil
	}

	// Create new SyncState with the actual ID
	newSyncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeAccessApplication,
		applicationID,
		pendingSyncState.Spec.AccountID,
		pendingSyncState.Spec.ZoneID,
		pendingSyncState.Spec.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("create new syncstate with application ID: %w", err)
	}

	// Copy sources from pending to new
	newSyncState.Spec.Sources = pendingSyncState.Spec.Sources
	newSyncState.Spec.CloudflareID = applicationID
	if err := s.Client.Update(ctx, newSyncState); err != nil {
		return fmt.Errorf("update new syncstate with sources: %w", err)
	}

	// Delete the pending SyncState
	if err := s.Client.Delete(ctx, pendingSyncState); err != nil {
		logger.Error(err, "Failed to delete pending SyncState", "pendingId", pendingID)
		// Non-fatal - the pending state will be orphaned but won't cause issues
	}

	logger.Info("Updated SyncState to use actual application ID",
		"oldId", pendingID,
		"newId", applicationID)
	return nil
}
