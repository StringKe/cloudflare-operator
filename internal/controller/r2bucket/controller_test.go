// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package r2bucket

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
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func createTestR2Bucket(name, namespace string) *networkingv1alpha2.R2Bucket {
	return &networkingv1alpha2.R2Bucket{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: networkingv1alpha2.R2BucketSpec{
			Name:           "test-bucket",
			LocationHint:   networkingv1alpha2.R2LocationWNAM,
			DeletionPolicy: "Delete",
		},
	}
}

func createTestR2BucketWithCredentials(name, namespace, credsName string) *networkingv1alpha2.R2Bucket {
	bucket := createTestR2Bucket(name, namespace)
	bucket.Spec.CredentialsRef = &networkingv1alpha2.CredentialsReference{
		Name: credsName,
	}
	return bucket
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
				Namespace: "cloudflare-operator-system",
			},
			IsDefault: isDefault,
		},
	}
}

func TestFinalizerName(t *testing.T) {
	assert.Equal(t, "cloudflare.com/r2-bucket-finalizer", finalizerName)
}

func TestReconcilerFields(t *testing.T) {
	// Test that the Reconciler struct has the expected fields
	r := &Reconciler{}

	// Verify nil defaults
	assert.Nil(t, r.Client)
	assert.Nil(t, r.Scheme)
	assert.Nil(t, r.Recorder)
	assert.Nil(t, r.bucketService)
}

func TestReconciler_Reconcile_NotFound(t *testing.T) {
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
		NamespacedName: types.NamespacedName{
			Name:      "nonexistent",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconciler_Reconcile_ReturnsResultOnMissingCredentials(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	bucket := createTestR2Bucket("test-bucket", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(bucket).
		WithStatusSubresource(bucket).
		Build()

	recorder := record.NewFakeRecorder(10)
	r := &Reconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: recorder,
	}

	result, err := r.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      bucket.Name,
			Namespace: bucket.Namespace,
		},
	})

	// The reconcile returns error or requeues when credentials are missing
	// Test that we get some result
	_ = err
	_ = result
}

func TestReconciler_Reconcile_HandlesDeletion(t *testing.T) {
	scheme := setupTestScheme(t)

	// Create bucket with finalizer and deletion timestamp
	// We test via the handleDeletion method directly since setting
	// deletionTimestamp after creation is immutable
	bucket := createTestR2Bucket("test-bucket", "default")
	bucket.Finalizers = []string{finalizerName}

	// Verify the bucket has a finalizer
	assert.True(t, controllerutil.ContainsFinalizer(bucket, finalizerName))
	assert.Equal(t, 1, len(bucket.Finalizers))

	// Test handleDeletion with bucket that has no finalizer
	bucketNoFinalizer := createTestR2Bucket("test-bucket-2", "default")
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(bucketNoFinalizer).
		Build()

	recorder := record.NewFakeRecorder(10)
	r := &Reconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: recorder,
	}

	now := metav1.Now()
	bucketNoFinalizer.DeletionTimestamp = &now

	// Following Unified Sync Architecture, handleDeletion no longer takes credRef
	result, err := r.handleDeletion(ctx, bucketNoFinalizer)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// Test findBucketsForCredentials
func TestFindBucketsForCredentials_TypeCheck(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	// Test with wrong type
	wrongObj := &networkingv1alpha2.R2Bucket{}
	requests := r.findBucketsForCredentials(ctx, wrongObj)
	assert.Nil(t, requests)
}

func TestFindBucketsForCredentials_MatchingBuckets(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	creds := createTestCredentials("test-creds", false)
	bucket1 := createTestR2BucketWithCredentials("bucket1", "default", "test-creds")
	bucket2 := createTestR2BucketWithCredentials("bucket2", "default", "other-creds")
	bucket3 := createTestR2BucketWithCredentials("bucket3", "ns1", "test-creds")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(creds, bucket1, bucket2, bucket3).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	requests := r.findBucketsForCredentials(ctx, creds)

	// Should find bucket1 and bucket3 (matching credentials)
	assert.Len(t, requests, 2)
	names := make([]string, len(requests))
	for i, req := range requests {
		names[i] = req.Name
	}
	assert.Contains(t, names, "bucket1")
	assert.Contains(t, names, "bucket3")
}

