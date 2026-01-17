// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package ruleset provides sync controllers for managing Cloudflare Ruleset resources.
//
//nolint:dupl // Similar patterns across resource types are intentional
package ruleset

import (
	"context"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	rulesetsvc "github.com/StringKe/cloudflare-operator/internal/service/ruleset"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

// ZoneRulesetController is the Sync Controller for Zone Ruleset Configuration.
// It watches CloudflareSyncState resources of type ZoneRuleset,
// extracts the configuration, and syncs to Cloudflare API.
type ZoneRulesetController struct {
	*common.BaseSyncController
}

// NewZoneRulesetController creates a new ZoneRulesetSyncController
func NewZoneRulesetController(c client.Client) *ZoneRulesetController {
	return &ZoneRulesetController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for zone ruleset.
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *ZoneRulesetController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "ZoneRulesetSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process ZoneRuleset type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceZoneRuleset {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing ZoneRuleset SyncState",
		"cloudflareId", syncState.Spec.CloudflareID,
		"sources", len(syncState.Spec.Sources))

	// Skip if there's a pending debounced request
	if r.Debouncer.IsPending(req.Name) {
		logger.V(1).Info("Skipping reconcile - debounced request pending")
		return ctrl.Result{}, nil
	}

	// Check if there are any sources
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, marking as synced (no-op)")
		if err := r.SetSyncStatus(ctx, syncState, v1alpha2.SyncStatusSynced); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Extract zone ruleset configuration from first source (1:1 mapping)
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract zone ruleset configuration")
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
		logger.Error(err, "Failed to sync zone ruleset to Cloudflare")
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

	logger.Info("Successfully synced zone ruleset to Cloudflare",
		"rulesetId", result.RulesetID,
		"ruleCount", result.RuleCount)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts zone ruleset configuration from SyncState sources.
// Zone rulesets have 1:1 mapping, so we use the ExtractFirstSourceConfig helper.
func (*ZoneRulesetController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*rulesetsvc.ZoneRulesetConfig, error) {
	return common.ExtractFirstSourceConfig[rulesetsvc.ZoneRulesetConfig](syncState)
}

// syncToCloudflare syncs the zone ruleset configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *ZoneRulesetController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *rulesetsvc.ZoneRulesetConfig,
) (*rulesetsvc.ZoneRulesetSyncResult, error) {
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
	rules := r.convertRules(config.Rules)

	logger.Info("Updating zone ruleset",
		"zoneId", zoneID,
		"phase", config.Phase,
		"ruleCount", len(rules))

	// Update entrypoint ruleset
	result, err := apiClient.UpdateEntrypointRuleset(
		ctx,
		zoneID,
		config.Phase,
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

	return &rulesetsvc.ZoneRulesetSyncResult{
		SyncResult: rulesetsvc.SyncResult{
			ID:        result.ID,
			AccountID: syncState.Spec.AccountID,
		},
		RulesetID:      result.ID,
		RulesetVersion: result.Version,
		ZoneID:         zoneID,
		RuleCount:      len(result.Rules),
	}, nil
}

// convertRules converts service config rules to Cloudflare RulesetRule format.
//
//nolint:revive // cognitive complexity is acceptable for rule conversion
func (r *ZoneRulesetController) convertRules(rules []rulesetsvc.RulesetRuleConfig) []cloudflare.RulesetRule {
	cfRules := make([]cloudflare.RulesetRule, len(rules))

	for i, rule := range rules {
		cfRule := cloudflare.RulesetRule{
			Action:      rule.Action,
			Expression:  rule.Expression,
			Description: rule.Description,
			Enabled:     ptr.To(rule.Enabled),
			Ref:         rule.Ref,
		}

		// Convert action parameters if present
		if rule.ActionParameters != nil {
			cfRule.ActionParameters = r.convertActionParameters(rule.ActionParameters)
		}

		// Convert rate limit if present
		if rule.RateLimit != nil {
			cfRule.RateLimit = r.convertRateLimit(rule.RateLimit)
		}

		cfRules[i] = cfRule
	}

	return cfRules
}

// convertActionParameters converts action parameters to Cloudflare format.
//
//nolint:revive // cognitive complexity is acceptable for parameter conversion
func (*ZoneRulesetController) convertActionParameters(params *v1alpha2.RulesetRuleActionParameters) *cloudflare.RulesetRuleActionParameters {
	cfParams := &cloudflare.RulesetRuleActionParameters{}

	// URI rewrite
	if params.URI != nil {
		cfParams.URI = &cloudflare.RulesetRuleActionParametersURI{}
		if params.URI.Path != nil {
			cfParams.URI.Path = &cloudflare.RulesetRuleActionParametersURIPath{
				Value:      params.URI.Path.Value,
				Expression: params.URI.Path.Expression,
			}
		}
		if params.URI.Query != nil {
			cfParams.URI.Query = &cloudflare.RulesetRuleActionParametersURIQuery{
				Expression: params.URI.Query.Expression,
			}
			if params.URI.Query.Value != "" {
				cfParams.URI.Query.Value = ptr.To(params.URI.Query.Value)
			}
		}
	}

	// Headers
	if len(params.Headers) > 0 {
		cfParams.Headers = make(map[string]cloudflare.RulesetRuleActionParametersHTTPHeader)
		for name, header := range params.Headers {
			cfParams.Headers[name] = cloudflare.RulesetRuleActionParametersHTTPHeader{
				Operation:  header.Operation,
				Value:      header.Value,
				Expression: header.Expression,
			}
		}
	}

	// Redirect
	if params.Redirect != nil {
		cfParams.FromValue = &cloudflare.RulesetRuleActionParametersFromValue{
			PreserveQueryString: ptr.To(params.Redirect.PreserveQueryString),
		}
		if params.Redirect.StatusCode > 0 {
			cfParams.FromValue.StatusCode = uint16(params.Redirect.StatusCode)
		}
		if params.Redirect.TargetURL != nil {
			cfParams.FromValue.TargetURL = cloudflare.RulesetRuleActionParametersTargetURL{
				Value:      params.Redirect.TargetURL.Value,
				Expression: params.Redirect.TargetURL.Expression,
			}
		}
	}

	// Products (for skip action)
	if len(params.Products) > 0 {
		cfParams.Products = params.Products
	}

	// Ruleset (for execute action)
	if params.Ruleset != "" {
		cfParams.ID = params.Ruleset
	}

	// Phases (for skip action)
	if len(params.Phases) > 0 {
		cfParams.Phases = params.Phases
	}

	// Rules (for skip action)
	if len(params.Rules) > 0 {
		cfParams.Rules = params.Rules
	}

	// Response (for serve_error action)
	if params.Response != nil {
		cfParams.Response = &cloudflare.RulesetRuleActionParametersBlockResponse{
			ContentType: params.Response.ContentType,
			Content:     params.Response.Content,
		}
		if params.Response.StatusCode > 0 {
			cfParams.Response.StatusCode = uint16(params.Response.StatusCode)
		}
	}

	return cfParams
}

// convertRateLimit converts rate limit settings to Cloudflare format.
func (*ZoneRulesetController) convertRateLimit(rl *v1alpha2.RulesetRuleRateLimit) *cloudflare.RulesetRuleRateLimit {
	cfRL := &cloudflare.RulesetRuleRateLimit{
		Characteristics:         rl.Characteristics,
		CountingExpression:      rl.CountingExpression,
		ScoreResponseHeaderName: rl.ScoreResponseHeaderName,
	}
	if rl.RequestsToOrigin != nil {
		cfRL.RequestsToOrigin = *rl.RequestsToOrigin
	}
	if rl.Period > 0 {
		cfRL.Period = rl.Period
	}
	if rl.RequestsPerPeriod > 0 {
		cfRL.RequestsPerPeriod = rl.RequestsPerPeriod
	}
	if rl.MitigationTimeout > 0 {
		cfRL.MitigationTimeout = rl.MitigationTimeout
	}
	if rl.ScorePerPeriod > 0 {
		cfRL.ScorePerPeriod = rl.ScorePerPeriod
	}
	return cfRL
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZoneRulesetController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourceZoneRuleset)).
		Complete(r)
}
