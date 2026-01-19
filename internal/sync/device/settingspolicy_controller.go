// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package device

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	devicesvc "github.com/StringKe/cloudflare-operator/internal/service/device"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

const (
	// SettingsPolicyFinalizerName is the finalizer for Device Settings Policy SyncState resources.
	SettingsPolicyFinalizerName = "devicesettingspolicy.sync.cloudflare-operator.io/finalizer"
)

// SettingsPolicyController is the Sync Controller for Device Settings Policy Configuration.
// It uses the unified aggregation pattern to merge configurations from multiple sources.
type SettingsPolicyController struct {
	*common.BaseSyncController
}

// NewSettingsPolicyController creates a new DeviceSettingsPolicySyncController.
func NewSettingsPolicyController(c client.Client) *SettingsPolicyController {
	return &SettingsPolicyController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for Device Settings Policy.
// It aggregates configurations from all sources and syncs to Cloudflare.
//
//nolint:revive // cognitive complexity is acceptable for sync controller reconciliation
func (r *SettingsPolicyController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "DeviceSettingsPolicySync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process DeviceSettingsPolicy type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceDeviceSettingsPolicy {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing DeviceSettingsPolicy SyncState",
		"cloudflareId", syncState.Spec.CloudflareID,
		"sources", len(syncState.Spec.Sources))

	// Handle deletion - aggregate remaining sources and sync
	if !syncState.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, syncState)
	}

	// Check if there are any sources - if none, sync empty config to Cloudflare
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, syncing empty configuration")
		return r.handleDeletion(ctx, syncState)
	}

	// Add finalizer if not present (with conflict retry)
	if !controllerutil.ContainsFinalizer(syncState, SettingsPolicyFinalizerName) {
		if err := common.AddFinalizerWithRetry(ctx, r.Client, syncState, SettingsPolicyFinalizerName); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip if there's a pending debounced request
	if r.Debouncer.IsPending(req.Name) {
		logger.V(1).Info("Skipping reconcile - debounced request pending")
		return ctrl.Result{}, nil
	}

	// Aggregate all sources using unified aggregation pattern
	aggregated, err := r.aggregateAllSources(syncState)
	if err != nil {
		logger.Error(err, "Failed to aggregate Device Settings Policy configuration")
		if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
	}

	// Compute hash for change detection
	newHash, hashErr := common.ComputeConfigHash(aggregated)
	if hashErr != nil {
		logger.Error(hashErr, "Failed to compute config hash")
		newHash = ""
	}

	if !r.ShouldSync(syncState, newHash) {
		logger.V(1).Info("Configuration unchanged, skipping sync",
			"hash", newHash)
		return ctrl.Result{}, nil
	}

	// Set syncing status
	if err := r.SetSyncStatus(ctx, syncState, v1alpha2.SyncStatusSyncing); err != nil {
		return ctrl.Result{}, err
	}

	// Sync aggregated configuration to Cloudflare API
	result, err := r.syncToCloudflare(ctx, syncState, aggregated)
	if err != nil {
		logger.Error(err, "Failed to sync Device Settings Policy to Cloudflare")
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

	logger.Info("Successfully synced Device Settings Policy to Cloudflare",
		"accountId", result.AccountID,
		"excludeCount", result.SplitTunnelExcludeCount,
		"includeCount", result.SplitTunnelIncludeCount,
		"fallbackDomainsCount", result.FallbackDomainsCount,
		"sourceCount", len(syncState.Spec.Sources))

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// AggregatedSettingsPolicy contains the merged configuration from all sources
type AggregatedSettingsPolicy struct {
	// SplitTunnelMode from highest priority source
	SplitTunnelMode string
	// SplitTunnelExclude merged from all sources
	SplitTunnelExclude []devicesvc.SplitTunnelEntry
	// SplitTunnelInclude merged from all sources
	SplitTunnelInclude []devicesvc.SplitTunnelEntry
	// FallbackDomains merged from all sources
	FallbackDomains []devicesvc.FallbackDomainEntry
	// SourceCount is the number of sources that contributed
	SourceCount int
}

// aggregateAllSources aggregates configurations from all sources in the SyncState.
// It merges split tunnel entries and fallback domains, tracking ownership via description.
//
//nolint:revive,unparam // cognitive complexity acceptable; error return for future use
func (r *SettingsPolicyController) aggregateAllSources(syncState *v1alpha2.CloudflareSyncState) (*AggregatedSettingsPolicy, error) {
	result := &AggregatedSettingsPolicy{
		SplitTunnelExclude: make([]devicesvc.SplitTunnelEntry, 0),
		SplitTunnelInclude: make([]devicesvc.SplitTunnelEntry, 0),
		FallbackDomains:    make([]devicesvc.FallbackDomainEntry, 0),
	}

	if len(syncState.Spec.Sources) == 0 {
		return result, nil
	}

	// Track seen keys for deduplication
	seenExclude := make(map[string]bool)
	seenInclude := make(map[string]bool)
	seenFallback := make(map[string]bool)

	// Process each source (already sorted by priority in SyncState)
	for _, source := range syncState.Spec.Sources {
		config, err := common.ParseSourceConfig[devicesvc.DeviceSettingsPolicyConfig](&source)
		if err != nil || config == nil {
			continue
		}

		result.SourceCount++

		// Create ownership marker for this source
		marker := common.NewOwnershipMarker(source.Ref)

		// Take SplitTunnelMode from first source (highest priority)
		if result.SplitTunnelMode == "" && config.SplitTunnelMode != "" {
			result.SplitTunnelMode = config.SplitTunnelMode
		}

		// Merge exclude entries with ownership tracking
		for _, entry := range config.SplitTunnelExclude {
			key := entry.Address + "|" + entry.Host
			if !seenExclude[key] {
				seenExclude[key] = true
				result.SplitTunnelExclude = append(result.SplitTunnelExclude, devicesvc.SplitTunnelEntry{
					Address:     entry.Address,
					Host:        entry.Host,
					Description: marker.AppendToDescription(entry.Description),
				})
			}
		}

		// Merge include entries with ownership tracking
		for _, entry := range config.SplitTunnelInclude {
			key := entry.Address + "|" + entry.Host
			if !seenInclude[key] {
				seenInclude[key] = true
				result.SplitTunnelInclude = append(result.SplitTunnelInclude, devicesvc.SplitTunnelEntry{
					Address:     entry.Address,
					Host:        entry.Host,
					Description: marker.AppendToDescription(entry.Description),
				})
			}
		}

		// Merge auto-populated routes based on mode
		for _, entry := range config.AutoPopulatedRoutes {
			if config.SplitTunnelMode == "include" {
				key := entry.Address + "|" + entry.Host
				if !seenInclude[key] {
					seenInclude[key] = true
					result.SplitTunnelInclude = append(result.SplitTunnelInclude, devicesvc.SplitTunnelEntry{
						Address:     entry.Address,
						Host:        entry.Host,
						Description: marker.AppendToDescription(entry.Description),
					})
				}
			} else {
				key := entry.Address + "|" + entry.Host
				if !seenExclude[key] {
					seenExclude[key] = true
					result.SplitTunnelExclude = append(result.SplitTunnelExclude, devicesvc.SplitTunnelEntry{
						Address:     entry.Address,
						Host:        entry.Host,
						Description: marker.AppendToDescription(entry.Description),
					})
				}
			}
		}

		// Merge fallback domains with ownership tracking
		for _, entry := range config.FallbackDomains {
			key := entry.Suffix
			if !seenFallback[key] {
				seenFallback[key] = true
				result.FallbackDomains = append(result.FallbackDomains, devicesvc.FallbackDomainEntry{
					Suffix:      entry.Suffix,
					Description: marker.AppendToDescription(entry.Description),
					DNSServer:   entry.DNSServer,
				})
			}
		}
	}

	return result, nil
}

// syncToCloudflare syncs the aggregated Device Settings Policy configuration to Cloudflare API.
func (r *SettingsPolicyController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *AggregatedSettingsPolicy,
) (*devicesvc.DeviceSettingsPolicySyncResult, error) {
	logger := log.FromContext(ctx)

	// Create API client using common helper
	apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
	if err != nil {
		return nil, err
	}

	// Validate account ID is present
	accountID, err := common.RequireAccountID(syncState)
	if err != nil {
		return nil, err
	}

	result := &devicesvc.DeviceSettingsPolicySyncResult{
		AccountID: accountID,
	}

	// Sync split tunnel exclude entries
	excludeEntries := make([]cf.SplitTunnelEntry, len(config.SplitTunnelExclude))
	for i, e := range config.SplitTunnelExclude {
		excludeEntries[i] = cf.SplitTunnelEntry{
			Address:     e.Address,
			Host:        e.Host,
			Description: e.Description,
		}
	}

	logger.Info("Updating split tunnel exclude entries",
		"count", len(excludeEntries))

	if err := apiClient.UpdateSplitTunnelExclude(ctx, excludeEntries); err != nil {
		return nil, fmt.Errorf("update split tunnel exclude: %w", err)
	}
	result.SplitTunnelExcludeCount = len(excludeEntries)

	// Sync split tunnel include entries
	includeEntries := make([]cf.SplitTunnelEntry, len(config.SplitTunnelInclude))
	for i, e := range config.SplitTunnelInclude {
		includeEntries[i] = cf.SplitTunnelEntry{
			Address:     e.Address,
			Host:        e.Host,
			Description: e.Description,
		}
	}

	logger.Info("Updating split tunnel include entries",
		"count", len(includeEntries))

	if err := apiClient.UpdateSplitTunnelInclude(ctx, includeEntries); err != nil {
		return nil, fmt.Errorf("update split tunnel include: %w", err)
	}
	result.SplitTunnelIncludeCount = len(includeEntries)

	// Sync fallback domains
	fallbackDomains := make([]cf.FallbackDomainEntry, len(config.FallbackDomains))
	for i, e := range config.FallbackDomains {
		fallbackDomains[i] = cf.FallbackDomainEntry{
			Suffix:      e.Suffix,
			Description: e.Description,
			DNSServer:   e.DNSServer,
		}
	}

	logger.Info("Updating fallback domains",
		"count", len(fallbackDomains))

	if err := apiClient.UpdateFallbackDomains(ctx, fallbackDomains); err != nil {
		return nil, fmt.Errorf("update fallback domains: %w", err)
	}
	result.FallbackDomainsCount = len(fallbackDomains)

	logger.Info("Updated Device Settings Policy",
		"accountId", accountID,
		"excludeCount", result.SplitTunnelExcludeCount,
		"includeCount", result.SplitTunnelIncludeCount,
		"fallbackDomainsCount", result.FallbackDomainsCount)

	return result, nil
}

// handleDeletion handles the deletion of Device Settings Policy from Cloudflare.
// It re-aggregates remaining sources and syncs to Cloudflare.
// If no sources remain, it syncs empty configuration (but does NOT clear external entries).
//
//nolint:revive // cognitive complexity is acceptable for deletion handling
func (r *SettingsPolicyController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, SettingsPolicyFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Create API client
	apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
	if err != nil {
		logger.Error(err, "Failed to create API client for deletion")
		// Continue with finalizer removal - don't block on API client creation failure
		goto removeFinalizer
	}

	// Re-aggregate remaining sources (if any)
	{
		aggregated, err := r.aggregateAllSources(syncState)
		if err != nil {
			logger.Error(err, "Failed to aggregate remaining sources")
			goto removeFinalizer
		}

		// Get current entries from Cloudflare to preserve external (non-operator-managed) entries
		existingExclude, err := apiClient.GetSplitTunnelExclude(ctx)
		if err != nil {
			logger.Error(err, "Failed to get existing exclude entries")
			// Continue anyway - will just use aggregated entries
		}

		existingInclude, err := apiClient.GetSplitTunnelInclude(ctx)
		if err != nil {
			logger.Error(err, "Failed to get existing include entries")
		}

		existingFallback, err := apiClient.GetFallbackDomains(ctx)
		if err != nil {
			logger.Error(err, "Failed to get existing fallback domains")
		}

		// Filter out operator-managed entries, keep external entries
		externalExclude := filterExternalEntries(existingExclude)
		externalInclude := filterExternalEntries(existingInclude)
		externalFallback := filterExternalFallbackDomains(existingFallback)

		// Merge external entries with remaining aggregated entries
		finalExclude := mergeWithExternal(aggregated.SplitTunnelExclude, externalExclude)
		finalInclude := mergeWithExternal(aggregated.SplitTunnelInclude, externalInclude)
		finalFallback := mergeWithExternalFallback(aggregated.FallbackDomains, externalFallback)

		// Sync merged configuration to Cloudflare
		logger.Info("Syncing merged configuration after source removal",
			"excludeCount", len(finalExclude),
			"includeCount", len(finalInclude),
			"fallbackCount", len(finalFallback),
			"remainingSources", len(syncState.Spec.Sources))

		if err := apiClient.UpdateSplitTunnelExclude(ctx, finalExclude); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to update split tunnel exclude entries")
				if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
					logger.Error(statusErr, "Failed to update error status")
				}
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}
		}

		if err := apiClient.UpdateSplitTunnelInclude(ctx, finalInclude); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to update split tunnel include entries")
				if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
					logger.Error(statusErr, "Failed to update error status")
				}
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}
		}

		if err := apiClient.UpdateFallbackDomains(ctx, finalFallback); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to update fallback domains")
				if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
					logger.Error(statusErr, "Failed to update error status")
				}
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}
		}

		logger.Info("Successfully synced Device Settings Policy after source removal")
	}

