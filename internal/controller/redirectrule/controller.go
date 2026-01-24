// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package redirectrule provides a controller for managing Cloudflare Redirect Rules.
// It directly calls Cloudflare API and writes status back to the CRD.
package redirectrule

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
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
	"github.com/StringKe/cloudflare-operator/internal/controller/common"
)

const (
	finalizerName = "cloudflare.com/redirect-rule-finalizer"
	// Phase for dynamic redirects
	redirectPhase = "http_request_dynamic_redirect"
)

// Reconciler reconciles a RedirectRule object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=redirectrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=redirectrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=redirectrules/finalizers,verbs=update

// Reconcile handles RedirectRule reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the RedirectRule resource
	rule := &networkingv1alpha2.RedirectRule{}
	if err := r.Get(ctx, req.NamespacedName, rule); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch RedirectRule")
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

	// Validate that at least one rule type is specified
	if len(rule.Spec.Rules) == 0 && len(rule.Spec.WildcardRules) == 0 {
		err := errors.New("at least one rule or wildcardRule must be specified")
		return r.updateStatusError(ctx, rule, err)
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CredentialsRef: rule.Spec.CredentialsRef,
		Namespace:      rule.Namespace,
		StatusZoneID:   rule.Status.ZoneID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, rule, err)
	}

	// Resolve Zone ID from domain name
	zoneID, zoneName, err := apiResult.API.GetZoneIDForDomain(ctx, rule.Spec.Zone)
	if err != nil {
		logger.Error(err, "Failed to resolve zone ID", "zone", rule.Spec.Zone)
		return r.updateStatusError(ctx, rule, fmt.Errorf("failed to resolve zone '%s': %w", rule.Spec.Zone, err))
	}

	// Sync RedirectRule to Cloudflare
	return r.syncRedirectRule(ctx, rule, apiResult, zoneID, zoneName)
}

