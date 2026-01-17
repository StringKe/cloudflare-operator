// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package access

//nolint:dupl // Similar patterns across resource types are intentional

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	accesssvc "github.com/StringKe/cloudflare-operator/internal/service/access"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

// GroupController is the Sync Controller for AccessGroup Configuration.
type GroupController struct {
	*common.BaseSyncController
}

// NewGroupController creates a new AccessGroup sync controller.
func NewGroupController(c client.Client) *GroupController {
	return &GroupController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for AccessGroup.
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *GroupController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "AccessGroupSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process AccessGroup type
	if syncState.Spec.ResourceType != accesssvc.ResourceTypeAccessGroup {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing AccessGroup SyncState",
		"cloudflareId", syncState.Spec.CloudflareID,
		"sources", len(syncState.Spec.Sources))

	// Skip if there's a pending debounced request
	if r.Debouncer.IsPending(req.Name) {
		logger.V(1).Info("Skipping reconcile - debounced request pending")
		return ctrl.Result{}, nil
	}

	// Check if there are any sources
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, marking as synced (no-op)")
		if err := r.SetSyncStatus(ctx, syncState, v1alpha2.SyncStatusSynced); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Extract AccessGroup configuration from first source
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract AccessGroup configuration")
		if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
	}

	// Compute hash for change detection
	newHash, hashErr := common.ComputeConfigHash(config)
	if hashErr != nil {
		logger.Error(hashErr, "Failed to compute config hash")
		newHash = "" // Force sync if hash fails
	}

	if !r.ShouldSync(syncState, newHash) {
		logger.V(1).Info("Configuration unchanged, skipping sync", "hash", newHash)
		return ctrl.Result{}, nil
	}

	// Set syncing status
	if err := r.SetSyncStatus(ctx, syncState, v1alpha2.SyncStatusSyncing); err != nil {
		return ctrl.Result{}, err
	}

	// Sync to Cloudflare API
	result, err := r.syncToCloudflare(ctx, syncState, config)
	if err != nil {
		logger.Error(err, "Failed to sync AccessGroup to Cloudflare")
		if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
	}

	// Update success status
	syncResult := &common.SyncResult{
		ConfigHash: newHash,
	}
	if err := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusSynced, syncResult, nil); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Successfully synced AccessGroup to Cloudflare",
		"groupId", result.ID,
		"name", config.Name)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts AccessGroup configuration from SyncState sources.
// AccessGroup has 1:1 mapping, so we use the ExtractFirstSourceConfig helper.
func (*GroupController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*accesssvc.AccessGroupConfig, error) {
	return common.ExtractFirstSourceConfig[accesssvc.AccessGroupConfig](syncState)
}

// syncToCloudflare syncs the AccessGroup configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *GroupController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *accesssvc.AccessGroupConfig,
) (*accesssvc.SyncResult, error) {
	logger := log.FromContext(ctx)

	// Create API client using common helper
	apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
	if err != nil {
		return nil, err
	}

	// Validate and set account ID
	accountID, err := common.RequireAccountID(syncState)
	if err != nil {
		return nil, err
	}
	apiClient.ValidAccountId = accountID

	// Build AccessGroup params
	params := r.buildParams(config)

	// Check if this is an existing group or new (pending)
	cloudflareID := syncState.Spec.CloudflareID
	var result *cf.AccessGroupResult

	if common.IsPendingID(cloudflareID) {
		// Create new AccessGroup
		logger.Info("Creating new AccessGroup",
			"name", config.Name)

		result, err = apiClient.CreateAccessGroup(params)
		if err != nil {
			return nil, fmt.Errorf("create AccessGroup: %w", err)
		}

		// Update SyncState CloudflareID with the actual group ID
		common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID)

		logger.Info("Created AccessGroup",
			"groupId", result.ID)
	} else {
		// Update existing AccessGroup
		logger.Info("Updating AccessGroup",
			"groupId", cloudflareID,
			"name", config.Name)

		result, err = apiClient.UpdateAccessGroup(cloudflareID, params)
		if err != nil {
			// Check if group was deleted externally
			if common.HandleNotFoundOnUpdate(err) {
				logger.Info("AccessGroup not found, recreating",
					"groupId", cloudflareID)
				result, err = apiClient.CreateAccessGroup(params)
				if err != nil {
					return nil, fmt.Errorf("recreate AccessGroup: %w", err)
				}

				// Update SyncState CloudflareID if ID changed
				if result.ID != cloudflareID {
					common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID)
				}
			} else {
				return nil, fmt.Errorf("update AccessGroup: %w", err)
			}
		}

		logger.Info("Updated AccessGroup",
			"groupId", result.ID)
	}

	return &accesssvc.SyncResult{
		ID:        result.ID,
		AccountID: accountID,
	}, nil
}

// buildParams builds AccessGroupParams from config.
func (*GroupController) buildParams(config *accesssvc.AccessGroupConfig) cf.AccessGroupParams {
	params := cf.AccessGroupParams{
		Name:      config.Name,
		IsDefault: config.IsDefault,
	}

	// Convert include rules
	if len(config.Include) > 0 {
		params.Include = convertGroupRules(config.Include)
	}

	// Convert exclude rules
	if len(config.Exclude) > 0 {
		params.Exclude = convertGroupRules(config.Exclude)
	}

	// Convert require rules
	if len(config.Require) > 0 {
		params.Require = convertGroupRules(config.Require)
	}

	return params
}

