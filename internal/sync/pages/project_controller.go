// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package pages provides the Pages Sync Controllers for managing Cloudflare Pages resources.
// Each controller handles a specific resource type: Project, Domain, and Deployment.
//
// Unified Sync Architecture Flow:
// K8s Resources → Resource Controllers → Core Services → SyncState CRD → Sync Controllers → Cloudflare API
//
// This Sync Controller is the SINGLE point that calls Cloudflare API for Pages projects.
package pages

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	pagessvc "github.com/StringKe/cloudflare-operator/internal/service/pages"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

const (
	// ProjectFinalizerName is the finalizer for Pages Project SyncState resources.
	ProjectFinalizerName = "pages-project.sync.cloudflare-operator.io/finalizer"
)

// ProjectSyncController is the Sync Controller for Pages Project Configuration.
// It watches CloudflareSyncState resources of type PagesProject,
// extracts the configuration, and syncs to Cloudflare API.
// This is the SINGLE point that calls Cloudflare Pages Project API.
type ProjectSyncController struct {
	*common.BaseSyncController
}

// NewProjectSyncController creates a new ProjectSyncController
func NewProjectSyncController(c client.Client) *ProjectSyncController {
	return &ProjectSyncController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for Pages project.
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *ProjectSyncController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "PagesProjectSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process PagesProject type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourcePagesProject {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing Pages Project SyncState",
		"cloudflareId", syncState.Spec.CloudflareID,
		"sources", len(syncState.Spec.Sources))

	// Handle deletion
	if !syncState.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, syncState)
	}

	// Check if there are any sources - if none, delete from Cloudflare
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, deleting from Cloudflare")
		return r.handleDeletion(ctx, syncState)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(syncState, ProjectFinalizerName) {
		if err := common.AddFinalizerWithRetry(ctx, r.Client, syncState, ProjectFinalizerName); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip if there's a pending debounced request
	if r.Debouncer.IsPending(req.Name) {
		logger.V(1).Info("Skipping reconcile - debounced request pending")
		return ctrl.Result{}, nil
	}

	// Extract Pages project configuration from first source
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract Pages project configuration")
		if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
	}

	// Compute hash for change detection
	newHash, hashErr := common.ComputeConfigHash(config)
	if hashErr != nil {
		logger.Error(hashErr, "Failed to compute config hash")
		newHash = "" // Force sync if hash fails
	}

	if !r.ShouldSync(syncState, newHash) {
		logger.V(1).Info("Configuration unchanged, skipping sync",
			"hash", newHash)
		return ctrl.Result{}, nil
	}

	// Set syncing status
	if err := r.SetSyncStatus(ctx, syncState, v1alpha2.SyncStatusSyncing); err != nil {
		return ctrl.Result{}, err
	}

	// Sync to Cloudflare API
	if err := r.syncToCloudflare(ctx, syncState, config); err != nil {
		logger.Error(err, "Failed to sync Pages project to Cloudflare")
		if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
	}

	// Update success status
	syncResult := &common.SyncResult{
		ConfigHash: newHash,
	}
	if err := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusSynced, syncResult, nil); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Successfully synced Pages project to Cloudflare",
		"projectName", config.Name)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts Pages project configuration from SyncState sources.
func (*ProjectSyncController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*pagessvc.PagesProjectConfig, error) {
	return common.ExtractFirstSourceConfig[pagessvc.PagesProjectConfig](syncState)
}

