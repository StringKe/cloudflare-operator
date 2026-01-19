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

// PolicyService handles reusable AccessPolicy configuration registration.
type PolicyService struct {
	*service.BaseService
}

// NewPolicyService creates a new reusable AccessPolicy service.
func NewPolicyService(c client.Client) *PolicyService {
	return &PolicyService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers a reusable AccessPolicy configuration to SyncState.
func (s *PolicyService) Register(ctx context.Context, opts ReusableAccessPolicyRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"accountId", opts.AccountID,
		"source", opts.Source.String(),
		"name", opts.Config.Name,
	)
	logger.V(1).Info("Registering reusable AccessPolicy configuration")

	// Generate SyncState ID:
	// - If PolicyID is known (existing policy), use it
	// - Otherwise, use a placeholder based on source
	syncStateID := opts.PolicyID
	if syncStateID == "" {
		syncStateID = fmt.Sprintf("pending-%s", opts.Source.Name)
	}

	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeAccessPolicy,
		syncStateID,
		opts.AccountID,
		"", // AccessPolicy doesn't use zone ID
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get/create syncstate for AccessPolicy: %w", err)
	}

	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityAccessPolicy); err != nil {
		return fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("AccessPolicy configuration registered successfully",
		"syncState", syncState.Name)
	return nil
}

// Unregister removes a configuration from the SyncState.
//
//nolint:revive // cognitive complexity is acceptable for state lookup logic
func (s *PolicyService) Unregister(ctx context.Context, policyID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"policyId", policyID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering AccessPolicy from SyncState")

	// Try multiple possible SyncState IDs
	syncStateIDs := []string{
		policyID,
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypeAccessPolicy, id)
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

		logger.Info("AccessPolicy unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// PolicySyncStatus represents the sync status of a reusable AccessPolicy
type PolicySyncStatus struct {
	IsSynced    bool
	PolicyID    string
	AccountID   string
	SyncStateID string
}

// GetSyncStatus returns the sync status for an AccessPolicy.
//
//nolint:revive // cognitive complexity acceptable for status lookup logic
func (s *PolicyService) GetSyncStatus(ctx context.Context, source service.Source, knownPolicyID string) (*PolicySyncStatus, error) {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"knownPolicyId", knownPolicyID,
	)

	// Try both possible SyncState IDs
	syncStateIDs := []string{
		knownPolicyID,
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypeAccessPolicy, id)
		if err != nil {
			logger.V(1).Info("Error getting SyncState", "syncStateId", id, "error", err)
			continue
		}
		if syncState == nil {
			continue
		}

		// Check if synced
		isSynced := syncState.Status.SyncStatus == v1alpha2.SyncStatusSynced
		policyID := syncState.Spec.CloudflareID

		// If the CloudflareID starts with "pending-", it's not a real ID
		if strings.HasPrefix(policyID, "pending-") {
			policyID = ""
		}

		return &PolicySyncStatus{
			IsSynced:    isSynced,
			PolicyID:    policyID,
			AccountID:   syncState.Spec.AccountID,
			SyncStateID: syncState.Name,
		}, nil
	}

	return nil, nil
}

// UpdatePolicyID updates the SyncState to use the actual policy ID
// after the policy is created.
func (s *PolicyService) UpdatePolicyID(ctx context.Context, source service.Source, policyID string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"policyId", policyID,
	)

	pendingID := fmt.Sprintf("pending-%s", source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceTypeAccessPolicy, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		logger.V(1).Info("No pending SyncState found, AccessPolicy may already use actual ID")
		return nil
	}

	// Create new SyncState with the actual ID
	newSyncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeAccessPolicy,
		policyID,
		pendingSyncState.Spec.AccountID,
		pendingSyncState.Spec.ZoneID,
		pendingSyncState.Spec.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("create new syncstate with policy ID: %w", err)
	}

	// Copy sources from pending to new
	newSyncState.Spec.Sources = pendingSyncState.Spec.Sources
	newSyncState.Spec.CloudflareID = policyID
	if err := s.Client.Update(ctx, newSyncState); err != nil {
		return fmt.Errorf("update new syncstate with sources: %w", err)
	}

	// Delete the pending SyncState
	if err := s.Client.Delete(ctx, pendingSyncState); err != nil {
		logger.Error(err, "Failed to delete pending SyncState", "pendingId", pendingID)
	}

	logger.Info("Updated SyncState to use actual policy ID",
		"oldId", pendingID,
		"newId", policyID)
	return nil
}
