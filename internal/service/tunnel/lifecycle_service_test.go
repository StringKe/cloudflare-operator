// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package tunnel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

func init() {
	_ = v1alpha2.AddToScheme(scheme.Scheme)
}

func TestNewLifecycleService(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	s := NewLifecycleService(client)

	require.NotNil(t, s)
	assert.NotNil(t, s.BaseService)
	assert.NotNil(t, s.Client)
}

func TestLifecycleService_RequestCreate_NewSyncState(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	s := NewLifecycleService(client)
	ctx := context.Background()

	opts := CreateTunnelOptions{
		TunnelName: "my-tunnel",
		AccountID:  "account-123",
		ConfigSrc:  "local",
		Source: service.Source{
			Kind:      "Tunnel",
			Namespace: "test-ns",
			Name:      "my-tunnel",
		},
		CredentialsRef: v1alpha2.CredentialsReference{
			Name: "cloudflare-creds",
		},
	}

	syncStateName, err := s.RequestCreate(ctx, opts)

	require.NoError(t, err)
	// SyncStateName generates: {resourceType-kebab}-{cloudflareID}
	// = "tunnel-lifecycle-tunnel-lifecycle-my-tunnel"
	assert.Equal(t, "tunnel-lifecycle-tunnel-lifecycle-my-tunnel", syncStateName)

	// Verify SyncState was created
	var syncState v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: syncStateName}, &syncState)
	require.NoError(t, err)

	assert.Equal(t, v1alpha2.SyncResourceTunnelLifecycle, syncState.Spec.ResourceType)
	assert.Equal(t, "account-123", syncState.Spec.AccountID)
	assert.Equal(t, "cloudflare-creds", syncState.Spec.CredentialsRef.Name)
	assert.Len(t, syncState.Spec.Sources, 1)
	assert.Equal(t, "Tunnel", syncState.Spec.Sources[0].Ref.Kind)
	assert.Equal(t, "test-ns", syncState.Spec.Sources[0].Ref.Namespace)
	assert.Equal(t, "my-tunnel", syncState.Spec.Sources[0].Ref.Name)
	assert.Equal(t, PriorityTunnelSettings, syncState.Spec.Sources[0].Priority)
}

func TestLifecycleService_RequestCreate_ExistingSyncState(t *testing.T) {
	// Pre-create a SyncState with the correct name format
	// SyncStateName generates: {resourceType-kebab}-{cloudflareID}
	existingSyncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tunnel-lifecycle-tunnel-lifecycle-my-tunnel",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceTunnelLifecycle,
			CloudflareID: "tunnel-lifecycle-my-tunnel",
			AccountID:    "account-123",
			CredentialsRef: v1alpha2.CredentialsReference{
				Name: "cloudflare-creds",
			},
			Sources: []v1alpha2.ConfigSource{},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(existingSyncState).
		Build()

	s := NewLifecycleService(client)
	ctx := context.Background()

	opts := CreateTunnelOptions{
		TunnelName: "my-tunnel",
		AccountID:  "account-123",
		ConfigSrc:  "local",
		Source: service.Source{
			Kind:      "Tunnel",
			Namespace: "test-ns",
			Name:      "my-tunnel",
		},
		CredentialsRef: v1alpha2.CredentialsReference{
			Name: "cloudflare-creds",
		},
	}

	syncStateName, err := s.RequestCreate(ctx, opts)

	require.NoError(t, err)
	assert.Equal(t, "tunnel-lifecycle-tunnel-lifecycle-my-tunnel", syncStateName)

	// Verify source was added
	var syncState v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: syncStateName}, &syncState)
	require.NoError(t, err)

	assert.Len(t, syncState.Spec.Sources, 1)
}

func TestLifecycleService_RequestDelete(t *testing.T) {
	// Pre-create a SyncState for the tunnel with correct name format
	existingSyncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tunnel-lifecycle-tunnel-lifecycle-my-tunnel",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceTunnelLifecycle,
			CloudflareID: "tunnel-lifecycle-my-tunnel",
			AccountID:    "account-123",
			CredentialsRef: v1alpha2.CredentialsReference{
				Name: "cloudflare-creds",
			},
			Sources: []v1alpha2.ConfigSource{},
		},
		Status: v1alpha2.CloudflareSyncStateStatus{
			SyncStatus: v1alpha2.SyncStatusSynced,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(existingSyncState).
		Build()

	s := NewLifecycleService(client)
	ctx := context.Background()

	opts := DeleteTunnelOptions{
		TunnelID:   "tunnel-abc-123",
		TunnelName: "my-tunnel",
		AccountID:  "account-123",
		Source: service.Source{
			Kind:      "Tunnel",
			Namespace: "test-ns",
			Name:      "my-tunnel",
		},
		CredentialsRef: v1alpha2.CredentialsReference{
			Name: "cloudflare-creds",
		},
	}

	syncStateName, err := s.RequestDelete(ctx, opts)

	require.NoError(t, err)
	assert.Equal(t, "tunnel-lifecycle-tunnel-lifecycle-my-tunnel", syncStateName)

	// Verify the source was updated with delete action
	var syncState v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: syncStateName}, &syncState)
	require.NoError(t, err)

	assert.Len(t, syncState.Spec.Sources, 1)
}

