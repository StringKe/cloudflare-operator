// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package pagespromotion implements the Controller for PagesPromotion CRD.
// It manages the promotion of Pages deployments to production using the Cloudflare Rollback API.
package pagespromotion

import (
	"context"
	"errors"
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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/controller/common"
)

const (
	FinalizerName = "pagespromotion.networking.cloudflare-operator.io/finalizer"

	// WaitingInterval is the interval for waiting on deployment readiness.
	WaitingInterval = 30 * time.Second

	// Event reasons
	EventReasonWaiting         = "WaitingForDeployment"
	EventReasonPromoting       = "Promoting"
	EventReasonPromoted        = "Promoted"
	EventReasonPromotionFailed = "PromotionFailed"
	EventReasonAlreadyPromoted = "AlreadyPromoted"

	// CloudflareEnvironmentProduction is the Cloudflare environment name for production deployments.
	CloudflareEnvironmentProduction = "production"
)

// PagesPromotionReconciler reconciles a PagesPromotion object.
// It manages the promotion of Pages deployments to production.
type PagesPromotionReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=pagespromotions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=pagespromotions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=pagespromotions/finalizers,verbs=update

//nolint:revive // cognitive complexity acceptable for reconcile loop
func (r *PagesPromotionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the PagesPromotion instance
	promotion := &networkingv1alpha2.PagesPromotion{}
	if err := r.Get(ctx, req.NamespacedName, promotion); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !promotion.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, promotion)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, promotion, FinalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &promotion.Spec.Cloudflare,
		Namespace:         promotion.Namespace,
		StatusAccountID:   promotion.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.setErrorStatus(ctx, promotion, err)
	}

	// Resolve project name
	projectName, err := r.resolveProjectName(ctx, promotion)
	if err != nil {
		logger.Error(err, "Failed to resolve project name")
		return r.setErrorStatus(ctx, promotion, err)
	}

	// Resolve deployment ID
	deploymentID, pagesDeployment, err := r.resolveDeploymentID(ctx, promotion)
	if err != nil {
		logger.Error(err, "Failed to resolve deployment ID")
		return r.setErrorStatus(ctx, promotion, err)
	}

	// Check if we should wait for deployment to succeed
	if pagesDeployment != nil && requireSuccessfulDeployment(promotion) {
		if pagesDeployment.Status.State != networkingv1alpha2.PagesDeploymentStateSucceeded {
			return r.setWaitingStatus(ctx, promotion, projectName, apiResult.AccountID, pagesDeployment)
		}
	}

	// Check if already promoted (idempotency)
	isProduction, currentProd, err := isAlreadyProduction(ctx, apiResult.API, projectName, deploymentID)
	if err != nil {
		logger.Error(err, "Failed to check production status")
		return r.setErrorStatus(ctx, promotion, err)
	}

	if isProduction {
		logger.Info("Deployment is already production", "deploymentId", deploymentID)
		return r.setPromotedStatus(ctx, promotion, projectName, apiResult.AccountID, currentProd, nil, pagesDeployment)
	}

	// Get current production deployment for tracking
	var previousProd *cf.PagesDeploymentResult
	if currentProd != nil && currentProd.ID != deploymentID {
		previousProd = currentProd
	}

	// Set promoting status
	r.setPromotingStatus(ctx, promotion, projectName, apiResult.AccountID)

	// Perform promotion via Cloudflare Rollback API
	logger.Info("Promoting deployment to production",
		"project", projectName,
		"deploymentId", deploymentID)

	result, err := apiResult.API.RollbackPagesDeployment(ctx, projectName, deploymentID)
	if err != nil {
		logger.Error(err, "Failed to promote deployment")
		r.Recorder.Event(promotion, corev1.EventTypeWarning, EventReasonPromotionFailed,
			fmt.Sprintf("Failed to promote: %s", cf.SanitizeErrorMessage(err)))
		return r.setErrorStatus(ctx, promotion, err)
	}

	r.Recorder.Event(promotion, corev1.EventTypeNormal, EventReasonPromoted,
		fmt.Sprintf("Deployment %s promoted to production", deploymentID))

	return r.setPromotedStatus(ctx, promotion, projectName, apiResult.AccountID, result, previousProd, pagesDeployment)
}

