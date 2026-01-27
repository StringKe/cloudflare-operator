// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesproject

import (
	"context"
	"fmt"

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

// GitOpsReconciler handles the gitops version management policy.
// It manages preview and production deployments based on version names.
type GitOpsReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Log      logr.Logger
}

// NewGitOpsReconciler creates a new GitOpsReconciler.
func NewGitOpsReconciler(k8sClient client.Client, scheme *runtime.Scheme, recorder record.EventRecorder, log logr.Logger) *GitOpsReconciler {
	return &GitOpsReconciler{
		Client:   k8sClient,
		Scheme:   scheme,
		Recorder: recorder,
		Log:      log.WithName("gitops"),
	}
}

// Reconcile handles the GitOps version management workflow.
func (r *GitOpsReconciler) Reconcile(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	apiClient *cf.API,
) error {
	gitops := project.Spec.VersionManagement.GitOps
	if gitops == nil {
		return nil
	}

	log := r.Log.WithValues("project", project.Name, "namespace", project.Namespace)

	// 1. Handle preview version
	if gitops.PreviewVersion != "" {
		if err := r.reconcilePreviewVersion(ctx, project, gitops); err != nil {
			log.Error(err, "Failed to reconcile preview version", "version", gitops.PreviewVersion)
			return err
		}
	}

	// 2. Handle production version
	if gitops.ProductionVersion != "" {
		if err := r.reconcileProductionVersion(ctx, project, gitops, apiClient); err != nil {
			log.Error(err, "Failed to reconcile production version", "version", gitops.ProductionVersion)
			return err
		}
	}

	return nil
}

// reconcilePreviewVersion ensures the preview deployment exists for the given version.
func (r *GitOpsReconciler) reconcilePreviewVersion(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	gitops *networkingv1alpha2.GitOpsVersionConfig,
) error {
	versionName := gitops.PreviewVersion
	log := r.Log.WithValues("version", versionName, "type", "preview")

	// Find existing deployment by version name
	deployment, err := r.findDeploymentByVersion(ctx, project, versionName)
	if err != nil {
		return err
	}

	if deployment != nil {
		log.V(1).Info("Preview deployment already exists", "deployment", deployment.Name)
		return nil
	}

	// Create new preview deployment with preview metadata
	log.Info("Creating preview deployment for version")
	return r.createPreviewDeployment(ctx, project, versionName, gitops.SourceTemplate, gitops.PreviewMetadata)
}

