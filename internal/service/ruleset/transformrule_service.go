// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package ruleset

//nolint:dupl // Similar patterns across resource types are intentional

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// TransformRuleService manages TransformRule configurations via CloudflareSyncState.
type TransformRuleService struct {
	*service.BaseService
}

// NewTransformRuleService creates a new TransformRule service.
func NewTransformRuleService(c client.Client) *TransformRuleService {
	return &TransformRuleService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers a TransformRule configuration with the SyncState.
func (s *TransformRuleService) Register(ctx context.Context, opts TransformRuleRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"zone", opts.Config.Zone,
		"type", opts.Config.Type,
		"source", opts.Source.String(),
	)
	logger.V(1).Info("Registering TransformRule configuration")

	// Generate SyncState ID
	syncStateID := opts.RulesetID
	if syncStateID == "" {
		syncStateID = fmt.Sprintf("pending-%s", opts.Source.Name)
	}

	// Get or create SyncState
	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeTransformRule,
		syncStateID,
		opts.AccountID,
		opts.ZoneID,
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get or create syncstate: %w", err)
	}

	// Update source in SyncState
	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityTransformRule); err != nil {
		return fmt.Errorf("update source: %w", err)
	}

	logger.Info("TransformRule configuration registered", "syncStateId", syncState.Name)
	return nil
}

// Unregister removes a configuration from the SyncState.
//
//nolint:revive // cognitive complexity is acceptable for state lookup logic
func (s *TransformRuleService) Unregister(ctx context.Context, rulesetID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"rulesetId", rulesetID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering TransformRule from SyncState")

	// Try multiple possible SyncState IDs
	syncStateIDs := []string{
		rulesetID,
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypeTransformRule, id)
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

		logger.Info("TransformRule unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// UpdateRulesetID updates the SyncState to use the actual ruleset ID
// after the ruleset is created.
func (s *TransformRuleService) UpdateRulesetID(ctx context.Context, source service.Source, rulesetID, zoneID string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"rulesetId", rulesetID,
	)

	pendingID := fmt.Sprintf("pending-%s", source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceTypeTransformRule, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		logger.V(1).Info("No pending SyncState found, TransformRule may already use actual ID")
		return nil
	}

	// Create new SyncState with the actual ruleset ID
	newSyncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeTransformRule,
		rulesetID,
		pendingSyncState.Spec.AccountID,
		zoneID,
		pendingSyncState.Spec.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("create new syncstate with ruleset ID: %w", err)
	}

	// Copy sources from pending to new
	newSyncState.Spec.Sources = pendingSyncState.Spec.Sources
	newSyncState.Spec.CloudflareID = rulesetID
	if err := s.Client.Update(ctx, newSyncState); err != nil {
		return fmt.Errorf("update new syncstate with sources: %w", err)
	}

	// Delete the pending SyncState
	if err := s.Client.Delete(ctx, pendingSyncState); err != nil {
		logger.Error(err, "Failed to delete pending SyncState", "pendingId", pendingID)
	}

	logger.Info("Updated SyncState to use actual ruleset ID",
		"oldId", pendingID,
		"newId", rulesetID)
	return nil
}

// UpdateStatus updates the K8s TransformRule resource status based on sync result.
func (s *TransformRuleService) UpdateStatus(
	ctx context.Context,
	rule *v1alpha2.TransformRule,
	result *TransformRuleSyncResult,
) error {
	rule.Status.State = v1alpha2.TransformRuleStateReady
	rule.Status.RulesetID = result.RulesetID
	rule.Status.ZoneID = result.ZoneID
	rule.Status.RuleCount = result.RuleCount
	rule.Status.ObservedGeneration = rule.Generation

	return s.Client.Status().Update(ctx, rule)
}
