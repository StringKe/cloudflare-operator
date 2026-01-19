// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pages

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	pagessvc "github.com/StringKe/cloudflare-operator/internal/service/pages"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

const (
	// DomainFinalizerName is the finalizer for Pages Domain SyncState resources.
	DomainFinalizerName = "pages-domain.sync.cloudflare-operator.io/finalizer"
)

// DomainSyncController is the Sync Controller for Pages Domain Configuration.
// It watches CloudflareSyncState resources of type PagesDomain,
// extracts the configuration, and syncs to Cloudflare API.
// This is the SINGLE point that calls Cloudflare Pages Domain API.
type DomainSyncController struct {
	*common.BaseSyncController
}

// NewDomainSyncController creates a new DomainSyncController
func NewDomainSyncController(c client.Client) *DomainSyncController {
	return &DomainSyncController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for Pages domain.
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *DomainSyncController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "PagesDomainSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process PagesDomain type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourcePagesDomain {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing Pages Domain SyncState",
		"cloudflareId", syncState.Spec.CloudflareID,
		"sources", len(syncState.Spec.Sources))

	// Handle deletion
	if !syncState.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, syncState)
	}

	// Check if there are any sources - if none, delete from Cloudflare
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, deleting from Cloudflare")
		return r.handleDeletion(ctx, syncState)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(syncState, DomainFinalizerName) {
		if err := common.AddFinalizerWithRetry(ctx, r.Client, syncState, DomainFinalizerName); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip if there's a pending debounced request
	if r.Debouncer.IsPending(req.Name) {
		logger.V(1).Info("Skipping reconcile - debounced request pending")
		return ctrl.Result{}, nil
	}

	// Extract Pages domain configuration from first source
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract Pages domain configuration")
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
		logger.V(1).Info("Configuration unchanged, skipping sync",
			"hash", newHash)
		return ctrl.Result{}, nil
	}

	// Set syncing status
	if err := r.SetSyncStatus(ctx, syncState, v1alpha2.SyncStatusSyncing); err != nil {
		return ctrl.Result{}, err
	}

	// Sync to Cloudflare API
	if err := r.syncToCloudflare(ctx, syncState, config); err != nil {
		logger.Error(err, "Failed to sync Pages domain to Cloudflare")
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

	logger.Info("Successfully synced Pages domain to Cloudflare",
		"domain", config.Domain,
		"projectName", config.ProjectName)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts Pages domain configuration from SyncState sources.
func (*DomainSyncController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*pagessvc.PagesDomainConfig, error) {
	return common.ExtractFirstSourceConfig[pagessvc.PagesDomainConfig](syncState)
}

// syncToCloudflare syncs the Pages domain configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *DomainSyncController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *pagessvc.PagesDomainConfig,
) error {
	logger := log.FromContext(ctx)

	// Create API client using common helper
	apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
	if err != nil {
		return err
	}

	// Check if this is a new domain or existing
	cloudflareID := syncState.Spec.CloudflareID

	if common.IsPendingID(cloudflareID) {
		// Add new domain
		logger.Info("Adding new Pages domain",
			"domain", config.Domain,
			"projectName", config.ProjectName)

		result, err := apiClient.AddPagesDomain(ctx, config.ProjectName, config.Domain)
		if err != nil {
			return fmt.Errorf("add Pages domain: %w", err)
		}

		// Update SyncState with actual domain ID
		common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID)

		logger.Info("Added Pages domain",
			"domainId", result.ID,
			"domain", result.Name,
			"status", result.Status)
	} else {
		// Check if domain exists (domains can only be added/deleted, not updated)
		logger.Info("Verifying Pages domain exists",
			"domain", config.Domain,
			"projectName", config.ProjectName)

		_, err := apiClient.GetPagesDomain(ctx, config.ProjectName, config.Domain)
		if err != nil {
			if cf.IsNotFoundError(err) {
				// Domain was deleted externally, recreate it
				logger.Info("Pages domain not found, recreating",
					"domain", config.Domain)
				result, err := apiClient.AddPagesDomain(ctx, config.ProjectName, config.Domain)
				if err != nil {
					return fmt.Errorf("recreate Pages domain: %w", err)
				}

				// Update SyncState with new domain ID
				common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID)
			} else {
				return fmt.Errorf("get Pages domain: %w", err)
			}
		}

		logger.Info("Pages domain verified",
			"domain", config.Domain)
	}

	return nil
}

// handleDeletion handles the deletion of Pages domain from Cloudflare.
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps
func (r *DomainSyncController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, DomainFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Get the Cloudflare domain ID
	cloudflareID := syncState.Spec.CloudflareID

	// Skip if pending ID (domain was never created)
	if common.IsPendingID(cloudflareID) {
		logger.Info("Skipping deletion - Pages domain was never created",
			"cloudflareId", cloudflareID)
	} else if cloudflareID != "" {
		// Extract config to get project name and domain
		config, err := r.extractConfig(syncState)
		if err != nil {
			logger.Error(err, "Failed to extract config for deletion")
			// Continue to remove finalizer even if we can't extract config
		} else {
			// Create API client
			apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
			if err != nil {
				logger.Error(err, "Failed to create API client for deletion")
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}

			logger.Info("Deleting Pages domain from Cloudflare",
				"domain", config.Domain,
				"projectName", config.ProjectName)

			if err := apiClient.DeletePagesDomain(ctx, config.ProjectName, config.Domain); err != nil {
				if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to delete Pages domain from Cloudflare")
					if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
						logger.Error(statusErr, "Failed to update error status")
					}
					return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
				}
				logger.Info("Pages domain already deleted from Cloudflare")
			} else {
				logger.Info("Successfully deleted Pages domain from Cloudflare",
					"domain", config.Domain)
			}
		}
	}

	// Remove finalizer
	if err := common.RemoveFinalizerWithRetry(ctx, r.Client, syncState, DomainFinalizerName); err != nil {
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
func (r *DomainSyncController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("pages-domain-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourcePagesDomain)).
		Complete(r)
}
