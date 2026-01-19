// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"context"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
)

// AccessApplicationParams contains parameters for creating/updating an Access Application.
type AccessApplicationParams struct {
	Name                     string
	Domain                   string
	SelfHostedDomains        []string
	Destinations             []AccessDestinationParams
	DomainType               string
	PrivateAddress           string
	Type                     string // self_hosted, saas, ssh, vnc, app_launcher, warp, biso, bookmark, dash_sso, infrastructure
	SessionDuration          string
	AllowedIdps              []string
	AutoRedirectToIdentity   *bool
	EnableBindingCookie      *bool
	HTTPOnlyCookieAttribute  *bool
	PathCookieAttribute      *bool
	SameSiteCookieAttribute  string
	LogoURL                  string
	SkipInterstitial         *bool
	OptionsPreflightBypass   *bool
	AppLauncherVisible       *bool
	ServiceAuth401Redirect   *bool
	CustomDenyMessage        string
	CustomDenyURL            string
	CustomNonIdentityDenyURL string
	AllowAuthenticateViaWarp *bool
	Tags                     []string
	CustomPages              []string
	GatewayRules             []string
	CorsHeaders              *AccessApplicationCorsHeadersParams
	SaasApp                  *SaasApplicationParams
	SCIMConfig               *AccessApplicationSCIMConfigParams
	AppLauncherCustomization *AccessAppLauncherCustomizationParams
	TargetContexts           []AccessInfrastructureTargetContextParams
}

// AccessDestinationParams represents a destination configuration.
type AccessDestinationParams struct {
	Type       string // public, private
	URI        string
	Hostname   string
	CIDR       string
	PortRange  string
	L4Protocol string
	VnetID     string
}

// AccessApplicationCorsHeadersParams represents CORS settings.
type AccessApplicationCorsHeadersParams struct {
	AllowedMethods   []string
	AllowedOrigins   []string
	AllowedHeaders   []string
	AllowAllMethods  bool
	AllowAllHeaders  bool
	AllowAllOrigins  bool
	AllowCredentials bool
	MaxAge           int
}

// SaasApplicationParams represents SaaS application configuration.
type SaasApplicationParams struct {
	AuthType                      string // saml, oidc
	ConsumerServiceURL            string
	SPEntityID                    string
	NameIDFormat                  string
	DefaultRelayState             string
	CustomAttributes              []SAMLAttributeConfigParams
	NameIDTransformJsonata        string
	SamlAttributeTransformJsonata string
	RedirectURIs                  []string
	GrantTypes                    []string
	Scopes                        []string
	AppLauncherURL                string
	GroupFilterRegex              string
	CustomClaims                  []OIDCClaimConfigParams
	AllowPKCEWithoutClientSecret  *bool
	AccessTokenLifetime           string
	RefreshTokenOptions           *RefreshTokenOptionsParams
	HybridAndImplicitOptions      *HybridAndImplicitOptionsParams
}

// SAMLAttributeConfigParams represents a SAML attribute configuration.
type SAMLAttributeConfigParams struct {
	Name         string
	NameFormat   string
	Source       SAMLAttributeSourceParams
	FriendlyName string
	Required     bool
}

// SAMLAttributeSourceParams represents the source of a SAML attribute.
type SAMLAttributeSourceParams struct {
	Name      string
	NameByIDP map[string]string
}

// OIDCClaimConfigParams represents an OIDC claim configuration.
type OIDCClaimConfigParams struct {
	Name     string
	Source   OIDCClaimSourceParams
	Required bool
	Scope    string
}

// OIDCClaimSourceParams represents the source of an OIDC claim.
type OIDCClaimSourceParams struct {
	Name      string
	NameByIDP map[string]string
}

// RefreshTokenOptionsParams represents refresh token options.
type RefreshTokenOptionsParams struct {
	Lifetime string
}

// HybridAndImplicitOptionsParams represents hybrid and implicit flow options.
type HybridAndImplicitOptionsParams struct {
	ReturnIDTokenFromAuthorizationEndpoint     *bool
	ReturnAccessTokenFromAuthorizationEndpoint *bool
}

// AccessApplicationSCIMConfigParams represents SCIM configuration.
type AccessApplicationSCIMConfigParams struct {
	Enabled            *bool
	RemoteURI          string
	Authentication     *SCIMAuthenticationParams
	IDPUID             string
	DeactivateOnDelete *bool
	Mappings           []SCIMMappingParams
}

// SCIMAuthenticationParams represents SCIM authentication.
type SCIMAuthenticationParams struct {
	Scheme           string // httpbasic, oauthbearertoken, oauth2
	User             string
	Password         string
	Token            string
	ClientID         string
	ClientSecret     string
	AuthorizationURL string
	TokenURL         string
	Scopes           []string
}

// SCIMMappingParams represents a SCIM mapping.
type SCIMMappingParams struct {
	Schema           string
	Enabled          *bool
	Filter           string
	TransformJsonata string
	Operations       *SCIMMappingOperationsParams
	Strictness       string
}

// SCIMMappingOperationsParams represents SCIM mapping operations.
type SCIMMappingOperationsParams struct {
	Create *bool
	Update *bool
	Delete *bool
}

// AccessAppLauncherCustomizationParams represents app launcher customization.
type AccessAppLauncherCustomizationParams struct {
	LandingPageDesign        *AccessLandingPageDesignParams
	AppLauncherLogoURL       string
	HeaderBackgroundColor    string
	BackgroundColor          string
	FooterLinks              []AccessFooterLinkParams
	SkipAppLauncherLoginPage *bool
}

// AccessLandingPageDesignParams represents landing page design.
type AccessLandingPageDesignParams struct {
	Title           string
	Message         string
	ImageURL        string
	ButtonColor     string
	ButtonTextColor string
}

// AccessFooterLinkParams represents a footer link.
type AccessFooterLinkParams struct {
	Name string
	URL  string
}

// AccessInfrastructureTargetContextParams represents target context for infrastructure apps.
type AccessInfrastructureTargetContextParams struct {
	TargetAttributes map[string][]string
	Port             int
	Protocol         string
}

// AccessApplicationResult contains the result of an Access Application operation.
type AccessApplicationResult struct {
	ID                     string
	AUD                    string
	Name                   string
	Domain                 string
	SelfHostedDomains      []string
	Type                   string
	SessionDuration        string
	AllowedIdps            []string
	AutoRedirectToIdentity bool
	SaasAppClientID        string
}

// CreateAccessApplication creates a new Access Application.
func (c *API) CreateAccessApplication(ctx context.Context, params AccessApplicationParams) (*AccessApplicationResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	createParams := cloudflare.CreateAccessApplicationParams{
		Name:                     params.Name,
		Domain:                   params.Domain,
		Type:                     cloudflare.AccessApplicationType(params.Type),
		SessionDuration:          params.SessionDuration,
		AllowedIdps:              params.AllowedIdps,
		AutoRedirectToIdentity:   params.AutoRedirectToIdentity,
		EnableBindingCookie:      params.EnableBindingCookie,
		HttpOnlyCookieAttribute:  params.HTTPOnlyCookieAttribute,
		PathCookieAttribute:      params.PathCookieAttribute,
		SameSiteCookieAttribute:  params.SameSiteCookieAttribute,
		LogoURL:                  params.LogoURL,
		SkipInterstitial:         params.SkipInterstitial,
		OptionsPreflightBypass:   params.OptionsPreflightBypass,
		AppLauncherVisible:       params.AppLauncherVisible,
		ServiceAuth401Redirect:   params.ServiceAuth401Redirect,
		CustomDenyMessage:        params.CustomDenyMessage,
		CustomDenyURL:            params.CustomDenyURL,
		CustomNonIdentityDenyURL: params.CustomNonIdentityDenyURL,
		PrivateAddress:           params.PrivateAddress,
	}

	// Set optional fields using helper functions
	applyCreateAccessAppOptionalParams(&createParams, params)

	app, err := c.CloudflareClient.CreateAccessApplication(ctx, rc, createParams)
	if err != nil {
		c.Log.Error(err, "error creating access application", "name", params.Name)
		return nil, err
	}

	c.Log.Info("Access Application created", "id", app.ID, "name", app.Name)

	return convertAccessApplicationToResult(app, c.ValidAccountId), nil
}

// GetAccessApplication retrieves an Access Application by ID.
func (c *API) GetAccessApplication(ctx context.Context, applicationID string) (*AccessApplicationResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	app, err := c.CloudflareClient.GetAccessApplication(ctx, rc, applicationID)
	if err != nil {
		c.Log.Error(err, "error getting access application", "id", applicationID)
		return nil, err
	}

	return convertAccessApplicationToResult(app, c.ValidAccountId), nil
}

