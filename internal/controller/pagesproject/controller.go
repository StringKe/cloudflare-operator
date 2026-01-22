// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package pagesproject implements the L2 Controller for PagesProject CRD.
// It registers project configurations to the Core Service for sync.
package pagesproject

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/service"
	pagessvc "github.com/StringKe/cloudflare-operator/internal/service/pages"
)

// PagesProjectReconciler reconciles a PagesProject object
type PagesProjectReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Recorder       record.EventRecorder
	projectService *pagessvc.ProjectService
	versionManager *VersionManager
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=pagesprojects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=pagesprojects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=pagesprojects/finalizers,verbs=update
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=pagesdeployments,verbs=get;list;watch;create;update;patch;delete

//nolint:revive // cognitive complexity is acceptable for this reconcile loop
func (r *PagesProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the PagesProject instance
	project := &networkingv1alpha2.PagesProject{}
	if err := r.Get(ctx, req.NamespacedName, project); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Resolve credentials
	credInfo, err := r.resolveCredentials(ctx, project)
	if err != nil {
		logger.Error(err, "Failed to resolve credentials")
		return r.updateStatusError(ctx, project, err)
	}

	// Handle deletion
	if !project.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, project)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(project, FinalizerName) {
		controllerutil.AddFinalizer(project, FinalizerName)
		if err := r.Update(ctx, project); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Register Pages project configuration to SyncState
	result, err := r.registerPagesProject(ctx, project, credInfo)
	if err != nil {
		return result, err
	}

	// Handle declarative version management if versions are specified
	if r.versionManager.HasVersions(project) {
		// Reconcile versions - create/update PagesDeployment resources
		if err := r.versionManager.Reconcile(ctx, project); err != nil {
			logger.Error(err, "Failed to reconcile versions")
			controller.RecordErrorEventAndCondition(r.Recorder, project,
				&project.Status.Conditions, "VersionReconcileFailed", err)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}

		// Reconcile production target - promote/demote deployments
		if err := r.reconcileProductionTarget(ctx, project); err != nil {
			logger.Error(err, "Failed to reconcile production target")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, err
		}

		// Prune old versions based on revisionHistoryLimit
		if err := r.pruneOldVersions(ctx, project); err != nil {
			logger.Error(err, "Failed to prune old versions")
			// Pruning errors are non-fatal, continue
		}

		// Aggregate status from managed deployments
		if err := r.aggregateVersionStatus(ctx, project); err != nil {
			logger.Error(err, "Failed to aggregate version status")
			// Status aggregation errors are non-fatal, continue
		}
	}

	return result, err
}

// resolveCredentials resolves the credentials for the Pages project.
func (r *PagesProjectReconciler) resolveCredentials(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
) (*controller.CredentialsInfo, error) {
	return controller.ResolveCredentialsForService(
		ctx,
		r.Client,
		log.FromContext(ctx),
		project.Spec.Cloudflare,
		project.Namespace,
		project.Status.AccountID,
	)
}

