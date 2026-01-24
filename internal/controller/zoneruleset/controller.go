// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package zoneruleset provides a controller for managing Cloudflare zone rulesets.
// It directly calls Cloudflare API and writes status back to the CRD.
package zoneruleset

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
	finalizerName = "cloudflare.com/zone-ruleset-finalizer"
)

// Reconciler reconciles a ZoneRuleset object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=zonerulesets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=zonerulesets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=zonerulesets/finalizers,verbs=update

// Reconcile handles ZoneRuleset reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the ZoneRuleset resource
	ruleset := &networkingv1alpha2.ZoneRuleset{}
	if err := r.Get(ctx, req.NamespacedName, ruleset); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch ZoneRuleset")
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !ruleset.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, ruleset)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, ruleset, finalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CredentialsRef: ruleset.Spec.CredentialsRef,
		Namespace:      ruleset.Namespace,
		StatusZoneID:   ruleset.Status.ZoneID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, ruleset, err)
	}

	// Resolve Zone ID from domain name
	zoneID, zoneName, err := apiResult.API.GetZoneIDForDomain(ctx, ruleset.Spec.Zone)
	if err != nil {
		logger.Error(err, "Failed to resolve zone ID", "zone", ruleset.Spec.Zone)
		return r.updateStatusError(ctx, ruleset, fmt.Errorf("failed to resolve zone '%s': %w", ruleset.Spec.Zone, err))
	}

	// Sync ZoneRuleset to Cloudflare
	return r.syncZoneRuleset(ctx, ruleset, apiResult, zoneID, zoneName)
}

