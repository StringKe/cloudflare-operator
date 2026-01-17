// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package access

import (
	"context"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
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
	// IdentityProviderFinalizerName is the finalizer for AccessIdentityProvider SyncState resources.
	IdentityProviderFinalizerName = "accessidentityprovider.sync.cloudflare-operator.io/finalizer"
)

// IdentityProviderController is the Sync Controller for AccessIdentityProvider Configuration.
type IdentityProviderController struct {
	*common.BaseSyncController
}

// NewIdentityProviderController creates a new AccessIdentityProvider sync controller.
func NewIdentityProviderController(c client.Client) *IdentityProviderController {
	return &IdentityProviderController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for AccessIdentityProvider.
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *IdentityProviderController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "AccessIdentityProviderSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process AccessIdentityProvider type
	if syncState.Spec.ResourceType != accesssvc.ResourceTypeAccessIdentityProvider {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing AccessIdentityProvider SyncState",
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

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(syncState, IdentityProviderFinalizerName) {
		controllerutil.AddFinalizer(syncState, IdentityProviderFinalizerName)
		if err := r.Client.Update(ctx, syncState); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip if there's a pending debounced request
	if r.Debouncer.IsPending(req.Name) {
		logger.V(1).Info("Skipping reconcile - debounced request pending")
		return ctrl.Result{}, nil
	}

	// Extract AccessIdentityProvider configuration from first source
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract AccessIdentityProvider configuration")
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
		logger.Error(err, "Failed to sync AccessIdentityProvider to Cloudflare")
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

	logger.Info("Successfully synced AccessIdentityProvider to Cloudflare",
		"providerId", result.ID,
		"name", config.Name,
		"type", config.Type)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts AccessIdentityProvider configuration from SyncState sources.
// AccessIdentityProvider has 1:1 mapping, so we use the ExtractFirstSourceConfig helper.
func (*IdentityProviderController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*accesssvc.AccessIdentityProviderConfig, error) {
	return common.ExtractFirstSourceConfig[accesssvc.AccessIdentityProviderConfig](syncState)
}

// syncToCloudflare syncs the AccessIdentityProvider configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *IdentityProviderController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *accesssvc.AccessIdentityProviderConfig,
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

	// Build AccessIdentityProvider params
	params := r.buildParams(config)

	// Check if this is an existing provider or new (pending)
	cloudflareID := syncState.Spec.CloudflareID
	var result *cf.AccessIdentityProviderResult

	if common.IsPendingID(cloudflareID) {
		// Create new identity provider
		logger.Info("Creating new AccessIdentityProvider",
			"name", config.Name,
			"type", config.Type)

		result, err = apiClient.CreateAccessIdentityProvider(params)
		if err != nil {
			return nil, fmt.Errorf("create AccessIdentityProvider: %w", err)
		}

		// Update SyncState CloudflareID with the actual provider ID
		common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID)

		logger.Info("Created AccessIdentityProvider",
			"providerId", result.ID)
	} else {
		// Update existing provider
		logger.Info("Updating AccessIdentityProvider",
			"providerId", cloudflareID,
			"name", config.Name)

		result, err = apiClient.UpdateAccessIdentityProvider(cloudflareID, params)
		if err != nil {
			// Check if provider was deleted externally
			if common.HandleNotFoundOnUpdate(err) {
				logger.Info("AccessIdentityProvider not found, recreating",
					"providerId", cloudflareID)
				result, err = apiClient.CreateAccessIdentityProvider(params)
				if err != nil {
					return nil, fmt.Errorf("recreate AccessIdentityProvider: %w", err)
				}

				// Update SyncState CloudflareID if ID changed
				if result.ID != cloudflareID {
					common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID)
				}
			} else {
				return nil, fmt.Errorf("update AccessIdentityProvider: %w", err)
			}
		}

		logger.Info("Updated AccessIdentityProvider",
			"providerId", result.ID)
	}

	return &accesssvc.SyncResult{
		ID:        result.ID,
		AccountID: accountID,
	}, nil
}

// buildParams builds AccessIdentityProviderParams from config.
//
//nolint:revive // cognitive complexity is acceptable for building params
func (*IdentityProviderController) buildParams(config *accesssvc.AccessIdentityProviderConfig) cf.AccessIdentityProviderParams {
	params := cf.AccessIdentityProviderParams{
		Name: config.Name,
		Type: config.Type,
	}

	// Build config
	if config.Config != nil {
		params.Config = buildIdPConfig(config.Config)
	}

	// Build SCIM config
	if config.ScimConfig != nil {
		params.ScimConfig = buildScimConfig(config.ScimConfig)
	}

	return params
}