// handleDeletion handles the deletion of a PagesProject.
func (r *PagesProjectReconciler) handleDeletion(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(project, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Check deletion policy
	if project.Spec.DeletionPolicy == "Orphan" {
		logger.Info("Orphan deletion policy, skipping Cloudflare deletion")
	} else {
		// Unregister from SyncState - this triggers Sync Controller to delete from Cloudflare
		source := service.Source{
			Kind:      "PagesProject",
			Namespace: project.Namespace,
			Name:      project.Name,
		}

		projectName := r.getProjectName(project)
		logger.Info("Unregistering Pages project from SyncState",
			"projectName", projectName,
			"source", fmt.Sprintf("%s/%s", project.Namespace, project.Name))

		if err := r.projectService.Unregister(ctx, projectName, source); err != nil {
			logger.Error(err, "Failed to unregister Pages project from SyncState")
			r.Recorder.Event(project, corev1.EventTypeWarning, "UnregisterFailed",
				fmt.Sprintf("Failed to unregister from SyncState: %s", err.Error()))
			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}

		r.Recorder.Event(project, corev1.EventTypeNormal, "Unregistered",
			"Unregistered from SyncState, Sync Controller will delete from Cloudflare")
	}

	// Remove finalizer with retry logic
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, project, func() {
		controllerutil.RemoveFinalizer(project, FinalizerName)
	}); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(project, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// registerPagesProject registers the Pages project configuration to SyncState.
func (r *PagesProjectReconciler) registerPagesProject(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	credInfo *controller.CredentialsInfo,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Create source reference
	source := service.Source{
		Kind:      "PagesProject",
		Namespace: project.Namespace,
		Name:      project.Name,
	}

	// Build Pages project configuration
	config := r.buildProjectConfig(project)
	projectName := r.getProjectName(project)

	// Register to SyncState
	opts := pagessvc.ProjectRegisterOptions{
		ProjectName:    projectName,
		AccountID:      credInfo.AccountID,
		Source:         source,
		Config:         config,
		CredentialsRef: credInfo.CredentialsRef,
	}

	if err := r.projectService.Register(ctx, opts); err != nil {
		logger.Error(err, "Failed to register Pages project configuration")
		r.Recorder.Event(project, corev1.EventTypeWarning, "RegisterFailed",
			fmt.Sprintf("Failed to register Pages project: %s", err.Error()))
		return r.updateStatusError(ctx, project, err)
	}

	r.Recorder.Event(project, corev1.EventTypeNormal, "Registered",
		fmt.Sprintf("Registered Pages Project '%s' configuration to SyncState", projectName))

	// Check if already synced
	syncStatus, err := r.projectService.GetSyncStatus(ctx, projectName)
	if err != nil {
		logger.Error(err, "Failed to get sync status")
		return r.updateStatusPending(ctx, project, credInfo.AccountID)
	}

	if syncStatus != nil && syncStatus.IsSynced {
		// Already synced, update status to Ready
		return r.updateStatusReady(ctx, project, credInfo.AccountID, syncStatus.Subdomain)
	}

	// Update status to Pending - actual sync happens via PagesSyncController
	return r.updateStatusPending(ctx, project, credInfo.AccountID)
}

// getProjectName returns the project name from spec or uses K8s resource name.
func (*PagesProjectReconciler) getProjectName(project *networkingv1alpha2.PagesProject) string {
	if project.Spec.Name != "" {
		return project.Spec.Name
	}
	return project.Name
}

// buildProjectConfig builds the Pages project configuration from the spec.
//
//nolint:revive // cognitive complexity is acceptable for building complex config
func (r *PagesProjectReconciler) buildProjectConfig(project *networkingv1alpha2.PagesProject) pagessvc.PagesProjectConfig {
	config := pagessvc.PagesProjectConfig{
		Name:             r.getProjectName(project),
		ProductionBranch: project.Spec.ProductionBranch,
	}

	// Convert source configuration
	if project.Spec.Source != nil {
		config.Source = &pagessvc.PagesSourceConfig{
			Type: string(project.Spec.Source.Type),
		}
		if project.Spec.Source.GitHub != nil {
			config.Source.GitHub = &pagessvc.PagesGitHubConfig{
				Owner:                        project.Spec.Source.GitHub.Owner,
				Repo:                         project.Spec.Source.GitHub.Repo,
				ProductionDeploymentsEnabled: project.Spec.Source.GitHub.ProductionDeploymentsEnabled,
				PreviewDeploymentsEnabled:    project.Spec.Source.GitHub.PreviewDeploymentsEnabled,
				PRCommentsEnabled:            project.Spec.Source.GitHub.PRCommentsEnabled,
				DeploymentsEnabled:           project.Spec.Source.GitHub.DeploymentsEnabled,
			}
		}
		if project.Spec.Source.GitLab != nil {
			config.Source.GitLab = &pagessvc.PagesGitLabConfig{
				Owner:                        project.Spec.Source.GitLab.Owner,
				Repo:                         project.Spec.Source.GitLab.Repo,
				ProductionDeploymentsEnabled: project.Spec.Source.GitLab.ProductionDeploymentsEnabled,
				PreviewDeploymentsEnabled:    project.Spec.Source.GitLab.PreviewDeploymentsEnabled,
				DeploymentsEnabled:           project.Spec.Source.GitLab.DeploymentsEnabled,
			}
		}
	}

	// Convert build configuration
	if project.Spec.BuildConfig != nil {
		config.BuildConfig = &pagessvc.PagesBuildConfig{
			BuildCommand:      project.Spec.BuildConfig.BuildCommand,
			DestinationDir:    project.Spec.BuildConfig.DestinationDir,
			RootDir:           project.Spec.BuildConfig.RootDir,
			BuildCaching:      project.Spec.BuildConfig.BuildCaching,
			WebAnalyticsTag:   project.Spec.BuildConfig.WebAnalyticsTag,
			WebAnalyticsToken: project.Spec.BuildConfig.WebAnalyticsToken,
		}
	}

	// Convert deployment configurations
	if project.Spec.DeploymentConfigs != nil {
		config.DeploymentConfigs = &pagessvc.PagesDeploymentConfigs{}
		if project.Spec.DeploymentConfigs.Preview != nil {
			config.DeploymentConfigs.Preview = r.convertDeploymentConfig(project.Spec.DeploymentConfigs.Preview)
		}
		if project.Spec.DeploymentConfigs.Production != nil {
			config.DeploymentConfigs.Production = r.convertDeploymentConfig(project.Spec.DeploymentConfigs.Production)
		}
	}

	// Set adoption policy and deployment history limit
	config.AdoptionPolicy = project.Spec.AdoptionPolicy
	if project.Spec.DeploymentHistoryLimit != nil {
		config.DeploymentHistoryLimit = *project.Spec.DeploymentHistoryLimit
	}

	// Enable Web Analytics (default true)
	config.EnableWebAnalytics = project.Spec.EnableWebAnalytics

	return config
}

// convertDeploymentConfig converts a deployment environment configuration.
//
//nolint:revive // cognitive complexity is acceptable for this conversion function
func (r *PagesProjectReconciler) convertDeploymentConfig(spec *networkingv1alpha2.PagesDeploymentConfig) *pagessvc.PagesDeploymentEnvConfig {
	config := &pagessvc.PagesDeploymentEnvConfig{
		CompatibilityDate:                spec.CompatibilityDate,
		CompatibilityFlags:               spec.CompatibilityFlags,
		UsageModel:                       spec.UsageModel,
		FailOpen:                         spec.FailOpen,
		AlwaysUseLatestCompatibilityDate: spec.AlwaysUseLatestCompatibilityDate,
	}

	// Convert environment variables
	if len(spec.EnvironmentVariables) > 0 {
		config.EnvironmentVariables = make(map[string]pagessvc.PagesEnvVar)
		for name, envVar := range spec.EnvironmentVariables {
			config.EnvironmentVariables[name] = pagessvc.PagesEnvVar{
				Value: envVar.Value,
				Type:  string(envVar.Type),
			}
		}
	}

	// Convert D1 bindings
	if len(spec.D1Bindings) > 0 {
		for _, b := range spec.D1Bindings {
			config.D1Bindings = append(config.D1Bindings, pagessvc.PagesBinding{
				Name: b.Name,
				ID:   b.DatabaseID,
			})
		}
	}

	// Convert KV bindings
	if len(spec.KVBindings) > 0 {
		for _, b := range spec.KVBindings {
			config.KVBindings = append(config.KVBindings, pagessvc.PagesBinding{
				Name: b.Name,
				ID:   b.NamespaceID,
			})
		}
	}

	// Convert R2 bindings
	if len(spec.R2Bindings) > 0 {
		for _, b := range spec.R2Bindings {
			config.R2Bindings = append(config.R2Bindings, pagessvc.PagesBinding{
				Name: b.Name,
				ID:   b.BucketName,
			})
		}
	}

	// Convert service bindings
	if len(spec.ServiceBindings) > 0 {
		for _, b := range spec.ServiceBindings {
			config.ServiceBindings = append(config.ServiceBindings, pagessvc.PagesServiceBinding{
				Name:        b.Name,
				Service:     b.Service,
				Environment: b.Environment,
			})
		}
	}

	// Convert Durable Object bindings
	if len(spec.DurableObjectBindings) > 0 {
		for _, b := range spec.DurableObjectBindings {
			config.DurableObjectBindings = append(config.DurableObjectBindings, pagessvc.PagesDurableObjectBinding{
				Name:            b.Name,
				ClassName:       b.ClassName,
				ScriptName:      b.ScriptName,
				EnvironmentName: b.EnvironmentName,
			})
		}
	}

	// Convert Queue bindings
	if len(spec.QueueBindings) > 0 {
		for _, b := range spec.QueueBindings {
			config.QueueBindings = append(config.QueueBindings, pagessvc.PagesBinding{
				Name: b.Name,
				ID:   b.QueueName,
			})
		}
	}

	// Convert AI bindings
	if len(spec.AIBindings) > 0 {
		for _, b := range spec.AIBindings {
			config.AIBindings = append(config.AIBindings, b.Name)
		}
	}

	// Convert Vectorize bindings
	if len(spec.VectorizeBindings) > 0 {
		for _, b := range spec.VectorizeBindings {
			config.VectorizeBindings = append(config.VectorizeBindings, pagessvc.PagesBinding{
				Name: b.Name,
				ID:   b.IndexName,
			})
		}
	}

	// Convert Hyperdrive bindings
	if len(spec.HyperdriveBindings) > 0 {
		for _, b := range spec.HyperdriveBindings {
			config.HyperdriveBindings = append(config.HyperdriveBindings, pagessvc.PagesBinding{
				Name: b.Name,
				ID:   b.ID,
			})
		}
	}

	// Convert mTLS certificates
	if len(spec.MTLSCertificates) > 0 {
		for _, c := range spec.MTLSCertificates {
			config.MTLSCertificates = append(config.MTLSCertificates, pagessvc.PagesBinding{
				Name: c.Name,
				ID:   c.CertificateID,
			})
		}
	}

	// Convert browser binding
	if spec.BrowserBinding != nil {
		config.BrowserBinding = spec.BrowserBinding.Name
	}

	// Convert placement
	if spec.Placement != nil {
		config.PlacementMode = spec.Placement.Mode
	}

	return config
}

func (r *PagesProjectReconciler) updateStatusError(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, project, func() {
		project.Status.State = networkingv1alpha2.PagesProjectStateError
		meta.SetStatusCondition(&project.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: project.Generation,
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		project.Status.ObservedGeneration = project.Generation
	})

	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *PagesProjectReconciler) updateStatusPending(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	accountID string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, project, func() {
		if project.Status.AccountID == "" {
			project.Status.AccountID = accountID
		}
		project.Status.ProjectID = r.getProjectName(project)
		project.Status.State = networkingv1alpha2.PagesProjectStatePending
		meta.SetStatusCondition(&project.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: project.Generation,
			Reason:             "Pending",
			Message:            "Pages project configuration registered, waiting for sync",
			LastTransitionTime: metav1.Now(),
		})
		project.Status.ObservedGeneration = project.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	// Requeue to check for sync completion
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *PagesProjectReconciler) updateStatusReady(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	accountID, subdomain string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, project, func() {
		project.Status.AccountID = accountID
		project.Status.ProjectID = r.getProjectName(project)
		project.Status.Subdomain = subdomain
		project.Status.State = networkingv1alpha2.PagesProjectStateReady
		meta.SetStatusCondition(&project.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: project.Generation,
			Reason:             "Synced",
			Message:            "Pages project synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		project.Status.ObservedGeneration = project.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *PagesProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("pagesproject-controller")

	// Initialize ProjectService
	r.projectService = pagessvc.NewProjectService(mgr.GetClient())

	// Initialize VersionManager
	r.versionManager = NewVersionManager(
		mgr.GetClient(),
		mgr.GetScheme(),
		ctrl.Log.WithName("pagesproject").WithName("versionmanager"),
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.PagesProject{}).
		Owns(&networkingv1alpha2.PagesDeployment{}). // Watch managed PagesDeployment resources
		Complete(r)
}
