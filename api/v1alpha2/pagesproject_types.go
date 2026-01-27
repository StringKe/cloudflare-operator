// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VersionPolicy defines the mode for version management.
// +kubebuilder:validation:Enum=none;targetVersion;declarativeVersions;fullVersions;gitops;latestPreview;autoPromote;external;gitopsLatest
type VersionPolicy string

const (
	// VersionPolicyNone - no version management, only project configuration
	VersionPolicyNone VersionPolicy = "none"
	// VersionPolicyTargetVersion - simplest mode: specify only target version
	VersionPolicyTargetVersion VersionPolicy = "targetVersion"
	// VersionPolicyDeclarativeVersions - declarative mode: version name list + template
	VersionPolicyDeclarativeVersions VersionPolicy = "declarativeVersions"
	// VersionPolicyFullVersions - full mode: complete version configurations (backward compatible)
	VersionPolicyFullVersions VersionPolicy = "fullVersions"
	// VersionPolicyGitOps - GitOps workflow: preview + production two-stage deployment
	VersionPolicyGitOps VersionPolicy = "gitops"
	// VersionPolicyLatestPreview - automatically track latest preview deployment
	VersionPolicyLatestPreview VersionPolicy = "latestPreview"
	// VersionPolicyAutoPromote - auto-promote after preview succeeds
	VersionPolicyAutoPromote VersionPolicy = "autoPromote"
	// VersionPolicyExternal - version controlled by external system
	VersionPolicyExternal VersionPolicy = "external"
	// VersionPolicyGitOpsLatest - CI triggers deployment, production managed by CF console
	VersionPolicyGitOpsLatest VersionPolicy = "gitopsLatest"
)

// SourceTemplateType defines the type of source template.
// +kubebuilder:validation:Enum=s3;http;oci
type SourceTemplateType string

const (
	// S3SourceTemplateType fetches from S3-compatible storage
	S3SourceTemplateType SourceTemplateType = "s3"
	// HTTPSourceTemplateType fetches from HTTP/HTTPS URL
	HTTPSourceTemplateType SourceTemplateType = "http"
	// OCISourceTemplateType fetches from OCI registry
	OCISourceTemplateType SourceTemplateType = "oci"
)

// GitOpsVersionConfig defines GitOps workflow configuration.
// CI system modifies previewVersion to trigger preview deployment.
// Operators manually modify productionVersion to promote to production.
type GitOpsVersionConfig struct {
	// PreviewVersion is the current preview environment version name.
	// CI system modifies this field to trigger preview deployment.
	// +kubebuilder:validation:Optional
	PreviewVersion string `json:"previewVersion,omitempty"`

	// ProductionVersion is the current production environment version name.
	// Operators manually modify this field to trigger promotion.
	// Must have passed preview validation when RequirePreviewValidation is true.
	// +kubebuilder:validation:Optional
	ProductionVersion string `json:"productionVersion,omitempty"`

	// SourceTemplate defines how to construct the source URL from version.
	// +kubebuilder:validation:Optional
	SourceTemplate *SourceTemplate `json:"sourceTemplate,omitempty"`

	// RequirePreviewValidation requires productionVersion to have passed preview validation.
	// When true (default), promotion fails if the version hasn't been successfully deployed as preview.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	RequirePreviewValidation *bool `json:"requirePreviewValidation,omitempty"`

	// ValidationLabels are labels that mark a version as validated.
	// The version must have all these labels to be considered validated.
	// +kubebuilder:validation:Optional
	ValidationLabels map[string]string `json:"validationLabels,omitempty"`

	// PreviewMetadata contains metadata for preview version deployments.
	// Overrides SourceTemplate.Metadata for preview deployments.
	// +kubebuilder:validation:Optional
	PreviewMetadata map[string]string `json:"previewMetadata,omitempty"`

	// ProductionMetadata contains metadata for production version deployments.
	// Overrides SourceTemplate.Metadata for production deployments.
	// +kubebuilder:validation:Optional
	ProductionMetadata map[string]string `json:"productionMetadata,omitempty"`
}

