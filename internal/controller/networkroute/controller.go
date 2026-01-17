// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package networkroute

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
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/service"
	routesvc "github.com/StringKe/cloudflare-operator/internal/service/networkroute"
)

const defaultValue = "default"

// Reconciler reconciles a NetworkRoute object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Service layer
	routeService *routesvc.Service

	// Runtime state
	ctx          context.Context
	log          logr.Logger
	networkRoute *networkingv1alpha2.NetworkRoute
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=networkroutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=networkroutes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=networkroutes/finalizers,verbs=update
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=tunnels,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=clustertunnels,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=virtualnetworks,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile implements the reconciliation loop for NetworkRoute resources.
//
//nolint:revive // cognitive complexity is acceptable for this controller reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrllog.FromContext(ctx)

	// Fetch the NetworkRoute resource
	r.networkRoute = &networkingv1alpha2.NetworkRoute{}
	if err := r.Get(ctx, req.NamespacedName, r.networkRoute); err != nil {
		if apierrors.IsNotFound(err) {
			r.log.Info("NetworkRoute deleted, nothing to do")
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch NetworkRoute")
		return ctrl.Result{}, err
	}

	// Check if NetworkRoute is being deleted
	// Following Unified Sync Architecture: only unregister from SyncState
	// Sync Controller handles actual Cloudflare API deletion
	if r.networkRoute.GetDeletionTimestamp() != nil {
		return r.handleDeletion()
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.networkRoute, controller.NetworkRouteFinalizer) {
		controllerutil.AddFinalizer(r.networkRoute, controller.NetworkRouteFinalizer)
		if err := r.Update(ctx, r.networkRoute); err != nil {
			r.log.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(r.networkRoute, corev1.EventTypeNormal, controller.EventReasonFinalizerSet, "Finalizer added")
	}

	// Reconcile the NetworkRoute through service layer
	if err := r.reconcileNetworkRoute(); err != nil {
		r.log.Error(err, "failed to reconcile NetworkRoute")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	return ctrl.Result{}, nil
}

// resolveCredentials resolves the credentials reference and returns the credentials info.
// Following Unified Sync Architecture, the Resource Controller only needs
// credential metadata (accountID, credRef) - it does not create a Cloudflare API client.
func (r *Reconciler) resolveCredentials() (*controller.CredentialsInfo, error) {
	// NetworkRoute is cluster-scoped, use operator namespace for legacy inline secrets
	info, err := controller.ResolveCredentialsForService(
		r.ctx,
		r.Client,
		r.log,
		r.networkRoute.Spec.Cloudflare,
		controller.OperatorNamespace,
		r.networkRoute.Status.AccountID,
	)
	if err != nil {
		r.log.Error(err, "failed to resolve credentials")
		r.Recorder.Event(r.networkRoute, corev1.EventTypeWarning, controller.EventReasonAPIError,
			"Failed to resolve credentials: "+err.Error())
		return nil, err
	}

	return info, nil
}

// handleDeletion handles the deletion of a NetworkRoute.
// Following Unified Sync Architecture:
// Resource Controller only unregisters from SyncState.
// NetworkRoute Sync Controller handles the actual Cloudflare API deletion.
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.networkRoute, controller.NetworkRouteFinalizer) {
		return ctrl.Result{}, nil
	}

	r.log.Info("Unregistering NetworkRoute from SyncState")

	// Get network and virtual network ID from status
	network := r.networkRoute.Status.Network
	if network == "" {
		network = r.networkRoute.Spec.Network
	}

	virtualNetworkID := r.networkRoute.Status.VirtualNetworkID
	if virtualNetworkID == "" && r.networkRoute.Spec.VirtualNetworkRef != nil {
		// Try to resolve from spec reference
		vnet := &networkingv1alpha2.VirtualNetwork{}
		if err := r.Get(r.ctx, apitypes.NamespacedName{Name: r.networkRoute.Spec.VirtualNetworkRef.Name}, vnet); err == nil {
			virtualNetworkID = vnet.Status.VirtualNetworkId
		}
	}
	if virtualNetworkID == "" {
		virtualNetworkID = defaultValue
	}

	// Unregister from SyncState - this triggers Sync Controller to delete from Cloudflare
	// Following: Resource Controller → Core Service → SyncState → Sync Controller → Cloudflare API
	source := service.Source{
		Kind: "NetworkRoute",
		Name: r.networkRoute.Name,
	}

	if err := r.routeService.Unregister(r.ctx, network, virtualNetworkID, source); err != nil {
		r.log.Error(err, "failed to unregister from SyncState")
		r.Recorder.Event(r.networkRoute, corev1.EventTypeWarning, "UnregisterFailed",
			fmt.Sprintf("Failed to unregister from SyncState: %s", err.Error()))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	r.Recorder.Event(r.networkRoute, corev1.EventTypeNormal, "Unregistered",
		"Unregistered from SyncState, Sync Controller will delete from Cloudflare")

	// Remove finalizer with retry logic to handle conflicts
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.networkRoute, func() {
		controllerutil.RemoveFinalizer(r.networkRoute, controller.NetworkRouteFinalizer)
	}); err != nil {
		r.log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(r.networkRoute, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// reconcileNetworkRoute ensures the NetworkRoute configuration is registered with the service layer.
// Following Unified Sync Architecture:
// Resource Controller → Core Service → SyncState → Sync Controller → Cloudflare API
//
//nolint:revive // cognitive complexity is acceptable for reconciliation with dependency resolution
func (r *Reconciler) reconcileNetworkRoute() error {
	// Resolve credentials (without creating API client)
	credInfo, err := r.resolveCredentials()
	if err != nil {
		return fmt.Errorf("resolve credentials: %w", err)
	}

	// Resolve tunnel reference to get tunnel ID
	tunnelID, tunnelName, err := r.resolveTunnelRef()
	if err != nil {
		r.log.Error(err, "failed to resolve tunnel reference")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonDependencyError, err.Error())
		return err
	}

	// Resolve virtual network reference if specified
	virtualNetworkID := ""
	if r.networkRoute.Spec.VirtualNetworkRef != nil {
		virtualNetworkID, err = r.resolveVirtualNetworkRef()
		if err != nil {
			r.log.Error(err, "failed to resolve virtual network reference")
			r.setCondition(metav1.ConditionFalse, controller.EventReasonDependencyError, err.Error())
			return err
		}
	}

	network := r.networkRoute.Spec.Network

	// Build the configuration
	config := routesvc.NetworkRouteConfig{
		Network:          network,
		TunnelID:         tunnelID,
		TunnelName:       tunnelName,
		VirtualNetworkID: virtualNetworkID,
		Comment:          r.buildManagedComment(),
	}

	// Build source reference
	source := service.Source{
		Kind: "NetworkRoute",
		Name: r.networkRoute.Name,
	}

	// Register with service using credentials info
	opts := routesvc.RegisterOptions{
		AccountID:        credInfo.AccountID,
		RouteNetwork:     r.networkRoute.Status.Network, // Use status network if already created
		VirtualNetworkID: virtualNetworkID,
		Source:           source,
		Config:           config,
		CredentialsRef:   credInfo.CredentialsRef,
	}

	if err := r.routeService.Register(r.ctx, opts); err != nil {
		r.log.Error(err, "failed to register NetworkRoute configuration")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonCreateFailed, err.Error())
		r.Recorder.Event(r.networkRoute, corev1.EventTypeWarning, controller.EventReasonCreateFailed, err.Error())
		return err
	}

	// Update status to Pending if not already synced
	return r.updateStatusPending(tunnelName, credInfo.AccountID)
}

// resolveTunnelRef resolves the TunnelRef to get the tunnel ID.
func (r *Reconciler) resolveTunnelRef() (string, string, error) {
	ref := r.networkRoute.Spec.TunnelRef

	if ref.Kind == "Tunnel" {
		// Get Tunnel resource
		tunnel := &networkingv1alpha2.Tunnel{}
		namespace := ref.Namespace
		if namespace == "" {
			namespace = defaultValue
		}
		if err := r.Get(r.ctx, apitypes.NamespacedName{Name: ref.Name, Namespace: namespace}, tunnel); err != nil {
			return "", "", fmt.Errorf("failed to get Tunnel %s/%s: %w", namespace, ref.Name, err)
		}
		if tunnel.Status.TunnelId == "" {
			return "", "", fmt.Errorf("tunnel %s/%s does not have a tunnelId yet", namespace, ref.Name)
		}
		return tunnel.Status.TunnelId, tunnel.Status.TunnelName, nil
	}

	// ClusterTunnel (default)
	clusterTunnel := &networkingv1alpha2.ClusterTunnel{}
	if err := r.Get(r.ctx, apitypes.NamespacedName{Name: ref.Name}, clusterTunnel); err != nil {
		return "", "", fmt.Errorf("failed to get ClusterTunnel %s: %w", ref.Name, err)
	}
	if clusterTunnel.Status.TunnelId == "" {
		return "", "", fmt.Errorf("ClusterTunnel %s does not have a tunnelId yet", ref.Name)
	}
	return clusterTunnel.Status.TunnelId, clusterTunnel.Status.TunnelName, nil
}

// resolveVirtualNetworkRef resolves the VirtualNetworkRef to get the virtual network ID.
func (r *Reconciler) resolveVirtualNetworkRef() (string, error) {
	ref := r.networkRoute.Spec.VirtualNetworkRef
	if ref == nil {
		return "", nil
	}

	vnet := &networkingv1alpha2.VirtualNetwork{}
	if err := r.Get(r.ctx, apitypes.NamespacedName{Name: ref.Name}, vnet); err != nil {
		return "", fmt.Errorf("failed to get VirtualNetwork %s: %w", ref.Name, err)
	}
	if vnet.Status.VirtualNetworkId == "" {
		return "", fmt.Errorf("VirtualNetwork %s does not have a virtualNetworkId yet", ref.Name)
	}
	return vnet.Status.VirtualNetworkId, nil
}

// buildManagedComment builds a comment with management marker.
func (r *Reconciler) buildManagedComment() string {
	mgmtInfo := controller.NewManagementInfo(r.networkRoute, "NetworkRoute")
	return controller.BuildManagedComment(mgmtInfo, r.networkRoute.Spec.Comment)
}

// updateStatusPending updates the NetworkRoute status to Pending state.
//
//nolint:revive // cognitive complexity unavoidable: status update requires conditional checks for multiple fields
func (r *Reconciler) updateStatusPending(tunnelName, accountID string) error {
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.networkRoute, func() {
		r.networkRoute.Status.ObservedGeneration = r.networkRoute.Generation

		// Keep existing state if already active, otherwise set to pending
		if r.networkRoute.Status.State != "active" {
			r.networkRoute.Status.State = "pending"
		}

		// Update tunnel name if we have it
		if tunnelName != "" && r.networkRoute.Status.TunnelName == "" {
			r.networkRoute.Status.TunnelName = tunnelName
		}

		// Set account ID
		if accountID != "" {
			r.networkRoute.Status.AccountID = accountID
		}

		r.setCondition(metav1.ConditionFalse, "Pending", "Configuration registered, waiting for sync")
	})

	if err != nil {
		r.log.Error(err, "failed to update NetworkRoute status")
		return err
	}

	r.log.Info("NetworkRoute configuration registered", "name", r.networkRoute.Name)
	r.Recorder.Event(r.networkRoute, corev1.EventTypeNormal, "Registered",
		"Configuration registered to SyncState")
	return nil
}

