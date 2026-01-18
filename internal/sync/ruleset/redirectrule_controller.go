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
	// Phase for dynamic redirects
	redirectPhase = "http_request_dynamic_redirect"

	// RedirectRuleFinalizerName is the finalizer for Redirect Rule SyncState resources.
	RedirectRuleFinalizerName = "redirectrule.sync.cloudflare-operator.io/finalizer"
)

// RedirectRuleController is the Sync Controller for Redirect Rule Configuration.
// It watches CloudflareSyncState resources of type RedirectRule,
// extracts the configuration, and syncs to Cloudflare API.
type RedirectRuleController struct {
	*common.BaseSyncController
}

// NewRedirectRuleController creates a new RedirectRuleSyncController
func NewRedirectRuleController(c client.Client) *RedirectRuleController {
	return &RedirectRuleController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for redirect rule.
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *RedirectRuleController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "RedirectRuleSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process RedirectRule type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceRedirectRule {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing RedirectRule SyncState",
		"cloudflareId", syncState.Spec.CloudflareID,
		"sources", len(syncState.Spec.Sources))

	// Handle deletion - this is the SINGLE point for Cloudflare API delete calls
	if !syncState.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, syncState)
	}

	// Check if there are any sources - if none, clear redirect rules from Cloudflare
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, clearing redirect rules from Cloudflare")
		return r.handleDeletion(ctx, syncState)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(syncState, RedirectRuleFinalizerName) {
		controllerutil.AddFinalizer(syncState, RedirectRuleFinalizerName)
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

	// Extract redirect rule configuration from first source (1:1 mapping)
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract redirect rule configuration")
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
		logger.Error(err, "Failed to sync redirect rule to Cloudflare")
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

	logger.Info("Successfully synced redirect rule to Cloudflare",
		"rulesetId", result.RulesetID,
		"ruleCount", result.RuleCount)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts redirect rule configuration from SyncState sources.
// Redirect rules have 1:1 mapping, so we use the ExtractFirstSourceConfig helper.
func (*RedirectRuleController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*rulesetsvc.RedirectRuleConfig, error) {
	return common.ExtractFirstSourceConfig[rulesetsvc.RedirectRuleConfig](syncState)
}

// syncToCloudflare syncs the redirect rule configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *RedirectRuleController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *rulesetsvc.RedirectRuleConfig,
) (*rulesetsvc.RedirectRuleSyncResult, error) {
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

	// Convert rules to Cloudflare format
	rules := r.convertRules(config)

	logger.Info("Updating redirect ruleset",
		"zoneId", zoneID,
		"ruleCount", len(rules))

	// Update entrypoint ruleset
	result, err := apiClient.UpdateEntrypointRuleset(
		ctx,
		zoneID,
		redirectPhase,
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

	return &rulesetsvc.RedirectRuleSyncResult{
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
func (*RedirectRuleController) convertRules(config *rulesetsvc.RedirectRuleConfig) []cloudflare.RulesetRule {
	totalRules := len(config.Rules) + len(config.WildcardRules)
	rules := make([]cloudflare.RulesetRule, 0, totalRules)

	// Convert expression-based rules
	for _, rule := range config.Rules {
		cfRule := cloudflare.RulesetRule{
			Action:      "redirect",
			Expression:  rule.Expression,
			Description: rule.Name,
			Enabled:     ptr.To(rule.Enabled),
			ActionParameters: &cloudflare.RulesetRuleActionParameters{
				FromValue: &cloudflare.RulesetRuleActionParametersFromValue{
					PreserveQueryString: ptr.To(rule.PreserveQueryString),
				},
			},
		}

		// Set status code
		if rule.StatusCode > 0 {
			cfRule.ActionParameters.FromValue.StatusCode = uint16(rule.StatusCode)
		} else {
			cfRule.ActionParameters.FromValue.StatusCode = 302
		}

		// Set target URL
		cfRule.ActionParameters.FromValue.TargetURL = cloudflare.RulesetRuleActionParametersTargetURL{}
		if rule.Target.URL != "" {
			cfRule.ActionParameters.FromValue.TargetURL.Value = rule.Target.URL
		}
		if rule.Target.Expression != "" {
			cfRule.ActionParameters.FromValue.TargetURL.Expression = rule.Target.Expression
		}

		rules = append(rules, cfRule)
	}

	// Convert wildcard-based rules
	for _, rule := range config.WildcardRules {
		// Build the expression from wildcard pattern
		expression := fmt.Sprintf(`http.request.full_uri wildcard "%s"`, rule.SourceURL)

		cfRule := cloudflare.RulesetRule{
			Action:      "redirect",
			Expression:  expression,
			Description: rule.Name,
			Enabled:     ptr.To(rule.Enabled),
			ActionParameters: &cloudflare.RulesetRuleActionParameters{
				FromValue: &cloudflare.RulesetRuleActionParametersFromValue{
					PreserveQueryString: ptr.To(rule.PreserveQueryString),
					TargetURL: cloudflare.RulesetRuleActionParametersTargetURL{
						Value: rule.TargetURL,
					},
				},
			},
		}

		// Set status code
		if rule.StatusCode > 0 {
			cfRule.ActionParameters.FromValue.StatusCode = uint16(rule.StatusCode)
		} else {
			cfRule.ActionParameters.FromValue.StatusCode = 301
		}

		rules = append(rules, cfRule)
	}

	return rules
}

// handleDeletion handles the deletion of Redirect Rule from Cloudflare.
// This clears the redirect rules by updating with an empty rules array.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller deletes from Cloudflare
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps
func (r *RedirectRuleController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, RedirectRuleFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Clear redirect rules from Cloudflare
	zoneID := syncState.Spec.ZoneID
	if zoneID != "" {
		apiClient, apiErr := common.CreateAPIClient(ctx, r.Client, syncState)
		if apiErr != nil {
			logger.Error(apiErr, "Failed to create API client for deletion")
			return ctrl.Result{RequeueAfter: common.RequeueAfterError(apiErr)}, nil
		}

		logger.Info("Clearing Redirect Rule from Cloudflare",
			"zoneId", zoneID,
			"phase", redirectPhase)

		// Update with empty rules to clear the ruleset
		_, clearErr := apiClient.UpdateEntrypointRuleset(ctx, zoneID, redirectPhase, "", []cloudflare.RulesetRule{})
		if clearErr != nil {
			if !cf.IsNotFoundError(clearErr) {
				logger.Error(clearErr, "Failed to clear Redirect Rule")
				if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, clearErr); statusErr != nil {
					logger.Error(statusErr, "Failed to update error status")
				}
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(clearErr)}, nil
			}
			logger.Info("Redirect ruleset already cleared or not found")
		} else {
			logger.Info("Successfully cleared Redirect Rule from Cloudflare")
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(syncState, RedirectRuleFinalizerName)
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
func (r *RedirectRuleController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("redirect-rule-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourceRedirectRule)).
		Complete(r)
}