// LatestPreviewConfig defines configuration for automatically tracking the latest preview deployment.
type LatestPreviewConfig struct {
	// SourceTemplate defines how to construct the source URL from version.
	// +kubebuilder:validation:Optional
	SourceTemplate *SourceTemplate `json:"sourceTemplate,omitempty"`

	// LabelSelector selects which PagesDeployment resources to track.
	// +kubebuilder:validation:Optional
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`

	// AutoPromote automatically promotes the latest successful preview to production.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	AutoPromote bool `json:"autoPromote,omitempty"`
}

// AutoPromoteConfig defines configuration for automatic promotion after preview succeeds.
type AutoPromoteConfig struct {
	// SourceTemplate defines how to construct the source URL from version.
	// +kubebuilder:validation:Optional
	SourceTemplate *SourceTemplate `json:"sourceTemplate,omitempty"`

	// PromoteAfter specifies the wait time after preview succeeds before promoting.
	// Default is immediate promotion.
	// +kubebuilder:validation:Optional
	PromoteAfter *metav1.Duration `json:"promoteAfter,omitempty"`

	// RequireHealthCheck requires a health check before promotion.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	RequireHealthCheck bool `json:"requireHealthCheck,omitempty"`

	// HealthCheckURL is the URL to check for health before promotion.
	// Only used when RequireHealthCheck is true.
	// +kubebuilder:validation:Optional
	HealthCheckURL string `json:"healthCheckUrl,omitempty"`

	// HealthCheckTimeout is the timeout for health check.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="30s"
	HealthCheckTimeout *metav1.Duration `json:"healthCheckTimeout,omitempty"`
}

// ExternalVersionConfig defines configuration for external version control.
type ExternalVersionConfig struct {
	// WebhookURL is the URL to notify when version changes are needed.
	// +kubebuilder:validation:Optional
	WebhookURL string `json:"webhookUrl,omitempty"`

	// SyncInterval is the interval to sync version status from external system.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="5m"
	SyncInterval *metav1.Duration `json:"syncInterval,omitempty"`

	// CurrentVersion is the externally-controlled current version.
	// External systems update this field to control which version is deployed.
	// +kubebuilder:validation:Optional
	CurrentVersion string `json:"currentVersion,omitempty"`

	// ProductionVersion is the externally-controlled production version.
	// +kubebuilder:validation:Optional
	ProductionVersion string `json:"productionVersion,omitempty"`

	// SourceTemplate defines how to construct the source URL from version.
	// When specified, allows building complete Source specs for external versions.
	// +kubebuilder:validation:Optional
	SourceTemplate *SourceTemplate `json:"sourceTemplate,omitempty"`

	// Metadata contains key-value pairs for external version deployments.
	// Overrides SourceTemplate.Metadata.
	// +kubebuilder:validation:Optional
	Metadata map[string]string `json:"metadata,omitempty"`
}

// GitOpsLatestConfig defines gitops-latest mode configuration.
// CI updates the version field to trigger deployment.
// Production version switching is fully managed by CF console.
type GitOpsLatestConfig struct {
	// Version is the current version name (e.g., "sha-abc123").
	// CI updates this field to trigger deployment.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Version string `json:"version"`

	// SourceTemplate defines how to construct the source URL from version.
	// +kubebuilder:validation:Required
	SourceTemplate SourceTemplate `json:"sourceTemplate"`

	// Environment specifies the deployment target environment.
	// - production: Deploy directly to production
	// - preview: Deploy to preview environment
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=production;preview
	// +kubebuilder:default=production
	Environment string `json:"environment,omitempty"`

	// Metadata contains version-specific key-value pairs.
	// Reserved keys for deployment trigger metadata:
	//   - "commitHash": Git commit SHA
	//   - "commitMessage": Commit or deployment description
	//   - "commitDirty": "true" or "false"
	//   - "branch": Git branch name (auto-inherited from productionBranch if empty)
	// +kubebuilder:validation:Optional
	Metadata map[string]string `json:"metadata,omitempty"`
}

// VersionManagement defines version management configuration.
// Only one mode should be used at a time.
type VersionManagement struct {
	// Policy specifies the version management policy.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=none;targetVersion;declarativeVersions;fullVersions;gitops;latestPreview;autoPromote;external;gitopsLatest
	// +kubebuilder:default="none"
	Policy VersionPolicy `json:"policy,omitempty"`

	// TargetVersion configuration (used when policy=targetVersion).
	// +kubebuilder:validation:Optional
	TargetVersion *TargetVersionSpec `json:"targetVersion,omitempty"`

	// DeclarativeVersions configuration (used when policy=declarativeVersions).
	// +kubebuilder:validation:Optional
	DeclarativeVersions *DeclarativeVersionsSpec `json:"declarativeVersions,omitempty"`

	// FullVersions configuration (used when policy=fullVersions).
	// +kubebuilder:validation:Optional
	FullVersions *FullVersionsSpec `json:"fullVersions,omitempty"`

	// GitOps configuration (used when policy=gitops).
	// +kubebuilder:validation:Optional
	GitOps *GitOpsVersionConfig `json:"gitops,omitempty"`

	// LatestPreview configuration (used when policy=latestPreview).
	// +kubebuilder:validation:Optional
	LatestPreview *LatestPreviewConfig `json:"latestPreview,omitempty"`

	// AutoPromote configuration (used when policy=autoPromote).
	// +kubebuilder:validation:Optional
	AutoPromote *AutoPromoteConfig `json:"autoPromote,omitempty"`

	// External configuration (used when policy=external).
	// +kubebuilder:validation:Optional
	External *ExternalVersionConfig `json:"external,omitempty"`

	// GitOpsLatest configuration (used when policy=gitopsLatest).
	// CI triggers deployment, production managed by CF console.
	// +kubebuilder:validation:Optional
	GitOpsLatest *GitOpsLatestConfig `json:"gitopsLatest,omitempty"`
}

// TargetVersionSpec defines the simplest mode configuration - just a single target version.
type TargetVersionSpec struct {
	// Version is the target version name (e.g., "sha-abc123").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Version string `json:"version"`

	// SourceTemplate defines how to construct the source URL from version.
	// +kubebuilder:validation:Required
	SourceTemplate SourceTemplate `json:"sourceTemplate"`

	// Metadata contains version-specific key-value pairs that override SourceTemplate.Metadata.
	// +kubebuilder:validation:Optional
	Metadata map[string]string `json:"metadata,omitempty"`
}

// DeclarativeVersionsSpec defines declarative mode configuration - version list + template.
type DeclarativeVersionsSpec struct {
	// Versions is a list of version names (sorted by priority, first is latest).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Versions []string `json:"versions"`

	// SourceTemplate defines how to construct the source URL from version.
	// +kubebuilder:validation:Required
	SourceTemplate SourceTemplate `json:"sourceTemplate"`

	// ProductionTarget specifies which version to promote to production.
	// - "latest": Use versions[0] (the first/newest version in the list)
	// - "<version-name>": Use a specific version by name
	// - "": Do not automatically promote to production
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="latest"
	ProductionTarget string `json:"productionTarget,omitempty"`

	// Metadata contains key-value pairs applied to all versions in this spec.
	// Overrides SourceTemplate.Metadata.
	// +kubebuilder:validation:Optional
	Metadata map[string]string `json:"metadata,omitempty"`
}

// FullVersionsSpec defines full mode configuration - complete version objects (backward compatible).
type FullVersionsSpec struct {
	// Versions is a list of complete version configurations.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Versions []ProjectVersion `json:"versions"`

	// ProductionTarget specifies which version to promote to production.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="latest"
	ProductionTarget string `json:"productionTarget,omitempty"`
}

// SourceTemplate defines how to construct source URLs from version names.
// Only one source type should be specified.
type SourceTemplate struct {
	// Type specifies the template type.
	// +kubebuilder:validation:Required
	Type SourceTemplateType `json:"type"`

	// S3 configuration (used when type=s3).
	// +kubebuilder:validation:Optional
	S3 *S3SourceTemplate `json:"s3,omitempty"`

	// HTTP configuration (used when type=http).
	// +kubebuilder:validation:Optional
	HTTP *HTTPSourceTemplate `json:"http,omitempty"`

	// OCI configuration (used when type=oci).
	// +kubebuilder:validation:Optional
	OCI *OCISourceTemplate `json:"oci,omitempty"`

	// Metadata contains default key-value pairs applied to all versions generated from this template.
	// These can be overridden by mode-specific Metadata fields.
	// Reserved keys for deployment trigger metadata:
	//   - "commitHash": Git commit SHA
	//   - "commitMessage": Commit or deployment description
	//   - "commitDirty": "true" or "false"
	//   - "branch": Git branch name (critical for production promotion)
	// +kubebuilder:validation:Optional
	Metadata map[string]string `json:"metadata,omitempty"`
}

// S3SourceTemplate defines S3 source template configuration.
type S3SourceTemplate struct {
	// Bucket is the S3 bucket name.
	// +kubebuilder:validation:Required
	Bucket string `json:"bucket"`

	// KeyTemplate is the object key template with {{.Version}} placeholder.
	// Example: "pages-artifacts/my-app/{{.Version}}.tar.gz"
	// +kubebuilder:validation:Required
	KeyTemplate string `json:"keyTemplate"`

	// Region is the AWS region.
	// +kubebuilder:validation:Required
	Region string `json:"region"`

	// Endpoint is the S3 endpoint URL (optional, for S3-compatible services).
	// +kubebuilder:validation:Optional
	Endpoint string `json:"endpoint,omitempty"`

	// CredentialsSecretRef references a Secret containing AWS credentials.
	// +kubebuilder:validation:Optional
	CredentialsSecretRef string `json:"credentialsSecretRef,omitempty"`

	// ArchiveType is the archive type (default: "tar.gz").
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=tar.gz;tar;zip;none
	// +kubebuilder:default="tar.gz"
	ArchiveType string `json:"archiveType,omitempty"`

	// UsePathStyle forces path-style addressing instead of virtual hosted-style.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	UsePathStyle bool `json:"usePathStyle,omitempty"`
}

// HTTPSourceTemplate defines HTTP source template configuration.
type HTTPSourceTemplate struct {
	// URLTemplate is the URL template with {{.Version}} placeholder.
	// Example: "https://releases.example.com/{{.Version}}/dist.tar.gz"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https?://`
	URLTemplate string `json:"urlTemplate"`

	// ArchiveType is the archive type (default: "tar.gz").
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=tar.gz;tar;zip;none
	// +kubebuilder:default="tar.gz"
	ArchiveType string `json:"archiveType,omitempty"`

	// HeadersSecretRef references a Secret containing HTTP headers.
	// +kubebuilder:validation:Optional
	HeadersSecretRef string `json:"headersSecretRef,omitempty"`
}

