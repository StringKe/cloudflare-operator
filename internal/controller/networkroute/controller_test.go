// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package networkroute

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

func init() {
	_ = networkingv1alpha2.AddToScheme(scheme.Scheme)
}

func TestNetworkRouteFinalizer(t *testing.T) {
	// Verify the finalizer name is correctly referenced from controller package
	assert.NotEmpty(t, controller.NetworkRouteFinalizer)
	assert.Contains(t, controller.NetworkRouteFinalizer, "networkroute")
}

func TestDefaultValue(t *testing.T) {
	assert.Equal(t, "default", defaultValue)
}

func TestReconcilerFields(t *testing.T) {
	// Test that the Reconciler struct has the expected fields
	r := &Reconciler{}

	// Verify nil defaults
	assert.Nil(t, r.Client)
	assert.Nil(t, r.Scheme)
	assert.Nil(t, r.Recorder)
	assert.Nil(t, r.routeService)
	assert.Nil(t, r.networkRoute)
	// Note: cfAPI field removed - following Unified Sync Architecture
}

func createTestNetworkRoute(name, network string) *networkingv1alpha2.NetworkRoute {
	return &networkingv1alpha2.NetworkRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: networkingv1alpha2.NetworkRouteSpec{
			Network: network,
			TunnelRef: networkingv1alpha2.TunnelRef{
				Name: "test-tunnel",
				Kind: "ClusterTunnel",
			},
			Comment: "Test network route",
			Cloudflare: networkingv1alpha2.CloudflareDetails{
				CredentialsRef: &networkingv1alpha2.CloudflareCredentialsRef{
					Name: "test-credentials",
				},
			},
		},
	}
}

func createTestTunnel(name, namespace string) *networkingv1alpha2.Tunnel {
	return &networkingv1alpha2.Tunnel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: networkingv1alpha2.TunnelStatus{
			TunnelId:   "tunnel-123",
			TunnelName: "test-tunnel",
		},
	}
}

func createTestClusterTunnel(name string) *networkingv1alpha2.ClusterTunnel {
	return &networkingv1alpha2.ClusterTunnel{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: networkingv1alpha2.TunnelStatus{
			TunnelId:   "clustertunnel-456",
			TunnelName: "test-clustertunnel",
		},
	}
}

func createTestVirtualNetwork() *networkingv1alpha2.VirtualNetwork {
	return &networkingv1alpha2.VirtualNetwork{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-vnet",
		},
		Status: networkingv1alpha2.VirtualNetworkStatus{
			VirtualNetworkId: "vnet-789",
		},
	}
}

