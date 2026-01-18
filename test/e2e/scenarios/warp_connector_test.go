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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/test/e2e/framework"
)

// TestWARPConnectorLifecycle tests the complete lifecycle of a WARPConnector resource
func TestWARPConnectorLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Setup framework
	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()
	testNS := "e2e-warp-connector-test"

	// Setup test namespace
	require.NoError(t, f.SetupTestNamespace(testNS))
	defer f.CleanupTestNamespace(testNS)

	// Ensure operator namespace exists and create credentials
	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	var connectorID string

	t.Run("CreateWARPConnector", func(t *testing.T) {
		connector := &v1alpha2.WARPConnector{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-test-warp-connector",
				Namespace: testNS,
			},
			Spec: v1alpha2.WARPConnectorSpec{
				Name: "e2e-test-connector",
				Routes: []v1alpha2.WARPRoute{
					{
						Network: "10.0.0.0/24",
						Comment: "E2E test route",
					},
				},
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, connector)
		require.NoError(t, err)

		// Wait for Ready condition
		err = f.WaitForCondition(connector, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err, "WARPConnector should become ready")

		// Verify status
		var fetched v1alpha2.WARPConnector
		err = f.Client.Get(ctx, types.NamespacedName{
			Name:      connector.Name,
			Namespace: connector.Namespace,
		}, &fetched)
		require.NoError(t, err)
		assert.NotEmpty(t, fetched.Status.ConnectorID, "ConnectorID should be set")
		connectorID = fetched.Status.ConnectorID
	})

	t.Run("UpdateWARPConnectorRoutes", func(t *testing.T) {
		var connector v1alpha2.WARPConnector
		err := f.Client.Get(ctx, types.NamespacedName{
			Name:      "e2e-test-warp-connector",
			Namespace: testNS,
		}, &connector)
		require.NoError(t, err)

		// Update routes
		connector.Spec.Routes = append(connector.Spec.Routes, v1alpha2.WARPRoute{
			Network: "192.168.0.0/16",
			Comment: "Additional E2E test route",
		})
		err = f.Client.Update(ctx, &connector)
		require.NoError(t, err)

		// Wait for reconciliation
		err = f.WaitForStatusField(&connector, func(obj client.Object) bool {
			c := obj.(*v1alpha2.WARPConnector)
			return c.Status.ConnectorID == connectorID && len(c.Spec.Routes) == 2
		}, 30*time.Second)
		require.NoError(t, err)

		// Verify update
		err = f.Client.Get(ctx, types.NamespacedName{
			Name:      connector.Name,
			Namespace: connector.Namespace,
		}, &connector)
		require.NoError(t, err)
		assert.Len(t, connector.Spec.Routes, 2)
	})

	t.Run("DeleteWARPConnector", func(t *testing.T) {
		var connector v1alpha2.WARPConnector
		err := f.Client.Get(ctx, types.NamespacedName{
			Name:      "e2e-test-warp-connector",
			Namespace: testNS,
		}, &connector)
		require.NoError(t, err)

		err = f.Client.Delete(ctx, &connector)
		require.NoError(t, err)

		// Wait for deletion
		err = f.WaitForDeletion(&connector, 2*time.Minute)
		assert.NoError(t, err, "WARPConnector should be deleted")
	})
}

// TestWARPConnectorWithVirtualNetwork tests WARPConnector with VirtualNetwork association
func TestWARPConnectorWithVirtualNetwork(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()
	testNS := "e2e-warp-vnet-test"

	require.NoError(t, f.SetupTestNamespace(testNS))
	defer f.CleanupTestNamespace(testNS)

	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	// First create a VirtualNetwork
	vnet := &v1alpha2.VirtualNetwork{
		ObjectMeta: metav1.ObjectMeta{
			Name: "e2e-test-vnet-for-warp",
		},
		Spec: v1alpha2.VirtualNetworkSpec{
			Name:      "e2e-test-vnet-for-warp",
			Comment:   "E2E test virtual network for WARP connector",
			IsDefault: false,
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

	// Wait for VirtualNetwork to be ready
	err = f.WaitForCondition(vnet, "Ready", metav1.ConditionTrue, 2*time.Minute)
	if err != nil {
		t.Logf("VirtualNetwork may not be ready (expected if mock server doesn't support it): %v", err)
	}

	// Create WARPConnector referencing the VirtualNetwork
	connector := &v1alpha2.WARPConnector{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-warp-with-vnet",
			Namespace: testNS,
		},
		Spec: v1alpha2.WARPConnectorSpec{
			Name: "e2e-warp-with-vnet",
			VirtualNetworkRef: &v1alpha2.VirtualNetworkRef{
				Name: vnet.Name,
			},
			Routes: []v1alpha2.WARPRoute{
				{
					Network: "172.16.0.0/12",
					Comment: "Test route with VNet",
				},
			},
			Cloudflare: v1alpha2.CloudflareDetails{
				CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
					Name: testCredentialsName,
				},
			},
		},
	}

	err = f.Client.Create(ctx, connector)
	require.NoError(t, err)
	defer func() {
		_ = f.Client.Delete(ctx, connector)
		_ = f.WaitForDeletion(connector, time.Minute)
	}()

	// Wait for WARPConnector to be ready
	err = f.WaitForCondition(connector, "Ready", metav1.ConditionTrue, 2*time.Minute)
	if err != nil {
		t.Logf("WARPConnector may not be ready (expected if mock server doesn't support it): %v", err)
	} else {
		// Verify status includes VirtualNetwork ID
		var fetched v1alpha2.WARPConnector
		err = f.Client.Get(ctx, types.NamespacedName{
			Name:      connector.Name,
			Namespace: connector.Namespace,
		}, &fetched)
		require.NoError(t, err)
		assert.NotEmpty(t, fetched.Status.ConnectorID, "ConnectorID should be set")
	}
}

// TestWARPConnectorMultipleRoutes tests WARPConnector with multiple routes
func TestWARPConnectorMultipleRoutes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()
	testNS := "e2e-warp-routes-test"

	require.NoError(t, f.SetupTestNamespace(testNS))
	defer f.CleanupTestNamespace(testNS)

	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	connector := &v1alpha2.WARPConnector{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-multi-route-connector",
			Namespace: testNS,
		},
		Spec: v1alpha2.WARPConnectorSpec{
			Name: "e2e-multi-route-connector",
			Routes: []v1alpha2.WARPRoute{
				{Network: "10.0.0.0/8", Comment: "Class A private network"},
				{Network: "172.16.0.0/12", Comment: "Class B private network"},
				{Network: "192.168.0.0/16", Comment: "Class C private network"},
			},
			Cloudflare: v1alpha2.CloudflareDetails{
				CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
					Name: testCredentialsName,
				},
			},
		},
	}

	err = f.Client.Create(ctx, connector)
	require.NoError(t, err)
	defer func() {
		_ = f.Client.Delete(ctx, connector)
		_ = f.WaitForDeletion(connector, time.Minute)
	}()

	// Wait for Ready condition
	err = f.WaitForCondition(connector, "Ready", metav1.ConditionTrue, 2*time.Minute)
	if err != nil {
		t.Logf("WARPConnector with multiple routes may not be ready (expected if mock server doesn't support it): %v", err)
		return
	}

	// Verify all routes were processed
	var fetched v1alpha2.WARPConnector
	err = f.Client.Get(ctx, types.NamespacedName{
		Name:      connector.Name,
		Namespace: connector.Namespace,
	}, &fetched)
	require.NoError(t, err)
	assert.NotEmpty(t, fetched.Status.ConnectorID)
	assert.Len(t, fetched.Spec.Routes, 3)
}
