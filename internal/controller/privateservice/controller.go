// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package privateservice

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
	pssvc "github.com/StringKe/cloudflare-operator/internal/service/privateservice"
)

// Reconciler reconciles a PrivateService object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Services
	privateServiceService *pssvc.Service

	// Runtime state (kept for backwards compatibility with watch handlers)
	ctx            context.Context
	log            logr.Logger
	privateService *networkingv1alpha2.PrivateService
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

// Reconcile implements the reconciliation loop for PrivateService resources.
// Following Unified Sync Architecture:
// K8s Resources → Resource Controllers → Core Services → SyncState CRD → Sync Controllers → Cloudflare API
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrllog.FromContext(ctx)

	// Fetch the PrivateService resource
	r.privateService = &networkingv1alpha2.PrivateService{}
	if err := r.Get(ctx, req.NamespacedName, r.privateService); err != nil {
		if apierrors.IsNotFound(err) {
			r.log.Info("PrivateService deleted, nothing to do")
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch PrivateService")
		return ctrl.Result{}, err
	}

	// Resolve credentials for account ID (needed for SyncState)
	credInfo, err := r.resolveCredentials()
	if err != nil {
		r.log.Error(err, "failed to resolve credentials")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonAPIError, err.Error())
		return ctrl.Result{}, err
	}

	// Check if PrivateService is being deleted
	// Following Unified Sync Architecture: only unregister from SyncState
	// Sync Controller handles actual Cloudflare API deletion
	if r.privateService.GetDeletionTimestamp() != nil {
		return r.handleDeletion()
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.privateService, controller.PrivateServiceFinalizer) {
		controllerutil.AddFinalizer(r.privateService, controller.PrivateServiceFinalizer)
		if err := r.Update(ctx, r.privateService); err != nil {
			r.log.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(r.privateService, corev1.EventTypeNormal, controller.EventReasonFinalizerSet, "Finalizer added")
	}

	// Register PrivateService configuration to SyncState
	// The actual sync to Cloudflare is handled by PrivateServiceSyncController
	return r.registerPrivateService(credInfo)
}

// resolveCredentials resolves the credentials reference and returns the credentials info.
// Following Unified Sync Architecture, the Resource Controller only needs
// credential metadata (accountID, credRef) - it does not create a Cloudflare API client.
func (r *Reconciler) resolveCredentials() (*controller.CredentialsInfo, error) {
	// PrivateService is namespace-scoped, use its namespace for legacy inline secrets
	info, err := controller.ResolveCredentialsForService(
		r.ctx,
		r.Client,
		r.log,
		r.privateService.Spec.Cloudflare,
		r.privateService.Namespace,
		r.privateService.Status.AccountID,
	)
	if err != nil {
		r.log.Error(err, "failed to resolve credentials")
		r.Recorder.Event(r.privateService, corev1.EventTypeWarning, controller.EventReasonAPIError,
			"Failed to resolve credentials: "+err.Error())
		return nil, err
	}

	return info, nil
}

