// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package gateway provides services for managing Cloudflare Gateway configurations.
//
//nolint:revive // max-public-structs is acceptable for API type definitions
package gateway

import (
	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// Resource Types for SyncState
const (
	// ResourceTypeGatewayRule is the SyncState resource type for GatewayRule
	ResourceTypeGatewayRule = v1alpha2.SyncResourceGatewayRule
	// ResourceTypeGatewayList is the SyncState resource type for GatewayList
	ResourceTypeGatewayList = v1alpha2.SyncResourceGatewayList
	// ResourceTypeGatewayConfiguration is the SyncState resource type for GatewayConfiguration
	ResourceTypeGatewayConfiguration = v1alpha2.SyncResourceGatewayConfiguration

	// Priority constants
	PriorityGatewayRule          = 100
	PriorityGatewayList          = 100
	PriorityGatewayConfiguration = 100
)

// GatewayRuleConfig contains the configuration for a Gateway rule.
type GatewayRuleConfig struct {
	// Name is the rule name
	Name string `json:"name"`
	// Description is an optional description
	Description string `json:"description,omitempty"`
	// Filters is the list of filter configurations
	Filters []GatewayRuleFilter `json:"filters,omitempty"`
	// TrafficType determines the traffic type (http, l4, dns)
	TrafficType string `json:"trafficType,omitempty"`
	// Action is the rule action
	Action string `json:"action,omitempty"`
	// RuleSettings contains additional rule settings
	RuleSettings *GatewayRuleSettings `json:"ruleSettings,omitempty"`
	// Priority is the rule priority
	Priority int `json:"priority,omitempty"`
	// Enabled indicates if the rule is enabled
	Enabled bool `json:"enabled"`
}

// GatewayRuleFilter contains filter configuration.
type GatewayRuleFilter struct {
	// Type is the filter type (e.g., http, l4, dns)
	Type string `json:"type,omitempty"`
	// Expression is the filter expression
	Expression string `json:"expression,omitempty"`
}

// GatewayRuleSettings contains additional rule settings.
type GatewayRuleSettings struct {
	// BlockPageEnabled enables the block page
	BlockPageEnabled *bool `json:"blockPageEnabled,omitempty"`
	// BlockReason is the reason shown on the block page
	BlockReason string `json:"blockReason,omitempty"`
	// OverrideHost is the host to override
	OverrideHost string `json:"overrideHost,omitempty"`
	// OverrideIPs are the IPs to override
	OverrideIPs []string `json:"overrideIPs,omitempty"`
	// InsecureDisableDNSSECValidation disables DNSSEC validation
	InsecureDisableDNSSECValidation *bool `json:"insecureDisableDnssecValidation,omitempty"`
	// AddHeaders are headers to add
	AddHeaders map[string]string `json:"addHeaders,omitempty"`
	// BISOAdminControls contains browser isolation admin controls
	BISOAdminControls *BISOAdminControls `json:"bisoAdminControls,omitempty"`
	// CheckSession contains session check settings
	CheckSession *CheckSessionSettings `json:"checkSession,omitempty"`
	// L4Override contains L4 override settings
	L4Override *L4OverrideSettings `json:"l4Override,omitempty"`
	// NotificationSettings contains notification settings
	NotificationSettings *NotificationSettings `json:"notificationSettings,omitempty"`
	// PayloadLog contains payload log settings
	PayloadLog *PayloadLogSettings `json:"payloadLog,omitempty"`
	// AuditSSH contains SSH audit settings
	AuditSSH *AuditSSHSettings `json:"auditSsh,omitempty"`
	// Untrusted certificate settings
	UntrustedCert *UntrustedCertSettings `json:"untrustedCert,omitempty"`
	// Egress settings
	Egress *EgressSettings `json:"egress,omitempty"`
	// DNS resolvers
	DNSResolvers *DNSResolverSettings `json:"dnsResolvers,omitempty"`
}

// BISOAdminControls contains browser isolation admin controls.
type BISOAdminControls struct {
	DisablePrinting          *bool `json:"disablePrinting,omitempty"`
	DisableCopyPaste         *bool `json:"disableCopyPaste,omitempty"`
	DisableDownload          *bool `json:"disableDownload,omitempty"`
	DisableUpload            *bool `json:"disableUpload,omitempty"`
	DisableKeyboard          *bool `json:"disableKeyboard,omitempty"`
	DisableClipboardRedirect *bool `json:"disableClipboardRedirect,omitempty"`
}

// CheckSessionSettings contains session check settings.
type CheckSessionSettings struct {
	Enforce  bool   `json:"enforce,omitempty"`
	Duration string `json:"duration,omitempty"`
}

// L4OverrideSettings contains L4 override settings.
type L4OverrideSettings struct {
	IP   string `json:"ip,omitempty"`
	Port int    `json:"port,omitempty"`
}

// NotificationSettings contains notification settings.
type NotificationSettings struct {
	Enabled    bool   `json:"enabled,omitempty"`
	Message    string `json:"message,omitempty"`
	SupportURL string `json:"supportUrl,omitempty"`
}

// PayloadLogSettings contains payload log settings.
type PayloadLogSettings struct {
	Enabled bool `json:"enabled,omitempty"`
}

// AuditSSHSettings contains SSH audit settings.
type AuditSSHSettings struct {
	CommandLogging bool `json:"commandLogging,omitempty"`
}

// UntrustedCertSettings contains untrusted certificate settings.
type UntrustedCertSettings struct {
	Action string `json:"action,omitempty"`
}

// EgressSettings contains egress settings.
type EgressSettings struct {
	Ipv4         string `json:"ipv4,omitempty"`
	Ipv6         string `json:"ipv6,omitempty"`
	Ipv4Fallback string `json:"ipv4Fallback,omitempty"`
}

// DNSResolverSettings contains DNS resolver settings.
type DNSResolverSettings struct {
	Ipv4 []DNSResolverAddress `json:"ipv4,omitempty"`
	Ipv6 []DNSResolverAddress `json:"ipv6,omitempty"`
}

// DNSResolverAddress contains a DNS resolver address.
type DNSResolverAddress struct {
	IP   string `json:"ip,omitempty"`
	Port int    `json:"port,omitempty"`
}

// GatewayListConfig contains the configuration for a Gateway list.
type GatewayListConfig struct {
	// Name is the list name
	Name string `json:"name"`
	// Description is an optional description
	Description string `json:"description,omitempty"`
	// Type is the list type (SERIAL, URL, DOMAIN, EMAIL, IP)
	Type string `json:"type"`
	// Items is the list of items
	Items []string `json:"items,omitempty"`
}

// GatewayConfigurationConfig contains the configuration for Gateway settings.
type GatewayConfigurationConfig struct {
	// TLSDecrypt contains TLS decryption settings
	TLSDecrypt *TLSDecryptSettings `json:"tlsDecrypt,omitempty"`
	// ActivityLog contains activity logging settings
	ActivityLog *ActivityLogSettings `json:"activityLog,omitempty"`
	// AntiVirus contains antivirus settings
	AntiVirus *AntiVirusSettings `json:"antiVirus,omitempty"`
	// BlockPage contains block page settings
	BlockPage *BlockPageSettings `json:"blockPage,omitempty"`
	// BodyScanning contains body scanning settings
	BodyScanning *BodyScanningSettings `json:"bodyScanning,omitempty"`
	// BrowserIsolation contains browser isolation settings
	BrowserIsolation *BrowserIsolationSettings `json:"browserIsolation,omitempty"`
	// FIPS contains FIPS settings
	FIPS *FIPSSettings `json:"fips,omitempty"`
	// ProtocolDetection contains protocol detection settings
	ProtocolDetection *ProtocolDetectionSettings `json:"protocolDetection,omitempty"`
	// CustomCertificate contains custom certificate settings
	CustomCertificate *CustomCertificateSettings `json:"customCertificate,omitempty"`
}

// TLSDecryptSettings contains TLS decryption settings.
type TLSDecryptSettings struct {
	Enabled bool `json:"enabled,omitempty"`
}

// ActivityLogSettings contains activity logging settings.
type ActivityLogSettings struct {
	Enabled bool `json:"enabled,omitempty"`
}

// AntiVirusSettings contains antivirus settings.
type AntiVirusSettings struct {
	EnabledDownloadPhase bool                  `json:"enabledDownloadPhase,omitempty"`
	EnabledUploadPhase   bool                  `json:"enabledUploadPhase,omitempty"`
	FailClosed           bool                  `json:"failClosed,omitempty"`
	NotificationSettings *NotificationSettings `json:"notificationSettings,omitempty"`
}

// BlockPageSettings contains block page settings.
type BlockPageSettings struct {
	Enabled         bool   `json:"enabled,omitempty"`
	FooterText      string `json:"footerText,omitempty"`
	HeaderText      string `json:"headerText,omitempty"`
	LogoPath        string `json:"logoPath,omitempty"`
	BackgroundColor string `json:"backgroundColor,omitempty"`
}

// BodyScanningSettings contains body scanning settings.
type BodyScanningSettings struct {
	InspectionMode string `json:"inspectionMode,omitempty"`
}

// BrowserIsolationSettings contains browser isolation settings.
type BrowserIsolationSettings struct {
	URLBrowserIsolationEnabled bool `json:"urlBrowserIsolationEnabled,omitempty"`
	NonIdentityEnabled         bool `json:"nonIdentityEnabled,omitempty"`
}

// FIPSSettings contains FIPS settings.
type FIPSSettings struct {
	TLS bool `json:"tls,omitempty"`
}

// ProtocolDetectionSettings contains protocol detection settings.
type ProtocolDetectionSettings struct {
	Enabled bool `json:"enabled,omitempty"`
}

// CustomCertificateSettings contains custom certificate settings.
type CustomCertificateSettings struct {
	Enabled bool   `json:"enabled,omitempty"`
	ID      string `json:"id,omitempty"`
}

// GatewayRuleRegisterOptions contains options for registering a GatewayRule.
type GatewayRuleRegisterOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// RuleID is the existing rule ID (empty for new)
	RuleID string
	// Source is the K8s resource source
	Source service.Source
	// Config is the gateway rule configuration
	Config GatewayRuleConfig
	// CredentialsRef references the CloudflareCredentials resource
	CredentialsRef v1alpha2.CredentialsReference
}

