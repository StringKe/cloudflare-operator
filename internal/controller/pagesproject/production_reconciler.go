// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesproject

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

// reconcileProductionTarget ensures the correct deployment is promoted to production.
func (r *PagesProjectReconciler) reconcileProductionTarget(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
) error {
	logger := log.FromContext(ctx)

	if project.Spec.ProductionTarget == "" {
		logger.V(1).Info("No production target specified, skipping production reconciliation")
		return nil
	}

	// 1. Resolve and find target deployment
	targetVersion := r.resolveTargetVersion(project)
	if targetVersion == "" {
		err := fmt.Errorf("failed to resolve production target %q", project.Spec.ProductionTarget)
		r.Recorder.Event(project, corev1.EventTypeWarning, "ProductionTargetInvalid", err.Error())
		return err
	}

	targetDeployment, err := r.findDeploymentForVersion(ctx, project, targetVersion)
	if err != nil {
		return fmt.Errorf("find deployment for version %s: %w", targetVersion, err)
	}
	if targetDeployment == nil {
		r.Recorder.Event(project, corev1.EventTypeWarning, "ProductionTargetNotFound",
			fmt.Sprintf("Deployment for version %q not found", targetVersion))
		return fmt.Errorf("deployment for version %s not found", targetVersion)
	}

	// 2. Promote target to production
	if err := r.promoteToProduction(ctx, project, targetDeployment, targetVersion); err != nil {
		return err
	}

	// 3. Demote other deployments
	return r.demoteOtherDeployments(ctx, project, targetDeployment)
}

// promoteToProduction promotes a deployment to production environment.
func (r *PagesProjectReconciler) promoteToProduction(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	deployment *networkingv1alpha2.PagesDeployment,
	version string,
) error {
	if deployment.Spec.Environment == networkingv1alpha2.PagesDeploymentEnvironmentProduction {
		return nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Promoting deployment to production", "version", version, "deployment", deployment.Name)

	if err := r.setEnvironment(ctx, deployment, networkingv1alpha2.PagesDeploymentEnvironmentProduction); err != nil {
		return fmt.Errorf("promote to production %s: %w", deployment.Name, err)
	}

	r.Recorder.Event(project, corev1.EventTypeNormal, "ProductionPromoted",
		fmt.Sprintf("Version %q promoted to production", version))
	return nil
}

// demoteOtherDeployments demotes all other managed deployments from production.
func (r *PagesProjectReconciler) demoteOtherDeployments(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	targetDeployment *networkingv1alpha2.PagesDeployment,
) error {
	allDeployments, err := r.versionManager.listManagedDeployments(ctx, project)
	if err != nil {
		return fmt.Errorf("list managed deployments: %w", err)
	}

	for i := range allDeployments {
		dep := &allDeployments[i]
		if err := r.demoteIfNeeded(ctx, project, dep, targetDeployment); err != nil {
			return err
		}
	}

	return nil
}

// demoteIfNeeded demotes a deployment from production if it's not the target.
func (r *PagesProjectReconciler) demoteIfNeeded(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	deployment *networkingv1alpha2.PagesDeployment,
	targetDeployment *networkingv1alpha2.PagesDeployment,
) error {
	if deployment.Name == targetDeployment.Name {
		return nil
	}

	if deployment.Spec.Environment != networkingv1alpha2.PagesDeploymentEnvironmentProduction {
		return nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Demoting deployment from production", "deployment", deployment.Name)

	if err := r.setEnvironment(ctx, deployment, networkingv1alpha2.PagesDeploymentEnvironmentPreview); err != nil {
		return fmt.Errorf("demote from production %s: %w", deployment.Name, err)
	}

	r.Recorder.Event(project, corev1.EventTypeNormal, "ProductionDemoted",
		fmt.Sprintf("Deployment %s demoted to preview", deployment.Name))
	return nil
}

// resolveTargetVersion resolves the production target to a specific version name.
func (*PagesProjectReconciler) resolveTargetVersion(project *networkingv1alpha2.PagesProject) string {
	if project.Spec.ProductionTarget == "latest" {
		if len(project.Spec.Versions) > 0 {
			return project.Spec.Versions[0].Name
		}
		return ""
	}
	return project.Spec.ProductionTarget
}

// findDeploymentForVersion finds the PagesDeployment for a specific version.
func (r *PagesProjectReconciler) findDeploymentForVersion(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	versionName string,
) (*networkingv1alpha2.PagesDeployment, error) {
	name := fmt.Sprintf("%s-%s", project.Name, versionName)
	deployment := &networkingv1alpha2.PagesDeployment{}

	err := r.Get(ctx, client.ObjectKey{Namespace: project.Namespace, Name: name}, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	// Verify it's managed by this project
	if deployment.Labels[ManagedByLabel] != ManagedByValue ||
		deployment.Labels[ManagedByNameLabel] != project.Name {
		return nil, nil
	}

	return deployment, nil
}

// setEnvironment updates the environment of a PagesDeployment.
func (r *PagesProjectReconciler) setEnvironment(
	ctx context.Context,
	deployment *networkingv1alpha2.PagesDeployment,
	env networkingv1alpha2.PagesDeploymentEnvironment,
) error {
	return controller.UpdateWithConflictRetry(ctx, r.Client, deployment, func() {
		deployment.Spec.Environment = env
	})
}
