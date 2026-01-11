// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package zoneruleset provides a controller for managing Cloudflare zone rulesets.
package zoneruleset

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
	finalizerName = "cloudflare.com/zone-ruleset-finalizer"
)

// Reconciler reconciles a ZoneRuleset object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Internal state
	ctx     context.Context
	log     logr.Logger
	ruleset *networkingv1alpha2.ZoneRuleset
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=zonerulesets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=zonerulesets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=zonerulesets/finalizers,verbs=update

// Reconcile handles ZoneRuleset reconciliation
//
//nolint:revive // cognitive complexity is acceptable for main reconcile loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrl.LoggerFrom(ctx)

	// Get the ZoneRuleset resource
	r.ruleset = &networkingv1alpha2.ZoneRuleset{}
	if err := r.Get(ctx, req.NamespacedName, r.ruleset); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch ZoneRuleset")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !r.ruleset.DeletionTimestamp.IsZero() {
		return r.handleDeletion()
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.ruleset, finalizerName) {
		controllerutil.AddFinalizer(r.ruleset, finalizerName)
		if err := r.Update(ctx, r.ruleset); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Create API client
	cfAPI, err := r.getAPIClient()
	if err != nil {
		r.updateState(networkingv1alpha2.ZoneRulesetStateError, fmt.Sprintf("Failed to get API client: %v", err))
		r.Recorder.Event(r.ruleset, corev1.EventTypeWarning, controller.EventReasonAPIError, cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Get zone ID
	zoneID, err := r.getZoneID(cfAPI)
	if err != nil {
		r.updateState(networkingv1alpha2.ZoneRulesetStateError, fmt.Sprintf("Failed to get zone ID: %v", err))
		r.Recorder.Event(r.ruleset, corev1.EventTypeWarning, controller.EventReasonAPIError, cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Sync ruleset
	return r.syncRuleset(cfAPI, zoneID)
}

// handleDeletion handles the deletion of ZoneRuleset
//
//nolint:revive // cognitive complexity is acceptable for deletion handling
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.ruleset, finalizerName) {
		return ctrl.Result{}, nil
	}

	// Clear rules from the entrypoint ruleset
	if r.ruleset.Status.ZoneID != "" {
		cfAPI, err := r.getAPIClient()
		if err == nil {
			// Update the entrypoint ruleset with empty rules
			_, err := cfAPI.UpdateEntrypointRuleset(
				r.ctx,
				r.ruleset.Status.ZoneID,
				string(r.ruleset.Spec.Phase),
				"",
				[]cloudflare.RulesetRule{},
			)
			if err != nil && !cf.IsNotFoundError(err) {
				r.log.Error(err, "Failed to clear ruleset rules")
				// Continue with deletion anyway
			} else {
				r.log.Info("Ruleset rules cleared", "phase", r.ruleset.Spec.Phase)
			}
		}
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.ruleset, func() {
		controllerutil.RemoveFinalizer(r.ruleset, finalizerName)
	}); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// syncRuleset syncs the ruleset to Cloudflare
//
//nolint:revive // cognitive complexity is acceptable for sync logic
func (r *Reconciler) syncRuleset(cfAPI *cf.API, zoneID string) (ctrl.Result, error) {
	r.updateState(networkingv1alpha2.ZoneRulesetStateSyncing, "Syncing ruleset")

	// Convert rules to Cloudflare format
	rules, err := r.convertRules()
	if err != nil {
		r.updateState(networkingv1alpha2.ZoneRulesetStateError, fmt.Sprintf("Failed to convert rules: %v", err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Update entrypoint ruleset
	result, err := cfAPI.UpdateEntrypointRuleset(
		r.ctx,
		zoneID,
		string(r.ruleset.Spec.Phase),
		r.ruleset.Spec.Description,
		rules,
	)
	if err != nil {
		r.updateState(networkingv1alpha2.ZoneRulesetStateError, fmt.Sprintf("Failed to update ruleset: %v", err))
		r.Recorder.Event(r.ruleset, corev1.EventTypeWarning, controller.EventReasonAPIError, cf.SanitizeErrorMessage(err))
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Update status
	r.ruleset.Status.RulesetID = result.ID
	r.ruleset.Status.RulesetVersion = result.Version
	r.ruleset.Status.ZoneID = zoneID
	r.ruleset.Status.RuleCount = len(result.Rules)
	if !result.LastUpdated.IsZero() {
		lastUpdated := metav1.NewTime(result.LastUpdated)
		r.ruleset.Status.LastUpdated = &lastUpdated
	}

	r.updateState(networkingv1alpha2.ZoneRulesetStateReady, "Ruleset synced successfully")
	r.Recorder.Event(r.ruleset, corev1.EventTypeNormal, "Synced",
		fmt.Sprintf("Ruleset synced with %d rules", len(result.Rules)))

	// Requeue periodically to detect drift
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// convertRules converts CRD rules to Cloudflare RulesetRule format
//
//nolint:revive,unparam // cyclomatic complexity is acceptable for rule conversion; error kept for future use
func (r *Reconciler) convertRules() ([]cloudflare.RulesetRule, error) {
	rules := make([]cloudflare.RulesetRule, len(r.ruleset.Spec.Rules))

	for i, rule := range r.ruleset.Spec.Rules {
		cfRule := cloudflare.RulesetRule{
			Action:      string(rule.Action),
			Expression:  rule.Expression,
			Description: rule.Description,
			Enabled:     ptr.To(rule.Enabled),
			Ref:         rule.Ref,
		}

		// Convert action parameters
		if rule.ActionParameters != nil {
			cfRule.ActionParameters = r.convertActionParameters(rule.ActionParameters)
		}

		// Convert rate limit
		if rule.RateLimit != nil {
			cfRule.RateLimit = r.convertRateLimit(rule.RateLimit)
		}

		rules[i] = cfRule
	}

	return rules, nil
}

// convertActionParameters converts action parameters to Cloudflare format
//
//nolint:revive // cyclomatic complexity is acceptable for parameter conversion
func (r *Reconciler) convertActionParameters(params *networkingv1alpha2.RulesetRuleActionParameters) *cloudflare.RulesetRuleActionParameters {
	cfParams := &cloudflare.RulesetRuleActionParameters{}

	// URI rewrite
	if params.URI != nil {
		cfParams.URI = &cloudflare.RulesetRuleActionParametersURI{}
		if params.URI.Path != nil {
			cfParams.URI.Path = &cloudflare.RulesetRuleActionParametersURIPath{
				Value:      params.URI.Path.Value,
				Expression: params.URI.Path.Expression,
			}
		}
		if params.URI.Query != nil {
			cfParams.URI.Query = &cloudflare.RulesetRuleActionParametersURIQuery{
				Expression: params.URI.Query.Expression,
			}
			if params.URI.Query.Value != "" {
				cfParams.URI.Query.Value = ptr.To(params.URI.Query.Value)
			}
		}
	}

	// Headers
	if len(params.Headers) > 0 {
		cfParams.Headers = make(map[string]cloudflare.RulesetRuleActionParametersHTTPHeader)
		for name, header := range params.Headers {
			cfParams.Headers[name] = cloudflare.RulesetRuleActionParametersHTTPHeader{
				Operation:  header.Operation,
				Value:      header.Value,
				Expression: header.Expression,
			}
		}
	}

	// Redirect
	if params.Redirect != nil {
		cfParams.FromValue = &cloudflare.RulesetRuleActionParametersFromValue{
			PreserveQueryString: ptr.To(params.Redirect.PreserveQueryString),
		}
		if params.Redirect.StatusCode > 0 {
			cfParams.FromValue.StatusCode = uint16(params.Redirect.StatusCode)
		}
		if params.Redirect.TargetURL != nil {
			cfParams.FromValue.TargetURL = cloudflare.RulesetRuleActionParametersTargetURL{
				Value:      params.Redirect.TargetURL.Value,
				Expression: params.Redirect.TargetURL.Expression,
			}
		}
	}

	// Origin
	if params.Origin != nil {
		cfParams.Origin = &cloudflare.RulesetRuleActionParametersOrigin{
			Host: params.Origin.Host,
		}
		if params.Origin.Port > 0 {
			cfParams.Origin.Port = uint16(params.Origin.Port)
		}
	}

	// Cache settings
	if params.Cache != nil {
		cfParams.Cache = params.Cache.Cache
		if params.Cache.EdgeTTL != nil {
			cfParams.EdgeTTL = r.convertCacheTTL(params.Cache.EdgeTTL)
		}
		if params.Cache.BrowserTTL != nil {
			cfParams.BrowserTTL = r.convertBrowserTTL(params.Cache.BrowserTTL)
		}
		if params.Cache.CacheKey != nil {
			cfParams.CacheKey = r.convertCacheKey(params.Cache.CacheKey)
		}
		cfParams.RespectStrongETags = params.Cache.RespectStrongETags
		cfParams.OriginErrorPagePassthru = params.Cache.OriginErrorPagePassthru
	}

	// Products (for skip action)
	if len(params.Products) > 0 {
		cfParams.Products = params.Products
	}

	// Ruleset (for execute action)
	if params.Ruleset != "" {
		cfParams.ID = params.Ruleset
	}

	// Phases (for skip action)
	if len(params.Phases) > 0 {
		cfParams.Phases = params.Phases
	}

	// Rules (for skip action)
	if len(params.Rules) > 0 {
		cfParams.Rules = params.Rules
	}

	// Response (for serve_error action)
	if params.Response != nil {
		cfParams.Response = &cloudflare.RulesetRuleActionParametersBlockResponse{
			ContentType: params.Response.ContentType,
			Content:     params.Response.Content,
		}
		if params.Response.StatusCode > 0 {
			cfParams.Response.StatusCode = uint16(params.Response.StatusCode)
		}
	}

	// Algorithms (for compress_response action)
	if len(params.Algorithms) > 0 {
		cfParams.Algorithms = make([]cloudflare.RulesetRuleActionParametersCompressionAlgorithm, len(params.Algorithms))
		for i, alg := range params.Algorithms {
			cfParams.Algorithms[i] = cloudflare.RulesetRuleActionParametersCompressionAlgorithm{
				Name: alg.Name,
			}
		}
	}

	return cfParams
}

// convertCacheTTL converts cache TTL settings
//
//nolint:revive // cognitive complexity is acceptable for TTL conversion
func (*Reconciler) convertCacheTTL(ttl *networkingv1alpha2.RulesetCacheTTL) *cloudflare.RulesetRuleActionParametersEdgeTTL {
	cfTTL := &cloudflare.RulesetRuleActionParametersEdgeTTL{
		Mode: ttl.Mode,
	}
	if ttl.Default != nil {
		cfTTL.Default = ptr.To(uint(*ttl.Default))
	}
	if len(ttl.StatusCodeTTL) > 0 {
		cfTTL.StatusCodeTTL = make([]cloudflare.RulesetRuleActionParametersStatusCodeTTL, len(ttl.StatusCodeTTL))
		for i, sct := range ttl.StatusCodeTTL {
			cfTTL.StatusCodeTTL[i] = cloudflare.RulesetRuleActionParametersStatusCodeTTL{
				Value: ptr.To(sct.Value),
			}
			if sct.StatusCodeRange != nil {
				cfTTL.StatusCodeTTL[i].StatusCodeRange = &cloudflare.RulesetRuleActionParametersStatusCodeRange{
					From: ptr.To(uint(sct.StatusCodeRange.From)),
					To:   ptr.To(uint(sct.StatusCodeRange.To)),
				}
			}
			if sct.StatusCodeValue != nil {
				cfTTL.StatusCodeTTL[i].StatusCodeValue = ptr.To(uint(*sct.StatusCodeValue))
			}
		}
	}
	return cfTTL
}

// convertBrowserTTL converts browser TTL settings
func (*Reconciler) convertBrowserTTL(ttl *networkingv1alpha2.RulesetCacheTTL) *cloudflare.RulesetRuleActionParametersBrowserTTL {
	cfTTL := &cloudflare.RulesetRuleActionParametersBrowserTTL{
		Mode: ttl.Mode,
	}
	if ttl.Default != nil {
		cfTTL.Default = ptr.To(uint(*ttl.Default))
	}
	return cfTTL
}

// convertCacheKey converts cache key settings
//
//nolint:revive // cyclomatic complexity is acceptable for cache key conversion
func (r *Reconciler) convertCacheKey(key *networkingv1alpha2.RulesetCacheKey) *cloudflare.RulesetRuleActionParametersCacheKey {
	cfKey := &cloudflare.RulesetRuleActionParametersCacheKey{
		IgnoreQueryStringsOrder: key.IgnoreQueryStringsOrder,
		CacheDeceptionArmor:     key.CacheDeceptionArmor,
	}

	if key.QueryString != nil {
		cfKey.CustomKey = &cloudflare.RulesetRuleActionParametersCustomKey{
			Query: &cloudflare.RulesetRuleActionParametersCustomKeyQuery{},
		}
		if key.QueryString.Exclude != nil {
			cfKey.CustomKey.Query.Exclude = &cloudflare.RulesetRuleActionParametersCustomKeyList{
				List: key.QueryString.Exclude.List,
				All:  false,
			}
			if key.QueryString.Exclude.All != nil && *key.QueryString.Exclude.All {
				cfKey.CustomKey.Query.Exclude.All = true
			}
		}
		if key.QueryString.Include != nil {
			cfKey.CustomKey.Query.Include = &cloudflare.RulesetRuleActionParametersCustomKeyList{
				List: key.QueryString.Include.List,
				All:  false,
			}
			if key.QueryString.Include.All != nil && *key.QueryString.Include.All {
				cfKey.CustomKey.Query.Include.All = true
			}
		}
	}

	if key.Header != nil {
		if cfKey.CustomKey == nil {
			cfKey.CustomKey = &cloudflare.RulesetRuleActionParametersCustomKey{}
		}
		cfKey.CustomKey.Header = &cloudflare.RulesetRuleActionParametersCustomKeyHeader{
			RulesetRuleActionParametersCustomKeyFields: cloudflare.RulesetRuleActionParametersCustomKeyFields{
				Include:       key.Header.Include,
				CheckPresence: key.Header.CheckPresence,
			},
			ExcludeOrigin: key.Header.ExcludeOrigin,
		}
	}

	if key.Cookie != nil {
		if cfKey.CustomKey == nil {
			cfKey.CustomKey = &cloudflare.RulesetRuleActionParametersCustomKey{}
		}
		cfKey.CustomKey.Cookie = &cloudflare.RulesetRuleActionParametersCustomKeyCookie{
			Include:       key.Cookie.Include,
			CheckPresence: key.Cookie.CheckPresence,
		}
	}

	if key.User != nil {
		if cfKey.CustomKey == nil {
			cfKey.CustomKey = &cloudflare.RulesetRuleActionParametersCustomKey{}
		}
		cfKey.CustomKey.User = &cloudflare.RulesetRuleActionParametersCustomKeyUser{
			DeviceType: key.User.DeviceType,
			Geo:        key.User.Geo,
			Lang:       key.User.Lang,
		}
	}

	if key.Host != nil {
		if cfKey.CustomKey == nil {
			cfKey.CustomKey = &cloudflare.RulesetRuleActionParametersCustomKey{}
		}
		cfKey.CustomKey.Host = &cloudflare.RulesetRuleActionParametersCustomKeyHost{
			Resolved: key.Host.Resolved,
		}
	}

	return cfKey
}

// convertRateLimit converts rate limit settings
func (*Reconciler) convertRateLimit(rl *networkingv1alpha2.RulesetRuleRateLimit) *cloudflare.RulesetRuleRateLimit {
	cfRL := &cloudflare.RulesetRuleRateLimit{
		Characteristics:         rl.Characteristics,
		CountingExpression:      rl.CountingExpression,
		ScoreResponseHeaderName: rl.ScoreResponseHeaderName,
	}
	if rl.RequestsToOrigin != nil {
		cfRL.RequestsToOrigin = *rl.RequestsToOrigin
	}
	if rl.Period > 0 {
		cfRL.Period = rl.Period
	}
	if rl.RequestsPerPeriod > 0 {
		cfRL.RequestsPerPeriod = rl.RequestsPerPeriod
	}
	if rl.MitigationTimeout > 0 {
		cfRL.MitigationTimeout = rl.MitigationTimeout
	}
	if rl.ScorePerPeriod > 0 {
		cfRL.ScorePerPeriod = rl.ScorePerPeriod
	}
	return cfRL
}

// getZoneID gets the zone ID for the zone name
func (r *Reconciler) getZoneID(cfAPI *cf.API) (string, error) {
	// Set the domain on the API
	cfAPI.Domain = r.ruleset.Spec.Zone

	// Get zone ID
	zoneID, err := cfAPI.GetZoneId()
	if err != nil {
		return "", fmt.Errorf("failed to get zone ID for %s: %w", r.ruleset.Spec.Zone, err)
	}
	return zoneID, nil
}

// getAPIClient creates a Cloudflare API client from credentials
func (r *Reconciler) getAPIClient() (*cf.API, error) {
	if r.ruleset.Spec.CredentialsRef != nil {
		ref := &networkingv1alpha2.CloudflareCredentialsRef{
			Name: r.ruleset.Spec.CredentialsRef.Name,
		}
		return cf.NewAPIClientFromCredentialsRef(r.ctx, r.Client, ref)
	}
	return cf.NewAPIClientFromDefaultCredentials(r.ctx, r.Client)
}

// updateState updates the state and status of the ZoneRuleset
func (r *Reconciler) updateState(state networkingv1alpha2.ZoneRulesetState, message string) {
	r.ruleset.Status.State = state
	r.ruleset.Status.Message = message
	r.ruleset.Status.ObservedGeneration = r.ruleset.Generation

	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		ObservedGeneration: r.ruleset.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(state),
		Message:            message,
	}

	if state == networkingv1alpha2.ZoneRulesetStateReady {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "RulesetReady"
	}

	controller.SetCondition(&r.ruleset.Status.Conditions, condition.Type, condition.Status, condition.Reason, condition.Message)

	if err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.ruleset, func() {
		r.ruleset.Status.State = state
		r.ruleset.Status.Message = message
		r.ruleset.Status.ObservedGeneration = r.ruleset.Generation
		controller.SetCondition(&r.ruleset.Status.Conditions, condition.Type, condition.Status, condition.Reason, condition.Message)
	}); err != nil {
		r.log.Error(err, "failed to update status")
	}
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
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.ZoneRuleset{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findRulesetsForCredentials)).
		Named("zoneruleset").
		Complete(r)
}
