// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package accessgroup

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
	FinalizerName = "accessgroup.networking.cloudflare-operator.io/finalizer"
)

// Reconciler reconciles an AccessGroup object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Service layer
	groupService *accesssvc.GroupService

	// Runtime state
	ctx         context.Context
	log         logr.Logger
	accessGroup *networkingv1alpha2.AccessGroup
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessgroups/finalizers,verbs=update

// Reconcile implements the reconciliation loop for AccessGroup resources.
//
//nolint:revive // cognitive complexity is acceptable for this controller reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrllog.FromContext(ctx)

	// Fetch the AccessGroup instance
	r.accessGroup = &networkingv1alpha2.AccessGroup{}
	if err := r.Get(ctx, req.NamespacedName, r.accessGroup); err != nil {
		if errors.IsNotFound(err) {
			r.log.Info("AccessGroup deleted, nothing to do")
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch AccessGroup")
		return ctrl.Result{}, err
	}

	// Check if AccessGroup is being deleted
	// Following Unified Sync Architecture: only unregister from SyncState
	// Sync Controller handles actual Cloudflare API deletion
	if r.accessGroup.GetDeletionTimestamp() != nil {
		return r.handleDeletion()
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.accessGroup, FinalizerName) {
		controllerutil.AddFinalizer(r.accessGroup, FinalizerName)
		if err := r.Update(ctx, r.accessGroup); err != nil {
			r.log.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(r.accessGroup, corev1.EventTypeNormal, controller.EventReasonFinalizerSet, "Finalizer added")
	}

	// Reconcile the AccessGroup through service layer
	result, err := r.reconcileAccessGroup()
	if err != nil {
		r.log.Error(err, "failed to reconcile AccessGroup")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	return result, nil
}

// resolveCredentials resolves the credentials reference and returns the credentials info.
// Following Unified Sync Architecture, the Resource Controller only needs
// credential metadata (accountID, credRef) - it does not create a Cloudflare API client.
func (r *Reconciler) resolveCredentials() (*controller.CredentialsInfo, error) {
	// AccessGroup is cluster-scoped, use operator namespace for legacy inline secrets
	info, err := controller.ResolveCredentialsForService(
		r.ctx,
		r.Client,
		r.log,
		r.accessGroup.Spec.Cloudflare,
		controller.OperatorNamespace,
		r.accessGroup.Status.AccountID,
	)
	if err != nil {
		r.log.Error(err, "failed to resolve credentials")
		r.Recorder.Event(r.accessGroup, corev1.EventTypeWarning, controller.EventReasonAPIError,
			"Failed to resolve credentials: "+err.Error())
		return nil, err
	}

	return info, nil
}

// handleDeletion handles the deletion of an AccessGroup.
// Following Unified Sync Architecture:
// Resource Controller only unregisters from SyncState.
// AccessGroup Sync Controller handles the actual Cloudflare API deletion.
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.accessGroup, FinalizerName) {
		return ctrl.Result{}, nil
	}

	r.log.Info("Unregistering AccessGroup from SyncState")

	// Get Group ID from status
	groupID := r.accessGroup.Status.GroupID

	// Unregister from SyncState - this triggers Sync Controller to delete from Cloudflare
	// Following: Resource Controller → Core Service → SyncState → Sync Controller → Cloudflare API
	source := service.Source{
		Kind: "AccessGroup",
		Name: r.accessGroup.Name,
	}

	if err := r.groupService.Unregister(r.ctx, groupID, source); err != nil {
		r.log.Error(err, "failed to unregister from SyncState")
		r.Recorder.Event(r.accessGroup, corev1.EventTypeWarning, "UnregisterFailed",
			fmt.Sprintf("Failed to unregister from SyncState: %s", err.Error()))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	r.Recorder.Event(r.accessGroup, corev1.EventTypeNormal, "Unregistered",
		"Unregistered from SyncState, Sync Controller will delete from Cloudflare")

	// Remove finalizer with retry logic to handle conflicts
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.accessGroup, func() {
		controllerutil.RemoveFinalizer(r.accessGroup, FinalizerName)
	}); err != nil {
		r.log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(r.accessGroup, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// reconcileAccessGroup ensures the AccessGroup configuration is registered with the service layer.
// Following Unified Sync Architecture:
// Resource Controller → Core Service → SyncState → Sync Controller → Cloudflare API
//
//nolint:revive // cognitive complexity acceptable for reconciliation logic
func (r *Reconciler) reconcileAccessGroup() (ctrl.Result, error) {
	groupName := r.accessGroup.GetAccessGroupName()

	// Resolve credentials (without creating API client)
	credInfo, err := r.resolveCredentials()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolve credentials: %w", err)
	}

	// Build the configuration
	config := accesssvc.AccessGroupConfig{
		Name:      groupName,
		Include:   r.accessGroup.Spec.Include,
		Exclude:   r.accessGroup.Spec.Exclude,
		Require:   r.accessGroup.Spec.Require,
		IsDefault: r.accessGroup.Spec.IsDefault,
	}

	// Build source reference
	source := service.Source{
		Kind: "AccessGroup",
		Name: r.accessGroup.Name,
	}

	// Register with service using credentials info
	opts := accesssvc.AccessGroupRegisterOptions{
		AccountID:      credInfo.AccountID,
		GroupID:        r.accessGroup.Status.GroupID,
		Source:         source,
		Config:         config,
		CredentialsRef: credInfo.CredentialsRef,
	}

	if err := r.groupService.Register(r.ctx, opts); err != nil {
		r.log.Error(err, "failed to register AccessGroup configuration")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonCreateFailed, err.Error())
		r.Recorder.Event(r.accessGroup, corev1.EventTypeWarning, controller.EventReasonCreateFailed, err.Error())
		return ctrl.Result{}, err
	}

	// Check if already synced (SyncState may have been created and synced in a previous reconcile)
	syncStatus, err := r.groupService.GetSyncStatus(r.ctx, source, r.accessGroup.Status.GroupID)
	if err != nil {
		r.log.Error(err, "failed to get sync status")
		if err := r.updateStatusPending(credInfo.AccountID); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	if syncStatus != nil && syncStatus.IsSynced && syncStatus.GroupID != "" {
		// Already synced, update status to Ready
		if err := r.updateStatusReady(credInfo.AccountID, syncStatus.GroupID); err != nil {
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

// updateStatusPending updates the AccessGroup status to Pending state.
func (r *Reconciler) updateStatusPending(accountID string) error {
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.accessGroup, func() {
		r.accessGroup.Status.ObservedGeneration = r.accessGroup.Generation

		// Keep existing state if already active, otherwise set to pending
		if r.accessGroup.Status.State != "Ready" {
			r.accessGroup.Status.State = "pending"
		}

		// Set account ID
		if accountID != "" {
			r.accessGroup.Status.AccountID = accountID
		}

		r.setCondition(metav1.ConditionFalse, "Pending", "Configuration registered, waiting for sync")
	})

	if err != nil {
		r.log.Error(err, "failed to update AccessGroup status")
		return err
	}

	r.log.Info("AccessGroup configuration registered", "name", r.accessGroup.Name)
	r.Recorder.Event(r.accessGroup, corev1.EventTypeNormal, "Registered",
		"Configuration registered to SyncState")
	return nil
}

// updateStatusReady updates the AccessGroup status to Ready state.
func (r *Reconciler) updateStatusReady(accountID, groupID string) error {
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.accessGroup, func() {
		r.accessGroup.Status.ObservedGeneration = r.accessGroup.Generation
		r.accessGroup.Status.State = "Ready"
		r.accessGroup.Status.AccountID = accountID
		r.accessGroup.Status.GroupID = groupID
		r.setCondition(metav1.ConditionTrue, "Synced", "AccessGroup synced to Cloudflare")
	})

	if err != nil {
		r.log.Error(err, "failed to update AccessGroup status to Ready")
		return err
	}

	r.log.Info("AccessGroup synced successfully", "name", r.accessGroup.Name, "groupId", groupID)
	r.Recorder.Event(r.accessGroup, corev1.EventTypeNormal, "Synced",
		fmt.Sprintf("AccessGroup synced to Cloudflare with ID %s", groupID))
	return nil
}

// setCondition sets a condition on the AccessGroup status.
func (r *Reconciler) setCondition(status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&r.accessGroup.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: r.accessGroup.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("accessgroup-controller")
	r.groupService = accesssvc.NewGroupService(mgr.GetClient())
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.AccessGroup{}).
		Complete(r)
}
