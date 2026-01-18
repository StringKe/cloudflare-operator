// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package warp

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

func TestNewConnectorService(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	s := NewConnectorService(client)

	require.NotNil(t, s)
	assert.NotNil(t, s.BaseService)
	assert.NotNil(t, s.Client)
}

func TestConnectorService_RequestCreate(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	s := NewConnectorService(client)
	ctx := context.Background()

	routes := []RouteConfig{
		{Network: "10.0.0.0/24", Comment: "Internal network"},
		{Network: "192.168.1.0/24", Comment: "Private network"},
	}

	opts := CreateConnectorOptions{
		ConnectorName:    "my-connector",
		AccountID:        "account-123",
		VirtualNetworkID: "vnet-456",
		Routes:           routes,
		Source: service.Source{
			Kind:      "WARPConnector",
			Namespace: "test-ns",
			Name:      "my-connector",
		},
		CredentialsRef: v1alpha2.CredentialsReference{
			Name: "cloudflare-creds",
		},
	}

	err := s.RequestCreate(ctx, opts)

	require.NoError(t, err)

	// Verify SyncState was created
	// SyncStateName generates: {resourceType-kebab}-{cloudflareID}
	// toKebabCase("WARPConnector") = "w-a-r-p-connector"
	var syncState v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: "w-a-r-p-connector-warpconnector-my-connector"}, &syncState)
	require.NoError(t, err)

	assert.Equal(t, v1alpha2.SyncResourceWARPConnector, syncState.Spec.ResourceType)
	assert.Equal(t, "account-123", syncState.Spec.AccountID)
	assert.Equal(t, "cloudflare-creds", syncState.Spec.CredentialsRef.Name)
	assert.Len(t, syncState.Spec.Sources, 1)
	assert.Equal(t, "WARPConnector", syncState.Spec.Sources[0].Ref.Kind)
	assert.Equal(t, service.PriorityDefault, syncState.Spec.Sources[0].Priority)
}

func TestConnectorService_RequestCreate_ExistingSyncState(t *testing.T) {
	// Pre-create a SyncState with correct name format
	existingSyncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "w-a-r-p-connector-warpconnector-my-connector",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceWARPConnector,
			CloudflareID: "warpconnector-my-connector",
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

	s := NewConnectorService(client)
	ctx := context.Background()

	opts := CreateConnectorOptions{
		ConnectorName:    "my-connector",
		AccountID:        "account-123",
		VirtualNetworkID: "vnet-456",
		Routes:           []RouteConfig{{Network: "10.0.0.0/24"}},
		Source: service.Source{
			Kind:      "WARPConnector",
			Namespace: "test-ns",
			Name:      "my-connector",
		},
		CredentialsRef: v1alpha2.CredentialsReference{
			Name: "cloudflare-creds",
		},
	}

	err := s.RequestCreate(ctx, opts)

	require.NoError(t, err)

	// Verify source was added
	var syncState v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: "w-a-r-p-connector-warpconnector-my-connector"}, &syncState)
	require.NoError(t, err)

	assert.Len(t, syncState.Spec.Sources, 1)
}

func TestConnectorService_RequestDelete(t *testing.T) {
	// Pre-create a SyncState for the connector with correct name format
	existingSyncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "w-a-r-p-connector-warpconnector-my-connector",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceWARPConnector,
			CloudflareID: "warpconnector-my-connector",
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

	s := NewConnectorService(client)
	ctx := context.Background()

	routes := []RouteConfig{
		{Network: "10.0.0.0/24", Comment: "To delete"},
	}

	opts := DeleteConnectorOptions{
		ConnectorID:      "connector-abc-123",
		ConnectorName:    "my-connector",
		TunnelID:         "tunnel-xyz-789",
		VirtualNetworkID: "vnet-456",
		AccountID:        "account-123",
		Routes:           routes,
		Source: service.Source{
			Kind:      "WARPConnector",
			Namespace: "test-ns",
			Name:      "my-connector",
		},
		CredentialsRef: v1alpha2.CredentialsReference{
			Name: "cloudflare-creds",
		},
	}

	err := s.RequestDelete(ctx, opts)

	require.NoError(t, err)

	// Verify the source was updated with delete action
	var syncState v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: "w-a-r-p-connector-warpconnector-my-connector"}, &syncState)
	require.NoError(t, err)

	assert.Len(t, syncState.Spec.Sources, 1)
}

