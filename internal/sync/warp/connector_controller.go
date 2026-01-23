// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package warp provides Sync Controllers for WARP-related Cloudflare resources.
package warp

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	warpsvc "github.com/StringKe/cloudflare-operator/internal/service/warp"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

const (
	// WARPConnectorFinalizerName is the finalizer for WARPConnector SyncState resources.
	WARPConnectorFinalizerName = "warpconnector.sync.cloudflare-operator.io/finalizer"
)

// ConnectorController is the Sync Controller for WARP Connector lifecycle operations.
// It watches CloudflareSyncState resources of type WARPConnector and
// performs the actual Cloudflare API calls for connector creation, deletion, and route updates.
type ConnectorController struct {
	*common.BaseSyncController
}

// NewConnectorController creates a new ConnectorController
func NewConnectorController(c client.Client) *ConnectorController {
	return &ConnectorController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for WARP connector lifecycle operations.
//
//nolint:revive // cognitive complexity is acceptable for this reconciliation loop
func (r *ConnectorController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "WARPConnectorSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState resource
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Verify this is a WARPConnector type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceWARPConnector {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing WARPConnector SyncState",
		"cloudflareId", syncState.Spec.CloudflareID,
		"sources", len(syncState.Spec.Sources))

	// Handle deletion - this is the SINGLE point for cleanup
	if !syncState.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, syncState)
	}

	// Check if there are any sources - if none, clean up
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, cleaning up")
		return r.handleDeletion(ctx, syncState)
	}

	// Add finalizer if not present (with conflict retry)
	if !controllerutil.ContainsFinalizer(syncState, WARPConnectorFinalizerName) {
		if err := common.AddFinalizerWithRetry(ctx, r.Client, syncState, WARPConnectorFinalizerName); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip if already synced (lifecycle operations are one-time)
	if syncState.Status.SyncStatus == v1alpha2.SyncStatusSynced {
		logger.V(1).Info("Connector operation already completed, skipping")
		return ctrl.Result{}, nil
	}

	// Skip if there's a pending debounced request
	if r.Debouncer.IsPending(req.Name) {
		logger.V(1).Info("Skipping reconcile - debounced request pending")
		return ctrl.Result{}, nil
	}

	// Set status to Syncing
	if err := r.SetSyncStatus(ctx, syncState, v1alpha2.SyncStatusSyncing); err != nil {
		logger.Error(err, "Failed to set syncing status")
	}

	// Get lifecycle config from sources
	config, err := r.getLifecycleConfig(syncState)
	if err != nil {
		return r.handleError(ctx, syncState, fmt.Errorf("get lifecycle config: %w", err))
	}

	logger.Info("Processing WARP connector lifecycle operation",
		"action", config.Action,
		"connectorName", config.ConnectorName,
		"connectorId", config.ConnectorID)

	// Create Cloudflare API client
	cfAPI, err := r.createAPIClient(ctx, syncState)
	if err != nil {
		return r.handleError(ctx, syncState, fmt.Errorf("create API client: %w", err))
	}

	// Execute lifecycle operation
	var result *warpsvc.ConnectorLifecycleResult
	switch config.Action {
	case warpsvc.ConnectorActionCreate:
		result, err = r.createConnector(ctx, cfAPI, config)
	case warpsvc.ConnectorActionDelete:
		err = r.deleteConnector(ctx, cfAPI, config)
		if err == nil {
			// For delete, we don't have a result to store
			result = &warpsvc.ConnectorLifecycleResult{
				ConnectorID: config.ConnectorID,
				TunnelID:    config.TunnelID,
			}
		}
	case warpsvc.ConnectorActionUpdate:
		result = r.updateConnectorRoutes(ctx, cfAPI, config)
	default:
		err = fmt.Errorf("unknown connector action: %s", config.Action)
	}

	if err != nil {
		return r.handleError(ctx, syncState, fmt.Errorf("%s connector: %w", config.Action, err))
	}

	// Update status with success
	if err := r.updateSuccessStatus(ctx, syncState, result); err != nil {
		logger.Error(err, "Failed to update success status")
		return ctrl.Result{}, err
	}

	logger.Info("WARP connector lifecycle operation completed successfully",
		"action", config.Action,
		"connectorId", result.ConnectorID,
		"tunnelId", result.TunnelID)

	return ctrl.Result{}, nil
}

