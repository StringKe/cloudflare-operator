// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pages

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

func TestResourceTypeConstants(t *testing.T) {
	assert.Equal(t, v1alpha2.SyncResourcePagesProject, ResourceTypePagesProject)
	assert.Equal(t, v1alpha2.SyncResourcePagesDomain, ResourceTypePagesDomain)
	assert.Equal(t, v1alpha2.SyncResourcePagesDeployment, ResourceTypePagesDeployment)
}

func TestPriorityConstants(t *testing.T) {
	assert.Equal(t, 100, PriorityPagesProject)
	assert.Equal(t, 110, PriorityPagesDomain)
	assert.Equal(t, 120, PriorityPagesDeployment)

	// Ensure priority order: Project < Domain < Deployment
	assert.Less(t, PriorityPagesProject, PriorityPagesDomain)
	assert.Less(t, PriorityPagesDomain, PriorityPagesDeployment)
}

func TestPagesProjectConfig(t *testing.T) {
	config := PagesProjectConfig{
		Name:             "my-project",
		ProductionBranch: "main",
		Source: &PagesSourceConfig{
			Type: "github",
			GitHub: &PagesGitHubConfig{
				Owner: "myorg",
				Repo:  "myrepo",
			},
		},
		BuildConfig: &PagesBuildConfig{
			BuildCommand:   "npm run build",
			DestinationDir: "dist",
		},
	}

	assert.Equal(t, "my-project", config.Name)
	assert.Equal(t, "main", config.ProductionBranch)
	assert.NotNil(t, config.Source)
	assert.Equal(t, "github", config.Source.Type)
	assert.NotNil(t, config.Source.GitHub)
	assert.Equal(t, "myorg", config.Source.GitHub.Owner)
	assert.NotNil(t, config.BuildConfig)
	assert.Equal(t, "npm run build", config.BuildConfig.BuildCommand)
}

func TestPagesSourceConfig_GitHub(t *testing.T) {
	enabled := true
	disabled := false

	source := &PagesSourceConfig{
		Type: "github",
		GitHub: &PagesGitHubConfig{
			Owner:                        "myorg",
			Repo:                         "myrepo",
			ProductionDeploymentsEnabled: &enabled,
			PreviewDeploymentsEnabled:    &enabled,
			PRCommentsEnabled:            &disabled,
			DeploymentsEnabled:           &enabled,
		},
	}

	assert.Equal(t, "github", source.Type)
	assert.NotNil(t, source.GitHub)
	assert.Equal(t, "myorg", source.GitHub.Owner)
	assert.Equal(t, "myrepo", source.GitHub.Repo)
	assert.True(t, *source.GitHub.ProductionDeploymentsEnabled)
	assert.True(t, *source.GitHub.PreviewDeploymentsEnabled)
	assert.False(t, *source.GitHub.PRCommentsEnabled)
	assert.True(t, *source.GitHub.DeploymentsEnabled)
}

func TestPagesSourceConfig_GitLab(t *testing.T) {
	enabled := true

	source := &PagesSourceConfig{
		Type: "gitlab",
		GitLab: &PagesGitLabConfig{
			Owner:                        "mygroup",
			Repo:                         "myproject",
			ProductionDeploymentsEnabled: &enabled,
			PreviewDeploymentsEnabled:    &enabled,
			DeploymentsEnabled:           &enabled,
		},
	}

	assert.Equal(t, "gitlab", source.Type)
	assert.NotNil(t, source.GitLab)
	assert.Equal(t, "mygroup", source.GitLab.Owner)
	assert.Equal(t, "myproject", source.GitLab.Repo)
}

func TestPagesBuildConfig(t *testing.T) {
	buildCaching := true

	config := &PagesBuildConfig{
		BuildCommand:      "npm run build",
		DestinationDir:    "dist",
		RootDir:           "/app",
		BuildCaching:      &buildCaching,
		WebAnalyticsTag:   "my-tag",
		WebAnalyticsToken: "my-token",
	}

	assert.Equal(t, "npm run build", config.BuildCommand)
	assert.Equal(t, "dist", config.DestinationDir)
	assert.Equal(t, "/app", config.RootDir)
	assert.True(t, *config.BuildCaching)
	assert.Equal(t, "my-tag", config.WebAnalyticsTag)
	assert.Equal(t, "my-token", config.WebAnalyticsToken)
}

