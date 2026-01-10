// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// TestConfigurationYAMLSerialization verifies that Configuration serializes
// to YAML with correct snake_case field names (not PascalCase).
// This is critical because cloudflared expects specific YAML key names.
func TestConfigurationYAMLSerialization(t *testing.T) {
	config := &Configuration{
		TunnelId:     "f25b7658-e14a-4f82-b4b9-0060d2ecaf01",
		SourceFile:   "/etc/cloudflared/creds/credentials.json",
		Metrics:      "0.0.0.0:2000",
		NoAutoUpdate: true,
		WarpRouting: WarpRoutingConfig{
			Enabled: true,
		},
		OriginRequest: OriginRequestConfig{
			NoTLSVerify: boolPtr(false),
		},
		Ingress: []UnvalidatedIngressRule{
			{
				Hostname: "app.example.com",
				Service:  "http://app-service.default.svc:80",
			},
			{
				Service: "http_status:404",
			},
		},
	}

	yamlBytes, err := yaml.Marshal(config)
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	// Verify snake_case keys are used (as per cloudflared expectations)
	assert.Contains(t, yamlStr, "tunnel:", "TunnelId should serialize as 'tunnel'")
	assert.Contains(t, yamlStr, "credentials-file:", "SourceFile should serialize as 'credentials-file'")
	assert.Contains(t, yamlStr, "metrics:", "Metrics should serialize as 'metrics'")
	assert.Contains(t, yamlStr, "no-autoupdate:", "NoAutoUpdate should serialize as 'no-autoupdate'")
	assert.Contains(t, yamlStr, "warp-routing:", "WarpRouting should serialize as 'warp-routing'")
	assert.Contains(t, yamlStr, "originRequest:", "OriginRequest should serialize as 'originRequest'")
	assert.Contains(t, yamlStr, "ingress:", "Ingress should serialize as 'ingress'")
	assert.Contains(t, yamlStr, "hostname:", "Hostname should serialize as 'hostname'")
	assert.Contains(t, yamlStr, "service:", "Service should serialize as 'service'")

	// Verify PascalCase keys are NOT used
	assert.NotContains(t, yamlStr, "TunnelId:", "Should not use PascalCase 'TunnelId'")
	assert.NotContains(t, yamlStr, "SourceFile:", "Should not use PascalCase 'SourceFile'")
	assert.NotContains(t, yamlStr, "NoAutoUpdate:", "Should not use PascalCase 'NoAutoUpdate'")
	assert.NotContains(t, yamlStr, "WarpRouting:", "Should not use PascalCase 'WarpRouting'")
	assert.NotContains(t, yamlStr, "Ingress:", "Should not use PascalCase 'Ingress'")
	assert.NotContains(t, yamlStr, "Hostname:", "Should not use PascalCase 'Hostname'")
	assert.NotContains(t, yamlStr, "Service:", "Should not use PascalCase 'Service'")

	// Verify actual values
	assert.Contains(t, yamlStr, "f25b7658-e14a-4f82-b4b9-0060d2ecaf01")
	assert.Contains(t, yamlStr, "/etc/cloudflared/creds/credentials.json")
	assert.Contains(t, yamlStr, "app.example.com")
	assert.Contains(t, yamlStr, "http_status:404")
}

// TestConfigurationYAMLDeserialization verifies that Configuration can be
// deserialized from YAML with snake_case field names.
func TestConfigurationYAMLDeserialization(t *testing.T) {
	yamlContent := `
tunnel: f25b7658-e14a-4f82-b4b9-0060d2ecaf01
credentials-file: /etc/cloudflared/creds/credentials.json
metrics: 0.0.0.0:2000
no-autoupdate: true
warp-routing:
  enabled: true
originRequest:
  noTLSVerify: false
ingress:
  - hostname: argocd.example.com
    service: http://argocd-server.argocd.svc:80
  - hostname: workflows.example.com
    service: http://workflows-server.workflows.svc:2746
  - service: http_status:404
`

	var config Configuration
	err := yaml.Unmarshal([]byte(yamlContent), &config)
	require.NoError(t, err)

	assert.Equal(t, "f25b7658-e14a-4f82-b4b9-0060d2ecaf01", config.TunnelId)
	assert.Equal(t, "/etc/cloudflared/creds/credentials.json", config.SourceFile)
	assert.Equal(t, "0.0.0.0:2000", config.Metrics)
	assert.True(t, config.NoAutoUpdate)
	assert.True(t, config.WarpRouting.Enabled)
	assert.NotNil(t, config.OriginRequest.NoTLSVerify)
	assert.False(t, *config.OriginRequest.NoTLSVerify)
	assert.Len(t, config.Ingress, 3)
	assert.Equal(t, "argocd.example.com", config.Ingress[0].Hostname)
	assert.Equal(t, "http://argocd-server.argocd.svc:80", config.Ingress[0].Service)
	assert.Equal(t, "http_status:404", config.Ingress[2].Service)
}

