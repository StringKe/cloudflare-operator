// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesproject

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// VersionIndex maintains a mapping of versionName -> deploymentId.
// This allows efficient lookup of deployments by version name.
type VersionIndex struct {
	client.Client
}

// NewVersionIndex creates a new VersionIndex.
func NewVersionIndex(k8sClient client.Client) *VersionIndex {
	return &VersionIndex{
		Client: k8sClient,
	}
}

// BuildIndex builds a version index for the given project.
// Returns a map of versionName -> deploymentId.
//
//nolint:revive // cognitive complexity acceptable for index building
func (idx *VersionIndex) BuildIndex(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
) (map[string]string, error) {
	index := make(map[string]string)

	// List all PagesDeployments in the namespace
	deployments := &networkingv1alpha2.PagesDeploymentList{}
	if err := idx.List(ctx, deployments, client.InNamespace(project.Namespace)); err != nil {
		return nil, err
	}

	for _, d := range deployments.Items {
		// Check if deployment belongs to this project
		if !idx.belongsToProject(&d, project) {
			continue
		}

		// Get version name from spec or status
		versionName := d.Spec.VersionName
		if versionName == "" {
			versionName = d.Status.VersionName
		}
		if versionName == "" {
			// Try version label
			versionName = d.Labels[VersionLabel]
		}

		// Only add if we have both version name and deployment ID
		if versionName != "" && d.Status.DeploymentID != "" {
			index[versionName] = d.Status.DeploymentID
		}
	}

	return index, nil
}

// FindDeploymentByVersion finds a PagesDeployment by version name.
//
//nolint:revive // cognitive complexity acceptable for search logic
func (idx *VersionIndex) FindDeploymentByVersion(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	versionName string,
) (*networkingv1alpha2.PagesDeployment, error) {
	deployments := &networkingv1alpha2.PagesDeploymentList{}
	if err := idx.List(ctx, deployments, client.InNamespace(project.Namespace)); err != nil {
		return nil, err
	}

	for i := range deployments.Items {
		d := &deployments.Items[i]

		// Check if deployment belongs to this project
		if !idx.belongsToProject(d, project) {
			continue
		}

		// Match by spec.versionName
		if d.Spec.VersionName == versionName {
			return d, nil
		}

		// Match by status.versionName
		if d.Status.VersionName == versionName {
			return d, nil
		}

		// Match by version label
		if d.Labels[VersionLabel] == versionName {
			return d, nil
		}
	}

	return nil, nil
}

// FindDeploymentByID finds a PagesDeployment by Cloudflare deployment ID.
func (idx *VersionIndex) FindDeploymentByID(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	deploymentID string,
) (*networkingv1alpha2.PagesDeployment, error) {
	deployments := &networkingv1alpha2.PagesDeploymentList{}
	if err := idx.List(ctx, deployments, client.InNamespace(project.Namespace)); err != nil {
		return nil, err
	}

	for i := range deployments.Items {
		d := &deployments.Items[i]

		// Check if deployment belongs to this project
		if !idx.belongsToProject(d, project) {
			continue
		}

		// Match by deployment ID
		if d.Status.DeploymentID == deploymentID {
			return d, nil
		}
	}

	return nil, nil
}

// ListVersions returns a list of all version names for a project.
func (idx *VersionIndex) ListVersions(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
) ([]string, error) {
	index, err := idx.BuildIndex(ctx, project)
	if err != nil {
		return nil, err
	}

	versions := make([]string, 0, len(index))
	for v := range index {
		versions = append(versions, v)
	}

	return versions, nil
}

// GetDeploymentID returns the deployment ID for a version name.
func (idx *VersionIndex) GetDeploymentID(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	versionName string,
) (string, error) {
	deployment, err := idx.FindDeploymentByVersion(ctx, project, versionName)
	if err != nil {
		return "", err
	}
	if deployment == nil {
		return "", nil
	}
	return deployment.Status.DeploymentID, nil
}

// belongsToProject checks if a deployment belongs to the given project.
func (*VersionIndex) belongsToProject(
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

// VersionInfo contains information about a version.
type VersionInfo struct {
	// Name is the version name
	Name string
	// DeploymentID is the Cloudflare deployment ID
	DeploymentID string
	// DeploymentName is the K8s PagesDeployment name
	DeploymentName string
	// State is the deployment state
	State networkingv1alpha2.PagesDeploymentState
	// Environment is the deployment environment
	Environment string
	// IsProduction indicates if this is the current production deployment
	IsProduction bool
}

// ListVersionsWithInfo returns detailed information about all versions.
//
//nolint:revive // cognitive complexity acceptable for list building
func (idx *VersionIndex) ListVersionsWithInfo(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
) ([]VersionInfo, error) {
	deployments := &networkingv1alpha2.PagesDeploymentList{}
	if err := idx.List(ctx, deployments, client.InNamespace(project.Namespace)); err != nil {
		return nil, err
	}

	versions := make([]VersionInfo, 0, len(deployments.Items))
	for _, d := range deployments.Items {
		// Check if deployment belongs to this project
		if !idx.belongsToProject(&d, project) {
			continue
		}

		// Get version name
		versionName := d.Spec.VersionName
		if versionName == "" {
			versionName = d.Status.VersionName
		}
		if versionName == "" {
			versionName = d.Labels[VersionLabel]
		}

		if versionName == "" {
			continue
		}

		versions = append(versions, VersionInfo{
			Name:           versionName,
			DeploymentID:   d.Status.DeploymentID,
			DeploymentName: d.Name,
			State:          d.Status.State,
			Environment:    d.Status.Environment,
			IsProduction:   d.Status.IsCurrentProduction,
		})
	}

	return versions, nil
}
