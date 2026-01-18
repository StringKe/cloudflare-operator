// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

//go:build e2e

package scenarios

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/test/e2e/framework"
)

// TestVirtualNetworkLifecycle tests the complete lifecycle of a VirtualNetwork resource
func TestVirtualNetworkLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()

	// Ensure operator namespace exists and create credentials
	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	var vnetID string

	// Cleanup any leftover resources from previous test runs
	existingVnet := &v1alpha2.VirtualNetwork{}
	if getErr := f.Client.Get(ctx, types.NamespacedName{Name: "e2e-test-vnet"}, existingVnet); getErr == nil {
		_ = f.Client.Delete(ctx, existingVnet)
		_ = f.WaitForDeletion(existingVnet, time.Minute)
	}

	t.Run("CreateVirtualNetwork", func(t *testing.T) {
		vnet := &v1alpha2.VirtualNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name: "e2e-test-vnet",
			},
			Spec: v1alpha2.VirtualNetworkSpec{
				Name:             "e2e-test-network",
				Comment:          "E2E test virtual network",
				IsDefaultNetwork: false,
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, vnet)
		require.NoError(t, err)

		// Wait for Ready condition
		err = f.WaitForCondition(vnet, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err, "VirtualNetwork should become ready")

		// Verify status
		var fetched v1alpha2.VirtualNetwork
		err = f.Client.Get(ctx, types.NamespacedName{Name: vnet.Name}, &fetched)
		require.NoError(t, err)
		assert.NotEmpty(t, fetched.Status.VirtualNetworkId, "VirtualNetworkId should be set")
		vnetID = fetched.Status.VirtualNetworkId
	})

	t.Run("UpdateVirtualNetwork", func(t *testing.T) {
		var vnet v1alpha2.VirtualNetwork
		err := f.Client.Get(ctx, types.NamespacedName{Name: "e2e-test-vnet"}, &vnet)
		require.NoError(t, err)

		// Update comment
		vnet.Spec.Comment = "Updated E2E test virtual network"
		err = f.Client.Update(ctx, &vnet)
		require.NoError(t, err)

		// Wait for reconciliation
		err = f.WaitForCondition(&vnet, "Ready", metav1.ConditionTrue, 30*time.Second)
		require.NoError(t, err)

		// Verify VirtualNetworkId is preserved
		var fetched v1alpha2.VirtualNetwork
		err = f.Client.Get(ctx, types.NamespacedName{Name: vnet.Name}, &fetched)
		require.NoError(t, err)
		assert.Equal(t, vnetID, fetched.Status.VirtualNetworkId, "VirtualNetworkId should be preserved after update")
	})

	t.Run("DeleteVirtualNetwork", func(t *testing.T) {
		var vnet v1alpha2.VirtualNetwork
		err := f.Client.Get(ctx, types.NamespacedName{Name: "e2e-test-vnet"}, &vnet)
		require.NoError(t, err)

		err = f.Client.Delete(ctx, &vnet)
		require.NoError(t, err)

		err = f.WaitForDeletion(&vnet, 2*time.Minute)
		assert.NoError(t, err, "VirtualNetwork should be deleted")
	})
}

// TestNetworkRouteLifecycle tests the complete lifecycle of a NetworkRoute resource
func TestNetworkRouteLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()

	// Ensure operator namespace exists and create credentials
	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	// Cleanup any leftover resources from previous test runs
	existingRoute := &v1alpha2.NetworkRoute{}
	if getErr := f.Client.Get(ctx, types.NamespacedName{Name: "e2e-test-route"}, existingRoute); getErr == nil {
		_ = f.Client.Delete(ctx, existingRoute)
		_ = f.WaitForDeletion(existingRoute, time.Minute)
	}
	existingVnet := &v1alpha2.VirtualNetwork{}
	if getErr := f.Client.Get(ctx, types.NamespacedName{Name: "e2e-route-vnet"}, existingVnet); getErr == nil {
		_ = f.Client.Delete(ctx, existingVnet)
		_ = f.WaitForDeletion(existingVnet, time.Minute)
	}
	existingTunnel := &v1alpha2.ClusterTunnel{}
	if getErr := f.Client.Get(ctx, types.NamespacedName{Name: "e2e-route-tunnel"}, existingTunnel); getErr == nil {
		_ = f.Client.Delete(ctx, existingTunnel)
		_ = f.WaitForDeletion(existingTunnel, time.Minute)
	}

	// First create a ClusterTunnel for the route (required)
	tunnel := &v1alpha2.ClusterTunnel{
		ObjectMeta: metav1.ObjectMeta{
			Name: "e2e-route-tunnel",
		},
		Spec: v1alpha2.TunnelSpec{
			NewTunnel: &v1alpha2.NewTunnel{
				Name: "e2e-route-tunnel",
			},
			Cloudflare: v1alpha2.CloudflareDetails{
				CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
					Name: testCredentialsName,
				},
			},
		},
	}

	err = f.Client.Create(ctx, tunnel)
	require.NoError(t, err)
	defer func() {
		_ = f.Client.Delete(ctx, tunnel)
		_ = f.WaitForDeletion(tunnel, time.Minute)
	}()

	err = f.WaitForCondition(tunnel, "Ready", metav1.ConditionTrue, 2*time.Minute)
	require.NoError(t, err, "ClusterTunnel must be ready before creating NetworkRoute")

	// Create a VirtualNetwork for the route
	vnet := &v1alpha2.VirtualNetwork{
		ObjectMeta: metav1.ObjectMeta{
			Name: "e2e-route-vnet",
		},
		Spec: v1alpha2.VirtualNetworkSpec{
			Name:    "e2e-route-network",
			Comment: "VNet for NetworkRoute E2E test",
			Cloudflare: v1alpha2.CloudflareDetails{
				CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
					Name: testCredentialsName,
				},
			},
		},
	}

	err = f.Client.Create(ctx, vnet)
	require.NoError(t, err)
	defer func() {
		_ = f.Client.Delete(ctx, vnet)
		_ = f.WaitForDeletion(vnet, time.Minute)
	}()

	err = f.WaitForCondition(vnet, "Ready", metav1.ConditionTrue, 2*time.Minute)
	require.NoError(t, err, "VirtualNetwork must be ready before creating NetworkRoute")

	t.Run("CreateNetworkRoute", func(t *testing.T) {
		route := &v1alpha2.NetworkRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name: "e2e-test-route",
			},
			Spec: v1alpha2.NetworkRouteSpec{
				Network: "10.0.0.0/24",
				Comment: "E2E test network route",
				TunnelRef: v1alpha2.TunnelRef{
					Kind: "ClusterTunnel",
					Name: tunnel.Name,
				},
				VirtualNetworkRef: &v1alpha2.VirtualNetworkRef{
					Name: vnet.Name,
				},
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, route)
		require.NoError(t, err)

		// Wait for Ready condition
		err = f.WaitForCondition(route, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err, "NetworkRoute should become ready")

		// Verify status
		var fetched v1alpha2.NetworkRoute
		err = f.Client.Get(ctx, types.NamespacedName{Name: route.Name}, &fetched)
		require.NoError(t, err)
		assert.NotEmpty(t, fetched.Status.Network, "Network should be set in status")
	})

	t.Run("DeleteNetworkRoute", func(t *testing.T) {
		var route v1alpha2.NetworkRoute
		err := f.Client.Get(ctx, types.NamespacedName{Name: "e2e-test-route"}, &route)
		require.NoError(t, err)

		err = f.Client.Delete(ctx, &route)
		require.NoError(t, err)

		err = f.WaitForDeletion(&route, 2*time.Minute)
		assert.NoError(t, err, "NetworkRoute should be deleted")
	})
}

