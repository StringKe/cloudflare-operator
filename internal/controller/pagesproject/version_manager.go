// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesproject

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// ErrDeploymentPendingDeletion is returned when a deployment is being deleted
// and we need to wait for it to complete before creating a new one.
var ErrDeploymentPendingDeletion = errors.New("deployment pending deletion, will retry")

// ResolvedVersions contains the resolved versions and production target.
type ResolvedVersions struct {
	Versions         []networkingv1alpha2.ProjectVersion
	ProductionTarget string
	// PreviewVersion is the version to deploy as preview (GitOps mode)
	PreviewVersion string
	// ProductionVersion is the version to deploy as production (GitOps mode)
	ProductionVersion string
	// Policy is the active version management policy
	Policy networkingv1alpha2.VersionPolicy
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

// getEffectivePolicy returns the effective version management policy.
// It handles backward compatibility with the deprecated Type field.
func getEffectivePolicy(mgmt *networkingv1alpha2.VersionManagement) networkingv1alpha2.VersionPolicy {
	if mgmt == nil {
		return networkingv1alpha2.VersionPolicyNone
	}

	if mgmt.Policy != "" {
		return mgmt.Policy
	}
	return networkingv1alpha2.VersionPolicyNone
}

// ResolveVersions resolves versions based on the version management configuration.
// Supports 8 modes: none, targetVersion, declarativeVersions, fullVersions, gitops, latestPreview, autoPromote, external.
func (vm *VersionManager) ResolveVersions(project *networkingv1alpha2.PagesProject) (*ResolvedVersions, error) {
	mgmt := project.Spec.VersionManagement

	policy := getEffectivePolicy(mgmt)

	// Get production branch for setting correct branch in metadata
	// This is critical for Cloudflare Rollback API compatibility
	productionBranch := project.Spec.ProductionBranch
	if productionBranch == "" {
		productionBranch = "main" // Default fallback
	}

	switch policy {
	case networkingv1alpha2.VersionPolicyNone, "":
		return &ResolvedVersions{
			Policy: networkingv1alpha2.VersionPolicyNone,
		}, nil

	case networkingv1alpha2.VersionPolicyTargetVersion:
		result, err := vm.resolveTargetVersion(mgmt.TargetVersion, productionBranch)
		if err != nil {
			return nil, err
		}
		result.Policy = policy
		return result, nil

	case networkingv1alpha2.VersionPolicyDeclarativeVersions:
		result, err := vm.resolveDeclarativeVersions(mgmt.DeclarativeVersions, productionBranch)
		if err != nil {
			return nil, err
		}
		result.Policy = policy
		return result, nil

	case networkingv1alpha2.VersionPolicyFullVersions:
		result, err := vm.resolveFullVersions(mgmt.FullVersions, productionBranch)
		if err != nil {
			return nil, err
		}
		result.Policy = policy
		return result, nil

	case networkingv1alpha2.VersionPolicyGitOps:
		return vm.resolveGitOps(mgmt.GitOps, productionBranch)

	case networkingv1alpha2.VersionPolicyLatestPreview:
		return vm.resolveLatestPreview(mgmt.LatestPreview)

	case networkingv1alpha2.VersionPolicyAutoPromote:
		return vm.resolveAutoPromote(mgmt.AutoPromote)

	case networkingv1alpha2.VersionPolicyExternal:
		return vm.resolveExternal(mgmt.External, productionBranch)

	case networkingv1alpha2.VersionPolicyGitOpsLatest:
		return vm.resolveGitOpsLatest(mgmt.GitOpsLatest, productionBranch)

	default:
		return nil, fmt.Errorf("unknown version management policy: %s", policy)
	}
}

// resolveTargetVersion resolves a single target version using the source template.
func (*VersionManager) resolveTargetVersion(spec *networkingv1alpha2.TargetVersionSpec, productionBranch string) (*ResolvedVersions, error) {
	if spec == nil {
		return nil, errors.New("targetVersion spec is nil")
	}

	// Ensure branch is set in metadata for production promotion compatibility
	metadata := spec.Metadata
	if metadata == nil {
		metadata = make(map[string]string)
	}
	if metadata["branch"] == "" && productionBranch != "" {
		metadata["branch"] = productionBranch
	}

	version, err := resolveFromTemplate(spec.Version, &spec.SourceTemplate, metadata)
	if err != nil {
		return nil, fmt.Errorf("resolve version %s: %w", spec.Version, err)
	}

	return &ResolvedVersions{
		Versions:         []networkingv1alpha2.ProjectVersion{version},
		ProductionTarget: spec.Version, // Single version is always the production target
	}, nil
}

// resolveDeclarativeVersions resolves versions from a version name list and template.
func (*VersionManager) resolveDeclarativeVersions(
	spec *networkingv1alpha2.DeclarativeVersionsSpec,
	productionBranch string,
) (*ResolvedVersions, error) {
	if spec == nil {
		return nil, errors.New("declarativeVersions spec is nil")
	}

	// Ensure branch is set in metadata for production promotion compatibility
	metadata := spec.Metadata
	if metadata == nil {
		metadata = make(map[string]string)
	}
	if metadata["branch"] == "" && productionBranch != "" {
		metadata["branch"] = productionBranch
	}

	versions := make([]networkingv1alpha2.ProjectVersion, 0, len(spec.Versions))
	for _, vName := range spec.Versions {
		version, err := resolveFromTemplate(vName, &spec.SourceTemplate, metadata)
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
// Ensures branch is set in metadata for production promotion compatibility.
func (*VersionManager) resolveFullVersions(spec *networkingv1alpha2.FullVersionsSpec, productionBranch string) (*ResolvedVersions, error) {
	if spec == nil {
		return nil, errors.New("fullVersions spec is nil")
	}

	// Deep copy versions and ensure branch is set
	versions := make([]networkingv1alpha2.ProjectVersion, 0, len(spec.Versions))
	for _, v := range spec.Versions {
		version := *v.DeepCopy()

		// Ensure branch is set in metadata for production promotion compatibility
		if version.Metadata == nil {
			version.Metadata = make(map[string]string)
		}
		if version.Metadata["branch"] == "" && productionBranch != "" {
			version.Metadata["branch"] = productionBranch
		}

		versions = append(versions, version)
	}

	return &ResolvedVersions{
		Versions:         versions,
		ProductionTarget: spec.ProductionTarget,
	}, nil
}

// resolveGitOps resolves versions for GitOps workflow (preview + production two-stage).
//
//nolint:revive // cognitive complexity acceptable for GitOps resolution
func (vm *VersionManager) resolveGitOps(spec *networkingv1alpha2.GitOpsVersionConfig, productionBranch string) (*ResolvedVersions, error) {
	if spec == nil {
		return &ResolvedVersions{
			Policy: networkingv1alpha2.VersionPolicyGitOps,
		}, nil
	}

	result := &ResolvedVersions{
		Policy:            networkingv1alpha2.VersionPolicyGitOps,
		PreviewVersion:    spec.PreviewVersion,
		ProductionVersion: spec.ProductionVersion,
	}

	// Build versions list from preview and production versions
	versions := make([]networkingv1alpha2.ProjectVersion, 0, 2)

	if spec.PreviewVersion != "" {
		version, err := vm.buildVersionFromGitOps(
			spec.PreviewVersion,
			spec.SourceTemplate,
			spec.PreviewMetadata,
			productionBranch,
		)
		if err != nil {
			return nil, fmt.Errorf("resolve preview version %s: %w", spec.PreviewVersion, err)
		}
		versions = append(versions, version)
	}

	// Only add production version if different from preview
	if spec.ProductionVersion != "" && spec.ProductionVersion != spec.PreviewVersion {
		version, err := vm.buildVersionFromGitOps(
			spec.ProductionVersion,
			spec.SourceTemplate,
			spec.ProductionMetadata,
			productionBranch,
		)
		if err != nil {
			return nil, fmt.Errorf("resolve production version %s: %w", spec.ProductionVersion, err)
		}
		versions = append(versions, version)
	}

	result.Versions = versions
	return result, nil
}

// buildVersionFromGitOps builds a ProjectVersion from GitOps config.
func (*VersionManager) buildVersionFromGitOps(
	versionName string,
	template *networkingv1alpha2.SourceTemplate,
	metadata map[string]string,
	productionBranch string,
) (networkingv1alpha2.ProjectVersion, error) {
	// Ensure branch is set in metadata for production promotion compatibility
	if metadata == nil {
		metadata = make(map[string]string)
	}
	if metadata["branch"] == "" && productionBranch != "" {
		metadata["branch"] = productionBranch
	}

	if template == nil {
		return networkingv1alpha2.ProjectVersion{
			Name:     versionName,
			Metadata: metadata,
		}, nil
	}

	return resolveFromTemplate(versionName, template, metadata)
}

// resolveLatestPreview resolves versions for latestPreview mode.
func (*VersionManager) resolveLatestPreview(_ *networkingv1alpha2.LatestPreviewConfig) (*ResolvedVersions, error) {
	// latestPreview mode tracks existing PagesDeployment resources
	// It doesn't create new versions, but watches for changes
	return &ResolvedVersions{
		Policy: networkingv1alpha2.VersionPolicyLatestPreview,
	}, nil
}

// resolveAutoPromote resolves versions for autoPromote mode.
func (*VersionManager) resolveAutoPromote(_ *networkingv1alpha2.AutoPromoteConfig) (*ResolvedVersions, error) {
	// autoPromote mode automatically promotes successful preview deployments
	// Similar to latestPreview but with automatic promotion
	return &ResolvedVersions{
		Policy: networkingv1alpha2.VersionPolicyAutoPromote,
	}, nil
}

// resolveExternal resolves versions for external mode.
//
//nolint:revive // cognitive complexity acceptable for external version resolution with SourceTemplate support
func (*VersionManager) resolveExternal(
	spec *networkingv1alpha2.ExternalVersionConfig,
	productionBranch string,
) (*ResolvedVersions, error) {
	if spec == nil {
		return &ResolvedVersions{
			Policy: networkingv1alpha2.VersionPolicyExternal,
		}, nil
	}

	result := &ResolvedVersions{
		Policy: networkingv1alpha2.VersionPolicyExternal,
	}

	// Build versions from external config
	if spec.CurrentVersion != "" {
		// Ensure branch is set in metadata for production promotion compatibility
		metadata := spec.Metadata
		if metadata == nil {
			metadata = make(map[string]string)
		}
		if metadata["branch"] == "" && productionBranch != "" {
			metadata["branch"] = productionBranch
		}

		var version networkingv1alpha2.ProjectVersion

		// If SourceTemplate is provided, use it to build complete Source
		if spec.SourceTemplate != nil {
			var err error
			version, err = resolveFromTemplate(spec.CurrentVersion, spec.SourceTemplate, metadata)
			if err != nil {
				return nil, fmt.Errorf("resolve current version: %w", err)
			}
		} else {
			// No template, create minimal version with metadata
			version = networkingv1alpha2.ProjectVersion{
				Name:     spec.CurrentVersion,
				Metadata: metadata,
			}
		}

		result.Versions = append(result.Versions, version)
	}

	if spec.ProductionVersion != "" {
		result.ProductionTarget = spec.ProductionVersion
		result.ProductionVersion = spec.ProductionVersion
	}

	return result, nil
}

// resolveGitOpsLatest resolves versions for gitopsLatest mode.
// Key difference from targetVersion: ProductionTarget is NOT set,
// so reconcileProductionTarget() will skip production switching.
// This allows CF console to fully manage production version switching.
func (*VersionManager) resolveGitOpsLatest(
	spec *networkingv1alpha2.GitOpsLatestConfig,
	productionBranch string,
) (*ResolvedVersions, error) {
	if spec == nil {
		return &ResolvedVersions{
			Policy: networkingv1alpha2.VersionPolicyGitOpsLatest,
		}, nil
	}

	result := &ResolvedVersions{
		Policy: networkingv1alpha2.VersionPolicyGitOpsLatest,
		// ProductionTarget intentionally left empty!
		// This prevents automatic production switching.
	}

	if spec.Version == "" {
		return result, nil
	}

	// Ensure branch is set in metadata
	metadata := spec.Metadata
	if metadata == nil {
		metadata = make(map[string]string)
	}
	if metadata["branch"] == "" && productionBranch != "" {
		metadata["branch"] = productionBranch
	}

	version, err := resolveFromTemplate(spec.Version, &spec.SourceTemplate, metadata)
	if err != nil {
		return nil, fmt.Errorf("resolve version %s: %w", spec.Version, err)
	}

	result.Versions = []networkingv1alpha2.ProjectVersion{version}
	return result, nil
}

// HasVersions checks if the project has any versions configured.
func (*VersionManager) HasVersions(project *networkingv1alpha2.PagesProject) bool {
	mgmt := project.Spec.VersionManagement
	if mgmt == nil {
		return false
	}

	policy := getEffectivePolicy(mgmt)

	switch policy {
	case networkingv1alpha2.VersionPolicyNone, "":
		return false

	case networkingv1alpha2.VersionPolicyTargetVersion:
		return mgmt.TargetVersion != nil

	case networkingv1alpha2.VersionPolicyDeclarativeVersions:
		return mgmt.DeclarativeVersions != nil &&
			len(mgmt.DeclarativeVersions.Versions) > 0

	case networkingv1alpha2.VersionPolicyFullVersions:
		return mgmt.FullVersions != nil &&
			len(mgmt.FullVersions.Versions) > 0

	case networkingv1alpha2.VersionPolicyGitOps:
		return mgmt.GitOps != nil &&
			(mgmt.GitOps.PreviewVersion != "" || mgmt.GitOps.ProductionVersion != "")

	case networkingv1alpha2.VersionPolicyLatestPreview:
		// latestPreview tracks existing deployments, always active if configured
		return mgmt.LatestPreview != nil

	case networkingv1alpha2.VersionPolicyAutoPromote:
		// autoPromote tracks existing deployments, always active if configured
		return mgmt.AutoPromote != nil

	case networkingv1alpha2.VersionPolicyExternal:
		return mgmt.External != nil &&
			(mgmt.External.CurrentVersion != "" || mgmt.External.ProductionVersion != "")

	case networkingv1alpha2.VersionPolicyGitOpsLatest:
		return mgmt.GitOpsLatest != nil && mgmt.GitOpsLatest.Version != ""
	}

	return false
}

// IsGitOpsMode checks if the project is using GitOps version management.
func (*VersionManager) IsGitOpsMode(project *networkingv1alpha2.PagesProject) bool {
	if project.Spec.VersionManagement == nil {
		return false
	}
	return getEffectivePolicy(project.Spec.VersionManagement) == networkingv1alpha2.VersionPolicyGitOps
}

// GetPolicy returns the effective version management policy.
func (*VersionManager) GetPolicy(project *networkingv1alpha2.PagesProject) networkingv1alpha2.VersionPolicy {
	return getEffectivePolicy(project.Spec.VersionManagement)
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

	// Check if deployment is being deleted - wait for it to complete
	if existingDep.DeletionTimestamp != nil {
		vm.log.Info("Deployment is being deleted, waiting", "version", versionName)
		return ErrDeploymentPendingDeletion
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
	// Build direct upload source with deployment metadata
	directUploadSource := vm.buildDirectUploadSource(version)

	// Determine environment based on policy
	env := vm.determineDeploymentEnvironment(project)

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
			Environment: env,
			Source: &networkingv1alpha2.PagesDeploymentSourceSpec{
				Type:         networkingv1alpha2.PagesDeploymentSourceTypeDirectUpload,
				DirectUpload: directUploadSource,
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

// determineDeploymentEnvironment determines the deployment environment based on policy.
func (*VersionManager) determineDeploymentEnvironment(
	project *networkingv1alpha2.PagesProject,
) networkingv1alpha2.PagesDeploymentEnvironment {
	// Default to preview
	env := networkingv1alpha2.PagesDeploymentEnvironmentPreview

	if project.Spec.VersionManagement == nil {
		return env
	}

	// For gitopsLatest, use the configured environment
	if project.Spec.VersionManagement.GitOpsLatest != nil {
		if project.Spec.VersionManagement.GitOpsLatest.Environment == "preview" {
			return networkingv1alpha2.PagesDeploymentEnvironmentPreview
		}
		// Default is production per spec (includes "production" and empty string)
		return networkingv1alpha2.PagesDeploymentEnvironmentProduction
	}

	return env
}

// buildDirectUploadSource builds a direct upload source from a ProjectVersion,
// extracting deployment metadata from the version's Metadata map.
//
//nolint:revive // cognitive complexity acceptable for metadata extraction with multiple optional fields
func (*VersionManager) buildDirectUploadSource(version networkingv1alpha2.ProjectVersion) *networkingv1alpha2.PagesDirectUploadSourceSpec {
	if version.Source == nil {
		return nil
	}

	// Start with the existing source configuration
	result := &networkingv1alpha2.PagesDirectUploadSourceSpec{
		Source:   version.Source.Source,
		Checksum: version.Source.Checksum,
		Archive:  version.Source.Archive,
		Branch:   version.Source.Branch,
	}

	// Extract deployment metadata from version.Metadata map
	if len(version.Metadata) > 0 {
		metadata := &networkingv1alpha2.DeploymentTriggerMetadata{}
		hasMetadata := false

		if commitHash, ok := version.Metadata["commitHash"]; ok && commitHash != "" {
			metadata.CommitHash = commitHash
			hasMetadata = true
		}
		if commitMessage, ok := version.Metadata["commitMessage"]; ok && commitMessage != "" {
			metadata.CommitMessage = commitMessage
			hasMetadata = true
		}
		if commitDirty, ok := version.Metadata["commitDirty"]; ok {
			dirty := commitDirty == "true"
			metadata.CommitDirty = &dirty
			hasMetadata = true
		}
		// Also allow branch to be specified in metadata
		if branch, ok := version.Metadata["branch"]; ok && branch != "" {
			metadata.Branch = branch
			hasMetadata = true
		}

		if hasMetadata {
			result.DeploymentMetadata = metadata
		}
	}

	// Merge with existing DeploymentMetadata if present in source
	if version.Source.DeploymentMetadata != nil {
		if result.DeploymentMetadata == nil {
			result.DeploymentMetadata = version.Source.DeploymentMetadata
		} else {
			// Source.DeploymentMetadata takes precedence
			dm := version.Source.DeploymentMetadata
			if dm.Branch != "" {
				result.DeploymentMetadata.Branch = dm.Branch
			}
			if dm.CommitHash != "" {
				result.DeploymentMetadata.CommitHash = dm.CommitHash
			}
			if dm.CommitMessage != "" {
				result.DeploymentMetadata.CommitMessage = dm.CommitMessage
			}
			if dm.CommitDirty != nil {
				result.DeploymentMetadata.CommitDirty = dm.CommitDirty
			}
		}
	}

	return result
}

// needsUpdate checks if a deployment needs to be recreated due to source changes.
// Uses semantic comparison to avoid false positives from DeploymentMetadata differences.
func (vm *VersionManager) needsUpdate(version networkingv1alpha2.ProjectVersion, existing *networkingv1alpha2.PagesDeployment) bool {
	// Build the expected source from version
	expectedSource := vm.buildDirectUploadSource(version)

	// Get actual source from existing deployment
	if existing.Spec.Source == nil || existing.Spec.Source.DirectUpload == nil {
		// If no existing source, needs update if we have a source
		return expectedSource != nil
	}
	actualSource := existing.Spec.Source.DirectUpload

	// Use semantic comparison that ignores DeploymentMetadata
	return !sourceSpecsEqual(expectedSource, actualSource)
}

// recreateDeployment recreates a deployment by deleting the old one and creating a new one.
// Returns ErrDeploymentPendingDeletion to trigger requeue when deletion is in progress.
func (vm *VersionManager) recreateDeployment(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	version networkingv1alpha2.ProjectVersion,
	existing *networkingv1alpha2.PagesDeployment,
) error {
	// Check if already being deleted
	if existing.DeletionTimestamp != nil {
		vm.log.Info("Deployment already being deleted, waiting", "version", version.Name, "deployment", existing.Name)
		return ErrDeploymentPendingDeletion
	}

	// Delete the old deployment
	if err := vm.Delete(ctx, existing); err != nil {
		if apierrors.IsNotFound(err) {
			// Already deleted, create new deployment
			vm.log.Info("Old deployment already deleted, creating new one", "version", version.Name)
			return vm.createDeployment(ctx, project, version)
		}
		return fmt.Errorf("delete old deployment: %w", err)
	}

	vm.log.Info("Triggered deletion of old deployment, will recreate on next reconcile",
		"version", version.Name, "deployment", existing.Name)

	// Return sentinel error to trigger requeue
	// The new deployment will be created on the next reconcile cycle after deletion completes
	return ErrDeploymentPendingDeletion
}

// =============================================================================
// Source Semantic Comparison Functions
// =============================================================================

// sourceSpecsEqual compares two PagesDirectUploadSourceSpec for semantic equality.
// It only compares fields that affect the deployment content (Source, Archive, Checksum, Branch),
// ignoring DeploymentMetadata which is dynamically generated.
//
//nolint:revive // cognitive complexity acceptable for semantic comparison
func sourceSpecsEqual(a, b *networkingv1alpha2.PagesDirectUploadSourceSpec) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Compare all content-affecting fields
	// NOTE: DeploymentMetadata is intentionally NOT compared (generated from version.Metadata)
	return directUploadSourceEqual(a.Source, b.Source) &&
		archiveConfigEqual(a.Archive, b.Archive) &&
		checksumConfigEqual(a.Checksum, b.Checksum) &&
		a.Branch == b.Branch
}

// directUploadSourceEqual compares two DirectUploadSource for equality.
func directUploadSourceEqual(a, b *networkingv1alpha2.DirectUploadSource) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Compare HTTP source
	if !httpSourceEqual(a.HTTP, b.HTTP) {
		return false
	}

	// Compare S3 source
	if !s3SourceEqual(a.S3, b.S3) {
		return false
	}

	// Compare OCI source
	if !ociSourceEqual(a.OCI, b.OCI) {
		return false
	}

	return true
}

// httpSourceEqual compares two HTTPSource for equality.
//
//nolint:revive // cyclomatic complexity acceptable for comprehensive comparison
func httpSourceEqual(a, b *networkingv1alpha2.HTTPSource) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	return a.URL == b.URL &&
		headersMapEqual(a.Headers, b.Headers) &&
		localObjectRefEqual(a.HeadersSecretRef, b.HeadersSecretRef) &&
		a.InsecureSkipVerify == b.InsecureSkipVerify
}

// headersMapEqual compares two headers maps for equality.
func headersMapEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// s3SourceEqual compares two S3Source for equality.
func s3SourceEqual(a, b *networkingv1alpha2.S3Source) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	return a.Bucket == b.Bucket &&
		a.Key == b.Key &&
		a.Region == b.Region &&
		a.Endpoint == b.Endpoint &&
		a.UsePathStyle == b.UsePathStyle &&
		localObjectRefEqual(a.CredentialsSecretRef, b.CredentialsSecretRef)
}

// ociSourceEqual compares two OCISource for equality.
func ociSourceEqual(a, b *networkingv1alpha2.OCISource) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	return a.Image == b.Image &&
		a.InsecureRegistry == b.InsecureRegistry &&
		localObjectRefEqual(a.CredentialsSecretRef, b.CredentialsSecretRef)
}

// archiveConfigEqual compares two ArchiveConfig for equality.
func archiveConfigEqual(a, b *networkingv1alpha2.ArchiveConfig) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	return a.Type == b.Type &&
		a.StripComponents == b.StripComponents &&
		a.SubPath == b.SubPath
}

// checksumConfigEqual compares two ChecksumConfig for equality.
func checksumConfigEqual(a, b *networkingv1alpha2.ChecksumConfig) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	return a.Algorithm == b.Algorithm && a.Value == b.Value
}

// localObjectRefEqual compares two LocalObjectReference for equality.
func localObjectRefEqual(a, b *corev1.LocalObjectReference) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	return a.Name == b.Name
}
