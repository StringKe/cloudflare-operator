// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package gateway

import (
	"context"
	"errors"
	"fmt"
	"sort"

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
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/controller/route"
	tunnelpkg "github.com/StringKe/cloudflare-operator/internal/controller/tunnel"
	"github.com/StringKe/cloudflare-operator/internal/credentials"
)

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	Recorder          record.EventRecorder
	OperatorNamespace string
}

// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/finalizers,verbs=update
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tcproutes,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=udproutes,verbs=get;list;watch

// Reconcile handles Gateway reconciliation
// nolint:revive // Cognitive complexity for Gateway reconciliation
func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get Gateway
	gateway := &gatewayv1.Gateway{}
	if err := r.Get(ctx, req.NamespacedName, gateway); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Check if Gateway's GatewayClass is managed by us
	isOurs, err := IsGatewayManagedByUs(ctx, r.Client, gateway)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// GatewayClass not found, skip
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if !isOurs {
		return ctrl.Result{}, nil
	}

	logger.Info("Reconciling Gateway", "name", gateway.Name, "namespace", gateway.Namespace)

	// Get GatewayClass and config
	gatewayClass, err := GetGatewayClassForGateway(ctx, r.Client, gateway)
	if err != nil {
		return r.setCondition(ctx, gateway, gatewayv1.GatewayConditionAccepted, false, "GatewayClassNotFound",
			"GatewayClass not found: "+err.Error())
	}

	config, err := GetTunnelGatewayClassConfig(ctx, r.Client, gatewayClass)
	if err != nil {
		return r.setCondition(ctx, gateway, gatewayv1.GatewayConditionAccepted, false, "ConfigNotFound",
			"TunnelGatewayClassConfig not found: "+err.Error())
	}

	// Handle deletion
	if gateway.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, gateway, config)
	}

	// Add finalizer if needed
	if !controllerutil.ContainsFinalizer(gateway, FinalizerName) {
		if err := controller.UpdateWithConflictRetry(ctx, r.Client, gateway, func() {
			controllerutil.AddFinalizer(gateway, FinalizerName)
		}); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Resolve tunnel
	resolver := tunnelpkg.NewResolver(r.Client, r.OperatorNamespace)
	tunnel, err := resolver.Resolve(ctx, config.Spec.TunnelRef, config.Namespace)
	if err != nil {
		r.Recorder.Event(gateway, corev1.EventTypeWarning, "TunnelNotFound", err.Error())
		return r.setCondition(ctx, gateway, gatewayv1.GatewayConditionAccepted, false, "TunnelNotFound",
			"Tunnel not found: "+err.Error())
	}

	// Build ingress rules from all attached routes
	rules, err := r.buildIngressRules(ctx, gateway, config)
	if err != nil {
		r.Recorder.Event(gateway, corev1.EventTypeWarning, "BuildRulesFailed", err.Error())
		return r.setCondition(ctx, gateway, gatewayv1.GatewayConditionProgrammed, false, "BuildRulesFailed",
			"Failed to build ingress rules: "+err.Error())
	}

	// Sync configuration to Cloudflare API
	// In token mode, cloudflared pulls configuration from cloud automatically
	if err := r.syncTunnelConfigToAPI(ctx, tunnel, config, rules); err != nil {
		r.Recorder.Event(gateway, corev1.EventTypeWarning, "APISyncFailed", cf.SanitizeErrorMessage(err))
		return r.setCondition(ctx, gateway, gatewayv1.GatewayConditionProgrammed, false, "APISyncFailed",
			"Failed to sync configuration to Cloudflare API: "+cf.SanitizeErrorMessage(err))
	}

	// Update status
	r.Recorder.Event(gateway, corev1.EventTypeNormal, "Reconciled", "Gateway configured successfully")
	return r.setConditions(ctx, gateway,
		metav1.Condition{
			Type:               string(gatewayv1.GatewayConditionAccepted),
			Status:             metav1.ConditionTrue,
			Reason:             "Accepted",
			Message:            "Gateway is accepted",
			ObservedGeneration: gateway.Generation,
		},
		metav1.Condition{
			Type:               string(gatewayv1.GatewayConditionProgrammed),
			Status:             metav1.ConditionTrue,
			Reason:             "Programmed",
			Message:            fmt.Sprintf("Gateway configured with %d ingress rules", len(rules)-1),
			ObservedGeneration: gateway.Generation,
		},
	)
}

