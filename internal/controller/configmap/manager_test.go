// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package configmap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

// mockTunnelInfo implements TunnelInfo interface for testing
type mockTunnelInfo struct {
	name      string
	namespace string
}

func (m *mockTunnelInfo) GetName() string      { return m.name }
func (m *mockTunnelInfo) GetNamespace() string { return m.namespace }

func TestNewManager(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	manager := NewManager(fakeClient)

	assert.NotNil(t, manager)
	assert.NotNil(t, manager.Client)
}

// TestUpdateIngressRulesPreservesExistingConfig is the critical test that
// verifies the ConfigMap update preserves all non-Ingress fields.
// This is the regression test for the TunnelIngressClassConfig bug.
func TestUpdateIngressRulesPreservesExistingConfig(t *testing.T) {
	// Setup
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Create existing ConfigMap with complete configuration
	existingConfig := cf.Configuration{
		TunnelId:     "f25b7658-e14a-4f82-b4b9-0060d2ecaf01",
		SourceFile:   "/etc/cloudflared/creds/credentials.json",
		Metrics:      "0.0.0.0:2000",
		NoAutoUpdate: true,
		WarpRouting: cf.WarpRoutingConfig{
			Enabled: true,
		},
		OriginRequest: cf.OriginRequestConfig{
			NoTLSVerify: boolPtr(false),
		},
		Ingress: []cf.UnvalidatedIngressRule{
			{
				Hostname: "old-app.example.com",
				Service:  "http://old-app:80",
			},
			{
				Service: "http_status:404",
			},
		},
	}

	configBytes, err := yaml.Marshal(&existingConfig)
	require.NoError(t, err)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-tunnel",
			Namespace: "default",
		},
		Data: map[string]string{
			ConfigKeyName: string(configBytes),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	manager := NewManager(fakeClient)
	tunnel := &mockTunnelInfo{name: "my-tunnel", namespace: "default"}

	// New ingress rules to update
	newRules := []cf.UnvalidatedIngressRule{
		{
			Hostname: "new-app.example.com",
			Service:  "http://new-app:8080",
		},
		{
			Service: "http_status:404",
		},
	}

	// Execute
	updated, err := manager.UpdateIngressRules(context.Background(), tunnel, newRules)

	// Verify
	require.NoError(t, err)
	assert.True(t, updated, "ConfigMap should be updated")

	// Get updated ConfigMap
	updatedCM := &corev1.ConfigMap{}
	err = fakeClient.Get(context.Background(),
		client.ObjectKey{Name: "my-tunnel", Namespace: "default"}, updatedCM)
	require.NoError(t, err)

	// Parse updated config
	var updatedConfig cf.Configuration
	err = yaml.Unmarshal([]byte(updatedCM.Data[ConfigKeyName]), &updatedConfig)
	require.NoError(t, err)

	// CRITICAL: Verify all non-Ingress fields are preserved
	assert.Equal(t, "f25b7658-e14a-4f82-b4b9-0060d2ecaf01", updatedConfig.TunnelId,
		"TunnelId must be preserved")
	assert.Equal(t, "/etc/cloudflared/creds/credentials.json", updatedConfig.SourceFile,
		"SourceFile must be preserved")
	assert.Equal(t, "0.0.0.0:2000", updatedConfig.Metrics,
		"Metrics must be preserved")
	assert.True(t, updatedConfig.NoAutoUpdate,
		"NoAutoUpdate must be preserved")
	assert.True(t, updatedConfig.WarpRouting.Enabled,
		"WarpRouting.Enabled must be preserved")
	assert.NotNil(t, updatedConfig.OriginRequest.NoTLSVerify,
		"OriginRequest.NoTLSVerify must be preserved")
	assert.False(t, *updatedConfig.OriginRequest.NoTLSVerify,
		"OriginRequest.NoTLSVerify value must be preserved")

	// Verify Ingress is updated
	assert.Len(t, updatedConfig.Ingress, 2)
	assert.Equal(t, "new-app.example.com", updatedConfig.Ingress[0].Hostname)
	assert.Equal(t, "http://new-app:8080", updatedConfig.Ingress[0].Service)
}

func TestUpdateIngressRulesNoChange(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Create ConfigMap with existing config
	existingConfig := cf.Configuration{
		TunnelId: "test-tunnel-id",
		Ingress: []cf.UnvalidatedIngressRule{
			{
				Hostname: "app.example.com",
				Service:  "http://app:80",
			},
			{
				Service: "http_status:404",
			},
		},
	}

	configBytes, _ := yaml.Marshal(&existingConfig)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-tunnel",
			Namespace: "default",
		},
		Data: map[string]string{
			ConfigKeyName: string(configBytes),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	manager := NewManager(fakeClient)
	tunnel := &mockTunnelInfo{name: "my-tunnel", namespace: "default"}

	// Same ingress rules
	sameRules := []cf.UnvalidatedIngressRule{
		{
			Hostname: "app.example.com",
			Service:  "http://app:80",
		},
		{
			Service: "http_status:404",
		},
	}

	updated, err := manager.UpdateIngressRules(context.Background(), tunnel, sameRules)

	require.NoError(t, err)
	assert.False(t, updated, "ConfigMap should not be updated when content is unchanged")
}

func TestUpdateIngressRulesConfigMapNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build() // No ConfigMap

	manager := NewManager(fakeClient)
	tunnel := &mockTunnelInfo{name: "non-existent", namespace: "default"}

	rules := []cf.UnvalidatedIngressRule{
		{Service: "http_status:404"},
	}

	updated, err := manager.UpdateIngressRules(context.Background(), tunnel, rules)

	assert.Error(t, err)
	assert.False(t, updated)
	assert.Contains(t, err.Error(), "not found")
}

func TestUpdateIngressRulesInvalidYAML(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Create ConfigMap with invalid YAML
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-tunnel",
			Namespace: "default",
		},
		Data: map[string]string{
			ConfigKeyName: "invalid: yaml: content: [",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	manager := NewManager(fakeClient)
	tunnel := &mockTunnelInfo{name: "my-tunnel", namespace: "default"}

	rules := []cf.UnvalidatedIngressRule{
		{Service: "http_status:404"},
	}

	_, err := manager.UpdateIngressRules(context.Background(), tunnel, rules)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestUpdateIngressRulesEmptyConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Create empty ConfigMap (no config.yaml key)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-tunnel",
			Namespace: "default",
		},
		Data: map[string]string{},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	manager := NewManager(fakeClient)
	tunnel := &mockTunnelInfo{name: "my-tunnel", namespace: "default"}

	rules := []cf.UnvalidatedIngressRule{
		{
			Hostname: "app.example.com",
			Service:  "http://app:80",
		},
		{
			Service: "http_status:404",
		},
	}

	updated, err := manager.UpdateIngressRules(context.Background(), tunnel, rules)

	require.NoError(t, err)
	assert.True(t, updated)

	// Verify ConfigMap was updated
	updatedCM := &corev1.ConfigMap{}
	err = fakeClient.Get(context.Background(),
		client.ObjectKey{Name: "my-tunnel", Namespace: "default"}, updatedCM)
	require.NoError(t, err)
	assert.NotEmpty(t, updatedCM.Data[ConfigKeyName])
}

func TestGetCurrentConfig(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	expectedConfig := cf.Configuration{
		TunnelId:   "test-tunnel-id",
		SourceFile: "/path/to/creds",
		Metrics:    "0.0.0.0:2000",
		WarpRouting: cf.WarpRoutingConfig{
			Enabled: true,
		},
		Ingress: []cf.UnvalidatedIngressRule{
			{
				Hostname: "app.example.com",
				Service:  "http://app:80",
			},
		},
	}

	configBytes, _ := yaml.Marshal(&expectedConfig)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-tunnel",
			Namespace: "default",
		},
		Data: map[string]string{
			ConfigKeyName: string(configBytes),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	manager := NewManager(fakeClient)
	tunnel := &mockTunnelInfo{name: "my-tunnel", namespace: "default"}

	config, err := manager.GetCurrentConfig(context.Background(), tunnel)

	require.NoError(t, err)
	assert.Equal(t, expectedConfig.TunnelId, config.TunnelId)
	assert.Equal(t, expectedConfig.SourceFile, config.SourceFile)
	assert.Equal(t, expectedConfig.Metrics, config.Metrics)
	assert.True(t, config.WarpRouting.Enabled)
	assert.Len(t, config.Ingress, 1)
}

func TestGetCurrentConfigNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	manager := NewManager(fakeClient)
	tunnel := &mockTunnelInfo{name: "non-existent", namespace: "default"}

	_, err := manager.GetCurrentConfig(context.Background(), tunnel)

	assert.Error(t, err)
}

func TestCalculateChecksum(t *testing.T) {
	tests := []struct {
		name   string
		input1 string
		input2 string
		same   bool
	}{
		{
			name:   "same content same checksum",
			input1: "tunnel: abc\ningress:\n  - service: http://app:80",
			input2: "tunnel: abc\ningress:\n  - service: http://app:80",
			same:   true,
		},
		{
			name:   "different content different checksum",
			input1: "tunnel: abc",
			input2: "tunnel: xyz",
			same:   false,
		},
		{
			name:   "empty string",
			input1: "",
			input2: "",
			same:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checksum1 := CalculateChecksum(tt.input1)
			checksum2 := CalculateChecksum(tt.input2)

			if tt.same {
				assert.Equal(t, checksum1, checksum2)
			} else {
				assert.NotEqual(t, checksum1, checksum2)
			}

			// Checksum should be 32 characters (MD5 hex)
			assert.Len(t, checksum1, 32)
		})
	}
}

