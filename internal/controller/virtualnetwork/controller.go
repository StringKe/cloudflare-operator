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
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

// Reconciler reconciles a VirtualNetwork object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Runtime state
	ctx   context.Context
	log   logr.Logger
	vnet  *networkingv1alpha2.VirtualNetwork
	cfAPI *cf.API
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=virtualnetworks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=virtualnetworks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=virtualnetworks/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile implements the reconciliation loop for VirtualNetwork resources.
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

	// Initialize API client
	if err := r.initAPIClient(); err != nil {
		r.log.Error(err, "failed to initialize API client")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonAPIError, err.Error())
		return ctrl.Result{}, err
	}

	// Check if VirtualNetwork is being deleted
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

	// Reconcile the VirtualNetwork
	if err := r.reconcileVirtualNetwork(); err != nil {
		r.log.Error(err, "failed to reconcile VirtualNetwork")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	return ctrl.Result{}, nil
}

// initAPIClient initializes the Cloudflare API client.
// For cluster-scoped resources like VirtualNetwork, credentials are loaded from:
// 1. credentialsRef (recommended) - references CloudflareCredentials resource
// 2. inline secret (legacy) - must be in cloudflare-operator-system namespace
// 3. default CloudflareCredentials - if no credentials specified
func (r *Reconciler) initAPIClient() error {
	// VirtualNetwork is cluster-scoped, use operator namespace for legacy inline secrets
	api, err := cf.NewAPIClientFromDetails(r.ctx, r.Client, controller.OperatorNamespace, r.vnet.Spec.Cloudflare)
	if err != nil {
		r.log.Error(err, "failed to initialize API client")
		r.Recorder.Event(r.vnet, corev1.EventTypeWarning, controller.EventReasonAPIError, "Failed to initialize API client: "+err.Error())
		return err
	}

	// Set additional fields from spec
	api.AccountName = r.vnet.Spec.Cloudflare.AccountName
	if r.vnet.Spec.Cloudflare.AccountId != "" {
		api.AccountId = r.vnet.Spec.Cloudflare.AccountId
	}
	api.ValidAccountId = r.vnet.Status.AccountId

	r.cfAPI = api
	return nil
}

