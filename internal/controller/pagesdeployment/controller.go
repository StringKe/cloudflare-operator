// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package pagesdeployment implements the Controller for PagesDeployment CRD.
// This controller directly calls Cloudflare API and writes status back to CRD,
// following the simplified 3-layer architecture (CRD → Controller → CF API).
package pagesdeployment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/controller/common"
	"github.com/StringKe/cloudflare-operator/internal/uploader"
)

const (
	FinalizerName = "pagesdeployment.networking.cloudflare-operator.io/finalizer"

	// AnnotationForceRedeploy is the annotation key to force a new deployment.
	// When this annotation value changes, a new deployment will be triggered
	// even if the spec hasn't changed. This is useful for re-deploying the
	// same configuration.
	AnnotationForceRedeploy = "cloudflare-operator.io/force-redeploy"

	// AnnotationLastForceRedeploy stores the last processed force-redeploy value
	AnnotationLastForceRedeploy = "cloudflare-operator.io/last-force-redeploy"

	// EventReasonProductionConflict indicates another production deployment exists
	EventReasonProductionConflict = "ProductionConflict"
	// EventReasonProductionProtected indicates production deletion is blocked
	EventReasonProductionProtected = "ProductionProtected"
	// EventReasonDeprecationWarning indicates deprecated fields are being used
	EventReasonDeprecationWarning = "DeprecationWarning"
	// EventReasonDeploymentCreated indicates a new deployment was created
	EventReasonDeploymentCreated = "DeploymentCreated"
	// EventReasonDeploymentPolling indicates polling deployment status
	EventReasonDeploymentPolling = "DeploymentPolling"
	// EventReasonDeploymentSucceeded indicates deployment succeeded
	EventReasonDeploymentSucceeded = "DeploymentSucceeded"
	// EventReasonDeploymentFailed indicates deployment failed
	EventReasonDeploymentFailed = "DeploymentFailed"

	// PollingInterval is the interval for polling in-progress deployments
	PollingInterval = 30 * time.Second
)

// PagesDeploymentReconciler reconciles a PagesDeployment object.
// It directly calls Cloudflare API and writes status back to CRD,
// following the simplified 3-layer architecture.
type PagesDeploymentReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=pagesdeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=pagesdeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=pagesdeployments/finalizers,verbs=update

//nolint:revive // cognitive complexity is acceptable for this reconcile loop
func (r *PagesDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the PagesDeployment instance
	deployment := &networkingv1alpha2.PagesDeployment{}
	if err := r.Get(ctx, req.NamespacedName, deployment); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Apply backward compatibility adapter
	r.applyBackwardCompatibilityAdapter(ctx, deployment)

	// Handle deletion FIRST - before resolving external dependencies
	// This ensures finalizer can be removed even if PagesProject is unavailable
	if !deployment.DeletionTimestamp.IsZero() {
		projectName, err := r.resolveProjectName(ctx, deployment)
		if err != nil {
			// Project resolution failed during deletion - force remove finalizer
			logger.Info("Project resolution failed during deletion, forcing finalizer removal", "error", err.Error())
			return r.forceRemoveFinalizer(ctx, deployment)
		}
		return r.handleDeletion(ctx, deployment, projectName)
	}

	// Resolve project name (only for non-deletion reconciliation)
	projectName, err := r.resolveProjectName(ctx, deployment)
	if err != nil {
		logger.Error(err, "Failed to resolve project name")
		return r.setErrorStatus(ctx, deployment, err)
	}

	// Validate production uniqueness
	if deployment.Spec.Environment == networkingv1alpha2.PagesDeploymentEnvironmentProduction {
		if err := r.validateProductionUniqueness(ctx, deployment, projectName); err != nil {
			logger.Error(err, "Production uniqueness validation failed")
			r.Recorder.Event(deployment, corev1.EventTypeWarning, EventReasonProductionConflict, err.Error())
			return r.setErrorStatus(ctx, deployment, err)
		}
	}

	// Get API client
	apiResult, err := r.getAPIClient(ctx, deployment)
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.setErrorStatus(ctx, deployment, err)
	}

	// Ensure finalizer
	if !controllerutil.ContainsFinalizer(deployment, FinalizerName) {
		controllerutil.AddFinalizer(deployment, FinalizerName)
		if err := r.Update(ctx, deployment); err != nil {
			return ctrl.Result{}, err
		}
		// Re-fetch to get updated version
		if err := r.Get(ctx, req.NamespacedName, deployment); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Determine if we need to create a new deployment
	needsDeployment := r.needsNewDeployment(deployment)

	if needsDeployment {
		// Create new deployment
		return r.createDeployment(ctx, deployment, projectName, apiResult)
	}

	// Check existing deployment status
	if deployment.Status.DeploymentID != "" {
		return r.pollDeploymentStatus(ctx, deployment, projectName, apiResult)
	}

	// No deployment ID and no need for new deployment - should not happen
	logger.Info("No deployment ID and no need for new deployment")
	return r.setErrorStatus(ctx, deployment, errors.New("no deployment ID found"))
}

