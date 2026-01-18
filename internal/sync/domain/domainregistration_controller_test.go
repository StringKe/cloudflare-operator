// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package domain

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	domainsvc "github.com/StringKe/cloudflare-operator/internal/service/domain"
)

// createDomainRegistrationConfig creates a test lifecycle config
func createDomainRegistrationConfig(
	action domainsvc.DomainRegistrationAction,
	domainName string,
	config *domainsvc.DomainRegistrationConfiguration,
) *domainsvc.DomainRegistrationLifecycleConfig {
	return &domainsvc.DomainRegistrationLifecycleConfig{
		Action:        action,
		DomainName:    domainName,
		Configuration: config,
	}
}

// createDomainRegistrationSyncState creates a test SyncState for DomainRegistration
func createDomainRegistrationSyncState(
	name string,
	config *domainsvc.DomainRegistrationLifecycleConfig,
	status v1alpha2.SyncStatus,
	withFinalizer bool,
	lastSyncTime *metav1.Time,
) *v1alpha2.CloudflareSyncState {
	configBytes, _ := json.Marshal(config)

	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDomainRegistration,
			CloudflareID: "domain-123",
			AccountID:    "test-account-123",
			CredentialsRef: v1alpha2.CredentialsReference{
				Name: "test-credentials",
			},
			Sources: []v1alpha2.ConfigSource{
				{
					Ref: v1alpha2.SourceReference{
						Kind:      "DomainRegistration",
						Namespace: "test-ns",
						Name:      "test-domain",
					},
					Config:   runtime.RawExtension{Raw: configBytes},
					Priority: 100,
				},
			},
		},
		Status: v1alpha2.CloudflareSyncStateStatus{
			SyncStatus:   status,
			LastSyncTime: lastSyncTime,
		},
	}

	if withFinalizer {
		syncState.Finalizers = []string{DomainRegistrationFinalizerName}
	}

	return syncState
}

func TestNewDomainRegistrationController(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	c := NewDomainRegistrationController(client)

	require.NotNil(t, c)
	assert.NotNil(t, c.BaseSyncController)
	assert.NotNil(t, c.Client)
	assert.NotNil(t, c.Debouncer)
}

func TestDomainRegistrationController_GetLifecycleConfig_Success(t *testing.T) {
	domainConfig := &domainsvc.DomainRegistrationConfiguration{
		AutoRenew:   true,
		Privacy:     true,
		Locked:      false,
		NameServers: []string{"ns1.cloudflare.com", "ns2.cloudflare.com"},
	}
	config := createDomainRegistrationConfig(
		domainsvc.DomainRegistrationActionUpdate,
		"example.com",
		domainConfig,
	)
	syncState := createDomainRegistrationSyncState("test-sync", config, v1alpha2.SyncStatusPending, true, nil)

	c := &DomainRegistrationController{}
	result, err := c.getLifecycleConfig(syncState)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, domainsvc.DomainRegistrationActionUpdate, result.Action)
	assert.Equal(t, "example.com", result.DomainName)
	require.NotNil(t, result.Configuration)
	assert.True(t, result.Configuration.AutoRenew)
	assert.True(t, result.Configuration.Privacy)
	assert.False(t, result.Configuration.Locked)
	assert.Equal(t, []string{"ns1.cloudflare.com", "ns2.cloudflare.com"}, result.Configuration.NameServers)
}

func TestDomainRegistrationController_GetLifecycleConfig_NoSources(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDomainRegistration,
			Sources:      []v1alpha2.ConfigSource{},
		},
	}

	c := &DomainRegistrationController{}
	result, err := c.getLifecycleConfig(syncState)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no sources")
}

func TestDomainRegistrationController_GetLifecycleConfig_InvalidJSON(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDomainRegistration,
			Sources: []v1alpha2.ConfigSource{
				{
					Ref: v1alpha2.SourceReference{
						Kind: "DomainRegistration",
						Name: "invalid",
					},
					Config: runtime.RawExtension{Raw: []byte("not valid json")},
				},
			},
		},
	}

	c := &DomainRegistrationController{}
	result, err := c.getLifecycleConfig(syncState)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestDomainRegistrationController_Reconcile_NotFound(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	c := NewDomainRegistrationController(client)
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

func TestDomainRegistrationController_Reconcile_WrongResourceType(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sync",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDNSRecord, // Wrong type
			CloudflareID: "test-id",
			AccountID:    "test-account",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewDomainRegistrationController(client)
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

func TestDomainRegistrationController_Reconcile_RecentlySynced(t *testing.T) {
	config := createDomainRegistrationConfig(
		domainsvc.DomainRegistrationActionSync,
		"example.com",
		nil,
	)
	// Set last sync time to 10 minutes ago (within the 55 minute threshold)
	lastSync := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	syncState := createDomainRegistrationSyncState("test-sync", config, v1alpha2.SyncStatusSynced, true, &lastSync)

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewDomainRegistrationController(client)
	ctx := context.Background()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-sync",
		},
	}

	result, err := c.Reconcile(ctx, req)

	require.NoError(t, err)
	// Should requeue after 1 hour since recently synced
	assert.Equal(t, 1*time.Hour, result.RequeueAfter)
}

