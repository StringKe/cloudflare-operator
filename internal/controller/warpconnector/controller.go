// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package warpconnector provides a controller for managing Cloudflare WARP Connectors.
// It directly calls Cloudflare API and writes status back to the CRD.
package warpconnector

import (
	"context"
	"fmt"
	"time"

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
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/controller/common"
)

const (
	finalizerName = "warpconnector.networking.cloudflare-operator.io/finalizer"
)

// Reconciler reconciles a WARPConnector object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=warpconnectors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=warpconnectors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=warpconnectors/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile handles WARPConnector reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the WARPConnector resource
	connector := &networkingv1alpha2.WARPConnector{}
	if err := r.Get(ctx, req.NamespacedName, connector); err != nil {
		if errors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch WARPConnector")
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !connector.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, connector)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, connector, finalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Reconcile the WARP connector
	return r.reconcileWARPConnector(ctx, connector)
}

// handleDeletion handles the deletion of WARPConnector.
//
//nolint:revive // cognitive complexity is acceptable for deletion logic
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	connector *networkingv1alpha2.WARPConnector,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(connector, finalizerName) {
		return common.NoRequeue(), nil
	}

	// Delete deployment first
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: connector.Name, Namespace: connector.Namespace}, deployment); err == nil {
		if err := r.Delete(ctx, deployment); err != nil && !errors.IsNotFound(err) {
			logger.Error(err, "Failed to delete Deployment")
		}
	}

	// Delete secret
	secret := &corev1.Secret{}
	secretName := connector.Name + "-token"
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: connector.Namespace}, secret); err == nil {
		if err := r.Delete(ctx, secret); err != nil && !errors.IsNotFound(err) {
			logger.Error(err, "Failed to delete Secret")
		}
	}

	// Delete WARP connector from Cloudflare
	if connector.Status.ConnectorID != "" {
		apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
			CloudflareDetails: &connector.Spec.Cloudflare,
			Namespace:         connector.Namespace,
			StatusAccountID:   connector.Status.AccountID,
		})
		if err != nil {
			logger.Error(err, "Failed to get API client for deletion")
			// Continue with finalizer removal
		} else {
			logger.Info("Deleting WARP Connector from Cloudflare", "connectorId", connector.Status.ConnectorID)
			if err := apiResult.API.DeleteWARPConnector(ctx, connector.Status.ConnectorID); err != nil {
				if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to delete WARP Connector from Cloudflare, continuing with finalizer removal")
					r.Recorder.Event(connector, corev1.EventTypeWarning, "DeleteFailed",
						fmt.Sprintf("Failed to delete from Cloudflare (will remove finalizer anyway): %s", cf.SanitizeErrorMessage(err)))
					// Don't block finalizer removal - resource may need manual cleanup in Cloudflare
				}
			} else {
				r.Recorder.Event(connector, corev1.EventTypeNormal, "Deleted",
					"WARP Connector deleted from Cloudflare")
			}
		}
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, connector, func() {
		controllerutil.RemoveFinalizer(connector, finalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(connector, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// reconcileWARPConnector reconciles the WARP connector.
//
//nolint:revive // cognitive complexity is acceptable for reconciliation logic
func (r *Reconciler) reconcileWARPConnector(
	ctx context.Context,
	connector *networkingv1alpha2.WARPConnector,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Resolve VirtualNetwork reference to get Cloudflare VirtualNetwork ID
	vnetID := ""
	if connector.Spec.VirtualNetworkRef != nil {
		vnet := &networkingv1alpha2.VirtualNetwork{}
		if err := r.Get(ctx, types.NamespacedName{Name: connector.Spec.VirtualNetworkRef.Name}, vnet); err != nil {
			if errors.IsNotFound(err) {
				logger.Error(err, "VirtualNetwork not found", "name", connector.Spec.VirtualNetworkRef.Name)
				return r.updateStatusError(ctx, connector, fmt.Errorf("VirtualNetwork '%s' not found", connector.Spec.VirtualNetworkRef.Name))
			}
			return r.updateStatusError(ctx, connector, err)
		}
		if vnet.Status.VirtualNetworkId == "" {
			logger.Info("VirtualNetwork not yet ready", "name", connector.Spec.VirtualNetworkRef.Name)
			return r.updateStatusError(ctx, connector, fmt.Errorf("VirtualNetwork '%s' is not ready (no Cloudflare ID)", connector.Spec.VirtualNetworkRef.Name))
		}
		vnetID = vnet.Status.VirtualNetworkId
		logger.V(1).Info("Resolved VirtualNetwork", "name", connector.Spec.VirtualNetworkRef.Name, "id", vnetID)
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &connector.Spec.Cloudflare,
		Namespace:         connector.Namespace,
		StatusAccountID:   connector.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, connector, err)
	}

	// Check if connector already exists
	if connector.Status.ConnectorID == "" {
		// Create new WARP connector
		return r.createWARPConnector(ctx, connector, apiResult, vnetID)
	}

	// Update existing WARP connector (deployment, etc.)
	return r.updateWARPConnector(ctx, connector, apiResult, vnetID)
}

// createWARPConnector creates a new WARP connector in Cloudflare.
func (r *Reconciler) createWARPConnector(
	ctx context.Context,
	connector *networkingv1alpha2.WARPConnector,
	apiResult *common.APIClientResult,
	vnetID string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	connectorName := connector.GetConnectorName()

	logger.Info("Creating WARP Connector in Cloudflare", "name", connectorName)

	result, err := apiResult.API.CreateWARPConnector(ctx, connectorName)
	if err != nil {
		logger.Error(err, "Failed to create WARP Connector")
		return r.updateStatusError(ctx, connector, err)
	}

	r.Recorder.Event(connector, corev1.EventTypeNormal, "Created",
		fmt.Sprintf("WARP Connector '%s' created in Cloudflare", connectorName))

	// Create tunnel token secret
	if err := r.reconcileSecret(ctx, connector, result.TunnelToken); err != nil {
		logger.Error(err, "Failed to create tunnel token secret")
		return r.updateStatusError(ctx, connector, err)
	}

	// Create deployment
	if err := r.reconcileDeployment(ctx, connector); err != nil {
		logger.Error(err, "Failed to create deployment")
		return r.updateStatusError(ctx, connector, err)
	}

	// Get deployment status
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: connector.Name, Namespace: connector.Namespace}, deployment); err != nil {
		return r.updateStatusError(ctx, connector, err)
	}

	return r.updateStatusSuccess(ctx, connector, apiResult.AccountID, result.ID, result.TunnelID, vnetID, deployment.Status.ReadyReplicas, len(connector.Spec.Routes))
}

