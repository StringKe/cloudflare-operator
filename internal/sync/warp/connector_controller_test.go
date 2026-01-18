// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package warp

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
	warpsvc "github.com/StringKe/cloudflare-operator/internal/service/warp"
)

func init() {
	_ = v1alpha2.AddToScheme(scheme.Scheme)
}

// createConnectorConfig creates a test connector lifecycle config
func createConnectorConfig(
	action warpsvc.ConnectorAction,
	connectorName, connectorID, tunnelID, vnetID string,
	routes []warpsvc.RouteConfig,
) *warpsvc.ConnectorLifecycleConfig {
	return &warpsvc.ConnectorLifecycleConfig{
		Action:           action,
		ConnectorName:    connectorName,
		ConnectorID:      connectorID,
		TunnelID:         tunnelID,
		VirtualNetworkID: vnetID,
		Routes:           routes,
	}
}

// createConnectorSyncState creates a test SyncState for WARP connector lifecycle
func createConnectorSyncState(
	name string,
	config *warpsvc.ConnectorLifecycleConfig,
	status v1alpha2.SyncStatus,
	withFinalizer bool,
) *v1alpha2.CloudflareSyncState {
	configBytes, _ := json.Marshal(config)

	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceWARPConnector,
			CloudflareID: config.ConnectorID,
			AccountID:    "test-account-123",
			CredentialsRef: v1alpha2.CredentialsReference{
				Name: "test-credentials",
			},
			Sources: []v1alpha2.ConfigSource{
				{
					Ref: v1alpha2.SourceReference{
						Kind:      "WARPConnector",
						Namespace: "test-ns",
						Name:      "test-connector",
					},
					Config:   runtime.RawExtension{Raw: configBytes},
					Priority: 100,
				},
			},
		},
		Status: v1alpha2.CloudflareSyncStateStatus{
			SyncStatus: status,
		},
	}

	if withFinalizer {
		syncState.Finalizers = []string{WARPConnectorFinalizerName}
	}

	return syncState
}

func TestNewConnectorController(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	c := NewConnectorController(client)

	require.NotNil(t, c)
	assert.NotNil(t, c.BaseSyncController)
	assert.NotNil(t, c.Client)
	assert.NotNil(t, c.Debouncer)
}

func TestConnectorController_GetLifecycleConfig_Success(t *testing.T) {
	routes := []warpsvc.RouteConfig{
		{Network: "10.0.0.0/24", Comment: "Test route"},
	}
	config := createConnectorConfig(
		warpsvc.ConnectorActionCreate,
		"my-connector",
		"",
		"",
		"vnet-123",
		routes,
	)
	syncState := createConnectorSyncState("test-sync", config, v1alpha2.SyncStatusPending, true)

	c := &ConnectorController{}
	result, err := c.getLifecycleConfig(syncState)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, warpsvc.ConnectorActionCreate, result.Action)
	assert.Equal(t, "my-connector", result.ConnectorName)
	assert.Equal(t, "vnet-123", result.VirtualNetworkID)
	assert.Len(t, result.Routes, 1)
	assert.Equal(t, "10.0.0.0/24", result.Routes[0].Network)
}

func TestConnectorController_GetLifecycleConfig_NoSources(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceWARPConnector,
			Sources:      []v1alpha2.ConfigSource{},
		},
	}

	c := &ConnectorController{}
	result, err := c.getLifecycleConfig(syncState)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no sources")
}

