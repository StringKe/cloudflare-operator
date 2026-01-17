// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package resolver

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = networkingv1alpha2.AddToScheme(scheme)
	return scheme
}

func createTestDomain(name, domain, zoneID, accountID string, isDefault bool) *networkingv1alpha2.CloudflareDomain {
	return &networkingv1alpha2.CloudflareDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: networkingv1alpha2.CloudflareDomainSpec{
			Domain:    domain,
			IsDefault: isDefault,
		},
		Status: networkingv1alpha2.CloudflareDomainStatus{
			ZoneID:    zoneID,
			AccountID: accountID,
		},
	}
}

func TestNewDomainResolver(t *testing.T) {
	scheme := newTestScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	resolver := NewDomainResolver(client, logr.Discard())

	assert.NotNil(t, resolver)
	assert.NotNil(t, resolver.client)
}

func TestResolve_ExactMatch(t *testing.T) {
	scheme := newTestScheme()

	domain := createTestDomain("example-com", "example.com", "zone-123", "account-123", false)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(domain).
		Build()

	resolver := NewDomainResolver(client, logr.Discard())
	ctx := context.Background()

	info, err := resolver.Resolve(ctx, "example.com")

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "example.com", info.Domain)
	assert.Equal(t, "zone-123", info.ZoneID)
	assert.Equal(t, "account-123", info.AccountID)
	assert.Equal(t, "example-com", info.CloudflareDomainName)
}

func TestResolve_SuffixMatch(t *testing.T) {
	scheme := newTestScheme()

	domain := createTestDomain("example-com", "example.com", "zone-123", "account-123", false)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(domain).
		Build()

	resolver := NewDomainResolver(client, logr.Discard())
	ctx := context.Background()

	info, err := resolver.Resolve(ctx, "api.example.com")

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "example.com", info.Domain)
	assert.Equal(t, "zone-123", info.ZoneID)
}

func TestResolve_LongestSuffixMatch(t *testing.T) {
	scheme := newTestScheme()

	// Create two domains where one is a subdomain of the other
	baseDomain := createTestDomain("example-com", "example.com", "zone-base", "account-123", false)
	subDomain := createTestDomain("staging-example-com", "staging.example.com", "zone-staging", "account-123", false)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(baseDomain, subDomain).
		Build()

	resolver := NewDomainResolver(client, logr.Discard())
	ctx := context.Background()

	// Should match the longer suffix (staging.example.com)
	info, err := resolver.Resolve(ctx, "api.staging.example.com")

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "staging.example.com", info.Domain)
	assert.Equal(t, "zone-staging", info.ZoneID)

	// A direct subdomain of example.com should match example.com
	info2, err := resolver.Resolve(ctx, "api.example.com")

	require.NoError(t, err)
	require.NotNil(t, info2)
	assert.Equal(t, "example.com", info2.Domain)
	assert.Equal(t, "zone-base", info2.ZoneID)
}

func TestResolve_DefaultFallback(t *testing.T) {
	scheme := newTestScheme()

	// Create a default domain
	defaultDomain := createTestDomain("default-domain", "default.com", "zone-default", "account-123", true)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(defaultDomain).
		Build()

	resolver := NewDomainResolver(client, logr.Discard())
	ctx := context.Background()

	// Non-matching hostname should fall back to default
	info, err := resolver.Resolve(ctx, "some.other.domain.com")

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "default.com", info.Domain)
	assert.Equal(t, "zone-default", info.ZoneID)
}

func TestResolve_NoMatch(t *testing.T) {
	scheme := newTestScheme()

	domain := createTestDomain("example-com", "example.com", "zone-123", "account-123", false)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(domain).
		Build()

	resolver := NewDomainResolver(client, logr.Discard())
	ctx := context.Background()

	// Hostname that doesn't match any domain
	info, err := resolver.Resolve(ctx, "other.domain.com")

	require.NoError(t, err)
	assert.Nil(t, info)
}

