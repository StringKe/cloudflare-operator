// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DevicePostureRuleSpec defines the desired state of DevicePostureRule
type DevicePostureRuleSpec struct {
	// Name of the Device Posture Rule in Cloudflare.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name,omitempty"`

	// Type is the posture rule type.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=file;application;serial_number;tanium;gateway;warp;disk_encryption;sentinelone;carbonblack;firewall;os_version;domain_joined;client_certificate;unique_client_id;kolide;tanium_s2s;crowdstrike_s2s;intune;workspace_one
	Type string `json:"type"`

	// Description is a human-readable description.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=1000
	Description string `json:"description,omitempty"`

	// Schedule determines how often the rule is evaluated.
	// +kubebuilder:validation:Optional
	Schedule string `json:"schedule,omitempty"`

	// Expiration is when the rule expires.
	// +kubebuilder:validation:Optional
	Expiration string `json:"expiration,omitempty"`

	// Match defines which devices this rule applies to.
	// +kubebuilder:validation:Optional
	Match []DevicePostureMatch `json:"match,omitempty"`

	// Input contains the rule-specific configuration.
	// +kubebuilder:validation:Optional
	Input *DevicePostureInput `json:"input,omitempty"`

	// Cloudflare contains the Cloudflare API credentials.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`
}

// DevicePostureMatch defines platform matching.
type DevicePostureMatch struct {
	// Platform is the OS platform.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=windows;mac;linux;android;ios;chromeos
	Platform string `json:"platform,omitempty"`
}

// DevicePostureInput contains rule-specific input.
type DevicePostureInput struct {
	// ID is a generic identifier for integrations.
	// +kubebuilder:validation:Optional
	ID string `json:"id,omitempty"`

	// Path is the file path to check.
	// +kubebuilder:validation:Optional
	Path string `json:"path,omitempty"`

	// Exists checks if file exists.
	// +kubebuilder:validation:Optional
	Exists *bool `json:"exists,omitempty"`

	// Sha256 is the expected file hash.
	// +kubebuilder:validation:Optional
	Sha256 string `json:"sha256,omitempty"`

	// Thumbprint is the certificate thumbprint.
	// +kubebuilder:validation:Optional
	Thumbprint string `json:"thumbprint,omitempty"`

	// Running checks if application is running.
	// +kubebuilder:validation:Optional
	Running *bool `json:"running,omitempty"`

	// RequireAll requires all conditions to match.
	// +kubebuilder:validation:Optional
	RequireAll *bool `json:"requireAll,omitempty"`

	// Enabled checks if feature is enabled.
	// +kubebuilder:validation:Optional
	Enabled *bool `json:"enabled,omitempty"`

	// Version is the minimum version.
	// +kubebuilder:validation:Optional
	Version string `json:"version,omitempty"`

	// Operator is the version comparison operator.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=<;<=;>;>=;==
	Operator string `json:"operator,omitempty"`

	// Domain is the expected domain for domain-joined checks.
	// +kubebuilder:validation:Optional
	Domain string `json:"domain,omitempty"`

	// ComplianceStatus is the Intune compliance status.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=compliant;noncompliant;unknown;notapplicable;ingraceperiod;error
	ComplianceStatus string `json:"complianceStatus,omitempty"`

	// ConnectionID is the third-party integration connection ID.
	// +kubebuilder:validation:Optional
	ConnectionID string `json:"connectionId,omitempty"`

	// LastSeen is the maximum time since device was last seen.
	// +kubebuilder:validation:Optional
	LastSeen string `json:"lastSeen,omitempty"`

	// ActiveThreats is the maximum active threat count.
	// +kubebuilder:validation:Optional
	ActiveThreats *int `json:"activeThreats,omitempty"`

	// NetworkStatus checks for network connection.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=connected;disconnected;disconnecting;connecting
	NetworkStatus string `json:"networkStatus,omitempty"`

	// SensorConfig checks sensor configuration.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=active;disabled;not_configured
	SensorConfig string `json:"sensorConfig,omitempty"`

	// VersionOperator for CrowdStrike version checks.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=<;<=;>;>=;==
	VersionOperator string `json:"versionOperator,omitempty"`

	// CountOperator for count comparisons.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=<;<=;>;>=;==
	CountOperator string `json:"countOperator,omitempty"`

	// IssueCount is the number of issues for SentinelOne.
	// +kubebuilder:validation:Optional
	IssueCount *int `json:"issueCount,omitempty"`

	// OSDistroName is the OS distribution name.
	// +kubebuilder:validation:Optional
	OSDistroName string `json:"osDistroName,omitempty"`

	// OSDistroRevision is the OS distribution revision.
	// +kubebuilder:validation:Optional
	OSDistroRevision string `json:"osDistroRevision,omitempty"`

	// CertificateID for client certificate checks.
	// +kubebuilder:validation:Optional
	CertificateID string `json:"certificateId,omitempty"`

	// CommonName for client certificate checks.
	// +kubebuilder:validation:Optional
	CommonName string `json:"commonName,omitempty"`

	// CheckDisks specifies which disks to check encryption.
	// +kubebuilder:validation:Optional
	CheckDisks []string `json:"checkDisks,omitempty"`
}

// DevicePostureRuleStatus defines the observed state
type DevicePostureRuleStatus struct {
	// RuleID is the Cloudflare Device Posture Rule ID.
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
// +kubebuilder:resource:scope=Cluster,shortName=posture
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="RuleID",type=string,JSONPath=`.status.ruleId`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DevicePostureRule is the Schema for the deviceposturerules API.
type DevicePostureRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DevicePostureRuleSpec   `json:"spec,omitempty"`
	Status DevicePostureRuleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DevicePostureRuleList contains a list of DevicePostureRule
type DevicePostureRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DevicePostureRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DevicePostureRule{}, &DevicePostureRuleList{})
}

// GetRuleName returns the name to use in Cloudflare.
func (d *DevicePostureRule) GetRuleName() string {
	if d.Spec.Name != "" {
		return d.Spec.Name
	}
	return d.Name
}
