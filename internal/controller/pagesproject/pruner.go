// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesproject

import (
	"context"
	"sort"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// pruneOldVersions deletes old managed deployments based on revisionHistoryLimit.
func (r *PagesProjectReconciler) pruneOldVersions(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
) error {
	logger := log.FromContext(ctx)

	// Get the revision limit and check if pruning is needed
	limit := r.getRevisionLimit(project)
	if limit == 0 {
		return nil
	}

	// Get and check all managed deployments
	allDeployments, err := r.versionManager.listManagedDeployments(ctx, project)
	if err != nil {
		return err
	}

	if len(allDeployments) <= int(limit) {
		logger.V(1).Info("Under revision limit, no pruning needed",
			"current", len(allDeployments), "limit", limit)
		return nil
	}

	// Sort and prune deployments
	r.sortDeploymentsByPriority(allDeployments)
	r.deleteOldDeployments(ctx, logger, allDeployments[limit:])

	return nil
}

// getRevisionLimit returns the revision history limit (default 10).
func (*PagesProjectReconciler) getRevisionLimit(project *networkingv1alpha2.PagesProject) int32 {
	if project.Spec.RevisionHistoryLimit != nil {
		return *project.Spec.RevisionHistoryLimit
	}
	return 10
}

// sortDeploymentsByPriority sorts deployments: production first, then by creation time (newest first).
func (*PagesProjectReconciler) sortDeploymentsByPriority(deployments []networkingv1alpha2.PagesDeployment) {
	sort.Slice(deployments, func(i, j int) bool {
		depI := &deployments[i]
		depJ := &deployments[j]

		// Production deployments always come first
		if depI.Spec.Environment != depJ.Spec.Environment {
			return depI.Spec.Environment == networkingv1alpha2.PagesDeploymentEnvironmentProduction
		}

		// Otherwise sort by creation time (newest first)
		return depI.CreationTimestamp.After(depJ.CreationTimestamp.Time)
	})
}

// deleteOldDeployments deletes the specified deployments.
func (r *PagesProjectReconciler) deleteOldDeployments(
	ctx context.Context,
	logger logr.Logger,
	toDelete []networkingv1alpha2.PagesDeployment,
) {
	for i := range toDelete {
		dep := &toDelete[i]

		// Safety check: never delete production deployments during pruning
		if dep.Spec.Environment == networkingv1alpha2.PagesDeploymentEnvironmentProduction {
			logger.Info("Skipping production deployment during pruning (safety protection)",
				"deployment", dep.Name)
			continue
		}

		logger.Info("Pruning old deployment", "deployment", dep.Name,
			"version", dep.Labels[VersionLabel],
			"age", dep.CreationTimestamp.String())

		if err := r.Delete(ctx, dep); err != nil {
			logger.Error(err, "Failed to delete old deployment during pruning", "deployment", dep.Name)
			// Continue with other deletions even if one fails
			continue
		}

		logger.Info("Successfully pruned deployment", "deployment", dep.Name)
	}
}