func TestResolve_SkipDomainWithoutZoneID(t *testing.T) {
	scheme := newTestScheme()

	// Domain without ZoneID (still pending verification)
	pendingDomain := createTestDomain("pending-com", "pending.com", "", "account-123", false)
	readyDomain := createTestDomain("ready-com", "ready.com", "zone-ready", "account-123", false)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pendingDomain, readyDomain).
		Build()

	resolver := NewDomainResolver(client, logr.Discard())
	ctx := context.Background()

	// Should not match pending domain
	info, err := resolver.Resolve(ctx, "api.pending.com")

	require.NoError(t, err)
	assert.Nil(t, info)

	// Should match ready domain
	info2, err := resolver.Resolve(ctx, "api.ready.com")

	require.NoError(t, err)
	require.NotNil(t, info2)
	assert.Equal(t, "ready.com", info2.Domain)
}

func TestResolveMultiple(t *testing.T) {
	scheme := newTestScheme()

	domain1 := createTestDomain("example-com", "example.com", "zone-1", "account-123", false)
	domain2 := createTestDomain("test-com", "test.com", "zone-2", "account-123", false)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(domain1, domain2).
		Build()

	resolver := NewDomainResolver(client, logr.Discard())
	ctx := context.Background()

	hostnames := []string{"api.example.com", "www.test.com", "other.domain.com"}
	result, err := resolver.ResolveMultiple(ctx, hostnames)

	require.NoError(t, err)
	assert.Len(t, result, 2) // Only 2 matched

	assert.Equal(t, "example.com", result["api.example.com"].Domain)
	assert.Equal(t, "test.com", result["www.test.com"].Domain)
	assert.Nil(t, result["other.domain.com"])
}

func TestGetZoneID(t *testing.T) {
	scheme := newTestScheme()

	domain := createTestDomain("example-com", "example.com", "zone-123", "account-123", false)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(domain).
		Build()

	resolver := NewDomainResolver(client, logr.Discard())
	ctx := context.Background()

	zoneID, err := resolver.GetZoneID(ctx, "api.example.com")

	require.NoError(t, err)
	assert.Equal(t, "zone-123", zoneID)

	// Non-matching hostname
	zoneID2, err := resolver.GetZoneID(ctx, "other.domain.com")

	require.NoError(t, err)
	assert.Equal(t, "", zoneID2)
}

func TestMustResolve_Success(t *testing.T) {
	scheme := newTestScheme()

	domain := createTestDomain("example-com", "example.com", "zone-123", "account-123", false)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(domain).
		Build()

	resolver := NewDomainResolver(client, logr.Discard())
	ctx := context.Background()

	info, err := resolver.MustResolve(ctx, "api.example.com")

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "example.com", info.Domain)
}

func TestMustResolve_Error(t *testing.T) {
	scheme := newTestScheme()

	domain := createTestDomain("example-com", "example.com", "zone-123", "account-123", false)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(domain).
		Build()

	resolver := NewDomainResolver(client, logr.Discard())
	ctx := context.Background()

	info, err := resolver.MustResolve(ctx, "other.domain.com")

	assert.Error(t, err)
	assert.Nil(t, info)
	assert.Contains(t, err.Error(), "no CloudflareDomain found for hostname")
}

func TestInvalidateCache(t *testing.T) {
	scheme := newTestScheme()

	domain := createTestDomain("example-com", "example.com", "zone-123", "account-123", false)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(domain).
		Build()

	resolver := NewDomainResolver(client, logr.Discard())
	ctx := context.Background()

	// First resolve to populate cache
	_, err := resolver.Resolve(ctx, "api.example.com")
	require.NoError(t, err)

	// Check cache is populated
	resolver.mu.RLock()
	assert.Len(t, resolver.domains, 1)
	resolver.mu.RUnlock()

	// Invalidate cache
	resolver.InvalidateCache()

	// Check cache is cleared
	resolver.mu.RLock()
	assert.Nil(t, resolver.domains)
	resolver.mu.RUnlock()
}