func TestConnectorController_GetLifecycleConfig_InvalidJSON(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceWARPConnector,
			Sources: []v1alpha2.ConfigSource{
				{
					Ref: v1alpha2.SourceReference{
						Kind: "WARPConnector",
						Name: "invalid",
					},
					Config: runtime.RawExtension{Raw: []byte("not valid json")},
				},
			},
		},
	}

	c := &ConnectorController{}
	result, err := c.getLifecycleConfig(syncState)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestConnectorController_Reconcile_NotFound(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	c := NewConnectorController(client)
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

func TestConnectorController_Reconcile_WrongResourceType(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sync",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceTunnelLifecycle, // Wrong type
			CloudflareID: "test-id",
			AccountID:    "test-account",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewConnectorController(client)
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

func TestConnectorController_Reconcile_AlreadySynced(t *testing.T) {
	routes := []warpsvc.RouteConfig{
		{Network: "10.0.0.0/24"},
	}
	config := createConnectorConfig(
		warpsvc.ConnectorActionCreate,
		"my-connector",
		"connector-123",
		"tunnel-456",
		"vnet-789",
		routes,
	)
	syncState := createConnectorSyncState("test-sync", config, v1alpha2.SyncStatusSynced, true)

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewConnectorController(client)
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

func TestConnectorController_Reconcile_NoSources_NoFinalizer(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sync",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceWARPConnector,
			CloudflareID: "pending-abc123",
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

	c := NewConnectorController(client)
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

func TestConnectorController_Reconcile_AddsFinalizer(t *testing.T) {
	routes := []warpsvc.RouteConfig{
		{Network: "10.0.0.0/24"},
	}
	config := createConnectorConfig(
		warpsvc.ConnectorActionCreate,
		"my-connector",
		"",
		"",
		"vnet-123",
		routes,
	)
	syncState := createConnectorSyncState("test-sync", config, v1alpha2.SyncStatusPending, false)

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewConnectorController(client)
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
	assert.Contains(t, updatedState.Finalizers, WARPConnectorFinalizerName)
}

func TestConnectorController_Reconcile_DebouncePending(t *testing.T) {
	routes := []warpsvc.RouteConfig{
		{Network: "10.0.0.0/24"},
	}
	config := createConnectorConfig(
		warpsvc.ConnectorActionCreate,
		"my-connector",
		"",
		"",
		"vnet-123",
		routes,
	)
	syncState := createConnectorSyncState("test-sync", config, v1alpha2.SyncStatusPending, true)

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewConnectorController(client)
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

func TestConnectorController_HandleDeletion_NoFinalizer(t *testing.T) {
	routes := []warpsvc.RouteConfig{
		{Network: "10.0.0.0/24"},
	}
	config := createConnectorConfig(
		warpsvc.ConnectorActionCreate,
		"my-connector",
		"connector-123",
		"tunnel-456",
		"vnet-789",
		routes,
	)
	syncState := createConnectorSyncState("test-sync", config, v1alpha2.SyncStatusSynced, false)

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewConnectorController(client)
	ctx := context.Background()

	result, err := c.handleDeletion(ctx, syncState)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestConnectorController_HandleDeletion_PendingID(t *testing.T) {
	config := createConnectorConfig(
		warpsvc.ConnectorActionCreate,
		"my-connector",
		"pending-123",
		"",
		"vnet-123",
		nil,
	)
	syncState := createConnectorSyncState("test-sync", config, v1alpha2.SyncStatusPending, true)
	syncState.Spec.CloudflareID = "pending-123"
	syncState.Spec.Sources = []v1alpha2.ConfigSource{} // Empty sources to trigger cleanup

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewConnectorController(client)
	ctx := context.Background()

	result, err := c.handleDeletion(ctx, syncState)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify SyncState was deleted (orphan cleanup)
	var updatedState v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: "test-sync"}, &updatedState)
	assert.True(t, err != nil, "SyncState should be deleted")
}

func TestConnectorController_SetupWithManager(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	c := NewConnectorController(client)

	// Verify the controller is properly configured
	assert.NotNil(t, c.Client)
	assert.NotNil(t, c.Debouncer)
}

func TestConnectorLifecycleConfig_AllActions(t *testing.T) {
	tests := []struct {
		name          string
		action        warpsvc.ConnectorAction
		connectorName string
		connectorID   string
		tunnelID      string
	}{
		{
			name:          "create action",
			action:        warpsvc.ConnectorActionCreate,
			connectorName: "new-connector",
			connectorID:   "",
			tunnelID:      "",
		},
		{
			name:          "delete action",
			action:        warpsvc.ConnectorActionDelete,
			connectorName: "existing-connector",
			connectorID:   "connector-abc-123",
			tunnelID:      "tunnel-xyz-789",
		},
		{
			name:          "update action",
			action:        warpsvc.ConnectorActionUpdate,
			connectorName: "updated-connector",
			connectorID:   "connector-def-456",
			tunnelID:      "tunnel-uvw-012",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &warpsvc.ConnectorLifecycleConfig{
				Action:        tt.action,
				ConnectorName: tt.connectorName,
				ConnectorID:   tt.connectorID,
				TunnelID:      tt.tunnelID,
			}

			configBytes, err := json.Marshal(config)
			require.NoError(t, err)

			var parsed warpsvc.ConnectorLifecycleConfig
			err = json.Unmarshal(configBytes, &parsed)
			require.NoError(t, err)

			assert.Equal(t, tt.action, parsed.Action)
			assert.Equal(t, tt.connectorName, parsed.ConnectorName)
			assert.Equal(t, tt.connectorID, parsed.ConnectorID)
			assert.Equal(t, tt.tunnelID, parsed.TunnelID)
		})
	}
}

func TestConnectorLifecycleResult_Serialization(t *testing.T) {
	result := &warpsvc.ConnectorLifecycleResult{
		ConnectorID:      "connector-123",
		TunnelID:         "tunnel-456",
		TunnelToken:      "secret-token",
		RoutesConfigured: 3,
	}

	resultBytes, err := json.Marshal(result)
	require.NoError(t, err)

	var parsed warpsvc.ConnectorLifecycleResult
	err = json.Unmarshal(resultBytes, &parsed)
	require.NoError(t, err)

	assert.Equal(t, result.ConnectorID, parsed.ConnectorID)
	assert.Equal(t, result.TunnelID, parsed.TunnelID)
	assert.Equal(t, result.TunnelToken, parsed.TunnelToken)
	assert.Equal(t, result.RoutesConfigured, parsed.RoutesConfigured)
}

func TestRouteConfig_Serialization(t *testing.T) {
	routes := []warpsvc.RouteConfig{
		{Network: "10.0.0.0/24", Comment: "Internal network"},
		{Network: "192.168.0.0/16", Comment: "Private network"},
	}

	routesBytes, err := json.Marshal(routes)
	require.NoError(t, err)

	var parsed []warpsvc.RouteConfig
	err = json.Unmarshal(routesBytes, &parsed)
	require.NoError(t, err)

	assert.Len(t, parsed, 2)
	assert.Equal(t, "10.0.0.0/24", parsed[0].Network)
	assert.Equal(t, "Internal network", parsed[0].Comment)
	assert.Equal(t, "192.168.0.0/16", parsed[1].Network)
	assert.Equal(t, "Private network", parsed[1].Comment)
}

func TestIsWARPConnectorSyncState(t *testing.T) {
	tests := []struct {
		name         string
		resourceType v1alpha2.SyncResourceType
		expected     bool
	}{
		{
			name:         "WARPConnector type",
			resourceType: v1alpha2.SyncResourceWARPConnector,
			expected:     true,
		},
		{
			name:         "DNSRecord type",
			resourceType: v1alpha2.SyncResourceDNSRecord,
			expected:     false,
		},
		{
			name:         "TunnelLifecycle type",
			resourceType: v1alpha2.SyncResourceTunnelLifecycle,
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

			result := isWARPConnectorSyncState(syncState)
			assert.Equal(t, tt.expected, result)
		})
	}
}
