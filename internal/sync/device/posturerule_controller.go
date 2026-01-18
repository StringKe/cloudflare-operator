// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package device provides Sync Controllers for managing Cloudflare Device resources.
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
	// PostureRuleFinalizerName is the finalizer for Device Posture Rule SyncState resources.
	PostureRuleFinalizerName = "deviceposturerule.sync.cloudflare-operator.io/finalizer"
)

// PostureRuleController is the Sync Controller for Device Posture Rule Configuration.
type PostureRuleController struct {
	*common.BaseSyncController
}

// NewPostureRuleController creates a new DevicePostureRuleSyncController.
func NewPostureRuleController(c client.Client) *PostureRuleController {
	return &PostureRuleController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for Device Posture Rule.
//
//nolint:revive // cognitive complexity is acceptable for sync controller reconciliation
func (r *PostureRuleController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "DevicePostureRuleSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process DevicePostureRule type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceDevicePostureRule {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing DevicePostureRule SyncState",
		"cloudflareId", syncState.Spec.CloudflareID,
		"sources", len(syncState.Spec.Sources))

	// Handle deletion - this is the SINGLE point for Cloudflare API delete calls
	if !syncState.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, syncState)
	}

	// Check if there are any sources - if none, delete from Cloudflare
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, deleting from Cloudflare")
		return r.handleDeletion(ctx, syncState)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(syncState, PostureRuleFinalizerName) {
		controllerutil.AddFinalizer(syncState, PostureRuleFinalizerName)
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

	// Extract Device Posture Rule configuration from first source
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract Device Posture Rule configuration")
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
		logger.Error(err, "Failed to sync Device Posture Rule to Cloudflare")
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

	logger.Info("Successfully synced Device Posture Rule to Cloudflare",
		"ruleId", result.RuleID)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts Device Posture Rule configuration from SyncState sources.
// Device Posture Rules have 1:1 mapping, so we use the ExtractFirstSourceConfig helper.
func (*PostureRuleController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*devicesvc.DevicePostureRuleConfig, error) {
	return common.ExtractFirstSourceConfig[devicesvc.DevicePostureRuleConfig](syncState)
}

// syncToCloudflare syncs the Device Posture Rule configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *PostureRuleController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *devicesvc.DevicePostureRuleConfig,
) (*devicesvc.DevicePostureRuleSyncResult, error) {
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

	// Build Device Posture Rule params
	params := cf.DevicePostureRuleParams{
		Name:        config.Name,
		Type:        config.Type,
		Description: config.Description,
		Schedule:    config.Schedule,
		Expiration:  config.Expiration,
	}

	// Build match rules
	for _, m := range config.Match {
		params.Match = append(params.Match, cf.DevicePostureMatchParams{
			Platform: m.Platform,
		})
	}

	// Build input
	if config.Input != nil {
		params.Input = r.convertInput(config.Input)
	}

	// Check if this is a new rule (pending) or existing (has real Cloudflare ID)
	cloudflareID := syncState.Spec.CloudflareID
	var result *cf.DevicePostureRuleResult

	if common.IsPendingID(cloudflareID) {
		// Create new rule
		logger.Info("Creating new Device Posture Rule",
			"name", config.Name,
			"type", config.Type)

		result, err = apiClient.CreateDevicePostureRule(params)
		if err != nil {
			return nil, fmt.Errorf("create Device Posture Rule: %w", err)
		}

		// Update SyncState with actual rule ID
		common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID)

		logger.Info("Created Device Posture Rule", "ruleId", result.ID)
	} else {
		// Update existing rule
		logger.Info("Updating Device Posture Rule",
			"ruleId", cloudflareID,
			"name", config.Name)

		result, err = apiClient.UpdateDevicePostureRule(cloudflareID, params)
		if err != nil {
			if common.HandleNotFoundOnUpdate(err) {
				// Rule deleted externally, recreate it
				logger.Info("Device Posture Rule not found, recreating", "ruleId", cloudflareID)
				result, err = apiClient.CreateDevicePostureRule(params)
				if err != nil {
					return nil, fmt.Errorf("recreate Device Posture Rule: %w", err)
				}

				common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID)
			} else {
				return nil, fmt.Errorf("update Device Posture Rule: %w", err)
			}
		}

		logger.Info("Updated Device Posture Rule", "ruleId", result.ID)
	}

	return &devicesvc.DevicePostureRuleSyncResult{
		RuleID:    result.ID,
		AccountID: accountID,
	}, nil
}

