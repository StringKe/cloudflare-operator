// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package virtualnetwork provides a controller for managing Cloudflare Virtual Networks.
// It directly calls Cloudflare API and writes status back to the CRD.
package virtualnetwork

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/controller/common"
)

const (
	finalizerName = "virtualnetwork.networking.cloudflare-operator.io/finalizer"
)

// Reconciler reconciles a VirtualNetwork object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=virtualnetworks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=virtualnetworks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=virtualnetworks/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile handles VirtualNetwork reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the VirtualNetwork resource
	vnet := &networkingv1alpha2.VirtualNetwork{}
	if err := r.Get(ctx, req.NamespacedName, vnet); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch VirtualNetwork")
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !vnet.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, vnet)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, vnet, finalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Get API client
	// VirtualNetwork is cluster-scoped, use operator namespace for legacy inline secrets
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &vnet.Spec.Cloudflare,
		Namespace:         common.OperatorNamespace,
		StatusAccountID:   vnet.Status.AccountId,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, vnet, err)
	}

	// Sync virtual network to Cloudflare
	return r.syncVirtualNetwork(ctx, vnet, apiResult)
}

// handleDeletion handles the deletion of VirtualNetwork.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	vnet *networkingv1alpha2.VirtualNetwork,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(vnet, finalizerName) {
		return common.NoRequeue(), nil
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &vnet.Spec.Cloudflare,
		Namespace:         common.OperatorNamespace,
		StatusAccountID:   vnet.Status.AccountId,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client for deletion")
		// Continue with finalizer removal
	} else if vnet.Status.VirtualNetworkId != "" {
		// Delete virtual network from Cloudflare
		logger.Info("Deleting VirtualNetwork from Cloudflare",
			"virtualNetworkId", vnet.Status.VirtualNetworkId)

		if err := apiResult.API.DeleteVirtualNetwork(ctx, vnet.Status.VirtualNetworkId); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete VirtualNetwork from Cloudflare, continuing with finalizer removal")
				r.Recorder.Event(vnet, corev1.EventTypeWarning, "DeleteFailed",
					fmt.Sprintf("Failed to delete from Cloudflare (will remove finalizer anyway): %s", cf.SanitizeErrorMessage(err)))
				// Don't block finalizer removal - resource may need manual cleanup in Cloudflare
			} else {
				logger.Info("VirtualNetwork not found in Cloudflare, may have been already deleted")
			}
		} else {
			r.Recorder.Event(vnet, corev1.EventTypeNormal, "Deleted",
				"VirtualNetwork deleted from Cloudflare")
		}
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, vnet, func() {
		controllerutil.RemoveFinalizer(vnet, finalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(vnet, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// syncVirtualNetwork syncs the VirtualNetwork to Cloudflare.
func (r *Reconciler) syncVirtualNetwork(
	ctx context.Context,
	vnet *networkingv1alpha2.VirtualNetwork,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Determine virtual network name
	vnetName := vnet.GetVirtualNetworkName()

	// Build params
	params := cf.VirtualNetworkParams{
		Name:             vnetName,
		Comment:          r.buildManagedComment(vnet),
		IsDefaultNetwork: vnet.Spec.IsDefaultNetwork,
	}

	// Check if virtual network already exists by ID
	if vnet.Status.VirtualNetworkId != "" {
		existing, err := apiResult.API.GetVirtualNetwork(ctx, vnet.Status.VirtualNetworkId)
		if err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to get VirtualNetwork from Cloudflare")
				return r.updateStatusError(ctx, vnet, err)
			}
			// VNet doesn't exist, will create
			logger.Info("VirtualNetwork not found in Cloudflare, will recreate",
				"virtualNetworkId", vnet.Status.VirtualNetworkId)
		} else {
			// VNet exists, update it
			logger.V(1).Info("Updating VirtualNetwork in Cloudflare",
				"virtualNetworkId", existing.ID,
				"name", vnetName)

			result, err := apiResult.API.UpdateVirtualNetwork(ctx, existing.ID, params)
			if err != nil {
				logger.Error(err, "Failed to update VirtualNetwork")
				return r.updateStatusError(ctx, vnet, err)
			}

			r.Recorder.Event(vnet, corev1.EventTypeNormal, "Updated",
				fmt.Sprintf("VirtualNetwork '%s' updated in Cloudflare", vnetName))

			return r.updateStatusReady(ctx, vnet, apiResult.AccountID, result)
		}
	}

	// Try to find existing virtual network by name
	existingByName, err := apiResult.API.GetVirtualNetworkByName(ctx, vnetName)
	if err != nil && !cf.IsNotFoundError(err) {
		logger.Error(err, "Failed to search for existing VirtualNetwork")
		return r.updateStatusError(ctx, vnet, err)
	}

	if existingByName != nil {
		// VNet already exists with this name, adopt it
		logger.Info("VirtualNetwork already exists with same name, adopting it",
			"virtualNetworkId", existingByName.ID,
			"name", vnetName)

		// Update the existing virtual network
		result, err := apiResult.API.UpdateVirtualNetwork(ctx, existingByName.ID, params)
		if err != nil {
			logger.Error(err, "Failed to update existing VirtualNetwork")
			return r.updateStatusError(ctx, vnet, err)
		}

		r.Recorder.Event(vnet, corev1.EventTypeNormal, "Adopted",
			fmt.Sprintf("Adopted existing VirtualNetwork '%s'", vnetName))

		return r.updateStatusReady(ctx, vnet, apiResult.AccountID, result)
	}

	// Create new virtual network
	logger.Info("Creating VirtualNetwork in Cloudflare",
		"name", vnetName)

	result, err := apiResult.API.CreateVirtualNetwork(ctx, params)
	if err != nil {
		logger.Error(err, "Failed to create VirtualNetwork")
		return r.updateStatusError(ctx, vnet, err)
	}

	r.Recorder.Event(vnet, corev1.EventTypeNormal, "Created",
		fmt.Sprintf("VirtualNetwork '%s' created in Cloudflare", vnetName))

	return r.updateStatusReady(ctx, vnet, apiResult.AccountID, result)
}

// buildManagedComment builds a comment with management marker.
func (r *Reconciler) buildManagedComment(vnet *networkingv1alpha2.VirtualNetwork) string {
	mgmtInfo := controller.NewManagementInfo(vnet, "VirtualNetwork")
	return controller.BuildManagedComment(mgmtInfo, vnet.Spec.Comment)
}

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	vnet *networkingv1alpha2.VirtualNetwork,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, vnet, func() {
		vnet.Status.State = "error"
		meta.SetStatusCondition(&vnet.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: vnet.Generation,
			Reason:             "Error",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		vnet.Status.ObservedGeneration = vnet.Generation
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
}

func (r *Reconciler) updateStatusReady(
	ctx context.Context,
	vnet *networkingv1alpha2.VirtualNetwork,
	accountID string,
	result *cf.VirtualNetworkResult,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, vnet, func() {
		vnet.Status.AccountId = accountID
		vnet.Status.VirtualNetworkId = result.ID
		vnet.Status.State = "active"
		vnet.Status.IsDefault = result.IsDefaultNetwork
		meta.SetStatusCondition(&vnet.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: vnet.Generation,
			Reason:             "Synced",
			Message:            "VirtualNetwork synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		vnet.Status.ObservedGeneration = vnet.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("virtualnetwork-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("virtualnetwork"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.VirtualNetwork{}).
		Named("virtualnetwork").
		Complete(r)
}
