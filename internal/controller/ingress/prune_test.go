// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package ingress

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/controller/tunnelconfig"
)

const pruneTestNamespace = "cloudflare-operator-system"

const pruneTestConfigName = "cf-tunnel"

func pruneTestIngress(name string, services ...string) *networkingv1.Ingress {
	paths := make([]networkingv1.HTTPIngressPath, 0, len(services))
	for _, svc := range services {
		paths = append(paths, networkingv1.HTTPIngressPath{
			Path:     "/" + svc,
			PathType: ptr.To(networkingv1.PathTypePrefix),
			Backend: networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: svc,
					Port: networkingv1.ServiceBackendPort{Number: 80},
				},
			},
		})
	}

	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			IngressClassName: ptr.To(pruneTestClassName),
			Rules: []networkingv1.IngressRule{{
				Host: "app.example.com",
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{Paths: paths},
				},
			}},
		},
	}
}

const pruneTestClassName = "cloudflare-tunnel"

// pruneTestObjects returns the IngressClass, TunnelIngressClassConfig and Tunnel that
// the Ingress controller needs to build the aggregate for the "cloudflare-tunnel" class.
func pruneTestObjects() []client.Object {
	return []client.Object{
		&networkingv1.IngressClass{
			ObjectMeta: metav1.ObjectMeta{Name: pruneTestClassName},
			Spec: networkingv1.IngressClassSpec{
				Controller: ControllerName,
				Parameters: &networkingv1.IngressClassParametersReference{
					APIGroup:  ptr.To("networking.cloudflare-operator.io"),
					Kind:      "TunnelIngressClassConfig",
					Name:      pruneTestConfigName,
					Namespace: ptr.To(pruneTestNamespace),
				},
			},
		},
		&networkingv1alpha2.TunnelIngressClassConfig{
			ObjectMeta: metav1.ObjectMeta{Name: pruneTestConfigName, Namespace: pruneTestNamespace},
			Spec: networkingv1alpha2.TunnelIngressClassConfigSpec{
				TunnelRef: networkingv1alpha2.TunnelReference{Kind: "Tunnel", Name: "my-tunnel"},
			},
		},
		&networkingv1alpha2.Tunnel{
			ObjectMeta: metav1.ObjectMeta{Name: "my-tunnel", Namespace: pruneTestNamespace},
			Status: networkingv1alpha2.TunnelStatus{
				TunnelId:   "tid-123",
				TunnelName: "my-tunnel",
				AccountId:  "acc-123",
			},
		},
	}
}

func pruneTestReconciler(t *testing.T, objs ...client.Object) (*Reconciler, client.Client) {
	t.Helper()

	scheme := setupTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&networkingv1alpha2.TunnelIngressClassConfig{}).
		WithObjects(objs...).
		Build()

	return &Reconciler{
		Client:            c,
		Scheme:            scheme,
		Recorder:          record.NewFakeRecorder(20),
		OperatorNamespace: pruneTestNamespace,
	}, c
}

func readAggregatedRules(t *testing.T, ctx context.Context, c client.Client) []tunnelconfig.IngressRule {
	t.Helper()

	writer := tunnelconfig.NewWriter(c, pruneTestNamespace)
	config, err := writer.GetTunnelConfig(ctx, "tid-123")
	require.NoError(t, err)
	require.NotNil(t, config, "tunnel config ConfigMap missing")

	source, ok := config.Sources["Ingress/"+pruneTestNamespace+"/"+pruneTestConfigName]
	require.True(t, ok, "aggregated Ingress source missing: %v", config.Sources)

	return source.Rules
}

// TestRebuildTunnelConfigPrunesRemovedIngressPath verifies that removing a path from an
// Ingress removes the corresponding rule from the aggregated source (issue #49).
func TestRebuildTunnelConfigPrunesRemovedIngressPath(t *testing.T) {
	ctx := context.Background()

	ingress := pruneTestIngress("web", "one", "two")
	objs := append(pruneTestObjects(), ingress)
	r, c := pruneTestReconciler(t, objs...)

	config := &networkingv1alpha2.TunnelIngressClassConfig{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: pruneTestNamespace, Name: pruneTestConfigName}, config))

	require.NoError(t, r.reconcileIngressConfig(ctx, ingress, config))
	require.Len(t, readAggregatedRules(t, ctx, c), 2)

	// Remove the /two path, as a user editing the Ingress would.
	updated := &networkingv1.Ingress{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(ingress), updated))
	updated.Spec.Rules[0].HTTP.Paths = updated.Spec.Rules[0].HTTP.Paths[:1]
	require.NoError(t, c.Update(ctx, updated))

	require.NoError(t, r.reconcileIngressConfig(ctx, updated, config))

	rules := readAggregatedRules(t, ctx, c)
	require.Len(t, rules, 1)
	assert.Equal(t, "/one(/.*)?$", rules[0].Path)
	assert.Equal(t, "http://one.default.svc:80", rules[0].Service)
}

