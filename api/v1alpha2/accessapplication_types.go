// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccessApplicationSpec defines the desired state of AccessApplication
type AccessApplicationSpec struct {
	// Name of the Access Application in Cloudflare.
	// If not specified, the Kubernetes resource name will be used.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name,omitempty"`

	// Domain is the primary domain/URL for the application.
	// +kubebuilder:validation:Required
	Domain string `json:"domain"`

	// SelfHostedDomains is a list of additional domains for the application.
	// This allows protecting multiple domains with a single Access Application.
	// Each domain should be a fully qualified domain name (e.g., "app.example.com" or "app.example.com/path").
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxItems=50
	SelfHostedDomains []string `json:"selfHostedDomains,omitempty"`

	// Destinations specifies the destination configurations for the application.
	// This is more flexible than SelfHostedDomains and supports both public and private destinations.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxItems=50
	Destinations []AccessDestination `json:"destinations,omitempty"`

	// DomainType specifies the type of domain (public or private).
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=public;private
	DomainType string `json:"domainType,omitempty"`

	// PrivateAddress is the private address for private applications.
	// +kubebuilder:validation:Optional
	PrivateAddress string `json:"privateAddress,omitempty"`

	// Type is the application type.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=self_hosted;saas;ssh;vnc;app_launcher;warp;biso;bookmark;dash_sso;infrastructure
	// +kubebuilder:default=self_hosted
	Type string `json:"type"`

	// SessionDuration is the amount of time that the token is valid for.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="24h"
	SessionDuration string `json:"sessionDuration,omitempty"`

	// AllowedIdps is the list of identity provider IDs allowed for this application.
	// +kubebuilder:validation:Optional
	AllowedIdps []string `json:"allowedIdps,omitempty"`

	// AllowedIdpRefs references AccessIdentityProvider resources by name.
	// +kubebuilder:validation:Optional
	AllowedIdpRefs []AccessIdentityProviderRef `json:"allowedIdpRefs,omitempty"`

	// AutoRedirectToIdentity enables automatic redirect to the identity provider.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	AutoRedirectToIdentity bool `json:"autoRedirectToIdentity,omitempty"`

	// EnableBindingCookie enables the binding cookie.
	// +kubebuilder:validation:Optional
	EnableBindingCookie *bool `json:"enableBindingCookie,omitempty"`

	// HttpOnlyCookieAttribute sets the HttpOnly attribute on the cookie.
	// +kubebuilder:validation:Optional
	HttpOnlyCookieAttribute *bool `json:"httpOnlyCookieAttribute,omitempty"`

	// PathCookieAttribute sets the Path attribute on the cookie.
	// +kubebuilder:validation:Optional
	PathCookieAttribute *bool `json:"pathCookieAttribute,omitempty"`

	// SameSiteCookieAttribute sets the SameSite attribute on the cookie.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=strict;lax;none
	SameSiteCookieAttribute string `json:"sameSiteCookieAttribute,omitempty"`

	// LogoURL is the URL of the application logo.
	// +kubebuilder:validation:Optional
	LogoURL string `json:"logoUrl,omitempty"`

	// SkipInterstitial skips the interstitial page.
	// +kubebuilder:validation:Optional
	SkipInterstitial *bool `json:"skipInterstitial,omitempty"`

	// OptionsPreflightBypass allows CORS preflight requests to bypass Access authentication.
	// +kubebuilder:validation:Optional
	OptionsPreflightBypass *bool `json:"optionsPreflightBypass,omitempty"`

	// AppLauncherVisible shows the application in the App Launcher.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	AppLauncherVisible *bool `json:"appLauncherVisible,omitempty"`

	// ServiceAuth401Redirect redirects unauthorized service auth requests.
	// +kubebuilder:validation:Optional
	ServiceAuth401Redirect *bool `json:"serviceAuth401Redirect,omitempty"`

	// CustomDenyMessage is a custom message shown when access is denied.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=1024
	CustomDenyMessage string `json:"customDenyMessage,omitempty"`

	// CustomDenyURL is a custom URL to redirect to when access is denied.
	// +kubebuilder:validation:Optional
	CustomDenyURL string `json:"customDenyUrl,omitempty"`

	// CustomNonIdentityDenyURL is a custom URL for non-identity deny.
	// +kubebuilder:validation:Optional
	CustomNonIdentityDenyURL string `json:"customNonIdentityDenyUrl,omitempty"`

	// AllowAuthenticateViaWarp allows authentication via WARP.
	// +kubebuilder:validation:Optional
	AllowAuthenticateViaWarp *bool `json:"allowAuthenticateViaWarp,omitempty"`

	// Tags are custom tags for the application.
	// +kubebuilder:validation:Optional
	Tags []string `json:"tags,omitempty"`

	// CustomPages is a list of custom page IDs to use for the application.
	// +kubebuilder:validation:Optional
	CustomPages []string `json:"customPages,omitempty"`

	// GatewayRules is a list of Gateway rule IDs associated with the application.
	// +kubebuilder:validation:Optional
	GatewayRules []string `json:"gatewayRules,omitempty"`

	// CorsHeaders configures Cross-Origin Resource Sharing (CORS) for the application.
	// +kubebuilder:validation:Optional
	CorsHeaders *AccessApplicationCorsHeaders `json:"corsHeaders,omitempty"`

	// SaasApp configures the SaaS application settings (for type=saas).
	// +kubebuilder:validation:Optional
	SaasApp *SaasApplicationConfig `json:"saasApp,omitempty"`

	// SCIMConfig configures SCIM provisioning for the application.
	// +kubebuilder:validation:Optional
	SCIMConfig *AccessApplicationSCIMConfig `json:"scimConfig,omitempty"`

	// AppLauncherCustomization configures the appearance of the app launcher.
	// +kubebuilder:validation:Optional
	AppLauncherCustomization *AccessAppLauncherCustomization `json:"appLauncherCustomization,omitempty"`

	// TargetContexts specifies the target criteria for infrastructure applications.
	// +kubebuilder:validation:Optional
	TargetContexts []AccessInfrastructureTargetContext `json:"targetContexts,omitempty"`

	// Policies defines the inline access policies for this application.
	// These policies are defined directly within the AccessApplication.
	// +kubebuilder:validation:Optional
	Policies []AccessPolicyRef `json:"policies,omitempty"`

	// ReusablePolicyRefs references reusable AccessPolicy resources.
	// These policies are managed independently and can be shared across multiple applications.
	// Reusable policies are applied in addition to inline policies.
	// +kubebuilder:validation:Optional
	ReusablePolicyRefs []ReusablePolicyRef `json:"reusablePolicyRefs,omitempty"`

	// Cloudflare contains the Cloudflare API credentials and account information.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`
}