// syncToCloudflare syncs the Pages project configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic with adoption
func (r *ProjectSyncController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *pagessvc.PagesProjectConfig,
) error {
	logger := log.FromContext(ctx)

	// Create API client using common helper
	apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
	if err != nil {
		return err
	}

	// Build create/update params using our cf.* wrapper types
	params := r.buildProjectParams(config)

	// Check if this is a new project (pending) or existing
	cloudflareID := syncState.Spec.CloudflareID

	if common.IsPendingID(cloudflareID) {
		// Handle adoption policy for new projects
		return r.handleNewProject(ctx, apiClient, syncState, config, params)
	}

	// Update existing project
	logger.Info("Updating Pages project",
		"projectName", cloudflareID)

	_, err = apiClient.UpdatePagesProject(ctx, cloudflareID, *params)
	if err != nil {
		// Check if project was deleted externally
		if common.HandleNotFoundOnUpdate(err) {
			// Project deleted externally, recreate it
			logger.Info("Pages project not found, recreating",
				"projectName", cloudflareID)

			result, err := apiClient.CreatePagesProject(ctx, *params)
			if err != nil {
				return fmt.Errorf("recreate Pages project: %w", err)
			}

			// Update SyncState with new project name
			if updateErr := common.UpdateCloudflareID(ctx, r.Client, syncState, result.Name); updateErr != nil {
				logger.Error(updateErr, "Failed to update CloudflareID after recreating")
			}
		} else {
			return fmt.Errorf("update Pages project: %w", err)
		}
	}

	logger.Info("Updated Pages project",
		"projectName", cloudflareID)

	return nil
}

// handleNewProject handles creation or adoption of a new project based on adoption policy.
//
//nolint:revive // cognitive complexity is acceptable for adoption logic
func (r *ProjectSyncController) handleNewProject(
	ctx context.Context,
	apiClient *cf.API,
	syncState *v1alpha2.CloudflareSyncState,
	config *pagessvc.PagesProjectConfig,
	params *cf.PagesProjectParams,
) error {
	logger := log.FromContext(ctx)

	// Get adoption policy
	adoptionPolicy := config.AdoptionPolicy
	if adoptionPolicy == "" {
		adoptionPolicy = v1alpha2.AdoptionPolicyMustNotExist
	}

	// Check if project already exists in Cloudflare
	existingProject, err := apiClient.GetPagesProject(ctx, config.Name)
	projectExists := err == nil && existingProject != nil

	// Handle not found as expected case, return other errors
	if err != nil && !cf.IsNotFoundError(err) {
		return fmt.Errorf("check existing project: %w", err)
	}

	// Apply adoption policy
	switch adoptionPolicy {
	case v1alpha2.AdoptionPolicyMustNotExist:
		if projectExists {
			return fmt.Errorf("project %q already exists in Cloudflare; set adoptionPolicy to IfExists or MustExist to adopt", config.Name)
		}

	case v1alpha2.AdoptionPolicyMustExist:
		if !projectExists {
			return fmt.Errorf("project %q does not exist in Cloudflare (adoptionPolicy: MustExist)", config.Name)
		}

	case v1alpha2.AdoptionPolicyIfExists:
		// Will adopt if exists, create if not
	}

	// Adopt existing project
	if projectExists {
		logger.Info("Adopting existing Cloudflare project",
			"projectName", config.Name,
			"adoptionPolicy", adoptionPolicy)

		// Store original configuration for reference
		if err := r.storeOriginalConfig(ctx, syncState, existingProject); err != nil {
			logger.Error(err, "Failed to store original config, continuing with adoption")
		}

		// Update project with K8s configuration
		_, err := apiClient.UpdatePagesProject(ctx, config.Name, *params)
		if err != nil {
			return fmt.Errorf("update adopted project: %w", err)
		}

		// Update SyncState with project name (must succeed)
		if err := common.UpdateCloudflareID(ctx, r.Client, syncState, config.Name); err != nil {
			return err
		}

		// Mark as adopted in annotations
		r.markAsAdopted(ctx, syncState)

		// Enable Web Analytics if configured
		if err := r.enableWebAnalyticsIfNeeded(ctx, apiClient, config, existingProject.Subdomain); err != nil {
			logger.Error(err, "Failed to enable Web Analytics for adopted project")
			// Non-fatal error, continue
		}

		logger.Info("Adopted Pages project",
			"projectName", config.Name,
			"subdomain", existingProject.Subdomain)

		return nil
	}

	// Create new project
	logger.Info("Creating new Pages project",
		"name", config.Name,
		"productionBranch", config.ProductionBranch)

	result, err := apiClient.CreatePagesProject(ctx, *params)
	if err != nil {
		return fmt.Errorf("create Pages project: %w", err)
	}

	// Update SyncState with actual project name (must succeed)
	if err := common.UpdateCloudflareID(ctx, r.Client, syncState, result.Name); err != nil {
		return err
	}

	// Enable Web Analytics if configured
	if err := r.enableWebAnalyticsIfNeeded(ctx, apiClient, config, result.Subdomain); err != nil {
		logger.Error(err, "Failed to enable Web Analytics for new project")
		// Non-fatal error, continue
	}

	logger.Info("Created Pages project",
		"projectName", result.Name,
		"subdomain", result.Subdomain)

	return nil
}

