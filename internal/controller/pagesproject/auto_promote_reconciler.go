// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesproject

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

const (
	// DefaultHealthCheckTimeout is the default timeout for health checks.
	DefaultHealthCheckTimeout = 30 * time.Second
	// HealthCheckSuccessCode is the HTTP status code considered as healthy.
	HealthCheckSuccessCode = http.StatusOK
)

// AutoPromoteReconciler handles the autoPromote version management policy.
// It automatically promotes preview deployments to production after they succeed,
// with optional delay and health check.
type AutoPromoteReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	Log        logr.Logger
	HTTPClient *http.Client
}

// NewAutoPromoteReconciler creates a new AutoPromoteReconciler.
func NewAutoPromoteReconciler(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
	log logr.Logger,
) *AutoPromoteReconciler {
	return &AutoPromoteReconciler{
		Client:   k8sClient,
		Scheme:   scheme,
		Recorder: recorder,
		Log:      log.WithName("autoPromote"),
		HTTPClient: &http.Client{
			Timeout: DefaultHealthCheckTimeout,
		},
	}
}

// Reconcile handles the autoPromote version management workflow.
//
//nolint:revive // cognitive complexity acceptable for reconciliation logic
func (r *AutoPromoteReconciler) Reconcile(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	apiClient *cf.API,
) (requeueAfter time.Duration, err error) {
	config := project.Spec.VersionManagement.AutoPromote
	if config == nil {
		return 0, nil
	}

	log := r.Log.WithValues("project", project.Name, "namespace", project.Namespace)

	// 1. Find all preview deployments for this project
	deployments, err := r.findPreviewDeployments(ctx, project)
	if err != nil {
		return 0, fmt.Errorf("failed to find preview deployments: %w", err)
	}

	if len(deployments) == 0 {
		log.V(1).Info("No preview deployments found")
		return 0, nil
	}

	// 2. Find the latest successful preview deployment
	latest := r.findLatestSuccessfulPreview(deployments)
	if latest == nil {
		log.V(1).Info("No successful preview deployment found")
		return 0, nil
	}

	log.Info("Found latest successful preview",
		"deployment", latest.Name,
		"version", latest.Spec.VersionName,
		"deploymentId", latest.Status.DeploymentID)

	// 3. Check if this deployment is already production
	if project.Status.CurrentProduction != nil &&
		project.Status.CurrentProduction.DeploymentID == latest.Status.DeploymentID {
		log.V(1).Info("Deployment is already production")
		return 0, nil
	}

	// 4. Check if we need to wait (PromoteAfter delay)
	if config.PromoteAfter != nil && config.PromoteAfter.Duration > 0 {
		waitTime, shouldWait := r.checkPromoteDelay(latest, config.PromoteAfter.Duration)
		if shouldWait {
			log.Info("Waiting before auto-promotion",
				"waitTime", waitTime,
				"promoteAfter", config.PromoteAfter.Duration)
			return waitTime, nil
		}
	}

	// 5. Run health check if required
	if config.RequireHealthCheck {
		healthCheckURL := config.HealthCheckURL
		if healthCheckURL == "" {
			// Use deployment URL as health check target
			healthCheckURL = latest.Status.URL
		}

		timeout := DefaultHealthCheckTimeout
		if config.HealthCheckTimeout != nil {
			timeout = config.HealthCheckTimeout.Duration
		}

		if err := r.performHealthCheck(ctx, healthCheckURL, timeout); err != nil {
			log.Error(err, "Health check failed, skipping promotion",
				"url", healthCheckURL)
			r.Recorder.Event(project, corev1.EventTypeWarning, "HealthCheckFailed",
				fmt.Sprintf("Health check failed for deployment %s: %s",
					latest.Name, err.Error()))
			// Requeue to retry health check
			return 30 * time.Second, nil
		}
		log.Info("Health check passed", "url", healthCheckURL)
	}

	// 6. Promote to production
	if err := r.promoteToProduction(ctx, project, latest, apiClient); err != nil {
		return 0, err
	}

	return 0, nil
}

