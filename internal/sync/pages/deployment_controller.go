// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pages

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// CRITICAL FIX: When config changes (hash changed) and we have an existing deployment,
	// we need to reset the CloudflareID to trigger a new deployment.
	// Otherwise, handleCreateDeployment will just return the existing deployment info
	// without creating a new one, even though the S3 key or other config has changed.
	cloudflareID := syncState.Spec.CloudflareID
	if !common.IsPendingID(cloudflareID) && cloudflareID != "" {
		// Config changed but we have an existing deployment ID
		// Reset to pending to force creation of a new deployment
		logger.Info("Config changed, resetting CloudflareID to trigger new deployment",
			"oldDeploymentId", cloudflareID,
			"newHash", newHash,
			"oldHash", syncState.Status.ConfigHash)

		// Generate new pending ID
		pendingID := ""
		if len(syncState.Spec.Sources) > 0 {
			src := syncState.Spec.Sources[0]
			pendingID = fmt.Sprintf("pending-%s-%s", src.Ref.Namespace, src.Ref.Name)
		} else {
			pendingID = fmt.Sprintf("pending-%s", syncState.Name)
		}

		if err := common.UpdateCloudflareID(ctx, r.Client, syncState, pendingID); err != nil {
			logger.Error(err, "Failed to reset CloudflareID for new deployment")
			return ctrl.Result{}, err
		}

		// Re-fetch SyncState after update
		if err := r.Client.Get(ctx, client.ObjectKey{Name: syncState.Name}, syncState); err != nil {
			return ctrl.Result{}, err
		}
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

	// Update PagesProject deployment history (non-blocking)
	// This stores the deployment metadata in PagesProject status for visibility and rollback support
	if err := r.updatePagesProjectHistory(ctx, config.ProjectName, result); err != nil {
		// Log but don't fail - deployment succeeded, history update is non-critical
		logger.Error(err, "Failed to update PagesProject history",
			"projectName", config.ProjectName)
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
	// SourceHash is the SHA-256 hash of the source package (for direct upload).
	SourceHash string
	// SourceURL is the URL where source was fetched from (for direct upload).
	SourceURL string
	// K8sResource identifies the K8s resource that created this deployment.
	// Format: "namespace/name"
	K8sResource string
	// Source describes the deployment source type.
	// Examples: "direct-upload:http", "git:main", "rollback:v5"
	Source string
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
		return r.handleRetryDeployment(ctx, apiClient, syncState, config)
	case "rollback":
		return r.handleRollbackDeployment(ctx, apiClient, syncState, config)
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

	// Update SyncState with actual deployment ID (must succeed)
	if err := common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID); err != nil {
		return nil, err
	}

	// Extract K8s resource info from SyncState sources
	k8sResource := ""
	if len(syncState.Spec.Sources) > 0 {
		src := syncState.Spec.Sources[0]
		k8sResource = fmt.Sprintf("%s/%s", src.Ref.Namespace, src.Ref.Name)
	}

	// Determine source description
	branch := config.Branch
	if branch == "" {
		branch = "main"
	}

	logger.Info("Created Pages deployment",
		"deploymentId", result.ID,
		"url", result.URL)

	return &DeploymentResult{
		DeploymentID: result.ID,
		URL:          result.URL,
		Stage:        result.Stage,
		K8sResource:  k8sResource,
		Source:       fmt.Sprintf("git:%s", branch),
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

	// Update SyncState with actual deployment ID (must succeed)
	if err := common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID); err != nil {
		return nil, err
	}

	// Extract K8s resource info from SyncState sources
	k8sResource := ""
	if len(syncState.Spec.Sources) > 0 {
		src := syncState.Spec.Sources[0]
		k8sResource = fmt.Sprintf("%s/%s", src.Ref.Namespace, src.Ref.Name)
	}

	logger.Info("Created direct upload deployment",
		"deploymentId", result.ID,
		"url", result.URL,
		"sourceHash", manifest.SourceHash)

	return &DeploymentResult{
		DeploymentID: result.ID,
		URL:          result.URL,
		Stage:        result.Stage,
		SourceHash:   manifest.SourceHash,
		SourceURL:    manifest.SourceURL,
		K8sResource:  k8sResource,
		Source:       "direct-upload",
	}, nil
}