func TestPagesDeploymentConfigs(t *testing.T) {
	configs := &PagesDeploymentConfigs{
		Preview: &PagesDeploymentEnvConfig{
			CompatibilityDate: "2024-01-01",
		},
		Production: &PagesDeploymentEnvConfig{
			CompatibilityDate:  "2024-01-01",
			CompatibilityFlags: []string{"nodejs_compat"},
		},
	}

	assert.NotNil(t, configs.Preview)
	assert.NotNil(t, configs.Production)
	assert.Equal(t, "2024-01-01", configs.Preview.CompatibilityDate)
	assert.Equal(t, "2024-01-01", configs.Production.CompatibilityDate)
	assert.Contains(t, configs.Production.CompatibilityFlags, "nodejs_compat")
}

func TestPagesDeploymentEnvConfig_EnvironmentVariables(t *testing.T) {
	config := &PagesDeploymentEnvConfig{
		EnvironmentVariables: map[string]PagesEnvVar{
			"API_KEY": {
				Value: "secret-value",
				Type:  "secret_text",
			},
			"NODE_ENV": {
				Value: "production",
				Type:  "plain_text",
			},
		},
	}

	assert.Len(t, config.EnvironmentVariables, 2)
	assert.Equal(t, "secret-value", config.EnvironmentVariables["API_KEY"].Value)
	assert.Equal(t, "secret_text", config.EnvironmentVariables["API_KEY"].Type)
	assert.Equal(t, "production", config.EnvironmentVariables["NODE_ENV"].Value)
	assert.Equal(t, "plain_text", config.EnvironmentVariables["NODE_ENV"].Type)
}

func TestPagesDeploymentEnvConfig_Bindings(t *testing.T) {
	config := &PagesDeploymentEnvConfig{
		D1Bindings: []PagesBinding{
			{Name: "MY_DB", ID: "db-123"},
		},
		KVBindings: []PagesBinding{
			{Name: "MY_KV", ID: "kv-456"},
		},
		R2Bindings: []PagesBinding{
			{Name: "MY_BUCKET", ID: "bucket-789"},
		},
		QueueBindings: []PagesBinding{
			{Name: "MY_QUEUE", ID: "queue-abc"},
		},
		VectorizeBindings: []PagesBinding{
			{Name: "MY_INDEX", ID: "index-xyz"},
		},
		HyperdriveBindings: []PagesBinding{
			{Name: "MY_HYPERDRIVE", ID: "hyperdrive-123"},
		},
		MTLSCertificates: []PagesBinding{
			{Name: "MY_CERT", ID: "cert-456"},
		},
	}

	assert.Len(t, config.D1Bindings, 1)
	assert.Equal(t, "MY_DB", config.D1Bindings[0].Name)
	assert.Equal(t, "db-123", config.D1Bindings[0].ID)

	assert.Len(t, config.KVBindings, 1)
	assert.Equal(t, "MY_KV", config.KVBindings[0].Name)

	assert.Len(t, config.R2Bindings, 1)
	assert.Equal(t, "MY_BUCKET", config.R2Bindings[0].Name)

	assert.Len(t, config.QueueBindings, 1)
	assert.Len(t, config.VectorizeBindings, 1)
	assert.Len(t, config.HyperdriveBindings, 1)
	assert.Len(t, config.MTLSCertificates, 1)
}

func TestPagesServiceBinding(t *testing.T) {
	binding := PagesServiceBinding{
		Name:        "MY_SERVICE",
		Service:     "my-worker",
		Environment: "production",
	}

	assert.Equal(t, "MY_SERVICE", binding.Name)
	assert.Equal(t, "my-worker", binding.Service)
	assert.Equal(t, "production", binding.Environment)
}

func TestPagesDurableObjectBinding(t *testing.T) {
	binding := PagesDurableObjectBinding{
		Name:            "MY_DO",
		ClassName:       "MyDurableObject",
		ScriptName:      "my-worker",
		EnvironmentName: "production",
	}

	assert.Equal(t, "MY_DO", binding.Name)
	assert.Equal(t, "MyDurableObject", binding.ClassName)
	assert.Equal(t, "my-worker", binding.ScriptName)
	assert.Equal(t, "production", binding.EnvironmentName)
}