// updateWARPConnector updates an existing WARP connector.
func (r *Reconciler) updateWARPConnector(
	ctx context.Context,
	connector *networkingv1alpha2.WARPConnector,
	apiResult *common.APIClientResult,
	vnetID string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get existing tunnel token for secret reconciliation
	tokenResult, err := apiResult.API.GetWARPConnectorToken(ctx, connector.Status.ConnectorID)
	if err != nil {
		logger.Error(err, "Failed to get tunnel token")
		// Continue anyway, might be temporary
	} else {
		// Update tunnel token secret
		if err := r.reconcileSecret(ctx, connector, tokenResult.Token); err != nil {
			logger.Error(err, "Failed to update tunnel token secret")
			return r.updateStatusError(ctx, connector, err)
		}
	}

	// Update deployment
	if err := r.reconcileDeployment(ctx, connector); err != nil {
		logger.Error(err, "Failed to update deployment")
		return r.updateStatusError(ctx, connector, err)
	}

	// Get deployment status
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: connector.Name, Namespace: connector.Namespace}, deployment); err != nil {
		return r.updateStatusError(ctx, connector, err)
	}

	return r.updateStatusSuccess(ctx, connector, apiResult.AccountID, connector.Status.ConnectorID, connector.Status.TunnelID, vnetID, deployment.Status.ReadyReplicas, len(connector.Spec.Routes))
}

// reconcileSecret creates or updates the tunnel token secret.
func (r *Reconciler) reconcileSecret(
	ctx context.Context,
	connector *networkingv1alpha2.WARPConnector,
	token string,
) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connector.Name + "-token",
			Namespace: connector.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		secret.Data = map[string][]byte{
			"TUNNEL_TOKEN": []byte(token),
		}
		return controllerutil.SetControllerReference(connector, secret, r.Scheme)
	})

	return err
}