// UpdateAccessApplication updates an existing Access Application.
func (c *API) UpdateAccessApplication(ctx context.Context, applicationID string, params AccessApplicationParams) (*AccessApplicationResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	updateParams := cloudflare.UpdateAccessApplicationParams{
		ID:                       applicationID,
		Name:                     params.Name,
		Domain:                   params.Domain,
		Type:                     cloudflare.AccessApplicationType(params.Type),
		SessionDuration:          params.SessionDuration,
		AllowedIdps:              params.AllowedIdps,
		AutoRedirectToIdentity:   params.AutoRedirectToIdentity,
		EnableBindingCookie:      params.EnableBindingCookie,
		HttpOnlyCookieAttribute:  params.HTTPOnlyCookieAttribute,
		PathCookieAttribute:      params.PathCookieAttribute,
		SameSiteCookieAttribute:  params.SameSiteCookieAttribute,
		LogoURL:                  params.LogoURL,
		SkipInterstitial:         params.SkipInterstitial,
		OptionsPreflightBypass:   params.OptionsPreflightBypass,
		AppLauncherVisible:       params.AppLauncherVisible,
		ServiceAuth401Redirect:   params.ServiceAuth401Redirect,
		CustomDenyMessage:        params.CustomDenyMessage,
		CustomDenyURL:            params.CustomDenyURL,
		CustomNonIdentityDenyURL: params.CustomNonIdentityDenyURL,
		PrivateAddress:           params.PrivateAddress,
	}

	// Set optional fields using helper functions
	applyUpdateAccessAppOptionalParams(&updateParams, params)

	app, err := c.CloudflareClient.UpdateAccessApplication(ctx, rc, updateParams)
	if err != nil {
		c.Log.Error(err, "error updating access application", "id", applicationID)
		return nil, err
	}

	c.Log.Info("Access Application updated", "id", app.ID, "name", app.Name)

	return convertAccessApplicationToResult(app, c.ValidAccountId), nil
}

// DeleteAccessApplication deletes an Access Application.
// This method is idempotent - returns nil if the application is already deleted.
func (c *API) DeleteAccessApplication(ctx context.Context, applicationID string) error {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	err := c.CloudflareClient.DeleteAccessApplication(ctx, rc, applicationID)
	if err != nil {
		if IsNotFoundError(err) {
			c.Log.Info("Access Application already deleted (not found)", "id", applicationID)
			return nil
		}
		c.Log.Error(err, "error deleting access application", "id", applicationID)
		return err
	}

	c.Log.Info("Access Application deleted", "id", applicationID)
	return nil
}

// AccessPolicyParams contains parameters for creating/updating an Access Policy.
type AccessPolicyParams struct {
	ApplicationID    string                  // Required: The Application ID this policy belongs to
	Name             string                  // Policy name
	Decision         string                  // allow, deny, bypass, non_identity
	Precedence       int                     // Order of evaluation (lower = higher priority)
	Include          []AccessGroupRuleParams // Include rules (e.g., group references)
	Exclude          []AccessGroupRuleParams // Exclude rules
	Require          []AccessGroupRuleParams // Require rules
	SessionDuration  *string                 // Optional session duration override
	ReusablePolicyID string                  // Optional: Reference to a reusable policy (instead of inline rules)
}

// AccessPolicyResult contains the result of an Access Policy operation.
type AccessPolicyResult struct {
	ID               string
	Name             string
	Decision         string
	Precedence       int
	ReusablePolicyID *string // Set if this policy references a reusable policy
}

// CreateAccessPolicy creates a new Access Policy for an application.
func (c *API) CreateAccessPolicy(ctx context.Context, params AccessPolicyParams) (*AccessPolicyResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	createParams := cloudflare.CreateAccessPolicyParams{
		ApplicationID: params.ApplicationID,
		Name:          params.Name,
		Decision:      params.Decision,
		Precedence:    params.Precedence,
		Include:       ConvertRulesToSDK(params.Include),
		Exclude:       ConvertRulesToSDK(params.Exclude),
		Require:       ConvertRulesToSDK(params.Require),
	}

	if params.SessionDuration != nil {
		createParams.SessionDuration = params.SessionDuration
	}

	policy, err := c.CloudflareClient.CreateAccessPolicy(ctx, rc, createParams)
	if err != nil {
		c.Log.Error(err, "error creating access policy",
			"applicationId", params.ApplicationID, "name", params.Name)
		return nil, err
	}

	c.Log.Info("Access Policy created",
		"id", policy.ID, "name", policy.Name, "applicationId", params.ApplicationID)

	return &AccessPolicyResult{
		ID:         policy.ID,
		Name:       policy.Name,
		Decision:   policy.Decision,
		Precedence: policy.Precedence,
	}, nil
}

// GetAccessPolicy retrieves an Access Policy by ID.
func (c *API) GetAccessPolicy(ctx context.Context, applicationID, policyID string) (*AccessPolicyResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	policy, err := c.CloudflareClient.GetAccessPolicy(ctx, rc, cloudflare.GetAccessPolicyParams{
		ApplicationID: applicationID,
		PolicyID:      policyID,
	})
	if err != nil {
		c.Log.Error(err, "error getting access policy",
			"applicationId", applicationID, "policyId", policyID)
		return nil, err
	}

	return &AccessPolicyResult{
		ID:         policy.ID,
		Name:       policy.Name,
		Decision:   policy.Decision,
		Precedence: policy.Precedence,
	}, nil
}

// UpdateAccessPolicy updates an existing Access Policy.
func (c *API) UpdateAccessPolicy(ctx context.Context, policyID string, params AccessPolicyParams) (*AccessPolicyResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	updateParams := cloudflare.UpdateAccessPolicyParams{
		ApplicationID: params.ApplicationID,
		PolicyID:      policyID,
		Name:          params.Name,
		Decision:      params.Decision,
		Precedence:    params.Precedence,
		Include:       ConvertRulesToSDK(params.Include),
		Exclude:       ConvertRulesToSDK(params.Exclude),
		Require:       ConvertRulesToSDK(params.Require),
	}

	if params.SessionDuration != nil {
		updateParams.SessionDuration = params.SessionDuration
	}

	policy, err := c.CloudflareClient.UpdateAccessPolicy(ctx, rc, updateParams)
	if err != nil {
		c.Log.Error(err, "error updating access policy",
			"applicationId", params.ApplicationID, "policyId", policyID)
		return nil, err
	}

	c.Log.Info("Access Policy updated",
		"id", policy.ID, "name", policy.Name, "applicationId", params.ApplicationID)

	return &AccessPolicyResult{
		ID:         policy.ID,
		Name:       policy.Name,
		Decision:   policy.Decision,
		Precedence: policy.Precedence,
	}, nil
}

// DeleteAccessPolicy deletes an Access Policy.
// This method is idempotent - returns nil if the policy is already deleted.
func (c *API) DeleteAccessPolicy(ctx context.Context, applicationID, policyID string) error {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	err := c.CloudflareClient.DeleteAccessPolicy(ctx, rc, cloudflare.DeleteAccessPolicyParams{
		ApplicationID: applicationID,
		PolicyID:      policyID,
	})
	if err != nil {
		if IsNotFoundError(err) {
			c.Log.Info("Access Policy already deleted (not found)",
				"applicationId", applicationID, "policyId", policyID)
			return nil
		}
		c.Log.Error(err, "error deleting access policy",
			"applicationId", applicationID, "policyId", policyID)
		return err
	}

	c.Log.Info("Access Policy deleted",
		"applicationId", applicationID, "policyId", policyID)
	return nil
}

// ListAccessPolicies lists all Access Policies for an application.
func (c *API) ListAccessPolicies(ctx context.Context, applicationID string) ([]AccessPolicyResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	policies, _, err := c.CloudflareClient.ListAccessPolicies(ctx, rc, cloudflare.ListAccessPoliciesParams{
		ApplicationID: applicationID,
	})
	if err != nil {
		c.Log.Error(err, "error listing access policies", "applicationId", applicationID)
		return nil, err
	}

	results := make([]AccessPolicyResult, 0, len(policies))
	for _, p := range policies {
		results = append(results, AccessPolicyResult{
			ID:         p.ID,
			Name:       p.Name,
			Decision:   p.Decision,
			Precedence: p.Precedence,
		})
	}

	return results, nil
}

// ============================================================================
// Reusable Access Policy Methods
// ============================================================================

// ReusableAccessPolicyParams contains parameters for creating/updating a reusable Access Policy.
type ReusableAccessPolicyParams struct {
	Name                         string
	Decision                     string // allow, deny, bypass, non_identity
	Precedence                   int
	Include                      []AccessGroupRuleParams
	Exclude                      []AccessGroupRuleParams
	Require                      []AccessGroupRuleParams
	SessionDuration              *string
	IsolationRequired            *bool
	PurposeJustificationRequired *bool
	PurposeJustificationPrompt   string
	ApprovalRequired             *bool
	ApprovalGroups               []AccessApprovalGroupParams
}

// AccessApprovalGroupParams contains approval group configuration.
type AccessApprovalGroupParams struct {
	EmailAddresses  []string
	EmailListUUID   string
	ApprovalsNeeded int
}

// ReusableAccessPolicyResult contains the result of a reusable Access Policy operation.
type ReusableAccessPolicyResult struct {
	ID         string
	Name       string
	Decision   string
	Precedence int
}