// setCondition sets a condition on the NetworkRoute status using meta.SetStatusCondition.
func (r *Reconciler) setCondition(status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&r.networkRoute.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: r.networkRoute.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
}

// findNetworkRoutesForVirtualNetwork returns reconcile requests for NetworkRoutes
// that reference the given VirtualNetwork
func (r *Reconciler) findNetworkRoutesForVirtualNetwork(ctx context.Context, obj client.Object) []reconcile.Request {
	vnet, ok := obj.(*networkingv1alpha2.VirtualNetwork)
	if !ok {
		return nil
	}
	log := ctrllog.FromContext(ctx)

	// List all NetworkRoutes
	routes := &networkingv1alpha2.NetworkRouteList{}
	if err := r.List(ctx, routes); err != nil {
		log.Error(err, "Failed to list NetworkRoutes for VirtualNetwork watch")
		return nil
	}

	var requests []reconcile.Request
	for _, route := range routes.Items {
		if route.Spec.VirtualNetworkRef != nil && route.Spec.VirtualNetworkRef.Name == vnet.Name {
			log.Info("VirtualNetwork changed, triggering NetworkRoute reconcile",
				"virtualnetwork", vnet.Name,
				"networkroute", route.Name)
			requests = append(requests, reconcile.Request{
				NamespacedName: apitypes.NamespacedName{
					Name: route.Name,
				},
			})
		}
	}

	return requests
}

