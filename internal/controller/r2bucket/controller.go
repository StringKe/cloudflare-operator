// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package r2bucket provides a controller for managing Cloudflare R2 storage buckets.
package r2bucket

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
	finalizerName = "cloudflare.com/r2-bucket-finalizer"
)

// Reconciler reconciles an R2Bucket object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Services
	bucketService *r2svc.BucketService
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=r2buckets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=r2buckets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=r2buckets/finalizers,verbs=update

// Reconcile handles R2Bucket reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the R2Bucket resource
	bucket := &networkingv1alpha2.R2Bucket{}
	if err := r.Get(ctx, req.NamespacedName, bucket); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch R2Bucket")
		return ctrl.Result{}, err
	}

	// Resolve credentials and account ID
	credRef, accountID, err := r.resolveCredentials(ctx, bucket)
	if err != nil {
		logger.Error(err, "Failed to resolve credentials")
		return r.updateStatusError(ctx, bucket, err)
	}

	// Handle deletion
	if !bucket.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, bucket, credRef)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(bucket, finalizerName) {
		controllerutil.AddFinalizer(bucket, finalizerName)
		if err := r.Update(ctx, bucket); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Register R2 bucket configuration to SyncState
	return r.registerBucket(ctx, bucket, accountID, credRef)
}

// resolveCredentials resolves the credentials reference and account ID.
func (r *Reconciler) resolveCredentials(
	ctx context.Context,
	bucket *networkingv1alpha2.R2Bucket,
) (credRef networkingv1alpha2.CredentialsReference, accountID string, err error) {
	logger := log.FromContext(ctx)

	// Get credentials reference
	if bucket.Spec.CredentialsRef != nil {
		credRef = networkingv1alpha2.CredentialsReference{
			Name: bucket.Spec.CredentialsRef.Name,
		}
	}

	// Get account ID from credentials
	cfCredRef := &networkingv1alpha2.CloudflareCredentialsRef{Name: credRef.Name}
	apiClient, err := cf.NewAPIClientFromCredentialsRef(ctx, r.Client, cfCredRef)
	if err != nil {
		return credRef, "", fmt.Errorf("failed to create API client: %w", err)
	}

	accountID = apiClient.AccountId
	if accountID == "" {
		logger.V(1).Info("Account ID not available from credentials, will be resolved during sync")
	}

	return credRef, accountID, nil
}

// handleDeletion handles the deletion of R2Bucket
//
//nolint:revive // cognitive complexity is acceptable for deletion handling
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	bucket *networkingv1alpha2.R2Bucket,
	credRef networkingv1alpha2.CredentialsReference,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(bucket, finalizerName) {
		return ctrl.Result{}, nil
	}

	// Check deletion policy
	if bucket.Spec.DeletionPolicy == "Orphan" {
		logger.Info("Deletion policy is Orphan, skipping bucket deletion")
	} else if bucket.Status.BucketName != "" {
		// Delete bucket from Cloudflare
		cfCredRef := &networkingv1alpha2.CloudflareCredentialsRef{Name: credRef.Name}
		cfAPI, err := cf.NewAPIClientFromCredentialsRef(ctx, r.Client, cfCredRef)
		if err == nil {
			if err := cfAPI.DeleteR2Bucket(ctx, bucket.Status.BucketName); err != nil {
				if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to delete R2 bucket")
					r.Recorder.Event(bucket, corev1.EventTypeWarning, "DeleteFailed", cf.SanitizeErrorMessage(err))
					return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
				}
			} else {
				logger.Info("R2 bucket deleted", "bucketName", bucket.Status.BucketName)
				r.Recorder.Event(bucket, corev1.EventTypeNormal, "Deleted",
					fmt.Sprintf("R2 bucket %s deleted", bucket.Status.BucketName))
			}
		}
	}

	// Unregister from SyncState
	source := service.Source{
		Kind:      "R2Bucket",
		Namespace: bucket.Namespace,
		Name:      bucket.Name,
	}
	if err := r.bucketService.Unregister(ctx, bucket.Status.BucketName, source); err != nil {
		logger.Error(err, "Failed to unregister R2 bucket from SyncState")
		// Non-fatal - continue with finalizer removal
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, bucket, func() {
		controllerutil.RemoveFinalizer(bucket, finalizerName)
	}); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(bucket, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")
	return ctrl.Result{}, nil
}

