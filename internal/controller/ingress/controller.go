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

package ingress

import (
	"context"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha1 "github.com/StringKe/cloudflare-operator/api/v1alpha1"
	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

const (
	// ControllerName is the name registered with IngressClass
	ControllerName = "cloudflare-operator.io/ingress-controller"

	// FinalizerName is the finalizer added to managed Ingresses
	FinalizerName = "ingress.cloudflare-operator.io/finalizer"

	// ManagedByAnnotation marks resources managed by this controller
	ManagedByAnnotation = "cloudflare.com/managed-by"

	// ManagedByValue is the value for ManagedByAnnotation
	ManagedByValue = "cloudflare-operator-ingress"

	// IngressClassAnnotation is the legacy annotation for ingress class
	IngressClassAnnotation = "kubernetes.io/ingress.class"
)

// Reconciler reconciles standard Kubernetes Ingress resources
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// OperatorNamespace is the namespace where the operator runs (for cluster-scoped resources)
	OperatorNamespace string
}

// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/finalizers,verbs=update
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingressclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=tunnelingressclassconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=tunnelingressclassconfigs/status,verbs=get;update;patch

// Reconcile handles Ingress reconciliation
// nolint:revive // Cognitive complexity is acceptable for a controller's main reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. Fetch Ingress
	ingress := &networkingv1.Ingress{}
	if err := r.Get(ctx, req.NamespacedName, ingress); err != nil {
		if apierrors.IsNotFound(err) {
			// Ingress deleted - trigger config rebuild
			logger.Info("Ingress deleted, triggering config rebuild")
			return r.handleIngressDeletion(ctx, req)
		}
		return ctrl.Result{}, err
	}

	// 2. Check if this Ingress is for our controller
	if !r.isOurIngress(ctx, ingress) {
		return ctrl.Result{}, nil
	}

	logger.Info("Reconciling Ingress", "ingress", req.NamespacedName)

	// 3. Get IngressClass configuration
	config, err := r.getIngressClassConfig(ctx, ingress)
	if err != nil {
		logger.Error(err, "Failed to get IngressClass configuration")
		r.Recorder.Event(ingress, corev1.EventTypeWarning, "ConfigError", err.Error())
		if statusErr := r.updateIngressStatus(ctx, ingress, nil, err); statusErr != nil {
			logger.Error(statusErr, "Failed to update Ingress status after config error")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// 4. Handle deletion
	if !ingress.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, ingress, config)
	}

	// 5. Add finalizer
	if !controllerutil.ContainsFinalizer(ingress, FinalizerName) {
		if err := controller.UpdateWithConflictRetry(ctx, r.Client, ingress, func() {
			controllerutil.AddFinalizer(ingress, FinalizerName)
		}); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 6. Reconcile: aggregate all Ingresses for this tunnel and update ConfigMap
	if err := r.reconcileIngressConfig(ctx, ingress, config); err != nil {
		logger.Error(err, "Failed to reconcile Ingress config")
		r.Recorder.Event(ingress, corev1.EventTypeWarning, "ReconcileError", cf.SanitizeErrorMessage(err))
		if statusErr := r.updateIngressStatus(ctx, ingress, config, err); statusErr != nil {
			logger.Error(statusErr, "Failed to update Ingress status after reconcile error")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// 7. Handle DNS
	if config.Spec.DNSManagement != networkingv1alpha2.DNSManagementManual {
		if err := r.reconcileDNS(ctx, ingress, config); err != nil {
			logger.Error(err, "Failed to reconcile DNS")
			r.Recorder.Event(ingress, corev1.EventTypeWarning, "DNSError", cf.SanitizeErrorMessage(err))
			if statusErr := r.updateIngressStatus(ctx, ingress, config, err); statusErr != nil {
				logger.Error(statusErr, "Failed to update Ingress status after DNS error")
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}
	}

	// 8. Update Ingress status
	if err := r.updateIngressStatus(ctx, ingress, config, nil); err != nil {
		logger.Error(err, "Failed to update Ingress status")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(ingress, corev1.EventTypeNormal, "Reconciled", "Ingress successfully reconciled")
	logger.Info("Ingress reconciled successfully")

	return ctrl.Result{}, nil
}

// isOurIngress checks if this Ingress should be handled by our controller
func (r *Reconciler) isOurIngress(ctx context.Context, ingress *networkingv1.Ingress) bool {
	// Check spec.ingressClassName first
	if ingress.Spec.IngressClassName != nil && *ingress.Spec.IngressClassName != "" {
		return r.isOurIngressClass(ctx, *ingress.Spec.IngressClassName)
	}

	// Check legacy annotation
	if className, ok := ingress.Annotations[IngressClassAnnotation]; ok && className != "" {
		return r.isOurIngressClass(ctx, className)
	}

	// Check if we're the default IngressClass
	return r.isDefaultIngressClass(ctx)
}

// isOurIngressClass checks if the given IngressClass is controlled by us
func (r *Reconciler) isOurIngressClass(ctx context.Context, className string) bool {
	ingressClass := &networkingv1.IngressClass{}
	if err := r.Get(ctx, apitypes.NamespacedName{Name: className}, ingressClass); err != nil {
		return false
	}
	return ingressClass.Spec.Controller == ControllerName
}

// isDefaultIngressClass checks if we have a default IngressClass
// nolint:revive // Nested conditionals are acceptable for checking annotations
func (r *Reconciler) isDefaultIngressClass(ctx context.Context) bool {
	ingressClasses := &networkingv1.IngressClassList{}
	if err := r.List(ctx, ingressClasses); err != nil {
		return false
	}

	for _, ic := range ingressClasses.Items {
		if ic.Spec.Controller == ControllerName {
			if ic.Annotations != nil {
				if ic.Annotations["ingressclass.kubernetes.io/is-default-class"] == "true" {
					return true
				}
			}
		}
	}
	return false
}

// getIngressClassConfig retrieves the TunnelIngressClassConfig for this Ingress
// nolint:revive // Cognitive complexity is acceptable for IngressClass parameter resolution
func (r *Reconciler) getIngressClassConfig(ctx context.Context, ingress *networkingv1.Ingress) (*networkingv1alpha2.TunnelIngressClassConfig, error) {
	// Determine IngressClass name
	className := ""
	if ingress.Spec.IngressClassName != nil && *ingress.Spec.IngressClassName != "" {
		className = *ingress.Spec.IngressClassName
	} else if cn, ok := ingress.Annotations[IngressClassAnnotation]; ok && cn != "" {
		className = cn
	} else {
		// Find default IngressClass
		var err error
		className, err = r.getDefaultIngressClassName(ctx)
		if err != nil {
			return nil, err
		}
	}

	// Get IngressClass
	ingressClass := &networkingv1.IngressClass{}
	if err := r.Get(ctx, apitypes.NamespacedName{Name: className}, ingressClass); err != nil {
		return nil, fmt.Errorf("IngressClass %q not found: %w", className, err)
	}

	// Validate controller
	if ingressClass.Spec.Controller != ControllerName {
		return nil, fmt.Errorf("IngressClass %q is not managed by %s", className, ControllerName)
	}

	// Get parameters
	if ingressClass.Spec.Parameters == nil {
		return nil, fmt.Errorf("IngressClass %q has no parameters configured", className)
	}

	params := ingressClass.Spec.Parameters
	apiGroup := ""
	if params.APIGroup != nil {
		apiGroup = *params.APIGroup
	}

	if apiGroup != "networking.cloudflare-operator.io" {
		return nil, fmt.Errorf("IngressClass %q has invalid parameters apiGroup: %s", className, apiGroup)
	}

	if params.Kind != "TunnelIngressClassConfig" {
		return nil, fmt.Errorf("IngressClass %q has invalid parameters kind: %s", className, params.Kind)
	}

	// Determine namespace
	namespace := r.OperatorNamespace
	if params.Namespace != nil && *params.Namespace != "" {
		namespace = *params.Namespace
	}

	// Get TunnelIngressClassConfig
	config := &networkingv1alpha2.TunnelIngressClassConfig{}
	if err := r.Get(ctx, apitypes.NamespacedName{
		Name:      params.Name,
		Namespace: namespace,
	}, config); err != nil {
		return nil, fmt.Errorf("tunnelingressclassconfig %s/%s not found: %w", namespace, params.Name, err)
	}

	return config, nil
}

// getDefaultIngressClassName finds the default IngressClass for our controller
// nolint:revive // Cognitive complexity for iterating IngressClasses
func (r *Reconciler) getDefaultIngressClassName(ctx context.Context) (string, error) {
	ingressClasses := &networkingv1.IngressClassList{}
	if err := r.List(ctx, ingressClasses); err != nil {
		return "", fmt.Errorf("failed to list IngressClasses: %w", err)
	}

	for _, ic := range ingressClasses.Items {
		if ic.Spec.Controller == ControllerName {
			if ic.Annotations != nil {
				if ic.Annotations["ingressclass.kubernetes.io/is-default-class"] == "true" {
					return ic.Name, nil
				}
			}
		}
	}

	return "", fmt.Errorf("no default IngressClass found for controller %s", ControllerName)
}

// handleDeletion handles Ingress deletion
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	ingress *networkingv1.Ingress,
	config *networkingv1alpha2.TunnelIngressClassConfig,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(ingress, FinalizerName) {
		return ctrl.Result{}, nil
	}

	logger.Info("Handling Ingress deletion")

	// Clean up DNS records
	if config.Spec.DNSManagement != networkingv1alpha2.DNSManagementManual {
		if err := r.cleanupDNS(ctx, ingress, config); err != nil {
			logger.Error(err, "Failed to cleanup DNS")
			// Continue with deletion even if DNS cleanup fails
		}
	}

	// Trigger config rebuild (without this Ingress)
	if err := r.rebuildTunnelConfig(ctx, config, ingress); err != nil {
		logger.Error(err, "Failed to rebuild tunnel config")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, ingress, func() {
		controllerutil.RemoveFinalizer(ingress, FinalizerName)
	}); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Ingress deletion completed")
	return ctrl.Result{}, nil
}

// handleIngressDeletion handles the case where an Ingress was deleted before we could process it
func (*Reconciler) handleIngressDeletion(_ context.Context, _ ctrl.Request) (ctrl.Result, error) {
	// We can't determine which config this Ingress belonged to, so we need to rebuild all configs
	// In practice, this is rare and the next reconcile will fix any inconsistencies
	return ctrl.Result{}, nil
}

// reconcileIngressConfig aggregates all Ingresses for a tunnel and updates ConfigMap
func (r *Reconciler) reconcileIngressConfig(
	ctx context.Context,
	_ *networkingv1.Ingress,
	config *networkingv1alpha2.TunnelIngressClassConfig,
) error {
	return r.rebuildTunnelConfig(ctx, config, nil)
}

// rebuildTunnelConfig rebuilds the tunnel ConfigMap with all Ingress rules
// nolint:revive // Cognitive complexity for ConfigMap rebuild logic
func (r *Reconciler) rebuildTunnelConfig(
	ctx context.Context,
	config *networkingv1alpha2.TunnelIngressClassConfig,
	excludeIngress *networkingv1.Ingress,
) error {
	logger := log.FromContext(ctx)

	// Get all Ingresses for this IngressClass
	allIngresses, err := r.getIngressesForConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to get Ingresses: %w", err)
	}

	// Exclude the deleting Ingress if specified
	if excludeIngress != nil {
		filtered := make([]*networkingv1.Ingress, 0, len(allIngresses))
		for _, ing := range allIngresses {
			if ing.Namespace != excludeIngress.Namespace || ing.Name != excludeIngress.Name {
				filtered = append(filtered, ing)
			}
		}
		allIngresses = filtered
	}

	// Get TunnelBindings for backward compatibility
	tunnelBindings, err := r.getTunnelBindingsForConfig(ctx, config)
	if err != nil {
		logger.Error(err, "Failed to list TunnelBindings, continuing without them")
		// Don't fail - TunnelBindings might not exist
	}

	// Log deprecation warning if TunnelBindings exist
	if len(tunnelBindings) > 0 {
		logger.Info("WARNING: TunnelBinding is deprecated, please migrate to Ingress or Gateway API",
			"tunnelBindingCount", len(tunnelBindings))
	}

	// Build ingress rules
	rules := r.buildIngressRules(ctx, allIngresses, tunnelBindings, config)

	// Get tunnel and update ConfigMap
	tunnel, err := r.getTunnel(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to get Tunnel: %w", err)
	}

	// Update ConfigMap
	if err := r.updateTunnelConfigMap(ctx, tunnel, rules, config); err != nil {
		return fmt.Errorf("failed to update ConfigMap: %w", err)
	}

	// Update TunnelIngressClassConfig status
	if err := r.updateConfigStatus(ctx, config, len(allIngresses)); err != nil {
		logger.Error(err, "Failed to update TunnelIngressClassConfig status")
	}

	return nil
}

// getIngressesForConfig returns all Ingresses that use this config's IngressClass
// nolint:revive // Cognitive complexity for gathering Ingresses across namespaces
func (r *Reconciler) getIngressesForConfig(
	ctx context.Context,
	config *networkingv1alpha2.TunnelIngressClassConfig,
) ([]*networkingv1.Ingress, error) {
	// Find IngressClasses that reference this config
	ingressClasses := &networkingv1.IngressClassList{}
	if err := r.List(ctx, ingressClasses); err != nil {
		return nil, err
	}

	var matchingClassNames []string
	for _, ic := range ingressClasses.Items {
		if ic.Spec.Controller != ControllerName {
			continue
		}
		if ic.Spec.Parameters == nil {
			continue
		}

		// Check if this IngressClass references our config
		namespace := r.OperatorNamespace
		if ic.Spec.Parameters.Namespace != nil && *ic.Spec.Parameters.Namespace != "" {
			namespace = *ic.Spec.Parameters.Namespace
		}

		if ic.Spec.Parameters.Name == config.Name && namespace == config.Namespace {
			matchingClassNames = append(matchingClassNames, ic.Name)
		}
	}

	if len(matchingClassNames) == 0 {
		return nil, nil
	}

	// Get all Ingresses
	allIngresses := &networkingv1.IngressList{}
	if len(config.Spec.WatchNamespaces) > 0 {
		// Watch specific namespaces
		var result []*networkingv1.Ingress
		for _, ns := range config.Spec.WatchNamespaces {
			ingressList := &networkingv1.IngressList{}
			if err := r.List(ctx, ingressList, client.InNamespace(ns)); err != nil {
				return nil, err
			}
			for i := range ingressList.Items {
				result = append(result, &ingressList.Items[i])
			}
		}
		// Filter by IngressClass
		return r.filterIngressesByClass(result, matchingClassNames), nil
	}

	// Watch all namespaces
	if err := r.List(ctx, allIngresses); err != nil {
		return nil, err
	}

	result := make([]*networkingv1.Ingress, 0, len(allIngresses.Items))
	for i := range allIngresses.Items {
		result = append(result, &allIngresses.Items[i])
	}

	return r.filterIngressesByClass(result, matchingClassNames), nil
}

// filterIngressesByClass filters Ingresses by IngressClass names
// nolint:revive // Cognitive complexity for filtering logic
func (r *Reconciler) filterIngressesByClass(ingresses []*networkingv1.Ingress, classNames []string) []*networkingv1.Ingress {
	classNameSet := make(map[string]bool)
	for _, name := range classNames {
		classNameSet[name] = true
	}

	var result []*networkingv1.Ingress
	for _, ing := range ingresses {
		// Check spec.ingressClassName
		if ing.Spec.IngressClassName != nil && *ing.Spec.IngressClassName != "" {
			if classNameSet[*ing.Spec.IngressClassName] {
				result = append(result, ing)
				continue
			}
		}

		// Check legacy annotation
		if className, ok := ing.Annotations[IngressClassAnnotation]; ok {
			if classNameSet[className] {
				result = append(result, ing)
			}
		}
	}

	return result
}

// getTunnelBindingsForConfig returns TunnelBindings for the same tunnel (backward compatibility)
// nolint:staticcheck // TunnelBinding is deprecated but we still support it for backward compatibility
func (r *Reconciler) getTunnelBindingsForConfig(
	ctx context.Context,
	config *networkingv1alpha2.TunnelIngressClassConfig,
) ([]networkingv1alpha1.TunnelBinding, error) { //nolint:staticcheck
	// List all TunnelBindings
	bindingList := &networkingv1alpha1.TunnelBindingList{} //nolint:staticcheck
	if err := r.List(ctx, bindingList); err != nil {
		return nil, err
	}

	var result []networkingv1alpha1.TunnelBinding //nolint:staticcheck
	for _, binding := range bindingList.Items {
		// Check if this binding references the same tunnel
		// Note: TunnelBinding has TunnelRef at top level, not under Spec
		if binding.TunnelRef.Kind == config.Spec.TunnelRef.Kind &&
			binding.TunnelRef.Name == config.Spec.TunnelRef.Name {
			result = append(result, binding)
		}
	}

	// Sort by name for deterministic ordering
	sort.Slice(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace {
			return result[i].Namespace < result[j].Namespace
		}
		return result[i].Name < result[j].Name
	})

	return result, nil
}

// getTunnel returns the Tunnel or ClusterTunnel referenced by the config
func (r *Reconciler) getTunnel(ctx context.Context, config *networkingv1alpha2.TunnelIngressClassConfig) (TunnelInterface, error) {
	ref := config.Spec.TunnelRef

	switch ref.Kind {
	case "Tunnel":
		tunnel := &networkingv1alpha2.Tunnel{}
		namespace := config.GetTunnelNamespace()
		if err := r.Get(ctx, apitypes.NamespacedName{
			Name:      ref.Name,
			Namespace: namespace,
		}, tunnel); err != nil {
			return nil, fmt.Errorf("tunnel %s/%s not found: %w", namespace, ref.Name, err)
		}
		return &TunnelWrapper{Tunnel: tunnel}, nil

	case "ClusterTunnel":
		clusterTunnel := &networkingv1alpha2.ClusterTunnel{}
		if err := r.Get(ctx, apitypes.NamespacedName{Name: ref.Name}, clusterTunnel); err != nil {
			return nil, fmt.Errorf("clustertunnel %s not found: %w", ref.Name, err)
		}
		return &ClusterTunnelWrapper{ClusterTunnel: clusterTunnel, OperatorNamespace: r.OperatorNamespace}, nil

	default:
		return nil, fmt.Errorf("invalid tunnel kind: %s", ref.Kind)
	}
}

// updateIngressStatus updates the Ingress status
// nolint:revive // Cognitive complexity for status update logic
func (r *Reconciler) updateIngressStatus(
	ctx context.Context,
	ingress *networkingv1.Ingress,
	config *networkingv1alpha2.TunnelIngressClassConfig,
	reconcileErr error,
) error {
	return controller.UpdateStatusWithConflictRetry(ctx, r.Client, ingress, func() {
		if reconcileErr != nil {
			// Clear load balancer status on error
			ingress.Status.LoadBalancer = networkingv1.IngressLoadBalancerStatus{}
			return
		}

		if config == nil {
			return
		}

		// Get tunnel for load balancer hostname
		tunnel, err := r.getTunnel(ctx, config)
		if err != nil {
			return
		}

		// Set load balancer status with tunnel hostname
		tunnelID := tunnel.GetStatus().TunnelId
		if tunnelID != "" {
			ingress.Status.LoadBalancer = networkingv1.IngressLoadBalancerStatus{
				Ingress: []networkingv1.IngressLoadBalancerIngress{
					{
						Hostname: fmt.Sprintf("%s.cfargotunnel.com", tunnelID),
					},
				},
			}
		}
	})
}

// updateConfigStatus updates the TunnelIngressClassConfig status
func (r *Reconciler) updateConfigStatus(ctx context.Context, config *networkingv1alpha2.TunnelIngressClassConfig, ingressCount int) error {
	return controller.UpdateStatusWithConflictRetry(ctx, r.Client, config, func() {
		config.Status.IngressCount = ingressCount
		config.Status.State = "active"
		config.Status.ObservedGeneration = config.Generation

		// Get tunnel info
		tunnel, err := r.getTunnel(ctx, config)
		if err == nil {
			config.Status.TunnelID = tunnel.GetStatus().TunnelId
			config.Status.TunnelName = tunnel.GetStatus().TunnelName
		}

		// Set Ready condition
		controller.SetSuccessCondition(&config.Status.Conditions, "Configuration is active")
	})
}

// SetupWithManager sets up the controller with the Manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("cloudflare-ingress-controller")

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		// Watch IngressClass changes
		Watches(
			&networkingv1.IngressClass{},
			handler.EnqueueRequestsFromMapFunc(r.findIngressesForIngressClass),
		).
		// Watch TunnelIngressClassConfig changes
		Watches(
			&networkingv1alpha2.TunnelIngressClassConfig{},
			handler.EnqueueRequestsFromMapFunc(r.findIngressesForTunnelConfig),
		).
		// Watch Tunnel changes
		Watches(
			&networkingv1alpha2.Tunnel{},
			handler.EnqueueRequestsFromMapFunc(r.findIngressesForTunnel),
		).
		// Watch ClusterTunnel changes
		Watches(
			&networkingv1alpha2.ClusterTunnel{},
			handler.EnqueueRequestsFromMapFunc(r.findIngressesForClusterTunnel),
		).
		// Watch Service changes
		Watches(
			&corev1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.findIngressesForService),
		).
		Complete(r)
}

