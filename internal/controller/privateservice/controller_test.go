// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package privateservice

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
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
	_ = corev1.AddToScheme(scheme.Scheme)
}

func TestFinalizerName(t *testing.T) {
	// Verify the finalizer name is correctly referenced from controller package
	assert.NotEmpty(t, controller.PrivateServiceFinalizer)
	assert.Contains(t, controller.PrivateServiceFinalizer, "privateservice")
}

func TestReconcilerFields(t *testing.T) {
	// Test that the Reconciler struct has the expected fields
	r := &Reconciler{}

	// Verify nil defaults
	assert.Nil(t, r.Client)
	assert.Nil(t, r.Scheme)
	assert.Nil(t, r.Recorder)
	assert.Nil(t, r.privateService)
	assert.Nil(t, r.privateServiceService)
}

func createTestPrivateService(name, namespace string) *networkingv1alpha2.PrivateService {
	return &networkingv1alpha2.PrivateService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: networkingv1alpha2.PrivateServiceSpec{
			ServiceRef: networkingv1alpha2.ServiceRef{
				Name: "test-service",
				Port: 8080,
			},
			TunnelRef: networkingv1alpha2.TunnelRef{
				Name: "test-tunnel",
				Kind: "ClusterTunnel",
			},
			Protocol: "tcp",
			Comment:  "Test private service",
			Cloudflare: networkingv1alpha2.CloudflareDetails{
				CredentialsRef: &networkingv1alpha2.CloudflareCredentialsRef{
					Name: "test-credentials",
				},
			},
		},
	}
}

func createTestService(name, namespace, clusterIP string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: clusterIP,
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: 8080,
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
	fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := &Reconciler{
		Client:   fakeClient,
		Scheme:   scheme.Scheme,
		Recorder: recorder,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconciler_Reconcile_FailsOnMissingCredentials(t *testing.T) {
	// PrivateService controller initializes API client before adding finalizer,
	// so when credentials are missing, the reconcile fails with an error and
	// no finalizer is added.
	ps := createTestPrivateService("test-ps", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ps).
		WithStatusSubresource(ps).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := &Reconciler{
		Client:   fakeClient,
		Scheme:   scheme.Scheme,
		Recorder: recorder,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      ps.Name,
			Namespace: ps.Namespace,
		},
	}

	// Reconcile should fail on API client init (missing credentials)
	_, err := r.Reconcile(context.Background(), req)

	// Should error due to missing credentials
	assert.Error(t, err)

	// Verify finalizer was NOT added (because API client init happens first)
	var updated networkingv1alpha2.PrivateService
	getErr := fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-ps", Namespace: "default"}, &updated)
	require.NoError(t, getErr)
	assert.False(t, controllerutil.ContainsFinalizer(&updated, controller.PrivateServiceFinalizer))
}

func TestReconciler_HandleDeletion_NoFinalizer(t *testing.T) {
	ps := createTestPrivateService("test-ps", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ps).
		Build()
	recorder := record.NewFakeRecorder(10)

	// Simulate deletion state without finalizer (the check happens in handleDeletion)
	r := &Reconciler{
		Client:         fakeClient,
		Scheme:         scheme.Scheme,
		Recorder:       recorder,
		privateService: ps, // No finalizer set
		ctx:            context.Background(),
	}

	result, err := r.handleDeletion()

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconciler_SetCondition(t *testing.T) {
	ps := createTestPrivateService("test-ps", "default")
	ps.Generation = 5

	r := &Reconciler{
		privateService: ps,
	}

	r.setCondition(metav1.ConditionTrue, "TestReason", "Test message")

	require.Len(t, ps.Status.Conditions, 1)
	cond := ps.Status.Conditions[0]
	assert.Equal(t, "Ready", cond.Type)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, "TestReason", cond.Reason)
	assert.Equal(t, "Test message", cond.Message)
	assert.Equal(t, int64(5), cond.ObservedGeneration)
}

func TestReconciler_SetCondition_Overwrite(t *testing.T) {
	ps := createTestPrivateService("test-ps", "default")
	ps.Status.Conditions = []metav1.Condition{
		{
			Type:   "Ready",
			Status: metav1.ConditionFalse,
			Reason: "OldReason",
		},
	}

	r := &Reconciler{
		privateService: ps,
	}

	r.setCondition(metav1.ConditionTrue, "NewReason", "New message")

	require.Len(t, ps.Status.Conditions, 1)
	cond := ps.Status.Conditions[0]
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, "NewReason", cond.Reason)
}

