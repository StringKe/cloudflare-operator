// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package r2bucketdomain provides a controller for managing Cloudflare R2 bucket custom domains.
package r2bucketdomain

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
	finalizerName = "cloudflare.com/r2-bucket-domain-finalizer"

	// domainStatusActive represents an active domain status
	domainStatusActive = "active"
)

// Reconciler reconciles an R2BucketDomain object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Internal state
	ctx    context.Context
	log    logr.Logger
	domain *networkingv1alpha2.R2BucketDomain
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=r2bucketdomains,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=r2bucketdomains/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=r2bucketdomains/finalizers,verbs=update

// Reconcile handles R2BucketDomain reconciliation
//
//nolint:revive // cognitive complexity is acceptable for main reconcile loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrl.LoggerFrom(ctx)

	// Get the R2BucketDomain resource
	r.domain = &networkingv1alpha2.R2BucketDomain{}
	if err := r.Get(ctx, req.NamespacedName, r.domain); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch R2BucketDomain")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !r.domain.DeletionTimestamp.IsZero() {
		return r.handleDeletion()
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.domain, finalizerName) {
		controllerutil.AddFinalizer(r.domain, finalizerName)
		if err := r.Update(ctx, r.domain); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Create API client
	cfAPI, err := r.getAPIClient()
	if err != nil {
		r.updateState(networkingv1alpha2.R2BucketDomainStateError,
			fmt.Sprintf("Failed to get API client: %v", err))
		r.Recorder.Event(r.domain, corev1.EventTypeWarning,
			controller.EventReasonAPIError, cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Reconcile the domain
	return r.reconcileDomain(cfAPI)
}

// handleDeletion handles the deletion of R2BucketDomain
//
//nolint:revive // cognitive complexity is acceptable for deletion handling
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.domain, finalizerName) {
		return ctrl.Result{}, nil
	}

	// Remove the custom domain from Cloudflare
	if r.domain.Status.DomainID != "" || r.domain.Spec.Domain != "" {
		cfAPI, err := r.getAPIClient()
		if err == nil {
			domain := r.domain.Spec.Domain
			if err := cfAPI.DeleteR2CustomDomain(r.ctx, r.domain.Spec.BucketName, domain); err != nil {
				if !cf.IsNotFoundError(err) {
					r.log.Error(err, "Failed to delete R2 custom domain")
					r.Recorder.Event(r.domain, corev1.EventTypeWarning, "DeleteFailed",
						cf.SanitizeErrorMessage(err))
					return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
				}
			} else {
				r.log.Info("R2 custom domain deleted", "domain", domain)
				r.Recorder.Event(r.domain, corev1.EventTypeNormal, "Deleted",
					fmt.Sprintf("Custom domain %s removed from bucket %s",
						domain, r.domain.Spec.BucketName))
			}
		}
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.domain, func() {
		controllerutil.RemoveFinalizer(r.domain, finalizerName)
	}); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileDomain reconciles the R2 bucket custom domain
//
//nolint:revive // cognitive complexity is acceptable for reconcile logic
func (r *Reconciler) reconcileDomain(cfAPI *cf.API) (ctrl.Result, error) {
	// Check if domain already exists
	existing, err := cfAPI.GetR2CustomDomain(r.ctx, r.domain.Spec.BucketName, r.domain.Spec.Domain)
	if err != nil {
		if cf.IsNotFoundError(err) {
			// Domain doesn't exist, create it
			return r.createDomain(cfAPI)
		}
		r.updateState(networkingv1alpha2.R2BucketDomainStateError,
			fmt.Sprintf("Failed to get domain: %v", err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Domain exists, check if update needed
	return r.updateDomain(cfAPI, existing)
}

// createDomain creates a new custom domain
func (r *Reconciler) createDomain(cfAPI *cf.API) (ctrl.Result, error) {
	r.updateState(networkingv1alpha2.R2BucketDomainStateInitializing, "Creating custom domain")

	params := cf.R2CustomDomainParams{
		Domain:  r.domain.Spec.Domain,
		ZoneID:  r.domain.Spec.ZoneID,
		MinTLS:  string(r.domain.Spec.MinTLS),
		Enabled: true,
	}

	result, err := cfAPI.AttachR2CustomDomain(r.ctx, r.domain.Spec.BucketName, params)
	if err != nil {
		r.updateState(networkingv1alpha2.R2BucketDomainStateError,
			fmt.Sprintf("Failed to create domain: %v", err))
		r.Recorder.Event(r.domain, corev1.EventTypeWarning, controller.EventReasonAPIError,
			cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Update status
	r.domain.Status.DomainID = result.Domain
	r.domain.Status.ZoneID = result.ZoneID
	r.domain.Status.Enabled = result.Enabled
	r.domain.Status.MinTLS = result.MinTLS
	r.domain.Status.PublicAccessEnabled = r.domain.Spec.EnablePublicAccess
	r.domain.Status.URL = fmt.Sprintf("https://%s", r.domain.Spec.Domain)

	// Determine state based on status
	state := networkingv1alpha2.R2BucketDomainStateInitializing
	if result.Status.SSL == domainStatusActive && result.Status.Ownership == domainStatusActive {
		state = networkingv1alpha2.R2BucketDomainStateActive
	}

	r.updateState(state, "Custom domain configured")
	r.Recorder.Event(r.domain, corev1.EventTypeNormal, "Created",
		fmt.Sprintf("Custom domain %s attached to bucket %s",
			r.domain.Spec.Domain, r.domain.Spec.BucketName))

	// Requeue to check activation status
	if state == networkingv1alpha2.R2BucketDomainStateInitializing {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// updateDomain updates an existing custom domain if needed
//
//nolint:revive // cognitive complexity is acceptable for update logic
func (r *Reconciler) updateDomain(cfAPI *cf.API, existing *cf.R2CustomDomain) (ctrl.Result, error) {
	// Check if update is needed
	needsUpdate := string(r.domain.Spec.MinTLS) != existing.MinTLS

	if needsUpdate {
		params := cf.R2CustomDomainParams{
			Domain:  r.domain.Spec.Domain,
			MinTLS:  string(r.domain.Spec.MinTLS),
			Enabled: true,
		}

		result, err := cfAPI.UpdateR2CustomDomain(
			r.ctx, r.domain.Spec.BucketName, r.domain.Spec.Domain, params)
		if err != nil {
			r.updateState(networkingv1alpha2.R2BucketDomainStateError,
				fmt.Sprintf("Failed to update domain: %v", err))
			r.Recorder.Event(r.domain, corev1.EventTypeWarning, controller.EventReasonAPIError,
				cf.SanitizeErrorMessage(err))
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		existing = result
		r.Recorder.Event(r.domain, corev1.EventTypeNormal, "Updated",
			fmt.Sprintf("Custom domain %s settings updated", r.domain.Spec.Domain))
	}

	// Update status from existing
	r.domain.Status.DomainID = existing.Domain
	r.domain.Status.ZoneID = existing.ZoneID
	r.domain.Status.Enabled = existing.Enabled
	r.domain.Status.MinTLS = existing.MinTLS
	r.domain.Status.PublicAccessEnabled = r.domain.Spec.EnablePublicAccess
	r.domain.Status.URL = fmt.Sprintf("https://%s", r.domain.Spec.Domain)

	// Determine state based on status
	state := networkingv1alpha2.R2BucketDomainStateInitializing
	message := "Domain is initializing"
	if existing.Status.SSL == domainStatusActive && existing.Status.Ownership == domainStatusActive {
		state = networkingv1alpha2.R2BucketDomainStateActive
		message = "Domain is active"
	} else if existing.Status.SSL != "" || existing.Status.Ownership != "" {
		message = fmt.Sprintf("SSL: %s, Ownership: %s", existing.Status.SSL, existing.Status.Ownership)
	}

	r.updateState(state, message)

	// Requeue based on state
	if state == networkingv1alpha2.R2BucketDomainStateInitializing {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// getAPIClient creates a Cloudflare API client from credentials
func (r *Reconciler) getAPIClient() (*cf.API, error) {
	if r.domain.Spec.CredentialsRef != nil {
		ref := &networkingv1alpha2.CloudflareCredentialsRef{
			Name: r.domain.Spec.CredentialsRef.Name,
		}
		return cf.NewAPIClientFromCredentialsRef(r.ctx, r.Client, ref)
	}
	return cf.NewAPIClientFromDefaultCredentials(r.ctx, r.Client)
}

// updateState updates the state and status of the R2BucketDomain
func (r *Reconciler) updateState(state networkingv1alpha2.R2BucketDomainState, message string) {
	r.domain.Status.State = state
	r.domain.Status.Message = message
	r.domain.Status.ObservedGeneration = r.domain.Generation

	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		ObservedGeneration: r.domain.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(state),
		Message:            message,
	}

	if state == networkingv1alpha2.R2BucketDomainStateActive {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "DomainActive"
	}

	controller.SetCondition(&r.domain.Status.Conditions, condition.Type,
		condition.Status, condition.Reason, condition.Message)

	if err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.domain, func() {
		r.domain.Status.State = state
		r.domain.Status.Message = message
		r.domain.Status.ObservedGeneration = r.domain.Generation
		controller.SetCondition(&r.domain.Status.Conditions, condition.Type,
			condition.Status, condition.Reason, condition.Message)
	}); err != nil {
		r.log.Error(err, "failed to update status")
	}
}

// findDomainsForCredentials returns R2BucketDomains that reference the given credentials
//
//nolint:revive // cognitive complexity is acceptable for watch handler
func (r *Reconciler) findDomainsForCredentials(
	ctx context.Context, obj client.Object,
) []reconcile.Request {
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
func (r *Reconciler) findDomainsForBucket(
	ctx context.Context, obj client.Object,
) []reconcile.Request {
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
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.R2BucketDomain{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findDomainsForCredentials)).
		Watches(&networkingv1alpha2.R2Bucket{},
			handler.EnqueueRequestsFromMapFunc(r.findDomainsForBucket)).
		Named("r2bucketdomain").
		Complete(r)
}
