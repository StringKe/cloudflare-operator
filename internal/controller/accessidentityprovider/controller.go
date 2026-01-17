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
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
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
	ctx   context.Context
	log   logr.Logger
	idp   *networkingv1alpha2.AccessIdentityProvider
	cfAPI *cf.API
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
	if r.idp.GetDeletionTimestamp() != nil {
		// Initialize API client for deletion
		if err := r.initAPIClient(); err != nil {
			r.log.Error(err, "failed to initialize API client for deletion")
			r.setCondition(metav1.ConditionFalse, controller.EventReasonAPIError, err.Error())
			return ctrl.Result{}, err
		}
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

// initAPIClient initializes the Cloudflare API client.
func (r *Reconciler) initAPIClient() error {
	// AccessIdentityProvider is cluster-scoped, use operator namespace for legacy inline secrets
	api, err := cf.NewAPIClientFromDetails(r.ctx, r.Client, controller.OperatorNamespace, r.idp.Spec.Cloudflare)
	if err != nil {
		r.log.Error(err, "failed to initialize API client")
		r.Recorder.Event(r.idp, corev1.EventTypeWarning, controller.EventReasonAPIError, "Failed to initialize API client: "+err.Error())
		return err
	}

	api.ValidAccountId = r.idp.Status.AccountID
	r.cfAPI = api
	return nil
}

// handleDeletion handles the deletion of an AccessIdentityProvider.
//
//nolint:revive // cognitive complexity is acceptable for deletion handling
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.idp, FinalizerName) {
		return ctrl.Result{}, nil
	}

	r.log.Info("Deleting AccessIdentityProvider")
	r.Recorder.Event(r.idp, corev1.EventTypeNormal, "Deleting", "Starting AccessIdentityProvider deletion")

	// Try to get Provider ID from status or by looking up by name
	providerID := r.idp.Status.ProviderID
	if providerID == "" {
		// Status ID is empty - try to find by name to prevent orphaned resources
		providerName := r.idp.GetProviderName()
		r.log.Info("Status.ProviderID is empty, trying to find provider by name", "name", providerName)
		existing, err := r.cfAPI.ListAccessIdentityProvidersByName(providerName)
		if err == nil && existing != nil {
			providerID = existing.ID
			r.log.Info("Found AccessIdentityProvider by name", "id", providerID)
		} else {
			r.log.Info("AccessIdentityProvider not found by name, assuming it was never created or already deleted")
		}
	}

	// Delete from Cloudflare if we have an ID
	if providerID != "" {
		if err := r.cfAPI.DeleteAccessIdentityProvider(providerID); err != nil {
			if !cf.IsNotFoundError(err) {
				r.log.Error(err, "failed to delete AccessIdentityProvider from Cloudflare")
				r.Recorder.Event(r.idp, corev1.EventTypeWarning,
					controller.EventReasonDeleteFailed, cf.SanitizeErrorMessage(err))
				return ctrl.Result{RequeueAfter: 30 * time.Second}, err
			}
			r.log.Info("AccessIdentityProvider already deleted from Cloudflare", "id", providerID)
			r.Recorder.Event(r.idp, corev1.EventTypeNormal,
				"AlreadyDeleted", "AccessIdentityProvider was already deleted from Cloudflare")
		} else {
			r.log.Info("AccessIdentityProvider deleted from Cloudflare", "id", providerID)
			r.Recorder.Event(r.idp, corev1.EventTypeNormal,
				controller.EventReasonDeleted, "Deleted from Cloudflare")
		}
	}

	// Unregister from SyncState
	source := service.Source{
		Kind: "AccessIdentityProvider",
		Name: r.idp.Name,
	}
	if err := r.idpService.Unregister(r.ctx, providerID, source); err != nil {
		r.log.Error(err, "failed to unregister from SyncState")
		// Non-fatal - continue with finalizer removal
	}

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
func (r *Reconciler) reconcileIdentityProvider() error {
	providerName := r.idp.GetProviderName()

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

	// Build credentials reference
	credRef := networkingv1alpha2.CredentialsReference{
		Name: r.idp.Spec.Cloudflare.CredentialsRef.Name,
	}

	// Get account ID - need to initialize API client first if not already done
	accountID := r.idp.Status.AccountID
	if accountID == "" {
		// Initialize API client to get account ID
		if err := r.initAPIClient(); err != nil {
			return fmt.Errorf("initialize API client for account ID: %w", err)
		}
		accountID, _ = r.cfAPI.GetAccountId()
	}

	// Register with service
	opts := accesssvc.AccessIdentityProviderRegisterOptions{
		AccountID:      accountID,
		ProviderID:     r.idp.Status.ProviderID,
		Source:         source,
		Config:         config,
		CredentialsRef: credRef,
	}

	if err := r.idpService.Register(r.ctx, opts); err != nil {
		r.log.Error(err, "failed to register AccessIdentityProvider configuration")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonCreateFailed, err.Error())
		r.Recorder.Event(r.idp, corev1.EventTypeWarning, controller.EventReasonCreateFailed, err.Error())
		return err
	}

	// Update status to Pending if not already synced
	return r.updateStatusPending()
}

// updateStatusPending updates the AccessIdentityProvider status to Pending state.
//
//nolint:revive // cognitive complexity is acceptable for status update logic
func (r *Reconciler) updateStatusPending() error {
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.idp, func() {
		r.idp.Status.ObservedGeneration = r.idp.Generation

		// Keep existing state if already active, otherwise set to pending
		if r.idp.Status.State != "Ready" {
			r.idp.Status.State = "pending"
		}

		// Set account ID if we have it
		if r.cfAPI != nil {
			if accountID, err := r.cfAPI.GetAccountId(); err == nil {
				r.idp.Status.AccountID = accountID
			}
		}

		r.setCondition(metav1.ConditionTrue, "Pending", "Configuration registered, waiting for sync")
	})

	if err != nil {
		r.log.Error(err, "failed to update AccessIdentityProvider status")
		return err
	}

	r.log.Info("AccessIdentityProvider configuration registered", "name", r.idp.Name)
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
