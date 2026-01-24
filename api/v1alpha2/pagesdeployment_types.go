// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PagesDeploymentState represents the state of the Pages deployment
// +kubebuilder:validation:Enum=Pending;Queued;Building;Deploying;Succeeded;Failed;Cancelled;Retrying;RollingBack
type PagesDeploymentState string

const (
	// PagesDeploymentStatePending means the deployment is waiting to start
	PagesDeploymentStatePending PagesDeploymentState = "Pending"
	// PagesDeploymentStateQueued means the deployment is queued
	PagesDeploymentStateQueued PagesDeploymentState = "Queued"
	// PagesDeploymentStateBuilding means the deployment is building
	PagesDeploymentStateBuilding PagesDeploymentState = "Building"
	// PagesDeploymentStateDeploying means the deployment is being deployed
	PagesDeploymentStateDeploying PagesDeploymentState = "Deploying"
	// PagesDeploymentStateSucceeded means the deployment completed successfully
	PagesDeploymentStateSucceeded PagesDeploymentState = "Succeeded"
	// PagesDeploymentStateFailed means the deployment failed
	PagesDeploymentStateFailed PagesDeploymentState = "Failed"
	// PagesDeploymentStateCancelled means the deployment was cancelled
	PagesDeploymentStateCancelled PagesDeploymentState = "Cancelled"
	// PagesDeploymentStateRetrying means the deployment is being retried
	PagesDeploymentStateRetrying PagesDeploymentState = "Retrying"
	// PagesDeploymentStateRollingBack means the deployment is rolling back
	PagesDeploymentStateRollingBack PagesDeploymentState = "RollingBack"
)

// PagesDeploymentAction defines the action to perform.
//
// Deprecated: Use Environment and Source instead. Action will be removed in v1alpha3.
//
// +kubebuilder:validation:Enum=create;retry;rollback
type PagesDeploymentAction string

const (
	// PagesDeploymentActionCreate creates a new deployment
	PagesDeploymentActionCreate PagesDeploymentAction = "create"
	// PagesDeploymentActionRetry retries a failed deployment
	PagesDeploymentActionRetry PagesDeploymentAction = "retry"
	// PagesDeploymentActionRollback rolls back to a previous deployment
	PagesDeploymentActionRollback PagesDeploymentAction = "rollback"
)

// PagesDeploymentEnvironment defines the deployment environment.
// +kubebuilder:validation:Enum=production;preview
type PagesDeploymentEnvironment string

const (
	// PagesDeploymentEnvironmentProduction is the production environment.
	// Only one PagesDeployment can be the production deployment for a given PagesProject.
	PagesDeploymentEnvironmentProduction PagesDeploymentEnvironment = "production"
	// PagesDeploymentEnvironmentPreview is the preview environment.
	// Multiple preview deployments can exist for a given PagesProject.
	PagesDeploymentEnvironmentPreview PagesDeploymentEnvironment = "preview"
)

// PagesDeploymentSourceType defines the type of deployment source.
// +kubebuilder:validation:Enum=git;directUpload
type PagesDeploymentSourceType string

const (
	// PagesDeploymentSourceTypeGit deploys from a git repository branch.
	PagesDeploymentSourceTypeGit PagesDeploymentSourceType = "git"
	// PagesDeploymentSourceTypeDirectUpload deploys static files via direct upload.
	PagesDeploymentSourceTypeDirectUpload PagesDeploymentSourceType = "directUpload"
)

// PagesDeploymentSourceSpec defines the source configuration for a deployment.
// Only one source type should be specified.
type PagesDeploymentSourceSpec struct {
	// Type is the source type (git or directUpload).
	// +kubebuilder:validation:Required
	Type PagesDeploymentSourceType `json:"type"`

	// Git contains git-based deployment configuration.
	// Required when type is "git".
	// +kubebuilder:validation:Optional
	Git *PagesGitSourceSpec `json:"git,omitempty"`

	// DirectUpload contains direct upload deployment configuration.
	// Required when type is "directUpload".
	// +kubebuilder:validation:Optional
	DirectUpload *PagesDirectUploadSourceSpec `json:"directUpload,omitempty"`
}

