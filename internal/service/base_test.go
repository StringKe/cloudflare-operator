// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package service

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
)

func init() {
	_ = v1alpha2.AddToScheme(scheme.Scheme)
}

func TestNewBaseService(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	svc := NewBaseService(client)

	require.NotNil(t, svc)
	assert.NotNil(t, svc.Client)
}

func TestBaseService_GetOrCreateSyncState_Create(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	svc := NewBaseService(client)
	ctx := context.Background()

	credRef := v1alpha2.CredentialsReference{Name: "test-creds"}

	syncState, err := svc.GetOrCreateSyncState(
		ctx,
		v1alpha2.SyncResourceDNSRecord,
		"record-123",
		"account-123",
		"zone-123",
		credRef,
	)

	require.NoError(t, err)
	require.NotNil(t, syncState)
	assert.Equal(t, v1alpha2.SyncResourceDNSRecord, syncState.Spec.ResourceType)
	assert.Equal(t, "record-123", syncState.Spec.CloudflareID)
	assert.Equal(t, "account-123", syncState.Spec.AccountID)
	assert.Equal(t, "zone-123", syncState.Spec.ZoneID)
	assert.Equal(t, "test-creds", syncState.Spec.CredentialsRef.Name)
}

func TestBaseService_GetOrCreateSyncState_Get(t *testing.T) {
	existingSyncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "d-n-s-record-record-123",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDNSRecord,
			CloudflareID: "record-123",
			AccountID:    "existing-account",
			ZoneID:       "existing-zone",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(existingSyncState).
		Build()

	svc := NewBaseService(client)
	ctx := context.Background()

	credRef := v1alpha2.CredentialsReference{Name: "test-creds"}

	syncState, err := svc.GetOrCreateSyncState(
		ctx,
		v1alpha2.SyncResourceDNSRecord,
		"record-123",
		"new-account",
		"new-zone",
		credRef,
	)

	require.NoError(t, err)
	require.NotNil(t, syncState)
	// Should return existing, not create new
	assert.Equal(t, "existing-account", syncState.Spec.AccountID)
}

func TestBaseService_UpdateSource_AddNew(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sync",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDNSRecord,
			CloudflareID: "test-id",
			Sources:      []v1alpha2.ConfigSource{},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	svc := NewBaseService(client)
	ctx := context.Background()

	source := Source{Kind: "DNSRecord", Namespace: "test-ns", Name: "test-record"}
	config := map[string]string{"name": "test.example.com", "type": "A"}

	err := svc.UpdateSource(ctx, syncState, source, config, 100)

	require.NoError(t, err)

	// Verify source was added
	var updated v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: "test-sync"}, &updated)
	require.NoError(t, err)
	assert.Len(t, updated.Spec.Sources, 1)
	assert.Equal(t, "DNSRecord", updated.Spec.Sources[0].Ref.Kind)
	assert.Equal(t, "test-ns", updated.Spec.Sources[0].Ref.Namespace)
	assert.Equal(t, "test-record", updated.Spec.Sources[0].Ref.Name)
}

func TestBaseService_UpdateSource_UpdateExisting(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sync",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDNSRecord,
			CloudflareID: "test-id",
			Sources: []v1alpha2.ConfigSource{
				{
					Ref: v1alpha2.SourceReference{
						Kind:      "DNSRecord",
						Namespace: "test-ns",
						Name:      "test-record",
					},
					Priority: 100,
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	svc := NewBaseService(client)
	ctx := context.Background()

	source := Source{Kind: "DNSRecord", Namespace: "test-ns", Name: "test-record"}
	config := map[string]string{"name": "updated.example.com", "type": "CNAME"}

	err := svc.UpdateSource(ctx, syncState, source, config, 50)

	require.NoError(t, err)

	// Verify source was updated (not added)
	var updated v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: "test-sync"}, &updated)
	require.NoError(t, err)
	assert.Len(t, updated.Spec.Sources, 1) // Still only 1 source
	assert.Equal(t, 50, updated.Spec.Sources[0].Priority)
}