// findPreviewDeployments finds all preview PagesDeployment resources for this project.
func (r *AutoPromoteReconciler) findPreviewDeployments(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
) ([]*networkingv1alpha2.PagesDeployment, error) {
	deployments := &networkingv1alpha2.PagesDeploymentList{}
	if err := r.List(ctx, deployments, client.InNamespace(project.Namespace)); err != nil {
		return nil, err
	}

	result := make([]*networkingv1alpha2.PagesDeployment, 0, len(deployments.Items))
	for i := range deployments.Items {
		d := &deployments.Items[i]

		// Check if deployment belongs to this project
		if !r.belongsToProject(d, project) {
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
func (*AutoPromoteReconciler) belongsToProject(
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
func (*AutoPromoteReconciler) findLatestSuccessfulPreview(
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

// checkPromoteDelay checks if the promotion delay has passed.
// Returns (waitTime, shouldWait).
func (*AutoPromoteReconciler) checkPromoteDelay(
	deployment *networkingv1alpha2.PagesDeployment,
	promoteAfter time.Duration,
) (time.Duration, bool) {
	finishedAt := deployment.Status.FinishedAt
	if finishedAt == nil {
		// Use creation timestamp if FinishedAt is not set
		finishedAt = &deployment.CreationTimestamp
	}

	promoteTime := finishedAt.Add(promoteAfter)
	now := time.Now()

	if now.Before(promoteTime) {
		return promoteTime.Sub(now), true
	}

	return 0, false
}

// performHealthCheck performs an HTTP health check on the given URL.
func (r *AutoPromoteReconciler) performHealthCheck(
	ctx context.Context,
	url string,
	timeout time.Duration,
) error {
	if url == "" {
		return errors.New("health check URL is empty")
	}

	// Create request with context
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set user agent
	req.Header.Set("User-Agent", "cloudflare-operator/health-check")

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer func() {
		// Drain and close body to reuse connection
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != HealthCheckSuccessCode {
		return fmt.Errorf("health check returned status %d, expected %d",
			resp.StatusCode, HealthCheckSuccessCode)
	}

	return nil
}

// promoteToProduction promotes the deployment to production.
func (r *AutoPromoteReconciler) promoteToProduction(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	deployment *networkingv1alpha2.PagesDeployment,
	apiClient *cf.API,
) error {
	log := r.Log.WithValues(
		"project", project.Name,
		"deployment", deployment.Name,
	)

	// Get project name for API call
	projectName := project.Spec.Name
	if projectName == "" {
		projectName = project.Name
	}

	// Promote via Cloudflare Rollback API
	log.Info("Auto-promoting preview deployment to production",
		"deploymentId", deployment.Status.DeploymentID)

	_, err := apiClient.RollbackPagesDeployment(ctx, projectName, deployment.Status.DeploymentID)
	if err != nil {
		r.Recorder.Event(project, corev1.EventTypeWarning, "AutoPromoteFailed",
			fmt.Sprintf("Failed to auto-promote deployment %s: %s",
				deployment.Name, cf.SanitizeErrorMessage(err)))
		return fmt.Errorf("failed to promote deployment: %w", err)
	}

	versionName := deployment.Spec.VersionName
	if versionName == "" {
		versionName = deployment.Status.VersionName
	}

	r.Recorder.Event(project, corev1.EventTypeNormal, "AutoPromoted",
		fmt.Sprintf("Auto-promoted deployment %s (version: %s) to production",
			deployment.Name, versionName))

	// Update preview deployment status
	if err := r.updatePreviewDeploymentStatus(ctx, project, deployment); err != nil {
		log.Error(err, "Failed to update preview deployment status")
		// Non-fatal
	}

	// Record validation history
	if err := r.recordValidation(ctx, project, deployment); err != nil {
		log.Error(err, "Failed to record validation history")
		// Non-fatal
	}

	return nil
}

// updatePreviewDeploymentStatus updates the preview deployment info in project status.
func (r *AutoPromoteReconciler) updatePreviewDeploymentStatus(
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

// recordValidation records a version validation in the project status.
func (r *AutoPromoteReconciler) recordValidation(
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
		ValidatedBy:      "autoPromote",
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

// GetRequeueAfter returns the recommended requeue duration for autoPromote mode.
func (*AutoPromoteReconciler) GetRequeueAfter() time.Duration {
	// Check for new preview deployments every 30 seconds
	return 30 * time.Second
}
