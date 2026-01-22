// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesproject

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// ResolvedVersions contains the resolved versions and production target.
type ResolvedVersions struct {
	Versions         []networkingv1alpha2.ProjectVersion
	ProductionTarget string
}

// VersionManager manages declarative versions for PagesProject.
type VersionManager struct {
	client.Client
	Scheme *runtime.Scheme
	log    logr.Logger
}

// NewVersionManager creates a new VersionManager.
func NewVersionManager(k8sClient client.Client, scheme *runtime.Scheme, log logr.Logger) *VersionManager {
	return &VersionManager{
		Client: k8sClient,
		Scheme: scheme,
		log:    log,
	}
}

// ResolveVersions resolves versions based on the version management configuration.
// This supports three modes: targetVersion, declarativeVersions, and fullVersions.
// For backward compatibility, it also supports the legacy spec.versions field.
func (vm *VersionManager) ResolveVersions(project *networkingv1alpha2.PagesProject) (*ResolvedVersions, error) {
	mgmt := project.Spec.VersionManagement

	// Backward compatibility: use legacy fields if VersionManagement is not set
	if mgmt == nil {
		//nolint:staticcheck // Intentionally using deprecated fields for backward compatibility
		return &ResolvedVersions{
			Versions:         project.Spec.Versions,
			ProductionTarget: project.Spec.ProductionTarget,
		}, nil
	}

	switch mgmt.Type {
	case networkingv1alpha2.TargetVersionMode:
		return vm.resolveTargetVersion(mgmt.TargetVersion)

	case networkingv1alpha2.DeclarativeVersionsMode:
		return vm.resolveDeclarativeVersions(mgmt.DeclarativeVersions)

	case networkingv1alpha2.FullVersionsMode:
		return vm.resolveFullVersions(mgmt.FullVersions)

	default:
		return nil, fmt.Errorf("unknown version management type: %s", mgmt.Type)
	}
}

// resolveTargetVersion resolves a single target version using the source template.
func (*VersionManager) resolveTargetVersion(spec *networkingv1alpha2.TargetVersionSpec) (*ResolvedVersions, error) {
	if spec == nil {
		return nil, errors.New("targetVersion spec is nil")
	}

	version, err := resolveFromTemplate(spec.Version, &spec.SourceTemplate)
	if err != nil {
		return nil, fmt.Errorf("resolve version %s: %w", spec.Version, err)
	}

	return &ResolvedVersions{
		Versions:         []networkingv1alpha2.ProjectVersion{version},
		ProductionTarget: spec.Version, // Single version is always the production target
	}, nil
}

// resolveDeclarativeVersions resolves versions from a version name list and template.
func (*VersionManager) resolveDeclarativeVersions(spec *networkingv1alpha2.DeclarativeVersionsSpec) (*ResolvedVersions, error) {
	if spec == nil {
		return nil, errors.New("declarativeVersions spec is nil")
	}

	versions := make([]networkingv1alpha2.ProjectVersion, 0, len(spec.Versions))
	for _, vName := range spec.Versions {
		version, err := resolveFromTemplate(vName, &spec.SourceTemplate)
		if err != nil {
			return nil, fmt.Errorf("resolve version %s: %w", vName, err)
		}
		versions = append(versions, version)
	}

	return &ResolvedVersions{
		Versions:         versions,
		ProductionTarget: spec.ProductionTarget,
	}, nil
}

// resolveFullVersions returns full versions directly (already complete).
func (*VersionManager) resolveFullVersions(spec *networkingv1alpha2.FullVersionsSpec) (*ResolvedVersions, error) {
	if spec == nil {
		return nil, errors.New("fullVersions spec is nil")
	}

	return &ResolvedVersions{
		Versions:         spec.Versions,
		ProductionTarget: spec.ProductionTarget,
	}, nil
}

// HasVersions checks if the project has any versions configured.
func (*VersionManager) HasVersions(project *networkingv1alpha2.PagesProject) bool {
	if project.Spec.VersionManagement != nil {
		switch project.Spec.VersionManagement.Type {
		case networkingv1alpha2.TargetVersionMode:
			return project.Spec.VersionManagement.TargetVersion != nil
		case networkingv1alpha2.DeclarativeVersionsMode:
			return project.Spec.VersionManagement.DeclarativeVersions != nil &&
				len(project.Spec.VersionManagement.DeclarativeVersions.Versions) > 0
		case networkingv1alpha2.FullVersionsMode:
			return project.Spec.VersionManagement.FullVersions != nil &&
				len(project.Spec.VersionManagement.FullVersions.Versions) > 0
		}
	}
	// Legacy mode - intentionally using deprecated field for backward compatibility
	//nolint:staticcheck
	return len(project.Spec.Versions) > 0
}

// Reconcile synchronizes the desired versions with actual PagesDeployment resources.
func (vm *VersionManager) Reconcile(ctx context.Context, project *networkingv1alpha2.PagesProject) error {
	// 1. Resolve versions based on configuration mode
	resolved, err := vm.ResolveVersions(project)
	if err != nil {
		return fmt.Errorf("resolve versions: %w", err)
	}

	// 2. Get existing managed deployments
	existingDeployments, err := vm.listManagedDeployments(ctx, project)
	if err != nil {
		return fmt.Errorf("list managed deployments: %w", err)
	}

	// 3. Build desired and existing maps
	desired := vm.buildDesiredMapFromResolved(resolved.Versions)
	existingMap := vm.buildExistingMap(existingDeployments)

	// 4. Reconcile deployments
	if err := vm.reconcileDeployments(ctx, project, desired, existingMap); err != nil {
		return err
	}

	// Note: We don't delete deployments here for versions removed from spec.versions.
	// That's handled by the pruner based on revisionHistoryLimit.
	// This allows users to keep historical deployments even after removing from spec.

	return nil
}