func TestLifecycleService_RequestDelete_NoExistingSyncState(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	s := NewLifecycleService(client)
	ctx := context.Background()

	opts := DeleteTunnelOptions{
		TunnelID:   "tunnel-abc-123",
		TunnelName: "my-tunnel",
		AccountID:  "account-123",
		Source: service.Source{
			Kind:      "Tunnel",
			Namespace: "test-ns",
			Name:      "my-tunnel",
		},
		CredentialsRef: v1alpha2.CredentialsReference{
			Name: "cloudflare-creds",
		},
	}

	syncStateName, err := s.RequestDelete(ctx, opts)

	require.NoError(t, err)
	assert.Equal(t, "tunnel-lifecycle-tunnel-lifecycle-my-tunnel", syncStateName)

	// Verify SyncState was created for deletion
	var syncState v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: syncStateName}, &syncState)
	require.NoError(t, err)

	assert.Len(t, syncState.Spec.Sources, 1)
}

func TestLifecycleService_RequestAdopt(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	s := NewLifecycleService(client)
	ctx := context.Background()

	opts := AdoptTunnelOptions{
		TunnelID:   "existing-tunnel-id",
		TunnelName: "existing-tunnel",
		AccountID:  "account-123",
		Source: service.Source{
			Kind:      "Tunnel",
			Namespace: "test-ns",
			Name:      "my-tunnel",
		},
		CredentialsRef: v1alpha2.CredentialsReference{
			Name: "cloudflare-creds",
		},
	}

	syncStateName, err := s.RequestAdopt(ctx, opts)

	require.NoError(t, err)
	assert.Equal(t, "tunnel-lifecycle-tunnel-lifecycle-existing-tunnel", syncStateName)

	// Verify SyncState was created
	var syncState v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: syncStateName}, &syncState)
	require.NoError(t, err)

	assert.Equal(t, v1alpha2.SyncResourceTunnelLifecycle, syncState.Spec.ResourceType)
	assert.Len(t, syncState.Spec.Sources, 1)
}

