// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesdeployment

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

func createTestPagesDeployment(name, namespace string) *networkingv1alpha2.PagesDeployment {
	return &networkingv1alpha2.PagesDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: networkingv1alpha2.PagesDeploymentSpec{
			ProjectRef: networkingv1alpha2.PagesProjectRef{
				Name: "my-project",
			},
			Branch: "main",
			Action: networkingv1alpha2.PagesDeploymentActionCreate,
			Cloudflare: networkingv1alpha2.CloudflareDetails{
				AccountId: "test-account-123",
				CredentialsRef: &networkingv1alpha2.CloudflareCredentialsRef{
					Name: "cloudflare-creds",
				},
			},
		},
	}
}

func createTestPagesDeploymentWithRetry(name, namespace, targetID string) *networkingv1alpha2.PagesDeployment {
	deployment := createTestPagesDeployment(name, namespace)
	deployment.Spec.Action = networkingv1alpha2.PagesDeploymentActionRetry
	deployment.Spec.TargetDeploymentID = targetID
	return deployment
}

func createTestPagesDeploymentWithRollback(name, namespace, targetID string) *networkingv1alpha2.PagesDeployment {
	deployment := createTestPagesDeployment(name, namespace)
	deployment.Spec.Action = networkingv1alpha2.PagesDeploymentActionRollback
	deployment.Spec.TargetDeploymentID = targetID
	return deployment
}

func createTestPagesProject(name, namespace string) *networkingv1alpha2.PagesProject {
	return &networkingv1alpha2.PagesProject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: networkingv1alpha2.PagesProjectSpec{
			Name:             name,
			ProductionBranch: "main",
			Cloudflare: networkingv1alpha2.CloudflareDetails{
				AccountId: "test-account-123",
			},
		},
		Status: networkingv1alpha2.PagesProjectStatus{
			State:     networkingv1alpha2.PagesProjectStateReady,
			ProjectID: name,
		},
	}
}

func TestFinalizerName(t *testing.T) {
	assert.Equal(t, "pagesdeployment.networking.cloudflare-operator.io/finalizer", FinalizerName)
}

func TestReconcilerFields(t *testing.T) {
	r := &PagesDeploymentReconciler{}

	assert.Nil(t, r.Client)
	assert.Nil(t, r.Scheme)
	assert.Nil(t, r.Recorder)
	assert.Nil(t, r.deploymentService)
}

func TestReconciler_Reconcile_NotFound(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &PagesDeploymentReconciler{
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

	deployment := createTestPagesDeployment("test-deployment", "default")
	deployment.Finalizers = []string{FinalizerName}

	assert.True(t, controllerutil.ContainsFinalizer(deployment, FinalizerName))
	assert.Equal(t, 1, len(deployment.Finalizers))

	// Test handleDeletion with deployment that has no finalizer
	deploymentNoFinalizer := createTestPagesDeployment("test-deployment-2", "default")
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deploymentNoFinalizer).
		Build()

	recorder := record.NewFakeRecorder(10)
	r := &PagesDeploymentReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: recorder,
	}

	now := metav1.Now()
	deploymentNoFinalizer.DeletionTimestamp = &now

	result, err := r.handleDeletion(ctx, deploymentNoFinalizer, "my-project")

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestFindDeploymentsForProject_TypeCheck(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &PagesDeploymentReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	// Test with wrong type
	wrongObj := &networkingv1alpha2.PagesDeployment{}
	requests := r.findDeploymentsForProject(ctx, wrongObj)
	assert.Nil(t, requests)
}

