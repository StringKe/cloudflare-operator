// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package transformrule provides a controller for managing Cloudflare Transform Rules.
// It directly calls Cloudflare API and writes status back to the CRD.
package transformrule

import (
	"context"
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
	finalizerName = "cloudflare.com/transform-rule-finalizer"
)

// Reconciler reconciles a TransformRule object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
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
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch TransformRule")
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

	// Sync TransformRule to Cloudflare
	return r.syncTransformRule(ctx, rule, apiResult, zoneID, zoneName)
}

// handleDeletion handles the deletion of TransformRule.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	rule *networkingv1alpha2.TransformRule,
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
		logger.Info("Deleting TransformRule from Cloudflare",
			"rulesetId", rule.Status.RulesetID,
			"zone", rule.Spec.Zone)

		if err := apiResult.API.DeleteRuleset(ctx, rule.Status.ZoneID, rule.Status.RulesetID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete TransformRule from Cloudflare, continuing with finalizer removal")
				r.Recorder.Event(rule, corev1.EventTypeWarning, "DeleteFailed",
					fmt.Sprintf("Failed to delete from Cloudflare (will remove finalizer anyway): %s", cf.SanitizeErrorMessage(err)))
				// Don't block finalizer removal - resource may need manual cleanup in Cloudflare
			} else {
				logger.Info("TransformRule not found in Cloudflare, may have been already deleted")
			}
		} else {
			r.Recorder.Event(rule, corev1.EventTypeNormal, "Deleted",
				"TransformRule deleted from Cloudflare")
		}
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

// syncTransformRule syncs the TransformRule to Cloudflare.
func (r *Reconciler) syncTransformRule(
	ctx context.Context,
	rule *networkingv1alpha2.TransformRule,
	apiResult *common.APIClientResult,
	zoneID, zoneName string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Determine the phase based on transform type
	phase := r.getPhase(rule)

	// Build rules
	rules := r.buildRules(rule)

	// Build description
	description := rule.Spec.Description
	if description == "" {
		description = fmt.Sprintf("Managed by cloudflare-operator: %s/%s", rule.Namespace, rule.Name)
	}

	// Update the entrypoint ruleset
	logger.V(1).Info("Updating transform ruleset in Cloudflare",
		"zoneId", zoneID,
		"phase", phase,
		"type", rule.Spec.Type,
		"rulesCount", len(rules))

	result, err := apiResult.API.UpdateEntrypointRuleset(ctx, zoneID, phase, description, rules)
	if err != nil {
		logger.Error(err, "Failed to update transform ruleset")
		return r.updateStatusError(ctx, rule, err)
	}

	r.Recorder.Event(rule, corev1.EventTypeNormal, "Updated",
		fmt.Sprintf("TransformRule for zone '%s' type '%s' updated in Cloudflare", zoneName, rule.Spec.Type))

	return r.updateStatusReady(ctx, rule, zoneID, result.ID, len(rules))
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

// buildRules builds Cloudflare ruleset rules from the spec.
//
//nolint:revive // cognitive complexity is acceptable for rule building
func (*Reconciler) buildRules(rule *networkingv1alpha2.TransformRule) []cloudflare.RulesetRule {
	rules := make([]cloudflare.RulesetRule, len(rule.Spec.Rules))

	for i, ruleSpec := range rule.Spec.Rules {
		cfRule := cloudflare.RulesetRule{
			Expression:  ruleSpec.Expression,
			Description: ruleSpec.Name,
			Enabled:     &ruleSpec.Enabled,
		}

		// Set action based on transform type
		switch rule.Spec.Type {
		case networkingv1alpha2.TransformRuleTypeURLRewrite:
			cfRule.Action = "rewrite"
			if ruleSpec.URLRewrite != nil {
				cfRule.ActionParameters = buildURLRewriteParams(ruleSpec.URLRewrite)
			}
		case networkingv1alpha2.TransformRuleTypeRequestHeader:
			cfRule.Action = "rewrite"
			if len(ruleSpec.Headers) > 0 {
				cfRule.ActionParameters = buildHeaderParams(ruleSpec.Headers)
			}
		case networkingv1alpha2.TransformRuleTypeResponseHeader:
			cfRule.Action = "rewrite"
			if len(ruleSpec.Headers) > 0 {
				cfRule.ActionParameters = buildHeaderParams(ruleSpec.Headers)
			}
		}

		rules[i] = cfRule
	}

	return rules
}

// buildURLRewriteParams builds action parameters for URL rewrite.
func buildURLRewriteParams(rewrite *networkingv1alpha2.URLRewriteConfig) *cloudflare.RulesetRuleActionParameters {
	params := &cloudflare.RulesetRuleActionParameters{}

	if rewrite.Path != nil {
		uri := &cloudflare.RulesetRuleActionParametersURI{
			Path: &cloudflare.RulesetRuleActionParametersURIPath{},
		}
		if rewrite.Path.Static != "" {
			uri.Path.Value = rewrite.Path.Static
		}
		if rewrite.Path.Expression != "" {
			uri.Path.Expression = rewrite.Path.Expression
		}
		params.URI = uri
	}

	if rewrite.Query != nil {
		if params.URI == nil {
			params.URI = &cloudflare.RulesetRuleActionParametersURI{}
		}
		query := &cloudflare.RulesetRuleActionParametersURIQuery{}
		if rewrite.Query.Static != "" {
			query.Value = &rewrite.Query.Static
		}
		if rewrite.Query.Expression != "" {
			query.Expression = rewrite.Query.Expression
		}
		params.URI.Query = query
	}

	return params
}

// buildHeaderParams builds action parameters for header modification.
func buildHeaderParams(headers []networkingv1alpha2.HeaderModification) *cloudflare.RulesetRuleActionParameters {
	params := &cloudflare.RulesetRuleActionParameters{
		Headers: make(map[string]cloudflare.RulesetRuleActionParametersHTTPHeader),
	}

	for _, h := range headers {
		params.Headers[h.Name] = cloudflare.RulesetRuleActionParametersHTTPHeader{
			Operation:  string(h.Operation),
			Value:      h.Value,
			Expression: h.Expression,
		}
	}

	return params
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
	rule *networkingv1alpha2.TransformRule,
	zoneID, rulesetID string,
	rulesCount int,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, rule, func() {
		rule.Status.ZoneID = zoneID
		rule.Status.RulesetID = rulesetID
		rule.Status.RuleCount = rulesCount
		rule.Status.State = networkingv1alpha2.TransformRuleStateReady
		rule.Status.Message = "TransformRule synced to Cloudflare"
		meta.SetStatusCondition(&rule.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: rule.Generation,
			Reason:             "Synced",
			Message:            "TransformRule synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		rule.Status.ObservedGeneration = rule.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
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
	r.Recorder = mgr.GetEventRecorderFor("transformrule-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("transformrule"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.TransformRule{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findRulesForCredentials)).
		Named("transformrule").
		Complete(r)
}
