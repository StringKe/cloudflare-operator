// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package ruleset

//nolint:dupl // Similar patterns across resource types are intentional

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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

// AggregatedRedirectRule contains merged rules from all sources.
type AggregatedRedirectRule struct {
	// Zone is the domain name
	Zone string
	// Description is the ruleset description
	Description string
	// Rules is the aggregated list of expression-based redirect rules with ownership tracking
	Rules []RedirectRuleWithOwner
	// WildcardRules is the aggregated list of wildcard-based redirect rules with ownership tracking
	WildcardRules []WildcardRedirectRuleWithOwner
	// SourceCount is the number of sources that contributed
	SourceCount int
}

// RedirectRuleWithOwner contains a redirect rule with its owner information.
type RedirectRuleWithOwner struct {
	Rule  rulesetsvc.RedirectRuleDefinitionConfig
	Owner v1alpha2.SourceReference
}

// WildcardRedirectRuleWithOwner contains a wildcard redirect rule with its owner information.
type WildcardRedirectRuleWithOwner struct {
	Rule  rulesetsvc.WildcardRedirectRuleConfig
	Owner v1alpha2.SourceReference
}

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

	// Add finalizer if not present (with conflict retry)
	if !controllerutil.ContainsFinalizer(syncState, RedirectRuleFinalizerName) {
		if err := common.AddFinalizerWithRetry(ctx, r.Client, syncState, RedirectRuleFinalizerName); err != nil {
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
		logger.Error(err, "Failed to aggregate redirect rule configuration")
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
		"ruleCount", result.RuleCount,
		"sourceCount", aggregated.SourceCount)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// aggregateAllSources aggregates rules from ALL sources using unified aggregation pattern.
//
//nolint:revive,unparam // cognitive complexity is acceptable for aggregation logic; error return for future use
func (r *RedirectRuleController) aggregateAllSources(syncState *v1alpha2.CloudflareSyncState) (*AggregatedRedirectRule, error) {
	if len(syncState.Spec.Sources) == 0 {
		return &AggregatedRedirectRule{
			Rules:         []RedirectRuleWithOwner{},
			WildcardRules: []WildcardRedirectRuleWithOwner{},
			SourceCount:   0,
		}, nil
	}

	// Sort sources by priority (lower number = higher priority)
	sources := make([]v1alpha2.ConfigSource, len(syncState.Spec.Sources))
	copy(sources, syncState.Spec.Sources)
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Priority < sources[j].Priority
	})

	result := &AggregatedRedirectRule{
		Rules:         make([]RedirectRuleWithOwner, 0),
		WildcardRules: make([]WildcardRedirectRuleWithOwner, 0),
	}

	// Process each source
	for _, source := range sources {
		if source.Config.Raw == nil {
			continue
		}

		// Parse source config
		var config rulesetsvc.RedirectRuleConfig
		if err := json.Unmarshal(source.Config.Raw, &config); err != nil {
			continue // Skip invalid configs
		}

		// Use first source's zone and description
		if result.Zone == "" {
			result.Zone = config.Zone
		}
		if result.Description == "" {
			result.Description = config.Description
		}

		// Add rules from this source with ownership tracking
		for _, rule := range config.Rules {
			result.Rules = append(result.Rules, RedirectRuleWithOwner{
				Rule:  rule,
				Owner: source.Ref,
			})
		}

		// Add wildcard rules from this source with ownership tracking
		for _, rule := range config.WildcardRules {
			result.WildcardRules = append(result.WildcardRules, WildcardRedirectRuleWithOwner{
				Rule:  rule,
				Owner: source.Ref,
			})
		}

		result.SourceCount++
	}

	return result, nil
}

