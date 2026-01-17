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

// ZoneRulesetService manages ZoneRuleset configurations via CloudflareSyncState.
type ZoneRulesetService struct {
	*service.BaseService
}

// NewZoneRulesetService creates a new ZoneRuleset service.
func NewZoneRulesetService(c client.Client) *ZoneRulesetService {
	return &ZoneRulesetService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers a ZoneRuleset configuration with the SyncState.
func (s *ZoneRulesetService) Register(ctx context.Context, opts ZoneRulesetRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"zone", opts.Config.Zone,
		"phase", opts.Config.Phase,
		"source", opts.Source.String(),
	)
	logger.V(1).Info("Registering ZoneRuleset configuration")

	// Generate SyncState ID using zone+phase combination
	syncStateID := opts.RulesetID
	if syncStateID == "" {
		syncStateID = fmt.Sprintf("pending-%s", opts.Source.Name)
	}

	// Get or create SyncState
	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeZoneRuleset,
		syncStateID,
		opts.AccountID,
		opts.ZoneID,
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get or create syncstate: %w", err)
	}

	// Update source in SyncState
	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityZoneRuleset); err != nil {
		return fmt.Errorf("update source: %w", err)
	}

	logger.Info("ZoneRuleset configuration registered", "syncStateId", syncState.Name)
	return nil
}

// Unregister removes a configuration from the SyncState.
//
//nolint:revive // cognitive complexity is acceptable for state lookup logic
func (s *ZoneRulesetService) Unregister(ctx context.Context, rulesetID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"rulesetId", rulesetID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering ZoneRuleset from SyncState")

	// Try multiple possible SyncState IDs
	syncStateIDs := []string{
		rulesetID,
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypeZoneRuleset, id)
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

		logger.Info("ZoneRuleset unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// UpdateRulesetID updates the SyncState to use the actual ruleset ID
// after the ruleset is created.
func (s *ZoneRulesetService) UpdateRulesetID(ctx context.Context, source service.Source, rulesetID, zoneID string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"rulesetId", rulesetID,
	)

	pendingID := fmt.Sprintf("pending-%s", source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceTypeZoneRuleset, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		logger.V(1).Info("No pending SyncState found, ZoneRuleset may already use actual ID")
		return nil
	}

	// Create new SyncState with the actual ruleset ID
	newSyncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeZoneRuleset,
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

// UpdateStatus updates the K8s ZoneRuleset resource status based on sync result.
func (s *ZoneRulesetService) UpdateStatus(
	ctx context.Context,
	ruleset *v1alpha2.ZoneRuleset,
	result *ZoneRulesetSyncResult,
) error {
	ruleset.Status.State = v1alpha2.ZoneRulesetStateReady
	ruleset.Status.RulesetID = result.RulesetID
	ruleset.Status.RulesetVersion = result.RulesetVersion
	ruleset.Status.ZoneID = result.ZoneID
	ruleset.Status.RuleCount = result.RuleCount
	ruleset.Status.ObservedGeneration = ruleset.Generation

	return s.Client.Status().Update(ctx, ruleset)
}