// PagesGitSourceSpec defines git-based deployment source.
type PagesGitSourceSpec struct {
	// Branch is the branch to deploy from.
	// If not specified, uses the project's production branch.
	// +kubebuilder:validation:Optional
	Branch string `json:"branch,omitempty"`

	// CommitSha is the specific commit SHA to deploy.
	// If specified, overrides the branch's HEAD.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^[a-f0-9]{7,40}$`
	CommitSha string `json:"commitSha,omitempty"`
}

// PagesDirectUploadSourceSpec defines direct upload deployment source.
// This reuses the existing PagesDirectUpload structure for backwards compatibility.
type PagesDirectUploadSourceSpec struct {
	// Source defines where to fetch the deployment files from.
	// Supports HTTP/HTTPS URLs, S3-compatible storage, and OCI registries.
	// +kubebuilder:validation:Required
	Source *DirectUploadSource `json:"source"`

	// Checksum for file integrity verification.
	// When specified, the downloaded file will be verified before extraction.
	// +kubebuilder:validation:Optional
	Checksum *ChecksumConfig `json:"checksum,omitempty"`

	// Archive configuration for compressed files.
	// Specifies how to extract the downloaded archive.
	// +kubebuilder:validation:Optional
	Archive *ArchiveConfig `json:"archive,omitempty"`
}

// PagesDirectUpload configures direct upload deployment.
type PagesDirectUpload struct {
	// Source defines where to fetch the deployment files from.
	// Supports HTTP/HTTPS URLs, S3-compatible storage, and OCI registries.
	// +kubebuilder:validation:Optional
	Source *DirectUploadSource `json:"source,omitempty"`

	// Checksum for file integrity verification.
	// When specified, the downloaded file will be verified before extraction.
	// +kubebuilder:validation:Optional
	Checksum *ChecksumConfig `json:"checksum,omitempty"`

	// Archive configuration for compressed files.
	// Specifies how to extract the downloaded archive.
	// +kubebuilder:validation:Optional
	Archive *ArchiveConfig `json:"archive,omitempty"`

	// ManifestConfigMapRef references a ConfigMap containing the file manifest.
	// The ConfigMap should have keys as file paths and values as file contents.
	// Deprecated: Use Source instead for better flexibility.
	// +kubebuilder:validation:Optional
	ManifestConfigMapRef string `json:"manifestConfigMapRef,omitempty"`

	// Manifest is an inline file manifest.
	// Keys are file paths, values are file contents (base64 encoded for binary files).
	// Deprecated: Use Source instead for better flexibility.
	// +kubebuilder:validation:Optional
	Manifest map[string]string `json:"manifest,omitempty"`
}

// DirectUploadSource defines the source for direct upload files.
// Only one source type should be specified.
type DirectUploadSource struct {
	// HTTP source - fetch from HTTP/HTTPS URL.
	// Supports presigned URLs for private storage (e.g., S3 presigned URLs, GCS signed URLs).
	// +kubebuilder:validation:Optional
	HTTP *HTTPSource `json:"http,omitempty"`

	// S3 source - fetch from S3-compatible storage.
	// Supports AWS S3, MinIO, Cloudflare R2, and other S3-compatible services.
	// +kubebuilder:validation:Optional
	S3 *S3Source `json:"s3,omitempty"`

	// OCI source - fetch from OCI registry.
	// Useful for storing build artifacts in container registries.
	// +kubebuilder:validation:Optional
	OCI *OCISource `json:"oci,omitempty"`
}

