// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package pagesdeployment implements the L2 Controller for PagesDeployment CRD.
// It registers deployment configurations to the Core Service for sync.
package pagesdeployment

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

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
func (r *PagesDeploymentReconciler) handleDeletion(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
	projectName string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(deployment, FinalizerName) {
		return ctrl.Result{}, nil
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

// registerPagesDeployment registers the Pages deployment configuration to SyncState.
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

	// Build Pages deployment configuration
	config := pagessvc.PagesDeploymentActionConfig{
		ProjectName:        projectName,
		Branch:             deployment.Spec.Branch,
		Action:             string(deployment.Spec.Action),
		TargetDeploymentID: deployment.Spec.TargetDeploymentID,
		PurgeBuildCache:    deployment.Spec.PurgeBuildCache,
		DirectUpload:       convertDirectUpload(deployment.Spec.DirectUpload),
		Rollback:           convertRollback(deployment.Spec.Rollback),
	}

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

	r.Recorder.Event(deployment, corev1.EventTypeNormal, "Registered",
		fmt.Sprintf("Registered Pages Deployment action '%s' configuration to SyncState", deployment.Spec.Action))

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

		// Determine state based on stage
		switch syncStatus.Stage {
		case "queued":
			deployment.Status.State = networkingv1alpha2.PagesDeploymentStateQueued
		case "initialize", "clone_repo", "build":
			deployment.Status.State = networkingv1alpha2.PagesDeploymentStateBuilding
		case "deploy":
			deployment.Status.State = networkingv1alpha2.PagesDeploymentStateDeploying
		case "success":
			deployment.Status.State = networkingv1alpha2.PagesDeploymentStateSucceeded
			deployment.Status.URL = syncStatus.URL
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
		Complete(r)
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
