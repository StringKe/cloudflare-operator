// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package deviceposturerule

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

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

	// Runtime state
	ctx  context.Context
	log  logr.Logger
	rule *networkingv1alpha2.DevicePostureRule
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=deviceposturerules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=deviceposturerules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=deviceposturerules/finalizers,verbs=update

func (r *DevicePostureRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrllog.FromContext(ctx)

	// Fetch the DevicePostureRule instance
	r.rule = &networkingv1alpha2.DevicePostureRule{}
	if err := r.Get(ctx, req.NamespacedName, r.rule); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Resolve credentials
	credInfo, err := r.resolveCredentials()
	if err != nil {
		r.log.Error(err, "Failed to resolve credentials")
		return r.updateStatusError(err)
	}

	// Handle deletion
	if !r.rule.DeletionTimestamp.IsZero() {
		return r.handleDeletion(credInfo)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.rule, FinalizerName) {
		controllerutil.AddFinalizer(r.rule, FinalizerName)
		if err := r.Update(ctx, r.rule); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Register Device Posture Rule configuration to SyncState
	return r.registerDevicePostureRule(credInfo)
}

// resolveCredentials resolves the credentials reference and returns the credentials info.
// Following Unified Sync Architecture, the Resource Controller only needs
// credential metadata (accountID, credRef) - it does not create a Cloudflare API client.
func (r *DevicePostureRuleReconciler) resolveCredentials() (*controller.CredentialsInfo, error) {
	// DevicePostureRule is cluster-scoped, use operator namespace for legacy inline secrets
	info, err := controller.ResolveCredentialsForService(
		r.ctx,
		r.Client,
		r.log,
		r.rule.Spec.Cloudflare,
		controller.OperatorNamespace,
		r.rule.Status.AccountID,
	)
	if err != nil {
		r.log.Error(err, "failed to resolve credentials")
		r.Recorder.Event(r.rule, corev1.EventTypeWarning, controller.EventReasonAPIError,
			"Failed to resolve credentials: "+err.Error())
		return nil, err
	}

	return info, nil
}

// handleDeletion handles the deletion of a DevicePostureRule.
// Following Unified Sync Architecture: Resource Controller only unregisters from SyncState,
// the L5 Sync Controller handles actual Cloudflare API deletion.
func (r *DevicePostureRuleReconciler) handleDeletion(
	_ *controller.CredentialsInfo,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.rule, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Unregister from SyncState - L5 Sync Controller will handle Cloudflare deletion
	source := service.Source{
		Kind:      "DevicePostureRule",
		Namespace: "",
		Name:      r.rule.Name,
	}
	if err := r.deviceService.Unregister(r.ctx, r.rule.Status.RuleID, source); err != nil {
		r.log.Error(err, "Failed to unregister Device Posture Rule from SyncState")
		r.Recorder.Event(r.rule, corev1.EventTypeWarning, "UnregisterFailed",
			fmt.Sprintf("Failed to unregister from SyncState: %s", err.Error()))
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	r.log.Info("Unregistered Device Posture Rule from SyncState, L5 Sync Controller will handle Cloudflare deletion")
	r.Recorder.Event(r.rule, corev1.EventTypeNormal, "Unregistered", "Unregistered from SyncState")

	// Remove finalizer with retry logic
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.rule, func() {
		controllerutil.RemoveFinalizer(r.rule, FinalizerName)
	}); err != nil {
		r.log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(r.rule, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// registerDevicePostureRule registers the Device Posture Rule configuration to SyncState.
func (r *DevicePostureRuleReconciler) registerDevicePostureRule(
	credInfo *controller.CredentialsInfo,
) (ctrl.Result, error) {
	// Build device posture rule configuration
	config := devicesvc.DevicePostureRuleConfig{
		Name:        r.rule.GetRuleName(),
		Type:        r.rule.Spec.Type,
		Description: r.rule.Spec.Description,
		Schedule:    r.rule.Spec.Schedule,
		Expiration:  r.rule.Spec.Expiration,
	}

	// Build match rules
	for _, m := range r.rule.Spec.Match {
		config.Match = append(config.Match, devicesvc.DevicePostureMatch{
			Platform: m.Platform,
		})
	}

	// Build input
	if r.rule.Spec.Input != nil {
		config.Input = r.buildInput(r.rule.Spec.Input)
	}

	// Create source reference
	source := service.Source{
		Kind:      "DevicePostureRule",
		Namespace: "",
		Name:      r.rule.Name,
	}

	// Register to SyncState
	opts := devicesvc.DevicePostureRuleRegisterOptions{
		AccountID:      credInfo.AccountID,
		RuleID:         r.rule.Status.RuleID,
		Source:         source,
		Config:         config,
		CredentialsRef: credInfo.CredentialsRef,
	}

	if err := r.deviceService.Register(r.ctx, opts); err != nil {
		r.log.Error(err, "Failed to register Device Posture Rule configuration")
		r.Recorder.Event(r.rule, corev1.EventTypeWarning, "RegisterFailed",
			fmt.Sprintf("Failed to register Device Posture Rule: %s", err.Error()))
		return r.updateStatusError(err)
	}

	r.Recorder.Event(r.rule, corev1.EventTypeNormal, "Registered",
		fmt.Sprintf("Registered Device Posture Rule '%s' configuration to SyncState", config.Name))

	// Update status to Pending - actual sync happens via DeviceSyncController
	return r.updateStatusPending(credInfo.AccountID)
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

func (r *DevicePostureRuleReconciler) updateStatusError(err error) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.rule, func() {
		r.rule.Status.State = "Error"
		meta.SetStatusCondition(&r.rule.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: r.rule.Generation,
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		r.rule.Status.ObservedGeneration = r.rule.Generation
	})
	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *DevicePostureRuleReconciler) updateStatusPending(
	accountID string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.rule, func() {
		if r.rule.Status.AccountID == "" {
			r.rule.Status.AccountID = accountID
		}
		r.rule.Status.State = "Pending"
		meta.SetStatusCondition(&r.rule.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: r.rule.Generation,
			Reason:             "Pending",
			Message:            "Device Posture Rule configuration registered, waiting for sync",
			LastTransitionTime: metav1.Now(),
		})
		r.rule.Status.ObservedGeneration = r.rule.Generation
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