// enableWebAnalyticsIfNeeded enables Web Analytics for the Pages project if configured.
func (*ProjectSyncController) enableWebAnalyticsIfNeeded(
	ctx context.Context,
	apiClient *cf.API,
	config *pagessvc.PagesProjectConfig,
	subdomain string,
) error {
	logger := log.FromContext(ctx)

	// Check if Web Analytics should be enabled (default: true)
	enableWebAnalytics := config.EnableWebAnalytics == nil || *config.EnableWebAnalytics

	if !enableWebAnalytics {
		logger.V(1).Info("Web Analytics disabled for project", "projectName", config.Name)
		return nil
	}

	// The hostname for Pages is the subdomain + .pages.dev (e.g., "myproject.pages.dev")
	// Note: subdomain from Cloudflare API is just the short name without .pages.dev suffix
	hostname := fmt.Sprintf("%s.pages.dev", subdomain)
	if subdomain == "" {
		hostname = fmt.Sprintf("%s.pages.dev", config.Name)
	}

	// Check if Web Analytics is already enabled
	existingSite, err := apiClient.GetWebAnalyticsSite(ctx, hostname)
	if err != nil {
		logger.V(1).Info("Failed to check existing Web Analytics site", "error", err)
		// Continue to try enabling
	}

	if existingSite != nil {
		logger.V(1).Info("Web Analytics already enabled for project",
			"projectName", config.Name,
			"hostname", hostname,
			"siteTag", existingSite.SiteTag)
		return nil
	}

	// Enable Web Analytics
	site, err := apiClient.EnableWebAnalytics(ctx, hostname)
	if err != nil {
		return fmt.Errorf("enable Web Analytics for %s: %w", hostname, err)
	}

	logger.Info("Enabled Web Analytics for Pages project",
		"projectName", config.Name,
		"hostname", hostname,
		"siteTag", site.SiteTag,
		"siteToken", site.SiteToken)

	return nil
}

// storeOriginalConfig stores the original Cloudflare configuration before adoption.
func (r *ProjectSyncController) storeOriginalConfig(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	project *cf.PagesProjectResult,
) error {
	// Build original config
	originalConfig := map[string]interface{}{
		"productionBranch": project.ProductionBranch,
		"subdomain":        project.Subdomain,
		"capturedAt":       time.Now().Format(time.RFC3339),
	}

	if project.Source != nil {
		originalConfig["source"] = project.Source
	}
	if project.BuildConfig != nil {
		originalConfig["buildConfig"] = project.BuildConfig
	}

	configJSON, err := json.Marshal(originalConfig)
	if err != nil {
		return fmt.Errorf("marshal original config: %w", err)
	}

	if syncState.Annotations == nil {
		syncState.Annotations = make(map[string]string)
	}
	syncState.Annotations["cloudflare-operator.io/original-config"] = string(configJSON)

	return r.Client.Update(ctx, syncState)
}

// markAsAdopted marks the SyncState as adopted.
func (r *ProjectSyncController) markAsAdopted(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) {
	if syncState.Annotations == nil {
		syncState.Annotations = make(map[string]string)
	}
	syncState.Annotations["cloudflare-operator.io/adopted"] = "true"
	syncState.Annotations["cloudflare-operator.io/adopted-at"] = metav1.Now().Format(time.RFC3339)

	if err := r.Client.Update(ctx, syncState); err != nil {
		log.FromContext(ctx).Error(err, "Failed to mark SyncState as adopted")
	}
}