// handleDeletion handles the deletion of RedirectRule.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	rule *networkingv1alpha2.RedirectRule,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(rule, finalizerName) {
		return common.NoRequeue(), nil
	}

	// Get API client for deletion
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CredentialsRef: rule.Spec.CredentialsRef,
		Namespace:      rule.Namespace,
		StatusZoneID:   rule.Status.ZoneID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client for deletion")
		// Continue with finalizer removal
	} else if rule.Status.RulesetID != "" && rule.Status.ZoneID != "" {
		// Delete ruleset from Cloudflare
		logger.Info("Deleting RedirectRule from Cloudflare",
			"rulesetId", rule.Status.RulesetID,
			"zone", rule.Spec.Zone)

		if err := apiResult.API.DeleteRuleset(ctx, rule.Status.ZoneID, rule.Status.RulesetID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete RedirectRule from Cloudflare")
				r.Recorder.Event(rule, corev1.EventTypeWarning, "DeleteFailed",
					fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
				return common.RequeueShort(), err
			}
			logger.Info("RedirectRule not found in Cloudflare, may have been already deleted")
		}

		r.Recorder.Event(rule, corev1.EventTypeNormal, "Deleted",
			"RedirectRule deleted from Cloudflare")
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

// syncRedirectRule syncs the RedirectRule to Cloudflare.
func (r *Reconciler) syncRedirectRule(
	ctx context.Context,
	rule *networkingv1alpha2.RedirectRule,
	apiResult *common.APIClientResult,
	zoneID, zoneName string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Build rules from both expression-based and wildcard rules
	rules := r.buildRules(rule)

	// Build description
	description := rule.Spec.Description
	if description == "" {
		description = fmt.Sprintf("Managed by cloudflare-operator: %s/%s", rule.Namespace, rule.Name)
	}

	// Update the entrypoint ruleset for dynamic redirects
	logger.V(1).Info("Updating redirect ruleset in Cloudflare",
		"zoneId", zoneID,
		"phase", redirectPhase,
		"rulesCount", len(rules))

	result, err := apiResult.API.UpdateEntrypointRuleset(ctx, zoneID, redirectPhase, description, rules)
	if err != nil {
		logger.Error(err, "Failed to update redirect ruleset")
		return r.updateStatusError(ctx, rule, err)
	}

	r.Recorder.Event(rule, corev1.EventTypeNormal, "Updated",
		fmt.Sprintf("RedirectRule for zone '%s' updated in Cloudflare", zoneName))

	return r.updateStatusReady(ctx, rule, zoneID, result.ID, len(rules))
}

// buildRules builds Cloudflare ruleset rules from the spec.
//
//nolint:revive // cognitive complexity is acceptable for rule building
func (r *Reconciler) buildRules(rule *networkingv1alpha2.RedirectRule) []cloudflare.RulesetRule {
	var rules []cloudflare.RulesetRule

	// Process expression-based rules
	for _, ruleSpec := range rule.Spec.Rules {
		cfRule := cloudflare.RulesetRule{
			Action:      "redirect",
			Expression:  ruleSpec.Expression,
			Description: ruleSpec.Name,
			Enabled:     &ruleSpec.Enabled,
		}

		// Build action parameters for redirect
		params := &cloudflare.RulesetRuleActionParameters{
			FromValue: &cloudflare.RulesetRuleActionParametersFromValue{
				PreserveQueryString: &ruleSpec.PreserveQueryString,
				StatusCode:          uint16(ruleSpec.StatusCode),
			},
		}

		// Set target URL (static or expression)
		if ruleSpec.Target.URL != "" {
			params.FromValue.TargetURL = cloudflare.RulesetRuleActionParametersTargetURL{
				Value: ruleSpec.Target.URL,
			}
		} else if ruleSpec.Target.Expression != "" {
			params.FromValue.TargetURL = cloudflare.RulesetRuleActionParametersTargetURL{
				Expression: ruleSpec.Target.Expression,
			}
		}

		cfRule.ActionParameters = params
		rules = append(rules, cfRule)
	}

	// Process wildcard-based rules
	for _, wildcardSpec := range rule.Spec.WildcardRules {
		cfRule := cloudflare.RulesetRule{
			Action:      "redirect",
			Description: wildcardSpec.Name,
			Enabled:     &wildcardSpec.Enabled,
		}

		// Build expression from wildcard source URL
		expression := r.buildWildcardExpression(wildcardSpec)
		cfRule.Expression = expression

		// Build action parameters
		params := &cloudflare.RulesetRuleActionParameters{
			FromValue: &cloudflare.RulesetRuleActionParametersFromValue{
				PreserveQueryString: &wildcardSpec.PreserveQueryString,
				StatusCode:          uint16(wildcardSpec.StatusCode),
			},
		}

		// Set target URL with dynamic replacement
		params.FromValue.TargetURL = cloudflare.RulesetRuleActionParametersTargetURL{
			Expression: r.buildWildcardTargetExpression(wildcardSpec),
		}

		cfRule.ActionParameters = params
		rules = append(rules, cfRule)
	}

	return rules
}

// buildWildcardExpression builds a filter expression from a wildcard redirect rule
func (*Reconciler) buildWildcardExpression(spec networkingv1alpha2.WildcardRedirectRule) string {
	// Basic expression for matching the source URL
	// This is a simplified implementation - real wildcard matching is complex
	expr := fmt.Sprintf(`(http.request.full_uri wildcard r"%s")`, spec.SourceURL)

	if spec.IncludeSubdomains {
		// Add subdomain matching to expression
		expr = fmt.Sprintf(`(http.host matches r".*\.%s" or %s)`, spec.SourceURL, expr)
	}

	return expr
}

// buildWildcardTargetExpression builds a target URL expression for wildcard rules
func (*Reconciler) buildWildcardTargetExpression(spec networkingv1alpha2.WildcardRedirectRule) string {
	// For subpath matching, preserve the path
	if spec.SubpathMatching {
		return fmt.Sprintf(`concat("%s", http.request.uri.path)`, spec.TargetURL)
	}
	return fmt.Sprintf(`"%s"`, spec.TargetURL)
}

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	rule *networkingv1alpha2.RedirectRule,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, rule, func() {
		rule.Status.State = networkingv1alpha2.RedirectRuleStateError
		rule.Status.Message = cf.SanitizeErrorMessage(err)
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
	rule *networkingv1alpha2.RedirectRule,
	zoneID, rulesetID string,
	rulesCount int,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, rule, func() {
		rule.Status.ZoneID = zoneID
		rule.Status.RulesetID = rulesetID
		rule.Status.RuleCount = rulesCount
		rule.Status.State = networkingv1alpha2.RedirectRuleStateReady
		rule.Status.Message = "RedirectRule synced to Cloudflare"
		meta.SetStatusCondition(&rule.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: rule.Generation,
			Reason:             "Synced",
			Message:            "RedirectRule synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		rule.Status.ObservedGeneration = rule.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
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
	r.Recorder = mgr.GetEventRecorderFor("redirectrule-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("redirectrule"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.RedirectRule{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findRulesForCredentials)).
		Named("redirectrule").
		Complete(r)
}
