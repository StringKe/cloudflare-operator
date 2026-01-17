// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package accessgroup

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	accesssvc "github.com/StringKe/cloudflare-operator/internal/service/access"
)

func setupTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, networkingv1alpha2.AddToScheme(scheme))
	return scheme
}

func TestFinalizerName(t *testing.T) {
	assert.Equal(t, "accessgroup.networking.cloudflare-operator.io/finalizer", FinalizerName)
	assert.Contains(t, FinalizerName, "accessgroup")
}

func TestReconcilerFields(t *testing.T) {
	// Test that the Reconciler struct has the expected fields
	r := &Reconciler{}

	// Verify nil defaults
	assert.Nil(t, r.Client)
	assert.Nil(t, r.Scheme)
	assert.Nil(t, r.Recorder)
	assert.Nil(t, r.groupService)
	assert.Nil(t, r.accessGroup)
	// Note: cfAPI field removed - following Unified Sync Architecture
}

func TestReconcileNotFound(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result, err := r.Reconcile(ctx, ctrl.Request{
		NamespacedName: client.ObjectKey{
			Name: "nonexistent",
		},
	})

	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)
}

func TestReconcileWithDeletingAccessGroup(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	now := metav1.Now()
	accessGroup := &networkingv1alpha2.AccessGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-group",
			DeletionTimestamp: &now,
			Finalizers:        []string{FinalizerName},
		},
		Spec: networkingv1alpha2.AccessGroupSpec{
			Name: "Test Group",
			Include: []networkingv1alpha2.AccessGroupRule{
				{
					Everyone: true,
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(accessGroup).
		WithStatusSubresource(accessGroup).
		Build()

	r := &Reconciler{
		Client:       fakeClient,
		Scheme:       scheme,
		Recorder:     record.NewFakeRecorder(10),
		groupService: accesssvc.NewGroupService(fakeClient),
	}

	// Following Unified Sync Architecture, handleDeletion only unregisters from SyncState
	// This should not fail - it just unregisters and removes finalizer
	result, err := r.Reconcile(ctx, ctrl.Request{
		NamespacedName: client.ObjectKey{
			Name: "test-group",
		},
	})

	// Deletion handling should succeed - no API client needed now
	assert.NoError(t, err)
	_ = result
}
