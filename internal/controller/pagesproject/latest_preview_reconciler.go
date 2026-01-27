// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesproject

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

// LatestPreviewReconciler handles the latestPreview version management policy.
// It automatically tracks the latest successful preview deployment and optionally auto-promotes.
type LatestPreviewReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Log      logr.Logger
}

// NewLatestPreviewReconciler creates a new LatestPreviewReconciler.
func NewLatestPreviewReconciler(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
	log logr.Logger,
) *LatestPreviewReconciler {
	return &LatestPreviewReconciler{
		Client:   k8sClient,
		Scheme:   scheme,
		Recorder: recorder,
		Log:      log.WithName("latestPreview"),
	}
}

// Reconcile handles the latestPreview version management workflow.
//
//nolint:revive // cognitive complexity acceptable for reconciliation logic
func (r *LatestPreviewReconciler) Reconcile(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	apiClient *cf.API,
) error {
	config := project.Spec.VersionManagement.LatestPreview
	if config == nil {
		return nil
	}

	log := r.Log.WithValues("project", project.Name, "namespace", project.Namespace)

	// 1. Find all preview deployments matching the selector
	deployments, err := r.findMatchingDeployments(ctx, project, config)
	if err != nil {
		return fmt.Errorf("failed to find matching deployments: %w", err)
	}

	if len(deployments) == 0 {
		log.V(1).Info("No matching preview deployments found")
		return nil
	}

	// 2. Find the latest successful preview deployment
	latest := r.findLatestSuccessfulPreview(deployments)
	if latest == nil {
		log.V(1).Info("No successful preview deployment found")
		return nil
	}

	log.Info("Found latest successful preview",
		"deployment", latest.Name,
		"version", latest.Spec.VersionName,
		"deploymentId", latest.Status.DeploymentID)

	// 3. Update preview deployment status in project
	if err := r.updatePreviewDeploymentStatus(ctx, project, latest); err != nil {
		log.Error(err, "Failed to update preview deployment status")
		// Non-fatal, continue
	}

	// 4. Handle auto-promote if enabled
	if config.AutoPromote {
		if err := r.handleAutoPromote(ctx, project, latest, apiClient); err != nil {
			log.Error(err, "Failed to auto-promote")
			return err
		}
	}

	return nil
}

// findMatchingDeployments finds all PagesDeployment resources matching the selector.
//
//nolint:revive // cognitive complexity acceptable for filtering logic
func (r *LatestPreviewReconciler) findMatchingDeployments(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	config *networkingv1alpha2.LatestPreviewConfig,
) ([]*networkingv1alpha2.PagesDeployment, error) {
	deployments := &networkingv1alpha2.PagesDeploymentList{}
	if err := r.List(ctx, deployments, client.InNamespace(project.Namespace)); err != nil {
		return nil, err
	}

	// Build label selector if specified
	var selector labels.Selector
	if config.LabelSelector != nil {
		var err error
		selector, err = metav1.LabelSelectorAsSelector(config.LabelSelector)
		if err != nil {
			return nil, fmt.Errorf("invalid label selector: %w", err)
		}
	}

	result := make([]*networkingv1alpha2.PagesDeployment, 0, len(deployments.Items))
	for i := range deployments.Items {
		d := &deployments.Items[i]

		// Check if deployment belongs to this project
		if !r.belongsToProject(d, project) {
			continue
		}

		// Check label selector if specified
		if selector != nil && !selector.Matches(labels.Set(d.Labels)) {
			continue
		}

		// Only include preview deployments
		if d.Spec.Environment != networkingv1alpha2.PagesDeploymentEnvironmentPreview {
			continue
		}

		result = append(result, d)
	}

	return result, nil
}