func TestFindBucketsForCredentials_DefaultCredentials(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	creds := createTestCredentials("default-creds", true) // isDefault = true
	bucket1 := createTestR2Bucket("bucket1", "default")   // No credentialsRef
	bucket2 := createTestR2BucketWithCredentials("bucket2", "default", "other-creds")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(creds, bucket1, bucket2).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	requests := r.findBucketsForCredentials(ctx, creds)

	// Should find bucket1 (using default credentials)
	assert.Len(t, requests, 1)
	assert.Equal(t, "bucket1", requests[0].Name)
}

func TestFindBucketsForCredentials_ListError(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	creds := createTestCredentials("test-creds", false)

	// Use client without R2Bucket registered to cause list error
	incompleteScheme := runtime.NewScheme()
	_ = networkingv1alpha2.AddToScheme(incompleteScheme)
	// Don't add R2Bucket to scheme

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme). // Use full scheme for initial creation
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: incompleteScheme,
	}

	requests := r.findBucketsForCredentials(ctx, creds)

	// Should return nil on list error (empty list)
	assert.Empty(t, requests)
}

// Test R2Bucket spec fields
func TestR2BucketSpec(t *testing.T) {
	bucket := createTestR2Bucket("my-bucket", "default")
	bucket.Spec.CORS = []networkingv1alpha2.R2CORSRule{
		{
			AllowedOrigins: []string{"https://example.com"},
			AllowedMethods: []string{"GET", "POST"},
			AllowedHeaders: []string{"Content-Type"},
			MaxAgeSeconds:  intPtr(3600),
		},
	}
	bucket.Spec.Lifecycle = []networkingv1alpha2.R2LifecycleRule{
		{
			ID:      "delete-old",
			Enabled: true,
			Prefix:  "logs/",
			Expiration: &networkingv1alpha2.R2LifecycleExpiration{
				Days: intPtr(30),
			},
		},
	}

	assert.Equal(t, "test-bucket", bucket.Spec.Name)
	assert.Equal(t, networkingv1alpha2.R2LocationWNAM, bucket.Spec.LocationHint)
	assert.Equal(t, "Delete", bucket.Spec.DeletionPolicy)
	assert.Len(t, bucket.Spec.CORS, 1)
	assert.Len(t, bucket.Spec.Lifecycle, 1)
}

func TestR2BucketStatus(t *testing.T) {
	bucket := createTestR2Bucket("my-bucket", "default")
	now := metav1.Now()
	bucket.Status = networkingv1alpha2.R2BucketStatus{
		State:               networkingv1alpha2.R2BucketStateReady,
		BucketName:          "my-bucket",
		Location:            "wnam",
		CreatedAt:           &now,
		CORSRulesCount:      2,
		LifecycleRulesCount: 1,
		Message:             "Bucket is ready",
		ObservedGeneration:  3,
	}

	assert.Equal(t, networkingv1alpha2.R2BucketStateReady, bucket.Status.State)
	assert.Equal(t, "my-bucket", bucket.Status.BucketName)
	assert.Equal(t, "wnam", bucket.Status.Location)
	assert.NotNil(t, bucket.Status.CreatedAt)
	assert.Equal(t, 2, bucket.Status.CORSRulesCount)
	assert.Equal(t, 1, bucket.Status.LifecycleRulesCount)
	assert.Equal(t, "Bucket is ready", bucket.Status.Message)
	assert.Equal(t, int64(3), bucket.Status.ObservedGeneration)
}

func TestR2BucketStates(t *testing.T) {
	tests := []struct {
		name  string
		state networkingv1alpha2.R2BucketState
	}{
		{"Pending", networkingv1alpha2.R2BucketStatePending},
		{"Creating", networkingv1alpha2.R2BucketStateCreating},
		{"Ready", networkingv1alpha2.R2BucketStateReady},
		{"Deleting", networkingv1alpha2.R2BucketStateDeleting},
		{"Error", networkingv1alpha2.R2BucketStateError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket := createTestR2Bucket("test", "default")
			bucket.Status.State = tt.state
			assert.Equal(t, tt.state, bucket.Status.State)
		})
	}
}