// ReusablePolicyRef references a reusable AccessPolicy resource.
// Exactly one of name, cloudflareId, or cloudflareName must be specified.
type ReusablePolicyRef struct {
	// Name is the name of a K8s AccessPolicy resource.
	// The controller will look up the AccessPolicy CR and use its PolicyID.
	// Mutually exclusive with cloudflareId and cloudflareName.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name,omitempty"`

	// CloudflareID is the UUID of an existing Cloudflare reusable Access Policy.
	// Use this to directly reference a Cloudflare-managed policy
	// without creating a corresponding Kubernetes AccessPolicy resource.
	// Mutually exclusive with name and cloudflareName.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
	CloudflareID string `json:"cloudflareId,omitempty"`

	// CloudflareName is the display name of an existing Cloudflare reusable Access Policy.
	// The controller will resolve this name to a PolicyID via the Cloudflare API.
	// Use this when you want to reference a Cloudflare policy by name
	// (e.g., policies created via Terraform or the Cloudflare dashboard).
	// Mutually exclusive with name and cloudflareId.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	CloudflareName string `json:"cloudflareName,omitempty"`

	// Precedence overrides the policy's default precedence when attached to this application.
	// If not specified, the policy's own precedence value is used.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	Precedence *int `json:"precedence,omitempty"`
}

