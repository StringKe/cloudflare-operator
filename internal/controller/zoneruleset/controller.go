// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package zoneruleset provides a controller for managing Cloudflare zone rulesets.
package zoneruleset

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
	finalizerName = "cloudflare.com/zone-ruleset-finalizer"
)

// Reconciler reconciles a ZoneRuleset object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Services
	zoneRulesetService *rulesetsvc.ZoneRulesetService
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
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch ZoneRuleset")
		return ctrl.Result{}, err
	}

	// Resolve credentials and zone ID
	creds, err := r.resolveCredentials(ctx, ruleset)
	if err != nil {
		logger.Error(err, "Failed to resolve credentials")
		return r.updateStatusError(ctx, ruleset, err)
	}
	credRef := networkingv1alpha2.CredentialsReference{Name: creds.CredentialsName}

	// Handle deletion
	if !ruleset.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, ruleset, credRef)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(ruleset, finalizerName) {
		controllerutil.AddFinalizer(ruleset, finalizerName)
		if err := r.Update(ctx, ruleset); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Register ZoneRuleset configuration to SyncState
	return r.registerRuleset(ctx, ruleset, creds.AccountID, creds.ZoneID, credRef)
}

// resolveCredentials resolves the credentials reference, account ID and zone ID.
func (r *Reconciler) resolveCredentials(
	ctx context.Context,
	ruleset *networkingv1alpha2.ZoneRuleset,
) (controller.CredentialsResult, error) {
	logger := log.FromContext(ctx)

	var result controller.CredentialsResult

	// Get credentials reference
	if ruleset.Spec.CredentialsRef != nil {
		result.CredentialsName = ruleset.Spec.CredentialsRef.Name
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
	apiClient.Domain = ruleset.Spec.Zone
	zoneID, err := apiClient.GetZoneId()
	if err != nil {
		return result, fmt.Errorf("failed to get zone ID for %s: %w", ruleset.Spec.Zone, err)
	}
	result.ZoneID = zoneID

	return result, nil
}

// handleDeletion handles the deletion of ZoneRuleset
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	ruleset *networkingv1alpha2.ZoneRuleset,
	credRef networkingv1alpha2.CredentialsReference,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(ruleset, finalizerName) {
		return ctrl.Result{}, nil
	}

	// Clear rules from Cloudflare
	if r.clearRulesFromCloudflare(ctx, ruleset, credRef) {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Unregister from SyncState
	source := service.Source{
		Kind:      "ZoneRuleset",
		Namespace: ruleset.Namespace,
		Name:      ruleset.Name,
	}
	if err := r.zoneRulesetService.Unregister(ctx, ruleset.Status.RulesetID, source); err != nil {
		logger.Error(err, "Failed to unregister ZoneRuleset from SyncState")
		// Non-fatal - continue with finalizer removal
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, ruleset, func() {
		controllerutil.RemoveFinalizer(ruleset, finalizerName)
	}); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(ruleset, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")
	return ctrl.Result{}, nil
}

// clearRulesFromCloudflare clears the ruleset rules from Cloudflare.
// Returns true if reconciliation should be requeued.
func (r *Reconciler) clearRulesFromCloudflare(
	ctx context.Context,
	ruleset *networkingv1alpha2.ZoneRuleset,
	credRef networkingv1alpha2.CredentialsReference,
) bool {
	if ruleset.Status.ZoneID == "" || ruleset.Status.RulesetID == "" {
		return false
	}

	logger := log.FromContext(ctx)
	cfCredRef := &networkingv1alpha2.CloudflareCredentialsRef{Name: credRef.Name}
	cfAPI, err := cf.NewAPIClientFromCredentialsRef(ctx, r.Client, cfCredRef)
	if err != nil {
		return false // Skip if can't create client
	}

	phase := string(ruleset.Spec.Phase)
	_, err = cfAPI.UpdateEntrypointRuleset(ctx, ruleset.Status.ZoneID, phase, "", nil)
	if err == nil {
		logger.Info("Ruleset rules cleared", "phase", phase)
		r.Recorder.Event(ruleset, corev1.EventTypeNormal, "Deleted",
			fmt.Sprintf("Ruleset rules cleared for phase %s", phase))
		return false
	}

	if cf.IsNotFoundError(err) {
		return false
	}

	logger.Error(err, "Failed to clear ruleset rules")
	r.Recorder.Event(ruleset, corev1.EventTypeWarning, "DeleteFailed", cf.SanitizeErrorMessage(err))
	return true
}

// registerRuleset registers the ZoneRuleset configuration to SyncState.
// The actual sync to Cloudflare is handled by ZoneRulesetSyncController.
func (r *Reconciler) registerRuleset(
	ctx context.Context,
	ruleset *networkingv1alpha2.ZoneRuleset,
	accountID, zoneID string,
	credRef networkingv1alpha2.CredentialsReference,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Build rules configuration
	rules := make([]rulesetsvc.RulesetRuleConfig, len(ruleset.Spec.Rules))
	for i, rule := range ruleset.Spec.Rules {
		rules[i] = rulesetsvc.RulesetRuleConfig{
			Action:           string(rule.Action),
			Expression:       rule.Expression,
			Description:      rule.Description,
			Enabled:          rule.Enabled,
			Ref:              rule.Ref,
			ActionParameters: rule.ActionParameters,
			RateLimit:        rule.RateLimit,
		}
	}

	// Build ruleset configuration
	config := rulesetsvc.ZoneRulesetConfig{
		Zone:        ruleset.Spec.Zone,
		Phase:       string(ruleset.Spec.Phase),
		Description: ruleset.Spec.Description,
		Rules:       rules,
	}

	// Create source reference
	source := service.Source{
		Kind:      "ZoneRuleset",
		Namespace: ruleset.Namespace,
		Name:      ruleset.Name,
	}

	// Register to SyncState
	opts := rulesetsvc.ZoneRulesetRegisterOptions{
		AccountID:      accountID,
		ZoneID:         zoneID,
		RulesetID:      ruleset.Status.RulesetID, // May be empty for new rulesets
		Source:         source,
		Config:         config,
		CredentialsRef: credRef,
	}

	if err := r.zoneRulesetService.Register(ctx, opts); err != nil {
		logger.Error(err, "Failed to register ZoneRuleset configuration")
		r.Recorder.Event(ruleset, corev1.EventTypeWarning, "RegisterFailed",
			fmt.Sprintf("Failed to register ZoneRuleset: %s", err.Error()))
		return r.updateStatusError(ctx, ruleset, err)
	}

	r.Recorder.Event(ruleset, corev1.EventTypeNormal, "Registered",
		fmt.Sprintf("Registered ZoneRuleset for zone '%s' phase '%s' to SyncState",
			ruleset.Spec.Zone, ruleset.Spec.Phase))

	// Update status to Pending - actual sync happens via ZoneRulesetSyncController
	return r.updateStatusPending(ctx, ruleset, zoneID)
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
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		ruleset.Status.ObservedGeneration = ruleset.Generation
	})

	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *Reconciler) updateStatusPending(
	ctx context.Context,
	ruleset *networkingv1alpha2.ZoneRuleset,
	zoneID string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, ruleset, func() {
		ruleset.Status.State = networkingv1alpha2.ZoneRulesetStateSyncing
		ruleset.Status.Message = "ZoneRuleset configuration registered, waiting for sync"
		ruleset.Status.ZoneID = zoneID
		meta.SetStatusCondition(&ruleset.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: ruleset.Generation,
			Reason:             "Pending",
			Message:            "ZoneRuleset configuration registered, waiting for sync",
			LastTransitionTime: metav1.Now(),
		})
		ruleset.Status.ObservedGeneration = ruleset.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	// Requeue to check for sync completion
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
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

// findRulesetsForSyncState returns ZoneRulesets that are sources for the given SyncState
func (*Reconciler) findRulesetsForSyncState(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	syncState, ok := obj.(*networkingv1alpha2.CloudflareSyncState)
	if !ok {
		return nil
	}

	// Only process ZoneRuleset type SyncStates
	if syncState.Spec.ResourceType != networkingv1alpha2.SyncResourceZoneRuleset {
		return nil
	}

	var requests []reconcile.Request
	for _, source := range syncState.Spec.Sources {
		if source.Ref.Kind == "ZoneRuleset" {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      source.Ref.Name,
					Namespace: source.Ref.Namespace,
				},
			})
		}
	}

	logger.V(1).Info("Found ZoneRulesets for SyncState update",
		"syncState", syncState.Name,
		"rulesetCount", len(requests))

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("zoneruleset-controller")

	// Initialize ZoneRulesetService
	r.zoneRulesetService = rulesetsvc.NewZoneRulesetService(mgr.GetClient())

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.ZoneRuleset{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findRulesetsForCredentials)).
		Watches(&networkingv1alpha2.CloudflareSyncState{},
			handler.EnqueueRequestsFromMapFunc(r.findRulesetsForSyncState)).
		Named("zoneruleset").
		Complete(r)
}
