// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package ruleset

//nolint:dupl // Similar patterns across resource types are intentional

import (
	"context"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	rulesetsvc "github.com/StringKe/cloudflare-operator/internal/service/ruleset"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

const (
	// TransformRuleFinalizerName is the finalizer for Transform Rule SyncState resources.
	TransformRuleFinalizerName = "transformrule.sync.cloudflare-operator.io/finalizer"
)

// TransformRuleController is the Sync Controller for Transform Rule Configuration.
// It watches CloudflareSyncState resources of type TransformRule,
// extracts the configuration, and syncs to Cloudflare API.
type TransformRuleController struct {
	*common.BaseSyncController
}

// NewTransformRuleController creates a new TransformRuleSyncController
func NewTransformRuleController(c client.Client) *TransformRuleController {
	return &TransformRuleController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for transform rule.
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *TransformRuleController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "TransformRuleSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process TransformRule type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceTransformRule {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing TransformRule SyncState",
		"cloudflareId", syncState.Spec.CloudflareID,
		"sources", len(syncState.Spec.Sources))

	// Handle deletion - this is the SINGLE point for Cloudflare API delete calls
	if !syncState.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, syncState)
	}

	// Check if there are any sources - if none, clear rules from Cloudflare
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, clearing transform rules from Cloudflare")
		return r.handleDeletion(ctx, syncState)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(syncState, TransformRuleFinalizerName) {
		controllerutil.AddFinalizer(syncState, TransformRuleFinalizerName)
		if err := r.Client.Update(ctx, syncState); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip if there's a pending debounced request
	if r.Debouncer.IsPending(req.Name) {
		logger.V(1).Info("Skipping reconcile - debounced request pending")
		return ctrl.Result{}, nil
	}

	// Extract transform rule configuration from first source (1:1 mapping)
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract transform rule configuration")
		if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
	}

	// Compute hash for change detection
	newHash, hashErr := common.ComputeConfigHash(config)
	if hashErr != nil {
		logger.Error(hashErr, "Failed to compute config hash")
		newHash = "" // Force sync if hash fails
	}

	if !r.ShouldSync(syncState, newHash) {
		logger.V(1).Info("Configuration unchanged, skipping sync", "hash", newHash)
		return ctrl.Result{}, nil
	}

	// Set syncing status
	if err := r.SetSyncStatus(ctx, syncState, v1alpha2.SyncStatusSyncing); err != nil {
		return ctrl.Result{}, err
	}

	// Sync to Cloudflare API
	result, err := r.syncToCloudflare(ctx, syncState, config)
	if err != nil {
		logger.Error(err, "Failed to sync transform rule to Cloudflare")
		if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
	}

	// Update success status
	syncResult := &common.SyncResult{
		ConfigHash: newHash,
	}
	if err := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusSynced, syncResult, nil); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Successfully synced transform rule to Cloudflare",
		"rulesetId", result.RulesetID,
		"ruleCount", result.RuleCount)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts transform rule configuration from SyncState sources.
// Transform rules have 1:1 mapping, so we use the ExtractFirstSourceConfig helper.
func (*TransformRuleController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*rulesetsvc.TransformRuleConfig, error) {
	return common.ExtractFirstSourceConfig[rulesetsvc.TransformRuleConfig](syncState)
}

// getPhase returns the Cloudflare ruleset phase based on transform rule type.
func (*TransformRuleController) getPhase(ruleType string) string {
	switch ruleType {
	case "request_header":
		return "http_request_late_transform"
	case "response_header":
		return "http_response_headers_transform"
	default: // includes "url_rewrite"
		return "http_request_transform"
	}
}

// syncToCloudflare syncs the transform rule configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *TransformRuleController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *rulesetsvc.TransformRuleConfig,
) (*rulesetsvc.TransformRuleSyncResult, error) {
	logger := log.FromContext(ctx)

	// Create API client using common helper
	apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
	if err != nil {
		return nil, err
	}

	// Validate zone ID is present
	zoneID, err := common.RequireZoneID(syncState)
	if err != nil {
		return nil, err
	}

	// Get the appropriate phase
	phase := r.getPhase(config.Type)

	// Convert rules to Cloudflare format
	rules := r.convertRules(config.Rules)

	logger.Info("Updating transform ruleset",
		"zoneId", zoneID,
		"phase", phase,
		"ruleCount", len(rules))

	// Update entrypoint ruleset
	result, err := apiClient.UpdateEntrypointRuleset(
		ctx,
		zoneID,
		phase,
		config.Description,
		rules,
	)
	if err != nil {
		return nil, fmt.Errorf("update entrypoint ruleset: %w", err)
	}

	// Update SyncState with actual ruleset ID if it was pending
	if common.IsPendingID(syncState.Spec.CloudflareID) && result.ID != "" {
		common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID)
	}

	return &rulesetsvc.TransformRuleSyncResult{
		SyncResult: rulesetsvc.SyncResult{
			ID:        result.ID,
			AccountID: syncState.Spec.AccountID,
		},
		RulesetID: result.ID,
		ZoneID:    zoneID,
		RuleCount: len(result.Rules),
	}, nil
}

