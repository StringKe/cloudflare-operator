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

package devicesettingspolicy

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

// Reconciler reconciles a DeviceSettingsPolicy object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Runtime state
	ctx    context.Context
	log    logr.Logger
	policy *networkingv1alpha2.DeviceSettingsPolicy
	cfAPI  *cf.API
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=devicesettingspolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=devicesettingspolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=devicesettingspolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=networkroutes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile implements the reconciliation loop for DeviceSettingsPolicy resources.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrllog.FromContext(ctx)

	// Fetch the DeviceSettingsPolicy resource
	r.policy = &networkingv1alpha2.DeviceSettingsPolicy{}
	if err := r.Get(ctx, req.NamespacedName, r.policy); err != nil {
		if apierrors.IsNotFound(err) {
			r.log.Info("DeviceSettingsPolicy deleted, nothing to do")
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch DeviceSettingsPolicy")
		return ctrl.Result{}, err
	}

	// Initialize API client
	if err := r.initAPIClient(); err != nil {
		r.log.Error(err, "failed to initialize API client")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonAPIError, err.Error())
		return ctrl.Result{}, err
	}

	// Check if DeviceSettingsPolicy is being deleted
	if r.policy.GetDeletionTimestamp() != nil {
		return r.handleDeletion()
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.policy, controller.DeviceSettingsPolicyFinalizer) {
		controllerutil.AddFinalizer(r.policy, controller.DeviceSettingsPolicyFinalizer)
		if err := r.Update(ctx, r.policy); err != nil {
			r.log.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(r.policy, corev1.EventTypeNormal, controller.EventReasonFinalizerSet, "Finalizer added")
	}

	// Reconcile the DeviceSettingsPolicy
	if err := r.reconcilePolicy(); err != nil {
		r.log.Error(err, "failed to reconcile DeviceSettingsPolicy")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	return ctrl.Result{}, nil
}

// initAPIClient initializes the Cloudflare API client using the unified credential loader.
func (r *Reconciler) initAPIClient() error {
	// Use the unified API client initialization
	api, err := cf.NewAPIClientFromDetails(r.ctx, r.Client, "", r.policy.Spec.Cloudflare)
	if err != nil {
		r.log.Error(err, "failed to initialize API client")
		r.Recorder.Event(r.policy, corev1.EventTypeWarning, controller.EventReasonAPIError, "Failed to initialize API client")
		return err
	}

	// Preserve validated account ID from status
	api.ValidAccountId = r.policy.Status.AccountID
	r.cfAPI = api

	return nil
}

// handleDeletion handles the deletion of a DeviceSettingsPolicy.
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.policy, controller.DeviceSettingsPolicyFinalizer) {
		return ctrl.Result{}, nil
	}

	r.log.Info("Deleting DeviceSettingsPolicy")
	r.Recorder.Event(r.policy, corev1.EventTypeNormal, "Deleting", "Starting DeviceSettingsPolicy deletion")

	// Note: We don't delete the split tunnel or fallback domain configurations
	// because they are account-wide settings. The user should manage this manually
	// or use another DeviceSettingsPolicy to reset them.

	// Remove finalizer
	controllerutil.RemoveFinalizer(r.policy, controller.DeviceSettingsPolicyFinalizer)
	if err := r.Update(r.ctx, r.policy); err != nil {
		r.log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(r.policy, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// reconcilePolicy ensures the device settings are configured in Cloudflare.
func (r *Reconciler) reconcilePolicy() error {
	var autoPopulatedCount int

	// Collect split tunnel entries
	excludeEntries := r.convertSplitTunnelEntries(r.policy.Spec.SplitTunnelExclude)
	includeEntries := r.convertSplitTunnelEntries(r.policy.Spec.SplitTunnelInclude)

	// Auto-populate from NetworkRoutes if enabled
	if r.policy.Spec.AutoPopulateFromRoutes != nil && r.policy.Spec.AutoPopulateFromRoutes.Enabled {
		autoEntries, err := r.getAutoPopulatedEntries()
		if err != nil {
			r.log.Error(err, "failed to auto-populate from NetworkRoutes")
			// Continue anyway with manual entries
		} else {
			autoPopulatedCount = len(autoEntries)
			if r.policy.Spec.SplitTunnelMode == "include" {
				includeEntries = append(includeEntries, autoEntries...)
			} else {
				// For exclude mode, we add the routes to exclude to ensure they go through the tunnel
				// Actually, for private network access, we'd want to INCLUDE them, not exclude
				// Let's add them to include instead
				includeEntries = append(includeEntries, autoEntries...)
			}
		}
	}

	// Update split tunnel exclude list
	if len(excludeEntries) > 0 || r.policy.Spec.SplitTunnelMode == "exclude" {
		if err := r.cfAPI.UpdateSplitTunnelExclude(excludeEntries); err != nil {
			r.log.Error(err, "failed to update split tunnel exclude list")
			r.setCondition(metav1.ConditionFalse, controller.EventReasonUpdateFailed, err.Error())
			return err
		}
	}

	// Update split tunnel include list
	if len(includeEntries) > 0 || r.policy.Spec.SplitTunnelMode == "include" {
		if err := r.cfAPI.UpdateSplitTunnelInclude(includeEntries); err != nil {
			r.log.Error(err, "failed to update split tunnel include list")
			r.setCondition(metav1.ConditionFalse, controller.EventReasonUpdateFailed, err.Error())
			return err
		}
	}

	// Update fallback domains
	if len(r.policy.Spec.FallbackDomains) > 0 {
		fallbackEntries := r.convertFallbackDomainEntries(r.policy.Spec.FallbackDomains)
		if err := r.cfAPI.UpdateFallbackDomains(fallbackEntries); err != nil {
			r.log.Error(err, "failed to update fallback domains")
			r.setCondition(metav1.ConditionFalse, controller.EventReasonUpdateFailed, err.Error())
			return err
		}
	}

	// Update status
	return r.updateStatus(len(excludeEntries), len(includeEntries), len(r.policy.Spec.FallbackDomains), autoPopulatedCount)
}

// convertSplitTunnelEntries converts spec entries to CF API entries.
func (r *Reconciler) convertSplitTunnelEntries(entries []networkingv1alpha2.SplitTunnelEntry) []cf.SplitTunnelEntry {
	result := make([]cf.SplitTunnelEntry, len(entries))
	for i, e := range entries {
		result[i] = cf.SplitTunnelEntry{
			Address:     e.Address,
			Host:        e.Host,
			Description: e.Description,
		}
	}
	return result
}

// convertFallbackDomainEntries converts spec entries to CF API entries.
func (r *Reconciler) convertFallbackDomainEntries(entries []networkingv1alpha2.FallbackDomainEntry) []cf.FallbackDomainEntry {
	result := make([]cf.FallbackDomainEntry, len(entries))
	for i, e := range entries {
		result[i] = cf.FallbackDomainEntry{
			Suffix:      e.Suffix,
			Description: e.Description,
			DNSServer:   e.DNSServer,
		}
	}
	return result
}

// getAutoPopulatedEntries retrieves entries from NetworkRoute resources.
func (r *Reconciler) getAutoPopulatedEntries() ([]cf.SplitTunnelEntry, error) {
	config := r.policy.Spec.AutoPopulateFromRoutes
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

	if err := r.List(r.ctx, routeList, listOpts...); err != nil {
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

// updateStatus updates the DeviceSettingsPolicy status.
func (r *Reconciler) updateStatus(excludeCount, includeCount, fallbackCount, autoPopulatedCount int) error {
	r.policy.Status.AccountID = r.cfAPI.ValidAccountId
	r.policy.Status.SplitTunnelExcludeCount = excludeCount
	r.policy.Status.SplitTunnelIncludeCount = includeCount
	r.policy.Status.FallbackDomainsCount = fallbackCount
	r.policy.Status.AutoPopulatedRoutesCount = autoPopulatedCount
	r.policy.Status.State = "active"
	r.policy.Status.ObservedGeneration = r.policy.Generation

	r.setCondition(metav1.ConditionTrue, controller.EventReasonReconciled, "DeviceSettingsPolicy reconciled successfully")

	if err := r.Status().Update(r.ctx, r.policy); err != nil {
		r.log.Error(err, "failed to update DeviceSettingsPolicy status")
		return err
	}

	r.log.Info("DeviceSettingsPolicy status updated",
		"excludeCount", excludeCount,
		"includeCount", includeCount,
		"fallbackCount", fallbackCount,
		"autoPopulatedCount", autoPopulatedCount)
	return nil
}

// setCondition sets a condition on the DeviceSettingsPolicy status.
func (r *Reconciler) setCondition(status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: r.policy.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	// Update or append condition
	found := false
	for i, c := range r.policy.Status.Conditions {
		if c.Type == condition.Type {
			r.policy.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		r.policy.Status.Conditions = append(r.policy.Status.Conditions, condition)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("devicesettingspolicy-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.DeviceSettingsPolicy{}).
		Complete(r)
}

// Ensure labels import is used
var _ = labels.Everything
