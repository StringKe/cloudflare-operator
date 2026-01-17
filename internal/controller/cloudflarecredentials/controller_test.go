// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cloudflarecredentials

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

func init() {
	_ = networkingv1alpha2.AddToScheme(scheme.Scheme)
	_ = corev1.AddToScheme(scheme.Scheme)
}

func TestFinalizerName(t *testing.T) {
	assert.Equal(t, "cloudflare.com/credentials-finalizer", finalizerName)
}

func TestReconcilerFields(t *testing.T) {
	// Test that the Reconciler struct has the expected fields
	r := &Reconciler{}

	// Verify nil defaults
	assert.Nil(t, r.Client)
	assert.Nil(t, r.Scheme)
	assert.Nil(t, r.Recorder)
	assert.Nil(t, r.creds)
}

func createTestCredentials(name string, isDefault bool) *networkingv1alpha2.CloudflareCredentials {
	return &networkingv1alpha2.CloudflareCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: networkingv1alpha2.CloudflareCredentialsSpec{
			AuthType:  networkingv1alpha2.AuthTypeAPIToken,
			AccountID: "test-account-123",
			SecretRef: networkingv1alpha2.SecretReference{
				Name:      "cloudflare-secret",
				Namespace: "default",
			},
			IsDefault: isDefault,
		},
	}
}

func createTestSecret(name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"CLOUDFLARE_API_TOKEN": []byte("test-token"),
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
			Name: "non-existent",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconciler_Reconcile_AddsFinalizer(t *testing.T) {
	creds := createTestCredentials("test-creds", false)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(creds).
		WithStatusSubresource(creds).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := &Reconciler{
		Client:   fakeClient,
		Scheme:   scheme.Scheme,
		Recorder: recorder,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: creds.Name,
		},
	}

	// First reconcile - add finalizer
	_, _ = r.Reconcile(context.Background(), req)

	// Verify finalizer was added
	var updated networkingv1alpha2.CloudflareCredentials
	err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-creds"}, &updated)
	require.NoError(t, err)
	assert.True(t, controllerutil.ContainsFinalizer(&updated, finalizerName))
}

func TestReconciler_HandleDeletion_NoFinalizer(t *testing.T) {
	creds := createTestCredentials("test-creds", false)
	// Not adding finalizer

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(creds).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
		creds:  creds,
		ctx:    context.Background(),
	}

	result, err := r.handleDeletion()

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconciler_HandleDeletion_WithFinalizer(t *testing.T) {
	creds := createTestCredentials("test-creds", false)
	controllerutil.AddFinalizer(creds, finalizerName)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(creds).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
		creds:  creds,
		ctx:    context.Background(),
	}

	result, err := r.handleDeletion()

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify finalizer was removed
	var updated networkingv1alpha2.CloudflareCredentials
	getErr := fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-creds"}, &updated)
	require.NoError(t, getErr)
	assert.False(t, controllerutil.ContainsFinalizer(&updated, finalizerName))
}

func TestReconciler_EnsureSingleDefault_NoOther(t *testing.T) {
	creds := createTestCredentials("test-creds", true)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(creds).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
		creds:  creds,
		ctx:    context.Background(),
	}

	err := r.ensureSingleDefault()

	require.NoError(t, err)
}

func TestReconciler_EnsureSingleDefault_WithOther(t *testing.T) {
	creds := createTestCredentials("test-creds", true)
	otherCreds := createTestCredentials("other-creds", true)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(creds, otherCreds).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
		creds:  creds,
		ctx:    context.Background(),
	}

	err := r.ensureSingleDefault()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already marked as default")
}

func TestReconciler_UpdateStatus_Validated(t *testing.T) {
	creds := createTestCredentials("test-creds", false)
	creds.Generation = 3

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(creds).
		WithStatusSubresource(creds).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
		creds:  creds,
		ctx:    context.Background(),
		log:    ctrl.Log.WithName("test"),
	}

	r.updateStatus("Ready", true, "Credentials validated successfully")

	assert.Equal(t, "Ready", creds.Status.State)
	assert.True(t, creds.Status.Validated)
	assert.Equal(t, int64(3), creds.Status.ObservedGeneration)
	assert.NotNil(t, creds.Status.LastValidatedTime)
	require.Len(t, creds.Status.Conditions, 1)
	assert.Equal(t, "Ready", creds.Status.Conditions[0].Type)
	assert.Equal(t, metav1.ConditionTrue, creds.Status.Conditions[0].Status)
	assert.Equal(t, "Validated", creds.Status.Conditions[0].Reason)
}

