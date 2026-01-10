// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GatewayConfigurationSpec defines the desired state of GatewayConfiguration
type GatewayConfigurationSpec struct {
	// Settings contains the Gateway configuration settings.
	// +kubebuilder:validation:Required
	Settings GatewaySettings `json:"settings"`

	// Cloudflare contains the Cloudflare API credentials.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`
}

// GatewaySettings contains Gateway configuration.
type GatewaySettings struct {
	// TLSDecrypt enables TLS decryption.
	// +kubebuilder:validation:Optional
	TLSDecrypt *TLSDecryptSettings `json:"tlsDecrypt,omitempty"`

	// ActivityLog configures activity logging.
	// +kubebuilder:validation:Optional
	ActivityLog *ActivityLogSettings `json:"activityLog,omitempty"`

	// AntiVirus configures AV scanning.
	// +kubebuilder:validation:Optional
	AntiVirus *AntiVirusSettings `json:"antiVirus,omitempty"`

	// BlockPage configures the block page.
	// +kubebuilder:validation:Optional
	BlockPage *BlockPageSettings `json:"blockPage,omitempty"`

	// BodyScanning configures body scanning.
	// +kubebuilder:validation:Optional
	BodyScanning *BodyScanningSettings `json:"bodyScanning,omitempty"`

	// BrowserIsolation configures browser isolation.
	// +kubebuilder:validation:Optional
	BrowserIsolation *BrowserIsolationSettings `json:"browserIsolation,omitempty"`

	// FIPS enables FIPS mode.
	// +kubebuilder:validation:Optional
	FIPS *FIPSSettings `json:"fips,omitempty"`

	// ProtocolDetection enables protocol detection.
	// +kubebuilder:validation:Optional
	ProtocolDetection *ProtocolDetectionSettings `json:"protocolDetection,omitempty"`

	// CustomCertificate configures custom root CA.
	// +kubebuilder:validation:Optional
	CustomCertificate *CustomCertificateSettings `json:"customCertificate,omitempty"`

	// NonIdentityBrowserIsolation configures non-identity isolation.
	// +kubebuilder:validation:Optional
	NonIdentityBrowserIsolation *NonIdentityBrowserIsolationSettings `json:"nonIdentityBrowserIsolation,omitempty"`
}

// TLSDecryptSettings for TLS decryption.
type TLSDecryptSettings struct {
	Enabled bool `json:"enabled"`
}

// ActivityLogSettings for activity logging.
type ActivityLogSettings struct {
	Enabled bool `json:"enabled"`
}

// AntiVirusSettings for AV scanning.
type AntiVirusSettings struct {
	Enabled              bool                  `json:"enabled"`
	EnabledDownloadPhase bool                  `json:"enabledDownloadPhase,omitempty"`
	EnabledUploadPhase   bool                  `json:"enabledUploadPhase,omitempty"`
	FailClosed           bool                  `json:"failClosed,omitempty"`
	NotificationSettings *NotificationSettings `json:"notificationSettings,omitempty"`
}

// BlockPageSettings for block page customization.
type BlockPageSettings struct {
	Enabled         bool   `json:"enabled"`
	Name            string `json:"name,omitempty"`
	FooterText      string `json:"footerText,omitempty"`
	HeaderText      string `json:"headerText,omitempty"`
	LogoPath        string `json:"logoPath,omitempty"`
	BackgroundColor string `json:"backgroundColor,omitempty"`
	MailtoAddress   string `json:"mailtoAddress,omitempty"`
	MailtoSubject   string `json:"mailtoSubject,omitempty"`
	SuppressFooter  bool   `json:"suppressFooter,omitempty"`
}

// BodyScanningSettings for body scanning.
type BodyScanningSettings struct {
	InspectionMode string `json:"inspectionMode,omitempty"` // deep, shallow
}

// BrowserIsolationSettings for browser isolation.
type BrowserIsolationSettings struct {
	URLBrowserIsolationEnabled bool `json:"urlBrowserIsolationEnabled,omitempty"`
	NonIdentityEnabled         bool `json:"nonIdentityEnabled,omitempty"`
}

// FIPSSettings for FIPS compliance.
type FIPSSettings struct {
	TLS bool `json:"tls,omitempty"`
}

// ProtocolDetectionSettings for protocol detection.
type ProtocolDetectionSettings struct {
	Enabled bool `json:"enabled"`
}

// CustomCertificateSettings for custom CA.
type CustomCertificateSettings struct {
	Enabled bool   `json:"enabled"`
	ID      string `json:"id,omitempty"`
}

// NonIdentityBrowserIsolationSettings for non-identity isolation.
type NonIdentityBrowserIsolationSettings struct {
	Enabled bool `json:"enabled"`
}

// GatewayConfigurationStatus defines the observed state
type GatewayConfigurationStatus struct {
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
// +kubebuilder:resource:scope=Cluster,shortName=gwconfig
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// GatewayConfiguration is the Schema for the gatewayconfigurations API.
type GatewayConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatewayConfigurationSpec   `json:"spec,omitempty"`
	Status GatewayConfigurationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GatewayConfigurationList contains a list of GatewayConfiguration
type GatewayConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GatewayConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GatewayConfiguration{}, &GatewayConfigurationList{})
}