removeFinalizer:
	// Remove finalizer (with conflict retry)
	if err := common.RemoveFinalizerWithRetry(ctx, r.Client, syncState, SettingsPolicyFinalizerName); err != nil {
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

// filterExternalEntries returns entries NOT managed by the operator.
// External entries don't have the "managed-by:" marker in their description.
func filterExternalEntries(entries []cf.SplitTunnelEntry) []cf.SplitTunnelEntry {
	var external []cf.SplitTunnelEntry
	for _, entry := range entries {
		if !common.IsManagedByOperator(entry.Description) {
			external = append(external, entry)
		}
	}
	return external
}

// filterExternalFallbackDomains returns fallback domains NOT managed by the operator.
func filterExternalFallbackDomains(entries []cf.FallbackDomainEntry) []cf.FallbackDomainEntry {
	var external []cf.FallbackDomainEntry
	for _, entry := range entries {
		if !common.IsManagedByOperator(entry.Description) {
			external = append(external, entry)
		}
	}
	return external
}

// mergeWithExternal merges aggregated entries with external entries.
// External entries are added at the end, duplicates are skipped.
func mergeWithExternal(aggregated []devicesvc.SplitTunnelEntry, external []cf.SplitTunnelEntry) []cf.SplitTunnelEntry {
	seen := make(map[string]bool)
	result := make([]cf.SplitTunnelEntry, 0, len(aggregated)+len(external))

	// Add aggregated entries first
	for _, e := range aggregated {
		key := e.Address + "|" + e.Host
		if !seen[key] {
			seen[key] = true
			result = append(result, cf.SplitTunnelEntry{
				Address:     e.Address,
				Host:        e.Host,
				Description: e.Description,
			})
		}
	}

	// Add external entries
	for _, e := range external {
		key := e.Address + "|" + e.Host
		if !seen[key] {
			seen[key] = true
			result = append(result, e)
		}
	}

	return result
}

// mergeWithExternalFallback merges aggregated fallback domains with external entries.
func mergeWithExternalFallback(aggregated []devicesvc.FallbackDomainEntry, external []cf.FallbackDomainEntry) []cf.FallbackDomainEntry {
	seen := make(map[string]bool)
	result := make([]cf.FallbackDomainEntry, 0, len(aggregated)+len(external))

	// Add aggregated entries first
	for _, e := range aggregated {
		if !seen[e.Suffix] {
			seen[e.Suffix] = true
			result = append(result, cf.FallbackDomainEntry{
				Suffix:      e.Suffix,
				Description: e.Description,
				DNSServer:   e.DNSServer,
			})
		}
	}

	// Add external entries
	for _, e := range external {
		if !seen[e.Suffix] {
			seen[e.Suffix] = true
			result = append(result, e)
		}
	}

	return result
}

// SetupWithManager sets up the controller with the Manager.
func (r *SettingsPolicyController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("device-settingspolicy-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourceDeviceSettingsPolicy)).
		Complete(r)
}
