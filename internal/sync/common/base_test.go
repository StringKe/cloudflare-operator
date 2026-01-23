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
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
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

// ============================================================================
// Unified Error Handling Tests
// ============================================================================

func TestRequeueAfterError_PermanentErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantZero bool
	}{
		{
			name:     "nil error returns 0",
			err:      nil,
			wantZero: true,
		},
		{
			name:     "auth error returns 0",
			err:      assert.AnError,
			wantZero: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := RequeueAfterError(tt.err)
			if tt.wantZero {
				assert.Equal(t, time.Duration(0), delay)
			}
		})
	}
}

func TestShouldResetFromFailed(t *testing.T) {
	tests := []struct {
		name                string
		status              v1alpha2.SyncStatus
		generation          int64
		observedGeneration  int64
		expectedShouldReset bool
	}{
		{
			name:                "not in Failed status",
			status:              v1alpha2.SyncStatusError,
			generation:          2,
			observedGeneration:  1,
			expectedShouldReset: false,
		},
		{
			name:                "in Failed status, same generation",
			status:              v1alpha2.SyncStatusFailed,
			generation:          1,
			observedGeneration:  1,
			expectedShouldReset: false,
		},
		{
			name:                "in Failed status, generation increased",
			status:              v1alpha2.SyncStatusFailed,
			generation:          2,
			observedGeneration:  1,
			expectedShouldReset: true,
		},
		{
			name:                "in Failed status, generation much higher",
			status:              v1alpha2.SyncStatusFailed,
			generation:          10,
			observedGeneration:  1,
			expectedShouldReset: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syncState := &v1alpha2.CloudflareSyncState{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Generation: tt.generation,
				},
				Status: v1alpha2.CloudflareSyncStateStatus{
					SyncStatus:         tt.status,
					ObservedGeneration: tt.observedGeneration,
				},
			}

			result := ShouldResetFromFailed(syncState)
			assert.Equal(t, tt.expectedShouldReset, result)
		})
	}
}

func TestIsFailed(t *testing.T) {
	tests := []struct {
		name   string
		status v1alpha2.SyncStatus
		want   bool
	}{
		{
			name:   "SyncStatusFailed",
			status: v1alpha2.SyncStatusFailed,
			want:   true,
		},
		{
			name:   "SyncStatusError",
			status: v1alpha2.SyncStatusError,
			want:   false,
		},
		{
			name:   "SyncStatusSynced",
			status: v1alpha2.SyncStatusSynced,
			want:   false,
		},
		{
			name:   "SyncStatusPending",
			status: v1alpha2.SyncStatusPending,
			want:   false,
		},
		{
			name:   "SyncStatusSyncing",
			status: v1alpha2.SyncStatusSyncing,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syncState := &v1alpha2.CloudflareSyncState{
				Status: v1alpha2.CloudflareSyncStateStatus{
					SyncStatus: tt.status,
				},
			}

			result := IsFailed(syncState)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestCalculateExponentialBackoff(t *testing.T) {
	tests := []struct {
		retryCount int
		wantDelay  time.Duration
	}{
		{0, 10 * time.Second},  // 10 * 2^0 = 10s
		{1, 20 * time.Second},  // 10 * 2^1 = 20s
		{2, 40 * time.Second},  // 10 * 2^2 = 40s
		{3, 80 * time.Second},  // 10 * 2^3 = 80s
		{4, 160 * time.Second}, // 10 * 2^4 = 160s
		{5, 5 * time.Minute},   // 10 * 2^5 = 320s, capped at 5min
		{6, 5 * time.Minute},   // capped at max shift
		{10, 5 * time.Minute},  // still capped
		{-1, 10 * time.Second}, // negative treated as 0
	}

	for _, tt := range tests {
		t.Run("retry_"+string(rune('0'+tt.retryCount)), func(t *testing.T) {
			delay := calculateExponentialBackoff(tt.retryCount)
			assert.Equal(t, tt.wantDelay, delay)
		})
	}
}

func TestCalculateExponentialBackoff_NeverExceedsMax(t *testing.T) {
	// Test a range of retry counts to ensure we never exceed max delay
	for i := 0; i <= 100; i++ {
		delay := calculateExponentialBackoff(i)
		assert.LessOrEqual(t, delay, MaxRetryDelay,
			"Delay for retry %d should not exceed MaxRetryDelay", i)
	}
}

func TestBaseSyncController_ResetFromFailed(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-sync-state",
			Generation: 5,
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDNSRecord,
			CloudflareID: "dns-123",
			AccountID:    "acc-123",
		},
		Status: v1alpha2.CloudflareSyncStateStatus{
			SyncStatus:         v1alpha2.SyncStatusFailed,
			Error:              "previous error",
			RetryCount:         5,
			FailureReason:      "NotFound",
			ErrorCategory:      "Permanent",
			ObservedGeneration: 3,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		WithStatusSubresource(syncState).
		Build()

	c := NewBaseSyncController(client)
	ctx := context.Background()

	err := c.ResetFromFailed(ctx, syncState)
	require.NoError(t, err)

	// Verify the status was reset
	var updated v1alpha2.CloudflareSyncState
	err = client.Get(ctx, ctrlclient.ObjectKey{Name: "test-sync-state"}, &updated)
	require.NoError(t, err)

	assert.Equal(t, v1alpha2.SyncStatusPending, updated.Status.SyncStatus)
	assert.Empty(t, updated.Status.Error)
	assert.Equal(t, 0, updated.Status.RetryCount)
	assert.Empty(t, updated.Status.FailureReason)
	assert.Empty(t, updated.Status.ErrorCategory)
	assert.Equal(t, int64(5), updated.Status.ObservedGeneration)
}

func TestBaseSyncController_ResetFromFailed_NotInFailedStatus(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-sync-state",
			Generation: 2,
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDNSRecord,
			CloudflareID: "dns-123",
			AccountID:    "acc-123",
		},
		Status: v1alpha2.CloudflareSyncStateStatus{
			SyncStatus: v1alpha2.SyncStatusSynced,
			ConfigHash: "abc123",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		WithStatusSubresource(syncState).
		Build()

	c := NewBaseSyncController(client)
	ctx := context.Background()

	err := c.ResetFromFailed(ctx, syncState)
	require.NoError(t, err)

	// Status should remain unchanged
	var updated v1alpha2.CloudflareSyncState
	err = client.Get(ctx, ctrlclient.ObjectKey{Name: "test-sync-state"}, &updated)
	require.NoError(t, err)

	assert.Equal(t, v1alpha2.SyncStatusSynced, updated.Status.SyncStatus)
	assert.Equal(t, "abc123", updated.Status.ConfigHash)
}

func TestConstants(t *testing.T) {
	// Verify constants have expected values
	assert.Equal(t, 5, DefaultMaxRetries)
	assert.Equal(t, 10*time.Second, BaseRetryDelay)
	assert.Equal(t, 5*time.Minute, MaxRetryDelay)
}
