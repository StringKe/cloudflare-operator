// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package access provides the Access Sync Controllers for managing Cloudflare Access resources.
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
	// ApplicationFinalizerName is the finalizer for AccessApplication SyncState resources.
	ApplicationFinalizerName = "accessapplication.sync.cloudflare-operator.io/finalizer"
)

// ApplicationController is the Sync Controller for AccessApplication Configuration.
type ApplicationController struct {
	*common.BaseSyncController
}

// NewApplicationController creates a new AccessApplication sync controller.
func NewApplicationController(c client.Client) *ApplicationController {
	return &ApplicationController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for AccessApplication.
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *ApplicationController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "AccessApplicationSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process AccessApplication type
	if syncState.Spec.ResourceType != accesssvc.ResourceTypeAccessApplication {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing AccessApplication SyncState",
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
	if !controllerutil.ContainsFinalizer(syncState, ApplicationFinalizerName) {
		controllerutil.AddFinalizer(syncState, ApplicationFinalizerName)
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

	// Extract AccessApplication configuration from first source
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract AccessApplication configuration")
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
		logger.Error(err, "Failed to sync AccessApplication to Cloudflare")
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

	logger.Info("Successfully synced AccessApplication to Cloudflare",
		"applicationId", result.ID,
		"name", config.Name)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts AccessApplication configuration from SyncState sources.
// AccessApplication has 1:1 mapping, so we use the ExtractFirstSourceConfig helper.
func (*ApplicationController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*accesssvc.AccessApplicationConfig, error) {
	return common.ExtractFirstSourceConfig[accesssvc.AccessApplicationConfig](syncState)
}

// syncToCloudflare syncs the AccessApplication configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *ApplicationController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *accesssvc.AccessApplicationConfig,
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

	// Build AccessApplication params
	params := r.buildParams(config)

	// Check if this is an existing app or new (pending)
	cloudflareID := syncState.Spec.CloudflareID
	var result *cf.AccessApplicationResult

	if common.IsPendingID(cloudflareID) {
		// Create new AccessApplication
		logger.Info("Creating new AccessApplication",
			"name", config.Name,
			"domain", config.Domain,
			"type", config.Type)

		result, err = apiClient.CreateAccessApplication(params)
		if err != nil {
			return nil, fmt.Errorf("create AccessApplication: %w", err)
		}

		// Update SyncState CloudflareID with the actual application ID
		common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID)

		logger.Info("Created AccessApplication",
			"applicationId", result.ID,
			"aud", result.AUD)
	} else {
		// Update existing AccessApplication
		logger.Info("Updating AccessApplication",
			"applicationId", cloudflareID,
			"name", config.Name)

		result, err = apiClient.UpdateAccessApplication(cloudflareID, params)
		if err != nil {
			// Check if app was deleted externally
			if common.HandleNotFoundOnUpdate(err) {
				logger.Info("AccessApplication not found, recreating",
					"applicationId", cloudflareID)
				result, err = apiClient.CreateAccessApplication(params)
				if err != nil {
					return nil, fmt.Errorf("recreate AccessApplication: %w", err)
				}

				// Update SyncState CloudflareID if ID changed
				if result.ID != cloudflareID {
					common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID)
				}
			} else {
				return nil, fmt.Errorf("update AccessApplication: %w", err)
			}
		}

		logger.Info("Updated AccessApplication",
			"applicationId", result.ID)
	}

	return &accesssvc.SyncResult{
		ID:        result.ID,
		AccountID: accountID,
	}, nil
}

