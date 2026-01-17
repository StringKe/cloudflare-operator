// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package virtualnetwork provides the VirtualNetwork Sync Controller for managing Cloudflare Virtual Networks.
// Unlike TunnelConfigSyncController which aggregates multiple sources,
// VirtualNetworkSyncController handles individual Virtual Networks with a 1:1 mapping.
//
// Unified Sync Architecture Flow:
// K8s Resources → Resource Controllers → Core Services → SyncState CRD → Sync Controllers → Cloudflare API
//
// This Sync Controller is the SINGLE point that calls Cloudflare API for Virtual Networks.
// It handles create, update, and delete operations based on SyncState changes.
package virtualnetwork

import (
	"context"
	"errors"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	vnetsvc "github.com/StringKe/cloudflare-operator/internal/service/virtualnetwork"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

const (
	// FinalizerName is the finalizer for VirtualNetwork SyncState resources.
	// This ensures we delete the VirtualNetwork from Cloudflare before removing SyncState.
	FinalizerName = "virtualnetwork.sync.cloudflare-operator.io/finalizer"
)

// Controller is the Sync Controller for VirtualNetwork Configuration.
// It watches CloudflareSyncState resources of type VirtualNetwork,
// extracts the configuration, and syncs to Cloudflare API.
// This is the SINGLE point that calls Cloudflare VirtualNetwork API.
type Controller struct {
	*common.BaseSyncController
}

// NewController creates a new VirtualNetworkSyncController
func NewController(c client.Client) *Controller {
	return &Controller{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for VirtualNetwork.
// Following Unified Sync Architecture:
// K8s Resources → Resource Controllers → Core Services → SyncState CRD → Sync Controllers → Cloudflare API
//
// The reconciliation flow:
// 1. Get the SyncState resource
// 2. Handle deletion (if being deleted or no sources)
// 3. Add finalizer for cleanup
// 4. Check for debounce
// 5. Extract VirtualNetwork configuration
// 6. Compute hash for change detection
// 7. If changed, sync to Cloudflare API
// 8. Update SyncState status
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "VirtualNetworkSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process VirtualNetwork type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceVirtualNetwork {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing VirtualNetwork SyncState",
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
	if !controllerutil.ContainsFinalizer(syncState, FinalizerName) {
		controllerutil.AddFinalizer(syncState, FinalizerName)
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

	// Extract VirtualNetwork configuration from first source
	// (VirtualNetwork has 1:1 mapping, so there should only be one source)
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract VirtualNetwork configuration")
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
	result, err := r.syncToCloudflare(ctx, syncState, config)
	if err != nil {
		logger.Error(err, "Failed to sync VirtualNetwork to Cloudflare")
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

	logger.Info("Successfully synced VirtualNetwork to Cloudflare",
		"vnetId", result.VirtualNetworkID,
		"name", result.Name)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts VirtualNetwork configuration from SyncState sources.
func (*Controller) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*vnetsvc.VirtualNetworkConfig, error) {
	if len(syncState.Spec.Sources) == 0 {
		return nil, errors.New("no sources in SyncState")
	}

	// VirtualNetwork has 1:1 mapping, use first source
	source := syncState.Spec.Sources[0]

	config, err := common.ParseSourceConfig[vnetsvc.VirtualNetworkConfig](&source)
	if err != nil {
		return nil, fmt.Errorf("parse VirtualNetwork config from source %s: %w", source.Ref.String(), err)
	}

	if config == nil {
		return nil, fmt.Errorf("empty VirtualNetwork config from source %s", source.Ref.String())
	}

	return config, nil
}

// syncToCloudflare syncs the VirtualNetwork configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *Controller) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *vnetsvc.VirtualNetworkConfig,
) (*vnetsvc.SyncResult, error) {
	logger := log.FromContext(ctx)

	// Convert CredentialsReference to CloudflareCredentialsRef for API client
	credRef := &v1alpha2.CloudflareCredentialsRef{
		Name: syncState.Spec.CredentialsRef.Name,
	}

	// Load credentials and create API client
	apiClient, err := cf.NewAPIClientFromCredentialsRef(ctx, r.Client, credRef)
	if err != nil {
		return nil, fmt.Errorf("create API client: %w", err)
	}

	// Set account ID from spec
	if syncState.Spec.AccountID != "" {
		apiClient.ValidAccountId = syncState.Spec.AccountID
	}

	// Build VirtualNetwork params
	params := cf.VirtualNetworkParams{
		Name:             config.Name,
		Comment:          config.Comment,
		IsDefaultNetwork: config.IsDefaultNetwork,
	}

	// Check if this is an existing network (has real Cloudflare ID) or new
	cloudflareID := syncState.Spec.CloudflareID
	isPending := len(cloudflareID) > 8 && cloudflareID[:8] == "pending-"

	var result *cf.VirtualNetworkResult

	if isPending {
		// Create new VirtualNetwork
		logger.Info("Creating new VirtualNetwork",
			"name", config.Name)

		result, err = apiClient.CreateVirtualNetwork(params)
		if err != nil {
			return nil, fmt.Errorf("create VirtualNetwork: %w", err)
		}

		// Update SyncState with actual VirtualNetwork ID
		syncState.Spec.CloudflareID = result.ID
		if updateErr := r.Client.Update(ctx, syncState); updateErr != nil {
			logger.Error(updateErr, "Failed to update SyncState with VirtualNetwork ID",
				"vnetId", result.ID)
			// Non-fatal - will be fixed on next reconcile
		}

		logger.Info("Created VirtualNetwork",
			"vnetId", result.ID,
			"name", result.Name)
	} else {
		// Update existing VirtualNetwork
		logger.Info("Updating VirtualNetwork",
			"vnetId", cloudflareID,
			"name", config.Name)

		result, err = apiClient.UpdateVirtualNetwork(cloudflareID, params)
		if err != nil {
			// Check if VirtualNetwork was deleted externally
			if cf.IsNotFoundError(err) {
				// VirtualNetwork deleted externally, recreate it
				logger.Info("VirtualNetwork not found, recreating",
					"vnetId", cloudflareID)
				result, err = apiClient.CreateVirtualNetwork(params)
				if err != nil {
					return nil, fmt.Errorf("recreate VirtualNetwork: %w", err)
				}

				// Update SyncState with new VirtualNetwork ID
				syncState.Spec.CloudflareID = result.ID
				if updateErr := r.Client.Update(ctx, syncState); updateErr != nil {
					logger.Error(updateErr, "Failed to update SyncState with new VirtualNetwork ID")
				}
			} else {
				return nil, fmt.Errorf("update VirtualNetwork: %w", err)
			}
		}

		logger.Info("Updated VirtualNetwork",
			"vnetId", result.ID,
			"name", result.Name)
	}

	return &vnetsvc.SyncResult{
		VirtualNetworkID: result.ID,
		Name:             result.Name,
		IsDefault:        result.IsDefaultNetwork,
	}, nil
}

// handleDeletion handles the deletion of VirtualNetwork from Cloudflare.
// This is the SINGLE point for Cloudflare VirtualNetwork deletion in the system.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller deletes from Cloudflare
//
// Note: VirtualNetwork deletion requires deleting associated routes first to avoid
// "virtual network is used by IP Route(s)" error.
func (r *Controller) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Get the Cloudflare VirtualNetwork ID
	cloudflareID := syncState.Spec.CloudflareID

	// Skip if pending ID (VirtualNetwork was never created)
	isPending := len(cloudflareID) > 8 && cloudflareID[:8] == "pending-"
	if isPending {
		logger.Info("Skipping deletion - VirtualNetwork was never created",
			"cloudflareId", cloudflareID)
	} else if cloudflareID != "" {
		// Create API client
		credRef := &v1alpha2.CloudflareCredentialsRef{
			Name: syncState.Spec.CredentialsRef.Name,
		}
		apiClient, err := cf.NewAPIClientFromCredentialsRef(ctx, r.Client, credRef)
		if err != nil {
			logger.Error(err, "Failed to create API client for deletion")
			return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
		}

		// Set account ID
		if syncState.Spec.AccountID != "" {
			apiClient.ValidAccountId = syncState.Spec.AccountID
		}

		// Delete all routes associated with this VirtualNetwork FIRST
		// This prevents the "virtual network is used by IP Route(s)" error
		deletedCount, err := apiClient.DeleteTunnelRoutesByVirtualNetworkID(cloudflareID)
		if err != nil {
			logger.Error(err, "Failed to delete routes for VirtualNetwork", "vnetId", cloudflareID)
			if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
				logger.Error(statusErr, "Failed to update error status")
			}
			return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
		}
		if deletedCount > 0 {
			logger.Info("Deleted routes before VirtualNetwork deletion",
				"vnetId", cloudflareID, "count", deletedCount)
		}

		// Now delete the VirtualNetwork
		logger.Info("Deleting VirtualNetwork from Cloudflare",
			"vnetId", cloudflareID)

		if err := apiClient.DeleteVirtualNetwork(cloudflareID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete VirtualNetwork from Cloudflare")
				if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
					logger.Error(statusErr, "Failed to update error status")
				}
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}
			logger.Info("VirtualNetwork already deleted from Cloudflare")
		} else {
			logger.Info("Successfully deleted VirtualNetwork from Cloudflare",
				"vnetId", cloudflareID)
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(syncState, FinalizerName)
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
func (r *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				syncState, ok := e.Object.(*v1alpha2.CloudflareSyncState)
				if !ok {
					return false
				}
				return syncState.Spec.ResourceType == v1alpha2.SyncResourceVirtualNetwork
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				syncState, ok := e.ObjectNew.(*v1alpha2.CloudflareSyncState)
				if !ok {
					return false
				}
				return syncState.Spec.ResourceType == v1alpha2.SyncResourceVirtualNetwork
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				syncState, ok := e.Object.(*v1alpha2.CloudflareSyncState)
				if !ok {
					return false
				}
				return syncState.Spec.ResourceType == v1alpha2.SyncResourceVirtualNetwork
			},
		}).
		Complete(r)
}
