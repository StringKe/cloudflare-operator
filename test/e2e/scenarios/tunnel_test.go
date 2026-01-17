// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

//go:build e2e

package scenarios

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

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
	opts.UseExistingCluster = true // Use existing cluster for CI
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()
	testNS := "e2e-tunnel-test"

	// Setup test namespace
	require.NoError(t, f.SetupTestNamespace(testNS))
	defer f.CleanupTestNamespace(testNS)

	// Setup credentials secret
	require.NoError(t, f.CreateSecret(framework.OperatorNamespace, "test-credentials", map[string]string{
		"apiToken": "test-api-token",
	}))

	t.Run("CreateTunnel", func(t *testing.T) {
		tunnel := &v1alpha2.Tunnel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-test-tunnel",
				Namespace: testNS,
			},
			Spec: v1alpha2.TunnelSpec{
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: "test-credentials",
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
	})

	t.Run("UpdateTunnel", func(t *testing.T) {
		var tunnel v1alpha2.Tunnel
		err := f.Client.Get(ctx, types.NamespacedName{
			Name:      "e2e-test-tunnel",
			Namespace: testNS,
		}, &tunnel)
		require.NoError(t, err)

		// Update spec
		tunnel.Spec.NoTLSVerify = true
		err = f.Client.Update(ctx, &tunnel)
		require.NoError(t, err)

		// Wait for reconciliation
		time.Sleep(5 * time.Second)

		// Verify update
		err = f.Client.Get(ctx, types.NamespacedName{
			Name:      tunnel.Name,
			Namespace: tunnel.Namespace,
		}, &tunnel)
		require.NoError(t, err)
		assert.True(t, tunnel.Spec.NoTLSVerify)
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

	t.Run("CreateClusterTunnel", func(t *testing.T) {
		clusterTunnel := &v1alpha2.ClusterTunnel{
			ObjectMeta: metav1.ObjectMeta{
				Name: "e2e-test-cluster-tunnel",
			},
			Spec: v1alpha2.ClusterTunnelSpec{
				TunnelSpec: v1alpha2.TunnelSpec{
					Cloudflare: v1alpha2.CloudflareDetails{
						CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
							Name: "test-credentials",
						},
					},
				},
			},
		}

		err := f.Client.Create(ctx, clusterTunnel)
		require.NoError(t, err)

		// Wait for Ready condition
		err = f.WaitForCondition(clusterTunnel, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err, "ClusterTunnel should become ready")

		// Cleanup
		defer func() {
			_ = f.Client.Delete(ctx, clusterTunnel)
			_ = f.WaitForDeletion(clusterTunnel, time.Minute)
		}()
	})
}
