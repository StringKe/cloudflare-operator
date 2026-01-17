// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package accessservicetoken

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
	"k8s.io/apimachinery/pkg/types"
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
	FinalizerName = "accessservicetoken.networking.cloudflare-operator.io/finalizer"
)

// Reconciler reconciles an AccessServiceToken object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Service layer
	tokenService *accesssvc.ServiceTokenService

	// Runtime state
	ctx   context.Context
	log   logr.Logger
	token *networkingv1alpha2.AccessServiceToken
	cfAPI *cf.API
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessservicetokens,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessservicetokens/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessservicetokens/finalizers,verbs=update

// Reconcile implements the reconciliation loop for AccessServiceToken resources.
//
//nolint:revive // cognitive complexity is acceptable for this controller reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrllog.FromContext(ctx)

	// Fetch the AccessServiceToken instance
	r.token = &networkingv1alpha2.AccessServiceToken{}
	if err := r.Get(ctx, req.NamespacedName, r.token); err != nil {
		if errors.IsNotFound(err) {
			r.log.Info("AccessServiceToken deleted, nothing to do")
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch AccessServiceToken")
		return ctrl.Result{}, err
	}

	// Check if AccessServiceToken is being deleted
	if r.token.GetDeletionTimestamp() != nil {
		// Initialize API client for deletion
		if err := r.initAPIClient(); err != nil {
			r.log.Error(err, "failed to initialize API client for deletion")
			r.setCondition(metav1.ConditionFalse, controller.EventReasonAPIError, err.Error())
			return ctrl.Result{}, err
		}
		return r.handleDeletion()
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.token, FinalizerName) {
		controllerutil.AddFinalizer(r.token, FinalizerName)
		if err := r.Update(ctx, r.token); err != nil {
			r.log.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(r.token, corev1.EventTypeNormal, controller.EventReasonFinalizerSet, "Finalizer added")
	}

	// Reconcile the AccessServiceToken through service layer
	if err := r.reconcileServiceToken(); err != nil {
		r.log.Error(err, "failed to reconcile AccessServiceToken")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	return ctrl.Result{}, nil
}

// initAPIClient initializes the Cloudflare API client.
func (r *Reconciler) initAPIClient() error {
	// AccessServiceToken is cluster-scoped, use operator namespace for legacy inline secrets
	api, err := cf.NewAPIClientFromDetails(r.ctx, r.Client, controller.OperatorNamespace, r.token.Spec.Cloudflare)
	if err != nil {
		r.log.Error(err, "failed to initialize API client")
		r.Recorder.Event(r.token, corev1.EventTypeWarning, controller.EventReasonAPIError, "Failed to initialize API client: "+err.Error())
		return err
	}

	api.ValidAccountId = r.token.Status.AccountID
	r.cfAPI = api
	return nil
}

// handleDeletion handles the deletion of an AccessServiceToken.
//
//nolint:revive // cognitive complexity is acceptable for deletion handling
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.token, FinalizerName) {
		return ctrl.Result{}, nil
	}

	r.log.Info("Deleting AccessServiceToken")
	r.Recorder.Event(r.token, corev1.EventTypeNormal, "Deleting", "Starting AccessServiceToken deletion")

	// Try to get Token ID from status or by looking up by name
	tokenID := r.token.Status.TokenID
	if tokenID == "" {
		// Status ID is empty - try to find by name to prevent orphaned resources
		tokenName := r.token.GetTokenName()
		r.log.Info("Status.TokenID is empty, trying to find token by name", "name", tokenName)
		existing, err := r.cfAPI.GetAccessServiceTokenByName(tokenName)
		if err == nil && existing != nil {
			tokenID = existing.TokenID
			r.log.Info("Found AccessServiceToken by name", "id", tokenID)
		} else {
			r.log.Info("AccessServiceToken not found by name, assuming it was never created or already deleted")
		}
	}

	// Delete from Cloudflare if we have an ID
	if tokenID != "" {
		if err := r.cfAPI.DeleteAccessServiceToken(tokenID); err != nil {
			if !cf.IsNotFoundError(err) {
				r.log.Error(err, "failed to delete AccessServiceToken from Cloudflare")
				r.Recorder.Event(r.token, corev1.EventTypeWarning,
					controller.EventReasonDeleteFailed, cf.SanitizeErrorMessage(err))
				return ctrl.Result{RequeueAfter: 30 * time.Second}, err
			}
			r.log.Info("AccessServiceToken already deleted from Cloudflare", "id", tokenID)
			r.Recorder.Event(r.token, corev1.EventTypeNormal,
				"AlreadyDeleted", "AccessServiceToken was already deleted from Cloudflare")
		} else {
			r.log.Info("AccessServiceToken deleted from Cloudflare", "id", tokenID)
			r.Recorder.Event(r.token, corev1.EventTypeNormal,
				controller.EventReasonDeleted, "Deleted from Cloudflare")
		}
	}

	// Remove secret finalizer before removing token finalizer
	if err := r.removeSecretFinalizer(); err != nil {
		r.log.Error(err, "Failed to remove secret finalizer, continuing with deletion")
		// Don't block on this - the secret might have been deleted already
	}

	// Unregister from SyncState
	source := service.Source{
		Kind: "AccessServiceToken",
		Name: r.token.Name,
	}
	if err := r.tokenService.Unregister(r.ctx, tokenID, source); err != nil {
		r.log.Error(err, "failed to unregister from SyncState")
		// Non-fatal - continue with finalizer removal
	}

	// Remove finalizer with retry logic to handle conflicts
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.token, func() {
		controllerutil.RemoveFinalizer(r.token, FinalizerName)
	}); err != nil {
		r.log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(r.token, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// reconcileServiceToken ensures the AccessServiceToken configuration is registered with the service layer.
//
//nolint:revive // cognitive complexity is acceptable for reconciliation logic
func (r *Reconciler) reconcileServiceToken() error {
	tokenName := r.token.GetTokenName()

	// Build the configuration
	config := accesssvc.AccessServiceTokenConfig{
		Name:     tokenName,
		Duration: r.token.Spec.Duration,
	}

	// Add SecretRef if specified
	if r.token.Spec.SecretRef.Name != "" {
		config.SecretRef = &accesssvc.SecretReference{
			Name:      r.token.Spec.SecretRef.Name,
			Namespace: r.token.Spec.SecretRef.Namespace,
		}
	}

	// Build source reference
	source := service.Source{
		Kind: "AccessServiceToken",
		Name: r.token.Name,
	}

	// Build credentials reference
	credRef := networkingv1alpha2.CredentialsReference{
		Name: r.token.Spec.Cloudflare.CredentialsRef.Name,
	}

	// Get account ID - need to initialize API client first if not already done
	accountID := r.token.Status.AccountID
	if accountID == "" {
		// Initialize API client to get account ID
		if err := r.initAPIClient(); err != nil {
			return fmt.Errorf("initialize API client for account ID: %w", err)
		}
		accountID, _ = r.cfAPI.GetAccountId()
	}

	// Register with service
	opts := accesssvc.AccessServiceTokenRegisterOptions{
		AccountID:      accountID,
		TokenID:        r.token.Status.TokenID,
		Source:         source,
		Config:         config,
		CredentialsRef: credRef,
	}

	if err := r.tokenService.Register(r.ctx, opts); err != nil {
		r.log.Error(err, "failed to register AccessServiceToken configuration")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonCreateFailed, err.Error())
		r.Recorder.Event(r.token, corev1.EventTypeWarning, controller.EventReasonCreateFailed, err.Error())
		return err
	}

	// Ensure secret has finalizer to prevent accidental deletion
	if r.token.Spec.SecretRef.Name != "" {
		if err := r.ensureSecretFinalizer(); err != nil {
			r.log.Error(err, "Failed to add finalizer to secret")
			// Don't fail reconciliation for this
		}
	}

	// Update status to Pending if not already synced
	return r.updateStatusPending()
}

// updateStatusPending updates the AccessServiceToken status to Pending state.
//
//nolint:revive // cognitive complexity is acceptable for status update logic
func (r *Reconciler) updateStatusPending() error {
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.token, func() {
		r.token.Status.ObservedGeneration = r.token.Generation

		// Keep existing state if already active, otherwise set to pending
		if r.token.Status.State != "Ready" {
			r.token.Status.State = "pending"
		}

		// Set account ID if we have it
		if r.cfAPI != nil {
			if accountID, err := r.cfAPI.GetAccountId(); err == nil {
				r.token.Status.AccountID = accountID
			}
		}

		r.setCondition(metav1.ConditionTrue, "Pending", "Configuration registered, waiting for sync")
	})

	if err != nil {
		r.log.Error(err, "failed to update AccessServiceToken status")
		return err
	}

	r.log.Info("AccessServiceToken configuration registered", "name", r.token.Name)
	return nil
}

