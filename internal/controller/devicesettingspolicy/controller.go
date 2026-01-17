// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package devicesettingspolicy implements the controller for DeviceSettingsPolicy resources.
package devicesettingspolicy

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	"github.com/StringKe/cloudflare-operator/internal/service"
	devicesvc "github.com/StringKe/cloudflare-operator/internal/service/device"
)

const (
	FinalizerName = "devicesettingspolicy.networking.cloudflare-operator.io/finalizer"
)

// Reconciler reconciles a DeviceSettingsPolicy object
type Reconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	deviceService *devicesvc.DeviceSettingsPolicyService
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=devicesettingspolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=devicesettingspolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=devicesettingspolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=networkroutes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile implements the reconciliation loop for DeviceSettingsPolicy resources.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the DeviceSettingsPolicy resource
	policy := &networkingv1alpha2.DeviceSettingsPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Resolve credentials
	credRef, accountID, err := r.resolveCredentials(ctx, policy)
	if err != nil {
		logger.Error(err, "Failed to resolve credentials")
		return r.updateStatusError(ctx, policy, err)
	}

	// Check if DeviceSettingsPolicy is being deleted
	if !policy.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, policy, accountID)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(policy, FinalizerName) {
		controllerutil.AddFinalizer(policy, FinalizerName)
		if err := r.Update(ctx, policy); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Event(policy, corev1.EventTypeNormal, controller.EventReasonFinalizerSet, "Finalizer added")
	}

	// Register Device Settings Policy configuration to SyncState
	return r.registerDeviceSettingsPolicy(ctx, policy, accountID, credRef)
}

// resolveCredentials resolves the credentials reference and account ID.
func (r *Reconciler) resolveCredentials(
	ctx context.Context,
	policy *networkingv1alpha2.DeviceSettingsPolicy,
) (credRef networkingv1alpha2.CredentialsReference, accountID string, err error) {
	// Get credentials reference
	if policy.Spec.Cloudflare.CredentialsRef != nil {
		credRef = networkingv1alpha2.CredentialsReference{
			Name: policy.Spec.Cloudflare.CredentialsRef.Name,
		}

		// Get account ID from credentials if available
		creds := &networkingv1alpha2.CloudflareCredentials{}
		if err := r.Get(ctx, client.ObjectKey{Name: credRef.Name}, creds); err != nil {
			return credRef, "", fmt.Errorf("get credentials: %w", err)
		}
		accountID = creds.Spec.AccountID
	}

	if credRef.Name == "" {
		return credRef, "", errors.New("credentials reference is required")
	}

	return credRef, accountID, nil
}