// buildIdPConfig builds cloudflare.AccessIdentityProviderConfiguration from spec.
//
//nolint:revive // cognitive complexity is acceptable for building IdP config with many fields
func buildIdPConfig(config *v1alpha2.IdentityProviderConfig) cloudflare.AccessIdentityProviderConfiguration {
	result := cloudflare.AccessIdentityProviderConfiguration{}

	if config.ClientID != "" {
		result.ClientID = config.ClientID
	}
	if config.ClientSecret != "" {
		result.ClientSecret = config.ClientSecret
	}
	if config.AuthURL != "" {
		result.AuthURL = config.AuthURL
	}
	if config.TokenURL != "" {
		result.TokenURL = config.TokenURL
	}
	if config.CertsURL != "" {
		result.CertsURL = config.CertsURL
	}
	// Handle IdP public cert (backward compatibility with deprecated IdPPublicCerts field)
	if config.IdPPublicCert != "" {
		result.IdpPublicCert = config.IdPPublicCert
	} else if len(config.IdPPublicCerts) > 0 { //nolint:staticcheck // backward compatibility
		result.IdpPublicCert = config.IdPPublicCerts[0] //nolint:staticcheck // backward compatibility
	}
	if config.SSOTargetURL != "" {
		result.SsoTargetURL = config.SSOTargetURL
	}
	if config.SignRequest != nil {
		result.SignRequest = *config.SignRequest
	}
	if config.EmailClaimName != "" {
		result.EmailClaimName = config.EmailClaimName
	}
	if len(config.Claims) > 0 {
		result.Claims = config.Claims
	}
	if len(config.Scopes) > 0 {
		result.Scopes = config.Scopes
	}
	if len(config.Attributes) > 0 {
		result.Attributes = config.Attributes
	}
	if config.DirectoryID != "" {
		result.DirectoryID = config.DirectoryID
	}
	if config.AppsDomain != "" {
		result.AppsDomain = config.AppsDomain
	}
	if config.PKCEEnabled != nil {
		result.PKCEEnabled = config.PKCEEnabled
	}
	if config.ConditionalAccessEnabled != nil {
		result.ConditionalAccessEnabled = *config.ConditionalAccessEnabled
	}
	if config.SupportGroups != nil {
		result.SupportGroups = *config.SupportGroups
	}
	if config.IssuerURL != "" {
		result.IssuerURL = config.IssuerURL
	}
	if config.EmailAttributeName != "" {
		result.EmailAttributeName = config.EmailAttributeName
	}
	if config.APIToken != "" {
		result.APIToken = config.APIToken
	}
	if config.OktaAccount != "" {
		result.OktaAccount = config.OktaAccount
	}
	if config.OktaAuthorizationServerID != "" {
		result.OktaAuthorizationServerID = config.OktaAuthorizationServerID
	}
	if config.OneloginAccount != "" {
		result.OneloginAccount = config.OneloginAccount
	}
	if config.PingEnvID != "" {
		result.PingEnvID = config.PingEnvID
	}
	if config.CentrifyAccount != "" {
		result.CentrifyAccount = config.CentrifyAccount
	}
	if config.CentrifyAppID != "" {
		result.CentrifyAppID = config.CentrifyAppID
	}
	if config.RedirectURL != "" {
		result.RedirectURL = config.RedirectURL
	}

	return result
}

// buildScimConfig builds cloudflare.AccessIdentityProviderScimConfiguration from spec.
func buildScimConfig(config *v1alpha2.IdentityProviderScimConfig) cloudflare.AccessIdentityProviderScimConfiguration {
	result := cloudflare.AccessIdentityProviderScimConfiguration{}

	if config.Enabled != nil {
		result.Enabled = *config.Enabled
	}
	if config.Secret != "" {
		result.Secret = config.Secret
	}
	if config.UserDeprovision != nil {
		result.UserDeprovision = *config.UserDeprovision
	}
	if config.SeatDeprovision != nil {
		result.SeatDeprovision = *config.SeatDeprovision
	}
	if config.GroupMemberDeprovision != nil {
		result.GroupMemberDeprovision = *config.GroupMemberDeprovision
	}
	if config.IdentityUpdateBehavior != "" {
		result.IdentityUpdateBehavior = config.IdentityUpdateBehavior
	}

	return result
}

// handleDeletion handles the deletion of AccessIdentityProvider from Cloudflare.
// This is the SINGLE point for Cloudflare AccessIdentityProvider deletion in the system.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller deletes from Cloudflare
func (r *IdentityProviderController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, IdentityProviderFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Get the Cloudflare provider ID
	cloudflareID := syncState.Spec.CloudflareID

	// Skip if pending ID (provider was never created)
	if common.IsPendingID(cloudflareID) {
		logger.Info("Skipping deletion - AccessIdentityProvider was never created",
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

		logger.Info("Deleting AccessIdentityProvider from Cloudflare",
			"providerId", cloudflareID)

		if err := apiClient.DeleteAccessIdentityProvider(cloudflareID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete AccessIdentityProvider from Cloudflare")
				if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
					logger.Error(statusErr, "Failed to update error status")
				}
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}
			logger.Info("AccessIdentityProvider already deleted from Cloudflare")
		} else {
			logger.Info("Successfully deleted AccessIdentityProvider from Cloudflare",
				"providerId", cloudflareID)
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(syncState, IdentityProviderFinalizerName)
	if err := r.Client.Update(ctx, syncState); err != nil {
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
func (r *IdentityProviderController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(accesssvc.ResourceTypeAccessIdentityProvider)).
		Complete(r)
}