// syncToCloudflare syncs the aggregated redirect rule configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *RedirectRuleController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	aggregated *AggregatedRedirectRule,
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

	// Convert aggregated rules to Cloudflare format with ownership markers
	rules := r.convertAggregatedRules(aggregated)

	logger.Info("Updating redirect ruleset",
		"zoneId", zoneID,
		"ruleCount", len(rules),
		"sourceCount", aggregated.SourceCount)

	// Update entrypoint ruleset
	result, err := apiClient.UpdateEntrypointRuleset(
		ctx,
		zoneID,
		redirectPhase,
		aggregated.Description,
		rules,
	)
	if err != nil {
		return nil, fmt.Errorf("update entrypoint ruleset: %w", err)
	}

	// Update SyncState with actual ruleset ID if it was pending (must succeed)
	if common.IsPendingID(syncState.Spec.CloudflareID) && result.ID != "" {
		if err := common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID); err != nil {
			return nil, err
		}
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

// convertAggregatedRules converts aggregated rules to Cloudflare RulesetRule format
// with ownership markers embedded in the description.
//
//nolint:revive // cognitive complexity is acceptable for rule conversion
func (r *RedirectRuleController) convertAggregatedRules(aggregated *AggregatedRedirectRule) []cloudflare.RulesetRule {
	totalRules := len(aggregated.Rules) + len(aggregated.WildcardRules)
	rules := make([]cloudflare.RulesetRule, 0, totalRules)

	// Convert expression-based rules with ownership markers
	for _, ruleWithOwner := range aggregated.Rules {
		rule := ruleWithOwner.Rule
		// Add ownership marker to description
		marker := common.NewOwnershipMarker(ruleWithOwner.Owner)
		description := marker.AppendToDescription(rule.Name)

		cfRule := cloudflare.RulesetRule{
			Action:      "redirect",
			Expression:  rule.Expression,
			Description: description,
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

	// Convert wildcard-based rules with ownership markers
	for _, ruleWithOwner := range aggregated.WildcardRules {
		rule := ruleWithOwner.Rule
		// Add ownership marker to description
		marker := common.NewOwnershipMarker(ruleWithOwner.Owner)
		description := marker.AppendToDescription(rule.Name)

		// Build the expression from wildcard pattern with IncludeSubdomains and SubpathMatching support
		expression := r.buildWildcardExpression(rule.SourceURL, WildcardExpressionOptions{
			IncludeSubdomains: rule.IncludeSubdomains,
			SubpathMatching:   rule.SubpathMatching,
		})

		cfRule := cloudflare.RulesetRule{
			Action:      "redirect",
			Expression:  expression,
			Description: description,
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

// handleDeletion handles the deletion using unified aggregation pattern.
// It re-aggregates remaining sources and preserves external rules.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller syncs remaining rules
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

	zoneID := syncState.Spec.ZoneID
	if zoneID == "" {
		// No zone ID, just cleanup
		return r.cleanupSyncState(ctx, syncState)
	}

	// Create API client
	apiClient, apiErr := common.CreateAPIClient(ctx, r.Client, syncState)
	if apiErr != nil {
		logger.Error(apiErr, "Failed to create API client for deletion")
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(apiErr)}, nil
	}

	// Re-aggregate remaining sources (unified aggregation pattern)
	aggregated, err := r.aggregateAllSources(syncState)
	if err != nil {
		logger.Error(err, "Failed to aggregate remaining sources")
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
	}

	// Get existing rules from Cloudflare
	existingRuleset, getErr := apiClient.GetEntrypointRuleset(ctx, zoneID, redirectPhase)
	if getErr != nil {
		if !cf.IsNotFoundError(getErr) {
			logger.Error(getErr, "Failed to get existing redirect rules")
			return ctrl.Result{RequeueAfter: common.RequeueAfterError(getErr)}, nil
		}
		// Ruleset doesn't exist, just cleanup
		logger.Info("Redirect ruleset not found, cleaning up SyncState")
		return r.cleanupSyncState(ctx, syncState)
	}

	// Filter external rules (not managed by operator)
	externalRules := r.filterExternalRules(existingRuleset.Rules)

	// Convert aggregated rules (from remaining sources)
	finalRules := r.convertAggregatedRules(aggregated)

	// Merge remaining rules with external rules
	finalRules = append(finalRules, externalRules...)

	managedRulesCount := len(finalRules) - len(externalRules)
	logger.Info("Syncing redirect rules after source removal",
		"zoneId", zoneID,
		"managedRules", managedRulesCount,
		"externalRules", len(externalRules),
		"totalRules", len(finalRules),
		"remainingSources", aggregated.SourceCount)

	// Update Cloudflare with merged rules
	_, updateErr := apiClient.UpdateEntrypointRuleset(
		ctx,
		zoneID,
		redirectPhase,
		aggregated.Description,
		finalRules,
	)
	if updateErr != nil {
		if !cf.IsNotFoundError(updateErr) {
			logger.Error(updateErr, "Failed to update redirect rules")
			if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, updateErr); statusErr != nil {
				logger.Error(statusErr, "Failed to update error status")
			}
			return ctrl.Result{RequeueAfter: common.RequeueAfterError(updateErr)}, nil
		}
	}

	logger.Info("Successfully updated redirect rules after source removal")

	// Cleanup SyncState
	return r.cleanupSyncState(ctx, syncState)
}

// cleanupSyncState removes finalizer and optionally deletes the SyncState.
//
//nolint:revive // cognitive complexity acceptable for cleanup logic with error handling
func (r *RedirectRuleController) cleanupSyncState(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Remove finalizer (with conflict retry)
	if err := common.RemoveFinalizerWithRetry(ctx, r.Client, syncState, RedirectRuleFinalizerName); err != nil {
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

// WildcardExpressionOptions contains options for building wildcard expressions.
type WildcardExpressionOptions struct {
	// IncludeSubdomains matches both the domain and its subdomains
	IncludeSubdomains bool
	// SubpathMatching appends /* if the pattern doesn't end with a wildcard
	SubpathMatching bool
}

// appendSubpathWildcard appends /* to the URL if it doesn't already end with a wildcard.
func appendSubpathWildcard(url string) string {
	if len(url) > 0 && url[len(url)-1] != '*' {
		return url + "/*"
	}
	return url
}

// buildSubdomainURL creates a subdomain wildcard URL from the original URL.
// For "https://example.com/path", returns "https://*.example.com/path".
// Returns the original URL if it doesn't match known protocol patterns.
func buildSubdomainURL(url string) string {
	const httpsPrefix, httpPrefix = "https://", "http://"
	switch {
	case strings.HasPrefix(url, httpsPrefix):
		return httpsPrefix + "*." + url[len(httpsPrefix):]
	case strings.HasPrefix(url, httpPrefix):
		return httpPrefix + "*." + url[len(httpPrefix):]
	default:
		return url
	}
}

// buildWildcardExpression builds a wirefilter expression from a wildcard URL pattern.
// It supports IncludeSubdomains and SubpathMatching options.
//
// Examples:
//   - Basic: https://example.com/path/* -> http.request.full_uri wildcard "https://example.com/path/*"
//   - IncludeSubdomains: matches both example.com and *.example.com
//   - SubpathMatching: appends /* if the pattern doesn't end with a wildcard
func (*RedirectRuleController) buildWildcardExpression(sourceURL string, opts WildcardExpressionOptions) string {
	effectiveURL := sourceURL
	if opts.SubpathMatching {
		effectiveURL = appendSubpathWildcard(effectiveURL)
	}
	baseExpression := fmt.Sprintf(`http.request.full_uri wildcard "%s"`, effectiveURL)

	if !opts.IncludeSubdomains {
		return baseExpression
	}

	// Build OR expression for domain and subdomains
	subdomainURL := buildSubdomainURL(effectiveURL)
	if subdomainURL == effectiveURL {
		return baseExpression
	}

	subdomainExpression := fmt.Sprintf(`http.request.full_uri wildcard "%s"`, subdomainURL)
	return fmt.Sprintf(`(%s) or (%s)`, baseExpression, subdomainExpression)
}

// filterExternalRules returns rules that are NOT managed by the operator.
// External rules are those without the "managed-by:" marker in their description.
func (*RedirectRuleController) filterExternalRules(rules []cloudflare.RulesetRule) []cloudflare.RulesetRule {
	external := make([]cloudflare.RulesetRule, 0)
	for _, rule := range rules {
		if !common.IsManagedByOperator(rule.Description) {
			external = append(external, rule)
		}
	}
	return external
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedirectRuleController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("redirect-rule-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourceRedirectRule)).
		Complete(r)
}
