// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package networkroute provides the NetworkRoute Sync Controller for managing Cloudflare Tunnel Routes.
// Unlike TunnelConfigSyncController which aggregates multiple sources,
// NetworkRouteSyncController handles individual routes with a 1:1 mapping.
//
// Unified Sync Architecture Flow:
// K8s Resources → Resource Controllers → Core Services → SyncState CRD → Sync Controllers → Cloudflare API
//
// This Sync Controller is the SINGLE point that calls Cloudflare API for Network Routes.
// It handles create, update, and delete operations based on SyncState changes.
package networkroute

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
	routesvc "github.com/StringKe/cloudflare-operator/internal/service/networkroute"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

const (
	// FinalizerName is the finalizer for NetworkRoute SyncState resources.
	// This ensures we delete the NetworkRoute from Cloudflare before removing SyncState.
	FinalizerName = "networkroute.sync.cloudflare-operator.io/finalizer"
)

// Controller is the Sync Controller for NetworkRoute Configuration.
// It watches CloudflareSyncState resources of type NetworkRoute,
// extracts the configuration, and syncs to Cloudflare API.
// This is the SINGLE point that calls Cloudflare NetworkRoute API.
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
// Following Unified Sync Architecture:
// K8s Resources → Resource Controllers → Core Services → SyncState CRD → Sync Controllers → Cloudflare API
//
// The reconciliation flow:
// 1. Get the SyncState resource
// 2. Handle deletion (if being deleted or no sources)
// 3. Add finalizer for cleanup
// 4. Check for debounce
// 5. Extract NetworkRoute configuration
// 6. Compute hash for change detection
// 7. If changed, sync to Cloudflare API
// 8. Update SyncState status
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

	// Handle deletion - this is the SINGLE point for Cloudflare API delete calls
	if !syncState.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, syncState)
	}

	// Check if there are any sources - if none, delete from Cloudflare
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, deleting from Cloudflare")
		return r.handleDeletion(ctx, syncState)
	}

	// Add finalizer if not present (with conflict retry)
	if !controllerutil.ContainsFinalizer(syncState, FinalizerName) {
		if err := common.AddFinalizerWithRetry(ctx, r.Client, syncState, FinalizerName); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip if there's a pending debounced request
	if r.Debouncer.IsPending(req.Name) {
		logger.V(1).Info("Skipping reconcile - debounced request pending")
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
		// Search for existing route across all Virtual Networks
		// This allows adopting routes that were created externally or by other resources.
		// Note: Empty VirtualNetworkID in ListTunnelRoutesByNetwork searches all VNets.
		existingRoutes, searchErr := apiClient.ListTunnelRoutesByNetwork(ctx, config.Network)

		var existing *cf.TunnelRouteResult
		if searchErr == nil && len(existingRoutes) > 0 {
			// Found existing route(s). If VNetID specified, find matching one.
			// Otherwise, use the first one (typically on default VNet).
			if config.VirtualNetworkID != "" {
				for i := range existingRoutes {
					if existingRoutes[i].VirtualNetworkID == config.VirtualNetworkID {
						existing = &existingRoutes[i]
						break
					}
				}
			}
			// If no VNetID specified or no match found, use first result
			if existing == nil {
				existing = &existingRoutes[0]
			}
		}

		if existing != nil {
			// Route exists, adopt it and update if needed
			logger.Info("Found existing NetworkRoute, adopting",
				"network", config.Network,
				"existingTunnelId", existing.TunnelID,
				"existingVNetId", existing.VirtualNetworkID)
			result = existing

			// Preserve the existing VirtualNetworkID if not explicitly specified
			if config.VirtualNetworkID == "" && existing.VirtualNetworkID != "" {
				params.VirtualNetworkID = existing.VirtualNetworkID
			}

			// If the existing route points to a different tunnel, update it
			if existing.TunnelID != config.TunnelID {
				logger.Info("Updating adopted NetworkRoute to point to correct tunnel",
					"network", config.Network,
					"oldTunnelId", existing.TunnelID,
					"newTunnelId", config.TunnelID)
				result, err = apiClient.UpdateTunnelRoute(ctx, config.Network, params)
				if err != nil {
					return nil, fmt.Errorf("update adopted NetworkRoute: %w", err)
				}
			}
		} else {
			// Create new NetworkRoute
			// If VirtualNetworkID not specified, Cloudflare will use the default VNet
			logger.Info("Creating new NetworkRoute",
				"network", config.Network,
				"tunnelId", config.TunnelID,
				"virtualNetworkId", config.VirtualNetworkID)

			result, err = apiClient.CreateTunnelRoute(ctx, params)
			if err != nil {
				// Handle conflict error (route already exists but wasn't found)
				if cf.IsConflictError(err) {
					logger.Info("NetworkRoute already exists (conflict), attempting to adopt",
						"network", config.Network)
					// Search again to find the conflicting route
					conflictRoutes, searchErr := apiClient.ListTunnelRoutesByNetwork(ctx, config.Network)
					if searchErr != nil || len(conflictRoutes) == 0 {
						return nil, fmt.Errorf("get existing NetworkRoute after conflict: %w", searchErr)
					}
					// Use the first matching route
					conflicting := &conflictRoutes[0]
					// Preserve VNetID for update
					if config.VirtualNetworkID == "" && conflicting.VirtualNetworkID != "" {
						params.VirtualNetworkID = conflicting.VirtualNetworkID
					}
					// Update to ensure consistency
					result, err = apiClient.UpdateTunnelRoute(ctx, config.Network, params)
					if err != nil {
						return nil, fmt.Errorf("update NetworkRoute after conflict: %w", err)
					}
				} else {
					return nil, fmt.Errorf("create NetworkRoute: %w", err)
				}
			}

			logger.Info("Created NetworkRoute",
				"network", result.Network,
				"tunnelId", result.TunnelID,
				"virtualNetworkId", result.VirtualNetworkID)
		}

		// Update SyncState CloudflareID with the network (since routes don't have a separate ID)
		syncState.Spec.CloudflareID = config.Network
		if updateErr := r.Client.Update(ctx, syncState); updateErr != nil {
			logger.Error(updateErr, "Failed to update SyncState with route network",
				"network", config.Network)
			// Non-fatal - will be fixed on next reconcile
		}
	} else {
		// Update existing NetworkRoute
		logger.Info("Updating NetworkRoute",
			"network", cloudflareID,
			"tunnelId", config.TunnelID)

		result, err = apiClient.UpdateTunnelRoute(ctx, cloudflareID, params)
		if err != nil {
			// Check if route was deleted externally
			if cf.IsNotFoundError(err) {
				// Route deleted externally, recreate it
				logger.Info("NetworkRoute not found, recreating",
					"network", cloudflareID)
				result, err = apiClient.CreateTunnelRoute(ctx, params)
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

// handleDeletion handles the deletion of NetworkRoute from Cloudflare.
// This is the SINGLE point for Cloudflare NetworkRoute deletion in the system.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller deletes from Cloudflare
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps and error handling
func (r *Controller) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Get the Cloudflare route network (used as ID for routes)
	cloudflareID := syncState.Spec.CloudflareID

	// Skip if pending ID (route was never created)
	isPending := len(cloudflareID) > 8 && cloudflareID[:8] == "pending-"
	if isPending {
		logger.Info("Skipping deletion - NetworkRoute was never created",
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

		// Determine the VirtualNetworkID for deletion
		// First try to get from config, then search for the route to find its actual VNetID
		var virtualNetworkID string
		if len(syncState.Spec.Sources) > 0 {
			// Try to extract from first source's config
			config, err := r.extractConfig(syncState)
			if err == nil && config.VirtualNetworkID != "" {
				virtualNetworkID = config.VirtualNetworkID
			}
		}

		// If VNetID still empty, search for the route to find its actual VNetID
		if virtualNetworkID == "" {
			existingRoutes, searchErr := apiClient.ListTunnelRoutesByNetwork(ctx, cloudflareID)
			if searchErr == nil && len(existingRoutes) > 0 {
				virtualNetworkID = existingRoutes[0].VirtualNetworkID
				logger.V(1).Info("Found route's VirtualNetworkID for deletion",
					"network", cloudflareID,
					"virtualNetworkId", virtualNetworkID)
			}
		}

		logger.Info("Deleting NetworkRoute from Cloudflare",
			"network", cloudflareID,
			"virtualNetworkId", virtualNetworkID)

		if err := apiClient.DeleteTunnelRoute(ctx, cloudflareID, virtualNetworkID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete NetworkRoute from Cloudflare")
				if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
					logger.Error(statusErr, "Failed to update error status")
				}
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}
			logger.Info("NetworkRoute already deleted from Cloudflare")
		} else {
			logger.Info("Successfully deleted NetworkRoute from Cloudflare",
				"network", cloudflareID)
		}
	}

	// Remove finalizer (with conflict retry)
	if err := common.RemoveFinalizerWithRetry(ctx, r.Client, syncState, FinalizerName); err != nil {
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
		Named("networkroute-sync").
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