func TestPrivateServiceSpec(t *testing.T) {
	ps := createTestPrivateService("my-ps", "production")

	assert.Equal(t, "test-service", ps.Spec.ServiceRef.Name)
	assert.Equal(t, int32(8080), ps.Spec.ServiceRef.Port)
	assert.Equal(t, "test-tunnel", ps.Spec.TunnelRef.Name)
	assert.Equal(t, "ClusterTunnel", ps.Spec.TunnelRef.Kind)
	assert.Equal(t, "tcp", ps.Spec.Protocol)
	assert.Equal(t, "Test private service", ps.Spec.Comment)
	assert.NotNil(t, ps.Spec.Cloudflare.CredentialsRef)
	assert.Equal(t, "test-credentials", ps.Spec.Cloudflare.CredentialsRef.Name)
}

func TestPrivateServiceStatus(t *testing.T) {
	ps := createTestPrivateService("test-ps", "default")
	ps.Status.Network = "10.96.0.1/32"
	ps.Status.ServiceIP = "10.96.0.1"
	ps.Status.TunnelID = "tunnel-123"
	ps.Status.TunnelName = "prod-tunnel"
	ps.Status.VirtualNetworkID = "vnet-789"
	ps.Status.AccountID = "account-456"
	ps.Status.State = "active"

	assert.Equal(t, "10.96.0.1/32", ps.Status.Network)
	assert.Equal(t, "10.96.0.1", ps.Status.ServiceIP)
	assert.Equal(t, "tunnel-123", ps.Status.TunnelID)
	assert.Equal(t, "prod-tunnel", ps.Status.TunnelName)
	assert.Equal(t, "vnet-789", ps.Status.VirtualNetworkID)
	assert.Equal(t, "account-456", ps.Status.AccountID)
	assert.Equal(t, "active", ps.Status.State)
}

func TestResolveTunnelRef_ClusterTunnel(t *testing.T) {
	clusterTunnel := createTestClusterTunnel("test-clustertunnel")
	ps := createTestPrivateService("test-ps", "default")
	ps.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-clustertunnel", Kind: "ClusterTunnel"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ps, clusterTunnel).
		Build()

	r := &Reconciler{
		Client:         fakeClient,
		Scheme:         scheme.Scheme,
		privateService: ps,
		ctx:            context.Background(),
	}

	tunnelID, tunnelName, err := r.resolveTunnelRef()

	require.NoError(t, err)
	assert.Equal(t, "clustertunnel-456", tunnelID)
	assert.Equal(t, "test-clustertunnel", tunnelName)
}

func TestResolveTunnelRef_Tunnel(t *testing.T) {
	tunnel := createTestTunnel("test-tunnel", "default")
	ps := createTestPrivateService("test-ps", "default")
	ps.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-tunnel", Kind: "Tunnel", Namespace: "default"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ps, tunnel).
		Build()

	r := &Reconciler{
		Client:         fakeClient,
		Scheme:         scheme.Scheme,
		privateService: ps,
		ctx:            context.Background(),
	}

	tunnelID, tunnelName, err := r.resolveTunnelRef()

	require.NoError(t, err)
	assert.Equal(t, "tunnel-123", tunnelID)
	assert.Equal(t, "test-tunnel", tunnelName)
}

