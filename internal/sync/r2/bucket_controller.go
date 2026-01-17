// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package r2 provides sync controllers for managing Cloudflare R2 resources.
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
	// BucketFinalizerName is the finalizer for R2Bucket SyncState resources.
	BucketFinalizerName = "r2bucket.sync.cloudflare-operator.io/finalizer"
)

// BucketController is the Sync Controller for R2 Bucket Configuration.
// It watches CloudflareSyncState resources of type R2Bucket,
// extracts the configuration, and syncs to Cloudflare API.
type BucketController struct {
	*common.BaseSyncController
}

// NewBucketController creates a new R2BucketSyncController
func NewBucketController(c client.Client) *BucketController {
	return &BucketController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for R2 bucket.
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *BucketController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "R2BucketSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process R2Bucket type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceR2Bucket {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing R2Bucket SyncState",
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
	if !controllerutil.ContainsFinalizer(syncState, BucketFinalizerName) {
		controllerutil.AddFinalizer(syncState, BucketFinalizerName)
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

	// Extract R2 bucket configuration from first source (1:1 mapping)
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract R2 bucket configuration")
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
		logger.Error(err, "Failed to sync R2 bucket to Cloudflare")
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

	logger.Info("Successfully synced R2 bucket to Cloudflare",
		"bucketName", result.BucketName,
		"location", result.Location)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts R2 bucket configuration from SyncState sources.
// R2 buckets have 1:1 mapping, so we use the ExtractFirstSourceConfig helper.
func (*BucketController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*r2svc.R2BucketConfig, error) {
	return common.ExtractFirstSourceConfig[r2svc.R2BucketConfig](syncState)
}

// syncToCloudflare syncs the R2 bucket configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *BucketController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *r2svc.R2BucketConfig,
) (*r2svc.R2BucketSyncResult, error) {
	logger := log.FromContext(ctx)

	// Create API client using common helper
	apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
	if err != nil {
		return nil, err
	}

	cloudflareID := syncState.Spec.CloudflareID
	var bucketName string
	var location string

	if common.IsPendingID(cloudflareID) {
		// Create new bucket
		bucketName = config.Name
		logger.Info("Creating new R2 bucket",
			"bucketName", bucketName,
			"locationHint", config.LocationHint)

		bucket, err := apiClient.CreateR2Bucket(ctx, cf.R2BucketParams{
			Name:         bucketName,
			LocationHint: config.LocationHint,
		})
		if err != nil {
			// Check if bucket already exists (conflict)
			if cf.IsConflictError(err) {
				// Try to get existing bucket
				existing, getErr := apiClient.GetR2Bucket(ctx, bucketName)
				if getErr == nil {
					// Adopt existing bucket
					bucket = existing
					logger.Info("Adopted existing R2 bucket", "bucketName", bucketName)
				} else {
					return nil, fmt.Errorf("create R2 bucket (conflict, but can't adopt): %w", err)
				}
			} else {
				return nil, fmt.Errorf("create R2 bucket: %w", err)
			}
		}

		bucketName = bucket.Name
		location = bucket.Location

		// Update SyncState with actual bucket name
		common.UpdateCloudflareID(ctx, r.Client, syncState, bucketName)

		logger.Info("Created R2 bucket", "bucketName", bucketName, "location", location)
	} else {
		// Verify existing bucket
		bucketName = cloudflareID
		logger.Info("Verifying existing R2 bucket", "bucketName", bucketName)

		bucket, err := apiClient.GetR2Bucket(ctx, bucketName)
		if err != nil {
			if common.HandleNotFoundOnUpdate(err) {
				// Bucket was deleted externally, recreate it
				logger.Info("R2 bucket not found, recreating", "bucketName", bucketName)
				bucket, err = apiClient.CreateR2Bucket(ctx, cf.R2BucketParams{
					Name:         config.Name,
					LocationHint: config.LocationHint,
				})
				if err != nil {
					return nil, fmt.Errorf("recreate R2 bucket: %w", err)
				}
				bucketName = bucket.Name
			} else {
				return nil, fmt.Errorf("get R2 bucket: %w", err)
			}
		}
		location = bucket.Location
	}

	// Sync CORS configuration
	corsCount, err := r.syncCORS(ctx, apiClient, bucketName, config)
	if err != nil {
		return nil, fmt.Errorf("sync CORS: %w", err)
	}

	// Sync Lifecycle configuration
	lifecycleCount, err := r.syncLifecycle(ctx, apiClient, bucketName, config)
	if err != nil {
		return nil, fmt.Errorf("sync lifecycle: %w", err)
	}

	return &r2svc.R2BucketSyncResult{
		SyncResult: r2svc.SyncResult{
			ID:        bucketName,
			AccountID: syncState.Spec.AccountID,
		},
		BucketName:          bucketName,
		Location:            location,
		CORSRulesCount:      corsCount,
		LifecycleRulesCount: lifecycleCount,
	}, nil
}

// syncCORS synchronizes CORS configuration for the bucket.
//
//nolint:revive // cognitive complexity is acceptable for CORS sync logic
func (r *BucketController) syncCORS(
	ctx context.Context,
	apiClient *cf.API,
	bucketName string,
	config *r2svc.R2BucketConfig,
) (int, error) {
	logger := log.FromContext(ctx)

	// If no CORS rules specified, delete existing CORS config
	if len(config.CORS) == 0 {
		existing, err := apiClient.GetR2CORS(ctx, bucketName)
		if err != nil && !cf.IsNotFoundError(err) {
			return 0, fmt.Errorf("get CORS: %w", err)
		}
		if len(existing) > 0 {
			if err := apiClient.DeleteR2CORS(ctx, bucketName); err != nil && !cf.IsNotFoundError(err) {
				return 0, fmt.Errorf("delete CORS: %w", err)
			}
			logger.Info("CORS configuration deleted", "bucket", bucketName)
		}
		return 0, nil
	}

	// Convert spec rules to API rules
	rules := make([]cf.R2CORSRule, len(config.CORS))
	for i, rule := range config.CORS {
		rules[i] = cf.R2CORSRule{
			ID:             rule.ID,
			AllowedOrigins: rule.AllowedOrigins,
			AllowedMethods: rule.AllowedMethods,
			AllowedHeaders: rule.AllowedHeaders,
			ExposeHeaders:  rule.ExposeHeaders,
			MaxAgeSeconds:  rule.MaxAgeSeconds,
		}
	}

	// Set CORS configuration
	if err := apiClient.SetR2CORS(ctx, bucketName, rules); err != nil {
		return 0, fmt.Errorf("set CORS: %w", err)
	}

	logger.Info("CORS configuration updated", "bucket", bucketName, "rulesCount", len(rules))
	return len(rules), nil
}

// syncLifecycle synchronizes lifecycle rules for the bucket.
//
//nolint:revive // cognitive complexity is acceptable for lifecycle sync logic
func (r *BucketController) syncLifecycle(
	ctx context.Context,
	apiClient *cf.API,
	bucketName string,
	config *r2svc.R2BucketConfig,
) (int, error) {
	logger := log.FromContext(ctx)

	// If no lifecycle rules specified, delete existing lifecycle config
	if config.Lifecycle == nil || len(config.Lifecycle.Rules) == 0 {
		existing, err := apiClient.GetR2Lifecycle(ctx, bucketName)
		if err != nil && !cf.IsNotFoundError(err) {
			return 0, fmt.Errorf("get lifecycle: %w", err)
		}
		if len(existing) > 0 {
			if err := apiClient.DeleteR2Lifecycle(ctx, bucketName); err != nil && !cf.IsNotFoundError(err) {
				return 0, fmt.Errorf("delete lifecycle: %w", err)
			}
			logger.Info("Lifecycle configuration deleted", "bucket", bucketName)
		}
		return 0, nil
	}

	// Convert spec rules to API rules
	rules := make([]cf.R2LifecycleRule, len(config.Lifecycle.Rules))
	for i, rule := range config.Lifecycle.Rules {
		apiRule := cf.R2LifecycleRule{
			ID:      rule.ID,
			Enabled: rule.Enabled,
			Prefix:  rule.Prefix,
		}

		if rule.Expiration != nil {
			apiRule.Expiration = &cf.R2LifecycleExpiration{
				Days: rule.Expiration.Days,
				Date: rule.Expiration.Date,
			}
		}

		if rule.AbortIncompleteMultipartUpload != nil {
			apiRule.AbortIncompleteMultipartUpload = &cf.R2LifecycleAbortUpload{
				DaysAfterInitiation: rule.AbortIncompleteMultipartUpload.DaysAfterInitiation,
			}
		}

		rules[i] = apiRule
	}

	// Set lifecycle configuration
	if err := apiClient.SetR2Lifecycle(ctx, bucketName, rules); err != nil {
		return 0, fmt.Errorf("set lifecycle: %w", err)
	}

	logger.Info("Lifecycle configuration updated", "bucket", bucketName, "rulesCount", len(rules))
	return len(rules), nil
}

// handleDeletion handles the deletion of R2Bucket from Cloudflare.
// This is the SINGLE point for Cloudflare R2Bucket deletion in the system.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller deletes from Cloudflare
//
//nolint:revive // cognitive complexity is acceptable for deletion handling
func (r *BucketController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, BucketFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Get the bucket name (CloudflareID)
	bucketName := syncState.Spec.CloudflareID

	// Skip if pending ID (bucket was never created)
	if common.IsPendingID(bucketName) {
		logger.Info("Skipping deletion - R2Bucket was never created",
			"cloudflareId", bucketName)
	} else if bucketName != "" {
		// Check if we should orphan the resource
		shouldDelete := true

		// Try to extract config to check DeletionPolicy
		if len(syncState.Spec.Sources) > 0 {
			config, err := r.extractConfig(syncState)
			if err == nil && config.Lifecycle != nil && config.Lifecycle.DeletionPolicy == "Orphan" {
				logger.Info("Deletion policy is Orphan, skipping bucket deletion",
					"bucketName", bucketName)
				shouldDelete = false
			}
		}

		if shouldDelete {
			// Create API client
			apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
			if err != nil {
				logger.Error(err, "Failed to create API client for deletion")
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}

			logger.Info("Deleting R2Bucket from Cloudflare",
				"bucketName", bucketName)

			if err := apiClient.DeleteR2Bucket(ctx, bucketName); err != nil {
				if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to delete R2Bucket from Cloudflare")
					if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
						logger.Error(statusErr, "Failed to update error status")
					}
					return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
				}
				logger.Info("R2Bucket already deleted from Cloudflare")
			} else {
				logger.Info("Successfully deleted R2Bucket from Cloudflare",
					"bucketName", bucketName)
			}
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(syncState, BucketFinalizerName)
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
func (r *BucketController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourceR2Bucket)).
		Complete(r)
}
