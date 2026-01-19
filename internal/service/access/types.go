// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package access provides the AccessService for managing Cloudflare Access resource configurations.
//
//nolint:revive // max-public-structs is acceptable for comprehensive Access API types
package access

import (
	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// Resource Types for SyncState
const (
	// ResourceTypeAccessApplication is the SyncState resource type for AccessApplication
	ResourceTypeAccessApplication = v1alpha2.SyncResourceType("AccessApplication")
	// ResourceTypeAccessGroup is the SyncState resource type for AccessGroup
	ResourceTypeAccessGroup = v1alpha2.SyncResourceType("AccessGroup")
	// ResourceTypeAccessPolicy is the SyncState resource type for reusable AccessPolicy
	ResourceTypeAccessPolicy = v1alpha2.SyncResourceType("AccessPolicy")
	// ResourceTypeAccessServiceToken is the SyncState resource type for AccessServiceToken
	ResourceTypeAccessServiceToken = v1alpha2.SyncResourceType("AccessServiceToken")
	// ResourceTypeAccessIdentityProvider is the SyncState resource type for AccessIdentityProvider
	ResourceTypeAccessIdentityProvider = v1alpha2.SyncResourceType("AccessIdentityProvider")

	// Priority constants
	PriorityAccessApplication      = 100
	PriorityAccessGroup            = 100
	PriorityAccessPolicy           = 100
	PriorityAccessServiceToken     = 100
	PriorityAccessIdentityProvider = 100
)

// AccessApplicationConfig contains the configuration for an AccessApplication.
type AccessApplicationConfig struct {
	// Name is the application name in Cloudflare
	Name string `json:"name"`
	// Domain is the primary domain for the application
	Domain string `json:"domain"`
	// SelfHostedDomains is a list of additional domains
	SelfHostedDomains []string `json:"selfHostedDomains,omitempty"`
	// Destinations specifies the destination configurations
	Destinations []v1alpha2.AccessDestination `json:"destinations,omitempty"`
	// DomainType specifies if the domain is public or private
	DomainType string `json:"domainType,omitempty"`
	// PrivateAddress is the private address for private applications
	PrivateAddress string `json:"privateAddress,omitempty"`
	// Type is the application type (self_hosted, saas, etc.)
	Type string `json:"type"`
	// SessionDuration is the token validity duration
	SessionDuration string `json:"sessionDuration,omitempty"`
	// AllowedIdps is the list of allowed identity provider IDs
	AllowedIdps []string `json:"allowedIdps,omitempty"`
	// AutoRedirectToIdentity enables automatic IdP redirect
	AutoRedirectToIdentity bool `json:"autoRedirectToIdentity,omitempty"`
	// EnableBindingCookie enables the binding cookie
	EnableBindingCookie *bool `json:"enableBindingCookie,omitempty"`
	// HTTPOnlyCookieAttribute sets HttpOnly on the cookie
	HTTPOnlyCookieAttribute *bool `json:"httpOnlyCookieAttribute,omitempty"`
	// PathCookieAttribute sets the Path attribute on the cookie
	PathCookieAttribute *bool `json:"pathCookieAttribute,omitempty"`
	// SameSiteCookieAttribute sets the SameSite attribute
	SameSiteCookieAttribute string `json:"sameSiteCookieAttribute,omitempty"`
	// LogoURL is the application logo URL
	LogoURL string `json:"logoUrl,omitempty"`
	// SkipInterstitial skips the interstitial page
	SkipInterstitial *bool `json:"skipInterstitial,omitempty"`
	// OptionsPreflightBypass allows CORS preflight to bypass auth
	OptionsPreflightBypass *bool `json:"optionsPreflightBypass,omitempty"`
	// AppLauncherVisible shows the app in the App Launcher
	AppLauncherVisible *bool `json:"appLauncherVisible,omitempty"`
	// ServiceAuth401Redirect redirects unauthorized service auth
	ServiceAuth401Redirect *bool `json:"serviceAuth401Redirect,omitempty"`
	// CustomDenyMessage is shown when access is denied
	CustomDenyMessage string `json:"customDenyMessage,omitempty"`
	// CustomDenyURL redirects when access is denied
	CustomDenyURL string `json:"customDenyUrl,omitempty"`
	// CustomNonIdentityDenyURL for non-identity deny
	CustomNonIdentityDenyURL string `json:"customNonIdentityDenyUrl,omitempty"`
	// AllowAuthenticateViaWarp allows WARP authentication
	AllowAuthenticateViaWarp *bool `json:"allowAuthenticateViaWarp,omitempty"`
	// Tags are custom tags
	Tags []string `json:"tags,omitempty"`
	// CustomPages is a list of custom page IDs
	CustomPages []string `json:"customPages,omitempty"`
	// GatewayRules is a list of Gateway rule IDs
	GatewayRules []string `json:"gatewayRules,omitempty"`
	// CorsHeaders configures CORS
	CorsHeaders *v1alpha2.AccessApplicationCorsHeaders `json:"corsHeaders,omitempty"`
	// SaasApp configures SaaS application settings
	SaasApp *v1alpha2.SaasApplicationConfig `json:"saasApp,omitempty"`
	// SCIMConfig configures SCIM provisioning
	SCIMConfig *v1alpha2.AccessApplicationSCIMConfig `json:"scimConfig,omitempty"`
	// AppLauncherCustomization configures app launcher appearance
	AppLauncherCustomization *v1alpha2.AccessAppLauncherCustomization `json:"appLauncherCustomization,omitempty"`
	// TargetContexts for infrastructure applications
	TargetContexts []v1alpha2.AccessInfrastructureTargetContext `json:"targetContexts,omitempty"`
	// Policies defines inline access policies
	Policies []AccessPolicyConfig `json:"policies,omitempty"`
	// ReusablePolicyRefs references reusable AccessPolicy resources
	ReusablePolicyRefs []ReusablePolicyRefConfig `json:"reusablePolicyRefs,omitempty"`
}

// AccessPolicyConfig contains policy configuration for AccessApplication
type AccessPolicyConfig struct {
	// GroupID is the resolved Cloudflare Access Group ID (set by L2 if resolving K8s AccessGroup)
	GroupID string `json:"groupId,omitempty"`
	// GroupName for display purposes
	GroupName string `json:"groupName,omitempty"`
	// Decision is the policy decision (allow, deny, bypass, non_identity)
	Decision string `json:"decision"`
	// Precedence is the order of evaluation
	Precedence int `json:"precedence"`
	// PolicyName is the name in Cloudflare
	PolicyName string `json:"policyName,omitempty"`
	// SessionDuration overrides application session duration
	SessionDuration string `json:"sessionDuration,omitempty"`

	// Group Reference fields (one of these will be set, resolved by L5 Sync Controller)
	// CloudflareGroupID is a direct Cloudflare group ID reference (validated in L5)
	CloudflareGroupID string `json:"cloudflareGroupId,omitempty"`
	// CloudflareGroupName is a Cloudflare group name to look up (resolved in L5)
	CloudflareGroupName string `json:"cloudflareGroupName,omitempty"`
	// K8sAccessGroupName is a Kubernetes AccessGroup resource name (resolved in L5)
	K8sAccessGroupName string `json:"k8sAccessGroupName,omitempty"`
}

// AccessGroupConfig contains the configuration for an AccessGroup.
type AccessGroupConfig struct {
	// Name is the group name in Cloudflare
	Name string `json:"name"`
	// Include rules (OR logic)
	Include []v1alpha2.AccessGroupRule `json:"include"`
	// Exclude rules (NOT logic)
	Exclude []v1alpha2.AccessGroupRule `json:"exclude,omitempty"`
	// Require rules (AND logic)
	Require []v1alpha2.AccessGroupRule `json:"require,omitempty"`
	// IsDefault indicates if this is the default group
	IsDefault *bool `json:"isDefault,omitempty"`
}

// AccessServiceTokenConfig contains the configuration for an AccessServiceToken.
type AccessServiceTokenConfig struct {
	// Name is the token name in Cloudflare
	Name string `json:"name"`
	// Duration is the token validity duration (e.g., "8760h")
	Duration string `json:"duration,omitempty"`
	// SecretRef references the K8s secret for storing credentials
	SecretRef *SecretReference `json:"secretRef,omitempty"`
}

// SecretReference contains information about a K8s secret
type SecretReference struct {
	// Name is the secret name
	Name string `json:"name"`
	// Namespace is the secret namespace
	Namespace string `json:"namespace,omitempty"`
}

// AccessIdentityProviderConfig contains the configuration for an AccessIdentityProvider.
type AccessIdentityProviderConfig struct {
	// Name is the IdP name in Cloudflare
	Name string `json:"name"`
	// Type is the IdP type (google, okta, etc.)
	Type string `json:"type"`
	// Config contains the IdP-specific configuration
	Config *v1alpha2.IdentityProviderConfig `json:"config,omitempty"`
	// ScimConfig contains SCIM configuration
	ScimConfig *v1alpha2.IdentityProviderScimConfig `json:"scimConfig,omitempty"`
}

// AccessApplicationRegisterOptions contains options for registering an AccessApplication.
type AccessApplicationRegisterOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// ApplicationID is the existing Cloudflare application ID (empty for new)
	ApplicationID string
	// Source is the K8s resource source
	Source service.Source
	// Config is the application configuration
	Config AccessApplicationConfig
	// CredentialsRef references the CloudflareCredentials resource
	CredentialsRef v1alpha2.CredentialsReference
}

