// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesdomain

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

func setupTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, networkingv1alpha2.AddToScheme(scheme))
	return scheme
}

func createTestPagesDomain(name string) *networkingv1alpha2.PagesDomain {
	return &networkingv1alpha2.PagesDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: networkingv1alpha2.PagesDomainSpec{
			Domain: "app.example.com",
			ProjectRef: networkingv1alpha2.PagesProjectRef{
				Name: "my-project",
			},
			Cloudflare: networkingv1alpha2.CloudflareDetails{
				AccountId: "test-account-123",
				CredentialsRef: &networkingv1alpha2.CloudflareCredentialsRef{
					Name: "cloudflare-creds",
				},
			},
		},
	}
}

func TestFinalizerNameConstant(t *testing.T) {
	assert.Equal(t, "pagesdomain.networking.cloudflare-operator.io/finalizer", FinalizerName)
}

func TestReconcilerFields(t *testing.T) {
	r := &PagesDomainReconciler{}

	assert.Nil(t, r.Client)
	assert.Nil(t, r.Scheme)
	assert.Nil(t, r.Recorder)
	assert.Nil(t, r.domainService)
}

func TestReconciler_Reconcile_NotFound(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &PagesDomainReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result, err := r.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "nonexistent",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconciler_Reconcile_HandlesDeletion(t *testing.T) {
	scheme := setupTestScheme(t)

	domain := createTestPagesDomain("test-domain")
	domain.Finalizers = []string{FinalizerName}

	assert.True(t, controllerutil.ContainsFinalizer(domain, FinalizerName))
	assert.Equal(t, 1, len(domain.Finalizers))

	// Test handleDeletion with domain that has no finalizer
	domainNoFinalizer := createTestPagesDomain("test-domain-2")
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(domainNoFinalizer).
		Build()

	recorder := record.NewFakeRecorder(10)
	r := &PagesDomainReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: recorder,
	}

	now := metav1.Now()
	domainNoFinalizer.DeletionTimestamp = &now

	result, err := r.handleDeletion(ctx, domainNoFinalizer, "my-project")

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestPagesDomainSpec(t *testing.T) {
	domain := createTestPagesDomain("my-domain")

	assert.Equal(t, "app.example.com", domain.Spec.Domain)
	assert.Equal(t, "my-project", domain.Spec.ProjectRef.Name)
	assert.Equal(t, "test-account-123", domain.Spec.Cloudflare.AccountId)
}

func TestPagesDomainStatus(t *testing.T) {
	domain := createTestPagesDomain("my-domain")
	domain.Status = networkingv1alpha2.PagesDomainStatus{
		State:            networkingv1alpha2.PagesDomainStateActive,
		DomainID:         "domain-123",
		ProjectName:      "my-project",
		AccountID:        "test-account-123",
		Status:           "active",
		ValidationStatus: "verified",
		Message:          "Domain is active",
	}

	assert.Equal(t, networkingv1alpha2.PagesDomainStateActive, domain.Status.State)
	assert.Equal(t, "domain-123", domain.Status.DomainID)
	assert.Equal(t, "my-project", domain.Status.ProjectName)
	assert.Equal(t, "active", domain.Status.Status)
	assert.Equal(t, "verified", domain.Status.ValidationStatus)
}

func TestPagesDomainStates(t *testing.T) {
	tests := []struct {
		name  string
		state networkingv1alpha2.PagesDomainState
	}{
		{"Pending", networkingv1alpha2.PagesDomainStatePending},
		{"Verifying", networkingv1alpha2.PagesDomainStateVerifying},
		{"Active", networkingv1alpha2.PagesDomainStateActive},
		{"Deleting", networkingv1alpha2.PagesDomainStateDeleting},
		{"Error", networkingv1alpha2.PagesDomainStateError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domain := createTestPagesDomain("test")
			domain.Status.State = tt.state
			assert.Equal(t, tt.state, domain.Status.State)
		})
	}
}

func TestReconcilerImplementsReconciler(_ *testing.T) {
	var _ reconcile.Reconciler = &PagesDomainReconciler{}
}

func TestReconcilerWithFakeClient(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &PagesDomainReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	domainList := &networkingv1alpha2.PagesDomainList{}
	err := r.List(ctx, domainList)
	require.NoError(t, err)
	assert.Empty(t, domainList.Items)
}

func TestFinalizerOperations(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	domain := createTestPagesDomain("test-domain")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(domain).
		Build()

	// Add finalizer
	controllerutil.AddFinalizer(domain, FinalizerName)
	err := fakeClient.Update(ctx, domain)
	require.NoError(t, err)

	// Verify finalizer was added
	var updated networkingv1alpha2.PagesDomain
	err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-domain", Namespace: "default"}, &updated)
	require.NoError(t, err)
	assert.True(t, controllerutil.ContainsFinalizer(&updated, FinalizerName))

	// Remove finalizer
	controllerutil.RemoveFinalizer(&updated, FinalizerName)
	err = fakeClient.Update(ctx, &updated)
	require.NoError(t, err)

	// Verify finalizer was removed
	err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-domain", Namespace: "default"}, &updated)
	require.NoError(t, err)
	assert.False(t, controllerutil.ContainsFinalizer(&updated, FinalizerName))
}

func TestHandleDeletion_NoFinalizer(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	domain := createTestPagesDomain("test-domain")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(domain).
		Build()

	recorder := record.NewFakeRecorder(10)
	r := &PagesDomainReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: recorder,
	}

	now := metav1.Now()
	domain.DeletionTimestamp = &now

	result, err := r.handleDeletion(ctx, domain, "my-project")

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestPagesProjectRef(t *testing.T) {
	domain := createTestPagesDomain("my-domain")

	// Test with Name reference
	assert.Equal(t, "my-project", domain.Spec.ProjectRef.Name)
	assert.Empty(t, domain.Spec.ProjectRef.CloudflareID)

	// Test with CloudflareID reference
	domain2 := createTestPagesDomain("my-domain-2")
	domain2.Spec.ProjectRef = networkingv1alpha2.PagesProjectRef{
		CloudflareID: "cf-project-id",
	}
	assert.Empty(t, domain2.Spec.ProjectRef.Name)
	assert.Equal(t, "cf-project-id", domain2.Spec.ProjectRef.CloudflareID)
}

func TestPagesDomainWithDeletionPolicy(t *testing.T) {
	domain := createTestPagesDomain("my-domain")
	domain.Spec.DeletionPolicy = "Orphan"

	assert.Equal(t, "Orphan", domain.Spec.DeletionPolicy)

	domain2 := createTestPagesDomain("my-domain-2")
	domain2.Spec.DeletionPolicy = "Delete"

	assert.Equal(t, "Delete", domain2.Spec.DeletionPolicy)
}
