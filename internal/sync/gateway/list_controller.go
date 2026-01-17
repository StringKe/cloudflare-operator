// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package gateway

//nolint:dupl // Similar patterns across resource types are intentional

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	gatewaysvc "github.com/StringKe/cloudflare-operator/internal/service/gateway"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

// ListController is the Sync Controller for Gateway List Configuration.
type ListController struct {
	*common.BaseSyncController
}

// NewListController creates a new GatewayListSyncController.
func NewListController(c client.Client) *ListController {
	return &ListController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for Gateway list.
//
//nolint:revive // cognitive complexity is acceptable for sync controller reconciliation
func (r *ListController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "GatewayListSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process GatewayList type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceGatewayList {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing GatewayList SyncState",
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

	// Extract Gateway list configuration from first source
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract Gateway list configuration")
		if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
	}

	// Compute hash for change detection
	newHash, hashErr := common.ComputeConfigHash(config)
	if hashErr != nil {
		logger.Error(hashErr, "Failed to compute config hash")
		newHash = ""
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
		logger.Error(err, "Failed to sync Gateway list to Cloudflare")
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

	logger.Info("Successfully synced Gateway list to Cloudflare",
		"listId", result.ListID)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts Gateway list configuration from SyncState sources.
// Gateway lists have 1:1 mapping, so we use the ExtractFirstSourceConfig helper.
func (*ListController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*gatewaysvc.GatewayListConfig, error) {
	return common.ExtractFirstSourceConfig[gatewaysvc.GatewayListConfig](syncState)
}

// syncToCloudflare syncs the Gateway list configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *ListController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *gatewaysvc.GatewayListConfig,
) (*gatewaysvc.GatewayListSyncResult, error) {
	logger := log.FromContext(ctx)

	// Create API client using common helper
	apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
	if err != nil {
		return nil, err
	}

	// Validate account ID is present
	accountID, err := common.RequireAccountID(syncState)
	if err != nil {
		return nil, err
	}

	// Build Gateway list params
	params := cf.GatewayListParams{
		Name:        config.Name,
		Description: config.Description,
		Type:        config.Type,
		Items:       config.Items,
	}

	// Check if this is a new list (pending) or existing (has real Cloudflare ID)
	cloudflareID := syncState.Spec.CloudflareID
	var result *cf.GatewayListResult

	if common.IsPendingID(cloudflareID) {
		// Create new list
		logger.Info("Creating new Gateway list",
			"name", config.Name,
			"type", config.Type)

		result, err = apiClient.CreateGatewayList(params)
		if err != nil {
			return nil, fmt.Errorf("create Gateway list: %w", err)
		}

		// Update SyncState with actual list ID
		common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID)

		logger.Info("Created Gateway list", "listId", result.ID)
	} else {
		// Update existing list
		logger.Info("Updating Gateway list",
			"listId", cloudflareID,
			"name", config.Name)

		result, err = apiClient.UpdateGatewayList(cloudflareID, params)
		if err != nil {
			if !common.HandleNotFoundOnUpdate(err) {
				return nil, fmt.Errorf("update Gateway list: %w", err)
			}
			// List deleted externally, recreate it
			logger.Info("Gateway list not found, recreating", "listId", cloudflareID)
			result, err = apiClient.CreateGatewayList(params)
			if err != nil {
				return nil, fmt.Errorf("recreate Gateway list: %w", err)
			}
			common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID)
		}

		logger.Info("Updated Gateway list", "listId", result.ID)
	}

	return &gatewaysvc.GatewayListSyncResult{
		ListID:    result.ID,
		AccountID: accountID,
		ItemCount: result.Count,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ListController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourceGatewayList)).
		Complete(r)
}