// AccessDestination represents a destination for an Access Application.
type AccessDestination struct {
	// Type specifies the destination type (public or private).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=public;private
	Type string `json:"type"`

	// URI is the destination URI for public destinations.
	// Required for public destinations.
	// +kubebuilder:validation:Optional
	URI string `json:"uri,omitempty"`

	// Hostname is the destination hostname for private destinations.
	// +kubebuilder:validation:Optional
	Hostname string `json:"hostname,omitempty"`

	// CIDR is the destination CIDR for private destinations.
	// +kubebuilder:validation:Optional
	CIDR string `json:"cidr,omitempty"`

	// PortRange specifies the port range for private destinations (e.g., "80", "80-443").
	// +kubebuilder:validation:Optional
	PortRange string `json:"portRange,omitempty"`

	// L4Protocol specifies the Layer 4 protocol for private destinations.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=tcp;udp
	L4Protocol string `json:"l4Protocol,omitempty"`

	// VnetID is the Virtual Network ID for private destinations.
	// +kubebuilder:validation:Optional
	VnetID string `json:"vnetId,omitempty"`
}

// AccessApplicationCorsHeaders represents CORS settings for an Access Application.
type AccessApplicationCorsHeaders struct {
	// AllowedMethods is a list of allowed HTTP methods.
	// +kubebuilder:validation:Optional
	AllowedMethods []string `json:"allowedMethods,omitempty"`

	// AllowedOrigins is a list of allowed origins.
	// +kubebuilder:validation:Optional
	AllowedOrigins []string `json:"allowedOrigins,omitempty"`

	// AllowedHeaders is a list of allowed headers.
	// +kubebuilder:validation:Optional
	AllowedHeaders []string `json:"allowedHeaders,omitempty"`

	// AllowAllMethods allows all HTTP methods.
	// +kubebuilder:validation:Optional
	AllowAllMethods bool `json:"allowAllMethods,omitempty"`

	// AllowAllHeaders allows all headers.
	// +kubebuilder:validation:Optional
	AllowAllHeaders bool `json:"allowAllHeaders,omitempty"`

	// AllowAllOrigins allows all origins.
	// +kubebuilder:validation:Optional
	AllowAllOrigins bool `json:"allowAllOrigins,omitempty"`

	// AllowCredentials allows credentials.
	// +kubebuilder:validation:Optional
	AllowCredentials bool `json:"allowCredentials,omitempty"`

	// MaxAge is the maximum age for CORS preflight cache in seconds.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=86400
	MaxAge int `json:"maxAge,omitempty"`
}

