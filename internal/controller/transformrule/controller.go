// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package transformrule provides a controller for managing Cloudflare Transform Rules.
package transformrule

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	rulesetsvc "github.com/StringKe/cloudflare-operator/internal/service/ruleset"
)

const (
	finalizerName = "cloudflare.com/transform-rule-finalizer"
)

// Reconciler reconciles a TransformRule object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Services
	transformRuleService *rulesetsvc.TransformRuleService
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=transformrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=transformrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=transformrules/finalizers,verbs=update

// Reconcile handles TransformRule reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the TransformRule resource
	rule := &networkingv1alpha2.TransformRule{}
	if err := r.Get(ctx, req.NamespacedName, rule); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch TransformRule")
		return ctrl.Result{}, err
	}

	// Resolve credentials and zone ID
	creds, err := r.resolveCredentials(ctx, rule)
	if err != nil {
		logger.Error(err, "Failed to resolve credentials")
		return r.updateStatusError(ctx, rule, err)
	}
	credRef := networkingv1alpha2.CredentialsReference{Name: creds.CredentialsName}

	// Handle deletion
	if !rule.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, rule, credRef)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(rule, finalizerName) {
		controllerutil.AddFinalizer(rule, finalizerName)
		if err := r.Update(ctx, rule); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Register TransformRule configuration to SyncState
	return r.registerRule(ctx, rule, creds.AccountID, creds.ZoneID, credRef)
}

// resolveCredentials resolves the credentials reference, account ID and zone ID.
func (r *Reconciler) resolveCredentials(
	ctx context.Context,
	rule *networkingv1alpha2.TransformRule,
) (controller.CredentialsResult, error) {
	logger := log.FromContext(ctx)

	var result controller.CredentialsResult

	// Get credentials reference
	if rule.Spec.CredentialsRef != nil {
		result.CredentialsName = rule.Spec.CredentialsRef.Name
	}

	// Get account ID and zone ID from credentials
	cfCredRef := &networkingv1alpha2.CloudflareCredentialsRef{Name: result.CredentialsName}
	apiClient, err := cf.NewAPIClientFromCredentialsRef(ctx, r.Client, cfCredRef)
	if err != nil {
		return result, fmt.Errorf("failed to create API client: %w", err)
	}

	result.AccountID = apiClient.AccountId
	if result.AccountID == "" {
		logger.V(1).Info("Account ID not available from credentials, will be resolved during sync")
	}

	// Get zone ID
	apiClient.Domain = rule.Spec.Zone
	zoneID, err := apiClient.GetZoneId()
	if err != nil {
		return result, fmt.Errorf("failed to get zone ID for %s: %w", rule.Spec.Zone, err)
	}
	result.ZoneID = zoneID

	return result, nil
}

