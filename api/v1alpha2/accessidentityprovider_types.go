// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccessIdentityProviderSpec defines the desired state of AccessIdentityProvider
type AccessIdentityProviderSpec struct {
	// Name of the Identity Provider in Cloudflare.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name,omitempty"`

	// Type is the identity provider type.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=onetimepin;azureAD;saml;centrify;facebook;github;google-apps;google;linkedin;oidc;okta;onelogin;pingone;yandex
	Type string `json:"type"`

	// Config contains provider-specific configuration.
	// +kubebuilder:validation:Optional
	Config *IdentityProviderConfig `json:"config,omitempty"`

	// ConfigSecretRef references a Secret containing sensitive config values.
	// +kubebuilder:validation:Optional
	ConfigSecretRef *SecretKeySelector `json:"configSecretRef,omitempty"`

	// ScimConfig contains SCIM provisioning configuration.
	// +kubebuilder:validation:Optional
	ScimConfig *IdentityProviderScimConfig `json:"scimConfig,omitempty"`

	// Cloudflare contains the Cloudflare API credentials.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`
}

// SecretKeySelector selects a key from a Secret.
type SecretKeySelector struct {
	// Name is the name of the Secret.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Key is the key in the Secret.
	// +kubebuilder:validation:Required
	Key string `json:"key"`

	// Namespace is the namespace of the Secret.
	// +kubebuilder:validation:Optional
	Namespace string `json:"namespace,omitempty"`
}

// IdentityProviderConfig contains provider configuration.
type IdentityProviderConfig struct {
	// ClientID is the OAuth client ID.
	// +kubebuilder:validation:Optional
	ClientID string `json:"clientId,omitempty"`

	// ClientSecret is the OAuth client secret (use ConfigSecretRef for sensitive values).
	// +kubebuilder:validation:Optional
	ClientSecret string `json:"clientSecret,omitempty"`

	// AppsDomain is the Google Workspace domain.
	// +kubebuilder:validation:Optional
	AppsDomain string `json:"appsDomain,omitempty"`

	// AuthURL is the authorization URL (OIDC/OAuth).
	// +kubebuilder:validation:Optional
	AuthURL string `json:"authUrl,omitempty"`

	// TokenURL is the token endpoint URL.
	// +kubebuilder:validation:Optional
	TokenURL string `json:"tokenUrl,omitempty"`

	// CertsURL is the JWKS endpoint URL.
	// +kubebuilder:validation:Optional
	CertsURL string `json:"certsUrl,omitempty"`

	// Scopes are the OAuth scopes to request.
	// +kubebuilder:validation:Optional
	Scopes []string `json:"scopes,omitempty"`

	// Attributes are custom attributes to include in tokens.
	// +kubebuilder:validation:Optional
	Attributes []string `json:"attributes,omitempty"`

	// IdPPublicCert is the IdP's public certificate for SAML (single cert).
	// +kubebuilder:validation:Optional
	IdPPublicCert string `json:"idpPublicCert,omitempty"`

	// IdPPublicCerts are the IdP's public certificates for SAML (multiple certs).
	// Deprecated: Use IdPPublicCert instead.
	// +kubebuilder:validation:Optional
	IdPPublicCerts []string `json:"idpPublicCerts,omitempty"`

	// IssuerURL is the OIDC issuer URL.
	// +kubebuilder:validation:Optional
	IssuerURL string `json:"issuerUrl,omitempty"`

	// SSOTargetURL is the SAML SSO URL.
	// +kubebuilder:validation:Optional
	SSOTargetURL string `json:"ssoTargetUrl,omitempty"`

	// SignRequest enables SAML request signing.
	// +kubebuilder:validation:Optional
	SignRequest *bool `json:"signRequest,omitempty"`

	// EmailClaimName is the claim containing the user's email.
	// +kubebuilder:validation:Optional
	EmailClaimName string `json:"emailClaimName,omitempty"`

	// DirectoryID is the Azure AD directory ID.
	// +kubebuilder:validation:Optional
	DirectoryID string `json:"directoryId,omitempty"`

	// SupportGroups enables group sync.
	// +kubebuilder:validation:Optional
	SupportGroups *bool `json:"supportGroups,omitempty"`

	// PKCEEnabled enables PKCE.
	// +kubebuilder:validation:Optional
	PKCEEnabled *bool `json:"pkceEnabled,omitempty"`

	// ConditionalAccessEnabled enables Azure AD conditional access.
	// +kubebuilder:validation:Optional
	ConditionalAccessEnabled *bool `json:"conditionalAccessEnabled,omitempty"`

	// Claims are custom OIDC claims to include.
	// +kubebuilder:validation:Optional
	Claims []string `json:"claims,omitempty"`

	// EmailAttributeName is the SAML attribute containing email.
	// +kubebuilder:validation:Optional
	EmailAttributeName string `json:"emailAttributeName,omitempty"`

	// HeaderAttributes are SAML attributes to pass as headers.
	// +kubebuilder:validation:Optional
	HeaderAttributes []SAMLHeaderAttribute `json:"headerAttributes,omitempty"`

	// APIToken is the API token (GitHub, etc).
	// +kubebuilder:validation:Optional
	APIToken string `json:"apiToken,omitempty"`

	// OktaAccount is the Okta organization URL.
	// +kubebuilder:validation:Optional
	OktaAccount string `json:"oktaAccount,omitempty"`

	// OktaAuthorizationServerID is the Okta authorization server ID.
	// +kubebuilder:validation:Optional
	OktaAuthorizationServerID string `json:"oktaAuthorizationServerId,omitempty"`

	// OneloginAccount is the OneLogin subdomain.
	// +kubebuilder:validation:Optional
	OneloginAccount string `json:"oneloginAccount,omitempty"`

	// PingEnvID is the PingOne environment ID.
	// +kubebuilder:validation:Optional
	PingEnvID string `json:"pingEnvId,omitempty"`

	// CentrifyAccount is the Centrify account.
	// +kubebuilder:validation:Optional
	CentrifyAccount string `json:"centrifyAccount,omitempty"`

	// CentrifyAppID is the Centrify app ID.
	// +kubebuilder:validation:Optional
	CentrifyAppID string `json:"centrifyAppId,omitempty"`

	// RedirectURL is the callback URL.
	// +kubebuilder:validation:Optional
	RedirectURL string `json:"redirectUrl,omitempty"`
}