// buildProjectParams builds the params for create/update from config.
// Uses our cf.* wrapper types which get converted to cloudflare SDK types in the API client.
//
//nolint:revive // cognitive complexity is acceptable for building complex params
func (r *ProjectSyncController) buildProjectParams(config *pagessvc.PagesProjectConfig) *cf.PagesProjectParams {
	params := &cf.PagesProjectParams{
		Name:             config.Name,
		ProductionBranch: config.ProductionBranch,
	}

	// Build source config using our wrapper types
	if config.Source != nil {
		params.Source = &cf.PagesSourceConfig{
			Type: config.Source.Type,
		}
		if config.Source.GitHub != nil {
			params.Source.GitHub = &cf.PagesGitHubConfig{
				Owner:                        config.Source.GitHub.Owner,
				Repo:                         config.Source.GitHub.Repo,
				ProductionDeploymentsEnabled: config.Source.GitHub.ProductionDeploymentsEnabled,
				PreviewDeploymentsEnabled:    config.Source.GitHub.PreviewDeploymentsEnabled,
				PRCommentsEnabled:            config.Source.GitHub.PRCommentsEnabled,
				DeploymentsEnabled:           config.Source.GitHub.DeploymentsEnabled,
			}
		}
		if config.Source.GitLab != nil {
			params.Source.GitLab = &cf.PagesGitLabConfig{
				Owner:                        config.Source.GitLab.Owner,
				Repo:                         config.Source.GitLab.Repo,
				ProductionDeploymentsEnabled: config.Source.GitLab.ProductionDeploymentsEnabled,
				PreviewDeploymentsEnabled:    config.Source.GitLab.PreviewDeploymentsEnabled,
				DeploymentsEnabled:           config.Source.GitLab.DeploymentsEnabled,
			}
		}
	}

	// Build build config using our wrapper types
	if config.BuildConfig != nil {
		params.BuildConfig = &cf.PagesBuildConfig{
			BuildCommand:      config.BuildConfig.BuildCommand,
			DestinationDir:    config.BuildConfig.DestinationDir,
			RootDir:           config.BuildConfig.RootDir,
			BuildCaching:      config.BuildConfig.BuildCaching,
			WebAnalyticsTag:   config.BuildConfig.WebAnalyticsTag,
			WebAnalyticsToken: config.BuildConfig.WebAnalyticsToken,
		}
	}

	// Build deployment configs using our wrapper types
	if config.DeploymentConfigs != nil {
		params.DeploymentConfig = &cf.PagesDeploymentConfigs{}
		if config.DeploymentConfigs.Preview != nil {
			params.DeploymentConfig.Preview = r.buildDeploymentConfig(config.DeploymentConfigs.Preview)
		}
		if config.DeploymentConfigs.Production != nil {
			params.DeploymentConfig.Production = r.buildDeploymentConfig(config.DeploymentConfigs.Production)
		}
	}

	return params
}