// registerPrivateService registers the PrivateService configuration to SyncState.
// Following Unified Sync Architecture:
// Resource Controller → Core Service → SyncState → Sync Controller → Cloudflare API
//
//nolint:revive // cognitive complexity is acceptable for configuration building
func (r *Reconciler) registerPrivateService(credInfo *controller.CredentialsInfo) (ctrl.Result, error) {
	// Get the referenced Service to obtain its ClusterIP
	svc := &corev1.Service{}
	if err := r.Get(r.ctx, apitypes.NamespacedName{
		Name:      r.privateService.Spec.ServiceRef.Name,
		Namespace: r.privateService.Namespace,
	}, svc); err != nil {
		r.log.Error(err, "failed to get Service")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonDependencyError, err.Error())
		return ctrl.Result{}, err
	}

	if svc.Spec.ClusterIP == "" || svc.Spec.ClusterIP == "None" {
		err := fmt.Errorf("service %s has no ClusterIP", svc.Name)
		r.log.Error(err, "Service must have a ClusterIP")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonInvalidConfig, err.Error())
		return ctrl.Result{}, err
	}

	// Create /32 network CIDR for the service IP
	network := fmt.Sprintf("%s/32", svc.Spec.ClusterIP)

	// Resolve tunnel reference to get tunnel ID
	tunnelID, tunnelName, err := r.resolveTunnelRef()
	if err != nil {
		r.log.Error(err, "failed to resolve tunnel reference")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonDependencyError, err.Error())
		return ctrl.Result{}, err
	}

	// Resolve virtual network reference if specified
	virtualNetworkID := ""
	if r.privateService.Spec.VirtualNetworkRef != nil {
		virtualNetworkID, err = r.resolveVirtualNetworkRef()
		if err != nil {
			r.log.Error(err, "failed to resolve virtual network reference")
			r.setCondition(metav1.ConditionFalse, controller.EventReasonDependencyError, err.Error())
			return ctrl.Result{}, err
		}
	}

	// Build comment with management marker to prevent adoption conflicts
	userComment := r.privateService.Spec.Comment
	if userComment == "" {
		userComment = fmt.Sprintf("PrivateService %s/%s", r.privateService.Namespace, r.privateService.Name)
	}
	mgmtInfo := controller.NewManagementInfo(r.privateService, "PrivateService")
	comment := controller.BuildManagedComment(mgmtInfo, userComment)

	// Build PrivateServiceConfig for registration
	config := pssvc.PrivateServiceConfig{
		Network:          network,
		TunnelID:         tunnelID,
		TunnelName:       tunnelName,
		VirtualNetworkID: virtualNetworkID,
		ServiceIP:        svc.Spec.ClusterIP,
		Comment:          comment,
	}

	// Determine route network for SyncState ID
	// Use existing network from status if available, otherwise use new network
	routeNetwork := r.privateService.Status.Network
	if routeNetwork == "" {
		routeNetwork = "" // Will use pending-{namespace}-{name} placeholder
	}

	// Register with service layer
	source := service.Source{
		Kind:      "PrivateService",
		Namespace: r.privateService.Namespace,
		Name:      r.privateService.Name,
	}

	opts := pssvc.RegisterOptions{
		AccountID:        credInfo.AccountID,
		RouteNetwork:     routeNetwork,
		VirtualNetworkID: virtualNetworkID,
		Source:           source,
		Config:           config,
		CredentialsRef:   credInfo.CredentialsRef,
	}

	if err := r.privateServiceService.Register(r.ctx, opts); err != nil {
		r.log.Error(err, "failed to register PrivateService configuration")
		r.Recorder.Event(r.privateService, corev1.EventTypeWarning, "RegisterFailed",
			fmt.Sprintf("Failed to register configuration: %s", err.Error()))
		r.setCondition(metav1.ConditionFalse, "RegisterFailed", err.Error())
		return ctrl.Result{}, err
	}

	// Check sync status - if already synced, update to Ready state
	// Reuse source from registration
	syncStatus, getSyncErr := r.privateServiceService.GetSyncStatus(r.ctx, source, r.privateService.Status.Network, virtualNetworkID)
	if getSyncErr != nil {
		r.log.V(1).Info("Failed to get sync status, will retry", "error", getSyncErr)
		// Continue with pending status
	}

	// If synced, update to Ready state
	if syncStatus != nil && syncStatus.IsSynced && syncStatus.Network != "" {
		if err := r.updateStatusReady(credInfo.AccountID, syncStatus.Network, svc.Spec.ClusterIP, tunnelID, tunnelName, virtualNetworkID); err != nil {
			return ctrl.Result{}, err
		}
		r.log.Info("PrivateService synced to Cloudflare",
			"network", syncStatus.Network,
			"tunnelId", tunnelID,
			"virtualNetworkId", virtualNetworkID)
		r.Recorder.Event(r.privateService, corev1.EventTypeNormal, "Synced",
			fmt.Sprintf("Route synced for network %s", syncStatus.Network))
		return ctrl.Result{}, nil
	}

	// Update status to pending - Sync Controller will update to active after sync
	if err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.privateService, func() {
		r.privateService.Status.Network = network
		r.privateService.Status.ServiceIP = svc.Spec.ClusterIP
		r.privateService.Status.TunnelID = tunnelID
		r.privateService.Status.TunnelName = tunnelName
		r.privateService.Status.VirtualNetworkID = virtualNetworkID
		r.privateService.Status.AccountID = credInfo.AccountID
		r.privateService.Status.State = "pending"
		r.privateService.Status.ObservedGeneration = r.privateService.Generation

		r.setCondition(metav1.ConditionFalse, "Pending", "Configuration registered, waiting for sync")
	}); err != nil {
		r.log.Error(err, "failed to update PrivateService status")
		return ctrl.Result{}, err
	}

	r.log.Info("PrivateService configuration registered to SyncState",
		"network", network,
		"tunnelId", tunnelID,
		"virtualNetworkId", virtualNetworkID)
	r.Recorder.Event(r.privateService, corev1.EventTypeNormal, "Registered",
		fmt.Sprintf("Configuration registered for network %s", network))

	// Requeue to check sync status
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// updateStatusReady updates the PrivateService status to Ready state after successful sync.
func (r *Reconciler) updateStatusReady(accountID, network, serviceIP, tunnelID, tunnelName, virtualNetworkID string) error {
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.privateService, func() {
		r.privateService.Status.ObservedGeneration = r.privateService.Generation
		r.privateService.Status.State = "active"
		r.privateService.Status.Network = network
		r.privateService.Status.ServiceIP = serviceIP
		r.privateService.Status.TunnelID = tunnelID
		r.privateService.Status.TunnelName = tunnelName
		r.privateService.Status.VirtualNetworkID = virtualNetworkID
		r.privateService.Status.AccountID = accountID

		r.setCondition(metav1.ConditionTrue, "Synced", "PrivateService synced to Cloudflare")
	})
	if err != nil {
		r.log.Error(err, "failed to update PrivateService status to Ready")
		return err
	}
	return nil
}