// AccessGroupRegisterOptions contains options for registering an AccessGroup.
type AccessGroupRegisterOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// GroupID is the existing Cloudflare group ID (empty for new)
	GroupID string
	// Source is the K8s resource source
	Source service.Source
	// Config is the group configuration
	Config AccessGroupConfig
	// CredentialsRef references the CloudflareCredentials resource
	CredentialsRef v1alpha2.CredentialsReference
}

// AccessServiceTokenRegisterOptions contains options for registering an AccessServiceToken.
type AccessServiceTokenRegisterOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// TokenID is the existing Cloudflare token ID (empty for new)
	TokenID string
	// Source is the K8s resource source
	Source service.Source
	// Config is the token configuration
	Config AccessServiceTokenConfig
	// CredentialsRef references the CloudflareCredentials resource
	CredentialsRef v1alpha2.CredentialsReference
}

// AccessIdentityProviderRegisterOptions contains options for registering an AccessIdentityProvider.
type AccessIdentityProviderRegisterOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// ProviderID is the existing Cloudflare provider ID (empty for new)
	ProviderID string
	// Source is the K8s resource source
	Source service.Source
	// Config is the IdP configuration
	Config AccessIdentityProviderConfig
	// CredentialsRef references the CloudflareCredentials resource
	CredentialsRef v1alpha2.CredentialsReference
}

