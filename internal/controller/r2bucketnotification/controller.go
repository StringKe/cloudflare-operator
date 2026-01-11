// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package r2bucketnotification provides a controller for managing R2 bucket event notifications.
package r2bucketnotification

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

const (
	finalizerName = "cloudflare.com/r2-bucket-notification-finalizer"
)

// Reconciler reconciles an R2BucketNotification object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Internal state
	ctx          context.Context
	log          logr.Logger
	notification *networkingv1alpha2.R2BucketNotification
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=r2bucketnotifications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=r2bucketnotifications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=r2bucketnotifications/finalizers,verbs=update

// Reconcile handles R2BucketNotification reconciliation
//
//nolint:revive // cognitive complexity is acceptable for main reconcile loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrl.LoggerFrom(ctx)

	// Get the R2BucketNotification resource
	r.notification = &networkingv1alpha2.R2BucketNotification{}
	if err := r.Get(ctx, req.NamespacedName, r.notification); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch R2BucketNotification")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !r.notification.DeletionTimestamp.IsZero() {
		return r.handleDeletion()
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.notification, finalizerName) {
		controllerutil.AddFinalizer(r.notification, finalizerName)
		if err := r.Update(ctx, r.notification); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Create API client
	cfAPI, err := r.getAPIClient()
	if err != nil {
		r.updateState(networkingv1alpha2.R2NotificationStateError,
			fmt.Sprintf("Failed to get API client: %v", err))
		r.Recorder.Event(r.notification, corev1.EventTypeWarning,
			controller.EventReasonAPIError, cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Reconcile the notification
	return r.reconcileNotification(cfAPI)
}

// handleDeletion handles the deletion of R2BucketNotification
//
//nolint:revive // cognitive complexity is acceptable for deletion handling
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.notification, finalizerName) {
		return ctrl.Result{}, nil
	}

	// Remove the notification from Cloudflare
	if r.notification.Status.QueueID != "" {
		cfAPI, err := r.getAPIClient()
		if err == nil {
			if err := cfAPI.DeleteR2Notification(
				r.ctx,
				r.notification.Spec.BucketName,
				r.notification.Status.QueueID,
			); err != nil {
				if !cf.IsNotFoundError(err) {
					r.log.Error(err, "Failed to delete R2 notification")
					r.Recorder.Event(r.notification, corev1.EventTypeWarning, "DeleteFailed",
						cf.SanitizeErrorMessage(err))
					return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
				}
			} else {
				r.log.Info("R2 notification deleted",
					"bucket", r.notification.Spec.BucketName,
					"queue", r.notification.Spec.QueueName)
				r.Recorder.Event(r.notification, corev1.EventTypeNormal, "Deleted",
					fmt.Sprintf("Notification rules removed from bucket %s",
						r.notification.Spec.BucketName))
			}
		}
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.notification, func() {
		controllerutil.RemoveFinalizer(r.notification, finalizerName)
	}); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileNotification reconciles the R2 bucket notification configuration
func (r *Reconciler) reconcileNotification(cfAPI *cf.API) (ctrl.Result, error) {
	r.updateState(networkingv1alpha2.R2NotificationStatePending, "Configuring notification rules")

	// Get Queue ID from queue name
	queueID, err := r.resolveQueueID(cfAPI)
	if err != nil {
		r.updateState(networkingv1alpha2.R2NotificationStateError,
			fmt.Sprintf("Failed to resolve queue: %v", err))
		r.Recorder.Event(r.notification, corev1.EventTypeWarning, "QueueResolveFailed",
			cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Convert spec rules to API rules
	rules := make([]cf.R2NotificationRule, len(r.notification.Spec.Rules))
	for i, rule := range r.notification.Spec.Rules {
		eventTypes := make([]string, len(rule.EventTypes))
		for j, et := range rule.EventTypes {
			eventTypes[j] = string(et)
		}
		rules[i] = cf.R2NotificationRule{
			Prefix:      rule.Prefix,
			Suffix:      rule.Suffix,
			EventTypes:  eventTypes,
			Description: rule.Description,
		}
	}

	// Set notification configuration
	if err := cfAPI.SetR2Notification(
		r.ctx,
		r.notification.Spec.BucketName,
		queueID,
		rules,
	); err != nil {
		r.updateState(networkingv1alpha2.R2NotificationStateError,
			fmt.Sprintf("Failed to set notification: %v", err))
		r.Recorder.Event(r.notification, corev1.EventTypeWarning, controller.EventReasonAPIError,
			cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Update status
	r.notification.Status.QueueID = queueID
	r.notification.Status.RuleCount = len(rules)
	r.updateState(networkingv1alpha2.R2NotificationStateActive, "Notification rules configured")
	r.Recorder.Event(r.notification, corev1.EventTypeNormal, "Configured",
		fmt.Sprintf("Notification rules configured for bucket %s",
			r.notification.Spec.BucketName))

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// resolveQueueID resolves the queue name to a queue ID
func (r *Reconciler) resolveQueueID(cfAPI *cf.API) (string, error) {
	// If we already have a QueueID and the queue name hasn't changed, use it
	if r.notification.Status.QueueID != "" {
		return r.notification.Status.QueueID, nil
	}

	// Get queue ID from Cloudflare
	queueID, err := cfAPI.GetQueueID(r.ctx, r.notification.Spec.QueueName)
	if err != nil {
		return "", fmt.Errorf("failed to get queue ID for %s: %w",
			r.notification.Spec.QueueName, err)
	}

	return queueID, nil
}

// getAPIClient creates a Cloudflare API client from credentials
func (r *Reconciler) getAPIClient() (*cf.API, error) {
	if r.notification.Spec.CredentialsRef != nil {
		ref := &networkingv1alpha2.CloudflareCredentialsRef{
			Name: r.notification.Spec.CredentialsRef.Name,
		}
		return cf.NewAPIClientFromCredentialsRef(r.ctx, r.Client, ref)
	}
	return cf.NewAPIClientFromDefaultCredentials(r.ctx, r.Client)
}

// updateState updates the state and status of the R2BucketNotification
func (r *Reconciler) updateState(state networkingv1alpha2.R2NotificationState, message string) {
	r.notification.Status.State = state
	r.notification.Status.Message = message
	r.notification.Status.ObservedGeneration = r.notification.Generation

	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		ObservedGeneration: r.notification.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(state),
		Message:            message,
	}

	if state == networkingv1alpha2.R2NotificationStateActive {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "NotificationActive"
	}

	controller.SetCondition(&r.notification.Status.Conditions, condition.Type,
		condition.Status, condition.Reason, condition.Message)

	if err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.notification, func() {
		r.notification.Status.State = state
		r.notification.Status.Message = message
		r.notification.Status.ObservedGeneration = r.notification.Generation
		controller.SetCondition(&r.notification.Status.Conditions, condition.Type,
			condition.Status, condition.Reason, condition.Message)
	}); err != nil {
		r.log.Error(err, "failed to update status")
	}
}

// findNotificationsForCredentials returns R2BucketNotifications that reference the given credentials
//
//nolint:revive // cognitive complexity is acceptable for watch handler
func (r *Reconciler) findNotificationsForCredentials(
	ctx context.Context, obj client.Object,
) []reconcile.Request {
	creds, ok := obj.(*networkingv1alpha2.CloudflareCredentials)
	if !ok {
		return nil
	}

	notificationList := &networkingv1alpha2.R2BucketNotificationList{}
	if err := r.List(ctx, notificationList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, notification := range notificationList.Items {
		if notification.Spec.CredentialsRef != nil &&
			notification.Spec.CredentialsRef.Name == creds.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				},
			})
		}

		if creds.Spec.IsDefault && notification.Spec.CredentialsRef == nil {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				},
			})
		}
	}

	return requests
}

// findNotificationsForBucket returns R2BucketNotifications that reference the given R2Bucket
func (r *Reconciler) findNotificationsForBucket(
	ctx context.Context, obj client.Object,
) []reconcile.Request {
	bucket, ok := obj.(*networkingv1alpha2.R2Bucket)
	if !ok {
		return nil
	}

	notificationList := &networkingv1alpha2.R2BucketNotificationList{}
	if err := r.List(ctx, notificationList, client.InNamespace(bucket.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, notification := range notificationList.Items {
		// Match by bucket name (either spec.name or metadata.name)
		bucketName := bucket.Spec.Name
		if bucketName == "" {
			bucketName = bucket.Name
		}
		if notification.Spec.BucketName == bucketName {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				},
			})
		}
	}

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.R2BucketNotification{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findNotificationsForCredentials)).
		Watches(&networkingv1alpha2.R2Bucket{},
			handler.EnqueueRequestsFromMapFunc(r.findNotificationsForBucket)).
		Named("r2bucketnotification").
		Complete(r)
}
