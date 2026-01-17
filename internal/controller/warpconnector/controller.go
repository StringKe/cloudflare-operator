// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package warpconnector

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

const (
	FinalizerName = "warpconnector.networking.cloudflare-operator.io/finalizer"
)

// WARPConnectorReconciler reconciles a WARPConnector object
type WARPConnectorReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Runtime state
	ctx       context.Context
	log       logr.Logger
	connector *networkingv1alpha2.WARPConnector
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=warpconnectors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=warpconnectors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=warpconnectors/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *WARPConnectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrllog.FromContext(ctx)

	// Fetch the WARPConnector instance
	r.connector = &networkingv1alpha2.WARPConnector{}
	if err := r.Get(ctx, req.NamespacedName, r.connector); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Resolve credentials (for logging/status purposes)
	credInfo, err := r.resolveCredentials()
	if err != nil {
		r.log.Error(err, "Failed to resolve credentials")
		return r.updateStatusError(err)
	}

	// Initialize API client for WARP connector operations
	// Note: WARPConnector still needs direct API access until migrated to Unified Sync Architecture
	apiClient, err := cf.NewAPIClientFromDetails(ctx, r.Client, r.connector.Namespace, r.connector.Spec.Cloudflare)
	if err != nil {
		r.log.Error(err, "Failed to initialize Cloudflare API client")
		return r.updateStatusError(err)
	}

	// Handle deletion
	if !r.connector.DeletionTimestamp.IsZero() {
		return r.handleDeletion(apiClient)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.connector, FinalizerName) {
		controllerutil.AddFinalizer(r.connector, FinalizerName)
		if err := r.Update(ctx, r.connector); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile the WARP connector
	return r.reconcileWARPConnector(apiClient, credInfo)
}

// resolveCredentials resolves the credentials reference and returns the credentials info.
// Following Unified Sync Architecture, the Resource Controller only needs
// credential metadata (accountID, credRef) - it does not create a Cloudflare API client.
// Note: WARPConnector still requires the API client for CRUD operations but this
// provides credentials info for status tracking and potential future migration.
func (r *WARPConnectorReconciler) resolveCredentials() (*controller.CredentialsInfo, error) {
	// WARPConnector is namespace-scoped, use its namespace for legacy inline secrets
	info, err := controller.ResolveCredentialsForService(
		r.ctx,
		r.Client,
		r.log,
		r.connector.Spec.Cloudflare,
		r.connector.Namespace,
		r.connector.Status.AccountID,
	)
	if err != nil {
		r.log.Error(err, "failed to resolve credentials")
		r.Recorder.Event(r.connector, corev1.EventTypeWarning, controller.EventReasonAPIError,
			"Failed to resolve credentials: "+err.Error())
		return nil, err
	}

	return info, nil
}

