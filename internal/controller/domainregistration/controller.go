// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package domainregistration provides a controller for managing Cloudflare Registrar domains.
package domainregistration

import (
	"context"
	"errors"
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
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/service"
	domainsvc "github.com/StringKe/cloudflare-operator/internal/service/domain"
)

const (
	finalizerName = "cloudflare.com/domain-registration-finalizer"
)

var (
	// errLifecyclePending indicates a lifecycle operation is pending completion.
	errLifecyclePending = errors.New("lifecycle operation pending, waiting for completion")
)

// Reconciler reconciles a DomainRegistration object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Service  *domainsvc.DomainRegistrationService // Core Service for SyncState management

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

	// Check if there's a pending lifecycle operation and apply result
	result, requeue, err := r.applyLifecycleResult()
	if err != nil {
		if errors.Is(err, errLifecyclePending) {
			// Operation pending, requeue to check status later
			return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	if requeue {
		return result, nil
	}

	// Request sync via Service (six-layer architecture)
	return r.requestSync()
}

// handleDeletion handles the deletion of DomainRegistration
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.domain, finalizerName) {
		return ctrl.Result{}, nil
	}

	// DomainRegistration is read-only for the domain itself
	// We don't delete the domain from Cloudflare, just unregister from SyncState and remove finalizer
	r.log.Info("DomainRegistration being deleted, unregistering from SyncState")

	// Unregister from SyncState
	source := service.Source{
		Kind: "DomainRegistration",
		Name: r.domain.Name,
	}
	if err := r.Service.Unregister(r.ctx, r.domain.Spec.DomainName, source); err != nil {
		r.log.Error(err, "failed to unregister from SyncState")
		// Continue with finalizer removal even if unregister fails
	}

	// Cleanup SyncState if no other sources
	if err := r.Service.CleanupSyncState(r.ctx, r.domain.Spec.DomainName); err != nil {
		r.log.V(1).Info("Failed to cleanup SyncState", "error", err)
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.domain, func() {
		controllerutil.RemoveFinalizer(r.domain, finalizerName)
	}); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// requestSync requests a sync of domain registration information via Service.
