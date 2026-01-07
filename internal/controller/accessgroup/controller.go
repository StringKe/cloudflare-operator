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

package accessgroup

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

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

const (
	FinalizerName = "accessgroup.networking.cloudflare-operator.io/finalizer"
)

// AccessGroupReconciler reconciles an AccessGroup object
type AccessGroupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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
	apiClient, err := cf.NewAPIClientFromDetails(ctx, r.Client, "", accessGroup.Spec.Cloudflare)
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
				logger.Error(err, "Failed to delete Access Group from Cloudflare")
			}
		}

		// Remove finalizer
		controllerutil.RemoveFinalizer(accessGroup, FinalizerName)
		if err := r.Update(ctx, accessGroup); err != nil {
			return ctrl.Result{}, err
		}
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
		result, err = apiClient.CreateAccessGroup(params)
	} else {
		// Update existing access group
		logger.Info("Updating Access Group", "groupId", accessGroup.Status.GroupID)
		result, err = apiClient.UpdateAccessGroup(accessGroup.Status.GroupID, params)
	}

	if err != nil {
		return r.updateStatusError(ctx, accessGroup, err)
	}

	// Update status
	return r.updateStatusSuccess(ctx, accessGroup, result)
}

func (r *AccessGroupReconciler) buildGroupRules(rules []networkingv1alpha2.AccessGroupRule) []interface{} {
	if len(rules) == 0 {
		return nil
	}

	result := make([]interface{}, 0, len(rules))
	for _, rule := range rules {
		ruleMap := make(map[string]interface{})

		if rule.Email != nil {
			ruleMap["email"] = map[string]string{"email": rule.Email.Email}
		}
		if rule.EmailDomain != nil {
			ruleMap["email_domain"] = map[string]string{"domain": rule.EmailDomain.Domain}
		}
		if rule.IPRanges != nil && len(rule.IPRanges.IP) > 0 {
			// Add first IP range - for multiple, create multiple rules
			ruleMap["ip"] = map[string]string{"ip": rule.IPRanges.IP[0]}
		}
		if rule.Everyone {
			ruleMap["everyone"] = struct{}{}
		}
		if rule.Group != nil {
			ruleMap["group"] = map[string]string{"id": rule.Group.ID}
		}
		if rule.AnyValidServiceToken {
			ruleMap["any_valid_service_token"] = struct{}{}
		}
		if rule.ServiceToken != nil {
			ruleMap["service_token"] = map[string]string{"token_id": rule.ServiceToken.TokenID}
		}
		if rule.ExternalEvaluation != nil {
			ruleMap["external_evaluation"] = map[string]string{
				"evaluate_url": rule.ExternalEvaluation.EvaluateURL,
				"keys_url":     rule.ExternalEvaluation.KeysURL,
			}
		}
		if rule.Country != nil && len(rule.Country.Country) > 0 {
			ruleMap["geo"] = map[string]string{"country_code": rule.Country.Country[0]}
		}
		if rule.DevicePosture != nil {
			ruleMap["device_posture"] = map[string]string{"integration_uid": rule.DevicePosture.IntegrationUID}
		}
		if rule.CommonName != nil {
			ruleMap["common_name"] = map[string]string{"common_name": rule.CommonName.CommonName}
		}
		if rule.Certificate {
			ruleMap["certificate"] = struct{}{}
		}
		if rule.SAML != nil {
			ruleMap["saml"] = map[string]interface{}{
				"attribute_name":       rule.SAML.AttributeName,
				"attribute_value":      rule.SAML.AttributeValue,
				"identity_provider_id": rule.SAML.IdentityProviderID,
			}
		}
		if rule.OIDC != nil {
			ruleMap["oidc"] = map[string]interface{}{
				"claim_name":           rule.OIDC.ClaimName,
				"claim_value":          rule.OIDC.ClaimValue,
				"identity_provider_id": rule.OIDC.IdentityProviderID,
			}
		}
		if rule.GSuite != nil {
			ruleMap["gsuite"] = map[string]interface{}{
				"email":                rule.GSuite.Email,
				"identity_provider_id": rule.GSuite.IdentityProviderID,
			}
		}
		if rule.Azure != nil {
			ruleMap["azure_ad"] = map[string]interface{}{
				"id":                   rule.Azure.ID,
				"identity_provider_id": rule.Azure.IdentityProviderID,
			}
		}
		if rule.GitHub != nil {
			ghMap := map[string]interface{}{
				"name":                 rule.GitHub.Name,
				"identity_provider_id": rule.GitHub.IdentityProviderID,
			}
			if len(rule.GitHub.Teams) > 0 {
				ghMap["teams"] = rule.GitHub.Teams
			}
			ruleMap["github_organization"] = ghMap
		}

		if len(ruleMap) > 0 {
			result = append(result, ruleMap)
		}
	}

	return result
}

func (r *AccessGroupReconciler) updateStatusError(ctx context.Context, accessGroup *networkingv1alpha2.AccessGroup, err error) (ctrl.Result, error) {
	accessGroup.Status.State = "Error"
	meta.SetStatusCondition(&accessGroup.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "ReconcileError",
		Message:            err.Error(),
		LastTransitionTime: metav1.Now(),
	})
	accessGroup.Status.ObservedGeneration = accessGroup.Generation

	if updateErr := r.Status().Update(ctx, accessGroup); updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *AccessGroupReconciler) updateStatusSuccess(ctx context.Context, accessGroup *networkingv1alpha2.AccessGroup, result *cf.AccessGroupResult) (ctrl.Result, error) {
	accessGroup.Status.GroupID = result.ID
	accessGroup.Status.State = "Ready"
	meta.SetStatusCondition(&accessGroup.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "Access Group successfully reconciled",
		LastTransitionTime: metav1.Now(),
	})
	accessGroup.Status.ObservedGeneration = accessGroup.Generation

	if err := r.Status().Update(ctx, accessGroup); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *AccessGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.AccessGroup{}).
		Complete(r)
}
