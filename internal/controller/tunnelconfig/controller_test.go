// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package tunnelconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testOperatorNamespace = "cloudflare-operator-system"

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	return scheme
}

func testConfigMap(t *testing.T, config *TunnelConfig) *corev1.ConfigMap {
	t.Helper()
	data, err := config.ToConfigMapData()
	require.NoError(t, err)

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName(config.TunnelID),
			Namespace: testOperatorNamespace,
			Labels: map[string]string{
				ConfigMapLabelTunnelID: config.TunnelID,
				ConfigMapLabelType:     ConfigMapTypeValue,
			},
		},
		Data: data,
	}
}

func testTunnelConfig(rules ...IngressRule) *TunnelConfig {
	return &TunnelConfig{
		TunnelID:  "tid-123",
		AccountID: "acc-123",
		Sources: map[string]*SourceConfig{
			"Ingress/cloudflare-operator-system/cf-tunnel": {
				Kind:      SourceKindIngress,
				Namespace: testOperatorNamespace,
				Name:      "cf-tunnel",
				Rules:     rules,
			},
		},
	}
}

// TestUpdateConfigMapPreservesConcurrentSourceWrites reproduces the lost-update that made
// rules removed from an Ingress stick around forever (issue #49): the tunnelconfig
// controller pushed a stale snapshot and then wrote that stale snapshot back into the
// ConfigMap, reverting a concurrent prune and recording its own hash as "Synced".
func TestUpdateConfigMapPreservesConcurrentSourceWrites(t *testing.T) {
	ctx := context.Background()

	staleRules := []IngressRule{
		{Hostname: "app.example.com", Path: "/one(/.*)?$", Service: "http://one.default.svc:80", Priority: PriorityIngress},
		{Hostname: "app.example.com", Path: "/two(/.*)?$", Service: "http://two.default.svc:80", Priority: PriorityIngress},
	}
	prunedRules := staleRules[:1]

	cm := testConfigMap(t, testTunnelConfig(staleRules...))
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(cm).Build()

	// Snapshot taken at the start of the tunnelconfig reconcile.
	staleCM := &corev1.ConfigMap{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(cm), staleCM))
	staleConfig, err := ParseConfig(staleCM)
	require.NoError(t, err)

	// Meanwhile the Ingress controller prunes the removed path.
	fresh := &corev1.ConfigMap{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(cm), fresh))
	freshConfig, err := ParseConfig(fresh)
	require.NoError(t, err)
	freshConfig.Sources["Ingress/cloudflare-operator-system/cf-tunnel"].Rules = prunedRules
	fresh.Data, err = freshConfig.ToConfigMapData()
	require.NoError(t, err)
	require.NoError(t, c.Update(ctx, fresh))

	// The tunnelconfig reconcile finishes after the push.
	now := metav1.Now()
	staleConfig.LastHash = "pushed-hash"
	staleConfig.SyncStatus = "Synced"
	staleConfig.LastSyncTime = &now

	r := &Reconciler{Client: c, OperatorNamespace: testOperatorNamespace}
	require.NoError(t, r.updateConfigMap(ctx, staleCM, staleConfig))

	updated := &corev1.ConfigMap{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(cm), updated))
	updatedConfig, err := ParseConfig(updated)
	require.NoError(t, err)

	rules := updatedConfig.Sources["Ingress/cloudflare-operator-system/cf-tunnel"].Rules
	require.Len(t, rules, 1, "concurrent prune was reverted by the status update")
	assert.Equal(t, "/one(/.*)?$", rules[0].Path)

	assert.Equal(t, "pushed-hash", updatedConfig.LastHash)
	assert.Equal(t, "Synced", updatedConfig.SyncStatus)
	assert.NotNil(t, updatedConfig.LastSyncTime)
}

// TestBuildCloudflareConfigAlwaysEndsWithCatchAll guards the empty-configuration case:
// when the last source is removed the pushed ingress list must still be valid, otherwise
// the push fails and the old (stale) rules stay active at the edge.
func TestBuildCloudflareConfigAlwaysEndsWithCatchAll(t *testing.T) {
	r := &Reconciler{}

	empty := r.buildCloudflareConfig(&TunnelConfig{
		TunnelID: "tid-123",
		Sources:  map[string]*SourceConfig{},
	})
	require.Len(t, empty.Ingress, 1)
	assert.Equal(t, "http_status:404", empty.Ingress[0].Service)

	withRules := r.buildCloudflareConfig(testTunnelConfig(
		IngressRule{Hostname: "app.example.com", Path: "/one(/.*)?$", Service: "http://one.default.svc:80", Priority: PriorityIngress},
	))
	require.Len(t, withRules.Ingress, 2)
	assert.Equal(t, "app.example.com", withRules.Ingress[0].Hostname)
	assert.Equal(t, "http_status:404", withRules.Ingress[1].Service)
}

// TestComputeHash only reacts to configuration content, so bookkeeping writes do not
// trigger pushes while a removed rule always does.
func TestComputeHash(t *testing.T) {
	rules := []IngressRule{
		{Hostname: "app.example.com", Path: "/one(/.*)?$", Service: "http://one.default.svc:80", Priority: PriorityIngress},
		{Hostname: "app.example.com", Path: "/two(/.*)?$", Service: "http://two.default.svc:80", Priority: PriorityIngress},
	}

	base := testTunnelConfig(rules...)
	baseHash := base.ComputeHash()

	// Timestamps and generation change on every write and must not affect the hash.
	bookkeepingOnly := testTunnelConfig(rules...)
	now := metav1.Now()
	bookkeepingOnly.LastSyncTime = &now
	bookkeepingOnly.SyncStatus = "Synced"
	bookkeepingOnly.LastHash = "something-else"
	bookkeepingOnly.Sources["Ingress/cloudflare-operator-system/cf-tunnel"].UpdatedAt = &now
	bookkeepingOnly.Sources["Ingress/cloudflare-operator-system/cf-tunnel"].Generation = 42

	assert.Equal(t, baseHash, bookkeepingOnly.ComputeHash())

	// Removing a rule - the issue #49 scenario - must change the hash.
	pruned := testTunnelConfig(rules[:1]...)
	assert.NotEqual(t, baseHash, pruned.ComputeHash())

	// Rule order within the aggregate must be stable regardless of map iteration order.
	other := &TunnelConfig{
		TunnelID:  base.TunnelID,
		AccountID: base.AccountID,
		Sources: map[string]*SourceConfig{
			"Ingress/second": {
				Kind:  SourceKindIngress,
				Name:  "second",
				Rules: rules[:1],
			},
			"Ingress/first": {
				Kind:  SourceKindIngress,
				Name:  "first",
				Rules: rules[1:],
			},
		},
	}
	reordered := &TunnelConfig{
		TunnelID:  other.TunnelID,
		AccountID: other.AccountID,
		Sources: map[string]*SourceConfig{
			"Ingress/first":  other.Sources["Ingress/first"],
			"Ingress/second": other.Sources["Ingress/second"],
		},
	}
	assert.Equal(t, other.ComputeHash(), reordered.ComputeHash())
}