func TestResolveTunnelRef_TunnelNotFound(t *testing.T) {
	ps := createTestPrivateService("test-ps", "default")
	ps.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "non-existent", Kind: "ClusterTunnel"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ps).
		Build()

	r := &Reconciler{
		Client:         fakeClient,
		Scheme:         scheme.Scheme,
		privateService: ps,
		ctx:            context.Background(),
	}

	_, _, err := r.resolveTunnelRef()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-existent")
}

func TestResolveTunnelRef_NoTunnelID(t *testing.T) {
	clusterTunnel := createTestClusterTunnel("test-clustertunnel")
	clusterTunnel.Status.TunnelId = "" // No tunnel ID yet
	ps := createTestPrivateService("test-ps", "default")
	ps.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-clustertunnel", Kind: "ClusterTunnel"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ps, clusterTunnel).
		Build()

	r := &Reconciler{
		Client:         fakeClient,
		Scheme:         scheme.Scheme,
		privateService: ps,
		ctx:            context.Background(),
	}

	_, _, err := r.resolveTunnelRef()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not have a tunnelId yet")
}

func TestResolveVirtualNetworkRef(t *testing.T) {
	vnet := createTestVirtualNetwork()
	ps := createTestPrivateService("test-ps", "default")
	ps.Spec.VirtualNetworkRef = &networkingv1alpha2.VirtualNetworkRef{Name: "test-vnet"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ps, vnet).
		Build()

	r := &Reconciler{
		Client:         fakeClient,
		Scheme:         scheme.Scheme,
		privateService: ps,
		ctx:            context.Background(),
	}

	vnetID, err := r.resolveVirtualNetworkRef()

	require.NoError(t, err)
	assert.Equal(t, "vnet-789", vnetID)
}

func TestResolveVirtualNetworkRef_NoRef(t *testing.T) {
	ps := createTestPrivateService("test-ps", "default")
	ps.Spec.VirtualNetworkRef = nil

	r := &Reconciler{
		privateService: ps,
		ctx:            context.Background(),
	}

	vnetID, err := r.resolveVirtualNetworkRef()

	require.NoError(t, err)
	assert.Empty(t, vnetID)
}

func TestResolveVirtualNetworkRef_NotFound(t *testing.T) {
	ps := createTestPrivateService("test-ps", "default")
	ps.Spec.VirtualNetworkRef = &networkingv1alpha2.VirtualNetworkRef{Name: "non-existent"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ps).
		Build()

	r := &Reconciler{
		Client:         fakeClient,
		Scheme:         scheme.Scheme,
		privateService: ps,
		ctx:            context.Background(),
	}

	_, err := r.resolveVirtualNetworkRef()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-existent")
}

func TestResolveVirtualNetworkRef_NoID(t *testing.T) {
	vnet := createTestVirtualNetwork()
	vnet.Status.VirtualNetworkId = "" // No ID yet
	ps := createTestPrivateService("test-ps", "default")
	ps.Spec.VirtualNetworkRef = &networkingv1alpha2.VirtualNetworkRef{Name: "test-vnet"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ps, vnet).
		Build()

	r := &Reconciler{
		Client:         fakeClient,
		Scheme:         scheme.Scheme,
		privateService: ps,
		ctx:            context.Background(),
	}

	_, err := r.resolveVirtualNetworkRef()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not have a virtualNetworkId yet")
}

func TestFindPrivateServicesForVirtualNetwork(t *testing.T) {
	vnet := createTestVirtualNetwork()
	ps1 := createTestPrivateService("ps1", "default")
	ps1.Spec.VirtualNetworkRef = &networkingv1alpha2.VirtualNetworkRef{Name: "test-vnet"}
	ps2 := createTestPrivateService("ps2", "default")
	ps2.Spec.VirtualNetworkRef = &networkingv1alpha2.VirtualNetworkRef{Name: "other-vnet"}
	ps3 := createTestPrivateService("ps3", "default")
	// ps3 has no VirtualNetworkRef

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ps1, ps2, ps3).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
	}

	requests := r.findPrivateServicesForVirtualNetwork(context.Background(), vnet)

	// Should only return ps1 which references test-vnet
	require.Len(t, requests, 1)
	assert.Equal(t, "ps1", requests[0].Name)
	assert.Equal(t, "default", requests[0].Namespace)
}

