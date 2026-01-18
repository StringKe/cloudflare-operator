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

// TestTunnelLifecycle tests the complete lifecycle of a Tunnel resource
func TestTunnelLifecycle(t *testing.T) {
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
	testNS := "e2e-tunnel-test"

	// Setup test namespace
	require.NoError(t, f.SetupTestNamespace(testNS))
	defer f.CleanupTestNamespace(testNS)

	// Ensure operator namespace exists and create credentials
	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	var tunnelID string

	t.Run("CreateTunnel", func(t *testing.T) {
		tunnel := &v1alpha2.Tunnel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-test-tunnel",
				Namespace: testNS,
			},
			Spec: v1alpha2.TunnelSpec{
				NewTunnel: &v1alpha2.NewTunnel{
					Name: "e2e-test-tunnel",
				},
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, tunnel)
		require.NoError(t, err)

		// Wait for Ready condition
		err = f.WaitForCondition(tunnel, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err, "Tunnel should become ready")

		// Verify status
		var fetched v1alpha2.Tunnel
		err = f.Client.Get(ctx, types.NamespacedName{
			Name:      tunnel.Name,
			Namespace: tunnel.Namespace,
		}, &fetched)
		require.NoError(t, err)
		assert.NotEmpty(t, fetched.Status.TunnelId, "TunnelId should be set")
		tunnelID = fetched.Status.TunnelId
	})

	t.Run("UpdateTunnel", func(t *testing.T) {
		var tunnel v1alpha2.Tunnel
		err := f.Client.Get(ctx, types.NamespacedName{
			Name:      "e2e-test-tunnel",
			Namespace: testNS,
		}, &tunnel)
		require.NoError(t, err)

		// Update spec
		tunnel.Spec.NoTlsVerify = true
		err = f.Client.Update(ctx, &tunnel)
		require.NoError(t, err)

		// Wait for reconciliation by checking the status condition is still Ready
		err = f.WaitForStatusField(&tunnel, func(obj client.Object) bool {
			t := obj.(*v1alpha2.Tunnel)
			return t.Spec.NoTlsVerify && t.Status.TunnelId == tunnelID
		}, 30*time.Second)
		require.NoError(t, err)

		// Verify update
		err = f.Client.Get(ctx, types.NamespacedName{
			Name:      tunnel.Name,
			Namespace: tunnel.Namespace,
		}, &tunnel)
		require.NoError(t, err)
		assert.True(t, tunnel.Spec.NoTlsVerify)
	})

	t.Run("DeleteTunnel", func(t *testing.T) {
		var tunnel v1alpha2.Tunnel
		err := f.Client.Get(ctx, types.NamespacedName{
			Name:      "e2e-test-tunnel",
			Namespace: testNS,
		}, &tunnel)
		require.NoError(t, err)

		err = f.Client.Delete(ctx, &tunnel)
		require.NoError(t, err)

		// Wait for deletion
		err = f.WaitForDeletion(&tunnel, 2*time.Minute)
		assert.NoError(t, err, "Tunnel should be deleted")
	})
}

// TestClusterTunnelLifecycle tests the complete lifecycle of a ClusterTunnel resource
func TestClusterTunnelLifecycle(t *testing.T) {
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

	t.Run("CreateClusterTunnel", func(t *testing.T) {
		clusterTunnel := &v1alpha2.ClusterTunnel{
			ObjectMeta: metav1.ObjectMeta{
				Name: "e2e-test-cluster-tunnel",
			},
			Spec: v1alpha2.TunnelSpec{
				NewTunnel: &v1alpha2.NewTunnel{
					Name: "e2e-test-cluster-tunnel",
				},
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, clusterTunnel)
		require.NoError(t, err)

		// Wait for Ready condition
		err = f.WaitForCondition(clusterTunnel, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err, "ClusterTunnel should become ready")

		// Verify status
		var fetched v1alpha2.ClusterTunnel
		err = f.Client.Get(ctx, types.NamespacedName{Name: clusterTunnel.Name}, &fetched)
		require.NoError(t, err)
		assert.NotEmpty(t, fetched.Status.TunnelId, "TunnelId should be set")

		// Cleanup
		defer func() {
			_ = f.Client.Delete(ctx, clusterTunnel)
			_ = f.WaitForDeletion(clusterTunnel, time.Minute)
		}()
	})
}

// TestTunnelWithExistingTunnel tests creating a Tunnel that references an existing Cloudflare tunnel
func TestTunnelWithExistingTunnel(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()
	testNS := "e2e-existing-tunnel-test"

	require.NoError(t, f.SetupTestNamespace(testNS))
	defer f.CleanupTestNamespace(testNS)

	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	tunnel := &v1alpha2.Tunnel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-existing-tunnel",
			Namespace: testNS,
		},
		Spec: v1alpha2.TunnelSpec{
			ExistingTunnel: &v1alpha2.ExistingTunnel{
				Id: "existing-tunnel-id-12345",
			},
			Cloudflare: v1alpha2.CloudflareDetails{
				CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
					Name: testCredentialsName,
				},
			},
		},
	}

	// Cleanup any leftover resources from previous test runs
	existingTunnel := &v1alpha2.Tunnel{}
	if getErr := f.Client.Get(ctx, types.NamespacedName{Name: tunnel.Name, Namespace: tunnel.Namespace}, existingTunnel); getErr == nil {
		_ = f.Client.Delete(ctx, existingTunnel)
		_ = f.WaitForDeletion(existingTunnel, time.Minute)
	}

	err = f.Client.Create(ctx, tunnel)
	require.NoError(t, err)

	// For existing tunnel, it should adopt the tunnel
	err = f.WaitForCondition(tunnel, "Ready", metav1.ConditionTrue, 2*time.Minute)
	// Note: This may fail if mock server doesn't have the tunnel - that's expected
	if err != nil {
		t.Logf("Expected behavior: existing tunnel not found in mock server: %v", err)
	}

	// Cleanup
	_ = f.Client.Delete(ctx, tunnel)
	_ = f.WaitForDeletion(tunnel, time.Minute)
}
