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
	// Following Unified Sync Architecture: only unregister from SyncState
	// Sync Controller handles actual Cloudflare API deletion
	if r.token.GetDeletionTimestamp() != nil {
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
	result, err := r.reconcileServiceToken()
	if err != nil {
		r.log.Error(err, "failed to reconcile AccessServiceToken")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	return result, nil
}

// resolveCredentials resolves the credentials reference and returns the credentials info.
// Following Unified Sync Architecture, the Resource Controller only needs
// credential metadata (accountID, credRef) - it does not create a Cloudflare API client.
func (r *Reconciler) resolveCredentials() (*controller.CredentialsInfo, error) {
	// AccessServiceToken is cluster-scoped, use operator namespace for legacy inline secrets
	info, err := controller.ResolveCredentialsForService(
		r.ctx,
		r.Client,
		r.log,
		r.token.Spec.Cloudflare,
		controller.OperatorNamespace,
		r.token.Status.AccountID,
	)
	if err != nil {
		r.log.Error(err, "failed to resolve credentials")
		r.Recorder.Event(r.token, corev1.EventTypeWarning, controller.EventReasonAPIError,
			"Failed to resolve credentials: "+err.Error())
		return nil, err
	}

	return info, nil
}

// handleDeletion handles the deletion of an AccessServiceToken.
// Following Unified Sync Architecture:
// Resource Controller only unregisters from SyncState.
// AccessServiceToken Sync Controller handles the actual Cloudflare API deletion.
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.token, FinalizerName) {
		return ctrl.Result{}, nil
	}

	r.log.Info("Unregistering AccessServiceToken from SyncState")

	// Get Token ID from status
	tokenID := r.token.Status.TokenID

	// Remove secret finalizer before removing token finalizer
	if err := r.removeSecretFinalizer(); err != nil {
		r.log.Error(err, "Failed to remove secret finalizer, continuing with deletion")
		// Don't block on this - the secret might have been deleted already
	}

	// Unregister from SyncState - this triggers Sync Controller to delete from Cloudflare
	// Following: Resource Controller → Core Service → SyncState → Sync Controller → Cloudflare API
	source := service.Source{
		Kind: "AccessServiceToken",
		Name: r.token.Name,
	}

	if err := r.tokenService.Unregister(r.ctx, tokenID, source); err != nil {
		r.log.Error(err, "failed to unregister from SyncState")
		r.Recorder.Event(r.token, corev1.EventTypeWarning, "UnregisterFailed",
			fmt.Sprintf("Failed to unregister from SyncState: %s", err.Error()))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	r.Recorder.Event(r.token, corev1.EventTypeNormal, "Unregistered",
		"Unregistered from SyncState, Sync Controller will delete from Cloudflare")

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
// Following Unified Sync Architecture:
// Resource Controller → Core Service → SyncState → Sync Controller → Cloudflare API
//
//nolint:revive // cognitive complexity is acceptable for reconciliation logic
func (r *Reconciler) reconcileServiceToken() (ctrl.Result, error) {
	tokenName := r.token.GetTokenName()

	// Resolve credentials (without creating API client)
	credInfo, err := r.resolveCredentials()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolve credentials: %w", err)
	}

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

	// Register with service using credentials info
	opts := accesssvc.AccessServiceTokenRegisterOptions{
		AccountID:      credInfo.AccountID,
		TokenID:        r.token.Status.TokenID,
		Source:         source,
		Config:         config,
		CredentialsRef: credInfo.CredentialsRef,
	}

	if err := r.tokenService.Register(r.ctx, opts); err != nil {
		r.log.Error(err, "failed to register AccessServiceToken configuration")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonCreateFailed, err.Error())
		r.Recorder.Event(r.token, corev1.EventTypeWarning, controller.EventReasonCreateFailed, err.Error())
		return ctrl.Result{}, err
	}

	// Ensure secret has finalizer to prevent accidental deletion
	if r.token.Spec.SecretRef.Name != "" {
		if err := r.ensureSecretFinalizer(); err != nil {
			r.log.Error(err, "Failed to add finalizer to secret")
			// Don't fail reconciliation for this
		}
	}

	// Check if already synced (SyncState may have been created and synced in a previous reconcile)
	syncStatus, err := r.tokenService.GetSyncStatus(r.ctx, source, r.token.Status.TokenID)
	if err != nil {
		r.log.Error(err, "failed to get sync status")
		if err := r.updateStatusPending(credInfo.AccountID); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	if syncStatus != nil && syncStatus.IsSynced && syncStatus.TokenID != "" {
		// Already synced, update status to Ready
		if err := r.updateStatusReady(credInfo.AccountID, syncStatus.TokenID); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Update status to Pending if not already synced
	if err := r.updateStatusPending(credInfo.AccountID); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// updateStatusPending updates the AccessServiceToken status to Pending state.
func (r *Reconciler) updateStatusPending(accountID string) error {
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.token, func() {
		r.token.Status.ObservedGeneration = r.token.Generation

		// Keep existing state if already active, otherwise set to pending
		if r.token.Status.State != "Ready" {
			r.token.Status.State = "pending"
		}

		// Set account ID
		if accountID != "" {
			r.token.Status.AccountID = accountID
		}

		r.setCondition(metav1.ConditionFalse, "Pending", "Configuration registered, waiting for sync")
	})

	if err != nil {
		r.log.Error(err, "failed to update AccessServiceToken status")
		return err
	}

	r.log.Info("AccessServiceToken configuration registered", "name", r.token.Name)
	r.Recorder.Event(r.token, corev1.EventTypeNormal, "Registered",
		"Configuration registered to SyncState")
	return nil
}

// updateStatusReady updates the AccessServiceToken status to Ready state.
func (r *Reconciler) updateStatusReady(accountID, tokenID string) error {
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.token, func() {
		r.token.Status.ObservedGeneration = r.token.Generation
		r.token.Status.State = "Ready"
		r.token.Status.AccountID = accountID
		r.token.Status.TokenID = tokenID
		r.setCondition(metav1.ConditionTrue, "Synced", "AccessServiceToken synced to Cloudflare")
	})

	if err != nil {
		r.log.Error(err, "failed to update AccessServiceToken status to Ready")
		return err
	}

	r.log.Info("AccessServiceToken synced successfully", "name", r.token.Name, "tokenId", tokenID)
	r.Recorder.Event(r.token, corev1.EventTypeNormal, "Synced",
		fmt.Sprintf("AccessServiceToken synced to Cloudflare with ID %s", tokenID))
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