// HTTPSource defines HTTP/HTTPS source configuration.
type HTTPSource struct {
	// URL is the HTTP/HTTPS URL to fetch files from.
	// Supports presigned URLs for private storage.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https?://`
	URL string `json:"url"`

	// Headers to include in the HTTP request.
	// Useful for authentication tokens or custom headers.
	// +kubebuilder:validation:Optional
	Headers map[string]string `json:"headers,omitempty"`

	// HeadersSecretRef references a Secret containing HTTP headers.
	// Keys in the secret become header names, values become header values.
	// This is useful for sensitive authentication headers.
	// +kubebuilder:validation:Optional
	HeadersSecretRef *corev1.LocalObjectReference `json:"headersSecretRef,omitempty"`

	// Timeout for the HTTP request.
	// +kubebuilder:default="5m"
	// +kubebuilder:validation:Optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// InsecureSkipVerify skips TLS certificate verification.
	// Use with caution, only for testing or internal URLs.
	// +kubebuilder:default=false
	// +kubebuilder:validation:Optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// S3Source defines S3-compatible storage source configuration.
type S3Source struct {
	// Bucket is the S3 bucket name.
	// +kubebuilder:validation:Required
	Bucket string `json:"bucket"`

	// Key is the object key (path) in the bucket.
	// +kubebuilder:validation:Required
	Key string `json:"key"`

	// Region is the S3 region.
	// Required for AWS S3, optional for other S3-compatible services.
	// +kubebuilder:validation:Optional
	Region string `json:"region,omitempty"`

	// Endpoint is the S3 endpoint URL.
	// Required for S3-compatible services like MinIO, Cloudflare R2.
	// Examples: "https://s3.us-west-2.amazonaws.com", "https://xxx.r2.cloudflarestorage.com"
	// +kubebuilder:validation:Optional
	Endpoint string `json:"endpoint,omitempty"`

	// CredentialsSecretRef references a Secret containing AWS credentials.
	// Expected keys: accessKeyId, secretAccessKey, sessionToken (optional)
	// +kubebuilder:validation:Optional
	CredentialsSecretRef *corev1.LocalObjectReference `json:"credentialsSecretRef,omitempty"`

	// UsePathStyle forces path-style addressing instead of virtual hosted-style.
	// Required for some S3-compatible services like MinIO.
	// +kubebuilder:default=false
	// +kubebuilder:validation:Optional
	UsePathStyle bool `json:"usePathStyle,omitempty"`
}

// OCISource defines OCI registry source configuration.
type OCISource struct {
	// Image is the OCI image reference.
	// Format: registry.example.com/repo:tag or registry.example.com/repo@sha256:digest
	// +kubebuilder:validation:Required
	Image string `json:"image"`

	// CredentialsSecretRef references a Secret containing registry credentials.
	// Supports two formats:
	// - Docker config: .dockerconfigjson key (same as imagePullSecrets)
	// - Basic auth: username and password keys
	// +kubebuilder:validation:Optional
	CredentialsSecretRef *corev1.LocalObjectReference `json:"credentialsSecretRef,omitempty"`

	// InsecureRegistry allows connecting to registries without TLS.
	// Use with caution, only for testing or internal registries.
	// +kubebuilder:default=false
	// +kubebuilder:validation:Optional
	InsecureRegistry bool `json:"insecureRegistry,omitempty"`
}

// ChecksumConfig defines file integrity verification.
type ChecksumConfig struct {
	// Algorithm is the checksum algorithm.
	// +kubebuilder:validation:Enum=sha256;sha512;md5
	// +kubebuilder:default="sha256"
	// +kubebuilder:validation:Optional
	Algorithm string `json:"algorithm,omitempty"`

	// Value is the expected checksum value in hexadecimal format.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-fA-F0-9]+$`
	Value string `json:"value"`
}

// ArchiveConfig defines archive extraction configuration.
type ArchiveConfig struct {
	// Type is the archive type.
	// - tar.gz: gzip compressed tar archive (default)
	// - tar: uncompressed tar archive
	// - zip: ZIP archive
	// - none: no extraction, treat as single file
	// +kubebuilder:validation:Enum=tar.gz;tar;zip;none
	// +kubebuilder:default="tar.gz"
	// +kubebuilder:validation:Optional
	Type string `json:"type,omitempty"`

	// StripComponents removes leading path components when extracting.
	// Similar to tar --strip-components.
	// Useful when archive contains a top-level directory.
	// +kubebuilder:default=0
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	// +kubebuilder:validation:Optional
	StripComponents int `json:"stripComponents,omitempty"`

	// SubPath extracts only a specific subdirectory from the archive.
	// Files outside this path are ignored.
	// +kubebuilder:validation:Optional
	SubPath string `json:"subPath,omitempty"`
}

// RollbackConfig defines rollback behavior.
type RollbackConfig struct {
	// Strategy defines how to select the rollback target.
	// - LastSuccessful: Roll back to the last successful deployment
	// - ByVersion: Roll back to a specific version number from history
	// - ExactDeploymentID: Roll back to a specific Cloudflare deployment ID
	// +kubebuilder:validation:Enum=LastSuccessful;ByVersion;ExactDeploymentID
	// +kubebuilder:default="LastSuccessful"
	// +kubebuilder:validation:Optional
	Strategy string `json:"strategy,omitempty"`

	// Version is the target version number (for ByVersion strategy).
	// This is the sequential version number tracked in deployment history.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Optional
	Version *int `json:"version,omitempty"`

	// DeploymentID is the exact Cloudflare deployment ID (for ExactDeploymentID strategy).
	// +kubebuilder:validation:Optional
	DeploymentID string `json:"deploymentId,omitempty"`
}

// Rollback strategy constants
const (
	// RollbackStrategyLastSuccessful rolls back to the last successful deployment.
	RollbackStrategyLastSuccessful = "LastSuccessful"

	// RollbackStrategyByVersion rolls back to a specific version number.
	RollbackStrategyByVersion = "ByVersion"

	// RollbackStrategyExactDeploymentID rolls back to a specific deployment ID.
	RollbackStrategyExactDeploymentID = "ExactDeploymentID"
)

// PagesDeploymentSpec defines the desired state of PagesDeployment
type PagesDeploymentSpec struct {
	// ProjectRef references the PagesProject.
	// Either Name or CloudflareID/CloudflareName must be specified.
	// +kubebuilder:validation:Required
	ProjectRef PagesProjectRef `json:"projectRef"`

	// Environment specifies the deployment environment.
	// - production: The production deployment (only one per project)
	// - preview: Preview deployments (multiple allowed)
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=production;preview
	Environment PagesDeploymentEnvironment `json:"environment,omitempty"`

	// Source defines the deployment source configuration.
	// When specified, uses the new declarative deployment model.
	// +kubebuilder:validation:Optional
	Source *PagesDeploymentSourceSpec `json:"source,omitempty"`

	// PurgeBuildCache purges the build cache before deployment.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	PurgeBuildCache bool `json:"purgeBuildCache,omitempty"`

	// Cloudflare contains Cloudflare-specific configuration.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`

	// ========== DEPRECATED FIELDS (for backward compatibility) ==========
	// These fields will be removed in v1alpha3. Use Environment and Source instead.

	// Branch is the branch to deploy from (for git-based deployments).
	//
	// Deprecated: Use Source.Git.Branch instead.
	//
	// +kubebuilder:validation:Optional
	Branch string `json:"branch,omitempty"`

	// Action is the deployment action to perform.
	//
	// Deprecated: Use Environment and Source instead. Rollback should be done by
	// creating a new deployment with the same source as the target.
	//
	// +kubebuilder:validation:Enum=create;retry;rollback
	// +kubebuilder:default=create
	Action PagesDeploymentAction `json:"action,omitempty"`

	// TargetDeploymentID is the deployment ID to retry or rollback to.
	// Deprecated: Use Rollback.DeploymentID or create a new deployment instead.
	// +kubebuilder:validation:Optional
	TargetDeploymentID string `json:"targetDeploymentId,omitempty"`

	// DirectUpload configures direct upload deployment.
	// Deprecated: Use Source.DirectUpload instead.
	// +kubebuilder:validation:Optional
	DirectUpload *PagesDirectUpload `json:"directUpload,omitempty"`

	// Rollback configuration for intelligent deployment rollback.
	// Deprecated: Create a new PagesDeployment with the desired source instead.
	// +kubebuilder:validation:Optional
	Rollback *RollbackConfig `json:"rollback,omitempty"`
}

// PagesDeploymentStatus defines the observed state of PagesDeployment
type PagesDeploymentStatus struct {
	// DeploymentID is the Cloudflare deployment ID.
	// +kubebuilder:validation:Optional
	DeploymentID string `json:"deploymentId,omitempty"`

	// ProjectName is the Cloudflare project name.
	// +kubebuilder:validation:Optional
	ProjectName string `json:"projectName,omitempty"`

	// AccountID is the Cloudflare account ID.
	// +kubebuilder:validation:Optional
	AccountID string `json:"accountId,omitempty"`

	// URL is the primary deployment URL.
	// For production: <project>.pages.dev
	// For preview: <hash>.<project>.pages.dev
	// +kubebuilder:validation:Optional
	URL string `json:"url,omitempty"`

	// HashURL is the unique hash-based URL for this deployment.
	// Format: <hash>.<project>.pages.dev
	// This URL is immutable and always points to this specific deployment.
	// +kubebuilder:validation:Optional
	HashURL string `json:"hashUrl,omitempty"`

	// BranchURL is the branch-based URL for this deployment (if applicable).
	// Format: <branch>.<project>.pages.dev
	// This URL points to the latest deployment for this branch.
	// +kubebuilder:validation:Optional
	BranchURL string `json:"branchUrl,omitempty"`

	// Environment is the deployment environment (production or preview).
	// +kubebuilder:validation:Optional
	Environment string `json:"environment,omitempty"`

	// IsCurrentProduction indicates if this is the current production deployment.
	// Only relevant when Environment is "production".
	// +kubebuilder:validation:Optional
	IsCurrentProduction bool `json:"isCurrentProduction,omitempty"`

	// Version is the sequential version number within the project.
	// Increments with each new deployment.
	// +kubebuilder:validation:Optional
	Version int `json:"version,omitempty"`

	// VersionName is the human-readable version identifier.
	// For managed deployments: the version name from PagesProject.spec.versions (e.g., "sha-abc123")
	// For direct deployments: extracted from the deployment name or source key.
	// +kubebuilder:validation:Optional
	VersionName string `json:"versionName,omitempty"`

	// ProductionBranch is the production branch used.
	// +kubebuilder:validation:Optional
	ProductionBranch string `json:"productionBranch,omitempty"`

	// Stage is the current deployment stage.
	// +kubebuilder:validation:Optional
	Stage string `json:"stage,omitempty"`

	// StageHistory is the history of deployment stages.
	// +kubebuilder:validation:Optional
	StageHistory []PagesStageHistory `json:"stageHistory,omitempty"`

	// BuildConfig shows the build configuration used.
	// +kubebuilder:validation:Optional
	BuildConfig *PagesBuildConfigStatus `json:"buildConfig,omitempty"`

	// Source is the deployment source info.
	// +kubebuilder:validation:Optional
	Source *PagesDeploymentSource `json:"source,omitempty"`

	// SourceDescription is a human-readable description of the deployment source.
	// Examples: "git:main", "git:feature-branch@abc123", "directUpload:http"
	// +kubebuilder:validation:Optional
	SourceDescription string `json:"sourceDescription,omitempty"`

	// State is the current state of the deployment.
	// +kubebuilder:validation:Optional
	State PagesDeploymentState `json:"state,omitempty"`

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

	// StartedAt is when the deployment started.
	// +kubebuilder:validation:Optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// FinishedAt is when the deployment finished.
	// +kubebuilder:validation:Optional
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`
}

// PagesStageHistory represents a stage in the deployment history.
type PagesStageHistory struct {
	// Name is the stage name (queued, initialize, clone_repo, build, deploy).
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`

	// StartedOn is when this stage started.
	// +kubebuilder:validation:Optional
	StartedOn string `json:"startedOn,omitempty"`

	// EndedOn is when this stage ended.
	// +kubebuilder:validation:Optional
	EndedOn string `json:"endedOn,omitempty"`

	// Status is the stage status (success, failure, active, idle, skipped).
	// +kubebuilder:validation:Optional
	Status string `json:"status,omitempty"`
}

// PagesBuildConfigStatus shows the build configuration used for a deployment.
type PagesBuildConfigStatus struct {
	// BuildCommand is the build command used.
	// +kubebuilder:validation:Optional
	BuildCommand string `json:"buildCommand,omitempty"`

	// DestinationDir is the build output directory.
	// +kubebuilder:validation:Optional
	DestinationDir string `json:"destinationDir,omitempty"`

	// RootDir is the root directory for the build.
	// +kubebuilder:validation:Optional
	RootDir string `json:"rootDir,omitempty"`

	// WebAnalyticsTag is the Web Analytics tag used.
	// +kubebuilder:validation:Optional
	WebAnalyticsTag string `json:"webAnalyticsTag,omitempty"`
}

// PagesDeploymentSource contains information about the deployment source.
type PagesDeploymentSource struct {
	// Type is the source type (github, gitlab, direct_upload).
	// +kubebuilder:validation:Optional
	Type string `json:"type,omitempty"`

	// Config contains source-specific configuration.
	// +kubebuilder:validation:Optional
	Config *PagesDeploymentSourceConfig `json:"config,omitempty"`
}

// PagesDeploymentSourceConfig contains source-specific configuration.
type PagesDeploymentSourceConfig struct {
	// Owner is the repository owner.
	// +kubebuilder:validation:Optional
	Owner string `json:"owner,omitempty"`

	// RepoName is the repository name.
	// +kubebuilder:validation:Optional
	RepoName string `json:"repoName,omitempty"`

	// ProductionBranch is the production branch.
	// +kubebuilder:validation:Optional
	ProductionBranch string `json:"productionBranch,omitempty"`

	// PRCommentEnabled indicates if PR comments are enabled.
	// +kubebuilder:validation:Optional
	PRCommentEnabled bool `json:"prCommentEnabled,omitempty"`

	// DeploymentsEnabled indicates if deployments are enabled.
	// +kubebuilder:validation:Optional
	DeploymentsEnabled bool `json:"deploymentsEnabled,omitempty"`

	// PreviewDeploymentsEnabled indicates if preview deployments are enabled.
	// +kubebuilder:validation:Optional
	PreviewDeploymentsEnabled bool `json:"previewDeploymentsEnabled,omitempty"`

	// ProductionDeploymentsEnabled indicates if production deployments are enabled.
	// +kubebuilder:validation:Optional
	ProductionDeploymentsEnabled bool `json:"productionDeploymentsEnabled,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cfpdeploy;pagesdeployment
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.status.projectName`
// +kubebuilder:printcolumn:name="DeploymentID",type=string,JSONPath=`.status.deploymentId`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.status.environment`
// +kubebuilder:printcolumn:name="Stage",type=string,JSONPath=`.status.stage`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PagesDeployment manages a deployment for a Cloudflare Pages project.
// This resource allows you to trigger deployments, retry failed deployments,
// or rollback to previous deployments.
//
// For git-based projects, deployments are typically triggered automatically
// by git pushes. Use this resource when you need to:
// - Manually trigger a deployment
// - Retry a failed deployment
// - Rollback to a previous deployment
// - Deploy static files via direct upload
type PagesDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PagesDeploymentSpec   `json:"spec,omitempty"`
	Status PagesDeploymentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PagesDeploymentList contains a list of PagesDeployment
type PagesDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PagesDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PagesDeployment{}, &PagesDeploymentList{})
}