// getSecretFinalizerName returns a valid RFC 1123 compliant finalizer name for the secret.
func (r *Reconciler) getSecretFinalizerName() string {
	return fmt.Sprintf("accessservicetoken.%s.secret-protection", r.token.Name)
}

// ensureSecretFinalizer adds a finalizer to the secret to prevent accidental deletion
func (r *Reconciler) ensureSecretFinalizer() error {
	secretName := r.token.Spec.SecretRef.Name
	secretNamespace := r.token.Spec.SecretRef.Namespace
	if secretNamespace == "" {
		secretNamespace = controller.OperatorNamespace
	}

	secret := &corev1.Secret{}
	if err := r.Get(r.ctx, types.NamespacedName{Name: secretName, Namespace: secretNamespace}, secret); err != nil {
		if errors.IsNotFound(err) {
			return nil // Secret doesn't exist yet
		}
		return err
	}

	finalizerName := r.getSecretFinalizerName()
	if !controllerutil.ContainsFinalizer(secret, finalizerName) {
		controllerutil.AddFinalizer(secret, finalizerName)
		return r.Update(r.ctx, secret)
	}
	return nil
}

// removeSecretFinalizer removes the finalizer from the secret
func (r *Reconciler) removeSecretFinalizer() error {
	if r.token.Spec.SecretRef.Name == "" {
		return nil
	}

	secretName := r.token.Spec.SecretRef.Name
	secretNamespace := r.token.Spec.SecretRef.Namespace
	if secretNamespace == "" {
		secretNamespace = controller.OperatorNamespace
	}

	secret := &corev1.Secret{}
	if err := r.Get(r.ctx, types.NamespacedName{Name: secretName, Namespace: secretNamespace}, secret); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	finalizerName := r.getSecretFinalizerName()
	if controllerutil.ContainsFinalizer(secret, finalizerName) {
		controllerutil.RemoveFinalizer(secret, finalizerName)
		return r.Update(r.ctx, secret)
	}
	return nil
}

// setCondition sets a condition on the AccessServiceToken status.
func (r *Reconciler) setCondition(status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&r.token.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: r.token.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("accessservicetoken-controller")
	r.tokenService = accesssvc.NewServiceTokenService(mgr.GetClient())
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.AccessServiceToken{}).
		Complete(r)
}