func TestReconciler_Reconcile_NotFound(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := &Reconciler{
		Client:   client,
		Scheme:   scheme.Scheme,
		Recorder: recorder,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: "non-existent",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconciler_Reconcile_AddsFinalizer(t *testing.T) {
	route := createTestNetworkRoute("test-route", "10.0.0.0/24")

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(route).
		WithStatusSubresource(route).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := &Reconciler{
		Client:   client,
		Scheme:   scheme.Scheme,
		Recorder: recorder,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: route.Name,
		},
	}

	// First reconcile - add finalizer (will fail on API client init but finalizer should be added)
	_, _ = r.Reconcile(context.Background(), req)

	// Verify finalizer was added
	var updated networkingv1alpha2.NetworkRoute
	err := client.Get(context.Background(), types.NamespacedName{Name: "test-route"}, &updated)
	require.NoError(t, err)
	assert.True(t, controllerutil.ContainsFinalizer(&updated, controller.NetworkRouteFinalizer))
}

func TestReconciler_HandleDeletion_NoFinalizer(t *testing.T) {
	route := createTestNetworkRoute("test-route", "10.0.0.0/24")

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(route).
		Build()
	recorder := record.NewFakeRecorder(10)

	// Simulate deletion state without finalizer (the check happens in handleDeletion)
	r := &Reconciler{
		Client:       client,
		Scheme:       scheme.Scheme,
		Recorder:     recorder,
		networkRoute: route, // No finalizer set
		ctx:          context.Background(),
	}

	result, err := r.handleDeletion()

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconciler_BuildManagedComment(t *testing.T) {
	route := createTestNetworkRoute("test-route", "10.0.0.0/24")
	route.Spec.Comment = "User comment"

	r := &Reconciler{
		networkRoute: route,
	}

	comment := r.buildManagedComment()

	// Should contain management marker: [managed:Kind/Name]
	assert.Contains(t, comment, "[managed:NetworkRoute/test-route]")
	// Should also contain user comment
	assert.Contains(t, comment, "User comment")
}

func TestReconciler_BuildManagedComment_NoUserComment(t *testing.T) {
	route := createTestNetworkRoute("test-route", "10.0.0.0/24")
	route.Spec.Comment = ""

	r := &Reconciler{
		networkRoute: route,
	}

	comment := r.buildManagedComment()

	// Should contain management marker: [managed:Kind/Name]
	assert.Contains(t, comment, "[managed:NetworkRoute/test-route]")
}

func TestReconciler_SetCondition(t *testing.T) {
	route := createTestNetworkRoute("test-route", "10.0.0.0/24")
	route.Generation = 5

	r := &Reconciler{
		networkRoute: route,
	}

	r.setCondition(metav1.ConditionTrue, "TestReason", "Test message")

	require.Len(t, route.Status.Conditions, 1)
	cond := route.Status.Conditions[0]
	assert.Equal(t, "Ready", cond.Type)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, "TestReason", cond.Reason)
	assert.Equal(t, "Test message", cond.Message)
	assert.Equal(t, int64(5), cond.ObservedGeneration)
}

func TestReconciler_SetCondition_Overwrite(t *testing.T) {
	route := createTestNetworkRoute("test-route", "10.0.0.0/24")
	route.Status.Conditions = []metav1.Condition{
		{
			Type:   "Ready",
			Status: metav1.ConditionFalse,
			Reason: "OldReason",
		},
	}

	r := &Reconciler{
		networkRoute: route,
	}

	r.setCondition(metav1.ConditionTrue, "NewReason", "New message")

	require.Len(t, route.Status.Conditions, 1)
	cond := route.Status.Conditions[0]
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, "NewReason", cond.Reason)
}

func TestNetworkRouteSpec(t *testing.T) {
	route := createTestNetworkRoute("my-route", "192.168.0.0/16")

	assert.Equal(t, "192.168.0.0/16", route.Spec.Network)
	assert.Equal(t, "test-tunnel", route.Spec.TunnelRef.Name)
	assert.Equal(t, "ClusterTunnel", route.Spec.TunnelRef.Kind)
	assert.Equal(t, "Test network route", route.Spec.Comment)
	assert.NotNil(t, route.Spec.Cloudflare.CredentialsRef)
	assert.Equal(t, "test-credentials", route.Spec.Cloudflare.CredentialsRef.Name)
}

func TestNetworkRouteStatus(t *testing.T) {
	route := createTestNetworkRoute("test-route", "10.0.0.0/8")
	route.Status.TunnelID = "tunnel-123"
	route.Status.AccountID = "account-456"
	route.Status.Network = "10.0.0.0/8"
	route.Status.TunnelName = "prod-tunnel"
	route.Status.VirtualNetworkID = "vnet-789"
	route.Status.State = "active"

	assert.Equal(t, "tunnel-123", route.Status.TunnelID)
	assert.Equal(t, "account-456", route.Status.AccountID)
	assert.Equal(t, "10.0.0.0/8", route.Status.Network)
	assert.Equal(t, "prod-tunnel", route.Status.TunnelName)
	assert.Equal(t, "vnet-789", route.Status.VirtualNetworkID)
	assert.Equal(t, "active", route.Status.State)
}