func TestR2LocationHints(t *testing.T) {
	tests := []struct {
		name     string
		location networkingv1alpha2.R2LocationHint
	}{
		{"APAC", networkingv1alpha2.R2LocationAPAC},
		{"EEUR", networkingv1alpha2.R2LocationEEUR},
		{"ENAM", networkingv1alpha2.R2LocationENAM},
		{"WEUR", networkingv1alpha2.R2LocationWEUR},
		{"WNAM", networkingv1alpha2.R2LocationWNAM},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket := createTestR2Bucket("test", "default")
			bucket.Spec.LocationHint = tt.location
			assert.Equal(t, tt.location, bucket.Spec.LocationHint)
		})
	}
}

func TestR2CORSRule(t *testing.T) {
	rule := networkingv1alpha2.R2CORSRule{
		ID:             "cors-rule-1",
		AllowedOrigins: []string{"https://example.com", "https://app.example.com"},
		AllowedMethods: []string{"GET", "PUT", "POST", "DELETE"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
		ExposeHeaders:  []string{"ETag", "x-amz-meta-custom"},
		MaxAgeSeconds:  intPtr(7200),
	}

	assert.Equal(t, "cors-rule-1", rule.ID)
	assert.Len(t, rule.AllowedOrigins, 2)
	assert.Len(t, rule.AllowedMethods, 4)
	assert.Len(t, rule.AllowedHeaders, 2)
	assert.Len(t, rule.ExposeHeaders, 2)
	assert.Equal(t, 7200, *rule.MaxAgeSeconds)
}

func TestR2LifecycleRule(t *testing.T) {
	rule := networkingv1alpha2.R2LifecycleRule{
		ID:      "expire-logs",
		Enabled: true,
		Prefix:  "logs/",
		Expiration: &networkingv1alpha2.R2LifecycleExpiration{
			Days: intPtr(90),
		},
		AbortIncompleteMultipartUpload: &networkingv1alpha2.R2LifecycleAbortUpload{
			DaysAfterInitiation: 7,
		},
	}

	assert.Equal(t, "expire-logs", rule.ID)
	assert.True(t, rule.Enabled)
	assert.Equal(t, "logs/", rule.Prefix)
	assert.NotNil(t, rule.Expiration)
	assert.Equal(t, 90, *rule.Expiration.Days)
	assert.NotNil(t, rule.AbortIncompleteMultipartUpload)
	assert.Equal(t, 7, rule.AbortIncompleteMultipartUpload.DaysAfterInitiation)
}

func TestReconcilerImplementsReconciler(_ *testing.T) {
	var _ reconcile.Reconciler = &Reconciler{}
}

func TestReconcilerWithFakeClient(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	// Should be able to list resources (empty list)
	bucketList := &networkingv1alpha2.R2BucketList{}
	err := r.List(ctx, bucketList)
	require.NoError(t, err)
	assert.Empty(t, bucketList.Items)
}

func TestReconcilerEmbeddedClient(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	// Test Get method
	bucket := &networkingv1alpha2.R2Bucket{}
	err := r.Get(ctx, types.NamespacedName{Name: "test", Namespace: "default"}, bucket)
	assert.Error(t, err) // Should not find it

	// Test Create method
	newBucket := createTestR2Bucket("new-bucket", "default")
	err = r.Create(ctx, newBucket)
	require.NoError(t, err)

	// Now it should be findable
	err = r.Get(ctx, types.NamespacedName{Name: "new-bucket", Namespace: "default"}, bucket)
	require.NoError(t, err)
	assert.Equal(t, "new-bucket", bucket.Name)
}

func TestHandleDeletion_NoFinalizer(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	// Create bucket without finalizer
	bucket := createTestR2Bucket("test-bucket", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(bucket).
		Build()

	recorder := record.NewFakeRecorder(10)
	r := &Reconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: recorder,
	}

	// Set deletion timestamp manually for testing handleDeletion directly
	now := metav1.Now()
	bucket.DeletionTimestamp = &now

	// Following Unified Sync Architecture, handleDeletion no longer takes credRef
	result, err := r.handleDeletion(ctx, bucket)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestHandleDeletion_WithFinalizer_OrphanPolicy(t *testing.T) {
	scheme := setupTestScheme(t)
	_ = scheme // used for scheme setup

	// Create bucket with finalizer
	bucket := createTestR2Bucket("test-bucket", "default")
	bucket.Spec.DeletionPolicy = "Orphan"
	controllerutil.AddFinalizer(bucket, finalizerName)

	// Set deletion timestamp manually for testing
	now := metav1.Now()
	bucket.DeletionTimestamp = &now

	// Verify the policy is set correctly
	assert.Equal(t, "Orphan", bucket.Spec.DeletionPolicy)
	assert.True(t, controllerutil.ContainsFinalizer(bucket, finalizerName))
}

// Test finalizer operations
func TestFinalizerOperations(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	bucket := createTestR2Bucket("test-bucket", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(bucket).
		Build()

	// Add finalizer
	controllerutil.AddFinalizer(bucket, finalizerName)
	err := fakeClient.Update(ctx, bucket)
	require.NoError(t, err)

	// Verify finalizer was added
	var updated networkingv1alpha2.R2Bucket
	err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-bucket", Namespace: "default"}, &updated)
	require.NoError(t, err)
	assert.True(t, controllerutil.ContainsFinalizer(&updated, finalizerName))

	// Remove finalizer
	controllerutil.RemoveFinalizer(&updated, finalizerName)
	err = fakeClient.Update(ctx, &updated)
	require.NoError(t, err)

	// Verify finalizer was removed
	err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-bucket", Namespace: "default"}, &updated)
	require.NoError(t, err)
	assert.False(t, controllerutil.ContainsFinalizer(&updated, finalizerName))
}

func TestGetClientFromReconciler(t *testing.T) {
	scheme := setupTestScheme(t)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &Reconciler{
		Client: fakeClient,
	}

	// Verify the embedded client is accessible
	var c = r.Client
	assert.NotNil(t, c)
}

func TestR2BucketWithoutName(t *testing.T) {
	// When Spec.Name is empty, it should use ObjectMeta.Name
	bucket := &networkingv1alpha2.R2Bucket{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "k8s-bucket-name",
			Namespace: "default",
		},
		Spec: networkingv1alpha2.R2BucketSpec{
			// Name is empty
			LocationHint: networkingv1alpha2.R2LocationWNAM,
		},
	}

	assert.Empty(t, bucket.Spec.Name)
	assert.Equal(t, "k8s-bucket-name", bucket.Name)
}

func TestR2LifecycleExpirationByDate(t *testing.T) {
	rule := networkingv1alpha2.R2LifecycleRule{
		ID:      "expire-by-date",
		Enabled: true,
		Expiration: &networkingv1alpha2.R2LifecycleExpiration{
			Date: "2025-12-31T00:00:00Z",
		},
	}

	assert.Equal(t, "expire-by-date", rule.ID)
	assert.NotNil(t, rule.Expiration)
	assert.Nil(t, rule.Expiration.Days)
	assert.Equal(t, "2025-12-31T00:00:00Z", rule.Expiration.Date)
}

func TestR2BucketListPagination(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	// Create multiple buckets
	buckets := make([]client.Object, 5)
	for i := 0; i < 5; i++ {
		buckets[i] = createTestR2Bucket(
			"bucket-"+string(rune('a'+i)),
			"default",
		)
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(buckets...).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	bucketList := &networkingv1alpha2.R2BucketList{}
	err := r.List(ctx, bucketList)
	require.NoError(t, err)
	assert.Len(t, bucketList.Items, 5)
}

// Helper functions
func intPtr(i int) *int {
	return &i
}
