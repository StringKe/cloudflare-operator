// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package r2bucketnotification provides a controller for managing R2 bucket event notifications.
// It directly calls Cloudflare API and writes status back to the CRD.
package r2bucketnotification

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	finalizerName = "cloudflare.com/r2-bucket-notification-finalizer"
)

// Reconciler reconciles an R2BucketNotification object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=r2bucketnotifications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=r2bucketnotifications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=r2bucketnotifications/finalizers,verbs=update

// Reconcile handles R2BucketNotification reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the R2BucketNotification resource
	notification := &networkingv1alpha2.R2BucketNotification{}
	if err := r.Get(ctx, req.NamespacedName, notification); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch R2BucketNotification")
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !notification.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, notification)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, notification, finalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CredentialsRef: notification.Spec.CredentialsRef,
		Namespace:      notification.Namespace,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, notification, err)
	}

	// Sync notification to Cloudflare
	return r.syncNotification(ctx, notification, apiResult)
}

// handleDeletion handles the deletion of R2BucketNotification.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	notification *networkingv1alpha2.R2BucketNotification,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(notification, finalizerName) {
		return common.NoRequeue(), nil
	}

	// Get API client and delete from Cloudflare
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CredentialsRef: notification.Spec.CredentialsRef,
		Namespace:      notification.Namespace,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client for deletion")
		// Continue with finalizer removal
	} else if notification.Status.QueueID != "" && notification.Spec.BucketName != "" {
		// Delete notification from Cloudflare
		logger.Info("Deleting R2 notification from Cloudflare",
			"bucketName", notification.Spec.BucketName,
			"queueId", notification.Status.QueueID)

		if err := apiResult.API.DeleteR2Notification(ctx, notification.Spec.BucketName, notification.Status.QueueID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete R2 notification from Cloudflare, continuing with finalizer removal")
				r.Recorder.Event(notification, corev1.EventTypeWarning, "DeleteFailed",
					fmt.Sprintf("Failed to delete from Cloudflare (will remove finalizer anyway): %s", cf.SanitizeErrorMessage(err)))
				// Don't block finalizer removal - resource may need manual cleanup in Cloudflare
			} else {
				logger.Info("R2 notification not found in Cloudflare, may have been already deleted")
			}
		} else {
			r.Recorder.Event(notification, corev1.EventTypeNormal, "Deleted",
				"R2 notification deleted from Cloudflare")
		}
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, notification, func() {
		controllerutil.RemoveFinalizer(notification, finalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(notification, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// syncNotification syncs the R2 notification to Cloudflare.
func (r *Reconciler) syncNotification(
	ctx context.Context,
	notification *networkingv1alpha2.R2BucketNotification,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	bucketName := notification.Spec.BucketName
	queueName := notification.Spec.QueueName

	// Build notification rules (convert R2EventType to string)
	rules := make([]cf.R2NotificationRule, 0, len(notification.Spec.Rules))
	for _, rule := range notification.Spec.Rules {
		eventTypes := make([]string, 0, len(rule.EventTypes))
		for _, et := range rule.EventTypes {
			eventTypes = append(eventTypes, string(et))
		}
		rules = append(rules, cf.R2NotificationRule{
			Prefix:      rule.Prefix,
			Suffix:      rule.Suffix,
			EventTypes:  eventTypes,
			Description: rule.Description,
		})
	}

	// Determine queue ID
	// If we have a queue ID in status, use it. Otherwise, use the queue name as ID.
	queueID := notification.Status.QueueID
	if queueID == "" {
		queueID = queueName
	}

	// Set notification rules
	logger.Info("Setting R2 notification rules",
		"bucketName", bucketName,
		"queueId", queueID,
		"ruleCount", len(rules))

	if err := apiResult.API.SetR2Notification(ctx, bucketName, queueID, rules); err != nil {
		logger.Error(err, "Failed to set R2 notification")
		return r.updateStatusError(ctx, notification, err)
	}

	r.Recorder.Event(notification, corev1.EventTypeNormal, "Synced",
		fmt.Sprintf("R2 notification rules set for bucket '%s' with queue '%s'", bucketName, queueID))

	return r.updateStatusReady(ctx, notification, apiResult.AccountID, queueID)
}

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	notification *networkingv1alpha2.R2BucketNotification,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, notification, func() {
		notification.Status.State = networkingv1alpha2.R2NotificationStateError
		notification.Status.Message = cf.SanitizeErrorMessage(err)
		meta.SetStatusCondition(&notification.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: notification.Generation,
			Reason:             "Error",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		notification.Status.ObservedGeneration = notification.Generation
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
}

func (r *Reconciler) updateStatusReady(
	ctx context.Context,
	notification *networkingv1alpha2.R2BucketNotification,
	_, queueID string, // accountID not stored in status
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, notification, func() {
		notification.Status.QueueID = queueID
		notification.Status.RuleCount = len(notification.Spec.Rules)
		notification.Status.State = networkingv1alpha2.R2NotificationStateActive
		notification.Status.Message = ""
		meta.SetStatusCondition(&notification.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: notification.Generation,
			Reason:             "Synced",
			Message:            "R2 notification synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		notification.Status.ObservedGeneration = notification.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
}

// findNotificationsForCredentials returns R2BucketNotifications that reference the given credentials
func (r *Reconciler) findNotificationsForCredentials(ctx context.Context, obj client.Object) []reconcile.Request {
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
func (r *Reconciler) findNotificationsForBucket(ctx context.Context, obj client.Object) []reconcile.Request {
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
	r.Recorder = mgr.GetEventRecorderFor("r2bucketnotification-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("r2bucketnotification"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.R2BucketNotification{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findNotificationsForCredentials)).
		Watches(&networkingv1alpha2.R2Bucket{},
			handler.EnqueueRequestsFromMapFunc(r.findNotificationsForBucket)).
		Named("r2bucketnotification").
		Complete(r)
}