func TestBaseService_RemoveSource_StillHasSources(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sync",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDNSRecord,
			CloudflareID: "test-id",
			Sources: []v1alpha2.ConfigSource{
				{
					Ref: v1alpha2.SourceReference{
						Kind:      "DNSRecord",
						Namespace: "ns1",
						Name:      "record1",
					},
				},
				{
					Ref: v1alpha2.SourceReference{
						Kind:      "DNSRecord",
						Namespace: "ns2",
						Name:      "record2",
					},
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	svc := NewBaseService(client)
	ctx := context.Background()

	source := Source{Kind: "DNSRecord", Namespace: "ns1", Name: "record1"}

	err := svc.RemoveSource(ctx, syncState, source)

	require.NoError(t, err)

	// Verify one source was removed
	var updated v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: "test-sync"}, &updated)
	require.NoError(t, err)
	assert.Len(t, updated.Spec.Sources, 1)
	assert.Equal(t, "record2", updated.Spec.Sources[0].Ref.Name)
}

func TestBaseService_RemoveSource_LastSource(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sync",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDNSRecord,
			CloudflareID: "test-id",
			Sources: []v1alpha2.ConfigSource{
				{
					Ref: v1alpha2.SourceReference{
						Kind:      "DNSRecord",
						Namespace: "ns1",
						Name:      "record1",
					},
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	svc := NewBaseService(client)
	ctx := context.Background()

	source := Source{Kind: "DNSRecord", Namespace: "ns1", Name: "record1"}

	err := svc.RemoveSource(ctx, syncState, source)

	require.NoError(t, err)

	// Verify SyncState was deleted
	var updated v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: "test-sync"}, &updated)
	assert.Error(t, err) // Should be NotFound
}

func TestBaseService_GetSyncState_Found(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "d-n-s-record-record-123",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDNSRecord,
			CloudflareID: "record-123",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	svc := NewBaseService(client)
	ctx := context.Background()

	result, err := svc.GetSyncState(ctx, v1alpha2.SyncResourceDNSRecord, "record-123")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "record-123", result.Spec.CloudflareID)
}

func TestBaseService_GetSyncState_NotFound(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	svc := NewBaseService(client)
	ctx := context.Background()

	result, err := svc.GetSyncState(ctx, v1alpha2.SyncResourceDNSRecord, "non-existent")

	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestSyncStateName(t *testing.T) {
	tests := []struct {
		resourceType v1alpha2.SyncResourceType
		cloudflareID string
		expected     string
	}{
		{v1alpha2.SyncResourceDNSRecord, "record-123", "d-n-s-record-record-123"},
		{v1alpha2.SyncResourceTunnelConfiguration, "tunnel-456", "tunnel-configuration-tunnel-456"},
		{v1alpha2.SyncResourceAccessApplication, "app-789", "access-application-app-789"},
	}

	for _, tt := range tests {
		t.Run(string(tt.resourceType), func(t *testing.T) {
			result := SyncStateName(tt.resourceType, tt.cloudflareID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToKebabCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"DNSRecord", "d-n-s-record"},
		{"TunnelConfiguration", "tunnel-configuration"},
		{"AccessApplication", "access-application"},
		{"Simple", "simple"},
		{"lowercase", "lowercase"},
		{"ABC", "a-b-c"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toKebabCase(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"valid-name", "valid-name"},
		{"UPPERCASE", "uppercase"},
		{"with_underscore", "with-underscore"},
		{"with.dot", "with-dot"},
		{"with spaces", "with-spaces"},
		{"---leading-trailing---", "leading-trailing"},
		{"a-very-long-name-that-exceeds-the-kubernetes-limit-of-63-characters-should-be-truncated", "a-very-long-name-that-exceeds-the-kubernetes-limit-of-63-charac"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeLabelValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"valid-value", "valid-value"},
		{"-leading-dash", "leading-dash"},
		{"trailing-dash-", "trailing-dash"},
		{"a-very-long-label-value-that-exceeds-the-kubernetes-limit-of-63-characters", "a-very-long-label-value-that-exceeds-the-kubernetes-limit-of-63"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeLabelValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
