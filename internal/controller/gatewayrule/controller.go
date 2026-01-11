// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package gatewayrule

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

const (
	FinalizerName = "gatewayrule.networking.cloudflare-operator.io/finalizer"
)

// GatewayRuleReconciler reconciles a GatewayRule object
type GatewayRuleReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
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
	// GatewayRule is cluster-scoped, use operator namespace for legacy inline secrets
	apiClient, err := cf.NewAPIClientFromDetails(ctx, r.Client, controller.OperatorNamespace, rule.Spec.Cloudflare)
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
				// P0 FIX: Check if resource already deleted
				if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to delete Gateway Rule from Cloudflare")
					r.Recorder.Event(rule, corev1.EventTypeWarning, controller.EventReasonDeleteFailed,
						fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
					return ctrl.Result{RequeueAfter: 30 * time.Second}, err
				}
				logger.Info("Gateway Rule already deleted from Cloudflare")
				r.Recorder.Event(rule, corev1.EventTypeNormal, "AlreadyDeleted", "Gateway Rule was already deleted from Cloudflare")
			} else {
				r.Recorder.Event(rule, corev1.EventTypeNormal, controller.EventReasonDeleted, "Deleted from Cloudflare")
			}
		}

		// P0 FIX: Remove finalizer with retry logic to handle conflicts
		if err := controller.UpdateWithConflictRetry(ctx, r.Client, rule, func() {
			controllerutil.RemoveFinalizer(rule, FinalizerName)
		}); err != nil {
			logger.Error(err, "failed to remove finalizer")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(rule, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")
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
		params.RuleSettings = buildRuleSettings(rule.Spec.RuleSettings)
	}

	// Build schedule
	if rule.Spec.Schedule != nil {
		params.Schedule = buildSchedule(rule.Spec.Schedule)
	}

	// Build expiration
	if rule.Spec.Expiration != nil {
		params.Expiration = buildExpiration(rule.Spec.Expiration)
	}

	var result *cf.GatewayRuleResult
	var err error

	if rule.Status.RuleID == "" {
		// Create new gateway rule
		logger.Info("Creating Gateway Rule", "name", params.Name, "action", params.Action)
		r.Recorder.Event(rule, corev1.EventTypeNormal, "Creating",
			fmt.Sprintf("Creating Gateway Rule '%s' (action: %s) in Cloudflare", params.Name, params.Action))
		result, err = apiClient.CreateGatewayRule(params)
		if err != nil {
			r.Recorder.Event(rule, corev1.EventTypeWarning, controller.EventReasonCreateFailed,
				fmt.Sprintf("Failed to create Gateway Rule: %s", cf.SanitizeErrorMessage(err)))
			return r.updateStatusError(ctx, rule, err)
		}
		r.Recorder.Event(rule, corev1.EventTypeNormal, controller.EventReasonCreated,
			fmt.Sprintf("Created Gateway Rule with ID '%s'", result.ID))
	} else {
		// Update existing gateway rule
		logger.Info("Updating Gateway Rule", "ruleId", rule.Status.RuleID)
		r.Recorder.Event(rule, corev1.EventTypeNormal, "Updating",
			fmt.Sprintf("Updating Gateway Rule '%s' in Cloudflare", rule.Status.RuleID))
		result, err = apiClient.UpdateGatewayRule(rule.Status.RuleID, params)
		if err != nil {
			r.Recorder.Event(rule, corev1.EventTypeWarning, controller.EventReasonUpdateFailed,
				fmt.Sprintf("Failed to update Gateway Rule: %s", cf.SanitizeErrorMessage(err)))
			return r.updateStatusError(ctx, rule, err)
		}
		r.Recorder.Event(rule, corev1.EventTypeNormal, controller.EventReasonUpdated,
			fmt.Sprintf("Updated Gateway Rule '%s'", result.ID))
	}

	// Update status
	return r.updateStatusSuccess(ctx, rule, result)
}