// reconcileDeployment creates or updates the cloudflared deployment.
func (r *Reconciler) reconcileDeployment(
	ctx context.Context,
	connector *networkingv1alpha2.WARPConnector,
) error {
	// Validate resource requirements
	resources, err := r.buildResources(connector.Spec.Resources)
	if err != nil {
		return fmt.Errorf("validate resources: %w", err)
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connector.Name,
			Namespace: connector.Namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		replicas := connector.Spec.Replicas
		if replicas == 0 {
			replicas = 1
		}

		image := connector.Spec.Image
		if image == "" {
			image = "cloudflare/cloudflared:latest"
		}

		labels := map[string]string{
			"app.kubernetes.io/name":       "warp-connector",
			"app.kubernetes.io/instance":   connector.Name,
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
					ServiceAccountName: connector.Spec.ServiceAccountName,
					NodeSelector:       connector.Spec.NodeSelector,
					Tolerations:        r.buildTolerations(connector.Spec.Tolerations),
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
												Name: connector.Name + "-token",
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

		return controllerutil.SetControllerReference(connector, deployment, r.Scheme)
	})

	return err
}

// buildTolerations converts API tolerations to corev1 tolerations.
func (*Reconciler) buildTolerations(tolerations []networkingv1alpha2.Toleration) []corev1.Toleration {
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

// buildResources converts API resource requirements to corev1 resource requirements.
//
//nolint:revive // cognitive complexity is acceptable for resource building
func (*Reconciler) buildResources(res *networkingv1alpha2.ResourceRequirements) (corev1.ResourceRequirements, error) {
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

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	connector *networkingv1alpha2.WARPConnector,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, connector, func() {
		connector.Status.State = "Error"
		meta.SetStatusCondition(&connector.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: connector.Generation,
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		connector.Status.ObservedGeneration = connector.Generation
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *Reconciler) updateStatusSuccess(
	ctx context.Context,
	connector *networkingv1alpha2.WARPConnector,
	accountID, connectorID, tunnelID, virtualNetworkID string,
	readyReplicas int32,
	routesConfigured int,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, connector, func() {
		connector.Status.ConnectorID = connectorID
		connector.Status.TunnelID = tunnelID
		connector.Status.VirtualNetworkID = virtualNetworkID
		connector.Status.AccountID = accountID
		connector.Status.ReadyReplicas = readyReplicas
		connector.Status.RoutesConfigured = routesConfigured
		connector.Status.State = "Ready"
		meta.SetStatusCondition(&connector.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: connector.Generation,
			Reason:             "Reconciled",
			Message:            "WARP Connector successfully reconciled",
			LastTransitionTime: metav1.Now(),
		})
		connector.Status.ObservedGeneration = connector.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// findWARPConnectorsForVirtualNetwork returns reconcile requests for WARPConnectors
// that reference the given VirtualNetwork.
func (r *Reconciler) findWARPConnectorsForVirtualNetwork(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	vnet, ok := obj.(*networkingv1alpha2.VirtualNetwork)
	if !ok {
		return nil
	}
	logger := log.FromContext(ctx)

	// List all WARPConnectors
	connectors := &networkingv1alpha2.WARPConnectorList{}
	if err := r.List(ctx, connectors); err != nil {
		logger.Error(err, "Failed to list WARPConnectors for VirtualNetwork watch")
		return nil
	}

	var requests []reconcile.Request
	for _, connector := range connectors.Items {
		if connector.Spec.VirtualNetworkRef != nil && connector.Spec.VirtualNetworkRef.Name == vnet.Name {
			logger.V(1).Info("VirtualNetwork changed, triggering WARPConnector reconcile",
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

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("warpconnector-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("warpconnector"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.WARPConnector{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Secret{}).
		// Watch VirtualNetwork changes to trigger WARPConnector reconcile
		Watches(
			&networkingv1alpha2.VirtualNetwork{},
			handler.EnqueueRequestsFromMapFunc(r.findWARPConnectorsForVirtualNetwork),
		).
		Named("warpconnector").
		Complete(r)
}
