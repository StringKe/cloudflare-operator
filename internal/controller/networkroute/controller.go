// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package networkroute provides a controller for managing Cloudflare Tunnel Routes.
// It directly calls Cloudflare API and writes status back to the CRD.
package networkroute

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/controller/common"
)

const (
	finalizerName = "networkroute.networking.cloudflare-operator.io/finalizer"
	defaultVNet   = "default"
)

// Reconciler reconciles a NetworkRoute object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=networkroutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=networkroutes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=networkroutes/finalizers,verbs=update
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=tunnels,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=clustertunnels,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=virtualnetworks,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile handles NetworkRoute reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the NetworkRoute resource
	route := &networkingv1alpha2.NetworkRoute{}
	if err := r.Get(ctx, req.NamespacedName, route); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch NetworkRoute")
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !route.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, route)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, route, finalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Get API client
	// NetworkRoute is cluster-scoped, use operator namespace for legacy inline secrets
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &route.Spec.Cloudflare,
		Namespace:         common.OperatorNamespace,
		StatusAccountID:   route.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, route, err)
	}

	// Sync network route to Cloudflare
	return r.syncNetworkRoute(ctx, route, apiResult)
}

// handleDeletion handles the deletion of NetworkRoute.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	route *networkingv1alpha2.NetworkRoute,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(route, finalizerName) {
		return common.NoRequeue(), nil
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &route.Spec.Cloudflare,
		Namespace:         common.OperatorNamespace,
		StatusAccountID:   route.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client for deletion")
		// Continue with finalizer removal
	} else if route.Status.Network != "" {
		// Resolve virtual network ID
		vnetID := route.Status.VirtualNetworkID
		if vnetID == "" {
			vnetID = defaultVNet
		}

		// Delete route from Cloudflare
		logger.Info("Deleting NetworkRoute from Cloudflare",
			"network", route.Status.Network,
			"virtualNetworkId", vnetID)

		if err := apiResult.API.DeleteTunnelRoute(ctx, route.Status.Network, vnetID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete NetworkRoute from Cloudflare")
				r.Recorder.Event(route, corev1.EventTypeWarning, "DeleteFailed",
					fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
				return common.RequeueShort(), err
			}
			logger.Info("NetworkRoute not found in Cloudflare, may have been already deleted")
		}

		r.Recorder.Event(route, corev1.EventTypeNormal, "Deleted",
			"NetworkRoute deleted from Cloudflare")
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, route, func() {
		controllerutil.RemoveFinalizer(route, finalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(route, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// syncNetworkRoute syncs the NetworkRoute to Cloudflare.
//
//nolint:revive // cognitive complexity is acceptable for reconciliation with dependency resolution
func (r *Reconciler) syncNetworkRoute(
	ctx context.Context,
	route *networkingv1alpha2.NetworkRoute,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Resolve tunnel reference to get tunnel ID
	tunnelID, tunnelName, err := r.resolveTunnelRef(ctx, route)
	if err != nil {
		logger.Error(err, "Failed to resolve tunnel reference")
		return r.updateStatusError(ctx, route, fmt.Errorf("tunnel dependency not ready: %w", err))
	}

	// Resolve virtual network reference if specified
	virtualNetworkID := ""
	if route.Spec.VirtualNetworkRef != nil {
		virtualNetworkID, err = r.resolveVirtualNetworkRef(ctx, route)
		if err != nil {
			logger.Error(err, "Failed to resolve virtual network reference")
			return r.updateStatusError(ctx, route, fmt.Errorf("virtual network dependency not ready: %w", err))
		}
	}

	network := route.Spec.Network

	// Build params
	params := cf.TunnelRouteParams{
		Network:          network,
		TunnelID:         tunnelID,
		VirtualNetworkID: virtualNetworkID,
		Comment:          r.buildManagedComment(route),
	}

	// Check if route already exists
	existing, err := apiResult.API.GetTunnelRoute(ctx, network, virtualNetworkID)
	if err != nil && !cf.IsNotFoundError(err) {
		logger.Error(err, "Failed to get NetworkRoute from Cloudflare")
		return r.updateStatusError(ctx, route, err)
	}

	if existing != nil {
		// Route exists, check if needs update
		needsUpdate := existing.TunnelID != tunnelID ||
			existing.VirtualNetworkID != virtualNetworkID

		if needsUpdate {
			logger.Info("Updating NetworkRoute in Cloudflare",
				"network", network,
				"tunnelId", tunnelID)

			result, err := apiResult.API.UpdateTunnelRoute(ctx, network, params)
			if err != nil {
				logger.Error(err, "Failed to update NetworkRoute")
				return r.updateStatusError(ctx, route, err)
			}

			r.Recorder.Event(route, corev1.EventTypeNormal, "Updated",
				fmt.Sprintf("NetworkRoute '%s' updated in Cloudflare", network))

			return r.updateStatusReady(ctx, route, apiResult.AccountID, result, tunnelName)
		}

		// No changes needed
		logger.V(1).Info("NetworkRoute already exists and is up to date",
			"network", network)

		return r.updateStatusReady(ctx, route, apiResult.AccountID, existing, tunnelName)
	}

	// Create new route
	logger.Info("Creating NetworkRoute in Cloudflare",
		"network", network,
		"tunnelId", tunnelID)

	result, err := apiResult.API.CreateTunnelRoute(ctx, params)
	if err != nil {
		// Check if it's a conflict (route already exists)
		if cf.IsConflictError(err) {
			existing, getErr := apiResult.API.GetTunnelRoute(ctx, network, virtualNetworkID)
			if getErr == nil && existing != nil {
				logger.Info("NetworkRoute already exists, adopting it",
					"network", network)
				r.Recorder.Event(route, corev1.EventTypeNormal, "Adopted",
					fmt.Sprintf("Adopted existing NetworkRoute '%s'", network))
				return r.updateStatusReady(ctx, route, apiResult.AccountID, existing, tunnelName)
			}
		}
		logger.Error(err, "Failed to create NetworkRoute")
		return r.updateStatusError(ctx, route, err)
	}

	r.Recorder.Event(route, corev1.EventTypeNormal, "Created",
		fmt.Sprintf("NetworkRoute '%s' created in Cloudflare", network))

	return r.updateStatusReady(ctx, route, apiResult.AccountID, result, tunnelName)
}

// resolveTunnelRef resolves the TunnelRef to get the tunnel ID.
func (r *Reconciler) resolveTunnelRef(ctx context.Context, route *networkingv1alpha2.NetworkRoute) (string, string, error) {
	ref := route.Spec.TunnelRef

	if ref.Kind == "Tunnel" {
		// Get Tunnel resource
		tunnel := &networkingv1alpha2.Tunnel{}
		namespace := ref.Namespace
		if namespace == "" {
			namespace = "default"
		}
		if err := r.Get(ctx, apitypes.NamespacedName{Name: ref.Name, Namespace: namespace}, tunnel); err != nil {
			return "", "", fmt.Errorf("failed to get Tunnel %s/%s: %w", namespace, ref.Name, err)
		}
		if tunnel.Status.TunnelId == "" {
			return "", "", fmt.Errorf("tunnel %s/%s does not have a tunnelId yet", namespace, ref.Name)
		}
		return tunnel.Status.TunnelId, tunnel.Status.TunnelName, nil
	}

	// ClusterTunnel (default)
	clusterTunnel := &networkingv1alpha2.ClusterTunnel{}
	if err := r.Get(ctx, apitypes.NamespacedName{Name: ref.Name}, clusterTunnel); err != nil {
		return "", "", fmt.Errorf("failed to get ClusterTunnel %s: %w", ref.Name, err)
	}
	if clusterTunnel.Status.TunnelId == "" {
		return "", "", fmt.Errorf("ClusterTunnel %s does not have a tunnelId yet", ref.Name)
	}
	return clusterTunnel.Status.TunnelId, clusterTunnel.Status.TunnelName, nil
}

// resolveVirtualNetworkRef resolves the VirtualNetworkRef to get the virtual network ID.
func (r *Reconciler) resolveVirtualNetworkRef(ctx context.Context, route *networkingv1alpha2.NetworkRoute) (string, error) {
	ref := route.Spec.VirtualNetworkRef
	if ref == nil {
		return "", nil
	}

	vnet := &networkingv1alpha2.VirtualNetwork{}
	if err := r.Get(ctx, apitypes.NamespacedName{Name: ref.Name}, vnet); err != nil {
		return "", fmt.Errorf("failed to get VirtualNetwork %s: %w", ref.Name, err)
	}
	if vnet.Status.VirtualNetworkId == "" {
		return "", fmt.Errorf("VirtualNetwork %s does not have a virtualNetworkId yet", ref.Name)
	}
	return vnet.Status.VirtualNetworkId, nil
}

// buildManagedComment builds a comment with management marker.
func (r *Reconciler) buildManagedComment(route *networkingv1alpha2.NetworkRoute) string {
	mgmtInfo := controller.NewManagementInfo(route, "NetworkRoute")
	return controller.BuildManagedComment(mgmtInfo, route.Spec.Comment)
}

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	route *networkingv1alpha2.NetworkRoute,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, route, func() {
		route.Status.State = "error"
		meta.SetStatusCondition(&route.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: route.Generation,
			Reason:             "Error",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		route.Status.ObservedGeneration = route.Generation
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
}

func (r *Reconciler) updateStatusReady(
	ctx context.Context,
	route *networkingv1alpha2.NetworkRoute,
	accountID string,
	result *cf.TunnelRouteResult,
	tunnelName string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, route, func() {
		route.Status.AccountID = accountID
		route.Status.Network = result.Network
		route.Status.TunnelID = result.TunnelID
		route.Status.TunnelName = tunnelName
		if result.TunnelName != "" {
			route.Status.TunnelName = result.TunnelName
		}
		route.Status.VirtualNetworkID = result.VirtualNetworkID
		route.Status.State = "active"
		meta.SetStatusCondition(&route.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: route.Generation,
			Reason:             "Synced",
			Message:            "NetworkRoute synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		route.Status.ObservedGeneration = route.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
}

// findNetworkRoutesForVirtualNetwork returns reconcile requests for NetworkRoutes
// that reference the given VirtualNetwork
func (r *Reconciler) findNetworkRoutesForVirtualNetwork(ctx context.Context, obj client.Object) []reconcile.Request {
	vnet, ok := obj.(*networkingv1alpha2.VirtualNetwork)
	if !ok {
		return nil
	}

	routes := &networkingv1alpha2.NetworkRouteList{}
	if err := r.List(ctx, routes); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, route := range routes.Items {
		if route.Spec.VirtualNetworkRef != nil && route.Spec.VirtualNetworkRef.Name == vnet.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: apitypes.NamespacedName{Name: route.Name},
			})
		}
	}
	return requests
}

// findNetworkRoutesForTunnel returns reconcile requests for NetworkRoutes that reference the given Tunnel
func (r *Reconciler) findNetworkRoutesForTunnel(ctx context.Context, obj client.Object) []reconcile.Request {
	tunnel, ok := obj.(*networkingv1alpha2.Tunnel)
	if !ok {
		return nil
	}

	routes := &networkingv1alpha2.NetworkRouteList{}
	if err := r.List(ctx, routes); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, route := range routes.Items {
		if route.Spec.TunnelRef.Kind == "Tunnel" && route.Spec.TunnelRef.Name == tunnel.Name {
			refNamespace := route.Spec.TunnelRef.Namespace
			if refNamespace == "" {
				refNamespace = "default"
			}
			if refNamespace == tunnel.Namespace {
				requests = append(requests, reconcile.Request{
					NamespacedName: apitypes.NamespacedName{Name: route.Name},
				})
			}
		}
	}
	return requests
}

// findNetworkRoutesForClusterTunnel returns reconcile requests for NetworkRoutes that reference the given ClusterTunnel
func (r *Reconciler) findNetworkRoutesForClusterTunnel(ctx context.Context, obj client.Object) []reconcile.Request {
	clusterTunnel, ok := obj.(*networkingv1alpha2.ClusterTunnel)
	if !ok {
		return nil
	}

	routes := &networkingv1alpha2.NetworkRouteList{}
	if err := r.List(ctx, routes); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, route := range routes.Items {
		// ClusterTunnel is the default kind or explicitly specified
		if (route.Spec.TunnelRef.Kind == "" || route.Spec.TunnelRef.Kind == "ClusterTunnel") &&
			route.Spec.TunnelRef.Name == clusterTunnel.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: apitypes.NamespacedName{Name: route.Name},
			})
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("networkroute-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("networkroute"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.NetworkRoute{}).
		Watches(&networkingv1alpha2.VirtualNetwork{},
			handler.EnqueueRequestsFromMapFunc(r.findNetworkRoutesForVirtualNetwork)).
		Watches(&networkingv1alpha2.Tunnel{},
			handler.EnqueueRequestsFromMapFunc(r.findNetworkRoutesForTunnel)).
		Watches(&networkingv1alpha2.ClusterTunnel{},
			handler.EnqueueRequestsFromMapFunc(r.findNetworkRoutesForClusterTunnel)).
		Named("networkroute").
		Complete(r)
}