// TestHandleIngressDeletionRebuildsAllConfigs verifies that a deletion we only observe as
// "object is gone" still rebuilds every class aggregate. Previously this was a no-op, so
// rules that went away with the deleted Ingress were never pruned.
func TestHandleIngressDeletionRebuildsAllConfigs(t *testing.T) {
	ctx := context.Background()

	// The tunnel ConfigMap still holds the rule of an Ingress that no longer exists.
	objs := append(pruneTestObjects(), pruneTestIngress("web", "one"))
	r, c := pruneTestReconciler(t, objs...)

	staleWriter := tunnelconfig.NewWriter(c, pruneTestNamespace)
	require.NoError(t, staleWriter.WriteSourceConfig(ctx, "tid-123", "acc-123", &tunnelconfig.SourceConfig{
		Kind:      tunnelconfig.SourceKindIngress,
		Namespace: pruneTestNamespace,
		Name:      pruneTestConfigName,
		Rules: []tunnelconfig.IngressRule{
			{Hostname: "app.example.com", Path: "/one(/.*)?$", Service: "http://one.default.svc:80", Priority: tunnelconfig.PriorityIngress},
			{Hostname: "app.example.com", Path: "/two(/.*)?$", Service: "http://two.default.svc:80", Priority: tunnelconfig.PriorityIngress},
		},
	}, nil, metav1.GroupVersionKind{}))
	require.Len(t, readAggregatedRules(t, ctx, c), 2)

	_, err := r.handleIngressDeletion(ctx, ctrl.Request{
		NamespacedName: client.ObjectKey{Namespace: "default", Name: "deleted"},
	})
	require.NoError(t, err)

	rules := readAggregatedRules(t, ctx, c)
	require.Len(t, rules, 1)
	assert.Equal(t, "/one(/.*)?$", rules[0].Path)
}

// TestReconcileSurfacesRebuildFailureOnConfig verifies that a failed rebuild marks the
// TunnelIngressClassConfig as not ready instead of leaving it Ready=True with stale rules.
func TestReconcileSurfacesRebuildFailureOnConfig(t *testing.T) {
	ctx := context.Background()

	objs := []client.Object{
		&networkingv1.IngressClass{
			ObjectMeta: metav1.ObjectMeta{Name: pruneTestClassName},
			Spec: networkingv1.IngressClassSpec{
				Controller: ControllerName,
				Parameters: &networkingv1.IngressClassParametersReference{
					APIGroup:  ptr.To("networking.cloudflare-operator.io"),
					Kind:      "TunnelIngressClassConfig",
					Name:      pruneTestConfigName,
					Namespace: ptr.To(pruneTestNamespace),
				},
			},
		},
		&networkingv1alpha2.TunnelIngressClassConfig{
			ObjectMeta: metav1.ObjectMeta{Name: pruneTestConfigName, Namespace: pruneTestNamespace},
			Spec: networkingv1alpha2.TunnelIngressClassConfigSpec{
				// Referenced tunnel does not exist, so the rebuild must fail.
				TunnelRef: networkingv1alpha2.TunnelReference{Kind: "Tunnel", Name: "missing-tunnel"},
			},
		},
		pruneTestIngress("web", "one"),
	}
	r, c := pruneTestReconciler(t, objs...)

	ingress := &networkingv1.Ingress{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "web"}, ingress))

	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(ingress)})
	require.Error(t, err)

	config := &networkingv1alpha2.TunnelIngressClassConfig{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: pruneTestNamespace, Name: pruneTestConfigName}, config))
	assert.Equal(t, "error", config.Status.State)

	ready := meta.FindStatusCondition(config.Status.Conditions, "Ready")
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionFalse, ready.Status)
	assert.Contains(t, ready.Message, "missing-tunnel")
}
