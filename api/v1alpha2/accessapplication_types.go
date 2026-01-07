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

	// Type is the application type.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=self_hosted;saas;ssh;vnc;app_launcher;warp;biso;bookmark;dash_sso
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

	// AllowAuthenticateViaWarp allows authentication via WARP.
	// +kubebuilder:validation:Optional
	AllowAuthenticateViaWarp *bool `json:"allowAuthenticateViaWarp,omitempty"`

	// Tags are custom tags for the application.
	// +kubebuilder:validation:Optional
	Tags []string `json:"tags,omitempty"`

	// Policies defines the access policies for this application.
	// +kubebuilder:validation:Optional
	Policies []AccessPolicyRef `json:"policies,omitempty"`

	// Cloudflare contains the Cloudflare API credentials and account information.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`
}

// AccessIdentityProviderRef references an AccessIdentityProvider resource.
type AccessIdentityProviderRef struct {
	// Name is the name of the AccessIdentityProvider resource.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// AccessPolicyRef references an access policy or defines an inline policy.
type AccessPolicyRef struct {
	// Name is the name of an AccessGroup resource to use as a policy.
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`

	// Decision is the policy decision (allow, deny, bypass, non_identity).
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=allow;deny;bypass;non_identity
	// +kubebuilder:default=allow
	Decision string `json:"decision,omitempty"`

	// Precedence is the order of evaluation.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	Precedence int `json:"precedence,omitempty"`
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

	// Domain is the configured domain.
	// +kubebuilder:validation:Optional
	Domain string `json:"domain,omitempty"`

	// State indicates the current state of the application.
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