// findIngressesForIngressClass returns Ingresses that use the given IngressClass
func (r *Reconciler) findIngressesForIngressClass(ctx context.Context, obj client.Object) []reconcile.Request {
	ingressClass, ok := obj.(*networkingv1.IngressClass)
	if !ok {
		return nil
	}

	// Only process our IngressClasses
	if ingressClass.Spec.Controller != ControllerName {
		return nil
	}

	// Find all Ingresses using this class
	ingressList := &networkingv1.IngressList{}
	if err := r.List(ctx, ingressList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, ing := range ingressList.Items {
		if ing.Spec.IngressClassName != nil && *ing.Spec.IngressClassName == ingressClass.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: apitypes.NamespacedName{
					Name:      ing.Name,
					Namespace: ing.Namespace,
				},
			})
		}
	}

	return requests
}

// findIngressesForTunnelConfig returns Ingresses that use the given TunnelIngressClassConfig
func (r *Reconciler) findIngressesForTunnelConfig(ctx context.Context, obj client.Object) []reconcile.Request {
	config, ok := obj.(*networkingv1alpha2.TunnelIngressClassConfig)
	if !ok {
		return nil
	}

	ingresses, err := r.getIngressesForConfig(ctx, config)
	if err != nil {
		return nil
	}

	requests := make([]reconcile.Request, 0, len(ingresses))
	for _, ing := range ingresses {
		requests = append(requests, reconcile.Request{
			NamespacedName: apitypes.NamespacedName{
				Name:      ing.Name,
				Namespace: ing.Namespace,
			},
		})
	}

	return requests
}

