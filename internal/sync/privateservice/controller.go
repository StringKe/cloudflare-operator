// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package privateservice provides the PrivateService Sync Controller for managing Cloudflare Tunnel Routes.
// Similar to NetworkRouteSyncController, but specifically for PrivateService resources
// which derive their route CIDR from K8s Service ClusterIPs.
//
// Unified Sync Architecture Flow:
// K8s Resources → Resource Controllers → Core Services → SyncState CRD → Sync Controllers → Cloudflare API
//
// This Sync Controller is the SINGLE point that calls Cloudflare API for PrivateService routes.
// It handles create, update, and delete operations based on SyncState changes.
package privateservice

import (
	"context"
	"errors"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	pssvc "github.com/StringKe/cloudflare-operator/internal/service/privateservice"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

const (
	// FinalizerName is the finalizer for PrivateService SyncState resources.
	// This ensures we delete the tunnel route from Cloudflare before removing SyncState.
	FinalizerName = "privateservice.sync.cloudflare-operator.io/finalizer"
)

// Controller is the Sync Controller for PrivateService Configuration.
// It watches CloudflareSyncState resources of type PrivateService,
// extracts the configuration, and syncs to Cloudflare API.
// This is the SINGLE point that calls Cloudflare Tunnel Route API for PrivateService.
type Controller struct {
	*common.BaseSyncController
}

// NewController creates a new PrivateServiceSyncController
func NewController(c client.Client) *Controller {
	return &Controller{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for PrivateService.
// Following Unified Sync Architecture:
// K8s Resources → Resource Controllers → Core Services → SyncState CRD → Sync Controllers → Cloudflare API
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "PrivateServiceSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process PrivateService type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourcePrivateService {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing PrivateService SyncState",
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

	// Extract PrivateService configuration from first source
	// (PrivateService has 1:1 mapping, so there should only be one source)
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract PrivateService configuration")
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
		logger.Error(err, "Failed to sync PrivateService to Cloudflare")
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

	logger.Info("Successfully synced PrivateService to Cloudflare",
		"network", result.Network,
		"tunnelId", result.TunnelID,
		"serviceIP", result.ServiceIP)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts PrivateService configuration from SyncState sources.
func (*Controller) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*pssvc.PrivateServiceConfig, error) {
	if len(syncState.Spec.Sources) == 0 {
		return nil, errors.New("no sources in SyncState")
	}

	// PrivateService has 1:1 mapping, use first source
	source := syncState.Spec.Sources[0]

	config, err := common.ParseSourceConfig[pssvc.PrivateServiceConfig](&source)
	if err != nil {
		return nil, fmt.Errorf("parse PrivateService config from source %s: %w", source.Ref.String(), err)
	}

	if config == nil {
		return nil, fmt.Errorf("empty PrivateService config from source %s", source.Ref.String())
	}

	return config, nil
}

// syncToCloudflare syncs the PrivateService configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *Controller) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *pssvc.PrivateServiceConfig,
) (*pssvc.SyncResult, error) {
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

	// Build TunnelRoute params
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
		// Check if route already exists (e.g., created by NetworkRoute or externally)
		existing, err := apiClient.GetTunnelRoute(config.Network, config.VirtualNetworkID)
		if err == nil && existing != nil {
			// Route exists, update SyncState and return
			logger.Info("Found existing tunnel route for PrivateService",
				"network", config.Network)
			result = existing
		} else {
			// Create new tunnel route
			logger.Info("Creating new tunnel route for PrivateService",
				"network", config.Network,
				"tunnelId", config.TunnelID)

			result, err = apiClient.CreateTunnelRoute(params)
			if err != nil {
				return nil, fmt.Errorf("create tunnel route: %w", err)
			}

			logger.Info("Created tunnel route for PrivateService",
				"network", result.Network,
				"tunnelId", result.TunnelID)
		}

		// Update SyncState CloudflareID with the network
		syncState.Spec.CloudflareID = config.Network
		if updateErr := r.Client.Update(ctx, syncState); updateErr != nil {
			logger.Error(updateErr, "Failed to update SyncState with route network",
				"network", config.Network)
			// Non-fatal - will be fixed on next reconcile
		}
	} else {
		// Update existing tunnel route
		logger.Info("Updating tunnel route for PrivateService",
			"network", cloudflareID,
			"tunnelId", config.TunnelID)

		result, err = apiClient.UpdateTunnelRoute(cloudflareID, params)
		if err != nil {
			// Check if route was deleted externally
			if cf.IsNotFoundError(err) {
				// Route deleted externally, recreate it
				logger.Info("Tunnel route not found, recreating for PrivateService",
					"network", cloudflareID)
				result, err = apiClient.CreateTunnelRoute(params)
				if err != nil {
					return nil, fmt.Errorf("recreate tunnel route: %w", err)
				}

				// Update SyncState CloudflareID if network changed
				if result.Network != cloudflareID {
					syncState.Spec.CloudflareID = result.Network
					if updateErr := r.Client.Update(ctx, syncState); updateErr != nil {
						logger.Error(updateErr, "Failed to update SyncState with new route network")
					}
				}
			} else {
				return nil, fmt.Errorf("update tunnel route: %w", err)
			}
		}

		logger.Info("Updated tunnel route for PrivateService",
			"network", result.Network,
			"tunnelId", result.TunnelID)
	}

	return &pssvc.SyncResult{
		Network:          result.Network,
		TunnelID:         result.TunnelID,
		TunnelName:       result.TunnelName,
		VirtualNetworkID: result.VirtualNetworkID,
		ServiceIP:        config.ServiceIP,
	}, nil
}

// handleDeletion handles the deletion of PrivateService tunnel route from Cloudflare.
// This is the SINGLE point for Cloudflare tunnel route deletion for PrivateService.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller deletes from Cloudflare
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
		logger.Info("Skipping deletion - PrivateService route was never created",
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

		// Get virtual network ID from the config (if sources exist) or default
		virtualNetworkID := "default"
		if len(syncState.Spec.Sources) > 0 {
			// Try to extract from first source's config
			config, err := r.extractConfig(syncState)
			if err == nil && config.VirtualNetworkID != "" {
				virtualNetworkID = config.VirtualNetworkID
			}
		}

		logger.Info("Deleting tunnel route for PrivateService from Cloudflare",
			"network", cloudflareID,
			"virtualNetworkId", virtualNetworkID)

		if err := apiClient.DeleteTunnelRoute(cloudflareID, virtualNetworkID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete tunnel route from Cloudflare")
				if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
					logger.Error(statusErr, "Failed to update error status")
				}
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}
			logger.Info("Tunnel route already deleted from Cloudflare")
		} else {
			logger.Info("Successfully deleted tunnel route from Cloudflare",
				"network", cloudflareID)
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
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourcePrivateService)).
		Complete(r)
}
