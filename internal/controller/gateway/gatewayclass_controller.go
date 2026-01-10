// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package gateway

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

// GatewayClassReconciler reconciles a GatewayClass object
type GatewayClassReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=tunnelgatewayclassconfigs,verbs=get;list;watch

// Reconcile handles GatewayClass reconciliation
// nolint:revive // Cognitive complexity for GatewayClass validation
func (r *GatewayClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get GatewayClass
	gatewayClass := &gatewayv1.GatewayClass{}
	if err := r.Get(ctx, req.NamespacedName, gatewayClass); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Skip if not our controller
	if !IsOurGatewayClass(gatewayClass) {
		return ctrl.Result{}, nil
	}

	logger.Info("Reconciling GatewayClass", "name", gatewayClass.Name)

	// Validate parametersRef
	if gatewayClass.Spec.ParametersRef == nil {
		return r.setAcceptedCondition(ctx, gatewayClass, false, ReasonInvalidParams,
			"GatewayClass must have parametersRef pointing to TunnelGatewayClassConfig")
	}

	// Validate parametersRef group and kind
	params := gatewayClass.Spec.ParametersRef
	if string(params.Group) != ParametersGroup {
		return r.setAcceptedCondition(ctx, gatewayClass, false, ReasonInvalidParams,
			"parametersRef.group must be networking.cloudflare-operator.io")
	}
	if string(params.Kind) != ParametersKind {
		return r.setAcceptedCondition(ctx, gatewayClass, false, ReasonUnsupportedKind,
			"parametersRef.kind must be TunnelGatewayClassConfig")
	}

	// Validate namespace is specified
	if params.Namespace == nil || *params.Namespace == "" {
		return r.setAcceptedCondition(ctx, gatewayClass, false, ReasonInvalidParams,
			"parametersRef.namespace must be specified for TunnelGatewayClassConfig")
	}

	// Get and validate TunnelGatewayClassConfig
	config, err := GetTunnelGatewayClassConfig(ctx, r.Client, gatewayClass)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.setAcceptedCondition(ctx, gatewayClass, false, ReasonParamsNotFound,
				"TunnelGatewayClassConfig not found: "+err.Error())
		}
		return r.setAcceptedCondition(ctx, gatewayClass, false, ReasonInvalidParams,
			"Failed to get TunnelGatewayClassConfig: "+err.Error())
	}

	logger.Info("GatewayClass validated", "name", gatewayClass.Name, "config", config.Name)

	// Set accepted condition
	return r.setAcceptedCondition(ctx, gatewayClass, true, ReasonAccepted,
		"GatewayClass is accepted by cloudflare-operator")
}

// setAcceptedCondition updates the GatewayClass Accepted condition
// nolint:revive // Control parameter is intentional for condition setting
func (r *GatewayClassReconciler) setAcceptedCondition(
	ctx context.Context,
	gatewayClass *gatewayv1.GatewayClass,
	accepted bool, //nolint:revive
	reason string,
	message string,
) (ctrl.Result, error) {
	status := metav1.ConditionFalse
	if accepted {
		status = metav1.ConditionTrue
	}

	condition := metav1.Condition{
		Type:               string(gatewayv1.GatewayClassConditionStatusAccepted),
		Status:             status,
		ObservedGeneration: gatewayClass.Generation,
		Reason:             reason,
		Message:            message,
	}

	// Update status with retry
	if err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, gatewayClass, func() {
		meta.SetStatusCondition(&gatewayClass.Status.Conditions, condition)
	}); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayClassReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.GatewayClass{}).
		Watches(
			&networkingv1alpha2.TunnelGatewayClassConfig{},
			handler.EnqueueRequestsFromMapFunc(r.findGatewayClassesForConfig),
		).
		Complete(r)
}

// findGatewayClassesForConfig finds GatewayClasses that reference a given TunnelGatewayClassConfig
// nolint:revive // Cognitive complexity for GatewayClass matching
func (r *GatewayClassReconciler) findGatewayClassesForConfig(ctx context.Context, obj client.Object) []reconcile.Request {
	config, ok := obj.(*networkingv1alpha2.TunnelGatewayClassConfig)
	if !ok {
		return nil
	}

	// List all GatewayClasses
	gatewayClassList := &gatewayv1.GatewayClassList{}
	if err := r.List(ctx, gatewayClassList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, gc := range gatewayClassList.Items {
		// Check if this GatewayClass references our config
		if gc.Spec.ParametersRef != nil &&
			string(gc.Spec.ParametersRef.Group) == ParametersGroup &&
			string(gc.Spec.ParametersRef.Kind) == ParametersKind &&
			gc.Spec.ParametersRef.Name == config.Name &&
			gc.Spec.ParametersRef.Namespace != nil &&
			string(*gc.Spec.ParametersRef.Namespace) == config.Namespace {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKey{Name: gc.Name},
			})
		}
	}

	return requests
}