// OCISourceTemplate defines OCI registry source template configuration.
type OCISourceTemplate struct {
	// Repository is the OCI repository address.
	// Example: "registry.example.com/my-app"
	// +kubebuilder:validation:Required
	Repository string `json:"repository"`

	// TagTemplate is the tag template with {{.Version}} placeholder.
	// Example: "{{.Version}}" or "v{{.Version}}"
	// +kubebuilder:validation:Required
	TagTemplate string `json:"tagTemplate"`

	// CredentialsSecretRef references a Secret containing registry credentials.
	// +kubebuilder:validation:Optional
	CredentialsSecretRef string `json:"credentialsSecretRef,omitempty"`
}

// PagesProjectState represents the state of the Pages project
// +kubebuilder:validation:Enum=Pending;Creating;Ready;Updating;Deleting;Error
type PagesProjectState string

const (
	// PagesProjectStatePending means the project is waiting to be created
	PagesProjectStatePending PagesProjectState = "Pending"
	// PagesProjectStateCreating means the project is being created
	PagesProjectStateCreating PagesProjectState = "Creating"
	// PagesProjectStateReady means the project is created and ready
	PagesProjectStateReady PagesProjectState = "Ready"
	// PagesProjectStateUpdating means the project is being updated
	PagesProjectStateUpdating PagesProjectState = "Updating"
	// PagesProjectStateDeleting means the project is being deleted
	PagesProjectStateDeleting PagesProjectState = "Deleting"
	// PagesProjectStateError means there was an error with the project
	PagesProjectStateError PagesProjectState = "Error"
)

// PagesSourceType defines the source type for the Pages project
// +kubebuilder:validation:Enum=github;gitlab;direct_upload
type PagesSourceType string

const (
	// PagesSourceTypeGitHub uses GitHub as the source
	PagesSourceTypeGitHub PagesSourceType = "github"
	// PagesSourceTypeGitLab uses GitLab as the source
	PagesSourceTypeGitLab PagesSourceType = "gitlab"
	// PagesSourceTypeDirectUpload uses direct upload
	PagesSourceTypeDirectUpload PagesSourceType = "direct_upload"
)

// PagesEnvVarType defines the type of environment variable
// +kubebuilder:validation:Enum=plain_text;secret_text
type PagesEnvVarType string

const (
	// PagesEnvVarTypePlainText is a plain text environment variable
	PagesEnvVarTypePlainText PagesEnvVarType = "plain_text"
	// PagesEnvVarTypeSecretText is a secret environment variable
	PagesEnvVarTypeSecretText PagesEnvVarType = "secret_text"
)

// PagesSourceConfig defines the source repository configuration.
type PagesSourceConfig struct {
	// Type is the source type (github, gitlab, direct_upload).
	// +kubebuilder:validation:Enum=github;gitlab;direct_upload
	// +kubebuilder:default=direct_upload
	Type PagesSourceType `json:"type,omitempty"`

	// GitHub config when type is github.
	// +kubebuilder:validation:Optional
	GitHub *PagesGitHubConfig `json:"github,omitempty"`

	// GitLab config when type is gitlab.
	// +kubebuilder:validation:Optional
	GitLab *PagesGitLabConfig `json:"gitlab,omitempty"`
}

