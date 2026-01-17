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
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
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
	cfAPI       *cf.API
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
	if r.accessGroup.GetDeletionTimestamp() != nil {
		// Initialize API client for deletion
		if err := r.initAPIClient(); err != nil {
			r.log.Error(err, "failed to initialize API client for deletion")
			r.setCondition(metav1.ConditionFalse, controller.EventReasonAPIError, err.Error())
			return ctrl.Result{}, err
		}
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
	if err := r.reconcileAccessGroup(); err != nil {
		r.log.Error(err, "failed to reconcile AccessGroup")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	return ctrl.Result{}, nil
}

// initAPIClient initializes the Cloudflare API client.
func (r *Reconciler) initAPIClient() error {
	// AccessGroup is cluster-scoped, use operator namespace for legacy inline secrets
	api, err := cf.NewAPIClientFromDetails(r.ctx, r.Client, controller.OperatorNamespace, r.accessGroup.Spec.Cloudflare)
	if err != nil {
		r.log.Error(err, "failed to initialize API client")
		r.Recorder.Event(r.accessGroup, corev1.EventTypeWarning, controller.EventReasonAPIError, "Failed to initialize API client: "+err.Error())
		return err
	}

	api.ValidAccountId = r.accessGroup.Status.AccountID
	r.cfAPI = api
	return nil
}

// handleDeletion handles the deletion of an AccessGroup.
//
//nolint:revive // cognitive complexity is acceptable for deletion handling
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.accessGroup, FinalizerName) {
		return ctrl.Result{}, nil
	}

	r.log.Info("Deleting AccessGroup")
	r.Recorder.Event(r.accessGroup, corev1.EventTypeNormal, "Deleting", "Starting AccessGroup deletion")

	// Try to get Group ID from status or by looking up by name
	groupID := r.accessGroup.Status.GroupID
	if groupID == "" {
		// Status ID is empty - try to find by name to prevent orphaned resources
		groupName := r.accessGroup.GetAccessGroupName()
		r.log.Info("Status.GroupID is empty, trying to find group by name", "name", groupName)
		existing, err := r.cfAPI.ListAccessGroupsByName(groupName)
		if err == nil && existing != nil {
			groupID = existing.ID
			r.log.Info("Found AccessGroup by name", "id", groupID)
		} else {
			r.log.Info("AccessGroup not found by name, assuming it was never created or already deleted")
		}
	}

	// Delete from Cloudflare if we have an ID
	if groupID != "" {
		if err := r.cfAPI.DeleteAccessGroup(groupID); err != nil {
			if !cf.IsNotFoundError(err) {
				r.log.Error(err, "failed to delete AccessGroup from Cloudflare")
				r.Recorder.Event(r.accessGroup, corev1.EventTypeWarning,
					controller.EventReasonDeleteFailed, cf.SanitizeErrorMessage(err))
				return ctrl.Result{RequeueAfter: 30 * time.Second}, err
			}
			r.log.Info("AccessGroup already deleted from Cloudflare", "id", groupID)
			r.Recorder.Event(r.accessGroup, corev1.EventTypeNormal,
				"AlreadyDeleted", "AccessGroup was already deleted from Cloudflare")
		} else {
			r.log.Info("AccessGroup deleted from Cloudflare", "id", groupID)
			r.Recorder.Event(r.accessGroup, corev1.EventTypeNormal,
				controller.EventReasonDeleted, "Deleted from Cloudflare")
		}
	}

	// Unregister from SyncState
	source := service.Source{
		Kind: "AccessGroup",
		Name: r.accessGroup.Name,
	}
	if err := r.groupService.Unregister(r.ctx, groupID, source); err != nil {
		r.log.Error(err, "failed to unregister from SyncState")
		// Non-fatal - continue with finalizer removal
	}

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
func (r *Reconciler) reconcileAccessGroup() error {
	groupName := r.accessGroup.GetAccessGroupName()

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

	// Build credentials reference
	credRef := networkingv1alpha2.CredentialsReference{
		Name: r.accessGroup.Spec.Cloudflare.CredentialsRef.Name,
	}

	// Get account ID - need to initialize API client first if not already done
	accountID := r.accessGroup.Status.AccountID
	if accountID == "" {
		// Initialize API client to get account ID
		if err := r.initAPIClient(); err != nil {
			return fmt.Errorf("initialize API client for account ID: %w", err)
		}
		accountID, _ = r.cfAPI.GetAccountId()
	}

	// Register with service
	opts := accesssvc.AccessGroupRegisterOptions{
		AccountID:      accountID,
		GroupID:        r.accessGroup.Status.GroupID,
		Source:         source,
		Config:         config,
		CredentialsRef: credRef,
	}

	if err := r.groupService.Register(r.ctx, opts); err != nil {
		r.log.Error(err, "failed to register AccessGroup configuration")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonCreateFailed, err.Error())
		r.Recorder.Event(r.accessGroup, corev1.EventTypeWarning, controller.EventReasonCreateFailed, err.Error())
		return err
	}

	// Update status to Pending if not already synced
	return r.updateStatusPending()
}

// updateStatusPending updates the AccessGroup status to Pending state.
//
//nolint:revive // cognitive complexity is acceptable for status update logic
func (r *Reconciler) updateStatusPending() error {
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.accessGroup, func() {
		r.accessGroup.Status.ObservedGeneration = r.accessGroup.Generation

		// Keep existing state if already active, otherwise set to pending
		if r.accessGroup.Status.State != "Ready" {
			r.accessGroup.Status.State = "pending"
		}

		// Set account ID if we have it
		if r.cfAPI != nil {
			if accountID, err := r.cfAPI.GetAccountId(); err == nil {
				r.accessGroup.Status.AccountID = accountID
			}
		}

		r.setCondition(metav1.ConditionTrue, "Pending", "Configuration registered, waiting for sync")
	})

	if err != nil {
		r.log.Error(err, "failed to update AccessGroup status")
		return err
	}

	r.log.Info("AccessGroup configuration registered", "name", r.accessGroup.Name)
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
