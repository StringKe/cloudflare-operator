// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package deviceposturerule provides a controller for managing Cloudflare Device Posture Rules.
// It directly calls Cloudflare API and writes status back to the CRD.
package deviceposturerule

import (
	"context"
	"fmt"

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
	"github.com/StringKe/cloudflare-operator/internal/controller/common"
)

const (
	finalizerName = "deviceposturerule.networking.cloudflare-operator.io/finalizer"
)

// Reconciler reconciles a DevicePostureRule object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=deviceposturerules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=deviceposturerules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=deviceposturerules/finalizers,verbs=update

// Reconcile handles DevicePostureRule reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the DevicePostureRule resource
	rule := &networkingv1alpha2.DevicePostureRule{}
	if err := r.Get(ctx, req.NamespacedName, rule); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch DevicePostureRule")
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
	// DevicePostureRule is cluster-scoped, use operator namespace for legacy inline secrets
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &rule.Spec.Cloudflare,
		Namespace:         common.OperatorNamespace,
		StatusAccountID:   rule.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, rule, err)
	}

	// Sync Device Posture Rule to Cloudflare
	return r.syncDevicePostureRule(ctx, rule, apiResult)
}

// handleDeletion handles the deletion of DevicePostureRule.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	rule *networkingv1alpha2.DevicePostureRule,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(rule, finalizerName) {
		return common.NoRequeue(), nil
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &rule.Spec.Cloudflare,
		Namespace:         common.OperatorNamespace,
		StatusAccountID:   rule.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client for deletion")
		// Continue with finalizer removal
	} else if rule.Status.RuleID != "" {
		// Delete Device Posture Rule from Cloudflare
		logger.Info("Deleting Device Posture Rule from Cloudflare",
			"ruleId", rule.Status.RuleID)

		if err := apiResult.API.DeleteDevicePostureRule(ctx, rule.Status.RuleID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete Device Posture Rule from Cloudflare, continuing with finalizer removal")
				r.Recorder.Event(rule, corev1.EventTypeWarning, "DeleteFailed",
					fmt.Sprintf("Failed to delete from Cloudflare (will remove finalizer anyway): %s", cf.SanitizeErrorMessage(err)))
				// Don't block finalizer removal - resource may need manual cleanup in Cloudflare
			} else {
				logger.Info("Device Posture Rule not found in Cloudflare, may have been already deleted")
			}
		} else {
			r.Recorder.Event(rule, corev1.EventTypeNormal, "Deleted",
				"Device Posture Rule deleted from Cloudflare")
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

// syncDevicePostureRule syncs the Device Posture Rule to Cloudflare.
func (r *Reconciler) syncDevicePostureRule(
	ctx context.Context,
	rule *networkingv1alpha2.DevicePostureRule,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Determine rule name
	ruleName := rule.GetRuleName()

	// Build params
	params := r.buildParams(rule, ruleName)

	// Check if rule already exists by ID
	if rule.Status.RuleID != "" {
		existing, err := apiResult.API.GetDevicePostureRule(ctx, rule.Status.RuleID)
		if err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to get Device Posture Rule from Cloudflare")
				return r.updateStatusError(ctx, rule, err)
			}
			// Rule doesn't exist, will create
			logger.Info("Device Posture Rule not found in Cloudflare, will recreate",
				"ruleId", rule.Status.RuleID)
		} else {
			// Rule exists, update it
			logger.V(1).Info("Updating Device Posture Rule in Cloudflare",
				"ruleId", existing.ID,
				"name", ruleName)

			result, err := apiResult.API.UpdateDevicePostureRule(ctx, existing.ID, params)
			if err != nil {
				logger.Error(err, "Failed to update Device Posture Rule")
				return r.updateStatusError(ctx, rule, err)
			}

			r.Recorder.Event(rule, corev1.EventTypeNormal, "Updated",
				fmt.Sprintf("Device Posture Rule '%s' updated in Cloudflare", ruleName))

			return r.updateStatusReady(ctx, rule, apiResult.AccountID, result.ID)
		}
	}

	// Try to find existing rule by name
	existingByName, err := apiResult.API.ListDevicePostureRulesByName(ctx, ruleName)
	if err != nil && !cf.IsNotFoundError(err) {
		logger.Error(err, "Failed to search for existing Device Posture Rule")
		return r.updateStatusError(ctx, rule, err)
	}

	if existingByName != nil {
		// Rule already exists with this name, adopt it
		logger.Info("Device Posture Rule already exists with same name, adopting it",
			"ruleId", existingByName.ID,
			"name", ruleName)

		// Update the existing rule
		result, err := apiResult.API.UpdateDevicePostureRule(ctx, existingByName.ID, params)
		if err != nil {
			logger.Error(err, "Failed to update existing Device Posture Rule")
			return r.updateStatusError(ctx, rule, err)
		}

		r.Recorder.Event(rule, corev1.EventTypeNormal, "Adopted",
			fmt.Sprintf("Adopted existing Device Posture Rule '%s'", ruleName))

		return r.updateStatusReady(ctx, rule, apiResult.AccountID, result.ID)
	}

	// Create new rule
	logger.Info("Creating Device Posture Rule in Cloudflare",
		"name", ruleName)

	result, err := apiResult.API.CreateDevicePostureRule(ctx, params)
	if err != nil {
		logger.Error(err, "Failed to create Device Posture Rule")
		return r.updateStatusError(ctx, rule, err)
	}

	r.Recorder.Event(rule, corev1.EventTypeNormal, "Created",
		fmt.Sprintf("Device Posture Rule '%s' created in Cloudflare", ruleName))

	return r.updateStatusReady(ctx, rule, apiResult.AccountID, result.ID)
}