// buildReusablePolicyParams converts ReusableAccessPolicyParams to cloudflare.CreateAccessPolicyParams.
func buildReusablePolicyParams(params ReusableAccessPolicyParams) cloudflare.CreateAccessPolicyParams {
	createParams := cloudflare.CreateAccessPolicyParams{
		ApplicationID:                "", // Empty = reusable policy
		Name:                         params.Name,
		Decision:                     params.Decision,
		Precedence:                   params.Precedence,
		Include:                      ConvertRulesToSDK(params.Include),
		Exclude:                      ConvertRulesToSDK(params.Exclude),
		Require:                      ConvertRulesToSDK(params.Require),
		SessionDuration:              params.SessionDuration,
		IsolationRequired:            params.IsolationRequired,
		PurposeJustificationRequired: params.PurposeJustificationRequired,
		ApprovalRequired:             params.ApprovalRequired,
	}

	if params.PurposeJustificationPrompt != "" {
		createParams.PurposeJustificationPrompt = &params.PurposeJustificationPrompt
	}

	if len(params.ApprovalGroups) > 0 {
		approvalGroups := make([]cloudflare.AccessApprovalGroup, 0, len(params.ApprovalGroups))
		for _, ag := range params.ApprovalGroups {
			approvalGroups = append(approvalGroups, cloudflare.AccessApprovalGroup{
				EmailAddresses:  ag.EmailAddresses,
				EmailListUuid:   ag.EmailListUUID,
				ApprovalsNeeded: ag.ApprovalsNeeded,
			})
		}
		createParams.ApprovalGroups = approvalGroups
	}

	return createParams
}

// CreateReusableAccessPolicy creates a new reusable Access Policy (not attached to any application).
func (c *API) CreateReusableAccessPolicy(ctx context.Context, params ReusableAccessPolicyParams) (*ReusableAccessPolicyResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)
	createParams := buildReusablePolicyParams(params)

	policy, err := c.CloudflareClient.CreateAccessPolicy(ctx, rc, createParams)
	if err != nil {
		c.Log.Error(err, "error creating reusable access policy", "name", params.Name)
		return nil, err
	}

	c.Log.Info("Reusable Access Policy created", "id", policy.ID, "name", policy.Name)

	return &ReusableAccessPolicyResult{
		ID:         policy.ID,
		Name:       policy.Name,
		Decision:   policy.Decision,
		Precedence: policy.Precedence,
	}, nil
}

// GetReusableAccessPolicy retrieves a reusable Access Policy by ID.
func (c *API) GetReusableAccessPolicy(ctx context.Context, policyID string) (*ReusableAccessPolicyResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	// Get policy using empty ApplicationID (reusable policy)
	policy, err := c.CloudflareClient.GetAccessPolicy(ctx, rc, cloudflare.GetAccessPolicyParams{
		ApplicationID: "", // Empty = reusable policy
		PolicyID:      policyID,
	})
	if err != nil {
		c.Log.Error(err, "error getting reusable access policy", "policyId", policyID)
		return nil, err
	}

	return &ReusableAccessPolicyResult{
		ID:         policy.ID,
		Name:       policy.Name,
		Decision:   policy.Decision,
		Precedence: policy.Precedence,
	}, nil
}

// GetReusableAccessPolicyByName finds a reusable Access Policy by name.
// Returns nil if no policy with the given name is found.
func (c *API) GetReusableAccessPolicyByName(ctx context.Context, name string) (*ReusableAccessPolicyResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	// List reusable policies (empty ApplicationID)
	policies, _, err := c.CloudflareClient.ListAccessPolicies(ctx, rc, cloudflare.ListAccessPoliciesParams{
		ApplicationID: "", // Empty = list only reusable policies
	})
	if err != nil {
		c.Log.Error(err, "error listing reusable access policies")
		return nil, err
	}

	for _, policy := range policies {
		if policy.Name == name {
			return &ReusableAccessPolicyResult{
				ID:         policy.ID,
				Name:       policy.Name,
				Decision:   policy.Decision,
				Precedence: policy.Precedence,
			}, nil
		}
	}

	return nil, nil // Not found, return nil without error
}

// UpdateReusableAccessPolicy updates an existing reusable Access Policy.
func (c *API) UpdateReusableAccessPolicy(ctx context.Context, policyID string, params ReusableAccessPolicyParams) (*ReusableAccessPolicyResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	updateParams := cloudflare.UpdateAccessPolicyParams{
		ApplicationID: "", // Empty = reusable policy
		PolicyID:      policyID,
		Name:          params.Name,
		Decision:      params.Decision,
		Precedence:    params.Precedence,
		Include:       ConvertRulesToSDK(params.Include),
		Exclude:       ConvertRulesToSDK(params.Exclude),
		Require:       ConvertRulesToSDK(params.Require),
	}

	if params.SessionDuration != nil {
		updateParams.SessionDuration = params.SessionDuration
	}

	if params.IsolationRequired != nil {
		updateParams.IsolationRequired = params.IsolationRequired
	}

	if params.PurposeJustificationRequired != nil {
		updateParams.PurposeJustificationRequired = params.PurposeJustificationRequired
	}

	if params.PurposeJustificationPrompt != "" {
		updateParams.PurposeJustificationPrompt = &params.PurposeJustificationPrompt
	}

	if params.ApprovalRequired != nil {
		updateParams.ApprovalRequired = params.ApprovalRequired
	}

	if len(params.ApprovalGroups) > 0 {
		approvalGroups := make([]cloudflare.AccessApprovalGroup, 0, len(params.ApprovalGroups))
		for _, ag := range params.ApprovalGroups {
			approvalGroups = append(approvalGroups, cloudflare.AccessApprovalGroup{
				EmailAddresses:  ag.EmailAddresses,
				EmailListUuid:   ag.EmailListUUID,
				ApprovalsNeeded: ag.ApprovalsNeeded,
			})
		}
		updateParams.ApprovalGroups = approvalGroups
	}

	policy, err := c.CloudflareClient.UpdateAccessPolicy(ctx, rc, updateParams)
	if err != nil {
		c.Log.Error(err, "error updating reusable access policy", "policyId", policyID)
		return nil, err
	}

	c.Log.Info("Reusable Access Policy updated", "id", policy.ID, "name", policy.Name)

	return &ReusableAccessPolicyResult{
		ID:         policy.ID,
		Name:       policy.Name,
		Decision:   policy.Decision,
		Precedence: policy.Precedence,
	}, nil
}

// DeleteReusableAccessPolicy deletes a reusable Access Policy.
// This method is idempotent - returns nil if the policy is already deleted.
func (c *API) DeleteReusableAccessPolicy(ctx context.Context, policyID string) error {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	err := c.CloudflareClient.DeleteAccessPolicy(ctx, rc, cloudflare.DeleteAccessPolicyParams{
		ApplicationID: "", // Empty = reusable policy
		PolicyID:      policyID,
	})
	if err != nil {
		if IsNotFoundError(err) {
			c.Log.Info("Reusable Access Policy already deleted (not found)", "policyId", policyID)
			return nil
		}
		c.Log.Error(err, "error deleting reusable access policy", "policyId", policyID)
		return err
	}

	c.Log.Info("Reusable Access Policy deleted", "policyId", policyID)
	return nil
}

// ListReusableAccessPolicies lists all reusable Access Policies.
func (c *API) ListReusableAccessPolicies(ctx context.Context) ([]ReusableAccessPolicyResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	// List reusable policies (empty ApplicationID)
	policies, _, err := c.CloudflareClient.ListAccessPolicies(ctx, rc, cloudflare.ListAccessPoliciesParams{
		ApplicationID: "", // Empty = list only reusable policies
	})
	if err != nil {
		c.Log.Error(err, "error listing reusable access policies")
		return nil, err
	}

	results := make([]ReusableAccessPolicyResult, 0, len(policies))
	for _, p := range policies {
		results = append(results, ReusableAccessPolicyResult{
			ID:         p.ID,
			Name:       p.Name,
			Decision:   p.Decision,
			Precedence: p.Precedence,
		})
	}

	return results, nil
}

// AccessGroupRuleParams represents a typed Access Group rule for SDK conversion.
// Each rule should have exactly one field set.
type AccessGroupRuleParams struct {
	Email                *AccessGroupEmailRuleParams
	EmailDomain          *AccessGroupEmailDomainRuleParams
	EmailList            *AccessGroupEmailListRuleParams
	Everyone             bool
	IPRanges             *AccessGroupIPRangesRuleParams
	IPList               *AccessGroupIPListRuleParams
	Country              *AccessGroupCountryRuleParams
	Group                *AccessGroupGroupRuleParams
	ServiceToken         *AccessGroupServiceTokenRuleParams
	AnyValidServiceToken bool
	Certificate          bool
	CommonName           *AccessGroupCommonNameRuleParams
	DevicePosture        *AccessGroupDevicePostureRuleParams
	GSuite               *AccessGroupGSuiteRuleParams
	GitHub               *AccessGroupGitHubRuleParams
	Azure                *AccessGroupAzureRuleParams
	Okta                 *AccessGroupOktaRuleParams
	OIDC                 *AccessGroupOIDCRuleParams
	SAML                 *AccessGroupSAMLRuleParams
	AuthMethod           *AccessGroupAuthMethodRuleParams
	AuthContext          *AccessGroupAuthContextRuleParams
	LoginMethod          *AccessGroupLoginMethodRuleParams
	ExternalEvaluation   *AccessGroupExternalEvaluationRuleParams
}