// handleDeletion handles the deletion of TransformRule
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	rule *networkingv1alpha2.TransformRule,
	credRef networkingv1alpha2.CredentialsReference,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(rule, finalizerName) {
		return ctrl.Result{}, nil
	}

	// Clear rules from Cloudflare
	if r.clearRulesFromCloudflare(ctx, rule, credRef) {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Unregister from SyncState
	source := service.Source{
		Kind:      "TransformRule",
		Namespace: rule.Namespace,
		Name:      rule.Name,
	}
	if err := r.transformRuleService.Unregister(ctx, rule.Status.RulesetID, source); err != nil {
		logger.Error(err, "Failed to unregister TransformRule from SyncState")
		// Non-fatal - continue with finalizer removal
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, rule, func() {
		controllerutil.RemoveFinalizer(rule, finalizerName)
	}); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(rule, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")
	return ctrl.Result{}, nil
}

// clearRulesFromCloudflare clears the transform rules from Cloudflare.
// Returns true if reconciliation should be requeued.
func (r *Reconciler) clearRulesFromCloudflare(
	ctx context.Context,
	rule *networkingv1alpha2.TransformRule,
	credRef networkingv1alpha2.CredentialsReference,
) bool {
	if rule.Status.ZoneID == "" || rule.Status.RulesetID == "" {
		return false
	}

	logger := log.FromContext(ctx)
	cfCredRef := &networkingv1alpha2.CloudflareCredentialsRef{Name: credRef.Name}
	cfAPI, err := cf.NewAPIClientFromCredentialsRef(ctx, r.Client, cfCredRef)
	if err != nil {
		return false // Skip if can't create client
	}

	phase := r.getPhase(rule)
	_, err = cfAPI.UpdateEntrypointRuleset(ctx, rule.Status.ZoneID, phase, "", nil)
	if err == nil {
		logger.Info("Transform rules cleared", "phase", phase)
		r.Recorder.Event(rule, corev1.EventTypeNormal, "Deleted",
			fmt.Sprintf("Transform rules cleared for phase %s", phase))
		return false
	}

	if cf.IsNotFoundError(err) {
		return false
	}

	logger.Error(err, "Failed to clear transform rules")
	r.Recorder.Event(rule, corev1.EventTypeWarning, "DeleteFailed", cf.SanitizeErrorMessage(err))
	return true
}

// registerRule registers the TransformRule configuration to SyncState.
// The actual sync to Cloudflare is handled by TransformRuleSyncController.
func (r *Reconciler) registerRule(
	ctx context.Context,
	rule *networkingv1alpha2.TransformRule,
	accountID, zoneID string,
	credRef networkingv1alpha2.CredentialsReference,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Build rules configuration
	rules := make([]rulesetsvc.TransformRuleDefinitionConfig, len(rule.Spec.Rules))
	for i, ruleSpec := range rule.Spec.Rules {
		rules[i] = rulesetsvc.TransformRuleDefinitionConfig{
			Name:       ruleSpec.Name,
			Expression: ruleSpec.Expression,
			Enabled:    ruleSpec.Enabled,
			URLRewrite: ruleSpec.URLRewrite,
			Headers:    ruleSpec.Headers,
		}
	}

	// Build transform rule configuration
	config := rulesetsvc.TransformRuleConfig{
		Zone:        rule.Spec.Zone,
		Type:        string(rule.Spec.Type),
		Description: rule.Spec.Description,
		Rules:       rules,
	}

	// Create source reference
	source := service.Source{
		Kind:      "TransformRule",
		Namespace: rule.Namespace,
		Name:      rule.Name,
	}

	// Register to SyncState
	opts := rulesetsvc.TransformRuleRegisterOptions{
		AccountID:      accountID,
		ZoneID:         zoneID,
		RulesetID:      rule.Status.RulesetID, // May be empty for new rulesets
		Source:         source,
		Config:         config,
		CredentialsRef: credRef,
	}

	if err := r.transformRuleService.Register(ctx, opts); err != nil {
		logger.Error(err, "Failed to register TransformRule configuration")
		r.Recorder.Event(rule, corev1.EventTypeWarning, "RegisterFailed",
			fmt.Sprintf("Failed to register TransformRule: %s", err.Error()))
		return r.updateStatusError(ctx, rule, err)
	}

	r.Recorder.Event(rule, corev1.EventTypeNormal, "Registered",
		fmt.Sprintf("Registered TransformRule for zone '%s' type '%s' to SyncState",
			rule.Spec.Zone, rule.Spec.Type))

	// Update status to Pending - actual sync happens via TransformRuleSyncController
	return r.updateStatusPending(ctx, rule, zoneID)
}

// getPhase returns the Cloudflare ruleset phase based on rule type
func (*Reconciler) getPhase(rule *networkingv1alpha2.TransformRule) string {
	switch rule.Spec.Type {
	case networkingv1alpha2.TransformRuleTypeRequestHeader:
		return "http_request_late_transform"
	case networkingv1alpha2.TransformRuleTypeResponseHeader:
		return "http_response_headers_transform"
	default: // includes TransformRuleTypeURLRewrite
		return "http_request_transform"
	}
}

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	rule *networkingv1alpha2.TransformRule,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, rule, func() {
		rule.Status.State = networkingv1alpha2.TransformRuleStateError
		rule.Status.Message = cf.SanitizeErrorMessage(err)
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

func (r *Reconciler) updateStatusPending(
	ctx context.Context,
	rule *networkingv1alpha2.TransformRule,
	zoneID string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, rule, func() {
		rule.Status.State = networkingv1alpha2.TransformRuleStateSyncing
		rule.Status.Message = "TransformRule configuration registered, waiting for sync"
		rule.Status.ZoneID = zoneID
		meta.SetStatusCondition(&rule.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: rule.Generation,
			Reason:             "Pending",
			Message:            "TransformRule configuration registered, waiting for sync",
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

// findRulesForCredentials returns TransformRules that reference the given credentials
//
//nolint:revive // cognitive complexity is acceptable for watch handler
func (r *Reconciler) findRulesForCredentials(
	ctx context.Context, obj client.Object,
) []reconcile.Request {
	creds, ok := obj.(*networkingv1alpha2.CloudflareCredentials)
	if !ok {
		return nil
	}

	ruleList := &networkingv1alpha2.TransformRuleList{}
	if err := r.List(ctx, ruleList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, rule := range ruleList.Items {
		if rule.Spec.CredentialsRef != nil && rule.Spec.CredentialsRef.Name == creds.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      rule.Name,
					Namespace: rule.Namespace,
				},
			})
		}

		if creds.Spec.IsDefault && rule.Spec.CredentialsRef == nil {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      rule.Name,
					Namespace: rule.Namespace,
				},
			})
		}
	}

	return requests
}

// findRulesForSyncState returns TransformRules that are sources for the given SyncState
func (*Reconciler) findRulesForSyncState(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	syncState, ok := obj.(*networkingv1alpha2.CloudflareSyncState)
	if !ok {
		return nil
	}

	// Only process TransformRule type SyncStates
	if syncState.Spec.ResourceType != networkingv1alpha2.SyncResourceTransformRule {
		return nil
	}

	var requests []reconcile.Request
	for _, source := range syncState.Spec.Sources {
		if source.Ref.Kind == "TransformRule" {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      source.Ref.Name,
					Namespace: source.Ref.Namespace,
				},
			})
		}
	}

	logger.V(1).Info("Found TransformRules for SyncState update",
		"syncState", syncState.Name,
		"ruleCount", len(requests))

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("transformrule-controller")

	// Initialize TransformRuleService
	r.transformRuleService = rulesetsvc.NewTransformRuleService(mgr.GetClient())

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.TransformRule{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findRulesForCredentials)).
		Watches(&networkingv1alpha2.CloudflareSyncState{},
			handler.EnqueueRequestsFromMapFunc(r.findRulesForSyncState)).
		Named("transformrule").
		Complete(r)
}