// getLifecycleConfig extracts the lifecycle configuration from SyncState sources
func (*ConnectorController) getLifecycleConfig(
	syncState *v1alpha2.CloudflareSyncState,
) (*warpsvc.ConnectorLifecycleConfig, error) {
	if len(syncState.Spec.Sources) == 0 {
		return nil, errors.New("no sources found in syncstate")
	}

	// Use the highest priority source (lowest priority number)
	source := syncState.Spec.Sources[0]
	return warpsvc.ParseLifecycleConfig(source.Config.Raw)
}

// createAPIClient creates a Cloudflare API client from the SyncState credentials
func (r *ConnectorController) createAPIClient(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (*cf.API, error) {
	credRef := &v1alpha2.CloudflareCredentialsRef{
		Name: syncState.Spec.CredentialsRef.Name,
	}
	return cf.NewAPIClientFromCredentialsRef(ctx, r.Client, credRef)
}

// createConnector creates a new WARP connector via Cloudflare API
func (*ConnectorController) createConnector(
	ctx context.Context,
	cfAPI *cf.API,
	config *warpsvc.ConnectorLifecycleConfig,
) (*warpsvc.ConnectorLifecycleResult, error) {
	logger := log.FromContext(ctx)

	logger.Info("Creating WARP connector", "name", config.ConnectorName)

	// Create WARP connector
	connectorResult, err := cfAPI.CreateWARPConnector(ctx, config.ConnectorName)
	if err != nil {
		// Check if connector already exists (conflict error)
		if cf.IsConflictError(err) {
			logger.Info("WARP connector already exists, attempting to adopt",
				"name", config.ConnectorName)
			// For WARP connectors, we can't easily adopt, so return error
			return nil, fmt.Errorf("WARP connector '%s' already exists", config.ConnectorName)
		}
		return nil, fmt.Errorf("create WARP connector: %w", err)
	}

	result := &warpsvc.ConnectorLifecycleResult{
		ConnectorID: connectorResult.ID,
		TunnelID:    connectorResult.TunnelID,
		TunnelToken: connectorResult.TunnelToken,
	}

	// Configure routes
	routesConfigured := 0
	for _, route := range config.Routes {
		logger.Info("Configuring route", "network", route.Network, "virtualNetworkId", config.VirtualNetworkID)
		routeParams := cf.TunnelRouteParams{
			Network:          route.Network,
			TunnelID:         connectorResult.TunnelID,
			VirtualNetworkID: config.VirtualNetworkID,
			Comment:          route.Comment,
		}
		if _, err := cfAPI.CreateTunnelRoute(ctx, routeParams); err != nil {
			// Log error but continue with other routes
			logger.Error(err, "Failed to create route", "network", route.Network)
		} else {
			routesConfigured++
		}
	}
	result.RoutesConfigured = routesConfigured

	logger.Info("WARP connector created successfully",
		"connectorId", connectorResult.ID,
		"tunnelId", connectorResult.TunnelID,
		"routesConfigured", routesConfigured)

	return result, nil
}

// deleteConnector deletes a WARP connector via Cloudflare API
//
//nolint:revive // cognitive complexity is acceptable for deletion logic
func (*ConnectorController) deleteConnector(
	ctx context.Context,
	cfAPI *cf.API,
	config *warpsvc.ConnectorLifecycleConfig,
) error {
	logger := log.FromContext(ctx)

	// Delete routes first
	var routeErrors []error
	for _, route := range config.Routes {
		logger.Info("Deleting route", "network", route.Network, "virtualNetworkId", config.VirtualNetworkID)
		if err := cfAPI.DeleteTunnelRoute(ctx, route.Network, config.VirtualNetworkID); err != nil {
			if cf.IsNotFoundError(err) {
				logger.Info("Route already deleted", "network", route.Network)
			} else {
				logger.Error(err, "Failed to delete route", "network", route.Network)
				routeErrors = append(routeErrors, fmt.Errorf("delete route %s: %w", route.Network, err))
			}
		}
	}

	// If any route deletion failed, aggregate errors and return
	if len(routeErrors) > 0 {
		return errors.Join(routeErrors...)
	}

	// Delete WARP connector
	if config.ConnectorID != "" {
		logger.Info("Deleting WARP connector", "connectorId", config.ConnectorID)
		if err := cfAPI.DeleteWARPConnector(ctx, config.ConnectorID); err != nil {
			if cf.IsNotFoundError(err) {
				logger.Info("WARP connector already deleted", "connectorId", config.ConnectorID)
			} else {
				return fmt.Errorf("delete WARP connector: %w", err)
			}
		}
	}

	logger.Info("WARP connector deleted successfully", "connectorId", config.ConnectorID)
	return nil
}

// getConnectorToken retrieves the tunnel token for a WARP connector
func getConnectorToken(ctx context.Context, cfAPI *cf.API, connectorID string) string {
	if connectorID == "" {
		return ""
	}
	tokenResult, err := cfAPI.GetWARPConnectorToken(ctx, connectorID)
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to get WARP connector token, continuing without it")
		return ""
	}
	return tokenResult.Token
}

