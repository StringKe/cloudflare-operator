// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package device

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// DevicePostureRuleService manages DevicePostureRule configurations via CloudflareSyncState.
type DevicePostureRuleService struct {
	*service.BaseService
}

// NewDevicePostureRuleService creates a new DevicePostureRule service.
func NewDevicePostureRuleService(c client.Client) *DevicePostureRuleService {
	return &DevicePostureRuleService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers a DevicePostureRule configuration with the SyncState.
func (s *DevicePostureRuleService) Register(ctx context.Context, opts DevicePostureRuleRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"ruleName", opts.Config.Name,
		"ruleType", opts.Config.Type,
		"source", opts.Source.String(),
	)
	logger.V(1).Info("Registering DevicePostureRule configuration")

	// Generate SyncState ID
	syncStateID := opts.RuleID
	if syncStateID == "" {
		syncStateID = fmt.Sprintf("pending-%s", opts.Source.Name)
	}

	// Get or create SyncState
	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeDevicePostureRule,
		syncStateID,
		opts.AccountID,
		"", // No zone ID for Device resources
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get or create syncstate: %w", err)
	}

	// Update source in SyncState
	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityDevicePostureRule); err != nil {
		return fmt.Errorf("update source: %w", err)
	}

	logger.Info("DevicePostureRule configuration registered", "syncStateId", syncState.Name)
	return nil
}

// Unregister removes a DevicePostureRule configuration from the SyncState.
//
//nolint:revive // cognitive complexity is acceptable for state lookup logic
func (s *DevicePostureRuleService) Unregister(ctx context.Context, ruleID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"ruleId", ruleID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering DevicePostureRule from SyncState")

	// Try multiple possible SyncState IDs
	syncStateIDs := []string{
		ruleID,
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypeDevicePostureRule, id)
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

		logger.Info("DevicePostureRule unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// UpdateRuleID updates the SyncState to use the actual rule ID.
func (s *DevicePostureRuleService) UpdateRuleID(ctx context.Context, source service.Source, ruleID, accountID string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"ruleId", ruleID,
	)

	pendingID := fmt.Sprintf("pending-%s", source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceTypeDevicePostureRule, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		logger.V(1).Info("No pending SyncState found, DevicePostureRule may already use actual ID")
		return nil
	}

	// Create new SyncState with the actual rule ID
	newSyncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeDevicePostureRule,
		ruleID,
		accountID,
		"",
		pendingSyncState.Spec.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("create new syncstate with rule ID: %w", err)
	}

	// Copy sources from pending to new
	newSyncState.Spec.Sources = pendingSyncState.Spec.Sources
	newSyncState.Spec.CloudflareID = ruleID
	if err := s.Client.Update(ctx, newSyncState); err != nil {
		return fmt.Errorf("update new syncstate with sources: %w", err)
	}

	// Delete the pending SyncState
	if err := s.Client.Delete(ctx, pendingSyncState); err != nil {
		logger.Error(err, "Failed to delete pending SyncState", "pendingId", pendingID)
	}

	logger.Info("Updated SyncState to use actual rule ID",
		"oldId", pendingID,
		"newId", ruleID)
	return nil
}

// UpdateStatus updates the K8s DevicePostureRule resource status based on sync result.
func (s *DevicePostureRuleService) UpdateStatus(
	ctx context.Context,
	rule *v1alpha2.DevicePostureRule,
	result *DevicePostureRuleSyncResult,
) error {
	rule.Status.State = service.StateReady
	rule.Status.RuleID = result.RuleID
	rule.Status.AccountID = result.AccountID
	rule.Status.ObservedGeneration = rule.Generation

	return s.Client.Status().Update(ctx, rule)
}