// buildParams builds the DevicePostureRuleParams from the spec.
func (r *Reconciler) buildParams(rule *networkingv1alpha2.DevicePostureRule, ruleName string) cf.DevicePostureRuleParams {
	params := cf.DevicePostureRuleParams{
		Name:        ruleName,
		Type:        rule.Spec.Type,
		Description: rule.Spec.Description,
		Schedule:    rule.Spec.Schedule,
		Expiration:  rule.Spec.Expiration,
	}

	// Build match rules
	for _, m := range rule.Spec.Match {
		params.Match = append(params.Match, cf.DevicePostureMatchParams{
			Platform: m.Platform,
		})
	}

	// Build input
	if rule.Spec.Input != nil {
		params.Input = r.buildInput(rule.Spec.Input)
	}

	return params
}

// buildInput builds the DevicePostureInputParams from the spec input.
func (r *Reconciler) buildInput(input *networkingv1alpha2.DevicePostureInput) *cf.DevicePostureInputParams {
	if input == nil {
		return nil
	}

	result := &cf.DevicePostureInputParams{
		ID:               input.ID,
		Path:             input.Path,
		Exists:           input.Exists,
		Sha256:           input.Sha256,
		Thumbprint:       input.Thumbprint,
		Running:          input.Running,
		RequireAll:       input.RequireAll,
		Enabled:          input.Enabled,
		Version:          input.Version,
		Operator:         input.Operator,
		Domain:           input.Domain,
		ComplianceStatus: input.ComplianceStatus,
		ConnectionID:     input.ConnectionID,
		LastSeen:         input.LastSeen,
		EidLastSeen:      input.EidLastSeen,
		ActiveThreats:    input.ActiveThreats,
		Infected:         input.Infected,
		IsActive:         input.IsActive,
		NetworkStatus:    input.NetworkStatus,
		SensorConfig:     input.SensorConfig,
		VersionOperator:  input.VersionOperator,
		CountOperator:    input.CountOperator,
		ScoreOperator:    input.ScoreOperator,
		IssueCount:       input.IssueCount,
		Score:            input.Score,
		TotalScore:       input.TotalScore,
		RiskLevel:        input.RiskLevel,
		Overall:          input.Overall,
		State:            input.State,
		OperationalState: input.OperationalState,
		OSDistroName:     input.OSDistroName,
		OSDistroRevision: input.OSDistroRevision,
		OSVersionExtra:   input.OSVersionExtra,
		OS:               input.OS,
		OperatingSystem:  input.OperatingSystem,
		CertificateID:    input.CertificateID,
		CommonName:       input.CommonName,
		Cn:               input.Cn,
		CheckPrivateKey:  input.CheckPrivateKey,
		ExtendedKeyUsage: input.ExtendedKeyUsage,
		CheckDisks:       input.CheckDisks,
	}

	// Convert locations
	if len(input.Locations) > 0 {
		result.Locations = make([]cf.DevicePostureLocationParams, len(input.Locations))
		for i, loc := range input.Locations {
			result.Locations[i] = cf.DevicePostureLocationParams{
				Paths:       loc.Paths,
				TrustStores: loc.TrustStores,
			}
		}
	}

	return result
}

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	rule *networkingv1alpha2.DevicePostureRule,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, rule, func() {
		rule.Status.State = "Error"
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
	rule *networkingv1alpha2.DevicePostureRule,
	accountID, ruleID string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, rule, func() {
		rule.Status.AccountID = accountID
		rule.Status.RuleID = ruleID
		rule.Status.State = "Ready"
		meta.SetStatusCondition(&rule.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: rule.Generation,
			Reason:             "Synced",
			Message:            "Device Posture Rule synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		rule.Status.ObservedGeneration = rule.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("deviceposturerule-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("deviceposturerule"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.DevicePostureRule{}).
		Named("deviceposturerule").
		Complete(r)
}
