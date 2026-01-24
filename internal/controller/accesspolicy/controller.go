// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package accesspolicy provides a controller for managing Cloudflare Reusable Access Policies.
// It directly calls Cloudflare API and writes status back to the CRD.
package accesspolicy

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
	finalizerName = "accesspolicy.networking.cloudflare-operator.io/finalizer"
)

// Reconciler reconciles an AccessPolicy object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accesspolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accesspolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accesspolicies/finalizers,verbs=update

// Reconcile handles AccessPolicy reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the AccessPolicy resource
	policy := &networkingv1alpha2.AccessPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch AccessPolicy")
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !policy.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, policy)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, policy, finalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Get API client
	// AccessPolicy is cluster-scoped, use operator namespace for legacy inline secrets
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &policy.Spec.Cloudflare,
		Namespace:         common.OperatorNamespace,
		StatusAccountID:   policy.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, policy, err)
	}

	// Sync policy to Cloudflare
	return r.syncPolicy(ctx, policy, apiResult)
}

// handleDeletion handles the deletion of AccessPolicy.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	policy *networkingv1alpha2.AccessPolicy,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(policy, finalizerName) {
		return common.NoRequeue(), nil
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &policy.Spec.Cloudflare,
		Namespace:         common.OperatorNamespace,
		StatusAccountID:   policy.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client for deletion")
		// Continue with finalizer removal
	} else if policy.Status.PolicyID != "" {
		// Delete policy from Cloudflare
		logger.Info("Deleting Access Policy from Cloudflare",
			"policyId", policy.Status.PolicyID)

		if err := apiResult.API.DeleteReusableAccessPolicy(ctx, policy.Status.PolicyID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete Access Policy from Cloudflare")
				r.Recorder.Event(policy, corev1.EventTypeWarning, "DeleteFailed",
					fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
				return common.RequeueShort(), err
			}
			logger.Info("Access Policy not found in Cloudflare, may have been already deleted")
		}

		r.Recorder.Event(policy, corev1.EventTypeNormal, "Deleted",
			"Access Policy deleted from Cloudflare")
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, policy, func() {
		controllerutil.RemoveFinalizer(policy, finalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(policy, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// syncPolicy syncs the Access Policy to Cloudflare.
func (r *Reconciler) syncPolicy(
	ctx context.Context,
	policy *networkingv1alpha2.AccessPolicy,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Determine policy name
	policyName := policy.GetAccessPolicyName()

	// Build params
	params := r.buildParams(policy, policyName)

	// Check if policy already exists by ID
	if policy.Status.PolicyID != "" {
		existing, err := apiResult.API.GetReusableAccessPolicy(ctx, policy.Status.PolicyID)
		if err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to get Access Policy from Cloudflare")
				return r.updateStatusError(ctx, policy, err)
			}
			// Policy doesn't exist, will create
			logger.Info("Access Policy not found in Cloudflare, will recreate",
				"policyId", policy.Status.PolicyID)
		} else {
			// Policy exists, update it
			logger.V(1).Info("Updating Access Policy in Cloudflare",
				"policyId", existing.ID,
				"name", policyName)

			result, err := apiResult.API.UpdateReusableAccessPolicy(ctx, existing.ID, params)
			if err != nil {
				logger.Error(err, "Failed to update Access Policy")
				return r.updateStatusError(ctx, policy, err)
			}

			r.Recorder.Event(policy, corev1.EventTypeNormal, "Updated",
				fmt.Sprintf("Access Policy '%s' updated in Cloudflare", policyName))

			return r.updateStatusReady(ctx, policy, apiResult.AccountID, result.ID)
		}
	}

	// Try to find existing policy by name
	existingByName, err := apiResult.API.GetReusableAccessPolicyByName(ctx, policyName)
	if err != nil && !cf.IsNotFoundError(err) {
		logger.Error(err, "Failed to search for existing Access Policy")
		return r.updateStatusError(ctx, policy, err)
	}

	if existingByName != nil {
		// Policy already exists with this name, adopt it
		logger.Info("Access Policy already exists with same name, adopting it",
			"policyId", existingByName.ID,
			"name", policyName)

		// Update the existing policy
		result, err := apiResult.API.UpdateReusableAccessPolicy(ctx, existingByName.ID, params)
		if err != nil {
			logger.Error(err, "Failed to update existing Access Policy")
			return r.updateStatusError(ctx, policy, err)
		}

		r.Recorder.Event(policy, corev1.EventTypeNormal, "Adopted",
			fmt.Sprintf("Adopted existing Access Policy '%s'", policyName))

		return r.updateStatusReady(ctx, policy, apiResult.AccountID, result.ID)
	}

	// Create new policy
	logger.Info("Creating Access Policy in Cloudflare",
		"name", policyName)

	result, err := apiResult.API.CreateReusableAccessPolicy(ctx, params)
	if err != nil {
		logger.Error(err, "Failed to create Access Policy")
		return r.updateStatusError(ctx, policy, err)
	}

	r.Recorder.Event(policy, corev1.EventTypeNormal, "Created",
		fmt.Sprintf("Access Policy '%s' created in Cloudflare", policyName))

	return r.updateStatusReady(ctx, policy, apiResult.AccountID, result.ID)
}

// buildParams builds the ReusableAccessPolicyParams from the AccessPolicy spec.
func (r *Reconciler) buildParams(policy *networkingv1alpha2.AccessPolicy, policyName string) cf.ReusableAccessPolicyParams {
	params := cf.ReusableAccessPolicyParams{
		Name:                         policyName,
		Decision:                     policy.Spec.Decision,
		Precedence:                   policy.Spec.Precedence,
		Include:                      convertRulesToCF(policy.Spec.Include),
		Exclude:                      convertRulesToCF(policy.Spec.Exclude),
		Require:                      convertRulesToCF(policy.Spec.Require),
		IsolationRequired:            policy.Spec.IsolationRequired,
		PurposeJustificationRequired: policy.Spec.PurposeJustificationRequired,
		PurposeJustificationPrompt:   policy.Spec.PurposeJustificationPrompt,
		ApprovalRequired:             policy.Spec.ApprovalRequired,
	}

	// Handle session duration
	if policy.Spec.SessionDuration != "" {
		params.SessionDuration = &policy.Spec.SessionDuration
	}

	// Convert approval groups
	if len(policy.Spec.ApprovalGroups) > 0 {
		params.ApprovalGroups = make([]cf.AccessApprovalGroupParams, 0, len(policy.Spec.ApprovalGroups))
		for _, ag := range policy.Spec.ApprovalGroups {
			params.ApprovalGroups = append(params.ApprovalGroups, cf.AccessApprovalGroupParams{
				EmailAddresses:  ag.EmailAddresses,
				EmailListUUID:   ag.EmailListUUID,
				ApprovalsNeeded: ag.ApprovalsNeeded,
			})
		}
	}

	return params
}

// convertRulesToCF converts AccessGroupRule slice to cf.AccessGroupRuleParams slice.
func convertRulesToCF(rules []networkingv1alpha2.AccessGroupRule) []cf.AccessGroupRuleParams {
	if len(rules) == 0 {
		return nil
	}

	result := make([]cf.AccessGroupRuleParams, 0, len(rules))
	for _, rule := range rules {
		cfRule := cf.AccessGroupRuleParams{}

		if rule.Email != nil {
			cfRule.Email = &cf.AccessGroupEmailRuleParams{Email: rule.Email.Email}
		}
		if rule.EmailDomain != nil {
			cfRule.EmailDomain = &cf.AccessGroupEmailDomainRuleParams{Domain: rule.EmailDomain.Domain}
		}
		if rule.EmailList != nil {
			cfRule.EmailList = &cf.AccessGroupEmailListRuleParams{ID: rule.EmailList.ID}
		}
		if rule.Everyone {
			cfRule.Everyone = true
		}
		if rule.IPRanges != nil && len(rule.IPRanges.IP) > 0 {
			cfRule.IPRanges = &cf.AccessGroupIPRangesRuleParams{IP: rule.IPRanges.IP}
		}
		if rule.IPList != nil {
			cfRule.IPList = &cf.AccessGroupIPListRuleParams{ID: rule.IPList.ID}
		}
		if rule.Country != nil && len(rule.Country.Country) > 0 {
			cfRule.Country = &cf.AccessGroupCountryRuleParams{Country: rule.Country.Country}
		}
		if rule.Group != nil {
			cfRule.Group = &cf.AccessGroupGroupRuleParams{ID: rule.Group.ID}
		}
		if rule.ServiceToken != nil {
			cfRule.ServiceToken = &cf.AccessGroupServiceTokenRuleParams{TokenID: rule.ServiceToken.TokenID}
		}
		if rule.AnyValidServiceToken {
			cfRule.AnyValidServiceToken = true
		}
		if rule.Certificate {
			cfRule.Certificate = true
		}
		if rule.CommonName != nil {
			cfRule.CommonName = &cf.AccessGroupCommonNameRuleParams{CommonName: rule.CommonName.CommonName}
		}
		if rule.DevicePosture != nil {
			cfRule.DevicePosture = &cf.AccessGroupDevicePostureRuleParams{IntegrationUID: rule.DevicePosture.IntegrationUID}
		}
		if rule.GSuite != nil {
			cfRule.GSuite = &cf.AccessGroupGSuiteRuleParams{
				Email:              rule.GSuite.Email,
				IdentityProviderID: rule.GSuite.IdentityProviderID,
			}
		}
		if rule.GitHub != nil {
			cfRule.GitHub = &cf.AccessGroupGitHubRuleParams{
				Name:               rule.GitHub.Name,
				Teams:              rule.GitHub.Teams,
				IdentityProviderID: rule.GitHub.IdentityProviderID,
			}
		}
		if rule.Azure != nil {
			cfRule.Azure = &cf.AccessGroupAzureRuleParams{
				ID:                 rule.Azure.ID,
				IdentityProviderID: rule.Azure.IdentityProviderID,
			}
		}
		if rule.Okta != nil {
			cfRule.Okta = &cf.AccessGroupOktaRuleParams{
				Name:               rule.Okta.Name,
				IdentityProviderID: rule.Okta.IdentityProviderID,
			}
		}
		if rule.OIDC != nil {
			cfRule.OIDC = &cf.AccessGroupOIDCRuleParams{
				ClaimName:          rule.OIDC.ClaimName,
				ClaimValue:         rule.OIDC.ClaimValue,
				IdentityProviderID: rule.OIDC.IdentityProviderID,
			}
		}
		if rule.SAML != nil {
			cfRule.SAML = &cf.AccessGroupSAMLRuleParams{
				AttributeName:      rule.SAML.AttributeName,
				AttributeValue:     rule.SAML.AttributeValue,
				IdentityProviderID: rule.SAML.IdentityProviderID,
			}
		}
		if rule.AuthMethod != nil {
			cfRule.AuthMethod = &cf.AccessGroupAuthMethodRuleParams{AuthMethod: rule.AuthMethod.AuthMethod}
		}
		if rule.AuthContext != nil {
			cfRule.AuthContext = &cf.AccessGroupAuthContextRuleParams{
				ID:                 rule.AuthContext.ID,
				AcID:               rule.AuthContext.AcID,
				IdentityProviderID: rule.AuthContext.IdentityProviderID,
			}
		}
		if rule.LoginMethod != nil {
			cfRule.LoginMethod = &cf.AccessGroupLoginMethodRuleParams{ID: rule.LoginMethod.ID}
		}
		if rule.ExternalEvaluation != nil {
			cfRule.ExternalEvaluation = &cf.AccessGroupExternalEvaluationRuleParams{
				EvaluateURL: rule.ExternalEvaluation.EvaluateURL,
				KeysURL:     rule.ExternalEvaluation.KeysURL,
			}
		}

		result = append(result, cfRule)
	}

	return result
}

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	policy *networkingv1alpha2.AccessPolicy,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, policy, func() {
		policy.Status.State = "Error"
		meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: policy.Generation,
			Reason:             "Error",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		policy.Status.ObservedGeneration = policy.Generation
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
}

func (r *Reconciler) updateStatusReady(
	ctx context.Context,
	policy *networkingv1alpha2.AccessPolicy,
	accountID, policyID string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, policy, func() {
		policy.Status.AccountID = accountID
		policy.Status.PolicyID = policyID
		policy.Status.State = "Ready"
		meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: policy.Generation,
			Reason:             "Synced",
			Message:            "Access Policy synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		policy.Status.ObservedGeneration = policy.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("accesspolicy-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("accesspolicy"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.AccessPolicy{}).
		Named("accesspolicy").
		Complete(r)
}
