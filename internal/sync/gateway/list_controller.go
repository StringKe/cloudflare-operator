// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package gateway

//nolint:dupl // Similar patterns across resource types are intentional

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	gatewaysvc "github.com/StringKe/cloudflare-operator/internal/service/gateway"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

const (
	// ListFinalizerName is the finalizer for Gateway List SyncState resources.
	ListFinalizerName = "gatewaylist.sync.cloudflare-operator.io/finalizer"
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
	if !controllerutil.ContainsFinalizer(syncState, ListFinalizerName) {
		if err := common.AddFinalizerWithRetry(ctx, r.Client, syncState, ListFinalizerName); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip if there's a pending debounced request
	if r.Debouncer.IsPending(req.Name) {
		logger.V(1).Info("Skipping reconcile - debounced request pending")
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

	// Build Gateway list params with item descriptions and deduplication
	// Deduplication is based on Value - first occurrence wins (preserves priority order)
	seen := make(map[string]bool)
	cfItems := make([]cf.GatewayListItem, 0, len(config.Items))
	duplicateCount := 0
	for _, item := range config.Items {
		if seen[item.Value] {
			duplicateCount++
			continue // Skip duplicate values
		}
		seen[item.Value] = true
		cfItems = append(cfItems, cf.GatewayListItem{
			Value:       item.Value,
			Description: item.Description,
		})
	}
	if duplicateCount > 0 {
		logger.Info("Deduplicated Gateway list items",
			"name", config.Name,
			"originalCount", len(config.Items),
			"deduplicatedCount", len(cfItems),
			"duplicatesRemoved", duplicateCount)
	}

	params := cf.GatewayListParams{
		Name:        config.Name,
		Description: config.Description,
		Type:        config.Type,
		Items:       cfItems,
	}

	// Check if this is a new list (pending) or existing (has real Cloudflare ID)
	cloudflareID := syncState.Spec.CloudflareID
	var result *cf.GatewayListResult

	if common.IsPendingID(cloudflareID) {
		// Create new list
		logger.Info("Creating new Gateway list",
			"name", config.Name,
			"type", config.Type)

		result, err = apiClient.CreateGatewayList(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("create Gateway list: %w", err)
		}

		// Update SyncState with actual list ID (must succeed)
		if err := common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID); err != nil {
			return nil, err
		}

		logger.Info("Created Gateway list", "listId", result.ID)
	} else {
		// Update existing list
		logger.Info("Updating Gateway list",
			"listId", cloudflareID,
			"name", config.Name)

		result, err = apiClient.UpdateGatewayList(ctx, cloudflareID, params)
		if err != nil {
			if !common.HandleNotFoundOnUpdate(err) {
				return nil, fmt.Errorf("update Gateway list: %w", err)
			}
			// List deleted externally, recreate it
			logger.Info("Gateway list not found, recreating", "listId", cloudflareID)
			result, err = apiClient.CreateGatewayList(ctx, params)
			if err != nil {
				return nil, fmt.Errorf("recreate Gateway list: %w", err)
			}
			if updateErr := common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID); updateErr != nil {
				logger.Error(updateErr, "Failed to update CloudflareID after recreating")
			}
		}

		logger.Info("Updated Gateway list", "listId", result.ID)
	}

	return &gatewaysvc.GatewayListSyncResult{
		ListID:    result.ID,
		AccountID: accountID,
		ItemCount: result.Count,
	}, nil
}

// handleDeletion handles the deletion of Gateway List from Cloudflare.
// This is the SINGLE point for Cloudflare Gateway List deletion in the system.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller deletes from Cloudflare
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps
func (r *ListController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, ListFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Get the Cloudflare list ID
	cloudflareID := syncState.Spec.CloudflareID

	// Skip if pending ID (list was never created)
	if common.IsPendingID(cloudflareID) {
		logger.Info("Skipping deletion - Gateway List was never created",
			"cloudflareId", cloudflareID)
	} else if cloudflareID != "" {
		// Delete from Cloudflare
		apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
		if err != nil {
			logger.Error(err, "Failed to create API client for deletion")
			return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
		}

		logger.Info("Deleting Gateway List from Cloudflare",
			"listId", cloudflareID)

		if err := apiClient.DeleteGatewayList(ctx, cloudflareID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete Gateway List from Cloudflare")
				if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
					logger.Error(statusErr, "Failed to update error status")
				}
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}
			logger.Info("Gateway List already deleted from Cloudflare")
		} else {
			logger.Info("Successfully deleted Gateway List from Cloudflare",
				"listId", cloudflareID)
		}
	}

	// Remove finalizer (with conflict retry)
	if err := common.RemoveFinalizerWithRetry(ctx, r.Client, syncState, ListFinalizerName); err != nil {
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
func (r *ListController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("gateway-list-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourceGatewayList)).
		Complete(r)
}
