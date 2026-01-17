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

// IdentityProviderService handles AccessIdentityProvider configuration registration.
type IdentityProviderService struct {
	*service.BaseService
}

// NewIdentityProviderService creates a new AccessIdentityProvider service.
func NewIdentityProviderService(c client.Client) *IdentityProviderService {
	return &IdentityProviderService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers an AccessIdentityProvider configuration to SyncState.
func (s *IdentityProviderService) Register(ctx context.Context, opts AccessIdentityProviderRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"accountId", opts.AccountID,
		"source", opts.Source.String(),
		"name", opts.Config.Name,
		"type", opts.Config.Type,
	)
	logger.V(1).Info("Registering AccessIdentityProvider configuration")

	// Generate SyncState ID:
	// - If ProviderID is known (existing provider), use it
	// - Otherwise, use a placeholder based on source
	syncStateID := opts.ProviderID
	if syncStateID == "" {
		syncStateID = fmt.Sprintf("pending-%s", opts.Source.Name)
	}

	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeAccessIdentityProvider,
		syncStateID,
		opts.AccountID,
		"", // AccessIdentityProvider doesn't use zone ID
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get/create syncstate for AccessIdentityProvider: %w", err)
	}

	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityAccessIdentityProvider); err != nil {
		return fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("AccessIdentityProvider configuration registered successfully",
		"syncState", syncState.Name)
	return nil
}

// Unregister removes a configuration from the SyncState.
//
//nolint:revive // cognitive complexity is acceptable for state lookup logic
func (s *IdentityProviderService) Unregister(ctx context.Context, providerID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"providerId", providerID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering AccessIdentityProvider from SyncState")

	// Try multiple possible SyncState IDs
	syncStateIDs := []string{
		providerID,
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypeAccessIdentityProvider, id)
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

		logger.Info("AccessIdentityProvider unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// UpdateProviderID updates the SyncState to use the actual provider ID
// after the provider is created.
func (s *IdentityProviderService) UpdateProviderID(ctx context.Context, source service.Source, providerID string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"providerId", providerID,
	)

	pendingID := fmt.Sprintf("pending-%s", source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceTypeAccessIdentityProvider, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		logger.V(1).Info("No pending SyncState found, AccessIdentityProvider may already use actual ID")
		return nil
	}

	// Create new SyncState with the actual ID
	newSyncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeAccessIdentityProvider,
		providerID,
		pendingSyncState.Spec.AccountID,
		pendingSyncState.Spec.ZoneID,
		pendingSyncState.Spec.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("create new syncstate with provider ID: %w", err)
	}

	// Copy sources from pending to new
	newSyncState.Spec.Sources = pendingSyncState.Spec.Sources
	newSyncState.Spec.CloudflareID = providerID
	if err := s.Client.Update(ctx, newSyncState); err != nil {
		return fmt.Errorf("update new syncstate with sources: %w", err)
	}

	// Delete the pending SyncState
	if err := s.Client.Delete(ctx, pendingSyncState); err != nil {
		logger.Error(err, "Failed to delete pending SyncState", "pendingId", pendingID)
	}

	logger.Info("Updated SyncState to use actual provider ID",
		"oldId", pendingID,
		"newId", providerID)
	return nil
}
