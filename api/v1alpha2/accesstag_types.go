// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccessTagSpec defines the desired state of AccessTag.
type AccessTagSpec struct {
	// Name is the Cloudflare Access tag. Cloudflare Access tags are account-level
	// objects that must exist before an AccessApplication can reference them via
	// spec.tags. This value must match the tag string used in those references.
	// The tag name is the resource's identity and cannot be changed once created;
	// rename by creating a new AccessTag.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Cloudflare contains the Cloudflare API credentials and account information.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`
}

// AccessTagStatus defines the observed state of AccessTag.
type AccessTagStatus struct {
	// AccountID is the Cloudflare Account ID the tag was ensured in.
	// +optional
	AccountID string `json:"accountId,omitempty"`

	// TagName is the Cloudflare-side tag name once it has been ensured to exist.
	// +optional
	TagName string `json:"tagName,omitempty"`

	// AppCount is the number of Access applications currently using this tag,
	// as last observed from Cloudflare.
	// +optional
	AppCount int `json:"appCount,omitempty"`

	// State indicates the current state of the tag.
	// +optional
	State string `json:"state,omitempty"`

	// Conditions represent the latest available observations of the tag's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed for this AccessTag.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=accesstag
// +kubebuilder:printcolumn:name="Tag",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Apps",type=integer,JSONPath=`.status.appCount`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AccessTag is the Schema for the accesstags API.
// An AccessTag ensures a Cloudflare Zero Trust Access tag exists in the account,
// so that AccessApplications can reference it via spec.tags.
type AccessTag struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccessTagSpec   `json:"spec,omitempty"`
	Status AccessTagStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AccessTagList contains a list of AccessTag.
type AccessTagList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccessTag `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AccessTag{}, &AccessTagList{})
}
