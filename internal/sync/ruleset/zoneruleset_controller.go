// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package ruleset provides sync controllers for managing Cloudflare Ruleset resources.
//
//nolint:dupl // Similar patterns across resource types are intentional
package ruleset

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

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
	// ZoneRulesetFinalizerName is the finalizer for Zone Ruleset SyncState resources.
	ZoneRulesetFinalizerName = "zoneruleset.sync.cloudflare-operator.io/finalizer"
)

// AggregatedZoneRuleset contains merged rules from all sources.
type AggregatedZoneRuleset struct {
	// Zone is the domain name
	Zone string
	// Phase is the ruleset phase
	Phase string
	// Description is the ruleset description
	Description string
	// Rules is the aggregated list of rules with ownership tracking
	Rules []RuleWithOwner
	// SourceCount is the number of sources that contributed
	SourceCount int
}

// RuleWithOwner contains a rule with its owner information.
type RuleWithOwner struct {
	Rule  rulesetsvc.RulesetRuleConfig
	Owner v1alpha2.SourceReference
}

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

	// Handle deletion - this is the SINGLE point for Cloudflare API delete calls
	if !syncState.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, syncState)
	}

	// Check if there are any sources - if none, clear ruleset from Cloudflare
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, clearing ruleset from Cloudflare")
		return r.handleDeletion(ctx, syncState)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(syncState, ZoneRulesetFinalizerName) {
		controllerutil.AddFinalizer(syncState, ZoneRulesetFinalizerName)
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

	// Aggregate rules from ALL sources (unified aggregation pattern)
	aggregated, err := r.aggregateAllSources(syncState)
	if err != nil {
		logger.Error(err, "Failed to aggregate zone ruleset configuration")
		if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
	}

	// Compute hash for change detection
	newHash, hashErr := common.ComputeConfigHash(aggregated)
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

	// Sync to Cloudflare API with aggregated rules
	result, err := r.syncToCloudflare(ctx, syncState, aggregated)
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
		"ruleCount", result.RuleCount,
		"sourceCount", aggregated.SourceCount)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// aggregateAllSources aggregates rules from ALL sources using unified aggregation pattern.
// This is the key function that enables multiple K8s resources to contribute rules
// to the same Zone+Phase without overwriting each other.
//
//nolint:revive,unparam // cognitive complexity is acceptable for aggregation logic; error return for future use
func (r *ZoneRulesetController) aggregateAllSources(syncState *v1alpha2.CloudflareSyncState) (*AggregatedZoneRuleset, error) {
	if len(syncState.Spec.Sources) == 0 {
		return &AggregatedZoneRuleset{
			Rules:       []RuleWithOwner{},
			SourceCount: 0,
		}, nil
	}

	// Sort sources by priority (lower number = higher priority)
	sources := make([]v1alpha2.ConfigSource, len(syncState.Spec.Sources))
	copy(sources, syncState.Spec.Sources)
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Priority < sources[j].Priority
	})

	result := &AggregatedZoneRuleset{
		Rules: make([]RuleWithOwner, 0),
	}

	// Process each source
	for _, source := range sources {
		if source.Config.Raw == nil {
			continue
		}

		// Parse source config
		var config rulesetsvc.ZoneRulesetConfig
		if err := json.Unmarshal(source.Config.Raw, &config); err != nil {
			continue // Skip invalid configs
		}

		// Use first source's zone, phase, and description
		if result.Zone == "" {
			result.Zone = config.Zone
		}
		if result.Phase == "" {
			result.Phase = config.Phase
		}
		if result.Description == "" {
			result.Description = config.Description
		}

		// Add rules from this source with ownership tracking
		for _, rule := range config.Rules {
			result.Rules = append(result.Rules, RuleWithOwner{
				Rule:  rule,
				Owner: source.Ref,
			})
		}

		result.SourceCount++
	}

	return result, nil
}