// buildRuleSettings converts CRD rule settings to API params.
//
//nolint:revive // cognitive complexity is acceptable for this conversion
func buildRuleSettings(settings *networkingv1alpha2.GatewayRuleSettings) *cf.GatewayRuleSettingsParams {
	if settings == nil {
		return nil
	}

	result := &cf.GatewayRuleSettingsParams{
		BlockPageEnabled:                settings.BlockPageEnabled,
		BlockReason:                     settings.BlockReason,
		OverrideIPs:                     settings.OverrideIPs,
		OverrideHost:                    settings.OverrideHost,
		InsecureDisableDNSSECValidation: settings.InsecureDisableDNSSECValidation,
		UntrustedCertAction:             settings.UntrustedCertificateAction,
		ResolveDNSThroughCloudflare:     settings.ResolveDNSThroughCloudflare,
		AllowChildBypass:                settings.AllowChildBypass,
		BypassParentRule:                settings.BypassParentRule,
		IgnoreCNAMECategoryMatches:      settings.IgnoreCNAMECategoryMatches,
		IPCategories:                    settings.IPCategories,
		IPIndicatorFeeds:                settings.IPIndicatorFeeds,
	}

	if settings.L4Override != nil {
		result.L4Override = &cf.GatewayL4OverrideParams{
			IP:   settings.L4Override.IP,
			Port: settings.L4Override.Port,
		}
	}
	if settings.BISOAdminControls != nil {
		result.BISOAdminControls = &cf.GatewayBISOAdminControlsParams{
			DisablePrinting:             settings.BISOAdminControls.DisablePrinting,
			DisableCopyPaste:            settings.BISOAdminControls.DisableCopyPaste,
			DisableDownload:             settings.BISOAdminControls.DisableDownload,
			DisableUpload:               settings.BISOAdminControls.DisableUpload,
			DisableKeyboard:             settings.BISOAdminControls.DisableKeyboard,
			DisableClipboardRedirection: settings.BISOAdminControls.DisableClipboardRedirection,
		}
	}
	if settings.CheckSession != nil {
		result.CheckSession = &cf.GatewayCheckSessionParams{
			Enforce:  settings.CheckSession.Enforce,
			Duration: settings.CheckSession.Duration,
		}
	}
	if settings.AddHeaders != nil {
		result.AddHeaders = settings.AddHeaders
	}
	if settings.Egress != nil {
		result.Egress = &cf.GatewayEgressParams{
			IPv4:         settings.Egress.IPv4,
			IPv6:         settings.Egress.IPv6,
			IPv4Fallback: settings.Egress.IPv4Fallback,
		}
	}
	if settings.PayloadLog != nil {
		result.PayloadLog = &cf.GatewayPayloadLogParams{
			Enabled: settings.PayloadLog.Enabled,
		}
	}
	if settings.AuditSSH != nil {
		result.AuditSSH = &cf.GatewayAuditSSHParams{
			CommandLogging: settings.AuditSSH.CommandLogging,
		}
	}
	if settings.ResolveDNSInternally != nil {
		fallback := ""
		if settings.ResolveDNSInternally.Fallback != nil && *settings.ResolveDNSInternally.Fallback {
			fallback = "public_dns"
		}
		result.ResolveDNSInternally = &cf.GatewayResolveDNSInternallyParams{
			ViewID:   settings.ResolveDNSInternally.ViewID,
			Fallback: fallback,
		}
	}
	if settings.DNSResolvers != nil {
		result.DNSResolvers = &cf.GatewayDNSResolversParams{}
		if len(settings.DNSResolvers.IPv4) > 0 {
			result.DNSResolvers.IPv4 = make([]cf.GatewayDNSResolverEntryParams, 0, len(settings.DNSResolvers.IPv4))
			for _, r := range settings.DNSResolvers.IPv4 {
				result.DNSResolvers.IPv4 = append(result.DNSResolvers.IPv4, cf.GatewayDNSResolverEntryParams{
					IP:                         r.IP,
					Port:                       r.Port,
					VNetID:                     r.VNetID,
					RouteThroughPrivateNetwork: r.RouteThroughPrivateNetwork,
				})
			}
		}
		if len(settings.DNSResolvers.IPv6) > 0 {
			result.DNSResolvers.IPv6 = make([]cf.GatewayDNSResolverEntryParams, 0, len(settings.DNSResolvers.IPv6))
			for _, r := range settings.DNSResolvers.IPv6 {
				result.DNSResolvers.IPv6 = append(result.DNSResolvers.IPv6, cf.GatewayDNSResolverEntryParams{
					IP:                         r.IP,
					Port:                       r.Port,
					VNetID:                     r.VNetID,
					RouteThroughPrivateNetwork: r.RouteThroughPrivateNetwork,
				})
			}
		}
	}
	if settings.NotificationSettings != nil {
		result.NotificationSettings = &cf.GatewayNotificationSettingsParams{
			Enabled:    settings.NotificationSettings.Enabled,
			Message:    settings.NotificationSettings.Message,
			SupportURL: settings.NotificationSettings.SupportURL,
		}
	}
	if settings.Quarantine != nil {
		result.Quarantine = &cf.GatewayQuarantineParams{
			FileTypes: settings.Quarantine.FileTypes,
		}
	}

	return result
}

func buildSchedule(schedule *networkingv1alpha2.GatewayRuleSchedule) *cf.GatewayRuleScheduleParams {
	if schedule == nil {
		return nil
	}
	return &cf.GatewayRuleScheduleParams{
		TimeZone: schedule.TimeZone,
		Mon:      schedule.Mon,
		Tue:      schedule.Tue,
		Wed:      schedule.Wed,
		Thu:      schedule.Thu,
		Fri:      schedule.Fri,
		Sat:      schedule.Sat,
		Sun:      schedule.Sun,
	}
}

func buildExpiration(expiration *networkingv1alpha2.GatewayRuleExpiration) *cf.GatewayRuleExpirationParams {
	if expiration == nil {
		return nil
	}
	return &cf.GatewayRuleExpirationParams{
		ExpiresAt: expiration.ExpiresAt,
		Duration:  expiration.Duration,
	}
}

func (r *GatewayRuleReconciler) updateStatusError(ctx context.Context, rule *networkingv1alpha2.GatewayRule, err error) (ctrl.Result, error) {
	// P0 FIX: Use UpdateStatusWithConflictRetry for status updates
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, rule, func() {
		rule.Status.State = "Error"
		meta.SetStatusCondition(&rule.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: rule.Generation,
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		rule.Status.ObservedGeneration = rule.Generation
	})
	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GatewayRuleReconciler) updateStatusSuccess(ctx context.Context, rule *networkingv1alpha2.GatewayRule, result *cf.GatewayRuleResult) (ctrl.Result, error) {
	// P0 FIX: Use UpdateStatusWithConflictRetry for status updates
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, rule, func() {
		rule.Status.RuleID = result.ID
		rule.Status.State = "Ready"
		meta.SetStatusCondition(&rule.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: rule.Generation,
			Reason:             "Reconciled",
			Message:            "Gateway Rule successfully reconciled",
			LastTransitionTime: metav1.Now(),
		})
		rule.Status.ObservedGeneration = rule.Generation
	})
	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *GatewayRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("gatewayrule-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.GatewayRule{}).
		Complete(r)
}