// handleRetryDeployment retries a failed deployment.
func (*DeploymentSyncController) handleRetryDeployment(
	ctx context.Context,
	apiClient *cf.API,
	syncState *v1alpha2.CloudflareSyncState,
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

	// Extract K8s resource info from SyncState sources
	k8sResource := ""
	if len(syncState.Spec.Sources) > 0 {
		src := syncState.Spec.Sources[0]
		k8sResource = fmt.Sprintf("%s/%s", src.Ref.Namespace, src.Ref.Name)
	}

	logger.Info("Retried Pages deployment",
		"deploymentId", result.ID,
		"url", result.URL)

	return &DeploymentResult{
		DeploymentID: result.ID,
		URL:          result.URL,
		Stage:        result.Stage,
		K8sResource:  k8sResource,
		Source:       fmt.Sprintf("retry:%s", config.TargetDeploymentID),
	}, nil
}

// handleRollbackDeployment rolls back to a previous deployment with smart strategies.
//
//nolint:revive // cognitive complexity is acceptable for multi-strategy rollback logic
func (r *DeploymentSyncController) handleRollbackDeployment(
	ctx context.Context,
	apiClient *cf.API,
	syncState *v1alpha2.CloudflareSyncState,
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

	// Extract K8s resource info from SyncState sources
	k8sResource := ""
	if len(syncState.Spec.Sources) > 0 {
		src := syncState.Spec.Sources[0]
		k8sResource = fmt.Sprintf("%s/%s", src.Ref.Namespace, src.Ref.Name)
	}

	logger.Info("Rolled back to Pages deployment",
		"deploymentId", result.ID,
		"url", result.URL)

	return &DeploymentResult{
		DeploymentID: result.ID,
		URL:          result.URL,
		Stage:        result.Stage,
		K8sResource:  k8sResource,
		Source:       fmt.Sprintf("rollback:%s", targetDeploymentID),
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

// findLastSuccessfulDeployment finds the last successful deployment from PagesProject status.
func (r *DeploymentSyncController) findLastSuccessfulDeployment(
	ctx context.Context,
	projectName string,
) (string, error) {
	// Find the PagesProject
	pagesProject, err := r.findPagesProjectByName(ctx, projectName)
	if err != nil {
		return "", fmt.Errorf("find pagesproject: %w", err)
	}

	if pagesProject == nil {
		return "", errors.New("PagesProject not found")
	}

	// Check LastSuccessfulDeploymentID in status
	if pagesProject.Status.LastSuccessfulDeploymentID != "" {
		return pagesProject.Status.LastSuccessfulDeploymentID, nil
	}

	return "", errors.New("no successful deployment found for rollback")
}

// findDeploymentByVersion finds a deployment ID by version number from PagesProject status history.
func (r *DeploymentSyncController) findDeploymentByVersion(
	ctx context.Context,
	projectName string,
	version int,
) (string, error) {
	// Find the PagesProject
	pagesProject, err := r.findPagesProjectByName(ctx, projectName)
	if err != nil {
		return "", fmt.Errorf("find pagesproject: %w", err)
	}

	if pagesProject == nil {
		return "", errors.New("PagesProject not found")
	}

	// Search deployment history for the version
	for _, entry := range pagesProject.Status.DeploymentHistory {
		if entry.Version == version {
			return entry.DeploymentID, nil
		}
	}

	return "", fmt.Errorf("deployment version %d not found in history", version)
}

// updatePagesProjectHistory updates the PagesProject's deployment history after a successful deployment.
// This stores the history in the PagesProject status for visibility and rollback support.
//
//nolint:revive // cyclomatic complexity is acceptable for history update logic
func (r *DeploymentSyncController) updatePagesProjectHistory(
	ctx context.Context,
	projectName string,
	deployment *DeploymentResult,
) error {
	logger := log.FromContext(ctx)

	// Find the PagesProject by name
	pagesProject, err := r.findPagesProjectByName(ctx, projectName)
	if err != nil {
		logger.Error(err, "Failed to find PagesProject for history update",
			"projectName", projectName)
		// Non-fatal: deployment succeeded, history update failure shouldn't fail the sync
		return nil
	}

	if pagesProject == nil {
		logger.V(1).Info("PagesProject not found for history update",
			"projectName", projectName)
		return nil
	}

	// Use FIFO 200 as default limit
	historyLimit := pagessvc.DefaultDeploymentHistoryLimit
	if pagesProject.Spec.DeploymentHistoryLimit != nil && *pagesProject.Spec.DeploymentHistoryLimit > 0 {
		historyLimit = *pagesProject.Spec.DeploymentHistoryLimit
		if historyLimit > pagessvc.MaxDeploymentHistoryLimit {
			historyLimit = pagessvc.MaxDeploymentHistoryLimit
		}
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Re-fetch to get latest version
		if err := r.Client.Get(ctx, client.ObjectKey{
			Namespace: pagesProject.Namespace,
			Name:      pagesProject.Name,
		}, pagesProject); err != nil {
			return fmt.Errorf("get pagesproject: %w", err)
		}

		// Get existing history
		history := pagesProject.Status.DeploymentHistory
		if history == nil {
			history = []v1alpha2.DeploymentHistoryEntry{}
		}

		// Determine version number
		version := 1
		if len(history) > 0 {
			version = history[0].Version + 1
		}

		// Create new entry with all metadata
		now := metav1.Now()
		newEntry := v1alpha2.DeploymentHistoryEntry{
			DeploymentID: deployment.DeploymentID,
			Version:      version,
			URL:          deployment.URL,
			Source:       deployment.Source,
			SourceHash:   deployment.SourceHash,
			SourceURL:    deployment.SourceURL,
			K8sResource:  deployment.K8sResource,
			CreatedAt:    now,
			Status:       deployment.Stage,
			IsProduction: deployment.Stage == "active" || deployment.Stage == "success",
		}

		// Add new entry at the beginning (newest first)
		history = append([]v1alpha2.DeploymentHistoryEntry{newEntry}, history...)

		// Mark previous production deployments as superseded
		for i := 1; i < len(history); i++ {
			if history[i].IsProduction {
				history[i].IsProduction = false
				history[i].Status = "superseded"
			}
		}

		// Trim to FIFO limit
		if len(history) > historyLimit {
			history = history[:historyLimit]
		}

		// Update PagesProject status
		pagesProject.Status.DeploymentHistory = history

		// Update last successful deployment ID if this is a successful deployment
		if deployment.Stage == "active" || deployment.Stage == "success" || deployment.Stage == "deploy" {
			pagesProject.Status.LastSuccessfulDeploymentID = deployment.DeploymentID
		}

		// Update latest deployment info
		pagesProject.Status.LatestDeployment = &v1alpha2.PagesDeploymentInfo{
			ID:        deployment.DeploymentID,
			URL:       deployment.URL,
			Stage:     deployment.Stage,
			CreatedOn: now.Format("2006-01-02T15:04:05Z"),
		}

		if err := r.Client.Status().Update(ctx, pagesProject); err != nil {
			return fmt.Errorf("update pagesproject status: %w", err)
		}

		logger.Info("Updated PagesProject deployment history",
			"projectName", projectName,
			"deploymentId", deployment.DeploymentID,
			"version", version,
			"historyCount", len(history))

		return nil
	})
}

// findPagesProjectByName finds a PagesProject by its Cloudflare project name.
// The project name is stored in spec.name or defaults to the resource name.
func (r *DeploymentSyncController) findPagesProjectByName(
	ctx context.Context,
	projectName string,
) (*v1alpha2.PagesProject, error) {
	// List all PagesProjects
	var projectList v1alpha2.PagesProjectList
	if err := r.Client.List(ctx, &projectList); err != nil {
		return nil, fmt.Errorf("list pagesprojects: %w", err)
	}

	// Find the one matching the project name
	for i := range projectList.Items {
		project := &projectList.Items[i]
		// Check spec.name first, then fall back to metadata.name
		cfProjectName := project.Spec.Name
		if cfProjectName == "" {
			cfProjectName = project.Name
		}
		if cfProjectName == projectName {
			return project, nil
		}
	}

	return nil, nil
}

// handleDeletion handles the cleanup when a Pages deployment SyncState is being deleted.
//
// IMPORTANT: This function does NOT delete the Cloudflare deployment!
//
// Cloudflare Pages behavior:
// - Active production deployments CANNOT be deleted (error 8000034)
// - Deployments are immutable and serve as version history
// - New deployments automatically replace old ones as the active deployment
//
// Design decision:
// - Deleting PagesDeployment K8s resource only cleans up K8s state
// - The Cloudflare deployment is preserved and will be replaced by the next deployment
// - This avoids the "cannot delete active production deployment" error that blocks finalizer removal
func (r *DeploymentSyncController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, DeploymentFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Get the Cloudflare deployment ID for logging
	cloudflareID := syncState.Spec.CloudflareID

	// Log the cleanup action
	if common.IsPendingID(cloudflareID) {
		logger.Info("Cleaning up SyncState - deployment was never created",
			"cloudflareId", cloudflareID)
	} else if cloudflareID != "" {
		// We intentionally do NOT delete the Cloudflare deployment
		// because:
		// 1. Active production deployments cannot be deleted (Cloudflare API limitation)
		// 2. Deployments are part of project history and should be preserved
		// 3. New deployments will automatically replace old ones
		logger.Info("Cleaning up SyncState - Cloudflare deployment preserved",
			"deploymentId", cloudflareID,
			"reason", "Pages deployments are immutable; active deployment cannot be deleted and will be replaced by next deployment")
	}

	// Remove finalizer - this allows the SyncState to be deleted
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