func TestPagesDomainConfig(t *testing.T) {
	config := PagesDomainConfig{
		Domain:      "app.example.com",
		ProjectName: "my-project",
	}

	assert.Equal(t, "app.example.com", config.Domain)
	assert.Equal(t, "my-project", config.ProjectName)
}

func TestPagesDeploymentConfig(t *testing.T) {
	// Test new Source-based configuration (preferred)
	gitConfig := PagesDeploymentConfig{
		ProjectName: "my-project",
		Environment: "production",
		Source: &DeploymentSourceConfig{
			Type: "git",
			Git: &GitSourceConfig{
				Branch: "main",
			},
		},
		PurgeBuildCache: true,
	}

	assert.Equal(t, "my-project", gitConfig.ProjectName)
	assert.Equal(t, "production", gitConfig.Environment)
	assert.NotNil(t, gitConfig.Source)
	assert.Equal(t, "git", gitConfig.Source.Type)
	assert.Equal(t, "main", gitConfig.Source.Git.Branch)
	assert.True(t, gitConfig.PurgeBuildCache)

	// Test legacy retry action (deprecated)
	retryConfig := PagesDeploymentConfig{
		ProjectName:        "my-project",
		Action:             "retry",
		TargetDeploymentID: "deploy-123",
	}

	assert.Equal(t, "retry", retryConfig.Action)
	assert.Equal(t, "deploy-123", retryConfig.TargetDeploymentID)

	// Test legacy rollback action (deprecated)
	rollbackConfig := PagesDeploymentConfig{
		ProjectName:        "my-project",
		Action:             "rollback",
		TargetDeploymentID: "deploy-456",
	}

	assert.Equal(t, "rollback", rollbackConfig.Action)
	assert.Equal(t, "deploy-456", rollbackConfig.TargetDeploymentID)

	// Test direct upload source
	directUploadConfig := PagesDeploymentConfig{
		ProjectName: "my-project",
		Environment: "preview",
		Source: &DeploymentSourceConfig{
			Type:         "directUpload",
			DirectUpload: &DirectUploadConfig{},
		},
	}

	assert.Equal(t, "preview", directUploadConfig.Environment)
	assert.Equal(t, "directUpload", directUploadConfig.Source.Type)
}

func TestProjectRegisterOptions(t *testing.T) {
	opts := ProjectRegisterOptions{
		ProjectName: "my-project",
		AccountID:   "account-123",
		Config: PagesProjectConfig{
			Name:             "my-project",
			ProductionBranch: "main",
		},
		CredentialsRef: v1alpha2.CredentialsReference{
			Name: "my-creds",
		},
	}

	assert.Equal(t, "my-project", opts.ProjectName)
	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "my-project", opts.Config.Name)
	assert.Equal(t, "my-creds", opts.CredentialsRef.Name)
}

func TestDomainRegisterOptions(t *testing.T) {
	opts := DomainRegisterOptions{
		DomainName:  "app.example.com",
		ProjectName: "my-project",
		AccountID:   "account-123",
		Config: PagesDomainConfig{
			Domain:      "app.example.com",
			ProjectName: "my-project",
		},
	}

	assert.Equal(t, "app.example.com", opts.DomainName)
	assert.Equal(t, "my-project", opts.ProjectName)
	assert.Equal(t, "account-123", opts.AccountID)
}

func TestDeploymentRegisterOptions(t *testing.T) {
	opts := DeploymentRegisterOptions{
		DeploymentID: "deploy-123",
		ProjectName:  "my-project",
		AccountID:    "account-123",
		Config: PagesDeploymentConfig{
			ProjectName: "my-project",
			Environment: "production",
			Source: &DeploymentSourceConfig{
				Type: "git",
				Git: &GitSourceConfig{
					Branch: "main",
				},
			},
		},
	}

	assert.Equal(t, "deploy-123", opts.DeploymentID)
	assert.Equal(t, "my-project", opts.ProjectName)
	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "production", opts.Config.Environment)
	assert.Equal(t, "git", opts.Config.Source.Type)
}

func TestPagesEnvVarTypes(t *testing.T) {
	tests := []struct {
		name     string
		envType  string
		expected string
	}{
		{"PlainText", "plain_text", "plain_text"},
		{"SecretText", "secret_text", "secret_text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envVar := PagesEnvVar{
				Value: "test-value",
				Type:  tt.envType,
			}
			assert.Equal(t, tt.expected, envVar.Type)
		})
	}
}