// GatewayListRegisterOptions contains options for registering a GatewayList.
type GatewayListRegisterOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// ListID is the existing list ID (empty for new)
	ListID string
	// Source is the K8s resource source
	Source service.Source
	// Config is the gateway list configuration
	Config GatewayListConfig
	// CredentialsRef references the CloudflareCredentials resource
	CredentialsRef v1alpha2.CredentialsReference
}

// GatewayConfigurationRegisterOptions contains options for registering a GatewayConfiguration.
type GatewayConfigurationRegisterOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// Source is the K8s resource source
	Source service.Source
	// Config is the gateway configuration
	Config GatewayConfigurationConfig
	// CredentialsRef references the CloudflareCredentials resource
	CredentialsRef v1alpha2.CredentialsReference
}

// GatewayRuleSyncResult contains GatewayRule-specific sync result.
type GatewayRuleSyncResult struct {
	// RuleID is the Cloudflare rule ID
	RuleID string
	// AccountID is the Cloudflare account ID
	AccountID string
}

// GatewayListSyncResult contains GatewayList-specific sync result.
type GatewayListSyncResult struct {
	// ListID is the Cloudflare list ID
	ListID string
	// AccountID is the Cloudflare account ID
	AccountID string
	// ItemCount is the number of items in the list
	ItemCount int
}

// GatewayConfigurationSyncResult contains GatewayConfiguration-specific sync result.
type GatewayConfigurationSyncResult struct {
	// AccountID is the Cloudflare account ID
	AccountID string
}