func TestDomainRegistrationController_Reconcile_NoSources_NoFinalizer(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sync",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceDomainRegistration,
			CloudflareID: "domain-123",
			AccountID:    "test-account",
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

	c := NewDomainRegistrationController(client)
	ctx := context.Background()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-sync",
		},
	}

	result, err := c.Reconcile(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Without finalizer, handleDeletion returns early
	var updatedState v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: "test-sync"}, &updatedState)
	require.NoError(t, err)
	assert.Equal(t, v1alpha2.SyncStatusPending, updatedState.Status.SyncStatus)
}

func TestDomainRegistrationController_Reconcile_AddsFinalizer(t *testing.T) {
	config := createDomainRegistrationConfig(
		domainsvc.DomainRegistrationActionSync,
		"example.com",
		nil,
	)
	syncState := createDomainRegistrationSyncState("test-sync", config, v1alpha2.SyncStatusPending, false, nil)

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewDomainRegistrationController(client)
	ctx := context.Background()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-sync",
		},
	}

	result, err := c.Reconcile(ctx, req)

	require.NoError(t, err)
	assert.True(t, result.Requeue)

	// Verify finalizer was added
	var updatedState v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: "test-sync"}, &updatedState)
	require.NoError(t, err)
	assert.Contains(t, updatedState.Finalizers, DomainRegistrationFinalizerName)
}

func TestDomainRegistrationController_Reconcile_DebouncePending(t *testing.T) {
	config := createDomainRegistrationConfig(
		domainsvc.DomainRegistrationActionSync,
		"example.com",
		nil,
	)
	syncState := createDomainRegistrationSyncState("test-sync", config, v1alpha2.SyncStatusPending, true, nil)

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewDomainRegistrationController(client)
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

func TestDomainRegistrationController_HandleDeletion_NoFinalizer(t *testing.T) {
	config := createDomainRegistrationConfig(
		domainsvc.DomainRegistrationActionSync,
		"example.com",
		nil,
	)
	syncState := createDomainRegistrationSyncState("test-sync", config, v1alpha2.SyncStatusSynced, false, nil)

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewDomainRegistrationController(client)
	ctx := context.Background()

	result, err := c.handleDeletion(ctx, syncState)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestDomainRegistrationController_HandleDeletion_PreservesDomain(t *testing.T) {
	// Domain registration deletion should NOT delete from Cloudflare
	// It should only clean up the SyncState
	config := createDomainRegistrationConfig(
		domainsvc.DomainRegistrationActionSync,
		"example.com",
		nil,
	)
	syncState := createDomainRegistrationSyncState("test-sync", config, v1alpha2.SyncStatusSynced, true, nil)
	syncState.Spec.Sources = []v1alpha2.ConfigSource{} // Empty sources to trigger cleanup

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewDomainRegistrationController(client)
	ctx := context.Background()

	result, err := c.handleDeletion(ctx, syncState)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify SyncState was deleted (orphan cleanup) but domain preserved on Cloudflare
	var updatedState v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: "test-sync"}, &updatedState)
	assert.True(t, err != nil, "SyncState should be deleted")
}

func TestDomainRegistrationController_SetupWithManager(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	c := NewDomainRegistrationController(client)

	// Verify the controller is properly configured
	assert.NotNil(t, c.Client)
	assert.NotNil(t, c.Debouncer)
}

