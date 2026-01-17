// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package gatewayrule implements the controller for GatewayRule resources.
package gatewayrule

import (
	"context"
	"errors"
	"fmt"
	"time"

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
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayrules/finalizers,verbs=update

func (r *GatewayRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the GatewayRule instance
	rule := &networkingv1alpha2.GatewayRule{}
	if err := r.Get(ctx, req.NamespacedName, rule); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Resolve credentials
	credRef, accountID, err := r.resolveCredentials(ctx, rule)
	if err != nil {
		logger.Error(err, "Failed to resolve credentials")
		return r.updateStatusError(ctx, rule, err)
	}

	// Handle deletion
	if !rule.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, rule, credRef)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(rule, FinalizerName) {
		controllerutil.AddFinalizer(rule, FinalizerName)
		if err := r.Update(ctx, rule); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Register Gateway rule configuration to SyncState
	return r.registerGatewayRule(ctx, rule, accountID, credRef)
}

// resolveCredentials resolves the credentials reference and account ID.
func (r *GatewayRuleReconciler) resolveCredentials(
	ctx context.Context,
	rule *networkingv1alpha2.GatewayRule,
) (credRef networkingv1alpha2.CredentialsReference, accountID string, err error) {
	// Get credentials reference
	if rule.Spec.Cloudflare.CredentialsRef != nil {
		credRef = networkingv1alpha2.CredentialsReference{
			Name: rule.Spec.Cloudflare.CredentialsRef.Name,
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

// handleDeletion handles the deletion of a GatewayRule.
func (r *GatewayRuleReconciler) handleDeletion(
	ctx context.Context,
	rule *networkingv1alpha2.GatewayRule,
	_ networkingv1alpha2.CredentialsReference,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(rule, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Delete from Cloudflare
	if result, shouldReturn := r.deleteFromCloudflare(ctx, rule); shouldReturn {
		return result, nil
	}

	// Unregister from SyncState
	source := service.Source{
		Kind:      "GatewayRule",
		Namespace: "",
		Name:      rule.Name,
	}
	if err := r.gatewayService.Unregister(ctx, rule.Status.RuleID, source); err != nil {
		logger.Error(err, "Failed to unregister Gateway rule from SyncState")
		// Non-fatal - continue with finalizer removal
	}

	// Remove finalizer with retry logic
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, rule, func() {
		controllerutil.RemoveFinalizer(rule, FinalizerName)
	}); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(rule, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// deleteFromCloudflare deletes the gateway rule from Cloudflare.
// Returns (result, shouldReturn) where shouldReturn indicates if caller should return.
func (r *GatewayRuleReconciler) deleteFromCloudflare(
	ctx context.Context,
	rule *networkingv1alpha2.GatewayRule,
) (ctrl.Result, bool) {
	if rule.Status.RuleID == "" {
		return ctrl.Result{}, false
	}

	logger := log.FromContext(ctx)
	apiClient, err := cf.NewAPIClientFromDetails(ctx, r.Client, controller.OperatorNamespace, rule.Spec.Cloudflare)
	if err != nil {
		logger.Error(err, "Failed to create API client for deletion")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, true
	}

	logger.Info("Deleting Gateway Rule from Cloudflare", "ruleId", rule.Status.RuleID)

	err = apiClient.DeleteGatewayRule(rule.Status.RuleID)
	if err == nil {
		r.Recorder.Event(rule, corev1.EventTypeNormal, controller.EventReasonDeleted, "Deleted from Cloudflare")
		return ctrl.Result{}, false
	}

	if cf.IsNotFoundError(err) {
		logger.Info("Gateway Rule already deleted from Cloudflare")
		r.Recorder.Event(rule, corev1.EventTypeNormal, "AlreadyDeleted",
			"Gateway Rule was already deleted from Cloudflare")
		return ctrl.Result{}, false
	}

	logger.Error(err, "Failed to delete Gateway Rule from Cloudflare")
	r.Recorder.Event(rule, corev1.EventTypeWarning, controller.EventReasonDeleteFailed,
		fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
	return ctrl.Result{RequeueAfter: 30 * time.Second}, true
}

// registerGatewayRule registers the Gateway rule configuration to SyncState.
func (r *GatewayRuleReconciler) registerGatewayRule(
	ctx context.Context,
	rule *networkingv1alpha2.GatewayRule,
	accountID string,
	credRef networkingv1alpha2.CredentialsReference,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Use the Name from the spec, or fall back to the resource name
	ruleName := rule.Spec.Name
	if ruleName == "" {
		ruleName = rule.Name
	}

	// Build Gateway rule configuration
	config := gatewaysvc.GatewayRuleConfig{
		Name:        ruleName,
		Description: rule.Spec.Description,
		TrafficType: "", // Determined by filters
		Action:      rule.Spec.Action,
		Enabled:     rule.Spec.Enabled,
		Priority:    rule.Spec.Precedence,
	}

	// Build filters
	for _, f := range rule.Spec.Filters {
		config.Filters = append(config.Filters, gatewaysvc.GatewayRuleFilter{
			Type:       f,
			Expression: rule.Spec.Traffic,
		})
	}

	// Build rule settings
	if rule.Spec.RuleSettings != nil {
		config.RuleSettings = r.buildRuleSettings(rule.Spec.RuleSettings)
	}

	// Create source reference
	source := service.Source{
		Kind:      "GatewayRule",
		Namespace: "",
		Name:      rule.Name,
	}

	// Register to SyncState
	opts := gatewaysvc.GatewayRuleRegisterOptions{
		AccountID:      accountID,
		RuleID:         rule.Status.RuleID,
		Source:         source,
		Config:         config,
		CredentialsRef: credRef,
	}

	if err := r.gatewayService.Register(ctx, opts); err != nil {
		logger.Error(err, "Failed to register Gateway rule configuration")
		r.Recorder.Event(rule, corev1.EventTypeWarning, "RegisterFailed",
			fmt.Sprintf("Failed to register Gateway rule: %s", err.Error()))
		return r.updateStatusError(ctx, rule, err)
	}

	r.Recorder.Event(rule, corev1.EventTypeNormal, "Registered",
		fmt.Sprintf("Registered Gateway Rule '%s' configuration to SyncState", ruleName))

	// Update status to Pending - actual sync happens via GatewaySyncController
	return r.updateStatusPending(ctx, rule, accountID)
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
		for _, r := range settings.DNSResolvers.IPv4 {
			result.DNSResolvers.Ipv4 = append(result.DNSResolvers.Ipv4, gatewaysvc.DNSResolverAddress{
				IP:   r.IP,
				Port: r.Port,
			})
		}
		for _, r := range settings.DNSResolvers.IPv6 {
			result.DNSResolvers.Ipv6 = append(result.DNSResolvers.Ipv6, gatewaysvc.DNSResolverAddress{
				IP:   r.IP,
				Port: r.Port,
			})
		}
	}

	return result
}

func (r *GatewayRuleReconciler) updateStatusError(
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

func (r *GatewayRuleReconciler) updateStatusPending(
	ctx context.Context,
	rule *networkingv1alpha2.GatewayRule,
	accountID string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, rule, func() {
		if rule.Status.AccountID == "" {
			rule.Status.AccountID = accountID
		}
		rule.Status.State = "Pending"
		meta.SetStatusCondition(&rule.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: rule.Generation,
			Reason:             "Pending",
			Message:            "Gateway rule configuration registered, waiting for sync",
			LastTransitionTime: metav1.Now(),
		})
		rule.Status.ObservedGeneration = rule.Generation
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
