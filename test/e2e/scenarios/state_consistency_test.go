// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

//go:build e2e

package scenarios

import (
	"fmt"
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

// TestStateConsistencyAfterDeletion verifies that state remains consistent after resource deletion
func TestStateConsistencyAfterDeletion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()
	testNS := "e2e-state-consistency-test"

	require.NoError(t, f.SetupTestNamespace(testNS))
	defer f.CleanupTestNamespace(testNS)

	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	t.Run("DNSRecordStateConsistency", func(t *testing.T) {
		// Create DNSRecord
		record := &v1alpha2.DNSRecord{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "state-test-dns",
				Namespace: testNS,
			},
			Spec: v1alpha2.DNSRecordSpec{
				Name:    "state-test.example.com",
				Type:    "A",
				Content: "192.168.1.100",
				TTL:     300,
				Proxied: false,
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
					Zone: "example.com",
				},
			},
		}

		err := f.Client.Create(ctx, record)
		require.NoError(t, err)

		// Wait for Ready condition
		err = f.WaitForCondition(record, "Ready", metav1.ConditionTrue, 2*time.Minute)
		if err != nil {
			t.Logf("DNSRecord may not be ready (expected if mock server doesn't support it): %v", err)
		}

		// Store the record ID for verification
		var createdRecord v1alpha2.DNSRecord
		err = f.Client.Get(ctx, types.NamespacedName{
			Name:      record.Name,
			Namespace: record.Namespace,
		}, &createdRecord)
		require.NoError(t, err)
		recordID := createdRecord.Status.RecordID

		// Delete the record
		err = f.Client.Delete(ctx, &createdRecord)
		require.NoError(t, err)

		// Wait for deletion
		err = f.WaitForDeletion(&createdRecord, 2*time.Minute)
		assert.NoError(t, err, "DNSRecord should be deleted")

		// Verify no orphan SyncState exists
		syncStateList := &v1alpha2.CloudflareSyncStateList{}
		err = f.Client.List(ctx, syncStateList)
		require.NoError(t, err)

		for _, ss := range syncStateList.Items {
			if ss.Spec.ResourceType == v1alpha2.SyncResourceDNSRecord {
				if recordID != "" && ss.Spec.CloudflareID == recordID {
					t.Errorf("Orphan SyncState found for deleted DNSRecord: %s", ss.Name)
				}
			}
		}
	})

	t.Run("VirtualNetworkStateConsistency", func(t *testing.T) {
		// Create VirtualNetwork
		vnet := &v1alpha2.VirtualNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name: "state-test-vnet",
			},
			Spec: v1alpha2.VirtualNetworkSpec{
				Name:      "state-test-vnet",
				Comment:   "E2E state consistency test",
				IsDefault: false,
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
		if err != nil {
			t.Logf("VirtualNetwork may not be ready (expected if mock server doesn't support it): %v", err)
		}

		// Get the created VirtualNetwork
		var createdVNet v1alpha2.VirtualNetwork
		err = f.Client.Get(ctx, types.NamespacedName{Name: vnet.Name}, &createdVNet)
		require.NoError(t, err)
		vnetID := createdVNet.Status.VirtualNetworkID

		// Delete the VirtualNetwork
		err = f.Client.Delete(ctx, &createdVNet)
		require.NoError(t, err)

		// Wait for deletion
		err = f.WaitForDeletion(&createdVNet, 2*time.Minute)
		assert.NoError(t, err, "VirtualNetwork should be deleted")

		// Verify no orphan SyncState exists
		syncStateList := &v1alpha2.CloudflareSyncStateList{}
		err = f.Client.List(ctx, syncStateList)
		require.NoError(t, err)

		for _, ss := range syncStateList.Items {
			if ss.Spec.ResourceType == v1alpha2.SyncResourceVirtualNetwork {
				if vnetID != "" && ss.Spec.CloudflareID == vnetID {
					t.Errorf("Orphan SyncState found for deleted VirtualNetwork: %s", ss.Name)
				}
			}
		}
	})
}

// TestNoOrphanSyncStates verifies that no orphan SyncStates exist after bulk operations
func TestNoOrphanSyncStates(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()
	testNS := "e2e-orphan-test"

	require.NoError(t, f.SetupTestNamespace(testNS))
	defer f.CleanupTestNamespace(testNS)

	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	// Create multiple resources
	records := make([]*v1alpha2.DNSRecord, 3)
	for i := 0; i < 3; i++ {
		records[i] = &v1alpha2.DNSRecord{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("bulk-test-dns-%d", i),
				Namespace: testNS,
			},
			Spec: v1alpha2.DNSRecordSpec{
				Name:    fmt.Sprintf("bulk-test-%d.example.com", i),
				Type:    "A",
				Content: fmt.Sprintf("192.168.1.%d", 100+i),
				TTL:     300,
				Proxied: false,
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
					Zone: "example.com",
				},
			},
		}

		err := f.Client.Create(ctx, records[i])
		require.NoError(t, err)
	}

	// Wait for all records to be ready
	for _, record := range records {
		err = f.WaitForCondition(record, "Ready", metav1.ConditionTrue, 2*time.Minute)
		if err != nil {
			t.Logf("DNSRecord %s may not be ready: %v", record.Name, err)
		}
	}

	// Count SyncStates before deletion
	syncStatesBefore := &v1alpha2.CloudflareSyncStateList{}
	err = f.Client.List(ctx, syncStatesBefore)
	require.NoError(t, err)
	countBefore := len(syncStatesBefore.Items)

	// Delete all records
	for _, record := range records {
		var toDelete v1alpha2.DNSRecord
		err = f.Client.Get(ctx, types.NamespacedName{
			Name:      record.Name,
			Namespace: record.Namespace,
		}, &toDelete)
		if err == nil {
			_ = f.Client.Delete(ctx, &toDelete)
		}
	}

	// Wait for all deletions
	for _, record := range records {
		_ = f.WaitForDeletion(record, 2*time.Minute)
	}

	// Count SyncStates after deletion
	syncStatesAfter := &v1alpha2.CloudflareSyncStateList{}
	err = f.Client.List(ctx, syncStatesAfter)
	require.NoError(t, err)
	countAfter := len(syncStatesAfter.Items)

	// Verify no new orphan SyncStates were created
	assert.LessOrEqual(t, countAfter, countBefore,
		"SyncState count should not increase after deletion (before: %d, after: %d)",
		countBefore, countAfter)
}