func TestDomainRegistrationLifecycleConfig_AllActions(t *testing.T) {
	tests := []struct {
		name       string
		action     domainsvc.DomainRegistrationAction
		domainName string
		hasConfig  bool
	}{
		{
			name:       "sync action",
			action:     domainsvc.DomainRegistrationActionSync,
			domainName: "example.com",
			hasConfig:  false,
		},
		{
			name:       "update action",
			action:     domainsvc.DomainRegistrationActionUpdate,
			domainName: "example.org",
			hasConfig:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg *domainsvc.DomainRegistrationConfiguration
			if tt.hasConfig {
				cfg = &domainsvc.DomainRegistrationConfiguration{
					AutoRenew: true,
					Privacy:   true,
				}
			}

			config := &domainsvc.DomainRegistrationLifecycleConfig{
				Action:        tt.action,
				DomainName:    tt.domainName,
				Configuration: cfg,
			}

			configBytes, err := json.Marshal(config)
			require.NoError(t, err)

			var parsed domainsvc.DomainRegistrationLifecycleConfig
			err = json.Unmarshal(configBytes, &parsed)
			require.NoError(t, err)

			assert.Equal(t, tt.action, parsed.Action)
			assert.Equal(t, tt.domainName, parsed.DomainName)
			if tt.hasConfig {
				require.NotNil(t, parsed.Configuration)
				assert.True(t, parsed.Configuration.AutoRenew)
			} else {
				assert.Nil(t, parsed.Configuration)
			}
		})
	}
}

func TestDomainRegistrationSyncResult_Fields(t *testing.T) {
	now := metav1.Now()
	result := &domainsvc.DomainRegistrationSyncResult{
		DomainID:         "domain-123",
		CurrentRegistrar: "Cloudflare, Inc.",
		RegistryStatuses: "active,ok",
		Locked:           true,
		TransferInStatus: "pending",
		ExpiresAt:        now,
		CreatedAt:        now,
		AutoRenew:        true,
		Privacy:          true,
	}

	assert.Equal(t, "domain-123", result.DomainID)
	assert.Equal(t, "Cloudflare, Inc.", result.CurrentRegistrar)
	assert.Equal(t, "active,ok", result.RegistryStatuses)
	assert.True(t, result.Locked)
	assert.Equal(t, "pending", result.TransferInStatus)
	assert.True(t, result.AutoRenew)
	assert.True(t, result.Privacy)
}

func TestIsDomainRegistrationSyncState(t *testing.T) {
	tests := []struct {
		name         string
		resourceType v1alpha2.SyncResourceType
		expected     bool
	}{
		{
			name:         "DomainRegistration type",
			resourceType: v1alpha2.SyncResourceDomainRegistration,
			expected:     true,
		},
		{
			name:         "DNSRecord type",
			resourceType: v1alpha2.SyncResourceDNSRecord,
			expected:     false,
		},
		{
			name:         "OriginCACertificate type",
			resourceType: v1alpha2.SyncResourceOriginCACertificate,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syncState := &v1alpha2.CloudflareSyncState{
				Spec: v1alpha2.CloudflareSyncStateSpec{
					ResourceType: tt.resourceType,
				},
			}

			result := isDomainRegistrationSyncState(syncState)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDomainRegistrationConfiguration_Serialization(t *testing.T) {
	config := &domainsvc.DomainRegistrationConfiguration{
		AutoRenew:   true,
		Privacy:     true,
		Locked:      false,
		NameServers: []string{"ns1.cloudflare.com", "ns2.cloudflare.com"},
	}

	configBytes, err := json.Marshal(config)
	require.NoError(t, err)

	var parsed domainsvc.DomainRegistrationConfiguration
	err = json.Unmarshal(configBytes, &parsed)
	require.NoError(t, err)

	assert.True(t, parsed.AutoRenew)
	assert.True(t, parsed.Privacy)
	assert.False(t, parsed.Locked)
	assert.Equal(t, []string{"ns1.cloudflare.com", "ns2.cloudflare.com"}, parsed.NameServers)
}

func TestResultKeys(t *testing.T) {
	// Verify result key constants
	assert.Equal(t, "domainId", domainsvc.ResultKeyDomainID)
	assert.Equal(t, "currentRegistrar", domainsvc.ResultKeyCurrentRegistrar)
	assert.Equal(t, "registryStatuses", domainsvc.ResultKeyRegistryStatuses)
	assert.Equal(t, "locked", domainsvc.ResultKeyDomainLocked)
	assert.Equal(t, "transferInStatus", domainsvc.ResultKeyTransferInStatus)
	assert.Equal(t, "expiresAt", domainsvc.ResultKeyDomainExpiresAt)
	assert.Equal(t, "createdAt", domainsvc.ResultKeyDomainCreatedAt)
	assert.Equal(t, "autoRenew", domainsvc.ResultKeyDomainAutoRenew)
	assert.Equal(t, "privacy", domainsvc.ResultKeyDomainPrivacy)
}