// TestConfigurationRoundTrip verifies that Configuration can be serialized
// and deserialized without data loss.
func TestConfigurationRoundTrip(t *testing.T) {
	original := &Configuration{
		TunnelId:     "abc-123-def-456",
		SourceFile:   "/path/to/creds.json",
		Metrics:      "127.0.0.1:3000",
		NoAutoUpdate: true,
		WarpRouting: WarpRoutingConfig{
			Enabled: true,
		},
		OriginRequest: OriginRequestConfig{
			NoTLSVerify: boolPtr(true),
			Http2Origin: boolPtr(false),
			BastionMode: boolPtr(false),
		},
		Ingress: []UnvalidatedIngressRule{
			{
				Hostname: "test.example.com",
				Path:     "/api/*",
				Service:  "http://api:8080",
				OriginRequest: OriginRequestConfig{
					NoTLSVerify: boolPtr(true),
				},
			},
			{
				Service: "http_status:404",
			},
		},
	}

	// Serialize
	yamlBytes, err := yaml.Marshal(original)
	require.NoError(t, err)

	// Deserialize
	var restored Configuration
	err = yaml.Unmarshal(yamlBytes, &restored)
	require.NoError(t, err)

	// Verify all fields preserved
	assert.Equal(t, original.TunnelId, restored.TunnelId)
	assert.Equal(t, original.SourceFile, restored.SourceFile)
	assert.Equal(t, original.Metrics, restored.Metrics)
	assert.Equal(t, original.NoAutoUpdate, restored.NoAutoUpdate)
	assert.Equal(t, original.WarpRouting.Enabled, restored.WarpRouting.Enabled)
	assert.Equal(t, *original.OriginRequest.NoTLSVerify, *restored.OriginRequest.NoTLSVerify)
	assert.Len(t, restored.Ingress, 2)
	assert.Equal(t, original.Ingress[0].Hostname, restored.Ingress[0].Hostname)
	assert.Equal(t, original.Ingress[0].Path, restored.Ingress[0].Path)
}

// TestConfigurationUpdatePreservesFields tests that updating only Ingress
// preserves all other fields (critical for the ConfigMap update bug).
func TestConfigurationUpdatePreservesFields(t *testing.T) {
	// Simulate existing ConfigMap content
	existingYAML := `
tunnel: f25b7658-e14a-4f82-b4b9-0060d2ecaf01
credentials-file: /etc/cloudflared/creds/credentials.json
metrics: 0.0.0.0:2000
no-autoupdate: true
warp-routing:
  enabled: true
originRequest:
  noTLSVerify: false
ingress:
  - hostname: old-app.example.com
    service: http://old-app:80
  - service: http_status:404
`

	// Parse existing config
	var existingConfig Configuration
	err := yaml.Unmarshal([]byte(existingYAML), &existingConfig)
	require.NoError(t, err)

	// Update only Ingress (simulating what the controller does)
	newIngress := []UnvalidatedIngressRule{
		{
			Hostname: "new-app.example.com",
			Service:  "http://new-app:8080",
		},
		{
			Service: "http_status:404",
		},
	}
	existingConfig.Ingress = newIngress

	// Serialize back
	updatedYAML, err := yaml.Marshal(&existingConfig)
	require.NoError(t, err)

	yamlStr := string(updatedYAML)

	// Verify critical fields are preserved
	assert.Contains(t, yamlStr, "tunnel: f25b7658-e14a-4f82-b4b9-0060d2ecaf01",
		"TunnelId must be preserved after update")
	assert.Contains(t, yamlStr, "credentials-file: /etc/cloudflared/creds/credentials.json",
		"SourceFile must be preserved after update")
	assert.Contains(t, yamlStr, "metrics: 0.0.0.0:2000",
		"Metrics must be preserved after update")
	assert.Contains(t, yamlStr, "no-autoupdate: true",
		"NoAutoUpdate must be preserved after update")
	assert.Contains(t, yamlStr, "warp-routing:",
		"WarpRouting must be preserved after update")
	assert.Contains(t, yamlStr, "enabled: true",
		"WarpRouting.Enabled must be preserved after update")

	// Verify Ingress is updated
	assert.Contains(t, yamlStr, "new-app.example.com", "New ingress hostname should be present")
	assert.NotContains(t, yamlStr, "old-app.example.com", "Old ingress hostname should be replaced")
}