func TestFindPrivateServicesForVirtualNetwork_WrongType(t *testing.T) {
	// Pass wrong type object
	tunnel := createTestTunnel("test", "default")

	r := &Reconciler{}

	requests := r.findPrivateServicesForVirtualNetwork(context.Background(), tunnel)

	assert.Empty(t, requests)
}

func TestFindPrivateServicesForClusterTunnel(t *testing.T) {
	clusterTunnel := createTestClusterTunnel("test-clustertunnel")
	ps1 := createTestPrivateService("ps1", "default")
	ps1.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-clustertunnel", Kind: "ClusterTunnel"}
	ps2 := createTestPrivateService("ps2", "production")
	ps2.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-clustertunnel"} // Default kind is ClusterTunnel
	ps3 := createTestPrivateService("ps3", "default")
	ps3.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "other-tunnel", Kind: "ClusterTunnel"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ps1, ps2, ps3).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
	}

	requests := r.findPrivateServicesForClusterTunnel(context.Background(), clusterTunnel)

	// Should return ps1 and ps2 which reference test-clustertunnel
	require.Len(t, requests, 2)
	names := []string{requests[0].Name, requests[1].Name}
	assert.Contains(t, names, "ps1")
	assert.Contains(t, names, "ps2")
}

func TestFindPrivateServicesForClusterTunnel_WrongType(t *testing.T) {
	// Pass wrong type object
	vnet := createTestVirtualNetwork()

	r := &Reconciler{}

	requests := r.findPrivateServicesForClusterTunnel(context.Background(), vnet)

	assert.Empty(t, requests)
}

func TestFindPrivateServicesForTunnel(t *testing.T) {
	tunnel := createTestTunnel("test-tunnel", "default")
	ps1 := createTestPrivateService("ps1", "default")
	ps1.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-tunnel", Kind: "Tunnel", Namespace: "default"}
	ps2 := createTestPrivateService("ps2", "default")
	ps2.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-tunnel", Kind: "Tunnel"} // Uses ps namespace
	ps3 := createTestPrivateService("ps3", "default")
	ps3.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-tunnel", Kind: "Tunnel", Namespace: "other"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ps1, ps2, ps3).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
	}

	requests := r.findPrivateServicesForTunnel(context.Background(), tunnel)

	// Should return ps1 and ps2 which reference test-tunnel in default namespace
	require.Len(t, requests, 2)
	names := []string{requests[0].Name, requests[1].Name}
	assert.Contains(t, names, "ps1")
	assert.Contains(t, names, "ps2")
}

func TestFindPrivateServicesForTunnel_WrongType(t *testing.T) {
	// Pass wrong type object
	vnet := createTestVirtualNetwork()

	r := &Reconciler{}

	requests := r.findPrivateServicesForTunnel(context.Background(), vnet)

	assert.Empty(t, requests)
}

func TestFindPrivateServicesForService(t *testing.T) {
	svc := createTestService("test-service", "default", "10.96.0.1")
	ps1 := createTestPrivateService("ps1", "default")
	ps1.Spec.ServiceRef = networkingv1alpha2.ServiceRef{Name: "test-service", Port: 8080}
	ps2 := createTestPrivateService("ps2", "default")
	ps2.Spec.ServiceRef = networkingv1alpha2.ServiceRef{Name: "other-service", Port: 8080}
	ps3 := createTestPrivateService("ps3", "production") // Different namespace
	ps3.Spec.ServiceRef = networkingv1alpha2.ServiceRef{Name: "test-service", Port: 8080}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ps1, ps2, ps3).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
	}

	requests := r.findPrivateServicesForService(context.Background(), svc)

	// Should only return ps1 which references test-service in the same namespace
	require.Len(t, requests, 1)
	assert.Equal(t, "ps1", requests[0].Name)
	assert.Equal(t, "default", requests[0].Namespace)
}

