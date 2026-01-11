// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package accessgroup

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
)

const (
	FinalizerName = "accessgroup.networking.cloudflare-operator.io/finalizer"
)

// AccessGroupReconciler reconciles an AccessGroup object
type AccessGroupReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessgroups/finalizers,verbs=update

func (r *AccessGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the AccessGroup instance
	accessGroup := &networkingv1alpha2.AccessGroup{}
	if err := r.Get(ctx, req.NamespacedName, accessGroup); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Initialize API client
	// AccessGroup is cluster-scoped, use operator namespace for legacy inline secrets
	apiClient, err := cf.NewAPIClientFromDetails(ctx, r.Client, controller.OperatorNamespace, accessGroup.Spec.Cloudflare)
	if err != nil {
		logger.Error(err, "Failed to initialize Cloudflare API client")
		return r.updateStatusError(ctx, accessGroup, err)
	}

	// Handle deletion
	if !accessGroup.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, accessGroup, apiClient)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(accessGroup, FinalizerName) {
		controllerutil.AddFinalizer(accessGroup, FinalizerName)
		if err := r.Update(ctx, accessGroup); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile the access group
	return r.reconcileAccessGroup(ctx, accessGroup, apiClient)
}

func (r *AccessGroupReconciler) handleDeletion(ctx context.Context, accessGroup *networkingv1alpha2.AccessGroup, apiClient *cf.API) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(accessGroup, FinalizerName) {
		// Delete from Cloudflare
		if accessGroup.Status.GroupID != "" {
			logger.Info("Deleting Access Group from Cloudflare", "groupId", accessGroup.Status.GroupID)
			if err := apiClient.DeleteAccessGroup(accessGroup.Status.GroupID); err != nil {
				// P0 FIX: Check if resource already deleted
				if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to delete Access Group from Cloudflare")
					r.Recorder.Event(accessGroup, corev1.EventTypeWarning, controller.EventReasonDeleteFailed,
						fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
					return ctrl.Result{RequeueAfter: 30 * time.Second}, err
				}
				logger.Info("Access Group already deleted from Cloudflare")
				r.Recorder.Event(accessGroup, corev1.EventTypeNormal, "AlreadyDeleted", "Access Group was already deleted from Cloudflare")
			} else {
				r.Recorder.Event(accessGroup, corev1.EventTypeNormal, controller.EventReasonDeleted, "Deleted from Cloudflare")
			}
		}

		// P0 FIX: Remove finalizer with retry logic to handle conflicts
		if err := controller.UpdateWithConflictRetry(ctx, r.Client, accessGroup, func() {
			controllerutil.RemoveFinalizer(accessGroup, FinalizerName)
		}); err != nil {
			logger.Error(err, "failed to remove finalizer")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(accessGroup, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")
	}

	return ctrl.Result{}, nil
}

func (r *AccessGroupReconciler) reconcileAccessGroup(ctx context.Context, accessGroup *networkingv1alpha2.AccessGroup, apiClient *cf.API) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Use the Name from the spec, or fall back to the resource name
	groupName := accessGroup.Spec.Name
	if groupName == "" {
		groupName = accessGroup.Name
	}

	// Build access group params
	params := cf.AccessGroupParams{
		Name: groupName,
	}

	// Build include, exclude, require rules from spec
	params.Include = r.buildGroupRules(accessGroup.Spec.Include)
	params.Exclude = r.buildGroupRules(accessGroup.Spec.Exclude)
	params.Require = r.buildGroupRules(accessGroup.Spec.Require)

	var result *cf.AccessGroupResult
	var err error

	if accessGroup.Status.GroupID == "" {
		// Create new access group
		logger.Info("Creating Access Group", "name", params.Name)
		r.Recorder.Event(accessGroup, corev1.EventTypeNormal, "Creating",
			fmt.Sprintf("Creating Access Group '%s' in Cloudflare", params.Name))
		result, err = apiClient.CreateAccessGroup(params)
		if err != nil {
			r.Recorder.Event(accessGroup, corev1.EventTypeWarning, controller.EventReasonCreateFailed,
				fmt.Sprintf("Failed to create Access Group: %s", cf.SanitizeErrorMessage(err)))
			return r.updateStatusError(ctx, accessGroup, err)
		}
		r.Recorder.Event(accessGroup, corev1.EventTypeNormal, controller.EventReasonCreated,
			fmt.Sprintf("Created Access Group with ID '%s'", result.ID))
	} else {
		// Update existing access group
		logger.Info("Updating Access Group", "groupId", accessGroup.Status.GroupID)
		r.Recorder.Event(accessGroup, corev1.EventTypeNormal, "Updating",
			fmt.Sprintf("Updating Access Group '%s' in Cloudflare", accessGroup.Status.GroupID))
		result, err = apiClient.UpdateAccessGroup(accessGroup.Status.GroupID, params)
		if err != nil {
			r.Recorder.Event(accessGroup, corev1.EventTypeWarning, controller.EventReasonUpdateFailed,
				fmt.Sprintf("Failed to update Access Group: %s", cf.SanitizeErrorMessage(err)))
			return r.updateStatusError(ctx, accessGroup, err)
		}
		r.Recorder.Event(accessGroup, corev1.EventTypeNormal, controller.EventReasonUpdated,
			fmt.Sprintf("Updated Access Group '%s'", result.ID))
	}

	// Update status
	return r.updateStatusSuccess(ctx, accessGroup, result)
}

