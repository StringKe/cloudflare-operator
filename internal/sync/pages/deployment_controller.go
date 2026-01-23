// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pages

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

	// Source type constants
	sourceTypeGit          = "git"
	sourceTypeDirectUpload = "directUpload"

	// Stage constants - Cloudflare Pages deployment stages
	stageQueued       = "queued"       // Deployment is queued
	stageInitializing = "initializing" // Deployment is initializing
	stageBuilding     = "building"     // Deployment is building
	stageDeploying    = "deploying"    // Deployment is deploying
	stageActive       = "active"       // Deployment is active (final - success)
	stageSuccess      = "success"      // Deployment succeeded (final - success)
	stageDeploy       = "deploy"       // Legacy stage name for active
	stageFailure      = "failure"      // Deployment failed (final - error)
	stageIdle         = "idle"         // Deployment is idle (final - success for direct upload)

	// Environment constants
	envProduction = "production"

	// DeploymentPollInterval is the interval to poll deployment status
	DeploymentPollInterval = 30 * time.Second
)

// isDeploymentInProgress checks if a deployment is still in progress (not yet completed)
func isDeploymentInProgress(stage string) bool {
	switch strings.ToLower(stage) {
	case stageQueued, stageInitializing, stageBuilding, stageDeploying:
		return true
	default:
		return false
	}
}

