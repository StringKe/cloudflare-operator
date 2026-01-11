// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package transformrule provides a controller for managing Cloudflare Transform Rules.
package transformrule

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

const (
	finalizerName = "cloudflare.com/transform-rule-finalizer"
)

// Reconciler reconciles a TransformRule object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Internal state
	ctx  context.Context
	log  logr.Logger
	rule *networkingv1alpha2.TransformRule
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=transformrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=transformrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=transformrules/finalizers,verbs=update

// Reconcile handles TransformRule reconciliation
//
//nolint:revive // cognitive complexity is acceptable for main reconcile loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrl.LoggerFrom(ctx)

	// Get the TransformRule resource
	r.rule = &networkingv1alpha2.TransformRule{}
	if err := r.Get(ctx, req.NamespacedName, r.rule); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch TransformRule")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !r.rule.DeletionTimestamp.IsZero() {
		return r.handleDeletion()
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.rule, finalizerName) {
		controllerutil.AddFinalizer(r.rule, finalizerName)
		if err := r.Update(ctx, r.rule); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Create API client
	cfAPI, err := r.getAPIClient()
	if err != nil {
		r.updateState(networkingv1alpha2.TransformRuleStateError,
			fmt.Sprintf("Failed to get API client: %v", err))
		r.Recorder.Event(r.rule, corev1.EventTypeWarning,
			controller.EventReasonAPIError, cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Get zone ID
	zoneID, err := r.getZoneID(cfAPI)
	if err != nil {
		r.updateState(networkingv1alpha2.TransformRuleStateError,
			fmt.Sprintf("Failed to get zone ID: %v", err))
		r.Recorder.Event(r.rule, corev1.EventTypeWarning,
			controller.EventReasonAPIError, cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Sync rules
	return r.syncRules(cfAPI, zoneID)
}

// handleDeletion handles the deletion of TransformRule
//
//nolint:revive // cognitive complexity is acceptable for deletion handling
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.rule, finalizerName) {
		return ctrl.Result{}, nil
	}

	// Clear rules from the entrypoint ruleset
	if r.rule.Status.ZoneID != "" {
		cfAPI, err := r.getAPIClient()
		if err == nil {
			phase := r.getPhase()
			_, err := cfAPI.UpdateEntrypointRuleset(
				r.ctx,
				r.rule.Status.ZoneID,
				phase,
				"",
				[]cloudflare.RulesetRule{},
			)
			if err != nil && !cf.IsNotFoundError(err) {
				r.log.Error(err, "Failed to clear transform rules")
			} else {
				r.log.Info("Transform rules cleared", "phase", phase)
			}
		}
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.rule, func() {
		controllerutil.RemoveFinalizer(r.rule, finalizerName)
	}); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// syncRules syncs the transform rules to Cloudflare
func (r *Reconciler) syncRules(cfAPI *cf.API, zoneID string) (ctrl.Result, error) {
	r.updateState(networkingv1alpha2.TransformRuleStateSyncing, "Syncing transform rules")

	// Convert rules to Cloudflare format
	rules := r.convertRules()

	// Get the appropriate phase
	phase := r.getPhase()

	// Update entrypoint ruleset
	result, err := cfAPI.UpdateEntrypointRuleset(
		r.ctx,
		zoneID,
		phase,
		r.rule.Spec.Description,
		rules,
	)
	if err != nil {
		r.updateState(networkingv1alpha2.TransformRuleStateError,
			fmt.Sprintf("Failed to update rules: %v", err))
		r.Recorder.Event(r.rule, corev1.EventTypeWarning,
			controller.EventReasonAPIError, cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Update status
	r.rule.Status.RulesetID = result.ID
	r.rule.Status.ZoneID = zoneID
	r.rule.Status.RuleCount = len(result.Rules)

	r.updateState(networkingv1alpha2.TransformRuleStateReady, "Transform rules synced successfully")
	r.Recorder.Event(r.rule, corev1.EventTypeNormal, "Synced",
		fmt.Sprintf("Transform rules synced with %d rules", len(result.Rules)))

	// Requeue periodically to detect drift
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// getPhase returns the Cloudflare ruleset phase based on rule type
func (r *Reconciler) getPhase() string {
	switch r.rule.Spec.Type {
	case networkingv1alpha2.TransformRuleTypeURLRewrite:
		return "http_request_transform"
	case networkingv1alpha2.TransformRuleTypeRequestHeader:
		return "http_request_late_transform"
	case networkingv1alpha2.TransformRuleTypeResponseHeader:
		return "http_response_headers_transform"
	default:
		return "http_request_transform"
	}
}

// convertRules converts TransformRule rules to Cloudflare RulesetRule format
//
//nolint:revive // cognitive complexity is acceptable for rule conversion
func (r *Reconciler) convertRules() []cloudflare.RulesetRule {
	rules := make([]cloudflare.RulesetRule, len(r.rule.Spec.Rules))

	for i, rule := range r.rule.Spec.Rules {
		cfRule := cloudflare.RulesetRule{
			Action:      "rewrite",
			Expression:  rule.Expression,
			Description: rule.Name,
			Enabled:     ptr.To(rule.Enabled),
		}

		// Build action parameters based on rule type
		cfRule.ActionParameters = r.buildActionParameters(rule)

		rules[i] = cfRule
	}

	return rules
}

// buildActionParameters builds action parameters based on rule configuration
//
//nolint:revive // cognitive complexity is acceptable for parameter building
func (r *Reconciler) buildActionParameters(rule networkingv1alpha2.TransformRuleDefinition) *cloudflare.RulesetRuleActionParameters {
	params := &cloudflare.RulesetRuleActionParameters{}

	// URL Rewrite
	if rule.URLRewrite != nil {
		params.URI = &cloudflare.RulesetRuleActionParametersURI{}

		if rule.URLRewrite.Path != nil {
			params.URI.Path = &cloudflare.RulesetRuleActionParametersURIPath{}
			if rule.URLRewrite.Path.Static != "" {
				params.URI.Path.Value = rule.URLRewrite.Path.Static
			}
			if rule.URLRewrite.Path.Expression != "" {
				params.URI.Path.Expression = rule.URLRewrite.Path.Expression
			}
		}

		if rule.URLRewrite.Query != nil {
			params.URI.Query = &cloudflare.RulesetRuleActionParametersURIQuery{}
			if rule.URLRewrite.Query.Static != "" {
				params.URI.Query.Value = ptr.To(rule.URLRewrite.Query.Static)
			}
			if rule.URLRewrite.Query.Expression != "" {
				params.URI.Query.Expression = rule.URLRewrite.Query.Expression
			}
		}
	}

	// Header modifications
	if len(rule.Headers) > 0 {
		params.Headers = make(map[string]cloudflare.RulesetRuleActionParametersHTTPHeader)

		for _, header := range rule.Headers {
			h := cloudflare.RulesetRuleActionParametersHTTPHeader{
				Operation: string(header.Operation),
			}
			if header.Value != "" {
				h.Value = header.Value
			}
			if header.Expression != "" {
				h.Expression = header.Expression
			}
			params.Headers[header.Name] = h
		}
	}

	return params
}

// getZoneID gets the zone ID for the zone name
func (r *Reconciler) getZoneID(cfAPI *cf.API) (string, error) {
	cfAPI.Domain = r.rule.Spec.Zone

	zoneID, err := cfAPI.GetZoneId()
	if err != nil {
		return "", fmt.Errorf("failed to get zone ID for %s: %w", r.rule.Spec.Zone, err)
	}
	return zoneID, nil
}

// getAPIClient creates a Cloudflare API client from credentials
func (r *Reconciler) getAPIClient() (*cf.API, error) {
	if r.rule.Spec.CredentialsRef != nil {
		ref := &networkingv1alpha2.CloudflareCredentialsRef{
			Name: r.rule.Spec.CredentialsRef.Name,
		}
		return cf.NewAPIClientFromCredentialsRef(r.ctx, r.Client, ref)
	}
	return cf.NewAPIClientFromDefaultCredentials(r.ctx, r.Client)
}

// updateState updates the state and status of the TransformRule
func (r *Reconciler) updateState(state networkingv1alpha2.TransformRuleState, message string) {
	r.rule.Status.State = state
	r.rule.Status.Message = message
	r.rule.Status.ObservedGeneration = r.rule.Generation

	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		ObservedGeneration: r.rule.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(state),
		Message:            message,
	}

	if state == networkingv1alpha2.TransformRuleStateReady {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "RulesReady"
	}

	controller.SetCondition(&r.rule.Status.Conditions, condition.Type,
		condition.Status, condition.Reason, condition.Message)

	if err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.rule, func() {
		r.rule.Status.State = state
		r.rule.Status.Message = message
		r.rule.Status.ObservedGeneration = r.rule.Generation
		controller.SetCondition(&r.rule.Status.Conditions, condition.Type,
			condition.Status, condition.Reason, condition.Message)
	}); err != nil {
		r.log.Error(err, "failed to update status")
	}
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

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.TransformRule{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findRulesForCredentials)).
		Named("transformrule").
		Complete(r)
}
