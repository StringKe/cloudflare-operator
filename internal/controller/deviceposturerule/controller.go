/*
Copyright 2025 Adyanth H.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package deviceposturerule

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/adyanth/cloudflare-operator/api/v1alpha2"
	"github.com/adyanth/cloudflare-operator/internal/clients/cf"
)

const (
	FinalizerName = "deviceposturerule.networking.cloudflare-operator.io/finalizer"
)

// DevicePostureRuleReconciler reconciles a DevicePostureRule object
type DevicePostureRuleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=deviceposturerules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=deviceposturerules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=deviceposturerules/finalizers,verbs=update

func (r *DevicePostureRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the DevicePostureRule instance
	rule := &networkingv1alpha2.DevicePostureRule{}
	if err := r.Get(ctx, req.NamespacedName, rule); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Initialize API client
	apiClient, err := cf.NewAPIClientFromDetails(ctx, r.Client, "", rule.Spec.Cloudflare)
	if err != nil {
		logger.Error(err, "Failed to initialize Cloudflare API client")
		return r.updateStatusError(ctx, rule, err)
	}

	// Handle deletion
	if !rule.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, rule, apiClient)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(rule, FinalizerName) {
		controllerutil.AddFinalizer(rule, FinalizerName)
		if err := r.Update(ctx, rule); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile the device posture rule
	return r.reconcileDevicePostureRule(ctx, rule, apiClient)
}

func (r *DevicePostureRuleReconciler) handleDeletion(ctx context.Context, rule *networkingv1alpha2.DevicePostureRule, apiClient *cf.API) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(rule, FinalizerName) {
		// Delete from Cloudflare
		if rule.Status.RuleID != "" {
			logger.Info("Deleting Device Posture Rule from Cloudflare", "ruleId", rule.Status.RuleID)
			if err := apiClient.DeleteDevicePostureRule(rule.Status.RuleID); err != nil {
				logger.Error(err, "Failed to delete Device Posture Rule from Cloudflare")
			}
		}

		// Remove finalizer
		controllerutil.RemoveFinalizer(rule, FinalizerName)
		if err := r.Update(ctx, rule); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *DevicePostureRuleReconciler) reconcileDevicePostureRule(ctx context.Context, rule *networkingv1alpha2.DevicePostureRule, apiClient *cf.API) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Build device posture rule params
	params := cf.DevicePostureRuleParams{
		Name:        rule.GetRuleName(),
		Type:        rule.Spec.Type,
		Description: rule.Spec.Description,
		Schedule:    rule.Spec.Schedule,
		Expiration:  rule.Spec.Expiration,
	}

	// Build match rules
	if len(rule.Spec.Match) > 0 {
		params.Match = make([]map[string]interface{}, 0, len(rule.Spec.Match))
		for _, m := range rule.Spec.Match {
			params.Match = append(params.Match, map[string]interface{}{"platform": m.Platform})
		}
	}

	// Build input
	if rule.Spec.Input != nil {
		params.Input = r.buildInput(rule.Spec.Input)
	}

	var result *cf.DevicePostureRuleResult
	var err error

	if rule.Status.RuleID == "" {
		// Create new device posture rule
		logger.Info("Creating Device Posture Rule", "name", params.Name, "type", params.Type)
		result, err = apiClient.CreateDevicePostureRule(params)
	} else {
		// Update existing device posture rule
		logger.Info("Updating Device Posture Rule", "ruleId", rule.Status.RuleID)
		result, err = apiClient.UpdateDevicePostureRule(rule.Status.RuleID, params)
	}

	if err != nil {
		return r.updateStatusError(ctx, rule, err)
	}

	// Update status
	return r.updateStatusSuccess(ctx, rule, result)
}

func (r *DevicePostureRuleReconciler) buildInput(input *networkingv1alpha2.DevicePostureInput) map[string]interface{} {
	result := make(map[string]interface{})

	if input.ID != "" {
		result["id"] = input.ID
	}
	if input.Path != "" {
		result["path"] = input.Path
	}
	if input.Exists != nil {
		result["exists"] = *input.Exists
	}
	if input.Sha256 != "" {
		result["sha256"] = input.Sha256
	}
	if input.Thumbprint != "" {
		result["thumbprint"] = input.Thumbprint
	}
	if input.Running != nil {
		result["running"] = *input.Running
	}
	if input.RequireAll != nil {
		result["require_all"] = *input.RequireAll
	}
	if input.Enabled != nil {
		result["enabled"] = *input.Enabled
	}
	if input.Version != "" {
		result["version"] = input.Version
	}
	if input.Operator != "" {
		result["operator"] = input.Operator
	}
	if input.Domain != "" {
		result["domain"] = input.Domain
	}
	if input.ComplianceStatus != "" {
		result["compliance_status"] = input.ComplianceStatus
	}
	if input.ConnectionID != "" {
		result["connection_id"] = input.ConnectionID
	}
	if input.LastSeen != "" {
		result["last_seen"] = input.LastSeen
	}
	if input.ActiveThreats != nil {
		result["active_threats"] = *input.ActiveThreats
	}
	if input.NetworkStatus != "" {
		result["network_status"] = input.NetworkStatus
	}
	if input.SensorConfig != "" {
		result["sensor_config"] = input.SensorConfig
	}
	if input.VersionOperator != "" {
		result["version_operator"] = input.VersionOperator
	}
	if input.CountOperator != "" {
		result["count_operator"] = input.CountOperator
	}
	if input.IssueCount != nil {
		result["issue_count"] = *input.IssueCount
	}
	if input.OSDistroName != "" {
		result["os_distro_name"] = input.OSDistroName
	}
	if input.OSDistroRevision != "" {
		result["os_distro_revision"] = input.OSDistroRevision
	}
	if input.CertificateID != "" {
		result["certificate_id"] = input.CertificateID
	}
	if input.CommonName != "" {
		result["common_name"] = input.CommonName
	}
	if len(input.CheckDisks) > 0 {
		result["check_disks"] = input.CheckDisks
	}

	return result
}

func (r *DevicePostureRuleReconciler) updateStatusError(ctx context.Context, rule *networkingv1alpha2.DevicePostureRule, err error) (ctrl.Result, error) {
	rule.Status.State = "Error"
	meta.SetStatusCondition(&rule.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "ReconcileError",
		Message:            err.Error(),
		LastTransitionTime: metav1.Now(),
	})
	rule.Status.ObservedGeneration = rule.Generation

	if updateErr := r.Status().Update(ctx, rule); updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *DevicePostureRuleReconciler) updateStatusSuccess(ctx context.Context, rule *networkingv1alpha2.DevicePostureRule, result *cf.DevicePostureRuleResult) (ctrl.Result, error) {
	rule.Status.RuleID = result.ID
	rule.Status.AccountID = result.AccountID
	rule.Status.State = "Ready"
	meta.SetStatusCondition(&rule.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "Device Posture Rule successfully reconciled",
		LastTransitionTime: metav1.Now(),
	})
	rule.Status.ObservedGeneration = rule.Generation

	if err := r.Status().Update(ctx, rule); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *DevicePostureRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.DevicePostureRule{}).
		Complete(r)
}
