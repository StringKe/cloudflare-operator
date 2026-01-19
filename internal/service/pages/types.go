// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package pages provides types and services for Cloudflare Pages configuration management.
// Pages allows deploying static sites and full-stack applications to Cloudflare's edge network.
//
//nolint:revive // max-public-structs is acceptable for this package with multiple config types
package pages

import (
	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// Resource type constants for Pages resources
const (
	// ResourceTypePagesProject is the SyncState resource type for Pages projects
	ResourceTypePagesProject = v1alpha2.SyncResourcePagesProject
	// ResourceTypePagesDomain is the SyncState resource type for Pages custom domains
	ResourceTypePagesDomain = v1alpha2.SyncResourcePagesDomain
	// ResourceTypePagesDeployment is the SyncState resource type for Pages deployments
	ResourceTypePagesDeployment = v1alpha2.SyncResourcePagesDeployment
)

// Priority constants for Pages resources
const (
	// PriorityPagesProject is the priority for Pages project configuration
	PriorityPagesProject = 100
	// PriorityPagesDomain is the priority for Pages domain configuration
	PriorityPagesDomain = 110
	// PriorityPagesDeployment is the priority for Pages deployment configuration
	PriorityPagesDeployment = 120
)

// PagesProjectConfig represents a Pages project configuration.
type PagesProjectConfig struct {
	// Name is the project name in Cloudflare Pages
	Name string `json:"name"`
	// ProductionBranch is the production branch for the project
	ProductionBranch string `json:"productionBranch"`
	// Source contains the source configuration
	Source *PagesSourceConfig `json:"source,omitempty"`
	// BuildConfig contains the build configuration
	BuildConfig *PagesBuildConfig `json:"buildConfig,omitempty"`
	// DeploymentConfigs contains environment-specific configurations
	DeploymentConfigs *PagesDeploymentConfigs `json:"deploymentConfigs,omitempty"`
	// AdoptionPolicy defines how to handle existing projects
	AdoptionPolicy string `json:"adoptionPolicy,omitempty"`
	// DeploymentHistoryLimit is the number of history entries to keep
	DeploymentHistoryLimit int `json:"deploymentHistoryLimit,omitempty"`
}

// PagesSourceConfig defines the source repository configuration.
type PagesSourceConfig struct {
	// Type is the source type (github, gitlab, direct_upload)
	Type string `json:"type,omitempty"`
	// GitHub config when type is github
	GitHub *PagesGitHubConfig `json:"github,omitempty"`
	// GitLab config when type is gitlab
	GitLab *PagesGitLabConfig `json:"gitlab,omitempty"`
}

// PagesGitHubConfig defines GitHub source configuration.
type PagesGitHubConfig struct {
	// Owner is the GitHub repository owner
	Owner string `json:"owner"`
	// Repo is the GitHub repository name
	Repo string `json:"repo"`
	// ProductionDeploymentsEnabled enables production deployments
	ProductionDeploymentsEnabled *bool `json:"productionDeploymentsEnabled,omitempty"`
	// PreviewDeploymentsEnabled enables preview deployments
	PreviewDeploymentsEnabled *bool `json:"previewDeploymentsEnabled,omitempty"`
	// PRCommentsEnabled enables PR comments
	PRCommentsEnabled *bool `json:"prCommentsEnabled,omitempty"`
	// DeploymentsEnabled enables deployments globally
	DeploymentsEnabled *bool `json:"deploymentsEnabled,omitempty"`
}

// PagesGitLabConfig defines GitLab source configuration.
type PagesGitLabConfig struct {
	// Owner is the GitLab namespace
	Owner string `json:"owner"`
	// Repo is the GitLab project name
	Repo string `json:"repo"`
	// ProductionDeploymentsEnabled enables production deployments
	ProductionDeploymentsEnabled *bool `json:"productionDeploymentsEnabled,omitempty"`
	// PreviewDeploymentsEnabled enables preview deployments
	PreviewDeploymentsEnabled *bool `json:"previewDeploymentsEnabled,omitempty"`
	// DeploymentsEnabled enables deployments globally
	DeploymentsEnabled *bool `json:"deploymentsEnabled,omitempty"`
}

// PagesBuildConfig defines the build configuration.
type PagesBuildConfig struct {
	// BuildCommand is the command to build the project
	BuildCommand string `json:"buildCommand,omitempty"`
	// DestinationDir is the build output directory
	DestinationDir string `json:"destinationDir,omitempty"`
	// RootDir is the root directory for the build
	RootDir string `json:"rootDir,omitempty"`
	// BuildCaching enables build caching
	BuildCaching *bool `json:"buildCaching,omitempty"`
	// WebAnalyticsTag is the Web Analytics tag
	WebAnalyticsTag string `json:"webAnalyticsTag,omitempty"`
	// WebAnalyticsToken is the Web Analytics token
	WebAnalyticsToken string `json:"webAnalyticsToken,omitempty"`
}

// PagesDeploymentConfigs contains preview and production configs.
type PagesDeploymentConfigs struct {
	// Preview contains preview environment configuration
	Preview *PagesDeploymentEnvConfig `json:"preview,omitempty"`
	// Production contains production environment configuration
	Production *PagesDeploymentEnvConfig `json:"production,omitempty"`
}

// PagesDeploymentEnvConfig defines environment-specific configuration.
type PagesDeploymentEnvConfig struct {
	// EnvironmentVariables for this environment
	EnvironmentVariables map[string]PagesEnvVar `json:"environmentVariables,omitempty"`
	// CompatibilityDate for Workers runtime
	CompatibilityDate string `json:"compatibilityDate,omitempty"`
	// CompatibilityFlags for Workers runtime
	CompatibilityFlags []string `json:"compatibilityFlags,omitempty"`
	// D1Bindings for D1 databases
	D1Bindings []PagesBinding `json:"d1Bindings,omitempty"`
	// KVBindings for KV namespaces
	KVBindings []PagesBinding `json:"kvBindings,omitempty"`
	// R2Bindings for R2 buckets
	R2Bindings []PagesBinding `json:"r2Bindings,omitempty"`
	// ServiceBindings for Workers services
	ServiceBindings []PagesServiceBinding `json:"serviceBindings,omitempty"`
	// DurableObjectBindings for Durable Objects
	DurableObjectBindings []PagesDurableObjectBinding `json:"durableObjectBindings,omitempty"`
	// QueueBindings for Queue producers
	QueueBindings []PagesBinding `json:"queueBindings,omitempty"`
	// AIBindings for Workers AI
	AIBindings []string `json:"aiBindings,omitempty"`
	// VectorizeBindings for Vectorize indexes
	VectorizeBindings []PagesBinding `json:"vectorizeBindings,omitempty"`
	// HyperdriveBindings for Hyperdrive configurations
	HyperdriveBindings []PagesBinding `json:"hyperdriveBindings,omitempty"`
	// MTLSCertificates for mTLS certificates
	MTLSCertificates []PagesBinding `json:"mtlsCertificates,omitempty"`
	// BrowserBinding for Browser Rendering
	BrowserBinding string `json:"browserBinding,omitempty"`
	// PlacementMode for Smart Placement
	PlacementMode string `json:"placementMode,omitempty"`
	// UsageModel for Workers Unbound
	UsageModel string `json:"usageModel,omitempty"`
	// FailOpen when Workers script fails
	FailOpen *bool `json:"failOpen,omitempty"`
	// AlwaysUseLatestCompatibilityDate to auto-update
	AlwaysUseLatestCompatibilityDate *bool `json:"alwaysUseLatestCompatibilityDate,omitempty"`
}

// PagesEnvVar defines an environment variable.
type PagesEnvVar struct {
	// Value is the plain text value
	Value string `json:"value,omitempty"`
	// Type is "plain_text" or "secret_text"
	Type string `json:"type,omitempty"`
}

// PagesBinding defines a generic binding (name -> id mapping).
type PagesBinding struct {
	// Name is the binding name
	Name string `json:"name"`
	// ID is the resource ID (database ID, namespace ID, etc.)
	ID string `json:"id"`
}

// PagesServiceBinding defines a Workers service binding.
type PagesServiceBinding struct {
	// Name is the binding name
	Name string `json:"name"`
	// Service is the Worker service name
	Service string `json:"service"`
	// Environment is the Worker environment
	Environment string `json:"environment,omitempty"`
}

// PagesDurableObjectBinding defines a Durable Object binding.
type PagesDurableObjectBinding struct {
	// Name is the binding name
	Name string `json:"name"`
	// ClassName is the Durable Object class name
	ClassName string `json:"className"`
	// ScriptName is the Worker script name
	ScriptName string `json:"scriptName,omitempty"`
	// EnvironmentName is the Worker environment name
	EnvironmentName string `json:"environmentName,omitempty"`
}

// PagesDomainConfig represents a Pages custom domain configuration.
type PagesDomainConfig struct {
	// Domain is the custom domain name
	Domain string `json:"domain"`
	// ProjectName is the Cloudflare project name
	ProjectName string `json:"projectName"`
}

// PagesDeploymentActionConfig represents a Pages deployment action configuration.
type PagesDeploymentActionConfig struct {
	// ProjectName is the Cloudflare project name
	ProjectName string `json:"projectName"`
	// Branch is the branch to deploy (for git-based deployments)
	Branch string `json:"branch,omitempty"`
	// Action is the deployment action (create, retry, rollback)
	Action string `json:"action"`
	// TargetDeploymentID is the deployment ID to retry or rollback to
	TargetDeploymentID string `json:"targetDeploymentId,omitempty"`
	// PurgeBuildCache purges the build cache before deployment
	PurgeBuildCache bool `json:"purgeBuildCache,omitempty"`
	// DirectUpload contains configuration for direct upload deployments
	DirectUpload *DirectUploadConfig `json:"directUpload,omitempty"`
	// Rollback contains configuration for intelligent rollback
	Rollback *RollbackConfig `json:"rollback,omitempty"`
}

// DirectUploadConfig contains configuration for direct upload deployments.
type DirectUploadConfig struct {
	// Source defines where to fetch the deployment files from
	Source *v1alpha2.DirectUploadSource `json:"source,omitempty"`
	// Checksum for file integrity verification
	Checksum *v1alpha2.ChecksumConfig `json:"checksum,omitempty"`
	// Archive configuration for compressed files
	Archive *v1alpha2.ArchiveConfig `json:"archive,omitempty"`
	// ManifestConfigMapRef references a ConfigMap containing file manifest (deprecated)
	ManifestConfigMapRef string `json:"manifestConfigMapRef,omitempty"`
	// Manifest contains inline file manifest (deprecated)
	Manifest map[string]string `json:"manifest,omitempty"`
}

// RollbackConfig contains configuration for intelligent rollback.
type RollbackConfig struct {
	// Strategy defines how to select the rollback target
	Strategy string `json:"strategy"`
	// Version is the target version number (for ByVersion strategy)
	Version *int `json:"version,omitempty"`
	// DeploymentID is the exact deployment ID (for ExactDeploymentID strategy)
	DeploymentID string `json:"deploymentId,omitempty"`
}

// ProjectRegisterOptions contains options for registering a Pages project configuration.
type ProjectRegisterOptions struct {
	// ProjectName is the Cloudflare project name (also used as CloudflareID)
	ProjectName string
	// AccountID is the Cloudflare Account ID
	AccountID string
	// Source identifies the K8s resource contributing this configuration
	Source service.Source
	// Config contains the Pages project configuration
	Config PagesProjectConfig
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
}

// DomainRegisterOptions contains options for registering a Pages domain configuration.
type DomainRegisterOptions struct {
	// DomainName is the custom domain name (also used as CloudflareID)
	DomainName string
	// ProjectName is the Cloudflare project name
	ProjectName string
	// AccountID is the Cloudflare Account ID
	AccountID string
	// Source identifies the K8s resource contributing this configuration
	Source service.Source
	// Config contains the Pages domain configuration
	Config PagesDomainConfig
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
}

// DeploymentRegisterOptions contains options for registering a Pages deployment configuration.
type DeploymentRegisterOptions struct {
	// DeploymentID is the Cloudflare deployment ID (or placeholder for new deployments)
	DeploymentID string
	// ProjectName is the Cloudflare project name
	ProjectName string
	// AccountID is the Cloudflare Account ID
	AccountID string
	// Source identifies the K8s resource contributing this configuration
	Source service.Source
	// Config contains the Pages deployment configuration
	Config PagesDeploymentActionConfig
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
}