// handleDeletion handles the deletion of a DeviceSettingsPolicy.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	policy *networkingv1alpha2.DeviceSettingsPolicy,
	accountID string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(policy, FinalizerName) {
		return ctrl.Result{}, nil
	}

	logger.Info("Deleting DeviceSettingsPolicy")
	r.Recorder.Event(policy, corev1.EventTypeNormal, "Deleting", "Starting DeviceSettingsPolicy deletion")

	// Note: We don't delete the split tunnel or fallback domain configurations
	// because they are account-wide settings. The user should manage this manually
	// or use another DeviceSettingsPolicy to reset them.

	// Unregister from SyncState
	source := service.Source{
		Kind:      "DeviceSettingsPolicy",
		Namespace: "",
		Name:      policy.Name,
	}
	if err := r.deviceService.Unregister(ctx, accountID, source); err != nil {
		logger.Error(err, "Failed to unregister DeviceSettingsPolicy from SyncState")
		// Non-fatal - continue with finalizer removal
	}

	// Remove finalizer with retry logic
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, policy, func() {
		controllerutil.RemoveFinalizer(policy, FinalizerName)
	}); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(policy, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// registerDeviceSettingsPolicy registers the Device Settings Policy configuration to SyncState.
//
//nolint:revive // cognitive complexity is acceptable for configuration registration
func (r *Reconciler) registerDeviceSettingsPolicy(
	ctx context.Context,
	policy *networkingv1alpha2.DeviceSettingsPolicy,
	accountID string,
	credRef networkingv1alpha2.CredentialsReference,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Build device settings policy configuration
	config := devicesvc.DeviceSettingsPolicyConfig{
		SplitTunnelMode: policy.Spec.SplitTunnelMode,
	}

	// Convert split tunnel exclude entries
	for _, e := range policy.Spec.SplitTunnelExclude {
		config.SplitTunnelExclude = append(config.SplitTunnelExclude, devicesvc.SplitTunnelEntry{
			Address:     e.Address,
			Host:        e.Host,
			Description: e.Description,
		})
	}

	// Convert split tunnel include entries
	for _, e := range policy.Spec.SplitTunnelInclude {
		config.SplitTunnelInclude = append(config.SplitTunnelInclude, devicesvc.SplitTunnelEntry{
			Address:     e.Address,
			Host:        e.Host,
			Description: e.Description,
		})
	}

	// Convert fallback domain entries
	for _, e := range policy.Spec.FallbackDomains {
		config.FallbackDomains = append(config.FallbackDomains, devicesvc.FallbackDomainEntry{
			Suffix:      e.Suffix,
			Description: e.Description,
			DNSServer:   e.DNSServer,
		})
	}

	// Auto-populate from NetworkRoutes if enabled
	if policy.Spec.AutoPopulateFromRoutes != nil && policy.Spec.AutoPopulateFromRoutes.Enabled {
		autoEntries, err := r.getAutoPopulatedEntries(ctx, policy)
		if err != nil {
			logger.Error(err, "Failed to auto-populate from NetworkRoutes")
			// Continue anyway with manual entries
		} else {
			config.AutoPopulatedRoutes = autoEntries
		}
	}

	// Create source reference
	source := service.Source{
		Kind:      "DeviceSettingsPolicy",
		Namespace: "",
		Name:      policy.Name,
	}

	// Register to SyncState
	opts := devicesvc.DeviceSettingsPolicyRegisterOptions{
		AccountID:      accountID,
		Source:         source,
		Config:         config,
		CredentialsRef: credRef,
	}

	if err := r.deviceService.Register(ctx, opts); err != nil {
		logger.Error(err, "Failed to register DeviceSettingsPolicy configuration")
		r.Recorder.Event(policy, corev1.EventTypeWarning, "RegisterFailed",
			fmt.Sprintf("Failed to register DeviceSettingsPolicy: %s", err.Error()))
		return r.updateStatusError(ctx, policy, err)
	}

	r.Recorder.Event(policy, corev1.EventTypeNormal, "Registered",
		"Registered DeviceSettingsPolicy configuration to SyncState")

	// Update status to Pending - actual sync happens via DeviceSyncController
	return r.updateStatusPending(ctx, policy, accountID, &config)
}

// getAutoPopulatedEntries retrieves entries from NetworkRoute resources.
//
//nolint:revive // cognitive complexity is acceptable for resource filtering logic
func (r *Reconciler) getAutoPopulatedEntries(
	ctx context.Context,
	policy *networkingv1alpha2.DeviceSettingsPolicy,
) ([]devicesvc.SplitTunnelEntry, error) {
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
	entries := make([]devicesvc.SplitTunnelEntry, 0, len(routeList.Items))
	prefix := config.DescriptionPrefix
	if prefix == "" {
		prefix = "Auto-populated from NetworkRoute: "
	}

	for _, route := range routeList.Items {
		if route.Spec.Network != "" {
			entries = append(entries, devicesvc.SplitTunnelEntry{
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
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		policy.Status.ObservedGeneration = policy.Generation
	})
	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *Reconciler) updateStatusPending(
	ctx context.Context,
	policy *networkingv1alpha2.DeviceSettingsPolicy,
	accountID string,
	config *devicesvc.DeviceSettingsPolicyConfig,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, policy, func() {
		if policy.Status.AccountID == "" {
			policy.Status.AccountID = accountID
		}
		policy.Status.State = "Pending"
		policy.Status.SplitTunnelExcludeCount = len(config.SplitTunnelExclude)
		policy.Status.SplitTunnelIncludeCount = len(config.SplitTunnelInclude)
		policy.Status.FallbackDomainsCount = len(config.FallbackDomains)
		policy.Status.AutoPopulatedRoutesCount = len(config.AutoPopulatedRoutes)
		meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: policy.Generation,
			Reason:             "Pending",
			Message:            "DeviceSettingsPolicy configuration registered, waiting for sync",
			LastTransitionTime: metav1.Now(),
		})
		policy.Status.ObservedGeneration = policy.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	// Requeue to check for sync completion
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
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

// findDeviceSettingsPoliciesForNetworkRoute returns a list of reconcile requests for DeviceSettingsPolicies
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

		logger.Info("NetworkRoute changed, triggering DeviceSettingsPolicy reconcile",
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

	// Initialize DeviceSettingsPolicyService
	r.deviceService = devicesvc.NewDeviceSettingsPolicyService(mgr.GetClient())

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.DeviceSettingsPolicy{}).
		// Watch NetworkRoute changes for auto-populate feature
		Watches(
			&networkingv1alpha2.NetworkRoute{},
			handler.EnqueueRequestsFromMapFunc(r.findDeviceSettingsPoliciesForNetworkRoute),
		).
		Complete(r)
}

// Ensure labels import is used
var _ = labels.Everything
