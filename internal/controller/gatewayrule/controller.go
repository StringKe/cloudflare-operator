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

package gatewayrule

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

const (
	FinalizerName = "gatewayrule.networking.cloudflare-operator.io/finalizer"
)

// GatewayRuleReconciler reconciles a GatewayRule object
type GatewayRuleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayrules/finalizers,verbs=update

func (r *GatewayRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the GatewayRule instance
	rule := &networkingv1alpha2.GatewayRule{}
	if err := r.Get(ctx, req.NamespacedName, rule); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Initialize API client
	apiClient, err := cf.NewAPIClientFromDetails(ctx, r.Client, "", rule.Spec.Cloudflare)
	if err != nil {
		logger.Error(err, "Failed to initialize Cloudflare API client")
		return r.updateStatusError(ctx, rule, err)
	}

	// Handle deletion
	if !rule.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, rule, apiClient)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(rule, FinalizerName) {
		controllerutil.AddFinalizer(rule, FinalizerName)
		if err := r.Update(ctx, rule); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile the gateway rule
	return r.reconcileGatewayRule(ctx, rule, apiClient)
}

func (r *GatewayRuleReconciler) handleDeletion(ctx context.Context, rule *networkingv1alpha2.GatewayRule, apiClient *cf.API) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(rule, FinalizerName) {
		// Delete from Cloudflare
		if rule.Status.RuleID != "" {
			logger.Info("Deleting Gateway Rule from Cloudflare", "ruleId", rule.Status.RuleID)
			if err := apiClient.DeleteGatewayRule(rule.Status.RuleID); err != nil {
				logger.Error(err, "Failed to delete Gateway Rule from Cloudflare")
			}
		}

		// Remove finalizer
		controllerutil.RemoveFinalizer(rule, FinalizerName)
		if err := r.Update(ctx, rule); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *GatewayRuleReconciler) reconcileGatewayRule(ctx context.Context, rule *networkingv1alpha2.GatewayRule, apiClient *cf.API) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Use the Name from the spec, or fall back to the resource name
	ruleName := rule.Spec.Name
	if ruleName == "" {
		ruleName = rule.Name
	}

	// Convert filters from []string to []cloudflare.TeamsFilterType
	var filters []cloudflare.TeamsFilterType
	for _, f := range rule.Spec.Filters {
		filters = append(filters, cloudflare.TeamsFilterType(f))
	}

	// Build gateway rule params
	params := cf.GatewayRuleParams{
		Name:          ruleName,
		Description:   rule.Spec.Description,
		Action:        rule.Spec.Action,
		Traffic:       rule.Spec.Traffic,
		Identity:      rule.Spec.Identity,
		DevicePosture: rule.Spec.DevicePosture,
		Enabled:       rule.Spec.Enabled,
		Precedence:    rule.Spec.Precedence,
		Filters:       filters,
	}

	// Build rule settings
	if rule.Spec.RuleSettings != nil {
		params.RuleSettings = r.buildRuleSettings(rule.Spec.RuleSettings)
	}

	var result *cf.GatewayRuleResult
	var err error

	if rule.Status.RuleID == "" {
		// Create new gateway rule
		logger.Info("Creating Gateway Rule", "name", params.Name, "action", params.Action)
		result, err = apiClient.CreateGatewayRule(params)
	} else {
		// Update existing gateway rule
		logger.Info("Updating Gateway Rule", "ruleId", rule.Status.RuleID)
		result, err = apiClient.UpdateGatewayRule(rule.Status.RuleID, params)
	}

	if err != nil {
		return r.updateStatusError(ctx, rule, err)
	}

	// Update status
	return r.updateStatusSuccess(ctx, rule, result)
}