// convertInput converts service input to CF API params.
//
//nolint:revive // cognitive complexity is acceptable for this conversion
func (*PostureRuleController) convertInput(input *devicesvc.DevicePostureInput) *cf.DevicePostureInputParams {
	if input == nil {
		return nil
	}

	return &cf.DevicePostureInputParams{
		ID:               input.ID,
		Path:             input.Path,
		Exists:           input.Exists,
		Sha256:           input.Sha256,
		Thumbprint:       input.Thumbprint,
		Running:          input.Running,
		RequireAll:       input.RequireAll,
		Enabled:          input.Enabled,
		Version:          input.Version,
		Operator:         input.Operator,
		Domain:           input.Domain,
		ComplianceStatus: input.ComplianceStatus,
		ConnectionID:     input.ConnectionID,
		LastSeen:         input.LastSeen,
		EidLastSeen:      input.EidLastSeen,
		ActiveThreats:    input.ActiveThreats,
		Infected:         input.Infected,
		IsActive:         input.IsActive,
		NetworkStatus:    input.NetworkStatus,
		SensorConfig:     input.SensorConfig,
		VersionOperator:  input.VersionOperator,
		CountOperator:    input.CountOperator,
		ScoreOperator:    input.ScoreOperator,
		IssueCount:       input.IssueCount,
		Score:            input.Score,
		TotalScore:       input.TotalScore,
		RiskLevel:        input.RiskLevel,
		Overall:          input.Overall,
		State:            input.State,
		OperationalState: input.OperationalState,
		OSDistroName:     input.OSDistroName,
		OSDistroRevision: input.OSDistroRevision,
		OSVersionExtra:   input.OSVersionExtra,
		OS:               input.OS,
		OperatingSystem:  input.OperatingSystem,
		CertificateID:    input.CertificateID,
		CommonName:       input.CommonName,
		Cn:               input.Cn,
		CheckPrivateKey:  input.CheckPrivateKey,
		ExtendedKeyUsage: input.ExtendedKeyUsage,
		CheckDisks:       input.CheckDisks,
	}
}

// handleDeletion handles the deletion of Device Posture Rule from Cloudflare.
// This is the SINGLE point for Cloudflare Device Posture Rule deletion in the system.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller deletes from Cloudflare
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps
func (r *PostureRuleController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, PostureRuleFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Get the Cloudflare rule ID
	cloudflareID := syncState.Spec.CloudflareID

	// Skip if pending ID (rule was never created)
	if common.IsPendingID(cloudflareID) {
		logger.Info("Skipping deletion - Device Posture Rule was never created",
			"cloudflareId", cloudflareID)
	} else if cloudflareID != "" {
		// Delete from Cloudflare
		apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
		if err != nil {
			logger.Error(err, "Failed to create API client for deletion")
			return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
		}

		logger.Info("Deleting Device Posture Rule from Cloudflare",
			"ruleId", cloudflareID)

		if err := apiClient.DeleteDevicePostureRule(cloudflareID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete Device Posture Rule from Cloudflare")
				if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
					logger.Error(statusErr, "Failed to update error status")
				}
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}
			logger.Info("Device Posture Rule already deleted from Cloudflare")
		} else {
			logger.Info("Successfully deleted Device Posture Rule from Cloudflare",
				"ruleId", cloudflareID)
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(syncState, PostureRuleFinalizerName)
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
func (r *PostureRuleController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("device-posturerule-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourceDevicePostureRule)).
		Complete(r)
}