// findIngressesForTunnel returns Ingresses that reference the given Tunnel
func (r *Reconciler) findIngressesForTunnel(ctx context.Context, obj client.Object) []reconcile.Request {
	tunnel, ok := obj.(*networkingv1alpha2.Tunnel)
	if !ok {
		return nil
	}
	return r.findIngressesForTunnelRef(ctx, "Tunnel", tunnel.Name, tunnel.Namespace)
}

// findIngressesForClusterTunnel returns Ingresses that reference the given ClusterTunnel
func (r *Reconciler) findIngressesForClusterTunnel(ctx context.Context, obj client.Object) []reconcile.Request {
	clusterTunnel, ok := obj.(*networkingv1alpha2.ClusterTunnel)
	if !ok {
		return nil
	}
	return r.findIngressesForTunnelRef(ctx, "ClusterTunnel", clusterTunnel.Name, "")
}

// findIngressesForTunnelRef finds Ingresses that reference a specific tunnel
// nolint:revive // Cognitive complexity for finding related Ingresses
func (r *Reconciler) findIngressesForTunnelRef(ctx context.Context, kind, name, namespace string) []reconcile.Request {
	// Find TunnelIngressClassConfigs that reference this tunnel
	configList := &networkingv1alpha2.TunnelIngressClassConfigList{}
	if err := r.List(ctx, configList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, config := range configList.Items {
		if config.Spec.TunnelRef.Kind == kind && config.Spec.TunnelRef.Name == name {
			// Check namespace for Tunnel
			if kind == "Tunnel" {
				configNs := config.GetTunnelNamespace()
				if configNs != namespace {
					continue
				}
			}

			// Get Ingresses for this config
			ingresses, err := r.getIngressesForConfig(ctx, &config)
			if err != nil {
				continue
			}

			for _, ing := range ingresses {
				requests = append(requests, reconcile.Request{
					NamespacedName: apitypes.NamespacedName{
						Name:      ing.Name,
						Namespace: ing.Namespace,
					},
				})
			}
		}
	}

	return requests
}

// findIngressesForService returns Ingresses that reference the given Service
// nolint:revive // Cognitive complexity for finding related Ingresses
func (r *Reconciler) findIngressesForService(ctx context.Context, obj client.Object) []reconcile.Request {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return nil
	}

	// Find Ingresses in the same namespace that reference this service
	ingressList := &networkingv1.IngressList{}
	if err := r.List(ctx, ingressList, client.InNamespace(svc.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, ing := range ingressList.Items {
		if !r.isOurIngress(ctx, &ing) {
			continue
		}

		// Check if any rule references this service
		for _, rule := range ing.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service != nil && path.Backend.Service.Name == svc.Name {
					requests = append(requests, reconcile.Request{
						NamespacedName: apitypes.NamespacedName{
							Name:      ing.Name,
							Namespace: ing.Namespace,
						},
					})
					break
				}
			}
		}
	}

	return requests
}