// getAPIClient returns a Cloudflare API client for the deployment.
func (r *PagesDeploymentReconciler) getAPIClient(ctx context.Context, deployment *networkingv1alpha2.PagesDeployment) (*common.APIClientResult, error) {
	return r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &deployment.Spec.Cloudflare,
		Namespace:         deployment.Namespace,
		StatusAccountID:   deployment.Status.AccountID,
	})
}

// needsNewDeployment determines if a new deployment should be created.
//
//nolint:revive // cognitive complexity acceptable for deployment decision logic
func (r *PagesDeploymentReconciler) needsNewDeployment(deployment *networkingv1alpha2.PagesDeployment) bool {
	// No deployment ID yet
	if deployment.Status.DeploymentID == "" {
		return true
	}

	// Check force-redeploy annotation
	currentForceRedeploy := ""
	if deployment.Annotations != nil {
		currentForceRedeploy = deployment.Annotations[AnnotationForceRedeploy]
	}
	lastForceRedeploy := ""
	if deployment.Annotations != nil {
		lastForceRedeploy = deployment.Annotations[AnnotationLastForceRedeploy]
	}
	if currentForceRedeploy != "" && currentForceRedeploy != lastForceRedeploy {
		return true
	}

	// Check if spec changed (generation changed and deployment succeeded)
	if deployment.Generation != deployment.Status.ObservedGeneration &&
		(deployment.Status.State == networkingv1alpha2.PagesDeploymentStateSucceeded ||
			deployment.Status.State == networkingv1alpha2.PagesDeploymentStateFailed ||
			deployment.Status.State == networkingv1alpha2.PagesDeploymentStateCancelled) {
		return true
	}

	return false
}

