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

// GatewayRuleSpec defines the desired state of GatewayRule
type GatewayRuleSpec struct {
	// Name of the Gateway Rule in Cloudflare.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name,omitempty"`

	// Description is a human-readable description.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=1000
	Description string `json:"description,omitempty"`

	// Precedence determines the order of rule evaluation (lower = earlier).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=0
	Precedence int `json:"precedence"`

	// Enabled controls whether the rule is active.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Action is what happens when the rule matches.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=allow;block;log;isolate;l4_override;egress;resolve;quarantine
	Action string `json:"action"`

	// Filters specifies which types of traffic this rule applies to.
	// +kubebuilder:validation:Optional
	Filters []string `json:"filters,omitempty"`

	// Traffic is the wirefilter expression for traffic matching.
	// +kubebuilder:validation:Optional
	Traffic string `json:"traffic,omitempty"`

	// Identity is the wirefilter expression for identity matching.
	// +kubebuilder:validation:Optional
	Identity string `json:"identity,omitempty"`

	// DevicePosture is the wirefilter expression for device posture matching.
	// +kubebuilder:validation:Optional
	DevicePosture string `json:"devicePosture,omitempty"`

	// RuleSettings contains action-specific settings.
	// +kubebuilder:validation:Optional
	RuleSettings *GatewayRuleSettings `json:"ruleSettings,omitempty"`

	// Cloudflare contains the Cloudflare API credentials.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`
}

// GatewayRuleSettings contains action-specific settings.
type GatewayRuleSettings struct {
	// BlockPageEnabled enables custom block page.
	// +kubebuilder:validation:Optional
	BlockPageEnabled *bool `json:"blockPageEnabled,omitempty"`

	// BlockReason is shown on the block page.
	// +kubebuilder:validation:Optional
	BlockReason string `json:"blockReason,omitempty"`

	// OverrideIPs for DNS override action.
	// +kubebuilder:validation:Optional
	OverrideIPs []string `json:"overrideIps,omitempty"`

	// OverrideHost for DNS override action.
	// +kubebuilder:validation:Optional
	OverrideHost string `json:"overrideHost,omitempty"`

	// L4Override for L4 override action.
	// +kubebuilder:validation:Optional
	L4Override *L4OverrideSettings `json:"l4Override,omitempty"`

	// BISOAdminControls for browser isolation.
	// +kubebuilder:validation:Optional
	BISOAdminControls *BISOAdminControls `json:"bisoAdminControls,omitempty"`

	// CheckSession enables session check.
	// +kubebuilder:validation:Optional
	CheckSession *SessionSettings `json:"checkSession,omitempty"`

	// AddHeaders adds headers to requests.
	// +kubebuilder:validation:Optional
	AddHeaders map[string]string `json:"addHeaders,omitempty"`

	// InsecureDisableDNSSECValidation disables DNSSEC validation.
	// +kubebuilder:validation:Optional
	InsecureDisableDNSSECValidation *bool `json:"insecureDisableDnssecValidation,omitempty"`

	// EgressSettings for egress action.
	// +kubebuilder:validation:Optional
	Egress *EgressSettings `json:"egress,omitempty"`

	// PayloadLog configures logging.
	// +kubebuilder:validation:Optional
	PayloadLog *PayloadLogSettings `json:"payloadLog,omitempty"`

	// UntrustedCertificateAction for TLS inspection.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=pass_through;block;error
	UntrustedCertificateAction string `json:"untrustedCertificateAction,omitempty"`

	// AuditSSH enables SSH command logging.
	// +kubebuilder:validation:Optional
	AuditSSH *AuditSSHSettings `json:"auditSsh,omitempty"`

	// ResolveDNSInternally for private DNS resolution.
	// +kubebuilder:validation:Optional
	ResolveDNSInternally *bool `json:"resolveDnsInternally,omitempty"`

	// DNSResolverIPv4 custom resolver.
	// +kubebuilder:validation:Optional
	DNSResolverIPv4 *DNSResolver `json:"dnsResolverIpv4,omitempty"`

	// DNSResolverIPv6 custom resolver.
	// +kubebuilder:validation:Optional
	DNSResolverIPv6 *DNSResolver `json:"dnsResolverIpv6,omitempty"`

	// NotificationSettings for alerts.
	// +kubebuilder:validation:Optional
	NotificationSettings *NotificationSettings `json:"notificationSettings,omitempty"`
}

// L4OverrideSettings for L4 override.
type L4OverrideSettings struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// BISOAdminControls for browser isolation.
type BISOAdminControls struct {
	DisablePrinting     *bool `json:"disablePrinting,omitempty"`
	DisableCopyPaste    *bool `json:"disableCopyPaste,omitempty"`
	DisableDownload     *bool `json:"disableDownload,omitempty"`
	DisableUpload       *bool `json:"disableUpload,omitempty"`
	DisableKeyboard     *bool `json:"disableKeyboard,omitempty"`
	DisableClipboardRedirection *bool `json:"disableClipboardRedirection,omitempty"`
}

// SessionSettings for session checks.
type SessionSettings struct {
	Enforce  bool   `json:"enforce"`
	Duration string `json:"duration"`
}

// EgressSettings for egress action.
type EgressSettings struct {
	IPv4      string `json:"ipv4,omitempty"`
	IPv6      string `json:"ipv6,omitempty"`
	IPv4Fallback string `json:"ipv4Fallback,omitempty"`
}

// PayloadLogSettings for logging.
type PayloadLogSettings struct {
	Enabled bool `json:"enabled"`
}

// AuditSSHSettings for SSH auditing.
type AuditSSHSettings struct {
	CommandLogging bool `json:"commandLogging"`
}

// DNSResolver for custom DNS.
type DNSResolver struct {
	IP                string `json:"ip,omitempty"`
	Port              int    `json:"port,omitempty"`
	VNetID            string `json:"vnetId,omitempty"`
	RouteThroughPrivateNetwork bool `json:"routeThroughPrivateNetwork,omitempty"`
}

// NotificationSettings for alerts.
type NotificationSettings struct {
	Enabled    bool   `json:"enabled"`
	Message    string `json:"message,omitempty"`
	SupportURL string `json:"supportUrl,omitempty"`
}

// GatewayRuleStatus defines the observed state
type GatewayRuleStatus struct {
	// RuleID is the Cloudflare Gateway Rule ID.
	// +kubebuilder:validation:Optional
	RuleID string `json:"ruleId,omitempty"`

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
// +kubebuilder:resource:scope=Cluster,shortName=gwrule
// +kubebuilder:printcolumn:name="Action",type=string,JSONPath=`.spec.action`
// +kubebuilder:printcolumn:name="Precedence",type=integer,JSONPath=`.spec.precedence`
// +kubebuilder:printcolumn:name="Enabled",type=boolean,JSONPath=`.spec.enabled`
// +kubebuilder:printcolumn:name="RuleID",type=string,JSONPath=`.status.ruleId`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// GatewayRule is the Schema for the gatewayrules API.
type GatewayRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatewayRuleSpec   `json:"spec,omitempty"`
	Status GatewayRuleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GatewayRuleList contains a list of GatewayRule
type GatewayRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GatewayRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GatewayRule{}, &GatewayRuleList{})
}

// GetGatewayRuleName returns the name to use in Cloudflare.
func (g *GatewayRule) GetGatewayRuleName() string {
	if g.Spec.Name != "" {
		return g.Spec.Name
	}
	return g.Name
}
