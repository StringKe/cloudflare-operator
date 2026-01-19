// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	pagessvc "github.com/StringKe/cloudflare-operator/internal/service/pages"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
	"github.com/StringKe/cloudflare-operator/internal/uploader"
)

const (
	// DeploymentFinalizerName is the finalizer for Pages Deployment SyncState resources.
	DeploymentFinalizerName = "pages-deployment.sync.cloudflare-operator.io/finalizer"
)

// DeploymentSyncController is the Sync Controller for Pages Deployment Configuration.
// It watches CloudflareSyncState resources of type PagesDeployment,
// extracts the configuration, and syncs to Cloudflare API.
// This is the SINGLE point that calls Cloudflare Pages Deployment API.
type DeploymentSyncController struct {
	*common.BaseSyncController
}

// NewDeploymentSyncController creates a new DeploymentSyncController
func NewDeploymentSyncController(c client.Client) *DeploymentSyncController {
	return &DeploymentSyncController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for Pages deployment.
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *DeploymentSyncController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "PagesDeploymentSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process PagesDeployment type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourcePagesDeployment {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing Pages Deployment SyncState",
		"cloudflareId", syncState.Spec.CloudflareID,
		"sources", len(syncState.Spec.Sources))

	// Handle deletion
	if !syncState.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, syncState)
	}

	// Check if there are any sources - if none, delete from Cloudflare
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, deleting from Cloudflare")
		return r.handleDeletion(ctx, syncState)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(syncState, DeploymentFinalizerName) {
		if err := common.AddFinalizerWithRetry(ctx, r.Client, syncState, DeploymentFinalizerName); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip if there's a pending debounced request
	if r.Debouncer.IsPending(req.Name) {
		logger.V(1).Info("Skipping reconcile - debounced request pending")
		return ctrl.Result{}, nil
	}

	// Extract Pages deployment configuration from first source
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract Pages deployment configuration")
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
		logger.Error(err, "Failed to sync Pages deployment to Cloudflare")
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

	logger.Info("Successfully synced Pages deployment to Cloudflare",
		"deploymentId", result.DeploymentID,
		"action", config.Action)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts Pages deployment configuration from SyncState sources.
func (*DeploymentSyncController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*pagessvc.PagesDeploymentActionConfig, error) {
	return common.ExtractFirstSourceConfig[pagessvc.PagesDeploymentActionConfig](syncState)
}

// DeploymentResult contains the result of a deployment action.
type DeploymentResult struct {
	DeploymentID string
	URL          string
	Stage        string
}

// syncToCloudflare syncs the Pages deployment configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic with multiple action types
func (r *DeploymentSyncController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *pagessvc.PagesDeploymentActionConfig,
) (*DeploymentResult, error) {
	// Create API client using common helper
	apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
	if err != nil {
		return nil, err
	}

	// Validate account ID is present (for logging purposes)
	if _, err := common.RequireAccountID(syncState); err != nil {
		return nil, err
	}

	// Handle different actions
	switch config.Action {
	case "create", "":
		return r.handleCreateDeployment(ctx, apiClient, syncState, config)
	case "retry":
		return r.handleRetryDeployment(ctx, apiClient, config)
	case "rollback":
		return r.handleRollbackDeployment(ctx, apiClient, config)
	default:
		return nil, fmt.Errorf("unsupported deployment action: %s", config.Action)
	}
}

// handleCreateDeployment creates a new deployment.
//
//nolint:revive // cognitive complexity is acceptable for create logic
func (r *DeploymentSyncController) handleCreateDeployment(
	ctx context.Context,
	apiClient *cf.API,
	syncState *v1alpha2.CloudflareSyncState,
	config *pagessvc.PagesDeploymentActionConfig,
) (*DeploymentResult, error) {
	logger := log.FromContext(ctx)

	cloudflareID := syncState.Spec.CloudflareID

	// If we already have a deployment ID (not pending), we're just monitoring it
	if !common.IsPendingID(cloudflareID) && cloudflareID != "" {
		// Get existing deployment status
		deployment, err := apiClient.GetPagesDeployment(ctx, config.ProjectName, cloudflareID)
		if err != nil {
			return nil, fmt.Errorf("get deployment status: %w", err)
		}

		return &DeploymentResult{
			DeploymentID: deployment.ID,
			URL:          deployment.URL,
			Stage:        deployment.Stage,
		}, nil
	}

	// Optionally purge build cache before deployment
	if config.PurgeBuildCache {
		logger.Info("Purging build cache",
			"projectName", config.ProjectName)
		if err := apiClient.PurgePagesProjectBuildCache(ctx, config.ProjectName); err != nil {
			logger.Error(err, "Failed to purge build cache, continuing with deployment")
			// Non-fatal error, continue with deployment
		}
	}

	// Check if this is a direct upload deployment
	if config.DirectUpload != nil && config.DirectUpload.Source != nil {
		return r.handleDirectUploadDeployment(ctx, apiClient, syncState, config)
	}

	// Create new git-based deployment
	logger.Info("Creating new Pages deployment",
		"projectName", config.ProjectName,
		"branch", config.Branch)

	result, err := apiClient.CreatePagesDeployment(ctx, config.ProjectName, config.Branch)
	if err != nil {
		return nil, fmt.Errorf("create Pages deployment: %w", err)
	}

	// Update SyncState with actual deployment ID
	common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID)

	logger.Info("Created Pages deployment",
		"deploymentId", result.ID,
		"url", result.URL)

	return &DeploymentResult{
		DeploymentID: result.ID,
		URL:          result.URL,
		Stage:        result.Stage,
	}, nil
}

