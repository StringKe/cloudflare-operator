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

// TestDNSRecordLifecycle tests the complete lifecycle of a DNSRecord resource
func TestDNSRecordLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()
	testNS := "e2e-dns-test"

	// Setup test namespace
	require.NoError(t, f.SetupTestNamespace(testNS))
	defer f.CleanupTestNamespace(testNS)

	t.Run("CreateARecord", func(t *testing.T) {
		record := &v1alpha2.DNSRecord{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-test-a-record",
				Namespace: testNS,
			},
			Spec: v1alpha2.DNSRecordSpec{
				Name:    "test.example.com",
				Type:    "A",
				Content: "192.168.1.1",
				TTL:     300,
				Proxied: true,
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: "test-credentials",
					},
				},
			},
		}

		err := f.Client.Create(ctx, record)
		require.NoError(t, err)

		// Wait for Ready condition
		err = f.WaitForCondition(record, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err, "DNSRecord should become ready")

		// Verify status
		var fetched v1alpha2.DNSRecord
		err = f.Client.Get(ctx, types.NamespacedName{
			Name:      record.Name,
			Namespace: record.Namespace,
		}, &fetched)
		require.NoError(t, err)
		assert.NotEmpty(t, fetched.Status.RecordID, "RecordID should be set")
	})

	t.Run("CreateCNAMERecord", func(t *testing.T) {
		record := &v1alpha2.DNSRecord{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-test-cname-record",
				Namespace: testNS,
			},
			Spec: v1alpha2.DNSRecordSpec{
				Name:    "alias.example.com",
				Type:    "CNAME",
				Content: "target.example.com",
				TTL:     1, // Automatic TTL
				Proxied: false,
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: "test-credentials",
					},
				},
			},
		}

		err := f.Client.Create(ctx, record)
		require.NoError(t, err)

		err = f.WaitForCondition(record, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err)
	})

	t.Run("UpdateDNSRecord", func(t *testing.T) {
		var record v1alpha2.DNSRecord
		err := f.Client.Get(ctx, types.NamespacedName{
			Name:      "e2e-test-a-record",
			Namespace: testNS,
		}, &record)
		require.NoError(t, err)

		// Update content
		record.Spec.Content = "192.168.1.2"
		err = f.Client.Update(ctx, &record)
		require.NoError(t, err)

		// Wait for reconciliation
		time.Sleep(5 * time.Second)

		// Verify update preserved
		err = f.Client.Get(ctx, types.NamespacedName{
			Name:      record.Name,
			Namespace: record.Namespace,
		}, &record)
		require.NoError(t, err)
		assert.Equal(t, "192.168.1.2", record.Spec.Content)
	})

	t.Run("DeleteDNSRecords", func(t *testing.T) {
		records := []string{"e2e-test-a-record", "e2e-test-cname-record"}

		for _, name := range records {
			var record v1alpha2.DNSRecord
			err := f.Client.Get(ctx, types.NamespacedName{
				Name:      name,
				Namespace: testNS,
			}, &record)
			if err != nil {
				continue // Already deleted or doesn't exist
			}

			err = f.Client.Delete(ctx, &record)
			require.NoError(t, err)

			err = f.WaitForDeletion(&record, time.Minute)
			assert.NoError(t, err, "DNSRecord %s should be deleted", name)
		}
	})
}

// TestDNSRecordWithMXPriority tests MX records with priority
func TestDNSRecordWithMXPriority(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()
	testNS := "e2e-dns-mx-test"

	require.NoError(t, f.SetupTestNamespace(testNS))
	defer f.CleanupTestNamespace(testNS)

	priority := 10
	record := &v1alpha2.DNSRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-test-mx-record",
			Namespace: testNS,
		},
		Spec: v1alpha2.DNSRecordSpec{
			Name:     "example.com",
			Type:     "MX",
			Content:  "mail.example.com",
			TTL:      300,
			Priority: &priority,
			Cloudflare: v1alpha2.CloudflareDetails{
				CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
					Name: "test-credentials",
				},
			},
		},
	}

	err = f.Client.Create(ctx, record)
	require.NoError(t, err)

	err = f.WaitForCondition(record, "Ready", metav1.ConditionTrue, 2*time.Minute)
	assert.NoError(t, err)

	// Cleanup
	_ = f.Client.Delete(ctx, record)
	_ = f.WaitForDeletion(record, time.Minute)
}