// isDeploymentFailed checks if a deployment has failed
func isDeploymentFailed(stage string) bool {
	return strings.ToLower(stage) == stageFailure
}

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

	// Check if we should reset from Failed state due to Spec changes
	if common.ShouldResetFromFailed(syncState) {
		logger.Info("Resetting from Failed state due to Spec change",
			"previousFailureReason", syncState.Status.FailureReason)
		if err := r.ResetFromFailed(ctx, syncState); err != nil {
			return ctrl.Result{}, err
		}
		// Re-fetch after reset
		if err := r.Client.Get(ctx, client.ObjectKey{Name: syncState.Name}, syncState); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Skip processing if in Failed state (requires Spec change to recover)
	if common.IsFailed(syncState) {
		logger.V(1).Info("SyncState is in Failed state, skipping until Spec changes",
			"failureReason", syncState.Status.FailureReason,
			"failedAt", syncState.Status.FailedAt)
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

		// Use unified error handling for config extraction errors
		errorResult, handleErr := r.HandleSyncError(ctx, syncState, err)
		if handleErr != nil {
			logger.Error(handleErr, "Failed to handle config extraction error")
			return ctrl.Result{}, handleErr
		}

		if errorResult.IsFailed {
			logger.Info("Pages deployment config extraction entered Failed state",
				"failureReason", syncState.Status.FailureReason)
			return ctrl.Result{}, nil
		}

		if errorResult.ShouldRequeue {
			return ctrl.Result{RequeueAfter: errorResult.RequeueAfter}, nil
		}

		return ctrl.Result{}, nil
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
	//
	// IMPORTANT: We must check storedHash != "" to avoid the infinite deployment loop:
	// - When SyncState is first created, ConfigHash is empty
	// - ShouldSync("", "abc123") returns true (hash changed)
	// - Without the storedHash check, we'd think "config changed" and reset CloudflareID
	// - This would create a new deployment, then on next poll (when ConfigHash might still be empty
	//   due to saveConfigHashForInProgress failure), we'd reset again → infinite loop
	cloudflareID := syncState.Spec.CloudflareID
	storedHash := syncState.Status.ConfigHash
	if !common.IsPendingID(cloudflareID) && cloudflareID != "" && storedHash != "" && storedHash != newHash {
		// True config change: we have an existing deployment with a saved hash, and the hash has changed
		// Reset to pending to force creation of a new deployment
		logger.Info("Config changed, resetting CloudflareID to trigger new deployment",
			"oldDeploymentId", cloudflareID,
			"newHash", newHash,
			"oldHash", storedHash)

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

		// Use unified error handling
		errorResult, handleErr := r.HandleSyncError(ctx, syncState, err)
		if handleErr != nil {
			logger.Error(handleErr, "Failed to handle sync error")
			return ctrl.Result{}, handleErr
		}

		if errorResult.IsFailed {
			logger.Info("Pages deployment sync entered Failed state",
				"failureReason", syncState.Status.FailureReason)
			return ctrl.Result{}, nil
		}

		if errorResult.ShouldRequeue {
			return ctrl.Result{RequeueAfter: errorResult.RequeueAfter}, nil
		}

		return ctrl.Result{}, nil
	}

	// Store deployment result data (Stage, URL) in SyncState.ResultData
	// This enables the L2 Controller to retrieve deployment status
	if err := r.storeDeploymentResultData(ctx, syncState, result); err != nil {
		logger.Error(err, "Failed to store deployment result data")
		// Non-fatal, continue
	}

	// Check deployment stage to determine if we need to continue polling
	// Cloudflare Pages deployment is async - stages: queued → initializing → building → deploying → active/success/failure
	if isDeploymentInProgress(result.Stage) {
		// Deployment still in progress - keep Syncing status and poll again
		logger.Info("Deployment in progress, will poll again",
			"deploymentId", result.DeploymentID,
			"stage", result.Stage,
			"pollInterval", DeploymentPollInterval)

		// CRITICAL FIX: Save ConfigHash immediately after deployment creation succeeds.
		// Without this, the next poll iteration will see:
		//   status.ConfigHash = null, newHash = "abc123"
		//   ShouldSync(null, "abc123") = true
		//   CloudflareID is not pending → "config changed" → reset CloudflareID → create another deployment
		// This caused infinite deployment creation loops.
		//
		// IMPORTANT: If saving ConfigHash fails, we MUST NOT continue polling.
		// Otherwise, the next reconcile will see ConfigHash="" and may create another deployment.
		// Returning an error allows the controller to re-queue and retry the entire reconcile,
		// which is safer than continuing with a potentially stale state.
		if err := r.saveConfigHashForInProgress(ctx, syncState, newHash); err != nil {
			logger.Error(err, "Failed to save config hash for in-progress deployment - will retry reconcile")
			return ctrl.Result{}, fmt.Errorf("save config hash for in-progress deployment: %w", err)
		}

		// Keep Syncing status (already set above)
		return ctrl.Result{RequeueAfter: DeploymentPollInterval}, nil
	}

	if isDeploymentFailed(result.Stage) {
		// Deployment failed - transition to Error state
		deploymentErr := fmt.Errorf("deployment failed: stage=%s", result.Stage)
		logger.Error(deploymentErr, "Cloudflare deployment failed",
			"deploymentId", result.DeploymentID,
			"stage", result.Stage)

		// Use unified error handling - this is a permanent error
		errorResult, handleErr := r.HandleSyncError(ctx, syncState, deploymentErr)
		if handleErr != nil {
			return ctrl.Result{}, handleErr
		}

		if errorResult.ShouldRequeue {
			return ctrl.Result{RequeueAfter: errorResult.RequeueAfter}, nil
		}
		return ctrl.Result{}, nil
	}

	// Deployment succeeded - update to Synced status
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

	logger.Info("Deployment completed successfully",
		"deploymentId", result.DeploymentID,
		"stage", result.Stage,
		"url", result.URL)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts Pages deployment configuration from SyncState sources.
func (*DeploymentSyncController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*pagessvc.PagesDeploymentConfig, error) {
	return common.ExtractFirstSourceConfig[pagessvc.PagesDeploymentConfig](syncState)
}

// DeploymentResult contains the result of a deployment action.
type DeploymentResult struct {
	DeploymentID string
	URL          string
	Stage        string
	// HashURL is the unique hash-based URL for this deployment.
	// Format: <hash>.<project>.pages.dev
	HashURL string
	// BranchURL is the branch-based URL for this deployment.
	// Format: <branch>.<project>.pages.dev
	BranchURL string
	// Environment is the deployment environment (production or preview)
	Environment string
	// IsCurrentProduction indicates if this is the current active production deployment
	IsCurrentProduction bool
	// Version is the sequential version number within the project
	Version int
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
// It supports both new (Environment/Source) and legacy (Action) configuration formats.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic with multiple action types
func (r *DeploymentSyncController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *pagessvc.PagesDeploymentConfig,
) (*DeploymentResult, error) {
	logger := log.FromContext(ctx)

	// Create API client using common helper
	apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
	if err != nil {
		return nil, err
	}

	// Validate account ID is present (for logging purposes)
	if _, err := common.RequireAccountID(syncState); err != nil {
		return nil, err
	}

	// Check if using new Source-based configuration
	if config.Source != nil {
		logger.V(1).Info("Using new Source-based deployment configuration",
			"sourceType", config.Source.Type,
			"environment", config.Environment)
		return r.handleSourceBasedDeployment(ctx, apiClient, syncState, config)
	}

	// Legacy: Handle different actions
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

// handleSourceBasedDeployment handles deployments using the new Source configuration.
//
//nolint:revive // cognitive complexity is acceptable for source-based deployment logic
func (r *DeploymentSyncController) handleSourceBasedDeployment(
	ctx context.Context,
	apiClient *cf.API,
	syncState *v1alpha2.CloudflareSyncState,
	config *pagessvc.PagesDeploymentConfig,
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
			DeploymentID:        deployment.ID,
			URL:                 deployment.URL,
			Stage:               deployment.Stage,
			Environment:         config.Environment,
			IsCurrentProduction: config.Environment == envProduction && deployment.Stage == stageActive,
		}, nil
	}

	// Optionally purge build cache before deployment
	if config.PurgeBuildCache {
		logger.Info("Purging build cache", "projectName", config.ProjectName)
		if err := apiClient.PurgePagesProjectBuildCache(ctx, config.ProjectName); err != nil {
			logger.Error(err, "Failed to purge build cache, continuing with deployment")
		}
	}

	// Handle based on source type
	switch config.Source.Type {
	case sourceTypeGit:
		return r.handleGitSourceDeployment(ctx, apiClient, syncState, config)
	case sourceTypeDirectUpload:
		return r.handleDirectUploadSourceDeployment(ctx, apiClient, syncState, config)
	default:
		return nil, fmt.Errorf("unsupported source type: %s", config.Source.Type)
	}
}

// handleGitSourceDeployment handles git-based deployments using the new Source config.
//
//nolint:revive // cognitive complexity is acceptable for git deployment logic
func (r *DeploymentSyncController) handleGitSourceDeployment(
	ctx context.Context,
	apiClient *cf.API,
	syncState *v1alpha2.CloudflareSyncState,
	config *pagessvc.PagesDeploymentConfig,
) (*DeploymentResult, error) {
	logger := log.FromContext(ctx)

	branch := ""
	if config.Source.Git != nil {
		branch = config.Source.Git.Branch
	}

	logger.Info("Creating new git-based Pages deployment",
		"projectName", config.ProjectName,
		"branch", branch,
		"environment", config.Environment)

	result, err := apiClient.CreatePagesDeployment(ctx, config.ProjectName, branch)
	if err != nil {
		return nil, fmt.Errorf("create Pages deployment: %w", err)
	}

	// Update SyncState with actual deployment ID
	if err := common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID); err != nil {
		return nil, err
	}

	// Extract K8s resource info
	k8sResource := ""
	if len(syncState.Spec.Sources) > 0 {
		src := syncState.Spec.Sources[0]
		k8sResource = fmt.Sprintf("%s/%s", src.Ref.Namespace, src.Ref.Name)
	}

	// Build source description
	sourceDesc := "git"
	if branch != "" {
		sourceDesc = fmt.Sprintf("git:%s", branch)
		if config.Source.Git != nil && config.Source.Git.CommitSha != "" {
			sourceDesc = fmt.Sprintf("git:%s@%s", branch, config.Source.Git.CommitSha[:7])
		}
	}

	// Extract hash from URL for HashURL
	hashURL := result.URL // Cloudflare returns the hash URL

	logger.Info("Created git-based Pages deployment",
		"deploymentId", result.ID,
		"url", result.URL,
		"environment", config.Environment)

	return &DeploymentResult{
		DeploymentID:        result.ID,
		URL:                 result.URL,
		Stage:               result.Stage,
		HashURL:             hashURL,
		Environment:         config.Environment,
		IsCurrentProduction: config.Environment == envProduction,
		K8sResource:         k8sResource,
		Source:              sourceDesc,
	}, nil
}

// handleDirectUploadSourceDeployment handles direct upload deployments using the new Source config.
func (r *DeploymentSyncController) handleDirectUploadSourceDeployment(
	ctx context.Context,
	apiClient *cf.API,
	syncState *v1alpha2.CloudflareSyncState,
	config *pagessvc.PagesDeploymentConfig,
) (*DeploymentResult, error) {
	logger := log.FromContext(ctx)

	if config.Source.DirectUpload == nil || config.Source.DirectUpload.Source == nil {
		return nil, errors.New("direct upload source not configured")
	}

	// Get namespace for Secret resolution
	namespace := syncState.Namespace
	if namespace == "" {
		namespace = common.OperatorNamespace
	}

	logger.Info("Processing direct upload source",
		"projectName", config.ProjectName,
		"namespace", namespace,
		"environment", config.Environment)

	// Process source: download, verify checksum, extract archive
	manifest, err := uploader.ProcessSource(
		ctx,
		r.Client,
		namespace,
		config.Source.DirectUpload.Source,
		config.Source.DirectUpload.Checksum,
		config.Source.DirectUpload.Archive,
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
	if err := common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID); err != nil {
		return nil, err
	}

	// Extract K8s resource info
	k8sResource := ""
	if len(syncState.Spec.Sources) > 0 {
		src := syncState.Spec.Sources[0]
		k8sResource = fmt.Sprintf("%s/%s", src.Ref.Namespace, src.Ref.Name)
	}

	logger.Info("Created direct upload deployment",
		"deploymentId", result.ID,
		"url", result.URL,
		"sourceHash", manifest.SourceHash,
		"environment", config.Environment)

	return &DeploymentResult{
		DeploymentID:        result.ID,
		URL:                 result.URL,
		Stage:               result.Stage,
		HashURL:             result.URL,
		Environment:         config.Environment,
		IsCurrentProduction: config.Environment == envProduction,
		SourceHash:          manifest.SourceHash,
		SourceURL:           manifest.SourceURL,
		K8sResource:         k8sResource,
		Source:              "directUpload",
	}, nil
}

// handleCreateDeployment creates a new deployment using legacy configuration.
// This is for backward compatibility with Action-based deployments.
//
//nolint:revive // cognitive complexity is acceptable for create logic
func (r *DeploymentSyncController) handleCreateDeployment(
	ctx context.Context,
	apiClient *cf.API,
	syncState *v1alpha2.CloudflareSyncState,
	config *pagessvc.PagesDeploymentConfig,
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
			DeploymentID:        deployment.ID,
			URL:                 deployment.URL,
			Stage:               deployment.Stage,
			Environment:         config.Environment,
			IsCurrentProduction: config.Environment == envProduction && deployment.Stage == stageActive,
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

	// Check if this is a direct upload deployment (using legacy LegacyDirectUpload field)
	if config.LegacyDirectUpload != nil && config.LegacyDirectUpload.Source != nil {
		return r.handleDirectUploadDeployment(ctx, apiClient, syncState, config)
	}

	// Create new git-based deployment (legacy: uses Branch field from Action config)
	branch := ""
	if config.Source != nil && config.Source.Git != nil {
		branch = config.Source.Git.Branch
	}

	logger.Info("Creating new Pages deployment (legacy mode)",
		"projectName", config.ProjectName,
		"branch", branch)

	result, err := apiClient.CreatePagesDeployment(ctx, config.ProjectName, branch)
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
	sourceDesc := "git"
	if branch != "" {
		sourceDesc = fmt.Sprintf("git:%s", branch)
	}

	logger.Info("Created Pages deployment",
		"deploymentId", result.ID,
		"url", result.URL,
		"environment", config.Environment)

	return &DeploymentResult{
		DeploymentID:        result.ID,
		URL:                 result.URL,
		Stage:               result.Stage,
		HashURL:             result.URL,
		Environment:         config.Environment,
		IsCurrentProduction: config.Environment == envProduction,
		K8sResource:         k8sResource,
		Source:              sourceDesc,
	}, nil
}

// handleDirectUploadDeployment handles direct upload deployments from external sources (legacy mode).
// This uses the LegacyDirectUpload field for backward compatibility.
//
//nolint:revive // cognitive complexity is acceptable for multi-step upload process
func (r *DeploymentSyncController) handleDirectUploadDeployment(
	ctx context.Context,
	apiClient *cf.API,
	syncState *v1alpha2.CloudflareSyncState,
	config *pagessvc.PagesDeploymentConfig,
) (*DeploymentResult, error) {
	logger := log.FromContext(ctx)

	directUpload := config.LegacyDirectUpload
	if directUpload == nil || directUpload.Source == nil {
		return nil, fmt.Errorf("direct upload source not configured")
	}

	// Get namespace for Secret resolution
	namespace := syncState.Namespace
	if namespace == "" {
		namespace = common.OperatorNamespace
	}

	logger.Info("Processing direct upload source (legacy mode)",
		"projectName", config.ProjectName,
		"namespace", namespace,
		"environment", config.Environment)

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
		"sourceHash", manifest.SourceHash,
		"environment", config.Environment)

	return &DeploymentResult{
		DeploymentID:        result.ID,
		URL:                 result.URL,
		Stage:               result.Stage,
		HashURL:             result.URL,
		Environment:         config.Environment,
		IsCurrentProduction: config.Environment == envProduction,
		SourceHash:          manifest.SourceHash,
		SourceURL:           manifest.SourceURL,
		K8sResource:         k8sResource,
		Source:              "directUpload",
	}, nil
}

