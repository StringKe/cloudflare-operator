// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package r2bucketnotification provides a controller for managing R2 bucket event notifications.
package r2bucketnotification

import (
	"context"
	"fmt"
	"time"

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
	"github.com/StringKe/cloudflare-operator/internal/service"
	r2svc "github.com/StringKe/cloudflare-operator/internal/service/r2"
)

const (
	finalizerName = "cloudflare.com/r2-bucket-notification-finalizer"
)

// Reconciler reconciles an R2BucketNotification object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Services
	notificationService *r2svc.NotificationService
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
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch R2BucketNotification")
		return ctrl.Result{}, err
	}

	// Resolve credentials and account ID
	credRef, accountID, err := r.resolveCredentials(ctx, notification)
	if err != nil {
		logger.Error(err, "Failed to resolve credentials")
		return r.updateStatusError(ctx, notification, err)
	}

	// Handle deletion
	// Following Unified Sync Architecture: only unregister from SyncState
	// Sync Controller handles actual Cloudflare API deletion
	if !notification.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, notification)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(notification, finalizerName) {
		controllerutil.AddFinalizer(notification, finalizerName)
		if err := r.Update(ctx, notification); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Register R2 bucket notification configuration to SyncState
	return r.registerNotification(ctx, notification, accountID, credRef)
}

// resolveCredentials resolves the credentials reference and account ID.
// Following Unified Sync Architecture, the Resource Controller only needs
// credential metadata (accountID, credRef) - it does not create a Cloudflare API client.
func (r *Reconciler) resolveCredentials(
	ctx context.Context,
	notification *networkingv1alpha2.R2BucketNotification,
) (credRef networkingv1alpha2.CredentialsReference, accountID string, err error) {
	logger := log.FromContext(ctx)

	// Resolve credentials using the unified controller helper
	credInfo, err := controller.ResolveCredentialsFromRef(ctx, r.Client, logger, notification.Spec.CredentialsRef)
	if err != nil {
		return networkingv1alpha2.CredentialsReference{}, "", fmt.Errorf("failed to resolve credentials: %w", err)
	}

	if credInfo.AccountID == "" {
		logger.V(1).Info("Account ID not available from credentials, will be resolved during sync")
	}

	return credInfo.CredentialsRef, credInfo.AccountID, nil
}

// handleDeletion handles the deletion of R2BucketNotification.
// Following Unified Sync Architecture:
// Resource Controller only unregisters from SyncState.
// R2BucketNotification Sync Controller handles the actual Cloudflare API deletion.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	notification *networkingv1alpha2.R2BucketNotification,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(notification, finalizerName) {
		return ctrl.Result{}, nil
	}

	logger.Info("Unregistering R2BucketNotification from SyncState")

	// Unregister from SyncState - this triggers Sync Controller to delete from Cloudflare
	// Following: Resource Controller → Core Service → SyncState → Sync Controller → Cloudflare API
	source := service.Source{
		Kind:      "R2BucketNotification",
		Namespace: notification.Namespace,
		Name:      notification.Name,
	}

	if err := r.notificationService.Unregister(ctx, notification.Status.QueueID, source); err != nil {
		logger.Error(err, "Failed to unregister R2 bucket notification from SyncState")
		r.Recorder.Event(notification, corev1.EventTypeWarning, "UnregisterFailed",
			fmt.Sprintf("Failed to unregister from SyncState: %s", err.Error()))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	r.Recorder.Event(notification, corev1.EventTypeNormal, "Unregistered",
		"Unregistered from SyncState, Sync Controller will delete from Cloudflare")

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, notification, func() {
		controllerutil.RemoveFinalizer(notification, finalizerName)
	}); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(notification, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")
	return ctrl.Result{}, nil
}