// buildParams builds AccessApplicationParams from config.
//
//nolint:revive // cognitive complexity is acceptable for building params
func (*ApplicationController) buildParams(config *accesssvc.AccessApplicationConfig) cf.AccessApplicationParams {
	params := cf.AccessApplicationParams{
		Name:                     config.Name,
		Domain:                   config.Domain,
		SelfHostedDomains:        config.SelfHostedDomains,
		DomainType:               config.DomainType,
		PrivateAddress:           config.PrivateAddress,
		Type:                     config.Type,
		SessionDuration:          config.SessionDuration,
		AllowedIdps:              config.AllowedIdps,
		AutoRedirectToIdentity:   boolPtr(config.AutoRedirectToIdentity),
		EnableBindingCookie:      config.EnableBindingCookie,
		HTTPOnlyCookieAttribute:  config.HTTPOnlyCookieAttribute,
		PathCookieAttribute:      config.PathCookieAttribute,
		SameSiteCookieAttribute:  config.SameSiteCookieAttribute,
		LogoURL:                  config.LogoURL,
		SkipInterstitial:         config.SkipInterstitial,
		OptionsPreflightBypass:   config.OptionsPreflightBypass,
		AppLauncherVisible:       config.AppLauncherVisible,
		ServiceAuth401Redirect:   config.ServiceAuth401Redirect,
		CustomDenyMessage:        config.CustomDenyMessage,
		CustomDenyURL:            config.CustomDenyURL,
		CustomNonIdentityDenyURL: config.CustomNonIdentityDenyURL,
		AllowAuthenticateViaWarp: config.AllowAuthenticateViaWarp,
		Tags:                     config.Tags,
		CustomPages:              config.CustomPages,
		GatewayRules:             config.GatewayRules,
	}

	// Convert destinations
	if len(config.Destinations) > 0 {
		params.Destinations = make([]cf.AccessDestinationParams, len(config.Destinations))
		for i, dest := range config.Destinations {
			params.Destinations[i] = cf.AccessDestinationParams{
				Type:       dest.Type,
				URI:        dest.URI,
				Hostname:   dest.Hostname,
				CIDR:       dest.CIDR,
				PortRange:  dest.PortRange,
				L4Protocol: dest.L4Protocol,
				VnetID:     dest.VnetID,
			}
		}
	}

	// Convert CORS headers
	if config.CorsHeaders != nil {
		params.CorsHeaders = &cf.AccessApplicationCorsHeadersParams{
			AllowedMethods:   config.CorsHeaders.AllowedMethods,
			AllowedOrigins:   config.CorsHeaders.AllowedOrigins,
			AllowedHeaders:   config.CorsHeaders.AllowedHeaders,
			AllowAllMethods:  config.CorsHeaders.AllowAllMethods,
			AllowAllHeaders:  config.CorsHeaders.AllowAllHeaders,
			AllowAllOrigins:  config.CorsHeaders.AllowAllOrigins,
			AllowCredentials: config.CorsHeaders.AllowCredentials,
			MaxAge:           config.CorsHeaders.MaxAge,
		}
	}

	// Convert SaaS app config
	if config.SaasApp != nil {
		params.SaasApp = convertSaasAppConfig(config.SaasApp)
	}

	// Convert SCIM config
	if config.SCIMConfig != nil {
		params.SCIMConfig = convertSCIMConfig(config.SCIMConfig)
	}

	// Convert App Launcher customization
	if config.AppLauncherCustomization != nil {
		params.AppLauncherCustomization = convertAppLauncherCustomization(config.AppLauncherCustomization)
	}

	// Convert target contexts
	if len(config.TargetContexts) > 0 {
		params.TargetContexts = make([]cf.AccessInfrastructureTargetContextParams, len(config.TargetContexts))
		for i, tc := range config.TargetContexts {
			params.TargetContexts[i] = cf.AccessInfrastructureTargetContextParams{
				TargetAttributes: tc.TargetAttributes,
				Port:             tc.Port,
				Protocol:         tc.Protocol,
			}
		}
	}

	return params
}

// Helper functions for type conversion

//nolint:revive // flag-parameter is acceptable for pointer conversion helper
func boolPtr(b bool) *bool {
	if !b {
		return nil
	}
	return &b
}

//nolint:revive // cognitive complexity is acceptable for config conversion
func convertSaasAppConfig(config *v1alpha2.SaasApplicationConfig) *cf.SaasApplicationParams {
	params := &cf.SaasApplicationParams{
		AuthType:                      config.AuthType,
		ConsumerServiceURL:            config.ConsumerServiceURL,
		SPEntityID:                    config.SPEntityID,
		NameIDFormat:                  config.NameIDFormat,
		DefaultRelayState:             config.DefaultRelayState,
		NameIDTransformJsonata:        config.NameIDTransformJsonata,
		SamlAttributeTransformJsonata: config.SamlAttributeTransformJsonata,
		RedirectURIs:                  config.RedirectURIs,
		GrantTypes:                    config.GrantTypes,
		Scopes:                        config.Scopes,
		AppLauncherURL:                config.AppLauncherURL,
		GroupFilterRegex:              config.GroupFilterRegex,
		AllowPKCEWithoutClientSecret:  config.AllowPKCEWithoutClientSecret,
		AccessTokenLifetime:           config.AccessTokenLifetime,
	}

	// Convert custom attributes
	if len(config.CustomAttributes) > 0 {
		params.CustomAttributes = make([]cf.SAMLAttributeConfigParams, len(config.CustomAttributes))
		for i, attr := range config.CustomAttributes {
			params.CustomAttributes[i] = cf.SAMLAttributeConfigParams{
				Name:         attr.Name,
				NameFormat:   attr.NameFormat,
				FriendlyName: attr.FriendlyName,
				Required:     attr.Required,
				Source: cf.SAMLAttributeSourceParams{
					Name:      attr.Source.Name,
					NameByIDP: attr.Source.NameByIDP,
				},
			}
		}
	}

	// Convert custom claims
	if len(config.CustomClaims) > 0 {
		params.CustomClaims = make([]cf.OIDCClaimConfigParams, len(config.CustomClaims))
		for i, claim := range config.CustomClaims {
			params.CustomClaims[i] = cf.OIDCClaimConfigParams{
				Name:     claim.Name,
				Required: claim.Required,
				Scope:    claim.Scope,
				Source: cf.OIDCClaimSourceParams{
					Name:      claim.Source.Name,
					NameByIDP: claim.Source.NameByIDP,
				},
			}
		}
	}

	// Convert refresh token options
	if config.RefreshTokenOptions != nil {
		params.RefreshTokenOptions = &cf.RefreshTokenOptionsParams{
			Lifetime: config.RefreshTokenOptions.Lifetime,
		}
	}

	// Convert hybrid and implicit options
	if config.HybridAndImplicitOptions != nil {
		params.HybridAndImplicitOptions = &cf.HybridAndImplicitOptionsParams{
			ReturnIDTokenFromAuthorizationEndpoint:     config.HybridAndImplicitOptions.ReturnIDTokenFromAuthorizationEndpoint,
			ReturnAccessTokenFromAuthorizationEndpoint: config.HybridAndImplicitOptions.ReturnAccessTokenFromAuthorizationEndpoint,
		}
	}

	return params
}