func TestChecksumAnnotation(t *testing.T) {
	assert.Equal(t, "cloudflare-operator.io/checksum", ChecksumAnnotation)
}

func TestConfigKeyName(t *testing.T) {
	assert.Equal(t, "config.yaml", ConfigKeyName)
}

// TestUpdateIngressRulesYAMLFormat verifies the YAML output format is correct
// (snake_case keys, not PascalCase)
func TestUpdateIngressRulesYAMLFormat(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-tunnel",
			Namespace: "default",
		},
		Data: map[string]string{
			ConfigKeyName: "tunnel: test-id\n",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	manager := NewManager(fakeClient)
	tunnel := &mockTunnelInfo{name: "my-tunnel", namespace: "default"}

	rules := []cf.UnvalidatedIngressRule{
		{
			Hostname: "app.example.com",
			Service:  "http://app:80",
			OriginRequest: cf.OriginRequestConfig{
				NoTLSVerify: boolPtr(true),
			},
		},
		{
			Service: "http_status:404",
		},
	}

	_, err := manager.UpdateIngressRules(context.Background(), tunnel, rules)
	require.NoError(t, err)

	// Get updated ConfigMap
	updatedCM := &corev1.ConfigMap{}
	err = fakeClient.Get(context.Background(),
		client.ObjectKey{Name: "my-tunnel", Namespace: "default"}, updatedCM)
	require.NoError(t, err)

	yamlContent := updatedCM.Data[ConfigKeyName]

	// Verify snake_case keys are used
	assert.Contains(t, yamlContent, "hostname:")
	assert.Contains(t, yamlContent, "service:")
	assert.Contains(t, yamlContent, "originRequest:")
	assert.Contains(t, yamlContent, "noTLSVerify:")

	// Verify PascalCase keys are NOT used
	assert.NotContains(t, yamlContent, "Hostname:")
	assert.NotContains(t, yamlContent, "Service:")
	assert.NotContains(t, yamlContent, "OriginRequest:")
	assert.NotContains(t, yamlContent, "NoTLSVerify:")
}

// TestUpdateIngressRulesMultipleUpdates tests multiple consecutive updates
func TestUpdateIngressRulesMultipleUpdates(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	initialConfig := cf.Configuration{
		TunnelId:    "persistent-tunnel-id",
		SourceFile:  "/etc/cloudflared/creds.json",
		Metrics:     "0.0.0.0:2000",
		WarpRouting: cf.WarpRoutingConfig{Enabled: true},
	}

	configBytes, _ := yaml.Marshal(&initialConfig)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-tunnel",
			Namespace: "default",
		},
		Data: map[string]string{
			ConfigKeyName: string(configBytes),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	manager := NewManager(fakeClient)
	tunnel := &mockTunnelInfo{name: "my-tunnel", namespace: "default"}

	// First update
	rules1 := []cf.UnvalidatedIngressRule{
		{Hostname: "app1.example.com", Service: "http://app1:80"},
		{Service: "http_status:404"},
	}
	_, err := manager.UpdateIngressRules(context.Background(), tunnel, rules1)
	require.NoError(t, err)

	// Second update
	rules2 := []cf.UnvalidatedIngressRule{
		{Hostname: "app2.example.com", Service: "http://app2:8080"},
		{Service: "http_status:404"},
	}
	_, err = manager.UpdateIngressRules(context.Background(), tunnel, rules2)
	require.NoError(t, err)

	// Third update
	rules3 := []cf.UnvalidatedIngressRule{
		{Hostname: "app3.example.com", Service: "http://app3:3000"},
		{Hostname: "api.example.com", Service: "http://api:8080"},
		{Service: "http_status:404"},
	}
	_, err = manager.UpdateIngressRules(context.Background(), tunnel, rules3)
	require.NoError(t, err)

	// Verify final state
	config, err := manager.GetCurrentConfig(context.Background(), tunnel)
	require.NoError(t, err)

	// Critical fields must still be preserved
	assert.Equal(t, "persistent-tunnel-id", config.TunnelId)
	assert.Equal(t, "/etc/cloudflared/creds.json", config.SourceFile)
	assert.Equal(t, "0.0.0.0:2000", config.Metrics)
	assert.True(t, config.WarpRouting.Enabled)

	// Final ingress rules
	assert.Len(t, config.Ingress, 3)
	assert.Equal(t, "app3.example.com", config.Ingress[0].Hostname)
	assert.Equal(t, "api.example.com", config.Ingress[1].Hostname)
}

// boolPtr helper function
func boolPtr(b bool) *bool {
	return &b
}