// findNetworkRoutesForTunnel returns reconcile requests for NetworkRoutes that reference the given Tunnel
//
//nolint:revive // cognitive-complexity: watch handler logic is inherently complex
func (r *Reconciler) findNetworkRoutesForTunnel(ctx context.Context, obj client.Object) []reconcile.Request {
	tunnel, ok := obj.(*networkingv1alpha2.Tunnel)
	if !ok {
		return nil
	}
	log := ctrllog.FromContext(ctx)

	// List all NetworkRoutes
	routes := &networkingv1alpha2.NetworkRouteList{}
	if err := r.List(ctx, routes); err != nil {
		log.Error(err, "Failed to list NetworkRoutes for Tunnel watch")
		return nil
	}

	var requests []reconcile.Request
	for _, route := range routes.Items {
		if route.Spec.TunnelRef.Kind == "Tunnel" &&
			route.Spec.TunnelRef.Name == tunnel.Name {
			// Check namespace match
			refNamespace := route.Spec.TunnelRef.Namespace
			if refNamespace == "" {
				refNamespace = defaultValue
			}
			if refNamespace == tunnel.Namespace {
				log.Info("Tunnel changed, triggering NetworkRoute reconcile",
					"tunnel", tunnel.Name,
					"networkroute", route.Name)
				requests = append(requests, reconcile.Request{
					NamespacedName: apitypes.NamespacedName{
						Name: route.Name,
					},
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
	log := ctrllog.FromContext(ctx)

	// List all NetworkRoutes
	routes := &networkingv1alpha2.NetworkRouteList{}
	if err := r.List(ctx, routes); err != nil {
		log.Error(err, "Failed to list NetworkRoutes for ClusterTunnel watch")
		return nil
	}

	var requests []reconcile.Request
	for _, route := range routes.Items {
		// ClusterTunnel is the default kind or explicitly specified
		if (route.Spec.TunnelRef.Kind == "" || route.Spec.TunnelRef.Kind == "ClusterTunnel") &&
			route.Spec.TunnelRef.Name == clusterTunnel.Name {
			log.Info("ClusterTunnel changed, triggering NetworkRoute reconcile",
				"clustertunnel", clusterTunnel.Name,
				"networkroute", route.Name)
			requests = append(requests, reconcile.Request{
				NamespacedName: apitypes.NamespacedName{
					Name: route.Name,
				},
			})
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("networkroute-controller")
	r.routeService = routesvc.NewService(mgr.GetClient())
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.NetworkRoute{}).
		// Watch VirtualNetwork changes to trigger NetworkRoute reconcile
		Watches(
			&networkingv1alpha2.VirtualNetwork{},
			handler.EnqueueRequestsFromMapFunc(r.findNetworkRoutesForVirtualNetwork),
		).
		// P0 FIX: Watch Tunnel changes to trigger NetworkRoute reconcile when TunnelId becomes available
		Watches(
			&networkingv1alpha2.Tunnel{},
			handler.EnqueueRequestsFromMapFunc(r.findNetworkRoutesForTunnel),
		).
		// P0 FIX: Watch ClusterTunnel changes
		Watches(
			&networkingv1alpha2.ClusterTunnel{},
			handler.EnqueueRequestsFromMapFunc(r.findNetworkRoutesForClusterTunnel),
		).
		Complete(r)
}
