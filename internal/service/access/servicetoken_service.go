// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package access

//nolint:dupl // Similar patterns across resource types are intentional

import (
	"context"
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// ServiceTokenService handles AccessServiceToken configuration registration.
type ServiceTokenService struct {
	*service.BaseService
}

// NewServiceTokenService creates a new AccessServiceToken service.
func NewServiceTokenService(c client.Client) *ServiceTokenService {
	return &ServiceTokenService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers an AccessServiceToken configuration to SyncState.
//
//nolint:revive // cognitive complexity is acceptable for SyncState lookup logic
func (s *ServiceTokenService) Register(ctx context.Context, opts AccessServiceTokenRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"accountId", opts.AccountID,
		"source", opts.Source.String(),
		"name", opts.Config.Name,
	)
	logger.V(1).Info("Registering AccessServiceToken configuration")

	// Try to find existing SyncState in this order:
	// 1. By pending-{source.Name} (for resources being created)
	// 2. By TokenID (for resources that were already synced)
	// This prevents creating duplicate SyncStates when TokenID changes
	var syncState *v1alpha2.CloudflareSyncState
	var err error

	// First, try to find by pending ID (primary lookup for resources in creation)
	pendingID := fmt.Sprintf("pending-%s", opts.Source.Name)
	syncState, err = s.GetSyncState(ctx, ResourceTypeAccessServiceToken, pendingID)
	if err != nil {
		return fmt.Errorf("lookup syncstate by pending ID: %w", err)
	}

	// If not found by pending ID and TokenID is known, try by TokenID
	if syncState == nil && opts.TokenID != "" {
		syncState, err = s.GetSyncState(ctx, ResourceTypeAccessServiceToken, opts.TokenID)
		if err != nil {
			return fmt.Errorf("lookup syncstate by token ID: %w", err)
		}
	}

	// If still not found, create a new SyncState
	if syncState == nil {
		syncStateID := pendingID
		if opts.TokenID != "" {
			syncStateID = opts.TokenID
		}
		syncState, err = s.GetOrCreateSyncState(
			ctx,
			ResourceTypeAccessServiceToken,
			syncStateID,
			opts.AccountID,
			"", // AccessServiceToken doesn't use zone ID
			opts.CredentialsRef,
		)
		if err != nil {
			return fmt.Errorf("get/create syncstate for AccessServiceToken: %w", err)
		}
	}

	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityAccessServiceToken); err != nil {
		return fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("AccessServiceToken configuration registered successfully",
		"syncState", syncState.Name)
	return nil
}

// Unregister removes a configuration from the SyncState.
//
//nolint:revive // cognitive complexity is acceptable for state lookup logic
func (s *ServiceTokenService) Unregister(ctx context.Context, tokenID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"tokenId", tokenID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering AccessServiceToken from SyncState")

	// Try multiple possible SyncState IDs
	syncStateIDs := []string{
		tokenID,
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypeAccessServiceToken, id)
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

		logger.Info("AccessServiceToken unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// ServiceTokenSyncStatus represents the sync status of an AccessServiceToken.
type ServiceTokenSyncStatus struct {
	IsSynced    bool
	TokenID     string
	AccountID   string
	SyncStateID string
}

// GetSyncStatus returns the sync status for an AccessServiceToken.
//
//nolint:revive // cognitive complexity acceptable for status lookup logic
func (s *ServiceTokenService) GetSyncStatus(ctx context.Context, source service.Source, knownTokenID string) (*ServiceTokenSyncStatus, error) {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"knownTokenId", knownTokenID,
	)

	// Try both possible SyncState IDs
	syncStateIDs := []string{
		knownTokenID,
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypeAccessServiceToken, id)
		if err != nil {
			logger.V(1).Info("Error getting SyncState", "syncStateId", id, "error", err)
			continue
		}
		if syncState == nil {
			continue
		}

		// Check if synced
		isSynced := syncState.Status.SyncStatus == v1alpha2.SyncStatusSynced
		tokenID := syncState.Spec.CloudflareID

		// If the CloudflareID starts with "pending-", it's not a real ID
		if strings.HasPrefix(tokenID, "pending-") {
			tokenID = ""
		}

		return &ServiceTokenSyncStatus{
			IsSynced:    isSynced,
			TokenID:     tokenID,
			AccountID:   syncState.Spec.AccountID,
			SyncStateID: syncState.Name,
		}, nil
	}

	return nil, nil
}

// UpdateTokenID updates the SyncState to use the actual token ID
// after the token is created.
func (s *ServiceTokenService) UpdateTokenID(ctx context.Context, source service.Source, tokenID string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"tokenId", tokenID,
	)

	pendingID := fmt.Sprintf("pending-%s", source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceTypeAccessServiceToken, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		logger.V(1).Info("No pending SyncState found, AccessServiceToken may already use actual ID")
		return nil
	}

	// Create new SyncState with the actual ID
	newSyncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeAccessServiceToken,
		tokenID,
		pendingSyncState.Spec.AccountID,
		pendingSyncState.Spec.ZoneID,
		pendingSyncState.Spec.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("create new syncstate with token ID: %w", err)
	}

	// Copy sources from pending to new
	newSyncState.Spec.Sources = pendingSyncState.Spec.Sources
	newSyncState.Spec.CloudflareID = tokenID
	if err := s.Client.Update(ctx, newSyncState); err != nil {
		return fmt.Errorf("update new syncstate with sources: %w", err)
	}

	// Delete the pending SyncState
	if err := s.Client.Delete(ctx, pendingSyncState); err != nil {
		logger.Error(err, "Failed to delete pending SyncState", "pendingId", pendingID)
	}

	logger.Info("Updated SyncState to use actual token ID",
		"oldId", pendingID,
		"newId", tokenID)
	return nil
}