// handleRetryDeployment retries a failed deployment.
func (*DeploymentSyncController) handleRetryDeployment(
	ctx context.Context,
	apiClient *cf.API,
	syncState *v1alpha2.CloudflareSyncState,
	config *pagessvc.PagesDeploymentConfig,
) (*DeploymentResult, error) {
	logger := log.FromContext(ctx)

	if config.TargetDeploymentID == "" {
		return nil, errors.New("targetDeploymentId is required for retry action")
	}

	logger.Info("Retrying Pages deployment",
		"projectName", config.ProjectName,
		"targetDeploymentId", config.TargetDeploymentID,
		"environment", config.Environment)

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
		"url", result.URL,
		"environment", config.Environment)

	return &DeploymentResult{
		DeploymentID:        result.ID,
		URL:                 result.URL,
		Stage:               result.Stage,
		HashURL:             result.URL,
		Environment:         config.Environment,
		IsCurrentProduction: config.Environment == envProduction,
		K8sResource:         k8sResource,
		Source:              fmt.Sprintf("retry:%s", config.TargetDeploymentID),
	}, nil
}

// handleRollbackDeployment rolls back to a previous deployment with smart strategies.
//
//nolint:revive // cognitive complexity is acceptable for multi-strategy rollback logic
func (r *DeploymentSyncController) handleRollbackDeployment(
	ctx context.Context,
	apiClient *cf.API,
	syncState *v1alpha2.CloudflareSyncState,
	config *pagessvc.PagesDeploymentConfig,
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
		"strategy", config.Rollback,
		"environment", config.Environment)

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
		"url", result.URL,
		"environment", config.Environment)

	return &DeploymentResult{
		DeploymentID:        result.ID,
		URL:                 result.URL,
		Stage:               result.Stage,
		HashURL:             result.URL,
		Environment:         config.Environment,
		IsCurrentProduction: config.Environment == envProduction,
		K8sResource:         k8sResource,
		Source:              fmt.Sprintf("rollback:%s", targetDeploymentID),
	}, nil
}