// PagesGitHubConfig defines GitHub source configuration.
type PagesGitHubConfig struct {
	// Owner is the GitHub repository owner.
	// +kubebuilder:validation:Required
	Owner string `json:"owner"`

	// Repo is the GitHub repository name.
	// +kubebuilder:validation:Required
	Repo string `json:"repo"`

	// ProductionDeploymentsEnabled enables production deployments.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	ProductionDeploymentsEnabled *bool `json:"productionDeploymentsEnabled,omitempty"`

	// PreviewDeploymentsEnabled enables preview deployments.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	PreviewDeploymentsEnabled *bool `json:"previewDeploymentsEnabled,omitempty"`

	// PRCommentsEnabled enables PR comments.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	PRCommentsEnabled *bool `json:"prCommentsEnabled,omitempty"`

	// DeploymentsEnabled enables deployments globally.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	DeploymentsEnabled *bool `json:"deploymentsEnabled,omitempty"`
}

// PagesGitLabConfig defines GitLab source configuration.
type PagesGitLabConfig struct {
	// Owner is the GitLab namespace.
	// +kubebuilder:validation:Required
	Owner string `json:"owner"`

	// Repo is the GitLab project name.
	// +kubebuilder:validation:Required
	Repo string `json:"repo"`

	// ProductionDeploymentsEnabled enables production deployments.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	ProductionDeploymentsEnabled *bool `json:"productionDeploymentsEnabled,omitempty"`

	// PreviewDeploymentsEnabled enables preview deployments.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	PreviewDeploymentsEnabled *bool `json:"previewDeploymentsEnabled,omitempty"`

	// DeploymentsEnabled enables deployments globally.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	DeploymentsEnabled *bool `json:"deploymentsEnabled,omitempty"`
}

// PagesBuildConfig defines the build configuration.
type PagesBuildConfig struct {
	// BuildCommand is the command to build the project.
	// +kubebuilder:validation:Optional
	BuildCommand string `json:"buildCommand,omitempty"`

	// DestinationDir is the build output directory.
	// +kubebuilder:validation:Optional
	DestinationDir string `json:"destinationDir,omitempty"`

	// RootDir is the root directory for the build.
	// +kubebuilder:validation:Optional
	RootDir string `json:"rootDir,omitempty"`

	// BuildCaching enables build caching.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	BuildCaching *bool `json:"buildCaching,omitempty"`

	// WebAnalyticsTag is the Web Analytics tag.
	// +kubebuilder:validation:Optional
	WebAnalyticsTag string `json:"webAnalyticsTag,omitempty"`

	// WebAnalyticsToken is the Web Analytics token.
	// +kubebuilder:validation:Optional
	WebAnalyticsToken string `json:"webAnalyticsToken,omitempty"`
}

// PagesDeploymentConfigs contains preview and production configs.
type PagesDeploymentConfigs struct {
	// Preview contains preview environment configuration.
	// +kubebuilder:validation:Optional
	Preview *PagesDeploymentConfig `json:"preview,omitempty"`

	// Production contains production environment configuration.
	// +kubebuilder:validation:Optional
	Production *PagesDeploymentConfig `json:"production,omitempty"`
}

// PagesDeploymentConfig defines environment-specific configuration.
type PagesDeploymentConfig struct {
	// EnvironmentVariables for this environment.
	// +kubebuilder:validation:Optional
	EnvironmentVariables map[string]PagesEnvVar `json:"environmentVariables,omitempty"`

	// CompatibilityDate for Workers runtime.
	// +kubebuilder:validation:Optional
	CompatibilityDate string `json:"compatibilityDate,omitempty"`

	// CompatibilityFlags for Workers runtime.
	// +kubebuilder:validation:Optional
	CompatibilityFlags []string `json:"compatibilityFlags,omitempty"`

	// D1Bindings for D1 databases.
	// +kubebuilder:validation:Optional
	D1Bindings []PagesD1Binding `json:"d1Bindings,omitempty"`

	// DurableObjectBindings for Durable Objects.
	// +kubebuilder:validation:Optional
	DurableObjectBindings []PagesDurableObjectBinding `json:"durableObjectBindings,omitempty"`

	// KVBindings for KV namespaces.
	// +kubebuilder:validation:Optional
	KVBindings []PagesKVBinding `json:"kvBindings,omitempty"`

	// R2Bindings for R2 buckets.
	// +kubebuilder:validation:Optional
	R2Bindings []PagesR2Binding `json:"r2Bindings,omitempty"`

	// ServiceBindings for Workers services.
	// +kubebuilder:validation:Optional
	ServiceBindings []PagesServiceBinding `json:"serviceBindings,omitempty"`

	// QueueBindings for Queue producers.
	// +kubebuilder:validation:Optional
	QueueBindings []PagesQueueBinding `json:"queueBindings,omitempty"`

	// AIBindings for Workers AI.
	// +kubebuilder:validation:Optional
	AIBindings []PagesAIBinding `json:"aiBindings,omitempty"`

	// VectorizeBindings for Vectorize indexes.
	// +kubebuilder:validation:Optional
	VectorizeBindings []PagesVectorizeBinding `json:"vectorizeBindings,omitempty"`

	// HyperdriveBindings for Hyperdrive configurations.
	// +kubebuilder:validation:Optional
	HyperdriveBindings []PagesHyperdriveBinding `json:"hyperdriveBindings,omitempty"`

	// MTLSCertificates for mTLS certificates.
	// +kubebuilder:validation:Optional
	MTLSCertificates []PagesMTLSCertificate `json:"mtlsCertificates,omitempty"`

	// BrowserBinding for Browser Rendering.
	// +kubebuilder:validation:Optional
	BrowserBinding *PagesBrowserBinding `json:"browserBinding,omitempty"`

	// Placement for Smart Placement.
	// +kubebuilder:validation:Optional
	Placement *PagesPlacement `json:"placement,omitempty"`

	// UsageModel for Workers Unbound.
	// +kubebuilder:validation:Enum=bundled;unbound
	// +kubebuilder:validation:Optional
	UsageModel string `json:"usageModel,omitempty"`

	// FailOpen when Workers script fails.
	// +kubebuilder:validation:Optional
	FailOpen *bool `json:"failOpen,omitempty"`

	// AlwaysUseLatestCompatibilityDate to auto-update.
	// +kubebuilder:validation:Optional
	AlwaysUseLatestCompatibilityDate *bool `json:"alwaysUseLatestCompatibilityDate,omitempty"`
}

