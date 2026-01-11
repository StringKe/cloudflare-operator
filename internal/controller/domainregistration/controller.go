// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package domainregistration provides a controller for managing Cloudflare Registrar domains.
package domainregistration

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
	finalizerName = "cloudflare.com/domain-registration-finalizer"
)

// Reconciler reconciles a DomainRegistration object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Internal state
	ctx    context.Context
	log    logr.Logger
	domain *networkingv1alpha2.DomainRegistration
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=domainregistrations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=domainregistrations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=domainregistrations/finalizers,verbs=update

// Reconcile handles DomainRegistration reconciliation
//
//nolint:revive // cognitive complexity is acceptable for main reconcile loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrl.LoggerFrom(ctx)

	// Get the DomainRegistration resource
	r.domain = &networkingv1alpha2.DomainRegistration{}
	if err := r.Get(ctx, req.NamespacedName, r.domain); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch DomainRegistration")
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
		r.updateState(networkingv1alpha2.DomainRegistrationStateError,
			fmt.Sprintf("Failed to get API client: %v", err))
		r.Recorder.Event(r.domain, corev1.EventTypeWarning,
			controller.EventReasonAPIError, cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Sync domain
	return r.syncDomain(cfAPI)
}

// handleDeletion handles the deletion of DomainRegistration
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.domain, finalizerName) {
		return ctrl.Result{}, nil
	}

	// DomainRegistration is read-only for the domain itself
	// We don't delete the domain from Cloudflare, just remove the finalizer
	r.log.Info("DomainRegistration being deleted, removing finalizer only (domain not deleted from Cloudflare)")

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.domain, func() {
		controllerutil.RemoveFinalizer(r.domain, finalizerName)
	}); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// syncDomain syncs the domain configuration with Cloudflare
//
//nolint:revive // cognitive complexity is acceptable for sync logic
func (r *Reconciler) syncDomain(cfAPI *cf.API) (ctrl.Result, error) {
	r.updateState(networkingv1alpha2.DomainRegistrationStateSyncing, "Syncing domain information")

	// Get domain information from Cloudflare
	domainInfo, err := cfAPI.GetRegistrarDomain(r.ctx, r.domain.Spec.DomainName)
	if err != nil {
		r.updateState(networkingv1alpha2.DomainRegistrationStateError,
			fmt.Sprintf("Failed to get domain: %v", err))
		r.Recorder.Event(r.domain, corev1.EventTypeWarning,
			controller.EventReasonAPIError, cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Update status with domain information
	r.domain.Status.DomainID = domainInfo.ID
	r.domain.Status.CurrentRegistrar = domainInfo.CurrentRegistrar
	r.domain.Status.RegistryStatuses = domainInfo.RegistryStatuses
	r.domain.Status.Locked = domainInfo.Locked
	r.domain.Status.TransferInStatus = domainInfo.TransferInStatus

	if !domainInfo.ExpiresAt.IsZero() {
		r.domain.Status.ExpiresAt = &metav1.Time{Time: domainInfo.ExpiresAt}
	}
	if !domainInfo.CreatedAt.IsZero() {
		r.domain.Status.CreatedAt = &metav1.Time{Time: domainInfo.CreatedAt}
	}

	// Check if configuration update is needed
	if r.domain.Spec.Configuration != nil {
		needsUpdate := false
		config := cf.RegistrarDomainConfig{
			AutoRenew: r.domain.Spec.Configuration.AutoRenew,
			Privacy:   r.domain.Spec.Configuration.Privacy,
			Locked:    r.domain.Spec.Configuration.Locked,
		}

		if len(r.domain.Spec.Configuration.NameServers) > 0 {
			config.NameServers = r.domain.Spec.Configuration.NameServers
		}

		// Check if update is needed (simplified comparison)
		// In production, you would compare with the current values
		needsUpdate = true

		if needsUpdate {
			updatedInfo, err := cfAPI.UpdateRegistrarDomain(r.ctx, r.domain.Spec.DomainName, config)
			if err != nil {
				r.updateState(networkingv1alpha2.DomainRegistrationStateError,
					fmt.Sprintf("Failed to update domain: %v", err))
				r.Recorder.Event(r.domain, corev1.EventTypeWarning,
					controller.EventReasonAPIError, cf.SanitizeErrorMessage(err))
				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}

			// Update status with new information
			r.domain.Status.Locked = updatedInfo.Locked
			r.domain.Status.AutoRenew = config.AutoRenew
			r.domain.Status.Privacy = config.Privacy
		}
	}

	// Determine state based on domain information
	state := r.determineState(domainInfo)
	r.updateState(state, r.getStateMessage(state))

	r.Recorder.Event(r.domain, corev1.EventTypeNormal, "Synced",
		fmt.Sprintf("Domain %s synced successfully", r.domain.Spec.DomainName))

	// Requeue periodically to monitor expiration
	return ctrl.Result{RequeueAfter: 1 * time.Hour}, nil
}

// determineState determines the domain state based on domain information
func (*Reconciler) determineState(info *cf.RegistrarDomainInfo) networkingv1alpha2.DomainRegistrationState {
	// Check for transfer in progress
	if info.TransferInStatus != "" && info.TransferInStatus != "complete" {
		return networkingv1alpha2.DomainRegistrationStateTransferPending
	}

	// Check for expiration
	if !info.ExpiresAt.IsZero() && info.ExpiresAt.Before(time.Now()) {
		return networkingv1alpha2.DomainRegistrationStateExpired
	}

	// Domain is active
	return networkingv1alpha2.DomainRegistrationStateActive
}

// getStateMessage returns a human-readable message for the state
func (*Reconciler) getStateMessage(state networkingv1alpha2.DomainRegistrationState) string {
	switch state {
	case networkingv1alpha2.DomainRegistrationStateActive:
		return "Domain is active and managed by Cloudflare"
	case networkingv1alpha2.DomainRegistrationStateTransferPending:
		return "Domain transfer is in progress"
	case networkingv1alpha2.DomainRegistrationStateExpired:
		return "Domain has expired"
	default:
		return "Domain state unknown"
	}
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

// updateState updates the state and status of the DomainRegistration
func (r *Reconciler) updateState(state networkingv1alpha2.DomainRegistrationState, message string) {
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

	if state == networkingv1alpha2.DomainRegistrationStateActive {
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

// findDomainsForCredentials returns DomainRegistrations that reference the given credentials
//
//nolint:revive // cognitive complexity is acceptable for watch handler
func (r *Reconciler) findDomainsForCredentials(
	ctx context.Context, obj client.Object,
) []reconcile.Request {
	creds, ok := obj.(*networkingv1alpha2.CloudflareCredentials)
	if !ok {
		return nil
	}

	domainList := &networkingv1alpha2.DomainRegistrationList{}
	if err := r.List(ctx, domainList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, domain := range domainList.Items {
		if domain.Spec.CredentialsRef != nil && domain.Spec.CredentialsRef.Name == creds.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: domain.Name,
				},
			})
		}

		if creds.Spec.IsDefault && domain.Spec.CredentialsRef == nil {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: domain.Name,
				},
			})
		}
	}

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.DomainRegistration{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findDomainsForCredentials)).
		Named("domainregistration").
		Complete(r)
}