// configureRoute creates a single route, returning true if successful or already exists
func configureRoute(ctx context.Context, cfAPI *cf.API, config *warpsvc.ConnectorLifecycleConfig, route warpsvc.RouteConfig) bool {
	logger := log.FromContext(ctx)
	routeParams := cf.TunnelRouteParams{
		Network:          route.Network,
		TunnelID:         config.TunnelID,
		VirtualNetworkID: config.VirtualNetworkID,
		Comment:          route.Comment,
	}
	_, err := cfAPI.CreateTunnelRoute(ctx, routeParams)
	if err == nil {
		return true
	}
	if cf.IsConflictError(err) {
		logger.V(1).Info("Route already exists", "network", route.Network)
		return true
	}
	logger.Error(err, "Failed to create route", "network", route.Network)
	return false
}

// updateConnectorRoutes updates routes for an existing WARP connector
func (*ConnectorController) updateConnectorRoutes(
	ctx context.Context,
	cfAPI *cf.API,
	config *warpsvc.ConnectorLifecycleConfig,
) *warpsvc.ConnectorLifecycleResult {
	logger := log.FromContext(ctx)

	logger.Info("Updating WARP connector routes",
		"connectorId", config.ConnectorID,
		"tunnelId", config.TunnelID)

	tunnelToken := getConnectorToken(ctx, cfAPI, config.ConnectorID)

	routesConfigured := 0
	for _, route := range config.Routes {
		logger.Info("Configuring route", "network", route.Network, "virtualNetworkId", config.VirtualNetworkID)
		if configureRoute(ctx, cfAPI, config, route) {
			routesConfigured++
		}
	}

	logger.Info("WARP connector routes updated successfully",
		"connectorId", config.ConnectorID,
		"routesConfigured", routesConfigured)

	return &warpsvc.ConnectorLifecycleResult{
		ConnectorID:      config.ConnectorID,
		TunnelID:         config.TunnelID,
		TunnelToken:      tunnelToken,
		RoutesConfigured: routesConfigured,
	}
}

// handleError updates the SyncState status with an error and returns appropriate result
func (r *ConnectorController) handleError(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	err error,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if updateErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); updateErr != nil {
		logger.Error(updateErr, "Failed to update error status")
	}

	return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, err
}