// handleDirectUploadDeployment handles direct upload deployments from external sources.
//
//nolint:revive // cognitive complexity is acceptable for multi-step upload process
func (r *DeploymentSyncController) handleDirectUploadDeployment(
	ctx context.Context,
	apiClient *cf.API,
	syncState *v1alpha2.CloudflareSyncState,
	config *pagessvc.PagesDeploymentActionConfig,
) (*DeploymentResult, error) {
	logger := log.FromContext(ctx)

	directUpload := config.DirectUpload
	if directUpload == nil || directUpload.Source == nil {
		return nil, fmt.Errorf("direct upload source not configured")
	}

	// Get namespace for Secret resolution
	namespace := syncState.Namespace
	if namespace == "" {
		namespace = common.OperatorNamespace
	}

	logger.Info("Processing direct upload source",
		"projectName", config.ProjectName,
		"namespace", namespace)

	// Process source: download, verify checksum, extract archive
	manifest, err := uploader.ProcessSource(
		ctx,
		r.Client,
		namespace,
		directUpload.Source,
		directUpload.Checksum,
		directUpload.Archive,
	)
	if err != nil {
		return nil, fmt.Errorf("process source: %w", err)
	}

	logger.Info("Extracted files from source",
		"fileCount", manifest.FileCount,
		"totalSize", manifest.TotalSize)

	// Call Cloudflare Direct Upload API
	result, err := apiClient.CreatePagesDirectUploadDeployment(ctx, config.ProjectName, manifest.Files)
	if err != nil {
		return nil, fmt.Errorf("create direct upload deployment: %w", err)
	}

	// Update SyncState with actual deployment ID
	common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID)

	logger.Info("Created direct upload deployment",
		"deploymentId", result.ID,
		"url", result.URL)

	return &DeploymentResult{
		DeploymentID: result.ID,
		URL:          result.URL,
		Stage:        result.Stage,
	}, nil
}

// handleRetryDeployment retries a failed deployment.
func (*DeploymentSyncController) handleRetryDeployment(
	ctx context.Context,
	apiClient *cf.API,
	config *pagessvc.PagesDeploymentActionConfig,
) (*DeploymentResult, error) {
	logger := log.FromContext(ctx)

	if config.TargetDeploymentID == "" {
		return nil, errors.New("targetDeploymentId is required for retry action")
	}

	logger.Info("Retrying Pages deployment",
		"projectName", config.ProjectName,
		"targetDeploymentId", config.TargetDeploymentID)

	result, err := apiClient.RetryPagesDeployment(ctx, config.ProjectName, config.TargetDeploymentID)
	if err != nil {
		return nil, fmt.Errorf("retry Pages deployment: %w", err)
	}

	logger.Info("Retried Pages deployment",
		"deploymentId", result.ID,
		"url", result.URL)

	return &DeploymentResult{
		DeploymentID: result.ID,
		URL:          result.URL,
		Stage:        result.Stage,
	}, nil
}