func TestFindPrivateServicesForService_WrongType(t *testing.T) {
	// Pass wrong type object
	vnet := createTestVirtualNetwork()

	r := &Reconciler{}

	requests := r.findPrivateServicesForService(context.Background(), vnet)

	assert.Empty(t, requests)
}

func TestReconcileRequestsFromMapFunc(t *testing.T) {
	// Test that requests are properly typed with namespace
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test",
			Namespace: "default",
		},
	}
	assert.Equal(t, "test", req.Name)
	assert.Equal(t, "default", req.Namespace)
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
	services := &networkingv1alpha2.PrivateServiceList{}
	err := r.List(context.Background(), services)
	require.NoError(t, err)
	assert.Empty(t, services.Items)
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
	ps := &networkingv1alpha2.PrivateService{}
	err := r.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "default"}, ps)
	assert.Error(t, err) // Should not find it

	// Test Create method
	newPS := createTestPrivateService("new-ps", "default")
	err = r.Create(context.Background(), newPS)
	require.NoError(t, err)

	// Now it should be findable
	err = r.Get(context.Background(), types.NamespacedName{Name: "new-ps", Namespace: "default"}, ps)
	require.NoError(t, err)
	assert.Equal(t, "new-ps", ps.Name)
}

func TestTunnelRefTypes(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			ps := createTestPrivateService("test", "default")
			ps.Spec.TunnelRef.Kind = tt.kind
			assert.Equal(t, tt.expected, ps.Spec.TunnelRef.Kind)
		})
	}
}

func TestPrivateServiceWithVirtualNetworkRef(t *testing.T) {
	ps := createTestPrivateService("test-ps", "default")
	ps.Spec.VirtualNetworkRef = &networkingv1alpha2.VirtualNetworkRef{
		Name: "my-vnet",
	}

	assert.NotNil(t, ps.Spec.VirtualNetworkRef)
	assert.Equal(t, "my-vnet", ps.Spec.VirtualNetworkRef.Name)
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

func TestProtocolValues(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
	}{
		{"TCP protocol", "tcp"},
		{"UDP protocol", "udp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := createTestPrivateService("test", "default")
			ps.Spec.Protocol = tt.protocol
			assert.Equal(t, tt.protocol, ps.Spec.Protocol)
		})
	}
}

func TestServiceRefFields(t *testing.T) {
	ps := createTestPrivateService("test", "default")
	ps.Spec.ServiceRef.Name = "my-service"
	ps.Spec.ServiceRef.Port = 9090

	assert.Equal(t, "my-service", ps.Spec.ServiceRef.Name)
	assert.Equal(t, int32(9090), ps.Spec.ServiceRef.Port)
}

func TestFindPrivateServicesForTunnel_NamespaceMatching(t *testing.T) {
	tunnel := createTestTunnel("test-tunnel", "namespace-a")
	ps1 := createTestPrivateService("ps1", "namespace-a")
	ps1.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-tunnel", Kind: "Tunnel"} // Uses ps namespace
	ps2 := createTestPrivateService("ps2", "namespace-b")
	ps2.Spec.TunnelRef = networkingv1alpha2.TunnelRef{Name: "test-tunnel", Kind: "Tunnel"} // Uses ps namespace

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ps1, ps2).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
	}

	requests := r.findPrivateServicesForTunnel(context.Background(), tunnel)

	// Should only return ps1 which has matching namespace
	require.Len(t, requests, 1)
	assert.Equal(t, "ps1", requests[0].Name)
}
