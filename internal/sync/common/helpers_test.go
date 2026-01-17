// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package common

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

func TestIsPendingID(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		expected bool
	}{
		{"valid pending ID", "pending-my-resource", true},
		{"valid pending ID with dashes", "pending-my-resource-name", true},
		{"real Cloudflare ID", "abc123def456", false},
		{"empty string", "", false},
		{"just prefix", "pending-", false},
		{"similar but not prefix", "pending", false},
		{"UUID format", "a1b2c3d4-e5f6-7890-abcd-ef1234567890", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPendingID(tt.id)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGeneratePendingID(t *testing.T) {
	tests := []struct {
		name         string
		resourceName string
		expected     string
	}{
		{"simple name", "my-resource", "pending-my-resource"},
		{"with namespace", "my-namespace-my-resource", "pending-my-namespace-my-resource"},
		{"empty name", "", "pending-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GeneratePendingID(tt.resourceName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPendingIDPrefix(t *testing.T) {
	assert.Equal(t, "pending-", PendingIDPrefix)
}

// TestConfig is a sample config structure for testing
type TestConfig struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Port    int    `json:"port"`
}

func createTestSyncState(sources []v1alpha2.ConfigSource) *v1alpha2.CloudflareSyncState {
	return &v1alpha2.CloudflareSyncState{
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDNSRecord,
			CloudflareID: "test-id",
			AccountID:    "test-account",
			Sources:      sources,
		},
	}
}

func createConfigSource(kind, namespace, name string, config interface{}) v1alpha2.ConfigSource {
	configBytes, _ := json.Marshal(config)
	return v1alpha2.ConfigSource{
		Ref: v1alpha2.SourceReference{
			Kind:      kind,
			Namespace: namespace,
			Name:      name,
		},
		Config: runtime.RawExtension{Raw: configBytes},
	}
}

func TestExtractFirstSourceConfig_Success(t *testing.T) {
	config := TestConfig{Name: "test", Enabled: true, Port: 8080}
	source := createConfigSource("DNSRecord", "test-ns", "my-record", config)
	syncState := createTestSyncState([]v1alpha2.ConfigSource{source})

	result, err := ExtractFirstSourceConfig[TestConfig](syncState)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "test", result.Name)
	assert.True(t, result.Enabled)
	assert.Equal(t, 8080, result.Port)
}

func TestExtractFirstSourceConfig_NoSources(t *testing.T) {
	syncState := createTestSyncState([]v1alpha2.ConfigSource{})

	result, err := ExtractFirstSourceConfig[TestConfig](syncState)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no sources")
}

func TestExtractFirstSourceConfig_MultipleSources(t *testing.T) {
	// Should return the first source
	config1 := TestConfig{Name: "first", Enabled: true, Port: 8080}
	config2 := TestConfig{Name: "second", Enabled: false, Port: 9090}

	sources := []v1alpha2.ConfigSource{
		createConfigSource("DNSRecord", "ns1", "record1", config1),
		createConfigSource("DNSRecord", "ns2", "record2", config2),
	}
	syncState := createTestSyncState(sources)

	result, err := ExtractFirstSourceConfig[TestConfig](syncState)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "first", result.Name)
}

func TestExtractFirstSourceConfig_InvalidJSON(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		Spec: v1alpha2.CloudflareSyncStateSpec{
			Sources: []v1alpha2.ConfigSource{
				{
					Ref: v1alpha2.SourceReference{
						Kind: "Test",
						Name: "invalid",
					},
					Config: runtime.RawExtension{Raw: []byte("not valid json")},
				},
			},
		},
	}

	result, err := ExtractFirstSourceConfig[TestConfig](syncState)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestExtractFirstSourceConfig_EmptyConfig(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		Spec: v1alpha2.CloudflareSyncStateSpec{
			Sources: []v1alpha2.ConfigSource{
				{
					Ref: v1alpha2.SourceReference{
						Kind: "Test",
						Name: "empty",
					},
					Config: runtime.RawExtension{Raw: nil},
				},
			},
		},
	}

	result, err := ExtractFirstSourceConfig[TestConfig](syncState)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "empty config")
}

func TestRequireAccountID_Present(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		Spec: v1alpha2.CloudflareSyncStateSpec{
			AccountID: "test-account-123",
		},
	}

	accountID, err := RequireAccountID(syncState)

	require.NoError(t, err)
	assert.Equal(t, "test-account-123", accountID)
}