// handleRollbackDeployment rolls back to a previous deployment with smart strategies.
//
//nolint:revive // cognitive complexity is acceptable for multi-strategy rollback logic
func (r *DeploymentSyncController) handleRollbackDeployment(
	ctx context.Context,
	apiClient *cf.API,
	config *pagessvc.PagesDeploymentActionConfig,
) (*DeploymentResult, error) {
	logger := log.FromContext(ctx)

	var targetDeploymentID string
	var err error

	// Determine rollback target based on configuration
	switch {
	case config.Rollback != nil && config.Rollback.Strategy != "":
		targetDeploymentID, err = r.resolveRollbackTarget(ctx, config)
		if err != nil {
			return nil, fmt.Errorf("resolve rollback target: %w", err)
		}
	case config.TargetDeploymentID != "":
		// Use explicit target deployment ID
		targetDeploymentID = config.TargetDeploymentID
	default:
		return nil, errors.New("either rollback config or targetDeploymentId is required for rollback action")
	}

	logger.Info("Rolling back to Pages deployment",
		"projectName", config.ProjectName,
		"targetDeploymentId", targetDeploymentID,
		"strategy", config.Rollback)

	result, err := apiClient.RollbackPagesDeployment(ctx, config.ProjectName, targetDeploymentID)
	if err != nil {
		return nil, fmt.Errorf("rollback Pages deployment: %w", err)
	}

	logger.Info("Rolled back to Pages deployment",
		"deploymentId", result.ID,
		"url", result.URL)

	return &DeploymentResult{
		DeploymentID: result.ID,
		URL:          result.URL,
		Stage:        result.Stage,
	}, nil
}

// resolveRollbackTarget resolves the target deployment ID based on rollback strategy.
func (r *DeploymentSyncController) resolveRollbackTarget(
	ctx context.Context,
	config *pagessvc.PagesDeploymentActionConfig,
) (string, error) {
	rollback := config.Rollback
	if rollback == nil {
		return "", errors.New("rollback config is nil")
	}

	switch rollback.Strategy {
	case v1alpha2.RollbackStrategyLastSuccessful:
		return r.findLastSuccessfulDeployment(ctx, config.ProjectName)

	case v1alpha2.RollbackStrategyByVersion:
		if rollback.Version == nil {
			return "", errors.New("version is required for ByVersion rollback strategy")
		}
		return r.findDeploymentByVersion(ctx, config.ProjectName, *rollback.Version)

	case v1alpha2.RollbackStrategyExactDeploymentID:
		if rollback.DeploymentID == "" {
			return "", errors.New("deploymentId is required for ExactDeploymentID rollback strategy")
		}
		return rollback.DeploymentID, nil

	default:
		return "", fmt.Errorf("unknown rollback strategy: %s", rollback.Strategy)
	}
}

// findLastSuccessfulDeployment finds the last successful deployment from project history.
func (r *DeploymentSyncController) findLastSuccessfulDeployment(
	ctx context.Context,
	projectName string,
) (string, error) {
	// Get project SyncState
	projectSyncState, err := r.getProjectSyncState(ctx, projectName)
	if err != nil {
		return "", fmt.Errorf("get project syncstate: %w", err)
	}

	// Check annotation for last successful deployment
	if projectSyncState.Annotations != nil {
		if deploymentID := projectSyncState.Annotations["cloudflare-operator.io/last-successful-deployment"]; deploymentID != "" {
			return deploymentID, nil
		}
	}

	return "", errors.New("no successful deployment found for rollback")
}

// findDeploymentByVersion finds a deployment ID by version number from history.
func (r *DeploymentSyncController) findDeploymentByVersion(
	ctx context.Context,
	projectName string,
	version int,
) (string, error) {
	// Get project SyncState
	projectSyncState, err := r.getProjectSyncState(ctx, projectName)
	if err != nil {
		return "", fmt.Errorf("get project syncstate: %w", err)
	}

	// Parse deployment history from annotations
	if projectSyncState.Annotations == nil {
		return "", errors.New("no deployment history found")
	}

	historyJSON := projectSyncState.Annotations["cloudflare-operator.io/deployment-history"]
	if historyJSON == "" {
		return "", errors.New("no deployment history found")
	}

	var history []v1alpha2.DeploymentHistoryEntry
	if err := json.Unmarshal([]byte(historyJSON), &history); err != nil {
		return "", fmt.Errorf("parse deployment history: %w", err)
	}

	for _, entry := range history {
		if entry.Version == version {
			return entry.DeploymentID, nil
		}
	}

	return "", fmt.Errorf("deployment version %d not found in history", version)
}

// getProjectSyncState retrieves the project's SyncState.
func (r *DeploymentSyncController) getProjectSyncState(
	ctx context.Context,
	projectName string,
) (*v1alpha2.CloudflareSyncState, error) {
	syncStateName := fmt.Sprintf("pages-project-%s", projectName)

	syncState := &v1alpha2.CloudflareSyncState{}
	if err := r.Client.Get(ctx, client.ObjectKey{
		Namespace: common.OperatorNamespace,
		Name:      syncStateName,
	}, syncState); err != nil {
		return nil, err
	}

	return syncState, nil
}

