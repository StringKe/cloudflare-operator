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

package networkroute

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

// Reconciler reconciles a NetworkRoute object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Runtime state
	ctx          context.Context
	log          logr.Logger
	networkRoute *networkingv1alpha2.NetworkRoute
	cfAPI        *cf.API
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

	// Initialize API client
	if err := r.initAPIClient(); err != nil {
		r.log.Error(err, "failed to initialize API client")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonAPIError, err.Error())
		return ctrl.Result{}, err
	}

	// Check if NetworkRoute is being deleted
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

	// Reconcile the NetworkRoute
	if err := r.reconcileNetworkRoute(); err != nil {
		r.log.Error(err, "failed to reconcile NetworkRoute")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	return ctrl.Result{}, nil
}

// initAPIClient initializes the Cloudflare API client from the referenced Secret.
func (r *Reconciler) initAPIClient() error {
	// Get the secret containing API credentials
	secret := &corev1.Secret{}
	secretName := r.networkRoute.Spec.Cloudflare.Secret
	namespace := "cloudflare-operator-system" // default namespace for cluster resources

	if err := r.Get(r.ctx, apitypes.NamespacedName{Name: secretName, Namespace: namespace}, secret); err != nil {
		r.log.Error(err, "failed to get secret", "secret", secretName, "namespace", namespace)
		r.Recorder.Event(r.networkRoute, corev1.EventTypeWarning, controller.EventReasonAPIError, "Failed to get API secret")
		return err
	}

	// Extract API token or key
	apiToken := string(secret.Data[r.networkRoute.Spec.Cloudflare.CLOUDFLARE_API_TOKEN])
	apiKey := string(secret.Data[r.networkRoute.Spec.Cloudflare.CLOUDFLARE_API_KEY])

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
		cloudflareClient, err = cloudflare.New(apiKey, r.networkRoute.Spec.Cloudflare.Email)
	}
	if err != nil {
		r.log.Error(err, "failed to create cloudflare client")
		return err
	}

	r.cfAPI = &cf.API{
		Log:              r.log,
		AccountName:      r.networkRoute.Spec.Cloudflare.AccountName,
		AccountId:        r.networkRoute.Spec.Cloudflare.AccountId,
		ValidAccountId:   r.networkRoute.Status.AccountID,
		CloudflareClient: cloudflareClient,
	}

	return nil
}

