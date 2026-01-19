// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

//go:build e2e

package scenarios

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/test/e2e/framework"
)

// TestPagesProjectLifecycle tests the complete lifecycle of a PagesProject resource
func TestPagesProjectLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()
	testNS := "e2e-pages-project-test"

	// Setup test namespace
	require.NoError(t, f.SetupTestNamespace(testNS))
	defer f.CleanupTestNamespace(testNS)

	// Ensure operator namespace exists and create credentials
	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	t.Run("CreatePagesProject", func(t *testing.T) {
		project := &v1alpha2.PagesProject{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-test-pages-project",
				Namespace: testNS,
			},
			Spec: v1alpha2.PagesProjectSpec{
				Name:             "e2e-test-project",
				ProductionBranch: "main",
				BuildConfig: &v1alpha2.PagesBuildConfig{
					BuildCommand:   "npm run build",
					DestinationDir: "dist",
				},
				Cloudflare: v1alpha2.CloudflareDetails{
					AccountId: testAccountID,
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, project)
		require.NoError(t, err)

		// Wait for Ready condition
		err = f.WaitForCondition(project, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err, "PagesProject should become ready")

		// Verify status
		var fetched v1alpha2.PagesProject
		err = f.Client.Get(ctx, types.NamespacedName{
			Name:      project.Name,
			Namespace: project.Namespace,
		}, &fetched)
		require.NoError(t, err)
		assert.NotEmpty(t, fetched.Status.ProjectID, "ProjectName should be set")
		assert.NotEmpty(t, fetched.Status.AccountID, "AccountID should be set")
	})

	t.Run("CreatePagesProjectWithSource", func(t *testing.T) {
		enabled := true
		project := &v1alpha2.PagesProject{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-test-pages-project-github",
				Namespace: testNS,
			},
			Spec: v1alpha2.PagesProjectSpec{
				Name:             "e2e-test-project-github",
				ProductionBranch: "main",
				Source: &v1alpha2.PagesSourceConfig{
					Type: "github",
					GitHub: &v1alpha2.PagesGitHubConfig{
						Owner:                        "myorg",
						Repo:                         "myrepo",
						ProductionDeploymentsEnabled: &enabled,
						PreviewDeploymentsEnabled:    &enabled,
					},
				},
				Cloudflare: v1alpha2.CloudflareDetails{
					AccountId: testAccountID,
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, project)
		require.NoError(t, err)

		err = f.WaitForCondition(project, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err)
	})

	t.Run("UpdatePagesProject", func(t *testing.T) {
		var project v1alpha2.PagesProject
		err := f.Client.Get(ctx, types.NamespacedName{
			Name:      "e2e-test-pages-project",
			Namespace: testNS,
		}, &project)
		require.NoError(t, err)

		// Update build command
		project.Spec.BuildConfig.BuildCommand = "npm run build:prod"
		err = f.Client.Update(ctx, &project)
		require.NoError(t, err)

		// Wait for reconciliation
		err = f.WaitForStatusField(&project, func(obj client.Object) bool {
			p := obj.(*v1alpha2.PagesProject)
			return p.Spec.BuildConfig.BuildCommand == "npm run build:prod"
		}, 30*time.Second)
		require.NoError(t, err)

		// Verify update preserved
		err = f.Client.Get(ctx, types.NamespacedName{
			Name:      project.Name,
			Namespace: project.Namespace,
		}, &project)
		require.NoError(t, err)
		assert.Equal(t, "npm run build:prod", project.Spec.BuildConfig.BuildCommand)
	})

	t.Run("DeletePagesProjects", func(t *testing.T) {
		projects := []string{"e2e-test-pages-project", "e2e-test-pages-project-github"}

		for _, name := range projects {
			var project v1alpha2.PagesProject
			err := f.Client.Get(ctx, types.NamespacedName{
				Name:      name,
				Namespace: testNS,
			}, &project)
			if err != nil {
				continue // Already deleted or doesn't exist
			}

			err = f.Client.Delete(ctx, &project)
			require.NoError(t, err)

			err = f.WaitForDeletion(&project, time.Minute)
			assert.NoError(t, err, "PagesProject %s should be deleted", name)
		}
	})
}

// TestPagesDomainLifecycle tests the complete lifecycle of a PagesDomain resource
func TestPagesDomainLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()
	testNS := "e2e-pages-domain-test"

	require.NoError(t, f.SetupTestNamespace(testNS))
	defer f.CleanupTestNamespace(testNS)

	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	// First create a PagesProject to reference
	project := &v1alpha2.PagesProject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-pages-project-for-domain",
			Namespace: testNS,
		},
		Spec: v1alpha2.PagesProjectSpec{
			Name:             "e2e-project-for-domain",
			ProductionBranch: "main",
			Cloudflare: v1alpha2.CloudflareDetails{
				AccountId: testAccountID,
				CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
					Name: testCredentialsName,
				},
			},
		},
	}

	err = f.Client.Create(ctx, project)
	require.NoError(t, err)
	defer func() {
		_ = f.Client.Delete(ctx, project)
		_ = f.WaitForDeletion(project, time.Minute)
	}()

	// Wait for project to be ready
	err = f.WaitForCondition(project, "Ready", metav1.ConditionTrue, 2*time.Minute)
	require.NoError(t, err)

	t.Run("CreatePagesDomain", func(t *testing.T) {
		domain := &v1alpha2.PagesDomain{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-test-pages-domain",
				Namespace: testNS,
			},
			Spec: v1alpha2.PagesDomainSpec{
				Domain: "app.example.com",
				ProjectRef: v1alpha2.PagesProjectRef{
					Name: project.Name,
				},
				Cloudflare: v1alpha2.CloudflareDetails{
					AccountId: testAccountID,
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, domain)
		require.NoError(t, err)

		// Wait for Ready condition
		err = f.WaitForCondition(domain, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err, "PagesDomain should become ready")

		// Verify status
		var fetched v1alpha2.PagesDomain
		err = f.Client.Get(ctx, types.NamespacedName{
			Name:      domain.Name,
			Namespace: domain.Namespace,
		}, &fetched)
		require.NoError(t, err)
		assert.NotEmpty(t, fetched.Status.ProjectName, "ProjectName should be set")
	})

	t.Run("CreatePagesDomainWithCloudflareID", func(t *testing.T) {
		domain := &v1alpha2.PagesDomain{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-test-pages-domain-cfid",
				Namespace: testNS,
			},
			Spec: v1alpha2.PagesDomainSpec{
				Domain: "api.example.com",
				ProjectRef: v1alpha2.PagesProjectRef{
					CloudflareID: "e2e-project-for-domain", // Reference by Cloudflare project name
				},
				Cloudflare: v1alpha2.CloudflareDetails{
					AccountId: testAccountID,
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, domain)
		require.NoError(t, err)

		err = f.WaitForCondition(domain, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err)
	})

	t.Run("DeletePagesDomains", func(t *testing.T) {
		domains := []string{"e2e-test-pages-domain", "e2e-test-pages-domain-cfid"}

		for _, name := range domains {
			var domain v1alpha2.PagesDomain
			err := f.Client.Get(ctx, types.NamespacedName{
				Name:      name,
				Namespace: testNS,
			}, &domain)
			if err != nil {
				continue
			}

			err = f.Client.Delete(ctx, &domain)
			require.NoError(t, err)

			err = f.WaitForDeletion(&domain, time.Minute)
			assert.NoError(t, err, "PagesDomain %s should be deleted", name)
		}
	})
}

// TestPagesDeploymentLifecycle tests the complete lifecycle of a PagesDeployment resource
func TestPagesDeploymentLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()
	testNS := "e2e-pages-deployment-test"

	require.NoError(t, f.SetupTestNamespace(testNS))
	defer f.CleanupTestNamespace(testNS)

	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	// First create a PagesProject to reference
	project := &v1alpha2.PagesProject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-pages-project-for-deployment",
			Namespace: testNS,
		},
		Spec: v1alpha2.PagesProjectSpec{
			Name:             "e2e-project-for-deployment",
			ProductionBranch: "main",
			Source: &v1alpha2.PagesSourceConfig{
				Type: "github",
				GitHub: &v1alpha2.PagesGitHubConfig{
					Owner: "myorg",
					Repo:  "myrepo",
				},
			},
			Cloudflare: v1alpha2.CloudflareDetails{
				AccountId: testAccountID,
				CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
					Name: testCredentialsName,
				},
			},
		},
	}

	err = f.Client.Create(ctx, project)
	require.NoError(t, err)
	defer func() {
		_ = f.Client.Delete(ctx, project)
		_ = f.WaitForDeletion(project, time.Minute)
	}()

	// Wait for project to be ready
	err = f.WaitForCondition(project, "Ready", metav1.ConditionTrue, 2*time.Minute)
	require.NoError(t, err)

	var deploymentID string

	t.Run("CreatePagesDeployment", func(t *testing.T) {
		deployment := &v1alpha2.PagesDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-test-pages-deployment",
				Namespace: testNS,
			},
			Spec: v1alpha2.PagesDeploymentSpec{
				ProjectRef: v1alpha2.PagesProjectRef{
					Name: project.Name,
				},
				Branch: "main",
				Action: "create",
				Cloudflare: v1alpha2.CloudflareDetails{
					AccountId: testAccountID,
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, deployment)
		require.NoError(t, err)

		// Wait for deployment to complete (either succeeded or failed)
		err = f.WaitForStatusField(deployment, func(obj client.Object) bool {
			d := obj.(*v1alpha2.PagesDeployment)
			return d.Status.State == v1alpha2.PagesDeploymentStateSucceeded ||
				d.Status.State == v1alpha2.PagesDeploymentStateFailed ||
				d.Status.DeploymentID != ""
		}, 5*time.Minute)
		assert.NoError(t, err, "PagesDeployment should complete")

		// Verify status
		var fetched v1alpha2.PagesDeployment
		err = f.Client.Get(ctx, types.NamespacedName{
			Name:      deployment.Name,
			Namespace: deployment.Namespace,
		}, &fetched)
		require.NoError(t, err)
		assert.NotEmpty(t, fetched.Status.DeploymentID, "DeploymentID should be set")
		deploymentID = fetched.Status.DeploymentID
	})

	t.Run("CreatePagesDeploymentWithPurgeBuildCache", func(t *testing.T) {
		deployment := &v1alpha2.PagesDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-test-pages-deployment-purge",
				Namespace: testNS,
			},
			Spec: v1alpha2.PagesDeploymentSpec{
				ProjectRef: v1alpha2.PagesProjectRef{
					Name: project.Name,
				},
				Branch:          "main",
				Action:          "create",
				PurgeBuildCache: true,
				Cloudflare: v1alpha2.CloudflareDetails{
					AccountId: testAccountID,
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, deployment)
		require.NoError(t, err)

		err = f.WaitForStatusField(deployment, func(obj client.Object) bool {
			d := obj.(*v1alpha2.PagesDeployment)
			return d.Status.DeploymentID != ""
		}, 5*time.Minute)
		assert.NoError(t, err)
	})

	t.Run("RetryPagesDeployment", func(t *testing.T) {
		if deploymentID == "" {
			t.Skip("No deployment ID available for retry test")
		}

		deployment := &v1alpha2.PagesDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-test-pages-deployment-retry",
				Namespace: testNS,
			},
			Spec: v1alpha2.PagesDeploymentSpec{
				ProjectRef: v1alpha2.PagesProjectRef{
					Name: project.Name,
				},
				Action:             "retry",
				TargetDeploymentID: deploymentID,
				Cloudflare: v1alpha2.CloudflareDetails{
					AccountId: testAccountID,
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, deployment)
		require.NoError(t, err)

		err = f.WaitForStatusField(deployment, func(obj client.Object) bool {
			d := obj.(*v1alpha2.PagesDeployment)
			return d.Status.DeploymentID != ""
		}, 5*time.Minute)
		assert.NoError(t, err)
	})

	t.Run("RollbackPagesDeployment", func(t *testing.T) {
		if deploymentID == "" {
			t.Skip("No deployment ID available for rollback test")
		}

		deployment := &v1alpha2.PagesDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-test-pages-deployment-rollback",
				Namespace: testNS,
			},
			Spec: v1alpha2.PagesDeploymentSpec{
				ProjectRef: v1alpha2.PagesProjectRef{
					Name: project.Name,
				},
				Action:             "rollback",
				TargetDeploymentID: deploymentID,
				Cloudflare: v1alpha2.CloudflareDetails{
					AccountId: testAccountID,
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, deployment)
		require.NoError(t, err)

		err = f.WaitForStatusField(deployment, func(obj client.Object) bool {
			d := obj.(*v1alpha2.PagesDeployment)
			return d.Status.DeploymentID != ""
		}, 5*time.Minute)
		assert.NoError(t, err)
	})

	t.Run("DeletePagesDeployments", func(t *testing.T) {
		deployments := []string{
			"e2e-test-pages-deployment",
			"e2e-test-pages-deployment-purge",
			"e2e-test-pages-deployment-retry",
			"e2e-test-pages-deployment-rollback",
		}

		for _, name := range deployments {
			var deployment v1alpha2.PagesDeployment
			err := f.Client.Get(ctx, types.NamespacedName{
				Name:      name,
				Namespace: testNS,
			}, &deployment)
			if err != nil {
				continue
			}

			err = f.Client.Delete(ctx, &deployment)
			require.NoError(t, err)

			err = f.WaitForDeletion(&deployment, time.Minute)
			assert.NoError(t, err, "PagesDeployment %s should be deleted", name)
		}
	})
}