func TestListDomains(t *testing.T) {
	scheme := newTestScheme()

	domain1 := createTestDomain("example-com", "example.com", "zone-1", "account-123", false)
	domain2 := createTestDomain("test-com", "test.com", "zone-2", "account-123", true)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(domain1, domain2).
		Build()

	resolver := NewDomainResolver(client, logr.Discard())
	ctx := context.Background()

	domains, err := resolver.ListDomains(ctx)

	require.NoError(t, err)
	assert.Len(t, domains, 2)
}

func TestGetDefaultDomain(t *testing.T) {
	scheme := newTestScheme()

	regularDomain := createTestDomain("regular-com", "regular.com", "zone-1", "account-123", false)
	defaultDomain := createTestDomain("default-com", "default.com", "zone-default", "account-123", true)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(regularDomain, defaultDomain).
		Build()

	resolver := NewDomainResolver(client, logr.Discard())
	ctx := context.Background()

	info, err := resolver.GetDefaultDomain(ctx)

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "default.com", info.Domain)
	assert.Equal(t, "zone-default", info.ZoneID)
}

func TestGetDefaultDomain_NoDefault(t *testing.T) {
	scheme := newTestScheme()

	regularDomain := createTestDomain("regular-com", "regular.com", "zone-1", "account-123", false)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(regularDomain).
		Build()

	resolver := NewDomainResolver(client, logr.Discard())
	ctx := context.Background()

	info, err := resolver.GetDefaultDomain(ctx)

	require.NoError(t, err)
	assert.Nil(t, info)
}

func TestGetDefaultDomain_DefaultWithoutZoneID(t *testing.T) {
	scheme := newTestScheme()

	// Default domain without ZoneID (still pending)
	pendingDefault := createTestDomain("pending-default", "pending.com", "", "account-123", true)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pendingDefault).
		Build()

	resolver := NewDomainResolver(client, logr.Discard())
	ctx := context.Background()

	info, err := resolver.GetDefaultDomain(ctx)

	require.NoError(t, err)
	assert.Nil(t, info) // Should not return pending domain
}

func TestResolve_EmptyDomainList(t *testing.T) {
	scheme := newTestScheme()

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	resolver := NewDomainResolver(client, logr.Discard())
	ctx := context.Background()

	info, err := resolver.Resolve(ctx, "api.example.com")

	require.NoError(t, err)
	assert.Nil(t, info)
}

func TestResolve_CredentialsRef(t *testing.T) {
	scheme := newTestScheme()

	domain := &networkingv1alpha2.CloudflareDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name: "example-com",
		},
		Spec: networkingv1alpha2.CloudflareDomainSpec{
			Domain: "example.com",
			CredentialsRef: &networkingv1alpha2.CredentialsReference{
				Name: "my-credentials",
			},
		},
		Status: networkingv1alpha2.CloudflareDomainStatus{
			ZoneID:    "zone-123",
			AccountID: "account-123",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(domain).
		Build()

	resolver := NewDomainResolver(client, logr.Discard())
	ctx := context.Background()

	info, err := resolver.Resolve(ctx, "api.example.com")

	require.NoError(t, err)
	require.NotNil(t, info)
	require.NotNil(t, info.CredentialsRef)
	assert.Equal(t, "my-credentials", info.CredentialsRef.Name)
}

func TestResolve_ConcurrentAccess(t *testing.T) {
	scheme := newTestScheme()

	domain := createTestDomain("example-com", "example.com", "zone-123", "account-123", false)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(domain).
		Build()

	resolver := NewDomainResolver(client, logr.Discard())
	ctx := context.Background()

	// Run multiple concurrent resolves
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			info, err := resolver.Resolve(ctx, "api.example.com")
			assert.NoError(t, err)
			assert.NotNil(t, info)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
