// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package accesstag provides the controller for the AccessTag CRD.
// It ensures account-level Cloudflare Zero Trust Access tags exist so that
// AccessApplications can reference them via spec.tags.
package accesstag

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/controller/common"
)

const (
	// FinalizerName guards the AccessTag so the controller can run its
	// app_count-checked teardown before the object is removed.
	FinalizerName = "cloudflare.com/accesstag-finalizer"
	// StateActive indicates the tag is ensured to exist in Cloudflare.
	StateActive = "active"
)

// Reconciler reconciles an AccessTag object. It ensures the named Cloudflare
// Access tag exists (adopt-or-create) and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accesstags,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accesstags/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accesstags/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrllog.FromContext(ctx)

	tag := &networkingv1alpha2.AccessTag{}
	if err := r.Get(ctx, req.NamespacedName, tag); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !tag.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, logger, tag)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, tag, FinalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Get API client - use resource namespace for credentials resolution
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &tag.Spec.Cloudflare,
		Namespace:         tag.Namespace,
		StatusAccountID:   tag.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.setErrorStatus(ctx, tag, err)
	}

	return r.reconcileTag(ctx, logger, tag, apiResult)
}

// reconcileTag ensures the Access tag exists in Cloudflare (adopt-or-create).
func (r *Reconciler) reconcileTag(
	ctx context.Context,
	logger logr.Logger,
	tag *networkingv1alpha2.AccessTag,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	// GetAccessTag returns (nil, nil) when the tag does not exist yet.
	result, err := apiResult.API.GetAccessTag(ctx, tag.Spec.Name)
	if err != nil {
		logger.Error(err, "Failed to get AccessTag from Cloudflare")
		return r.setErrorStatus(ctx, tag, err)
	}

	if result == nil {
		result, err = apiResult.API.CreateAccessTag(ctx, tag.Spec.Name)
		if err != nil {
			logger.Error(err, "Failed to create AccessTag")
			return r.setErrorStatus(ctx, tag, err)
		}
		r.Recorder.Event(tag, corev1.EventTypeNormal, "Created",
			fmt.Sprintf("AccessTag '%s' created in Cloudflare", tag.Spec.Name))
	} else {
		logger.V(1).Info("AccessTag already exists in Cloudflare, adopting", "name", tag.Spec.Name)
	}

	return r.setSuccessStatus(ctx, tag, apiResult.AccountID, result)
}

// handleDeletion runs the app_count-guarded teardown, then removes the finalizer.
// It never blocks deletion: on any read/delete failure the finalizer is removed
// anyway, so a transient Cloudflare error cannot wedge the object.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	logger logr.Logger,
	tag *networkingv1alpha2.AccessTag,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(tag, FinalizerName) {
		return common.NoRequeue(), nil
	}

	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &tag.Spec.Cloudflare,
		Namespace:         tag.Namespace,
		StatusAccountID:   tag.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client for deletion")
		// Continue with finalizer removal - resource may need manual cleanup.
	} else {
		r.tryDeleteTag(ctx, logger, tag, apiResult.API)
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, tag, func() {
		controllerutil.RemoveFinalizer(tag, FinalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(tag, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// tryDeleteTag best-effort deletes the Cloudflare tag, but only when nothing
// references it (app_count == 0). Tags are account-global while AccessTag CRs are
// namespaced, so a tag still in use - by an application or a sibling CR in another
// namespace - must not be removed. Never returns an error: teardown is best-effort.
func (r *Reconciler) tryDeleteTag(
	ctx context.Context,
	logger logr.Logger,
	tag *networkingv1alpha2.AccessTag,
	api *cf.API,
) {
	existing, err := api.GetAccessTag(ctx, tag.Spec.Name)
	switch {
	case err != nil:
		logger.Error(err, "Failed to read AccessTag during deletion, removing finalizer anyway", "name", tag.Spec.Name)
	case existing == nil:
		logger.Info("AccessTag not found in Cloudflare, may have been already deleted", "name", tag.Spec.Name)
	case shouldDeleteTag(existing.AppCount):
		if delErr := api.DeleteAccessTag(ctx, tag.Spec.Name); delErr != nil {
			logger.Error(delErr, "Failed to delete AccessTag from Cloudflare, removing finalizer anyway", "name", tag.Spec.Name)
			r.Recorder.Event(tag, corev1.EventTypeWarning, "DeleteFailed",
				fmt.Sprintf("Failed to delete from Cloudflare (will remove finalizer anyway): %s", cf.SanitizeErrorMessage(delErr)))
		} else {
			r.Recorder.Event(tag, corev1.EventTypeNormal, "Deleted", "AccessTag deleted from Cloudflare")
		}
	default:
		logger.Info("AccessTag still in use, leaving it in Cloudflare",
			"name", tag.Spec.Name, "appCount", existing.AppCount)
		r.Recorder.Event(tag, corev1.EventTypeNormal, "Retained",
			fmt.Sprintf("AccessTag '%s' still used by %d application(s); not deleted", tag.Spec.Name, existing.AppCount))
	}
}

// shouldDeleteTag reports whether a Cloudflare Access tag is safe to delete: only
// when no Access applications reference it. Pure helper, separated for unit testing.
func shouldDeleteTag(appCount int) bool {
	return appCount == 0
}

// setSuccessStatus updates the AccessTag status after a successful sync.
func (r *Reconciler) setSuccessStatus(
	ctx context.Context,
	tag *networkingv1alpha2.AccessTag,
	accountID string,
	result *cf.AccessTagResult,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, tag, func() {
		tag.Status.AccountID = accountID
		tag.Status.TagName = result.Name
		tag.Status.AppCount = result.AppCount
		tag.Status.State = StateActive
		tag.Status.ObservedGeneration = tag.Generation

		meta.SetStatusCondition(&tag.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: tag.Generation,
			Reason:             "Synced",
			Message:            "AccessTag synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
	})
	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
}

// setErrorStatus updates the AccessTag status with an error and requeues.
func (r *Reconciler) setErrorStatus(
	ctx context.Context,
	tag *networkingv1alpha2.AccessTag,
	cause error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, tag, func() {
		tag.Status.State = "error"
		tag.Status.ObservedGeneration = tag.Generation
		meta.SetStatusCondition(&tag.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: tag.Generation,
			Reason:             "Error",
			Message:            cf.SanitizeErrorMessage(cause),
			LastTransitionTime: metav1.Now(),
		})
	})
	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("accesstag-controller")
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("accesstag"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.AccessTag{}).
		Complete(r)
}