// buildGroupRules converts CRD group rules to API params.
//
//nolint:revive // cognitive complexity is acceptable for this conversion
func (*AccessGroupReconciler) buildGroupRules(rules []networkingv1alpha2.AccessGroupRule) []cf.AccessGroupRuleParams {
	if len(rules) == 0 {
		return nil
	}

	result := make([]cf.AccessGroupRuleParams, 0, len(rules))
	for _, rule := range rules {
		params := cf.AccessGroupRuleParams{}
		hasRule := false

		if rule.Email != nil {
			params.Email = &cf.AccessGroupEmailRuleParams{Email: rule.Email.Email}
			hasRule = true
		}
		if rule.EmailDomain != nil {
			params.EmailDomain = &cf.AccessGroupEmailDomainRuleParams{Domain: rule.EmailDomain.Domain}
			hasRule = true
		}
		if rule.EmailList != nil {
			params.EmailList = &cf.AccessGroupEmailListRuleParams{ID: rule.EmailList.ID}
			hasRule = true
		}
		if rule.IPRanges != nil && len(rule.IPRanges.IP) > 0 {
			params.IPRanges = &cf.AccessGroupIPRangesRuleParams{IP: rule.IPRanges.IP}
			hasRule = true
		}
		if rule.IPList != nil {
			params.IPList = &cf.AccessGroupIPListRuleParams{ID: rule.IPList.ID}
			hasRule = true
		}
		if rule.Everyone {
			params.Everyone = true
			hasRule = true
		}
		if rule.Group != nil {
			params.Group = &cf.AccessGroupGroupRuleParams{ID: rule.Group.ID}
			hasRule = true
		}
		if rule.AnyValidServiceToken {
			params.AnyValidServiceToken = true
			hasRule = true
		}
		if rule.ServiceToken != nil {
			params.ServiceToken = &cf.AccessGroupServiceTokenRuleParams{TokenID: rule.ServiceToken.TokenID}
			hasRule = true
		}
		if rule.ExternalEvaluation != nil {
			params.ExternalEvaluation = &cf.AccessGroupExternalEvaluationRuleParams{
				EvaluateURL: rule.ExternalEvaluation.EvaluateURL,
				KeysURL:     rule.ExternalEvaluation.KeysURL,
			}
			hasRule = true
		}
		if rule.Country != nil && len(rule.Country.Country) > 0 {
			params.Country = &cf.AccessGroupCountryRuleParams{Country: rule.Country.Country}
			hasRule = true
		}
		if rule.DevicePosture != nil {
			params.DevicePosture = &cf.AccessGroupDevicePostureRuleParams{IntegrationUID: rule.DevicePosture.IntegrationUID}
			hasRule = true
		}
		if rule.CommonName != nil {
			params.CommonName = &cf.AccessGroupCommonNameRuleParams{CommonName: rule.CommonName.CommonName}
			hasRule = true
		}
		if rule.Certificate {
			params.Certificate = true
			hasRule = true
		}
		if rule.SAML != nil {
			params.SAML = &cf.AccessGroupSAMLRuleParams{
				AttributeName:      rule.SAML.AttributeName,
				AttributeValue:     rule.SAML.AttributeValue,
				IdentityProviderID: rule.SAML.IdentityProviderID,
			}
			hasRule = true
		}
		if rule.OIDC != nil {
			params.OIDC = &cf.AccessGroupOIDCRuleParams{
				ClaimName:          rule.OIDC.ClaimName,
				ClaimValue:         rule.OIDC.ClaimValue,
				IdentityProviderID: rule.OIDC.IdentityProviderID,
			}
			hasRule = true
		}
		if rule.GSuite != nil {
			params.GSuite = &cf.AccessGroupGSuiteRuleParams{
				Email:              rule.GSuite.Email,
				IdentityProviderID: rule.GSuite.IdentityProviderID,
			}
			hasRule = true
		}
		if rule.Azure != nil {
			params.Azure = &cf.AccessGroupAzureRuleParams{
				ID:                 rule.Azure.ID,
				IdentityProviderID: rule.Azure.IdentityProviderID,
			}
			hasRule = true
		}
		if rule.GitHub != nil {
			params.GitHub = &cf.AccessGroupGitHubRuleParams{
				Name:               rule.GitHub.Name,
				Teams:              rule.GitHub.Teams,
				IdentityProviderID: rule.GitHub.IdentityProviderID,
			}
			hasRule = true
		}
		if rule.Okta != nil {
			params.Okta = &cf.AccessGroupOktaRuleParams{
				Name:               rule.Okta.Name,
				IdentityProviderID: rule.Okta.IdentityProviderID,
			}
			hasRule = true
		}
		if rule.AuthMethod != nil {
			params.AuthMethod = &cf.AccessGroupAuthMethodRuleParams{AuthMethod: rule.AuthMethod.AuthMethod}
			hasRule = true
		}
		if rule.AuthContext != nil {
			params.AuthContext = &cf.AccessGroupAuthContextRuleParams{
				ID:                 rule.AuthContext.ID,
				AcID:               rule.AuthContext.AcID,
				IdentityProviderID: rule.AuthContext.IdentityProviderID,
			}
			hasRule = true
		}
		if rule.LoginMethod != nil {
			params.LoginMethod = &cf.AccessGroupLoginMethodRuleParams{ID: rule.LoginMethod.ID}
			hasRule = true
		}

		if hasRule {
			result = append(result, params)
		}
	}

	return result
}