//
//nolint:revive // cognitive complexity is acceptable for sync request logic
func (r *Reconciler) requestSync() (ctrl.Result, error) {
	r.updateState(networkingv1alpha2.DomainRegistrationStateSyncing, "Requesting domain sync")

	// Resolve credentials
	credRef := r.buildCredentialsRef()

	// Get account ID from credentials
	accountID := ""
	if credRef.Name != "" {
		creds := &networkingv1alpha2.CloudflareCredentials{}
		if err := r.Get(r.ctx, types.NamespacedName{Name: credRef.Name}, creds); err == nil {
			accountID = creds.Spec.AccountID
		}
	} else {
		// Try default credentials
		credsList := &networkingv1alpha2.CloudflareCredentialsList{}
		if err := r.List(r.ctx, credsList); err == nil {
			for _, cred := range credsList.Items {
				if cred.Spec.IsDefault {
					accountID = cred.Spec.AccountID
					credRef.Name = cred.Name
					break
				}
			}
		}
	}

	// Build configuration if present
	var config *domainsvc.DomainRegistrationConfiguration
	if r.domain.Spec.Configuration != nil {
		config = &domainsvc.DomainRegistrationConfiguration{
			AutoRenew:   r.domain.Spec.Configuration.AutoRenew,
			Privacy:     r.domain.Spec.Configuration.Privacy,
			Locked:      r.domain.Spec.Configuration.Locked,
			NameServers: r.domain.Spec.Configuration.NameServers,
		}
	}

	// Request sync via Service
	source := service.Source{
		Kind: "DomainRegistration",
		Name: r.domain.Name,
	}

	_, err := r.Service.RequestSync(r.ctx, domainsvc.DomainRegistrationRegisterOptions{
		AccountID:      accountID,
		Source:         source,
		CredentialsRef: credRef,
		DomainName:     r.domain.Spec.DomainName,
		Configuration:  config,
	})
	if err != nil {
		r.updateState(networkingv1alpha2.DomainRegistrationStateError,
			fmt.Sprintf("Failed to request sync: %v", err))
		r.Recorder.Event(r.domain, corev1.EventTypeWarning,
			controller.EventReasonAPIError, fmt.Sprintf("Failed to request sync: %v", err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Update state to syncing and requeue to check result
	r.updateState(networkingv1alpha2.DomainRegistrationStateSyncing, "Sync requested, waiting for completion")
	return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
}

// applyLifecycleResult checks and applies results from SyncState.
//
//nolint:revive // cognitive complexity is acceptable for result application logic
func (r *Reconciler) applyLifecycleResult() (ctrl.Result, bool, error) {
	// Check if operation completed
	completed, err := r.Service.IsLifecycleCompleted(r.ctx, r.domain.Spec.DomainName)
	if err != nil {
		r.log.V(1).Info("Error checking lifecycle status", "error", err)
		return ctrl.Result{}, false, nil // No SyncState yet, proceed with sync
	}

	if !completed {
		// Check for error
		errMsg, _ := r.Service.GetLifecycleError(r.ctx, r.domain.Spec.DomainName)
		if errMsg != "" {
			r.updateState(networkingv1alpha2.DomainRegistrationStateError, errMsg)
			r.Recorder.Event(r.domain, corev1.EventTypeWarning,
				controller.EventReasonAPIError, errMsg)
			return ctrl.Result{RequeueAfter: time.Minute}, true, nil
		}

		// Still pending
		return ctrl.Result{}, false, errLifecyclePending
	}

	// Get the result
	result, err := r.Service.GetLifecycleResult(r.ctx, r.domain.Spec.DomainName)
	if err != nil {
		r.log.Error(err, "Failed to get lifecycle result")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, true, nil
	}

	if result == nil {
		// No result yet, proceed with sync
		return ctrl.Result{}, false, nil
	}

	// Apply result to status
	r.domain.Status.DomainID = result.DomainID
	r.domain.Status.CurrentRegistrar = result.CurrentRegistrar
	r.domain.Status.RegistryStatuses = result.RegistryStatuses
	r.domain.Status.Locked = result.Locked
	r.domain.Status.TransferInStatus = result.TransferInStatus
	r.domain.Status.AutoRenew = result.AutoRenew
	r.domain.Status.Privacy = result.Privacy

	//nolint:staticcheck // Time embedded access required for zero check
	if !result.ExpiresAt.Time.IsZero() {
		r.domain.Status.ExpiresAt = &metav1.Time{Time: result.ExpiresAt.Time}
	}
	//nolint:staticcheck // Time embedded access required for zero check
	if !result.CreatedAt.Time.IsZero() {
		r.domain.Status.CreatedAt = &metav1.Time{Time: result.CreatedAt.Time}
	}

	// Determine state based on result
	state := r.determineState(result)
	r.updateState(state, r.getStateMessage(state))

	r.Recorder.Event(r.domain, corev1.EventTypeNormal, "Synced",
		fmt.Sprintf("Domain %s synced successfully", r.domain.Spec.DomainName))

	// Requeue periodically to monitor expiration
	return ctrl.Result{RequeueAfter: 1 * time.Hour}, true, nil
}

// determineState determines the domain state based on sync result.
func (*Reconciler) determineState(result *domainsvc.DomainRegistrationSyncResult) networkingv1alpha2.DomainRegistrationState {
	// Check for transfer in progress
	if result.TransferInStatus != "" && result.TransferInStatus != "complete" {
		return networkingv1alpha2.DomainRegistrationStateTransferPending
	}

	// Check for expiration
	//nolint:staticcheck // Time embedded access required for zero check
	if !result.ExpiresAt.Time.IsZero() && result.ExpiresAt.Time.Before(time.Now()) {
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

// buildCredentialsRef builds a CredentialsReference from the domain spec.
func (r *Reconciler) buildCredentialsRef() networkingv1alpha2.CredentialsReference {
	if r.domain.Spec.CredentialsRef != nil {
		return networkingv1alpha2.CredentialsReference{
			Name: r.domain.Spec.CredentialsRef.Name,
		}
	}
	return networkingv1alpha2.CredentialsReference{}
}

// updateState updates the state and status of the DomainRegistration.
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
