// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package pagesproject implements the Controller for PagesProject CRD.
// It directly manages Cloudflare Pages projects.
package pagesproject

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	"github.com/StringKe/cloudflare-operator/internal/controller/common"
)

// PagesProjectReconciler reconciles a PagesProject object.
// It directly calls Cloudflare API and writes status back to the CRD.
type PagesProjectReconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	Recorder                record.EventRecorder
	APIFactory              *common.APIClientFactory
	versionManager          *VersionManager
	gitopsReconciler        *GitOpsReconciler
	latestPreviewReconciler *LatestPreviewReconciler
	autoPromoteReconciler   *AutoPromoteReconciler
	externalReconciler      *ExternalReconciler
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
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !project.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, project)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, project, FinalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &project.Spec.Cloudflare,
		Namespace:         project.Namespace,
		StatusAccountID:   project.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, project, err)
	}

	// Sync project to Cloudflare
	result, err := r.syncProject(ctx, project, apiResult)
	if err != nil {
		return result, err
	}

	// Handle version management based on policy
	policy := r.versionManager.GetPolicy(project)
	if r.versionManager.HasVersions(project) {
		var requeueAfter time.Duration

		switch policy {
		case networkingv1alpha2.VersionPolicyGitOps:
			// GitOps mode: use GitOpsReconciler
			if err := r.gitopsReconciler.Reconcile(ctx, project, apiResult.API); err != nil {
				logger.Error(err, "Failed to reconcile GitOps versions")
				controller.RecordErrorEventAndCondition(r.Recorder, project,
					&project.Status.Conditions, "GitOpsReconcileFailed", err)
				return ctrl.Result{RequeueAfter: 30 * time.Second}, err
			}

			// Update version mapping and status
			if err := r.gitopsReconciler.UpdateVersionMapping(ctx, project); err != nil {
				logger.Error(err, "Failed to update version mapping")
				// Non-fatal, continue
			}

		case networkingv1alpha2.VersionPolicyLatestPreview:
			// LatestPreview mode: track latest successful preview deployment
			if err := r.latestPreviewReconciler.Reconcile(ctx, project, apiResult.API); err != nil {
				logger.Error(err, "Failed to reconcile latestPreview")
				controller.RecordErrorEventAndCondition(r.Recorder, project,
					&project.Status.Conditions, "LatestPreviewReconcileFailed", err)
				return ctrl.Result{RequeueAfter: 30 * time.Second}, err
			}
			requeueAfter = r.latestPreviewReconciler.GetRequeueAfter()

		case networkingv1alpha2.VersionPolicyAutoPromote:
			// AutoPromote mode: automatically promote after preview succeeds
			requeue, err := r.autoPromoteReconciler.Reconcile(ctx, project, apiResult.API)
			if err != nil {
				logger.Error(err, "Failed to reconcile autoPromote")
				controller.RecordErrorEventAndCondition(r.Recorder, project,
					&project.Status.Conditions, "AutoPromoteReconcileFailed", err)
				return ctrl.Result{RequeueAfter: 30 * time.Second}, err
			}
			if requeue > 0 {
				requeueAfter = requeue
			} else {
				requeueAfter = r.autoPromoteReconciler.GetRequeueAfter()
			}

		case networkingv1alpha2.VersionPolicyExternal:
			// External mode: external system controls versioning
			requeue, err := r.externalReconciler.Reconcile(ctx, project, apiResult.API)
			if err != nil {
				logger.Error(err, "Failed to reconcile external versions")
				controller.RecordErrorEventAndCondition(r.Recorder, project,
					&project.Status.Conditions, "ExternalReconcileFailed", err)
				return ctrl.Result{RequeueAfter: 30 * time.Second}, err
			}
			requeueAfter = requeue

		default:
			// Other modes: use VersionManager
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

		// Update active policy in status
		if err := r.updateActivePolicy(ctx, project, policy); err != nil {
			logger.Error(err, "Failed to update active policy")
			// Non-fatal, continue
		}

		// Return with requeue if needed
		if requeueAfter > 0 {
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
	}

	return result, nil
}

// updateActivePolicy updates the active policy in project status.
func (r *PagesProjectReconciler) updateActivePolicy(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	policy networkingv1alpha2.VersionPolicy,
) error {
	if project.Status.ActivePolicy == policy {
		return nil
	}

	return controller.UpdateStatusWithConflictRetry(ctx, r.Client, project, func() {
		project.Status.ActivePolicy = policy
	})
}

// handleDeletion handles the deletion of a PagesProject.
func (r *PagesProjectReconciler) handleDeletion(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(project, FinalizerName) {
		return common.NoRequeue(), nil
	}

	// Check deletion policy
	if project.Spec.DeletionPolicy == "Orphan" {
		logger.Info("Orphan deletion policy, skipping Cloudflare deletion")
	} else {
		// Get API client
		apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
			CloudflareDetails: &project.Spec.Cloudflare,
			Namespace:         project.Namespace,
			StatusAccountID:   project.Status.AccountID,
		})
		if err != nil {
			logger.Error(err, "Failed to get API client for deletion")
			// Continue with finalizer removal
		} else {
			// Delete project from Cloudflare
			projectName := r.getProjectName(project)
			logger.Info("Deleting Pages project from Cloudflare",
				"projectName", projectName)

			if err := apiResult.API.DeletePagesProject(ctx, projectName); err != nil {
				if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to delete Pages project from Cloudflare")
					r.Recorder.Event(project, corev1.EventTypeWarning, "DeleteFailed",
						fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
					return common.RequeueShort(), err
				}
				logger.Info("Pages project not found in Cloudflare, may have been already deleted")
			}

			r.Recorder.Event(project, corev1.EventTypeNormal, "Deleted",
				"Pages project deleted from Cloudflare")
		}
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, project, func() {
		controllerutil.RemoveFinalizer(project, FinalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(project, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// syncProject syncs the Pages project configuration to Cloudflare.
func (r *PagesProjectReconciler) syncProject(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	projectName := r.getProjectName(project)

	// Build API parameters
	params := r.buildProjectParams(project)

	// Check if project exists
	existing, err := apiResult.API.GetPagesProject(ctx, projectName)
	if err != nil {
		if !cf.IsNotFoundError(err) {
			logger.Error(err, "Failed to get Pages project from Cloudflare")
			return r.updateStatusError(ctx, project, err)
		}
		// Project doesn't exist, create it
		existing = nil
	}

	var result *cf.PagesProjectResult
	if existing != nil {
		// Update existing project
		result, err = apiResult.API.UpdatePagesProject(ctx, projectName, params)
		if err != nil {
			logger.Error(err, "Failed to update Pages project")
			return r.updateStatusError(ctx, project, err)
		}
		logger.V(1).Info("Pages project updated in Cloudflare",
			"projectName", projectName)
	} else {
		// Create new project
		result, err = apiResult.API.CreatePagesProject(ctx, params)
		if err != nil {
			// Check adoption policy
			if cf.IsConflictError(err) && project.Spec.AdoptionPolicy == "Adopt" {
				// Try to adopt existing project
				logger.Info("Pages project already exists, attempting to adopt",
					"projectName", projectName)
				existing, err = apiResult.API.GetPagesProject(ctx, projectName)
				if err != nil {
					logger.Error(err, "Failed to get existing project for adoption")
					return r.updateStatusError(ctx, project, err)
				}
				result, err = apiResult.API.UpdatePagesProject(ctx, projectName, params)
				if err != nil {
					logger.Error(err, "Failed to update adopted project")
					return r.updateStatusError(ctx, project, err)
				}
				r.Recorder.Event(project, corev1.EventTypeNormal, "Adopted",
					fmt.Sprintf("Adopted existing Pages project '%s'", projectName))
			} else {
				logger.Error(err, "Failed to create Pages project")
				return r.updateStatusError(ctx, project, err)
			}
		} else {
			r.Recorder.Event(project, corev1.EventTypeNormal, "Created",
				fmt.Sprintf("Pages project '%s' created in Cloudflare", projectName))
		}
	}

	// Update status
	return r.updateStatusReady(ctx, project, apiResult.AccountID, result.Subdomain)
}

// getProjectName returns the project name from spec or uses K8s resource name.
func (*PagesProjectReconciler) getProjectName(project *networkingv1alpha2.PagesProject) string {
	if project.Spec.Name != "" {
		return project.Spec.Name
	}
	return project.Name
}

// buildProjectParams builds the Cloudflare API parameters from the spec.
//
//nolint:revive // cognitive complexity is acceptable for building complex config
func (r *PagesProjectReconciler) buildProjectParams(project *networkingv1alpha2.PagesProject) cf.PagesProjectParams {
	params := cf.PagesProjectParams{
		Name:             r.getProjectName(project),
		ProductionBranch: project.Spec.ProductionBranch,
	}

	// Convert source configuration
	if project.Spec.Source != nil {
		params.Source = &cf.PagesSourceConfig{
			Type: string(project.Spec.Source.Type),
		}
		if project.Spec.Source.GitHub != nil {
			params.Source.GitHub = &cf.PagesGitHubConfig{
				Owner:                        project.Spec.Source.GitHub.Owner,
				Repo:                         project.Spec.Source.GitHub.Repo,
				ProductionDeploymentsEnabled: project.Spec.Source.GitHub.ProductionDeploymentsEnabled,
				PreviewDeploymentsEnabled:    project.Spec.Source.GitHub.PreviewDeploymentsEnabled,
				PRCommentsEnabled:            project.Spec.Source.GitHub.PRCommentsEnabled,
				DeploymentsEnabled:           project.Spec.Source.GitHub.DeploymentsEnabled,
			}
		}
		if project.Spec.Source.GitLab != nil {
			params.Source.GitLab = &cf.PagesGitLabConfig{
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
		params.BuildConfig = &cf.PagesBuildConfig{
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
		params.DeploymentConfig = &cf.PagesDeploymentConfigs{}
		if project.Spec.DeploymentConfigs.Preview != nil {
			params.DeploymentConfig.Preview = r.convertDeploymentEnvConfig(project.Spec.DeploymentConfigs.Preview)
		}
		if project.Spec.DeploymentConfigs.Production != nil {
			params.DeploymentConfig.Production = r.convertDeploymentEnvConfig(project.Spec.DeploymentConfigs.Production)
		}
	}

	return params
}

// convertDeploymentEnvConfig converts a deployment environment configuration.
//
//nolint:revive // cognitive complexity is acceptable for this conversion function
func (r *PagesProjectReconciler) convertDeploymentEnvConfig(spec *networkingv1alpha2.PagesDeploymentConfig) *cf.PagesDeploymentEnvConfig {
	config := &cf.PagesDeploymentEnvConfig{
		CompatibilityDate:       spec.CompatibilityDate,
		CompatibilityFlags:      spec.CompatibilityFlags,
		UsageModel:              spec.UsageModel,
		FailOpen:                spec.FailOpen,
		AlwaysUseLatestCompDate: spec.AlwaysUseLatestCompatibilityDate,
	}

	// Convert environment variables
	if len(spec.EnvironmentVariables) > 0 {
		config.EnvironmentVariables = make(map[string]cf.PagesEnvVar)
		for name, envVar := range spec.EnvironmentVariables {
			config.EnvironmentVariables[name] = cf.PagesEnvVar{
				Value: envVar.Value,
				Type:  string(envVar.Type),
			}
		}
	}

	// Convert D1 bindings (map: name -> databaseID)
	if len(spec.D1Bindings) > 0 {
		config.D1Bindings = make(map[string]string)
		for _, b := range spec.D1Bindings {
			config.D1Bindings[b.Name] = b.DatabaseID
		}
	}

	// Convert KV bindings (map: name -> namespaceID)
	if len(spec.KVBindings) > 0 {
		config.KVBindings = make(map[string]string)
		for _, b := range spec.KVBindings {
			config.KVBindings[b.Name] = b.NamespaceID
		}
	}

	// Convert R2 bindings (map: name -> bucketName)
	if len(spec.R2Bindings) > 0 {
		config.R2Bindings = make(map[string]string)
		for _, b := range spec.R2Bindings {
			config.R2Bindings[b.Name] = b.BucketName
		}
	}

	// Convert service bindings (map: name -> config)
	if len(spec.ServiceBindings) > 0 {
		config.ServiceBindings = make(map[string]cf.PagesServiceBindingConfig)
		for _, b := range spec.ServiceBindings {
			config.ServiceBindings[b.Name] = cf.PagesServiceBindingConfig{
				Service:     b.Service,
				Environment: b.Environment,
			}
		}
	}

	// Convert Durable Object bindings (map: name -> config)
	if len(spec.DurableObjectBindings) > 0 {
		config.DurableObjectBindings = make(map[string]cf.PagesDurableObjectBindingConfig)
		for _, b := range spec.DurableObjectBindings {
			config.DurableObjectBindings[b.Name] = cf.PagesDurableObjectBindingConfig{
				ClassName:       b.ClassName,
				ScriptName:      b.ScriptName,
				EnvironmentName: b.EnvironmentName,
			}
		}
	}

	// Convert Queue bindings (map: name -> queueName)
	if len(spec.QueueBindings) > 0 {
		config.QueueBindings = make(map[string]string)
		for _, b := range spec.QueueBindings {
			config.QueueBindings[b.Name] = b.QueueName
		}
	}

	// Convert AI bindings (slice of binding names)
	if len(spec.AIBindings) > 0 {
		config.AIBindings = make([]string, len(spec.AIBindings))
		for i, b := range spec.AIBindings {
			config.AIBindings[i] = b.Name
		}
	}

	// Convert Vectorize bindings (map: name -> indexName)
	if len(spec.VectorizeBindings) > 0 {
		config.VectorizeBindings = make(map[string]string)
		for _, b := range spec.VectorizeBindings {
			config.VectorizeBindings[b.Name] = b.IndexName
		}
	}

	// Convert Hyperdrive bindings (map: name -> configID)
	if len(spec.HyperdriveBindings) > 0 {
		config.HyperdriveBindings = make(map[string]string)
		for _, b := range spec.HyperdriveBindings {
			config.HyperdriveBindings[b.Name] = b.ID
		}
	}

	// Convert mTLS certificates (map: name -> certificateID)
	if len(spec.MTLSCertificates) > 0 {
		config.MTLSCertificates = make(map[string]string)
		for _, c := range spec.MTLSCertificates {
			config.MTLSCertificates[c.Name] = c.CertificateID
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
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
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
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
}

func (r *PagesProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("pagesproject-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("pagesproject"))

	logger := ctrl.Log.WithName("pagesproject")

	// Initialize VersionManager
	r.versionManager = NewVersionManager(
		mgr.GetClient(),
		mgr.GetScheme(),
		logger.WithName("versionmanager"),
	)

	// Initialize GitOpsReconciler
	r.gitopsReconciler = NewGitOpsReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		r.Recorder,
		logger,
	)

	// Initialize LatestPreviewReconciler
	r.latestPreviewReconciler = NewLatestPreviewReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		r.Recorder,
		logger,
	)

	// Initialize AutoPromoteReconciler
	r.autoPromoteReconciler = NewAutoPromoteReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		r.Recorder,
		logger,
	)

	// Initialize ExternalReconciler
	r.externalReconciler = NewExternalReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		r.Recorder,
		logger,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.PagesProject{}).
		Owns(&networkingv1alpha2.PagesDeployment{}). // Watch managed PagesDeployment resources
		Complete(r)
}