// createDeployment creates a new Cloudflare Pages deployment.
//
//nolint:revive // cognitive complexity acceptable for deployment creation
func (r *PagesDeploymentReconciler) createDeployment(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
	projectName string,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	api := apiResult.API

	logger.Info("Creating new Pages deployment",
		"project", projectName,
		"environment", deployment.Spec.Environment,
		"sourceType", r.getSourceType(deployment))

	var result *cf.PagesDeploymentResult
	var err error

	// Determine source type and create deployment
	if deployment.Spec.Source != nil {
		switch deployment.Spec.Source.Type {
		case networkingv1alpha2.PagesDeploymentSourceTypeGit:
			// Git-based deployment
			branch := ""
			if deployment.Spec.Source.Git != nil {
				branch = deployment.Spec.Source.Git.Branch
			}
			result, err = api.CreatePagesDeployment(ctx, projectName, branch)

		case networkingv1alpha2.PagesDeploymentSourceTypeDirectUpload:
			// Direct upload deployment
			if deployment.Spec.Source.DirectUpload == nil || deployment.Spec.Source.DirectUpload.Source == nil {
				return r.setErrorStatus(ctx, deployment, errors.New("direct upload source is required"))
			}
			files, loadErr := r.loadDirectUploadFiles(ctx, deployment)
			if loadErr != nil {
				return r.setErrorStatus(ctx, deployment, fmt.Errorf("failed to load files: %w", loadErr))
			}

			// Build deployment metadata
			metadata := r.buildDeploymentMetadata(deployment)

			directResult, uploadErr := api.CreatePagesDirectUploadDeployment(ctx, projectName, files, metadata)
			if uploadErr != nil {
				return r.setErrorStatus(ctx, deployment, fmt.Errorf("failed to create direct upload deployment: %w", uploadErr))
			}
			// Convert direct upload result to deployment result
			result = &cf.PagesDeploymentResult{
				ID:          directResult.ID,
				URL:         directResult.URL,
				Stage:       directResult.Stage,
				ProjectName: projectName,
			}

		default:
			return r.setErrorStatus(ctx, deployment, fmt.Errorf("unsupported source type: %s", deployment.Spec.Source.Type))
		}
	} else if deployment.Spec.Action == networkingv1alpha2.PagesDeploymentActionRollback {
		// Rollback action
		targetID := deployment.Spec.TargetDeploymentID
		if deployment.Spec.Rollback != nil && deployment.Spec.Rollback.DeploymentID != "" {
			targetID = deployment.Spec.Rollback.DeploymentID
		}
		if targetID == "" {
			return r.setErrorStatus(ctx, deployment, errors.New("rollback target deployment ID is required"))
		}
		result, err = api.RollbackPagesDeployment(ctx, projectName, targetID)
	} else {
		// Default: git deployment with default branch
		branch := deployment.Spec.Branch // Legacy field
		result, err = api.CreatePagesDeployment(ctx, projectName, branch)
	}

	if err != nil {
		logger.Error(err, "Failed to create Pages deployment")
		return r.setErrorStatus(ctx, deployment, err)
	}

	// Record event
	r.Recorder.Event(deployment, corev1.EventTypeNormal, EventReasonDeploymentCreated,
		fmt.Sprintf("Created deployment %s (stage: %s)", result.ID, result.Stage))

	// Update force-redeploy tracking
	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}
	if forceRedeploy := deployment.Annotations[AnnotationForceRedeploy]; forceRedeploy != "" {
		deployment.Annotations[AnnotationLastForceRedeploy] = forceRedeploy
		if updateErr := r.Update(ctx, deployment); updateErr != nil {
			logger.Error(updateErr, "Failed to update force-redeploy annotation")
			// Don't fail reconciliation for this
		}
	}

	// Update status with deployment info
	return r.updateDeploymentStatus(ctx, deployment, projectName, apiResult.AccountID, result)
}

