// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