// updateSuccessStatus updates the SyncState status with success and result data
//
//nolint:revive // cognitive complexity is acceptable for building result data
func (r *ConnectorController) updateSuccessStatus(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	result *warpsvc.ConnectorLifecycleResult,
) error {
	// Build result data map
	resultData := make(map[string]string)
	if result != nil {
		resultData[warpsvc.ResultKeyConnectorID] = result.ConnectorID
		resultData[warpsvc.ResultKeyTunnelID] = result.TunnelID
		if result.TunnelToken != "" {
			resultData[warpsvc.ResultKeyTunnelToken] = result.TunnelToken
		}
		resultData[warpsvc.ResultKeyRoutesConfigured] = strconv.Itoa(result.RoutesConfigured)
	}

	// Update CloudflareID with actual connector ID (with conflict retry)
	if result != nil && result.ConnectorID != "" {
		if err := common.UpdateCloudflareID(ctx, r.Client, syncState, result.ConnectorID); err != nil {
			return fmt.Errorf("update syncstate CloudflareID: %w", err)
		}
	}

	syncResult := &common.SyncResult{
		ConfigHash: "", // Lifecycle operations don't use config hash
	}

	// Update status with result data (with conflict retry built into UpdateSyncStatus)
	syncState.Status.ResultData = resultData
	if err := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusSynced, syncResult, nil); err != nil {
		return fmt.Errorf("update syncstate status: %w", err)
	}

	return nil
}

// handleDeletion handles the deletion of WARPConnector SyncState.
// It deletes the WARP connector from Cloudflare to maintain state consistency.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller deletes from Cloudflare
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps
func (r *ConnectorController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, WARPConnectorFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Delete WARP connector from Cloudflare to maintain state consistency
	connectorID := syncState.Spec.CloudflareID
	if connectorID != "" && !common.IsPendingID(connectorID) {
		logger.Info("Deleting WARP connector from Cloudflare",
			"connectorId", connectorID)

		// Create API client
		cfAPI, err := r.createAPIClient(ctx, syncState)
		if err != nil {
			logger.Error(err, "Failed to create API client for connector deletion")
			// Continue with finalizer removal - don't block on API client creation failure
		} else {
			// Try to get config for route information
			var routesToDelete []warpsvc.RouteConfig
			var virtualNetworkID string
			if len(syncState.Spec.Sources) > 0 {
				config, err := r.getLifecycleConfig(syncState)
				if err == nil && config != nil {
					routesToDelete = config.Routes
					virtualNetworkID = config.VirtualNetworkID
				}
			}

			// Delete routes first (best effort)
			for _, route := range routesToDelete {
				logger.V(1).Info("Deleting route", "network", route.Network)
				if err := cfAPI.DeleteTunnelRoute(ctx, route.Network, virtualNetworkID); err != nil {
					if !cf.IsNotFoundError(err) {
						logger.Error(err, "Failed to delete route", "network", route.Network)
					}
				}
			}

			// Delete the WARP connector
			if err := cfAPI.DeleteWARPConnector(ctx, connectorID); err != nil {
				if cf.IsNotFoundError(err) {
					logger.Info("WARP connector already deleted", "connectorId", connectorID)
				} else {
					logger.Error(err, "Failed to delete WARP connector", "connectorId", connectorID)
					// Requeue to retry deletion
					return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, err
				}
			} else {
				logger.Info("WARP connector deleted successfully", "connectorId", connectorID)
			}
		}
	} else {
		logger.Info("No valid connector ID to delete, cleaning up SyncState only")
	}

	// Remove finalizer (with conflict retry)
	if err := common.RemoveFinalizerWithRetry(ctx, r.Client, syncState, WARPConnectorFinalizerName); err != nil {
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

// isWARPConnectorSyncState checks if the object is a WARPConnector SyncState
func isWARPConnectorSyncState(obj client.Object) bool {
	ss, ok := obj.(*v1alpha2.CloudflareSyncState)
	return ok && ss.Spec.ResourceType == v1alpha2.SyncResourceWARPConnector
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConnectorController) SetupWithManager(mgr ctrl.Manager) error {
	// Predicate to only watch WARPConnector type SyncStates
	connectorPredicate := predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return isWARPConnectorSyncState(e.Object) },
		UpdateFunc:  func(e event.UpdateEvent) bool { return isWARPConnectorSyncState(e.ObjectNew) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return isWARPConnectorSyncState(e.Object) },
		GenericFunc: func(e event.GenericEvent) bool { return isWARPConnectorSyncState(e.Object) },
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("warpconnector-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(connectorPredicate).
		Complete(r)
}
