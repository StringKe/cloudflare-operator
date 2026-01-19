// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccessPolicySpec defines the desired state of AccessPolicy.
// This represents a reusable Access Policy in Cloudflare that can be
// attached to multiple Access Applications.
type AccessPolicySpec struct {
	// Name of the Access Policy in Cloudflare.
	// If not specified, the Kubernetes resource name will be used.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name,omitempty"`

	// Decision is the policy decision (allow, deny, bypass, non_identity).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=allow;deny;bypass;non_identity
	// +kubebuilder:default=allow
	Decision string `json:"decision"`

	// Precedence is the order in which policies are evaluated.
	// Lower numbers are evaluated first.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	Precedence int `json:"precedence,omitempty"`

	// Include defines the rules that must match (OR logic).
	// At least one include rule must match for the policy to apply.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Include []AccessGroupRule `json:"include"`

	// Exclude defines the rules that must NOT match (NOT logic).
	// If any exclude rule matches, the policy does not apply.
	// +kubebuilder:validation:Optional
	Exclude []AccessGroupRule `json:"exclude,omitempty"`

	// Require defines the rules that must ALL match (AND logic).
	// All require rules must match for the policy to apply.
	// +kubebuilder:validation:Optional
	Require []AccessGroupRule `json:"require,omitempty"`

	// SessionDuration overrides the application's session duration for this policy.
	// Format: Go duration string (e.g., "24h", "30m").
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^[0-9]+(h|m|s)+$`
	SessionDuration string `json:"sessionDuration,omitempty"`

	// IsolationRequired enables browser isolation for this policy.
	// When enabled, users must access the application through Cloudflare Browser Isolation.
	// +kubebuilder:validation:Optional
	IsolationRequired *bool `json:"isolationRequired,omitempty"`

	// PurposeJustificationRequired requires users to provide a justification for access.
	// +kubebuilder:validation:Optional
	PurposeJustificationRequired *bool `json:"purposeJustificationRequired,omitempty"`

	// PurposeJustificationPrompt is the custom prompt shown when justification is required.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=1024
	PurposeJustificationPrompt string `json:"purposeJustificationPrompt,omitempty"`

	// ApprovalRequired requires admin approval for access.
	// When enabled, users must request and receive approval before accessing.
	// +kubebuilder:validation:Optional
	ApprovalRequired *bool `json:"approvalRequired,omitempty"`

	// ApprovalGroups defines the groups that can approve access requests.
	// Required when ApprovalRequired is true.
	// +kubebuilder:validation:Optional
	ApprovalGroups []ApprovalGroup `json:"approvalGroups,omitempty"`

	// Cloudflare contains the Cloudflare API credentials and account information.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`
}

// ApprovalGroup defines an approval group configuration for access requests.
type ApprovalGroup struct {
	// EmailAddresses is a list of email addresses that can approve access requests.
	// +kubebuilder:validation:Optional
	EmailAddresses []string `json:"emailAddresses,omitempty"`

	// EmailListUUID is the UUID of an email list that can approve access requests.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
	EmailListUUID string `json:"emailListUuid,omitempty"`

	// ApprovalsNeeded is the number of approvals required from this group.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	ApprovalsNeeded int `json:"approvalsNeeded,omitempty"`
}

// AccessPolicyStatus defines the observed state of AccessPolicy.
type AccessPolicyStatus struct {
	// PolicyID is the Cloudflare ID of the reusable Access Policy.
	// +kubebuilder:validation:Optional
	PolicyID string `json:"policyId,omitempty"`

	// AccountID is the Cloudflare Account ID.
	// +kubebuilder:validation:Optional
	AccountID string `json:"accountId,omitempty"`

	// State indicates the current state of the policy.
	// Possible values: pending, Ready, Error
	// +kubebuilder:validation:Optional
	State string `json:"state,omitempty"`

	// Conditions represent the latest available observations.
	// +kubebuilder:validation:Optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed.
	// +kubebuilder:validation:Optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=accesspol
// +kubebuilder:printcolumn:name="Decision",type=string,JSONPath=`.spec.decision`
// +kubebuilder:printcolumn:name="PolicyID",type=string,JSONPath=`.status.policyId`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AccessPolicy is the Schema for the accesspolicies API.
// An AccessPolicy represents a reusable Cloudflare Access Policy that can be
// attached to multiple Access Applications. This allows for centralized policy
// management and consistent access control across applications.
type AccessPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccessPolicySpec   `json:"spec,omitempty"`
	Status AccessPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AccessPolicyList contains a list of AccessPolicy
type AccessPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccessPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AccessPolicy{}, &AccessPolicyList{})
}

// GetAccessPolicyName returns the name to use in Cloudflare.
func (a *AccessPolicy) GetAccessPolicyName() string {
	if a.Spec.Name != "" {
		return a.Spec.Name
	}
	return a.Name
}