// resolveProjectName resolves the Cloudflare project name from the ProjectRef.
func (r *PagesPromotionReconciler) resolveProjectName(
	ctx context.Context,
	promotion *networkingv1alpha2.PagesPromotion,
) (string, error) {
	// Priority 1: CloudflareID/CloudflareName directly specified
	if promotion.Spec.ProjectRef.CloudflareID != "" {
		return promotion.Spec.ProjectRef.CloudflareID, nil
	}
	if promotion.Spec.ProjectRef.CloudflareName != "" {
		return promotion.Spec.ProjectRef.CloudflareName, nil
	}

	// Priority 2: Reference to PagesProject K8s resource
	if promotion.Spec.ProjectRef.Name != "" {
		project := &networkingv1alpha2.PagesProject{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: promotion.Namespace,
			Name:      promotion.Spec.ProjectRef.Name,
		}, project); err != nil {
			return "", fmt.Errorf("failed to get referenced PagesProject %s: %w",
				promotion.Spec.ProjectRef.Name, err)
		}

		if project.Spec.Name != "" {
			return project.Spec.Name, nil
		}
		return project.Name, nil
	}

	return "", errors.New("project reference is required: specify name, cloudflareId, or cloudflareName")
}

// resolveDeploymentID resolves the Cloudflare deployment ID from the DeploymentRef.
// Returns the deployment ID and optionally the PagesDeployment object if referenced by name.
func (r *PagesPromotionReconciler) resolveDeploymentID(
	ctx context.Context,
	promotion *networkingv1alpha2.PagesPromotion,
) (string, *networkingv1alpha2.PagesDeployment, error) {
	// Priority 1: Direct deployment ID
	if promotion.Spec.DeploymentRef.DeploymentID != "" {
		return promotion.Spec.DeploymentRef.DeploymentID, nil, nil
	}

	// Priority 2: Reference to PagesDeployment K8s resource
	if promotion.Spec.DeploymentRef.Name != "" {
		deployment := &networkingv1alpha2.PagesDeployment{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: promotion.Namespace,
			Name:      promotion.Spec.DeploymentRef.Name,
		}, deployment); err != nil {
			return "", nil, fmt.Errorf("failed to get referenced PagesDeployment %s: %w",
				promotion.Spec.DeploymentRef.Name, err)
		}

		if deployment.Status.DeploymentID == "" {
			return "", deployment, errors.New("referenced PagesDeployment has no deployment ID yet")
		}

		return deployment.Status.DeploymentID, deployment, nil
	}

	return "", nil, errors.New("deployment reference is required: specify name or deploymentId")
}

// requireSuccessfulDeployment returns whether the promotion requires a successful deployment.
func requireSuccessfulDeployment(promotion *networkingv1alpha2.PagesPromotion) bool {
	if promotion.Spec.RequireSuccessfulDeployment == nil {
		return true // default true
	}
	return *promotion.Spec.RequireSuccessfulDeployment
}

// isAlreadyProduction checks if the deployment is already the production deployment.
//
//nolint:revive // cognitive complexity acceptable for production check logic
func isAlreadyProduction(
	ctx context.Context,
	api *cf.API,
	projectName, deploymentID string,
) (bool, *cf.PagesDeploymentResult, error) {
	// Get the project to find the current production deployment
	project, err := api.GetPagesProject(ctx, projectName)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get project: %w", err)
	}

	// Check if latest deployment is production and matches
	if project.LatestDeployment != nil {
		latest := project.LatestDeployment
		if latest.Environment == CloudflareEnvironmentProduction && latest.ID == deploymentID {
			return true, latest, nil
		}
		// Return current production if exists
		if latest.Environment == CloudflareEnvironmentProduction {
			return false, latest, nil
		}
	}

	// Need to list deployments to find current production
	deployments, err := api.ListPagesDeployments(ctx, projectName)
	if err != nil {
		return false, nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	var currentProd *cf.PagesDeploymentResult
	for i := range deployments {
		d := &deployments[i]
		if d.Environment == CloudflareEnvironmentProduction {
			if d.ID == deploymentID {
				return true, d, nil
			}
			if currentProd == nil {
				currentProd = d
			}
		}
	}

	return false, currentProd, nil
}

