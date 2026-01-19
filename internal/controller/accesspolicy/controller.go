// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package accesspolicy

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
	FinalizerName = "accesspolicy.networking.cloudflare-operator.io/finalizer"
)

// Reconciler reconciles an AccessPolicy object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Service layer
	policyService *accesssvc.PolicyService

	// Runtime state
	ctx          context.Context
	log          logr.Logger
	accessPolicy *networkingv1alpha2.AccessPolicy
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accesspolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accesspolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accesspolicies/finalizers,verbs=update

// Reconcile implements the reconciliation loop for AccessPolicy resources.
//
//nolint:revive // cognitive complexity is acceptable for this controller reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrllog.FromContext(ctx)

	// Fetch the AccessPolicy instance
	r.accessPolicy = &networkingv1alpha2.AccessPolicy{}
	if err := r.Get(ctx, req.NamespacedName, r.accessPolicy); err != nil {
		if errors.IsNotFound(err) {
			r.log.Info("AccessPolicy deleted, nothing to do")
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch AccessPolicy")
		return ctrl.Result{}, err
	}

	// Check if AccessPolicy is being deleted
	// Following Unified Sync Architecture: only unregister from SyncState
	// Sync Controller handles actual Cloudflare API deletion
	if r.accessPolicy.GetDeletionTimestamp() != nil {
		return r.handleDeletion()
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.accessPolicy, FinalizerName) {
		controllerutil.AddFinalizer(r.accessPolicy, FinalizerName)
		if err := r.Update(ctx, r.accessPolicy); err != nil {
			r.log.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(r.accessPolicy, corev1.EventTypeNormal, controller.EventReasonFinalizerSet, "Finalizer added")
	}

	// Reconcile the AccessPolicy through service layer
	result, err := r.reconcileAccessPolicy()
	if err != nil {
		r.log.Error(err, "failed to reconcile AccessPolicy")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	return result, nil
}

// resolveCredentials resolves the credentials reference and returns the credentials info.
// Following Unified Sync Architecture, the Resource Controller only needs
// credential metadata (accountID, credRef) - it does not create a Cloudflare API client.
func (r *Reconciler) resolveCredentials() (*controller.CredentialsInfo, error) {
	// AccessPolicy is cluster-scoped, use operator namespace for legacy inline secrets
	info, err := controller.ResolveCredentialsForService(
		r.ctx,
		r.Client,
		r.log,
		r.accessPolicy.Spec.Cloudflare,
		controller.OperatorNamespace,
		r.accessPolicy.Status.AccountID,
	)
	if err != nil {
		r.log.Error(err, "failed to resolve credentials")
		r.Recorder.Event(r.accessPolicy, corev1.EventTypeWarning, controller.EventReasonAPIError,
			"Failed to resolve credentials: "+err.Error())
		return nil, err
	}

	return info, nil
}

// handleDeletion handles the deletion of an AccessPolicy.
// Following Unified Sync Architecture:
// Resource Controller only unregisters from SyncState.
// AccessPolicy Sync Controller handles the actual Cloudflare API deletion.
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.accessPolicy, FinalizerName) {
		return ctrl.Result{}, nil
	}

	r.log.Info("Unregistering AccessPolicy from SyncState")

	// Get Policy ID from status
	policyID := r.accessPolicy.Status.PolicyID

	// Unregister from SyncState - this triggers Sync Controller to delete from Cloudflare
	// Following: Resource Controller → Core Service → SyncState → Sync Controller → Cloudflare API
	source := service.Source{
		Kind: "AccessPolicy",
		Name: r.accessPolicy.Name,
	}

	if err := r.policyService.Unregister(r.ctx, policyID, source); err != nil {
		r.log.Error(err, "failed to unregister from SyncState")
		r.Recorder.Event(r.accessPolicy, corev1.EventTypeWarning, "UnregisterFailed",
			fmt.Sprintf("Failed to unregister from SyncState: %s", err.Error()))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	r.Recorder.Event(r.accessPolicy, corev1.EventTypeNormal, "Unregistered",
		"Unregistered from SyncState, Sync Controller will delete from Cloudflare")

	// Remove finalizer with retry logic to handle conflicts
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.accessPolicy, func() {
		controllerutil.RemoveFinalizer(r.accessPolicy, FinalizerName)
	}); err != nil {
		r.log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(r.accessPolicy, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// reconcileAccessPolicy ensures the AccessPolicy configuration is registered with the service layer.
// Following Unified Sync Architecture:
// Resource Controller → Core Service → SyncState → Sync Controller → Cloudflare API
//
//nolint:revive // cognitive complexity acceptable for reconciliation logic
func (r *Reconciler) reconcileAccessPolicy() (ctrl.Result, error) {
	policyName := r.accessPolicy.GetAccessPolicyName()

	// Resolve credentials (without creating API client)
	credInfo, err := r.resolveCredentials()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolve credentials: %w", err)
	}

	// Build the configuration
	config := r.buildConfig(policyName)

	// Build source reference
	source := service.Source{
		Kind: "AccessPolicy",
		Name: r.accessPolicy.Name,
	}

	// Register with service using credentials info
	opts := accesssvc.ReusableAccessPolicyRegisterOptions{
		AccountID:      credInfo.AccountID,
		PolicyID:       r.accessPolicy.Status.PolicyID,
		Source:         source,
		Config:         config,
		CredentialsRef: credInfo.CredentialsRef,
	}

	if err := r.policyService.Register(r.ctx, opts); err != nil {
		r.log.Error(err, "failed to register AccessPolicy configuration")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonCreateFailed, err.Error())
		r.Recorder.Event(r.accessPolicy, corev1.EventTypeWarning, controller.EventReasonCreateFailed, err.Error())
		return ctrl.Result{}, err
	}

	// Check if already synced (SyncState may have been created and synced in a previous reconcile)
	syncStatus, err := r.policyService.GetSyncStatus(r.ctx, source, r.accessPolicy.Status.PolicyID)
	if err != nil {
		r.log.Error(err, "failed to get sync status")
		if err := r.updateStatusPending(credInfo.AccountID); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	if syncStatus != nil && syncStatus.IsSynced && syncStatus.PolicyID != "" {
		// Already synced, update status to Ready
		if err := r.updateStatusReady(credInfo.AccountID, syncStatus.PolicyID); err != nil {
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

// buildConfig builds the ReusableAccessPolicyConfig from the AccessPolicy spec.
func (r *Reconciler) buildConfig(policyName string) accesssvc.ReusableAccessPolicyConfig {
	config := accesssvc.ReusableAccessPolicyConfig{
		Name:                         policyName,
		Decision:                     r.accessPolicy.Spec.Decision,
		Precedence:                   r.accessPolicy.Spec.Precedence,
		Include:                      r.accessPolicy.Spec.Include,
		Exclude:                      r.accessPolicy.Spec.Exclude,
		Require:                      r.accessPolicy.Spec.Require,
		SessionDuration:              r.accessPolicy.Spec.SessionDuration,
		IsolationRequired:            r.accessPolicy.Spec.IsolationRequired,
		PurposeJustificationRequired: r.accessPolicy.Spec.PurposeJustificationRequired,
		PurposeJustificationPrompt:   r.accessPolicy.Spec.PurposeJustificationPrompt,
		ApprovalRequired:             r.accessPolicy.Spec.ApprovalRequired,
	}

	// Convert approval groups
	if len(r.accessPolicy.Spec.ApprovalGroups) > 0 {
		config.ApprovalGroups = make([]accesssvc.ApprovalGroupConfig, 0, len(r.accessPolicy.Spec.ApprovalGroups))
		for _, ag := range r.accessPolicy.Spec.ApprovalGroups {
			config.ApprovalGroups = append(config.ApprovalGroups, accesssvc.ApprovalGroupConfig{
				EmailAddresses:  ag.EmailAddresses,
				EmailListUUID:   ag.EmailListUUID,
				ApprovalsNeeded: ag.ApprovalsNeeded,
			})
		}
	}

	return config
}

// updateStatusPending updates the AccessPolicy status to Pending state.
func (r *Reconciler) updateStatusPending(accountID string) error {
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.accessPolicy, func() {
		r.accessPolicy.Status.ObservedGeneration = r.accessPolicy.Generation

		// Keep existing state if already active, otherwise set to pending
		if r.accessPolicy.Status.State != "Ready" {
			r.accessPolicy.Status.State = "pending"
		}

		// Set account ID
		if accountID != "" {
			r.accessPolicy.Status.AccountID = accountID
		}

		r.setCondition(metav1.ConditionFalse, "Pending", "Configuration registered, waiting for sync")
	})

	if err != nil {
		r.log.Error(err, "failed to update AccessPolicy status")
		return err
	}

	r.log.Info("AccessPolicy configuration registered", "name", r.accessPolicy.Name)
	r.Recorder.Event(r.accessPolicy, corev1.EventTypeNormal, "Registered",
		"Configuration registered to SyncState")
	return nil
}

// updateStatusReady updates the AccessPolicy status to Ready state.
func (r *Reconciler) updateStatusReady(accountID, policyID string) error {
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.accessPolicy, func() {
		r.accessPolicy.Status.ObservedGeneration = r.accessPolicy.Generation
		r.accessPolicy.Status.State = "Ready"
		r.accessPolicy.Status.AccountID = accountID
		r.accessPolicy.Status.PolicyID = policyID
		r.setCondition(metav1.ConditionTrue, "Synced", "AccessPolicy synced to Cloudflare")
	})

	if err != nil {
		r.log.Error(err, "failed to update AccessPolicy status to Ready")
		return err
	}

	r.log.Info("AccessPolicy synced successfully", "name", r.accessPolicy.Name, "policyId", policyID)
	r.Recorder.Event(r.accessPolicy, corev1.EventTypeNormal, "Synced",
		fmt.Sprintf("AccessPolicy synced to Cloudflare with ID %s", policyID))
	return nil
}

// setCondition sets a condition on the AccessPolicy status.
func (r *Reconciler) setCondition(status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&r.accessPolicy.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: r.accessPolicy.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("accesspolicy-controller")
	r.policyService = accesssvc.NewPolicyService(mgr.GetClient())
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.AccessPolicy{}).
		Complete(r)
}
