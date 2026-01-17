// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package deviceposturerule

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
	devicesvc "github.com/StringKe/cloudflare-operator/internal/service/device"
)

const (
	FinalizerName = "deviceposturerule.networking.cloudflare-operator.io/finalizer"
)

// DevicePostureRuleReconciler reconciles a DevicePostureRule object
type DevicePostureRuleReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	deviceService *devicesvc.DevicePostureRuleService
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=deviceposturerules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=deviceposturerules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=deviceposturerules/finalizers,verbs=update

func (r *DevicePostureRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the DevicePostureRule instance
	rule := &networkingv1alpha2.DevicePostureRule{}
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

	// Register Device Posture Rule configuration to SyncState
	return r.registerDevicePostureRule(ctx, rule, accountID, credRef)
}

// resolveCredentials resolves the credentials reference and account ID.
func (r *DevicePostureRuleReconciler) resolveCredentials(
	ctx context.Context,
	rule *networkingv1alpha2.DevicePostureRule,
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

func (r *DevicePostureRuleReconciler) handleDeletion(
	ctx context.Context,
	rule *networkingv1alpha2.DevicePostureRule,
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
		Kind:      "DevicePostureRule",
		Namespace: "",
		Name:      rule.Name,
	}
	if err := r.deviceService.Unregister(ctx, rule.Status.RuleID, source); err != nil {
		logger.Error(err, "Failed to unregister Device Posture Rule from SyncState")
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

// deleteFromCloudflare deletes the device posture rule from Cloudflare.
// Returns (result, shouldReturn) where shouldReturn indicates if caller should return.
func (r *DevicePostureRuleReconciler) deleteFromCloudflare(
	ctx context.Context,
	rule *networkingv1alpha2.DevicePostureRule,
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

	logger.Info("Deleting Device Posture Rule from Cloudflare", "ruleId", rule.Status.RuleID)

	err = apiClient.DeleteDevicePostureRule(rule.Status.RuleID)
	if err == nil {
		r.Recorder.Event(rule, corev1.EventTypeNormal, controller.EventReasonDeleted, "Deleted from Cloudflare")
		return ctrl.Result{}, false
	}

	if cf.IsNotFoundError(err) {
		logger.Info("Device Posture Rule already deleted from Cloudflare")
		r.Recorder.Event(rule, corev1.EventTypeNormal, "AlreadyDeleted",
			"Device Posture Rule was already deleted from Cloudflare")
		return ctrl.Result{}, false
	}

	logger.Error(err, "Failed to delete Device Posture Rule from Cloudflare")
	r.Recorder.Event(rule, corev1.EventTypeWarning, controller.EventReasonDeleteFailed,
		fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
	return ctrl.Result{RequeueAfter: 30 * time.Second}, true
}

// registerDevicePostureRule registers the Device Posture Rule configuration to SyncState.
func (r *DevicePostureRuleReconciler) registerDevicePostureRule(
	ctx context.Context,
	rule *networkingv1alpha2.DevicePostureRule,
	accountID string,
	credRef networkingv1alpha2.CredentialsReference,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Build device posture rule configuration
	config := devicesvc.DevicePostureRuleConfig{
		Name:        rule.GetRuleName(),
		Type:        rule.Spec.Type,
		Description: rule.Spec.Description,
		Schedule:    rule.Spec.Schedule,
		Expiration:  rule.Spec.Expiration,
	}

	// Build match rules
	for _, m := range rule.Spec.Match {
		config.Match = append(config.Match, devicesvc.DevicePostureMatch{
			Platform: m.Platform,
		})
	}

	// Build input
	if rule.Spec.Input != nil {
		config.Input = r.buildInput(rule.Spec.Input)
	}

	// Create source reference
	source := service.Source{
		Kind:      "DevicePostureRule",
		Namespace: "",
		Name:      rule.Name,
	}

	// Register to SyncState
	opts := devicesvc.DevicePostureRuleRegisterOptions{
		AccountID:      accountID,
		RuleID:         rule.Status.RuleID,
		Source:         source,
		Config:         config,
		CredentialsRef: credRef,
	}

	if err := r.deviceService.Register(ctx, opts); err != nil {
		logger.Error(err, "Failed to register Device Posture Rule configuration")
		r.Recorder.Event(rule, corev1.EventTypeWarning, "RegisterFailed",
			fmt.Sprintf("Failed to register Device Posture Rule: %s", err.Error()))
		return r.updateStatusError(ctx, rule, err)
	}

	r.Recorder.Event(rule, corev1.EventTypeNormal, "Registered",
		fmt.Sprintf("Registered Device Posture Rule '%s' configuration to SyncState", config.Name))

	// Update status to Pending - actual sync happens via DeviceSyncController
	return r.updateStatusPending(ctx, rule, accountID)
}

func (*DevicePostureRuleReconciler) buildInput(input *networkingv1alpha2.DevicePostureInput) *devicesvc.DevicePostureInput {
	if input == nil {
		return nil
	}

	return &devicesvc.DevicePostureInput{
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
}

func (r *DevicePostureRuleReconciler) updateStatusError(ctx context.Context, rule *networkingv1alpha2.DevicePostureRule, err error) (ctrl.Result, error) {
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

func (r *DevicePostureRuleReconciler) updateStatusPending(
	ctx context.Context,
	rule *networkingv1alpha2.DevicePostureRule,
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
			Message:            "Device Posture Rule configuration registered, waiting for sync",
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

func (r *DevicePostureRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("deviceposturerule-controller")

	// Initialize DevicePostureRuleService
	r.deviceService = devicesvc.NewDevicePostureRuleService(mgr.GetClient())

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.DevicePostureRule{}).
		Complete(r)
}