// TestPagesProjectWithDeploymentConfigs tests PagesProject with environment configurations
func TestPagesProjectWithDeploymentConfigs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()
	testNS := "e2e-pages-config-test"

	require.NoError(t, f.SetupTestNamespace(testNS))
	defer f.CleanupTestNamespace(testNS)

	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	project := &v1alpha2.PagesProject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-test-pages-project-config",
			Namespace: testNS,
		},
		Spec: v1alpha2.PagesProjectSpec{
			Name:             "e2e-project-config",
			ProductionBranch: "main",
			BuildConfig: &v1alpha2.PagesBuildConfig{
				BuildCommand:   "npm run build",
				DestinationDir: "dist",
			},
			DeploymentConfigs: &v1alpha2.PagesDeploymentConfigs{
				Preview: &v1alpha2.PagesDeploymentConfig{
					CompatibilityDate: "2024-01-01",
					EnvironmentVariables: map[string]v1alpha2.PagesEnvVar{
						"NODE_ENV": {
							Value: "development",
							Type:  "plain_text",
						},
					},
				},
				Production: &v1alpha2.PagesDeploymentConfig{
					CompatibilityDate:  "2024-01-01",
					CompatibilityFlags: []string{"nodejs_compat"},
					EnvironmentVariables: map[string]v1alpha2.PagesEnvVar{
						"NODE_ENV": {
							Value: "production",
							Type:  "plain_text",
						},
						"API_SECRET": {
							Value: "secret-value",
							Type:  "secret_text",
						},
					},
					KVBindings: []v1alpha2.PagesKVBinding{
						{Name: "MY_KV", NamespaceID: "kv-namespace-123"},
					},
				},
			},
			Cloudflare: v1alpha2.CloudflareDetails{
				AccountId: testAccountID,
				CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
					Name: testCredentialsName,
				},
			},
		},
	}

	// Pre-cleanup
	_ = f.Client.Delete(ctx, project, client.PropagationPolicy(metav1.DeletePropagationForeground))
	_ = f.WaitForDeletion(project, 30*time.Second)

	err = f.Client.Create(ctx, project)
	require.NoError(t, err)

	err = f.WaitForCondition(project, "Ready", metav1.ConditionTrue, 2*time.Minute)
	assert.NoError(t, err)

	// Verify status
	var fetched v1alpha2.PagesProject
	err = f.Client.Get(ctx, types.NamespacedName{
		Name:      project.Name,
		Namespace: project.Namespace,
	}, &fetched)
	require.NoError(t, err)
	assert.NotEmpty(t, fetched.Status.ProjectID)

	// Cleanup
	_ = f.Client.Delete(ctx, project)
	_ = f.WaitForDeletion(project, time.Minute)
}
