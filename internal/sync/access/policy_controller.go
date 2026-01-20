// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package access

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
	accesssvc "github.com/StringKe/cloudflare-operator/internal/service/access"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

const (
	// PolicyFinalizerName is the finalizer for AccessPolicy SyncState resources.
	PolicyFinalizerName = "accesspolicy.sync.cloudflare-operator.io/finalizer"
)

// PolicyController is the Sync Controller for reusable AccessPolicy Configuration.
type PolicyController struct {
	*common.BaseSyncController
}

// NewPolicyController creates a new reusable AccessPolicy sync controller.
func NewPolicyController(c client.Client) *PolicyController {
	return &PolicyController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for reusable AccessPolicy.
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *PolicyController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "AccessPolicySync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process AccessPolicy type
	if syncState.Spec.ResourceType != accesssvc.ResourceTypeAccessPolicy {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing AccessPolicy SyncState",
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
	if !controllerutil.ContainsFinalizer(syncState, PolicyFinalizerName) {
		if err := common.AddFinalizerWithRetry(ctx, r.Client, syncState, PolicyFinalizerName); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip if there's a pending debounced request
	if r.Debouncer.IsPending(req.Name) {
		logger.V(1).Info("Skipping reconcile - debounced request pending")
		return ctrl.Result{}, nil
	}

	// Extract AccessPolicy configuration from first source
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract AccessPolicy configuration")
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
		// Even if config hasn't changed, ensure status is "Synced" if resource exists in Cloudflare
		if syncState.Status.SyncStatus != v1alpha2.SyncStatusSynced && syncState.Spec.CloudflareID != "" {
			syncResult := &common.SyncResult{ConfigHash: newHash}
			if err := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusSynced, syncResult, nil); err != nil {
				logger.Error(err, "Failed to update status to Synced")
				return ctrl.Result{Requeue: true}, nil
			}
		}
		logger.V(1).Info("Configuration unchanged, skipping sync", "hash", newHash)
		return ctrl.Result{}, nil
	}

	// Set syncing status
	if err := r.SetSyncStatus(ctx, syncState, v1alpha2.SyncStatusSyncing); err != nil {
		return ctrl.Result{}, err
	}

	// Sync to Cloudflare API
	result, err := r.syncToCloudflare(ctx, syncState, config)
	if err != nil {
		logger.Error(err, "Failed to sync AccessPolicy to Cloudflare")
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

	logger.Info("Successfully synced AccessPolicy to Cloudflare",
		"policyId", result.ID,
		"name", config.Name)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts AccessPolicy configuration from SyncState sources.
// AccessPolicy has 1:1 mapping, so we use the ExtractFirstSourceConfig helper.
func (*PolicyController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*accesssvc.ReusableAccessPolicyConfig, error) {
	return common.ExtractFirstSourceConfig[accesssvc.ReusableAccessPolicyConfig](syncState)
}

// syncToCloudflare syncs the AccessPolicy configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *PolicyController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *accesssvc.ReusableAccessPolicyConfig,
) (*accesssvc.SyncResult, error) {
	logger := log.FromContext(ctx)

	// Create API client using common helper
	apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
	if err != nil {
		return nil, err
	}

	// Validate and set account ID
	accountID, err := common.RequireAccountID(syncState)
	if err != nil {
		return nil, err
	}
	apiClient.ValidAccountId = accountID

	// Build AccessPolicy params
	params := r.buildParams(config)

	// Check if this is an existing policy or new (pending)
	cloudflareID := syncState.Spec.CloudflareID
	var result *cf.ReusableAccessPolicyResult

	if common.IsPendingID(cloudflareID) {
		// Create new AccessPolicy
		logger.Info("Creating new reusable AccessPolicy",
			"name", config.Name)

		result, err = apiClient.CreateReusableAccessPolicy(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("create reusable AccessPolicy: %w", err)
		}

		// Update SyncState CloudflareID with the actual policy ID (must succeed)
		if err := common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID); err != nil {
			return nil, err
		}

		logger.Info("Created reusable AccessPolicy",
			"policyId", result.ID)
	} else {
		// Update existing AccessPolicy
		logger.Info("Updating reusable AccessPolicy",
			"policyId", cloudflareID,
			"name", config.Name)

		result, err = apiClient.UpdateReusableAccessPolicy(ctx, cloudflareID, params)
		if err != nil {
			// Check if policy was deleted externally
			if common.HandleNotFoundOnUpdate(err) {
				logger.Info("AccessPolicy not found, recreating",
					"policyId", cloudflareID)
				result, err = apiClient.CreateReusableAccessPolicy(ctx, params)
				if err != nil {
					return nil, fmt.Errorf("recreate reusable AccessPolicy: %w", err)
				}

				// Update SyncState CloudflareID if ID changed
				if result.ID != cloudflareID {
					if updateErr := common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID); updateErr != nil {
						logger.Error(updateErr, "Failed to update CloudflareID after recreating")
						// Continue - resource exists, next reconcile will retry
					}
				}
			} else {
				return nil, fmt.Errorf("update reusable AccessPolicy: %w", err)
			}
		}

		logger.Info("Updated reusable AccessPolicy",
			"policyId", result.ID)
	}

	return &accesssvc.SyncResult{
		ID:        result.ID,
		AccountID: accountID,
	}, nil
}

