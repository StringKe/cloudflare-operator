// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package accessapplication provides the controller for AccessApplication CRD.
// It directly manages Access Applications in Cloudflare Zero Trust.
package accessapplication

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/controller/common"
	"github.com/StringKe/cloudflare-operator/internal/controller/refs"
)

const (
	FinalizerName = "cloudflare.com/accessapplication-finalizer"
	// StateActive indicates the resource is actively synced with Cloudflare.
	StateActive = "active"
)

// Reconciler reconciles an AccessApplication object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessapplications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessapplications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessapplications/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrllog.FromContext(ctx)

	// Fetch the AccessApplication instance
	app := &networkingv1alpha2.AccessApplication{}
	if err := r.Get(ctx, req.NamespacedName, app); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !app.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, logger, app)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, app, FinalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Get API client - use resource namespace for credentials resolution
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &app.Spec.Cloudflare,
		Namespace:         app.Namespace, // AccessApplication is now namespaced
		StatusAccountID:   app.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.setErrorStatus(ctx, app, err)
	}

	// Reconcile the application
	return r.reconcileApplication(ctx, logger, app, apiResult)
}

// handleDeletion handles the deletion of an AccessApplication.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	logger logr.Logger,
	app *networkingv1alpha2.AccessApplication,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(app, FinalizerName) {
		return common.NoRequeue(), nil
	}

	// Get API client for deletion - use resource namespace for credentials resolution
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &app.Spec.Cloudflare,
		Namespace:         app.Namespace, // AccessApplication is now namespaced
		StatusAccountID:   app.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client for deletion")
		// Continue with finalizer removal
	} else if app.Status.ApplicationID != "" {
		// Delete from Cloudflare
		logger.Info("Deleting AccessApplication from Cloudflare",
			"applicationID", app.Status.ApplicationID)

		if err := apiResult.API.DeleteAccessApplication(ctx, app.Status.ApplicationID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete AccessApplication from Cloudflare, continuing with finalizer removal")
				r.Recorder.Event(app, corev1.EventTypeWarning, "DeleteFailed",
					fmt.Sprintf("Failed to delete from Cloudflare (will remove finalizer anyway): %s", cf.SanitizeErrorMessage(err)))
				// Don't block finalizer removal - resource may need manual cleanup in Cloudflare
			} else {
				logger.Info("AccessApplication not found in Cloudflare, may have been already deleted")
			}
		} else {
			r.Recorder.Event(app, corev1.EventTypeNormal, "Deleted",
				"AccessApplication deleted from Cloudflare")
		}
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, app, func() {
		controllerutil.RemoveFinalizer(app, FinalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(app, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// reconcileApplication ensures the AccessApplication is synced with Cloudflare.
func (r *Reconciler) reconcileApplication(
	ctx context.Context,
	logger logr.Logger,
	app *networkingv1alpha2.AccessApplication,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	appName := app.GetAccessApplicationName()

	// Resolve IdP references
	allowedIdps := r.resolveAllowedIdps(ctx, logger, app, apiResult.API)

	// Resolve reusable policies
	policyIDs, err := r.resolvePolicies(ctx, logger, app, apiResult.API)
	if err != nil {
		logger.Error(err, "Failed to resolve policies")
		r.Recorder.Event(app, corev1.EventTypeWarning, "PolicyResolutionFailed",
			fmt.Sprintf("Failed to resolve policies: %s", cf.SanitizeErrorMessage(err)))
		return r.setErrorStatus(ctx, app, err)
	}

	// Build API parameters
	params := r.buildAPIParams(ctx, app, appName, allowedIdps, policyIDs, apiResult.API)

	// Check if application exists
	var result *cf.AccessApplicationResult
	if app.Status.ApplicationID != "" {
		// Try to get existing application
		existing, err := apiResult.API.GetAccessApplication(ctx, app.Status.ApplicationID)
		if err != nil {
			if cf.IsNotFoundError(err) {
				// Application was deleted externally, create new one
				logger.Info("AccessApplication not found in Cloudflare, creating new one")
				result, err = apiResult.API.CreateAccessApplication(ctx, params)
				if err != nil {
					logger.Error(err, "Failed to create AccessApplication")
					return r.setErrorStatus(ctx, app, err)
				}
				r.Recorder.Event(app, corev1.EventTypeNormal, "Created",
					fmt.Sprintf("AccessApplication '%s' created in Cloudflare", appName))
			} else {
				logger.Error(err, "Failed to get AccessApplication from Cloudflare")
				return r.setErrorStatus(ctx, app, err)
			}
		} else {
			// Update existing application
			result, err = apiResult.API.UpdateAccessApplication(ctx, app.Status.ApplicationID, params)
			if err != nil {
				logger.Error(err, "Failed to update AccessApplication")
				return r.setErrorStatus(ctx, app, err)
			}
			logger.V(1).Info("AccessApplication updated in Cloudflare",
				"applicationID", existing.ID)
		}
	} else {
		// Try to find existing by name first
		existing, err := apiResult.API.ListAccessApplicationsByName(ctx, appName)
		if err == nil && existing != nil {
			// Found existing, adopt it
			logger.Info("Found existing AccessApplication in Cloudflare, adopting",
				"applicationID", existing.ID, "name", appName)
			result, err = apiResult.API.UpdateAccessApplication(ctx, existing.ID, params)
			if err != nil {
				logger.Error(err, "Failed to update adopted AccessApplication")
				return r.setErrorStatus(ctx, app, err)
			}
			r.Recorder.Event(app, corev1.EventTypeNormal, "Adopted",
				fmt.Sprintf("Adopted existing AccessApplication '%s' (ID: %s)", appName, existing.ID))
		} else {
			// Create new application
			result, err = apiResult.API.CreateAccessApplication(ctx, params)
			if err != nil {
				logger.Error(err, "Failed to create AccessApplication")
				return r.setErrorStatus(ctx, app, err)
			}
			r.Recorder.Event(app, corev1.EventTypeNormal, "Created",
				fmt.Sprintf("AccessApplication '%s' created in Cloudflare", appName))
		}
	}

	// Update status with success
	return r.setSuccessStatus(ctx, app, apiResult.AccountID, result, policyIDs)
}

// resolvePolicies resolves ReusablePolicyRefs to Cloudflare policy IDs.
func (r *Reconciler) resolvePolicies(
	ctx context.Context,
	logger logr.Logger,
	app *networkingv1alpha2.AccessApplication,
	api *cf.API,
) ([]string, error) {
	if len(app.Spec.ReusablePolicyRefs) == 0 {
		return nil, nil
	}

	policyIDs := make([]string, 0, len(app.Spec.ReusablePolicyRefs))

	for i, ref := range app.Spec.ReusablePolicyRefs {
		policyID, err := r.resolvePolicyRef(ctx, logger, ref, api)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve policy ref at index %d: %w", i, err)
		}
		if policyID != "" {
			policyIDs = append(policyIDs, policyID)
		}
	}

	return policyIDs, nil
}

// resolvePolicyRef resolves a single ReusablePolicyRef to a Cloudflare policy ID.
//
//nolint:revive // cognitive complexity is acceptable for this resolution function
func (r *Reconciler) resolvePolicyRef(
	ctx context.Context,
	logger logr.Logger,
	ref networkingv1alpha2.ReusablePolicyRef,
	api *cf.API,
) (string, error) {
	// Priority 1: Direct Cloudflare ID
	if ref.CloudflareID != "" {
		logger.V(1).Info("Using direct Cloudflare policy ID", "policyId", ref.CloudflareID)
		return ref.CloudflareID, nil
	}

	// Priority 2: K8s AccessPolicy CR reference
	if ref.Name != "" {
		policy := &networkingv1alpha2.AccessPolicy{}
		if err := r.Get(ctx, apitypes.NamespacedName{Name: ref.Name}, policy); err != nil {
			if apierrors.IsNotFound(err) {
				return "", fmt.Errorf("AccessPolicy %q not found", ref.Name)
			}
			logger.Error(err, "Failed to get AccessPolicy", "name", ref.Name)
			return "", fmt.Errorf("failed to get AccessPolicy %q: %w", ref.Name, err)
		}
		if policy.Status.PolicyID == "" {
			return "", fmt.Errorf("AccessPolicy %q is not ready (no PolicyID in status)", ref.Name)
		}
		logger.V(1).Info("Resolved K8s AccessPolicy to Cloudflare policy ID",
			"accessPolicyName", ref.Name, "policyId", policy.Status.PolicyID)
		return policy.Status.PolicyID, nil
	}

	// Priority 3: Cloudflare name lookup
	if ref.CloudflareName != "" {
		cfPolicy, err := api.GetReusableAccessPolicyByName(ctx, ref.CloudflareName)
		if err != nil {
			return "", fmt.Errorf("failed to find policy by name %q: %w", ref.CloudflareName, err)
		}
		if cfPolicy == nil {
			return "", fmt.Errorf("policy %q not found in Cloudflare", ref.CloudflareName)
		}
		logger.V(1).Info("Resolved Cloudflare policy name to ID",
			"policyName", ref.CloudflareName, "policyId", cfPolicy.ID)
		return cfPolicy.ID, nil
	}

	return "", errors.New("invalid ReusablePolicyRef: must specify name, cloudflareId, or cloudflareName")
}

// resolveAllowedIdps resolves the allowed IdP IDs from direct IDs and refs.
// Uses the unified Resolver for consistent resolution across all reference types.
func (r *Reconciler) resolveAllowedIdps(
	ctx context.Context,
	logger logr.Logger,
	app *networkingv1alpha2.AccessApplication,
	api *cf.API,
) []string {
	resolver := refs.NewResolver(r.Client, api)

	// Use the unified resolution method
	result, errs := resolver.ResolveAllIdentityProviders(
		ctx,
		app.Spec.AllowedIdps,
		app.Spec.IdentityProviderRefs,
	)

	// Log any resolution errors
	for _, err := range errs {
		logger.Error(err, "Failed to resolve IdP reference")
	}

	return result
}

// buildAPIParams builds the Cloudflare API parameters from the spec.
// It resolves VnetRef references in destinations using the provided API client.
func (r *Reconciler) buildAPIParams(
	ctx context.Context,
	app *networkingv1alpha2.AccessApplication,
	appName string,
	allowedIdps []string,
	policyIDs []string,
	api *cf.API,
) cf.AccessApplicationParams {
	logger := ctrllog.FromContext(ctx)
	resolver := refs.NewResolver(r.Client, api)
	params := cf.AccessApplicationParams{
		Name:                     appName,
		Domain:                   app.Spec.Domain,
		SelfHostedDomains:        app.Spec.SelfHostedDomains,
		DomainType:               app.Spec.DomainType,
		PrivateAddress:           app.Spec.PrivateAddress,
		Type:                     app.Spec.Type,
		SessionDuration:          app.Spec.SessionDuration,
		AllowedIdps:              allowedIdps,
		AutoRedirectToIdentity:   &app.Spec.AutoRedirectToIdentity,
		EnableBindingCookie:      app.Spec.EnableBindingCookie,
		HTTPOnlyCookieAttribute:  app.Spec.HttpOnlyCookieAttribute,
		PathCookieAttribute:      app.Spec.PathCookieAttribute,
		SameSiteCookieAttribute:  app.Spec.SameSiteCookieAttribute,
		LogoURL:                  app.Spec.LogoURL,
		SkipInterstitial:         app.Spec.SkipInterstitial,
		OptionsPreflightBypass:   app.Spec.OptionsPreflightBypass,
		AppLauncherVisible:       app.Spec.AppLauncherVisible,
		ServiceAuth401Redirect:   app.Spec.ServiceAuth401Redirect,
		CustomDenyMessage:        app.Spec.CustomDenyMessage,
		CustomDenyURL:            app.Spec.CustomDenyURL,
		CustomNonIdentityDenyURL: app.Spec.CustomNonIdentityDenyURL,
		AllowAuthenticateViaWarp: app.Spec.AllowAuthenticateViaWarp,
		Tags:                     app.Spec.Tags,
		CustomPages:              app.Spec.CustomPages,
		GatewayRules:             app.Spec.GatewayRules,
		Policies:                 policyIDs,
	}

	// Convert destinations with VnetRef resolution
	if len(app.Spec.Destinations) > 0 {
		params.Destinations = make([]cf.AccessDestinationParams, len(app.Spec.Destinations))
		for i, dest := range app.Spec.Destinations {
			var vnetID string

			// Resolve VnetRef if specified
			if dest.VnetRef != nil {
				resolvedVnetID, err := resolver.ResolveVirtualNetwork(ctx, dest.VnetRef)
				if err != nil {
					logger.Error(err, "Failed to resolve VnetRef for destination", "index", i)
				} else {
					vnetID = resolvedVnetID
				}
			}

			params.Destinations[i] = cf.AccessDestinationParams{
				Type:       dest.Type,
				URI:        dest.URI,
				Hostname:   dest.Hostname,
				CIDR:       dest.CIDR,
				PortRange:  dest.PortRange,
				L4Protocol: dest.L4Protocol,
				VnetID:     vnetID,
			}
		}
	}

	// Convert CORS headers
	if app.Spec.CorsHeaders != nil {
		params.CorsHeaders = &cf.AccessApplicationCorsHeadersParams{
			AllowedMethods:   app.Spec.CorsHeaders.AllowedMethods,
			AllowedOrigins:   app.Spec.CorsHeaders.AllowedOrigins,
			AllowedHeaders:   app.Spec.CorsHeaders.AllowedHeaders,
			AllowAllMethods:  app.Spec.CorsHeaders.AllowAllMethods,
			AllowAllHeaders:  app.Spec.CorsHeaders.AllowAllHeaders,
			AllowAllOrigins:  app.Spec.CorsHeaders.AllowAllOrigins,
			AllowCredentials: app.Spec.CorsHeaders.AllowCredentials,
			MaxAge:           app.Spec.CorsHeaders.MaxAge,
		}
	}

	// Convert SaaS app config
	if app.Spec.SaasApp != nil {
		params.SaasApp = r.convertSaasAppConfig(app.Spec.SaasApp)
	}

	// Convert SCIM config
	if app.Spec.SCIMConfig != nil {
		params.SCIMConfig = r.convertSCIMConfig(app.Spec.SCIMConfig)
	}

	// Convert App Launcher customization
	if app.Spec.AppLauncherCustomization != nil {
		params.AppLauncherCustomization = r.convertAppLauncherCustomization(app.Spec.AppLauncherCustomization)
	}

	// Convert target contexts
	if len(app.Spec.TargetContexts) > 0 {
		params.TargetContexts = make([]cf.AccessInfrastructureTargetContextParams, len(app.Spec.TargetContexts))
		for i, tc := range app.Spec.TargetContexts {
			params.TargetContexts[i] = cf.AccessInfrastructureTargetContextParams{
				TargetAttributes: tc.TargetAttributes,
				Port:             tc.Port,
				Protocol:         tc.Protocol,
			}
		}
	}

	return params
}

// convertSaasAppConfig converts SaaS app configuration to API params.
func (r *Reconciler) convertSaasAppConfig(saas *networkingv1alpha2.SaasApplicationConfig) *cf.SaasApplicationParams {
	params := &cf.SaasApplicationParams{
		AuthType:                      saas.AuthType,
		ConsumerServiceURL:            saas.ConsumerServiceURL,
		SPEntityID:                    saas.SPEntityID,
		NameIDFormat:                  saas.NameIDFormat,
		DefaultRelayState:             saas.DefaultRelayState,
		NameIDTransformJsonata:        saas.NameIDTransformJsonata,
		SamlAttributeTransformJsonata: saas.SamlAttributeTransformJsonata,
		RedirectURIs:                  saas.RedirectURIs,
		GrantTypes:                    saas.GrantTypes,
		Scopes:                        saas.Scopes,
		AppLauncherURL:                saas.AppLauncherURL,
		GroupFilterRegex:              saas.GroupFilterRegex,
		AllowPKCEWithoutClientSecret:  saas.AllowPKCEWithoutClientSecret,
		AccessTokenLifetime:           saas.AccessTokenLifetime,
	}

	// Convert custom attributes
	if len(saas.CustomAttributes) > 0 {
		params.CustomAttributes = make([]cf.SAMLAttributeConfigParams, len(saas.CustomAttributes))
		for i, attr := range saas.CustomAttributes {
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
	if len(saas.CustomClaims) > 0 {
		params.CustomClaims = make([]cf.OIDCClaimConfigParams, len(saas.CustomClaims))
		for i, claim := range saas.CustomClaims {
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
	if saas.RefreshTokenOptions != nil {
		params.RefreshTokenOptions = &cf.RefreshTokenOptionsParams{
			Lifetime: saas.RefreshTokenOptions.Lifetime,
		}
	}

	// Convert hybrid and implicit options
	if saas.HybridAndImplicitOptions != nil {
		params.HybridAndImplicitOptions = &cf.HybridAndImplicitOptionsParams{
			ReturnIDTokenFromAuthorizationEndpoint:     saas.HybridAndImplicitOptions.ReturnIDTokenFromAuthorizationEndpoint,
			ReturnAccessTokenFromAuthorizationEndpoint: saas.HybridAndImplicitOptions.ReturnAccessTokenFromAuthorizationEndpoint,
		}
	}

	return params
}

// convertSCIMConfig converts SCIM configuration to API params.
func (r *Reconciler) convertSCIMConfig(scim *networkingv1alpha2.AccessApplicationSCIMConfig) *cf.AccessApplicationSCIMConfigParams {
	params := &cf.AccessApplicationSCIMConfigParams{
		Enabled:            scim.Enabled,
		RemoteURI:          scim.RemoteURI,
		IDPUID:             scim.IDPUID,
		DeactivateOnDelete: scim.DeactivateOnDelete,
	}

	// Convert authentication
	if scim.Authentication != nil {
		params.Authentication = &cf.SCIMAuthenticationParams{
			Scheme:           scim.Authentication.Scheme,
			User:             scim.Authentication.User,
			Password:         scim.Authentication.Password,
			Token:            scim.Authentication.Token,
			ClientID:         scim.Authentication.ClientID,
			ClientSecret:     scim.Authentication.ClientSecret,
			AuthorizationURL: scim.Authentication.AuthorizationURL,
			TokenURL:         scim.Authentication.TokenURL,
			Scopes:           scim.Authentication.Scopes,
		}
	}

	// Convert mappings
	if len(scim.Mappings) > 0 {
		params.Mappings = make([]cf.SCIMMappingParams, len(scim.Mappings))
		for i, mapping := range scim.Mappings {
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

// convertAppLauncherCustomization converts App Launcher customization to API params.
func (r *Reconciler) convertAppLauncherCustomization(
	custom *networkingv1alpha2.AccessAppLauncherCustomization,
) *cf.AccessAppLauncherCustomizationParams {
	params := &cf.AccessAppLauncherCustomizationParams{
		AppLauncherLogoURL:       custom.AppLauncherLogoURL,
		HeaderBackgroundColor:    custom.HeaderBackgroundColor,
		BackgroundColor:          custom.BackgroundColor,
		SkipAppLauncherLoginPage: custom.SkipAppLauncherLoginPage,
	}

	// Convert landing page design
	if custom.LandingPageDesign != nil {
		params.LandingPageDesign = &cf.AccessLandingPageDesignParams{
			Title:           custom.LandingPageDesign.Title,
			Message:         custom.LandingPageDesign.Message,
			ImageURL:        custom.LandingPageDesign.ImageURL,
			ButtonColor:     custom.LandingPageDesign.ButtonColor,
			ButtonTextColor: custom.LandingPageDesign.ButtonTextColor,
		}
	}

	// Convert footer links
	if len(custom.FooterLinks) > 0 {
		params.FooterLinks = make([]cf.AccessFooterLinkParams, len(custom.FooterLinks))
		for i, link := range custom.FooterLinks {
			params.FooterLinks[i] = cf.AccessFooterLinkParams{
				Name: link.Name,
				URL:  link.URL,
			}
		}
	}

	return params
}

// setSuccessStatus updates the application status with success.
func (r *Reconciler) setSuccessStatus(
	ctx context.Context,
	app *networkingv1alpha2.AccessApplication,
	accountID string,
	result *cf.AccessApplicationResult,
	policyIDs []string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, app, func() {
		app.Status.AccountID = accountID
		app.Status.ApplicationID = result.ID
		app.Status.AUD = result.AUD
		app.Status.Domain = result.Domain
		app.Status.SelfHostedDomains = result.SelfHostedDomains
		app.Status.State = StateActive
		app.Status.SaasAppClientID = result.SaasAppClientID
		app.Status.ObservedGeneration = app.Generation
		app.Status.ResolvedPolicyIDs = policyIDs

		meta.SetStatusCondition(&app.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: app.Generation,
			Reason:             "Synced",
			Message:            "AccessApplication synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
}

// setErrorStatus updates the application status with an error.
func (r *Reconciler) setErrorStatus(
	ctx context.Context,
	app *networkingv1alpha2.AccessApplication,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, app, func() {
		app.Status.State = "error"
		app.Status.ObservedGeneration = app.Generation
		meta.SetStatusCondition(&app.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: app.Generation,
			Reason:             "Error",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
}

// ============================================================================
// Watch handler helper functions
// ============================================================================

// appReferencesIDP checks if an AccessApplication references the given AccessIdentityProvider.
func appReferencesIDP(app *networkingv1alpha2.AccessApplication, idpName string) bool {
	// Check IdentityProviderRefs
	for _, ref := range app.Spec.IdentityProviderRefs {
		if ref.Name == idpName {
			return true
		}
	}
	return false
}

// appReferencesAccessGroup checks if an AccessApplication references the given AccessGroup.
func appReferencesAccessGroup(app *networkingv1alpha2.AccessApplication, groupName string) bool {
	for _, policy := range app.Spec.Policies {
		if policy.Name == groupName {
			return true
		}
	}
	return false
}

// appReferencesAccessPolicy checks if an AccessApplication references the given AccessPolicy.
func appReferencesAccessPolicy(app *networkingv1alpha2.AccessApplication, policyName string) bool {
	for _, ref := range app.Spec.ReusablePolicyRefs {
		if ref.Name == policyName {
			return true
		}
	}
	return false
}

// findAccessApplicationsForIdentityProvider returns reconcile requests for AccessApplications
// that reference the given AccessIdentityProvider.
func (r *Reconciler) findAccessApplicationsForIdentityProvider(ctx context.Context, obj client.Object) []reconcile.Request {
	idp, ok := obj.(*networkingv1alpha2.AccessIdentityProvider)
	if !ok {
		return nil
	}
	logger := ctrllog.FromContext(ctx)

	appList := &networkingv1alpha2.AccessApplicationList{}
	if err := r.List(ctx, appList); err != nil {
		logger.Error(err, "Failed to list AccessApplications for IdentityProvider watch")
		return nil
	}

	var requests []reconcile.Request
	for i := range appList.Items {
		app := &appList.Items[i]
		if appReferencesIDP(app, idp.Name) {
			requests = append(requests, reconcile.Request{
				NamespacedName: apitypes.NamespacedName{
					Name:      app.Name,
					Namespace: app.Namespace,
				},
			})
		}
	}

	return requests
}

// findAccessApplicationsForAccessGroup returns reconcile requests for AccessApplications
// that reference the given AccessGroup.
func (r *Reconciler) findAccessApplicationsForAccessGroup(ctx context.Context, obj client.Object) []reconcile.Request {
	accessGroup, ok := obj.(*networkingv1alpha2.AccessGroup)
	if !ok {
		return nil
	}
	logger := ctrllog.FromContext(ctx)

	appList := &networkingv1alpha2.AccessApplicationList{}
	if err := r.List(ctx, appList); err != nil {
		logger.Error(err, "Failed to list AccessApplications for AccessGroup watch")
		return nil
	}

	var requests []reconcile.Request
	for i := range appList.Items {
		app := &appList.Items[i]
		if appReferencesAccessGroup(app, accessGroup.Name) {
			requests = append(requests, reconcile.Request{
				NamespacedName: apitypes.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
		}
	}

	return requests
}

// findAccessApplicationsForAccessPolicy returns reconcile requests for AccessApplications
// that reference the given AccessPolicy.
func (r *Reconciler) findAccessApplicationsForAccessPolicy(ctx context.Context, obj client.Object) []reconcile.Request {
	policy, ok := obj.(*networkingv1alpha2.AccessPolicy)
	if !ok {
		return nil
	}
	logger := ctrllog.FromContext(ctx)

	appList := &networkingv1alpha2.AccessApplicationList{}
	if err := r.List(ctx, appList); err != nil {
		logger.Error(err, "Failed to list AccessApplications for AccessPolicy watch")
		return nil
	}

	var requests []reconcile.Request
	for i := range appList.Items {
		app := &appList.Items[i]
		if appReferencesAccessPolicy(app, policy.Name) {
			requests = append(requests, reconcile.Request{
				NamespacedName: apitypes.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
		}
	}

	return requests
}

// findAccessApplicationsForIngress returns reconcile requests for AccessApplications
// whose domains match the Ingress hosts.
func (r *Reconciler) findAccessApplicationsForIngress(ctx context.Context, obj client.Object) []reconcile.Request {
	ing, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return nil
	}
	logger := ctrllog.FromContext(ctx)

	var hosts []string
	for _, rule := range ing.Spec.Rules {
		if rule.Host != "" {
			hosts = append(hosts, rule.Host)
		}
	}
	if len(hosts) == 0 {
		return nil
	}

	appList := &networkingv1alpha2.AccessApplicationList{}
	if err := r.List(ctx, appList); err != nil {
		logger.Error(err, "Failed to list AccessApplications for Ingress watch")
		return nil
	}

	var requests []reconcile.Request
	for i := range appList.Items {
		app := &appList.Items[i]
		// Only trigger if the app is not yet ready
		if app.Status.State == StateActive {
			continue
		}
		if domainMatchesHosts(app.Spec.Domain, hosts) ||
			anyDomainMatchesHosts(app.Spec.SelfHostedDomains, hosts) {
			requests = append(requests, reconcile.Request{
				NamespacedName: apitypes.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
		}
	}

	return requests
}

// findAccessApplicationsForTunnel returns reconcile requests when a Tunnel changes.
func (r *Reconciler) findAccessApplicationsForTunnel(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := ctrllog.FromContext(ctx)

	appList := &networkingv1alpha2.AccessApplicationList{}
	if err := r.List(ctx, appList); err != nil {
		logger.Error(err, "Failed to list AccessApplications for Tunnel watch")
		return nil
	}

	var requests []reconcile.Request
	for i := range appList.Items {
		app := &appList.Items[i]
		if app.Status.State != StateActive {
			requests = append(requests, reconcile.Request{
				NamespacedName: apitypes.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
		}
	}

	return requests
}

// domainMatchesHosts checks if a domain matches any of the given hosts.
func domainMatchesHosts(domain string, hosts []string) bool {
	if domain == "" {
		return false
	}
	domainLower := strings.ToLower(domain)
	for _, host := range hosts {
		hostLower := strings.ToLower(host)
		if domainLower == hostLower {
			return true
		}
		// Wildcard matching
		if len(hostLower) > 2 && hostLower[:2] == "*." {
			baseDomain := hostLower[2:]
			if strings.HasSuffix(domainLower, "."+baseDomain) || domainLower == baseDomain {
				return true
			}
		}
		if len(domainLower) > 2 && domainLower[:2] == "*." {
			baseDomain := domainLower[2:]
			if strings.HasSuffix(hostLower, "."+baseDomain) || hostLower == baseDomain {
				return true
			}
		}
	}
	return false
}

// anyDomainMatchesHosts checks if any domain in the list matches any host.
func anyDomainMatchesHosts(domains []string, hosts []string) bool {
	for _, domain := range domains {
		if domainMatchesHosts(domain, hosts) {
			return true
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("accessapplication-controller")
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("accessapplication"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.AccessApplication{}).
		Watches(
			&networkingv1alpha2.AccessIdentityProvider{},
			handler.EnqueueRequestsFromMapFunc(r.findAccessApplicationsForIdentityProvider),
		).
		Watches(
			&networkingv1alpha2.AccessGroup{},
			handler.EnqueueRequestsFromMapFunc(r.findAccessApplicationsForAccessGroup),
		).
		Watches(
			&networkingv1alpha2.AccessPolicy{},
			handler.EnqueueRequestsFromMapFunc(r.findAccessApplicationsForAccessPolicy),
		).
		Watches(
			&networkingv1.Ingress{},
			handler.EnqueueRequestsFromMapFunc(r.findAccessApplicationsForIngress),
		).
		Watches(
			&networkingv1alpha2.ClusterTunnel{},
			handler.EnqueueRequestsFromMapFunc(r.findAccessApplicationsForTunnel),
		).
		Watches(
			&networkingv1alpha2.Tunnel{},
			handler.EnqueueRequestsFromMapFunc(r.findAccessApplicationsForTunnel),
		).
		Complete(r)
}