// handleDeletion handles the deletion of a VirtualNetwork.
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.vnet, controller.VirtualNetworkFinalizer) {
		return ctrl.Result{}, nil
	}

	r.log.Info("Deleting VirtualNetwork")
	r.Recorder.Event(r.vnet, corev1.EventTypeNormal, "Deleting", "Starting VirtualNetwork deletion")

	// Try to get VNet ID from status or by looking up by name
	vnetID := r.vnet.Status.VirtualNetworkId
	if vnetID == "" {
		// Status ID is empty - try to find by name to prevent orphaned resources
		vnetName := r.vnet.GetVirtualNetworkName()
		r.log.Info("Status.VirtualNetworkId is empty, trying to find VNet by name", "name", vnetName)
		existing, err := r.cfAPI.GetVirtualNetworkByName(vnetName)
		if err == nil && existing != nil {
			vnetID = existing.ID
			r.log.Info("Found VirtualNetwork by name", "id", vnetID)
		} else {
			r.log.Info("VirtualNetwork not found by name, assuming it was never created or already deleted")
		}
	}

	// Delete from Cloudflare if we have an ID
	if vnetID != "" {
		// P0 FIX: Delete all routes associated with this VirtualNetwork BEFORE deleting the VNet
		// This prevents the "virtual network is used by IP Route(s)" error
		deletedCount, err := r.cfAPI.DeleteTunnelRoutesByVirtualNetworkID(vnetID)
		if err != nil {
			r.log.Error(err, "failed to delete routes for VirtualNetwork", "id", vnetID)
			r.Recorder.Event(r.vnet, corev1.EventTypeWarning,
				"FailedDeletingRoutes", fmt.Sprintf("Failed to delete routes: %v", cf.SanitizeErrorMessage(err)))
			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}
		if deletedCount > 0 {
			r.log.Info("Deleted routes before VirtualNetwork deletion", "vnetId", vnetID, "count", deletedCount)
			r.Recorder.Event(r.vnet, corev1.EventTypeNormal,
				"RoutesDeleted", fmt.Sprintf("Deleted %d routes", deletedCount))
		}

		if err := r.cfAPI.DeleteVirtualNetwork(vnetID); err != nil {
			// P0 FIX: Check if resource is already deleted (NotFound error)
			if !cf.IsNotFoundError(err) {
				r.log.Error(err, "failed to delete VirtualNetwork from Cloudflare")
				r.Recorder.Event(r.vnet, corev1.EventTypeWarning,
					controller.EventReasonDeleteFailed, cf.SanitizeErrorMessage(err))
				return ctrl.Result{RequeueAfter: 30 * time.Second}, err
			}
			r.log.Info("VirtualNetwork already deleted from Cloudflare", "id", vnetID)
			r.Recorder.Event(r.vnet, corev1.EventTypeNormal,
				"AlreadyDeleted", "VirtualNetwork was already deleted from Cloudflare")
		} else {
			r.log.Info("VirtualNetwork deleted from Cloudflare", "id", vnetID)
			r.Recorder.Event(r.vnet, corev1.EventTypeNormal,
				controller.EventReasonDeleted, "Deleted from Cloudflare")
		}
	}

	// P2 FIX: Remove finalizer with retry logic to handle conflicts
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.vnet, func() {
		controllerutil.RemoveFinalizer(r.vnet, controller.VirtualNetworkFinalizer)
	}); err != nil {
		r.log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(r.vnet, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// reconcileVirtualNetwork ensures the VirtualNetwork exists in Cloudflare with the correct configuration.
func (r *Reconciler) reconcileVirtualNetwork() error {
	vnetName := r.vnet.GetVirtualNetworkName()

	// Check if VirtualNetwork already exists in Cloudflare
	if r.vnet.Status.VirtualNetworkId != "" {
		// Update existing VirtualNetwork
		return r.updateVirtualNetwork(vnetName)
	}

	// Try to find existing VirtualNetwork by name
	existing, err := r.cfAPI.GetVirtualNetworkByName(vnetName)
	if err == nil && existing != nil {
		// Check if we can adopt this resource (no conflict with another K8s resource)
		mgmtInfo := controller.NewManagementInfo(r.vnet, "VirtualNetwork")
		if conflict := controller.GetConflictingManager(existing.Comment, mgmtInfo); conflict != nil {
			err := fmt.Errorf("VirtualNetwork %s is already managed by %s/%s", existing.Name, conflict.Kind, conflict.Name)
			r.log.Error(err, "adoption conflict detected")
			r.Recorder.Event(r.vnet, corev1.EventTypeWarning, controller.EventReasonAdoptionConflict, err.Error())
			r.setCondition(metav1.ConditionFalse, controller.EventReasonAdoptionConflict, err.Error())
			return err
		}
		// Found existing - adopt it and update with management marker
		r.log.Info("Found existing VirtualNetwork, adopting", "id", existing.ID, "name", existing.Name)
		r.Recorder.Event(r.vnet, corev1.EventTypeNormal,
			controller.EventReasonAdopted, fmt.Sprintf("Adopted VirtualNetwork: %s", existing.ID))
		return r.updateVirtualNetwork(vnetName)
	}

	// Create new VirtualNetwork
	return r.createVirtualNetwork(vnetName)
}

// createVirtualNetwork creates a new VirtualNetwork in Cloudflare.
func (r *Reconciler) createVirtualNetwork(name string) error {
	r.log.Info("Creating VirtualNetwork", "name", name)
	r.Recorder.Event(r.vnet, corev1.EventTypeNormal, "Creating", fmt.Sprintf("Creating VirtualNetwork: %s", name))

	// Build comment with management marker to prevent adoption conflicts
	mgmtInfo := controller.NewManagementInfo(r.vnet, "VirtualNetwork")
	comment := controller.BuildManagedComment(mgmtInfo, r.vnet.Spec.Comment)

	result, err := r.cfAPI.CreateVirtualNetwork(cf.VirtualNetworkParams{
		Name:             name,
		Comment:          comment,
		IsDefaultNetwork: r.vnet.Spec.IsDefaultNetwork,
	})
	if err != nil {
		r.log.Error(err, "failed to create VirtualNetwork")
		r.Recorder.Event(r.vnet, corev1.EventTypeWarning, controller.EventReasonCreateFailed, err.Error())
		r.setCondition(metav1.ConditionFalse, controller.EventReasonCreateFailed, err.Error())
		return err
	}

	r.Recorder.Event(r.vnet, corev1.EventTypeNormal, controller.EventReasonCreated, fmt.Sprintf("Created VirtualNetwork: %s", result.ID))
	return r.updateStatus(result)
}

// updateVirtualNetwork updates an existing VirtualNetwork in Cloudflare.
func (r *Reconciler) updateVirtualNetwork(name string) error {
	r.log.Info("Updating VirtualNetwork", "id", r.vnet.Status.VirtualNetworkId, "name", name)

	// Build comment with management marker to prevent adoption conflicts
	mgmtInfo := controller.NewManagementInfo(r.vnet, "VirtualNetwork")
	comment := controller.BuildManagedComment(mgmtInfo, r.vnet.Spec.Comment)

	result, err := r.cfAPI.UpdateVirtualNetwork(r.vnet.Status.VirtualNetworkId, cf.VirtualNetworkParams{
		Name:             name,
		Comment:          comment,
		IsDefaultNetwork: r.vnet.Spec.IsDefaultNetwork,
	})
	if err != nil {
		r.log.Error(err, "failed to update VirtualNetwork")
		r.Recorder.Event(r.vnet, corev1.EventTypeWarning, controller.EventReasonUpdateFailed, err.Error())
		r.setCondition(metav1.ConditionFalse, controller.EventReasonUpdateFailed, err.Error())
		return err
	}

	r.Recorder.Event(r.vnet, corev1.EventTypeNormal, controller.EventReasonUpdated, "VirtualNetwork updated")
	return r.updateStatus(result)
}

// updateStatus updates the VirtualNetwork status with the Cloudflare state.
func (r *Reconciler) updateStatus(result *cf.VirtualNetworkResult) error {
	// Use retry logic for status updates to handle conflicts
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.vnet, func() {
		r.vnet.Status.VirtualNetworkId = result.ID
		r.vnet.Status.AccountId = r.cfAPI.ValidAccountId
		r.vnet.Status.IsDefault = result.IsDefaultNetwork
		r.vnet.Status.ObservedGeneration = r.vnet.Generation

		if result.DeletedAt != nil {
			r.vnet.Status.State = "deleted"
		} else {
			r.vnet.Status.State = "active"
		}

		r.setCondition(metav1.ConditionTrue, controller.EventReasonReconciled, "VirtualNetwork reconciled successfully")
	})

	if err != nil {
		r.log.Error(err, "failed to update VirtualNetwork status")
		return err
	}

	r.log.Info("VirtualNetwork status updated", "id", r.vnet.Status.VirtualNetworkId, "state", r.vnet.Status.State)
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
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.VirtualNetwork{}).
		Complete(r)
}