// handleDeletion handles Gateway deletion
// nolint:revive // Cognitive complexity for deletion logic
func (r *GatewayReconciler) handleDeletion(
	ctx context.Context,
	gateway *gatewayv1.Gateway,
	config *networkingv1alpha2.TunnelGatewayClassConfig,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(gateway, FinalizerName) {
		return ctrl.Result{}, nil
	}

	logger.Info("Handling Gateway deletion", "name", gateway.Name)

	// Resolve tunnel to update config
	resolver := tunnelpkg.NewResolver(r.Client, r.OperatorNamespace)
	tunnel, err := resolver.Resolve(ctx, config.Spec.TunnelRef, config.Namespace)
	if err != nil {
		// Tunnel not found, just remove finalizer
		logger.Info("Tunnel not found during deletion, removing finalizer")
	} else {
		// Build rules without this gateway's routes and update
		// For now, just rebuild from remaining gateways
		rules, buildErr := r.buildIngressRulesExcluding(ctx, gateway.Name, gateway.Namespace, config)
		if buildErr != nil {
			logger.Error(buildErr, "Failed to build ingress rules during deletion")
		} else {
			// Sync to Cloudflare API
			if syncErr := r.syncTunnelConfigToAPI(ctx, tunnel, config, rules); syncErr != nil {
				logger.Error(syncErr, "Failed to sync configuration to API during deletion")
			}
		}
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, gateway, func() {
		controllerutil.RemoveFinalizer(gateway, FinalizerName)
	}); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// buildIngressRules builds cloudflared ingress rules from all routes attached to the gateway
// nolint:revive // Cognitive complexity for route aggregation
func (r *GatewayReconciler) buildIngressRules(
	ctx context.Context,
	gateway *gatewayv1.Gateway,
	config *networkingv1alpha2.TunnelGatewayClassConfig,
) ([]cf.UnvalidatedIngressRule, error) {
	var allRules []cf.UnvalidatedIngressRule
	var errs []error

	// Get HTTPRoutes
	httpRoutes, err := r.getHTTPRoutesForGateway(ctx, gateway)
	if err != nil {
		errs = append(errs, fmt.Errorf("list HTTPRoutes: %w", err))
	} else {
		for _, hr := range httpRoutes {
			rules := r.convertHTTPRouteToRules(ctx, &hr, gateway, config)
			allRules = append(allRules, rules...)
		}
	}

	// Get TCPRoutes
	tcpRoutes, err := r.getTCPRoutesForGateway(ctx, gateway)
	if err != nil {
		errs = append(errs, fmt.Errorf("list TCPRoutes: %w", err))
	} else {
		for _, tr := range tcpRoutes {
			rules := r.convertTCPRouteToRules(ctx, &tr, gateway)
			allRules = append(allRules, rules...)
		}
	}

	// Get UDPRoutes
	udpRoutes, err := r.getUDPRoutesForGateway(ctx, gateway)
	if err != nil {
		errs = append(errs, fmt.Errorf("list UDPRoutes: %w", err))
	} else {
		for _, ur := range udpRoutes {
			rules := r.convertUDPRouteToRules(ctx, &ur, gateway)
			allRules = append(allRules, rules...)
		}
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	// Sort rules for deterministic config
	sort.Slice(allRules, func(i, j int) bool {
		if allRules[i].Hostname != allRules[j].Hostname {
			return allRules[i].Hostname < allRules[j].Hostname
		}
		return allRules[i].Path < allRules[j].Path
	})

	// Add fallback rule
	fallbackTarget := config.Spec.FallbackTarget
	if fallbackTarget == "" {
		fallbackTarget = "http_status:404"
	}
	allRules = append(allRules, cf.UnvalidatedIngressRule{
		Service: fallbackTarget,
	})

	return allRules, nil
}

// buildIngressRulesExcluding builds rules excluding a specific gateway (for deletion)
// nolint:revive // Cognitive complexity for rule aggregation logic
func (r *GatewayReconciler) buildIngressRulesExcluding(
	ctx context.Context,
	excludeName string,
	excludeNamespace string,
	config *networkingv1alpha2.TunnelGatewayClassConfig,
) ([]cf.UnvalidatedIngressRule, error) {
	// List all gateways using this config
	gatewayList := &gatewayv1.GatewayList{}
	if err := r.List(ctx, gatewayList); err != nil {
		return nil, err
	}

	var allRules []cf.UnvalidatedIngressRule
	for _, gw := range gatewayList.Items {
		// Skip the excluded gateway
		if gw.Name == excludeName && gw.Namespace == excludeNamespace {
			continue
		}

		// Check if this gateway uses our controller
		isOurs, err := IsGatewayManagedByUs(ctx, r.Client, &gw)
		if err != nil || !isOurs {
			continue
		}

		rules, err := r.buildIngressRules(ctx, &gw, config)
		if err != nil {
			continue
		}
		// Remove the fallback rule (we'll add one at the end)
		if len(rules) > 0 {
			allRules = append(allRules, rules[:len(rules)-1]...)
		}
	}

	// Add fallback rule
	fallbackTarget := config.Spec.FallbackTarget
	if fallbackTarget == "" {
		fallbackTarget = "http_status:404"
	}
	allRules = append(allRules, cf.UnvalidatedIngressRule{
		Service: fallbackTarget,
	})

	return allRules, nil
}

// getHTTPRoutesForGateway returns HTTPRoutes attached to the gateway
func (r *GatewayReconciler) getHTTPRoutesForGateway(ctx context.Context, gateway *gatewayv1.Gateway) ([]gatewayv1.HTTPRoute, error) {
	httpRouteList := &gatewayv1.HTTPRouteList{}
	if err := r.List(ctx, httpRouteList); err != nil {
		return nil, err
	}

	var result []gatewayv1.HTTPRoute
	for _, hr := range httpRouteList.Items {
		if r.routeReferencesGateway(hr.Spec.ParentRefs, gateway) {
			result = append(result, hr)
		}
	}
	return result, nil
}

// getTCPRoutesForGateway returns TCPRoutes attached to the gateway
func (r *GatewayReconciler) getTCPRoutesForGateway(ctx context.Context, gateway *gatewayv1.Gateway) ([]gatewayv1alpha2.TCPRoute, error) {
	tcpRouteList := &gatewayv1alpha2.TCPRouteList{}
	if err := r.List(ctx, tcpRouteList); err != nil {
		return nil, err
	}

	var result []gatewayv1alpha2.TCPRoute
	for _, tr := range tcpRouteList.Items {
		if r.routeReferencesGateway(tr.Spec.ParentRefs, gateway) {
			result = append(result, tr)
		}
	}
	return result, nil
}

// getUDPRoutesForGateway returns UDPRoutes attached to the gateway
func (r *GatewayReconciler) getUDPRoutesForGateway(ctx context.Context, gateway *gatewayv1.Gateway) ([]gatewayv1alpha2.UDPRoute, error) {
	udpRouteList := &gatewayv1alpha2.UDPRouteList{}
	if err := r.List(ctx, udpRouteList); err != nil {
		return nil, err
	}

	var result []gatewayv1alpha2.UDPRoute
	for _, ur := range udpRouteList.Items {
		if r.routeReferencesGateway(ur.Spec.ParentRefs, gateway) {
			result = append(result, ur)
		}
	}
	return result, nil
}

// routeReferencesGateway checks if any parent ref points to the given gateway
// nolint:revive // Cognitive complexity for parent ref checking
func (r *GatewayReconciler) routeReferencesGateway(parentRefs []gatewayv1.ParentReference, gateway *gatewayv1.Gateway) bool {
	for _, ref := range parentRefs {
		// Check group - defaults to gateway.networking.k8s.io
		if ref.Group != nil && *ref.Group != gatewayv1.GroupName {
			continue
		}
		// Check kind - defaults to Gateway
		if ref.Kind != nil && *ref.Kind != KindGateway {
			continue
		}
		// Check name
		if string(ref.Name) != gateway.Name {
			continue
		}
		// Check namespace - defaults to route's namespace which should match gateway's namespace
		if ref.Namespace != nil && string(*ref.Namespace) != gateway.Namespace {
			continue
		}
		return true
	}
	return false
}

// convertHTTPRouteToRules converts an HTTPRoute to cloudflared ingress rules
// nolint:revive // Cognitive complexity for HTTPRoute conversion
func (r *GatewayReconciler) convertHTTPRouteToRules(
	ctx context.Context,
	httpRoute *gatewayv1.HTTPRoute,
	_ *gatewayv1.Gateway,
	config *networkingv1alpha2.TunnelGatewayClassConfig,
) []cf.UnvalidatedIngressRule {
	var rules []cf.UnvalidatedIngressRule
	logger := log.FromContext(ctx)

	// Get hostnames - use route hostnames or empty for all
	hostnames := httpRoute.Spec.Hostnames
	if len(hostnames) == 0 {
		hostnames = []gatewayv1.Hostname{""}
	}

	for _, rule := range httpRoute.Spec.Rules {
		for _, hostname := range hostnames {
			for _, match := range rule.Matches {
				for _, backendRef := range rule.BackendRefs {
					// Resolve backend
					target := r.resolveHTTPBackendRef(ctx, httpRoute.Namespace, backendRef)
					if target == "" {
						logger.Info("Skipping backend with unresolved target",
							"route", httpRoute.Name, "backend", backendRef.Name)
						continue
					}

					// Build path
					path := ""
					if match.Path != nil && match.Path.Value != nil {
						path = route.ConvertGatewayPathType(*match.Path.Value, match.Path.Type)
					}

					// Build origin request
					originReq := route.NewOriginRequestBuilder().
						WithDefaults(config.Spec.DefaultOriginRequest).
						Build()

					rules = append(rules, cf.UnvalidatedIngressRule{
						Hostname:      string(hostname),
						Path:          path,
						Service:       target,
						OriginRequest: originReq,
					})
				}
			}

			// Handle rules without matches (match all)
			if len(rule.Matches) == 0 {
				for _, backendRef := range rule.BackendRefs {
					target := r.resolveHTTPBackendRef(ctx, httpRoute.Namespace, backendRef)
					if target == "" {
						continue
					}

					originReq := route.NewOriginRequestBuilder().
						WithDefaults(config.Spec.DefaultOriginRequest).
						Build()

					rules = append(rules, cf.UnvalidatedIngressRule{
						Hostname:      string(hostname),
						Service:       target,
						OriginRequest: originReq,
					})
				}
			}
		}
	}

	return rules
}

// resolveHTTPBackendRef resolves an HTTP backend reference to a service URL
// nolint:revive // Cognitive complexity for backend resolution
func (r *GatewayReconciler) resolveHTTPBackendRef(
	ctx context.Context,
	routeNamespace string,
	backendRef gatewayv1.HTTPBackendRef,
) string {
	// Only support Service backends
	if backendRef.Kind != nil && *backendRef.Kind != KindService {
		return ""
	}

	namespace := routeNamespace
	if backendRef.Namespace != nil {
		namespace = string(*backendRef.Namespace)
	}

	port := "80"
	if backendRef.Port != nil {
		port = fmt.Sprintf("%d", *backendRef.Port)
	}

	// Try to get service to determine protocol
	svc := &corev1.Service{}
	protocol := route.ProtocolHTTP
	if err := r.Get(ctx, apitypes.NamespacedName{
		Name:      string(backendRef.Name),
		Namespace: namespace,
	}, svc); err == nil {
		// Check if service port suggests HTTPS
		for _, p := range svc.Spec.Ports {
			if fmt.Sprintf("%d", p.Port) == port {
				protocol = route.InferProtocolFromPort(port)
				break
			}
		}
	}

	return route.BuildServiceURL(protocol, string(backendRef.Name), namespace, port)
}

// convertTCPRouteToRules converts a TCPRoute to cloudflared ingress rules
func (r *GatewayReconciler) convertTCPRouteToRules(
	ctx context.Context,
	tcpRoute *gatewayv1alpha2.TCPRoute,
	_ *gatewayv1.Gateway,
) []cf.UnvalidatedIngressRule {
	var rules []cf.UnvalidatedIngressRule

	for _, rule := range tcpRoute.Spec.Rules {
		for _, backendRef := range rule.BackendRefs {
			target := r.resolveTCPBackendRef(ctx, tcpRoute.Namespace, backendRef)
			if target == "" {
				continue
			}

			rules = append(rules, cf.UnvalidatedIngressRule{
				Service: target,
			})
		}
	}

	return rules
}

// resolveTCPBackendRef resolves a TCP backend reference to a service URL
func (*GatewayReconciler) resolveTCPBackendRef(
	_ context.Context,
	routeNamespace string,
	backendRef gatewayv1.BackendRef,
) string {
	if backendRef.Kind != nil && *backendRef.Kind != KindService {
		return ""
	}

	namespace := routeNamespace
	if backendRef.Namespace != nil {
		namespace = string(*backendRef.Namespace)
	}

	port := "80"
	if backendRef.Port != nil {
		port = fmt.Sprintf("%d", *backendRef.Port)
	}

	return route.BuildServiceURL(route.ProtocolTCP, string(backendRef.Name), namespace, port)
}

// convertUDPRouteToRules converts a UDPRoute to cloudflared ingress rules
func (r *GatewayReconciler) convertUDPRouteToRules(
	ctx context.Context,
	udpRoute *gatewayv1alpha2.UDPRoute,
	_ *gatewayv1.Gateway,
) []cf.UnvalidatedIngressRule {
	var rules []cf.UnvalidatedIngressRule

	for _, rule := range udpRoute.Spec.Rules {
		for _, backendRef := range rule.BackendRefs {
			target := r.resolveUDPBackendRef(ctx, udpRoute.Namespace, backendRef)
			if target == "" {
				continue
			}

			rules = append(rules, cf.UnvalidatedIngressRule{
				Service: target,
			})
		}
	}

	return rules
}

// resolveUDPBackendRef resolves a UDP backend reference to a service URL
func (*GatewayReconciler) resolveUDPBackendRef(
	_ context.Context,
	routeNamespace string,
	backendRef gatewayv1.BackendRef,
) string {
	if backendRef.Kind != nil && *backendRef.Kind != KindService {
		return ""
	}

	namespace := routeNamespace
	if backendRef.Namespace != nil {
		namespace = string(*backendRef.Namespace)
	}

	port := "80"
	if backendRef.Port != nil {
		port = fmt.Sprintf("%d", *backendRef.Port)
	}

	return route.BuildServiceURL(route.ProtocolUDP, string(backendRef.Name), namespace, port)
}

// setCondition updates a single condition on the Gateway status
// nolint:unparam,revive // status parameter kept for API consistency
func (r *GatewayReconciler) setCondition(
	ctx context.Context,
	gateway *gatewayv1.Gateway,
	condType gatewayv1.GatewayConditionType,
	status bool, //nolint:unparam,revive
	reason string,
	message string,
) (ctrl.Result, error) {
	condStatus := metav1.ConditionFalse
	if status {
		condStatus = metav1.ConditionTrue
	}

	return r.setConditions(ctx, gateway, metav1.Condition{
		Type:               string(condType),
		Status:             condStatus,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: gateway.Generation,
	})
}

// setConditions updates multiple conditions on the Gateway status
func (r *GatewayReconciler) setConditions(
	ctx context.Context,
	gateway *gatewayv1.Gateway,
	conditions ...metav1.Condition,
) (ctrl.Result, error) {
	if err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, gateway, func() {
		for _, cond := range conditions {
			meta.SetStatusCondition(&gateway.Status.Conditions, cond)
		}
	}); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.Gateway{}).
		Watches(
			&gatewayv1.HTTPRoute{},
			handler.EnqueueRequestsFromMapFunc(r.findGatewaysForHTTPRoute),
		).
		Watches(
			&gatewayv1alpha2.TCPRoute{},
			handler.EnqueueRequestsFromMapFunc(r.findGatewaysForTCPRoute),
		).
		Watches(
			&gatewayv1alpha2.UDPRoute{},
			handler.EnqueueRequestsFromMapFunc(r.findGatewaysForUDPRoute),
		).
		Complete(r)
}