//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps
func (r *WARPConnectorReconciler) handleDeletion(apiClient *cf.API) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.connector, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Delete deployment first
	deployment := &appsv1.Deployment{}
	if err := r.Get(r.ctx, types.NamespacedName{Name: r.connector.Name, Namespace: r.connector.Namespace}, deployment); err == nil {
		if err := r.Delete(r.ctx, deployment); err != nil && !errors.IsNotFound(err) {
			r.log.Error(err, "Failed to delete Deployment")
		}
	}

	// Delete secret
	secret := &corev1.Secret{}
	if err := r.Get(r.ctx, types.NamespacedName{Name: r.connector.Name + "-token", Namespace: r.connector.Namespace}, secret); err == nil {
		if err := r.Delete(r.ctx, secret); err != nil && !errors.IsNotFound(err) {
			r.log.Error(err, "Failed to delete Secret")
		}
	}

	// P1 FIX: Delete routes from Cloudflare with error aggregation
	// All routes must be successfully deleted before removing finalizer
	// Use saved VirtualNetworkID from status for proper route deletion
	if r.connector.Status.TunnelID != "" {
		var routeErrors []error
		vnetID := r.connector.Status.VirtualNetworkID // Use saved VirtualNetworkID
		for _, route := range r.connector.Spec.Routes {
			r.log.Info("Deleting route", "network", route.Network, "virtualNetworkId", vnetID)
			if err := apiClient.DeleteTunnelRoute(route.Network, vnetID); err != nil {
				// P0 FIX: Check if route is already deleted (NotFound error)
				if cf.IsNotFoundError(err) {
					r.log.Info("Route already deleted from Cloudflare", "network", route.Network)
				} else {
					r.log.Error(err, "Failed to delete route", "network", route.Network)
					routeErrors = append(routeErrors, fmt.Errorf("delete route %s: %w", route.Network, err))
				}
			}
		}
		// If any route deletion failed, aggregate errors and retry later
		if len(routeErrors) > 0 {
			aggregatedErr := stderrors.Join(routeErrors...)
			r.log.Error(aggregatedErr, "Some routes failed to delete, will retry")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, aggregatedErr
		}
	}

	// Delete WARP connector from Cloudflare
	if r.connector.Status.ConnectorID != "" {
		r.log.Info("Deleting WARP Connector from Cloudflare", "connectorId", r.connector.Status.ConnectorID)
		if err := apiClient.DeleteWARPConnector(r.connector.Status.ConnectorID); err != nil {
			// P0 FIX: Check if connector is already deleted (NotFound error)
			if !cf.IsNotFoundError(err) {
				r.log.Error(err, "Failed to delete WARP Connector from Cloudflare")
				return ctrl.Result{RequeueAfter: 30 * time.Second}, err
			}
			r.log.Info("WARP Connector already deleted from Cloudflare",
				"connectorId", r.connector.Status.ConnectorID)
		}
	}

	// P2 FIX: Remove finalizer with retry logic to handle conflicts
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.connector, func() {
		controllerutil.RemoveFinalizer(r.connector, FinalizerName)
	}); err != nil {
		r.log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.log.Info("Finalizer removed successfully")
	r.Recorder.Event(r.connector, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

//nolint:revive // cognitive complexity unavoidable: reconciliation requires multiple sequential operations
func (r *WARPConnectorReconciler) reconcileWARPConnector(apiClient *cf.API, credInfo *controller.CredentialsInfo) (ctrl.Result, error) {
	var connectorID, tunnelID, tunnelToken string
	var err error

	if r.connector.Status.ConnectorID == "" {
		// Create new WARP connector
		r.log.Info("Creating WARP Connector", "name", r.connector.GetConnectorName())
		result, err := apiClient.CreateWARPConnector(r.connector.GetConnectorName())
		if err != nil {
			return r.updateStatusError(err)
		}
		connectorID = result.ID
		tunnelID = result.TunnelID
		tunnelToken = result.TunnelToken
	} else {
		connectorID = r.connector.Status.ConnectorID
		tunnelID = r.connector.Status.TunnelID
		// Get existing tunnel token
		result, err := apiClient.GetWARPConnectorToken(connectorID)
		if err != nil {
			return r.updateStatusError(err)
		}
		tunnelToken = result.Token
	}

	// Create or update tunnel token secret
	if err := r.reconcileSecret(tunnelToken); err != nil {
		return r.updateStatusError(err)
	}

	// Resolve VirtualNetwork reference to get Cloudflare VirtualNetwork ID
	vnetID := ""
	if r.connector.Spec.VirtualNetworkRef != nil {
		vnet := &networkingv1alpha2.VirtualNetwork{}
		if err := r.Get(r.ctx, types.NamespacedName{Name: r.connector.Spec.VirtualNetworkRef.Name}, vnet); err != nil {
			if errors.IsNotFound(err) {
				r.log.Error(err, "VirtualNetwork not found", "name", r.connector.Spec.VirtualNetworkRef.Name)
				return r.updateStatusError(fmt.Errorf("VirtualNetwork '%s' not found", r.connector.Spec.VirtualNetworkRef.Name))
			}
			return r.updateStatusError(err)
		}
		if vnet.Status.VirtualNetworkId == "" {
			r.log.Info("VirtualNetwork not yet ready", "name", r.connector.Spec.VirtualNetworkRef.Name)
			errMsg := fmt.Errorf("VirtualNetwork '%s' is not ready (no Cloudflare ID)",
				r.connector.Spec.VirtualNetworkRef.Name)
			return r.updateStatusError(errMsg)
		}
		vnetID = vnet.Status.VirtualNetworkId
		r.log.Info("Resolved VirtualNetwork", "name", r.connector.Spec.VirtualNetworkRef.Name, "id", vnetID)
	}

	// Configure routes
	routesConfigured := 0
	for _, route := range r.connector.Spec.Routes {
		r.log.Info("Configuring route", "network", route.Network)
		routeParams := cf.TunnelRouteParams{
			Network:          route.Network,
			TunnelID:         tunnelID,
			VirtualNetworkID: vnetID,
			Comment:          route.Comment,
		}
		if _, err := apiClient.CreateTunnelRoute(routeParams); err != nil {
			r.log.Error(err, "Failed to create route", "network", route.Network)
		} else {
			routesConfigured++
		}
	}

	// Create or update deployment
	if err := r.reconcileDeployment(); err != nil {
		return r.updateStatusError(err)
	}

	// Get deployment status
	deployment := &appsv1.Deployment{}
	if err = r.Get(r.ctx, types.NamespacedName{Name: r.connector.Name, Namespace: r.connector.Namespace}, deployment); err != nil {
		return r.updateStatusError(err)
	}

	// Update status
	return r.updateStatusSuccess(connectorID, tunnelID, vnetID, credInfo.AccountID, deployment.Status.ReadyReplicas, routesConfigured)
}

func (r *WARPConnectorReconciler) reconcileSecret(token string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.connector.Name + "-token",
			Namespace: r.connector.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(r.ctx, r.Client, secret, func() error {
		secret.Data = map[string][]byte{
			"TUNNEL_TOKEN": []byte(token),
		}
		return controllerutil.SetControllerReference(r.connector, secret, r.Scheme)
	})

	return err
}

func (r *WARPConnectorReconciler) reconcileDeployment() error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.connector.Name,
			Namespace: r.connector.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(r.ctx, r.Client, deployment, func() error {
		replicas := r.connector.Spec.Replicas
		if replicas == 0 {
			replicas = 1
		}

		image := r.connector.Spec.Image
		if image == "" {
			image = "cloudflare/cloudflared:latest"
		}

		labels := map[string]string{
			"app.kubernetes.io/name":       "warp-connector",
			"app.kubernetes.io/instance":   r.connector.Name,
			"app.kubernetes.io/managed-by": "cloudflare-operator",
		}

		deployment.Spec = appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: r.connector.Spec.ServiceAccountName,
					NodeSelector:       r.connector.Spec.NodeSelector,
					Tolerations:        r.buildTolerations(r.connector.Spec.Tolerations),
					Containers: []corev1.Container{
						{
							Name:  "cloudflared",
							Image: image,
							Args:  []string{"tunnel", "--no-autoupdate", "run"},
							Env: []corev1.EnvVar{
								{
									Name: "TUNNEL_TOKEN",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: r.connector.Name + "-token",
											},
											Key: "TUNNEL_TOKEN",
										},
									},
								},
							},
							Resources: r.buildResources(r.connector.Spec.Resources),
						},
					},
				},
			},
		}

		return controllerutil.SetControllerReference(r.connector, deployment, r.Scheme)
	})

	return err
}

