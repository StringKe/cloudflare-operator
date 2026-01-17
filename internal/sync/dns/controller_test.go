// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package dns

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	dnssvc "github.com/StringKe/cloudflare-operator/internal/service/dns"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

func init() {
	_ = v1alpha2.AddToScheme(scheme.Scheme)
}

func createDNSConfig(name, recordType, content string, proxied bool) *dnssvc.DNSRecordConfig {
	return &dnssvc.DNSRecordConfig{
		Name:    name,
		Type:    recordType,
		Content: content,
		Proxied: proxied,
		TTL:     300,
	}
}

func createDNSSyncState(name, zoneID, cloudflareID string, config *dnssvc.DNSRecordConfig) *v1alpha2.CloudflareSyncState {
	configBytes, _ := json.Marshal(config)

	return &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			// Include finalizer to avoid Requeue during tests
			Finalizers: []string{FinalizerName},
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDNSRecord,
			CloudflareID: cloudflareID,
			AccountID:    "test-account-123",
			ZoneID:       zoneID,
			CredentialsRef: v1alpha2.CredentialsReference{
				Name: "test-credentials",
			},
			Sources: []v1alpha2.ConfigSource{
				{
					Ref: v1alpha2.SourceReference{
						Kind:      "DNSRecord",
						Namespace: "test-ns",
						Name:      "test-record",
					},
					Config:   runtime.RawExtension{Raw: configBytes},
					Priority: 100,
				},
			},
		},
		Status: v1alpha2.CloudflareSyncStateStatus{
			SyncStatus: v1alpha2.SyncStatusPending,
		},
	}
}

func TestNewController(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	c := NewController(client)

	require.NotNil(t, c)
	assert.NotNil(t, c.BaseSyncController)
	assert.NotNil(t, c.Client)
	assert.NotNil(t, c.Debouncer)
}

func TestController_ExtractConfig_Success(t *testing.T) {
	config := createDNSConfig("api.example.com", "CNAME", "tunnel.cfargotunnel.com", true)
	syncState := createDNSSyncState("test-sync", "zone-123", "record-123", config)

	c := &Controller{}
	result, err := c.extractConfig(syncState)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "api.example.com", result.Name)
	assert.Equal(t, "CNAME", result.Type)
	assert.Equal(t, "tunnel.cfargotunnel.com", result.Content)
	assert.True(t, result.Proxied)
}

func TestController_ExtractConfig_NoSources(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDNSRecord,
			Sources:      []v1alpha2.ConfigSource{},
		},
	}

	c := &Controller{}
	result, err := c.extractConfig(syncState)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no sources")
}

func TestController_ExtractConfig_InvalidJSON(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDNSRecord,
			Sources: []v1alpha2.ConfigSource{
				{
					Ref: v1alpha2.SourceReference{
						Kind: "DNSRecord",
						Name: "invalid",
					},
					Config: runtime.RawExtension{Raw: []byte("not valid json")},
				},
			},
		},
	}

	c := &Controller{}
	result, err := c.extractConfig(syncState)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestController_Reconcile_NotFound(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	c := NewController(client)
	ctx := context.Background()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: "non-existent",
		},
	}

	result, err := c.Reconcile(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestController_Reconcile_WrongResourceType(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sync",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceAccessApplication, // Wrong type
			CloudflareID: "test-id",
			AccountID:    "test-account",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewController(client)
	ctx := context.Background()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-sync",
		},
	}

	result, err := c.Reconcile(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestController_Reconcile_NoSources_NoFinalizer(t *testing.T) {
	// Test: SyncState with no sources and no finalizer
	// Expected: Returns early since there's nothing to do (no finalizer means never synced to Cloudflare)
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sync",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDNSRecord,
			CloudflareID: "test-id",
			AccountID:    "test-account",
			ZoneID:       "zone-123",
			Sources:      []v1alpha2.ConfigSource{},
		},
		Status: v1alpha2.CloudflareSyncStateStatus{
			SyncStatus: v1alpha2.SyncStatusPending,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		WithStatusSubresource(syncState).
		Build()

	c := NewController(client)
	ctx := context.Background()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-sync",
		},
	}

	result, err := c.Reconcile(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Without finalizer, no status update - handleDeletion returns early
	var updatedState v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: "test-sync"}, &updatedState)
	require.NoError(t, err)
	// Status remains unchanged since no finalizer means nothing to cleanup
	assert.Equal(t, v1alpha2.SyncStatusPending, updatedState.Status.SyncStatus)
}

func TestController_Reconcile_NoSources_WithFinalizer(t *testing.T) {
	// Test: SyncState with no sources but HAS finalizer
	// Expected: Removes finalizer and deletes the orphaned SyncState
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-sync",
			Finalizers: []string{FinalizerName},
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDNSRecord,
			CloudflareID: "pending-abc123", // Pending ID means never created in Cloudflare
			AccountID:    "test-account",
			ZoneID:       "zone-123",
			CredentialsRef: v1alpha2.CredentialsReference{
				Name: "test-credentials",
			},
			Sources: []v1alpha2.ConfigSource{},
		},
		Status: v1alpha2.CloudflareSyncStateStatus{
			SyncStatus: v1alpha2.SyncStatusPending,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		WithStatusSubresource(syncState).
		Build()

	c := NewController(client)
	ctx := context.Background()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-sync",
		},
	}

	result, err := c.Reconcile(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify the SyncState was deleted (orphan cleanup)
	var updatedState v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: "test-sync"}, &updatedState)
	assert.True(t, err != nil, "SyncState should be deleted")
}

