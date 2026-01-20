// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package pagesdeployment implements the L2 Controller for PagesDeployment CRD.
// It registers deployment configurations to the Core Service for sync.
package pagesdeployment

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/StringKe/cloudflare-operator/internal/service"
	pagessvc "github.com/StringKe/cloudflare-operator/internal/service/pages"
)

const (
	FinalizerName = "pagesdeployment.networking.cloudflare-operator.io/finalizer"

	// AnnotationForceRedeploy is the annotation key to force a new deployment.
	// When this annotation value changes, a new deployment will be triggered
	// even if the spec hasn't changed. This is useful for re-deploying the
	// same configuration.
	AnnotationForceRedeploy = "cloudflare-operator.io/force-redeploy"

	// EventReasonProductionConflict indicates another production deployment exists
	EventReasonProductionConflict = "ProductionConflict"
	// EventReasonProductionProtected indicates production deletion is blocked
	EventReasonProductionProtected = "ProductionProtected"
	// EventReasonDeprecationWarning indicates deprecated fields are being used
	EventReasonDeprecationWarning = "DeprecationWarning"
)

// PagesDeploymentReconciler reconciles a PagesDeployment object
type PagesDeploymentReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	Recorder          record.EventRecorder
	deploymentService *pagessvc.DeploymentService
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

	// Apply backward compatibility adapter - converts deprecated fields to new format
	r.applyBackwardCompatibilityAdapter(ctx, deployment)

	// Resolve credentials
	credInfo, err := r.resolveCredentials(ctx, deployment)
	if err != nil {
		logger.Error(err, "Failed to resolve credentials")
		return r.updateStatusError(ctx, deployment, err)
	}

	// Resolve project name
	projectName, err := r.resolveProjectName(ctx, deployment)
	if err != nil {
		logger.Error(err, "Failed to resolve project name")
		return r.updateStatusError(ctx, deployment, err)
	}

	// Handle deletion
	if !deployment.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, deployment, projectName)
	}

	// Validate production uniqueness - only one production deployment per project
	if deployment.Spec.Environment == networkingv1alpha2.PagesDeploymentEnvironmentProduction {
		if err := r.validateProductionUniqueness(ctx, deployment, projectName); err != nil {
			logger.Error(err, "Production uniqueness validation failed")
			r.Recorder.Event(deployment, corev1.EventTypeWarning, EventReasonProductionConflict, err.Error())
			return r.updateStatusError(ctx, deployment, err)
		}
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(deployment, FinalizerName) {
		controllerutil.AddFinalizer(deployment, FinalizerName)
		if err := r.Update(ctx, deployment); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Register Pages deployment configuration to SyncState
	return r.registerPagesDeployment(ctx, deployment, projectName, credInfo)
}

// resolveCredentials resolves the credentials for the Pages deployment.
func (r *PagesDeploymentReconciler) resolveCredentials(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
) (*controller.CredentialsInfo, error) {
	return controller.ResolveCredentialsForService(
		ctx,
		r.Client,
		log.FromContext(ctx),
		deployment.Spec.Cloudflare,
		deployment.Namespace,
		deployment.Status.AccountID,
	)
}

// applyBackwardCompatibilityAdapter converts deprecated fields to new format.
// This allows existing manifests using the old Action/Branch/DirectUpload fields
// to work with the new Environment/Source model.
//
//nolint:revive,gocritic // cyclomatic complexity and ifElseChain acceptable for backward compatibility logic
func (r *PagesDeploymentReconciler) applyBackwardCompatibilityAdapter(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
) {
	logger := log.FromContext(ctx)

	// Track if we're using deprecated fields
	usingDeprecated := false

	// If new Source field is not specified, convert from deprecated fields
	if deployment.Spec.Source == nil {
		// Check for deprecated DirectUpload field
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
			// Check for deprecated Branch field
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
			// Default to git source with empty branch (uses project's production branch)
			deployment.Spec.Source = &networkingv1alpha2.PagesDeploymentSourceSpec{
				Type: networkingv1alpha2.PagesDeploymentSourceTypeGit,
				Git:  &networkingv1alpha2.PagesGitSourceSpec{},
			}
		}
	}

	// If Environment is not specified, infer from context
	if deployment.Spec.Environment == "" {
		// Default to production for create action without explicit branch
		// Default to preview for create action with explicit branch
		if deployment.Spec.Source != nil && deployment.Spec.Source.Type == networkingv1alpha2.PagesDeploymentSourceTypeGit {
			if deployment.Spec.Source.Git != nil && deployment.Spec.Source.Git.Branch != "" {
				deployment.Spec.Environment = networkingv1alpha2.PagesDeploymentEnvironmentPreview
			} else {
				deployment.Spec.Environment = networkingv1alpha2.PagesDeploymentEnvironmentProduction
			}
		} else if deployment.Spec.Source != nil && deployment.Spec.Source.Type == networkingv1alpha2.PagesDeploymentSourceTypeDirectUpload {
			// Direct upload defaults to preview
			deployment.Spec.Environment = networkingv1alpha2.PagesDeploymentEnvironmentPreview
		}
	}

	// Emit deprecation warning event
	if usingDeprecated {
		r.Recorder.Event(deployment, corev1.EventTypeWarning, EventReasonDeprecationWarning,
			"Using deprecated fields (Branch, DirectUpload, Action). Please migrate to Environment and Source fields. These fields will be removed in v1alpha3.")
	}
}