func TestFindDeploymentsForProject_MatchingDeployments(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	project := createTestPagesProject("my-project", "default")
	deployment1 := createTestPagesDeployment("deployment1", "default")
	deployment2 := createTestPagesDeployment("deployment2", "default")
	deployment2.Spec.ProjectRef.Name = "other-project"
	deployment3 := createTestPagesDeployment("deployment3", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(project, deployment1, deployment2, deployment3).
		Build()

	r := &PagesDeploymentReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	requests := r.findDeploymentsForProject(ctx, project)

	// Should find deployment1 and deployment3 (referencing my-project)
	assert.Len(t, requests, 2)
	names := make([]string, len(requests))
	for i, req := range requests {
		names[i] = req.Name
	}
	assert.Contains(t, names, "deployment1")
	assert.Contains(t, names, "deployment3")
}

func TestPagesDeploymentSpec(t *testing.T) {
	deployment := createTestPagesDeployment("my-deployment", "default")

	assert.Equal(t, "my-project", deployment.Spec.ProjectRef.Name)
	assert.Equal(t, "main", deployment.Spec.Branch)
	assert.Equal(t, networkingv1alpha2.PagesDeploymentActionCreate, deployment.Spec.Action)
	assert.Equal(t, "test-account-123", deployment.Spec.Cloudflare.AccountId)
}

func TestPagesDeploymentStatus(t *testing.T) {
	deployment := createTestPagesDeployment("my-deployment", "default")
	deployment.Status = networkingv1alpha2.PagesDeploymentStatus{
		State:        networkingv1alpha2.PagesDeploymentStateSucceeded,
		DeploymentID: "deploy-123",
		ProjectName:  "my-project",
		AccountID:    "test-account-123",
		URL:          "https://abc123.pages.dev",
		Environment:  "production",
		Stage:        "deploy",
		Message:      "Deployment succeeded",
	}

	assert.Equal(t, networkingv1alpha2.PagesDeploymentStateSucceeded, deployment.Status.State)
	assert.Equal(t, "deploy-123", deployment.Status.DeploymentID)
	assert.Equal(t, "my-project", deployment.Status.ProjectName)
	assert.Equal(t, "https://abc123.pages.dev", deployment.Status.URL)
	assert.Equal(t, "production", deployment.Status.Environment)
	assert.Equal(t, "deploy", deployment.Status.Stage)
}

func TestPagesDeploymentStates(t *testing.T) {
	tests := []struct {
		name  string
		state networkingv1alpha2.PagesDeploymentState
	}{
		{"Pending", networkingv1alpha2.PagesDeploymentStatePending},
		{"Queued", networkingv1alpha2.PagesDeploymentStateQueued},
		{"Building", networkingv1alpha2.PagesDeploymentStateBuilding},
		{"Deploying", networkingv1alpha2.PagesDeploymentStateDeploying},
		{"Succeeded", networkingv1alpha2.PagesDeploymentStateSucceeded},
		{"Failed", networkingv1alpha2.PagesDeploymentStateFailed},
		{"Cancelled", networkingv1alpha2.PagesDeploymentStateCancelled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment := createTestPagesDeployment("test", "default")
			deployment.Status.State = tt.state
			assert.Equal(t, tt.state, deployment.Status.State)
		})
	}
}

func TestPagesDeploymentActions(t *testing.T) {
	tests := []struct {
		name   string
		action networkingv1alpha2.PagesDeploymentAction
	}{
		{"Create", networkingv1alpha2.PagesDeploymentActionCreate},
		{"Retry", networkingv1alpha2.PagesDeploymentActionRetry},
		{"Rollback", networkingv1alpha2.PagesDeploymentActionRollback},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment := createTestPagesDeployment("test", "default")
			deployment.Spec.Action = tt.action
			assert.Equal(t, tt.action, deployment.Spec.Action)
		})
	}
}

func TestPagesDeploymentRetryAction(t *testing.T) {
	deployment := createTestPagesDeploymentWithRetry("my-deployment", "default", "deploy-to-retry-123")

	assert.Equal(t, networkingv1alpha2.PagesDeploymentActionRetry, deployment.Spec.Action)
	assert.Equal(t, "deploy-to-retry-123", deployment.Spec.TargetDeploymentID)
}

func TestPagesDeploymentRollbackAction(t *testing.T) {
	deployment := createTestPagesDeploymentWithRollback("my-deployment", "default", "deploy-to-rollback-123")

	assert.Equal(t, networkingv1alpha2.PagesDeploymentActionRollback, deployment.Spec.Action)
	assert.Equal(t, "deploy-to-rollback-123", deployment.Spec.TargetDeploymentID)
}