func TestReconciler_UpdateStatus_Failed(t *testing.T) {
	creds := createTestCredentials("test-creds", false)
	creds.Generation = 2

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(creds).
		WithStatusSubresource(creds).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
		creds:  creds,
		ctx:    context.Background(),
		log:    ctrl.Log.WithName("test"),
	}

	r.updateStatus("Error", false, "Failed to validate")

	assert.Equal(t, "Error", creds.Status.State)
	assert.False(t, creds.Status.Validated)
	assert.Equal(t, int64(2), creds.Status.ObservedGeneration)
	assert.Nil(t, creds.Status.LastValidatedTime)
	require.Len(t, creds.Status.Conditions, 1)
	assert.Equal(t, "Ready", creds.Status.Conditions[0].Type)
	assert.Equal(t, metav1.ConditionFalse, creds.Status.Conditions[0].Status)
	assert.Equal(t, "ValidationFailed", creds.Status.Conditions[0].Reason)
}

func TestReconciler_UpdateStatus_OverwriteCondition(t *testing.T) {
	creds := createTestCredentials("test-creds", false)
	creds.Status.Conditions = []metav1.Condition{
		{
			Type:   "Ready",
			Status: metav1.ConditionFalse,
			Reason: "OldReason",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(creds).
		WithStatusSubresource(creds).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
		creds:  creds,
		ctx:    context.Background(),
		log:    ctrl.Log.WithName("test"),
	}

	r.updateStatus("Ready", true, "Success")

	require.Len(t, creds.Status.Conditions, 1)
	assert.Equal(t, metav1.ConditionTrue, creds.Status.Conditions[0].Status)
	assert.Equal(t, "Validated", creds.Status.Conditions[0].Reason)
}

func TestCloudflareCredentialsSpec(t *testing.T) {
	creds := createTestCredentials("my-creds", true)

	assert.Equal(t, networkingv1alpha2.AuthTypeAPIToken, creds.Spec.AuthType)
	assert.Equal(t, "test-account-123", creds.Spec.AccountID)
	assert.Equal(t, "cloudflare-secret", creds.Spec.SecretRef.Name)
	assert.Equal(t, "default", creds.Spec.SecretRef.Namespace)
	assert.True(t, creds.Spec.IsDefault)
}

func TestCloudflareCredentialsStatus(t *testing.T) {
	creds := createTestCredentials("test-creds", false)
	now := metav1.Now()
	creds.Status.State = "Ready"
	creds.Status.Validated = true
	creds.Status.AccountName = "Test Account"
	creds.Status.ObservedGeneration = 5
	creds.Status.LastValidatedTime = &now

	assert.Equal(t, "Ready", creds.Status.State)
	assert.True(t, creds.Status.Validated)
	assert.Equal(t, "Test Account", creds.Status.AccountName)
	assert.Equal(t, int64(5), creds.Status.ObservedGeneration)
	assert.NotNil(t, creds.Status.LastValidatedTime)
}

func TestAuthTypes(t *testing.T) {
	tests := []struct {
		name     string
		authType networkingv1alpha2.CloudflareAuthType
	}{
		{"APIToken", networkingv1alpha2.AuthTypeAPIToken},
		{"GlobalAPIKey", networkingv1alpha2.AuthTypeGlobalAPIKey},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds := createTestCredentials("test", false)
			creds.Spec.AuthType = tt.authType
			assert.Equal(t, tt.authType, creds.Spec.AuthType)
		})
	}
}

func TestReconcilerImplementsReconciler(t *testing.T) {
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
	credsList := &networkingv1alpha2.CloudflareCredentialsList{}
	err := r.List(context.Background(), credsList)
	require.NoError(t, err)
	assert.Empty(t, credsList.Items)
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
	creds := &networkingv1alpha2.CloudflareCredentials{}
	err := r.Get(context.Background(), types.NamespacedName{Name: "test"}, creds)
	assert.Error(t, err) // Should not find it

	// Test Create method
	newCreds := createTestCredentials("new-creds", false)
	err = r.Create(context.Background(), newCreds)
	require.NoError(t, err)

	// Now it should be findable
	err = r.Get(context.Background(), types.NamespacedName{Name: "new-creds"}, creds)
	require.NoError(t, err)
	assert.Equal(t, "new-creds", creds.Name)
}

func TestValidateCredentials_MissingSecret(t *testing.T) {
	creds := createTestCredentials("test-creds", false)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(creds).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
		creds:  creds,
		ctx:    context.Background(),
	}

	err := r.validateCredentials()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get secret")
}

func TestSecretRefFields(t *testing.T) {
	creds := createTestCredentials("test", false)
	creds.Spec.SecretRef.APITokenKey = "MY_TOKEN"
	creds.Spec.SecretRef.APIKeyKey = "MY_KEY"
	creds.Spec.SecretRef.EmailKey = "MY_EMAIL"

	assert.Equal(t, "MY_TOKEN", creds.Spec.SecretRef.APITokenKey)
	assert.Equal(t, "MY_KEY", creds.Spec.SecretRef.APIKeyKey)
	assert.Equal(t, "MY_EMAIL", creds.Spec.SecretRef.EmailKey)
}

func TestGetClientFromReconciler(t *testing.T) {
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		Build()

	r := &Reconciler{
		Client: fakeClient,
	}

	// Verify the embedded client is accessible
	var c client.Client = r.Client
	assert.NotNil(t, c)
}

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = networkingv1alpha2.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	return s
}