// PagesEnvVar defines an environment variable.
type PagesEnvVar struct {
	// Value is the plain text value.
	// +kubebuilder:validation:Optional
	Value string `json:"value,omitempty"`

	// Type is "plain_text" or "secret_text".
	// +kubebuilder:validation:Enum=plain_text;secret_text
	// +kubebuilder:default=plain_text
	Type PagesEnvVarType `json:"type,omitempty"`
}

// PagesD1Binding defines a D1 database binding.
type PagesD1Binding struct {
	// Name is the binding name.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// DatabaseID is the D1 database ID.
	// +kubebuilder:validation:Required
	DatabaseID string `json:"databaseId"`
}

// PagesDurableObjectBinding defines a Durable Object binding.
type PagesDurableObjectBinding struct {
	// Name is the binding name.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// ClassName is the Durable Object class name.
	// +kubebuilder:validation:Required
	ClassName string `json:"className"`

	// ScriptName is the Worker script name.
	// +kubebuilder:validation:Optional
	ScriptName string `json:"scriptName,omitempty"`

	// EnvironmentName is the Worker environment name.
	// +kubebuilder:validation:Optional
	EnvironmentName string `json:"environmentName,omitempty"`
}

// PagesKVBinding defines a KV namespace binding.
type PagesKVBinding struct {
	// Name is the binding name.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// NamespaceID is the KV namespace ID.
	// +kubebuilder:validation:Required
	NamespaceID string `json:"namespaceId"`
}

// PagesR2Binding defines an R2 bucket binding.
type PagesR2Binding struct {
	// Name is the binding name.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// BucketName is the R2 bucket name.
	// +kubebuilder:validation:Required
	BucketName string `json:"bucketName"`
}

// PagesServiceBinding defines a Workers service binding.
type PagesServiceBinding struct {
	// Name is the binding name.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Service is the Worker service name.
	// +kubebuilder:validation:Required
	Service string `json:"service"`

	// Environment is the Worker environment.
	// +kubebuilder:validation:Optional
	Environment string `json:"environment,omitempty"`
}

// PagesQueueBinding defines a Queue producer binding.
type PagesQueueBinding struct {
	// Name is the binding name.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// QueueName is the Queue name.
	// +kubebuilder:validation:Required
	QueueName string `json:"queueName"`
}

