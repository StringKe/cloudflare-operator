// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package device provides services for managing Cloudflare Device configurations.
//
//nolint:revive // max-public-structs is acceptable for API type definitions
package device

import (
	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// Resource Types for SyncState
const (
	// ResourceTypeDevicePostureRule is the SyncState resource type for DevicePostureRule
	ResourceTypeDevicePostureRule = v1alpha2.SyncResourceDevicePostureRule
	// ResourceTypeDeviceSettingsPolicy is the SyncState resource type for DeviceSettingsPolicy
	ResourceTypeDeviceSettingsPolicy = v1alpha2.SyncResourceDeviceSettingsPolicy

	// Priority constants
	PriorityDevicePostureRule    = 100
	PriorityDeviceSettingsPolicy = 100
)

// DevicePostureRuleConfig contains the configuration for a Device Posture Rule.
type DevicePostureRuleConfig struct {
	// Name is the rule name
	Name string `json:"name"`
	// Type is the posture check type (e.g., file, serial_number, application)
	Type string `json:"type"`
	// Description is an optional description
	Description string `json:"description,omitempty"`
	// Schedule is the schedule for the posture check
	Schedule string `json:"schedule,omitempty"`
	// Expiration is the expiration time for the posture check
	Expiration string `json:"expiration,omitempty"`
	// Match contains the platform matchers
	Match []DevicePostureMatch `json:"match,omitempty"`
	// Input contains the posture check input parameters
	Input *DevicePostureInput `json:"input,omitempty"`
}

// DevicePostureMatch contains platform matcher configuration.
type DevicePostureMatch struct {
	// Platform is the platform to match (e.g., windows, mac, linux, ios, android)
	Platform string `json:"platform,omitempty"`
}

// DevicePostureInput contains the posture check input parameters.
type DevicePostureInput struct {
	// ID is a generic identifier field
	ID string `json:"id,omitempty"`
	// Path is the file or directory path
	Path string `json:"path,omitempty"`
	// Exists indicates if the file should exist
	Exists *bool `json:"exists,omitempty"`
	// Sha256 is the expected SHA256 hash
	Sha256 string `json:"sha256,omitempty"`
	// Thumbprint is the certificate thumbprint
	Thumbprint string `json:"thumbprint,omitempty"`
	// Running indicates if the process should be running
	Running *bool `json:"running,omitempty"`
	// RequireAll indicates if all conditions must be met
	RequireAll *bool `json:"requireAll,omitempty"`
	// Enabled indicates if the check is enabled
	Enabled *bool `json:"enabled,omitempty"`
	// Version is the version to check
	Version string `json:"version,omitempty"`
	// Operator is the comparison operator
	Operator string `json:"operator,omitempty"`
	// Domain is the domain to check
	Domain string `json:"domain,omitempty"`
	// ComplianceStatus is the compliance status
	ComplianceStatus string `json:"complianceStatus,omitempty"`
	// ConnectionID is the connection ID
	ConnectionID string `json:"connectionId,omitempty"`
	// LastSeen is the last seen time
	LastSeen string `json:"lastSeen,omitempty"`
	// EidLastSeen is the EID last seen time
	EidLastSeen string `json:"eidLastSeen,omitempty"`
	// ActiveThreats is the number of active threats
	ActiveThreats *int `json:"activeThreats,omitempty"`
	// Infected indicates if the device is infected
	Infected *bool `json:"infected,omitempty"`
	// IsActive indicates if the check is active
	IsActive *bool `json:"isActive,omitempty"`
	// NetworkStatus is the network status
	NetworkStatus string `json:"networkStatus,omitempty"`
	// SensorConfig is the sensor configuration
	SensorConfig string `json:"sensorConfig,omitempty"`
	// VersionOperator is the version comparison operator
	VersionOperator string `json:"versionOperator,omitempty"`
	// CountOperator is the count comparison operator
	CountOperator string `json:"countOperator,omitempty"`
	// ScoreOperator is the score comparison operator
	ScoreOperator string `json:"scoreOperator,omitempty"`
	// IssueCount is the number of issues
	IssueCount *int `json:"issueCount,omitempty"`
	// Score is the score
	Score *int `json:"score,omitempty"`
	// TotalScore is the total score
	TotalScore *int `json:"totalScore,omitempty"`
	// RiskLevel is the risk level
	RiskLevel string `json:"riskLevel,omitempty"`
	// Overall is the overall status
	Overall string `json:"overall,omitempty"`
	// State is the state
	State string `json:"state,omitempty"`
	// OperationalState is the operational state
	OperationalState string `json:"operationalState,omitempty"`
	// OSDistroName is the OS distribution name
	OSDistroName string `json:"osDistroName,omitempty"`
	// OSDistroRevision is the OS distribution revision
	OSDistroRevision string `json:"osDistroRevision,omitempty"`
	// OSVersionExtra is extra OS version info
	OSVersionExtra string `json:"osVersionExtra,omitempty"`
	// OS is the operating system
	OS string `json:"os,omitempty"`
	// OperatingSystem is the full operating system string
	OperatingSystem string `json:"operatingSystem,omitempty"`
	// CertificateID is the certificate ID
	CertificateID string `json:"certificateId,omitempty"`
	// CommonName is the certificate common name
	CommonName string `json:"commonName,omitempty"`
	// Cn is the certificate CN
	Cn string `json:"cn,omitempty"`
	// CheckPrivateKey indicates whether to check the private key
	CheckPrivateKey *bool `json:"checkPrivateKey,omitempty"`
	// ExtendedKeyUsage is the extended key usage
	ExtendedKeyUsage []string `json:"extendedKeyUsage,omitempty"`
	// CheckDisks contains disk paths to check (string list)
	CheckDisks []string `json:"checkDisks,omitempty"`
}

// DeviceSettingsPolicyConfig contains the configuration for Device Settings Policy.
type DeviceSettingsPolicyConfig struct {
	// SplitTunnelMode is the split tunnel mode (include or exclude)
	SplitTunnelMode string `json:"splitTunnelMode,omitempty"`
	// SplitTunnelExclude contains routes to exclude from tunnel
	SplitTunnelExclude []SplitTunnelEntry `json:"splitTunnelExclude,omitempty"`
	// SplitTunnelInclude contains routes to include in tunnel
	SplitTunnelInclude []SplitTunnelEntry `json:"splitTunnelInclude,omitempty"`
	// FallbackDomains contains fallback domain configurations
	FallbackDomains []FallbackDomainEntry `json:"fallbackDomains,omitempty"`
	// AutoPopulatedRoutes contains routes auto-populated from NetworkRoute resources
	AutoPopulatedRoutes []SplitTunnelEntry `json:"autoPopulatedRoutes,omitempty"`
}

// SplitTunnelEntry contains a split tunnel entry.
type SplitTunnelEntry struct {
	// Address is the IP address or CIDR
	Address string `json:"address,omitempty"`
	// Host is the hostname
	Host string `json:"host,omitempty"`
	// Description is the entry description
	Description string `json:"description,omitempty"`
}

// FallbackDomainEntry contains a fallback domain entry.
type FallbackDomainEntry struct {
	// Suffix is the domain suffix
	Suffix string `json:"suffix,omitempty"`
	// Description is the entry description
	Description string `json:"description,omitempty"`
	// DNSServer is the DNS server addresses
	DNSServer []string `json:"dnsServer,omitempty"`
}

// DevicePostureRuleRegisterOptions contains options for registering a DevicePostureRule.
type DevicePostureRuleRegisterOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// RuleID is the existing rule ID (empty for new)
	RuleID string
	// Source is the K8s resource source
	Source service.Source
	// Config is the device posture rule configuration
	Config DevicePostureRuleConfig
	// CredentialsRef references the CloudflareCredentials resource
	CredentialsRef v1alpha2.CredentialsReference
}

// DeviceSettingsPolicyRegisterOptions contains options for registering a DeviceSettingsPolicy.
type DeviceSettingsPolicyRegisterOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// Source is the K8s resource source
	Source service.Source
	// Config is the device settings policy configuration
	Config DeviceSettingsPolicyConfig
	// CredentialsRef references the CloudflareCredentials resource
	CredentialsRef v1alpha2.CredentialsReference
}

// DevicePostureRuleSyncResult contains DevicePostureRule-specific sync result.
type DevicePostureRuleSyncResult struct {
	// RuleID is the Cloudflare rule ID
	RuleID string
	// AccountID is the Cloudflare account ID
	AccountID string
}

// DeviceSettingsPolicySyncResult contains DeviceSettingsPolicy-specific sync result.
type DeviceSettingsPolicySyncResult struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// SplitTunnelExcludeCount is the number of exclude entries
	SplitTunnelExcludeCount int
	// SplitTunnelIncludeCount is the number of include entries
	SplitTunnelIncludeCount int
	// FallbackDomainsCount is the number of fallback domains
	FallbackDomainsCount int
	// AutoPopulatedRoutesCount is the number of auto-populated routes
	AutoPopulatedRoutesCount int
}
