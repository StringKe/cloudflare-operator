// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesproject

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

// DefaultExternalSyncInterval is the default interval for external version sync.
const DefaultExternalSyncInterval = 5 * time.Minute

// ExternalReconciler handles the external version management policy.
// It allows external systems to control versioning by updating the External config fields.
type ExternalReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Log      logr.Logger
}

// NewExternalReconciler creates a new ExternalReconciler.
func NewExternalReconciler(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
	log logr.Logger,
) *ExternalReconciler {
	return &ExternalReconciler{
		Client:   k8sClient,
		Scheme:   scheme,
		Recorder: recorder,
		Log:      log.WithName("external"),
	}
}

// Reconcile handles the external version management workflow.
// External systems update spec.versionManagement.external.currentVersion and productionVersion.
// This reconciler ensures the corresponding deployments exist and production is promoted.
//
//nolint:revive // cognitive complexity acceptable for reconciliation logic
func (r *ExternalReconciler) Reconcile(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	apiClient *cf.API,
) (requeueAfter time.Duration, err error) {
	config := project.Spec.VersionManagement.External
	if config == nil {
		return 0, nil
	}

	log := r.Log.WithValues("project", project.Name, "namespace", project.Namespace)

	// Calculate sync interval for requeue
	syncInterval := DefaultExternalSyncInterval
	if config.SyncInterval != nil && config.SyncInterval.Duration > 0 {
		syncInterval = config.SyncInterval.Duration
	}

	// 1. Handle currentVersion - ensure deployment exists
	if config.CurrentVersion != "" {
		if err := r.reconcileCurrentVersion(ctx, project, config); err != nil {
			log.Error(err, "Failed to reconcile current version", "version", config.CurrentVersion)
			return syncInterval, err
		}
	}

	// 2. Handle productionVersion - promote to production
	if config.ProductionVersion != "" {
		if err := r.reconcileProductionVersion(ctx, project, config, apiClient); err != nil {
			log.Error(err, "Failed to reconcile production version", "version", config.ProductionVersion)
			return syncInterval, err
		}
	}

	// 3. Update version mapping in status
	if err := r.updateVersionMapping(ctx, project); err != nil {
		log.Error(err, "Failed to update version mapping")
		// Non-fatal, continue
	}

	return syncInterval, nil
}

// reconcileCurrentVersion ensures a deployment exists for the current version.
func (r *ExternalReconciler) reconcileCurrentVersion(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	config *networkingv1alpha2.ExternalVersionConfig,
) error {
	versionName := config.CurrentVersion
	log := r.Log.WithValues("version", versionName, "type", "current")

	// Find existing deployment by version name
	deployment, err := r.findDeploymentByVersion(ctx, project, versionName)
	if err != nil {
		return err
	}

	if deployment != nil {
		log.V(1).Info("Deployment for current version already exists", "deployment", deployment.Name)
		return nil
	}

	// Create new deployment for this version
	log.Info("Creating deployment for current version")
	return r.createDeployment(ctx, project, versionName, config)
}

// reconcileProductionVersion validates and promotes the production version.
//
//nolint:revive // cognitive complexity acceptable for promotion logic
func (r *ExternalReconciler) reconcileProductionVersion(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	config *networkingv1alpha2.ExternalVersionConfig,
	_ *cf.API, // apiClient no longer needed for promotion (uses environment change)
) error {
	versionName := config.ProductionVersion
	log := r.Log.WithValues("version", versionName, "type", "production")

	// Find deployment by version name
	deployment, err := r.findDeploymentByVersion(ctx, project, versionName)
	if err != nil {
		return err
	}

	if deployment == nil {
		return fmt.Errorf("version %s not found, cannot promote to production", versionName)
	}

	// Validate deployment is ready for promotion (includes succeeded check)
	if err := ValidateDeploymentForPromotion(deployment); err != nil {
		log.Info("Deployment not ready for promotion", "reason", err.Error())
		return err
	}

	// Check if already production
	if project.Status.CurrentProduction != nil &&
		project.Status.CurrentProduction.DeploymentID == deployment.Status.DeploymentID {
		log.V(1).Info("Version is already production")
		return nil
	}

	// Promote by changing environment (works for all deployments, not just previous production)
	log.Info("Promoting deployment to production", "deployment", deployment.Name)

	if err := PromoteDeploymentToProduction(ctx, r.Client, deployment); err != nil {
		r.Recorder.Event(project, corev1.EventTypeWarning, "PromotionFailed",
			fmt.Sprintf("Failed to promote version %s: %s", versionName, err.Error()))
		return fmt.Errorf("failed to promote deployment: %w", err)
	}

	r.Recorder.Event(project, corev1.EventTypeNormal, "VersionPromoted",
		fmt.Sprintf("Version %s promoted to production (external)", versionName))

	// Record validation history
	if err := r.recordValidation(ctx, project, versionName, deployment.Status.DeploymentID); err != nil {
		log.Error(err, "Failed to record validation history")
		// Non-fatal, continue
	}

	return nil
}

