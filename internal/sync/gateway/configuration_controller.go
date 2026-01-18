// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package gateway

//nolint:dupl // Similar patterns across resource types are intentional

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	gatewaysvc "github.com/StringKe/cloudflare-operator/internal/service/gateway"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

const (
	// ConfigurationFinalizerName is the finalizer for Gateway Configuration SyncState resources.
	ConfigurationFinalizerName = "gatewayconfiguration.sync.cloudflare-operator.io/finalizer"
)

// ConfigurationController is the Sync Controller for Gateway Configuration.
type ConfigurationController struct {
	*common.BaseSyncController
}

// NewConfigurationController creates a new GatewayConfigurationSyncController.
func NewConfigurationController(c client.Client) *ConfigurationController {
	return &ConfigurationController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for Gateway configuration.
//
//nolint:revive // cognitive complexity is acceptable for sync controller reconciliation
func (r *ConfigurationController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "GatewayConfigurationSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process GatewayConfiguration type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceGatewayConfiguration {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing GatewayConfiguration SyncState",
		"cloudflareId", syncState.Spec.CloudflareID,
		"sources", len(syncState.Spec.Sources))

	// Handle deletion - this is the SINGLE point for cleanup
	// Note: Gateway Configuration settings are NOT reset on Cloudflare - they persist
	if !syncState.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, syncState)
	}

	// Check if there are any sources - if none, clean up
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, cleaning up")
		return r.handleDeletion(ctx, syncState)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(syncState, ConfigurationFinalizerName) {
		controllerutil.AddFinalizer(syncState, ConfigurationFinalizerName)
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

	// Extract Gateway configuration from first source
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract Gateway configuration")
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
		logger.Error(err, "Failed to sync Gateway configuration to Cloudflare")
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

	logger.Info("Successfully synced Gateway configuration to Cloudflare",
		"accountId", result.AccountID)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts Gateway configuration from SyncState sources.
// Gateway configuration has 1:1 mapping, so we use the ExtractFirstSourceConfig helper.
func (*ConfigurationController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*gatewaysvc.GatewayConfigurationConfig, error) {
	return common.ExtractFirstSourceConfig[gatewaysvc.GatewayConfigurationConfig](syncState)
}

// syncToCloudflare syncs the Gateway configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *ConfigurationController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *gatewaysvc.GatewayConfigurationConfig,
) (*gatewaysvc.GatewayConfigurationSyncResult, error) {
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

	// Build Gateway configuration params
	params := cf.GatewayConfigurationParams{}

	if config.TLSDecrypt != nil {
		params.TLSDecrypt = &cf.TLSDecryptSettings{
			Enabled: config.TLSDecrypt.Enabled,
		}
	}

	if config.ActivityLog != nil {
		params.ActivityLog = &cf.ActivityLogSettings{
			Enabled: config.ActivityLog.Enabled,
		}
	}

	if config.AntiVirus != nil {
		params.AntiVirus = &cf.AntiVirusSettings{
			EnabledDownloadPhase: config.AntiVirus.EnabledDownloadPhase,
			EnabledUploadPhase:   config.AntiVirus.EnabledUploadPhase,
			FailClosed:           config.AntiVirus.FailClosed,
		}
		if config.AntiVirus.NotificationSettings != nil {
			params.AntiVirus.NotificationSettings = &cf.NotificationSettings{
				Enabled:    config.AntiVirus.NotificationSettings.Enabled,
				Message:    config.AntiVirus.NotificationSettings.Message,
				SupportURL: config.AntiVirus.NotificationSettings.SupportURL,
			}
		}
	}

	if config.BlockPage != nil {
		params.BlockPage = &cf.BlockPageSettings{
			Enabled:         config.BlockPage.Enabled,
			FooterText:      config.BlockPage.FooterText,
			HeaderText:      config.BlockPage.HeaderText,
			LogoPath:        config.BlockPage.LogoPath,
			BackgroundColor: config.BlockPage.BackgroundColor,
		}
	}

	if config.BodyScanning != nil {
		params.BodyScanning = &cf.BodyScanningSettings{
			InspectionMode: config.BodyScanning.InspectionMode,
		}
	}

	if config.BrowserIsolation != nil {
		params.BrowserIsolation = &cf.BrowserIsolationSettings{
			URLBrowserIsolationEnabled: config.BrowserIsolation.URLBrowserIsolationEnabled,
			NonIdentityEnabled:         config.BrowserIsolation.NonIdentityEnabled,
		}
	}

	if config.FIPS != nil {
		params.FIPS = &cf.FIPSSettings{
			TLS: config.FIPS.TLS,
		}
	}

	if config.ProtocolDetection != nil {
		params.ProtocolDetection = &cf.ProtocolDetectionSettings{
			Enabled: config.ProtocolDetection.Enabled,
		}
	}

	if config.CustomCertificate != nil {
		params.CustomCertificate = &cf.CustomCertificateSettings{
			Enabled: config.CustomCertificate.Enabled,
			ID:      config.CustomCertificate.ID,
		}
	}

	logger.Info("Updating Gateway configuration",
		"accountId", accountID)

	result, err := apiClient.UpdateGatewayConfiguration(params)
	if err != nil {
		return nil, fmt.Errorf("update Gateway configuration: %w", err)
	}

	logger.Info("Updated Gateway configuration", "accountId", result.AccountID)

	return &gatewaysvc.GatewayConfigurationSyncResult{
		AccountID: accountID,
	}, nil
}

// handleDeletion handles the deletion of Gateway Configuration.
// Note: Gateway Configuration settings are NOT reset on Cloudflare - they persist.
// This method only cleans up the SyncState resource.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller cleans up SyncState
func (r *ConfigurationController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, ConfigurationFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Note: We intentionally do NOT reset Gateway Configuration on Cloudflare
	// These are account-level settings that should remain even after
	// the GatewayConfiguration resource is deleted from Kubernetes.
	logger.Info("GatewayConfiguration deleted - settings will be preserved on Cloudflare",
		"accountId", syncState.Spec.AccountID)

	// Remove finalizer
	controllerutil.RemoveFinalizer(syncState, ConfigurationFinalizerName)
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
func (r *ConfigurationController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("gateway-configuration-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourceGatewayConfiguration)).
		Complete(r)
}