// SaasApplicationConfig represents the SaaS application configuration.
type SaasApplicationConfig struct {
	// AuthType specifies the authentication type (saml or oidc).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=saml;oidc
	AuthType string `json:"authType"`

	// SAML Configuration (when authType=saml)

	// ConsumerServiceURL is the SAML consumer service URL.
	// +kubebuilder:validation:Optional
	ConsumerServiceURL string `json:"consumerServiceUrl,omitempty"`

	// SPEntityID is the SAML service provider entity ID.
	// +kubebuilder:validation:Optional
	SPEntityID string `json:"spEntityId,omitempty"`

	// NameIDFormat is the SAML name ID format.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=id;email
	NameIDFormat string `json:"nameIdFormat,omitempty"`

	// DefaultRelayState is the default relay state for SAML.
	// +kubebuilder:validation:Optional
	DefaultRelayState string `json:"defaultRelayState,omitempty"`

	// CustomAttributes defines custom SAML attributes.
	// +kubebuilder:validation:Optional
	CustomAttributes []SAMLAttributeConfig `json:"customAttributes,omitempty"`

	// NameIDTransformJsonata is a JSONata expression for transforming the name ID.
	// +kubebuilder:validation:Optional
	NameIDTransformJsonata string `json:"nameIdTransformJsonata,omitempty"`

	// SamlAttributeTransformJsonata is a JSONata expression for transforming SAML attributes.
	// +kubebuilder:validation:Optional
	SamlAttributeTransformJsonata string `json:"samlAttributeTransformJsonata,omitempty"`

	// OIDC Configuration (when authType=oidc)

	// RedirectURIs is a list of allowed redirect URIs for OIDC.
	// +kubebuilder:validation:Optional
	RedirectURIs []string `json:"redirectUris,omitempty"`

	// GrantTypes is a list of allowed grant types for OIDC.
	// +kubebuilder:validation:Optional
	GrantTypes []string `json:"grantTypes,omitempty"`

	// Scopes is a list of allowed scopes for OIDC.
	// +kubebuilder:validation:Optional
	Scopes []string `json:"scopes,omitempty"`

	// AppLauncherURL is the URL to launch the app from the app launcher.
	// +kubebuilder:validation:Optional
	AppLauncherURL string `json:"appLauncherUrl,omitempty"`

	// GroupFilterRegex is a regex for filtering groups in OIDC claims.
	// +kubebuilder:validation:Optional
	GroupFilterRegex string `json:"groupFilterRegex,omitempty"`

	// CustomClaims defines custom OIDC claims.
	// +kubebuilder:validation:Optional
	CustomClaims []OIDCClaimConfig `json:"customClaims,omitempty"`

	// AllowPKCEWithoutClientSecret allows PKCE without a client secret.
	// +kubebuilder:validation:Optional
	AllowPKCEWithoutClientSecret *bool `json:"allowPkceWithoutClientSecret,omitempty"`

	// AccessTokenLifetime is the lifetime of the access token.
	// +kubebuilder:validation:Optional
	AccessTokenLifetime string `json:"accessTokenLifetime,omitempty"`

	// RefreshTokenOptions configures refresh token behavior.
	// +kubebuilder:validation:Optional
	RefreshTokenOptions *RefreshTokenOptions `json:"refreshTokenOptions,omitempty"`

	// HybridAndImplicitOptions configures hybrid and implicit flow options.
	// +kubebuilder:validation:Optional
	HybridAndImplicitOptions *HybridAndImplicitOptions `json:"hybridAndImplicitOptions,omitempty"`
}

// SAMLAttributeConfig represents a custom SAML attribute.
type SAMLAttributeConfig struct {
	// Name is the attribute name.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// NameFormat is the attribute name format.
	// +kubebuilder:validation:Optional
	NameFormat string `json:"nameFormat,omitempty"`

	// Source specifies the source of the attribute value.
	// +kubebuilder:validation:Required
	Source SAMLAttributeSource `json:"source"`

	// FriendlyName is the friendly name of the attribute.
	// +kubebuilder:validation:Optional
	FriendlyName string `json:"friendlyName,omitempty"`

	// Required indicates if this attribute is required.
	// +kubebuilder:validation:Optional
	Required bool `json:"required,omitempty"`
}

// SAMLAttributeSource specifies the source of a SAML attribute.
type SAMLAttributeSource struct {
	// Name is the name of the source attribute.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// NameByIDP is a map of IdP name to attribute name.
	// +kubebuilder:validation:Optional
	NameByIDP map[string]string `json:"nameByIdp,omitempty"`
}

// OIDCClaimConfig represents a custom OIDC claim.
type OIDCClaimConfig struct {
	// Name is the claim name.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Source specifies the source of the claim value.
	// +kubebuilder:validation:Required
	Source OIDCClaimSource `json:"source"`

	// Required indicates if this claim is required.
	// +kubebuilder:validation:Optional
	Required bool `json:"required,omitempty"`

	// Scope is the scope for this claim.
	// +kubebuilder:validation:Optional
	Scope string `json:"scope,omitempty"`
}

// OIDCClaimSource specifies the source of an OIDC claim.
type OIDCClaimSource struct {
	// Name is the name of the source attribute.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// NameByIDP is a map of IdP name to claim name.
	// +kubebuilder:validation:Optional
	NameByIDP map[string]string `json:"nameByIdp,omitempty"`
}