func (*WARPConnectorReconciler) buildTolerations(tolerations []networkingv1alpha2.Toleration) []corev1.Toleration {
	if len(tolerations) == 0 {
		return nil
	}

	result := make([]corev1.Toleration, 0, len(tolerations))
	for _, t := range tolerations {
		toleration := corev1.Toleration{
			Key:      t.Key,
			Operator: corev1.TolerationOperator(t.Operator),
			Value:    t.Value,
			Effect:   corev1.TaintEffect(t.Effect),
		}
		if t.TolerationSeconds != nil {
			toleration.TolerationSeconds = t.TolerationSeconds
		}
		result = append(result, toleration)
	}

	return result
}

func (r *WARPConnectorReconciler) buildResources(res *networkingv1alpha2.ResourceRequirements) corev1.ResourceRequirements {
	if res == nil {
		return corev1.ResourceRequirements{}
	}

	result := corev1.ResourceRequirements{}

	if res.Limits != nil {
		result.Limits = make(corev1.ResourceList)
		for k, v := range res.Limits {
			result.Limits[corev1.ResourceName(k)] = resource.MustParse(v)
		}
	}

	if res.Requests != nil {
		result.Requests = make(corev1.ResourceList)
		for k, v := range res.Requests {
			result.Requests[corev1.ResourceName(k)] = resource.MustParse(v)
		}
	}

	return result
}