// handleDeletion handles the deletion of a PrivateService.
// Following Unified Sync Architecture:
// Resource Controller only unregisters from SyncState.
// PrivateService Sync Controller handles the actual Cloudflare API deletion.
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.privateService, controller.PrivateServiceFinalizer) {
		return ctrl.Result{}, nil
	}

	r.log.Info("Unregistering PrivateService from SyncState")

	// Get network and virtual network ID for SyncState lookup
	network := r.privateService.Status.Network
	virtualNetworkID := r.privateService.Status.VirtualNetworkID
	if virtualNetworkID == "" {
		virtualNetworkID = "default"
	}

	// Unregister from SyncState - this triggers Sync Controller to delete from Cloudflare
	// Following: Resource Controller → Core Service → SyncState → Sync Controller → Cloudflare API
	source := service.Source{
		Kind:      "PrivateService",
		Namespace: r.privateService.Namespace,
		Name:      r.privateService.Name,
	}

	if err := r.privateServiceService.Unregister(r.ctx, network, virtualNetworkID, source); err != nil {
		r.log.Error(err, "Failed to unregister PrivateService from SyncState")
		r.Recorder.Event(r.privateService, corev1.EventTypeWarning, "UnregisterFailed",
			fmt.Sprintf("Failed to unregister from SyncState: %s", err.Error()))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	r.Recorder.Event(r.privateService, corev1.EventTypeNormal, "Unregistered",
		"Unregistered from SyncState, Sync Controller will delete from Cloudflare")

	// Remove finalizer with retry logic to handle conflicts
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.privateService, func() {
		controllerutil.RemoveFinalizer(r.privateService, controller.PrivateServiceFinalizer)
	}); err != nil {
		r.log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(r.privateService, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// resolveTunnelRef resolves the TunnelRef to get the tunnel ID.
func (r *Reconciler) resolveTunnelRef() (string, string, error) {
	ref := r.privateService.Spec.TunnelRef

	if ref.Kind == "Tunnel" {
		// Get Tunnel resource
		tunnel := &networkingv1alpha2.Tunnel{}
		namespace := ref.Namespace
		if namespace == "" {
			namespace = r.privateService.Namespace
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
	ref := r.privateService.Spec.VirtualNetworkRef
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

// setCondition sets a condition on the PrivateService status using meta.SetStatusCondition.
func (r *Reconciler) setCondition(status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&r.privateService.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: r.privateService.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
}

// findPrivateServicesForTunnel returns reconcile requests for PrivateServices that reference the given Tunnel
//
//nolint:revive // cognitive-complexity: watch handler logic is inherently complex
func (r *Reconciler) findPrivateServicesForTunnel(ctx context.Context, obj client.Object) []reconcile.Request {
	tunnel, ok := obj.(*networkingv1alpha2.Tunnel)
	if !ok {
		return nil
	}
	log := ctrllog.FromContext(ctx)

	// List all PrivateServices
	privateServices := &networkingv1alpha2.PrivateServiceList{}
	if err := r.List(ctx, privateServices); err != nil {
		log.Error(err, "Failed to list PrivateServices for Tunnel watch")
		return nil
	}

	var requests []reconcile.Request
	for _, ps := range privateServices.Items {
		if ps.Spec.TunnelRef.Kind == "Tunnel" &&
			ps.Spec.TunnelRef.Name == tunnel.Name {
			// Check namespace match (use PrivateService namespace if TunnelRef namespace is empty)
			refNamespace := ps.Spec.TunnelRef.Namespace
			if refNamespace == "" {
				refNamespace = ps.Namespace
			}
			if refNamespace == tunnel.Namespace {
				log.Info("Tunnel changed, triggering PrivateService reconcile",
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
	log := ctrllog.FromContext(ctx)

	// List all PrivateServices
	privateServices := &networkingv1alpha2.PrivateServiceList{}
	if err := r.List(ctx, privateServices); err != nil {
		log.Error(err, "Failed to list PrivateServices for ClusterTunnel watch")
		return nil
	}

	var requests []reconcile.Request
	for _, ps := range privateServices.Items {
		// ClusterTunnel is the default kind or explicitly specified
		if (ps.Spec.TunnelRef.Kind == "" || ps.Spec.TunnelRef.Kind == "ClusterTunnel") &&
			ps.Spec.TunnelRef.Name == clusterTunnel.Name {
			log.Info("ClusterTunnel changed, triggering PrivateService reconcile",
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
	log := ctrllog.FromContext(ctx)

	// List all PrivateServices
	privateServices := &networkingv1alpha2.PrivateServiceList{}
	if err := r.List(ctx, privateServices); err != nil {
		log.Error(err, "Failed to list PrivateServices for VirtualNetwork watch")
		return nil
	}

	var requests []reconcile.Request
	for _, ps := range privateServices.Items {
		if ps.Spec.VirtualNetworkRef != nil && ps.Spec.VirtualNetworkRef.Name == vnet.Name {
			log.Info("VirtualNetwork changed, triggering PrivateService reconcile",
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
	log := ctrllog.FromContext(ctx)

	// List all PrivateServices in the same namespace
	privateServices := &networkingv1alpha2.PrivateServiceList{}
	if err := r.List(ctx, privateServices, client.InNamespace(svc.Namespace)); err != nil {
		log.Error(err, "Failed to list PrivateServices for Service watch")
		return nil
	}

	var requests []reconcile.Request
	for _, ps := range privateServices.Items {
		if ps.Spec.ServiceRef.Name == svc.Name {
			log.Info("Service changed, triggering PrivateService reconcile",
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
	// Initialize the service layer for Unified Sync Architecture
	r.privateServiceService = pssvc.NewService(r.Client)
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.PrivateService{}).
		// P0 FIX: Watch Tunnel changes to trigger PrivateService reconcile when TunnelId becomes available
		Watches(
			&networkingv1alpha2.Tunnel{},
			handler.EnqueueRequestsFromMapFunc(r.findPrivateServicesForTunnel),
		).
		// P0 FIX: Watch ClusterTunnel changes
		Watches(
			&networkingv1alpha2.ClusterTunnel{},
			handler.EnqueueRequestsFromMapFunc(r.findPrivateServicesForClusterTunnel),
		).
		// P0 FIX: Watch VirtualNetwork changes
		Watches(
			&networkingv1alpha2.VirtualNetwork{},
			handler.EnqueueRequestsFromMapFunc(r.findPrivateServicesForVirtualNetwork),
		).
		// P2 FIX: Watch Service changes for ClusterIP updates
		Watches(
			&corev1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.findPrivateServicesForService),
		).
		Complete(r)
}
