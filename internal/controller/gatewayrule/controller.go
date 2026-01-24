// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package gatewayrule provides a controller for managing Cloudflare Gateway Rules.
// It directly calls Cloudflare API and writes status back to the CRD.
package gatewayrule

import (
	"context"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	"github.com/StringKe/cloudflare-operator/internal/controller/common"
)

const (
	finalizerName = "gatewayrule.networking.cloudflare-operator.io/finalizer"
)

// Reconciler reconciles a GatewayRule object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayrules/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile handles GatewayRule reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the GatewayRule resource
	rule := &networkingv1alpha2.GatewayRule{}
	if err := r.Get(ctx, req.NamespacedName, rule); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch GatewayRule")
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !rule.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, rule)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, rule, finalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Get API client
	// GatewayRule is cluster-scoped, use operator namespace for legacy inline secrets
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &rule.Spec.Cloudflare,
		Namespace:         common.OperatorNamespace,
		StatusAccountID:   rule.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, rule, err)
	}

	// Sync Gateway rule to Cloudflare
	return r.syncGatewayRule(ctx, rule, apiResult)
}

// handleDeletion handles the deletion of GatewayRule.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	rule *networkingv1alpha2.GatewayRule,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(rule, finalizerName) {
		return common.NoRequeue(), nil
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &rule.Spec.Cloudflare,
		Namespace:         common.OperatorNamespace,
		StatusAccountID:   rule.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client for deletion")
		// Continue with finalizer removal
	} else if rule.Status.RuleID != "" {
		// Delete Gateway rule from Cloudflare
		logger.Info("Deleting Gateway Rule from Cloudflare",
			"ruleId", rule.Status.RuleID)

		if err := apiResult.API.DeleteGatewayRule(ctx, rule.Status.RuleID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete Gateway Rule from Cloudflare")
				r.Recorder.Event(rule, corev1.EventTypeWarning, "DeleteFailed",
					fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
				return common.RequeueShort(), err
			}
			logger.Info("Gateway Rule not found in Cloudflare, may have been already deleted")
		}

		r.Recorder.Event(rule, corev1.EventTypeNormal, "Deleted",
			"Gateway Rule deleted from Cloudflare")
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, rule, func() {
		controllerutil.RemoveFinalizer(rule, finalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(rule, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// syncGatewayRule syncs the Gateway Rule to Cloudflare.
func (r *Reconciler) syncGatewayRule(
	ctx context.Context,
	rule *networkingv1alpha2.GatewayRule,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Determine rule name
	ruleName := rule.GetGatewayRuleName()

	// Build params
	params := r.buildParams(rule, ruleName)

	// Check if rule already exists by ID
	if rule.Status.RuleID != "" {
		existing, err := apiResult.API.GetGatewayRule(ctx, rule.Status.RuleID)
		if err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to get Gateway Rule from Cloudflare")
				return r.updateStatusError(ctx, rule, err)
			}
			// Rule doesn't exist, will create
			logger.Info("Gateway Rule not found in Cloudflare, will recreate",
				"ruleId", rule.Status.RuleID)
		} else {
			// Rule exists, update it
			logger.V(1).Info("Updating Gateway Rule in Cloudflare",
				"ruleId", existing.ID,
				"name", ruleName)

			result, err := apiResult.API.UpdateGatewayRule(ctx, existing.ID, params)
			if err != nil {
				logger.Error(err, "Failed to update Gateway Rule")
				return r.updateStatusError(ctx, rule, err)
			}

			r.Recorder.Event(rule, corev1.EventTypeNormal, "Updated",
				fmt.Sprintf("Gateway Rule '%s' updated in Cloudflare", ruleName))

			return r.updateStatusReady(ctx, rule, apiResult.AccountID, result.ID)
		}
	}

	// Try to find existing rule by name
	existingByName, err := apiResult.API.ListGatewayRulesByName(ctx, ruleName)
	if err != nil && !cf.IsNotFoundError(err) {
		logger.Error(err, "Failed to search for existing Gateway Rule")
		return r.updateStatusError(ctx, rule, err)
	}

	if existingByName != nil {
		// Rule already exists with this name, adopt it
		logger.Info("Gateway Rule already exists with same name, adopting it",
			"ruleId", existingByName.ID,
			"name", ruleName)

		// Update the existing rule
		result, err := apiResult.API.UpdateGatewayRule(ctx, existingByName.ID, params)
		if err != nil {
			logger.Error(err, "Failed to update existing Gateway Rule")
			return r.updateStatusError(ctx, rule, err)
		}

		r.Recorder.Event(rule, corev1.EventTypeNormal, "Adopted",
			fmt.Sprintf("Adopted existing Gateway Rule '%s'", ruleName))

		return r.updateStatusReady(ctx, rule, apiResult.AccountID, result.ID)
	}

	// Create new rule
	logger.Info("Creating Gateway Rule in Cloudflare",
		"name", ruleName)

	result, err := apiResult.API.CreateGatewayRule(ctx, params)
	if err != nil {
		logger.Error(err, "Failed to create Gateway Rule")
		return r.updateStatusError(ctx, rule, err)
	}

	r.Recorder.Event(rule, corev1.EventTypeNormal, "Created",
		fmt.Sprintf("Gateway Rule '%s' created in Cloudflare", ruleName))

	return r.updateStatusReady(ctx, rule, apiResult.AccountID, result.ID)
}

// buildParams builds the GatewayRuleParams from the GatewayRule spec.
func (r *Reconciler) buildParams(rule *networkingv1alpha2.GatewayRule, ruleName string) cf.GatewayRuleParams {
	params := cf.GatewayRuleParams{
		Name:          ruleName,
		Description:   rule.Spec.Description,
		Precedence:    rule.Spec.Precedence,
		Enabled:       rule.Spec.Enabled,
		Action:        rule.Spec.Action,
		Traffic:       rule.Spec.Traffic,
		Identity:      rule.Spec.Identity,
		DevicePosture: rule.Spec.DevicePosture,
	}

	// Convert filters
	for _, f := range rule.Spec.Filters {
		params.Filters = append(params.Filters, cloudflare.TeamsFilterType(f))
	}

	// Convert schedule
	if rule.Spec.Schedule != nil {
		params.Schedule = &cf.GatewayRuleScheduleParams{
			TimeZone: rule.Spec.Schedule.TimeZone,
			Mon:      rule.Spec.Schedule.Mon,
			Tue:      rule.Spec.Schedule.Tue,
			Wed:      rule.Spec.Schedule.Wed,
			Thu:      rule.Spec.Schedule.Thu,
			Fri:      rule.Spec.Schedule.Fri,
			Sat:      rule.Spec.Schedule.Sat,
			Sun:      rule.Spec.Schedule.Sun,
		}
	}

	// Convert expiration
	if rule.Spec.Expiration != nil {
		params.Expiration = &cf.GatewayRuleExpirationParams{
			ExpiresAt: rule.Spec.Expiration.ExpiresAt,
			Duration:  rule.Spec.Expiration.Duration,
		}
	}

	// Convert rule settings
	if rule.Spec.RuleSettings != nil {
		params.RuleSettings = r.buildRuleSettings(rule.Spec.RuleSettings)
	}

	return params
}

// buildRuleSettings converts CRD rule settings to CF API type.
//
//nolint:revive // cognitive complexity is acceptable for this conversion
func (r *Reconciler) buildRuleSettings(settings *networkingv1alpha2.GatewayRuleSettings) *cf.GatewayRuleSettingsParams {
	if settings == nil {
		return nil
	}

	result := &cf.GatewayRuleSettingsParams{
		BlockPageEnabled:                settings.BlockPageEnabled,
		BlockReason:                     settings.BlockReason,
		OverrideHost:                    settings.OverrideHost,
		OverrideIPs:                     settings.OverrideIPs,
		InsecureDisableDNSSECValidation: settings.InsecureDisableDNSSECValidation,
		AddHeaders:                      settings.AddHeaders,
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

	if settings.NotificationSettings != nil {
		result.NotificationSettings = &cf.GatewayNotificationSettingsParams{
			Enabled:    settings.NotificationSettings.Enabled,
			Message:    settings.NotificationSettings.Message,
			SupportURL: settings.NotificationSettings.SupportURL,
		}
	}

	if settings.DNSResolvers != nil {
		result.DNSResolvers = &cf.GatewayDNSResolversParams{}
		for _, resolver := range settings.DNSResolvers.IPv4 {
			result.DNSResolvers.IPv4 = append(result.DNSResolvers.IPv4, cf.GatewayDNSResolverEntryParams{
				IP:                         resolver.IP,
				Port:                       resolver.Port,
				VNetID:                     resolver.VNetID,
				RouteThroughPrivateNetwork: resolver.RouteThroughPrivateNetwork,
			})
		}
		for _, resolver := range settings.DNSResolvers.IPv6 {
			result.DNSResolvers.IPv6 = append(result.DNSResolvers.IPv6, cf.GatewayDNSResolverEntryParams{
				IP:                         resolver.IP,
				Port:                       resolver.Port,
				VNetID:                     resolver.VNetID,
				RouteThroughPrivateNetwork: resolver.RouteThroughPrivateNetwork,
			})
		}
	}

	if settings.ResolveDNSInternally != nil {
		fallback := "none"
		if settings.ResolveDNSInternally.Fallback != nil && *settings.ResolveDNSInternally.Fallback {
			fallback = "public_dns"
		}
		result.ResolveDNSInternally = &cf.GatewayResolveDNSInternallyParams{
			ViewID:   settings.ResolveDNSInternally.ViewID,
			Fallback: fallback,
		}
	}

	if settings.Quarantine != nil {
		result.Quarantine = &cf.GatewayQuarantineParams{
			FileTypes: settings.Quarantine.FileTypes,
		}
	}

	return result
}

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	rule *networkingv1alpha2.GatewayRule,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, rule, func() {
		rule.Status.State = "Error"
		meta.SetStatusCondition(&rule.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: rule.Generation,
			Reason:             "Error",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		rule.Status.ObservedGeneration = rule.Generation
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
}

func (r *Reconciler) updateStatusReady(
	ctx context.Context,
	rule *networkingv1alpha2.GatewayRule,
	accountID, ruleID string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, rule, func() {
		rule.Status.AccountID = accountID
		rule.Status.RuleID = ruleID
		rule.Status.State = "Ready"
		meta.SetStatusCondition(&rule.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: rule.Generation,
			Reason:             "Synced",
			Message:            "Gateway Rule synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		rule.Status.ObservedGeneration = rule.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("gatewayrule-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("gatewayrule"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.GatewayRule{}).
		Named("gatewayrule").
		Complete(r)
}
