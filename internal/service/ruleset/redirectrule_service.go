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

// RedirectRuleService manages RedirectRule configurations via CloudflareSyncState.
type RedirectRuleService struct {
	*service.BaseService
}

// NewRedirectRuleService creates a new RedirectRule service.
func NewRedirectRuleService(c client.Client) *RedirectRuleService {
	return &RedirectRuleService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers a RedirectRule configuration with the SyncState.
func (s *RedirectRuleService) Register(ctx context.Context, opts RedirectRuleRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"zone", opts.Config.Zone,
		"source", opts.Source.String(),
	)
	logger.V(1).Info("Registering RedirectRule configuration")

	// Generate SyncState ID
	syncStateID := opts.RulesetID
	if syncStateID == "" {
		syncStateID = fmt.Sprintf("pending-%s", opts.Source.Name)
	}

	// Get or create SyncState
	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeRedirectRule,
		syncStateID,
		opts.AccountID,
		opts.ZoneID,
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get or create syncstate: %w", err)
	}

	// Update source in SyncState
	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityRedirectRule); err != nil {
		return fmt.Errorf("update source: %w", err)
	}

	logger.Info("RedirectRule configuration registered", "syncStateId", syncState.Name)
	return nil
}

// Unregister removes a configuration from the SyncState.
//
//nolint:revive // cognitive complexity is acceptable for state lookup logic
func (s *RedirectRuleService) Unregister(ctx context.Context, rulesetID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"rulesetId", rulesetID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering RedirectRule from SyncState")

	// Try multiple possible SyncState IDs
	syncStateIDs := []string{
		rulesetID,
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypeRedirectRule, id)
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

		logger.Info("RedirectRule unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// UpdateRulesetID updates the SyncState to use the actual ruleset ID
// after the ruleset is created.
func (s *RedirectRuleService) UpdateRulesetID(ctx context.Context, source service.Source, rulesetID, zoneID string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"rulesetId", rulesetID,
	)

	pendingID := fmt.Sprintf("pending-%s", source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceTypeRedirectRule, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		logger.V(1).Info("No pending SyncState found, RedirectRule may already use actual ID")
		return nil
	}

	// Create new SyncState with the actual ruleset ID
	newSyncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeRedirectRule,
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

// UpdateStatus updates the K8s RedirectRule resource status based on sync result.
func (s *RedirectRuleService) UpdateStatus(
	ctx context.Context,
	rule *v1alpha2.RedirectRule,
	result *RedirectRuleSyncResult,
) error {
	rule.Status.State = v1alpha2.RedirectRuleStateReady
	rule.Status.RulesetID = result.RulesetID
	rule.Status.ZoneID = result.ZoneID
	rule.Status.RuleCount = result.RuleCount
	rule.Status.ObservedGeneration = rule.Generation

	return s.Client.Status().Update(ctx, rule)
}