// handleDeletion handles the deletion of a NetworkRoute.
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.networkRoute, controller.NetworkRouteFinalizer) {
		return ctrl.Result{}, nil
	}

	r.log.Info("Deleting NetworkRoute")
	r.Recorder.Event(r.networkRoute, corev1.EventTypeNormal, "Deleting", "Starting NetworkRoute deletion")

	// Delete from Cloudflare if it exists
	if r.networkRoute.Status.Network != "" {
		virtualNetworkID := r.networkRoute.Status.VirtualNetworkID
		if virtualNetworkID == "" {
			// If no vnet is specified, we need to get the default one
			virtualNetworkID = "default"
		}

		if err := r.cfAPI.DeleteTunnelRoute(r.networkRoute.Status.Network, virtualNetworkID); err != nil {
			r.log.Error(err, "failed to delete NetworkRoute from Cloudflare")
			r.Recorder.Event(r.networkRoute, corev1.EventTypeWarning, controller.EventReasonDeleteFailed, err.Error())
			return ctrl.Result{}, err
		}
		r.log.Info("NetworkRoute deleted from Cloudflare", "network", r.networkRoute.Status.Network)
		r.Recorder.Event(r.networkRoute, corev1.EventTypeNormal, controller.EventReasonDeleted, "Deleted from Cloudflare")
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(r.networkRoute, controller.NetworkRouteFinalizer)
	if err := r.Update(r.ctx, r.networkRoute); err != nil {
		r.log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(r.networkRoute, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// reconcileNetworkRoute ensures the NetworkRoute exists in Cloudflare with the correct configuration.
func (r *Reconciler) reconcileNetworkRoute() error {
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

	// Check if route already exists in Cloudflare
	if r.networkRoute.Status.Network != "" {
		// Update existing route
		return r.updateNetworkRoute(network, tunnelID, tunnelName, virtualNetworkID)
	}

	// Try to find existing route
	existing, err := r.cfAPI.GetTunnelRoute(network, virtualNetworkID)
	if err == nil && existing != nil {
		// Found existing - adopt it
		r.log.Info("Found existing NetworkRoute, adopting", "network", existing.Network, "tunnelId", existing.TunnelID)
		return r.updateStatus(existing, tunnelName)
	}

	// Create new route
	return r.createNetworkRoute(network, tunnelID, tunnelName, virtualNetworkID)
}

// resolveTunnelRef resolves the TunnelRef to get the tunnel ID.
func (r *Reconciler) resolveTunnelRef() (string, string, error) {
	ref := r.networkRoute.Spec.TunnelRef

	if ref.Kind == "Tunnel" {
		// Get Tunnel resource
		tunnel := &networkingv1alpha2.Tunnel{}
		namespace := ref.Namespace
		if namespace == "" {
			namespace = "default"
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

// createNetworkRoute creates a new NetworkRoute in Cloudflare.
func (r *Reconciler) createNetworkRoute(network, tunnelID, tunnelName, virtualNetworkID string) error {
	r.log.Info("Creating NetworkRoute", "network", network, "tunnelId", tunnelID)
	r.Recorder.Event(r.networkRoute, corev1.EventTypeNormal, "Creating", fmt.Sprintf("Creating NetworkRoute: %s", network))

	result, err := r.cfAPI.CreateTunnelRoute(cf.TunnelRouteParams{
		Network:          network,
		TunnelID:         tunnelID,
		VirtualNetworkID: virtualNetworkID,
		Comment:          r.networkRoute.Spec.Comment,
	})
	if err != nil {
		r.log.Error(err, "failed to create NetworkRoute")
		r.Recorder.Event(r.networkRoute, corev1.EventTypeWarning, controller.EventReasonCreateFailed, err.Error())
		r.setCondition(metav1.ConditionFalse, controller.EventReasonCreateFailed, err.Error())
		return err
	}

	r.Recorder.Event(r.networkRoute, corev1.EventTypeNormal, controller.EventReasonCreated, fmt.Sprintf("Created NetworkRoute: %s", result.Network))
	return r.updateStatus(result, tunnelName)
}

// updateNetworkRoute updates an existing NetworkRoute in Cloudflare.
func (r *Reconciler) updateNetworkRoute(network, tunnelID, tunnelName, virtualNetworkID string) error {
	r.log.Info("Updating NetworkRoute", "network", network, "tunnelId", tunnelID)

	result, err := r.cfAPI.UpdateTunnelRoute(r.networkRoute.Status.Network, cf.TunnelRouteParams{
		Network:          network,
		TunnelID:         tunnelID,
		VirtualNetworkID: virtualNetworkID,
		Comment:          r.networkRoute.Spec.Comment,
	})
	if err != nil {
		r.log.Error(err, "failed to update NetworkRoute")
		r.Recorder.Event(r.networkRoute, corev1.EventTypeWarning, controller.EventReasonUpdateFailed, err.Error())
		r.setCondition(metav1.ConditionFalse, controller.EventReasonUpdateFailed, err.Error())
		return err
	}

	r.Recorder.Event(r.networkRoute, corev1.EventTypeNormal, controller.EventReasonUpdated, "NetworkRoute updated")
	return r.updateStatus(result, tunnelName)
}

// updateStatus updates the NetworkRoute status with the Cloudflare state.
func (r *Reconciler) updateStatus(result *cf.TunnelRouteResult, tunnelName string) error {
	r.networkRoute.Status.Network = result.Network
	r.networkRoute.Status.TunnelID = result.TunnelID
	r.networkRoute.Status.TunnelName = tunnelName
	if result.TunnelName != "" {
		r.networkRoute.Status.TunnelName = result.TunnelName
	}
	r.networkRoute.Status.VirtualNetworkID = result.VirtualNetworkID
	r.networkRoute.Status.AccountID = r.cfAPI.ValidAccountId
	r.networkRoute.Status.State = "active"
	r.networkRoute.Status.ObservedGeneration = r.networkRoute.Generation

	r.setCondition(metav1.ConditionTrue, controller.EventReasonReconciled, "NetworkRoute reconciled successfully")

	if err := r.Status().Update(r.ctx, r.networkRoute); err != nil {
		r.log.Error(err, "failed to update NetworkRoute status")
		return err
	}

	r.log.Info("NetworkRoute status updated", "network", r.networkRoute.Status.Network, "state", r.networkRoute.Status.State)
	return nil
}

// setCondition sets a condition on the NetworkRoute status.
func (r *Reconciler) setCondition(status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: r.networkRoute.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	// Update or append condition
	found := false
	for i, c := range r.networkRoute.Status.Conditions {
		if c.Type == condition.Type {
			r.networkRoute.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		r.networkRoute.Status.Conditions = append(r.networkRoute.Status.Conditions, condition)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("networkroute-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.NetworkRoute{}).
		Complete(r)
}