func (r *WARPConnectorReconciler) updateStatusError(err error) (ctrl.Result, error) {
	// Use retry logic for status updates to handle conflicts
	updateErr := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.connector, func() {
		r.connector.Status.State = "Error"
		meta.SetStatusCondition(&r.connector.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: r.connector.Generation,
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		r.connector.Status.ObservedGeneration = r.connector.Generation
	})

	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *WARPConnectorReconciler) updateStatusSuccess(
	connectorID, tunnelID, virtualNetworkID, accountID string,
	readyReplicas int32,
	routesConfigured int,
) (ctrl.Result, error) {
	// Use retry logic for status updates to handle conflicts
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.connector, func() {
		r.connector.Status.ConnectorID = connectorID
		r.connector.Status.TunnelID = tunnelID
		r.connector.Status.VirtualNetworkID = virtualNetworkID // Save for deletion
		r.connector.Status.AccountID = accountID               // Save from credentials
		r.connector.Status.ReadyReplicas = readyReplicas
		r.connector.Status.RoutesConfigured = routesConfigured
		r.connector.Status.State = "Ready"
		meta.SetStatusCondition(&r.connector.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: r.connector.Generation,
			Reason:             "Reconciled",
			Message:            "WARP Connector successfully reconciled",
			LastTransitionTime: metav1.Now(),
		})
		r.connector.Status.ObservedGeneration = r.connector.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// findWARPConnectorsForVirtualNetwork returns reconcile requests for WARPConnectors
// that reference the given VirtualNetwork
func (r *WARPConnectorReconciler) findWARPConnectorsForVirtualNetwork(ctx context.Context, obj client.Object) []reconcile.Request {
	vnet, ok := obj.(*networkingv1alpha2.VirtualNetwork)
	if !ok {
		return nil
	}
	logger := ctrllog.FromContext(ctx)

	// List all WARPConnectors
	connectors := &networkingv1alpha2.WARPConnectorList{}
	if err := r.List(ctx, connectors); err != nil {
		logger.Error(err, "Failed to list WARPConnectors for VirtualNetwork watch")
		return nil
	}

	var requests []reconcile.Request
	for _, connector := range connectors.Items {
		if connector.Spec.VirtualNetworkRef != nil && connector.Spec.VirtualNetworkRef.Name == vnet.Name {
			logger.Info("VirtualNetwork changed, triggering WARPConnector reconcile",
				"virtualnetwork", vnet.Name,
				"warpconnector", connector.Name,
				"namespace", connector.Namespace)
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      connector.Name,
					Namespace: connector.Namespace,
				},
			})
		}
	}

	return requests
}

func (r *WARPConnectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("warpconnector-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.WARPConnector{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Secret{}).
		// Watch VirtualNetwork changes to trigger WARPConnector reconcile
		Watches(
			&networkingv1alpha2.VirtualNetwork{},
			handler.EnqueueRequestsFromMapFunc(r.findWARPConnectorsForVirtualNetwork),
		).
		Complete(r)
}
