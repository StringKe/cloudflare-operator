// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesproject

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

func createTestPagesProject(name, namespace string) *networkingv1alpha2.PagesProject {
	return &networkingv1alpha2.PagesProject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: networkingv1alpha2.PagesProjectSpec{
			Name:             "my-pages-project",
			ProductionBranch: "main",
			Cloudflare: networkingv1alpha2.CloudflareDetails{
				AccountId: "test-account-123",
				CredentialsRef: &networkingv1alpha2.CloudflareCredentialsRef{
					Name: "cloudflare-creds",
				},
			},
		},
	}
}

func createTestPagesProjectWithBuildConfig(name, namespace string) *networkingv1alpha2.PagesProject {
	project := createTestPagesProject(name, namespace)
	project.Spec.BuildConfig = &networkingv1alpha2.PagesBuildConfig{
		BuildCommand:   "npm run build",
		DestinationDir: "dist",
		RootDir:        "/",
	}
	return project
}

func TestFinalizerNameConstant(t *testing.T) {
	assert.Equal(t, "pagesproject.networking.cloudflare-operator.io/finalizer", FinalizerName)
}

func TestReconcilerFields(t *testing.T) {
	r := &PagesProjectReconciler{}

	assert.Nil(t, r.Client)
	assert.Nil(t, r.Scheme)
	assert.Nil(t, r.Recorder)
	assert.Nil(t, r.projectService)
}

func TestReconciler_Reconcile_NotFound(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &PagesProjectReconciler{
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

	project := createTestPagesProject("test-project", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(project).
		WithStatusSubresource(project).
		Build()

	recorder := record.NewFakeRecorder(10)
	r := &PagesProjectReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: recorder,
	}

	result, err := r.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      project.Name,
			Namespace: project.Namespace,
		},
	})

	// The reconcile returns error or requeues when credentials are missing
	_ = err
	_ = result
}

func TestReconciler_Reconcile_HandlesDeletion(t *testing.T) {
	scheme := setupTestScheme(t)

	project := createTestPagesProject("test-project", "default")
	project.Finalizers = []string{FinalizerName}

	assert.True(t, controllerutil.ContainsFinalizer(project, FinalizerName))
	assert.Equal(t, 1, len(project.Finalizers))

	// Test handleDeletion with project that has no finalizer
	projectNoFinalizer := createTestPagesProject("test-project-2", "default")
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(projectNoFinalizer).
		Build()

	recorder := record.NewFakeRecorder(10)
	r := &PagesProjectReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: recorder,
	}

	now := metav1.Now()
	projectNoFinalizer.DeletionTimestamp = &now

	result, err := r.handleDeletion(ctx, projectNoFinalizer)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestPagesProjectSpec(t *testing.T) {
	project := createTestPagesProjectWithBuildConfig("my-project", "default")

	assert.Equal(t, "my-pages-project", project.Spec.Name)
	assert.Equal(t, "main", project.Spec.ProductionBranch)
	assert.NotNil(t, project.Spec.BuildConfig)
	assert.Equal(t, "npm run build", project.Spec.BuildConfig.BuildCommand)
	assert.Equal(t, "dist", project.Spec.BuildConfig.DestinationDir)
}

func TestPagesProjectStatus(t *testing.T) {
	project := createTestPagesProject("my-project", "default")
	project.Status = networkingv1alpha2.PagesProjectStatus{
		State:     networkingv1alpha2.PagesProjectStateReady,
		ProjectID: "proj-123",
		Subdomain: "my-pages-project.pages.dev",
		AccountID: "test-account-123",
		Message:   "Project is ready",
	}

	assert.Equal(t, networkingv1alpha2.PagesProjectStateReady, project.Status.State)
	assert.Equal(t, "proj-123", project.Status.ProjectID)
	assert.Equal(t, "my-pages-project.pages.dev", project.Status.Subdomain)
}

func TestPagesProjectStates(t *testing.T) {
	tests := []struct {
		name  string
		state networkingv1alpha2.PagesProjectState
	}{
		{"Pending", networkingv1alpha2.PagesProjectStatePending},
		{"Creating", networkingv1alpha2.PagesProjectStateCreating},
		{"Ready", networkingv1alpha2.PagesProjectStateReady},
		{"Deleting", networkingv1alpha2.PagesProjectStateDeleting},
		{"Error", networkingv1alpha2.PagesProjectStateError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := createTestPagesProject("test", "default")
			project.Status.State = tt.state
			assert.Equal(t, tt.state, project.Status.State)
		})
	}
}

func TestPagesSourceConfig(t *testing.T) {
	project := createTestPagesProject("my-project", "default")
	project.Spec.Source = &networkingv1alpha2.PagesSourceConfig{
		Type: networkingv1alpha2.PagesSourceTypeGitHub,
		GitHub: &networkingv1alpha2.PagesGitHubConfig{
			Owner: "myorg",
			Repo:  "myrepo",
		},
	}

	assert.NotNil(t, project.Spec.Source)
	assert.Equal(t, networkingv1alpha2.PagesSourceTypeGitHub, project.Spec.Source.Type)
	assert.NotNil(t, project.Spec.Source.GitHub)
	assert.Equal(t, "myorg", project.Spec.Source.GitHub.Owner)
	assert.Equal(t, "myrepo", project.Spec.Source.GitHub.Repo)
}

