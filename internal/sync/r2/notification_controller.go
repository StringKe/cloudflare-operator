// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package r2

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
	r2svc "github.com/StringKe/cloudflare-operator/internal/service/r2"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

const (
	// NotificationFinalizerName is the finalizer for R2BucketNotification SyncState resources.
	NotificationFinalizerName = "r2bucketnotification.sync.cloudflare-operator.io/finalizer"
)

// NotificationController is the Sync Controller for R2 Bucket Notification Configuration.
// It watches CloudflareSyncState resources of type R2BucketNotification,
// extracts the configuration, and syncs to Cloudflare API.
type NotificationController struct {
	*common.BaseSyncController
}

// NewNotificationController creates a new R2BucketNotificationSyncController
func NewNotificationController(c client.Client) *NotificationController {
	return &NotificationController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for R2 bucket notification.
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *NotificationController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "R2BucketNotificationSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process R2BucketNotification type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceR2BucketNotification {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing R2BucketNotification SyncState",
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
	if !controllerutil.ContainsFinalizer(syncState, NotificationFinalizerName) {
		controllerutil.AddFinalizer(syncState, NotificationFinalizerName)
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

	// Extract R2 bucket notification configuration from first source (1:1 mapping)
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract R2 bucket notification configuration")
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
		logger.Error(err, "Failed to sync R2 bucket notification to Cloudflare")
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

	logger.Info("Successfully synced R2 bucket notification to Cloudflare",
		"queueId", result.QueueID,
		"ruleCount", result.RuleCount)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts R2 bucket notification configuration from SyncState sources.
// R2 bucket notifications have 1:1 mapping, so we use the ExtractFirstSourceConfig helper.
func (*NotificationController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*r2svc.R2BucketNotificationConfig, error) {
	return common.ExtractFirstSourceConfig[r2svc.R2BucketNotificationConfig](syncState)
}

// syncToCloudflare syncs the R2 bucket notification configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *NotificationController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *r2svc.R2BucketNotificationConfig,
) (*r2svc.R2BucketNotificationSyncResult, error) {
	logger := log.FromContext(ctx)

	// Create API client using common helper
	apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
	if err != nil {
		return nil, err
	}

	// Resolve Queue ID from queue name
	queueID, err := r.resolveQueueID(ctx, apiClient, syncState, config.QueueName)
	if err != nil {
		return nil, fmt.Errorf("resolve queue ID: %w", err)
	}

	// Convert spec rules to API rules
	rules := make([]cf.R2NotificationRule, len(config.Rules))
	for i, rule := range config.Rules {
		eventTypes := make([]string, len(rule.EventTypes))
		for j, et := range rule.EventTypes {
			eventTypes[j] = string(et)
		}
		rules[i] = cf.R2NotificationRule{
			Prefix:      rule.Prefix,
			Suffix:      rule.Suffix,
			EventTypes:  eventTypes,
			Description: rule.Description,
		}
	}

	// Set notification configuration
	logger.Info("Setting R2 notification configuration",
		"bucketName", config.BucketName,
		"queueId", queueID,
		"ruleCount", len(rules))

	if err := apiClient.SetR2Notification(ctx, config.BucketName, queueID, rules); err != nil {
		return nil, fmt.Errorf("set R2 notification: %w", err)
	}

	logger.Info("Successfully configured R2 notification",
		"bucketName", config.BucketName,
		"queueId", queueID)

	return &r2svc.R2BucketNotificationSyncResult{
		SyncResult: r2svc.SyncResult{
			ID:        queueID,
			AccountID: syncState.Spec.AccountID,
		},
		QueueID:   queueID,
		RuleCount: len(rules),
	}, nil
}

// resolveQueueID resolves the queue name to a queue ID.
func (r *NotificationController) resolveQueueID(
	ctx context.Context,
	apiClient *cf.API,
	syncState *v1alpha2.CloudflareSyncState,
	queueName string,
) (string, error) {
	cloudflareID := syncState.Spec.CloudflareID

	// If we already have a resolved queue ID, use it
	if !common.IsPendingID(cloudflareID) && cloudflareID != "" {
		return cloudflareID, nil
	}

	// Get queue ID from Cloudflare
	queueID, err := apiClient.GetQueueID(ctx, queueName)
	if err != nil {
		return "", fmt.Errorf("get queue ID for %s: %w", queueName, err)
	}

	// Update SyncState with actual queue ID if it was pending
	if common.IsPendingID(cloudflareID) {
		common.UpdateCloudflareID(ctx, r.Client, syncState, queueID)
	}

	return queueID, nil
}

// handleDeletion handles the deletion of R2BucketNotification from Cloudflare.
// This is the SINGLE point for Cloudflare R2BucketNotification deletion in the system.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller deletes from Cloudflare
func (r *NotificationController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, NotificationFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Get the queue ID (CloudflareID)
	queueID := syncState.Spec.CloudflareID

	// Skip if pending ID (notification was never created)
	if common.IsPendingID(queueID) {
		logger.Info("Skipping deletion - R2BucketNotification was never created",
			"cloudflareId", queueID)
	} else if queueID != "" {
		// Extract config to get bucket name
		config, err := r.extractConfig(syncState)
		if err == nil && config.BucketName != "" {
			// Create API client
			apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
			if err != nil {
				logger.Error(err, "Failed to create API client for deletion")
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}

			logger.Info("Deleting R2BucketNotification from Cloudflare",
				"queueId", queueID,
				"bucketName", config.BucketName)

			if err := apiClient.DeleteR2Notification(ctx, config.BucketName, queueID); err != nil {
				if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to delete R2BucketNotification from Cloudflare")
					if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
						logger.Error(statusErr, "Failed to update error status")
					}
					return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
				}
				logger.Info("R2BucketNotification already deleted from Cloudflare")
			} else {
				logger.Info("Successfully deleted R2BucketNotification from Cloudflare",
					"queueId", queueID)
			}
		} else {
			logger.Info("Cannot delete R2BucketNotification - missing bucket name in config")
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(syncState, NotificationFinalizerName)
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
func (r *NotificationController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourceR2BucketNotification)).
		Complete(r)
}
