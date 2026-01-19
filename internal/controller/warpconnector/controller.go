// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package warpconnector

import (
	"context"
	"fmt"
	"strconv"
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
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/service"
	warpsvc "github.com/StringKe/cloudflare-operator/internal/service/warp"
)

const (
	FinalizerName = "warpconnector.networking.cloudflare-operator.io/finalizer"
)

// WARPConnectorReconciler reconciles a WARPConnector object
type WARPConnectorReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Service  *warpsvc.ConnectorService // L3 Service for WARP connector operations

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

	// Handle deletion
	if !r.connector.DeletionTimestamp.IsZero() {
		return r.handleDeletion(credInfo)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.connector, FinalizerName) {
		controllerutil.AddFinalizer(r.connector, FinalizerName)
		if err := r.Update(ctx, r.connector); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile the WARP connector
	return r.reconcileWARPConnector(credInfo)
}

// resolveCredentials resolves the credentials reference and returns the credentials info.
// Following Unified Sync Architecture, the Resource Controller only needs
// credential metadata (accountID, credRef) - it does not create a Cloudflare API client.
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
func (r *WARPConnectorReconciler) handleDeletion(credInfo *controller.CredentialsInfo) (ctrl.Result, error) {
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
	secretName := r.connector.Name + "-token"
	if err := r.Get(r.ctx, types.NamespacedName{Name: secretName, Namespace: r.connector.Namespace}, secret); err == nil {
		if err := r.Delete(r.ctx, secret); err != nil && !errors.IsNotFound(err) {
			r.log.Error(err, "Failed to delete Secret")
		}
	}

	// Build routes for deletion
	routes := make([]warpsvc.RouteConfig, len(r.connector.Spec.Routes))
	for i, route := range r.connector.Spec.Routes {
		routes[i] = warpsvc.RouteConfig{
			Network: route.Network,
			Comment: route.Comment,
		}
	}

	// Request deletion via L3 Service (will be processed by L5 Sync Controller)
	if r.connector.Status.ConnectorID != "" || len(r.connector.Spec.Routes) > 0 {
		err := r.Service.RequestDelete(r.ctx, warpsvc.DeleteConnectorOptions{
			ConnectorID:      r.connector.Status.ConnectorID,
			ConnectorName:    r.connector.GetConnectorName(),
			TunnelID:         r.connector.Status.TunnelID,
			VirtualNetworkID: r.connector.Status.VirtualNetworkID,
			AccountID:        credInfo.AccountID,
			Routes:           routes,
			Source: service.Source{
				Kind:      "WARPConnector",
				Namespace: r.connector.Namespace,
				Name:      r.connector.Name,
			},
			CredentialsRef: credInfo.CredentialsRef,
		})
		if err != nil {
			r.log.Error(err, "Failed to request WARP connector deletion")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}
	}

	// Remove finalizer with retry logic to handle conflicts
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
func (r *WARPConnectorReconciler) reconcileWARPConnector(credInfo *controller.CredentialsInfo) (ctrl.Result, error) {
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

	// Build routes config
	routes := make([]warpsvc.RouteConfig, len(r.connector.Spec.Routes))
	for i, route := range r.connector.Spec.Routes {
		routes[i] = warpsvc.RouteConfig{
			Network: route.Network,
			Comment: route.Comment,
		}
	}

	source := service.Source{
		Kind:      "WARPConnector",
		Namespace: r.connector.Namespace,
		Name:      r.connector.Name,
	}

	// Check if connector already exists
	if r.connector.Status.ConnectorID == "" {
		// Request creation via L3 Service
		err := r.Service.RequestCreate(r.ctx, warpsvc.CreateConnectorOptions{
			ConnectorName:    r.connector.GetConnectorName(),
			AccountID:        credInfo.AccountID,
			VirtualNetworkID: vnetID,
			Routes:           routes,
			Source:           source,
			CredentialsRef:   credInfo.CredentialsRef,
		})
		if err != nil {
			r.log.Error(err, "Failed to request WARP connector creation")
			return r.updateStatusError(err)
		}
	} else {
		// Request update via L3 Service
		err := r.Service.RequestUpdate(r.ctx, warpsvc.UpdateConnectorOptions{
			ConnectorID:      r.connector.Status.ConnectorID,
			ConnectorName:    r.connector.GetConnectorName(),
			TunnelID:         r.connector.Status.TunnelID,
			VirtualNetworkID: vnetID,
			AccountID:        credInfo.AccountID,
			Routes:           routes,
			Source:           source,
			CredentialsRef:   credInfo.CredentialsRef,
		})
		if err != nil {
			r.log.Error(err, "Failed to request WARP connector update")
			return r.updateStatusError(err)
		}
	}

	// Check SyncState for results
	syncState, err := r.Service.GetSyncState(r.ctx, r.connector.GetConnectorName())
	if err != nil {
		r.log.V(1).Info("SyncState not yet available, will check on next reconcile")
		return r.updateStatusPending(credInfo.AccountID, vnetID)
	}

	// If SyncState has results, update status from it
	if syncState.Status.SyncStatus == networkingv1alpha2.SyncStatusSynced && syncState.Status.ResultData != nil {
		connectorID := syncState.Status.ResultData[warpsvc.ResultKeyConnectorID]
		tunnelID := syncState.Status.ResultData[warpsvc.ResultKeyTunnelID]
		tunnelToken := syncState.Status.ResultData[warpsvc.ResultKeyTunnelToken]

		// Create or update tunnel token secret
		if tunnelToken != "" {
			if err := r.reconcileSecret(tunnelToken); err != nil {
				return r.updateStatusError(err)
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

		// Parse routes configured
		routesConfigured := 0
		if rc, ok := syncState.Status.ResultData[warpsvc.ResultKeyRoutesConfigured]; ok {
			if parsed, parseErr := strconv.Atoi(rc); parseErr == nil {
				routesConfigured = parsed
			}
		}

		return r.updateStatusSuccess(connectorID, tunnelID, vnetID, credInfo.AccountID, deployment.Status.ReadyReplicas, routesConfigured)
	}

	// SyncState exists but not yet synced
	if syncState.Status.SyncStatus == networkingv1alpha2.SyncStatusError {
		return r.updateStatusError(fmt.Errorf("sync error: %s", syncState.Status.Error))
	}

	return r.updateStatusPending(credInfo.AccountID, vnetID)
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
	// Validate resource requirements before creating/updating deployment
	resources, err := r.buildResources(r.connector.Spec.Resources)
	if err != nil {
		return fmt.Errorf("validate resources: %w", err)
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.connector.Name,
			Namespace: r.connector.Namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(r.ctx, r.Client, deployment, func() error {
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
							Resources: resources,
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

//nolint:revive // cognitive complexity is acceptable for validation logic with error handling
func (*WARPConnectorReconciler) buildResources(res *networkingv1alpha2.ResourceRequirements) (corev1.ResourceRequirements, error) {
	if res == nil {
		return corev1.ResourceRequirements{}, nil
	}

	result := corev1.ResourceRequirements{}

	if res.Limits != nil {
		result.Limits = make(corev1.ResourceList)
		for k, v := range res.Limits {
			quantity, err := resource.ParseQuantity(v)
			if err != nil {
				return corev1.ResourceRequirements{}, fmt.Errorf("invalid resource limit %s=%s: %w", k, v, err)
			}
			result.Limits[corev1.ResourceName(k)] = quantity
		}
	}

	if res.Requests != nil {
		result.Requests = make(corev1.ResourceList)
		for k, v := range res.Requests {
			quantity, err := resource.ParseQuantity(v)
			if err != nil {
				return corev1.ResourceRequirements{}, fmt.Errorf("invalid resource request %s=%s: %w", k, v, err)
			}
			result.Requests[corev1.ResourceName(k)] = quantity
		}
	}

	return result, nil
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
			Message:            err.Error(),
			LastTransitionTime: metav1.Now(),
		})
		r.connector.Status.ObservedGeneration = r.connector.Generation
	})

	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *WARPConnectorReconciler) updateStatusPending(accountID, virtualNetworkID string) (ctrl.Result, error) {
	// Use retry logic for status updates to handle conflicts
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.connector, func() {
		r.connector.Status.State = "Pending"
		r.connector.Status.AccountID = accountID
		r.connector.Status.VirtualNetworkID = virtualNetworkID
		meta.SetStatusCondition(&r.connector.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: r.connector.Generation,
			Reason:             "Pending",
			Message:            "Waiting for WARP connector sync",
			LastTransitionTime: metav1.Now(),
		})
		r.connector.Status.ObservedGeneration = r.connector.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
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
