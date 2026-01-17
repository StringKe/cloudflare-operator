// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package r2bucketdomain provides a controller for managing Cloudflare R2 bucket custom domains.
package r2bucketdomain

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
	finalizerName = "cloudflare.com/r2-bucket-domain-finalizer"
)

// Reconciler reconciles an R2BucketDomain object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Services
	domainService *r2svc.DomainService
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
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch R2BucketDomain")
		return ctrl.Result{}, err
	}

	// Resolve credentials and account ID
	creds, err := r.resolveCredentials(ctx, domain)
	if err != nil {
		logger.Error(err, "Failed to resolve credentials")
		return r.updateStatusError(ctx, domain, err)
	}
	credRef := networkingv1alpha2.CredentialsReference{Name: creds.CredentialsName}

	// Handle deletion
	if !domain.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, domain, credRef)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(domain, finalizerName) {
		controllerutil.AddFinalizer(domain, finalizerName)
		if err := r.Update(ctx, domain); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Register R2 bucket domain configuration to SyncState
	return r.registerDomain(ctx, domain, creds.AccountID, creds.ZoneID, credRef)
}

// resolveCredentials resolves the credentials reference, account ID and zone ID.
func (r *Reconciler) resolveCredentials(
	ctx context.Context,
	domain *networkingv1alpha2.R2BucketDomain,
) (controller.CredentialsResult, error) {
	logger := log.FromContext(ctx)

	var result controller.CredentialsResult

	// Get credentials reference
	if domain.Spec.CredentialsRef != nil {
		result.CredentialsName = domain.Spec.CredentialsRef.Name
	}

	// Get zone ID from spec
	result.ZoneID = domain.Spec.ZoneID

	// Get account ID from credentials
	cfCredRef := &networkingv1alpha2.CloudflareCredentialsRef{Name: result.CredentialsName}
	apiClient, err := cf.NewAPIClientFromCredentialsRef(ctx, r.Client, cfCredRef)
	if err != nil {
		return result, fmt.Errorf("failed to create API client: %w", err)
	}

	result.AccountID = apiClient.AccountId
	if result.AccountID == "" {
		logger.V(1).Info("Account ID not available from credentials, will be resolved during sync")
	}

	return result, nil
}

// handleDeletion handles the deletion of R2BucketDomain
//
//nolint:revive // cognitive complexity is acceptable for deletion handling
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	domain *networkingv1alpha2.R2BucketDomain,
	credRef networkingv1alpha2.CredentialsReference,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(domain, finalizerName) {
		return ctrl.Result{}, nil
	}

	// Remove the custom domain from Cloudflare
	if domain.Status.DomainID != "" || domain.Spec.Domain != "" {
		cfCredRef := &networkingv1alpha2.CloudflareCredentialsRef{Name: credRef.Name}
		cfAPI, err := cf.NewAPIClientFromCredentialsRef(ctx, r.Client, cfCredRef)
		if err == nil {
			domainName := domain.Spec.Domain
			if err := cfAPI.DeleteR2CustomDomain(ctx, domain.Spec.BucketName, domainName); err != nil {
				if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to delete R2 custom domain")
					r.Recorder.Event(domain, corev1.EventTypeWarning, "DeleteFailed",
						cf.SanitizeErrorMessage(err))
					return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
				}
			} else {
				logger.Info("R2 custom domain deleted", "domain", domainName)
				r.Recorder.Event(domain, corev1.EventTypeNormal, "Deleted",
					fmt.Sprintf("Custom domain %s removed from bucket %s",
						domainName, domain.Spec.BucketName))
			}
		}
	}

	// Unregister from SyncState
	source := service.Source{
		Kind:      "R2BucketDomain",
		Namespace: domain.Namespace,
		Name:      domain.Name,
	}
	if err := r.domainService.Unregister(ctx, domain.Status.DomainID, source); err != nil {
		logger.Error(err, "Failed to unregister R2 bucket domain from SyncState")
		// Non-fatal - continue with finalizer removal
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, domain, func() {
		controllerutil.RemoveFinalizer(domain, finalizerName)
	}); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(domain, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")
	return ctrl.Result{}, nil
}