func TestReconcilerImplementsReconciler(_ *testing.T) {
	var _ reconcile.Reconciler = &PagesDeploymentReconciler{}
}

func TestReconcilerWithFakeClient(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &PagesDeploymentReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	deploymentList := &networkingv1alpha2.PagesDeploymentList{}
	err := r.List(ctx, deploymentList)
	require.NoError(t, err)
	assert.Empty(t, deploymentList.Items)
}

func TestFinalizerOperations(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	deployment := createTestPagesDeployment("test-deployment", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment).
		Build()

	// Add finalizer
	controllerutil.AddFinalizer(deployment, FinalizerName)
	err := fakeClient.Update(ctx, deployment)
	require.NoError(t, err)

	// Verify finalizer was added
	var updated networkingv1alpha2.PagesDeployment
	err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-deployment", Namespace: "default"}, &updated)
	require.NoError(t, err)
	assert.True(t, controllerutil.ContainsFinalizer(&updated, FinalizerName))

	// Remove finalizer
	controllerutil.RemoveFinalizer(&updated, FinalizerName)
	err = fakeClient.Update(ctx, &updated)
	require.NoError(t, err)

	// Verify finalizer was removed
	err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-deployment", Namespace: "default"}, &updated)
	require.NoError(t, err)
	assert.False(t, controllerutil.ContainsFinalizer(&updated, FinalizerName))
}

func TestHandleDeletion_NoFinalizer(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	deployment := createTestPagesDeployment("test-deployment", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment).
		Build()

	recorder := record.NewFakeRecorder(10)
	r := &PagesDeploymentReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: recorder,
	}

	now := metav1.Now()
	deployment.DeletionTimestamp = &now

	result, err := r.handleDeletion(ctx, deployment, "my-project")

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestPagesDeploymentWithPurgeBuildCache(t *testing.T) {
	deployment := createTestPagesDeployment("my-deployment", "default")
	deployment.Spec.PurgeBuildCache = true

	assert.True(t, deployment.Spec.PurgeBuildCache)
}

func TestPagesDeploymentStageHistory(t *testing.T) {
	deployment := createTestPagesDeployment("my-deployment", "default")
	deployment.Status.StageHistory = []networkingv1alpha2.PagesStageHistory{
		{
			Name:      "queued",
			StartedOn: "2024-01-01T10:00:00Z",
			EndedOn:   "2024-01-01T10:00:05Z",
			Status:    "success",
		},
		{
			Name:      "initialize",
			StartedOn: "2024-01-01T10:00:05Z",
			EndedOn:   "2024-01-01T10:00:30Z",
			Status:    "success",
		},
		{
			Name:      "build",
			StartedOn: "2024-01-01T10:00:30Z",
			EndedOn:   "2024-01-01T10:05:00Z",
			Status:    "success",
		},
		{
			Name:      "deploy",
			StartedOn: "2024-01-01T10:05:00Z",
			Status:    "active",
		},
	}

	assert.Len(t, deployment.Status.StageHistory, 4)
	assert.Equal(t, "queued", deployment.Status.StageHistory[0].Name)
	assert.Equal(t, "success", deployment.Status.StageHistory[0].Status)
	assert.Equal(t, "active", deployment.Status.StageHistory[3].Status)
}

func TestPagesProjectRef(t *testing.T) {
	deployment := createTestPagesDeployment("my-deployment", "default")

	// Test with Name reference
	assert.Equal(t, "my-project", deployment.Spec.ProjectRef.Name)
	assert.Empty(t, deployment.Spec.ProjectRef.CloudflareID)

	// Test with CloudflareID reference
	deployment2 := createTestPagesDeployment("my-deployment-2", "default")
	deployment2.Spec.ProjectRef = networkingv1alpha2.PagesProjectRef{
		CloudflareID: "cf-project-id",
	}
	assert.Empty(t, deployment2.Spec.ProjectRef.Name)
	assert.Equal(t, "cf-project-id", deployment2.Spec.ProjectRef.CloudflareID)
}
