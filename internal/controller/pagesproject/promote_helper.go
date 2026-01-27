// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesproject

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

// PromoteDeploymentToProduction promotes a deployment to production by updating its environment.
// This approach works for all deployments, not just those that were previously production.
// The PagesDeployment controller will detect the environment change and create a new deployment.
//
// Why use environment change instead of Cloudflare Rollback API:
// - Rollback API only works for deployments that were PREVIOUSLY production
// - If a version was never deployed as production, Rollback API will fail
// - Environment change works for all deployments and is more reliable
func PromoteDeploymentToProduction(
	ctx context.Context,
	k8sClient client.Client,
	deployment *networkingv1alpha2.PagesDeployment,
) error {
	if deployment.Spec.Environment == networkingv1alpha2.PagesDeploymentEnvironmentProduction {
		return nil // Already production
	}

	return controller.UpdateWithConflictRetry(ctx, k8sClient, deployment, func() {
		deployment.Spec.Environment = networkingv1alpha2.PagesDeploymentEnvironmentProduction
	})
}

// DemoteDeploymentToPreview demotes a deployment from production to preview.
// Used when another deployment is promoted to production.
func DemoteDeploymentToPreview(
	ctx context.Context,
	k8sClient client.Client,
	deployment *networkingv1alpha2.PagesDeployment,
) error {
	if deployment.Spec.Environment == networkingv1alpha2.PagesDeploymentEnvironmentPreview {
		return nil // Already preview
	}

	return controller.UpdateWithConflictRetry(ctx, k8sClient, deployment, func() {
		deployment.Spec.Environment = networkingv1alpha2.PagesDeploymentEnvironmentPreview
	})
}

// GetDeploymentVersionName extracts the version name from a deployment.
// Checks spec.versionName, status.versionName, and version label in order.
func GetDeploymentVersionName(deployment *networkingv1alpha2.PagesDeployment) string {
	if deployment.Spec.VersionName != "" {
		return deployment.Spec.VersionName
	}
	if deployment.Status.VersionName != "" {
		return deployment.Status.VersionName
	}
	return deployment.Labels[VersionLabel]
}

// IsDeploymentSucceeded checks if a deployment has succeeded.
func IsDeploymentSucceeded(deployment *networkingv1alpha2.PagesDeployment) bool {
	return deployment.Status.State == networkingv1alpha2.PagesDeploymentStateSucceeded
}

// IsDeploymentProduction checks if a deployment is in production environment.
func IsDeploymentProduction(deployment *networkingv1alpha2.PagesDeployment) bool {
	return deployment.Spec.Environment == networkingv1alpha2.PagesDeploymentEnvironmentProduction
}

// HasCloudflareDeploymentID checks if a deployment has a Cloudflare deployment ID.
func HasCloudflareDeploymentID(deployment *networkingv1alpha2.PagesDeployment) bool {
	return deployment.Status.DeploymentID != ""
}

// ValidateDeploymentForPromotion validates that a deployment can be promoted to production.
// Returns an error if the deployment is not ready for promotion.
func ValidateDeploymentForPromotion(deployment *networkingv1alpha2.PagesDeployment) error {
	if !IsDeploymentSucceeded(deployment) {
		return fmt.Errorf("deployment %s has not succeeded yet (state: %s)",
			deployment.Name, deployment.Status.State)
	}

	if !HasCloudflareDeploymentID(deployment) {
		return fmt.Errorf("deployment %s has no Cloudflare deployment ID", deployment.Name)
	}

	return nil
}