// convertRules converts service config rules to Cloudflare RulesetRule format.
//
//nolint:revive // cognitive complexity is acceptable for rule conversion
func (*TransformRuleController) convertRules(rules []rulesetsvc.TransformRuleDefinitionConfig) []cloudflare.RulesetRule {
	cfRules := make([]cloudflare.RulesetRule, len(rules))

	for i, rule := range rules {
		cfRule := cloudflare.RulesetRule{
			Action:      "rewrite",
			Expression:  rule.Expression,
			Description: rule.Name,
			Enabled:     ptr.To(rule.Enabled),
		}

		// Build action parameters
		params := &cloudflare.RulesetRuleActionParameters{}

		// URL Rewrite
		if rule.URLRewrite != nil {
			params.URI = &cloudflare.RulesetRuleActionParametersURI{}

			if rule.URLRewrite.Path != nil {
				params.URI.Path = &cloudflare.RulesetRuleActionParametersURIPath{}
				if rule.URLRewrite.Path.Static != "" {
					params.URI.Path.Value = rule.URLRewrite.Path.Static
				}
				if rule.URLRewrite.Path.Expression != "" {
					params.URI.Path.Expression = rule.URLRewrite.Path.Expression
				}
			}

			if rule.URLRewrite.Query != nil {
				params.URI.Query = &cloudflare.RulesetRuleActionParametersURIQuery{}
				if rule.URLRewrite.Query.Static != "" {
					params.URI.Query.Value = ptr.To(rule.URLRewrite.Query.Static)
				}
				if rule.URLRewrite.Query.Expression != "" {
					params.URI.Query.Expression = rule.URLRewrite.Query.Expression
				}
			}
		}

		// Header modifications
		if len(rule.Headers) > 0 {
			params.Headers = make(map[string]cloudflare.RulesetRuleActionParametersHTTPHeader)

			for _, header := range rule.Headers {
				h := cloudflare.RulesetRuleActionParametersHTTPHeader{
					Operation: string(header.Operation),
				}
				if header.Value != "" {
					h.Value = header.Value
				}
				if header.Expression != "" {
					h.Expression = header.Expression
				}
				params.Headers[header.Name] = h
			}
		}

		cfRule.ActionParameters = params
		cfRules[i] = cfRule
	}

	return cfRules
}

// handleDeletion handles the deletion of Transform Rule from Cloudflare.
// This clears the transform rules by updating with an empty rules array.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller deletes from Cloudflare
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps
func (r *TransformRuleController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, TransformRuleFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Try to extract config to get the type (which determines phase) for clearing rules
	config, err := r.extractConfig(syncState)
	if err == nil && config != nil && config.Type != "" {
		// We have config with type, clear the transform rules
		apiClient, apiErr := common.CreateAPIClient(ctx, r.Client, syncState)
		if apiErr != nil {
			logger.Error(apiErr, "Failed to create API client for deletion")
			return ctrl.Result{RequeueAfter: common.RequeueAfterError(apiErr)}, nil
		}

		zoneID := syncState.Spec.ZoneID
		if zoneID != "" {
			phase := r.getPhase(config.Type)
			logger.Info("Clearing Transform Rule from Cloudflare",
				"zoneId", zoneID,
				"phase", phase)

			// Update with empty rules to clear the ruleset
			_, clearErr := apiClient.UpdateEntrypointRuleset(ctx, zoneID, phase, "", []cloudflare.RulesetRule{})
			if clearErr != nil {
				if !cf.IsNotFoundError(clearErr) {
					logger.Error(clearErr, "Failed to clear Transform Rule")
					if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, clearErr); statusErr != nil {
						logger.Error(statusErr, "Failed to update error status")
					}
					return ctrl.Result{RequeueAfter: common.RequeueAfterError(clearErr)}, nil
				}
				logger.Info("Transform ruleset already cleared or not found")
			} else {
				logger.Info("Successfully cleared Transform Rule from Cloudflare")
			}
		}
	} else {
		// No config available - just log and clean up SyncState
		logger.Info("No config available for Transform Rule deletion, cleaning up SyncState only")
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(syncState, TransformRuleFinalizerName)
	if err := r.Client.Update(ctx, syncState); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	// If sources are empty (not a deletion timestamp trigger), delete the SyncState itself
	if syncState.DeletionTimestamp.IsZero() && len(syncState.Spec.Sources) == 0 {
		logger.Info("Deleting orphaned SyncState")
		if err := r.Client.Delete(ctx, syncState); err != nil {
			if client.IgnoreNotFound(err) != nil {
				logger.Error(err, "Failed to delete SyncState")
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TransformRuleController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("transform-rule-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourceTransformRule)).
		Complete(r)
}