func TestConnectorService_RequestDelete_NewSyncState(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	s := NewConnectorService(client)
	ctx := context.Background()

	opts := DeleteConnectorOptions{
		ConnectorID:      "connector-abc-123",
		ConnectorName:    "my-connector",
		TunnelID:         "tunnel-xyz-789",
		VirtualNetworkID: "vnet-456",
		AccountID:        "account-123",
		Routes:           []RouteConfig{{Network: "10.0.0.0/24"}},
		Source: service.Source{
			Kind:      "WARPConnector",
			Namespace: "test-ns",
			Name:      "my-connector",
		},
		CredentialsRef: v1alpha2.CredentialsReference{
			Name: "cloudflare-creds",
		},
	}

	err := s.RequestDelete(ctx, opts)

	require.NoError(t, err)

	// Verify SyncState was created for deletion
	var syncState v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: "w-a-r-p-connector-warpconnector-my-connector"}, &syncState)
	require.NoError(t, err)

	assert.Len(t, syncState.Spec.Sources, 1)
}

func TestConnectorService_RequestUpdate(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	s := NewConnectorService(client)
	ctx := context.Background()

	routes := []RouteConfig{
		{Network: "10.0.0.0/24", Comment: "Updated route 1"},
		{Network: "172.16.0.0/16", Comment: "Updated route 2"},
	}

	opts := UpdateConnectorOptions{
		ConnectorID:      "connector-abc-123",
		ConnectorName:    "my-connector",
		TunnelID:         "tunnel-xyz-789",
		VirtualNetworkID: "vnet-456",
		AccountID:        "account-123",
		Routes:           routes,
		Source: service.Source{
			Kind:      "WARPConnector",
			Namespace: "test-ns",
			Name:      "my-connector",
		},
		CredentialsRef: v1alpha2.CredentialsReference{
			Name: "cloudflare-creds",
		},
	}

	err := s.RequestUpdate(ctx, opts)

	require.NoError(t, err)

	// Verify SyncState was created/updated
	var syncState v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: "w-a-r-p-connector-warpconnector-my-connector"}, &syncState)
	require.NoError(t, err)

	assert.Equal(t, v1alpha2.SyncResourceWARPConnector, syncState.Spec.ResourceType)
	assert.Len(t, syncState.Spec.Sources, 1)
}

func TestConnectorService_Unregister(t *testing.T) {
	// Pre-create a SyncState with a source and correct name format
	existingSyncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "w-a-r-p-connector-warpconnector-my-connector",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceWARPConnector,
			CloudflareID: "warpconnector-my-connector",
			AccountID:    "account-123",
			CredentialsRef: v1alpha2.CredentialsReference{
				Name: "cloudflare-creds",
			},
			Sources: []v1alpha2.ConfigSource{
				{
					Ref: v1alpha2.SourceReference{
						Kind:      "WARPConnector",
						Namespace: "test-ns",
						Name:      "my-connector",
					},
					Priority: 100,
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(existingSyncState).
		Build()

	s := NewConnectorService(client)
	ctx := context.Background()

	source := service.Source{
		Kind:      "WARPConnector",
		Namespace: "test-ns",
		Name:      "my-connector",
	}

	err := s.Unregister(ctx, "my-connector", source)

	require.NoError(t, err)

	// Since this was the only source, the SyncState should be deleted
	var syncState v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: "w-a-r-p-connector-warpconnector-my-connector"}, &syncState)
	assert.True(t, err != nil, "SyncState should be deleted when no sources remain")
}

func TestConnectorService_Unregister_NotFound(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	s := NewConnectorService(client)
	ctx := context.Background()

	source := service.Source{
		Kind:      "WARPConnector",
		Namespace: "test-ns",
		Name:      "non-existent",
	}

	err := s.Unregister(ctx, "non-existent", source)

	require.NoError(t, err) // Should not error on not found
}