// syncToCloudflare syncs the aggregated zone ruleset configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *ZoneRulesetController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	aggregated *AggregatedZoneRuleset,
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

	// Convert aggregated rules to Cloudflare format with ownership markers
	rules := r.convertAggregatedRules(aggregated.Rules)

	logger.Info("Updating zone ruleset",
		"zoneId", zoneID,
		"phase", aggregated.Phase,
		"ruleCount", len(rules),
		"sourceCount", aggregated.SourceCount)

	// Update entrypoint ruleset
	result, err := apiClient.UpdateEntrypointRuleset(
		ctx,
		zoneID,
		aggregated.Phase,
		aggregated.Description,
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

// convertAggregatedRules converts aggregated rules to Cloudflare RulesetRule format
// with ownership markers embedded in the description.
func (r *ZoneRulesetController) convertAggregatedRules(rules []RuleWithOwner) []cloudflare.RulesetRule {
	cfRules := make([]cloudflare.RulesetRule, len(rules))

	for i, ruleWithOwner := range rules {
		rule := ruleWithOwner.Rule
		// Add ownership marker to description
		marker := common.NewOwnershipMarker(ruleWithOwner.Owner)
		description := marker.AppendToDescription(rule.Description)

		cfRule := cloudflare.RulesetRule{
			Action:      rule.Action,
			Expression:  rule.Expression,
			Description: description,
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

// handleDeletion handles the deletion of Zone Ruleset from Cloudflare.
// Uses unified aggregation pattern: re-aggregate remaining sources and preserve external rules.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller re-aggregates remaining
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps
func (r *ZoneRulesetController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, ZoneRulesetFinalizerName) {
		return ctrl.Result{}, nil
	}

	zoneID := syncState.Spec.ZoneID
	if zoneID == "" {
		logger.Info("No zone ID available, cleaning up SyncState only")
		return r.cleanupSyncState(ctx, syncState)
	}

	// Create API client
	apiClient, apiErr := common.CreateAPIClient(ctx, r.Client, syncState)
	if apiErr != nil {
		logger.Error(apiErr, "Failed to create API client for deletion")
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(apiErr)}, nil
	}

	// Re-aggregate remaining sources (may be empty if all sources removed)
	aggregated, err := r.aggregateAllSources(syncState)
	if err != nil {
		logger.Error(err, "Failed to aggregate remaining sources")
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
	}

	// Get existing rules from Cloudflare to preserve external rules
	phase := aggregated.Phase
	if phase == "" {
		// Try to get phase from last known config
		phase = r.getPhaseFromSyncState(syncState)
	}

	if phase != "" {
		// Get existing ruleset to preserve external rules
		existingRuleset, getErr := apiClient.GetEntrypointRuleset(ctx, zoneID, phase)
		var existingRules []cloudflare.RulesetRule
		if getErr != nil {
			if !cf.IsNotFoundError(getErr) {
				logger.Error(getErr, "Failed to get existing rules from Cloudflare")
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(getErr)}, nil
			}
			// Ruleset not found, no existing rules
			existingRules = []cloudflare.RulesetRule{}
		} else {
			existingRules = existingRuleset.Rules
		}

		// Filter external rules (not managed by operator)
		externalRules := r.filterExternalRules(existingRules)

		// Convert aggregated rules to Cloudflare format
		finalRules := r.convertAggregatedRules(aggregated.Rules)
		managedRuleCount := len(finalRules)

		// Merge: aggregated managed rules + external rules
		finalRules = append(finalRules, externalRules...)

		logger.Info("Updating Zone Ruleset with remaining sources",
			"zoneId", zoneID,
			"phase", phase,
			"managedRuleCount", managedRuleCount,
			"externalRuleCount", len(externalRules),
			"totalRuleCount", len(finalRules))

		// Sync merged configuration to Cloudflare
		_, syncErr := apiClient.UpdateEntrypointRuleset(ctx, zoneID, phase, aggregated.Description, finalRules)
		if syncErr != nil {
			if !cf.IsNotFoundError(syncErr) {
				logger.Error(syncErr, "Failed to sync Zone Ruleset rules")
				if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, syncErr); statusErr != nil {
					logger.Error(statusErr, "Failed to update error status")
				}
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(syncErr)}, nil
			}
			logger.Info("Ruleset not found, continuing with cleanup")
		} else {
			logger.Info("Successfully synced Zone Ruleset with remaining sources")
		}
	}

	return r.cleanupSyncState(ctx, syncState)
}

// cleanupSyncState removes the finalizer and optionally deletes the SyncState.
//
//nolint:revive // cognitive complexity acceptable for cleanup logic with error handling
func (r *ZoneRulesetController) cleanupSyncState(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Remove finalizer
	controllerutil.RemoveFinalizer(syncState, ZoneRulesetFinalizerName)
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

// getPhaseFromSyncState tries to extract phase from stored config in SyncState.
func (*ZoneRulesetController) getPhaseFromSyncState(syncState *v1alpha2.CloudflareSyncState) string {
	for _, source := range syncState.Spec.Sources {
		if source.Config.Raw == nil {
			continue
		}
		var config rulesetsvc.ZoneRulesetConfig
		if err := json.Unmarshal(source.Config.Raw, &config); err == nil && config.Phase != "" {
			return config.Phase
		}
	}
	return ""
}

// filterExternalRules returns rules NOT managed by the operator.
// External rules do not have the "managed-by:" marker in their description.
func (*ZoneRulesetController) filterExternalRules(rules []cloudflare.RulesetRule) []cloudflare.RulesetRule {
	var external []cloudflare.RulesetRule
	for _, rule := range rules {
		if !common.IsManagedByOperator(rule.Description) {
			external = append(external, rule)
		}
	}
	return external
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZoneRulesetController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("zone-ruleset-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourceZoneRuleset)).
		Complete(r)
}
