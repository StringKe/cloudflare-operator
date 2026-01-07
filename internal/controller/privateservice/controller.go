/*
Copyright 2025 Adyanth H.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

// Reconciler reconciles a PrivateService object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Runtime state
	ctx            context.Context
	log            logr.Logger
	privateService *networkingv1alpha2.PrivateService
	cfAPI          *cf.API
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

	// Initialize API client
	if err := r.initAPIClient(); err != nil {
		r.log.Error(err, "failed to initialize API client")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonAPIError, err.Error())
		return ctrl.Result{}, err
	}

	// Check if PrivateService is being deleted
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

	// Reconcile the PrivateService
	if err := r.reconcilePrivateService(); err != nil {
		r.log.Error(err, "failed to reconcile PrivateService")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	return ctrl.Result{}, nil
}

// initAPIClient initializes the Cloudflare API client using the unified credential loader.
func (r *Reconciler) initAPIClient() error {
	// Use the unified API client initialization
	// PrivateService is namespace-scoped, so pass the namespace for legacy secret lookup
	api, err := cf.NewAPIClientFromDetails(r.ctx, r.Client, r.privateService.Namespace, r.privateService.Spec.Cloudflare)
	if err != nil {
		r.log.Error(err, "failed to initialize API client")
		r.Recorder.Event(r.privateService, corev1.EventTypeWarning, controller.EventReasonAPIError, "Failed to initialize API client")
		return err
	}

	// Preserve validated account ID from status
	api.ValidAccountId = r.privateService.Status.AccountID
	r.cfAPI = api

	return nil
}

// handleDeletion handles the deletion of a PrivateService.
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.privateService, controller.PrivateServiceFinalizer) {
		return ctrl.Result{}, nil
	}

	r.log.Info("Deleting PrivateService")
	r.Recorder.Event(r.privateService, corev1.EventTypeNormal, "Deleting", "Starting PrivateService deletion")

	// Try to get network from status or compute from Service to prevent orphaned resources
	network := r.privateService.Status.Network
	if network == "" {
		// Status is empty - try to compute network from Service
		r.log.Info("Status.Network is empty, trying to compute from Service")
		svc := &corev1.Service{}
		if err := r.Get(r.ctx, apitypes.NamespacedName{
			Name:      r.privateService.Spec.ServiceRef.Name,
			Namespace: r.privateService.Namespace,
		}, svc); err == nil && svc.Spec.ClusterIP != "" && svc.Spec.ClusterIP != "None" {
			network = fmt.Sprintf("%s/32", svc.Spec.ClusterIP)
			r.log.Info("Computed network from Service ClusterIP", "network", network)
		} else {
			r.log.Info("Could not compute network from Service, assuming route was never created or already deleted")
		}
	}

	// Delete from Cloudflare if we have a network
	if network != "" {
		// Determine virtual network ID - prefer status, fall back to resolving from spec
		virtualNetworkID := r.privateService.Status.VirtualNetworkID
		if virtualNetworkID == "" && r.privateService.Spec.VirtualNetworkRef != nil {
			// Try to resolve from spec reference
			vnet := &networkingv1alpha2.VirtualNetwork{}
			if err := r.Get(r.ctx, apitypes.NamespacedName{Name: r.privateService.Spec.VirtualNetworkRef.Name}, vnet); err == nil {
				virtualNetworkID = vnet.Status.VirtualNetworkId
			}
		}
		if virtualNetworkID == "" {
			virtualNetworkID = "default"
		}

		if err := r.cfAPI.DeleteTunnelRoute(network, virtualNetworkID); err != nil {
			// P0 FIX: Check if route is already deleted (NotFound error)
			if !cf.IsNotFoundError(err) {
				r.log.Error(err, "failed to delete tunnel route from Cloudflare")
				r.Recorder.Event(r.privateService, corev1.EventTypeWarning,
					controller.EventReasonDeleteFailed, cf.SanitizeErrorMessage(err))
				return ctrl.Result{RequeueAfter: 30 * time.Second}, err
			}
			r.log.Info("Tunnel route already deleted from Cloudflare", "network", network)
			r.Recorder.Event(r.privateService, corev1.EventTypeNormal,
				"AlreadyDeleted", "Tunnel route was already deleted from Cloudflare")
		} else {
			r.log.Info("Tunnel route deleted from Cloudflare", "network", network)
			r.Recorder.Event(r.privateService, corev1.EventTypeNormal,
				controller.EventReasonDeleted, "Deleted from Cloudflare")
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(r.privateService, controller.PrivateServiceFinalizer)
	if err := r.Update(r.ctx, r.privateService); err != nil {
		r.log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(r.privateService, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// reconcilePrivateService ensures the PrivateService exists in Cloudflare.
func (r *Reconciler) reconcilePrivateService() error {
	// Get the referenced Service to obtain its ClusterIP
	svc := &corev1.Service{}
	if err := r.Get(r.ctx, apitypes.NamespacedName{
		Name:      r.privateService.Spec.ServiceRef.Name,
		Namespace: r.privateService.Namespace,
	}, svc); err != nil {
		r.log.Error(err, "failed to get Service")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonDependencyError, err.Error())
		return err
	}

	if svc.Spec.ClusterIP == "" || svc.Spec.ClusterIP == "None" {
		err := fmt.Errorf("Service %s has no ClusterIP", svc.Name)
		r.log.Error(err, "Service must have a ClusterIP")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonInvalidConfig, err.Error())
		return err
	}

	// Create /32 network CIDR for the service IP
	network := fmt.Sprintf("%s/32", svc.Spec.ClusterIP)

	// Resolve tunnel reference to get tunnel ID
	tunnelID, tunnelName, err := r.resolveTunnelRef()
	if err != nil {
		r.log.Error(err, "failed to resolve tunnel reference")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonDependencyError, err.Error())
		return err
	}

	// Resolve virtual network reference if specified
	virtualNetworkID := ""
	if r.privateService.Spec.VirtualNetworkRef != nil {
		virtualNetworkID, err = r.resolveVirtualNetworkRef()
		if err != nil {
			r.log.Error(err, "failed to resolve virtual network reference")
			r.setCondition(metav1.ConditionFalse, controller.EventReasonDependencyError, err.Error())
			return err
		}
	}

	// Check if route already exists
	if r.privateService.Status.Network != "" {
		// If the service IP changed, we need to update the route
		if r.privateService.Status.Network != network {
			return r.updatePrivateService(network, tunnelID, tunnelName, virtualNetworkID, svc.Spec.ClusterIP)
		}
		// Otherwise, just ensure state is current
		return r.updateStatus(network, tunnelID, tunnelName, virtualNetworkID, svc.Spec.ClusterIP)
	}

	// Try to find existing route
	existing, err := r.cfAPI.GetTunnelRoute(network, virtualNetworkID)
	if err == nil && existing != nil {
		// Check if we can adopt this resource (no conflict with another K8s resource)
		mgmtInfo := controller.NewManagementInfo(r.privateService, "PrivateService")
		if conflict := controller.GetConflictingManager(existing.Comment, mgmtInfo); conflict != nil {
			err := fmt.Errorf("tunnel route %s is already managed by %s/%s/%s", existing.Network, conflict.Kind, conflict.Namespace, conflict.Name)
			r.log.Error(err, "adoption conflict detected")
			r.Recorder.Event(r.privateService, corev1.EventTypeWarning, controller.EventReasonAdoptionConflict, err.Error())
			r.setCondition(metav1.ConditionFalse, controller.EventReasonAdoptionConflict, err.Error())
			return err
		}
		// Found existing - adopt it
		r.log.Info("Found existing tunnel route, adopting", "network", existing.Network)
		r.Recorder.Event(r.privateService, corev1.EventTypeNormal,
			controller.EventReasonAdopted, fmt.Sprintf("Adopted route: %s", existing.Network))
		// Update with management marker
		return r.createPrivateService(network, tunnelID, tunnelName, virtualNetworkID, svc.Spec.ClusterIP)
	}

	// Create new route
	return r.createPrivateService(network, tunnelID, tunnelName, virtualNetworkID, svc.Spec.ClusterIP)
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
			return "", "", fmt.Errorf("Tunnel %s/%s does not have a tunnelId yet", namespace, ref.Name)
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

// createPrivateService creates a new tunnel route for the private service.
func (r *Reconciler) createPrivateService(network, tunnelID, tunnelName, virtualNetworkID, serviceIP string) error {
	r.log.Info("Creating tunnel route for PrivateService", "network", network, "tunnelId", tunnelID)
	r.Recorder.Event(r.privateService, corev1.EventTypeNormal, "Creating", fmt.Sprintf("Creating tunnel route: %s", network))

	// Build comment with management marker to prevent adoption conflicts
	userComment := r.privateService.Spec.Comment
	if userComment == "" {
		userComment = fmt.Sprintf("PrivateService %s/%s", r.privateService.Namespace, r.privateService.Name)
	}
	mgmtInfo := controller.NewManagementInfo(r.privateService, "PrivateService")
	comment := controller.BuildManagedComment(mgmtInfo, userComment)

	result, err := r.cfAPI.CreateTunnelRoute(cf.TunnelRouteParams{
		Network:          network,
		TunnelID:         tunnelID,
		VirtualNetworkID: virtualNetworkID,
		Comment:          comment,
	})
	if err != nil {
		r.log.Error(err, "failed to create tunnel route")
		r.Recorder.Event(r.privateService, corev1.EventTypeWarning, controller.EventReasonCreateFailed, err.Error())
		r.setCondition(metav1.ConditionFalse, controller.EventReasonCreateFailed, err.Error())
		return err
	}

	r.Recorder.Event(r.privateService, corev1.EventTypeNormal, controller.EventReasonCreated, fmt.Sprintf("Created tunnel route: %s", result.Network))
	return r.updateStatus(result.Network, result.TunnelID, tunnelName, result.VirtualNetworkID, serviceIP)
}

// updatePrivateService updates the tunnel route when the service IP changes.
func (r *Reconciler) updatePrivateService(network, tunnelID, tunnelName, virtualNetworkID, serviceIP string) error {
	r.log.Info("Service IP changed, updating tunnel route", "oldNetwork", r.privateService.Status.Network, "newNetwork", network)

	// Delete old route first
	oldVNetID := r.privateService.Status.VirtualNetworkID
	if oldVNetID == "" {
		oldVNetID = "default"
	}
	if err := r.cfAPI.DeleteTunnelRoute(r.privateService.Status.Network, oldVNetID); err != nil {
		r.log.Error(err, "failed to delete old tunnel route")
		// Continue anyway to create new route
	}

	// Create new route
	return r.createPrivateService(network, tunnelID, tunnelName, virtualNetworkID, serviceIP)
}

// updateStatus updates the PrivateService status.
func (r *Reconciler) updateStatus(network, tunnelID, tunnelName, virtualNetworkID, serviceIP string) error {
	// Use retry logic for status updates to handle conflicts
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.privateService, func() {
		r.privateService.Status.Network = network
		r.privateService.Status.ServiceIP = serviceIP
		r.privateService.Status.TunnelID = tunnelID
		r.privateService.Status.TunnelName = tunnelName
		r.privateService.Status.VirtualNetworkID = virtualNetworkID
		r.privateService.Status.AccountID = r.cfAPI.ValidAccountId
		r.privateService.Status.State = "active"
		r.privateService.Status.ObservedGeneration = r.privateService.Generation

		r.setCondition(metav1.ConditionTrue, controller.EventReasonReconciled, "PrivateService reconciled successfully")
	})

	if err != nil {
		r.log.Error(err, "failed to update PrivateService status")
		return err
	}

	r.log.Info("PrivateService status updated", "network", r.privateService.Status.Network, "state", r.privateService.Status.State)
	return nil
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

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("privateservice-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.PrivateService{}).
		Complete(r)
}