// TestConfigurationEmptyIngress tests serialization with empty ingress rules.
func TestConfigurationEmptyIngress(t *testing.T) {
	config := &Configuration{
		TunnelId:   "test-tunnel-id",
		SourceFile: "/path/to/creds",
		Ingress:    []UnvalidatedIngressRule{},
	}

	yamlBytes, err := yaml.Marshal(config)
	require.NoError(t, err)

	yamlStr := string(yamlBytes)
	assert.Contains(t, yamlStr, "tunnel: test-tunnel-id")
	// Empty ingress should be omitted due to omitempty
	assert.NotContains(t, yamlStr, "ingress:")
}

// TestConfigurationNilPointerFields tests that nil pointer fields are omitted.
func TestConfigurationNilPointerFields(t *testing.T) {
	config := &Configuration{
		TunnelId:      "test-tunnel-id",
		SourceFile:    "/path/to/creds",
		OriginRequest: OriginRequestConfig{
			// All pointer fields are nil
		},
		Ingress: []UnvalidatedIngressRule{
			{
				Hostname: "test.example.com",
				Service:  "http://test:80",
				OriginRequest: OriginRequestConfig{
					NoTLSVerify: boolPtr(true),
					// Other fields are nil
				},
			},
		},
	}

	yamlBytes, err := yaml.Marshal(config)
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	// Nil pointer fields should be omitted (not serialized as null)
	assert.NotContains(t, yamlStr, "bastionMode: null")
	assert.NotContains(t, yamlStr, "http2Origin: null")
	assert.NotContains(t, yamlStr, "connectTimeout: null")

	// But explicit values should be present
	assert.Contains(t, yamlStr, "noTLSVerify: true")
}

// TestUnvalidatedIngressRuleYAMLSerialization tests individual ingress rule serialization.
// nolint:revive // cognitive complexity is acceptable for comprehensive test coverage
func TestUnvalidatedIngressRuleYAMLSerialization(t *testing.T) {
	tests := []struct {
		name     string
		rule     UnvalidatedIngressRule
		wantKeys []string
		wantVals []string
	}{
		{
			name: "basic rule with hostname",
			rule: UnvalidatedIngressRule{
				Hostname: "app.example.com",
				Service:  "http://app:80",
			},
			wantKeys: []string{"hostname:", "service:"},
			wantVals: []string{"app.example.com", "http://app:80"},
		},
		{
			name: "rule with path",
			rule: UnvalidatedIngressRule{
				Hostname: "api.example.com",
				Path:     "/v1/*",
				Service:  "http://api:8080",
			},
			wantKeys: []string{"hostname:", "path:", "service:"},
			wantVals: []string{"api.example.com", "/v1/*", "http://api:8080"},
		},
		{
			name: "catch-all rule",
			rule: UnvalidatedIngressRule{
				Service: "http_status:404",
			},
			wantKeys: []string{"service:"},
			wantVals: []string{"http_status:404"},
		},
		{
			name: "rule with origin request",
			rule: UnvalidatedIngressRule{
				Hostname: "secure.example.com",
				Service:  "https://secure:443",
				OriginRequest: OriginRequestConfig{
					NoTLSVerify: boolPtr(true),
					Http2Origin: boolPtr(true),
				},
			},
			wantKeys: []string{"hostname:", "service:", "originRequest:", "noTLSVerify:", "http2Origin:"},
			wantVals: []string{"secure.example.com", "https://secure:443"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yamlBytes, err := yaml.Marshal(&tt.rule)
			require.NoError(t, err)

			yamlStr := string(yamlBytes)

			for _, key := range tt.wantKeys {
				assert.Contains(t, yamlStr, key, "Expected key %s to be present", key)
			}

			for _, val := range tt.wantVals {
				assert.Contains(t, yamlStr, val, "Expected value %s to be present", val)
			}

			// Verify no PascalCase keys
			assert.NotContains(t, yamlStr, "Hostname:")
			assert.NotContains(t, yamlStr, "Service:")
			assert.NotContains(t, yamlStr, "Path:")
			assert.NotContains(t, yamlStr, "OriginRequest:")
		})
	}
}

