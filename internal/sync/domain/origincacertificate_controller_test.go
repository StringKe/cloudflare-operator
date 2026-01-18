// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package domain

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
	domainsvc "github.com/StringKe/cloudflare-operator/internal/service/domain"
)

func init() {
	_ = v1alpha2.AddToScheme(scheme.Scheme)
}

// createOriginCACertificateConfig creates a test lifecycle config
func createOriginCACertificateConfig(
	action domainsvc.OriginCACertificateAction,
	certificateID string,
	hostnames []string,
) *domainsvc.OriginCACertificateLifecycleConfig {
	return &domainsvc.OriginCACertificateLifecycleConfig{
		Action:        action,
		CertificateID: certificateID,
		Hostnames:     hostnames,
		RequestType:   "origin-rsa",
		ValidityDays:  5475,
	}
}

// createOriginCACertificateSyncState creates a test SyncState for OriginCACertificate
func createOriginCACertificateSyncState(
	name string,
	config *domainsvc.OriginCACertificateLifecycleConfig,
	status v1alpha2.SyncStatus,
	withFinalizer bool,
) *v1alpha2.CloudflareSyncState {
	configBytes, _ := json.Marshal(config)

	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceOriginCACertificate,
			CloudflareID: config.CertificateID,
			AccountID:    "test-account-123",
			ZoneID:       "zone-abc",
			CredentialsRef: v1alpha2.CredentialsReference{
				Name: "test-credentials",
			},
			Sources: []v1alpha2.ConfigSource{
				{
					Ref: v1alpha2.SourceReference{
						Kind:      "OriginCACertificate",
						Namespace: "test-ns",
						Name:      "test-cert",
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
		syncState.Finalizers = []string{OriginCACertificateFinalizerName}
	}

	return syncState
}

func TestNewOriginCACertificateController(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	c := NewOriginCACertificateController(client)

	require.NotNil(t, c)
	assert.NotNil(t, c.BaseSyncController)
	assert.NotNil(t, c.Client)
	assert.NotNil(t, c.Debouncer)
}

func TestOriginCACertificateController_GetLifecycleConfig_Success(t *testing.T) {
	config := createOriginCACertificateConfig(
		domainsvc.OriginCACertificateActionCreate,
		"",
		[]string{"example.com", "*.example.com"},
	)
	syncState := createOriginCACertificateSyncState("test-sync", config, v1alpha2.SyncStatusPending, true)

	c := &OriginCACertificateController{}
	result, err := c.getLifecycleConfig(syncState)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, domainsvc.OriginCACertificateActionCreate, result.Action)
	assert.Equal(t, []string{"example.com", "*.example.com"}, result.Hostnames)
	assert.Equal(t, "origin-rsa", result.RequestType)
	assert.Equal(t, 5475, result.ValidityDays)
}

func TestOriginCACertificateController_GetLifecycleConfig_NoSources(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceOriginCACertificate,
			Sources:      []v1alpha2.ConfigSource{},
		},
	}

	c := &OriginCACertificateController{}
	result, err := c.getLifecycleConfig(syncState)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no sources")
}