// RefreshTokenOptions configures refresh token behavior.
type RefreshTokenOptions struct {
	// Lifetime is the lifetime of the refresh token.
	// +kubebuilder:validation:Optional
	Lifetime string `json:"lifetime,omitempty"`
}

// HybridAndImplicitOptions configures hybrid and implicit flow options.
type HybridAndImplicitOptions struct {
	// ReturnIDTokenFromAuthorizationEndpoint indicates whether to return an ID token
	// from the authorization endpoint.
	// +kubebuilder:validation:Optional
	ReturnIDTokenFromAuthorizationEndpoint *bool `json:"returnIdTokenFromAuthorizationEndpoint,omitempty"`

	// ReturnAccessTokenFromAuthorizationEndpoint indicates whether to return an access token
	// from the authorization endpoint.
	// +kubebuilder:validation:Optional
	ReturnAccessTokenFromAuthorizationEndpoint *bool `json:"returnAccessTokenFromAuthorizationEndpoint,omitempty"`
}

// AccessApplicationSCIMConfig represents SCIM configuration for an Access Application.
type AccessApplicationSCIMConfig struct {
	// Enabled enables SCIM provisioning.
	// +kubebuilder:validation:Optional
	Enabled *bool `json:"enabled,omitempty"`

	// RemoteURI is the remote SCIM endpoint URI.
	// +kubebuilder:validation:Optional
	RemoteURI string `json:"remoteUri,omitempty"`

	// Authentication configures SCIM authentication.
	// +kubebuilder:validation:Optional
	Authentication *SCIMAuthentication `json:"authentication,omitempty"`

	// IDPUID is the identity provider UID for SCIM.
	// +kubebuilder:validation:Optional
	IDPUID string `json:"idpUid,omitempty"`

	// DeactivateOnDelete deactivates users on delete instead of deleting.
	// +kubebuilder:validation:Optional
	DeactivateOnDelete *bool `json:"deactivateOnDelete,omitempty"`

	// Mappings defines SCIM attribute mappings.
	// +kubebuilder:validation:Optional
	Mappings []SCIMMapping `json:"mappings,omitempty"`
}

// SCIMAuthentication represents SCIM authentication configuration.
type SCIMAuthentication struct {
	// Scheme is the authentication scheme (httpbasic, oauthbearertoken, oauth2).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=httpbasic;oauthbearertoken;oauth2
	Scheme string `json:"scheme"`

	// User is the username for HTTP basic authentication.
	// +kubebuilder:validation:Optional
	User string `json:"user,omitempty"`

	// Password is the password for HTTP basic authentication.
	// Should be stored in a Secret and referenced.
	// +kubebuilder:validation:Optional
	Password string `json:"password,omitempty"`

	// Token is the bearer token for OAuth bearer token authentication.
	// Should be stored in a Secret and referenced.
	// +kubebuilder:validation:Optional
	Token string `json:"token,omitempty"`

	// ClientID is the OAuth client ID.
	// +kubebuilder:validation:Optional
	ClientID string `json:"clientId,omitempty"`

	// ClientSecret is the OAuth client secret.
	// Should be stored in a Secret and referenced.
	// +kubebuilder:validation:Optional
	ClientSecret string `json:"clientSecret,omitempty"`

	// AuthorizationURL is the OAuth authorization URL.
	// +kubebuilder:validation:Optional
	AuthorizationURL string `json:"authorizationUrl,omitempty"`

	// TokenURL is the OAuth token URL.
	// +kubebuilder:validation:Optional
	TokenURL string `json:"tokenUrl,omitempty"`

	// Scopes is a list of OAuth scopes.
	// +kubebuilder:validation:Optional
	Scopes []string `json:"scopes,omitempty"`
}

