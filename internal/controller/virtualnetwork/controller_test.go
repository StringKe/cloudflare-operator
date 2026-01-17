// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package virtualnetwork

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

func init() {
	_ = networkingv1alpha2.AddToScheme(scheme.Scheme)
}

func createTestVirtualNetwork(name string, isDefault bool) *networkingv1alpha2.VirtualNetwork {
	return &networkingv1alpha2.VirtualNetwork{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: networkingv1alpha2.VirtualNetworkSpec{
			Comment:          "Test virtual network",
			IsDefaultNetwork: isDefault,
			Cloudflare: networkingv1alpha2.CloudflareDetails{
				CredentialsRef: &networkingv1alpha2.CloudflareCredentialsRef{
					Name: "test-credentials",
				},
			},
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
	vnet := createTestVirtualNetwork("test-vnet", false)

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(vnet).
		WithStatusSubresource(vnet).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := &Reconciler{
		Client:   client,
		Scheme:   scheme.Scheme,
		Recorder: recorder,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: vnet.Name,
		},
	}

	// First reconcile - add finalizer (will fail on API client init but finalizer should be added)
	_, _ = r.Reconcile(context.Background(), req)

	// Verify finalizer was added
	var updated networkingv1alpha2.VirtualNetwork
	err := client.Get(context.Background(), types.NamespacedName{Name: "test-vnet"}, &updated)
	require.NoError(t, err)
	assert.True(t, controllerutil.ContainsFinalizer(&updated, controller.VirtualNetworkFinalizer))
}

func TestReconciler_HandleDeletion_NoFinalizer(t *testing.T) {
	vnet := createTestVirtualNetwork("test-vnet", false)

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(vnet).
		Build()
	recorder := record.NewFakeRecorder(10)

	// Simulate deletion state without finalizer (the check happens in handleDeletion)
	r := &Reconciler{
		Client:   client,
		Scheme:   scheme.Scheme,
		Recorder: recorder,
		vnet:     vnet, // No finalizer set
		ctx:      context.Background(),
	}

	result, err := r.handleDeletion()

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconciler_BuildManagedComment(t *testing.T) {
	vnet := createTestVirtualNetwork("test-vnet", false)
	vnet.Spec.Comment = "User comment"

	r := &Reconciler{
		vnet: vnet,
	}

	comment := r.buildManagedComment()

	// Should contain management marker: [managed:Kind/Name]
	assert.Contains(t, comment, "[managed:VirtualNetwork/test-vnet]")
	// Should also contain user comment
	assert.Contains(t, comment, "User comment")
}

func TestReconciler_BuildManagedComment_NoUserComment(t *testing.T) {
	vnet := createTestVirtualNetwork("test-vnet", false)
	vnet.Spec.Comment = ""

	r := &Reconciler{
		vnet: vnet,
	}

	comment := r.buildManagedComment()

	// Should contain management marker: [managed:Kind/Name]
	assert.Contains(t, comment, "[managed:VirtualNetwork/test-vnet]")
}

func TestReconciler_SetCondition(t *testing.T) {
	vnet := createTestVirtualNetwork("test-vnet", false)
	vnet.Generation = 5

	r := &Reconciler{
		vnet: vnet,
	}

	r.setCondition(metav1.ConditionTrue, "TestReason", "Test message")

	require.Len(t, vnet.Status.Conditions, 1)
	cond := vnet.Status.Conditions[0]
	assert.Equal(t, "Ready", cond.Type)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, "TestReason", cond.Reason)
	assert.Equal(t, "Test message", cond.Message)
	assert.Equal(t, int64(5), cond.ObservedGeneration)
}

func TestReconciler_SetCondition_Overwrite(t *testing.T) {
	vnet := createTestVirtualNetwork("test-vnet", false)
	vnet.Status.Conditions = []metav1.Condition{
		{
			Type:   "Ready",
			Status: metav1.ConditionFalse,
			Reason: "OldReason",
		},
	}

	r := &Reconciler{
		vnet: vnet,
	}

	r.setCondition(metav1.ConditionTrue, "NewReason", "New message")

	require.Len(t, vnet.Status.Conditions, 1)
	cond := vnet.Status.Conditions[0]
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, "NewReason", cond.Reason)
}

func TestVirtualNetworkSpec(t *testing.T) {
	vnet := createTestVirtualNetwork("my-vnet", true)

	assert.Equal(t, "Test virtual network", vnet.Spec.Comment)
	assert.True(t, vnet.Spec.IsDefaultNetwork)
	assert.NotNil(t, vnet.Spec.Cloudflare.CredentialsRef)
	assert.Equal(t, "test-credentials", vnet.Spec.Cloudflare.CredentialsRef.Name)
}

func TestVirtualNetworkStatus(t *testing.T) {
	vnet := createTestVirtualNetwork("test-vnet", false)
	vnet.Status.VirtualNetworkId = "vnet-123"
	vnet.Status.AccountId = "account-456"
	vnet.Status.State = "active"

	assert.Equal(t, "vnet-123", vnet.Status.VirtualNetworkId)
	assert.Equal(t, "account-456", vnet.Status.AccountId)
	assert.Equal(t, "active", vnet.Status.State)
}

func TestReconciler_UpdateStatusPending(t *testing.T) {
	vnet := createTestVirtualNetwork("test-vnet", false)
	vnet.Generation = 3

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(vnet).
		WithStatusSubresource(vnet).
		Build()

	r := &Reconciler{
		Client: client,
		Scheme: scheme.Scheme,
		vnet:   vnet,
		ctx:    context.Background(),
	}

	err := r.updateStatusPending()

	require.NoError(t, err)

	// Fetch and verify
	var updated networkingv1alpha2.VirtualNetwork
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-vnet"}, &updated)
	require.NoError(t, err)
	assert.Equal(t, int64(3), updated.Status.ObservedGeneration)
	assert.Equal(t, "pending", updated.Status.State)
}

func TestReconciler_UpdateStatusPending_KeepsActive(t *testing.T) {
	vnet := createTestVirtualNetwork("test-vnet", false)
	vnet.Status.State = "active" // Already active

	client := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(vnet).
		WithStatusSubresource(vnet).
		Build()

	r := &Reconciler{
		Client: client,
		Scheme: scheme.Scheme,
		vnet:   vnet,
		ctx:    context.Background(),
	}

	err := r.updateStatusPending()

	require.NoError(t, err)

	// Fetch and verify - should still be active
	var updated networkingv1alpha2.VirtualNetwork
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-vnet"}, &updated)
	require.NoError(t, err)
	assert.Equal(t, "active", updated.Status.State)
}

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = networkingv1alpha2.AddToScheme(s)
	return s
}