func convertSCIMConfig(config *v1alpha2.AccessApplicationSCIMConfig) *cf.AccessApplicationSCIMConfigParams {
	params := &cf.AccessApplicationSCIMConfigParams{
		Enabled:            config.Enabled,
		RemoteURI:          config.RemoteURI,
		IDPUID:             config.IDPUID,
		DeactivateOnDelete: config.DeactivateOnDelete,
	}

	// Convert authentication
	if config.Authentication != nil {
		params.Authentication = &cf.SCIMAuthenticationParams{
			Scheme:           config.Authentication.Scheme,
			User:             config.Authentication.User,
			Password:         config.Authentication.Password,
			Token:            config.Authentication.Token,
			ClientID:         config.Authentication.ClientID,
			ClientSecret:     config.Authentication.ClientSecret,
			AuthorizationURL: config.Authentication.AuthorizationURL,
			TokenURL:         config.Authentication.TokenURL,
			Scopes:           config.Authentication.Scopes,
		}
	}

	// Convert mappings
	if len(config.Mappings) > 0 {
		params.Mappings = make([]cf.SCIMMappingParams, len(config.Mappings))
		for i, mapping := range config.Mappings {
			params.Mappings[i] = cf.SCIMMappingParams{
				Schema:           mapping.Schema,
				Enabled:          mapping.Enabled,
				Filter:           mapping.Filter,
				TransformJsonata: mapping.TransformJsonata,
				Strictness:       mapping.Strictness,
			}
			if mapping.Operations != nil {
				params.Mappings[i].Operations = &cf.SCIMMappingOperationsParams{
					Create: mapping.Operations.Create,
					Update: mapping.Operations.Update,
					Delete: mapping.Operations.Delete,
				}
			}
		}
	}

	return params
}

func convertAppLauncherCustomization(config *v1alpha2.AccessAppLauncherCustomization) *cf.AccessAppLauncherCustomizationParams {
	params := &cf.AccessAppLauncherCustomizationParams{
		AppLauncherLogoURL:       config.AppLauncherLogoURL,
		HeaderBackgroundColor:    config.HeaderBackgroundColor,
		BackgroundColor:          config.BackgroundColor,
		SkipAppLauncherLoginPage: config.SkipAppLauncherLoginPage,
	}

	// Convert landing page design
	if config.LandingPageDesign != nil {
		params.LandingPageDesign = &cf.AccessLandingPageDesignParams{
			Title:           config.LandingPageDesign.Title,
			Message:         config.LandingPageDesign.Message,
			ImageURL:        config.LandingPageDesign.ImageURL,
			ButtonColor:     config.LandingPageDesign.ButtonColor,
			ButtonTextColor: config.LandingPageDesign.ButtonTextColor,
		}
	}

	// Convert footer links
	if len(config.FooterLinks) > 0 {
		params.FooterLinks = make([]cf.AccessFooterLinkParams, len(config.FooterLinks))
		for i, link := range config.FooterLinks {
			params.FooterLinks[i] = cf.AccessFooterLinkParams{
				Name: link.Name,
				URL:  link.URL,
			}
		}
	}

	return params
}

// handleDeletion handles the deletion of AccessApplication from Cloudflare.
// This is the SINGLE point for Cloudflare AccessApplication deletion in the system.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller deletes from Cloudflare
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps and error handling
func (r *ApplicationController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, ApplicationFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Get the Cloudflare application ID
	cloudflareID := syncState.Spec.CloudflareID

	// Skip if pending ID (application was never created)
	if common.IsPendingID(cloudflareID) {
		logger.Info("Skipping deletion - AccessApplication was never created",
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

		logger.Info("Deleting AccessApplication from Cloudflare",
			"applicationId", cloudflareID)

		if err := apiClient.DeleteAccessApplication(cloudflareID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete AccessApplication from Cloudflare")
				if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
					logger.Error(statusErr, "Failed to update error status")
				}
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}
			logger.Info("AccessApplication already deleted from Cloudflare")
		} else {
			logger.Info("Successfully deleted AccessApplication from Cloudflare",
				"applicationId", cloudflareID)
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(syncState, ApplicationFinalizerName)
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
func (r *ApplicationController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(accesssvc.ResourceTypeAccessApplication)).
		Complete(r)
}
