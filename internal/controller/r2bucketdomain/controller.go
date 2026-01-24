// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package r2bucketdomain provides a controller for managing Cloudflare R2 bucket custom domains.
// It directly calls Cloudflare API and writes status back to the CRD.
package r2bucketdomain

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
	finalizerName = "cloudflare.com/r2-bucket-domain-finalizer"
)

// Reconciler reconciles an R2BucketDomain object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=r2bucketdomains,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=r2bucketdomains/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=r2bucketdomains/finalizers,verbs=update

// Reconcile handles R2BucketDomain reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the R2BucketDomain resource
	domain := &networkingv1alpha2.R2BucketDomain{}
	if err := r.Get(ctx, req.NamespacedName, domain); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch R2BucketDomain")
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !domain.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, domain)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, domain, finalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CredentialsRef: domain.Spec.CredentialsRef,
		Namespace:      domain.Namespace,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, domain, err)
	}

	// Sync domain to Cloudflare
	return r.syncDomain(ctx, domain, apiResult)
}

// handleDeletion handles the deletion of R2BucketDomain.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	domain *networkingv1alpha2.R2BucketDomain,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(domain, finalizerName) {
		return common.NoRequeue(), nil
	}

	// Get API client and delete from Cloudflare
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CredentialsRef: domain.Spec.CredentialsRef,
		Namespace:      domain.Namespace,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client for deletion")
		// Continue with finalizer removal
	} else if domain.Spec.Domain != "" && domain.Spec.BucketName != "" {
		// Delete custom domain from Cloudflare
		logger.Info("Deleting R2 custom domain from Cloudflare",
			"bucketName", domain.Spec.BucketName,
			"domain", domain.Spec.Domain)

		if err := apiResult.API.DeleteR2CustomDomain(ctx, domain.Spec.BucketName, domain.Spec.Domain); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete R2 custom domain from Cloudflare")
				r.Recorder.Event(domain, corev1.EventTypeWarning, "DeleteFailed",
					fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
				return common.RequeueShort(), err
			}
			logger.Info("R2 custom domain not found in Cloudflare, may have been already deleted")
		}

		r.Recorder.Event(domain, corev1.EventTypeNormal, "Deleted",
			"R2 custom domain deleted from Cloudflare")
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, domain, func() {
		controllerutil.RemoveFinalizer(domain, finalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(domain, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// syncDomain syncs the R2 custom domain to Cloudflare.
func (r *Reconciler) syncDomain(
	ctx context.Context,
	domain *networkingv1alpha2.R2BucketDomain,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	bucketName := domain.Spec.BucketName
	domainName := domain.Spec.Domain

	// Check if custom domain already exists
	existing, err := apiResult.API.GetR2CustomDomain(ctx, bucketName, domainName)
	if err != nil {
		if !cf.IsNotFoundError(err) {
			logger.Error(err, "Failed to get R2 custom domain from Cloudflare")
			return r.updateStatusError(ctx, domain, err)
		}
		// Domain doesn't exist, will create
		existing = nil
	}

	// Build params
	params := cf.R2CustomDomainParams{
		Domain:  domainName,
		ZoneID:  domain.Spec.ZoneID,
		MinTLS:  string(domain.Spec.MinTLS),
		Enabled: true,
	}

	if existing != nil {
		// Domain exists, check if update is needed
		needsUpdate := existing.MinTLS != string(domain.Spec.MinTLS) ||
			existing.ZoneID != domain.Spec.ZoneID ||
			!existing.Enabled

		if needsUpdate {
			logger.Info("Updating R2 custom domain in Cloudflare",
				"bucketName", bucketName,
				"domain", domainName)

			result, err := apiResult.API.UpdateR2CustomDomain(ctx, bucketName, domainName, params)
			if err != nil {
				logger.Error(err, "Failed to update R2 custom domain")
				return r.updateStatusError(ctx, domain, err)
			}

			r.Recorder.Event(domain, corev1.EventTypeNormal, "Updated",
				fmt.Sprintf("R2 custom domain '%s' updated in Cloudflare", domainName))

			return r.updateStatusFromResult(ctx, domain, apiResult.AccountID, result)
		}

		// No changes needed, update status from existing
		logger.V(1).Info("R2 custom domain already exists and is up to date",
			"bucketName", bucketName,
			"domain", domainName)

		return r.updateStatusFromResult(ctx, domain, apiResult.AccountID, existing)
	}

	// Create new custom domain
	logger.Info("Attaching R2 custom domain to bucket",
		"bucketName", bucketName,
		"domain", domainName)

	result, err := apiResult.API.AttachR2CustomDomain(ctx, bucketName, params)
	if err != nil {
		// Check if it's a conflict (domain already attached)
		if cf.IsConflictError(err) {
			existing, getErr := apiResult.API.GetR2CustomDomain(ctx, bucketName, domainName)
			if getErr == nil && existing != nil {
				logger.Info("R2 custom domain already attached, adopting it",
					"bucketName", bucketName,
					"domain", domainName)
				r.Recorder.Event(domain, corev1.EventTypeNormal, "Adopted",
					fmt.Sprintf("Adopted existing R2 custom domain '%s'", domainName))
				return r.updateStatusFromResult(ctx, domain, apiResult.AccountID, existing)
			}
		}
		logger.Error(err, "Failed to attach R2 custom domain")
		return r.updateStatusError(ctx, domain, err)
	}

	r.Recorder.Event(domain, corev1.EventTypeNormal, "Created",
		fmt.Sprintf("R2 custom domain '%s' attached to bucket '%s'", domainName, bucketName))

	// Handle public access if specified
	if domain.Spec.EnablePublicAccess {
		if err := apiResult.API.EnableR2PublicAccess(ctx, bucketName, true); err != nil {
			logger.Error(err, "Failed to enable public access for R2 bucket")
			// Continue - domain was created successfully
		}
	}

	return r.updateStatusFromResult(ctx, domain, apiResult.AccountID, result)
}

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	domain *networkingv1alpha2.R2BucketDomain,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, domain, func() {
		domain.Status.State = networkingv1alpha2.R2BucketDomainStateError
		domain.Status.Message = cf.SanitizeErrorMessage(err)
		meta.SetStatusCondition(&domain.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: domain.Generation,
			Reason:             "Error",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		domain.Status.ObservedGeneration = domain.Generation
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
}

func (r *Reconciler) updateStatusFromResult(
	ctx context.Context,
	domain *networkingv1alpha2.R2BucketDomain,
	_ string, // accountID - not stored in status
	result *cf.R2CustomDomain,
) (ctrl.Result, error) {
	// Check if domain is still pending
	isPending := result.Status.SSL == "pending" || result.Status.Ownership == "pending"

	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, domain, func() {
		domain.Status.DomainID = result.Domain // Use domain name as ID
		domain.Status.ZoneID = result.ZoneID
		domain.Status.Enabled = result.Enabled
		domain.Status.MinTLS = result.MinTLS
		domain.Status.PublicAccessEnabled = domain.Spec.EnablePublicAccess
		domain.Status.URL = fmt.Sprintf("https://%s", result.Domain)

		if isPending {
			domain.Status.State = networkingv1alpha2.R2BucketDomainStatePending
			domain.Status.Message = fmt.Sprintf("Domain is pending verification (SSL: %s, Ownership: %s)",
				result.Status.SSL, result.Status.Ownership)
			meta.SetStatusCondition(&domain.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				ObservedGeneration: domain.Generation,
				Reason:             "Pending",
				Message:            "Domain is pending verification",
				LastTransitionTime: metav1.Now(),
			})
		} else {
			domain.Status.State = networkingv1alpha2.R2BucketDomainStateActive
			domain.Status.Message = ""
			meta.SetStatusCondition(&domain.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				ObservedGeneration: domain.Generation,
				Reason:             "Synced",
				Message:            "R2 custom domain synced to Cloudflare",
				LastTransitionTime: metav1.Now(),
			})
		}
		domain.Status.ObservedGeneration = domain.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	// Continue polling if pending
	if isPending {
		return common.RequeueMedium(), nil
	}

	return common.NoRequeue(), nil
}

// findDomainsForCredentials returns R2BucketDomains that reference the given credentials
func (r *Reconciler) findDomainsForCredentials(ctx context.Context, obj client.Object) []reconcile.Request {
	creds, ok := obj.(*networkingv1alpha2.CloudflareCredentials)
	if !ok {
		return nil
	}

	domainList := &networkingv1alpha2.R2BucketDomainList{}
	if err := r.List(ctx, domainList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, domain := range domainList.Items {
		if domain.Spec.CredentialsRef != nil && domain.Spec.CredentialsRef.Name == creds.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      domain.Name,
					Namespace: domain.Namespace,
				},
			})
		}

		if creds.Spec.IsDefault && domain.Spec.CredentialsRef == nil {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      domain.Name,
					Namespace: domain.Namespace,
				},
			})
		}
	}

	return requests
}

// findDomainsForBucket returns R2BucketDomains that reference the given R2Bucket
func (r *Reconciler) findDomainsForBucket(ctx context.Context, obj client.Object) []reconcile.Request {
	bucket, ok := obj.(*networkingv1alpha2.R2Bucket)
	if !ok {
		return nil
	}

	domainList := &networkingv1alpha2.R2BucketDomainList{}
	if err := r.List(ctx, domainList, client.InNamespace(bucket.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, domain := range domainList.Items {
		// Match by bucket name (either spec.name or metadata.name)
		bucketName := bucket.Spec.Name
		if bucketName == "" {
			bucketName = bucket.Name
		}
		if domain.Spec.BucketName == bucketName {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      domain.Name,
					Namespace: domain.Namespace,
				},
			})
		}
	}

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("r2bucketdomain-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("r2bucketdomain"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.R2BucketDomain{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findDomainsForCredentials)).
		Watches(&networkingv1alpha2.R2Bucket{},
			handler.EnqueueRequestsFromMapFunc(r.findDomainsForBucket)).
		Named("r2bucketdomain").
		Complete(r)
}