// convertGroupRules converts AccessGroupRule slice to AccessGroupRuleParams slice.
//
//nolint:revive // cognitive complexity is acceptable for rule conversion
func convertGroupRules(rules []v1alpha2.AccessGroupRule) []cf.AccessGroupRuleParams {
	result := make([]cf.AccessGroupRuleParams, len(rules))
	for i, rule := range rules {
		result[i] = convertGroupRule(rule)
	}
	return result
}

// convertGroupRule converts a single AccessGroupRule to AccessGroupRuleParams.
//
//nolint:revive // cognitive complexity is acceptable for rule conversion with many fields
func convertGroupRule(rule v1alpha2.AccessGroupRule) cf.AccessGroupRuleParams {
	params := cf.AccessGroupRuleParams{
		Everyone:             rule.Everyone,
		AnyValidServiceToken: rule.AnyValidServiceToken,
		Certificate:          rule.Certificate,
	}

	if rule.Email != nil {
		params.Email = &cf.AccessGroupEmailRuleParams{
			Email: rule.Email.Email,
		}
	}

	if rule.EmailDomain != nil {
		params.EmailDomain = &cf.AccessGroupEmailDomainRuleParams{
			Domain: rule.EmailDomain.Domain,
		}
	}

	if rule.EmailList != nil {
		params.EmailList = &cf.AccessGroupEmailListRuleParams{
			ID: rule.EmailList.ID,
		}
	}

	if rule.IPRanges != nil {
		params.IPRanges = &cf.AccessGroupIPRangesRuleParams{
			IP: rule.IPRanges.IP,
		}
	}

	if rule.IPList != nil {
		params.IPList = &cf.AccessGroupIPListRuleParams{
			ID: rule.IPList.ID,
		}
	}

	if rule.Country != nil {
		params.Country = &cf.AccessGroupCountryRuleParams{
			Country: rule.Country.Country,
		}
	}

	if rule.Group != nil {
		params.Group = &cf.AccessGroupGroupRuleParams{
			ID: rule.Group.ID,
		}
	}

	if rule.ServiceToken != nil {
		params.ServiceToken = &cf.AccessGroupServiceTokenRuleParams{
			TokenID: rule.ServiceToken.TokenID,
		}
	}

	if rule.CommonName != nil {
		params.CommonName = &cf.AccessGroupCommonNameRuleParams{
			CommonName: rule.CommonName.CommonName,
		}
	}

	if rule.DevicePosture != nil {
		params.DevicePosture = &cf.AccessGroupDevicePostureRuleParams{
			IntegrationUID: rule.DevicePosture.IntegrationUID,
		}
	}

	if rule.GSuite != nil {
		params.GSuite = &cf.AccessGroupGSuiteRuleParams{
			Email:              rule.GSuite.Email,
			IdentityProviderID: rule.GSuite.IdentityProviderID,
		}
	}

	if rule.GitHub != nil {
		params.GitHub = &cf.AccessGroupGitHubRuleParams{
			Name:               rule.GitHub.Name,
			IdentityProviderID: rule.GitHub.IdentityProviderID,
			Teams:              rule.GitHub.Teams,
		}
	}

	if rule.Azure != nil {
		params.Azure = &cf.AccessGroupAzureRuleParams{
			ID:                 rule.Azure.ID,
			IdentityProviderID: rule.Azure.IdentityProviderID,
		}
	}

	if rule.Okta != nil {
		params.Okta = &cf.AccessGroupOktaRuleParams{
			Name:               rule.Okta.Name,
			IdentityProviderID: rule.Okta.IdentityProviderID,
		}
	}

	if rule.OIDC != nil {
		params.OIDC = &cf.AccessGroupOIDCRuleParams{
			ClaimName:          rule.OIDC.ClaimName,
			ClaimValue:         rule.OIDC.ClaimValue,
			IdentityProviderID: rule.OIDC.IdentityProviderID,
		}
	}

	if rule.SAML != nil {
		params.SAML = &cf.AccessGroupSAMLRuleParams{
			AttributeName:      rule.SAML.AttributeName,
			AttributeValue:     rule.SAML.AttributeValue,
			IdentityProviderID: rule.SAML.IdentityProviderID,
		}
	}

	if rule.AuthMethod != nil {
		params.AuthMethod = &cf.AccessGroupAuthMethodRuleParams{
			AuthMethod: rule.AuthMethod.AuthMethod,
		}
	}

	if rule.AuthContext != nil {
		params.AuthContext = &cf.AccessGroupAuthContextRuleParams{
			ID:                 rule.AuthContext.ID,
			AcID:               rule.AuthContext.AcID,
			IdentityProviderID: rule.AuthContext.IdentityProviderID,
		}
	}

	if rule.LoginMethod != nil {
		params.LoginMethod = &cf.AccessGroupLoginMethodRuleParams{
			ID: rule.LoginMethod.ID,
		}
	}

	if rule.ExternalEvaluation != nil {
		params.ExternalEvaluation = &cf.AccessGroupExternalEvaluationRuleParams{
			EvaluateURL: rule.ExternalEvaluation.EvaluateURL,
			KeysURL:     rule.ExternalEvaluation.KeysURL,
		}
	}

	return params
}

// SetupWithManager sets up the controller with the Manager.
func (r *GroupController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(accesssvc.ResourceTypeAccessGroup)).
		Complete(r)
}