func TestConnectorService_GetSyncState(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "w-a-r-p-connector-warpconnector-my-connector",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceWARPConnector,
			CloudflareID: "warpconnector-my-connector",
			AccountID:    "account-123",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	s := NewConnectorService(client)
	ctx := context.Background()

	result, err := s.GetSyncState(ctx, "my-connector")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "w-a-r-p-connector-warpconnector-my-connector", result.Name)
	assert.Equal(t, "warpconnector-my-connector", result.Spec.CloudflareID)
}

func TestConnectorService_GetSyncState_NotFound(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	s := NewConnectorService(client)
	ctx := context.Background()

	result, err := s.GetSyncState(ctx, "non-existent")

	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestParseLifecycleConfig_Valid(t *testing.T) {
	configJSON := `{
		"action": "create",
		"connectorName": "my-connector",
		"virtualNetworkId": "vnet-123",
		"routes": [
			{"network": "10.0.0.0/24", "comment": "Test route"}
		]
	}`

	config, err := ParseLifecycleConfig([]byte(configJSON))

	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, ConnectorActionCreate, config.Action)
	assert.Equal(t, "my-connector", config.ConnectorName)
	assert.Equal(t, "vnet-123", config.VirtualNetworkID)
	assert.Len(t, config.Routes, 1)
	assert.Equal(t, "10.0.0.0/24", config.Routes[0].Network)
	assert.Equal(t, "Test route", config.Routes[0].Comment)
}

func TestParseLifecycleConfig_Invalid(t *testing.T) {
	config, err := ParseLifecycleConfig([]byte("not json"))

	assert.Error(t, err)
	assert.Nil(t, config)
}

func TestParseLifecycleConfig_AllFields(t *testing.T) {
	configJSON := `{
		"action": "delete",
		"connectorName": "my-connector",
		"connectorId": "connector-123",
		"tunnelId": "tunnel-456",
		"virtualNetworkId": "vnet-789",
		"routes": [
			{"network": "10.0.0.0/24"},
			{"network": "192.168.0.0/16", "comment": "Private"}
		]
	}`

	config, err := ParseLifecycleConfig([]byte(configJSON))

	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, ConnectorActionDelete, config.Action)
	assert.Equal(t, "my-connector", config.ConnectorName)
	assert.Equal(t, "connector-123", config.ConnectorID)
	assert.Equal(t, "tunnel-456", config.TunnelID)
	assert.Equal(t, "vnet-789", config.VirtualNetworkID)
	assert.Len(t, config.Routes, 2)
}

func TestConnectorAction_Values(t *testing.T) {
	assert.Equal(t, ConnectorAction("create"), ConnectorActionCreate)
	assert.Equal(t, ConnectorAction("delete"), ConnectorActionDelete)
	assert.Equal(t, ConnectorAction("update"), ConnectorActionUpdate)
}

func TestConnectorLifecycleResult_Fields(t *testing.T) {
	result := ConnectorLifecycleResult{
		ConnectorID:      "connector-123",
		TunnelID:         "tunnel-456",
		TunnelToken:      "secret-token",
		RoutesConfigured: 5,
	}

	assert.Equal(t, "connector-123", result.ConnectorID)
	assert.Equal(t, "tunnel-456", result.TunnelID)
	assert.Equal(t, "secret-token", result.TunnelToken)
	assert.Equal(t, 5, result.RoutesConfigured)
}

func TestResultKeys(t *testing.T) {
	assert.Equal(t, "connectorId", ResultKeyConnectorID)
	assert.Equal(t, "tunnelId", ResultKeyTunnelID)
	assert.Equal(t, "tunnelToken", ResultKeyTunnelToken)
	assert.Equal(t, "routesConfigured", ResultKeyRoutesConfigured)
}

func TestConnectorResourceType(t *testing.T) {
	assert.Equal(t, v1alpha2.SyncResourceWARPConnector, ConnectorResourceType)
}