// buildDeploymentConfig builds a PagesDeploymentEnvConfig from pagessvc config.
// Uses our cf.* wrapper types which get converted to cloudflare SDK types in the API client.
//
//nolint:revive // cognitive complexity is acceptable for building complex binding configs
func (*ProjectSyncController) buildDeploymentConfig(spec *pagessvc.PagesDeploymentEnvConfig) *cf.PagesDeploymentEnvConfig {
	env := &cf.PagesDeploymentEnvConfig{
		CompatibilityDate:  spec.CompatibilityDate,
		CompatibilityFlags: spec.CompatibilityFlags,
		UsageModel:         spec.UsageModel,
		FailOpen:           spec.FailOpen,
	}

	// Environment variables
	if len(spec.EnvironmentVariables) > 0 {
		env.EnvironmentVariables = make(map[string]cf.PagesEnvVar)
		for name, v := range spec.EnvironmentVariables {
			env.EnvironmentVariables[name] = cf.PagesEnvVar{
				Value: v.Value,
				Type:  v.Type,
			}
		}
	}

	// D1 bindings
	if len(spec.D1Bindings) > 0 {
		env.D1Bindings = make(map[string]string)
		for _, binding := range spec.D1Bindings {
			env.D1Bindings[binding.Name] = binding.ID
		}
	}

	// KV bindings
	if len(spec.KVBindings) > 0 {
		env.KVBindings = make(map[string]string)
		for _, binding := range spec.KVBindings {
			env.KVBindings[binding.Name] = binding.ID
		}
	}

	// R2 bindings
	if len(spec.R2Bindings) > 0 {
		env.R2Bindings = make(map[string]string)
		for _, binding := range spec.R2Bindings {
			env.R2Bindings[binding.Name] = binding.ID
		}
	}

	// Service bindings
	if len(spec.ServiceBindings) > 0 {
		env.ServiceBindings = make(map[string]cf.PagesServiceBindingConfig)
		for _, binding := range spec.ServiceBindings {
			env.ServiceBindings[binding.Name] = cf.PagesServiceBindingConfig{
				Service:     binding.Service,
				Environment: binding.Environment,
			}
		}
	}

	// Durable Object bindings
	if len(spec.DurableObjectBindings) > 0 {
		env.DurableObjectBindings = make(map[string]cf.PagesDurableObjectBindingConfig)
		for _, binding := range spec.DurableObjectBindings {
			env.DurableObjectBindings[binding.Name] = cf.PagesDurableObjectBindingConfig{
				ClassName:       binding.ClassName,
				ScriptName:      binding.ScriptName,
				EnvironmentName: binding.EnvironmentName,
			}
		}
	}

	// Queue bindings
	if len(spec.QueueBindings) > 0 {
		env.QueueBindings = make(map[string]string)
		for _, binding := range spec.QueueBindings {
			env.QueueBindings[binding.Name] = binding.ID
		}
	}

	// AI bindings - simple string list
	if len(spec.AIBindings) > 0 {
		env.AIBindings = append([]string(nil), spec.AIBindings...)
	}

	// Vectorize bindings
	if len(spec.VectorizeBindings) > 0 {
		env.VectorizeBindings = make(map[string]string)
		for _, binding := range spec.VectorizeBindings {
			env.VectorizeBindings[binding.Name] = binding.ID
		}
	}

	// Hyperdrive bindings
	if len(spec.HyperdriveBindings) > 0 {
		env.HyperdriveBindings = make(map[string]string)
		for _, binding := range spec.HyperdriveBindings {
			env.HyperdriveBindings[binding.Name] = binding.ID
		}
	}

	// mTLS certificates
	if len(spec.MTLSCertificates) > 0 {
		env.MTLSCertificates = make(map[string]string)
		for _, cert := range spec.MTLSCertificates {
			env.MTLSCertificates[cert.Name] = cert.ID
		}
	}

	// Browser binding - simple string
	if spec.BrowserBinding != "" {
		env.BrowserBinding = spec.BrowserBinding
	}

	// Placement mode - simple string
	if spec.PlacementMode != "" {
		env.PlacementMode = spec.PlacementMode
	}

	return env
}

// handleDeletion handles the deletion of Pages project from Cloudflare.
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps
func (r *ProjectSyncController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, ProjectFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Get the Cloudflare project name
	cloudflareID := syncState.Spec.CloudflareID

	// Skip if pending ID (project was never created)
	if common.IsPendingID(cloudflareID) {
		logger.Info("Skipping deletion - Pages project was never created",
			"cloudflareId", cloudflareID)
	} else if cloudflareID != "" {
		// Create API client
		apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
		if err != nil {
			logger.Error(err, "Failed to create API client for deletion")
			return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
		}

		logger.Info("Deleting Pages project from Cloudflare",
			"projectName", cloudflareID)

		if err := apiClient.DeletePagesProject(ctx, cloudflareID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete Pages project from Cloudflare")
				if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
					logger.Error(statusErr, "Failed to update error status")
				}
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}
			logger.Info("Pages project already deleted from Cloudflare")
		} else {
			logger.Info("Successfully deleted Pages project from Cloudflare",
				"projectName", cloudflareID)
		}
	}

	// Remove finalizer
	if err := common.RemoveFinalizerWithRetry(ctx, r.Client, syncState, ProjectFinalizerName); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	// If sources are empty (not a deletion timestamp trigger), delete the SyncState itself
	if syncState.DeletionTimestamp.IsZero() && len(syncState.Spec.Sources) == 0 {
		logger.Info("Deleting orphaned SyncState")
		if err := r.Client.Delete(ctx, syncState); err != nil {
			if client.IgnoreNotFound(err) != nil {
				logger.Error(err, "Failed to delete SyncState")
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProjectSyncController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("pages-project-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourcePagesProject)).
		Complete(r)
}