// TestPrivateServiceLifecycle tests the complete lifecycle of a PrivateService resource
func TestPrivateServiceLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()
	testNS := "e2e-private-svc-test"

	// Setup test namespace
	require.NoError(t, f.SetupTestNamespace(testNS))
	defer f.CleanupTestNamespace(testNS)

	// Ensure operator namespace exists and create credentials
	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	// Cleanup any leftover resources from previous test runs
	existingPrivateSvc := &v1alpha2.PrivateService{}
	if getErr := f.Client.Get(ctx, types.NamespacedName{Name: "e2e-test-private-service", Namespace: testNS}, existingPrivateSvc); getErr == nil {
		_ = f.Client.Delete(ctx, existingPrivateSvc)
		_ = f.WaitForDeletion(existingPrivateSvc, time.Minute)
	}
	existingTunnel := &v1alpha2.ClusterTunnel{}
	if getErr := f.Client.Get(ctx, types.NamespacedName{Name: "e2e-private-svc-tunnel"}, existingTunnel); getErr == nil {
		_ = f.Client.Delete(ctx, existingTunnel)
		_ = f.WaitForDeletion(existingTunnel, time.Minute)
	}

	// Create a ClusterTunnel for the PrivateService (required)
	tunnel := &v1alpha2.ClusterTunnel{
		ObjectMeta: metav1.ObjectMeta{
			Name: "e2e-private-svc-tunnel",
		},
		Spec: v1alpha2.TunnelSpec{
			NewTunnel: &v1alpha2.NewTunnel{
				Name: "e2e-private-svc-tunnel",
			},
			Cloudflare: v1alpha2.CloudflareDetails{
				CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
					Name: testCredentialsName,
				},
			},
		},
	}

	err = f.Client.Create(ctx, tunnel)
	require.NoError(t, err)
	defer func() {
		_ = f.Client.Delete(ctx, tunnel)
		_ = f.WaitForDeletion(tunnel, time.Minute)
	}()

	err = f.WaitForCondition(tunnel, "Ready", metav1.ConditionTrue, 2*time.Minute)
	require.NoError(t, err, "ClusterTunnel must be ready before creating PrivateService")

	// Create a test service
	require.NoError(t, f.CreateTestService(testNS, "e2e-backend-svc", 80))

	t.Run("CreatePrivateService", func(t *testing.T) {
		privateSvc := &v1alpha2.PrivateService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-test-private-service",
				Namespace: testNS,
			},
			Spec: v1alpha2.PrivateServiceSpec{
				ServiceRef: v1alpha2.ServiceRef{
					Name: "e2e-backend-svc",
					Port: 80,
				},
				TunnelRef: v1alpha2.TunnelRef{
					Kind: "ClusterTunnel",
					Name: tunnel.Name,
				},
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, privateSvc)
		require.NoError(t, err)
		defer func() {
			_ = f.Client.Delete(ctx, privateSvc)
			_ = f.WaitForDeletion(privateSvc, time.Minute)
		}()

		// Wait for Ready condition
		err = f.WaitForCondition(privateSvc, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err, "PrivateService should become ready")

		// Verify status
		var fetched v1alpha2.PrivateService
		err = f.Client.Get(ctx, types.NamespacedName{Name: privateSvc.Name, Namespace: privateSvc.Namespace}, &fetched)
		require.NoError(t, err)
		assert.NotEmpty(t, fetched.Status.Network, "Network should be set in status")
		assert.NotEmpty(t, fetched.Status.TunnelID, "TunnelID should be set in status")
	})
}
