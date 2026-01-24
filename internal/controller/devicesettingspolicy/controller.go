// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package devicesettingspolicy provides a controller for managing Cloudflare Device Settings Policy.
// It directly calls Cloudflare API and writes status back to the CRD.
package devicesettingspolicy

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
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
	finalizerName = "devicesettingspolicy.networking.cloudflare-operator.io/finalizer"
)

// Reconciler reconciles a DeviceSettingsPolicy object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=devicesettingspolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=devicesettingspolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=devicesettingspolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=networkroutes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile handles DeviceSettingsPolicy reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the DeviceSettingsPolicy resource
	policy := &networkingv1alpha2.DeviceSettingsPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch DeviceSettingsPolicy")
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !policy.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, policy)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, policy, finalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Get API client
	// DeviceSettingsPolicy is cluster-scoped, use operator namespace for legacy inline secrets
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &policy.Spec.Cloudflare,
		Namespace:         common.OperatorNamespace,
		StatusAccountID:   policy.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, policy, err)
	}

	// Sync Device Settings Policy to Cloudflare
	return r.syncDeviceSettingsPolicy(ctx, policy, apiResult)
}

// handleDeletion handles the deletion of DeviceSettingsPolicy.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	policy *networkingv1alpha2.DeviceSettingsPolicy,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(policy, finalizerName) {
		return common.NoRequeue(), nil
	}

	// Note: We don't delete the split tunnel or fallback domain configurations
	// because they are account-wide settings. The user should manage this manually
	// or use another DeviceSettingsPolicy to reset them.
	logger.Info("Removing finalizer for DeviceSettingsPolicy (account-wide settings not deleted from CF)")

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, policy, func() {
		controllerutil.RemoveFinalizer(policy, finalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(policy, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// syncDeviceSettingsPolicy syncs the Device Settings Policy to Cloudflare.
//
//nolint:revive // cognitive complexity is acceptable for policy synchronization
func (r *Reconciler) syncDeviceSettingsPolicy(
	ctx context.Context,
	policy *networkingv1alpha2.DeviceSettingsPolicy,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Collect split tunnel entries
	var excludeEntries, includeEntries []cf.SplitTunnelEntry

	// Convert split tunnel exclude entries
	for _, e := range policy.Spec.SplitTunnelExclude {
		excludeEntries = append(excludeEntries, cf.SplitTunnelEntry{
			Address:     e.Address,
			Host:        e.Host,
			Description: e.Description,
		})
	}

	// Convert split tunnel include entries
	for _, e := range policy.Spec.SplitTunnelInclude {
		includeEntries = append(includeEntries, cf.SplitTunnelEntry{
			Address:     e.Address,
			Host:        e.Host,
			Description: e.Description,
		})
	}

	// Auto-populate from NetworkRoutes if enabled
	autoPopulatedCount := 0
	if policy.Spec.AutoPopulateFromRoutes != nil && policy.Spec.AutoPopulateFromRoutes.Enabled {
		autoEntries, err := r.getAutoPopulatedEntries(ctx, policy)
		if err != nil {
			logger.Error(err, "Failed to auto-populate from NetworkRoutes")
			// Continue anyway with manual entries
		} else {
			autoPopulatedCount = len(autoEntries)
			// Add auto-populated entries to appropriate list based on mode
			if policy.Spec.SplitTunnelMode == "include" {
				includeEntries = append(includeEntries, autoEntries...)
			} else {
				// Default to exclude mode
				excludeEntries = append(excludeEntries, autoEntries...)
			}
		}
	}

	// Convert fallback domain entries
	var fallbackEntries []cf.FallbackDomainEntry
	for _, e := range policy.Spec.FallbackDomains {
		fallbackEntries = append(fallbackEntries, cf.FallbackDomainEntry{
			Suffix:      e.Suffix,
			Description: e.Description,
			DNSServer:   e.DNSServer,
		})
	}

	// Update Split Tunnel Exclude
	logger.V(1).Info("Updating Split Tunnel Exclude list in Cloudflare",
		"count", len(excludeEntries))
	if err := apiResult.API.UpdateSplitTunnelExclude(ctx, excludeEntries); err != nil {
		logger.Error(err, "Failed to update Split Tunnel Exclude list")
		return r.updateStatusError(ctx, policy, err)
	}

	// Update Split Tunnel Include
	logger.V(1).Info("Updating Split Tunnel Include list in Cloudflare",
		"count", len(includeEntries))
	if err := apiResult.API.UpdateSplitTunnelInclude(ctx, includeEntries); err != nil {
		logger.Error(err, "Failed to update Split Tunnel Include list")
		return r.updateStatusError(ctx, policy, err)
	}

	// Update Fallback Domains
	logger.V(1).Info("Updating Fallback Domains in Cloudflare",
		"count", len(fallbackEntries))
	if err := apiResult.API.UpdateFallbackDomains(ctx, fallbackEntries); err != nil {
		logger.Error(err, "Failed to update Fallback Domains")
		return r.updateStatusError(ctx, policy, err)
	}

	r.Recorder.Event(policy, corev1.EventTypeNormal, "Updated",
		fmt.Sprintf("Device Settings Policy updated in Cloudflare (exclude=%d, include=%d, fallback=%d)",
			len(excludeEntries), len(includeEntries), len(fallbackEntries)))

	return r.updateStatusReady(ctx, policy, apiResult.AccountID,
		len(excludeEntries), len(includeEntries), len(fallbackEntries), autoPopulatedCount)
}

// getAutoPopulatedEntries retrieves entries from NetworkRoute resources.
func (r *Reconciler) getAutoPopulatedEntries(
	ctx context.Context,
	policy *networkingv1alpha2.DeviceSettingsPolicy,
) ([]cf.SplitTunnelEntry, error) {
	config := policy.Spec.AutoPopulateFromRoutes
	if config == nil {
		return nil, nil
	}

	// List NetworkRoutes with optional label selector
	routeList := &networkingv1alpha2.NetworkRouteList{}
	listOpts := []client.ListOption{}

	if config.LabelSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(config.LabelSelector)
		if err != nil {
			return nil, fmt.Errorf("invalid label selector: %w", err)
		}
		listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
	}

	if err := r.List(ctx, routeList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list NetworkRoutes: %w", err)
	}

	// Convert to split tunnel entries
	entries := make([]cf.SplitTunnelEntry, 0, len(routeList.Items))
	prefix := config.DescriptionPrefix
	if prefix == "" {
		prefix = "Auto-populated from NetworkRoute: "
	}

	for _, route := range routeList.Items {
		if route.Spec.Network != "" {
			entries = append(entries, cf.SplitTunnelEntry{
				Address:     route.Spec.Network,
				Description: prefix + route.Name,
			})
		}
	}

	return entries, nil
}

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	policy *networkingv1alpha2.DeviceSettingsPolicy,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, policy, func() {
		policy.Status.State = "Error"
		meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: policy.Generation,
			Reason:             "Error",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		policy.Status.ObservedGeneration = policy.Generation
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
}

func (r *Reconciler) updateStatusReady(
	ctx context.Context,
	policy *networkingv1alpha2.DeviceSettingsPolicy,
	accountID string,
	excludeCount, includeCount, fallbackCount, autoPopulatedCount int,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, policy, func() {
		policy.Status.AccountID = accountID
		policy.Status.SplitTunnelExcludeCount = excludeCount
		policy.Status.SplitTunnelIncludeCount = includeCount
		policy.Status.FallbackDomainsCount = fallbackCount
		policy.Status.AutoPopulatedRoutesCount = autoPopulatedCount
		policy.Status.State = "Ready"
		meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: policy.Generation,
			Reason:             "Synced",
			Message:            "Device Settings Policy synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		policy.Status.ObservedGeneration = policy.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
}

// policyMatchesNetworkRoute checks if a DeviceSettingsPolicy should be triggered by a NetworkRoute change.
func policyMatchesNetworkRoute(policy *networkingv1alpha2.DeviceSettingsPolicy, route *networkingv1alpha2.NetworkRoute) bool {
	if policy.Spec.AutoPopulateFromRoutes == nil || !policy.Spec.AutoPopulateFromRoutes.Enabled {
		return false
	}

	// Check if the NetworkRoute matches the label selector (if any)
	if policy.Spec.AutoPopulateFromRoutes.LabelSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(policy.Spec.AutoPopulateFromRoutes.LabelSelector)
		if err != nil {
			return false
		}
		if !selector.Matches(labels.Set(route.Labels)) {
			return false
		}
	}

	return true
}

// findDeviceSettingsPoliciesForNetworkRoute returns reconcile requests for DeviceSettingsPolicies
// that have AutoPopulateFromRoutes enabled and might reference the given NetworkRoute.
func (r *Reconciler) findDeviceSettingsPoliciesForNetworkRoute(ctx context.Context, obj client.Object) []reconcile.Request {
	route, ok := obj.(*networkingv1alpha2.NetworkRoute)
	if !ok {
		return nil
	}
	logger := log.FromContext(ctx)

	// Find all DeviceSettingsPolicies that have AutoPopulateFromRoutes enabled
	policyList := &networkingv1alpha2.DeviceSettingsPolicyList{}
	if err := r.List(ctx, policyList); err != nil {
		logger.Error(err, "Failed to list DeviceSettingsPolicies for NetworkRoute watch")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(policyList.Items))
	for i := range policyList.Items {
		policy := &policyList.Items[i]
		if !policyMatchesNetworkRoute(policy, route) {
			continue
		}

		logger.V(1).Info("NetworkRoute changed, triggering DeviceSettingsPolicy reconcile",
			"networkroute", route.Name,
			"devicesettingspolicy", policy.Name)
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(policy),
		})
	}

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("devicesettingspolicy-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("devicesettingspolicy"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.DeviceSettingsPolicy{}).
		// Watch NetworkRoute changes for auto-populate feature
		Watches(
			&networkingv1alpha2.NetworkRoute{},
			handler.EnqueueRequestsFromMapFunc(r.findDeviceSettingsPoliciesForNetworkRoute),
		).
		Named("devicesettingspolicy").
		Complete(r)
}
