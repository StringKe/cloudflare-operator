// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package redirectrule provides a controller for managing Cloudflare Redirect Rules.
package redirectrule

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
	finalizerName = "cloudflare.com/redirect-rule-finalizer"
	// Phase for dynamic redirects
	redirectPhase = "http_request_dynamic_redirect"
)

// Reconciler reconciles a RedirectRule object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Internal state
	ctx  context.Context
	log  logr.Logger
	rule *networkingv1alpha2.RedirectRule
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=redirectrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=redirectrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=redirectrules/finalizers,verbs=update

// Reconcile handles RedirectRule reconciliation
//
//nolint:revive // cognitive complexity is acceptable for main reconcile loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrl.LoggerFrom(ctx)

	// Get the RedirectRule resource
	r.rule = &networkingv1alpha2.RedirectRule{}
	if err := r.Get(ctx, req.NamespacedName, r.rule); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch RedirectRule")
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

	// Validate that at least one rule type is specified
	if len(r.rule.Spec.Rules) == 0 && len(r.rule.Spec.WildcardRules) == 0 {
		r.updateState(networkingv1alpha2.RedirectRuleStateError,
			"At least one rule or wildcardRule must be specified")
		return ctrl.Result{}, nil
	}

	// Create API client
	cfAPI, err := r.getAPIClient()
	if err != nil {
		r.updateState(networkingv1alpha2.RedirectRuleStateError,
			fmt.Sprintf("Failed to get API client: %v", err))
		r.Recorder.Event(r.rule, corev1.EventTypeWarning,
			controller.EventReasonAPIError, cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Get zone ID
	zoneID, err := r.getZoneID(cfAPI)
	if err != nil {
		r.updateState(networkingv1alpha2.RedirectRuleStateError,
			fmt.Sprintf("Failed to get zone ID: %v", err))
		r.Recorder.Event(r.rule, corev1.EventTypeWarning,
			controller.EventReasonAPIError, cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Sync rules
	return r.syncRules(cfAPI, zoneID)
}

// handleDeletion handles the deletion of RedirectRule
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
			_, err := cfAPI.UpdateEntrypointRuleset(
				r.ctx,
				r.rule.Status.ZoneID,
				redirectPhase,
				"",
				[]cloudflare.RulesetRule{},
			)
			if err != nil && !cf.IsNotFoundError(err) {
				r.log.Error(err, "Failed to clear redirect rules")
			} else {
				r.log.Info("Redirect rules cleared")
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

// syncRules syncs the redirect rules to Cloudflare
func (r *Reconciler) syncRules(cfAPI *cf.API, zoneID string) (ctrl.Result, error) {
	r.updateState(networkingv1alpha2.RedirectRuleStateSyncing, "Syncing redirect rules")

	// Convert rules to Cloudflare format
	rules := r.convertRules()

	// Update entrypoint ruleset
	result, err := cfAPI.UpdateEntrypointRuleset(
		r.ctx,
		zoneID,
		redirectPhase,
		r.rule.Spec.Description,
		rules,
	)
	if err != nil {
		r.updateState(networkingv1alpha2.RedirectRuleStateError,
			fmt.Sprintf("Failed to update rules: %v", err))
		r.Recorder.Event(r.rule, corev1.EventTypeWarning,
			controller.EventReasonAPIError, cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Update status
	r.rule.Status.RulesetID = result.ID
	r.rule.Status.ZoneID = zoneID
	r.rule.Status.RuleCount = len(result.Rules)

	r.updateState(networkingv1alpha2.RedirectRuleStateReady, "Redirect rules synced successfully")
	r.Recorder.Event(r.rule, corev1.EventTypeNormal, "Synced",
		fmt.Sprintf("Redirect rules synced with %d rules", len(result.Rules)))

	// Requeue periodically to detect drift
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// convertRules converts RedirectRule rules to Cloudflare RulesetRule format
//
//nolint:revive // cognitive complexity is acceptable for rule conversion
func (r *Reconciler) convertRules() []cloudflare.RulesetRule {
	totalRules := len(r.rule.Spec.Rules) + len(r.rule.Spec.WildcardRules)
	rules := make([]cloudflare.RulesetRule, 0, totalRules)

	// Convert expression-based rules
	for _, rule := range r.rule.Spec.Rules {
		cfRule := cloudflare.RulesetRule{
			Action:      "redirect",
			Expression:  rule.Expression,
			Description: rule.Name,
			Enabled:     ptr.To(rule.Enabled),
			ActionParameters: &cloudflare.RulesetRuleActionParameters{
				FromValue: &cloudflare.RulesetRuleActionParametersFromValue{
					PreserveQueryString: ptr.To(rule.PreserveQueryString),
				},
			},
		}

		// Set status code
		if rule.StatusCode > 0 {
			cfRule.ActionParameters.FromValue.StatusCode = uint16(rule.StatusCode)
		} else {
			cfRule.ActionParameters.FromValue.StatusCode = 302
		}

		// Set target URL
		cfRule.ActionParameters.FromValue.TargetURL = cloudflare.RulesetRuleActionParametersTargetURL{}
		if rule.Target.URL != "" {
			cfRule.ActionParameters.FromValue.TargetURL.Value = rule.Target.URL
		}
		if rule.Target.Expression != "" {
			cfRule.ActionParameters.FromValue.TargetURL.Expression = rule.Target.Expression
		}

		rules = append(rules, cfRule)
	}

	// Convert wildcard-based rules
	for _, rule := range r.rule.Spec.WildcardRules {
		// Build the expression from wildcard pattern
		expression := r.buildWildcardExpression(rule)

		cfRule := cloudflare.RulesetRule{
			Action:      "redirect",
			Expression:  expression,
			Description: rule.Name,
			Enabled:     ptr.To(rule.Enabled),
			ActionParameters: &cloudflare.RulesetRuleActionParameters{
				FromValue: &cloudflare.RulesetRuleActionParametersFromValue{
					PreserveQueryString: ptr.To(rule.PreserveQueryString),
					TargetURL: cloudflare.RulesetRuleActionParametersTargetURL{
						Value: rule.TargetURL,
					},
				},
			},
		}

		// Set status code
		if rule.StatusCode > 0 {
			cfRule.ActionParameters.FromValue.StatusCode = uint16(rule.StatusCode)
		} else {
			cfRule.ActionParameters.FromValue.StatusCode = 301
		}

		rules = append(rules, cfRule)
	}

	return rules
}

// buildWildcardExpression builds a filter expression from a wildcard URL pattern
func (*Reconciler) buildWildcardExpression(rule networkingv1alpha2.WildcardRedirectRule) string {
	// For simplicity, convert wildcard patterns to expression
	// In production, you might want more sophisticated pattern matching
	// Example: https://example.com/blog/* -> http.request.uri.path matches "^/blog/.*"

	// This is a simplified implementation
	// Cloudflare's wildcard redirects use a different mechanism (Single Redirects with wildcards)
	// For the API, we need to construct proper expressions

	// For now, use a basic expression that matches the full URL
	return fmt.Sprintf(`http.request.full_uri wildcard "%s"`, rule.SourceURL)
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

// updateState updates the state and status of the RedirectRule
func (r *Reconciler) updateState(state networkingv1alpha2.RedirectRuleState, message string) {
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

	if state == networkingv1alpha2.RedirectRuleStateReady {
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

// findRulesForCredentials returns RedirectRules that reference the given credentials
//
//nolint:revive // cognitive complexity is acceptable for watch handler
func (r *Reconciler) findRulesForCredentials(
	ctx context.Context, obj client.Object,
) []reconcile.Request {
	creds, ok := obj.(*networkingv1alpha2.CloudflareCredentials)
	if !ok {
		return nil
	}

	ruleList := &networkingv1alpha2.RedirectRuleList{}
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
		For(&networkingv1alpha2.RedirectRule{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findRulesForCredentials)).
		Named("redirectrule").
		Complete(r)
}
