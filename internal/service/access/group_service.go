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

// GroupService handles AccessGroup configuration registration.
type GroupService struct {
	*service.BaseService
}

// NewGroupService creates a new AccessGroup service.
func NewGroupService(c client.Client) *GroupService {
	return &GroupService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers an AccessGroup configuration to SyncState.
func (s *GroupService) Register(ctx context.Context, opts AccessGroupRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"accountId", opts.AccountID,
		"source", opts.Source.String(),
		"name", opts.Config.Name,
	)
	logger.V(1).Info("Registering AccessGroup configuration")

	// Generate SyncState ID:
	// - If GroupID is known (existing group), use it
	// - Otherwise, use a placeholder based on source
	syncStateID := opts.GroupID
	if syncStateID == "" {
		syncStateID = fmt.Sprintf("pending-%s", opts.Source.Name)
	}

	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeAccessGroup,
		syncStateID,
		opts.AccountID,
		"", // AccessGroup doesn't use zone ID
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get/create syncstate for AccessGroup: %w", err)
	}

	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityAccessGroup); err != nil {
		return fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("AccessGroup configuration registered successfully",
		"syncState", syncState.Name)
	return nil
}

// Unregister removes a configuration from the SyncState.
//
//nolint:revive // cognitive complexity is acceptable for state lookup logic
func (s *GroupService) Unregister(ctx context.Context, groupID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"groupId", groupID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering AccessGroup from SyncState")

	// Try multiple possible SyncState IDs
	syncStateIDs := []string{
		groupID,
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypeAccessGroup, id)
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

		logger.Info("AccessGroup unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// UpdateGroupID updates the SyncState to use the actual group ID
// after the group is created.
func (s *GroupService) UpdateGroupID(ctx context.Context, source service.Source, groupID string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"groupId", groupID,
	)

	pendingID := fmt.Sprintf("pending-%s", source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceTypeAccessGroup, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		logger.V(1).Info("No pending SyncState found, AccessGroup may already use actual ID")
		return nil
	}

	// Create new SyncState with the actual ID
	newSyncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeAccessGroup,
		groupID,
		pendingSyncState.Spec.AccountID,
		pendingSyncState.Spec.ZoneID,
		pendingSyncState.Spec.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("create new syncstate with group ID: %w", err)
	}

	// Copy sources from pending to new
	newSyncState.Spec.Sources = pendingSyncState.Spec.Sources
	newSyncState.Spec.CloudflareID = groupID
	if err := s.Client.Update(ctx, newSyncState); err != nil {
		return fmt.Errorf("update new syncstate with sources: %w", err)
	}

	// Delete the pending SyncState
	if err := s.Client.Delete(ctx, pendingSyncState); err != nil {
		logger.Error(err, "Failed to delete pending SyncState", "pendingId", pendingID)
	}

	logger.Info("Updated SyncState to use actual group ID",
		"oldId", pendingID,
		"newId", groupID)
	return nil
}
