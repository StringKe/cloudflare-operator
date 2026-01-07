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

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccessGroupSpec defines the desired state of AccessGroup
type AccessGroupSpec struct {
	// Name of the Access Group in Cloudflare.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name,omitempty"`

	// Include defines rules that users must match to be included.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Include []AccessGroupRule `json:"include"`

	// Exclude defines rules that exclude users even if they match include rules.
	// +kubebuilder:validation:Optional
	Exclude []AccessGroupRule `json:"exclude,omitempty"`

	// Require defines rules that all users must match in addition to include rules.
	// +kubebuilder:validation:Optional
	Require []AccessGroupRule `json:"require,omitempty"`

	// Cloudflare contains the Cloudflare API credentials.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`
}

// AccessGroupRule defines a single rule in an Access Group.
type AccessGroupRule struct {
	// Email matches a specific email address.
	// +kubebuilder:validation:Optional
	Email *AccessGroupEmailRule `json:"email,omitempty"`

	// EmailDomain matches all emails from a domain.
	// +kubebuilder:validation:Optional
	EmailDomain *AccessGroupEmailDomainRule `json:"emailDomain,omitempty"`

	// Everyone matches all users.
	// +kubebuilder:validation:Optional
	Everyone bool `json:"everyone,omitempty"`

	// IPRanges matches users from specific IP ranges.
	// +kubebuilder:validation:Optional
	IPRanges *AccessGroupIPRangesRule `json:"ipRanges,omitempty"`

	// Country matches users from specific countries.
	// +kubebuilder:validation:Optional
	Country *AccessGroupCountryRule `json:"country,omitempty"`

	// Group matches users in a specific IdP group.
	// +kubebuilder:validation:Optional
	Group *AccessGroupGroupRule `json:"group,omitempty"`

	// ServiceToken matches requests with a specific service token.
	// +kubebuilder:validation:Optional
	ServiceToken *AccessGroupServiceTokenRule `json:"serviceToken,omitempty"`

	// AnyValidServiceToken matches any valid service token.
	// +kubebuilder:validation:Optional
	AnyValidServiceToken bool `json:"anyValidServiceToken,omitempty"`

	// Certificate matches requests with a valid mTLS certificate.
	// +kubebuilder:validation:Optional
	Certificate bool `json:"certificate,omitempty"`

	// CommonName matches mTLS certificates with a specific common name.
	// +kubebuilder:validation:Optional
	CommonName *AccessGroupCommonNameRule `json:"commonName,omitempty"`

	// DevicePosture matches devices that pass posture checks.
	// +kubebuilder:validation:Optional
	DevicePosture *AccessGroupDevicePostureRule `json:"devicePosture,omitempty"`

	// GSUITE matches users from Google Workspace.
	// +kubebuilder:validation:Optional
	GSuite *AccessGroupGSuiteRule `json:"gsuite,omitempty"`

	// GitHub matches users from GitHub organizations.
	// +kubebuilder:validation:Optional
	GitHub *AccessGroupGitHubRule `json:"github,omitempty"`

	// Azure matches users from Azure AD groups.
	// +kubebuilder:validation:Optional
	Azure *AccessGroupAzureRule `json:"azure,omitempty"`

	// OIDC matches users based on OIDC claims.
	// +kubebuilder:validation:Optional
	OIDC *AccessGroupOIDCRule `json:"oidc,omitempty"`

	// SAML matches users based on SAML attributes.
	// +kubebuilder:validation:Optional
	SAML *AccessGroupSAMLRule `json:"saml,omitempty"`

	// ExternalEvaluation calls an external endpoint for evaluation.
	// +kubebuilder:validation:Optional
	ExternalEvaluation *AccessGroupExternalEvaluationRule `json:"externalEvaluation,omitempty"`
}

// AccessGroupEmailRule matches a specific email.
type AccessGroupEmailRule struct {
	Email string `json:"email"`
}

// AccessGroupEmailDomainRule matches emails from a domain.
type AccessGroupEmailDomainRule struct {
	Domain string `json:"domain"`
}

// AccessGroupIPRangesRule matches IP ranges.
type AccessGroupIPRangesRule struct {
	IP []string `json:"ip"`
}

// AccessGroupCountryRule matches countries.
type AccessGroupCountryRule struct {
	Country []string `json:"country"`
}

// AccessGroupGroupRule matches IdP groups.
type AccessGroupGroupRule struct {
	ID string `json:"id"`
}

// AccessGroupServiceTokenRule matches a service token.
type AccessGroupServiceTokenRule struct {
	TokenID string `json:"tokenId"`
}

// AccessGroupCommonNameRule matches certificate common names.
type AccessGroupCommonNameRule struct {
	CommonName string `json:"commonName"`
}

// AccessGroupDevicePostureRule matches device posture.
type AccessGroupDevicePostureRule struct {
	IntegrationUID string `json:"integrationUid"`
}

// AccessGroupGSuiteRule matches Google Workspace users.
type AccessGroupGSuiteRule struct {
	Email              string `json:"email"`
	IdentityProviderID string `json:"identityProviderId"`
}

// AccessGroupGitHubRule matches GitHub users.
type AccessGroupGitHubRule struct {
	Name               string   `json:"name"`
	IdentityProviderID string   `json:"identityProviderId"`
	Teams              []string `json:"teams,omitempty"`
}

// AccessGroupAzureRule matches Azure AD users.
type AccessGroupAzureRule struct {
	ID                 string `json:"id"`
	IdentityProviderID string `json:"identityProviderId"`
}

// AccessGroupOIDCRule matches OIDC claims.
type AccessGroupOIDCRule struct {
	ClaimName          string `json:"claimName"`
	ClaimValue         string `json:"claimValue"`
	IdentityProviderID string `json:"identityProviderId"`
}

// AccessGroupSAMLRule matches SAML attributes.
type AccessGroupSAMLRule struct {
	AttributeName      string `json:"attributeName"`
	AttributeValue     string `json:"attributeValue"`
	IdentityProviderID string `json:"identityProviderId"`
}

// AccessGroupExternalEvaluationRule calls external endpoint.
type AccessGroupExternalEvaluationRule struct {
	EvaluateURL string `json:"evaluateUrl"`
	KeysURL     string `json:"keysUrl"`
}

// AccessGroupStatus defines the observed state of AccessGroup
type AccessGroupStatus struct {
	// GroupID is the Cloudflare ID of the Access Group.
	// +kubebuilder:validation:Optional
	GroupID string `json:"groupId,omitempty"`

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
// +kubebuilder:resource:scope=Cluster,shortName=accessgrp
// +kubebuilder:printcolumn:name="GroupID",type=string,JSONPath=`.status.groupId`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AccessGroup is the Schema for the accessgroups API.
type AccessGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccessGroupSpec   `json:"spec,omitempty"`
	Status AccessGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AccessGroupList contains a list of AccessGroup
type AccessGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccessGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AccessGroup{}, &AccessGroupList{})
}

// GetAccessGroupName returns the name to use in Cloudflare.
func (a *AccessGroup) GetAccessGroupName() string {
	if a.Spec.Name != "" {
		return a.Spec.Name
	}
	return a.Name
}
