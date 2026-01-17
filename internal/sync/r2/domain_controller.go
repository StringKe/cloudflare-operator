// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package r2

//nolint:dupl // Similar patterns across resource types are intentional

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	r2svc "github.com/StringKe/cloudflare-operator/internal/service/r2"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

// DomainController is the Sync Controller for R2 Bucket Custom Domain Configuration.
// It watches CloudflareSyncState resources of type R2BucketDomain,
// extracts the configuration, and syncs to Cloudflare API.
type DomainController struct {
	*common.BaseSyncController
}

// NewDomainController creates a new R2BucketDomainSyncController
func NewDomainController(c client.Client) *DomainController {
	return &DomainController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for R2 bucket domain.
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *DomainController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "R2BucketDomainSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process R2BucketDomain type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceR2BucketDomain {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing R2BucketDomain SyncState",
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

	// Extract R2 bucket domain configuration from first source (1:1 mapping)
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract R2 bucket domain configuration")
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
		logger.Error(err, "Failed to sync R2 bucket domain to Cloudflare")
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

	logger.Info("Successfully synced R2 bucket domain to Cloudflare",
		"domain", config.Domain,
		"enabled", result.Enabled)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts R2 bucket domain configuration from SyncState sources.
// R2 bucket domains have 1:1 mapping, so we use the ExtractFirstSourceConfig helper.
func (*DomainController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*r2svc.R2BucketDomainConfig, error) {
	return common.ExtractFirstSourceConfig[r2svc.R2BucketDomainConfig](syncState)
}

// syncToCloudflare syncs the R2 bucket domain configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *DomainController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *r2svc.R2BucketDomainConfig,
) (*r2svc.R2BucketDomainSyncResult, error) {
	logger := log.FromContext(ctx)

	// Create API client using common helper
	apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
	if err != nil {
		return nil, err
	}

	cloudflareID := syncState.Spec.CloudflareID
	var result *cf.R2CustomDomain

	if common.IsPendingID(cloudflareID) {
		// Check if domain already exists
		existing, err := apiClient.GetR2CustomDomain(ctx, config.BucketName, config.Domain)
		if err != nil && !cf.IsNotFoundError(err) {
			return nil, fmt.Errorf("check existing domain: %w", err)
		}

		if existing != nil {
			// Domain already exists, use it
			result = existing
			logger.Info("Found existing R2 custom domain", "domain", config.Domain)
		} else {
			// Create new domain
			logger.Info("Creating new R2 custom domain",
				"domain", config.Domain,
				"bucketName", config.BucketName)

			params := cf.R2CustomDomainParams{
				Domain:  config.Domain,
				ZoneID:  config.ZoneID,
				MinTLS:  config.MinTLS,
				Enabled: true,
			}

			result, err = apiClient.AttachR2CustomDomain(ctx, config.BucketName, params)
			if err != nil {
				return nil, fmt.Errorf("attach R2 custom domain: %w", err)
			}

			logger.Info("Created R2 custom domain", "domain", config.Domain)
		}

		// Update SyncState with actual domain ID
		common.UpdateCloudflareID(ctx, r.Client, syncState, result.Domain)
	} else {
		// Update existing domain
		logger.Info("Updating existing R2 custom domain",
			"domain", config.Domain,
			"bucketName", config.BucketName)

		// First check if it exists
		existing, err := apiClient.GetR2CustomDomain(ctx, config.BucketName, config.Domain)
		if err != nil {
			if common.HandleNotFoundOnUpdate(err) {
				// Domain was deleted externally, recreate it
				logger.Info("R2 custom domain not found, recreating", "domain", config.Domain)
				params := cf.R2CustomDomainParams{
					Domain:  config.Domain,
					ZoneID:  config.ZoneID,
					MinTLS:  config.MinTLS,
					Enabled: true,
				}
				result, err = apiClient.AttachR2CustomDomain(ctx, config.BucketName, params)
				if err != nil {
					return nil, fmt.Errorf("recreate R2 custom domain: %w", err)
				}
			} else {
				return nil, fmt.Errorf("get R2 custom domain: %w", err)
			}
		} else {
			// Check if update is needed
			needsUpdate := config.MinTLS != existing.MinTLS
			if needsUpdate {
				params := cf.R2CustomDomainParams{
					Domain:  config.Domain,
					MinTLS:  config.MinTLS,
					Enabled: true,
				}
				result, err = apiClient.UpdateR2CustomDomain(ctx, config.BucketName, config.Domain, params)
				if err != nil {
					return nil, fmt.Errorf("update R2 custom domain: %w", err)
				}
				logger.Info("Updated R2 custom domain", "domain", config.Domain)
			} else {
				result = existing
			}
		}
	}

	// Determine enabled status from public access setting
	publicAccessEnabled := false
	if config.EnablePublicAccess != nil {
		publicAccessEnabled = *config.EnablePublicAccess
	}

	return &r2svc.R2BucketDomainSyncResult{
		SyncResult: r2svc.SyncResult{
			ID:        result.Domain,
			AccountID: syncState.Spec.AccountID,
		},
		DomainID:            result.Domain,
		ZoneID:              result.ZoneID,
		Enabled:             result.Enabled,
		MinTLS:              result.MinTLS,
		PublicAccessEnabled: publicAccessEnabled,
		URL:                 fmt.Sprintf("https://%s", config.Domain),
		SSLStatus:           result.Status.SSL,
		OwnershipStatus:     result.Status.Ownership,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DomainController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourceR2BucketDomain)).
		Complete(r)
}