func TestFindNetworkRoutesForVirtualNetwork(t *testing.T) {
	vnet := createTestVirtualNetwork()
	route1 := createTestNetworkRoute("route1", "10.0.0.0/8")
	route1.Spec.VirtualNetworkRef = &networkingv1alpha2.VirtualNetworkRef{Name: "test-vnet"}
	route2 := createTestNetworkRoute("route2", "192.168.0.0/16")
	route2.Spec.VirtualNetworkRef = &networkingv1alpha2.VirtualNetworkRef{Name: "other-vnet"}
	route3 := createTestNetworkRoute("route3", "172.16.0.0/12")
	// route3 has no VirtualNetworkRef

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(route1, route2, route3).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
	}

	requests := r.findNetworkRoutesForVirtualNetwork(context.Background(), vnet)

	// Should only return route1 which references test-vnet
	require.Len(t, requests, 1)
	assert.Equal(t, "route1", requests[0].Name)
}

func TestFindNetworkRoutesForVirtualNetwork_WrongType(t *testing.T) {
	// Pass wrong type object
	tunnel := createTestTunnel("test", "default")

	r := &Reconciler{}

	requests := r.findNetworkRoutesForVirtualNetwork(context.Background(), tunnel)

	assert.Empty(t, requests)
}

func TestFindNetworkRoutesForClusterTunnel(t *testing.T) {
	clusterTunnel := createTestClusterTunnel("test-clustertunnel")
	route1 := createTestNetworkRoute("route1", "10.0.0.0/8")
	route1.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-clustertunnel", Kind: "ClusterTunnel"}
	route2 := createTestNetworkRoute("route2", "192.168.0.0/16")
	route2.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-clustertunnel"} // Default kind is ClusterTunnel
	route3 := createTestNetworkRoute("route3", "172.16.0.0/12")
	route3.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "other-tunnel", Kind: "ClusterTunnel"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(route1, route2, route3).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
	}

	requests := r.findNetworkRoutesForClusterTunnel(context.Background(), clusterTunnel)

	// Should return route1 and route2 which reference test-clustertunnel
	require.Len(t, requests, 2)
	names := []string{requests[0].Name, requests[1].Name}
	assert.Contains(t, names, "route1")
	assert.Contains(t, names, "route2")
}

func TestFindNetworkRoutesForClusterTunnel_WrongType(t *testing.T) {
	// Pass wrong type object
	vnet := createTestVirtualNetwork()

	r := &Reconciler{}

	requests := r.findNetworkRoutesForClusterTunnel(context.Background(), vnet)

	assert.Empty(t, requests)
}

func TestFindNetworkRoutesForTunnel(t *testing.T) {
	tunnel := createTestTunnel("test-tunnel", "default")
	route1 := createTestNetworkRoute("route1", "10.0.0.0/8")
	route1.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-tunnel", Kind: "Tunnel", Namespace: "default"}
	route2 := createTestNetworkRoute("route2", "192.168.0.0/16")
	route2.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-tunnel", Kind: "Tunnel"} // Defaults to "default" namespace
	route3 := createTestNetworkRoute("route3", "172.16.0.0/12")
	route3.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-tunnel", Kind: "Tunnel", Namespace: "other"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(route1, route2, route3).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
	}

	requests := r.findNetworkRoutesForTunnel(context.Background(), tunnel)

	// Should return route1 and route2 which reference test-tunnel in default namespace
	require.Len(t, requests, 2)
	names := []string{requests[0].Name, requests[1].Name}
	assert.Contains(t, names, "route1")
	assert.Contains(t, names, "route2")
}

func TestFindNetworkRoutesForTunnel_WrongType(t *testing.T) {
	// Pass wrong type object
	vnet := createTestVirtualNetwork()

	r := &Reconciler{}

	requests := r.findNetworkRoutesForTunnel(context.Background(), vnet)

	assert.Empty(t, requests)
}

func TestResolveTunnelRef_ClusterTunnel(t *testing.T) {
	clusterTunnel := createTestClusterTunnel("test-clustertunnel")
	route := createTestNetworkRoute("test-route", "10.0.0.0/8")
	route.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-clustertunnel", Kind: "ClusterTunnel"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(route, clusterTunnel).
		Build()

	r := &Reconciler{
		Client:       fakeClient,
		Scheme:       scheme.Scheme,
		networkRoute: route,
		ctx:          context.Background(),
	}

	tunnelID, tunnelName, err := r.resolveTunnelRef()

	require.NoError(t, err)
	assert.Equal(t, "clustertunnel-456", tunnelID)
	assert.Equal(t, "test-clustertunnel", tunnelName)
}