func TestLifecycleService_GetLifecycleResult_NotFound(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	s := NewLifecycleService(client)
	ctx := context.Background()

	result, err := s.GetLifecycleResult(ctx, "non-existent")

	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestLifecycleService_GetLifecycleResult_NotSynced(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tunnel-lifecycle-tunnel-lifecycle-my-tunnel",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceTunnelLifecycle,
			CloudflareID: "tunnel-lifecycle-my-tunnel",
			AccountID:    "account-123",
		},
		Status: v1alpha2.CloudflareSyncStateStatus{
			SyncStatus: v1alpha2.SyncStatusPending, // Not synced yet
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	s := NewLifecycleService(client)
	ctx := context.Background()

	result, err := s.GetLifecycleResult(ctx, "my-tunnel")

	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestLifecycleService_GetLifecycleResult_Synced(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tunnel-lifecycle-tunnel-lifecycle-my-tunnel",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceTunnelLifecycle,
			CloudflareID: "tunnel-lifecycle-my-tunnel",
			AccountID:    "account-123",
		},
		Status: v1alpha2.CloudflareSyncStateStatus{
			SyncStatus: v1alpha2.SyncStatusSynced,
			ResultData: map[string]string{
				ResultKeyTunnelID:    "tunnel-123",
				ResultKeyTunnelName:  "my-tunnel",
				ResultKeyTunnelToken: "secret-token",
				ResultKeyCredentials: "base64-creds",
				ResultKeyAccountTag:  "account-tag",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	s := NewLifecycleService(client)
	ctx := context.Background()

	result, err := s.GetLifecycleResult(ctx, "my-tunnel")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "tunnel-123", result.TunnelID)
	assert.Equal(t, "my-tunnel", result.TunnelName)
	assert.Equal(t, "secret-token", result.TunnelToken)
	assert.Equal(t, "base64-creds", result.Credentials)
	assert.Equal(t, "account-tag", result.AccountTag)
}

func TestLifecycleService_IsLifecycleCompleted_NotFound(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	s := NewLifecycleService(client)
	ctx := context.Background()

	completed, err := s.IsLifecycleCompleted(ctx, "non-existent")

	require.NoError(t, err)
	assert.False(t, completed)
}

func TestLifecycleService_IsLifecycleCompleted_Pending(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tunnel-lifecycle-tunnel-lifecycle-my-tunnel",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceTunnelLifecycle,
			CloudflareID: "tunnel-lifecycle-my-tunnel",
		},
		Status: v1alpha2.CloudflareSyncStateStatus{
			SyncStatus: v1alpha2.SyncStatusPending,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	s := NewLifecycleService(client)
	ctx := context.Background()

	completed, err := s.IsLifecycleCompleted(ctx, "my-tunnel")

	require.NoError(t, err)
	assert.False(t, completed)
}

func TestLifecycleService_IsLifecycleCompleted_Synced(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tunnel-lifecycle-tunnel-lifecycle-my-tunnel",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceTunnelLifecycle,
			CloudflareID: "tunnel-lifecycle-my-tunnel",
		},
		Status: v1alpha2.CloudflareSyncStateStatus{
			SyncStatus: v1alpha2.SyncStatusSynced,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	s := NewLifecycleService(client)
	ctx := context.Background()

	completed, err := s.IsLifecycleCompleted(ctx, "my-tunnel")

	require.NoError(t, err)
	assert.True(t, completed)
}

func TestLifecycleService_GetLifecycleError_NoError(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tunnel-lifecycle-tunnel-lifecycle-my-tunnel",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceTunnelLifecycle,
			CloudflareID: "tunnel-lifecycle-my-tunnel",
		},
		Status: v1alpha2.CloudflareSyncStateStatus{
			SyncStatus: v1alpha2.SyncStatusSynced,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	s := NewLifecycleService(client)
	ctx := context.Background()

	errMsg, err := s.GetLifecycleError(ctx, "my-tunnel")

	require.NoError(t, err)
	assert.Empty(t, errMsg)
}

func TestLifecycleService_GetLifecycleError_HasError(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tunnel-lifecycle-tunnel-lifecycle-my-tunnel",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceTunnelLifecycle,
			CloudflareID: "tunnel-lifecycle-my-tunnel",
		},
		Status: v1alpha2.CloudflareSyncStateStatus{
			SyncStatus: v1alpha2.SyncStatusError,
			Error:      "failed to create tunnel: API error",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	s := NewLifecycleService(client)
	ctx := context.Background()

	errMsg, err := s.GetLifecycleError(ctx, "my-tunnel")

	require.NoError(t, err)
	assert.Equal(t, "failed to create tunnel: API error", errMsg)
}

func TestLifecycleService_CleanupSyncState(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tunnel-lifecycle-tunnel-lifecycle-my-tunnel",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceTunnelLifecycle,
			CloudflareID: "tunnel-lifecycle-my-tunnel",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	s := NewLifecycleService(client)
	ctx := context.Background()

	err := s.CleanupSyncState(ctx, "my-tunnel")
	require.NoError(t, err)

	// Verify SyncState was deleted
	var deletedState v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: "tunnel-lifecycle-tunnel-lifecycle-my-tunnel"}, &deletedState)
	assert.True(t, err != nil, "SyncState should be deleted")
}

func TestLifecycleService_CleanupSyncState_NotFound(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	s := NewLifecycleService(client)
	ctx := context.Background()

	err := s.CleanupSyncState(ctx, "non-existent")

	require.NoError(t, err)
}

func TestGetSyncStateName(t *testing.T) {
	tests := []struct {
		tunnelName string
		expected   string
	}{
		{"my-tunnel", "tunnel-lifecycle-my-tunnel"},
		{"test", "tunnel-lifecycle-test"},
		{"production-tunnel-1", "tunnel-lifecycle-production-tunnel-1"},
	}

	for _, tt := range tests {
		t.Run(tt.tunnelName, func(t *testing.T) {
			result := GetSyncStateName(tt.tunnelName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseLifecycleConfig_Valid(t *testing.T) {
	configJSON := `{
		"action": "create",
		"tunnelName": "my-tunnel",
		"configSrc": "local"
	}`

	config, err := ParseLifecycleConfig([]byte(configJSON))

	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, LifecycleActionCreate, config.Action)
	assert.Equal(t, "my-tunnel", config.TunnelName)
	assert.Equal(t, "local", config.ConfigSrc)
}

func TestParseLifecycleConfig_Invalid(t *testing.T) {
	config, err := ParseLifecycleConfig([]byte("not json"))

	assert.Error(t, err)
	assert.Nil(t, config)
}

func TestParseLifecycleConfig_AllFields(t *testing.T) {
	configJSON := `{
		"action": "adopt",
		"tunnelName": "my-tunnel",
		"tunnelId": "tunnel-123",
		"configSrc": "cloudflare",
		"existingTunnelId": "tunnel-existing"
	}`

	config, err := ParseLifecycleConfig([]byte(configJSON))

	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, LifecycleActionAdopt, config.Action)
	assert.Equal(t, "my-tunnel", config.TunnelName)
	assert.Equal(t, "tunnel-123", config.TunnelID)
	assert.Equal(t, "cloudflare", config.ConfigSrc)
	assert.Equal(t, "tunnel-existing", config.ExistingTunnelID)
}