// reconcileProductionVersion validates and promotes the production version.
//
//nolint:revive // cyclomatic complexity acceptable for promotion logic
func (r *GitOpsReconciler) reconcileProductionVersion(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	gitops *networkingv1alpha2.GitOpsVersionConfig,
	_ *cf.API, // apiClient no longer needed for promotion (uses environment change)
) error {
	versionName := gitops.ProductionVersion
	log := r.Log.WithValues("version", versionName, "type", "production")

	// Find deployment by version name
	deployment, err := r.findDeploymentByVersion(ctx, project, versionName)
	if err != nil {
		return err
	}

	if deployment == nil {
		return fmt.Errorf("version %s not found, cannot promote to production", versionName)
	}

	// Check if validation is required
	requireValidation := gitops.RequirePreviewValidation == nil || *gitops.RequirePreviewValidation
	if requireValidation {
		if !r.isVersionValidated(ctx, project, deployment, gitops.ValidationLabels) {
			return fmt.Errorf("version %s has not passed preview validation", versionName)
		}
	}

	// Check if deployment has succeeded
	if deployment.Status.State != networkingv1alpha2.PagesDeploymentStateSucceeded {
		log.Info("Waiting for deployment to succeed", "state", deployment.Status.State)
		return fmt.Errorf("deployment %s has not succeeded yet (state: %s)", deployment.Name, deployment.Status.State)
	}

	// Validate deployment is ready for promotion
	if err := ValidateDeploymentForPromotion(deployment); err != nil {
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
		fmt.Sprintf("Version %s promoted to production", versionName))

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
func (r *GitOpsReconciler) findDeploymentByVersion(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	versionName string,
) (*networkingv1alpha2.PagesDeployment, error) {
	// Method 1: Find by spec.versionName
	deployments := &networkingv1alpha2.PagesDeploymentList{}
	if err := r.List(ctx, deployments, client.InNamespace(project.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	for i := range deployments.Items {
		d := &deployments.Items[i]
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

// isVersionValidated checks if a version has been validated through preview.
//
//nolint:revive // cognitive complexity acceptable for validation logic
func (*GitOpsReconciler) isVersionValidated(
	_ context.Context,
	project *networkingv1alpha2.PagesProject,
	deployment *networkingv1alpha2.PagesDeployment,
	validationLabels map[string]string,
) bool {
	// Check deployment state
	if deployment.Status.State != networkingv1alpha2.PagesDeploymentStateSucceeded {
		return false
	}

	// Check validation labels if specified
	if len(validationLabels) > 0 {
		for k, v := range validationLabels {
			if deployment.Labels[k] != v {
				return false
			}
		}
	}

	// Check validation history
	for _, validation := range project.Status.ValidationHistory {
		if validation.VersionName == deployment.Spec.VersionName ||
			validation.VersionName == deployment.Status.VersionName {
			if validation.ValidationResult == "passed" {
				return true
			}
		}
	}

	// If environment was preview and succeeded, consider validated
	if deployment.Status.Environment == "preview" {
		return true
	}

	return false
}

// createPreviewDeployment creates a new PagesDeployment for preview.
func (r *GitOpsReconciler) createPreviewDeployment(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	versionName string,
	sourceTemplate *networkingv1alpha2.SourceTemplate,
	metadata map[string]string,
) error {
	deploymentName := fmt.Sprintf("%s-%s", project.Name, versionName)

	// Build source spec from template with metadata
	var source *networkingv1alpha2.PagesDeploymentSourceSpec
	if sourceTemplate != nil {
		directUpload, err := buildDirectUploadFromTemplate(versionName, sourceTemplate, metadata, project.Spec.ProductionBranch)
		if err != nil {
			return fmt.Errorf("failed to build source from template: %w", err)
		}
		source = &networkingv1alpha2.PagesDeploymentSourceSpec{
			Type:         networkingv1alpha2.PagesDeploymentSourceTypeDirectUpload,
			DirectUpload: directUpload,
		}
	}

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
			Source:      source,
			Cloudflare:  project.Spec.Cloudflare,
		},
	}

	// Set owner reference
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

	r.Log.Info("Created preview deployment", "deployment", deploymentName, "version", versionName)
	r.Recorder.Event(project, corev1.EventTypeNormal, "PreviewDeploymentCreated",
		fmt.Sprintf("Created preview deployment for version %s", versionName))

	return nil
}

// buildDirectUploadFromTemplate builds a PagesDirectUploadSourceSpec from a SourceTemplate.
// It merges metadata and ensures the branch is set correctly for production promotion.
//
//nolint:revive // cognitive complexity acceptable for metadata extraction with multiple optional fields
func buildDirectUploadFromTemplate(
	versionName string,
	template *networkingv1alpha2.SourceTemplate,
	metadata map[string]string,
	productionBranch string,
) (*networkingv1alpha2.PagesDirectUploadSourceSpec, error) {
	// Ensure branch is set in metadata for production promotion compatibility
	if metadata == nil {
		metadata = make(map[string]string)
	}
	if metadata["branch"] == "" && productionBranch != "" {
		metadata["branch"] = productionBranch
	}

	version, err := resolveFromTemplate(versionName, template, metadata)
	if err != nil {
		return nil, err
	}

	result := version.Source
	if result == nil {
		return nil, nil
	}

	// Extract metadata to DeploymentMetadata for Cloudflare API
	if len(version.Metadata) > 0 {
		dm := &networkingv1alpha2.DeploymentTriggerMetadata{}
		hasMetadata := false

		if v, ok := version.Metadata["commitHash"]; ok && v != "" {
			dm.CommitHash = v
			hasMetadata = true
		}
		if v, ok := version.Metadata["commitMessage"]; ok && v != "" {
			dm.CommitMessage = v
			hasMetadata = true
		}
		if v, ok := version.Metadata["commitDirty"]; ok {
			dirty := v == "true"
			dm.CommitDirty = &dirty
			hasMetadata = true
		}
		if v, ok := version.Metadata["branch"]; ok && v != "" {
			dm.Branch = v
			hasMetadata = true
		}

		if hasMetadata {
			result.DeploymentMetadata = dm
		}
	}

	return result, nil
}

// recordValidation records a version validation in the project status.
func (r *GitOpsReconciler) recordValidation(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	versionName, deploymentID string,
) error {
	now := metav1.Now()

	// Create new validation record
	validation := networkingv1alpha2.VersionValidation{
		VersionName:      versionName,
		DeploymentID:     deploymentID,
		ValidatedAt:      &now,
		ValidatedBy:      "gitops",
		ValidationResult: "passed",
	}

	// Update project status
	return r.updateProjectStatus(ctx, project, func(status *networkingv1alpha2.PagesProjectStatus) {
		// Add to validation history (keep last 50)
		status.ValidationHistory = append([]networkingv1alpha2.VersionValidation{validation}, status.ValidationHistory...)
		if len(status.ValidationHistory) > 50 {
			status.ValidationHistory = status.ValidationHistory[:50]
		}
	})
}

// updateProjectStatus updates the project status with a modification function.
func (r *GitOpsReconciler) updateProjectStatus(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	modify func(*networkingv1alpha2.PagesProjectStatus),
) error {
	// Get fresh copy
	fresh := &networkingv1alpha2.PagesProject{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(project), fresh); err != nil {
		return err
	}

	modify(&fresh.Status)
	return r.Status().Update(ctx, fresh)
}

// UpdateVersionMapping updates the version mapping in project status.
func (r *GitOpsReconciler) UpdateVersionMapping(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
) error {
	index := NewVersionIndex(r.Client)
	mapping, err := index.BuildIndex(ctx, project)
	if err != nil {
		return err
	}

	return r.updateProjectStatus(ctx, project, func(status *networkingv1alpha2.PagesProjectStatus) {
		status.VersionMapping = mapping
	})
}

// UpdatePreviewDeploymentStatus updates the preview deployment info in project status.
func (r *GitOpsReconciler) UpdatePreviewDeploymentStatus(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	deployment *networkingv1alpha2.PagesDeployment,
) error {
	return r.updateProjectStatus(ctx, project, func(status *networkingv1alpha2.PagesProjectStatus) {
		versionName := deployment.Spec.VersionName
		if versionName == "" {
			versionName = deployment.Status.VersionName
		}

		status.PreviewDeployment = &networkingv1alpha2.PreviewDeploymentInfo{
			VersionName:    versionName,
			DeploymentID:   deployment.Status.DeploymentID,
			DeploymentName: deployment.Name,
			URL:            deployment.Status.URL,
			HashURL:        deployment.Status.HashURL,
			State:          string(deployment.Status.State),
		}

		if deployment.Status.FinishedAt != nil {
			status.PreviewDeployment.DeployedAt = deployment.Status.FinishedAt
		}
	})
}
