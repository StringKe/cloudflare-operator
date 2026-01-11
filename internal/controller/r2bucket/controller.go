// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package r2bucket provides a controller for managing Cloudflare R2 storage buckets.
package r2bucket

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
	finalizerName = "cloudflare.com/r2-bucket-finalizer"
)

// Reconciler reconciles an R2Bucket object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Internal state
	ctx    context.Context
	log    logr.Logger
	bucket *networkingv1alpha2.R2Bucket
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=r2buckets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=r2buckets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=r2buckets/finalizers,verbs=update

// Reconcile handles R2Bucket reconciliation
//
//nolint:revive // cognitive complexity is acceptable for main reconcile loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrl.LoggerFrom(ctx)

	// Get the R2Bucket resource
	r.bucket = &networkingv1alpha2.R2Bucket{}
	if err := r.Get(ctx, req.NamespacedName, r.bucket); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch R2Bucket")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !r.bucket.DeletionTimestamp.IsZero() {
		return r.handleDeletion()
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.bucket, finalizerName) {
		controllerutil.AddFinalizer(r.bucket, finalizerName)
		if err := r.Update(ctx, r.bucket); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Create API client
	cfAPI, err := r.getAPIClient()
	if err != nil {
		r.updateState(networkingv1alpha2.R2BucketStateError, fmt.Sprintf("Failed to get API client: %v", err))
		r.Recorder.Event(r.bucket, corev1.EventTypeWarning, controller.EventReasonAPIError, cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Check if bucket already exists
	if r.bucket.Status.BucketName != "" {
		return r.reconcileExisting(cfAPI)
	}

	// Create bucket
	return r.createBucket(cfAPI)
}

// handleDeletion handles the deletion of R2Bucket
//
//nolint:revive // cognitive complexity is acceptable for deletion handling
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.bucket, finalizerName) {
		return ctrl.Result{}, nil
	}

	// Check deletion policy
	if r.bucket.Spec.DeletionPolicy == "Orphan" {
		r.log.Info("Deletion policy is Orphan, skipping bucket deletion")
	} else if r.bucket.Status.BucketName != "" {
		// Delete bucket from Cloudflare
		cfAPI, err := r.getAPIClient()
		if err == nil {
			if err := cfAPI.DeleteR2Bucket(r.ctx, r.bucket.Status.BucketName); err != nil {
				if !cf.IsNotFoundError(err) {
					r.log.Error(err, "Failed to delete R2 bucket")
					r.Recorder.Event(r.bucket, corev1.EventTypeWarning, "DeleteFailed", cf.SanitizeErrorMessage(err))
					return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
				}
			} else {
				r.log.Info("R2 bucket deleted", "bucketName", r.bucket.Status.BucketName)
				r.Recorder.Event(r.bucket, corev1.EventTypeNormal, "Deleted",
					fmt.Sprintf("R2 bucket %s deleted", r.bucket.Status.BucketName))
			}
		}
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.bucket, func() {
		controllerutil.RemoveFinalizer(r.bucket, finalizerName)
	}); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileExisting handles an existing R2 bucket
//
//nolint:revive // cognitive complexity is acceptable for reconcile existing logic
func (r *Reconciler) reconcileExisting(cfAPI *cf.API) (ctrl.Result, error) {
	// Verify bucket still exists in Cloudflare
	bucket, err := cfAPI.GetR2Bucket(r.ctx, r.bucket.Status.BucketName)
	if err != nil {
		if cf.IsNotFoundError(err) {
			// Bucket was deleted externally, recreate it
			r.log.Info("R2 bucket not found in Cloudflare, recreating")
			r.bucket.Status.BucketName = ""
			return r.createBucket(cfAPI)
		}
		r.updateState(networkingv1alpha2.R2BucketStateError, fmt.Sprintf("Failed to get bucket: %v", err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Update status with current bucket info
	r.bucket.Status.Location = bucket.Location
	if !bucket.CreationDate.IsZero() {
		createdAt := metav1.NewTime(bucket.CreationDate)
		r.bucket.Status.CreatedAt = &createdAt
	}

	// Sync CORS configuration
	if err := r.syncCORS(cfAPI); err != nil {
		r.updateState(networkingv1alpha2.R2BucketStateError, fmt.Sprintf("Failed to sync CORS: %v", err))
		r.Recorder.Event(r.bucket, corev1.EventTypeWarning, "CORSSyncFailed", cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Sync Lifecycle configuration
	if err := r.syncLifecycle(cfAPI); err != nil {
		r.updateState(networkingv1alpha2.R2BucketStateError, fmt.Sprintf("Failed to sync lifecycle: %v", err))
		r.Recorder.Event(r.bucket, corev1.EventTypeWarning, "LifecycleSyncFailed", cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	r.updateState(networkingv1alpha2.R2BucketStateReady, "Bucket is ready")

	// Requeue periodically to verify bucket still exists
	return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
}

// createBucket creates a new R2 bucket
//
//nolint:revive // cognitive complexity is acceptable for bucket creation logic
func (r *Reconciler) createBucket(cfAPI *cf.API) (ctrl.Result, error) {
	r.updateState(networkingv1alpha2.R2BucketStateCreating, "Creating bucket")

	// Determine bucket name
	bucketName := r.bucket.Spec.Name
	if bucketName == "" {
		bucketName = r.bucket.Name
	}

	// Create bucket
	bucket, err := cfAPI.CreateR2Bucket(r.ctx, cf.R2BucketParams{
		Name:         bucketName,
		LocationHint: string(r.bucket.Spec.LocationHint),
	})
	if err != nil {
		// Check if bucket already exists (conflict)
		if cf.IsConflictError(err) {
			// Try to get existing bucket
			existing, getErr := cfAPI.GetR2Bucket(r.ctx, bucketName)
			if getErr == nil {
				// Adopt existing bucket
				r.bucket.Status.BucketName = existing.Name
				r.bucket.Status.Location = existing.Location
				if !existing.CreationDate.IsZero() {
					createdAt := metav1.NewTime(existing.CreationDate)
					r.bucket.Status.CreatedAt = &createdAt
				}
				r.updateState(networkingv1alpha2.R2BucketStateReady, "Adopted existing bucket")
				r.Recorder.Event(r.bucket, corev1.EventTypeNormal, "Adopted",
					fmt.Sprintf("Adopted existing R2 bucket %s", bucketName))
				return ctrl.Result{}, nil
			}
		}

		r.updateState(networkingv1alpha2.R2BucketStateError, fmt.Sprintf("Failed to create bucket: %v", err))
		r.Recorder.Event(r.bucket, corev1.EventTypeWarning, controller.EventReasonAPIError, cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Update status
	r.bucket.Status.BucketName = bucket.Name
	r.bucket.Status.Location = bucket.Location
	if !bucket.CreationDate.IsZero() {
		createdAt := metav1.NewTime(bucket.CreationDate)
		r.bucket.Status.CreatedAt = &createdAt
	}

	r.Recorder.Event(r.bucket, corev1.EventTypeNormal, "Created",
		fmt.Sprintf("R2 bucket %s created in %s", bucket.Name, bucket.Location))

	// Configure CORS if specified
	if err := r.syncCORS(cfAPI); err != nil {
		r.updateState(networkingv1alpha2.R2BucketStateError, fmt.Sprintf("Failed to configure CORS: %v", err))
		r.Recorder.Event(r.bucket, corev1.EventTypeWarning, "CORSConfigFailed", cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Configure Lifecycle if specified
	if err := r.syncLifecycle(cfAPI); err != nil {
		r.updateState(networkingv1alpha2.R2BucketStateError, fmt.Sprintf("Failed to configure lifecycle: %v", err))
		r.Recorder.Event(r.bucket, corev1.EventTypeWarning, "LifecycleConfigFailed", cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	r.updateState(networkingv1alpha2.R2BucketStateReady, "Bucket created successfully")

	return ctrl.Result{}, nil
}

// getAPIClient creates a Cloudflare API client from credentials
func (r *Reconciler) getAPIClient() (*cf.API, error) {
	if r.bucket.Spec.CredentialsRef != nil {
		ref := &networkingv1alpha2.CloudflareCredentialsRef{
			Name: r.bucket.Spec.CredentialsRef.Name,
		}
		return cf.NewAPIClientFromCredentialsRef(r.ctx, r.Client, ref)
	}
	return cf.NewAPIClientFromDefaultCredentials(r.ctx, r.Client)
}

// syncCORS synchronizes CORS configuration for the bucket
//
//nolint:revive // cognitive complexity is acceptable for CORS synchronization logic
func (r *Reconciler) syncCORS(cfAPI *cf.API) error {
	bucketName := r.bucket.Status.BucketName
	if bucketName == "" {
		return nil
	}

	// If no CORS rules specified, delete existing CORS config
	if len(r.bucket.Spec.CORS) == 0 {
		// Check if CORS exists and delete it
		existing, err := cfAPI.GetR2CORS(r.ctx, bucketName)
		if err != nil && !cf.IsNotFoundError(err) {
			return fmt.Errorf("failed to get CORS: %w", err)
		}
		if len(existing) > 0 {
			if err := cfAPI.DeleteR2CORS(r.ctx, bucketName); err != nil && !cf.IsNotFoundError(err) {
				return fmt.Errorf("failed to delete CORS: %w", err)
			}
			r.log.Info("CORS configuration deleted", "bucket", bucketName)
		}
		r.bucket.Status.CORSRulesCount = 0
		return nil
	}

	// Convert spec rules to API rules
	rules := make([]cf.R2CORSRule, len(r.bucket.Spec.CORS))
	for i, rule := range r.bucket.Spec.CORS {
		rules[i] = cf.R2CORSRule{
			ID:             rule.ID,
			AllowedOrigins: rule.AllowedOrigins,
			AllowedMethods: rule.AllowedMethods,
			AllowedHeaders: rule.AllowedHeaders,
			ExposeHeaders:  rule.ExposeHeaders,
			MaxAgeSeconds:  rule.MaxAgeSeconds,
		}
	}

	// Set CORS configuration
	if err := cfAPI.SetR2CORS(r.ctx, bucketName, rules); err != nil {
		return fmt.Errorf("failed to set CORS: %w", err)
	}

	r.bucket.Status.CORSRulesCount = len(rules)
	r.log.Info("CORS configuration updated", "bucket", bucketName, "rulesCount", len(rules))
	return nil
}

// syncLifecycle synchronizes lifecycle rules for the bucket
//
//nolint:revive // cognitive complexity is acceptable for lifecycle synchronization logic
func (r *Reconciler) syncLifecycle(cfAPI *cf.API) error {
	bucketName := r.bucket.Status.BucketName
	if bucketName == "" {
		return nil
	}

	// If no lifecycle rules specified, delete existing lifecycle config
	if len(r.bucket.Spec.Lifecycle) == 0 {
		// Check if lifecycle exists and delete it
		existing, err := cfAPI.GetR2Lifecycle(r.ctx, bucketName)
		if err != nil && !cf.IsNotFoundError(err) {
			return fmt.Errorf("failed to get lifecycle: %w", err)
		}
		if len(existing) > 0 {
			if err := cfAPI.DeleteR2Lifecycle(r.ctx, bucketName); err != nil && !cf.IsNotFoundError(err) {
				return fmt.Errorf("failed to delete lifecycle: %w", err)
			}
			r.log.Info("Lifecycle configuration deleted", "bucket", bucketName)
		}
		r.bucket.Status.LifecycleRulesCount = 0
		return nil
	}

	// Convert spec rules to API rules
	rules := make([]cf.R2LifecycleRule, len(r.bucket.Spec.Lifecycle))
	for i, rule := range r.bucket.Spec.Lifecycle {
		apiRule := cf.R2LifecycleRule{
			ID:      rule.ID,
			Enabled: rule.Enabled,
			Prefix:  rule.Prefix,
		}

		if rule.Expiration != nil {
			apiRule.Expiration = &cf.R2LifecycleExpiration{
				Days: rule.Expiration.Days,
				Date: rule.Expiration.Date,
			}
		}

		if rule.AbortIncompleteMultipartUpload != nil {
			apiRule.AbortIncompleteMultipartUpload = &cf.R2LifecycleAbortUpload{
				DaysAfterInitiation: rule.AbortIncompleteMultipartUpload.DaysAfterInitiation,
			}
		}

		rules[i] = apiRule
	}

	// Set lifecycle configuration
	if err := cfAPI.SetR2Lifecycle(r.ctx, bucketName, rules); err != nil {
		return fmt.Errorf("failed to set lifecycle: %w", err)
	}

	r.bucket.Status.LifecycleRulesCount = len(rules)
	r.log.Info("Lifecycle configuration updated", "bucket", bucketName, "rulesCount", len(rules))
	return nil
}

// updateState updates the state and status of the R2Bucket
func (r *Reconciler) updateState(state networkingv1alpha2.R2BucketState, message string) {
	r.bucket.Status.State = state
	r.bucket.Status.Message = message
	r.bucket.Status.ObservedGeneration = r.bucket.Generation

	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		ObservedGeneration: r.bucket.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(state),
		Message:            message,
	}

	if state == networkingv1alpha2.R2BucketStateReady {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "BucketReady"
	}

	controller.SetCondition(&r.bucket.Status.Conditions, condition.Type, condition.Status, condition.Reason, condition.Message)

	if err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.bucket, func() {
		r.bucket.Status.State = state
		r.bucket.Status.Message = message
		r.bucket.Status.ObservedGeneration = r.bucket.Generation
		controller.SetCondition(&r.bucket.Status.Conditions, condition.Type, condition.Status, condition.Reason, condition.Message)
	}); err != nil {
		r.log.Error(err, "failed to update status")
	}
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

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.R2Bucket{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findBucketsForCredentials)).
		Named("r2bucket").
		Complete(r)
}