// Rule params types
type AccessGroupEmailRuleParams struct{ Email string }
type AccessGroupEmailDomainRuleParams struct{ Domain string }
type AccessGroupEmailListRuleParams struct{ ID string }
type AccessGroupIPRangesRuleParams struct{ IP []string }
type AccessGroupIPListRuleParams struct{ ID string }
type AccessGroupCountryRuleParams struct{ Country []string }
type AccessGroupGroupRuleParams struct{ ID string }
type AccessGroupServiceTokenRuleParams struct{ TokenID string }
type AccessGroupCommonNameRuleParams struct{ CommonName string }
type AccessGroupDevicePostureRuleParams struct{ IntegrationUID string }
type AccessGroupGSuiteRuleParams struct {
	Email              string
	IdentityProviderID string
}
type AccessGroupGitHubRuleParams struct {
	Name               string
	Teams              []string
	IdentityProviderID string
}
type AccessGroupAzureRuleParams struct {
	ID                 string
	IdentityProviderID string
}
type AccessGroupOktaRuleParams struct {
	Name               string
	IdentityProviderID string
}
type AccessGroupOIDCRuleParams struct {
	ClaimName          string
	ClaimValue         string
	IdentityProviderID string
}
type AccessGroupSAMLRuleParams struct {
	AttributeName      string
	AttributeValue     string
	IdentityProviderID string
}
type AccessGroupAuthMethodRuleParams struct{ AuthMethod string }
type AccessGroupAuthContextRuleParams struct {
	ID                 string
	AcID               string
	IdentityProviderID string
}
type AccessGroupLoginMethodRuleParams struct{ ID string }
type AccessGroupExternalEvaluationRuleParams struct {
	EvaluateURL string
	KeysURL     string
}