// findDeploymentByVersion finds a PagesDeployment by version name.
//
//nolint:revive // cognitive complexity acceptable for search logic
func (r *ExternalReconciler) findDeploymentByVersion(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	versionName string,
) (*networkingv1alpha2.PagesDeployment, error) {
	deployments := &networkingv1alpha2.PagesDeploymentList{}
	if err := r.List(ctx, deployments, client.InNamespace(project.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	for i := range deployments.Items {
		d := &deployments.Items[i]

		// Check if deployment belongs to this project
		if !r.belongsToProject(d, project) {
			continue
		}

		// Check spec.versionName
		if d.Spec.VersionName == versionName {
			return d, nil
		}
		// Check status.versionName
		if d.Status.VersionName == versionName {
			return d, nil
		}
		// Check version label
		if d.Labels[VersionLabel] == versionName {
			return d, nil
		}
	}

	return nil, nil
}

// belongsToProject checks if a deployment belongs to the given project.
func (*ExternalReconciler) belongsToProject(
	deployment *networkingv1alpha2.PagesDeployment,
	project *networkingv1alpha2.PagesProject,
) bool {
	// Check by project reference
	if deployment.Spec.ProjectRef.Name == project.Name {
		return true
	}

	// Check by Cloudflare project name
	projectName := project.Spec.Name
	if projectName == "" {
		projectName = project.Name
	}

	if deployment.Spec.ProjectRef.CloudflareID == projectName ||
		deployment.Spec.ProjectRef.CloudflareName == projectName ||
		deployment.Status.ProjectName == projectName {
		return true
	}

	// Check by managed-by labels
	if deployment.Labels[ManagedByNameLabel] == project.Name {
		return true
	}

	return false
}

// createDeployment creates a new PagesDeployment for a version.
func (r *ExternalReconciler) createDeployment(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	versionName string,
	_ *networkingv1alpha2.ExternalVersionConfig,
) error {
	deploymentName := fmt.Sprintf("%s-%s", project.Name, versionName)

	deployment := &networkingv1alpha2.PagesDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: project.Namespace,
			Labels: map[string]string{
				ManagedByLabel:     ManagedByValue,
				ManagedByNameLabel: project.Name,
				ManagedByUIDLabel:  string(project.UID),
				VersionLabel:       versionName,
			},
			Annotations: map[string]string{
				ManagedAnnotation: "true",
			},
		},
		Spec: networkingv1alpha2.PagesDeploymentSpec{
			ProjectRef: networkingv1alpha2.PagesProjectRef{
				Name: project.Name,
			},
			VersionName: versionName,
			Environment: networkingv1alpha2.PagesDeploymentEnvironmentPreview,
			Cloudflare:  project.Spec.Cloudflare,
		},
	}

	// Set owner reference for cascade deletion
	if err := ctrl.SetControllerReference(project, deployment, r.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := r.Create(ctx, deployment); err != nil {
		if apierrors.IsAlreadyExists(err) {
			r.Log.V(1).Info("Deployment already exists", "deployment", deploymentName)
			return nil
		}
		return fmt.Errorf("failed to create deployment: %w", err)
	}

	r.Log.Info("Created deployment for external version", "deployment", deploymentName, "version", versionName)
	r.Recorder.Event(project, corev1.EventTypeNormal, "DeploymentCreated",
		fmt.Sprintf("Created deployment for external version %s", versionName))

	return nil
}

// recordValidation records a version validation in the project status.
func (r *ExternalReconciler) recordValidation(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	versionName, deploymentID string,
) error {
	// Get fresh copy
	fresh := &networkingv1alpha2.PagesProject{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(project), fresh); err != nil {
		return err
	}

	now := metav1.Now()
	validation := networkingv1alpha2.VersionValidation{
		VersionName:      versionName,
		DeploymentID:     deploymentID,
		ValidatedAt:      &now,
		ValidatedBy:      "external",
		ValidationResult: "passed",
	}

	// Add to validation history (keep last 50)
	fresh.Status.ValidationHistory = append(
		[]networkingv1alpha2.VersionValidation{validation},
		fresh.Status.ValidationHistory...,
	)
	if len(fresh.Status.ValidationHistory) > 50 {
		fresh.Status.ValidationHistory = fresh.Status.ValidationHistory[:50]
	}

	return r.Status().Update(ctx, fresh)
}

// updateVersionMapping updates the version mapping in project status.
func (r *ExternalReconciler) updateVersionMapping(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
) error {
	index := NewVersionIndex(r.Client)
	mapping, err := index.BuildIndex(ctx, project)
	if err != nil {
		return err
	}

	// Get fresh copy
	fresh := &networkingv1alpha2.PagesProject{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(project), fresh); err != nil {
		return err
	}

	fresh.Status.VersionMapping = mapping
	return r.Status().Update(ctx, fresh)
}

// GetSyncInterval returns the configured or default sync interval.
func (*ExternalReconciler) GetSyncInterval(config *networkingv1alpha2.ExternalVersionConfig) time.Duration {
	if config == nil || config.SyncInterval == nil || config.SyncInterval.Duration <= 0 {
		return DefaultExternalSyncInterval
	}
	return config.SyncInterval.Duration
}