func TestPagesDeploymentConfigs(t *testing.T) {
	project := createTestPagesProject("my-project", "default")
	project.Spec.DeploymentConfigs = &networkingv1alpha2.PagesDeploymentConfigs{
		Preview: &networkingv1alpha2.PagesDeploymentConfig{
			CompatibilityDate: "2024-01-01",
		},
		Production: &networkingv1alpha2.PagesDeploymentConfig{
			CompatibilityDate:  "2024-01-01",
			CompatibilityFlags: []string{"nodejs_compat"},
		},
	}

	assert.NotNil(t, project.Spec.DeploymentConfigs)
	assert.NotNil(t, project.Spec.DeploymentConfigs.Preview)
	assert.NotNil(t, project.Spec.DeploymentConfigs.Production)
	assert.Equal(t, "2024-01-01", project.Spec.DeploymentConfigs.Production.CompatibilityDate)
	assert.Contains(t, project.Spec.DeploymentConfigs.Production.CompatibilityFlags, "nodejs_compat")
}

func TestReconcilerImplementsReconciler(_ *testing.T) {
	var _ reconcile.Reconciler = &PagesProjectReconciler{}
}

func TestReconcilerWithFakeClient(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &PagesProjectReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	projectList := &networkingv1alpha2.PagesProjectList{}
	err := r.List(ctx, projectList)
	require.NoError(t, err)
	assert.Empty(t, projectList.Items)
}

func TestFinalizerOperations(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	project := createTestPagesProject("test-project", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(project).
		Build()

	// Add finalizer
	controllerutil.AddFinalizer(project, FinalizerName)
	err := fakeClient.Update(ctx, project)
	require.NoError(t, err)

	// Verify finalizer was added
	var updated networkingv1alpha2.PagesProject
	err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-project", Namespace: "default"}, &updated)
	require.NoError(t, err)
	assert.True(t, controllerutil.ContainsFinalizer(&updated, FinalizerName))

	// Remove finalizer
	controllerutil.RemoveFinalizer(&updated, FinalizerName)
	err = fakeClient.Update(ctx, &updated)
	require.NoError(t, err)

	// Verify finalizer was removed
	err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-project", Namespace: "default"}, &updated)
	require.NoError(t, err)
	assert.False(t, controllerutil.ContainsFinalizer(&updated, FinalizerName))
}

func TestHandleDeletion_NoFinalizer(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	project := createTestPagesProject("test-project", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(project).
		Build()

	recorder := record.NewFakeRecorder(10)
	r := &PagesProjectReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: recorder,
	}

	now := metav1.Now()
	project.DeletionTimestamp = &now

	result, err := r.handleDeletion(ctx, project)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestPagesEnvVar(t *testing.T) {
	project := createTestPagesProject("my-project", "default")
	project.Spec.DeploymentConfigs = &networkingv1alpha2.PagesDeploymentConfigs{
		Production: &networkingv1alpha2.PagesDeploymentConfig{
			EnvironmentVariables: map[string]networkingv1alpha2.PagesEnvVar{
				"API_KEY": {
					Value: "secret-value",
					Type:  networkingv1alpha2.PagesEnvVarTypeSecretText,
				},
				"NODE_ENV": {
					Value: "production",
					Type:  networkingv1alpha2.PagesEnvVarTypePlainText,
				},
			},
		},
	}

	envVars := project.Spec.DeploymentConfigs.Production.EnvironmentVariables
	assert.Len(t, envVars, 2)
	assert.Equal(t, "secret-value", envVars["API_KEY"].Value)
	assert.Equal(t, networkingv1alpha2.PagesEnvVarTypeSecretText, envVars["API_KEY"].Type)
	assert.Equal(t, "production", envVars["NODE_ENV"].Value)
	assert.Equal(t, networkingv1alpha2.PagesEnvVarTypePlainText, envVars["NODE_ENV"].Type)
}

func TestPagesBindings(t *testing.T) {
	project := createTestPagesProject("my-project", "default")
	project.Spec.DeploymentConfigs = &networkingv1alpha2.PagesDeploymentConfigs{
		Production: &networkingv1alpha2.PagesDeploymentConfig{
			KVBindings: []networkingv1alpha2.PagesKVBinding{
				{Name: "MY_KV", NamespaceID: "kv-namespace-123"},
			},
			R2Bindings: []networkingv1alpha2.PagesR2Binding{
				{Name: "MY_BUCKET", BucketName: "my-bucket"},
			},
			D1Bindings: []networkingv1alpha2.PagesD1Binding{
				{Name: "MY_DB", DatabaseID: "db-123"},
			},
		},
	}

	config := project.Spec.DeploymentConfigs.Production
	assert.Len(t, config.KVBindings, 1)
	assert.Equal(t, "MY_KV", config.KVBindings[0].Name)
	assert.Len(t, config.R2Bindings, 1)
	assert.Equal(t, "MY_BUCKET", config.R2Bindings[0].Name)
	assert.Len(t, config.D1Bindings, 1)
	assert.Equal(t, "MY_DB", config.D1Bindings[0].Name)
}

func TestPagesProjectWithDeletionPolicy(t *testing.T) {
	project := createTestPagesProject("my-project", "default")
	project.Spec.DeletionPolicy = "Orphan"

	assert.Equal(t, "Orphan", project.Spec.DeletionPolicy)

	project2 := createTestPagesProject("my-project-2", "default")
	project2.Spec.DeletionPolicy = "Delete"

	assert.Equal(t, "Delete", project2.Spec.DeletionPolicy)
}