// registerDomain registers the R2 bucket domain configuration to SyncState.
// The actual sync to Cloudflare is handled by R2BucketDomainSyncController.
func (r *Reconciler) registerDomain(
	ctx context.Context,
	domain *networkingv1alpha2.R2BucketDomain,
	accountID, zoneID string,
	credRef networkingv1alpha2.CredentialsReference,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Build domain configuration
	config := r2svc.R2BucketDomainConfig{
		BucketName:         domain.Spec.BucketName,
		Domain:             domain.Spec.Domain,
		ZoneID:             zoneID,
		MinTLS:             string(domain.Spec.MinTLS),
		EnablePublicAccess: &domain.Spec.EnablePublicAccess,
	}

	// Create source reference
	source := service.Source{
		Kind:      "R2BucketDomain",
		Namespace: domain.Namespace,
		Name:      domain.Name,
	}

	// Register to SyncState
	opts := r2svc.R2BucketDomainRegisterOptions{
		AccountID:      accountID,
		DomainID:       domain.Status.DomainID, // May be empty for new domains
		ZoneID:         zoneID,
		Source:         source,
		Config:         config,
		CredentialsRef: credRef,
	}

	if err := r.domainService.Register(ctx, opts); err != nil {
		logger.Error(err, "Failed to register R2 bucket domain configuration")
		r.Recorder.Event(domain, corev1.EventTypeWarning, "RegisterFailed",
			fmt.Sprintf("Failed to register R2 bucket domain: %s", err.Error()))
		return r.updateStatusError(ctx, domain, err)
	}

	r.Recorder.Event(domain, corev1.EventTypeNormal, "Registered",
		fmt.Sprintf("Registered R2 Bucket Domain '%s' configuration to SyncState", domain.Spec.Domain))

	// Update status to Pending - actual sync happens via R2BucketDomainSyncController
	return r.updateStatusPending(ctx, domain)
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
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		domain.Status.ObservedGeneration = domain.Generation
	})

	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *Reconciler) updateStatusPending(
	ctx context.Context,
	domain *networkingv1alpha2.R2BucketDomain,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, domain, func() {
		domain.Status.State = networkingv1alpha2.R2BucketDomainStateInitializing
		domain.Status.Message = "R2 bucket domain configuration registered, waiting for sync"
		meta.SetStatusCondition(&domain.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: domain.Generation,
			Reason:             "Pending",
			Message:            "R2 bucket domain configuration registered, waiting for sync",
			LastTransitionTime: metav1.Now(),
		})
		domain.Status.ObservedGeneration = domain.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	// Requeue to check for sync completion
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
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

// findDomainsForSyncState returns R2BucketDomains that are sources for the given SyncState
func (*Reconciler) findDomainsForSyncState(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	syncState, ok := obj.(*networkingv1alpha2.CloudflareSyncState)
	if !ok {
		return nil
	}

	// Only process R2BucketDomain type SyncStates
	if syncState.Spec.ResourceType != networkingv1alpha2.SyncResourceR2BucketDomain {
		return nil
	}

	var requests []reconcile.Request
	for _, source := range syncState.Spec.Sources {
		if source.Ref.Kind == "R2BucketDomain" {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      source.Ref.Name,
					Namespace: source.Ref.Namespace,
				},
			})
		}
	}

	logger.V(1).Info("Found R2BucketDomains for SyncState update",
		"syncState", syncState.Name,
		"domainCount", len(requests))

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("r2bucketdomain-controller")

	// Initialize DomainService
	r.domainService = r2svc.NewDomainService(mgr.GetClient())

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.R2BucketDomain{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findDomainsForCredentials)).
		Watches(&networkingv1alpha2.R2Bucket{},
			handler.EnqueueRequestsFromMapFunc(r.findDomainsForBucket)).
		Watches(&networkingv1alpha2.CloudflareSyncState{},
			handler.EnqueueRequestsFromMapFunc(r.findDomainsForSyncState)).
		Named("r2bucketdomain").
		Complete(r)
}