func (r *GatewayRuleReconciler) buildRuleSettings(settings *networkingv1alpha2.GatewayRuleSettings) map[string]interface{} {
	result := make(map[string]interface{})

	if settings.BlockPageEnabled != nil {
		result["block_page_enabled"] = *settings.BlockPageEnabled
	}
	if settings.BlockReason != "" {
		result["block_reason"] = settings.BlockReason
	}
	if settings.OverrideIPs != nil {
		result["override_ips"] = settings.OverrideIPs
	}
	if settings.OverrideHost != "" {
		result["override_host"] = settings.OverrideHost
	}
	if settings.L4Override != nil {
		result["l4override"] = map[string]interface{}{
			"ip":   settings.L4Override.IP,
			"port": settings.L4Override.Port,
		}
	}
	if settings.BISOAdminControls != nil {
		bisoMap := make(map[string]interface{})
		if settings.BISOAdminControls.DisablePrinting != nil {
			bisoMap["dp"] = *settings.BISOAdminControls.DisablePrinting
		}
		if settings.BISOAdminControls.DisableCopyPaste != nil {
			bisoMap["dcp"] = *settings.BISOAdminControls.DisableCopyPaste
		}
		if settings.BISOAdminControls.DisableDownload != nil {
			bisoMap["dd"] = *settings.BISOAdminControls.DisableDownload
		}
		if settings.BISOAdminControls.DisableUpload != nil {
			bisoMap["du"] = *settings.BISOAdminControls.DisableUpload
		}
		if settings.BISOAdminControls.DisableKeyboard != nil {
			bisoMap["dk"] = *settings.BISOAdminControls.DisableKeyboard
		}
		result["biso_admin_controls"] = bisoMap
	}
	if settings.CheckSession != nil {
		result["check_session"] = map[string]interface{}{
			"enforce":  settings.CheckSession.Enforce,
			"duration": settings.CheckSession.Duration,
		}
	}
	if settings.AddHeaders != nil {
		result["add_headers"] = settings.AddHeaders
	}
	if settings.InsecureDisableDNSSECValidation != nil {
		result["insecure_disable_dnssec_validation"] = *settings.InsecureDisableDNSSECValidation
	}
	if settings.Egress != nil {
		egressMap := make(map[string]interface{})
		if settings.Egress.IPv4 != "" {
			egressMap["ipv4"] = settings.Egress.IPv4
		}
		if settings.Egress.IPv6 != "" {
			egressMap["ipv6"] = settings.Egress.IPv6
		}
		if settings.Egress.IPv4Fallback != "" {
			egressMap["ipv4_fallback"] = settings.Egress.IPv4Fallback
		}
		result["egress"] = egressMap
	}
	if settings.PayloadLog != nil {
		result["payload_log"] = map[string]interface{}{
			"enabled": settings.PayloadLog.Enabled,
		}
	}
	if settings.UntrustedCertificateAction != "" {
		result["untrusted_cert"] = map[string]interface{}{
			"action": settings.UntrustedCertificateAction,
		}
	}
	if settings.ResolveDNSInternally != nil {
		result["resolve_dns_internally"] = *settings.ResolveDNSInternally
	}
	if settings.NotificationSettings != nil {
		result["notification_settings"] = map[string]interface{}{
			"enabled":     settings.NotificationSettings.Enabled,
			"msg":         settings.NotificationSettings.Message,
			"support_url": settings.NotificationSettings.SupportURL,
		}
	}

	return result
}

func (r *GatewayRuleReconciler) updateStatusError(ctx context.Context, rule *networkingv1alpha2.GatewayRule, err error) (ctrl.Result, error) {
	rule.Status.State = "Error"
	meta.SetStatusCondition(&rule.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "ReconcileError",
		Message:            err.Error(),
		LastTransitionTime: metav1.Now(),
	})
	rule.Status.ObservedGeneration = rule.Generation

	if updateErr := r.Status().Update(ctx, rule); updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GatewayRuleReconciler) updateStatusSuccess(ctx context.Context, rule *networkingv1alpha2.GatewayRule, result *cf.GatewayRuleResult) (ctrl.Result, error) {
	rule.Status.RuleID = result.ID
	rule.Status.State = "Ready"
	meta.SetStatusCondition(&rule.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "Gateway Rule successfully reconciled",
		LastTransitionTime: metav1.Now(),
	})
	rule.Status.ObservedGeneration = rule.Generation

	if err := r.Status().Update(ctx, rule); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *GatewayRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.GatewayRule{}).
		Complete(r)
}