// buildParams builds ReusableAccessPolicyParams from config.
func (*PolicyController) buildParams(config *accesssvc.ReusableAccessPolicyConfig) cf.ReusableAccessPolicyParams {
	params := cf.ReusableAccessPolicyParams{
		Name:                         config.Name,
		Decision:                     config.Decision,
		IsolationRequired:            config.IsolationRequired,
		PurposeJustificationRequired: config.PurposeJustificationRequired,
		PurposeJustificationPrompt:   config.PurposeJustificationPrompt,
		ApprovalRequired:             config.ApprovalRequired,
	}

	// Convert SessionDuration string to pointer
	if config.SessionDuration != "" {
		params.SessionDuration = &config.SessionDuration
	}

	// Convert include rules
	if len(config.Include) > 0 {
		params.Include = convertGroupRules(config.Include)
	}

	// Convert exclude rules
	if len(config.Exclude) > 0 {
		params.Exclude = convertGroupRules(config.Exclude)
	}

	// Convert require rules
	if len(config.Require) > 0 {
		params.Require = convertGroupRules(config.Require)
	}

	// Convert approval groups
	if len(config.ApprovalGroups) > 0 {
		params.ApprovalGroups = make([]cf.AccessApprovalGroupParams, len(config.ApprovalGroups))
		for i, ag := range config.ApprovalGroups {
			params.ApprovalGroups[i] = cf.AccessApprovalGroupParams{
				EmailAddresses:  ag.EmailAddresses,
				EmailListUUID:   ag.EmailListUUID,
				ApprovalsNeeded: ag.ApprovalsNeeded,
			}
		}
	}

	return params
}

// handleDeletion handles the deletion of reusable AccessPolicy from Cloudflare.
// This is the SINGLE point for Cloudflare AccessPolicy deletion in the system.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller deletes from Cloudflare
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps and error handling
func (r *PolicyController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, PolicyFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Get the Cloudflare policy ID
	cloudflareID := syncState.Spec.CloudflareID

	// Skip if pending ID (policy was never created)
	if common.IsPendingID(cloudflareID) {
		logger.Info("Skipping deletion - AccessPolicy was never created",
			"cloudflareId", cloudflareID)
	} else if cloudflareID != "" {
		// Create API client
		apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
		if err != nil {
			logger.Error(err, "Failed to create API client for deletion")
			return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
		}

		// Set account ID
		if syncState.Spec.AccountID != "" {
			apiClient.ValidAccountId = syncState.Spec.AccountID
		}

		logger.Info("Deleting reusable AccessPolicy from Cloudflare",
			"policyId", cloudflareID)

		if err := apiClient.DeleteReusableAccessPolicy(ctx, cloudflareID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete reusable AccessPolicy from Cloudflare")
				if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
					logger.Error(statusErr, "Failed to update error status")
				}
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}
			logger.Info("AccessPolicy already deleted from Cloudflare")
		} else {
			logger.Info("Successfully deleted reusable AccessPolicy from Cloudflare",
				"policyId", cloudflareID)
		}
	}

	// Remove finalizer (with conflict retry)
	if err := common.RemoveFinalizerWithRetry(ctx, r.Client, syncState, PolicyFinalizerName); err != nil {
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
func (r *PolicyController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("access-policy-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(accesssvc.ResourceTypeAccessPolicy)).
		Complete(r)
}
