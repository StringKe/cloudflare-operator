// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PagesPromotionState represents the state of the Pages promotion
// +kubebuilder:validation:Enum=Pending;Validating;Promoting;Promoted;Failed
type PagesPromotionState string

const (
	// PagesPromotionStatePending means the promotion is waiting to start
	PagesPromotionStatePending PagesPromotionState = "Pending"
	// PagesPromotionStateValidating means the promotion is validating the deployment
	PagesPromotionStateValidating PagesPromotionState = "Validating"
	// PagesPromotionStatePromoting means the promotion is in progress
	PagesPromotionStatePromoting PagesPromotionState = "Promoting"
	// PagesPromotionStatePromoted means the promotion completed successfully
	PagesPromotionStatePromoted PagesPromotionState = "Promoted"
	// PagesPromotionStateFailed means the promotion failed
	PagesPromotionStateFailed PagesPromotionState = "Failed"
)

// PagesDeploymentRef references a deployment to promote.
// Either Name (for K8s PagesDeployment) or DeploymentID (for direct CF deployment) must be specified.
type PagesDeploymentRef struct {
	// Name is the name of a K8s PagesDeployment resource in the same namespace.
	// The controller will wait for this deployment to succeed before promoting.
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`

	// DeploymentID is a Cloudflare deployment ID to promote directly.
	// Use this when the deployment was created outside of K8s management.
	// +kubebuilder:validation:Optional
	DeploymentID string `json:"deploymentId,omitempty"`
}

// PromotedDeploymentInfo contains information about the promoted deployment.
type PromotedDeploymentInfo struct {
	// DeploymentID is the Cloudflare deployment ID that was promoted.
	// +kubebuilder:validation:Required
	DeploymentID string `json:"deploymentId"`

	// URL is the production URL after promotion.
	// Format: <project>.pages.dev
	// +kubebuilder:validation:Optional
	URL string `json:"url,omitempty"`

	// HashURL is the unique hash-based URL for this deployment.
	// Format: <hash>.<project>.pages.dev
	// +kubebuilder:validation:Optional
	HashURL string `json:"hashUrl,omitempty"`

	// SourceDeploymentName is the K8s PagesDeployment resource name that was promoted.
	// Only set when promoting from a K8s PagesDeployment.
	// +kubebuilder:validation:Optional
	SourceDeploymentName string `json:"sourceDeploymentName,omitempty"`

	// PromotedAt is when this deployment was promoted to production.
	// +kubebuilder:validation:Optional
	PromotedAt *metav1.Time `json:"promotedAt,omitempty"`
}

// PreviousDeploymentInfo contains information about the deployment that was replaced.
type PreviousDeploymentInfo struct {
	// DeploymentID is the Cloudflare deployment ID that was replaced.
	// +kubebuilder:validation:Required
	DeploymentID string `json:"deploymentId"`

	// URL was the production URL before promotion.
	// +kubebuilder:validation:Optional
	URL string `json:"url,omitempty"`

	// HashURL is the unique hash-based URL for the previous deployment.
	// +kubebuilder:validation:Optional
	HashURL string `json:"hashUrl,omitempty"`

	// ReplacedAt is when this deployment was replaced.
	// +kubebuilder:validation:Optional
	ReplacedAt *metav1.Time `json:"replacedAt,omitempty"`
}

// PagesPromotionSpec defines the desired state of PagesPromotion
type PagesPromotionSpec struct {
	// ProjectRef references the PagesProject.
	// Either Name or CloudflareID/CloudflareName must be specified.
	// +kubebuilder:validation:Required
	ProjectRef PagesProjectRef `json:"projectRef"`

	// DeploymentRef references the deployment to promote to production.
	// Either Name (K8s PagesDeployment) or DeploymentID (Cloudflare) must be specified.
	// +kubebuilder:validation:Required
	DeploymentRef PagesDeploymentRef `json:"deploymentRef"`

	// Cloudflare contains Cloudflare-specific configuration.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`

	// RequireSuccessfulDeployment requires the referenced PagesDeployment to be in Succeeded state.
	// Only applies when DeploymentRef.Name is specified.
	// When true (default), the promotion waits for the deployment to succeed.
	// When false, the deployment is promoted regardless of state.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	RequireSuccessfulDeployment *bool `json:"requireSuccessfulDeployment,omitempty"`
}

// PagesPromotionStatus defines the observed state of PagesPromotion
type PagesPromotionStatus struct {
	// State is the current state of the promotion.
	// +kubebuilder:validation:Optional
	State PagesPromotionState `json:"state,omitempty"`

	// ProjectName is the Cloudflare project name.
	// +kubebuilder:validation:Optional
	ProjectName string `json:"projectName,omitempty"`

	// AccountID is the Cloudflare account ID.
	// +kubebuilder:validation:Optional
	AccountID string `json:"accountId,omitempty"`

	// PromotedDeployment contains information about the current production deployment.
	// +kubebuilder:validation:Optional
	PromotedDeployment *PromotedDeploymentInfo `json:"promotedDeployment,omitempty"`

	// PreviousDeployment contains information about the deployment that was replaced.
	// Useful for tracking what was previously in production before this promotion.
	// +kubebuilder:validation:Optional
	PreviousDeployment *PreviousDeploymentInfo `json:"previousDeployment,omitempty"`

	// Conditions represent the latest available observations.
	// +kubebuilder:validation:Optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the last observed generation.
	// +kubebuilder:validation:Optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastPromotionTime is when the last successful promotion occurred.
	// +kubebuilder:validation:Optional
	LastPromotionTime *metav1.Time `json:"lastPromotionTime,omitempty"`

	// Message provides additional information about the current state.
	// +kubebuilder:validation:Optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cfppromo;pagespromotion
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.status.projectName`
// +kubebuilder:printcolumn:name="Deployment",type=string,JSONPath=`.spec.deploymentRef.name`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.promotedDeployment.url`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PagesPromotion manages the promotion of a Pages deployment to production.
// This resource separates the "upload" and "production switch" concerns, enabling
// GitOps-friendly workflows where preview deployments can be verified before promotion.
//
// The controller uses Cloudflare's rollback API to switch the production deployment.
// This is an atomic operation that makes the specified deployment the new production.
//
// Typical workflow:
// 1. Create a PagesDeployment with environment: preview
// 2. Verify the preview deployment
// 3. Create/update a PagesPromotion to promote to production
// 4. The controller waits for the deployment to succeed, then promotes it
type PagesPromotion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PagesPromotionSpec   `json:"spec,omitempty"`
	Status PagesPromotionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PagesPromotionList contains a list of PagesPromotion
type PagesPromotionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PagesPromotion `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PagesPromotion{}, &PagesPromotionList{})
}
