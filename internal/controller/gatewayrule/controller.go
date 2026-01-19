// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package gatewayrule implements the controller for GatewayRule resources.
package gatewayrule

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/service"
	gatewaysvc "github.com/StringKe/cloudflare-operator/internal/service/gateway"
)

const (
	FinalizerName = "gatewayrule.networking.cloudflare-operator.io/finalizer"
)

// GatewayRuleReconciler reconciles a GatewayRule object
type GatewayRuleReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Recorder       record.EventRecorder
	gatewayService *gatewaysvc.GatewayRuleService

	// Runtime state
	ctx  context.Context
	log  logr.Logger
	rule *networkingv1alpha2.GatewayRule
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayrules/finalizers,verbs=update

func (r *GatewayRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrllog.FromContext(ctx)

	// Fetch the GatewayRule instance
	r.rule = &networkingv1alpha2.GatewayRule{}
	if err := r.Get(ctx, req.NamespacedName, r.rule); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Resolve credentials
	credInfo, err := r.resolveCredentials()
	if err != nil {
		r.log.Error(err, "Failed to resolve credentials")
		return r.updateStatusError(err)
	}

	// Handle deletion
	if !r.rule.DeletionTimestamp.IsZero() {
		return r.handleDeletion(credInfo)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.rule, FinalizerName) {
		controllerutil.AddFinalizer(r.rule, FinalizerName)
		if err := r.Update(ctx, r.rule); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Register Gateway rule configuration to SyncState
	return r.registerGatewayRule(credInfo)
}

// resolveCredentials resolves the credentials reference and returns the credentials info.
// Following Unified Sync Architecture, the Resource Controller only needs
// credential metadata (accountID, credRef) - it does not create a Cloudflare API client.
func (r *GatewayRuleReconciler) resolveCredentials() (*controller.CredentialsInfo, error) {
	// GatewayRule is cluster-scoped, use operator namespace for legacy inline secrets
	info, err := controller.ResolveCredentialsForService(
		r.ctx,
		r.Client,
		r.log,
		r.rule.Spec.Cloudflare,
		controller.OperatorNamespace,
		r.rule.Status.AccountID,
	)
	if err != nil {
		r.log.Error(err, "failed to resolve credentials")
		r.Recorder.Event(r.rule, corev1.EventTypeWarning, controller.EventReasonAPIError,
			"Failed to resolve credentials: "+err.Error())
		return nil, err
	}

	return info, nil
}

// handleDeletion handles the deletion of a GatewayRule.
// Following Unified Sync Architecture: Resource Controller only unregisters from SyncState,
// the L5 Sync Controller handles actual Cloudflare API deletion.
func (r *GatewayRuleReconciler) handleDeletion(
	_ *controller.CredentialsInfo,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.rule, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Unregister from SyncState - L5 Sync Controller will handle Cloudflare deletion
	source := service.Source{
		Kind:      "GatewayRule",
		Namespace: "",
		Name:      r.rule.Name,
	}
	if err := r.gatewayService.Unregister(r.ctx, r.rule.Status.RuleID, source); err != nil {
		r.log.Error(err, "Failed to unregister Gateway rule from SyncState")
		r.Recorder.Event(r.rule, corev1.EventTypeWarning, "UnregisterFailed",
			fmt.Sprintf("Failed to unregister from SyncState: %s", err.Error()))
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	r.log.Info("Unregistered Gateway rule from SyncState, L5 Sync Controller will handle Cloudflare deletion")
	r.Recorder.Event(r.rule, corev1.EventTypeNormal, "Unregistered", "Unregistered from SyncState")

	// Remove finalizer with retry logic
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.rule, func() {
		controllerutil.RemoveFinalizer(r.rule, FinalizerName)
	}); err != nil {
		r.log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(r.rule, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// registerGatewayRule registers the Gateway rule configuration to SyncState.
func (r *GatewayRuleReconciler) registerGatewayRule(
	credInfo *controller.CredentialsInfo,
) (ctrl.Result, error) {
	// Use the Name from the spec, or fall back to the resource name
	ruleName := r.rule.Spec.Name
	if ruleName == "" {
		ruleName = r.rule.Name
	}

	// Build Gateway rule configuration
	config := gatewaysvc.GatewayRuleConfig{
		Name:          ruleName,
		Description:   r.rule.Spec.Description,
		TrafficType:   "", // Determined by filters
		Action:        r.rule.Spec.Action,
		Enabled:       r.rule.Spec.Enabled,
		Priority:      r.rule.Spec.Precedence,
		Identity:      r.rule.Spec.Identity,
		DevicePosture: r.rule.Spec.DevicePosture,
	}

	// Build filters
	for _, f := range r.rule.Spec.Filters {
		config.Filters = append(config.Filters, gatewaysvc.GatewayRuleFilter{
			Type:       f,
			Expression: r.rule.Spec.Traffic,
		})
	}

	// Build schedule
	if r.rule.Spec.Schedule != nil {
		config.Schedule = &gatewaysvc.GatewayRuleSchedule{
			TimeZone: r.rule.Spec.Schedule.TimeZone,
			Mon:      r.rule.Spec.Schedule.Mon,
			Tue:      r.rule.Spec.Schedule.Tue,
			Wed:      r.rule.Spec.Schedule.Wed,
			Thu:      r.rule.Spec.Schedule.Thu,
			Fri:      r.rule.Spec.Schedule.Fri,
			Sat:      r.rule.Spec.Schedule.Sat,
			Sun:      r.rule.Spec.Schedule.Sun,
		}
	}

	// Build expiration
	if r.rule.Spec.Expiration != nil {
		config.Expiration = &gatewaysvc.GatewayRuleExpiration{
			ExpiresAt: r.rule.Spec.Expiration.ExpiresAt,
			Duration:  r.rule.Spec.Expiration.Duration,
		}
	}

	// Build rule settings
	if r.rule.Spec.RuleSettings != nil {
		config.RuleSettings = r.buildRuleSettings(r.rule.Spec.RuleSettings)
	}

	// Create source reference
	source := service.Source{
		Kind:      "GatewayRule",
		Namespace: "",
		Name:      r.rule.Name,
	}

	// Register to SyncState
	opts := gatewaysvc.GatewayRuleRegisterOptions{
		AccountID:      credInfo.AccountID,
		RuleID:         r.rule.Status.RuleID,
		Source:         source,
		Config:         config,
		CredentialsRef: credInfo.CredentialsRef,
	}

	if err := r.gatewayService.Register(r.ctx, opts); err != nil {
		r.log.Error(err, "Failed to register Gateway rule configuration")
		r.Recorder.Event(r.rule, corev1.EventTypeWarning, "RegisterFailed",
			fmt.Sprintf("Failed to register Gateway rule: %s", err.Error()))
		return r.updateStatusError(err)
	}

	r.Recorder.Event(r.rule, corev1.EventTypeNormal, "Registered",
		fmt.Sprintf("Registered Gateway Rule '%s' configuration to SyncState", ruleName))

	// Update status to Pending - actual sync happens via GatewaySyncController
	return r.updateStatusPending(credInfo.AccountID)
}

// buildRuleSettings converts CRD rule settings to service layer type.
//
//nolint:revive // cognitive complexity is acceptable for this conversion
func (*GatewayRuleReconciler) buildRuleSettings(settings *networkingv1alpha2.GatewayRuleSettings) *gatewaysvc.GatewayRuleSettings {
	if settings == nil {
		return nil
	}

	result := &gatewaysvc.GatewayRuleSettings{
		BlockPageEnabled:                settings.BlockPageEnabled,
		BlockReason:                     settings.BlockReason,
		OverrideHost:                    settings.OverrideHost,
		OverrideIPs:                     settings.OverrideIPs,
		InsecureDisableDNSSECValidation: settings.InsecureDisableDNSSECValidation,
		AddHeaders:                      settings.AddHeaders,
		UntrustedCertificateAction:      settings.UntrustedCertificateAction,
		ResolveDNSThroughCloudflare:     settings.ResolveDNSThroughCloudflare,
		AllowChildBypass:                settings.AllowChildBypass,
		BypassParentRule:                settings.BypassParentRule,
		IgnoreCNAMECategoryMatches:      settings.IgnoreCNAMECategoryMatches,
		IPCategories:                    settings.IPCategories,
		IPIndicatorFeeds:                settings.IPIndicatorFeeds,
	}

	if settings.L4Override != nil {
		result.L4Override = &gatewaysvc.L4OverrideSettings{
			IP:   settings.L4Override.IP,
			Port: settings.L4Override.Port,
		}
	}

	if settings.BISOAdminControls != nil {
		result.BISOAdminControls = &gatewaysvc.BISOAdminControls{
			DisablePrinting:          settings.BISOAdminControls.DisablePrinting,
			DisableCopyPaste:         settings.BISOAdminControls.DisableCopyPaste,
			DisableDownload:          settings.BISOAdminControls.DisableDownload,
			DisableUpload:            settings.BISOAdminControls.DisableUpload,
			DisableKeyboard:          settings.BISOAdminControls.DisableKeyboard,
			DisableClipboardRedirect: settings.BISOAdminControls.DisableClipboardRedirection,
		}
	}

	if settings.CheckSession != nil {
		result.CheckSession = &gatewaysvc.CheckSessionSettings{
			Enforce:  settings.CheckSession.Enforce,
			Duration: settings.CheckSession.Duration,
		}
	}

	if settings.Egress != nil {
		result.Egress = &gatewaysvc.EgressSettings{
			Ipv4:         settings.Egress.IPv4,
			Ipv6:         settings.Egress.IPv6,
			Ipv4Fallback: settings.Egress.IPv4Fallback,
		}
	}

	if settings.PayloadLog != nil {
		result.PayloadLog = &gatewaysvc.PayloadLogSettings{
			Enabled: settings.PayloadLog.Enabled,
		}
	}

	if settings.AuditSSH != nil {
		result.AuditSSH = &gatewaysvc.AuditSSHSettings{
			CommandLogging: settings.AuditSSH.CommandLogging,
		}
	}

	if settings.NotificationSettings != nil {
		result.NotificationSettings = &gatewaysvc.NotificationSettings{
			Enabled:    settings.NotificationSettings.Enabled,
			Message:    settings.NotificationSettings.Message,
			SupportURL: settings.NotificationSettings.SupportURL,
		}
	}

	if settings.DNSResolvers != nil {
		result.DNSResolvers = &gatewaysvc.DNSResolverSettings{}
		for _, resolver := range settings.DNSResolvers.IPv4 {
			result.DNSResolvers.Ipv4 = append(result.DNSResolvers.Ipv4, gatewaysvc.DNSResolverAddress{
				IP:                         resolver.IP,
				Port:                       resolver.Port,
				VNetID:                     resolver.VNetID,
				RouteThroughPrivateNetwork: resolver.RouteThroughPrivateNetwork,
			})
		}
		for _, resolver := range settings.DNSResolvers.IPv6 {
			result.DNSResolvers.Ipv6 = append(result.DNSResolvers.Ipv6, gatewaysvc.DNSResolverAddress{
				IP:                         resolver.IP,
				Port:                       resolver.Port,
				VNetID:                     resolver.VNetID,
				RouteThroughPrivateNetwork: resolver.RouteThroughPrivateNetwork,
			})
		}
	}

	if settings.ResolveDNSInternally != nil {
		result.ResolveDNSInternally = &gatewaysvc.ResolveDNSInternallySettings{
			ViewID: settings.ResolveDNSInternally.ViewID,
		}
		if settings.ResolveDNSInternally.Fallback != nil {
			if *settings.ResolveDNSInternally.Fallback {
				result.ResolveDNSInternally.Fallback = "public_dns"
			} else {
				result.ResolveDNSInternally.Fallback = "none"
			}
		}
	}

	if settings.Quarantine != nil {
		result.Quarantine = &gatewaysvc.QuarantineSettings{
			FileTypes: settings.Quarantine.FileTypes,
		}
	}

	return result
}

func (r *GatewayRuleReconciler) updateStatusError(
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.rule, func() {
		r.rule.Status.State = "Error"
		meta.SetStatusCondition(&r.rule.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: r.rule.Generation,
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		r.rule.Status.ObservedGeneration = r.rule.Generation
	})
	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GatewayRuleReconciler) updateStatusPending(
	accountID string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.rule, func() {
		if r.rule.Status.AccountID == "" {
			r.rule.Status.AccountID = accountID
		}
		r.rule.Status.State = "Pending"
		meta.SetStatusCondition(&r.rule.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: r.rule.Generation,
			Reason:             "Pending",
			Message:            "Gateway rule configuration registered, waiting for sync",
			LastTransitionTime: metav1.Now(),
		})
		r.rule.Status.ObservedGeneration = r.rule.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	// Requeue to check for sync completion
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *GatewayRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("gatewayrule-controller")

	// Initialize GatewayRuleService
	r.gatewayService = gatewaysvc.NewGatewayRuleService(mgr.GetClient())

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.GatewayRule{}).
		Complete(r)
}