// belongsToProject checks if a deployment belongs to the given project.
func (*LatestPreviewReconciler) belongsToProject(
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

// findLatestSuccessfulPreview finds the most recently succeeded preview deployment.
//
//nolint:revive // cognitive complexity acceptable for sorting logic
func (*LatestPreviewReconciler) findLatestSuccessfulPreview(
	deployments []*networkingv1alpha2.PagesDeployment,
) *networkingv1alpha2.PagesDeployment {
	// Filter to only succeeded deployments
	var succeeded []*networkingv1alpha2.PagesDeployment
	for _, d := range deployments {
		if d.Status.State == networkingv1alpha2.PagesDeploymentStateSucceeded {
			succeeded = append(succeeded, d)
		}
	}

	if len(succeeded) == 0 {
		return nil
	}

	// Sort by FinishedAt time (most recent first)
	sort.Slice(succeeded, func(i, j int) bool {
		ti := succeeded[i].Status.FinishedAt
		tj := succeeded[j].Status.FinishedAt
		if ti == nil && tj == nil {
			// Fall back to creation timestamp
			return succeeded[i].CreationTimestamp.After(succeeded[j].CreationTimestamp.Time)
		}
		if ti == nil {
			return false
		}
		if tj == nil {
			return true
		}
		return ti.After(tj.Time)
	})

	return succeeded[0]
}

// updatePreviewDeploymentStatus updates the preview deployment info in project status.
func (r *LatestPreviewReconciler) updatePreviewDeploymentStatus(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	deployment *networkingv1alpha2.PagesDeployment,
) error {
	// Get fresh copy
	fresh := &networkingv1alpha2.PagesProject{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(project), fresh); err != nil {
		return err
	}

	versionName := deployment.Spec.VersionName
	if versionName == "" {
		versionName = deployment.Status.VersionName
	}
	if versionName == "" {
		versionName = deployment.Labels[VersionLabel]
	}

	fresh.Status.PreviewDeployment = &networkingv1alpha2.PreviewDeploymentInfo{
		VersionName:    versionName,
		DeploymentID:   deployment.Status.DeploymentID,
		DeploymentName: deployment.Name,
		URL:            deployment.Status.URL,
		HashURL:        deployment.Status.HashURL,
		State:          string(deployment.Status.State),
	}

	if deployment.Status.FinishedAt != nil {
		fresh.Status.PreviewDeployment.DeployedAt = deployment.Status.FinishedAt
	}

	return r.Status().Update(ctx, fresh)
}

// handleAutoPromote promotes the latest successful preview to production if enabled.
func (r *LatestPreviewReconciler) handleAutoPromote(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	deployment *networkingv1alpha2.PagesDeployment,
	_ *cf.API, // apiClient no longer needed for promotion (uses environment change)
) error {
	log := r.Log.WithValues(
		"project", project.Name,
		"deployment", deployment.Name,
	)

	// Check if already production
	if project.Status.CurrentProduction != nil &&
		project.Status.CurrentProduction.DeploymentID == deployment.Status.DeploymentID {
		log.V(1).Info("Deployment is already production")
		return nil
	}

	// Promote by changing environment (works for all deployments, not just previous production)
	log.Info("Auto-promoting preview deployment to production",
		"deployment", deployment.Name)

	if err := PromoteDeploymentToProduction(ctx, r.Client, deployment); err != nil {
		r.Recorder.Event(project, corev1.EventTypeWarning, "AutoPromoteFailed",
			fmt.Sprintf("Failed to auto-promote deployment %s: %s",
				deployment.Name, err.Error()))
		return fmt.Errorf("failed to promote deployment: %w", err)
	}

	versionName := GetDeploymentVersionName(deployment)

	r.Recorder.Event(project, corev1.EventTypeNormal, "AutoPromoted",
		fmt.Sprintf("Auto-promoted deployment %s (version: %s) to production",
			deployment.Name, versionName))

	// Record validation history
	if err := r.recordValidation(ctx, project, deployment); err != nil {
		log.Error(err, "Failed to record validation history")
		// Non-fatal
	}

	return nil
}

// recordValidation records a version validation in the project status.
func (r *LatestPreviewReconciler) recordValidation(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	deployment *networkingv1alpha2.PagesDeployment,
) error {
	// Get fresh copy
	fresh := &networkingv1alpha2.PagesProject{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(project), fresh); err != nil {
		return err
	}

	versionName := deployment.Spec.VersionName
	if versionName == "" {
		versionName = deployment.Status.VersionName
	}

	now := metav1.Now()
	validation := networkingv1alpha2.VersionValidation{
		VersionName:      versionName,
		DeploymentID:     deployment.Status.DeploymentID,
		ValidatedAt:      &now,
		ValidatedBy:      "latestPreview",
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

// GetRequeueAfter returns the recommended requeue duration for latestPreview mode.
func (*LatestPreviewReconciler) GetRequeueAfter() time.Duration {
	// Check for new preview deployments every 30 seconds
	return 30 * time.Second
}
