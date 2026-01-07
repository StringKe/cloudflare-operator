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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/cloudflare/cloudflare-go"

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

// initAPIClient initializes the Cloudflare API client from the referenced Secret.
func (r *Reconciler) initAPIClient() error {
	// Get the secret containing API credentials
	secret := &corev1.Secret{}
	secretName := r.privateService.Spec.Cloudflare.Secret
	namespace := r.privateService.Namespace

	if err := r.Get(r.ctx, apitypes.NamespacedName{Name: secretName, Namespace: namespace}, secret); err != nil {
		r.log.Error(err, "failed to get secret", "secret", secretName, "namespace", namespace)
		r.Recorder.Event(r.privateService, corev1.EventTypeWarning, controller.EventReasonAPIError, "Failed to get API secret")
		return err
	}

	// Extract API token or key
	apiToken := string(secret.Data[r.privateService.Spec.Cloudflare.CLOUDFLARE_API_TOKEN])
	apiKey := string(secret.Data[r.privateService.Spec.Cloudflare.CLOUDFLARE_API_KEY])

	if apiToken == "" && apiKey == "" {
		err := fmt.Errorf("neither API token nor API key found in secret")
		r.log.Error(err, "missing credentials in secret")
		return err
	}

	// Create cloudflare client
	var cloudflareClient *cloudflare.API
	var err error
	if apiToken != "" {
		cloudflareClient, err = cloudflare.NewWithAPIToken(apiToken)
	} else {
		cloudflareClient, err = cloudflare.New(apiKey, r.privateService.Spec.Cloudflare.Email)
	}
	if err != nil {
		r.log.Error(err, "failed to create cloudflare client")
		return err
	}

	r.cfAPI = &cf.API{
		Log:              r.log,
		AccountName:      r.privateService.Spec.Cloudflare.AccountName,
		AccountId:        r.privateService.Spec.Cloudflare.AccountId,
		ValidAccountId:   r.privateService.Status.AccountID,
		CloudflareClient: cloudflareClient,
	}

	return nil
}

// handleDeletion handles the deletion of a PrivateService.
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.privateService, controller.PrivateServiceFinalizer) {
		return ctrl.Result{}, nil
	}

	r.log.Info("Deleting PrivateService")
	r.Recorder.Event(r.privateService, corev1.EventTypeNormal, "Deleting", "Starting PrivateService deletion")

	// Delete the tunnel route from Cloudflare if it exists
	if r.privateService.Status.Network != "" {
		virtualNetworkID := r.privateService.Status.VirtualNetworkID
		if virtualNetworkID == "" {
			virtualNetworkID = "default"
		}

		if err := r.cfAPI.DeleteTunnelRoute(r.privateService.Status.Network, virtualNetworkID); err != nil {
			r.log.Error(err, "failed to delete tunnel route from Cloudflare")
			r.Recorder.Event(r.privateService, corev1.EventTypeWarning, controller.EventReasonDeleteFailed, err.Error())
			return ctrl.Result{}, err
		}
		r.log.Info("Tunnel route deleted from Cloudflare", "network", r.privateService.Status.Network)
		r.Recorder.Event(r.privateService, corev1.EventTypeNormal, controller.EventReasonDeleted, "Deleted from Cloudflare")
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
		r.log.Info("Found existing tunnel route, adopting", "network", existing.Network)
		return r.updateStatus(network, existing.TunnelID, tunnelName, existing.VirtualNetworkID, svc.Spec.ClusterIP)
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

	comment := r.privateService.Spec.Comment
	if comment == "" {
		comment = fmt.Sprintf("PrivateService %s/%s", r.privateService.Namespace, r.privateService.Name)
	}

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
	r.privateService.Status.Network = network
	r.privateService.Status.ServiceIP = serviceIP
	r.privateService.Status.TunnelID = tunnelID
	r.privateService.Status.TunnelName = tunnelName
	r.privateService.Status.VirtualNetworkID = virtualNetworkID
	r.privateService.Status.AccountID = r.cfAPI.ValidAccountId
	r.privateService.Status.State = "active"
	r.privateService.Status.ObservedGeneration = r.privateService.Generation

	r.setCondition(metav1.ConditionTrue, controller.EventReasonReconciled, "PrivateService reconciled successfully")

	if err := r.Status().Update(r.ctx, r.privateService); err != nil {
		r.log.Error(err, "failed to update PrivateService status")
		return err
	}

	r.log.Info("PrivateService status updated", "network", r.privateService.Status.Network, "state", r.privateService.Status.State)
	return nil
}

// setCondition sets a condition on the PrivateService status.
func (r *Reconciler) setCondition(status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: r.privateService.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	// Update or append condition
	found := false
	for i, c := range r.privateService.Status.Conditions {
		if c.Type == condition.Type {
			r.privateService.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		r.privateService.Status.Conditions = append(r.privateService.Status.Conditions, condition)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("privateservice-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.PrivateService{}).
		Complete(r)
}