func TestRequireAccountID_Missing(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		Spec: v1alpha2.CloudflareSyncStateSpec{
			AccountID: "",
		},
	}

	accountID, err := RequireAccountID(syncState)

	assert.Error(t, err)
	assert.Empty(t, accountID)
	assert.Contains(t, err.Error(), "account ID")
}

func TestRequireZoneID_Present(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ZoneID: "test-zone-456",
		},
	}

	zoneID, err := RequireZoneID(syncState)

	require.NoError(t, err)
	assert.Equal(t, "test-zone-456", zoneID)
}

func TestRequireZoneID_Missing(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ZoneID: "",
		},
	}

	zoneID, err := RequireZoneID(syncState)

	assert.Error(t, err)
	assert.Empty(t, zoneID)
	assert.Contains(t, err.Error(), "zone ID")
}

func TestFilterSourcesByKind_SingleKind(t *testing.T) {
	sources := []v1alpha2.ConfigSource{
		{Ref: v1alpha2.SourceReference{Kind: "Tunnel", Name: "t1"}},
		{Ref: v1alpha2.SourceReference{Kind: "Ingress", Name: "i1"}},
		{Ref: v1alpha2.SourceReference{Kind: "Tunnel", Name: "t2"}},
		{Ref: v1alpha2.SourceReference{Kind: "TunnelBinding", Name: "b1"}},
	}

	result := FilterSourcesByKind(sources, "Tunnel")

	assert.Len(t, result, 2)
	assert.Equal(t, "t1", result[0].Ref.Name)
	assert.Equal(t, "t2", result[1].Ref.Name)
}

func TestFilterSourcesByKind_MultipleKinds(t *testing.T) {
	sources := []v1alpha2.ConfigSource{
		{Ref: v1alpha2.SourceReference{Kind: "Tunnel", Name: "t1"}},
		{Ref: v1alpha2.SourceReference{Kind: "Ingress", Name: "i1"}},
		{Ref: v1alpha2.SourceReference{Kind: "Tunnel", Name: "t2"}},
		{Ref: v1alpha2.SourceReference{Kind: "TunnelBinding", Name: "b1"}},
	}

	result := FilterSourcesByKind(sources, "Tunnel", "Ingress")

	assert.Len(t, result, 3)
}

func TestFilterSourcesByKind_NoMatch(t *testing.T) {
	sources := []v1alpha2.ConfigSource{
		{Ref: v1alpha2.SourceReference{Kind: "Tunnel", Name: "t1"}},
		{Ref: v1alpha2.SourceReference{Kind: "Ingress", Name: "i1"}},
	}

	result := FilterSourcesByKind(sources, "DNSRecord")

	assert.Empty(t, result)
}

func TestFilterSourcesByKind_EmptySources(t *testing.T) {
	result := FilterSourcesByKind([]v1alpha2.ConfigSource{}, "Tunnel")
	assert.Empty(t, result)
}

func TestRequeueAfterError(t *testing.T) {
	// Any error should return 30 seconds
	duration := RequeueAfterError(assert.AnError)
	assert.Equal(t, 30*time.Second, duration)
}

func TestRequeueAfterSuccess(t *testing.T) {
	// Success should return 0 (no periodic refresh needed)
	duration := RequeueAfterSuccess()
	assert.Equal(t, time.Duration(0), duration)
}

func TestSourceReference_String(t *testing.T) {
	tests := []struct {
		name     string
		ref      v1alpha2.SourceReference
		expected string
	}{
		{
			name:     "cluster-scoped resource",
			ref:      v1alpha2.SourceReference{Kind: "ClusterTunnel", Name: "my-tunnel"},
			expected: "ClusterTunnel/my-tunnel",
		},
		{
			name:     "namespaced resource",
			ref:      v1alpha2.SourceReference{Kind: "Ingress", Namespace: "default", Name: "my-ingress"},
			expected: "Ingress/default/my-ingress",
		},
		{
			name:     "empty namespace",
			ref:      v1alpha2.SourceReference{Kind: "Tunnel", Namespace: "", Name: "tunnel1"},
			expected: "Tunnel/tunnel1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ref.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}
