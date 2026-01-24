// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package r2bucket provides a controller for managing Cloudflare R2 storage buckets.
// It directly calls Cloudflare API and writes status back to the CRD.
package r2bucket

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
	finalizerName = "cloudflare.com/r2-bucket-finalizer"
)

// Reconciler reconciles an R2Bucket object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
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
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch R2Bucket")
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !bucket.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, bucket)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, bucket, finalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CredentialsRef: bucket.Spec.CredentialsRef,
		Namespace:      bucket.Namespace,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, bucket, err)
	}

	// Sync bucket to Cloudflare
	return r.syncBucket(ctx, bucket, apiResult)
}

// handleDeletion handles the deletion of R2Bucket.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	bucket *networkingv1alpha2.R2Bucket,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(bucket, finalizerName) {
		return common.NoRequeue(), nil
	}

	// Check deletion policy
	if bucket.Spec.DeletionPolicy == "Orphan" {
		logger.Info("Orphan deletion policy, skipping Cloudflare deletion")
	} else {
		// Get API client
		apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
			CredentialsRef: bucket.Spec.CredentialsRef,
			Namespace:      bucket.Namespace,
		})
		if err != nil {
			logger.Error(err, "Failed to get API client for deletion")
			// Continue with finalizer removal
		} else if bucket.Status.BucketName != "" {
			// Delete bucket from Cloudflare
			logger.Info("Deleting R2 bucket from Cloudflare",
				"bucketName", bucket.Status.BucketName)

			if err := apiResult.API.DeleteR2Bucket(ctx, bucket.Status.BucketName); err != nil {
				if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to delete R2 bucket from Cloudflare")
					r.Recorder.Event(bucket, corev1.EventTypeWarning, "DeleteFailed",
						fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
					return common.RequeueShort(), err
				}
				logger.Info("R2 bucket not found in Cloudflare, may have been already deleted")
			}

			r.Recorder.Event(bucket, corev1.EventTypeNormal, "Deleted",
				"R2 bucket deleted from Cloudflare")
		}
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, bucket, func() {
		controllerutil.RemoveFinalizer(bucket, finalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(bucket, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// syncBucket syncs the R2 bucket to Cloudflare.
func (r *Reconciler) syncBucket(
	ctx context.Context,
	bucket *networkingv1alpha2.R2Bucket,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Determine bucket name
	bucketName := bucket.Spec.Name
	if bucketName == "" {
		bucketName = bucket.Name
	}

	// Check if bucket exists
	existing, err := apiResult.API.GetR2Bucket(ctx, bucketName)
	if err != nil {
		if !cf.IsNotFoundError(err) {
			logger.Error(err, "Failed to get R2 bucket from Cloudflare")
			return r.updateStatusError(ctx, bucket, err)
		}
		// Bucket doesn't exist, create it
		existing = nil
	}

	if existing != nil {
		// Bucket exists, update status
		logger.V(1).Info("R2 bucket already exists in Cloudflare",
			"bucketName", bucketName,
			"location", existing.Location)
		return r.updateStatusReady(ctx, bucket, apiResult.AccountID, existing)
	}

	// Create new bucket
	logger.Info("Creating R2 bucket in Cloudflare",
		"bucketName", bucketName,
		"locationHint", bucket.Spec.LocationHint)

	params := cf.R2BucketParams{
		Name:         bucketName,
		LocationHint: string(bucket.Spec.LocationHint),
	}

	result, err := apiResult.API.CreateR2Bucket(ctx, params)
	if err != nil {
		// Check if it's a conflict (bucket already exists)
		if cf.IsConflictError(err) {
			// Try to get the existing bucket
			existing, getErr := apiResult.API.GetR2Bucket(ctx, bucketName)
			if getErr == nil && existing != nil {
				logger.Info("R2 bucket already exists, adopting it",
					"bucketName", bucketName)
				r.Recorder.Event(bucket, corev1.EventTypeNormal, "Adopted",
					fmt.Sprintf("Adopted existing R2 bucket '%s'", bucketName))
				return r.updateStatusReady(ctx, bucket, apiResult.AccountID, existing)
			}
		}
		logger.Error(err, "Failed to create R2 bucket")
		return r.updateStatusError(ctx, bucket, err)
	}

	r.Recorder.Event(bucket, corev1.EventTypeNormal, "Created",
		fmt.Sprintf("R2 bucket '%s' created in Cloudflare", bucketName))

	return r.updateStatusReady(ctx, bucket, apiResult.AccountID, result)
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
			Reason:             "Error",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		bucket.Status.ObservedGeneration = bucket.Generation
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
}

func (r *Reconciler) updateStatusReady(
	ctx context.Context,
	bucket *networkingv1alpha2.R2Bucket,
	_ string, // accountID - not stored in status
	result *cf.R2BucketResult,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, bucket, func() {
		bucket.Status.BucketName = result.Name
		bucket.Status.Location = result.Location
		bucket.Status.State = networkingv1alpha2.R2BucketStateReady
		bucket.Status.Message = ""
		meta.SetStatusCondition(&bucket.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: bucket.Generation,
			Reason:             "Synced",
			Message:            "R2 bucket synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		bucket.Status.ObservedGeneration = bucket.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
}

// findBucketsForCredentials returns R2Buckets that reference the given credentials
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
	r.Recorder = mgr.GetEventRecorderFor("r2bucket-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("r2bucket"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.R2Bucket{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findBucketsForCredentials)).
		Named("r2bucket").
		Complete(r)
}