// TestOriginRequestConfigYAMLSerialization tests OriginRequestConfig serialization.
func TestOriginRequestConfigYAMLSerialization(t *testing.T) {
	timeout := 30 * time.Second
	keepAlive := 60 * time.Second
	host := "custom-host.example.com"
	port := uint(8080)

	config := OriginRequestConfig{
		ConnectTimeout:         &timeout,
		TCPKeepAlive:           &keepAlive,
		HTTPHostHeader:         &host,
		NoTLSVerify:            boolPtr(true),
		Http2Origin:            boolPtr(false),
		DisableChunkedEncoding: boolPtr(true),
		BastionMode:            boolPtr(false),
		ProxyPort:              &port,
	}

	yamlBytes, err := yaml.Marshal(&config)
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	// Verify snake_case/camelCase keys as per cloudflared spec
	assert.Contains(t, yamlStr, "connectTimeout:")
	assert.Contains(t, yamlStr, "tcpKeepAlive:")
	assert.Contains(t, yamlStr, "httpHostHeader:")
	assert.Contains(t, yamlStr, "noTLSVerify:")
	assert.Contains(t, yamlStr, "http2Origin:")
	assert.Contains(t, yamlStr, "disableChunkedEncoding:")
	assert.Contains(t, yamlStr, "bastionMode:")
	assert.Contains(t, yamlStr, "proxyPort:")

	// Verify values
	assert.Contains(t, yamlStr, "custom-host.example.com")
}

// TestWarpRoutingConfigYAMLSerialization tests WarpRoutingConfig serialization.
func TestWarpRoutingConfigYAMLSerialization(t *testing.T) {
	tests := []struct {
		name    string
		config  WarpRoutingConfig
		wantStr string
	}{
		{
			name:    "enabled",
			config:  WarpRoutingConfig{Enabled: true},
			wantStr: "enabled: true",
		},
		{
			name:    "disabled",
			config:  WarpRoutingConfig{Enabled: false},
			wantStr: "{}", // omitempty should omit false
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yamlBytes, err := yaml.Marshal(&tt.config)
			require.NoError(t, err)

			yamlStr := strings.TrimSpace(string(yamlBytes))
			assert.Contains(t, yamlStr, tt.wantStr)
		})
	}
}

// TestConfigurationWithAllFields tests a complete configuration with all fields.
func TestConfigurationWithAllFields(t *testing.T) {
	timeout := 30 * time.Second

	config := &Configuration{
		TunnelId:     "complete-tunnel-id",
		SourceFile:   "/etc/cloudflared/creds/credentials.json",
		Metrics:      "0.0.0.0:2000",
		NoAutoUpdate: true,
		WarpRouting: WarpRoutingConfig{
			Enabled: true,
		},
		OriginRequest: OriginRequestConfig{
			ConnectTimeout: &timeout,
			NoTLSVerify:    boolPtr(false),
		},
		Ingress: []UnvalidatedIngressRule{
			{
				Hostname: "web.example.com",
				Service:  "http://web:80",
				OriginRequest: OriginRequestConfig{
					NoTLSVerify: boolPtr(true),
				},
			},
			{
				Hostname: "api.example.com",
				Path:     "/v1/*",
				Service:  "http://api:8080",
			},
			{
				Service: "http_status:404",
			},
		},
	}

	yamlBytes, err := yaml.Marshal(config)
	require.NoError(t, err)

	// Deserialize and verify
	var restored Configuration
	err = yaml.Unmarshal(yamlBytes, &restored)
	require.NoError(t, err)

	assert.Equal(t, config.TunnelId, restored.TunnelId)
	assert.Equal(t, config.SourceFile, restored.SourceFile)
	assert.Equal(t, config.Metrics, restored.Metrics)
	assert.Equal(t, config.NoAutoUpdate, restored.NoAutoUpdate)
	assert.Equal(t, config.WarpRouting.Enabled, restored.WarpRouting.Enabled)
	assert.Len(t, restored.Ingress, 3)
}

// TestIngressIPRuleYAMLSerialization tests IngressIPRule serialization.
func TestIngressIPRuleYAMLSerialization(t *testing.T) {
	prefix := "192.168.1.0/24"
	rule := IngressIPRule{
		Prefix: &prefix,
		Ports:  []int{80, 443, 8080},
		Allow:  true,
	}

	yamlBytes, err := yaml.Marshal(&rule)
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	assert.Contains(t, yamlStr, "prefix:")
	assert.Contains(t, yamlStr, "ports:")
	assert.Contains(t, yamlStr, "allow:")
	assert.Contains(t, yamlStr, "192.168.1.0/24")
}

// boolPtr is a helper function to create a pointer to a bool.
func boolPtr(b bool) *bool {
	return &b
}