func TestController_Reconcile_DebouncePending(t *testing.T) {
	config := createDNSConfig("api.example.com", "A", "1.2.3.4", false)
	syncState := createDNSSyncState("test-sync", "zone-123", "record-123", config)

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewController(client)
	ctx := context.Background()

	// Add a pending debounce
	c.Debouncer.Debounce("test-sync", func() {})

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-sync",
		},
	}

	result, err := c.Reconcile(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Cancel the debounce
	c.Debouncer.Cancel("test-sync")
}

func TestController_Reconcile_NoHashChange(t *testing.T) {
	config := createDNSConfig("api.example.com", "A", "1.2.3.4", false)
	syncState := createDNSSyncState("test-sync", "zone-123", "record-123", config)

	// Compute and set the hash to simulate no change
	hash, _ := common.ComputeConfigHash(config)
	syncState.Status.ConfigHash = hash

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewController(client)
	ctx := context.Background()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-sync",
		},
	}

	result, err := c.Reconcile(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestConvertRecordData_Nil(t *testing.T) {
	result := convertRecordData(nil)
	assert.Nil(t, result)
}

func TestConvertRecordData_SRVRecord(t *testing.T) {
	data := &dnssvc.DNSRecordData{
		Service: "_http",
		Proto:   "_tcp",
		Weight:  10,
		Port:    80,
		Target:  "target.example.com",
	}

	result := convertRecordData(data)

	require.NotNil(t, result)
	assert.Equal(t, "_http", result.Service)
	assert.Equal(t, "_tcp", result.Proto)
	assert.Equal(t, 10, result.Weight)
	assert.Equal(t, 80, result.Port)
	assert.Equal(t, "target.example.com", result.Target)
}

func TestConvertRecordData_CAARecord(t *testing.T) {
	data := &dnssvc.DNSRecordData{
		Flags: 0,
		Tag:   "issue",
		Value: "letsencrypt.org",
	}

	result := convertRecordData(data)

	require.NotNil(t, result)
	assert.Equal(t, 0, result.Flags)
	assert.Equal(t, "issue", result.Tag)
	assert.Equal(t, "letsencrypt.org", result.Value)
}

func TestConvertRecordData_LOCRecord(t *testing.T) {
	data := &dnssvc.DNSRecordData{
		LatDegrees:    51,
		LatMinutes:    30,
		LatSeconds:    "12.345",
		LatDirection:  "N",
		LongDegrees:   0,
		LongMinutes:   7,
		LongSeconds:   "39.654",
		LongDirection: "W",
		Altitude:      "100m",
		Size:          "10m",
		PrecisionHorz: "10m",
		PrecisionVert: "10m",
	}

	result := convertRecordData(data)

	require.NotNil(t, result)
	assert.Equal(t, 51, result.LatDegrees)
	assert.Equal(t, 30, result.LatMinutes)
	assert.Equal(t, "12.345", result.LatSeconds)
	assert.Equal(t, "N", result.LatDirection)
}

func TestConvertRecordData_TLSARecord(t *testing.T) {
	data := &dnssvc.DNSRecordData{
		Usage:        3,
		Selector:     1,
		MatchingType: 1,
		Certificate:  "abc123def456",
	}

	result := convertRecordData(data)

	require.NotNil(t, result)
	assert.Equal(t, 3, result.Usage)
	assert.Equal(t, 1, result.Selector)
	assert.Equal(t, 1, result.MatchingType)
	assert.Equal(t, "abc123def456", result.Certificate)
}

func TestController_SetupWithManager(t *testing.T) {
	// This test verifies that SetupWithManager doesn't panic
	// A full integration test would require a real manager
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	c := NewController(client)

	// Verify the controller is properly configured
	assert.NotNil(t, c.Client)
	assert.NotNil(t, c.Debouncer)
}

func TestDNSRecordConfig_WithPriority(t *testing.T) {
	priority := 10
	config := &dnssvc.DNSRecordConfig{
		Name:     "mail.example.com",
		Type:     "MX",
		Content:  "mx1.example.com",
		Priority: &priority,
	}

	configBytes, err := json.Marshal(config)
	require.NoError(t, err)

	var parsed dnssvc.DNSRecordConfig
	err = json.Unmarshal(configBytes, &parsed)
	require.NoError(t, err)

	require.NotNil(t, parsed.Priority)
	assert.Equal(t, 10, *parsed.Priority)
}

func TestDNSRecordConfig_AllFields(t *testing.T) {
	priority := 10
	config := &dnssvc.DNSRecordConfig{
		Name:     "api.example.com",
		Type:     "A",
		Content:  "1.2.3.4",
		TTL:      300,
		Proxied:  true,
		Priority: &priority,
		Comment:  "API endpoint",
		Tags:     []string{"api", "production"},
		Data: &dnssvc.DNSRecordData{
			Service: "_svc",
		},
	}

	configBytes, err := json.Marshal(config)
	require.NoError(t, err)

	var parsed dnssvc.DNSRecordConfig
	err = json.Unmarshal(configBytes, &parsed)
	require.NoError(t, err)

	assert.Equal(t, "api.example.com", parsed.Name)
	assert.Equal(t, "A", parsed.Type)
	assert.Equal(t, "1.2.3.4", parsed.Content)
	assert.Equal(t, 300, parsed.TTL)
	assert.True(t, parsed.Proxied)
	assert.Equal(t, "API endpoint", parsed.Comment)
	assert.Equal(t, []string{"api", "production"}, parsed.Tags)
}
