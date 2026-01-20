// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesproject

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

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

// Reconcile synchronizes the desired versions with actual PagesDeployment resources.
func (vm *VersionManager) Reconcile(ctx context.Context, project *networkingv1alpha2.PagesProject) error {
	// 1. Get existing managed deployments
	existingDeployments, err := vm.listManagedDeployments(ctx, project)
	if err != nil {
		return fmt.Errorf("list managed deployments: %w", err)
	}

	// 2. Build desired and existing maps
	desired := vm.buildDesiredMap(project)
	existingMap := vm.buildExistingMap(existingDeployments)

	// 3. Reconcile deployments
	if err := vm.reconcileDeployments(ctx, project, desired, existingMap); err != nil {
		return err
	}

	// Note: We don't delete deployments here for versions removed from spec.versions.
	// That's handled by the pruner based on revisionHistoryLimit.
	// This allows users to keep historical deployments even after removing from spec.

	return nil
}

// buildDesiredMap builds a map of desired versions.
func (*VersionManager) buildDesiredMap(project *networkingv1alpha2.PagesProject) map[string]networkingv1alpha2.ProjectVersion {
	desired := make(map[string]networkingv1alpha2.ProjectVersion)
	for _, v := range project.Spec.Versions {
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