// registerBucket registers the R2 bucket configuration to SyncState.
// The actual sync to Cloudflare is handled by R2BucketSyncController.
func (r *Reconciler) registerBucket(
	ctx context.Context,
	bucket *networkingv1alpha2.R2Bucket,
	accountID string,
	credRef networkingv1alpha2.CredentialsReference,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Determine bucket name
	bucketName := bucket.Spec.Name
	if bucketName == "" {
		bucketName = bucket.Name
	}

	// Build bucket configuration
	config := r2svc.R2BucketConfig{
		Name:         bucketName,
		LocationHint: string(bucket.Spec.LocationHint),
	}

	// Build CORS rules
	if len(bucket.Spec.CORS) > 0 {
		config.CORS = bucket.Spec.CORS
	}

	// Build lifecycle rules
	if len(bucket.Spec.Lifecycle) > 0 {
		config.Lifecycle = &r2svc.R2LifecycleConfig{
			Rules:          bucket.Spec.Lifecycle,
			DeletionPolicy: bucket.Spec.DeletionPolicy,
		}
	}

	// Create source reference
	source := service.Source{
		Kind:      "R2Bucket",
		Namespace: bucket.Namespace,
		Name:      bucket.Name,
	}

	// Register to SyncState
	opts := r2svc.R2BucketRegisterOptions{
		AccountID:      accountID,
		BucketName:     bucket.Status.BucketName, // May be empty for new buckets
		Source:         source,
		Config:         config,
		CredentialsRef: credRef,
	}

	if err := r.bucketService.Register(ctx, opts); err != nil {
		logger.Error(err, "Failed to register R2 bucket configuration")
		r.Recorder.Event(bucket, corev1.EventTypeWarning, "RegisterFailed",
			fmt.Sprintf("Failed to register R2 bucket: %s", err.Error()))
		return r.updateStatusError(ctx, bucket, err)
	}

	r.Recorder.Event(bucket, corev1.EventTypeNormal, "Registered",
		fmt.Sprintf("Registered R2 Bucket '%s' configuration to SyncState", bucketName))

	// Update status to Pending - actual sync happens via R2BucketSyncController
	return r.updateStatusPending(ctx, bucket)
}

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	bucket *networkingv1alpha2.R2Bucket,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, bucket, func() {
		bucket.Status.State = networkingv1alpha2.R2BucketStateError
		bucket.Status.Message = cf.SanitizeErrorMessage(err)
		meta.SetStatusCondition(&bucket.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: bucket.Generation,
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		bucket.Status.ObservedGeneration = bucket.Generation
	})

	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *Reconciler) updateStatusPending(
	ctx context.Context,
	bucket *networkingv1alpha2.R2Bucket,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, bucket, func() {
		bucket.Status.State = networkingv1alpha2.R2BucketStateCreating
		bucket.Status.Message = "R2 bucket configuration registered, waiting for sync"
		meta.SetStatusCondition(&bucket.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: bucket.Generation,
			Reason:             "Pending",
			Message:            "R2 bucket configuration registered, waiting for sync",
			LastTransitionTime: metav1.Now(),
		})
		bucket.Status.ObservedGeneration = bucket.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	// Requeue to check for sync completion
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// findBucketsForCredentials returns R2Buckets that reference the given credentials
//
//nolint:revive // cognitive complexity is acceptable for watch handler
func (r *Reconciler) findBucketsForCredentials(ctx context.Context, obj client.Object) []reconcile.Request {
	creds, ok := obj.(*networkingv1alpha2.CloudflareCredentials)
	if !ok {
		return nil
	}

	bucketList := &networkingv1alpha2.R2BucketList{}
	if err := r.List(ctx, bucketList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, bucket := range bucketList.Items {
		if bucket.Spec.CredentialsRef != nil && bucket.Spec.CredentialsRef.Name == creds.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      bucket.Name,
					Namespace: bucket.Namespace,
				},
			})
		}

		if creds.Spec.IsDefault && bucket.Spec.CredentialsRef == nil {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      bucket.Name,
					Namespace: bucket.Namespace,
				},
			})
		}
	}

	return requests
}

// findBucketsForSyncState returns R2Buckets that are sources for the given SyncState
func (*Reconciler) findBucketsForSyncState(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	syncState, ok := obj.(*networkingv1alpha2.CloudflareSyncState)
	if !ok {
		return nil
	}

	// Only process R2Bucket type SyncStates
	if syncState.Spec.ResourceType != networkingv1alpha2.SyncResourceR2Bucket {
		return nil
	}

	var requests []reconcile.Request
	for _, source := range syncState.Spec.Sources {
		if source.Ref.Kind == "R2Bucket" {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      source.Ref.Name,
					Namespace: source.Ref.Namespace,
				},
			})
		}
	}

	logger.V(1).Info("Found R2Buckets for SyncState update",
		"syncState", syncState.Name,
		"bucketCount", len(requests))

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("r2bucket-controller")

	// Initialize BucketService
	r.bucketService = r2svc.NewBucketService(mgr.GetClient())

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.R2Bucket{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findBucketsForCredentials)).
		Watches(&networkingv1alpha2.CloudflareSyncState{},
			handler.EnqueueRequestsFromMapFunc(r.findBucketsForSyncState)).
		Named("r2bucket").
		Complete(r)
}
