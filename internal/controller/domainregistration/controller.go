// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package domainregistration provides a controller for managing Cloudflare Registrar domains.
// It directly calls Cloudflare API and writes status back to the CRD.
package domainregistration

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
	"github.com/StringKe/cloudflare-operator/internal/controller/common"
)

const (
	finalizerName = "cloudflare.com/domain-registration-finalizer"
)

// Reconciler reconciles a DomainRegistration object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=domainregistrations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=domainregistrations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=domainregistrations/finalizers,verbs=update

// Reconcile handles DomainRegistration reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the DomainRegistration resource
	domain := &networkingv1alpha2.DomainRegistration{}
	if err := r.Get(ctx, req.NamespacedName, domain); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch DomainRegistration")
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

	// Sync domain registration from Cloudflare
	return r.syncDomainRegistration(ctx, domain)
}

// handleDeletion handles the deletion of DomainRegistration.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	domain *networkingv1alpha2.DomainRegistration,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(domain, finalizerName) {
		return common.NoRequeue(), nil
	}

	// DomainRegistration is read-only for the domain itself
	// We don't delete the domain from Cloudflare, just remove finalizer
	logger.Info("DomainRegistration being deleted, removing finalizer (domains not deleted from CF)")

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

// syncDomainRegistration syncs domain registration information from Cloudflare.
//
//nolint:revive // cognitive complexity is acceptable for sync logic
func (r *Reconciler) syncDomainRegistration(
	ctx context.Context,
	domain *networkingv1alpha2.DomainRegistration,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	r.updateState(ctx, domain, networkingv1alpha2.DomainRegistrationStateSyncing, "Syncing domain from Cloudflare")

	// Get API client
	// DomainRegistration is cluster-scoped, use operator namespace for legacy secrets
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CredentialsRef:  r.buildCredentialsRef(domain),
		Namespace:       common.OperatorNamespace,
		StatusAccountID: domain.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, domain, err)
	}

	// Get domain information from Cloudflare
	logger.V(1).Info("Getting domain registration info from Cloudflare",
		"domainName", domain.Spec.DomainName)

	domainInfo, err := apiResult.API.GetRegistrarDomain(ctx, domain.Spec.DomainName)
	if err != nil {
		logger.Error(err, "Failed to get registrar domain info")
		return r.updateStatusError(ctx, domain, err)
	}

	// Update configuration if specified
	if domain.Spec.Configuration != nil {
		config := cf.RegistrarDomainConfig{
			AutoRenew:   domain.Spec.Configuration.AutoRenew,
			Privacy:     domain.Spec.Configuration.Privacy,
			Locked:      domain.Spec.Configuration.Locked,
			NameServers: domain.Spec.Configuration.NameServers,
		}

		logger.V(1).Info("Updating domain configuration in Cloudflare",
			"autoRenew", config.AutoRenew,
			"privacy", config.Privacy,
			"locked", config.Locked)

		updatedInfo, err := apiResult.API.UpdateRegistrarDomain(ctx, domain.Spec.DomainName, config)
		if err != nil {
			logger.Error(err, "Failed to update registrar domain config")
			// Continue with current info, don't fail the sync
			r.Recorder.Event(domain, corev1.EventTypeWarning, "ConfigUpdateFailed",
				fmt.Sprintf("Failed to update domain config: %s", cf.SanitizeErrorMessage(err)))
		} else {
			domainInfo = updatedInfo
		}
	}

	r.Recorder.Event(domain, corev1.EventTypeNormal, "Synced",
		fmt.Sprintf("Domain %s synced from Cloudflare Registrar", domain.Spec.DomainName))

	return r.updateStatusReady(ctx, domain, apiResult.AccountID, domainInfo)
}

// buildCredentialsRef builds a CredentialsReference from spec.
func (*Reconciler) buildCredentialsRef(
	domain *networkingv1alpha2.DomainRegistration,
) *networkingv1alpha2.CredentialsReference {
	if domain.Spec.CredentialsRef != nil {
		return &networkingv1alpha2.CredentialsReference{
			Name: domain.Spec.CredentialsRef.Name,
		}
	}
	return nil
}

// updateState updates the state and status of the DomainRegistration.
func (r *Reconciler) updateState(
	ctx context.Context,
	domain *networkingv1alpha2.DomainRegistration,
	state networkingv1alpha2.DomainRegistrationState,
	message string,
) {
	logger := log.FromContext(ctx)

	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		ObservedGeneration: domain.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(state),
		Message:            message,
	}

	if state == networkingv1alpha2.DomainRegistrationStateActive {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "DomainActive"
	}

	if err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, domain, func() {
		domain.Status.State = state
		domain.Status.Message = message
		domain.Status.ObservedGeneration = domain.Generation
		meta.SetStatusCondition(&domain.Status.Conditions, condition)
	}); err != nil {
		logger.Error(err, "failed to update status")
	}
}

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	domain *networkingv1alpha2.DomainRegistration,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, domain, func() {
		domain.Status.State = networkingv1alpha2.DomainRegistrationStateError
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

func (r *Reconciler) updateStatusReady(
	ctx context.Context,
	domain *networkingv1alpha2.DomainRegistration,
	accountID string,
	domainInfo *cf.RegistrarDomainInfo,
) (ctrl.Result, error) {
	// Determine state based on domain info
	state := r.determineState(domainInfo)
	message := r.getStateMessage(state)

	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, domain, func() {
		domain.Status.AccountID = accountID
		domain.Status.DomainID = domainInfo.ID
		domain.Status.CurrentRegistrar = domainInfo.CurrentRegistrar
		domain.Status.RegistryStatuses = domainInfo.RegistryStatuses
		domain.Status.Locked = domainInfo.Locked
		domain.Status.TransferInStatus = domainInfo.TransferInStatus
		domain.Status.AutoRenew = domainInfo.CanCancelTransfer // Use existing field for auto-renew state

		if !domainInfo.ExpiresAt.IsZero() {
			domain.Status.ExpiresAt = &metav1.Time{Time: domainInfo.ExpiresAt}
		}
		if !domainInfo.CreatedAt.IsZero() {
			domain.Status.CreatedAt = &metav1.Time{Time: domainInfo.CreatedAt}
		}

		domain.Status.State = state
		domain.Status.Message = message

		conditionStatus := metav1.ConditionFalse
		if state == networkingv1alpha2.DomainRegistrationStateActive {
			conditionStatus = metav1.ConditionTrue
		}

		meta.SetStatusCondition(&domain.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             conditionStatus,
			ObservedGeneration: domain.Generation,
			Reason:             string(state),
			Message:            message,
			LastTransitionTime: metav1.Now(),
		})
		domain.Status.ObservedGeneration = domain.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	// Requeue periodically to monitor expiration
	return ctrl.Result{RequeueAfter: 1 * time.Hour}, nil
}

// determineState determines the domain state based on domain info.
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

// getStateMessage returns a human-readable message for the state.
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

// findDomainsForCredentials returns DomainRegistrations that reference the given credentials.
func (r *Reconciler) findDomainsForCredentials(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	creds, ok := obj.(*networkingv1alpha2.CloudflareCredentials)
	if !ok {
		return nil
	}
	logger := log.FromContext(ctx)

	domainList := &networkingv1alpha2.DomainRegistrationList{}
	if err := r.List(ctx, domainList); err != nil {
		logger.Error(err, "Failed to list DomainRegistrations for credentials watch")
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
	r.Recorder = mgr.GetEventRecorderFor("domainregistration-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("domainregistration"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.DomainRegistration{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findDomainsForCredentials)).
		Named("domainregistration").
		Complete(r)
}