// handleDeletion handles the deletion of ZoneRuleset.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	ruleset *networkingv1alpha2.ZoneRuleset,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(ruleset, finalizerName) {
		return common.NoRequeue(), nil
	}

	// Get API client for deletion
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CredentialsRef: ruleset.Spec.CredentialsRef,
		Namespace:      ruleset.Namespace,
		StatusZoneID:   ruleset.Status.ZoneID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client for deletion")
		// Continue with finalizer removal
	} else if ruleset.Status.RulesetID != "" && ruleset.Status.ZoneID != "" {
		// Delete ruleset from Cloudflare
		logger.Info("Deleting ZoneRuleset from Cloudflare",
			"rulesetId", ruleset.Status.RulesetID,
			"zone", ruleset.Spec.Zone)

		if err := apiResult.API.DeleteRuleset(ctx, ruleset.Status.ZoneID, ruleset.Status.RulesetID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete ZoneRuleset from Cloudflare")
				r.Recorder.Event(ruleset, corev1.EventTypeWarning, "DeleteFailed",
					fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
				return common.RequeueShort(), err
			}
			logger.Info("ZoneRuleset not found in Cloudflare, may have been already deleted")
		}

		r.Recorder.Event(ruleset, corev1.EventTypeNormal, "Deleted",
			"ZoneRuleset deleted from Cloudflare")
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, ruleset, func() {
		controllerutil.RemoveFinalizer(ruleset, finalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(ruleset, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// syncZoneRuleset syncs the ZoneRuleset to Cloudflare.
func (r *Reconciler) syncZoneRuleset(
	ctx context.Context,
	ruleset *networkingv1alpha2.ZoneRuleset,
	apiResult *common.APIClientResult,
	zoneID, zoneName string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Build rules
	rules := r.buildRules(ruleset)

	// Get or create the entrypoint ruleset for this phase
	phase := string(ruleset.Spec.Phase)
	description := ruleset.Spec.Description
	if description == "" {
		description = fmt.Sprintf("Managed by cloudflare-operator: %s/%s", ruleset.Namespace, ruleset.Name)
	}

	// Update the entrypoint ruleset
	logger.V(1).Info("Updating entrypoint ruleset in Cloudflare",
		"zoneId", zoneID,
		"phase", phase,
		"rulesCount", len(rules))

	result, err := apiResult.API.UpdateEntrypointRuleset(ctx, zoneID, phase, description, rules)
	if err != nil {
		logger.Error(err, "Failed to update entrypoint ruleset")
		return r.updateStatusError(ctx, ruleset, err)
	}

	r.Recorder.Event(ruleset, corev1.EventTypeNormal, "Updated",
		fmt.Sprintf("ZoneRuleset for zone '%s' phase '%s' updated in Cloudflare", zoneName, phase))

	return r.updateStatusReady(ctx, ruleset, zoneID, result.ID, len(rules))
}

// buildRules builds Cloudflare ruleset rules from the spec.
//
//nolint:revive // cognitive complexity is acceptable for rule building
func (*Reconciler) buildRules(ruleset *networkingv1alpha2.ZoneRuleset) []cloudflare.RulesetRule {
	rules := make([]cloudflare.RulesetRule, len(ruleset.Spec.Rules))

	for i, rule := range ruleset.Spec.Rules {
		cfRule := cloudflare.RulesetRule{
			Action:      string(rule.Action),
			Expression:  rule.Expression,
			Description: rule.Description,
			Enabled:     &rule.Enabled,
			Ref:         rule.Ref,
		}

		// Handle action parameters if provided
		if rule.ActionParameters != nil {
			cfRule.ActionParameters = convertActionParameters(rule.ActionParameters)
		}

		// Handle rate limit if provided
		if rule.RateLimit != nil {
			cfRule.RateLimit = convertRateLimit(rule.RateLimit)
		}

		rules[i] = cfRule
	}

	return rules
}

// convertActionParameters converts our RulesetRuleActionParameters to cloudflare type.
func convertActionParameters(params *networkingv1alpha2.RulesetRuleActionParameters) *cloudflare.RulesetRuleActionParameters {
	if params == nil {
		return nil
	}

	result := &cloudflare.RulesetRuleActionParameters{}

	// Convert URI rewrite if present
	if params.URI != nil {
		result.URI = &cloudflare.RulesetRuleActionParametersURI{}
		if params.URI.Path != nil {
			result.URI.Path = &cloudflare.RulesetRuleActionParametersURIPath{
				Value:      params.URI.Path.Value,
				Expression: params.URI.Path.Expression,
			}
		}
		if params.URI.Query != nil {
			query := &cloudflare.RulesetRuleActionParametersURIQuery{
				Expression: params.URI.Query.Expression,
			}
			if params.URI.Query.Value != "" {
				query.Value = &params.URI.Query.Value
			}
			result.URI.Query = query
		}
	}

	// Convert headers if present
	if params.Headers != nil {
		result.Headers = make(map[string]cloudflare.RulesetRuleActionParametersHTTPHeader, len(params.Headers))
		for name, header := range params.Headers {
			result.Headers[name] = cloudflare.RulesetRuleActionParametersHTTPHeader{
				Operation:  header.Operation,
				Value:      header.Value,
				Expression: header.Expression,
			}
		}
	}

	return result
}

// convertRateLimit converts our RulesetRuleRateLimit to cloudflare type.
func convertRateLimit(rl *networkingv1alpha2.RulesetRuleRateLimit) *cloudflare.RulesetRuleRateLimit {
	if rl == nil {
		return nil
	}
	result := &cloudflare.RulesetRuleRateLimit{
		Characteristics:         rl.Characteristics,
		Period:                  rl.Period,
		RequestsPerPeriod:       rl.RequestsPerPeriod,
		MitigationTimeout:       rl.MitigationTimeout,
		CountingExpression:      rl.CountingExpression,
		ScorePerPeriod:          rl.ScorePerPeriod,
		ScoreResponseHeaderName: rl.ScoreResponseHeaderName,
	}
	if rl.RequestsToOrigin != nil {
		result.RequestsToOrigin = *rl.RequestsToOrigin
	}
	return result
}

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	ruleset *networkingv1alpha2.ZoneRuleset,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, ruleset, func() {
		ruleset.Status.State = networkingv1alpha2.ZoneRulesetStateError
		ruleset.Status.Message = cf.SanitizeErrorMessage(err)
		meta.SetStatusCondition(&ruleset.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: ruleset.Generation,
			Reason:             "Error",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		ruleset.Status.ObservedGeneration = ruleset.Generation
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
}

func (r *Reconciler) updateStatusReady(
	ctx context.Context,
	ruleset *networkingv1alpha2.ZoneRuleset,
	zoneID, rulesetID string,
	rulesCount int,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, ruleset, func() {
		ruleset.Status.ZoneID = zoneID
		ruleset.Status.RulesetID = rulesetID
		ruleset.Status.RuleCount = rulesCount
		ruleset.Status.State = networkingv1alpha2.ZoneRulesetStateReady
		ruleset.Status.Message = "ZoneRuleset synced to Cloudflare"
		meta.SetStatusCondition(&ruleset.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: ruleset.Generation,
			Reason:             "Synced",
			Message:            "ZoneRuleset synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		ruleset.Status.ObservedGeneration = ruleset.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
}

// findRulesetsForCredentials returns ZoneRulesets that reference the given credentials
//
//nolint:revive // cognitive complexity is acceptable for watch handler
func (r *Reconciler) findRulesetsForCredentials(ctx context.Context, obj client.Object) []reconcile.Request {
	creds, ok := obj.(*networkingv1alpha2.CloudflareCredentials)
	if !ok {
		return nil
	}

	rulesetList := &networkingv1alpha2.ZoneRulesetList{}
	if err := r.List(ctx, rulesetList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, rs := range rulesetList.Items {
		if rs.Spec.CredentialsRef != nil && rs.Spec.CredentialsRef.Name == creds.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      rs.Name,
					Namespace: rs.Namespace,
				},
			})
		}

		if creds.Spec.IsDefault && rs.Spec.CredentialsRef == nil {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      rs.Name,
					Namespace: rs.Namespace,
				},
			})
		}
	}

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("zoneruleset-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("zoneruleset"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.ZoneRuleset{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findRulesetsForCredentials)).
		Named("zoneruleset").
		Complete(r)
}
