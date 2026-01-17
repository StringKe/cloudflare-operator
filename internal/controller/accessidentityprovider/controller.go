// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package accessidentityprovider

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/service"
	accesssvc "github.com/StringKe/cloudflare-operator/internal/service/access"
)

const (
	FinalizerName = "accessidentityprovider.networking.cloudflare-operator.io/finalizer"
)

// Reconciler reconciles an AccessIdentityProvider object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Service layer
	idpService *accesssvc.IdentityProviderService

	// Runtime state
	ctx context.Context
	log logr.Logger
	idp *networkingv1alpha2.AccessIdentityProvider
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessidentityproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessidentityproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessidentityproviders/finalizers,verbs=update

// Reconcile implements the reconciliation loop for AccessIdentityProvider resources.
//
//nolint:revive // cognitive complexity is acceptable for this controller reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrllog.FromContext(ctx)

	// Fetch the AccessIdentityProvider instance
	r.idp = &networkingv1alpha2.AccessIdentityProvider{}
	if err := r.Get(ctx, req.NamespacedName, r.idp); err != nil {
		if errors.IsNotFound(err) {
			r.log.Info("AccessIdentityProvider deleted, nothing to do")
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch AccessIdentityProvider")
		return ctrl.Result{}, err
	}

	// Check if AccessIdentityProvider is being deleted
	// Following Unified Sync Architecture: only unregister from SyncState
	// Sync Controller handles actual Cloudflare API deletion
	if r.idp.GetDeletionTimestamp() != nil {
		return r.handleDeletion()
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.idp, FinalizerName) {
		controllerutil.AddFinalizer(r.idp, FinalizerName)
		if err := r.Update(ctx, r.idp); err != nil {
			r.log.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(r.idp, corev1.EventTypeNormal, controller.EventReasonFinalizerSet, "Finalizer added")
	}

	// Reconcile the AccessIdentityProvider through service layer
	if err := r.reconcileIdentityProvider(); err != nil {
		r.log.Error(err, "failed to reconcile AccessIdentityProvider")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	return ctrl.Result{}, nil
}

// resolveCredentials resolves the credentials reference and returns the credentials info.
// Following Unified Sync Architecture, the Resource Controller only needs
// credential metadata (accountID, credRef) - it does not create a Cloudflare API client.
func (r *Reconciler) resolveCredentials() (*controller.CredentialsInfo, error) {
	// AccessIdentityProvider is cluster-scoped, use operator namespace for legacy inline secrets
	info, err := controller.ResolveCredentialsForService(
		r.ctx,
		r.Client,
		r.log,
		r.idp.Spec.Cloudflare,
		controller.OperatorNamespace,
		r.idp.Status.AccountID,
	)
	if err != nil {
		r.log.Error(err, "failed to resolve credentials")
		r.Recorder.Event(r.idp, corev1.EventTypeWarning, controller.EventReasonAPIError,
			"Failed to resolve credentials: "+err.Error())
		return nil, err
	}

	return info, nil
}

// handleDeletion handles the deletion of an AccessIdentityProvider.
// Following Unified Sync Architecture:
// Resource Controller only unregisters from SyncState.
// AccessIdentityProvider Sync Controller handles the actual Cloudflare API deletion.
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.idp, FinalizerName) {
		return ctrl.Result{}, nil
	}

	r.log.Info("Unregistering AccessIdentityProvider from SyncState")

	// Get Provider ID from status
	providerID := r.idp.Status.ProviderID

	// Unregister from SyncState - this triggers Sync Controller to delete from Cloudflare
	// Following: Resource Controller → Core Service → SyncState → Sync Controller → Cloudflare API
	source := service.Source{
		Kind: "AccessIdentityProvider",
		Name: r.idp.Name,
	}

	if err := r.idpService.Unregister(r.ctx, providerID, source); err != nil {
		r.log.Error(err, "failed to unregister from SyncState")
		r.Recorder.Event(r.idp, corev1.EventTypeWarning, "UnregisterFailed",
			fmt.Sprintf("Failed to unregister from SyncState: %s", err.Error()))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	r.Recorder.Event(r.idp, corev1.EventTypeNormal, "Unregistered",
		"Unregistered from SyncState, Sync Controller will delete from Cloudflare")

	// Remove finalizer with retry logic to handle conflicts
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.idp, func() {
		controllerutil.RemoveFinalizer(r.idp, FinalizerName)
	}); err != nil {
		r.log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(r.idp, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// reconcileIdentityProvider ensures the AccessIdentityProvider configuration is registered with the service layer.
// Following Unified Sync Architecture:
// Resource Controller → Core Service → SyncState → Sync Controller → Cloudflare API
func (r *Reconciler) reconcileIdentityProvider() error {
	providerName := r.idp.GetProviderName()

	// Resolve credentials (without creating API client)
	credInfo, err := r.resolveCredentials()
	if err != nil {
		return fmt.Errorf("resolve credentials: %w", err)
	}

	// Build the configuration
	config := accesssvc.AccessIdentityProviderConfig{
		Name:       providerName,
		Type:       r.idp.Spec.Type,
		Config:     r.idp.Spec.Config,
		ScimConfig: r.idp.Spec.ScimConfig,
	}

	// Build source reference
	source := service.Source{
		Kind: "AccessIdentityProvider",
		Name: r.idp.Name,
	}

	// Register with service using credentials info
	opts := accesssvc.AccessIdentityProviderRegisterOptions{
		AccountID:      credInfo.AccountID,
		ProviderID:     r.idp.Status.ProviderID,
		Source:         source,
		Config:         config,
		CredentialsRef: credInfo.CredentialsRef,
	}

	if err := r.idpService.Register(r.ctx, opts); err != nil {
		r.log.Error(err, "failed to register AccessIdentityProvider configuration")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonCreateFailed, err.Error())
		r.Recorder.Event(r.idp, corev1.EventTypeWarning, controller.EventReasonCreateFailed, err.Error())
		return err
	}

	// Update status to Pending if not already synced
	return r.updateStatusPending(credInfo.AccountID)
}

// updateStatusPending updates the AccessIdentityProvider status to Pending state.
func (r *Reconciler) updateStatusPending(accountID string) error {
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.idp, func() {
		r.idp.Status.ObservedGeneration = r.idp.Generation

		// Keep existing state if already active, otherwise set to pending
		if r.idp.Status.State != "Ready" {
			r.idp.Status.State = "pending"
		}

		// Set account ID
		if accountID != "" {
			r.idp.Status.AccountID = accountID
		}

		r.setCondition(metav1.ConditionFalse, "Pending", "Configuration registered, waiting for sync")
	})

	if err != nil {
		r.log.Error(err, "failed to update AccessIdentityProvider status")
		return err
	}

	r.log.Info("AccessIdentityProvider configuration registered", "name", r.idp.Name)
	r.Recorder.Event(r.idp, corev1.EventTypeNormal, "Registered",
		"Configuration registered to SyncState")
	return nil
}

// setCondition sets a condition on the AccessIdentityProvider status.
func (r *Reconciler) setCondition(status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&r.idp.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: r.idp.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("accessidentityprovider-controller")
	r.idpService = accesssvc.NewIdentityProviderService(mgr.GetClient())
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.AccessIdentityProvider{}).
		Complete(r)
}