// handleDeletion handles the deletion of a PagesPromotion.
// No Cloudflare cleanup needed - we don't roll back promotions.
func (r *PagesPromotionReconciler) handleDeletion(
	ctx context.Context,
	promotion *networkingv1alpha2.PagesPromotion,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(promotion, FinalizerName) {
		return common.NoRequeue(), nil
	}

	// No Cloudflare cleanup needed for promotions
	// The production deployment remains as-is

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, promotion, func() {
		controllerutil.RemoveFinalizer(promotion, FinalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}

	r.Recorder.Event(promotion, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")
	return common.NoRequeue(), nil
}

// setWaitingStatus updates the promotion status to waiting for deployment.
func (r *PagesPromotionReconciler) setWaitingStatus(
	ctx context.Context,
	promotion *networkingv1alpha2.PagesPromotion,
	projectName, accountID string,
	deployment *networkingv1alpha2.PagesDeployment,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	message := fmt.Sprintf("Waiting for deployment '%s' to succeed (current state: %s)",
		deployment.Name, deployment.Status.State)

	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, promotion, func() {
		promotion.Status.State = networkingv1alpha2.PagesPromotionStateValidating
		promotion.Status.ProjectName = projectName
		promotion.Status.AccountID = accountID
		promotion.Status.Message = message

		meta.SetStatusCondition(&promotion.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: promotion.Generation,
			Reason:             "WaitingForDeployment",
			Message:            message,
			LastTransitionTime: metav1.Now(),
		})
		promotion.Status.ObservedGeneration = promotion.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	logger.Info(message)
	r.Recorder.Event(promotion, corev1.EventTypeNormal, EventReasonWaiting, message)

	return ctrl.Result{RequeueAfter: WaitingInterval}, nil
}

// setPromotingStatus updates the promotion status to promoting.
func (r *PagesPromotionReconciler) setPromotingStatus(
	ctx context.Context,
	promotion *networkingv1alpha2.PagesPromotion,
	projectName, accountID string,
) {
	_ = controller.UpdateStatusWithConflictRetry(ctx, r.Client, promotion, func() {
		promotion.Status.State = networkingv1alpha2.PagesPromotionStatePromoting
		promotion.Status.ProjectName = projectName
		promotion.Status.AccountID = accountID
		promotion.Status.Message = "Promoting deployment to production"

		meta.SetStatusCondition(&promotion.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: promotion.Generation,
			Reason:             "Promoting",
			Message:            "Promoting deployment to production",
			LastTransitionTime: metav1.Now(),
		})
	})
}

// setPromotedStatus updates the promotion status to promoted.
//
//nolint:revive // cognitive complexity acceptable for status update with multiple fields
func (r *PagesPromotionReconciler) setPromotedStatus(
	ctx context.Context,
	promotion *networkingv1alpha2.PagesPromotion,
	projectName, accountID string,
	promoted *cf.PagesDeploymentResult,
	previous *cf.PagesDeploymentResult,
	sourceDeployment *networkingv1alpha2.PagesDeployment,
) (ctrl.Result, error) {
	now := metav1.Now()

	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, promotion, func() {
		promotion.Status.State = networkingv1alpha2.PagesPromotionStatePromoted
		promotion.Status.ProjectName = projectName
		promotion.Status.AccountID = accountID
		promotion.Status.Message = "Deployment promoted to production"
		promotion.Status.LastPromotionTime = &now

		// Build promoted deployment info
		promotedInfo := &networkingv1alpha2.PromotedDeploymentInfo{
			DeploymentID: promoted.ID,
			URL:          promoted.URL,
			PromotedAt:   &now,
		}

		// Extract hash URL from aliases
		for _, alias := range promoted.Aliases {
			if alias != promoted.URL && alias != "" {
				promotedInfo.HashURL = alias
				break
			}
		}

		// Set source deployment name if available
		if sourceDeployment != nil {
			promotedInfo.SourceDeploymentName = sourceDeployment.Name
		}

		promotion.Status.PromotedDeployment = promotedInfo

		// Build previous deployment info if available
		if previous != nil {
			previousInfo := &networkingv1alpha2.PreviousDeploymentInfo{
				DeploymentID: previous.ID,
				URL:          previous.URL,
				ReplacedAt:   &now,
			}
			for _, alias := range previous.Aliases {
				if alias != previous.URL && alias != "" {
					previousInfo.HashURL = alias
					break
				}
			}
			promotion.Status.PreviousDeployment = previousInfo
		}

		meta.SetStatusCondition(&promotion.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: promotion.Generation,
			Reason:             "Promoted",
			Message:            fmt.Sprintf("Deployment %s is now production at %s", promoted.ID, promoted.URL),
			LastTransitionTime: now,
		})
		promotion.Status.ObservedGeneration = promotion.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
}

// setErrorStatus updates the promotion status with an error.
func (r *PagesPromotionReconciler) setErrorStatus(
	ctx context.Context,
	promotion *networkingv1alpha2.PagesPromotion,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, promotion, func() {
		promotion.Status.State = networkingv1alpha2.PagesPromotionStateFailed
		promotion.Status.Message = cf.SanitizeErrorMessage(err)

		meta.SetStatusCondition(&promotion.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: promotion.Generation,
			Reason:             "Error",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		promotion.Status.ObservedGeneration = promotion.Generation
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	r.Recorder.Event(promotion, corev1.EventTypeWarning, EventReasonPromotionFailed,
		cf.SanitizeErrorMessage(err))

	// Determine requeue based on error type
	if cf.IsPermanentError(err) {
		return common.NoRequeue(), nil
	}
	return common.RequeueShort(), nil
}

// findPromotionsForDeployment returns PagesPromotions that reference the given PagesDeployment.
func (r *PagesPromotionReconciler) findPromotionsForDeployment(ctx context.Context, obj client.Object) []reconcile.Request {
	deployment, ok := obj.(*networkingv1alpha2.PagesDeployment)
	if !ok {
		return nil
	}

	// List all PagesPromotions in the same namespace
	promotionList := &networkingv1alpha2.PagesPromotionList{}
	if err := r.List(ctx, promotionList, client.InNamespace(deployment.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, promotion := range promotionList.Items {
		// Check if this promotion references the deployment
		if promotion.Spec.DeploymentRef.Name == deployment.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&promotion),
			})
		}
	}

	return requests
}

// findPromotionsForProject returns PagesPromotions that reference the given PagesProject.
func (r *PagesPromotionReconciler) findPromotionsForProject(ctx context.Context, obj client.Object) []reconcile.Request {
	project, ok := obj.(*networkingv1alpha2.PagesProject)
	if !ok {
		return nil
	}

	// List all PagesPromotions in the same namespace
	promotionList := &networkingv1alpha2.PagesPromotionList{}
	if err := r.List(ctx, promotionList, client.InNamespace(project.Namespace)); err != nil {
		return nil
	}

	projectName := project.Name
	if project.Spec.Name != "" {
		projectName = project.Spec.Name
	}

	var requests []reconcile.Request
	for _, promotion := range promotionList.Items {
		// Check if this promotion references the project
		if promotion.Spec.ProjectRef.Name == project.Name ||
			promotion.Spec.ProjectRef.CloudflareID == projectName ||
			promotion.Spec.ProjectRef.CloudflareName == projectName {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&promotion),
			})
		}
	}

	return requests
}

func (r *PagesPromotionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("pagespromotion-controller")
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("pagespromotion"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.PagesPromotion{}).
		Watches(&networkingv1alpha2.PagesDeployment{},
			handler.EnqueueRequestsFromMapFunc(r.findPromotionsForDeployment)).
		Watches(&networkingv1alpha2.PagesProject{},
			handler.EnqueueRequestsFromMapFunc(r.findPromotionsForProject)).
		Complete(r)
}
