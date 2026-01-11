// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

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
	// +kubebuilder:validation:Enum=on;off;allow;block;scan;noscan;safesearch;ytrestricted;isolate;noisolate;override;l4_override;egress;resolve;quarantine
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

	// Schedule defines when the rule is active.
	// +kubebuilder:validation:Optional
	Schedule *GatewayRuleSchedule `json:"schedule,omitempty"`

	// Expiration defines when the rule expires (for DNS policies).
	// +kubebuilder:validation:Optional
	Expiration *GatewayRuleExpiration `json:"expiration,omitempty"`

	// Cloudflare contains the Cloudflare API credentials.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`
}

// GatewayRuleSchedule defines when a rule is active.
type GatewayRuleSchedule struct {
	// TimeZone is the time zone for the schedule (e.g., "America/New_York").
	// +kubebuilder:validation:Optional
	TimeZone string `json:"timeZone,omitempty"`

	// Mon is the schedule for Monday (e.g., "09:00-17:00").
	// +kubebuilder:validation:Optional
	Mon string `json:"mon,omitempty"`

	// Tue is the schedule for Tuesday.
	// +kubebuilder:validation:Optional
	Tue string `json:"tue,omitempty"`

	// Wed is the schedule for Wednesday.
	// +kubebuilder:validation:Optional
	Wed string `json:"wed,omitempty"`

	// Thu is the schedule for Thursday.
	// +kubebuilder:validation:Optional
	Thu string `json:"thu,omitempty"`

	// Fri is the schedule for Friday.
	// +kubebuilder:validation:Optional
	Fri string `json:"fri,omitempty"`

	// Sat is the schedule for Saturday.
	// +kubebuilder:validation:Optional
	Sat string `json:"sat,omitempty"`

	// Sun is the schedule for Sunday.
	// +kubebuilder:validation:Optional
	Sun string `json:"sun,omitempty"`
}

// GatewayRuleExpiration defines when a DNS rule expires.
type GatewayRuleExpiration struct {
	// ExpiresAt is when the rule expires (RFC3339 format).
	// +kubebuilder:validation:Optional
	ExpiresAt string `json:"expiresAt,omitempty"`

	// Duration is the default expiration duration (e.g., "1h", "24h").
	// +kubebuilder:validation:Optional
	Duration string `json:"duration,omitempty"`
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

	// ResolveDNSInternally enables internal DNS resolution with view_id.
	// +kubebuilder:validation:Optional
	ResolveDNSInternally *ResolveDNSInternallySettings `json:"resolveDnsInternally,omitempty"`

	// ResolveDNSThroughCloudflare sends DNS to 1.1.1.1.
	// +kubebuilder:validation:Optional
	ResolveDNSThroughCloudflare *bool `json:"resolveDnsThroughCloudflare,omitempty"`

	// DNSResolvers contains custom DNS resolver settings.
	// +kubebuilder:validation:Optional
	DNSResolvers *DNSResolversSettings `json:"dnsResolvers,omitempty"`

	// NotificationSettings for alerts.
	// +kubebuilder:validation:Optional
	NotificationSettings *NotificationSettings `json:"notificationSettings,omitempty"`

	// AllowChildBypass allows child MSP accounts to bypass.
	// +kubebuilder:validation:Optional
	AllowChildBypass *bool `json:"allowChildBypass,omitempty"`

	// BypassParentRule allows bypassing parent MSP rules.
	// +kubebuilder:validation:Optional
	BypassParentRule *bool `json:"bypassParentRule,omitempty"`

	// IgnoreCNAMECategoryMatches ignores category at CNAME domains.
	// +kubebuilder:validation:Optional
	IgnoreCNAMECategoryMatches *bool `json:"ignoreCnameCategoryMatches,omitempty"`

	// IPCategories enables IPs in DNS resolver category blocks.
	// +kubebuilder:validation:Optional
	IPCategories *bool `json:"ipCategories,omitempty"`

	// IPIndicatorFeeds includes IPs in indicator feed blocks.
	// +kubebuilder:validation:Optional
	IPIndicatorFeeds *bool `json:"ipIndicatorFeeds,omitempty"`

	// Quarantine settings for quarantine action.
	// +kubebuilder:validation:Optional
	Quarantine *QuarantineSettings `json:"quarantine,omitempty"`
}

// L4OverrideSettings for L4 override.
type L4OverrideSettings struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// BISOAdminControls for browser isolation.
type BISOAdminControls struct {
	DisablePrinting             *bool `json:"disablePrinting,omitempty"`
	DisableCopyPaste            *bool `json:"disableCopyPaste,omitempty"`
	DisableDownload             *bool `json:"disableDownload,omitempty"`
	DisableUpload               *bool `json:"disableUpload,omitempty"`
	DisableKeyboard             *bool `json:"disableKeyboard,omitempty"`
	DisableClipboardRedirection *bool `json:"disableClipboardRedirection,omitempty"`
}

// SessionSettings for session checks.
type SessionSettings struct {
	Enforce  bool   `json:"enforce"`
	Duration string `json:"duration"`
}

// EgressSettings for egress action.
type EgressSettings struct {
	IPv4         string `json:"ipv4,omitempty"`
	IPv6         string `json:"ipv6,omitempty"`
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

// DNSResolversSettings contains IPv4 and IPv6 DNS resolvers.
type DNSResolversSettings struct {
	// IPv4 resolvers.
	// +kubebuilder:validation:Optional
	IPv4 []DNSResolverEntry `json:"ipv4,omitempty"`

	// IPv6 resolvers.
	// +kubebuilder:validation:Optional
	IPv6 []DNSResolverEntry `json:"ipv6,omitempty"`
}

// DNSResolverEntry for custom DNS resolver.
type DNSResolverEntry struct {
	// IP is the resolver IP address.
	IP string `json:"ip"`

	// Port is the resolver port.
	// +kubebuilder:validation:Optional
	Port int `json:"port,omitempty"`

	// VNetID is the virtual network ID.
	// +kubebuilder:validation:Optional
	VNetID string `json:"vnetId,omitempty"`

	// RouteThroughPrivateNetwork routes through private network.
	// +kubebuilder:validation:Optional
	RouteThroughPrivateNetwork *bool `json:"routeThroughPrivateNetwork,omitempty"`
}

// ResolveDNSInternallySettings for internal DNS resolution.
type ResolveDNSInternallySettings struct {
	// ViewID is the DNS view ID for internal resolution.
	// +kubebuilder:validation:Optional
	ViewID string `json:"viewId,omitempty"`

	// Fallback determines behavior when internal resolution fails.
	// +kubebuilder:validation:Optional
	Fallback *bool `json:"fallback,omitempty"`
}

// QuarantineSettings for quarantine action.
type QuarantineSettings struct {
	// FileTypes to quarantine.
	// +kubebuilder:validation:Optional
	FileTypes []string `json:"fileTypes,omitempty"`
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
