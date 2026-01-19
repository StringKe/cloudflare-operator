// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
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

// PagesDeploymentAction defines the action to perform
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

// PagesDirectUpload configures direct upload deployment.
type PagesDirectUpload struct {
	// ManifestConfigMapRef references a ConfigMap containing the file manifest.
	// The ConfigMap should have keys as file paths and values as file contents.
	// +kubebuilder:validation:Optional
	ManifestConfigMapRef string `json:"manifestConfigMapRef,omitempty"`

	// Manifest is an inline file manifest.
	// Keys are file paths, values are file contents (base64 encoded for binary files).
	// +kubebuilder:validation:Optional
	Manifest map[string]string `json:"manifest,omitempty"`
}

// PagesDeploymentSpec defines the desired state of PagesDeployment
type PagesDeploymentSpec struct {
	// ProjectRef references the PagesProject.
	// Either Name or CloudflareID/CloudflareName must be specified.
	// +kubebuilder:validation:Required
	ProjectRef PagesProjectRef `json:"projectRef"`

	// Branch is the branch to deploy from (for git-based deployments).
	// If not specified, uses the project's production branch.
	// +kubebuilder:validation:Optional
	Branch string `json:"branch,omitempty"`

	// Action is the deployment action to perform.
	// - create: Create a new deployment (default)
	// - retry: Retry a failed deployment
	// - rollback: Rollback to a previous deployment
	// +kubebuilder:validation:Enum=create;retry;rollback
	// +kubebuilder:default=create
	Action PagesDeploymentAction `json:"action,omitempty"`

	// TargetDeploymentID is the deployment ID to retry or rollback to.
	// Required when action is "retry" or "rollback".
	// +kubebuilder:validation:Optional
	TargetDeploymentID string `json:"targetDeploymentId,omitempty"`

	// PurgeBuildCache purges the build cache before deployment.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	PurgeBuildCache bool `json:"purgeBuildCache,omitempty"`

	// DirectUpload configures direct upload deployment.
	// Use this for deploying static files without a git repository.
	// +kubebuilder:validation:Optional
	DirectUpload *PagesDirectUpload `json:"directUpload,omitempty"`

	// Cloudflare contains Cloudflare-specific configuration.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`
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

	// URL is the deployment URL.
	// +kubebuilder:validation:Optional
	URL string `json:"url,omitempty"`

	// Environment is the deployment environment (production or preview).
	// +kubebuilder:validation:Optional
	Environment string `json:"environment,omitempty"`

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
