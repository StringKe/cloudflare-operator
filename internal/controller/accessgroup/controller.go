// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package accessgroup provides a controller for managing Cloudflare Access Groups.
// It directly calls Cloudflare API and writes status back to the CRD.
package accessgroup

import (
	"context"
	"fmt"

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
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/controller/common"
	"github.com/StringKe/cloudflare-operator/internal/controller/refs"
)

const (
	finalizerName = "accessgroup.networking.cloudflare-operator.io/finalizer"
)

// Reconciler reconciles an AccessGroup object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessgroups/finalizers,verbs=update

// Reconcile handles AccessGroup reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the AccessGroup resource
	accessGroup := &networkingv1alpha2.AccessGroup{}
	if err := r.Get(ctx, req.NamespacedName, accessGroup); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch AccessGroup")
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !accessGroup.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, accessGroup)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, accessGroup, finalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Get API client
	// AccessGroup is cluster-scoped, use operator namespace for legacy inline secrets
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &accessGroup.Spec.Cloudflare,
		Namespace:         common.OperatorNamespace,
		StatusAccountID:   accessGroup.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, accessGroup, err)
	}

	// Sync access group to Cloudflare
	return r.syncAccessGroup(ctx, accessGroup, apiResult)
}

// handleDeletion handles the deletion of AccessGroup.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	accessGroup *networkingv1alpha2.AccessGroup,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(accessGroup, finalizerName) {
		return common.NoRequeue(), nil
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &accessGroup.Spec.Cloudflare,
		Namespace:         common.OperatorNamespace,
		StatusAccountID:   accessGroup.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client for deletion")
		// Continue with finalizer removal
	} else if accessGroup.Status.GroupID != "" {
		// Delete group from Cloudflare
		logger.Info("Deleting Access Group from Cloudflare",
			"groupId", accessGroup.Status.GroupID)

		if err := apiResult.API.DeleteAccessGroup(ctx, accessGroup.Status.GroupID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete Access Group from Cloudflare, continuing with finalizer removal")
				r.Recorder.Event(accessGroup, corev1.EventTypeWarning, "DeleteFailed",
					fmt.Sprintf("Failed to delete from Cloudflare (will remove finalizer anyway): %s", cf.SanitizeErrorMessage(err)))
				// Don't block finalizer removal - resource may need manual cleanup in Cloudflare
			} else {
				logger.Info("Access Group not found in Cloudflare, may have been already deleted")
			}
		} else {
			r.Recorder.Event(accessGroup, corev1.EventTypeNormal, "Deleted",
				"Access Group deleted from Cloudflare")
		}
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, accessGroup, func() {
		controllerutil.RemoveFinalizer(accessGroup, finalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(accessGroup, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// syncAccessGroup syncs the Access Group to Cloudflare.
func (r *Reconciler) syncAccessGroup(
	ctx context.Context,
	accessGroup *networkingv1alpha2.AccessGroup,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Create resolver for IdP references
	resolver := refs.NewResolver(r.Client, apiResult.API)

	// Determine group name
	groupName := accessGroup.GetAccessGroupName()

	// Build params with resolved IdP references
	params := cf.AccessGroupParams{
		Name:      groupName,
		Include:   r.convertRulesToCF(ctx, accessGroup.Spec.Include, resolver),
		Exclude:   r.convertRulesToCF(ctx, accessGroup.Spec.Exclude, resolver),
		Require:   r.convertRulesToCF(ctx, accessGroup.Spec.Require, resolver),
		IsDefault: accessGroup.Spec.IsDefault,
	}

	// Check if group already exists
	if accessGroup.Status.GroupID != "" {
		// Try to get existing group
		existing, err := apiResult.API.GetAccessGroup(ctx, accessGroup.Status.GroupID)
		if err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to get Access Group from Cloudflare")
				return r.updateStatusError(ctx, accessGroup, err)
			}
			// Group doesn't exist, will create
			logger.Info("Access Group not found in Cloudflare, will recreate",
				"groupId", accessGroup.Status.GroupID)
		} else {
			// Group exists, update it
			logger.V(1).Info("Updating Access Group in Cloudflare",
				"groupId", existing.ID,
				"name", groupName)

			result, err := apiResult.API.UpdateAccessGroup(ctx, existing.ID, params)
			if err != nil {
				logger.Error(err, "Failed to update Access Group")
				return r.updateStatusError(ctx, accessGroup, err)
			}

			r.Recorder.Event(accessGroup, corev1.EventTypeNormal, "Updated",
				fmt.Sprintf("Access Group '%s' updated in Cloudflare", groupName))

			return r.updateStatusReady(ctx, accessGroup, apiResult.AccountID, result.ID)
		}
	}

	// Try to find existing group by name
	existingByName, err := apiResult.API.ListAccessGroupsByName(ctx, groupName)
	if err != nil {
		logger.Error(err, "Failed to search for existing Access Group")
		return r.updateStatusError(ctx, accessGroup, err)
	}

	if existingByName != nil {
		// Group already exists with this name, adopt it
		logger.Info("Access Group already exists with same name, adopting it",
			"groupId", existingByName.ID,
			"name", groupName)

		// Update the existing group
		result, err := apiResult.API.UpdateAccessGroup(ctx, existingByName.ID, params)
		if err != nil {
			logger.Error(err, "Failed to update existing Access Group")
			return r.updateStatusError(ctx, accessGroup, err)
		}

		r.Recorder.Event(accessGroup, corev1.EventTypeNormal, "Adopted",
			fmt.Sprintf("Adopted existing Access Group '%s'", groupName))

		return r.updateStatusReady(ctx, accessGroup, apiResult.AccountID, result.ID)
	}

	// Create new group
	logger.Info("Creating Access Group in Cloudflare",
		"name", groupName)

	result, err := apiResult.API.CreateAccessGroup(ctx, params)
	if err != nil {
		logger.Error(err, "Failed to create Access Group")
		return r.updateStatusError(ctx, accessGroup, err)
	}

	r.Recorder.Event(accessGroup, corev1.EventTypeNormal, "Created",
		fmt.Sprintf("Access Group '%s' created in Cloudflare", groupName))

	return r.updateStatusReady(ctx, accessGroup, apiResult.AccountID, result.ID)
}

// convertRulesToCF converts AccessGroupRule slice to cf.AccessGroupRuleParams slice.
// It resolves IdpRef references using the provided resolver.
//
//nolint:revive // cognitive complexity is acceptable for this conversion function
func (r *Reconciler) convertRulesToCF(
	ctx context.Context,
	rules []networkingv1alpha2.AccessGroupRule,
	resolver *refs.Resolver,
) []cf.AccessGroupRuleParams {
	if len(rules) == 0 {
		return nil
	}

	logger := log.FromContext(ctx)
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
			idpID := r.resolveIdpRef(ctx, logger, resolver, rule.GSuite.IdpRef)
			cfRule.GSuite = &cf.AccessGroupGSuiteRuleParams{
				Email:              rule.GSuite.Email,
				IdentityProviderID: idpID,
			}
		}
		if rule.GitHub != nil {
			idpID := r.resolveIdpRef(ctx, logger, resolver, rule.GitHub.IdpRef)
			cfRule.GitHub = &cf.AccessGroupGitHubRuleParams{
				Name:               rule.GitHub.Name,
				Teams:              rule.GitHub.Teams,
				IdentityProviderID: idpID,
			}
		}
		if rule.Azure != nil {
			idpID := r.resolveIdpRef(ctx, logger, resolver, rule.Azure.IdpRef)
			cfRule.Azure = &cf.AccessGroupAzureRuleParams{
				ID:                 rule.Azure.ID,
				IdentityProviderID: idpID,
			}
		}
		if rule.Okta != nil {
			idpID := r.resolveIdpRef(ctx, logger, resolver, rule.Okta.IdpRef)
			cfRule.Okta = &cf.AccessGroupOktaRuleParams{
				Name:               rule.Okta.Name,
				IdentityProviderID: idpID,
			}
		}
		if rule.OIDC != nil {
			idpID := r.resolveIdpRef(ctx, logger, resolver, rule.OIDC.IdpRef)
			cfRule.OIDC = &cf.AccessGroupOIDCRuleParams{
				ClaimName:          rule.OIDC.ClaimName,
				ClaimValue:         rule.OIDC.ClaimValue,
				IdentityProviderID: idpID,
			}
		}
		if rule.SAML != nil {
			idpID := r.resolveIdpRef(ctx, logger, resolver, rule.SAML.IdpRef)
			cfRule.SAML = &cf.AccessGroupSAMLRuleParams{
				AttributeName:      rule.SAML.AttributeName,
				AttributeValue:     rule.SAML.AttributeValue,
				IdentityProviderID: idpID,
			}
		}
		if rule.AuthMethod != nil {
			cfRule.AuthMethod = &cf.AccessGroupAuthMethodRuleParams{AuthMethod: rule.AuthMethod.AuthMethod}
		}
		if rule.AuthContext != nil {
			idpID := r.resolveIdpRef(ctx, logger, resolver, rule.AuthContext.IdpRef)
			cfRule.AuthContext = &cf.AccessGroupAuthContextRuleParams{
				ID:                 rule.AuthContext.ID,
				AcID:               rule.AuthContext.AcID,
				IdentityProviderID: idpID,
			}
		}
		if rule.LoginMethod != nil {
			idpID := r.resolveIdpRef(ctx, logger, resolver, rule.LoginMethod.IdpRef)
			cfRule.LoginMethod = &cf.AccessGroupLoginMethodRuleParams{ID: idpID}
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

// resolveIdpRef resolves an IdpRef to a Cloudflare IdP ID.
func (*Reconciler) resolveIdpRef(
	ctx context.Context,
	logger logr.Logger,
	resolver *refs.Resolver,
	idpRef *networkingv1alpha2.AccessIdentityProviderRefV2,
) string {
	if idpRef == nil {
		return ""
	}
	idpID, err := resolver.ResolveIdentityProvider(ctx, idpRef)
	if err != nil {
		logger.Error(err, "Failed to resolve IdpRef")
		return ""
	}
	return idpID
}

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	accessGroup *networkingv1alpha2.AccessGroup,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, accessGroup, func() {
		accessGroup.Status.State = "Error"
		meta.SetStatusCondition(&accessGroup.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: accessGroup.Generation,
			Reason:             "Error",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		accessGroup.Status.ObservedGeneration = accessGroup.Generation
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
}

func (r *Reconciler) updateStatusReady(
	ctx context.Context,
	accessGroup *networkingv1alpha2.AccessGroup,
	accountID, groupID string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, accessGroup, func() {
		accessGroup.Status.AccountID = accountID
		accessGroup.Status.GroupID = groupID
		accessGroup.Status.State = "Ready"
		meta.SetStatusCondition(&accessGroup.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: accessGroup.Generation,
			Reason:             "Synced",
			Message:            "Access Group synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		accessGroup.Status.ObservedGeneration = accessGroup.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("accessgroup-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("accessgroup"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.AccessGroup{}).
		Named("accessgroup").
		Complete(r)
}