// validateProductionUniqueness ensures only one production deployment exists per project.
// Allows temporary coexistence of managed deployments (by PagesProject), as the PagesProject
// controller will reconcile them to ensure eventual consistency.
//
//nolint:revive // cognitive complexity is acceptable for validation logic
func (r *PagesDeploymentReconciler) validateProductionUniqueness(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
	projectName string,
) error {
	logger := log.FromContext(ctx)

	// List all PagesDeployments in the same namespace
	deploymentList := &networkingv1alpha2.PagesDeploymentList{}
	if err := r.List(ctx, deploymentList, client.InNamespace(deployment.Namespace)); err != nil {
		return fmt.Errorf("failed to list PagesDeployments: %w", err)
	}

	for _, other := range deploymentList.Items {
		// Skip self
		if other.Name == deployment.Name && other.Namespace == deployment.Namespace {
			continue
		}

		// Skip if not production
		if other.Spec.Environment != networkingv1alpha2.PagesDeploymentEnvironmentProduction {
			continue
		}

		// Check if same project
		otherProjectName, err := r.resolveProjectNameFromRef(ctx, &other)
		if err != nil {
			continue // Skip if we can't resolve
		}

		if otherProjectName == projectName {
			// Check if the other deployment is managed by PagesProject
			// Managed deployments are allowed to temporarily coexist during reconciliation
			const ManagedByLabel = "networking.cloudflare-operator.io/managed-by"
			const ManagedByValue = "pagesproject"
			if other.Labels[ManagedByLabel] == ManagedByValue {
				logger.Info("Found managed production deployment, allowing temporary coexistence",
					"existing", other.Name, "managed", true)
				// PagesProject controller will handle reconciliation
				continue
			}

			// Check if the other deployment is in a terminal state (failed/succeeded)
			// Only block if the other production deployment is active
			if other.Status.State != networkingv1alpha2.PagesDeploymentStateFailed &&
				other.Status.State != networkingv1alpha2.PagesDeploymentStateCancelled {
				return fmt.Errorf("production deployment '%s' already exists for project '%s' (user-created). "+
					"Only one production deployment is allowed per project. "+
					"Delete the existing production deployment or use environment: preview",
					other.Name, projectName)
			}
		}
	}

	return nil
}

// resolveProjectNameFromRef resolves the project name from a PagesDeployment's ProjectRef.
// This is a helper for validateProductionUniqueness.
func (r *PagesDeploymentReconciler) resolveProjectNameFromRef(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
) (string, error) {
	if deployment.Spec.ProjectRef.CloudflareID != "" {
		return deployment.Spec.ProjectRef.CloudflareID, nil
	}
	if deployment.Spec.ProjectRef.CloudflareName != "" {
		return deployment.Spec.ProjectRef.CloudflareName, nil
	}
	if deployment.Spec.ProjectRef.Name != "" {
		project := &networkingv1alpha2.PagesProject{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: deployment.Namespace,
			Name:      deployment.Spec.ProjectRef.Name,
		}, project); err != nil {
			return "", err
		}
		if project.Spec.Name != "" {
			return project.Spec.Name, nil
		}
		return project.Name, nil
	}
	return "", errors.New("no project reference found")
}

