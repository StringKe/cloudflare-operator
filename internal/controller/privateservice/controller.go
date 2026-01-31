// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package privateservice provides a controller for managing Cloudflare Private Services.
// It directly calls Cloudflare API and writes status back to the CRD.
package privateservice

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
	finalizerName = "privateservice.networking.cloudflare-operator.io/finalizer"
)

// Reconciler reconciles a PrivateService object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=privateservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=privateservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=privateservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=tunnels,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=clustertunnels,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=virtualnetworks,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile handles PrivateService reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the PrivateService resource
	ps := &networkingv1alpha2.PrivateService{}
	if err := r.Get(ctx, req.NamespacedName, ps); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch PrivateService")
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !ps.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, ps)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, ps, finalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Resolve dependencies before getting API client
	serviceIP, err := r.resolveServiceIP(ctx, ps)
	if err != nil {
		logger.Error(err, "Failed to resolve Service IP")
		return r.updateStatusError(ctx, ps, err, "DependencyError")
	}

	tunnelID, tunnelName, err := r.resolveTunnelRef(ctx, ps)
	if err != nil {
		logger.Error(err, "Failed to resolve Tunnel reference")
		return r.updateStatusError(ctx, ps, err, "DependencyError")
	}

	virtualNetworkID, err := r.resolveVirtualNetworkRef(ctx, ps)
	if err != nil {
		logger.Error(err, "Failed to resolve VirtualNetwork reference")
		return r.updateStatusError(ctx, ps, err, "DependencyError")
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &ps.Spec.Cloudflare,
		Namespace:         ps.Namespace,
		StatusAccountID:   ps.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, ps, err, "APIError")
	}

	// Sync tunnel route to Cloudflare
	return r.syncTunnelRoute(ctx, ps, apiResult, serviceIP, tunnelID, tunnelName, virtualNetworkID)
}

// resolveServiceIP gets the ClusterIP of the referenced Service.
func (r *Reconciler) resolveServiceIP(ctx context.Context, ps *networkingv1alpha2.PrivateService) (string, error) {
	svc := &corev1.Service{}
	if err := r.Get(ctx, apitypes.NamespacedName{
		Name:      ps.Spec.ServiceRef.Name,
		Namespace: ps.Namespace,
	}, svc); err != nil {
		return "", fmt.Errorf("failed to get Service %s: %w", ps.Spec.ServiceRef.Name, err)
	}

	if svc.Spec.ClusterIP == "" || svc.Spec.ClusterIP == "None" {
		return "", fmt.Errorf("service %s has no ClusterIP (headless services not supported)", svc.Name)
	}

	return svc.Spec.ClusterIP, nil
}