// SyncResult contains the result of a sync operation.
type SyncResult struct {
	// ID is the Cloudflare resource ID
	ID string
	// AccountID is the Cloudflare account ID
	AccountID string
}

// AccessApplicationSyncResult contains AccessApplication-specific sync result.
type AccessApplicationSyncResult struct {
	SyncResult
	// AUD is the Application Audience Tag
	AUD string
	// Domain is the primary domain
	Domain string
	// SelfHostedDomains is the list of all domains
	SelfHostedDomains []string
	// SaasAppClientID for SaaS OIDC applications
	SaasAppClientID string
	// ResolvedPolicies contains resolved policy information
	ResolvedPolicies []v1alpha2.ResolvedPolicyStatus
}

// AccessServiceTokenSyncResult contains AccessServiceToken-specific sync result.
type AccessServiceTokenSyncResult struct {
	SyncResult
	// ClientID is the service token client ID
	ClientID string
	// ClientSecret is only available on creation
	ClientSecret string
	// ExpiresAt is the token expiration time
	ExpiresAt string
	// CreatedAt is the token creation time
	CreatedAt string
	// UpdatedAt is the token last update time
	UpdatedAt string
	// LastSeenAt is when the token was last used
	LastSeenAt string
	// ClientSecretVersion is the version of the client secret
	ClientSecretVersion string
}

