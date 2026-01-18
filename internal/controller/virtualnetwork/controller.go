// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package virtualnetwork

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	vnetsvc "github.com/StringKe/cloudflare-operator/internal/service/virtualnetwork"
)

// Reconciler reconciles a VirtualNetwork object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Service layer
	vnetService *vnetsvc.Service

	// Runtime state
	ctx  context.Context
	log  logr.Logger
	vnet *networkingv1alpha2.VirtualNetwork
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=virtualnetworks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=virtualnetworks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=virtualnetworks/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile implements the reconciliation loop for VirtualNetwork resources.
//
//nolint:revive // cognitive complexity is acceptable for this controller reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrllog.FromContext(ctx)

	// Fetch the VirtualNetwork resource
	r.vnet = &networkingv1alpha2.VirtualNetwork{}
	if err := r.Get(ctx, req.NamespacedName, r.vnet); err != nil {
		if apierrors.IsNotFound(err) {
			r.log.Info("VirtualNetwork deleted, nothing to do")
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch VirtualNetwork")
		return ctrl.Result{}, err
	}

	// Check if VirtualNetwork is being deleted
	// Following Unified Sync Architecture: only unregister from SyncState
	// Sync Controller handles actual Cloudflare API deletion
	if r.vnet.GetDeletionTimestamp() != nil {
		return r.handleDeletion()
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.vnet, controller.VirtualNetworkFinalizer) {
		controllerutil.AddFinalizer(r.vnet, controller.VirtualNetworkFinalizer)
		if err := r.Update(ctx, r.vnet); err != nil {
			r.log.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(r.vnet, corev1.EventTypeNormal, controller.EventReasonFinalizerSet, "Finalizer added")
	}

	// Reconcile the VirtualNetwork through service layer
	result, err := r.reconcileVirtualNetwork()
	if err != nil {
		r.log.Error(err, "failed to reconcile VirtualNetwork")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	return result, nil
}

// resolveCredentials resolves the credentials reference and returns the credentials info.
// Following Unified Sync Architecture, the Resource Controller only needs
// credential metadata (accountID, credRef) - it does not create a Cloudflare API client.
func (r *Reconciler) resolveCredentials() (*controller.CredentialsInfo, error) {
	// VirtualNetwork is cluster-scoped, use operator namespace for legacy inline secrets
	info, err := controller.ResolveCredentialsForService(
		r.ctx,
		r.Client,
		r.log,
		r.vnet.Spec.Cloudflare,
		controller.OperatorNamespace,
		r.vnet.Status.AccountId,
	)
	if err != nil {
		r.log.Error(err, "failed to resolve credentials")
		r.Recorder.Event(r.vnet, corev1.EventTypeWarning, controller.EventReasonAPIError,
			"Failed to resolve credentials: "+err.Error())
		return nil, err
	}

	return info, nil
}

// handleDeletion handles the deletion of a VirtualNetwork.
// Following Unified Sync Architecture:
// Resource Controller only unregisters from SyncState.
// VirtualNetwork Sync Controller handles the actual Cloudflare API deletion.
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.vnet, controller.VirtualNetworkFinalizer) {
		return ctrl.Result{}, nil
	}

	r.log.Info("Unregistering VirtualNetwork from SyncState")

	// Get VNet ID from status
	vnetID := r.vnet.Status.VirtualNetworkId

	// Unregister from SyncState - this triggers Sync Controller to delete from Cloudflare
	// Following: Resource Controller → Core Service → SyncState → Sync Controller → Cloudflare API
	source := service.Source{
		Kind: "VirtualNetwork",
		Name: r.vnet.Name,
	}

	if err := r.vnetService.Unregister(r.ctx, vnetID, source); err != nil {
		r.log.Error(err, "failed to unregister from SyncState")
		r.Recorder.Event(r.vnet, corev1.EventTypeWarning, "UnregisterFailed",
			fmt.Sprintf("Failed to unregister from SyncState: %s", err.Error()))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	r.Recorder.Event(r.vnet, corev1.EventTypeNormal, "Unregistered",
		"Unregistered from SyncState, Sync Controller will delete from Cloudflare")

	// Remove finalizer with retry logic to handle conflicts
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.vnet, func() {
		controllerutil.RemoveFinalizer(r.vnet, controller.VirtualNetworkFinalizer)
	}); err != nil {
		r.log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(r.vnet, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// reconcileVirtualNetwork ensures the VirtualNetwork configuration is registered with the service layer.
// Following Unified Sync Architecture:
// Resource Controller → Core Service → SyncState → Sync Controller → Cloudflare API
//
//nolint:revive // cognitive complexity is acceptable for reconciliation logic
func (r *Reconciler) reconcileVirtualNetwork() (ctrl.Result, error) {
	vnetName := r.vnet.GetVirtualNetworkName()

	// Resolve credentials (without creating API client)
	credInfo, err := r.resolveCredentials()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolve credentials: %w", err)
	}

	// Build the configuration
	config := vnetsvc.VirtualNetworkConfig{
		Name:             vnetName,
		Comment:          r.buildManagedComment(),
		IsDefaultNetwork: r.vnet.Spec.IsDefaultNetwork,
	}

	// Build source reference
	source := service.Source{
		Kind: "VirtualNetwork",
		Name: r.vnet.Name,
	}

	// Register with service using credentials info
	opts := vnetsvc.RegisterOptions{
		AccountID:        credInfo.AccountID,
		VirtualNetworkID: r.vnet.Status.VirtualNetworkId,
		Source:           source,
		Config:           config,
		CredentialsRef:   credInfo.CredentialsRef,
	}

	if err := r.vnetService.Register(r.ctx, opts); err != nil {
		r.log.Error(err, "failed to register VirtualNetwork configuration")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonCreateFailed, err.Error())
		r.Recorder.Event(r.vnet, corev1.EventTypeWarning, controller.EventReasonCreateFailed, err.Error())
		return ctrl.Result{}, err
	}

	// Check if already synced (SyncState may have been created and synced in a previous reconcile)
	syncStatus, err := r.vnetService.GetSyncStatus(r.ctx, source, r.vnet.Status.VirtualNetworkId)
	if err != nil {
		r.log.Error(err, "failed to get sync status")
		if err := r.updateStatusPending(credInfo.AccountID); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	if syncStatus != nil && syncStatus.IsSynced && syncStatus.VirtualNetworkID != "" {
		// Already synced, update status to Ready
		if err := r.updateStatusReady(credInfo.AccountID, syncStatus.VirtualNetworkID); err != nil {
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

// buildManagedComment builds a comment with management marker.
func (r *Reconciler) buildManagedComment() string {
	mgmtInfo := controller.NewManagementInfo(r.vnet, "VirtualNetwork")
	return controller.BuildManagedComment(mgmtInfo, r.vnet.Spec.Comment)
}

// updateStatusPending updates the VirtualNetwork status to Pending state.
func (r *Reconciler) updateStatusPending(accountID string) error {
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.vnet, func() {
		r.vnet.Status.ObservedGeneration = r.vnet.Generation

		// Keep existing state if already active, otherwise set to pending
		if r.vnet.Status.State != "active" {
			r.vnet.Status.State = "pending"
		}

		// Set account ID
		if accountID != "" {
			r.vnet.Status.AccountId = accountID
		}

		r.setCondition(metav1.ConditionFalse, "Pending", "Configuration registered, waiting for sync")
	})

	if err != nil {
		r.log.Error(err, "failed to update VirtualNetwork status")
		return err
	}

	r.log.Info("VirtualNetwork configuration registered", "name", r.vnet.Name)
	r.Recorder.Event(r.vnet, corev1.EventTypeNormal, "Registered",
		"Configuration registered to SyncState")
	return nil
}

// updateStatusReady updates the VirtualNetwork status to Ready state.
func (r *Reconciler) updateStatusReady(accountID, vnetID string) error {
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.vnet, func() {
		r.vnet.Status.ObservedGeneration = r.vnet.Generation
		r.vnet.Status.State = "active"
		r.vnet.Status.AccountId = accountID
		r.vnet.Status.VirtualNetworkId = vnetID
		r.setCondition(metav1.ConditionTrue, "Synced", "VirtualNetwork synced to Cloudflare")
	})

	if err != nil {
		r.log.Error(err, "failed to update VirtualNetwork status to Ready")
		return err
	}

	r.log.Info("VirtualNetwork synced successfully", "name", r.vnet.Name, "vnetId", vnetID)
	r.Recorder.Event(r.vnet, corev1.EventTypeNormal, "Synced",
		fmt.Sprintf("VirtualNetwork synced to Cloudflare with ID %s", vnetID))
	return nil
}

// setCondition sets a condition on the VirtualNetwork status using meta.SetStatusCondition.
func (r *Reconciler) setCondition(status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&r.vnet.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: r.vnet.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("virtualnetwork-controller")
	r.vnetService = vnetsvc.NewService(mgr.GetClient())
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.VirtualNetwork{}).
		Complete(r)
}