// resolveTunnelRef resolves the TunnelRef to get the tunnel ID and name.
func (r *Reconciler) resolveTunnelRef(ctx context.Context, ps *networkingv1alpha2.PrivateService) (string, string, error) {
	ref := ps.Spec.TunnelRef

	if ref.Kind == "Tunnel" {
		tunnel := &networkingv1alpha2.Tunnel{}
		namespace := ref.Namespace
		if namespace == "" {
			namespace = ps.Namespace
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
func (r *Reconciler) resolveVirtualNetworkRef(ctx context.Context, ps *networkingv1alpha2.PrivateService) (string, error) {
	ref := ps.Spec.VirtualNetworkRef
	if ref == nil {
		return "", nil // Will use default VNet
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

// handleDeletion handles the deletion of PrivateService.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	ps *networkingv1alpha2.PrivateService,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(ps, finalizerName) {
		return common.NoRequeue(), nil
	}

	// Only delete if we have a network in status
	if ps.Status.Network != "" {
		// Get API client
		apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
			CloudflareDetails: &ps.Spec.Cloudflare,
			Namespace:         ps.Namespace,
			StatusAccountID:   ps.Status.AccountID,
		})
		if err != nil {
			logger.Error(err, "Failed to get API client for deletion")
			// Continue with finalizer removal
		} else {
			// Determine virtual network ID for deletion
			virtualNetworkID := ps.Status.VirtualNetworkID
			if virtualNetworkID == "" {
				// Try to get default virtual network
				defaultVNet, vnetErr := apiResult.API.GetDefaultVirtualNetwork(ctx)
				if vnetErr != nil {
					logger.Error(vnetErr, "Failed to get default virtual network, will try empty vnet ID")
				} else {
					virtualNetworkID = defaultVNet.ID
				}
			}

			// Delete tunnel route from Cloudflare
			logger.Info("Deleting tunnel route from Cloudflare",
				"network", ps.Status.Network,
				"virtualNetworkId", virtualNetworkID)

			if err := apiResult.API.DeleteTunnelRoute(ctx, ps.Status.Network, virtualNetworkID); err != nil {
				if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to delete tunnel route from Cloudflare, continuing with finalizer removal")
					r.Recorder.Event(ps, corev1.EventTypeWarning, "DeleteFailed",
						fmt.Sprintf("Failed to delete from Cloudflare (will remove finalizer anyway): %s", cf.SanitizeErrorMessage(err)))
					// Don't block finalizer removal - resource may need manual cleanup in Cloudflare
				} else {
					logger.Info("Tunnel route not found in Cloudflare, may have been already deleted")
				}
			} else {
				r.Recorder.Event(ps, corev1.EventTypeNormal, "Deleted",
					"Tunnel route deleted from Cloudflare")
			}
		}
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, ps, func() {
		controllerutil.RemoveFinalizer(ps, finalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(ps, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// syncTunnelRoute syncs the tunnel route to Cloudflare.
//
//nolint:revive // cognitive complexity is acceptable for sync logic
func (r *Reconciler) syncTunnelRoute(
	ctx context.Context,
	ps *networkingv1alpha2.PrivateService,
	apiResult *common.APIClientResult,
	serviceIP, tunnelID, _ /* tunnelName */, virtualNetworkID string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Create /32 network CIDR for the service IP
	network := fmt.Sprintf("%s/32", serviceIP)

	// Build comment with management marker
	userComment := ps.Spec.Comment
	if userComment == "" {
		userComment = fmt.Sprintf("PrivateService %s/%s", ps.Namespace, ps.Name)
	}
	mgmtInfo := controller.NewManagementInfo(ps, "PrivateService")
	comment := controller.BuildManagedComment(mgmtInfo, userComment)

	// Get effective virtual network ID (use default if not specified)
	effectiveVNetID := virtualNetworkID
	if effectiveVNetID == "" {
		defaultVNet, err := apiResult.API.GetDefaultVirtualNetwork(ctx)
		if err != nil {
			logger.Error(err, "Failed to get default virtual network")
			return r.updateStatusError(ctx, ps, err, "APIError")
		}
		effectiveVNetID = defaultVNet.ID
	}

	// Build params
	params := cf.TunnelRouteParams{
		Network:          network,
		TunnelID:         tunnelID,
		VirtualNetworkID: effectiveVNetID,
		Comment:          comment,
	}

	// Check if route already exists by looking for exact network match
	existing, err := apiResult.API.GetTunnelRoute(ctx, network, effectiveVNetID)
	if err != nil && !cf.IsNotFoundError(err) {
		logger.Error(err, "Failed to get tunnel route from Cloudflare")
		return r.updateStatusError(ctx, ps, err, "APIError")
	}

	if existing != nil {
		// Route exists - check if it needs update
		needsUpdate := existing.TunnelID != tunnelID || existing.Comment != comment

		if needsUpdate {
			// Check for adoption conflict
			if conflict := controller.GetConflictingManager(existing.Comment, mgmtInfo); conflict != nil {
				err := fmt.Errorf("tunnel route %s is managed by %s/%s, cannot adopt",
					network, conflict.Kind, conflict.Name)
				logger.Error(err, "Resource adoption conflict")
				return r.updateStatusError(ctx, ps, err, "ConflictError")
			}

			logger.V(1).Info("Updating tunnel route in Cloudflare",
				"network", network,
				"tunnelId", tunnelID)

			result, updateErr := apiResult.API.UpdateTunnelRoute(ctx, network, params)
			if updateErr != nil {
				logger.Error(updateErr, "Failed to update tunnel route")
				return r.updateStatusError(ctx, ps, updateErr, "APIError")
			}

			r.Recorder.Event(ps, corev1.EventTypeNormal, "Updated",
				fmt.Sprintf("Tunnel route '%s' updated in Cloudflare", network))

			return r.updateStatusReady(ctx, ps, apiResult.AccountID, result, serviceIP)
		}

		// Route exists and matches - just update status
		return r.updateStatusReady(ctx, ps, apiResult.AccountID, existing, serviceIP)
	}

	// Also check if route exists in a different VNet (for adoption)
	existingByNetwork, err := apiResult.API.GetTunnelRouteByNetwork(ctx, network)
	if err != nil && !cf.IsNotFoundError(err) {
		logger.Error(err, "Failed to search for existing tunnel route")
		return r.updateStatusError(ctx, ps, err, "APIError")
	}

	if existingByNetwork != nil {
		// Route exists in different VNet
		if existingByNetwork.VirtualNetworkID != effectiveVNetID {
			err := fmt.Errorf("tunnel route %s exists in different virtual network %s",
				network, existingByNetwork.VirtualNetworkID)
			logger.Error(err, "Route exists in different VNet")
			return r.updateStatusError(ctx, ps, err, "ConflictError")
		}
	}

	// Create new route
	logger.Info("Creating tunnel route in Cloudflare",
		"network", network,
		"tunnelId", tunnelID,
		"virtualNetworkId", effectiveVNetID)

	result, err := apiResult.API.CreateTunnelRoute(ctx, params)
	if err != nil {
		logger.Error(err, "Failed to create tunnel route")
		return r.updateStatusError(ctx, ps, err, "APIError")
	}

	r.Recorder.Event(ps, corev1.EventTypeNormal, "Created",
		fmt.Sprintf("Tunnel route '%s' created in Cloudflare", network))

	return r.updateStatusReady(ctx, ps, apiResult.AccountID, result, serviceIP)
}

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	ps *networkingv1alpha2.PrivateService,
	err error,
	reason string,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, ps, func() {
		ps.Status.State = "error"
		meta.SetStatusCondition(&ps.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: ps.Generation,
			Reason:             reason,
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		ps.Status.ObservedGeneration = ps.Generation
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
}

func (r *Reconciler) updateStatusReady(
	ctx context.Context,
	ps *networkingv1alpha2.PrivateService,
	accountID string,
	result *cf.TunnelRouteResult,
	serviceIP string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, ps, func() {
		ps.Status.AccountID = accountID
		ps.Status.Network = result.Network
		ps.Status.ServiceIP = serviceIP
		ps.Status.TunnelID = result.TunnelID
		ps.Status.TunnelName = result.TunnelName
		ps.Status.VirtualNetworkID = result.VirtualNetworkID
		ps.Status.State = "active"
		meta.SetStatusCondition(&ps.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: ps.Generation,
			Reason:             "Synced",
			Message:            "Tunnel route synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		ps.Status.ObservedGeneration = ps.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
}

// findPrivateServicesForTunnel returns reconcile requests for PrivateServices that reference the given Tunnel
//
//nolint:revive // cognitive-complexity: watch handler logic is inherently complex
func (r *Reconciler) findPrivateServicesForTunnel(ctx context.Context, obj client.Object) []reconcile.Request {
	tunnel, ok := obj.(*networkingv1alpha2.Tunnel)
	if !ok {
		return nil
	}
	logger := log.FromContext(ctx)

	privateServices := &networkingv1alpha2.PrivateServiceList{}
	if err := r.List(ctx, privateServices); err != nil {
		logger.Error(err, "Failed to list PrivateServices for Tunnel watch")
		return nil
	}

	var requests []reconcile.Request
	for _, ps := range privateServices.Items {
		if ps.Spec.TunnelRef.Kind == "Tunnel" &&
			ps.Spec.TunnelRef.Name == tunnel.Name {
			refNamespace := ps.Spec.TunnelRef.Namespace
			if refNamespace == "" {
				refNamespace = ps.Namespace
			}
			if refNamespace == tunnel.Namespace {
				logger.V(1).Info("Tunnel changed, triggering PrivateService reconcile",
					"tunnel", tunnel.Name,
					"privateservice", ps.Name)
				requests = append(requests, reconcile.Request{
					NamespacedName: apitypes.NamespacedName{
						Name:      ps.Name,
						Namespace: ps.Namespace,
					},
				})
			}
		}
	}
	return requests
}

// findPrivateServicesForClusterTunnel returns reconcile requests for PrivateServices that reference the given ClusterTunnel
func (r *Reconciler) findPrivateServicesForClusterTunnel(ctx context.Context, obj client.Object) []reconcile.Request {
	clusterTunnel, ok := obj.(*networkingv1alpha2.ClusterTunnel)
	if !ok {
		return nil
	}
	logger := log.FromContext(ctx)

	privateServices := &networkingv1alpha2.PrivateServiceList{}
	if err := r.List(ctx, privateServices); err != nil {
		logger.Error(err, "Failed to list PrivateServices for ClusterTunnel watch")
		return nil
	}

	var requests []reconcile.Request
	for _, ps := range privateServices.Items {
		if (ps.Spec.TunnelRef.Kind == "" || ps.Spec.TunnelRef.Kind == "ClusterTunnel") &&
			ps.Spec.TunnelRef.Name == clusterTunnel.Name {
			logger.V(1).Info("ClusterTunnel changed, triggering PrivateService reconcile",
				"clustertunnel", clusterTunnel.Name,
				"privateservice", ps.Name)
			requests = append(requests, reconcile.Request{
				NamespacedName: apitypes.NamespacedName{
					Name:      ps.Name,
					Namespace: ps.Namespace,
				},
			})
		}
	}
	return requests
}

// findPrivateServicesForVirtualNetwork returns reconcile requests for PrivateServices that reference the given VirtualNetwork
func (r *Reconciler) findPrivateServicesForVirtualNetwork(ctx context.Context, obj client.Object) []reconcile.Request {
	vnet, ok := obj.(*networkingv1alpha2.VirtualNetwork)
	if !ok {
		return nil
	}
	logger := log.FromContext(ctx)

	privateServices := &networkingv1alpha2.PrivateServiceList{}
	if err := r.List(ctx, privateServices); err != nil {
		logger.Error(err, "Failed to list PrivateServices for VirtualNetwork watch")
		return nil
	}

	var requests []reconcile.Request
	for _, ps := range privateServices.Items {
		if ps.Spec.VirtualNetworkRef != nil && ps.Spec.VirtualNetworkRef.Name == vnet.Name {
			logger.V(1).Info("VirtualNetwork changed, triggering PrivateService reconcile",
				"virtualnetwork", vnet.Name,
				"privateservice", ps.Name)
			requests = append(requests, reconcile.Request{
				NamespacedName: apitypes.NamespacedName{
					Name:      ps.Name,
					Namespace: ps.Namespace,
				},
			})
		}
	}
	return requests
}

// findPrivateServicesForService returns reconcile requests for PrivateServices that reference the given Service
func (r *Reconciler) findPrivateServicesForService(ctx context.Context, obj client.Object) []reconcile.Request {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return nil
	}
	logger := log.FromContext(ctx)

	privateServices := &networkingv1alpha2.PrivateServiceList{}
	if err := r.List(ctx, privateServices, client.InNamespace(svc.Namespace)); err != nil {
		logger.Error(err, "Failed to list PrivateServices for Service watch")
		return nil
	}

	var requests []reconcile.Request
	for _, ps := range privateServices.Items {
		if ps.Spec.ServiceRef.Name == svc.Name {
			logger.V(1).Info("Service changed, triggering PrivateService reconcile",
				"service", svc.Name,
				"privateservice", ps.Name)
			requests = append(requests, reconcile.Request{
				NamespacedName: apitypes.NamespacedName{
					Name:      ps.Name,
					Namespace: ps.Namespace,
				},
			})
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("privateservice-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("privateservice"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.PrivateService{}).
		Watches(
			&networkingv1alpha2.Tunnel{},
			handler.EnqueueRequestsFromMapFunc(r.findPrivateServicesForTunnel),
		).
		Watches(
			&networkingv1alpha2.ClusterTunnel{},
			handler.EnqueueRequestsFromMapFunc(r.findPrivateServicesForClusterTunnel),
		).
		Watches(
			&networkingv1alpha2.VirtualNetwork{},
			handler.EnqueueRequestsFromMapFunc(r.findPrivateServicesForVirtualNetwork),
		).
		Watches(
			&corev1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.findPrivateServicesForService),
		).
		Named("privateservice").
		Complete(r)
}
