// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package tunnel

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
	tunnelsvc "github.com/StringKe/cloudflare-operator/internal/service/tunnel"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

const (
	// TunnelLifecycleFinalizerName is the finalizer for TunnelLifecycle SyncState resources.
	TunnelLifecycleFinalizerName = "tunnellifecycle.sync.cloudflare-operator.io/finalizer"
)

// LifecycleController is the Sync Controller for Tunnel Lifecycle operations.
// It watches CloudflareSyncState resources of type TunnelLifecycle and
// performs the actual Cloudflare API calls for tunnel creation, deletion, and adoption.
type LifecycleController struct {
	*common.BaseSyncController
}

// NewLifecycleController creates a new TunnelLifecycleSyncController
func NewLifecycleController(c client.Client) *LifecycleController {
	return &LifecycleController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for tunnel lifecycle operations.
//
//nolint:revive // cognitive complexity is acceptable for this reconciliation loop
func (r *LifecycleController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "TunnelLifecycleSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState resource
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Verify this is a TunnelLifecycle type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceTunnelLifecycle {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing TunnelLifecycle SyncState",
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

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(syncState, TunnelLifecycleFinalizerName) {
		controllerutil.AddFinalizer(syncState, TunnelLifecycleFinalizerName)
		if err := r.Client.Update(ctx, syncState); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip if already synced (lifecycle operations are one-time)
	if syncState.Status.SyncStatus == v1alpha2.SyncStatusSynced {
		logger.V(1).Info("Lifecycle operation already completed, skipping")
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

	logger.Info("Processing tunnel lifecycle operation",
		"action", config.Action,
		"tunnelName", config.TunnelName,
		"tunnelId", config.TunnelID)

	// Create Cloudflare API client
	cfAPI, err := r.createAPIClient(ctx, syncState)
	if err != nil {
		return r.handleError(ctx, syncState, fmt.Errorf("create API client: %w", err))
	}

	// Execute lifecycle operation
	var result *tunnelsvc.LifecycleResult
	switch config.Action {
	case tunnelsvc.LifecycleActionCreate:
		result, err = r.createTunnel(ctx, cfAPI, config)
	case tunnelsvc.LifecycleActionDelete:
		err = r.deleteTunnel(ctx, cfAPI, config)
		if err == nil {
			// For delete, we don't have a result to store
			result = &tunnelsvc.LifecycleResult{
				TunnelID:   config.TunnelID,
				TunnelName: config.TunnelName,
			}
		}
	case tunnelsvc.LifecycleActionAdopt:
		result, err = r.adoptTunnel(ctx, cfAPI, config)
	default:
		err = fmt.Errorf("unknown lifecycle action: %s", config.Action)
	}

	if err != nil {
		return r.handleError(ctx, syncState, fmt.Errorf("%s tunnel: %w", config.Action, err))
	}

	// Update status with success
	if err := r.updateSuccessStatus(ctx, syncState, result); err != nil {
		logger.Error(err, "Failed to update success status")
		return ctrl.Result{}, err
	}

	logger.Info("Tunnel lifecycle operation completed successfully",
		"action", config.Action,
		"tunnelId", result.TunnelID,
		"tunnelName", result.TunnelName)

	return ctrl.Result{}, nil
}

// getLifecycleConfig extracts the lifecycle configuration from SyncState sources
func (*LifecycleController) getLifecycleConfig(syncState *v1alpha2.CloudflareSyncState) (*tunnelsvc.LifecycleConfig, error) {
	if len(syncState.Spec.Sources) == 0 {
		return nil, errors.New("no sources found in syncstate")
	}

	// Use the highest priority source (lowest priority number)
	source := syncState.Spec.Sources[0]
	return tunnelsvc.ParseLifecycleConfig(source.Config.Raw)
}

// createAPIClient creates a Cloudflare API client from the SyncState credentials
func (r *LifecycleController) createAPIClient(ctx context.Context, syncState *v1alpha2.CloudflareSyncState) (*cf.API, error) {
	credRef := &v1alpha2.CloudflareCredentialsRef{
		Name: syncState.Spec.CredentialsRef.Name,
	}
	return cf.NewAPIClientFromCredentialsRef(ctx, r.Client, credRef)
}

// createTunnel creates a new tunnel via Cloudflare API
func (r *LifecycleController) createTunnel(
	ctx context.Context,
	cfAPI *cf.API,
	config *tunnelsvc.LifecycleConfig,
) (*tunnelsvc.LifecycleResult, error) {
	logger := log.FromContext(ctx)

	logger.Info("Creating tunnel", "name", config.TunnelName)

	// Create tunnel using parameterized method
	tunnelResult, err := cfAPI.CreateTunnelWithParams(config.TunnelName, config.ConfigSrc)
	if err != nil {
		// Check if tunnel already exists (conflict error)
		if cf.IsConflictError(err) {
			logger.Info("Tunnel already exists, attempting to adopt",
				"name", config.TunnelName)
			return r.adoptTunnelByName(ctx, cfAPI, config.TunnelName)
		}
		return nil, fmt.Errorf("create tunnel: %w", err)
	}

	// Get tunnel token
	token, err := cfAPI.GetTunnelToken(tunnelResult.ID)
	if err != nil {
		logger.Error(err, "Failed to get tunnel token, tunnel created but token unavailable",
			"tunnelId", tunnelResult.ID)
		// Continue - tunnel is created, token can be fetched later
	}

	result := &tunnelsvc.LifecycleResult{
		TunnelID:    tunnelResult.ID,
		TunnelName:  tunnelResult.Name,
		TunnelToken: token,
	}

	// Encode credentials as base64 JSON
	if tunnelResult.Credentials != nil {
		credsJSON, err := json.Marshal(tunnelResult.Credentials)
		if err == nil {
			result.Credentials = base64.StdEncoding.EncodeToString(credsJSON)
			result.AccountTag = tunnelResult.Credentials.AccountTag
		}
	}

	logger.Info("Tunnel created successfully",
		"tunnelId", tunnelResult.ID,
		"tunnelName", tunnelResult.Name)

	return result, nil
}

// deleteTunnel deletes a tunnel via Cloudflare API
func (*LifecycleController) deleteTunnel(
	ctx context.Context,
	cfAPI *cf.API,
	config *tunnelsvc.LifecycleConfig,
) error {
	logger := log.FromContext(ctx)

	logger.Info("Deleting tunnel", "tunnelId", config.TunnelID)

	// Delete associated routes first
	if _, err := cfAPI.DeleteTunnelRoutesByTunnelID(config.TunnelID); err != nil {
		if !cf.IsNotFoundError(err) {
			logger.Error(err, "Failed to delete tunnel routes, continuing with tunnel deletion")
		}
	}

	// Delete tunnel using parameterized method
	if err := cfAPI.DeleteTunnelByID(config.TunnelID); err != nil {
		if cf.IsNotFoundError(err) {
			logger.Info("Tunnel already deleted", "tunnelId", config.TunnelID)
			return nil
		}
		return fmt.Errorf("delete tunnel: %w", err)
	}

	logger.Info("Tunnel deleted successfully", "tunnelId", config.TunnelID)
	return nil
}

// adoptTunnel adopts an existing tunnel
func (*LifecycleController) adoptTunnel(
	ctx context.Context,
	cfAPI *cf.API,
	config *tunnelsvc.LifecycleConfig,
) (*tunnelsvc.LifecycleResult, error) {
	logger := log.FromContext(ctx)

	tunnelID := config.TunnelID
	if tunnelID == "" && config.ExistingTunnelID != "" {
		tunnelID = config.ExistingTunnelID
	}

	logger.Info("Adopting tunnel", "tunnelId", tunnelID, "tunnelName", config.TunnelName)

	// Get tunnel token
	token, err := cfAPI.GetTunnelToken(tunnelID)
	if err != nil {
		return nil, fmt.Errorf("get tunnel token: %w", err)
	}

	// Get tunnel credentials
	creds, err := cfAPI.GetTunnelCredsByID(tunnelID)
	if err != nil {
		logger.Error(err, "Failed to get tunnel credentials, continuing without them")
		// Continue - we have the token at least
	}

	result := &tunnelsvc.LifecycleResult{
		TunnelID:    tunnelID,
		TunnelName:  config.TunnelName,
		TunnelToken: token,
	}

	if creds != nil {
		credsJSON, err := json.Marshal(creds)
		if err == nil {
			result.Credentials = base64.StdEncoding.EncodeToString(credsJSON)
			result.AccountTag = creds.AccountTag
		}
	}

	logger.Info("Tunnel adopted successfully", "tunnelId", tunnelID)
	return result, nil
}

// adoptTunnelByName looks up a tunnel by name and adopts it
func (r *LifecycleController) adoptTunnelByName(
	ctx context.Context,
	cfAPI *cf.API,
	tunnelName string,
) (*tunnelsvc.LifecycleResult, error) {
	logger := log.FromContext(ctx)

	// Get tunnel ID by name using parameterized method
	tunnelID, err := cfAPI.GetTunnelIDByName(tunnelName)
	if err != nil {
		return nil, fmt.Errorf("get tunnel ID by name: %w", err)
	}

	logger.Info("Found existing tunnel by name", "name", tunnelName, "tunnelId", tunnelID)

	config := &tunnelsvc.LifecycleConfig{
		TunnelID:   tunnelID,
		TunnelName: tunnelName,
	}
	return r.adoptTunnel(ctx, cfAPI, config)
}

// handleError updates the SyncState status with an error and returns appropriate result
func (r *LifecycleController) handleError(
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
func (r *LifecycleController) updateSuccessStatus(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	result *tunnelsvc.LifecycleResult,
) error {
	// Build result data map
	resultData := make(map[string]string)
	if result != nil {
		resultData[tunnelsvc.ResultKeyTunnelID] = result.TunnelID
		resultData[tunnelsvc.ResultKeyTunnelName] = result.TunnelName
		if result.TunnelToken != "" {
			resultData[tunnelsvc.ResultKeyTunnelToken] = result.TunnelToken
		}
		if result.Credentials != "" {
			resultData[tunnelsvc.ResultKeyCredentials] = result.Credentials
		}
		if result.AccountTag != "" {
			resultData[tunnelsvc.ResultKeyAccountTag] = result.AccountTag
		}
	}

	// Update CloudflareID with actual tunnel ID
	if result != nil && result.TunnelID != "" {
		syncState.Spec.CloudflareID = result.TunnelID
	}

	syncResult := &common.SyncResult{
		ConfigHash: "", // Lifecycle operations don't use config hash
	}

	// First update the spec to set CloudflareID
	if err := r.Client.Update(ctx, syncState); err != nil {
		return fmt.Errorf("update syncstate spec: %w", err)
	}

	// Then update the status
	syncState.Status.ResultData = resultData
	if err := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusSynced, syncResult, nil); err != nil {
		return fmt.Errorf("update syncstate status: %w", err)
	}

	return nil
}

// handleDeletion handles the deletion of TunnelLifecycle SyncState.
// This deletes the tunnel from Cloudflare to maintain state consistency.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller deletes from Cloudflare
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps
func (r *LifecycleController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, TunnelLifecycleFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Delete tunnel from Cloudflare to maintain state consistency
	tunnelID := syncState.Spec.CloudflareID
	if tunnelID != "" && !common.IsPendingID(tunnelID) {
		cfAPI, err := r.createAPIClient(ctx, syncState)
		if err != nil {
			logger.Error(err, "Failed to create API client for deletion")
			return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
		}

		logger.Info("Deleting tunnel from Cloudflare",
			"tunnelId", tunnelID)

		// Delete associated routes first
		if _, err := cfAPI.DeleteTunnelRoutesByTunnelID(tunnelID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete tunnel routes, continuing with tunnel deletion")
			}
		}

		// Delete the tunnel
		if err := cfAPI.DeleteTunnelByID(tunnelID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete tunnel")
				if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
					logger.Error(statusErr, "Failed to update error status")
				}
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}
			logger.Info("Tunnel already deleted or not found")
		} else {
			logger.Info("Successfully deleted tunnel from Cloudflare")
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(syncState, TunnelLifecycleFinalizerName)
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

// isTunnelLifecycleSyncState checks if the object is a TunnelLifecycle SyncState
func isTunnelLifecycleSyncState(obj client.Object) bool {
	ss, ok := obj.(*v1alpha2.CloudflareSyncState)
	return ok && ss.Spec.ResourceType == v1alpha2.SyncResourceTunnelLifecycle
}

// SetupWithManager sets up the controller with the Manager.
func (r *LifecycleController) SetupWithManager(mgr ctrl.Manager) error {
	// Predicate to only watch TunnelLifecycle type SyncStates
	lifecyclePredicate := predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return isTunnelLifecycleSyncState(e.Object) },
		UpdateFunc:  func(e event.UpdateEvent) bool { return isTunnelLifecycleSyncState(e.ObjectNew) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return isTunnelLifecycleSyncState(e.Object) },
		GenericFunc: func(e event.GenericEvent) bool { return isTunnelLifecycleSyncState(e.Object) },
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("tunnel-lifecycle-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(lifecyclePredicate).
		Complete(r)
}