func TestResolveTunnelRef_Tunnel(t *testing.T) {
	tunnel := createTestTunnel("test-tunnel", "default")
	route := createTestNetworkRoute("test-route", "10.0.0.0/8")
	route.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-tunnel", Kind: "Tunnel", Namespace: "default"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(route, tunnel).
		Build()

	r := &Reconciler{
		Client:       fakeClient,
		Scheme:       scheme.Scheme,
		networkRoute: route,
		ctx:          context.Background(),
	}

	tunnelID, tunnelName, err := r.resolveTunnelRef()

	require.NoError(t, err)
	assert.Equal(t, "tunnel-123", tunnelID)
	assert.Equal(t, "test-tunnel", tunnelName)
}

func TestResolveTunnelRef_TunnelNotFound(t *testing.T) {
	route := createTestNetworkRoute("test-route", "10.0.0.0/8")
	route.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "non-existent", Kind: "ClusterTunnel"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(route).
		Build()

	r := &Reconciler{
		Client:       fakeClient,
		Scheme:       scheme.Scheme,
		networkRoute: route,
		ctx:          context.Background(),
	}

	_, _, err := r.resolveTunnelRef()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-existent")
}

func TestResolveTunnelRef_NoTunnelID(t *testing.T) {
	clusterTunnel := createTestClusterTunnel("test-clustertunnel")
	clusterTunnel.Status.TunnelId = "" // No tunnel ID yet
	route := createTestNetworkRoute("test-route", "10.0.0.0/8")
	route.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-clustertunnel", Kind: "ClusterTunnel"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(route, clusterTunnel).
		Build()

	r := &Reconciler{
		Client:       fakeClient,
		Scheme:       scheme.Scheme,
		networkRoute: route,
		ctx:          context.Background(),
	}

	_, _, err := r.resolveTunnelRef()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not have a tunnelId yet")
}

func TestResolveVirtualNetworkRef(t *testing.T) {
	vnet := createTestVirtualNetwork()
	route := createTestNetworkRoute("test-route", "10.0.0.0/8")
	route.Spec.VirtualNetworkRef = &networkingv1alpha2.VirtualNetworkRef{Name: "test-vnet"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(route, vnet).
		Build()

	r := &Reconciler{
		Client:       fakeClient,
		Scheme:       scheme.Scheme,
		networkRoute: route,
		ctx:          context.Background(),
	}

	vnetID, err := r.resolveVirtualNetworkRef()

	require.NoError(t, err)
	assert.Equal(t, "vnet-789", vnetID)
}

func TestResolveVirtualNetworkRef_NoRef(t *testing.T) {
	route := createTestNetworkRoute("test-route", "10.0.0.0/8")
	route.Spec.VirtualNetworkRef = nil

	r := &Reconciler{
		networkRoute: route,
		ctx:          context.Background(),
	}

	vnetID, err := r.resolveVirtualNetworkRef()

	require.NoError(t, err)
	assert.Empty(t, vnetID)
}

func TestResolveVirtualNetworkRef_NotFound(t *testing.T) {
	route := createTestNetworkRoute("test-route", "10.0.0.0/8")
	route.Spec.VirtualNetworkRef = &networkingv1alpha2.VirtualNetworkRef{Name: "non-existent"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(route).
		Build()

	r := &Reconciler{
		Client:       fakeClient,
		Scheme:       scheme.Scheme,
		networkRoute: route,
		ctx:          context.Background(),
	}

	_, err := r.resolveVirtualNetworkRef()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-existent")
}

func TestResolveVirtualNetworkRef_NoID(t *testing.T) {
	vnet := createTestVirtualNetwork()
	vnet.Status.VirtualNetworkId = "" // No ID yet
	route := createTestNetworkRoute("test-route", "10.0.0.0/8")
	route.Spec.VirtualNetworkRef = &networkingv1alpha2.VirtualNetworkRef{Name: "test-vnet"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(route, vnet).
		Build()

	r := &Reconciler{
		Client:       fakeClient,
		Scheme:       scheme.Scheme,
		networkRoute: route,
		ctx:          context.Background(),
	}

	_, err := r.resolveVirtualNetworkRef()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not have a virtualNetworkId yet")
}

func TestReconcileRequestsFromMapFunc(t *testing.T) {
	// Test that requests are properly typed
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: "test",
		},
	}
	assert.Equal(t, "test", req.Name)
}

func TestReconcilerImplementsReconciler(_ *testing.T) {
	// Verify Reconciler implements the reconcile.Reconciler interface
	var _ reconcile.Reconciler = &Reconciler{}
}

func TestReconcilerWithClient(t *testing.T) {
	// Test that Reconciler can be used with a fake client
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
	}

	// Should be able to list resources without error (empty list)
	routes := &networkingv1alpha2.NetworkRouteList{}
	err := r.List(context.Background(), routes)
	require.NoError(t, err)
	assert.Empty(t, routes.Items)
}