func TestOriginCACertificateController_GetLifecycleConfig_InvalidJSON(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceOriginCACertificate,
			Sources: []v1alpha2.ConfigSource{
				{
					Ref: v1alpha2.SourceReference{
						Kind: "OriginCACertificate",
						Name: "invalid",
					},
					Config: runtime.RawExtension{Raw: []byte("not valid json")},
				},
			},
		},
	}

	c := &OriginCACertificateController{}
	result, err := c.getLifecycleConfig(syncState)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestOriginCACertificateController_Reconcile_NotFound(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	c := NewOriginCACertificateController(client)
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

func TestOriginCACertificateController_Reconcile_WrongResourceType(t *testing.T) {
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

	c := NewOriginCACertificateController(client)
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

func TestOriginCACertificateController_Reconcile_AlreadySynced(t *testing.T) {
	config := createOriginCACertificateConfig(
		domainsvc.OriginCACertificateActionCreate,
		"cert-123",
		[]string{"example.com"},
	)
	syncState := createOriginCACertificateSyncState("test-sync", config, v1alpha2.SyncStatusSynced, true)

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewOriginCACertificateController(client)
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

func TestOriginCACertificateController_Reconcile_NoSources_NoFinalizer(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sync",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: v1alpha2.SyncResourceOriginCACertificate,
			CloudflareID: "pending-abc123",
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

	c := NewOriginCACertificateController(client)
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

func TestOriginCACertificateController_Reconcile_AddsFinalizer(t *testing.T) {
	config := createOriginCACertificateConfig(
		domainsvc.OriginCACertificateActionCreate,
		"",
		[]string{"example.com"},
	)
	syncState := createOriginCACertificateSyncState("test-sync", config, v1alpha2.SyncStatusPending, false)

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewOriginCACertificateController(client)
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
	assert.Contains(t, updatedState.Finalizers, OriginCACertificateFinalizerName)
}

func TestOriginCACertificateController_Reconcile_DebouncePending(t *testing.T) {
	config := createOriginCACertificateConfig(
		domainsvc.OriginCACertificateActionCreate,
		"",
		[]string{"example.com"},
	)
	syncState := createOriginCACertificateSyncState("test-sync", config, v1alpha2.SyncStatusPending, true)

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewOriginCACertificateController(client)
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

func TestOriginCACertificateController_HandleDeletion_NoFinalizer(t *testing.T) {
	config := createOriginCACertificateConfig(
		domainsvc.OriginCACertificateActionCreate,
		"cert-123",
		[]string{"example.com"},
	)
	syncState := createOriginCACertificateSyncState("test-sync", config, v1alpha2.SyncStatusSynced, false)

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewOriginCACertificateController(client)
	ctx := context.Background()

	result, err := c.handleDeletion(ctx, syncState)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestOriginCACertificateController_HandleDeletion_PendingID(t *testing.T) {
	config := createOriginCACertificateConfig(
		domainsvc.OriginCACertificateActionCreate,
		"pending-123",
		[]string{"example.com"},
	)
	syncState := createOriginCACertificateSyncState("test-sync", config, v1alpha2.SyncStatusPending, true)
	syncState.Spec.CloudflareID = "pending-123"
	syncState.Spec.Sources = []v1alpha2.ConfigSource{} // Empty sources to trigger cleanup

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(syncState).
		Build()

	c := NewOriginCACertificateController(client)
	ctx := context.Background()

	result, err := c.handleDeletion(ctx, syncState)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify SyncState was deleted (orphan cleanup)
	var updatedState v1alpha2.CloudflareSyncState
	err = client.Get(ctx, types.NamespacedName{Name: "test-sync"}, &updatedState)
	assert.True(t, err != nil, "SyncState should be deleted")
}

func TestOriginCACertificateController_SetupWithManager(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	c := NewOriginCACertificateController(client)

	// Verify the controller is properly configured
	assert.NotNil(t, c.Client)
	assert.NotNil(t, c.Debouncer)
}

func TestOriginCACertificateLifecycleConfig_AllActions(t *testing.T) {
	tests := []struct {
		name          string
		action        domainsvc.OriginCACertificateAction
		certificateID string
		hostnames     []string
	}{
		{
			name:          "create action",
			action:        domainsvc.OriginCACertificateActionCreate,
			certificateID: "",
			hostnames:     []string{"example.com", "*.example.com"},
		},
		{
			name:          "revoke action",
			action:        domainsvc.OriginCACertificateActionRevoke,
			certificateID: "cert-abc-123",
			hostnames:     nil,
		},
		{
			name:          "renew action",
			action:        domainsvc.OriginCACertificateActionRenew,
			certificateID: "cert-xyz-456",
			hostnames:     []string{"new.example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &domainsvc.OriginCACertificateLifecycleConfig{
				Action:        tt.action,
				CertificateID: tt.certificateID,
				Hostnames:     tt.hostnames,
			}

			configBytes, err := json.Marshal(config)
			require.NoError(t, err)

			var parsed domainsvc.OriginCACertificateLifecycleConfig
			err = json.Unmarshal(configBytes, &parsed)
			require.NoError(t, err)

			assert.Equal(t, tt.action, parsed.Action)
			assert.Equal(t, tt.certificateID, parsed.CertificateID)
			assert.Equal(t, tt.hostnames, parsed.Hostnames)
		})
	}
}

func TestOriginCACertificateSyncResult_Fields(t *testing.T) {
	result := &domainsvc.OriginCACertificateSyncResult{
		CertificateID: "cert-123",
		Certificate:   "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
		PrivateKey:    "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----",
	}

	assert.Equal(t, "cert-123", result.CertificateID)
	assert.Contains(t, result.Certificate, "BEGIN CERTIFICATE")
	assert.Contains(t, result.PrivateKey, "BEGIN PRIVATE KEY")
}

func TestIsOriginCACertificateSyncState(t *testing.T) {
	tests := []struct {
		name         string
		resourceType v1alpha2.SyncResourceType
		expected     bool
	}{
		{
			name:         "OriginCACertificate type",
			resourceType: v1alpha2.SyncResourceOriginCACertificate,
			expected:     true,
		},
		{
			name:         "DNSRecord type",
			resourceType: v1alpha2.SyncResourceDNSRecord,
			expected:     false,
		},
		{
			name:         "DomainRegistration type",
			resourceType: v1alpha2.SyncResourceDomainRegistration,
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

			result := isOriginCACertificateSyncState(syncState)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseLifecycleConfig_Valid(t *testing.T) {
	configJSON := `{
		"action": "create",
		"hostnames": ["example.com", "*.example.com"],
		"requestType": "origin-rsa",
		"validityDays": 5475
	}`

	config, err := ParseLifecycleConfig([]byte(configJSON))

	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, domainsvc.OriginCACertificateActionCreate, config.Action)
	assert.Equal(t, []string{"example.com", "*.example.com"}, config.Hostnames)
	assert.Equal(t, "origin-rsa", config.RequestType)
	assert.Equal(t, 5475, config.ValidityDays)
}

func TestParseLifecycleConfig_Invalid(t *testing.T) {
	config, err := ParseLifecycleConfig([]byte("not json"))

	assert.Error(t, err)
	assert.Nil(t, config)
}

func TestGetResultFromSyncState_Nil(t *testing.T) {
	result := GetResultFromSyncState(nil)
	assert.Nil(t, result)
}

func TestGetResultFromSyncState_NoResultData(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		Status: v1alpha2.CloudflareSyncStateStatus{
			ResultData: nil,
		},
	}

	result := GetResultFromSyncState(syncState)
	assert.Nil(t, result)
}

func TestGetResultFromSyncState_WithData(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		Status: v1alpha2.CloudflareSyncStateStatus{
			ResultData: map[string]string{
				domainsvc.ResultKeyOriginCACertificateID: "cert-123",
				domainsvc.ResultKeyOriginCACertificate:   "-----BEGIN CERTIFICATE-----",
				domainsvc.ResultKeyOriginCAHostnames:     "example.com,*.example.com",
			},
		},
	}

	result := GetResultFromSyncState(syncState)

	require.NotNil(t, result)
	assert.Equal(t, "cert-123", result.CertificateID)
	assert.Equal(t, "-----BEGIN CERTIFICATE-----", result.Certificate)
}