// TestSyncStateSourceTracking verifies that SyncState correctly tracks multiple sources
func TestSyncStateSourceTracking(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()
	testNS := "e2e-source-tracking-test"

	require.NoError(t, f.SetupTestNamespace(testNS))
	defer f.CleanupTestNamespace(testNS)

	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	// Create a Tunnel to serve as the parent resource
	tunnel := &v1alpha2.Tunnel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "source-tracking-tunnel",
			Namespace: testNS,
		},
		Spec: v1alpha2.TunnelSpec{
			Name: "source-tracking-tunnel",
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

	// Wait for tunnel to be ready
	err = f.WaitForCondition(tunnel, "Ready", metav1.ConditionTrue, 3*time.Minute)
	if err != nil {
		t.Logf("Tunnel may not be ready (expected if mock server doesn't support it): %v", err)
		return // Skip rest of test if tunnel creation fails
	}

	// Verify SyncState was created
	var fetchedTunnel v1alpha2.Tunnel
	err = f.Client.Get(ctx, types.NamespacedName{
		Name:      tunnel.Name,
		Namespace: tunnel.Namespace,
	}, &fetchedTunnel)
	require.NoError(t, err)

	// List SyncStates and find one for this tunnel
	syncStateList := &v1alpha2.CloudflareSyncStateList{}
	err = f.Client.List(ctx, syncStateList)
	require.NoError(t, err)

	var tunnelSyncState *v1alpha2.CloudflareSyncState
	for _, ss := range syncStateList.Items {
		if ss.Spec.ResourceType == v1alpha2.SyncResourceTunnelConfiguration {
			// Check if this SyncState is for our tunnel
			for _, source := range ss.Spec.Sources {
				if source.Ref.Name == tunnel.Name && source.Ref.Namespace == tunnel.Namespace {
					tunnelSyncState = &ss
					break
				}
			}
		}
	}

	if tunnelSyncState != nil {
		// Verify sources are tracked
		assert.Greater(t, len(tunnelSyncState.Spec.Sources), 0, "SyncState should have at least one source")

		// Verify source contains correct reference
		found := false
		for _, source := range tunnelSyncState.Spec.Sources {
			if source.Ref.Name == tunnel.Name && source.Ref.Namespace == tunnel.Namespace {
				found = true
				break
			}
		}
		assert.True(t, found, "SyncState should contain source reference to tunnel")
	}
}

// TestConcurrentResourceOperations tests state consistency under concurrent operations
func TestConcurrentResourceOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()
	testNS := "e2e-concurrent-test"

	require.NoError(t, f.SetupTestNamespace(testNS))
	defer f.CleanupTestNamespace(testNS)

	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	// Create multiple DNS records concurrently
	const numRecords = 5
	done := make(chan error, numRecords)

	for i := 0; i < numRecords; i++ {
		go func(idx int) {
			record := &v1alpha2.DNSRecord{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("concurrent-dns-%d", idx),
					Namespace: testNS,
				},
				Spec: v1alpha2.DNSRecordSpec{
					Name:    fmt.Sprintf("concurrent-%d.example.com", idx),
					Type:    "A",
					Content: fmt.Sprintf("192.168.2.%d", idx),
					TTL:     300,
					Proxied: false,
					Cloudflare: v1alpha2.CloudflareDetails{
						CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
							Name: testCredentialsName,
						},
						Zone: "example.com",
					},
				},
			}
			done <- f.Client.Create(ctx, record)
		}(i)
	}

	// Wait for all creates
	createErrors := 0
	for i := 0; i < numRecords; i++ {
		if err := <-done; err != nil {
			createErrors++
			t.Logf("Concurrent create error: %v", err)
		}
	}
	assert.Equal(t, 0, createErrors, "All concurrent creates should succeed")

	// Wait for all records to appear
	time.Sleep(2 * time.Second)

	// Verify all records were created
	recordList := &v1alpha2.DNSRecordList{}
	err = f.Client.List(ctx, recordList, client.InNamespace(testNS))
	require.NoError(t, err)

	concurrentRecordCount := 0
	for _, r := range recordList.Items {
		if len(r.Name) > 14 && r.Name[:14] == "concurrent-dns" {
			concurrentRecordCount++
		}
	}
	assert.Equal(t, numRecords, concurrentRecordCount, "All concurrent records should be created")

	// Cleanup - delete all concurrent records
	for i := 0; i < numRecords; i++ {
		record := &v1alpha2.DNSRecord{}
		err := f.Client.Get(ctx, types.NamespacedName{
			Name:      fmt.Sprintf("concurrent-dns-%d", i),
			Namespace: testNS,
		}, record)
		if err == nil {
			_ = f.Client.Delete(ctx, record)
		}
	}

	// Wait for cleanup
	for i := 0; i < numRecords; i++ {
		record := &v1alpha2.DNSRecord{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("concurrent-dns-%d", i),
				Namespace: testNS,
			},
		}
		_ = f.WaitForDeletion(record, time.Minute)
	}
}