// pollDeploymentStatus polls the status of an existing deployment.
func (r *PagesDeploymentReconciler) pollDeploymentStatus(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
	projectName string,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	api := apiResult.API

	// Get deployment status from Cloudflare
	result, err := api.GetPagesDeployment(ctx, projectName, deployment.Status.DeploymentID)
	if err != nil {
		if cf.IsNotFoundError(err) {
			logger.Info("Deployment not found, will create new one")
			// Clear deployment ID and retry
			deployment.Status.DeploymentID = ""
			if updateErr := r.Status().Update(ctx, deployment); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Failed to get deployment status")
		return r.setErrorStatus(ctx, deployment, err)
	}

	// Update status
	return r.updateDeploymentStatus(ctx, deployment, projectName, apiResult.AccountID, result)
}

// updateDeploymentStatus updates the CRD status based on deployment result.
//
//nolint:revive // cognitive complexity acceptable for status update with multiple fields
func (r *PagesDeploymentReconciler) updateDeploymentStatus(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
	projectName string,
	accountID string,
	result *cf.PagesDeploymentResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, deployment, func() {
		deployment.Status.AccountID = accountID
		deployment.Status.ProjectName = projectName
		deployment.Status.DeploymentID = result.ID
		deployment.Status.URL = result.URL
		deployment.Status.Environment = result.Environment

		// Extract HashURL from Aliases (first .pages.dev URL containing the short ID)
		deployment.Status.HashURL = extractHashURL(result.Aliases, result.ShortID, projectName)

		// Extract VersionName from labels or deployment name
		deployment.Status.VersionName = extractVersionName(deployment)

		// Determine state based on stage
		stage := result.Stage
		if result.StageStatus != "" && result.StageStatus != "active" {
			stage = result.StageStatus
		}

		switch stage {
		case "queued":
			deployment.Status.State = networkingv1alpha2.PagesDeploymentStateQueued
		case "initialize", "clone_repo", "build":
			deployment.Status.State = networkingv1alpha2.PagesDeploymentStateBuilding
		case "deploy":
			deployment.Status.State = networkingv1alpha2.PagesDeploymentStateDeploying
		case "success", "active":
			deployment.Status.State = networkingv1alpha2.PagesDeploymentStateSucceeded
		case "failure", "failed":
			deployment.Status.State = networkingv1alpha2.PagesDeploymentStateFailed
		case "canceled", "cancelled":
			deployment.Status.State = networkingv1alpha2.PagesDeploymentStateCancelled
		default:
			deployment.Status.State = networkingv1alpha2.PagesDeploymentStatePending
		}

		// Build condition
		conditionStatus := metav1.ConditionFalse
		reason := "InProgress"
		message := fmt.Sprintf("Deployment is %s", stage)

		switch deployment.Status.State {
		case networkingv1alpha2.PagesDeploymentStateSucceeded:
			conditionStatus = metav1.ConditionTrue
			reason = "Succeeded"
			message = fmt.Sprintf("Deployment succeeded: %s", result.URL)
		case networkingv1alpha2.PagesDeploymentStateFailed:
			reason = "Failed"
			message = "Deployment failed"
		case networkingv1alpha2.PagesDeploymentStateCancelled:
			reason = "Cancelled"
			message = "Deployment was cancelled"
		}

		meta.SetStatusCondition(&deployment.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             conditionStatus,
			ObservedGeneration: deployment.Generation,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: metav1.Now(),
		})

		deployment.Status.ObservedGeneration = deployment.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	// Determine if we need to continue polling
	switch deployment.Status.State {
	case networkingv1alpha2.PagesDeploymentStateSucceeded:
		logger.Info("Deployment succeeded", "url", result.URL)
		r.Recorder.Event(deployment, corev1.EventTypeNormal, EventReasonDeploymentSucceeded,
			fmt.Sprintf("Deployment succeeded: %s", result.URL))
		return ctrl.Result{}, nil

	case networkingv1alpha2.PagesDeploymentStateFailed:
		logger.Info("Deployment failed")
		r.Recorder.Event(deployment, corev1.EventTypeWarning, EventReasonDeploymentFailed,
			"Deployment failed")
		return ctrl.Result{}, nil

	case networkingv1alpha2.PagesDeploymentStateCancelled:
		logger.Info("Deployment cancelled")
		return ctrl.Result{}, nil

	default:
		// Continue polling for in-progress deployments
		logger.Info("Deployment in progress, will poll again",
			"state", deployment.Status.State,
			"stage", result.Stage,
			"interval", PollingInterval)
		r.Recorder.Event(deployment, corev1.EventTypeNormal, EventReasonDeploymentPolling,
			fmt.Sprintf("Deployment in progress: %s", result.Stage))
		return ctrl.Result{RequeueAfter: PollingInterval}, nil
	}
}

// forceRemoveFinalizer removes the finalizer when external resources are unavailable.
// This is called during deletion when the project reference cannot be resolved.
func (r *PagesDeploymentReconciler) forceRemoveFinalizer(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(deployment, FinalizerName) {
		return ctrl.Result{}, nil
	}

	logger.Info("Force removing finalizer due to missing project reference")
	r.Recorder.Event(deployment, corev1.EventTypeWarning, "ForcedCleanup",
		"Project reference not found, forcing finalizer removal. Deployment may remain in Cloudflare.")

	if err := controller.UpdateWithConflictRetry(ctx, r.Client, deployment, func() {
		controllerutil.RemoveFinalizer(deployment, FinalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(deployment, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer force removed")
	return ctrl.Result{}, nil
}

// handleDeletion handles the deletion of a PagesDeployment.
//
//nolint:revive // cognitive complexity acceptable for deletion logic
func (r *PagesDeploymentReconciler) handleDeletion(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
	projectName string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(deployment, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Production deletion protection warning
	if deployment.Spec.Environment == networkingv1alpha2.PagesDeploymentEnvironmentProduction &&
		deployment.Status.State == networkingv1alpha2.PagesDeploymentStateSucceeded {
		if err := r.validateProductionDeletion(ctx, deployment, projectName); err != nil {
			logger.Error(err, "Production deletion protection triggered")
			r.Recorder.Event(deployment, corev1.EventTypeWarning, EventReasonProductionProtected, err.Error())
			// Don't block deletion, just warn
		}
	}

	// Try to delete deployment from Cloudflare
	// Note: Active production deployments cannot be deleted via API
	if deployment.Status.DeploymentID != "" {
		apiResult, err := r.getAPIClient(ctx, deployment)
		if err != nil {
			logger.Error(err, "Failed to get API client for deletion")
			// Continue with finalizer removal
		} else {
			// First try without force
			err := apiResult.API.DeletePagesDeployment(ctx, projectName, deployment.Status.DeploymentID, false)
			if err != nil {
				// If aliased deployment error, retry with force=true
				if cf.IsAliasedDeploymentError(err) {
					logger.Info("Deployment has aliases, retrying with force=true",
						"deploymentID", deployment.Status.DeploymentID)
					err = apiResult.API.DeletePagesDeployment(ctx, projectName, deployment.Status.DeploymentID, true)
				}
			}
			if err != nil {
				if cf.IsActiveProductionDeploymentError(err) {
					logger.Info("Cannot delete active production deployment, this is expected")
					r.Recorder.Event(deployment, corev1.EventTypeNormal, "ProductionPreserved",
						"Active production deployment preserved in Cloudflare")
				} else if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to delete deployment from Cloudflare, continuing with finalizer removal")
					r.Recorder.Event(deployment, corev1.EventTypeWarning, controller.EventReasonDeleteFailed,
						fmt.Sprintf("Failed to delete from Cloudflare (will remove finalizer anyway): %s", cf.SanitizeErrorMessage(err)))
					// Don't block finalizer removal - resource may need manual cleanup in Cloudflare
				}
			}
		}
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, deployment, func() {
		controllerutil.RemoveFinalizer(deployment, FinalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(deployment, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")
	return ctrl.Result{}, nil
}

// setErrorStatus updates the deployment status with an error.
func (r *PagesDeploymentReconciler) setErrorStatus(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, deployment, func() {
		deployment.Status.State = networkingv1alpha2.PagesDeploymentStateFailed
		meta.SetStatusCondition(&deployment.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: deployment.Generation,
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		deployment.Status.ObservedGeneration = deployment.Generation
	})

	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	r.Recorder.Event(deployment, corev1.EventTypeWarning, controller.EventReasonReconcileFailed,
		cf.SanitizeErrorMessage(err))

	// Determine requeue based on error type
	if cf.IsPermanentError(err) {
		return ctrl.Result{}, nil // Don't retry permanent errors
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// getSourceType returns a string describing the deployment source type.
func (r *PagesDeploymentReconciler) getSourceType(deployment *networkingv1alpha2.PagesDeployment) string {
	if deployment.Spec.Source != nil {
		return string(deployment.Spec.Source.Type)
	}
	if deployment.Spec.Action == networkingv1alpha2.PagesDeploymentActionRollback {
		return "rollback"
	}
	if deployment.Spec.DirectUpload != nil {
		return "directUpload"
	}
	return "git"
}

// loadDirectUploadFiles loads files for direct upload from the specified source.
// Supports: HTTP URL, S3, and OCI sources with archive extraction (tar.gz, tar, zip).
func (r *PagesDeploymentReconciler) loadDirectUploadFiles(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
) (map[string][]byte, error) {
	logger := log.FromContext(ctx)

	if deployment.Spec.Source == nil || deployment.Spec.Source.DirectUpload == nil {
		return nil, errors.New("direct upload source not specified")
	}

	du := deployment.Spec.Source.DirectUpload
	if du.Source == nil {
		return nil, errors.New("direct upload source configuration is required")
	}

	// Log source type
	switch {
	case du.Source.HTTP != nil:
		logger.Info("Direct upload from HTTP source", "url", du.Source.HTTP.URL)
	case du.Source.S3 != nil:
		logger.Info("Direct upload from S3 source",
			"bucket", du.Source.S3.Bucket,
			"key", du.Source.S3.Key,
			"region", du.Source.S3.Region)
	case du.Source.OCI != nil:
		logger.Info("Direct upload from OCI source", "image", du.Source.OCI.Image)
	}

	// Use the uploader package to download, verify, and extract files
	manifest, err := uploader.ProcessSource(
		ctx,
		r.Client,
		deployment.Namespace,
		du.Source,
		du.Checksum,
		du.Archive,
	)
	if err != nil {
		return nil, fmt.Errorf("process direct upload source: %w", err)
	}

	logger.Info("Direct upload files loaded",
		"fileCount", manifest.FileCount,
		"totalSize", manifest.TotalSize,
		"sourceHash", manifest.SourceHash)

	return manifest.Files, nil
}

// resolveProjectName resolves the Cloudflare project name from the ProjectRef.
//
//nolint:revive // cognitive complexity acceptable for resolution logic
func (r *PagesDeploymentReconciler) resolveProjectName(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
) (string, error) {
	// Priority 1: CloudflareID/CloudflareName directly specified
	if deployment.Spec.ProjectRef.CloudflareID != "" {
		return deployment.Spec.ProjectRef.CloudflareID, nil
	}
	if deployment.Spec.ProjectRef.CloudflareName != "" {
		return deployment.Spec.ProjectRef.CloudflareName, nil
	}

	// Priority 2: Reference to PagesProject K8s resource
	if deployment.Spec.ProjectRef.Name != "" {
		project := &networkingv1alpha2.PagesProject{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: deployment.Namespace,
			Name:      deployment.Spec.ProjectRef.Name,
		}, project); err != nil {
			return "", fmt.Errorf("failed to get referenced PagesProject %s: %w",
				deployment.Spec.ProjectRef.Name, err)
		}

		if project.Spec.Name != "" {
			return project.Spec.Name, nil
		}
		return project.Name, nil
	}

	return "", errors.New("project reference is required: specify name, cloudflareId, or cloudflareName")
}

// resolveProjectNameFromRef resolves the project name from a PagesDeployment's ProjectRef.
func (r *PagesDeploymentReconciler) resolveProjectNameFromRef(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
) (string, error) {
	return r.resolveProjectName(ctx, deployment)
}

// validateProductionUniqueness ensures only one production deployment exists per project.
//
//nolint:revive // cognitive complexity acceptable for validation logic
func (r *PagesDeploymentReconciler) validateProductionUniqueness(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
	projectName string,
) error {
	deploymentList := &networkingv1alpha2.PagesDeploymentList{}
	if err := r.List(ctx, deploymentList, client.InNamespace(deployment.Namespace)); err != nil {
		return fmt.Errorf("failed to list PagesDeployments: %w", err)
	}

	for _, other := range deploymentList.Items {
		if other.Name == deployment.Name && other.Namespace == deployment.Namespace {
			continue
		}
		if other.Spec.Environment != networkingv1alpha2.PagesDeploymentEnvironmentProduction {
			continue
		}

		otherProjectName, err := r.resolveProjectNameFromRef(ctx, &other)
		if err != nil {
			continue
		}

		if otherProjectName == projectName {
			// Allow managed deployments to coexist temporarily
			const ManagedByLabel = "networking.cloudflare-operator.io/managed-by"
			if other.Labels[ManagedByLabel] == "pagesproject" {
				continue
			}

			// Allow if other is in terminal failed state
			if other.Status.State == networkingv1alpha2.PagesDeploymentStateFailed ||
				other.Status.State == networkingv1alpha2.PagesDeploymentStateCancelled {
				continue
			}

			return fmt.Errorf("production deployment '%s' already exists for project '%s'", other.Name, projectName)
		}
	}

	return nil
}

// validateProductionDeletion checks if deleting a production deployment is safe.
//
//nolint:revive // cognitive complexity acceptable for validation logic
func (r *PagesDeploymentReconciler) validateProductionDeletion(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
	projectName string,
) error {
	deploymentList := &networkingv1alpha2.PagesDeploymentList{}
	if err := r.List(ctx, deploymentList, client.InNamespace(deployment.Namespace)); err != nil {
		return fmt.Errorf("failed to list PagesDeployments: %w", err)
	}

	for _, other := range deploymentList.Items {
		if other.Name == deployment.Name && other.Namespace == deployment.Namespace {
			continue
		}
		if !other.DeletionTimestamp.IsZero() {
			continue
		}

		otherProjectName, err := r.resolveProjectNameFromRef(ctx, &other)
		if err != nil {
			continue
		}

		if otherProjectName == projectName {
			return nil // Another deployment exists
		}
	}

	return fmt.Errorf("deleting the only PagesDeployment for project '%s'", projectName)
}

// applyBackwardCompatibilityAdapter converts deprecated fields to new format.
//
//nolint:revive,gocritic // acceptable for backward compatibility logic
func (r *PagesDeploymentReconciler) applyBackwardCompatibilityAdapter(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
) {
	logger := log.FromContext(ctx)
	usingDeprecated := false

	if deployment.Spec.Source == nil {
		if deployment.Spec.DirectUpload != nil && deployment.Spec.DirectUpload.Source != nil {
			usingDeprecated = true
			deployment.Spec.Source = &networkingv1alpha2.PagesDeploymentSourceSpec{
				Type: networkingv1alpha2.PagesDeploymentSourceTypeDirectUpload,
				DirectUpload: &networkingv1alpha2.PagesDirectUploadSourceSpec{
					Source:   deployment.Spec.DirectUpload.Source,
					Checksum: deployment.Spec.DirectUpload.Checksum,
					Archive:  deployment.Spec.DirectUpload.Archive,
				},
			}
			logger.V(1).Info("Converted deprecated DirectUpload field to Source.DirectUpload")
		} else if deployment.Spec.Branch != "" {
			usingDeprecated = true
			deployment.Spec.Source = &networkingv1alpha2.PagesDeploymentSourceSpec{
				Type: networkingv1alpha2.PagesDeploymentSourceTypeGit,
				Git: &networkingv1alpha2.PagesGitSourceSpec{
					Branch: deployment.Spec.Branch,
				},
			}
			logger.V(1).Info("Converted deprecated Branch field to Source.Git")
		} else if deployment.Spec.Action == networkingv1alpha2.PagesDeploymentActionCreate ||
			deployment.Spec.Action == "" {
			deployment.Spec.Source = &networkingv1alpha2.PagesDeploymentSourceSpec{
				Type: networkingv1alpha2.PagesDeploymentSourceTypeGit,
				Git:  &networkingv1alpha2.PagesGitSourceSpec{},
			}
		}
	}

	if deployment.Spec.Environment == "" {
		if deployment.Spec.Source != nil && deployment.Spec.Source.Type == networkingv1alpha2.PagesDeploymentSourceTypeGit {
			if deployment.Spec.Source.Git != nil && deployment.Spec.Source.Git.Branch != "" {
				deployment.Spec.Environment = networkingv1alpha2.PagesDeploymentEnvironmentPreview
			} else {
				deployment.Spec.Environment = networkingv1alpha2.PagesDeploymentEnvironmentProduction
			}
		} else if deployment.Spec.Source != nil && deployment.Spec.Source.Type == networkingv1alpha2.PagesDeploymentSourceTypeDirectUpload {
			deployment.Spec.Environment = networkingv1alpha2.PagesDeploymentEnvironmentPreview
		}
	}

	if usingDeprecated {
		r.Recorder.Event(deployment, corev1.EventTypeWarning, EventReasonDeprecationWarning,
			"Using deprecated fields. Please migrate to Environment and Source fields.")
	}
}

// findDeploymentsForProject returns PagesDeployments that may need reconciliation when a PagesProject changes.
func (r *PagesDeploymentReconciler) findDeploymentsForProject(ctx context.Context, obj client.Object) []reconcile.Request {
	project, ok := obj.(*networkingv1alpha2.PagesProject)
	if !ok {
		return nil
	}

	deploymentList := &networkingv1alpha2.PagesDeploymentList{}
	if err := r.List(ctx, deploymentList, client.InNamespace(project.Namespace)); err != nil {
		return nil
	}

	projectName := project.Name
	if project.Spec.Name != "" {
		projectName = project.Spec.Name
	}

	var requests []reconcile.Request
	for _, deployment := range deploymentList.Items {
		if deployment.Spec.ProjectRef.Name == project.Name ||
			deployment.Spec.ProjectRef.CloudflareID == projectName ||
			deployment.Spec.ProjectRef.CloudflareName == projectName {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&deployment),
			})
		}
	}

	return requests
}

// findDeploymentsForSameProject returns PagesDeployments for production uniqueness validation.
//
//nolint:revive // cognitive complexity acceptable for watch handler
func (r *PagesDeploymentReconciler) findDeploymentsForSameProject(ctx context.Context, obj client.Object) []reconcile.Request {
	changed, ok := obj.(*networkingv1alpha2.PagesDeployment)
	if !ok {
		return nil
	}

	if changed.Spec.Environment != networkingv1alpha2.PagesDeploymentEnvironmentProduction {
		return nil
	}

	changedProjectName, err := r.resolveProjectNameFromRef(ctx, changed)
	if err != nil {
		return nil
	}

	deploymentList := &networkingv1alpha2.PagesDeploymentList{}
	if err := r.List(ctx, deploymentList, client.InNamespace(changed.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, deployment := range deploymentList.Items {
		if deployment.Name == changed.Name && deployment.Namespace == changed.Namespace {
			continue
		}
		if deployment.Spec.Environment != networkingv1alpha2.PagesDeploymentEnvironmentProduction {
			continue
		}

		projectName, err := r.resolveProjectNameFromRef(ctx, &deployment)
		if err != nil {
			continue
		}

		if projectName == changedProjectName {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&deployment),
			})
		}
	}

	return requests
}

func (r *PagesDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("pagesdeployment-controller")
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), mgr.GetLogger())

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.PagesDeployment{}).
		Watches(&networkingv1alpha2.PagesProject{},
			handler.EnqueueRequestsFromMapFunc(r.findDeploymentsForProject)).
		Watches(&networkingv1alpha2.PagesDeployment{},
			handler.EnqueueRequestsFromMapFunc(r.findDeploymentsForSameProject)).
		Complete(r)
}

// extractHashURL extracts the hash-based URL from aliases.
// The hash URL format is: <shortId>.<projectName>.pages.dev
//
//nolint:revive // cognitive complexity acceptable for URL extraction logic
func extractHashURL(aliases []string, shortID, projectName string) string {
	// First, try to find exact match with shortID in aliases
	pagesDevSuffix := ".pages.dev"
	for _, alias := range aliases {
		if strings.HasSuffix(alias, pagesDevSuffix) && strings.Contains(alias, shortID) {
			return alias
		}
	}

	// If shortID is available, construct the hash URL
	if shortID != "" && projectName != "" {
		return fmt.Sprintf("%s.%s.pages.dev", shortID, projectName)
	}

	// Fallback: find any pages.dev URL that's not the main project URL
	mainURL := projectName + pagesDevSuffix
	for _, alias := range aliases {
		if strings.HasSuffix(alias, pagesDevSuffix) && alias != mainURL {
			return alias
		}
	}

	return ""
}

// extractVersionName extracts the version name from deployment labels or name.
// Priority:
// 1. Label "networking.cloudflare-operator.io/version" (set by PagesProject version manager)
// 2. Deployment name (if it follows version naming pattern)
func extractVersionName(deployment *networkingv1alpha2.PagesDeployment) string {
	// Check for version label (set by PagesProject version manager)
	const versionLabel = "networking.cloudflare-operator.io/version"
	if version, ok := deployment.Labels[versionLabel]; ok && version != "" {
		return version
	}

	// Fallback: use deployment name as version identifier
	// This allows CI pipelines to use meaningful deployment names like "sha-abc123"
	return deployment.Name
}

// buildDeploymentMetadata constructs deployment metadata from the deployment spec.
// This handles both direct upload and git source configurations.
//
//nolint:revive // cognitive complexity acceptable for metadata building
func (r *PagesDeploymentReconciler) buildDeploymentMetadata(
	deployment *networkingv1alpha2.PagesDeployment,
) *cf.PagesDeploymentMetadata {
	metadata := &cf.PagesDeploymentMetadata{}

	if deployment.Spec.Source == nil {
		return metadata
	}

	switch deployment.Spec.Source.Type {
	case networkingv1alpha2.PagesDeploymentSourceTypeGit:
		if git := deployment.Spec.Source.Git; git != nil {
			metadata.Branch = git.Branch
			metadata.CommitHash = git.CommitSha
			metadata.CommitMessage = git.CommitMessage
		}

	case networkingv1alpha2.PagesDeploymentSourceTypeDirectUpload:
		if du := deployment.Spec.Source.DirectUpload; du != nil {
			// Determine branch based on environment and user config:
			// - production: empty string (uses project's production branch, updates main URL)
			// - preview: use specified branch or default to "preview"
			if deployment.Spec.Environment == networkingv1alpha2.PagesDeploymentEnvironmentPreview {
				if du.Branch != "" {
					metadata.Branch = du.Branch
				} else {
					metadata.Branch = "preview"
				}
			}

			// Override with DeploymentMetadata if specified
			if dm := du.DeploymentMetadata; dm != nil {
				if dm.Branch != "" {
					metadata.Branch = dm.Branch
				}
				if dm.CommitHash != "" {
					metadata.CommitHash = dm.CommitHash
				}
				if dm.CommitMessage != "" {
					metadata.CommitMessage = dm.CommitMessage
				}
				metadata.CommitDirty = dm.CommitDirty
			}
		}
	}

	return metadata
}
