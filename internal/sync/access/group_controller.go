// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package access

//nolint:dupl // Similar patterns across resource types are intentional

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	accesssvc "github.com/StringKe/cloudflare-operator/internal/service/access"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

const (
	// GroupFinalizerName is the finalizer for AccessGroup SyncState resources.
	GroupFinalizerName = "accessgroup.sync.cloudflare-operator.io/finalizer"
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

	// Handle deletion - this is the SINGLE point for Cloudflare API delete calls
	if !syncState.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, syncState)
	}

	// Check if there are any sources - if none, delete from Cloudflare
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, deleting from Cloudflare")
		return r.handleDeletion(ctx, syncState)
	}

	// Add finalizer if not present (with conflict retry)
	if !controllerutil.ContainsFinalizer(syncState, GroupFinalizerName) {
		if err := common.AddFinalizerWithRetry(ctx, r.Client, syncState, GroupFinalizerName); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip if there's a pending debounced request
	if r.Debouncer.IsPending(req.Name) {
		logger.V(1).Info("Skipping reconcile - debounced request pending")
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
		// Even if config hasn't changed, ensure status is "Synced" if resource exists in Cloudflare
		if syncState.Status.SyncStatus != v1alpha2.SyncStatusSynced && syncState.Spec.CloudflareID != "" {
			syncResult := &common.SyncResult{ConfigHash: newHash}
			if err := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusSynced, syncResult, nil); err != nil {
				logger.Error(err, "Failed to update status to Synced")
				return ctrl.Result{Requeue: true}, nil
			}
		}
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

		result, err = apiClient.CreateAccessGroup(ctx, params)
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

		result, err = apiClient.UpdateAccessGroup(ctx, cloudflareID, params)
		if err != nil {
			// Check if group was deleted externally
			if common.HandleNotFoundOnUpdate(err) {
				logger.Info("AccessGroup not found, recreating",
					"groupId", cloudflareID)
				result, err = apiClient.CreateAccessGroup(ctx, params)
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

// handleDeletion handles the deletion of AccessGroup from Cloudflare.
// This is the SINGLE point for Cloudflare AccessGroup deletion in the system.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller deletes from Cloudflare
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps and error handling
func (r *GroupController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, GroupFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Get the Cloudflare group ID
	cloudflareID := syncState.Spec.CloudflareID

	// Skip if pending ID (group was never created)
	if common.IsPendingID(cloudflareID) {
		logger.Info("Skipping deletion - AccessGroup was never created",
			"cloudflareId", cloudflareID)
	} else if cloudflareID != "" {
		// Create API client
		apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
		if err != nil {
			logger.Error(err, "Failed to create API client for deletion")
			return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
		}

		// Set account ID
		if syncState.Spec.AccountID != "" {
			apiClient.ValidAccountId = syncState.Spec.AccountID
		}

		logger.Info("Deleting AccessGroup from Cloudflare",
			"groupId", cloudflareID)

		if err := apiClient.DeleteAccessGroup(ctx, cloudflareID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete AccessGroup from Cloudflare")
				if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
					logger.Error(statusErr, "Failed to update error status")
				}
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}
			logger.Info("AccessGroup already deleted from Cloudflare")
		} else {
			logger.Info("Successfully deleted AccessGroup from Cloudflare",
				"groupId", cloudflareID)
		}
	}

	// Remove finalizer (with conflict retry)
	if err := common.RemoveFinalizerWithRetry(ctx, r.Client, syncState, GroupFinalizerName); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	// If sources are empty (not a deletion timestamp trigger), delete the SyncState itself
	if syncState.DeletionTimestamp.IsZero() && len(syncState.Spec.Sources) == 0 {
		logger.Info("Deleting orphaned SyncState")
		if err := r.Client.Delete(ctx, syncState); err != nil {
			if client.IgnoreNotFound(err) != nil {
				logger.Error(err, "Failed to delete SyncState")
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GroupController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("access-group-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(accesssvc.ResourceTypeAccessGroup)).
		Complete(r)
}