// resolveProjectName resolves the Cloudflare project name from the ProjectRef.
//
//nolint:revive // cognitive complexity is acceptable for resolution logic
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

		// Get project name from the PagesProject spec
		if project.Spec.Name != "" {
			return project.Spec.Name, nil
		}
		return project.Name, nil
	}

	return "", fmt.Errorf("project reference is required: specify name, cloudflareId, or cloudflareName")
}

// handleDeletion handles the deletion of a PagesDeployment.
//
//nolint:revive // cyclomatic complexity is acceptable for deletion logic with protection
func (r *PagesDeploymentReconciler) handleDeletion(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
	projectName string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(deployment, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Production deletion protection: if this is a production deployment that succeeded,
	// check if there's another deployment that can take over
	if deployment.Spec.Environment == networkingv1alpha2.PagesDeploymentEnvironmentProduction &&
		deployment.Status.State == networkingv1alpha2.PagesDeploymentStateSucceeded {
		if err := r.validateProductionDeletion(ctx, deployment, projectName); err != nil {
			logger.Error(err, "Production deletion protection triggered")
			r.Recorder.Event(deployment, corev1.EventTypeWarning, EventReasonProductionProtected, err.Error())
			// Don't return error - allow deletion to proceed but log the warning
			// The Cloudflare deployment will be preserved anyway
		}
	}

	// Unregister from SyncState - this triggers Sync Controller to delete from Cloudflare
	source := service.Source{
		Kind:      "PagesDeployment",
		Namespace: deployment.Namespace,
		Name:      deployment.Name,
	}

	logger.Info("Unregistering Pages deployment from SyncState",
		"projectName", projectName,
		"deploymentId", deployment.Status.DeploymentID,
		"environment", deployment.Spec.Environment,
		"source", fmt.Sprintf("%s/%s", deployment.Namespace, deployment.Name))

	if err := r.deploymentService.Unregister(ctx, deployment.Status.DeploymentID, source); err != nil {
		logger.Error(err, "Failed to unregister Pages deployment from SyncState")
		r.Recorder.Event(deployment, corev1.EventTypeWarning, "UnregisterFailed",
			fmt.Sprintf("Failed to unregister from SyncState: %s", err.Error()))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	r.Recorder.Event(deployment, corev1.EventTypeNormal, "Unregistered",
		"Unregistered from SyncState, Sync Controller will delete from Cloudflare")

	// Remove finalizer with retry logic
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, deployment, func() {
		controllerutil.RemoveFinalizer(deployment, FinalizerName)
	}); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(deployment, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// validateProductionDeletion checks if deleting a production deployment is safe.
// This warns if deleting the only successful production deployment.
//
//nolint:revive // cognitive complexity is acceptable for validation logic
func (r *PagesDeploymentReconciler) validateProductionDeletion(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
	projectName string,
) error {
	// List all PagesDeployments in the same namespace
	deploymentList := &networkingv1alpha2.PagesDeploymentList{}
	if err := r.List(ctx, deploymentList, client.InNamespace(deployment.Namespace)); err != nil {
		return fmt.Errorf("failed to list PagesDeployments: %w", err)
	}

	// Check if there's another successful deployment for this project
	for _, other := range deploymentList.Items {
		// Skip self
		if other.Name == deployment.Name && other.Namespace == deployment.Namespace {
			continue
		}

		// Skip if being deleted
		if !other.DeletionTimestamp.IsZero() {
			continue
		}

		// Check if same project
		otherProjectName, err := r.resolveProjectNameFromRef(ctx, &other)
		if err != nil {
			continue
		}

		if otherProjectName == projectName {
			// Found another deployment for this project - deletion is safe
			// The Cloudflare deployment will be preserved anyway
			return nil
		}
	}

	// No other deployments found - warn but allow deletion
	return fmt.Errorf("deleting the only PagesDeployment for project '%s'. "+
		"Note: The Cloudflare deployment will be preserved and can be re-adopted", projectName)
}

// registerPagesDeployment registers the Pages deployment configuration to SyncState.
//
//nolint:revive // cyclomatic complexity is acceptable for registration logic
func (r *PagesDeploymentReconciler) registerPagesDeployment(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
	projectName string,
	credInfo *controller.CredentialsInfo,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Create source reference
	source := service.Source{
		Kind:      "PagesDeployment",
		Namespace: deployment.Namespace,
		Name:      deployment.Name,
	}

	// Get force-redeploy annotation value
	// When this value changes, it forces a new deployment even if spec is unchanged
	forceRedeploy := ""
	if deployment.Annotations != nil {
		forceRedeploy = deployment.Annotations[AnnotationForceRedeploy]
	}

	// Build Pages deployment configuration using new or legacy fields
	config := r.buildDeploymentConfig(deployment, projectName, forceRedeploy)

	// Register to SyncState
	opts := pagessvc.DeploymentRegisterOptions{
		DeploymentID:   deployment.Status.DeploymentID,
		ProjectName:    projectName,
		AccountID:      credInfo.AccountID,
		Source:         source,
		Config:         config,
		CredentialsRef: credInfo.CredentialsRef,
	}

	if err := r.deploymentService.Register(ctx, opts); err != nil {
		logger.Error(err, "Failed to register Pages deployment configuration")
		r.Recorder.Event(deployment, corev1.EventTypeWarning, "RegisterFailed",
			fmt.Sprintf("Failed to register Pages deployment: %s", err.Error()))
		return r.updateStatusError(ctx, deployment, err)
	}

	// Build description for event
	sourceDesc := r.buildSourceDescription(deployment)
	r.Recorder.Event(deployment, corev1.EventTypeNormal, "Registered",
		fmt.Sprintf("Registered Pages Deployment (%s, %s) configuration to SyncState",
			deployment.Spec.Environment, sourceDesc))

	// Check if already synced
	syncStatus, err := r.deploymentService.GetSyncStatus(ctx, source, deployment.Status.DeploymentID)
	if err != nil {
		logger.Error(err, "Failed to get sync status")
		return r.updateStatusPending(ctx, deployment, projectName, credInfo.AccountID)
	}

	if syncStatus != nil && syncStatus.IsSynced {
		// Already synced, update status based on action result
		return r.updateStatusSynced(ctx, deployment, projectName, credInfo.AccountID, syncStatus)
	}

	// Update status to Pending - actual sync happens via PagesSyncController
	return r.updateStatusPending(ctx, deployment, projectName, credInfo.AccountID)
}

// buildDeploymentConfig builds the deployment configuration from spec fields.
// It handles both new (Environment/Source) and legacy (Action/Branch/DirectUpload) fields.
func (*PagesDeploymentReconciler) buildDeploymentConfig(
	deployment *networkingv1alpha2.PagesDeployment,
	projectName string,
	forceRedeploy string,
) pagessvc.PagesDeploymentConfig {
	config := pagessvc.PagesDeploymentConfig{
		ProjectName:     projectName,
		Environment:     string(deployment.Spec.Environment),
		PurgeBuildCache: deployment.Spec.PurgeBuildCache,
		ForceRedeploy:   forceRedeploy,
	}

	// Use new Source field if available
	if deployment.Spec.Source != nil {
		config.Source = convertSourceSpec(deployment.Spec.Source)
	}

	// Include legacy fields for backward compatibility during transition
	// These are used by the Sync Controller if Source is not set
	config.Action = string(deployment.Spec.Action)
	config.TargetDeploymentID = deployment.Spec.TargetDeploymentID
	config.Rollback = convertRollback(deployment.Spec.Rollback)

	// Convert legacy DirectUpload if Source.DirectUpload is not set
	if config.Source == nil || config.Source.DirectUpload == nil {
		if deployment.Spec.DirectUpload != nil {
			config.LegacyDirectUpload = convertDirectUpload(deployment.Spec.DirectUpload)
		}
	}

	return config
}

// buildSourceDescription creates a human-readable description of the deployment source.
//
//nolint:revive // cognitive complexity is acceptable for source description building
func (*PagesDeploymentReconciler) buildSourceDescription(deployment *networkingv1alpha2.PagesDeployment) string {
	if deployment.Spec.Source != nil {
		switch deployment.Spec.Source.Type {
		case networkingv1alpha2.PagesDeploymentSourceTypeGit:
			if deployment.Spec.Source.Git != nil && deployment.Spec.Source.Git.Branch != "" {
				if deployment.Spec.Source.Git.CommitSha != "" {
					return fmt.Sprintf("git:%s@%s", deployment.Spec.Source.Git.Branch,
						deployment.Spec.Source.Git.CommitSha[:7])
				}
				return fmt.Sprintf("git:%s", deployment.Spec.Source.Git.Branch)
			}
			return "git:default"
		case networkingv1alpha2.PagesDeploymentSourceTypeDirectUpload:
			return "directUpload"
		}
	}

	// Legacy format
	if deployment.Spec.DirectUpload != nil {
		return "directUpload"
	}
	if deployment.Spec.Branch != "" {
		return fmt.Sprintf("git:%s", deployment.Spec.Branch)
	}
	return string(deployment.Spec.Action)
}

// convertSourceSpec converts CRD Source spec to service type.
func convertSourceSpec(src *networkingv1alpha2.PagesDeploymentSourceSpec) *pagessvc.DeploymentSourceConfig {
	if src == nil {
		return nil
	}

	config := &pagessvc.DeploymentSourceConfig{
		Type: string(src.Type),
	}

	if src.Git != nil {
		config.Git = &pagessvc.GitSourceConfig{
			Branch:    src.Git.Branch,
			CommitSha: src.Git.CommitSha,
		}
	}

	if src.DirectUpload != nil {
		config.DirectUpload = &pagessvc.DirectUploadConfig{
			Source:   src.DirectUpload.Source,
			Checksum: src.DirectUpload.Checksum,
			Archive:  src.DirectUpload.Archive,
		}
	}

	return config
}

// findDeploymentsForProject returns PagesDeployments that may need reconciliation when a PagesProject changes.
func (r *PagesDeploymentReconciler) findDeploymentsForProject(ctx context.Context, obj client.Object) []reconcile.Request {
	project, ok := obj.(*networkingv1alpha2.PagesProject)
	if !ok {
		return nil
	}

	// List all PagesDeployments in the same namespace
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
		// Check if this deployment references the project
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

func (r *PagesDeploymentReconciler) updateStatusError(
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

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *PagesDeploymentReconciler) updateStatusPending(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
	projectName, accountID string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, deployment, func() {
		if deployment.Status.AccountID == "" {
			deployment.Status.AccountID = accountID
		}
		deployment.Status.ProjectName = projectName
		deployment.Status.State = networkingv1alpha2.PagesDeploymentStatePending
		meta.SetStatusCondition(&deployment.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: deployment.Generation,
			Reason:             "Pending",
			Message:            "Pages deployment configuration registered, waiting for sync",
			LastTransitionTime: metav1.Now(),
		})
		deployment.Status.ObservedGeneration = deployment.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	// Requeue to check for sync completion
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

//nolint:revive // cyclomatic complexity is acceptable for this status update function
func (r *PagesDeploymentReconciler) updateStatusSynced(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
	projectName, accountID string,
	syncStatus *pagessvc.DeploymentSyncStatus,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, deployment, func() {
		deployment.Status.AccountID = accountID
		deployment.Status.ProjectName = projectName
		deployment.Status.DeploymentID = syncStatus.DeploymentID

		// Update new status fields
		deployment.Status.URL = syncStatus.URL
		deployment.Status.HashURL = syncStatus.HashURL
		deployment.Status.BranchURL = syncStatus.BranchURL
		if syncStatus.Environment != "" {
			deployment.Status.Environment = syncStatus.Environment
		} else {
			deployment.Status.Environment = string(deployment.Spec.Environment)
		}
		deployment.Status.IsCurrentProduction = syncStatus.IsCurrentProduction
		deployment.Status.Version = syncStatus.Version
		deployment.Status.SourceDescription = syncStatus.SourceDescription

		// Determine state based on stage
		switch syncStatus.Stage {
		case "queued":
			deployment.Status.State = networkingv1alpha2.PagesDeploymentStateQueued
		case "initialize", "clone_repo", "build":
			deployment.Status.State = networkingv1alpha2.PagesDeploymentStateBuilding
		case "deploy":
			deployment.Status.State = networkingv1alpha2.PagesDeploymentStateDeploying
		case "success", "active":
			deployment.Status.State = networkingv1alpha2.PagesDeploymentStateSucceeded
		case "failure":
			deployment.Status.State = networkingv1alpha2.PagesDeploymentStateFailed
		default:
			deployment.Status.State = networkingv1alpha2.PagesDeploymentStatePending
		}

		conditionStatus := metav1.ConditionFalse
		reason := "InProgress"
		message := fmt.Sprintf("Deployment is %s", syncStatus.Stage)

		switch deployment.Status.State {
		case networkingv1alpha2.PagesDeploymentStateSucceeded:
			conditionStatus = metav1.ConditionTrue
			reason = "Succeeded"
			message = "Deployment succeeded"
		case networkingv1alpha2.PagesDeploymentStateFailed:
			reason = "Failed"
			message = "Deployment failed"
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

	// If deployment is still in progress, requeue to check status
	if deployment.Status.State != networkingv1alpha2.PagesDeploymentStateSucceeded &&
		deployment.Status.State != networkingv1alpha2.PagesDeploymentStateFailed {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func (r *PagesDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("pagesdeployment-controller")

	// Initialize DeploymentService
	r.deploymentService = pagessvc.NewDeploymentService(mgr.GetClient())

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.PagesDeployment{}).
		Watches(&networkingv1alpha2.PagesProject{},
			handler.EnqueueRequestsFromMapFunc(r.findDeploymentsForProject)).
		// Watch other PagesDeployments for production uniqueness validation
		Watches(&networkingv1alpha2.PagesDeployment{},
			handler.EnqueueRequestsFromMapFunc(r.findDeploymentsForSameProject)).
		Complete(r)
}

// findDeploymentsForSameProject returns PagesDeployments that may need re-validation
// when another PagesDeployment for the same project changes (for production uniqueness).
//
//nolint:revive // cyclomatic complexity is acceptable for watch handler logic
func (r *PagesDeploymentReconciler) findDeploymentsForSameProject(ctx context.Context, obj client.Object) []reconcile.Request {
	changed, ok := obj.(*networkingv1alpha2.PagesDeployment)
	if !ok {
		return nil
	}

	// Only trigger if the changed deployment is production
	if changed.Spec.Environment != networkingv1alpha2.PagesDeploymentEnvironmentProduction {
		return nil
	}

	// Resolve project name for the changed deployment
	changedProjectName, err := r.resolveProjectNameFromRef(ctx, changed)
	if err != nil {
		return nil
	}

	// List all PagesDeployments in the same namespace
	deploymentList := &networkingv1alpha2.PagesDeploymentList{}
	if err := r.List(ctx, deploymentList, client.InNamespace(changed.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, deployment := range deploymentList.Items {
		// Skip the deployment that triggered this watch
		if deployment.Name == changed.Name && deployment.Namespace == changed.Namespace {
			continue
		}

		// Skip if not production
		if deployment.Spec.Environment != networkingv1alpha2.PagesDeploymentEnvironmentProduction {
			continue
		}

		// Check if same project
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

// convertDirectUpload converts CRD DirectUpload spec to service type.
func convertDirectUpload(du *networkingv1alpha2.PagesDirectUpload) *pagessvc.DirectUploadConfig {
	if du == nil {
		return nil
	}
	return &pagessvc.DirectUploadConfig{
		Source:               du.Source,
		Checksum:             du.Checksum,
		Archive:              du.Archive,
		ManifestConfigMapRef: du.ManifestConfigMapRef,
		Manifest:             du.Manifest,
	}
}

// convertRollback converts CRD Rollback spec to service type.
func convertRollback(rb *networkingv1alpha2.RollbackConfig) *pagessvc.RollbackConfig {
	if rb == nil {
		return nil
	}
	return &pagessvc.RollbackConfig{
		Strategy:     rb.Strategy,
		Version:      rb.Version,
		DeploymentID: rb.DeploymentID,
	}
}
