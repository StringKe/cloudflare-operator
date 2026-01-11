// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package deviceposturerule

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	cf "github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

func TestBuildInput(t *testing.T) {
	reconciler := &DevicePostureRuleReconciler{}

	tests := []struct {
		name     string
		input    *networkingv1alpha2.DevicePostureInput
		wantNil  bool
		validate func(t *testing.T, result *cf.DevicePostureInputParams)
	}{
		{
			name:    "nil input returns nil",
			input:   nil,
			wantNil: true,
		},
		{
			name: "disk encryption check",
			input: &networkingv1alpha2.DevicePostureInput{
				RequireAll: boolPtr(true),
				Enabled:    boolPtr(true),
			},
			validate: func(t *testing.T, result *cf.DevicePostureInputParams) {
				require.NotNil(t, result)
				require.NotNil(t, result.RequireAll)
				assert.True(t, *result.RequireAll)
				require.NotNil(t, result.Enabled)
				assert.True(t, *result.Enabled)
			},
		},
		{
			name: "file check",
			input: &networkingv1alpha2.DevicePostureInput{
				Path:   "C:\\Program Files\\MyApp\\app.exe",
				Exists: boolPtr(true),
				Sha256: "abc123def456",
			},
			validate: func(t *testing.T, result *cf.DevicePostureInputParams) {
				require.NotNil(t, result)
				assert.Equal(t, "C:\\Program Files\\MyApp\\app.exe", result.Path)
				require.NotNil(t, result.Exists)
				assert.True(t, *result.Exists)
				assert.Equal(t, "abc123def456", result.Sha256)
			},
		},
		{
			name: "application check",
			input: &networkingv1alpha2.DevicePostureInput{
				Path:    "/Applications/MyApp.app",
				Running: boolPtr(true),
			},
			validate: func(t *testing.T, result *cf.DevicePostureInputParams) {
				require.NotNil(t, result)
				assert.Equal(t, "/Applications/MyApp.app", result.Path)
				require.NotNil(t, result.Running)
				assert.True(t, *result.Running)
			},
		},
		{
			name: "domain joined check",
			input: &networkingv1alpha2.DevicePostureInput{
				Domain: "example.com",
			},
			validate: func(t *testing.T, result *cf.DevicePostureInputParams) {
				require.NotNil(t, result)
				assert.Equal(t, "example.com", result.Domain)
			},
		},
		{
			name: "OS version check",
			input: &networkingv1alpha2.DevicePostureInput{
				OS:               "Windows",
				Version:          "10.0.19041",
				Operator:         ">=",
				VersionOperator:  ">=",
				OSDistroName:     "Windows 10",
				OSDistroRevision: "19041",
				OSVersionExtra:   "Enterprise",
				OperatingSystem:  "windows",
			},
			validate: func(t *testing.T, result *cf.DevicePostureInputParams) {
				require.NotNil(t, result)
				assert.Equal(t, "Windows", result.OS)
				assert.Equal(t, "10.0.19041", result.Version)
				assert.Equal(t, ">=", result.Operator)
				assert.Equal(t, ">=", result.VersionOperator)
				assert.Equal(t, "Windows 10", result.OSDistroName)
				assert.Equal(t, "19041", result.OSDistroRevision)
				assert.Equal(t, "Enterprise", result.OSVersionExtra)
				assert.Equal(t, "windows", result.OperatingSystem)
			},
		},
		{
			name: "firewall check",
			input: &networkingv1alpha2.DevicePostureInput{
				Enabled: boolPtr(true),
			},
			validate: func(t *testing.T, result *cf.DevicePostureInputParams) {
				require.NotNil(t, result)
				require.NotNil(t, result.Enabled)
				assert.True(t, *result.Enabled)
			},
		},
		{
			name: "sentinel one check",
			input: &networkingv1alpha2.DevicePostureInput{
				ActiveThreats:    intPtr(0),
				NetworkStatus:    "connected",
				SensorConfig:     "default",
				Infected:         boolPtr(false),
				IsActive:         boolPtr(true),
				OperationalState: "running",
			},
			validate: func(t *testing.T, result *cf.DevicePostureInputParams) {
				require.NotNil(t, result)
				require.NotNil(t, result.ActiveThreats)
				assert.Equal(t, 0, *result.ActiveThreats)
				assert.Equal(t, "connected", result.NetworkStatus)
				assert.Equal(t, "default", result.SensorConfig)
				require.NotNil(t, result.Infected)
				assert.False(t, *result.Infected)
				require.NotNil(t, result.IsActive)
				assert.True(t, *result.IsActive)
				assert.Equal(t, "running", result.OperationalState)
			},
		},
		{
			name: "certificate check",
			input: &networkingv1alpha2.DevicePostureInput{
				CertificateID:    "cert-123",
				CommonName:       "*.example.com",
				Cn:               "CN=test",
				CheckPrivateKey:  boolPtr(true),
				ExtendedKeyUsage: []string{"serverAuth", "clientAuth"},
			},
			validate: func(t *testing.T, result *cf.DevicePostureInputParams) {
				require.NotNil(t, result)
				assert.Equal(t, "cert-123", result.CertificateID)
				assert.Equal(t, "*.example.com", result.CommonName)
				assert.Equal(t, "CN=test", result.Cn)
				require.NotNil(t, result.CheckPrivateKey)
				assert.True(t, *result.CheckPrivateKey)
				assert.Equal(t, []string{"serverAuth", "clientAuth"}, result.ExtendedKeyUsage)
			},
		},
		{
			name: "tanium check",
			input: &networkingv1alpha2.DevicePostureInput{
				EidLastSeen:   "2025-01-01T00:00:00Z",
				RiskLevel:     "low",
				TotalScore:    intPtr(100),
				ScoreOperator: ">=",
			},
			validate: func(t *testing.T, result *cf.DevicePostureInputParams) {
				require.NotNil(t, result)
				assert.Equal(t, "2025-01-01T00:00:00Z", result.EidLastSeen)
				assert.Equal(t, "low", result.RiskLevel)
				require.NotNil(t, result.TotalScore)
				assert.Equal(t, 100, *result.TotalScore)
				assert.Equal(t, ">=", result.ScoreOperator)
			},
		},
		{
			name: "intune check",
			input: &networkingv1alpha2.DevicePostureInput{
				ComplianceStatus: "compliant",
				ConnectionID:     "conn-456",
			},
			validate: func(t *testing.T, result *cf.DevicePostureInputParams) {
				require.NotNil(t, result)
				assert.Equal(t, "compliant", result.ComplianceStatus)
				assert.Equal(t, "conn-456", result.ConnectionID)
			},
		},
		{
			name: "crowdstrike check",
			input: &networkingv1alpha2.DevicePostureInput{
				State:         "online",
				Overall:       "pass",
				Score:         intPtr(85),
				ScoreOperator: ">=",
				IssueCount:    intPtr(0),
				CountOperator: "<=",
				LastSeen:      "2025-01-10T12:00:00Z",
			},
			validate: func(t *testing.T, result *cf.DevicePostureInputParams) {
				require.NotNil(t, result)
				assert.Equal(t, "online", result.State)
				assert.Equal(t, "pass", result.Overall)
				require.NotNil(t, result.Score)
				assert.Equal(t, 85, *result.Score)
				assert.Equal(t, ">=", result.ScoreOperator)
				require.NotNil(t, result.IssueCount)
				assert.Equal(t, 0, *result.IssueCount)
				assert.Equal(t, "<=", result.CountOperator)
				assert.Equal(t, "2025-01-10T12:00:00Z", result.LastSeen)
			},
		},
		{
			name: "kolide check",
			input: &networkingv1alpha2.DevicePostureInput{
				IssueCount:    intPtr(5),
				CountOperator: "<",
			},
			validate: func(t *testing.T, result *cf.DevicePostureInputParams) {
				require.NotNil(t, result)
				require.NotNil(t, result.IssueCount)
				assert.Equal(t, 5, *result.IssueCount)
				assert.Equal(t, "<", result.CountOperator)
			},
		},
		{
			name: "thumbprint check",
			input: &networkingv1alpha2.DevicePostureInput{
				Thumbprint: "sha256fingerprint",
			},
			validate: func(t *testing.T, result *cf.DevicePostureInputParams) {
				require.NotNil(t, result)
				assert.Equal(t, "sha256fingerprint", result.Thumbprint)
			},
		},
		{
			name: "check disks",
			input: &networkingv1alpha2.DevicePostureInput{
				CheckDisks: []string{"C:", "D:"},
			},
			validate: func(t *testing.T, result *cf.DevicePostureInputParams) {
				require.NotNil(t, result)
				assert.Equal(t, []string{"C:", "D:"}, result.CheckDisks)
			},
		},
		{
			name: "ID check",
			input: &networkingv1alpha2.DevicePostureInput{
				ID: "custom-check-id",
			},
			validate: func(t *testing.T, result *cf.DevicePostureInputParams) {
				require.NotNil(t, result)
				assert.Equal(t, "custom-check-id", result.ID)
			},
		},
		{
			name:  "empty input struct",
			input: &networkingv1alpha2.DevicePostureInput{},
			validate: func(t *testing.T, result *cf.DevicePostureInputParams) {
				require.NotNil(t, result)
				// All fields should be zero/nil values
				assert.Empty(t, result.Path)
				assert.Nil(t, result.Exists)
				assert.Empty(t, result.Domain)
			},
		},
		{
			name: "comprehensive input with many fields",
			input: &networkingv1alpha2.DevicePostureInput{
				ID:               "full-check",
				Path:             "/usr/bin/test",
				Exists:           boolPtr(true),
				Sha256:           "checksum",
				Thumbprint:       "thumbprint",
				Running:          boolPtr(true),
				RequireAll:       boolPtr(true),
				Enabled:          boolPtr(true),
				Version:          "1.0.0",
				Operator:         "==",
				Domain:           "corp.example.com",
				ComplianceStatus: "compliant",
				ConnectionID:     "conn-789",
				LastSeen:         "2025-01-11",
				EidLastSeen:      "2025-01-10",
				ActiveThreats:    intPtr(0),
				Infected:         boolPtr(false),
				IsActive:         boolPtr(true),
				NetworkStatus:    "online",
				SensorConfig:     "configured",
				VersionOperator:  ">=",
				CountOperator:    "==",
				ScoreOperator:    ">",
				IssueCount:       intPtr(0),
				Score:            intPtr(100),
				TotalScore:       intPtr(100),
				RiskLevel:        "none",
				Overall:          "pass",
				State:            "active",
				OperationalState: "running",
				OSDistroName:     "Ubuntu",
				OSDistroRevision: "22.04",
				OSVersionExtra:   "LTS",
				OS:               "Linux",
				OperatingSystem:  "linux",
				CertificateID:    "cert-abc",
				CommonName:       "test.example.com",
				Cn:               "test",
				CheckPrivateKey:  boolPtr(true),
				ExtendedKeyUsage: []string{"any"},
				CheckDisks:       []string{"/"},
			},
			validate: func(t *testing.T, result *cf.DevicePostureInputParams) {
				require.NotNil(t, result)
				// Spot check several fields
				assert.Equal(t, "full-check", result.ID)
				assert.Equal(t, "/usr/bin/test", result.Path)
				assert.Equal(t, "Ubuntu", result.OSDistroName)
				assert.Equal(t, "linux", result.OperatingSystem)
				assert.Equal(t, []string{"/"}, result.CheckDisks)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reconciler.buildInput(tt.input)

			if tt.wantNil {
				assert.Nil(t, result)
				return
			}

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

// Helper functions
func boolPtr(b bool) *bool {
	return &b
}

func intPtr(i int) *int {
	return &i
}