// PagesAIBinding defines a Workers AI binding.
type PagesAIBinding struct {
	// Name is the binding name.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// PagesVectorizeBinding defines a Vectorize index binding.
type PagesVectorizeBinding struct {
	// Name is the binding name.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// IndexName is the Vectorize index name.
	// +kubebuilder:validation:Required
	IndexName string `json:"indexName"`
}

// PagesHyperdriveBinding defines a Hyperdrive binding.
type PagesHyperdriveBinding struct {
	// Name is the binding name.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// ID is the Hyperdrive configuration ID.
	// +kubebuilder:validation:Required
	ID string `json:"id"`
}

// PagesMTLSCertificate defines an mTLS certificate binding.
type PagesMTLSCertificate struct {
	// Name is the binding name.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// CertificateID is the mTLS certificate ID.
	// +kubebuilder:validation:Required
	CertificateID string `json:"certificateId"`
}

// PagesBrowserBinding defines a Browser Rendering binding.
type PagesBrowserBinding struct {
	// Name is the binding name (optional, defaults to "BROWSER").
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=BROWSER
	Name string `json:"name,omitempty"`
}

// PagesPlacement defines Smart Placement configuration.
type PagesPlacement struct {
	// Mode is the placement mode ("smart" for Smart Placement).
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=smart
	Mode string `json:"mode,omitempty"`
}

// ProjectVersion defines a single deployment version for declarative version management.
type ProjectVersion struct {
	// Name is the version identifier (e.g., "v1.2.3", "2025-01-20").
	// Must be unique within the versions list.
	// Used to construct the PagesDeployment name as "<project-name>-<version-name>".
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`

	// Source defines where to fetch the deployment files from.
	// Supports HTTP/HTTPS URLs, S3-compatible storage, and OCI registries.
	// This field already includes Archive and Checksum configuration.
	// +kubebuilder:validation:Optional
	Source *PagesDirectUploadSourceSpec `json:"source,omitempty"`

	// Metadata contains optional key-value pairs for this version.
	// Reserved keys for deployment trigger metadata:
	//   - "commitHash": Git commit SHA (e.g., "abc123def456")
	//   - "commitMessage": Commit or deployment description
	//   - "commitDirty": "true" or "false" - indicates uncommitted changes
	// Additional keys for reference:
	//   - "author": Commit author
	//   - "buildId": CI/CD build identifier
	//   - "releaseNotes": Version release notes
	// +kubebuilder:validation:Optional
	Metadata map[string]string `json:"metadata,omitempty"`
}

// PreviewDeploymentInfo contains information about the current preview deployment.
type PreviewDeploymentInfo struct {
	// VersionName is the version name being previewed.
	// +kubebuilder:validation:Required
	VersionName string `json:"versionName"`

	// DeploymentID is the Cloudflare deployment ID.
	// +kubebuilder:validation:Optional
	DeploymentID string `json:"deploymentId,omitempty"`

	// DeploymentName is the PagesDeployment resource name.
	// +kubebuilder:validation:Required
	DeploymentName string `json:"deploymentName"`

	// URL is the preview deployment URL.
	// +kubebuilder:validation:Optional
	URL string `json:"url,omitempty"`

	// HashURL is the deployment-specific URL (e.g., abc123.my-app.pages.dev).
	// +kubebuilder:validation:Optional
	HashURL string `json:"hashUrl,omitempty"`

	// State is the current state of the preview deployment.
	// +kubebuilder:validation:Optional
	State string `json:"state,omitempty"`

	// DeployedAt is when this preview version was deployed.
	// +kubebuilder:validation:Optional
	DeployedAt *metav1.Time `json:"deployedAt,omitempty"`
}

// VersionValidation records validation history for a version.
type VersionValidation struct {
	// VersionName is the version name that was validated.
	// +kubebuilder:validation:Required
	VersionName string `json:"versionName"`

	// DeploymentID is the Cloudflare deployment ID that was validated.
	// +kubebuilder:validation:Optional
	DeploymentID string `json:"deploymentId,omitempty"`

	// ValidatedAt is when the version was validated.
	// +kubebuilder:validation:Optional
	ValidatedAt *metav1.Time `json:"validatedAt,omitempty"`

	// ValidatedBy indicates how the version was validated.
	// Values: "preview", "manual", "autoPromote"
	// +kubebuilder:validation:Optional
	ValidatedBy string `json:"validatedBy,omitempty"`

	// ValidationResult indicates whether validation passed or failed.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=passed;failed
	ValidationResult string `json:"validationResult,omitempty"`

	// Message contains additional information about the validation.
	// +kubebuilder:validation:Optional
	Message string `json:"message,omitempty"`
}

// ProductionDeploymentInfo contains information about the current production deployment.
type ProductionDeploymentInfo struct {
	// Version is the version name (from ProjectVersion).
	// +kubebuilder:validation:Required
	Version string `json:"version"`

	// DeploymentID is the Cloudflare deployment ID.
	// +kubebuilder:validation:Optional
	DeploymentID string `json:"deploymentId,omitempty"`

	// DeploymentName is the PagesDeployment resource name.
	// +kubebuilder:validation:Required
	DeploymentName string `json:"deploymentName"`

	// URL is the production deployment URL.
	// +kubebuilder:validation:Optional
	URL string `json:"url,omitempty"`

	// HashURL is the deployment-specific URL (e.g., abc123.my-app.pages.dev).
	// +kubebuilder:validation:Optional
	HashURL string `json:"hashUrl,omitempty"`

	// DeployedAt is when this version became production.
	// +kubebuilder:validation:Optional
	DeployedAt *metav1.Time `json:"deployedAt,omitempty"`
}

// ManagedVersionStatus represents the status of a managed version deployment.
type ManagedVersionStatus struct {
	// Name is the version name.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// DeploymentName is the PagesDeployment resource name.
	// +kubebuilder:validation:Required
	DeploymentName string `json:"deploymentName"`

	// State is the deployment state (Pending, Building, Succeeded, Failed, etc.).
	// +kubebuilder:validation:Optional
	State string `json:"state,omitempty"`

	// IsProduction indicates if this is the current production deployment.
	// +kubebuilder:validation:Optional
	IsProduction bool `json:"isProduction"`

	// DeploymentID is the Cloudflare deployment ID.
	// +kubebuilder:validation:Optional
	DeploymentID string `json:"deploymentId,omitempty"`

	// LastTransitionTime is when the state last changed.
	// +kubebuilder:validation:Optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
}

// WebAnalyticsSiteStatus represents the status of a single Web Analytics site.
type WebAnalyticsSiteStatus struct {
	// Hostname is the hostname being tracked.
	// +kubebuilder:validation:Required
	Hostname string `json:"hostname"`

	// SiteTag is the unique identifier for this site.
	// +kubebuilder:validation:Optional
	SiteTag string `json:"siteTag,omitempty"`

	// SiteToken is the token used for tracking.
	// +kubebuilder:validation:Optional
	SiteToken string `json:"siteToken,omitempty"`

	// AutoInstall indicates whether automatic script injection is enabled.
	// +kubebuilder:validation:Optional
	AutoInstall bool `json:"autoInstall"`

	// Enabled indicates whether this site is currently enabled.
	// +kubebuilder:validation:Optional
	Enabled bool `json:"enabled"`
}

// WebAnalyticsStatus represents the status of Cloudflare Web Analytics for this project.
type WebAnalyticsStatus struct {
	// Enabled indicates whether Web Analytics is currently enabled for any sites.
	// +kubebuilder:validation:Optional
	Enabled bool `json:"enabled"`

	// Sites contains the status of all Web Analytics sites.
	// +kubebuilder:validation:Optional
	Sites []WebAnalyticsSiteStatus `json:"sites,omitempty"`

	// --- Deprecated fields for backward compatibility ---

	// SiteTag is deprecated, use Sites[].SiteTag instead.
	// +kubebuilder:validation:Optional
	SiteTag string `json:"siteTag,omitempty"`

	// SiteToken is deprecated, use Sites[].SiteToken instead.
	// +kubebuilder:validation:Optional
	SiteToken string `json:"siteToken,omitempty"`

	// Hostname is deprecated, use Sites[].Hostname instead.
	// +kubebuilder:validation:Optional
	Hostname string `json:"hostname,omitempty"`

	// AutoInstall is deprecated, use Sites[].AutoInstall instead.
	// +kubebuilder:validation:Optional
	AutoInstall bool `json:"autoInstall"`

	// LastChecked is the last time Web Analytics status was verified.
	// +kubebuilder:validation:Optional
	LastChecked *metav1.Time `json:"lastChecked,omitempty"`

	// Message contains additional information about Web Analytics status.
	// +kubebuilder:validation:Optional
	Message string `json:"message,omitempty"`
}

// PagesDeploymentInfo contains information about a deployment.
type PagesDeploymentInfo struct {
	// ID is the deployment ID.
	// +kubebuilder:validation:Optional
	ID string `json:"id,omitempty"`

	// URL is the deployment URL.
	// +kubebuilder:validation:Optional
	URL string `json:"url,omitempty"`

	// Stage is the deployment stage (building, succeeded, failed).
	// +kubebuilder:validation:Optional
	Stage string `json:"stage,omitempty"`

	// CreatedOn is the time the deployment was created.
	// +kubebuilder:validation:Optional
	CreatedOn string `json:"createdOn,omitempty"`
}

// AdoptionPolicy constants for PagesProject
const (
	// AdoptionPolicyIfExists adopts if project exists in Cloudflare, creates if not.
	AdoptionPolicyIfExists = "IfExists"

	// AdoptionPolicyMustExist requires the project to already exist in Cloudflare.
	// Useful for importing existing projects into Kubernetes management.
	AdoptionPolicyMustExist = "MustExist"

	// AdoptionPolicyMustNotExist requires the project to NOT exist in Cloudflare.
	// This is the default behavior - creates new projects only.
	AdoptionPolicyMustNotExist = "MustNotExist"
)

// PagesProjectSpec defines the desired state of PagesProject
type PagesProjectSpec struct {
	// Name is the project name in Cloudflare Pages.
	// Must be unique within the account.
	// If not specified, defaults to the Kubernetes resource name.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^[a-z0-9][a-z0-9-]*[a-z0-9]$`
	// +kubebuilder:validation:MaxLength=58
	Name string `json:"name,omitempty"`

	// ProductionBranch is the production branch for the project.
	// +kubebuilder:validation:Required
	ProductionBranch string `json:"productionBranch"`

	// Source contains the source configuration.
	// +kubebuilder:validation:Optional
	Source *PagesSourceConfig `json:"source,omitempty"`

	// BuildConfig contains the build configuration.
	// +kubebuilder:validation:Optional
	BuildConfig *PagesBuildConfig `json:"buildConfig,omitempty"`

	// DeploymentConfigs contains environment-specific configurations.
	// +kubebuilder:validation:Optional
	DeploymentConfigs *PagesDeploymentConfigs `json:"deploymentConfigs,omitempty"`

	// Cloudflare contains Cloudflare-specific configuration.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`

	// AdoptionPolicy defines how to handle existing Cloudflare projects.
	// - IfExists: Adopt if exists, create if not
	// - MustExist: Require project to exist in Cloudflare
	// - MustNotExist: Require project to NOT exist (default, creates new)
	// +kubebuilder:validation:Enum=IfExists;MustExist;MustNotExist
	// +kubebuilder:default=MustNotExist
	// +kubebuilder:validation:Optional
	AdoptionPolicy string `json:"adoptionPolicy,omitempty"`

	// DeploymentHistoryLimit is the number of deployment records to keep in history.
	// Used for intelligent rollback feature.
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:validation:Optional
	DeploymentHistoryLimit *int `json:"deploymentHistoryLimit,omitempty"`

	// EnableWebAnalytics enables Cloudflare Web Analytics for this project.
	// When true (default), Web Analytics will be automatically enabled with auto-install
	// for the *.pages.dev domain. For custom domains, Web Analytics needs to be enabled
	// separately or through PagesDomain resources.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	EnableWebAnalytics *bool `json:"enableWebAnalytics,omitempty"`

	// DeletionPolicy specifies what happens when the Kubernetes resource is deleted.
	// Delete: The Pages project will be deleted from Cloudflare.
	// Orphan: The Pages project will be left in Cloudflare.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=Delete;Orphan
	// +kubebuilder:default=Delete
	DeletionPolicy string `json:"deletionPolicy,omitempty"`

	// VersionManagement defines the version management configuration.
	// This is the recommended way to manage deployment versions.
	// When specified, the controller uses this instead of the deprecated Versions field.
	// +kubebuilder:validation:Optional
	VersionManagement *VersionManagement `json:"versionManagement,omitempty"`

	// RevisionHistoryLimit limits the number of managed PagesDeployment resources to keep.
	// When exceeded, oldest non-production deployments are automatically deleted.
	// Production deployments are never deleted by pruning.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=10
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
}

// PagesProjectStatus defines the observed state of PagesProject
type PagesProjectStatus struct {
	// ProjectID is the Cloudflare project ID (same as name).
	// +kubebuilder:validation:Optional
	ProjectID string `json:"projectId,omitempty"`

	// AccountID is the Cloudflare account ID.
	// +kubebuilder:validation:Optional
	AccountID string `json:"accountId,omitempty"`

	// Subdomain is the *.pages.dev subdomain.
	// +kubebuilder:validation:Optional
	Subdomain string `json:"subdomain,omitempty"`

	// Domains are the custom domains configured for this project.
	// +kubebuilder:validation:Optional
	Domains []string `json:"domains,omitempty"`

	// LatestDeployment is the latest deployment info.
	// +kubebuilder:validation:Optional
	LatestDeployment *PagesDeploymentInfo `json:"latestDeployment,omitempty"`

	// State is the current state of the project.
	// +kubebuilder:validation:Optional
	State PagesProjectState `json:"state,omitempty"`

	// Conditions represent the latest available observations.
	// +kubebuilder:validation:Optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the last observed generation.
	// +kubebuilder:validation:Optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Message provides additional information about the current state.
	// +kubebuilder:validation:Optional
	Message string `json:"message,omitempty"`

	// Adopted indicates whether this project was adopted from an existing Cloudflare project.
	// +kubebuilder:validation:Optional
	Adopted bool `json:"adopted,omitempty"`

	// AdoptedAt is the timestamp when the project was adopted.
	// +kubebuilder:validation:Optional
	AdoptedAt *metav1.Time `json:"adoptedAt,omitempty"`

	// OriginalConfig stores the original Cloudflare configuration before adoption.
	// Useful for reference or potential rollback.
	// +kubebuilder:validation:Optional
	OriginalConfig *PagesProjectOriginalConfig `json:"originalConfig,omitempty"`

	// DeploymentHistory contains recent deployment records for this project.
	// Used for intelligent rollback feature.
	// +kubebuilder:validation:Optional
	DeploymentHistory []DeploymentHistoryEntry `json:"deploymentHistory,omitempty"`

	// LastSuccessfulDeploymentID is the ID of the last successful deployment.
	// Used for LastSuccessful rollback strategy.
	// +kubebuilder:validation:Optional
	LastSuccessfulDeploymentID string `json:"lastSuccessfulDeploymentId,omitempty"`

	// CurrentProduction contains information about the current production deployment.
	// Only populated when using version management policies.
	// +kubebuilder:validation:Optional
	CurrentProduction *ProductionDeploymentInfo `json:"currentProduction,omitempty"`

	// ManagedDeployments is the count of PagesDeployment resources managed by this PagesProject.
	// Only counts deployments created from spec.versionManagement.
	// +kubebuilder:validation:Optional
	ManagedDeployments int32 `json:"managedDeployments,omitempty"`

	// ManagedVersions contains status summary for each managed version.
	// Provides quick overview of all declarative versions without querying child resources.
	// +kubebuilder:validation:Optional
	ManagedVersions []ManagedVersionStatus `json:"managedVersions,omitempty"`

	// WebAnalytics contains the Web Analytics configuration status.
	// +kubebuilder:validation:Optional
	WebAnalytics *WebAnalyticsStatus `json:"webAnalytics,omitempty"`

	// VersionMapping contains the current versionName -> deploymentId mapping.
	// Only populated when using GitOps or other version-based policies.
	// +kubebuilder:validation:Optional
	VersionMapping map[string]string `json:"versionMapping,omitempty"`

	// PreviewDeployment contains information about the current preview deployment.
	// Only populated when using GitOps or latestPreview policies.
	// +kubebuilder:validation:Optional
	PreviewDeployment *PreviewDeploymentInfo `json:"previewDeployment,omitempty"`

	// ValidationHistory contains version validation history.
	// Records which versions have been validated through preview.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxItems=50
	ValidationHistory []VersionValidation `json:"validationHistory,omitempty"`

	// ActivePolicy is the current active version management policy.
	// +kubebuilder:validation:Optional
	ActivePolicy VersionPolicy `json:"activePolicy,omitempty"`
}

// PagesProjectOriginalConfig stores the original Cloudflare configuration before adoption.
type PagesProjectOriginalConfig struct {
	// ProductionBranch from Cloudflare.
	// +kubebuilder:validation:Optional
	ProductionBranch string `json:"productionBranch,omitempty"`

	// Source configuration from Cloudflare.
	// +kubebuilder:validation:Optional
	Source *PagesSourceConfig `json:"source,omitempty"`

	// BuildConfig from Cloudflare.
	// +kubebuilder:validation:Optional
	BuildConfig *PagesBuildConfig `json:"buildConfig,omitempty"`

	// DeploymentConfigs from Cloudflare.
	// +kubebuilder:validation:Optional
	DeploymentConfigs *PagesDeploymentConfigs `json:"deploymentConfigs,omitempty"`

	// Subdomain is the *.pages.dev subdomain.
	// +kubebuilder:validation:Optional
	Subdomain string `json:"subdomain,omitempty"`

	// CapturedAt is when this configuration was captured.
	// +kubebuilder:validation:Required
	CapturedAt metav1.Time `json:"capturedAt"`
}

// DeploymentHistoryEntry represents a deployment in history.
type DeploymentHistoryEntry struct {
	// DeploymentID is the Cloudflare deployment ID.
	// +kubebuilder:validation:Required
	DeploymentID string `json:"deploymentId"`

	// Version is the sequential deployment version number.
	// Starts at 1 and increments with each deployment.
	// +kubebuilder:validation:Required
	Version int `json:"version"`

	// URL is the deployment URL.
	// +kubebuilder:validation:Optional
	URL string `json:"url,omitempty"`

	// Environment is the deployment environment (production or preview).
	// +kubebuilder:validation:Optional
	Environment string `json:"environment,omitempty"`

	// Source describes the deployment source.
	// Examples: "git:main", "direct-upload:http", "rollback:v5"
	// +kubebuilder:validation:Optional
	Source string `json:"source,omitempty"`

	// SourceHash is the SHA-256 hash of the source package file.
	// Used for tracking and identifying deployments from the same source.
	// +kubebuilder:validation:Optional
	SourceHash string `json:"sourceHash,omitempty"`

	// SourceURL is the URL where the source was fetched from.
	// Only set for direct upload deployments from HTTP/S3/OCI sources.
	// +kubebuilder:validation:Optional
	SourceURL string `json:"sourceUrl,omitempty"`

	// K8sResource identifies the K8s resource that created this deployment.
	// Format: "namespace/name"
	// +kubebuilder:validation:Optional
	K8sResource string `json:"k8sResource,omitempty"`

	// CreatedAt is when the deployment was created.
	// +kubebuilder:validation:Required
	CreatedAt metav1.Time `json:"createdAt"`

	// Status is the deployment status.
	// Examples: "active", "failed", "superseded"
	// +kubebuilder:validation:Optional
	Status string `json:"status,omitempty"`

	// IsProduction indicates if this is the current production deployment.
	// +kubebuilder:validation:Optional
	IsProduction bool `json:"isProduction,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cfpages;pagesproject
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.status.projectId`
// +kubebuilder:printcolumn:name="Subdomain",type=string,JSONPath=`.status.subdomain`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PagesProject manages a Cloudflare Pages project.
// Cloudflare Pages is a JAMstack platform for deploying static sites and full-stack applications.
//
// The controller creates and manages Pages projects in your Cloudflare account,
// including build configuration, environment variables, and resource bindings.
type PagesProject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PagesProjectSpec   `json:"spec,omitempty"`
	Status PagesProjectStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PagesProjectList contains a list of PagesProject
type PagesProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PagesProject `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PagesProject{}, &PagesProjectList{})
}