// AccessIdentityProviderSyncResult contains AccessIdentityProvider-specific sync result.
type AccessIdentityProviderSyncResult struct {
	SyncResult
}

// ReusableAccessPolicyConfig contains the configuration for a reusable AccessPolicy.
type ReusableAccessPolicyConfig struct {
	// Name is the policy name in Cloudflare
	Name string `json:"name"`
	// Decision is the policy decision (allow, deny, bypass, non_identity)
	Decision string `json:"decision"`
	// Precedence is the order of evaluation
	Precedence int `json:"precedence,omitempty"`
	// Include rules (OR logic)
	Include []v1alpha2.AccessGroupRule `json:"include"`
	// Exclude rules (NOT logic)
	Exclude []v1alpha2.AccessGroupRule `json:"exclude,omitempty"`
	// Require rules (AND logic)
	Require []v1alpha2.AccessGroupRule `json:"require,omitempty"`
	// SessionDuration overrides application session duration
	SessionDuration string `json:"sessionDuration,omitempty"`
	// IsolationRequired enables browser isolation
	IsolationRequired *bool `json:"isolationRequired,omitempty"`
	// PurposeJustificationRequired requires users to provide justification
	PurposeJustificationRequired *bool `json:"purposeJustificationRequired,omitempty"`
	// PurposeJustificationPrompt is the prompt shown when justification is required
	PurposeJustificationPrompt string `json:"purposeJustificationPrompt,omitempty"`
	// ApprovalRequired requires admin approval for access
	ApprovalRequired *bool `json:"approvalRequired,omitempty"`
	// ApprovalGroups defines the groups that can approve access
	ApprovalGroups []ApprovalGroupConfig `json:"approvalGroups,omitempty"`
}

// ApprovalGroupConfig contains approval group configuration.
type ApprovalGroupConfig struct {
	// EmailAddresses is the list of email addresses that can approve
	EmailAddresses []string `json:"emailAddresses,omitempty"`
	// EmailListUUID is the UUID of an email list that can approve
	EmailListUUID string `json:"emailListUuid,omitempty"`
	// ApprovalsNeeded is the number of approvals required
	ApprovalsNeeded int `json:"approvalsNeeded,omitempty"`
}

// ReusableAccessPolicyRegisterOptions contains options for registering a reusable AccessPolicy.
type ReusableAccessPolicyRegisterOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// PolicyID is the existing Cloudflare policy ID (empty for new)
	PolicyID string
	// Source is the K8s resource source
	Source service.Source
	// Config is the policy configuration
	Config ReusableAccessPolicyConfig
	// CredentialsRef references the CloudflareCredentials resource
	CredentialsRef v1alpha2.CredentialsReference
}

// ReusableAccessPolicySyncResult contains reusable AccessPolicy-specific sync result.
type ReusableAccessPolicySyncResult struct {
	SyncResult
	// Name is the policy name
	Name string
	// Decision is the policy decision
	Decision string
}

// ReusablePolicyRefConfig contains a reference to a reusable policy for AccessApplication.
type ReusablePolicyRefConfig struct {
	// Name is the K8s AccessPolicy resource name
	Name string `json:"name,omitempty"`
	// CloudflareID is a direct Cloudflare policy ID reference
	CloudflareID string `json:"cloudflareId,omitempty"`
	// CloudflareName is a Cloudflare policy name to look up
	CloudflareName string `json:"cloudflareName,omitempty"`
	// Precedence overrides the policy's default precedence
	Precedence *int `json:"precedence,omitempty"`
}