// SCIMMapping represents a SCIM attribute mapping.
type SCIMMapping struct {
	// Schema is the SCIM schema (e.g., "urn:ietf:params:scim:schemas:core:2.0:User").
	// +kubebuilder:validation:Required
	Schema string `json:"schema"`

	// Enabled enables this mapping.
	// +kubebuilder:validation:Optional
	Enabled *bool `json:"enabled,omitempty"`

	// Filter is a SCIM filter expression.
	// +kubebuilder:validation:Optional
	Filter string `json:"filter,omitempty"`

	// TransformJsonata is a JSONata expression for transforming the mapping.
	// +kubebuilder:validation:Optional
	TransformJsonata string `json:"transformJsonata,omitempty"`

	// Operations configures which SCIM operations are supported.
	// +kubebuilder:validation:Optional
	Operations *SCIMMappingOperations `json:"operations,omitempty"`

	// Strictness specifies how strictly to enforce the schema.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=strict;loose
	Strictness string `json:"strictness,omitempty"`
}

// SCIMMappingOperations specifies which SCIM operations are supported.
type SCIMMappingOperations struct {
	// Create enables the create operation.
	// +kubebuilder:validation:Optional
	Create *bool `json:"create,omitempty"`

	// Update enables the update operation.
	// +kubebuilder:validation:Optional
	Update *bool `json:"update,omitempty"`

	// Delete enables the delete operation.
	// +kubebuilder:validation:Optional
	Delete *bool `json:"delete,omitempty"`
}

// AccessAppLauncherCustomization represents the App Launcher customization settings.
type AccessAppLauncherCustomization struct {
	// LandingPageDesign configures the landing page appearance.
	// +kubebuilder:validation:Optional
	LandingPageDesign *AccessLandingPageDesign `json:"landingPageDesign,omitempty"`

	// AppLauncherLogoURL is the URL of the app launcher logo.
	// +kubebuilder:validation:Optional
	AppLauncherLogoURL string `json:"appLauncherLogoUrl,omitempty"`

	// HeaderBackgroundColor is the header background color (hex format).
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^#[0-9a-fA-F]{6}$`
	HeaderBackgroundColor string `json:"headerBackgroundColor,omitempty"`

	// BackgroundColor is the background color (hex format).
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^#[0-9a-fA-F]{6}$`
	BackgroundColor string `json:"backgroundColor,omitempty"`

	// FooterLinks is a list of footer links.
	// +kubebuilder:validation:Optional
	FooterLinks []AccessFooterLink `json:"footerLinks,omitempty"`

	// SkipAppLauncherLoginPage skips the app launcher login page.
	// +kubebuilder:validation:Optional
	SkipAppLauncherLoginPage *bool `json:"skipAppLauncherLoginPage,omitempty"`
}

// AccessLandingPageDesign represents the landing page design configuration.
type AccessLandingPageDesign struct {
	// Title is the landing page title.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	Title string `json:"title,omitempty"`

	// Message is the landing page message.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=1024
	Message string `json:"message,omitempty"`

	// ImageURL is the URL of the landing page image.
	// +kubebuilder:validation:Optional
	ImageURL string `json:"imageUrl,omitempty"`

	// ButtonColor is the button color (hex format).
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^#[0-9a-fA-F]{6}$`
	ButtonColor string `json:"buttonColor,omitempty"`

	// ButtonTextColor is the button text color (hex format).
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^#[0-9a-fA-F]{6}$`
	ButtonTextColor string `json:"buttonTextColor,omitempty"`
}

// AccessFooterLink represents a footer link in the App Launcher.
type AccessFooterLink struct {
	// Name is the display name of the link.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// URL is the link URL.
	// +kubebuilder:validation:Required
	URL string `json:"url"`
}

// AccessInfrastructureTargetContext specifies target criteria for infrastructure applications.
type AccessInfrastructureTargetContext struct {
	// TargetAttributes is a map of target attribute names to their allowed values.
	// +kubebuilder:validation:Required
	TargetAttributes map[string][]string `json:"targetAttributes"`

	// Port is the target port.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int `json:"port"`

	// Protocol is the target protocol (SSH, RDP, etc.).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=SSH;RDP
	Protocol string `json:"protocol"`
}

