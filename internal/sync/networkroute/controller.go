// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package networkroute provides the NetworkRoute Sync Controller for managing Cloudflare Tunnel Routes.
// Unlike TunnelConfigSyncController which aggregates multiple sources,
// NetworkRouteSyncController handles individual routes with a 1:1 mapping.
package networkroute

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
	routesvc "github.com/StringKe/cloudflare-operator/internal/service/networkroute"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

// Controller is the Sync Controller for NetworkRoute Configuration.
// It watches CloudflareSyncState resources of type NetworkRoute,
// extracts the configuration, and syncs to Cloudflare API.
type Controller struct {
	*common.BaseSyncController
}

// NewController creates a new NetworkRouteSyncController
func NewController(c client.Client) *Controller {
	return &Controller{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for NetworkRoute.
// The reconciliation flow:
// 1. Get the SyncState resource
// 2. Check for debounce
// 3. Extract NetworkRoute configuration
// 4. Compute hash for change detection
// 5. If changed, sync to Cloudflare API
// 6. Update SyncState status
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "NetworkRouteSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process NetworkRoute type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceNetworkRoute {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing NetworkRoute SyncState",
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

	// Extract NetworkRoute configuration from first source
	// (NetworkRoute has 1:1 mapping, so there should only be one source)
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract NetworkRoute configuration")
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
		logger.Error(err, "Failed to sync NetworkRoute to Cloudflare")
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

	logger.Info("Successfully synced NetworkRoute to Cloudflare",
		"network", result.Network,
		"tunnelId", result.TunnelID)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts NetworkRoute configuration from SyncState sources.
func (*Controller) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*routesvc.NetworkRouteConfig, error) {
	if len(syncState.Spec.Sources) == 0 {
		return nil, errors.New("no sources in SyncState")
	}

	// NetworkRoute has 1:1 mapping, use first source
	source := syncState.Spec.Sources[0]

	config, err := common.ParseSourceConfig[routesvc.NetworkRouteConfig](&source)
	if err != nil {
		return nil, fmt.Errorf("parse NetworkRoute config from source %s: %w", source.Ref.String(), err)
	}

	if config == nil {
		return nil, fmt.Errorf("empty NetworkRoute config from source %s", source.Ref.String())
	}

	return config, nil
}

// syncToCloudflare syncs the NetworkRoute configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *Controller) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *routesvc.NetworkRouteConfig,
) (*routesvc.SyncResult, error) {
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

	// Build NetworkRoute params
	params := cf.TunnelRouteParams{
		Network:          config.Network,
		TunnelID:         config.TunnelID,
		VirtualNetworkID: config.VirtualNetworkID,
		Comment:          config.Comment,
	}

	// Check if this is an existing route or new (pending)
	cloudflareID := syncState.Spec.CloudflareID
	isPending := len(cloudflareID) > 8 && cloudflareID[:8] == "pending-"

	var result *cf.TunnelRouteResult

	if isPending {
		// Create new NetworkRoute
		logger.Info("Creating new NetworkRoute",
			"network", config.Network,
			"tunnelId", config.TunnelID)

		result, err = apiClient.CreateTunnelRoute(params)
		if err != nil {
			return nil, fmt.Errorf("create NetworkRoute: %w", err)
		}

		// Update SyncState CloudflareID with the network (since routes don't have a separate ID)
		syncState.Spec.CloudflareID = config.Network
		if updateErr := r.Client.Update(ctx, syncState); updateErr != nil {
			logger.Error(updateErr, "Failed to update SyncState with route network",
				"network", config.Network)
			// Non-fatal - will be fixed on next reconcile
		}

		logger.Info("Created NetworkRoute",
			"network", result.Network,
			"tunnelId", result.TunnelID)
	} else {
		// Update existing NetworkRoute
		logger.Info("Updating NetworkRoute",
			"network", cloudflareID,
			"tunnelId", config.TunnelID)

		result, err = apiClient.UpdateTunnelRoute(cloudflareID, params)
		if err != nil {
			// Check if route was deleted externally
			if cf.IsNotFoundError(err) {
				// Route deleted externally, recreate it
				logger.Info("NetworkRoute not found, recreating",
					"network", cloudflareID)
				result, err = apiClient.CreateTunnelRoute(params)
				if err != nil {
					return nil, fmt.Errorf("recreate NetworkRoute: %w", err)
				}

				// Update SyncState CloudflareID if network changed
				if result.Network != cloudflareID {
					syncState.Spec.CloudflareID = result.Network
					if updateErr := r.Client.Update(ctx, syncState); updateErr != nil {
						logger.Error(updateErr, "Failed to update SyncState with new route network")
					}
				}
			} else {
				return nil, fmt.Errorf("update NetworkRoute: %w", err)
			}
		}

		logger.Info("Updated NetworkRoute",
			"network", result.Network,
			"tunnelId", result.TunnelID)
	}

	return &routesvc.SyncResult{
		Network:          result.Network,
		TunnelID:         result.TunnelID,
		TunnelName:       result.TunnelName,
		VirtualNetworkID: result.VirtualNetworkID,
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
				return syncState.Spec.ResourceType == v1alpha2.SyncResourceNetworkRoute
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				syncState, ok := e.ObjectNew.(*v1alpha2.CloudflareSyncState)
				if !ok {
					return false
				}
				return syncState.Spec.ResourceType == v1alpha2.SyncResourceNetworkRoute
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				syncState, ok := e.Object.(*v1alpha2.CloudflareSyncState)
				if !ok {
					return false
				}
				return syncState.Spec.ResourceType == v1alpha2.SyncResourceNetworkRoute
			},
		}).
		Complete(r)
}