// convertRuleToSDK converts a typed rule to SDK-compatible map format.
//
//nolint:revive // cognitive complexity is acceptable for this conversion
func convertRuleToSDK(rule AccessGroupRuleParams) map[string]interface{} {
	result := make(map[string]interface{})

	if rule.Email != nil {
		result["email"] = map[string]string{"email": rule.Email.Email}
	}
	if rule.EmailDomain != nil {
		result["email_domain"] = map[string]string{"domain": rule.EmailDomain.Domain}
	}
	if rule.EmailList != nil {
		result["email_list"] = map[string]string{"id": rule.EmailList.ID}
	}
	if rule.Everyone {
		result["everyone"] = struct{}{}
	}
	if rule.IPRanges != nil && len(rule.IPRanges.IP) > 0 {
		result["ip"] = map[string]string{"ip": rule.IPRanges.IP[0]}
	}
	if rule.IPList != nil {
		result["ip_list"] = map[string]string{"id": rule.IPList.ID}
	}
	if rule.Country != nil && len(rule.Country.Country) > 0 {
		result["geo"] = map[string]string{"country_code": rule.Country.Country[0]}
	}
	if rule.Group != nil {
		result["group"] = map[string]string{"id": rule.Group.ID}
	}
	if rule.ServiceToken != nil {
		result["service_token"] = map[string]string{"token_id": rule.ServiceToken.TokenID}
	}
	if rule.AnyValidServiceToken {
		result["any_valid_service_token"] = struct{}{}
	}
	if rule.Certificate {
		result["certificate"] = struct{}{}
	}
	if rule.CommonName != nil {
		result["common_name"] = map[string]string{"common_name": rule.CommonName.CommonName}
	}
	if rule.DevicePosture != nil {
		result["device_posture"] = map[string]string{"integration_uid": rule.DevicePosture.IntegrationUID}
	}
	if rule.GSuite != nil {
		result["gsuite"] = map[string]interface{}{
			"email":                rule.GSuite.Email,
			"identity_provider_id": rule.GSuite.IdentityProviderID,
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
		result["github_organization"] = ghMap
	}
	if rule.Azure != nil {
		result["azure_ad"] = map[string]interface{}{
			"id":                   rule.Azure.ID,
			"identity_provider_id": rule.Azure.IdentityProviderID,
		}
	}
	if rule.Okta != nil {
		result["okta"] = map[string]interface{}{
			"name":                 rule.Okta.Name,
			"identity_provider_id": rule.Okta.IdentityProviderID,
		}
	}
	if rule.OIDC != nil {
		result["oidc"] = map[string]interface{}{
			"claim_name":           rule.OIDC.ClaimName,
			"claim_value":          rule.OIDC.ClaimValue,
			"identity_provider_id": rule.OIDC.IdentityProviderID,
		}
	}
	if rule.SAML != nil {
		result["saml"] = map[string]interface{}{
			"attribute_name":       rule.SAML.AttributeName,
			"attribute_value":      rule.SAML.AttributeValue,
			"identity_provider_id": rule.SAML.IdentityProviderID,
		}
	}
	if rule.AuthMethod != nil {
		result["auth_method"] = map[string]string{"auth_method": rule.AuthMethod.AuthMethod}
	}
	if rule.AuthContext != nil {
		result["auth_context"] = map[string]interface{}{
			"id":                   rule.AuthContext.ID,
			"ac_id":                rule.AuthContext.AcID,
			"identity_provider_id": rule.AuthContext.IdentityProviderID,
		}
	}
	if rule.LoginMethod != nil {
		result["login_method"] = map[string]string{"id": rule.LoginMethod.ID}
	}
	if rule.ExternalEvaluation != nil {
		result["external_evaluation"] = map[string]string{
			"evaluate_url": rule.ExternalEvaluation.EvaluateURL,
			"keys_url":     rule.ExternalEvaluation.KeysURL,
		}
	}

	return result
}

// ConvertRulesToSDK converts typed rules to SDK-compatible format.
func ConvertRulesToSDK(rules []AccessGroupRuleParams) []interface{} {
	if len(rules) == 0 {
		return nil
	}
	result := make([]interface{}, 0, len(rules))
	for _, rule := range rules {
		ruleMap := convertRuleToSDK(rule)
		if len(ruleMap) > 0 {
			result = append(result, ruleMap)
		}
	}
	return result
}

// BuildGroupIncludeRule constructs an include rule that references an Access Group.
// This uses the "group" rule type with the group's UUID.
func BuildGroupIncludeRule(groupID string) AccessGroupRuleParams {
	return AccessGroupRuleParams{
		Group: &AccessGroupGroupRuleParams{ID: groupID},
	}
}

// AccessGroupParams contains parameters for creating/updating an Access Group.
type AccessGroupParams struct {
	Name      string
	Include   []AccessGroupRuleParams
	Exclude   []AccessGroupRuleParams
	Require   []AccessGroupRuleParams
	IsDefault *bool
}

// AccessGroupResult contains the result of an Access Group operation.
type AccessGroupResult struct {
	ID   string
	Name string
}

// CreateAccessGroup creates a new Access Group.
func (c *API) CreateAccessGroup(ctx context.Context, params AccessGroupParams) (*AccessGroupResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	createParams := cloudflare.CreateAccessGroupParams{
		Name:    params.Name,
		Include: ConvertRulesToSDK(params.Include),
		Exclude: ConvertRulesToSDK(params.Exclude),
		Require: ConvertRulesToSDK(params.Require),
	}

	group, err := c.CloudflareClient.CreateAccessGroup(ctx, rc, createParams)
	if err != nil {
		c.Log.Error(err, "error creating access group", "name", params.Name)
		return nil, err
	}

	c.Log.Info("Access Group created", "id", group.ID, "name", group.Name)

	return &AccessGroupResult{
		ID:   group.ID,
		Name: group.Name,
	}, nil
}

// GetAccessGroup retrieves an Access Group by ID.
func (c *API) GetAccessGroup(ctx context.Context, groupID string) (*AccessGroupResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	group, err := c.CloudflareClient.GetAccessGroup(ctx, rc, groupID)
	if err != nil {
		c.Log.Error(err, "error getting access group", "id", groupID)
		return nil, err
	}

	return &AccessGroupResult{
		ID:   group.ID,
		Name: group.Name,
	}, nil
}

// UpdateAccessGroup updates an existing Access Group.
func (c *API) UpdateAccessGroup(ctx context.Context, groupID string, params AccessGroupParams) (*AccessGroupResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	updateParams := cloudflare.UpdateAccessGroupParams{
		ID:      groupID,
		Name:    params.Name,
		Include: ConvertRulesToSDK(params.Include),
		Exclude: ConvertRulesToSDK(params.Exclude),
		Require: ConvertRulesToSDK(params.Require),
	}

	group, err := c.CloudflareClient.UpdateAccessGroup(ctx, rc, updateParams)
	if err != nil {
		c.Log.Error(err, "error updating access group", "id", groupID)
		return nil, err
	}

	c.Log.Info("Access Group updated", "id", group.ID, "name", group.Name)

	return &AccessGroupResult{
		ID:   group.ID,
		Name: group.Name,
	}, nil
}

// DeleteAccessGroup deletes an Access Group.
// This method is idempotent - returns nil if the group is already deleted.
func (c *API) DeleteAccessGroup(ctx context.Context, groupID string) error {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	err := c.CloudflareClient.DeleteAccessGroup(ctx, rc, groupID)
	if err != nil {
		if IsNotFoundError(err) {
			c.Log.Info("Access Group already deleted (not found)", "id", groupID)
			return nil
		}
		c.Log.Error(err, "error deleting access group", "id", groupID)
		return err
	}

	c.Log.Info("Access Group deleted", "id", groupID)
	return nil
}

// AccessIdentityProviderParams contains parameters for an Access Identity Provider.
type AccessIdentityProviderParams struct {
	Name       string
	Type       string
	Config     cloudflare.AccessIdentityProviderConfiguration
	ScimConfig cloudflare.AccessIdentityProviderScimConfiguration
}

// AccessIdentityProviderResult contains the result of an Access Identity Provider operation.
type AccessIdentityProviderResult struct {
	ID   string
	Name string
	Type string
}

// CreateAccessIdentityProvider creates a new Access Identity Provider.
func (c *API) CreateAccessIdentityProvider(ctx context.Context, params AccessIdentityProviderParams) (*AccessIdentityProviderResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	createParams := cloudflare.CreateAccessIdentityProviderParams{
		Name:       params.Name,
		Type:       params.Type,
		Config:     params.Config,
		ScimConfig: params.ScimConfig,
	}

	idp, err := c.CloudflareClient.CreateAccessIdentityProvider(ctx, rc, createParams)
	if err != nil {
		c.Log.Error(err, "error creating access identity provider", "name", params.Name)
		return nil, err
	}

	c.Log.Info("Access Identity Provider created", "id", idp.ID, "name", idp.Name)

	return &AccessIdentityProviderResult{
		ID:   idp.ID,
		Name: idp.Name,
		Type: idp.Type,
	}, nil
}

// GetAccessIdentityProvider retrieves an Access Identity Provider by ID.
func (c *API) GetAccessIdentityProvider(ctx context.Context, idpID string) (*AccessIdentityProviderResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	idp, err := c.CloudflareClient.GetAccessIdentityProvider(ctx, rc, idpID)
	if err != nil {
		c.Log.Error(err, "error getting access identity provider", "id", idpID)
		return nil, err
	}

	return &AccessIdentityProviderResult{
		ID:   idp.ID,
		Name: idp.Name,
		Type: idp.Type,
	}, nil
}

// UpdateAccessIdentityProvider updates an existing Access Identity Provider.
func (c *API) UpdateAccessIdentityProvider(
	ctx context.Context,
	idpID string,
	params AccessIdentityProviderParams,
) (*AccessIdentityProviderResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	updateParams := cloudflare.UpdateAccessIdentityProviderParams{
		ID:         idpID,
		Name:       params.Name,
		Type:       params.Type,
		Config:     params.Config,
		ScimConfig: params.ScimConfig,
	}

	idp, err := c.CloudflareClient.UpdateAccessIdentityProvider(ctx, rc, updateParams)
	if err != nil {
		c.Log.Error(err, "error updating access identity provider", "id", idpID)
		return nil, err
	}

	c.Log.Info("Access Identity Provider updated", "id", idp.ID, "name", idp.Name)

	return &AccessIdentityProviderResult{
		ID:   idp.ID,
		Name: idp.Name,
		Type: idp.Type,
	}, nil
}

// DeleteAccessIdentityProvider deletes an Access Identity Provider.
// This method is idempotent - returns nil if the identity provider is already deleted.
func (c *API) DeleteAccessIdentityProvider(ctx context.Context, idpID string) error {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	_, err := c.CloudflareClient.DeleteAccessIdentityProvider(ctx, rc, idpID)
	if err != nil {
		if IsNotFoundError(err) {
			c.Log.Info("Access Identity Provider already deleted (not found)", "id", idpID)
			return nil
		}
		c.Log.Error(err, "error deleting access identity provider", "id", idpID)
		return err
	}

	c.Log.Info("Access Identity Provider deleted", "id", idpID)
	return nil
}

// AccessServiceTokenResult contains the result of an Access Service Token operation.
type AccessServiceTokenResult struct {
	ID                  string
	TokenID             string
	Name                string
	ClientID            string
	ClientSecret        string
	AccountID           string
	ExpiresAt           string
	CreatedAt           string
	UpdatedAt           string
	LastSeenAt          string
	ClientSecretVersion int64
}

// convertServiceToken converts a Cloudflare service token to our result type
func (c *API) convertServiceToken(token cloudflare.AccessServiceToken) *AccessServiceTokenResult {
	expiresAt := ""
	if token.ExpiresAt != nil {
		expiresAt = token.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
	}
	createdAt := ""
	if token.CreatedAt != nil {
		createdAt = token.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	updatedAt := ""
	if token.UpdatedAt != nil {
		updatedAt = token.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	lastSeenAt := ""
	if token.LastSeenAt != nil {
		lastSeenAt = token.LastSeenAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return &AccessServiceTokenResult{
		ID:                  token.ID,
		TokenID:             token.ID,
		Name:                token.Name,
		ClientID:            token.ClientID,
		AccountID:           c.ValidAccountId,
		ExpiresAt:           expiresAt,
		CreatedAt:           createdAt,
		UpdatedAt:           updatedAt,
		LastSeenAt:          lastSeenAt,
		ClientSecretVersion: token.ClientSecretVersion,
	}
}

// GetAccessServiceTokenByName retrieves an Access Service Token by name.
// Returns nil if no token with the given name is found.
func (c *API) GetAccessServiceTokenByName(ctx context.Context, name string) (*AccessServiceTokenResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	tokens, _, err := c.CloudflareClient.ListAccessServiceTokens(ctx, rc, cloudflare.ListAccessServiceTokensParams{})
	if err != nil {
		c.Log.Error(err, "error listing access service tokens")
		return nil, err
	}

	for _, token := range tokens {
		if token.Name == name {
			return c.convertServiceToken(token), nil
		}
	}

	return nil, nil
}

// CreateAccessServiceToken creates a new Access Service Token.
func (c *API) CreateAccessServiceToken(ctx context.Context, name string, duration string) (*AccessServiceTokenResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	createParams := cloudflare.CreateAccessServiceTokenParams{
		Name:                name,
		Duration:            duration,
		ClientSecretVersion: 1, // Required: minimum version is 1
	}

	token, err := c.CloudflareClient.CreateAccessServiceToken(ctx, rc, createParams)
	if err != nil {
		c.Log.Error(err, "error creating access service token", "name", name)
		return nil, err
	}

	c.Log.Info("Access Service Token created", "id", token.ID, "name", token.Name)

	expiresAt := ""
	if token.ExpiresAt != nil {
		expiresAt = token.ExpiresAt.String()
	}

	return &AccessServiceTokenResult{
		ID:           token.ID,
		TokenID:      token.ID,
		Name:         token.Name,
		ClientID:     token.ClientID,
		ClientSecret: token.ClientSecret,
		AccountID:    c.ValidAccountId,
		ExpiresAt:    expiresAt,
	}, nil
}

// UpdateAccessServiceToken updates an existing Access Service Token.
func (c *API) UpdateAccessServiceToken(ctx context.Context, tokenID string, name string, duration string) (*AccessServiceTokenResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	updateParams := cloudflare.UpdateAccessServiceTokenParams{
		UUID:     tokenID,
		Name:     name,
		Duration: duration,
	}

	token, err := c.CloudflareClient.UpdateAccessServiceToken(ctx, rc, updateParams)
	if err != nil {
		c.Log.Error(err, "error updating access service token", "id", tokenID)
		return nil, err
	}

	c.Log.Info("Access Service Token updated", "id", token.ID, "name", token.Name)

	expiresAt := ""
	if token.ExpiresAt != nil {
		expiresAt = token.ExpiresAt.String()
	}

	return &AccessServiceTokenResult{
		ID:           token.ID,
		TokenID:      token.ID,
		Name:         token.Name,
		ClientID:     token.ClientID,
		ClientSecret: "", // ClientSecret not returned on update
		AccountID:    c.ValidAccountId,
		ExpiresAt:    expiresAt,
	}, nil
}

// RefreshAccessServiceToken refreshes an Access Service Token, generating a new client secret.
func (c *API) RefreshAccessServiceToken(ctx context.Context, tokenID string) (*AccessServiceTokenResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	token, err := c.CloudflareClient.RefreshAccessServiceToken(ctx, rc, tokenID)
	if err != nil {
		c.Log.Error(err, "error refreshing access service token", "id", tokenID)
		return nil, err
	}

	c.Log.Info("Access Service Token refreshed", "id", token.ID, "name", token.Name)

	expiresAt := ""
	if token.ExpiresAt != nil {
		expiresAt = token.ExpiresAt.String()
	}

	return &AccessServiceTokenResult{
		ID:           token.ID,
		TokenID:      token.ID,
		Name:         token.Name,
		ClientID:     token.ClientID,
		ClientSecret: "", // ClientSecret returned separately
		AccountID:    c.ValidAccountId,
		ExpiresAt:    expiresAt,
	}, nil
}

// DeleteAccessServiceToken deletes an Access Service Token.
// This method is idempotent - returns nil if the service token is already deleted.
func (c *API) DeleteAccessServiceToken(ctx context.Context, tokenID string) error {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	_, err := c.CloudflareClient.DeleteAccessServiceToken(ctx, rc, tokenID)
	if err != nil {
		if IsNotFoundError(err) {
			c.Log.Info("Access Service Token already deleted (not found)", "id", tokenID)
			return nil
		}
		c.Log.Error(err, "error deleting access service token", "id", tokenID)
		return err
	}

	c.Log.Info("Access Service Token deleted", "id", tokenID)
	return nil
}

// DevicePostureMatchParams contains platform matching for Device Posture Rule.
type DevicePostureMatchParams struct {
	Platform string
}

// DevicePostureInputParams contains rule-specific input for Device Posture Rule.
type DevicePostureInputParams struct {
	ID               string
	Path             string
	Exists           *bool
	Sha256           string
	Thumbprint       string
	Running          *bool
	RequireAll       *bool
	Enabled          *bool
	Version          string
	Operator         string
	Domain           string
	ComplianceStatus string
	ConnectionID     string
	LastSeen         string
	EidLastSeen      string
	ActiveThreats    *int
	Infected         *bool
	IsActive         *bool
	NetworkStatus    string
	SensorConfig     string
	VersionOperator  string
	CountOperator    string
	ScoreOperator    string
	IssueCount       *int
	Score            *int
	TotalScore       *int
	RiskLevel        string
	Overall          string
	State            string
	OperationalState string
	OSDistroName     string
	OSDistroRevision string
	OSVersionExtra   string
	OS               string
	OperatingSystem  string
	CertificateID    string
	CommonName       string
	Cn               string
	CheckPrivateKey  *bool
	ExtendedKeyUsage []string
	Locations        []DevicePostureLocationParams
	CheckDisks       []string
}

// DevicePostureLocationParams contains location parameters for Device Posture Rule.
type DevicePostureLocationParams struct {
	Paths       []string
	TrustStores []string
}

// DevicePostureRuleParams contains parameters for a Device Posture Rule.
type DevicePostureRuleParams struct {
	Name        string
	Type        string
	Description string
	Schedule    string
	Expiration  string
	Match       []DevicePostureMatchParams
	Input       *DevicePostureInputParams
}

// DevicePostureRuleResult contains the result of a Device Posture Rule operation.
type DevicePostureRuleResult struct {
	ID          string
	Name        string
	Type        string
	Description string
	AccountID   string
}

// convertToDevicePostureRuleInput converts DevicePostureInputParams to cloudflare.DevicePostureRuleInput.
func convertToDevicePostureRuleInput(input *DevicePostureInputParams) cloudflare.DevicePostureRuleInput {
	result := cloudflare.DevicePostureRuleInput{}

	if input == nil {
		return result
	}

	// String fields
	result.ID = input.ID
	result.Path = input.Path
	result.Sha256 = input.Sha256
	result.Thumbprint = input.Thumbprint
	result.Version = input.Version
	result.Operator = input.Operator
	result.Domain = input.Domain
	result.ComplianceStatus = input.ComplianceStatus
	result.ConnectionID = input.ConnectionID
	result.LastSeen = input.LastSeen
	result.EidLastSeen = input.EidLastSeen
	result.NetworkStatus = input.NetworkStatus
	result.SensorConfig = input.SensorConfig
	result.VersionOperator = input.VersionOperator
	result.CountOperator = input.CountOperator
	result.ScoreOperator = input.ScoreOperator
	result.RiskLevel = input.RiskLevel
	result.Overall = input.Overall
	result.State = input.State
	result.Os = input.OS
	result.OsDistroName = input.OSDistroName
	result.OsDistroRevision = input.OSDistroRevision
	result.OSVersionExtra = input.OSVersionExtra
	result.CertificateID = input.CertificateID
	// CommonName maps to "cn" in JSON, covering both CommonName and Cn inputs
	if input.CommonName != "" {
		result.CommonName = input.CommonName
	} else if input.Cn != "" {
		result.CommonName = input.Cn
	}

	// String pointer field
	if input.OperationalState != "" {
		result.OperationalState = &input.OperationalState
	}

	// Bool pointer fields
	result.Exists = input.Exists
	result.Running = input.Running
	result.RequireAll = input.RequireAll
	result.Enabled = input.Enabled
	result.Infected = input.Infected
	result.IsActive = input.IsActive
	result.CheckPrivateKey = input.CheckPrivateKey

	// Int fields
	if input.TotalScore != nil {
		result.TotalScore = *input.TotalScore
	}
	if input.ActiveThreats != nil {
		result.ActiveThreats = *input.ActiveThreats
	}
	if input.Score != nil {
		result.Score = *input.Score
	}
	if input.IssueCount != nil {
		result.IssueCount = fmt.Sprintf("%d", *input.IssueCount)
	}

	// Slice fields
	result.CheckDisks = input.CheckDisks
	result.ExtendedKeyUsage = input.ExtendedKeyUsage

	// Convert locations - SDK uses CertificateLocations type
	if len(input.Locations) > 0 {
		// CertificateLocations is not a slice but a single struct
		// Take the first location entry for now (SDK limitation)
		result.Locations = cloudflare.CertificateLocations{
			Paths:       input.Locations[0].Paths,
			TrustStores: input.Locations[0].TrustStores,
		}
	}

	return result
}

// CreateDevicePostureRule creates a new Device Posture Rule.
func (c *API) CreateDevicePostureRule(ctx context.Context, params DevicePostureRuleParams) (*DevicePostureRuleResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	// Convert match to DevicePostureRuleMatch
	var match []cloudflare.DevicePostureRuleMatch
	for _, m := range params.Match {
		match = append(match, cloudflare.DevicePostureRuleMatch{
			Platform: m.Platform,
		})
	}

	// Convert input to DevicePostureRuleInput using the helper function
	input := convertToDevicePostureRuleInput(params.Input)

	rule := cloudflare.DevicePostureRule{
		Name:        params.Name,
		Type:        params.Type,
		Description: params.Description,
		Schedule:    params.Schedule,
		Expiration:  params.Expiration,
		Match:       match,
		Input:       input,
	}

	result, err := c.CloudflareClient.CreateDevicePostureRule(ctx, c.ValidAccountId, rule)
	if err != nil {
		c.Log.Error(err, "error creating device posture rule", "name", params.Name)
		return nil, err
	}

	c.Log.Info("Device Posture Rule created", "id", result.ID, "name", result.Name)

	return &DevicePostureRuleResult{
		ID:          result.ID,
		Name:        result.Name,
		Type:        result.Type,
		Description: result.Description,
		AccountID:   c.ValidAccountId,
	}, nil
}

// GetDevicePostureRule retrieves a Device Posture Rule by ID.
func (c *API) GetDevicePostureRule(ctx context.Context, ruleID string) (*DevicePostureRuleResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rule, err := c.CloudflareClient.DevicePostureRule(ctx, c.ValidAccountId, ruleID)
	if err != nil {
		c.Log.Error(err, "error getting device posture rule", "id", ruleID)
		return nil, err
	}

	return &DevicePostureRuleResult{
		ID:          rule.ID,
		Name:        rule.Name,
		Type:        rule.Type,
		Description: rule.Description,
		AccountID:   c.ValidAccountId,
	}, nil
}

// UpdateDevicePostureRule updates an existing Device Posture Rule.
func (c *API) UpdateDevicePostureRule(ctx context.Context, ruleID string, params DevicePostureRuleParams) (*DevicePostureRuleResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	// Convert match to DevicePostureRuleMatch
	var match []cloudflare.DevicePostureRuleMatch
	for _, m := range params.Match {
		match = append(match, cloudflare.DevicePostureRuleMatch{
			Platform: m.Platform,
		})
	}

	// Convert input to DevicePostureRuleInput using the helper function
	input := convertToDevicePostureRuleInput(params.Input)

	rule := cloudflare.DevicePostureRule{
		ID:          ruleID,
		Name:        params.Name,
		Type:        params.Type,
		Description: params.Description,
		Schedule:    params.Schedule,
		Expiration:  params.Expiration,
		Match:       match,
		Input:       input,
	}

	result, err := c.CloudflareClient.UpdateDevicePostureRule(ctx, c.ValidAccountId, rule)
	if err != nil {
		c.Log.Error(err, "error updating device posture rule", "id", ruleID)
		return nil, err
	}

	c.Log.Info("Device Posture Rule updated", "id", result.ID, "name", result.Name)

	return &DevicePostureRuleResult{
		ID:          result.ID,
		Name:        result.Name,
		Type:        result.Type,
		Description: result.Description,
		AccountID:   c.ValidAccountId,
	}, nil
}

// DeleteDevicePostureRule deletes a Device Posture Rule.
// This method is idempotent - returns nil if the rule is already deleted.
func (c *API) DeleteDevicePostureRule(ctx context.Context, ruleID string) error {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	err := c.CloudflareClient.DeleteDevicePostureRule(ctx, c.ValidAccountId, ruleID)
	if err != nil {
		if IsNotFoundError(err) {
			c.Log.Info("Device Posture Rule already deleted (not found)", "id", ruleID)
			return nil
		}
		c.Log.Error(err, "error deleting device posture rule", "id", ruleID)
		return err
	}

	c.Log.Info("Device Posture Rule deleted", "id", ruleID)
	return nil
}

// ListAccessGroupsByName finds an Access Group by name.
// Returns nil if no group with the given name is found.
func (c *API) ListAccessGroupsByName(ctx context.Context, name string) (*AccessGroupResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	groups, _, err := c.CloudflareClient.ListAccessGroups(ctx, rc, cloudflare.ListAccessGroupsParams{})
	if err != nil {
		c.Log.Error(err, "error listing access groups")
		return nil, err
	}

	for _, group := range groups {
		if group.Name == name {
			return &AccessGroupResult{
				ID:   group.ID,
				Name: group.Name,
			}, nil
		}
	}

	return nil, nil // Not found, return nil without error
}

// ListAccessIdentityProvidersByName finds an Access Identity Provider by name.
// Returns nil if no provider with the given name is found.
func (c *API) ListAccessIdentityProvidersByName(ctx context.Context, name string) (*AccessIdentityProviderResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	providers, _, err := c.CloudflareClient.ListAccessIdentityProviders(ctx, rc, cloudflare.ListAccessIdentityProvidersParams{})
	if err != nil {
		c.Log.Error(err, "error listing access identity providers")
		return nil, err
	}

	for _, provider := range providers {
		if provider.Name == name {
			return &AccessIdentityProviderResult{
				ID:   provider.ID,
				Name: provider.Name,
				Type: provider.Type,
			}, nil
		}
	}

	return nil, nil // Not found, return nil without error
}

// ListDevicePostureRulesByName finds a Device Posture Rule by name.
// Returns nil if no rule with the given name is found.
func (c *API) ListDevicePostureRulesByName(ctx context.Context, name string) (*DevicePostureRuleResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rules, _, err := c.CloudflareClient.DevicePostureRules(ctx, c.ValidAccountId)
	if err != nil {
		c.Log.Error(err, "error listing device posture rules")
		return nil, err
	}

	for _, rule := range rules {
		if rule.Name == name {
			return &DevicePostureRuleResult{
				ID:          rule.ID,
				Name:        rule.Name,
				Type:        rule.Type,
				Description: rule.Description,
				AccountID:   c.ValidAccountId,
			}, nil
		}
	}

	return nil, nil // Not found, return nil without error
}

// ListAccessApplicationsByName finds an Access Application by name.
func (c *API) ListAccessApplicationsByName(ctx context.Context, name string) (*AccessApplicationResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	apps, _, err := c.CloudflareClient.ListAccessApplications(ctx, rc, cloudflare.ListAccessApplicationsParams{})
	if err != nil {
		c.Log.Error(err, "error listing access applications")
		return nil, err
	}

	for _, app := range apps {
		if app.Name == name {
			return convertAccessApplicationToResult(app, c.ValidAccountId), nil
		}
	}

	return nil, fmt.Errorf("access application not found: %s", name)
}

// ============================================================================
// Conversion helper functions for AccessApplication
// ============================================================================

// convertAccessApplicationToResult converts a Cloudflare AccessApplication to our result type.
func convertAccessApplicationToResult(app cloudflare.AccessApplication, _ string) *AccessApplicationResult {
	autoRedirect := false
	if app.AutoRedirectToIdentity != nil {
		autoRedirect = *app.AutoRedirectToIdentity
	}

	// Extract SelfHostedDomains from Destinations
	var selfHostedDomains []string
	for _, dest := range app.Destinations {
		if string(dest.Type) == "public" && dest.URI != "" {
			selfHostedDomains = append(selfHostedDomains, dest.URI)
		}
	}

	// Extract SaaS app client ID if available
	var saasAppClientID string
	if app.SaasApplication != nil {
		saasAppClientID = app.SaasApplication.ClientID
	}

	return &AccessApplicationResult{
		ID:                     app.ID,
		AUD:                    app.AUD,
		Name:                   app.Name,
		Domain:                 app.Domain,
		SelfHostedDomains:      selfHostedDomains,
		Type:                   string(app.Type),
		SessionDuration:        app.SessionDuration,
		AllowedIdps:            app.AllowedIdps,
		AutoRedirectToIdentity: autoRedirect,
		SaasAppClientID:        saasAppClientID,
	}
}

// convertDestinationsToCloudflare converts destination params to Cloudflare format.
func convertDestinationsToCloudflare(destinations []AccessDestinationParams) []cloudflare.AccessDestination {
	result := make([]cloudflare.AccessDestination, 0, len(destinations))
	for _, dest := range destinations {
		result = append(result, cloudflare.AccessDestination{
			Type:       cloudflare.AccessDestinationType(dest.Type),
			URI:        dest.URI,
			Hostname:   dest.Hostname,
			CIDR:       dest.CIDR,
			PortRange:  dest.PortRange,
			L4Protocol: dest.L4Protocol,
			VnetID:     dest.VnetID,
		})
	}
	return result
}

// applyCreateAccessAppOptionalParams applies optional parameters to create params.
//
//nolint:revive // function extracts optional param logic to reduce main function complexity
func applyCreateAccessAppOptionalParams(
	createParams *cloudflare.CreateAccessApplicationParams,
	params AccessApplicationParams,
) {
	if params.DomainType != "" {
		createParams.DomainType = cloudflare.AccessDestinationType(params.DomainType)
	}
	if destinations := buildAccessAppDestinations(params); len(destinations) > 0 {
		createParams.Destinations = destinations
	}
	createParams.CorsHeaders = convertCorsHeadersToCloudflare(params.CorsHeaders)
	createParams.SaasApplication = convertSaasAppToCloudflare(params.SaasApp)
	createParams.SCIMConfig = convertSCIMConfigToCloudflare(params.SCIMConfig)
	if params.AppLauncherCustomization != nil {
		createParams.AccessAppLauncherCustomization = convertAppLauncherCustomizationToCloudflare(params.AppLauncherCustomization)
	}
	if len(params.TargetContexts) > 0 {
		contexts := convertTargetContextsToCloudflare(params.TargetContexts)
		createParams.TargetContexts = &contexts
	}
	createParams.GatewayRules = buildGatewayRules(params.GatewayRules)
	createParams.AllowAuthenticateViaWarp = params.AllowAuthenticateViaWarp
	createParams.Tags = params.Tags
	createParams.CustomPages = params.CustomPages
}

// applyUpdateAccessAppOptionalParams applies optional parameters to update params.
//
//nolint:revive // function extracts optional param logic to reduce main function complexity
func applyUpdateAccessAppOptionalParams(
	updateParams *cloudflare.UpdateAccessApplicationParams,
	params AccessApplicationParams,
) {
	if params.DomainType != "" {
		updateParams.DomainType = cloudflare.AccessDestinationType(params.DomainType)
	}
	if destinations := buildAccessAppDestinations(params); len(destinations) > 0 {
		updateParams.Destinations = destinations
	}
	updateParams.CorsHeaders = convertCorsHeadersToCloudflare(params.CorsHeaders)
	updateParams.SaasApplication = convertSaasAppToCloudflare(params.SaasApp)
	updateParams.SCIMConfig = convertSCIMConfigToCloudflare(params.SCIMConfig)
	if params.AppLauncherCustomization != nil {
		updateParams.AccessAppLauncherCustomization = convertAppLauncherCustomizationToCloudflare(params.AppLauncherCustomization)
	}
	if len(params.TargetContexts) > 0 {
		contexts := convertTargetContextsToCloudflare(params.TargetContexts)
		updateParams.TargetContexts = &contexts
	}
	updateParams.GatewayRules = buildGatewayRules(params.GatewayRules)
	updateParams.AllowAuthenticateViaWarp = params.AllowAuthenticateViaWarp
	updateParams.Tags = params.Tags
	updateParams.CustomPages = params.CustomPages
}

// buildAccessAppDestinations builds the full destinations list including Domain and SelfHostedDomains.
func buildAccessAppDestinations(params AccessApplicationParams) []cloudflare.AccessDestination {
	destinations := convertDestinationsToCloudflare(params.Destinations)
	// Add main domain as public destination if not already included
	if params.Domain != "" {
		destinations = append(destinations, cloudflare.AccessDestination{
			Type: cloudflare.AccessDestinationType("public"),
			URI:  params.Domain,
		})
	}
	// Convert SelfHostedDomains to public destinations
	for _, domain := range params.SelfHostedDomains {
		destinations = append(destinations, cloudflare.AccessDestination{
			Type: cloudflare.AccessDestinationType("public"),
			URI:  domain,
		})
	}
	return destinations
}

// buildGatewayRules converts gateway rule IDs to Cloudflare format.
func buildGatewayRules(gatewayRules []string) []cloudflare.AccessApplicationGatewayRule {
	if len(gatewayRules) == 0 {
		return nil
	}
	result := make([]cloudflare.AccessApplicationGatewayRule, 0, len(gatewayRules))
	for _, ruleID := range gatewayRules {
		result = append(result, cloudflare.AccessApplicationGatewayRule{ID: ruleID})
	}
	return result
}

// convertCorsHeadersToCloudflare converts CORS headers params to Cloudflare format.
func convertCorsHeadersToCloudflare(cors *AccessApplicationCorsHeadersParams) *cloudflare.AccessApplicationCorsHeaders {
	if cors == nil {
		return nil
	}
	return &cloudflare.AccessApplicationCorsHeaders{
		AllowedMethods:   cors.AllowedMethods,
		AllowedOrigins:   cors.AllowedOrigins,
		AllowedHeaders:   cors.AllowedHeaders,
		AllowAllMethods:  cors.AllowAllMethods,
		AllowAllHeaders:  cors.AllowAllHeaders,
		AllowAllOrigins:  cors.AllowAllOrigins,
		AllowCredentials: cors.AllowCredentials,
		MaxAge:           cors.MaxAge,
	}
}

// convertSaasAppToCloudflare converts SaaS app params to Cloudflare format.
//
//nolint:revive // cognitive complexity is acceptable for this conversion
func convertSaasAppToCloudflare(saas *SaasApplicationParams) *cloudflare.SaasApplication {
	if saas == nil {
		return nil
	}

	result := &cloudflare.SaasApplication{
		AuthType:                      saas.AuthType,
		ConsumerServiceUrl:            saas.ConsumerServiceURL,
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

	// Convert SAML custom attributes
	if len(saas.CustomAttributes) > 0 {
		attrs := make([]cloudflare.SAMLAttributeConfig, 0, len(saas.CustomAttributes))
		for _, attr := range saas.CustomAttributes {
			attrs = append(attrs, cloudflare.SAMLAttributeConfig{
				Name:         attr.Name,
				NameFormat:   attr.NameFormat,
				FriendlyName: attr.FriendlyName,
				Required:     attr.Required,
				Source: cloudflare.SourceConfig{
					Name:      attr.Source.Name,
					NameByIDP: attr.Source.NameByIDP,
				},
			})
		}
		result.CustomAttributes = &attrs
	}

	// Convert OIDC custom claims
	if len(saas.CustomClaims) > 0 {
		claims := make([]cloudflare.OIDCClaimConfig, 0, len(saas.CustomClaims))
		for _, claim := range saas.CustomClaims {
			required := claim.Required
			claims = append(claims, cloudflare.OIDCClaimConfig{
				Name:     claim.Name,
				Required: &required,
				Scope:    claim.Scope,
				Source: cloudflare.SourceConfig{
					Name:      claim.Source.Name,
					NameByIDP: claim.Source.NameByIDP,
				},
			})
		}
		result.CustomClaims = &claims
	}

	// Convert refresh token options
	if saas.RefreshTokenOptions != nil {
		result.RefreshTokenOptions = &cloudflare.RefreshTokenOptions{
			Lifetime: saas.RefreshTokenOptions.Lifetime,
		}
	}

	// Convert hybrid and implicit options
	if saas.HybridAndImplicitOptions != nil {
		result.HybridAndImplicitOptions = &cloudflare.AccessApplicationHybridAndImplicitOptions{
			ReturnIDTokenFromAuthorizationEndpoint:     saas.HybridAndImplicitOptions.ReturnIDTokenFromAuthorizationEndpoint,
			ReturnAccessTokenFromAuthorizationEndpoint: saas.HybridAndImplicitOptions.ReturnAccessTokenFromAuthorizationEndpoint,
		}
	}

	return result
}

// convertSCIMConfigToCloudflare converts SCIM config params to Cloudflare format.
//
//nolint:revive // cognitive complexity is acceptable for this conversion
func convertSCIMConfigToCloudflare(scim *AccessApplicationSCIMConfigParams) *cloudflare.AccessApplicationSCIMConfig {
	if scim == nil {
		return nil
	}

	result := &cloudflare.AccessApplicationSCIMConfig{
		Enabled:            scim.Enabled,
		RemoteURI:          scim.RemoteURI,
		IdPUID:             scim.IDPUID,
		DeactivateOnDelete: scim.DeactivateOnDelete,
	}

	// Convert authentication based on scheme type (polymorphic)
	if scim.Authentication != nil {
		result.Authentication = convertSCIMAuthenticationToCloudflare(scim.Authentication)
	}

	// Convert mappings
	if len(scim.Mappings) > 0 {
		mappings := make([]*cloudflare.AccessApplicationScimMapping, 0, len(scim.Mappings))
		for _, m := range scim.Mappings {
			mapping := &cloudflare.AccessApplicationScimMapping{
				Schema:           m.Schema,
				Enabled:          m.Enabled,
				Filter:           m.Filter,
				TransformJsonata: m.TransformJsonata,
				Strictness:       m.Strictness,
			}
			if m.Operations != nil {
				mapping.Operations = &cloudflare.AccessApplicationScimMappingOperations{
					Create: m.Operations.Create,
					Update: m.Operations.Update,
					Delete: m.Operations.Delete,
				}
			}
			mappings = append(mappings, mapping)
		}
		result.Mappings = mappings
	}

	return result
}

// convertAppLauncherCustomizationToCloudflare converts app launcher customization to Cloudflare format.
func convertAppLauncherCustomizationToCloudflare(custom *AccessAppLauncherCustomizationParams) cloudflare.AccessAppLauncherCustomization {
	result := cloudflare.AccessAppLauncherCustomization{
		LogoURL:                  custom.AppLauncherLogoURL,
		HeaderBackgroundColor:    custom.HeaderBackgroundColor,
		BackgroundColor:          custom.BackgroundColor,
		SkipAppLauncherLoginPage: custom.SkipAppLauncherLoginPage,
	}

	// Convert landing page design
	if custom.LandingPageDesign != nil {
		result.LandingPageDesign = cloudflare.AccessLandingPageDesign{
			Title:           custom.LandingPageDesign.Title,
			Message:         custom.LandingPageDesign.Message,
			ImageURL:        custom.LandingPageDesign.ImageURL,
			ButtonColor:     custom.LandingPageDesign.ButtonColor,
			ButtonTextColor: custom.LandingPageDesign.ButtonTextColor,
		}
	}

	// Convert footer links
	if len(custom.FooterLinks) > 0 {
		links := make([]cloudflare.AccessFooterLink, 0, len(custom.FooterLinks))
		for _, link := range custom.FooterLinks {
			links = append(links, cloudflare.AccessFooterLink{
				Name: link.Name,
				URL:  link.URL,
			})
		}
		result.FooterLinks = links
	}

	return result
}

// convertTargetContextsToCloudflare converts target contexts to Cloudflare format.
func convertTargetContextsToCloudflare(contexts []AccessInfrastructureTargetContextParams) []cloudflare.AccessInfrastructureTargetContext {
	result := make([]cloudflare.AccessInfrastructureTargetContext, 0, len(contexts))
	for _, ctx := range contexts {
		result = append(result, cloudflare.AccessInfrastructureTargetContext{
			TargetAttributes: ctx.TargetAttributes,
			Port:             ctx.Port,
			Protocol:         cloudflare.AccessInfrastructureProtocol(ctx.Protocol),
		})
	}
	return result
}

// convertSCIMAuthenticationToCloudflare converts SCIM authentication params to Cloudflare format.
// This handles the polymorphic nature of SCIM authentication types.
func convertSCIMAuthenticationToCloudflare(auth *SCIMAuthenticationParams) *cloudflare.AccessApplicationScimAuthenticationJson {
	if auth == nil || auth.Scheme == "" {
		return nil
	}

	var authValue cloudflare.AccessApplicationScimAuthentication

	switch cloudflare.AccessApplicationScimAuthenticationScheme(auth.Scheme) {
	case cloudflare.AccessApplicationScimAuthenticationSchemeHttpBasic:
		authValue = &cloudflare.AccessApplicationScimAuthenticationHttpBasic{
			User:     auth.User,
			Password: auth.Password,
		}
	case cloudflare.AccessApplicationScimAuthenticationSchemeOauthBearerToken:
		authValue = &cloudflare.AccessApplicationScimAuthenticationOauthBearerToken{
			Token: auth.Token,
		}
	case cloudflare.AccessApplicationScimAuthenticationSchemeOauth2:
		authValue = &cloudflare.AccessApplicationScimAuthenticationOauth2{
			ClientID:         auth.ClientID,
			ClientSecret:     auth.ClientSecret,
			AuthorizationURL: auth.AuthorizationURL,
			TokenURL:         auth.TokenURL,
			Scopes:           auth.Scopes,
		}
	case cloudflare.AccessApplicationScimAuthenticationAccessServiceToken:
		authValue = &cloudflare.AccessApplicationScimAuthenticationServiceToken{
			ClientID:     auth.ClientID,
			ClientSecret: auth.ClientSecret,
		}
	default:
		// Unknown scheme, return nil
		return nil
	}

	return &cloudflare.AccessApplicationScimAuthenticationJson{
		Value: authValue,
	}
}