func (r *AccessGroupReconciler) updateStatusError(ctx context.Context, accessGroup *networkingv1alpha2.AccessGroup, err error) (ctrl.Result, error) {
	// P0 FIX: Use UpdateStatusWithConflictRetry for status updates
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, accessGroup, func() {
		accessGroup.Status.State = "Error"
		meta.SetStatusCondition(&accessGroup.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: accessGroup.Generation,
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		accessGroup.Status.ObservedGeneration = accessGroup.Generation
	})
	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *AccessGroupReconciler) updateStatusSuccess(ctx context.Context, accessGroup *networkingv1alpha2.AccessGroup, result *cf.AccessGroupResult) (ctrl.Result, error) {
	// P0 FIX: Use UpdateStatusWithConflictRetry for status updates
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, accessGroup, func() {
		accessGroup.Status.GroupID = result.ID
		accessGroup.Status.State = "Ready"
		meta.SetStatusCondition(&accessGroup.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: accessGroup.Generation,
			Reason:             "Reconciled",
			Message:            "Access Group successfully reconciled",
			LastTransitionTime: metav1.Now(),
		})
		accessGroup.Status.ObservedGeneration = accessGroup.Generation
	})
	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *AccessGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("accessgroup-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.AccessGroup{}).
		Complete(r)
}