// findGatewaysForHTTPRoute finds Gateways that an HTTPRoute is attached to
func (r *GatewayReconciler) findGatewaysForHTTPRoute(ctx context.Context, obj client.Object) []reconcile.Request {
	httpRoute, ok := obj.(*gatewayv1.HTTPRoute)
	if !ok {
		return nil
	}
	return r.findGatewaysFromParentRefs(ctx, httpRoute.Spec.ParentRefs, httpRoute.Namespace)
}

// findGatewaysForTCPRoute finds Gateways that a TCPRoute is attached to
func (r *GatewayReconciler) findGatewaysForTCPRoute(ctx context.Context, obj client.Object) []reconcile.Request {
	tcpRoute, ok := obj.(*gatewayv1alpha2.TCPRoute)
	if !ok {
		return nil
	}
	return r.findGatewaysFromParentRefs(ctx, tcpRoute.Spec.ParentRefs, tcpRoute.Namespace)
}

// findGatewaysForUDPRoute finds Gateways that a UDPRoute is attached to
func (r *GatewayReconciler) findGatewaysForUDPRoute(ctx context.Context, obj client.Object) []reconcile.Request {
	udpRoute, ok := obj.(*gatewayv1alpha2.UDPRoute)
	if !ok {
		return nil
	}
	return r.findGatewaysFromParentRefs(ctx, udpRoute.Spec.ParentRefs, udpRoute.Namespace)
}