// IdentityProviderScimConfig contains SCIM provisioning configuration.
type IdentityProviderScimConfig struct {
	// Enabled enables SCIM provisioning.
	// +kubebuilder:validation:Optional
	Enabled *bool `json:"enabled,omitempty"`

	// Secret is the SCIM secret (should use a Secret reference in production).
	// +kubebuilder:validation:Optional
	Secret string `json:"secret,omitempty"`

	// UserDeprovision enables automatic user deprovisioning.
	// +kubebuilder:validation:Optional
	UserDeprovision *bool `json:"userDeprovision,omitempty"`

	// SeatDeprovision enables automatic seat deprovisioning.
	// +kubebuilder:validation:Optional
	SeatDeprovision *bool `json:"seatDeprovision,omitempty"`

	// GroupMemberDeprovision enables automatic group member deprovisioning.
	// +kubebuilder:validation:Optional
	GroupMemberDeprovision *bool `json:"groupMemberDeprovision,omitempty"`

	// IdentityUpdateBehavior controls how identity updates are handled.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=automatic;reauth;no_action
	IdentityUpdateBehavior string `json:"identityUpdateBehavior,omitempty"`
}

// SAMLHeaderAttribute defines a SAML attribute to header mapping.
type SAMLHeaderAttribute struct {
	// AttributeName is the SAML attribute name.
	AttributeName string `json:"attributeName"`

	// HeaderName is the HTTP header name.
	HeaderName string `json:"headerName"`

	// Required indicates if this attribute is required.
	// +kubebuilder:validation:Optional
	Required bool `json:"required,omitempty"`
}

// AccessIdentityProviderStatus defines the observed state
type AccessIdentityProviderStatus struct {
	// ProviderID is the Cloudflare ID.
	// +kubebuilder:validation:Optional
	ProviderID string `json:"providerId,omitempty"`

	// AccountID is the Cloudflare Account ID.
	// +kubebuilder:validation:Optional
	AccountID string `json:"accountId,omitempty"`

	// State indicates the current state.
	// +kubebuilder:validation:Optional
	State string `json:"state,omitempty"`

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
// +kubebuilder:resource:scope=Cluster,shortName=accessidp
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="ProviderID",type=string,JSONPath=`.status.providerId`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AccessIdentityProvider is the Schema for the accessidentityproviders API.
type AccessIdentityProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccessIdentityProviderSpec   `json:"spec,omitempty"`
	Status AccessIdentityProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AccessIdentityProviderList contains a list of AccessIdentityProvider
type AccessIdentityProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccessIdentityProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AccessIdentityProvider{}, &AccessIdentityProviderList{})
}

// GetProviderName returns the name to use in Cloudflare.
func (a *AccessIdentityProvider) GetProviderName() string {
	if a.Spec.Name != "" {
		return a.Spec.Name
	}
	return a.Name
}