// buildDesiredMapFromResolved builds a map of desired versions from resolved versions.
func (*VersionManager) buildDesiredMapFromResolved(versions []networkingv1alpha2.ProjectVersion) map[string]networkingv1alpha2.ProjectVersion {
	desired := make(map[string]networkingv1alpha2.ProjectVersion)
	for _, v := range versions {
		desired[v.Name] = v
	}
	return desired
}

// buildExistingMap builds a map of existing deployments.
func (*VersionManager) buildExistingMap(deployments []networkingv1alpha2.PagesDeployment) map[string]*networkingv1alpha2.PagesDeployment {
	existingMap := make(map[string]*networkingv1alpha2.PagesDeployment)
	for i := range deployments {
		dep := &deployments[i]
		versionName := dep.Labels[VersionLabel]
		existingMap[versionName] = dep
	}
	return existingMap
}

// reconcileDeployments creates or updates deployments as needed.
func (vm *VersionManager) reconcileDeployments(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	desired map[string]networkingv1alpha2.ProjectVersion,
	existing map[string]*networkingv1alpha2.PagesDeployment,
) error {
	for versionName, version := range desired {
		if err := vm.reconcileVersion(ctx, project, versionName, version, existing); err != nil {
			return err
		}
	}
	return nil
}

// reconcileVersion reconciles a single version.
func (vm *VersionManager) reconcileVersion(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	versionName string,
	version networkingv1alpha2.ProjectVersion,
	existing map[string]*networkingv1alpha2.PagesDeployment,
) error {
	existingDep, exists := existing[versionName]
	if !exists {
		if err := vm.createDeployment(ctx, project, version); err != nil {
			return fmt.Errorf("create deployment for version %s: %w", versionName, err)
		}
		return nil
	}

	// Check if deployment needs update
	// PagesDeployment is immutable, so we need to recreate if changed
	if vm.needsUpdate(version, existingDep) {
		vm.log.Info("Deployment source changed, recreating", "version", versionName)
		if err := vm.recreateDeployment(ctx, project, version, existingDep); err != nil {
			return fmt.Errorf("recreate deployment for version %s: %w", versionName, err)
		}
	}
	return nil
}

// listManagedDeployments lists all PagesDeployment resources managed by this PagesProject.
func (vm *VersionManager) listManagedDeployments(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
) ([]networkingv1alpha2.PagesDeployment, error) {
	list := &networkingv1alpha2.PagesDeploymentList{}

	labelSelector := client.MatchingLabels{
		ManagedByLabel:     ManagedByValue,
		ManagedByNameLabel: project.Name,
		ManagedByUIDLabel:  string(project.UID),
	}

	if err := vm.List(ctx, list, client.InNamespace(project.Namespace), labelSelector); err != nil {
		return nil, err
	}

	return list.Items, nil
}

// createDeployment creates a new PagesDeployment for a version.
func (vm *VersionManager) createDeployment(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	version networkingv1alpha2.ProjectVersion,
) error {
	deployment := &networkingv1alpha2.PagesDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", project.Name, version.Name),
			Namespace: project.Namespace,
			Labels: map[string]string{
				ManagedByLabel:     ManagedByValue,
				ManagedByNameLabel: project.Name,
				ManagedByUIDLabel:  string(project.UID),
				VersionLabel:       version.Name,
			},
			Annotations: map[string]string{
				ManagedAnnotation: "true",
			},
		},
		Spec: networkingv1alpha2.PagesDeploymentSpec{
			ProjectRef: networkingv1alpha2.PagesProjectRef{
				Name: project.Name,
			},
			Environment: networkingv1alpha2.PagesDeploymentEnvironmentPreview, // Default to preview
			Source: &networkingv1alpha2.PagesDeploymentSourceSpec{
				Type:         networkingv1alpha2.PagesDeploymentSourceTypeDirectUpload,
				DirectUpload: version.Source,
			},
			Cloudflare: project.Spec.Cloudflare,
		},
	}

	// Set owner reference for cascade deletion
	if err := ctrl.SetControllerReference(project, deployment, vm.Scheme); err != nil {
		return fmt.Errorf("set owner reference: %w", err)
	}

	if err := vm.Create(ctx, deployment); err != nil {
		return fmt.Errorf("create deployment: %w", err)
	}

	vm.log.Info("Created managed deployment", "version", version.Name, "deployment", deployment.Name)
	return nil
}

// needsUpdate checks if a deployment needs to be recreated due to source changes.
func (*VersionManager) needsUpdate(version networkingv1alpha2.ProjectVersion, existing *networkingv1alpha2.PagesDeployment) bool {
	// Compare source configuration
	if !reflect.DeepEqual(version.Source, existing.Spec.Source.DirectUpload) {
		return true
	}

	// If we add more fields to ProjectVersion in the future, compare them here

	return false
}

// recreateDeployment recreates a deployment by deleting the old one and creating a new one.
func (vm *VersionManager) recreateDeployment(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	version networkingv1alpha2.ProjectVersion,
	existing *networkingv1alpha2.PagesDeployment,
) error {
	// Delete the old deployment
	if err := vm.Delete(ctx, existing); err != nil {
		return fmt.Errorf("delete old deployment: %w", err)
	}

	vm.log.Info("Deleted old deployment for recreation", "version", version.Name, "deployment", existing.Name)

	// Create the new deployment
	// Note: The deletion may not be complete immediately, but Kubernetes will handle it
	// The new deployment will be created with the same name once the old one is fully deleted
	return vm.createDeployment(ctx, project, version)
}
