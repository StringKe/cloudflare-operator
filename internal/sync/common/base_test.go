// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package common

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

func init() {
	_ = v1alpha2.AddToScheme(scheme.Scheme)
}

func TestNewBaseSyncController(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	c := NewBaseSyncController(client)

	require.NotNil(t, c)
	assert.NotNil(t, c.Client)
	assert.NotNil(t, c.Debouncer)
	assert.Equal(t, DefaultDebounceDelay, c.Debouncer.GetDelay())
}

func TestNewBaseSyncControllerWithDelay(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	customDelay := 100 * time.Millisecond

	c := NewBaseSyncControllerWithDelay(client, customDelay)

	require.NotNil(t, c)
	assert.Equal(t, customDelay, c.Debouncer.GetDelay())
}

func TestBaseSyncController_GetSyncState(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sync-state",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDNSRecord,
			CloudflareID: "dns-123",
			AccountID:    "acc-123",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewBaseSyncController(client)
	ctx := context.Background()

	result, err := c.GetSyncState(ctx, "test-sync-state")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "test-sync-state", result.Name)
	assert.Equal(t, v1alpha2.SyncResourceDNSRecord, result.Spec.ResourceType)
}

func TestBaseSyncController_GetSyncState_NotFound(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()

	c := NewBaseSyncController(client)
	ctx := context.Background()

	result, err := c.GetSyncState(ctx, "non-existent")

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestBaseSyncController_ShouldSync_HashChanged(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		Status: v1alpha2.CloudflareSyncStateStatus{
			ConfigHash: "old-hash-abc123",
		},
	}

	c := &BaseSyncController{}

	assert.True(t, c.ShouldSync(syncState, "new-hash-xyz789"))
}

func TestBaseSyncController_ShouldSync_SameHash(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		Status: v1alpha2.CloudflareSyncStateStatus{
			ConfigHash: "same-hash-123",
		},
	}

	c := &BaseSyncController{}

	assert.False(t, c.ShouldSync(syncState, "same-hash-123"))
}

func TestBaseSyncController_ShouldSync_EmptyPreviousHash(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		Status: v1alpha2.CloudflareSyncStateStatus{
			ConfigHash: "",
		},
	}

	c := &BaseSyncController{}

	assert.True(t, c.ShouldSync(syncState, "new-hash"))
}

func TestSyncError_Error(t *testing.T) {
	err := &SyncError{
		Op:      "sync",
		Err:     assert.AnError,
		Retries: 3,
	}

	errorMsg := err.Error()
	assert.Contains(t, errorMsg, "sync")
	assert.Contains(t, errorMsg, "retries: 3")
}

func TestSyncError_Unwrap(t *testing.T) {
	innerErr := assert.AnError
	err := &SyncError{
		Op:      "test",
		Err:     innerErr,
		Retries: 1,
	}

	unwrapped := err.Unwrap()
	assert.Equal(t, innerErr, unwrapped)
}

func TestSyncResult(t *testing.T) {
	result := SyncResult{
		ConfigVersion: 42,
		ConfigHash:    "abc123",
	}

	assert.Equal(t, 42, result.ConfigVersion)
	assert.Equal(t, "abc123", result.ConfigHash)
}

func TestBaseSyncController_StoreAggregatedConfig(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{}

	config := map[string]interface{}{
		"name":    "test-config",
		"enabled": true,
		"count":   123,
	}

	c := &BaseSyncController{}
	err := c.StoreAggregatedConfig(syncState, config)

	require.NoError(t, err)
	require.NotNil(t, syncState.Status.AggregatedConfig)

	// Verify the stored config can be unmarshaled
	var stored map[string]interface{}
	err = json.Unmarshal(syncState.Status.AggregatedConfig.Raw, &stored)
	require.NoError(t, err)
	assert.Equal(t, "test-config", stored["name"])
}

func TestParseSourceConfig_Success(t *testing.T) {
	type TestConfig struct {
		Name   string `json:"name"`
		Value  int    `json:"value"`
		Active bool   `json:"active"`
	}

	config := TestConfig{Name: "test", Value: 42, Active: true}
	configBytes, _ := json.Marshal(config)

	source := &v1alpha2.ConfigSource{
		Ref: v1alpha2.SourceReference{
			Kind: "Test",
			Name: "test-resource",
		},
		Config: runtime.RawExtension{Raw: configBytes},
	}

	result, err := ParseSourceConfig[TestConfig](source)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "test", result.Name)
	assert.Equal(t, 42, result.Value)
	assert.True(t, result.Active)
}

func TestParseSourceConfig_NilSource(t *testing.T) {
	result, err := ParseSourceConfig[struct{}](nil)

	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestParseSourceConfig_NilConfig(t *testing.T) {
	source := &v1alpha2.ConfigSource{
		Ref: v1alpha2.SourceReference{
			Kind: "Test",
			Name: "empty",
		},
		Config: runtime.RawExtension{Raw: nil},
	}

	result, err := ParseSourceConfig[struct{}](source)

	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestParseSourceConfig_InvalidJSON(t *testing.T) {
	source := &v1alpha2.ConfigSource{
		Ref: v1alpha2.SourceReference{
			Kind: "Test",
			Name: "invalid",
		},
		Config: runtime.RawExtension{Raw: []byte("not valid json")},
	}

	result, err := ParseSourceConfig[struct{ Name string }](source)

	assert.Error(t, err)
	assert.Nil(t, result)
}
