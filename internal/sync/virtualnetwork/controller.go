// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package virtualnetwork provides the VirtualNetwork Sync Controller for managing Cloudflare Virtual Networks.
// Unlike TunnelConfigSyncController which aggregates multiple sources,
// VirtualNetworkSyncController handles individual Virtual Networks with a 1:1 mapping.
package virtualnetwork

import (
	"context"
	"errors"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	vnetsvc "github.com/StringKe/cloudflare-operator/internal/service/virtualnetwork"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

// Controller is the Sync Controller for VirtualNetwork Configuration.
// It watches CloudflareSyncState resources of type VirtualNetwork,
// extracts the configuration, and syncs to Cloudflare API.
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
// The reconciliation flow:
// 1. Get the SyncState resource
// 2. Check for debounce
// 3. Extract VirtualNetwork configuration
// 4. Compute hash for change detection
// 5. If changed, sync to Cloudflare API
// 6. Update SyncState status
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
