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

// DeviceSettingsPolicySpec defines the desired state of DeviceSettingsPolicy
type DeviceSettingsPolicySpec struct {
	// SplitTunnelMode determines how split tunneling is configured.
	// "exclude" means traffic to listed addresses bypasses the tunnel (default WARP behavior).
	// "include" means only traffic to listed addresses goes through the tunnel.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=exclude;include
	// +kubebuilder:default=exclude
	SplitTunnelMode string `json:"splitTunnelMode,omitempty"`

	// SplitTunnelExclude lists addresses/hosts to exclude from the tunnel.
	// Only used when SplitTunnelMode is "exclude".
	// +kubebuilder:validation:Optional
	SplitTunnelExclude []SplitTunnelEntry `json:"splitTunnelExclude,omitempty"`

	// SplitTunnelInclude lists addresses/hosts to include in the tunnel.
	// Only used when SplitTunnelMode is "include".
	// +kubebuilder:validation:Optional
	SplitTunnelInclude []SplitTunnelEntry `json:"splitTunnelInclude,omitempty"`

	// FallbackDomains lists domains that should use the specified DNS servers
	// instead of Gateway DNS.
	// +kubebuilder:validation:Optional
	FallbackDomains []FallbackDomainEntry `json:"fallbackDomains,omitempty"`

	// AutoPopulateFromRoutes automatically populates split tunnel entries
	// from NetworkRoute resources in the cluster.
	// +kubebuilder:validation:Optional
	AutoPopulateFromRoutes *AutoPopulateConfig `json:"autoPopulateFromRoutes,omitempty"`

	// Cloudflare contains the Cloudflare API credentials and account information.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`
}

// SplitTunnelEntry represents a single split tunnel entry.
type SplitTunnelEntry struct {
	// Address is a CIDR notation for IP addresses to match.
	// Either Address or Host must be specified.
	// +kubebuilder:validation:Optional
	Address string `json:"address,omitempty"`

	// Host is a domain name to match.
	// Either Address or Host must be specified.
	// +kubebuilder:validation:Optional
	Host string `json:"host,omitempty"`

	// Description is an optional description for this entry.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=200
	Description string `json:"description,omitempty"`
}

// FallbackDomainEntry represents a fallback domain configuration.
type FallbackDomainEntry struct {
	// Suffix is the domain suffix to match (e.g., "internal.company.com").
	// +kubebuilder:validation:Required
	Suffix string `json:"suffix"`

	// Description is an optional description for this entry.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=200
	Description string `json:"description,omitempty"`

	// DNSServer is a list of DNS server IPs to use for this domain.
	// +kubebuilder:validation:Optional
	DNSServer []string `json:"dnsServer,omitempty"`
}

// AutoPopulateConfig configures automatic population of split tunnel entries.
type AutoPopulateConfig struct {
	// Enabled enables automatic population from NetworkRoute resources.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// LabelSelector selects which NetworkRoute resources to include.
	// If empty, all NetworkRoute resources are included.
	// +kubebuilder:validation:Optional
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`

	// DescriptionPrefix is prepended to auto-generated descriptions.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="Auto-populated from NetworkRoute: "
	DescriptionPrefix string `json:"descriptionPrefix,omitempty"`
}

// DeviceSettingsPolicyStatus defines the observed state of DeviceSettingsPolicy
type DeviceSettingsPolicyStatus struct {
	// AccountID is the Cloudflare Account ID.
	// +kubebuilder:validation:Optional
	AccountID string `json:"accountId,omitempty"`

	// SplitTunnelExcludeCount is the number of exclude entries configured.
	// +kubebuilder:validation:Optional
	SplitTunnelExcludeCount int `json:"splitTunnelExcludeCount,omitempty"`

	// SplitTunnelIncludeCount is the number of include entries configured.
	// +kubebuilder:validation:Optional
	SplitTunnelIncludeCount int `json:"splitTunnelIncludeCount,omitempty"`

	// FallbackDomainsCount is the number of fallback domain entries configured.
	// +kubebuilder:validation:Optional
	FallbackDomainsCount int `json:"fallbackDomainsCount,omitempty"`

	// AutoPopulatedRoutesCount is the number of routes auto-populated from NetworkRoutes.
	// +kubebuilder:validation:Optional
	AutoPopulatedRoutesCount int `json:"autoPopulatedRoutesCount,omitempty"`

	// State indicates the current state of the policy.
	// +kubebuilder:validation:Optional
	State string `json:"state,omitempty"`

	// Conditions represent the latest available observations of the DeviceSettingsPolicy's state.
	// +kubebuilder:validation:Optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +kubebuilder:validation:Optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=dsp
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.splitTunnelMode`
// +kubebuilder:printcolumn:name="Exclude",type=integer,JSONPath=`.status.splitTunnelExcludeCount`
// +kubebuilder:printcolumn:name="Include",type=integer,JSONPath=`.status.splitTunnelIncludeCount`
// +kubebuilder:printcolumn:name="Fallback",type=integer,JSONPath=`.status.fallbackDomainsCount`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DeviceSettingsPolicy is the Schema for the devicesettingspolicies API.
// A DeviceSettingsPolicy configures WARP client device settings including
// split tunnel rules and fallback domains for an account.
type DeviceSettingsPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeviceSettingsPolicySpec   `json:"spec,omitempty"`
	Status DeviceSettingsPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DeviceSettingsPolicyList contains a list of DeviceSettingsPolicy
type DeviceSettingsPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DeviceSettingsPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DeviceSettingsPolicy{}, &DeviceSettingsPolicyList{})
}