// resolveRollbackTarget resolves the target deployment ID based on rollback strategy.
func (r *DeploymentSyncController) resolveRollbackTarget(
	ctx context.Context,
	config *pagessvc.PagesDeploymentConfig,
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
			IsProduction: deployment.Stage == stageActive || deployment.Stage == stageSuccess,
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
		if deployment.Stage == stageActive || deployment.Stage == stageSuccess || deployment.Stage == stageDeploy {
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

// storeDeploymentResultData stores the deployment result in SyncState.ResultData.
// This enables the L2 Controller to retrieve deployment status without querying Cloudflare API.
//
// Uses UpdateStatusWithConflictRetry to ensure proper conflict handling and re-fetch
// after successful update, preventing "object has been modified" errors in subsequent operations.
func (r *DeploymentSyncController) storeDeploymentResultData(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	result *DeploymentResult,
) error {
	if result == nil {
		return nil
	}

	return common.UpdateStatusWithConflictRetry(ctx, r.Client, syncState, func() {
		// Initialize ResultData if nil
		if syncState.Status.ResultData == nil {
			syncState.Status.ResultData = make(map[string]string)
		}

		// Store deployment result - basic fields
		syncState.Status.ResultData["deploymentId"] = result.DeploymentID
		syncState.Status.ResultData["stage"] = result.Stage
		syncState.Status.ResultData["url"] = result.URL

		// Store new fields for enhanced status reporting
		if result.HashURL != "" {
			syncState.Status.ResultData["hashUrl"] = result.HashURL
		}
		if result.BranchURL != "" {
			syncState.Status.ResultData["branchUrl"] = result.BranchURL
		}
		if result.Environment != "" {
			syncState.Status.ResultData["environment"] = result.Environment
		}
		if result.IsCurrentProduction {
			syncState.Status.ResultData["isCurrentProduction"] = "true"
		}
		if result.Version > 0 {
			syncState.Status.ResultData["version"] = fmt.Sprintf("%d", result.Version)
		}
		if result.SourceHash != "" {
			syncState.Status.ResultData["sourceHash"] = result.SourceHash
		}
		if result.Source != "" {
			syncState.Status.ResultData["sourceDescription"] = result.Source
		}
	})
}

// saveConfigHashForInProgress saves the config hash while deployment is still in progress.
// This is CRITICAL to prevent the "config changed" detection loop:
//   - Without this, next reconcile sees ConfigHash=null vs newHash="abc"
//   - ShouldSync returns true, CloudflareID gets reset, new deployment created
//   - This causes infinite deployment creation loops
func (r *DeploymentSyncController) saveConfigHashForInProgress(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	configHash string,
) error {
	return common.UpdateStatusWithConflictRetry(ctx, r.Client, syncState, func() {
		syncState.Status.ConfigHash = configHash
		// Keep the Syncing status - don't change other fields
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeploymentSyncController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("pages-deployment-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourcePagesDeployment)).
		Complete(r)
}
