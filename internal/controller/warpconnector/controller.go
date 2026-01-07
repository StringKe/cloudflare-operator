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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/adyanth/cloudflare-operator/api/v1alpha2"
	"github.com/adyanth/cloudflare-operator/internal/clients/cf"
)

const (
	FinalizerName = "warpconnector.networking.cfargotunnel.com/finalizer"
)

// WARPConnectorReconciler reconciles a WARPConnector object
type WARPConnectorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=networking.cfargotunnel.com,resources=warpconnectors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cfargotunnel.com,resources=warpconnectors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cfargotunnel.com,resources=warpconnectors/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *WARPConnectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the WARPConnector instance
	connector := &networkingv1alpha2.WARPConnector{}
	if err := r.Get(ctx, req.NamespacedName, connector); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Initialize API client
	apiClient, err := cf.NewAPIClientFromDetails(ctx, r.Client, connector.Namespace, connector.Spec.Cloudflare)
	if err != nil {
		logger.Error(err, "Failed to initialize Cloudflare API client")
		return r.updateStatusError(ctx, connector, err)
	}

	// Handle deletion
	if !connector.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, connector, apiClient)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(connector, FinalizerName) {
		controllerutil.AddFinalizer(connector, FinalizerName)
		if err := r.Update(ctx, connector); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile the WARP connector
	return r.reconcileWARPConnector(ctx, connector, apiClient)
}

func (r *WARPConnectorReconciler) handleDeletion(ctx context.Context, connector *networkingv1alpha2.WARPConnector, apiClient *cf.API) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(connector, FinalizerName) {
		// Delete deployment first
		deployment := &appsv1.Deployment{}
		if err := r.Get(ctx, types.NamespacedName{Name: connector.Name, Namespace: connector.Namespace}, deployment); err == nil {
			if err := r.Delete(ctx, deployment); err != nil && !errors.IsNotFound(err) {
				logger.Error(err, "Failed to delete Deployment")
			}
		}

		// Delete secret
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: connector.Name + "-token", Namespace: connector.Namespace}, secret); err == nil {
			if err := r.Delete(ctx, secret); err != nil && !errors.IsNotFound(err) {
				logger.Error(err, "Failed to delete Secret")
			}
		}

		// Delete routes from Cloudflare
		if connector.Status.TunnelID != "" {
			for _, route := range connector.Spec.Routes {
				logger.Info("Deleting route", "network", route.Network)
				if err := apiClient.DeleteTunnelRoute(route.Network, ""); err != nil {
					logger.Error(err, "Failed to delete route", "network", route.Network)
				}
			}
		}

		// Delete WARP connector from Cloudflare
		if connector.Status.ConnectorID != "" {
			logger.Info("Deleting WARP Connector from Cloudflare", "connectorId", connector.Status.ConnectorID)
			if err := apiClient.DeleteWARPConnector(connector.Status.ConnectorID); err != nil {
				logger.Error(err, "Failed to delete WARP Connector from Cloudflare")
			}
		}

		// Remove finalizer
		controllerutil.RemoveFinalizer(connector, FinalizerName)
		if err := r.Update(ctx, connector); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *WARPConnectorReconciler) reconcileWARPConnector(ctx context.Context, connector *networkingv1alpha2.WARPConnector, apiClient *cf.API) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var connectorID, tunnelID, tunnelToken string
	var err error

	if connector.Status.ConnectorID == "" {
		// Create new WARP connector
		logger.Info("Creating WARP Connector", "name", connector.GetConnectorName())
		result, err := apiClient.CreateWARPConnector(connector.GetConnectorName())
		if err != nil {
			return r.updateStatusError(ctx, connector, err)
		}
		connectorID = result.ID
		tunnelID = result.TunnelID
		tunnelToken = result.TunnelToken
	} else {
		connectorID = connector.Status.ConnectorID
		tunnelID = connector.Status.TunnelID
		// Get existing tunnel token
		result, err := apiClient.GetWARPConnectorToken(connectorID)
		if err != nil {
			return r.updateStatusError(ctx, connector, err)
		}
		tunnelToken = result.Token
	}

	// Create or update tunnel token secret
	if err := r.reconcileSecret(ctx, connector, tunnelToken); err != nil {
		return r.updateStatusError(ctx, connector, err)
	}

	// Configure routes
	routesConfigured := 0
	for _, route := range connector.Spec.Routes {
		logger.Info("Configuring route", "network", route.Network)
		vnetID := ""
		if connector.Spec.VirtualNetworkRef != nil {
			vnetID = connector.Spec.VirtualNetworkRef.Name
		}
		routeParams := cf.TunnelRouteParams{
			Network:          route.Network,
			TunnelID:         tunnelID,
			VirtualNetworkID: vnetID,
			Comment:          route.Comment,
		}
		if _, err := apiClient.CreateTunnelRoute(routeParams); err != nil {
			logger.Error(err, "Failed to create route", "network", route.Network)
		} else {
			routesConfigured++
		}
	}

	// Create or update deployment
	if err := r.reconcileDeployment(ctx, connector); err != nil {
		return r.updateStatusError(ctx, connector, err)
	}

	// Get deployment status
	deployment := &appsv1.Deployment{}
	if err = r.Get(ctx, types.NamespacedName{Name: connector.Name, Namespace: connector.Namespace}, deployment); err != nil {
		return r.updateStatusError(ctx, connector, err)
	}

	// Update status
	return r.updateStatusSuccess(ctx, connector, connectorID, tunnelID, deployment.Status.ReadyReplicas, routesConfigured)
}

func (r *WARPConnectorReconciler) reconcileSecret(ctx context.Context, connector *networkingv1alpha2.WARPConnector, token string) error {
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

func (r *WARPConnectorReconciler) reconcileDeployment(ctx context.Context, connector *networkingv1alpha2.WARPConnector) error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connector.Name,
			Namespace: connector.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
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
							Resources: r.buildResources(connector.Spec.Resources),
						},
					},
				},
			},
		}

		return controllerutil.SetControllerReference(connector, deployment, r.Scheme)
	})

	return err
}

func (r *WARPConnectorReconciler) buildTolerations(tolerations []networkingv1alpha2.Toleration) []corev1.Toleration {
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

func (r *WARPConnectorReconciler) updateStatusError(ctx context.Context, connector *networkingv1alpha2.WARPConnector, err error) (ctrl.Result, error) {
	connector.Status.State = "Error"
	meta.SetStatusCondition(&connector.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "ReconcileError",
		Message:            err.Error(),
		LastTransitionTime: metav1.Now(),
	})
	connector.Status.ObservedGeneration = connector.Generation

	if updateErr := r.Status().Update(ctx, connector); updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *WARPConnectorReconciler) updateStatusSuccess(ctx context.Context, connector *networkingv1alpha2.WARPConnector, connectorID, tunnelID string, readyReplicas int32, routesConfigured int) (ctrl.Result, error) {
	connector.Status.ConnectorID = connectorID
	connector.Status.TunnelID = tunnelID
	connector.Status.ReadyReplicas = readyReplicas
	connector.Status.RoutesConfigured = routesConfigured
	connector.Status.State = "Ready"
	meta.SetStatusCondition(&connector.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "WARP Connector successfully reconciled",
		LastTransitionTime: metav1.Now(),
	})
	connector.Status.ObservedGeneration = connector.Generation

	if err := r.Status().Update(ctx, connector); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *WARPConnectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.WARPConnector{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
