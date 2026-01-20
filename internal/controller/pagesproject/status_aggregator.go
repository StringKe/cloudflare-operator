// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesproject

import (
	"context"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

// aggregateVersionStatus aggregates status from all managed deployments.
func (r *PagesProjectReconciler) aggregateVersionStatus(ctx context.Context, project *networkingv1alpha2.PagesProject) error {
	deployments, err := r.versionManager.listManagedDeployments(ctx, project)
	if err != nil {
		return err
	}

	managedVersions := make([]networkingv1alpha2.ManagedVersionStatus, 0, len(deployments))
	var currentProduction *networkingv1alpha2.ProductionDeploymentInfo

	for i := range deployments {
		dep := &deployments[i]
		versionName := dep.Labels[VersionLabel]

		status := networkingv1alpha2.ManagedVersionStatus{
			Name:           versionName,
			DeploymentName: dep.Name,
			State:          string(dep.Status.State),
			IsProduction:   dep.Spec.Environment == networkingv1alpha2.PagesDeploymentEnvironmentProduction,
			DeploymentID:   dep.Status.DeploymentID,
		}

		// Get last transition time from conditions
		if len(dep.Status.Conditions) > 0 {
			lastCond := dep.Status.Conditions[len(dep.Status.Conditions)-1]
			status.LastTransitionTime = &lastCond.LastTransitionTime
		}

		managedVersions = append(managedVersions, status)

		// Track current production deployment
		if status.IsProduction && dep.Status.State == networkingv1alpha2.PagesDeploymentStateSucceeded {
			currentProduction = &networkingv1alpha2.ProductionDeploymentInfo{
				Version:        versionName,
				DeploymentID:   dep.Status.DeploymentID,
				DeploymentName: dep.Name,
				URL:            dep.Status.URL,
				HashURL:        dep.Status.HashURL,
				DeployedAt:     dep.Status.FinishedAt,
			}
		}
	}

	// Update status
	return controller.UpdateStatusWithConflictRetry(ctx, r.Client, project, func() {
		project.Status.ManagedDeployments = int32(len(deployments))
		project.Status.ManagedVersions = managedVersions
		project.Status.CurrentProduction = currentProduction
	})
}