// findGatewaysFromParentRefs extracts Gateway references from ParentRefs
// nolint:revive // Cognitive complexity for parent ref extraction
func (r *GatewayReconciler) findGatewaysFromParentRefs(ctx context.Context, refs []gatewayv1.ParentReference, defaultNamespace string) []reconcile.Request {
	requests := make([]reconcile.Request, 0, len(refs))

	for _, ref := range refs {
		// Only handle Gateway kind
		if ref.Group != nil && *ref.Group != gatewayv1.GroupName {
			continue
		}
		if ref.Kind != nil && *ref.Kind != KindGateway {
			continue
		}

		namespace := defaultNamespace
		if ref.Namespace != nil {
			namespace = string(*ref.Namespace)
		}

		// Check if this gateway is managed by us
		gateway := &gatewayv1.Gateway{}
		if err := r.Get(ctx, apitypes.NamespacedName{
			Name:      string(ref.Name),
			Namespace: namespace,
		}, gateway); err != nil {
			continue
		}

		isOurs, err := IsGatewayManagedByUs(ctx, r.Client, gateway)
		if err != nil || !isOurs {
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: apitypes.NamespacedName{
				Name:      string(ref.Name),
				Namespace: namespace,
			},
		})
	}

	return requests
}

// syncTunnelConfigToAPI syncs the tunnel configuration to Cloudflare API.
// In token mode, cloudflared pulls configuration from cloud automatically.
func (r *GatewayReconciler) syncTunnelConfigToAPI(
	ctx context.Context,
	tunnel tunnelpkg.Interface,
	_ *networkingv1alpha2.TunnelGatewayClassConfig,
	rules []cf.UnvalidatedIngressRule,
) error {
	logger := log.FromContext(ctx)

	tunnelID := tunnel.GetStatus().TunnelId
	if tunnelID == "" {
		logger.Info("Tunnel ID not available, skipping API sync")
		return nil
	}

	// Determine namespace for credentials
	namespace := tunnel.GetNamespace()
	if namespace == "" {
		namespace = controller.OperatorNamespace
	}

	// Create credentials loader and load credentials
	loader := credentials.NewLoader(r.Client, logger)
	tunnelSpec := tunnel.GetSpec()
	creds, err := loader.LoadFromCloudflareDetails(ctx, &tunnelSpec.Cloudflare, namespace)
	if err != nil {
		return fmt.Errorf("failed to load credentials: %w", err)
	}

	// Create Cloudflare client
	cloudflareClient, err := controller.CreateCloudflareClientFromCreds(creds)
	if err != nil {
		return fmt.Errorf("failed to create cloudflare client: %w", err)
	}

	// Build API client
	apiClient := &cf.API{
		Log:              logger,
		AccountId:        creds.AccountID,
		AccountName:      tunnelSpec.Cloudflare.AccountName,
		Domain:           creds.Domain,
		ValidAccountId:   tunnel.GetStatus().AccountId,
		ValidTunnelId:    tunnelID,
		ValidTunnelName:  tunnel.GetStatus().TunnelName,
		ValidZoneId:      tunnel.GetStatus().ZoneId,
		CloudflareClient: cloudflareClient,
	}

	// Get WarpRouting config from tunnel spec
	var warpRouting *cf.WarpRoutingConfig
	if tunnelSpec.EnableWarpRouting {
		warpRouting = &cf.WarpRoutingConfig{
			Enabled: true,
		}
	}

	// Sync to Cloudflare API
	if err := apiClient.SyncTunnelConfigurationToAPI(tunnelID, rules, warpRouting); err != nil {
		return fmt.Errorf("failed to sync tunnel configuration: %w", err)
	}

	logger.Info("Tunnel configuration synced to Cloudflare API",
		"tunnel", tunnel.GetName(),
		"tunnelId", tunnelID,
		"ingressRules", len(rules))

	return nil
}