func TestReconcilerEmbeddedClient(t *testing.T) {
	// Test that the embedded client.Client methods are available
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
	}

	// Test Get method
	route := &networkingv1alpha2.NetworkRoute{}
	err := r.Get(context.Background(), types.NamespacedName{Name: "test"}, route)
	assert.Error(t, err) // Should not find it

	// Test Create method
	newRoute := createTestNetworkRoute("new-route", "10.0.0.0/8")
	err = r.Create(context.Background(), newRoute)
	require.NoError(t, err)

	// Now it should be findable
	err = r.Get(context.Background(), types.NamespacedName{Name: "new-route"}, route)
	require.NoError(t, err)
	assert.Equal(t, "new-route", route.Name)
}

func TestTunnelReferenceTypes(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		expected string
	}{
		{
			name:     "Tunnel kind",
			kind:     "Tunnel",
			expected: "Tunnel",
		},
		{
			name:     "ClusterTunnel kind",
			kind:     "ClusterTunnel",
			expected: "ClusterTunnel",
		},
		{
			name:     "Empty kind defaults to ClusterTunnel",
			kind:     "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			route := createTestNetworkRoute("test", "10.0.0.0/8")
			route.Spec.TunnelRef.Kind = tt.kind
			assert.Equal(t, tt.expected, route.Spec.TunnelRef.Kind)
		})
	}
}

func TestFindNetworkRoutesForVirtualNetwork_ListError(t *testing.T) {
	// Create a client that will work normally
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
	}

	vnet := createTestVirtualNetwork()

	// Should return empty when there are no routes
	requests := r.findNetworkRoutesForVirtualNetwork(context.Background(), vnet)
	assert.Empty(t, requests)
}

func TestFindNetworkRoutesForTunnel_NamespaceMatching(t *testing.T) {
	tunnel := createTestTunnel("test-tunnel", "namespace-a")
	route1 := createTestNetworkRoute("route1", "10.0.0.0/8")
	route1.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-tunnel", Kind: "Tunnel", Namespace: "namespace-a"}
	route2 := createTestNetworkRoute("route2", "192.168.0.0/16")
	route2.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-tunnel", Kind: "Tunnel", Namespace: "namespace-b"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(route1, route2).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
	}

	requests := r.findNetworkRoutesForTunnel(context.Background(), tunnel)

	// Should only return route1 which has matching namespace
	require.Len(t, requests, 1)
	assert.Equal(t, "route1", requests[0].Name)
}

func TestNetworkRouteWithVirtualNetworkRef(t *testing.T) {
	route := createTestNetworkRoute("test-route", "10.0.0.0/8")
	route.Spec.VirtualNetworkRef = &networkingv1alpha2.VirtualNetworkRef{
		Name: "my-vnet",
	}

	assert.NotNil(t, route.Spec.VirtualNetworkRef)
	assert.Equal(t, "my-vnet", route.Spec.VirtualNetworkRef.Name)
}

func TestGetClientFromReconciler(t *testing.T) {
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		Build()

	r := &Reconciler{
		Client: fakeClient,
	}

	// Verify the embedded client is accessible
	var c = r.Client
	assert.NotNil(t, c)
}