// registerNotification registers the R2 bucket notification configuration to SyncState.
// The actual sync to Cloudflare is handled by R2BucketNotificationSyncController.
func (r *Reconciler) registerNotification(
	ctx context.Context,
	notification *networkingv1alpha2.R2BucketNotification,
	accountID string,
	credRef networkingv1alpha2.CredentialsReference,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Build notification rules
	rules := make([]networkingv1alpha2.R2NotificationRule, len(notification.Spec.Rules))
	for i, rule := range notification.Spec.Rules {
		rules[i] = networkingv1alpha2.R2NotificationRule{
			Prefix:      rule.Prefix,
			Suffix:      rule.Suffix,
			EventTypes:  rule.EventTypes,
			Description: rule.Description,
		}
	}

	// Build notification configuration
	config := r2svc.R2BucketNotificationConfig{
		BucketName: notification.Spec.BucketName,
		QueueName:  notification.Spec.QueueName,
		Rules:      rules,
	}

	// Create source reference
	source := service.Source{
		Kind:      "R2BucketNotification",
		Namespace: notification.Namespace,
		Name:      notification.Name,
	}

	// Register to SyncState
	opts := r2svc.R2BucketNotificationRegisterOptions{
		AccountID:      accountID,
		QueueID:        notification.Status.QueueID, // May be empty for new notifications
		Source:         source,
		Config:         config,
		CredentialsRef: credRef,
	}

	if err := r.notificationService.Register(ctx, opts); err != nil {
		logger.Error(err, "Failed to register R2 bucket notification configuration")
		r.Recorder.Event(notification, corev1.EventTypeWarning, "RegisterFailed",
			fmt.Sprintf("Failed to register R2 bucket notification: %s", err.Error()))
		return r.updateStatusError(ctx, notification, err)
	}

	r.Recorder.Event(notification, corev1.EventTypeNormal, "Registered",
		fmt.Sprintf("Registered R2 Bucket Notification for bucket '%s' to SyncState",
			notification.Spec.BucketName))

	// Update status to Pending - actual sync happens via R2BucketNotificationSyncController
	return r.updateStatusPending(ctx, notification)
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
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		notification.Status.ObservedGeneration = notification.Generation
	})

	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *Reconciler) updateStatusPending(
	ctx context.Context,
	notification *networkingv1alpha2.R2BucketNotification,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, notification, func() {
		notification.Status.State = networkingv1alpha2.R2NotificationStatePending
		notification.Status.Message = "R2 bucket notification configuration registered, waiting for sync"
		meta.SetStatusCondition(&notification.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: notification.Generation,
			Reason:             "Pending",
			Message:            "R2 bucket notification configuration registered, waiting for sync",
			LastTransitionTime: metav1.Now(),
		})
		notification.Status.ObservedGeneration = notification.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	// Requeue to check for sync completion
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
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

// findNotificationsForSyncState returns R2BucketNotifications that are sources for the given SyncState
func (*Reconciler) findNotificationsForSyncState(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	syncState, ok := obj.(*networkingv1alpha2.CloudflareSyncState)
	if !ok {
		return nil
	}

	// Only process R2BucketNotification type SyncStates
	if syncState.Spec.ResourceType != networkingv1alpha2.SyncResourceR2BucketNotification {
		return nil
	}

	var requests []reconcile.Request
	for _, source := range syncState.Spec.Sources {
		if source.Ref.Kind == "R2BucketNotification" {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      source.Ref.Name,
					Namespace: source.Ref.Namespace,
				},
			})
		}
	}

	logger.V(1).Info("Found R2BucketNotifications for SyncState update",
		"syncState", syncState.Name,
		"notificationCount", len(requests))

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("r2bucketnotification-controller")

	// Initialize NotificationService
	r.notificationService = r2svc.NewNotificationService(mgr.GetClient())

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.R2BucketNotification{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findNotificationsForCredentials)).
		Watches(&networkingv1alpha2.R2Bucket{},
			handler.EnqueueRequestsFromMapFunc(r.findNotificationsForBucket)).
		Watches(&networkingv1alpha2.CloudflareSyncState{},
			handler.EnqueueRequestsFromMapFunc(r.findNotificationsForSyncState)).
		Named("r2bucketnotification").
		Complete(r)
}
