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

	// Handle deletion - this is the SINGLE point for Cloudflare API delete calls
	if !syncState.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, syncState)
	}

	// Check if there are any sources - if none, clear settings on Cloudflare
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, clearing settings from Cloudflare")
		return r.handleDeletion(ctx, syncState)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(syncState, SettingsPolicyFinalizerName) {
		controllerutil.AddFinalizer(syncState, SettingsPolicyFinalizerName)
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

	// Extract Device Settings Policy configuration from first source
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract Device Settings Policy configuration")
		if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
	}

	// Compute hash for change detection
	newHash, hashErr := common.ComputeConfigHash(config)
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

	// Sync to Cloudflare API
	result, err := r.syncToCloudflare(ctx, syncState, config)
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
		"fallbackDomainsCount", result.FallbackDomainsCount)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts Device Settings Policy configuration from SyncState sources.
// Device Settings Policies have 1:1 mapping, so we use the ExtractFirstSourceConfig helper.
func (*SettingsPolicyController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*devicesvc.DeviceSettingsPolicyConfig, error) {
	return common.ExtractFirstSourceConfig[devicesvc.DeviceSettingsPolicyConfig](syncState)
}

// syncToCloudflare syncs the Device Settings Policy configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *SettingsPolicyController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *devicesvc.DeviceSettingsPolicyConfig,
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

	// Merge auto-populated routes with manual entries based on mode
	allExcludeEntries := config.SplitTunnelExclude
	allIncludeEntries := config.SplitTunnelInclude

	// Add auto-populated routes to the appropriate list based on split tunnel mode
	if len(config.AutoPopulatedRoutes) > 0 {
		if config.SplitTunnelMode == "include" {
			allIncludeEntries = append(allIncludeEntries, config.AutoPopulatedRoutes...)
		} else {
			// Default to exclude mode
			allExcludeEntries = append(allExcludeEntries, config.AutoPopulatedRoutes...)
		}
	}

	// Sync split tunnel exclude entries
	if len(allExcludeEntries) > 0 {
		excludeEntries := make([]cf.SplitTunnelEntry, len(allExcludeEntries))
		for i, e := range allExcludeEntries {
			excludeEntries[i] = cf.SplitTunnelEntry{
				Address:     e.Address,
				Host:        e.Host,
				Description: e.Description,
			}
		}

		logger.Info("Updating split tunnel exclude entries",
			"count", len(excludeEntries))

		if err := apiClient.UpdateSplitTunnelExclude(excludeEntries); err != nil {
			return nil, fmt.Errorf("update split tunnel exclude: %w", err)
		}
		result.SplitTunnelExcludeCount = len(excludeEntries)
	}

	// Sync split tunnel include entries
	if len(allIncludeEntries) > 0 {
		includeEntries := make([]cf.SplitTunnelEntry, len(allIncludeEntries))
		for i, e := range allIncludeEntries {
			includeEntries[i] = cf.SplitTunnelEntry{
				Address:     e.Address,
				Host:        e.Host,
				Description: e.Description,
			}
		}

		logger.Info("Updating split tunnel include entries",
			"count", len(includeEntries))

		if err := apiClient.UpdateSplitTunnelInclude(includeEntries); err != nil {
			return nil, fmt.Errorf("update split tunnel include: %w", err)
		}
		result.SplitTunnelIncludeCount = len(includeEntries)
	}

	// Sync fallback domains
	if len(config.FallbackDomains) > 0 {
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

		if err := apiClient.UpdateFallbackDomains(fallbackDomains); err != nil {
			return nil, fmt.Errorf("update fallback domains: %w", err)
		}
		result.FallbackDomainsCount = len(fallbackDomains)
	}

	result.AutoPopulatedRoutesCount = len(config.AutoPopulatedRoutes)

	logger.Info("Updated Device Settings Policy",
		"accountId", accountID,
		"excludeCount", result.SplitTunnelExcludeCount,
		"includeCount", result.SplitTunnelIncludeCount,
		"fallbackDomainsCount", result.FallbackDomainsCount,
		"autoPopulatedRoutesCount", result.AutoPopulatedRoutesCount)

	return result, nil
}

// handleDeletion handles the deletion of Device Settings Policy from Cloudflare.
// This clears split tunnel entries and fallback domains by setting them to empty arrays.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller deletes from Cloudflare
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps
func (r *SettingsPolicyController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, SettingsPolicyFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Clear settings from Cloudflare by setting empty arrays
	apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
	if err != nil {
		logger.Error(err, "Failed to create API client for deletion")
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
	}

	logger.Info("Clearing Device Settings Policy from Cloudflare")

	// Clear split tunnel exclude entries
	if err := apiClient.UpdateSplitTunnelExclude([]cf.SplitTunnelEntry{}); err != nil {
		if !cf.IsNotFoundError(err) {
			logger.Error(err, "Failed to clear split tunnel exclude entries")
			if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
				logger.Error(statusErr, "Failed to update error status")
			}
			return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
		}
	}

	// Clear split tunnel include entries
	if err := apiClient.UpdateSplitTunnelInclude([]cf.SplitTunnelEntry{}); err != nil {
		if !cf.IsNotFoundError(err) {
			logger.Error(err, "Failed to clear split tunnel include entries")
			if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
				logger.Error(statusErr, "Failed to update error status")
			}
			return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
		}
	}

	// Clear fallback domains
	if err := apiClient.UpdateFallbackDomains([]cf.FallbackDomainEntry{}); err != nil {
		if !cf.IsNotFoundError(err) {
			logger.Error(err, "Failed to clear fallback domains")
			if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
				logger.Error(statusErr, "Failed to update error status")
			}
			return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
		}
	}

	logger.Info("Successfully cleared Device Settings Policy from Cloudflare")

	// Remove finalizer
	controllerutil.RemoveFinalizer(syncState, SettingsPolicyFinalizerName)
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
func (r *SettingsPolicyController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("device-settingspolicy-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourceDeviceSettingsPolicy)).
		Complete(r)
}