// AccessIdentityProviderRef references an AccessIdentityProvider resource.
type AccessIdentityProviderRef struct {
	// Name is the name of the AccessIdentityProvider resource.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// AccessPolicyRef references an access policy or defines an inline policy.
// You can either:
// 1. Reference an AccessGroup using name, groupId, or cloudflareGroupName (simple mode)
// 2. Define inline include/exclude/require rules directly (advanced mode)
// When using inline rules, group references are ignored.
type AccessPolicyRef struct {
	// --- Group Reference Mode (Simple) ---
	// Use one of these to reference an existing AccessGroup.

	// Name is the name of an AccessGroup resource (Kubernetes) to use as a policy.
	// If specified, the controller will look up the AccessGroup CR and use its GroupID.
	// Mutually exclusive with groupId, cloudflareGroupName, and inline rules.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name,omitempty"`

	// GroupID is the UUID of an existing Cloudflare Access Group.
	// Use this to directly reference a Cloudflare-managed Access Group
	// without creating a corresponding Kubernetes AccessGroup resource.
	// Mutually exclusive with name, cloudflareGroupName, and inline rules.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
	GroupID string `json:"groupId,omitempty"`

	// CloudflareGroupName is the display name of an existing Cloudflare Access Group.
	// The controller will resolve this name to a GroupID via the Cloudflare API.
	// Use this when you want to reference a Cloudflare Access Group by name
	// (e.g., groups created via Terraform or the Cloudflare dashboard).
	// Mutually exclusive with name, groupId, and inline rules.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	CloudflareGroupName string `json:"cloudflareGroupName,omitempty"`

	// --- Inline Rules Mode (Advanced) ---
	// Define include/exclude/require rules directly in the policy.
	// When any of these are specified, group references are ignored.

	// Include defines the rules that grant access. Users matching ANY rule in the include
	// list will be granted access (OR logic). At least one include rule is required
	// when using inline rules mode.
	// Uses the same rule types as AccessGroup (email, emailDomain, group, ipRanges, etc.).
	// +kubebuilder:validation:Optional
	Include []AccessGroupRule `json:"include,omitempty"`

	// Exclude defines the rules that deny access. Users matching ANY rule in the exclude
	// list will be denied access, even if they match an include rule (NOT logic).
	// +kubebuilder:validation:Optional
	Exclude []AccessGroupRule `json:"exclude,omitempty"`

	// Require defines the rules that must ALL be satisfied (AND logic).
	// Users must match ALL require rules in addition to at least one include rule.
	// +kubebuilder:validation:Optional
	Require []AccessGroupRule `json:"require,omitempty"`

	// --- Common Fields ---

	// Decision is the policy decision (allow, deny, bypass, non_identity).
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=allow;deny;bypass;non_identity
	// +kubebuilder:default=allow
	Decision string `json:"decision,omitempty"`

	// Precedence is the order of evaluation. Lower numbers are evaluated first.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	Precedence int `json:"precedence,omitempty"`

	// PolicyName is the name for this policy in Cloudflare.
	// If not specified, a name will be auto-generated based on the AccessApplication name and precedence.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	PolicyName string `json:"policyName,omitempty"`

	// SessionDuration overrides the application's session duration for this policy.
	// +kubebuilder:validation:Optional
	SessionDuration string `json:"sessionDuration,omitempty"`
}

// ResolvedPolicyStatus contains resolved policy information for debugging and status tracking.
type ResolvedPolicyStatus struct {
	// Precedence is the policy precedence (order of evaluation).
	Precedence int `json:"precedence"`

	// PolicyID is the Cloudflare policy ID.
	// +kubebuilder:validation:Optional
	PolicyID string `json:"policyId,omitempty"`

	// GroupID is the resolved Cloudflare Access Group ID.
	// +kubebuilder:validation:Optional
	GroupID string `json:"groupId,omitempty"`

	// GroupName is the name of the Access Group (for display purposes).
	// +kubebuilder:validation:Optional
	GroupName string `json:"groupName,omitempty"`

	// Source indicates how the group was resolved.
	// Possible values: k8s, groupId, cloudflareGroupName
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=k8s;groupId;cloudflareGroupName
	Source string `json:"source,omitempty"`

	// Decision is the policy decision (allow, deny, bypass, non_identity).
	// +kubebuilder:validation:Optional
	Decision string `json:"decision,omitempty"`
}

// ResolvedReusablePolicyStatus contains resolved reusable policy information.
type ResolvedReusablePolicyStatus struct {
	// PolicyID is the Cloudflare reusable policy ID.
	// +kubebuilder:validation:Optional
	PolicyID string `json:"policyId,omitempty"`

	// PolicyName is the name of the reusable policy (for display purposes).
	// +kubebuilder:validation:Optional
	PolicyName string `json:"policyName,omitempty"`

	// Source indicates how the policy was resolved.
	// Possible values: k8s, cloudflareId, cloudflareName
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=k8s;cloudflareId;cloudflareName
	Source string `json:"source,omitempty"`

	// Precedence is the policy precedence when attached to this application.
	// +kubebuilder:validation:Optional
	Precedence int `json:"precedence,omitempty"`

	// Decision is the policy decision (allow, deny, bypass, non_identity).
	// +kubebuilder:validation:Optional
	Decision string `json:"decision,omitempty"`
}

// AccessApplicationStatus defines the observed state of AccessApplication
type AccessApplicationStatus struct {
	// ApplicationID is the Cloudflare ID of the Access Application.
	// +kubebuilder:validation:Optional
	ApplicationID string `json:"applicationId,omitempty"`

	// AUD is the Application Audience (AUD) Tag.
	// +kubebuilder:validation:Optional
	AUD string `json:"aud,omitempty"`

	// AccountID is the Cloudflare Account ID.
	// +kubebuilder:validation:Optional
	AccountID string `json:"accountId,omitempty"`

	// Domain is the primary configured domain.
	// +kubebuilder:validation:Optional
	Domain string `json:"domain,omitempty"`

	// SelfHostedDomains is the list of all configured domains (from Cloudflare API response).
	// +kubebuilder:validation:Optional
	SelfHostedDomains []string `json:"selfHostedDomains,omitempty"`

	// State indicates the current state of the application.
	// +kubebuilder:validation:Optional
	State string `json:"state,omitempty"`

	// SaasAppClientID is the OIDC client ID (for SaaS applications with OIDC).
	// +kubebuilder:validation:Optional
	SaasAppClientID string `json:"saasAppClientId,omitempty"`

	// ResolvedPolicies contains the resolved policy information for each inline policy.
	// This helps with debugging and understanding policy state.
	// +kubebuilder:validation:Optional
	// +listType=map
	// +listMapKey=precedence
	ResolvedPolicies []ResolvedPolicyStatus `json:"resolvedPolicies,omitempty"`

	// ResolvedReusablePolicies contains the resolved reusable policy information.
	// These are policies referenced via reusablePolicyRefs.
	// +kubebuilder:validation:Optional
	ResolvedReusablePolicies []ResolvedReusablePolicyStatus `json:"resolvedReusablePolicies,omitempty"`

	// Conditions represent the latest available observations.
	// +kubebuilder:validation:Optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed.
	// +kubebuilder:validation:Optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=accessapp
// +kubebuilder:printcolumn:name="Domain",type=string,JSONPath=`.spec.domain`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="AppID",type=string,JSONPath=`.status.applicationId`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AccessApplication is the Schema for the accessapplications API.
// An AccessApplication represents a Cloudflare Access Application,
// which protects internal resources with Zero Trust policies.
type AccessApplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccessApplicationSpec   `json:"spec,omitempty"`
	Status AccessApplicationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AccessApplicationList contains a list of AccessApplication
type AccessApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccessApplication `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AccessApplication{}, &AccessApplicationList{})
}

// GetAccessApplicationName returns the name to use in Cloudflare.
func (a *AccessApplication) GetAccessApplicationName() string {
	if a.Spec.Name != "" {
		return a.Spec.Name
	}
	return a.Name
}