// updateDeploymentHistory updates the project's deployment history after a successful deployment.
// TODO: Connect this to the sync flow when deployment history tracking is fully implemented.
//
//nolint:revive,unused // cyclomatic complexity is acceptable; function will be connected in future PR
func (r *DeploymentSyncController) updateDeploymentHistory(
	ctx context.Context,
	projectName string,
	deployment *DeploymentResult,
	source string,
	historyLimit int,
) error {
	if historyLimit <= 0 {
		historyLimit = 10
	}

	syncStateName := fmt.Sprintf("pages-project-%s", projectName)

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		syncState := &v1alpha2.CloudflareSyncState{}
		if err := r.Client.Get(ctx, client.ObjectKey{
			Namespace: common.OperatorNamespace,
			Name:      syncStateName,
		}, syncState); err != nil {
			return fmt.Errorf("get project syncstate: %w", err)
		}

		if syncState.Annotations == nil {
			syncState.Annotations = make(map[string]string)
		}

		// Parse existing history
		var history []v1alpha2.DeploymentHistoryEntry
		if historyJSON := syncState.Annotations["cloudflare-operator.io/deployment-history"]; historyJSON != "" {
			if err := json.Unmarshal([]byte(historyJSON), &history); err != nil {
				// Log but continue with empty history
				log.FromContext(ctx).Error(err, "Failed to parse existing deployment history")
				history = nil
			}
		}

		// Determine version number
		version := 1
		if len(history) > 0 {
			version = history[0].Version + 1
		}

		// Add new entry at the beginning
		newEntry := v1alpha2.DeploymentHistoryEntry{
			DeploymentID: deployment.DeploymentID,
			Version:      version,
			URL:          deployment.URL,
			Source:       source,
			Status:       deployment.Stage,
		}

		history = append([]v1alpha2.DeploymentHistoryEntry{newEntry}, history...)

		// Trim to limit
		if len(history) > historyLimit {
			history = history[:historyLimit]
		}

		// Save history
		historyJSON, err := json.Marshal(history)
		if err != nil {
			return fmt.Errorf("marshal history: %w", err)
		}
		syncState.Annotations["cloudflare-operator.io/deployment-history"] = string(historyJSON)

		// Update last successful deployment if appropriate
		if deployment.Stage == "active" || deployment.Stage == "success" || deployment.Stage == "deploy" {
			syncState.Annotations["cloudflare-operator.io/last-successful-deployment"] = deployment.DeploymentID
		}

		return r.Client.Update(ctx, syncState)
	})
}

// handleDeletion handles the deletion of Pages deployment from Cloudflare.
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps
func (r *DeploymentSyncController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, DeploymentFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Get the Cloudflare deployment ID
	cloudflareID := syncState.Spec.CloudflareID

	// Skip if pending ID (deployment was never created)
	if common.IsPendingID(cloudflareID) {
		logger.Info("Skipping deletion - Pages deployment was never created",
			"cloudflareId", cloudflareID)
	} else if cloudflareID != "" {
		// Extract config to get project name
		config, err := r.extractConfig(syncState)
		if err != nil {
			logger.Error(err, "Failed to extract config for deletion")
			// Continue to remove finalizer even if we can't extract config
		} else {
			// Create API client
			apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
			if err != nil {
				logger.Error(err, "Failed to create API client for deletion")
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}

			logger.Info("Deleting Pages deployment from Cloudflare",
				"deploymentId", cloudflareID,
				"projectName", config.ProjectName)

			if err := apiClient.DeletePagesDeployment(ctx, config.ProjectName, cloudflareID); err != nil {
				if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to delete Pages deployment from Cloudflare")
					if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
						logger.Error(statusErr, "Failed to update error status")
					}
					return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
				}
				logger.Info("Pages deployment already deleted from Cloudflare")
			} else {
				logger.Info("Successfully deleted Pages deployment from Cloudflare",
					"deploymentId", cloudflareID)
			}
		}
	}

	// Remove finalizer
	if err := common.RemoveFinalizerWithRetry(ctx, r.Client, syncState, DeploymentFinalizerName); err != nil {
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
func (r *DeploymentSyncController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("pages-deployment-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourcePagesDeployment)).
		Complete(r)
}
