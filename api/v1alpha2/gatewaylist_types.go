// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GatewayListSpec defines the desired state of GatewayList
type GatewayListSpec struct {
	// Name of the Gateway List in Cloudflare.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name,omitempty"`

	// Description is a human-readable description.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=1000
	Description string `json:"description,omitempty"`

	// Type is the list type.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=SERIAL;URL;DOMAIN;EMAIL;IP
	Type string `json:"type"`

	// Items are the list entries.
	// +kubebuilder:validation:Optional
	Items []GatewayListItem `json:"items,omitempty"`

	// ItemsFromConfigMap references a ConfigMap containing list items.
	// +kubebuilder:validation:Optional
	ItemsFromConfigMap *ConfigMapKeyRef `json:"itemsFromConfigMap,omitempty"`

	// Cloudflare contains the Cloudflare API credentials.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`
}

// GatewayListItem represents a single list item.
type GatewayListItem struct {
	// Value is the list entry value.
	// +kubebuilder:validation:Required
	Value string `json:"value"`

	// Description is an optional description for this item.
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`
}

// ConfigMapKeyRef references a key in a ConfigMap.
type ConfigMapKeyRef struct {
	// Name is the ConfigMap name.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Key is the key in the ConfigMap.
	// +kubebuilder:validation:Required
	Key string `json:"key"`

	// Namespace is the ConfigMap namespace.
	// +kubebuilder:validation:Optional
	Namespace string `json:"namespace,omitempty"`
}

// GatewayListStatus defines the observed state
type GatewayListStatus struct {
	// ListID is the Cloudflare Gateway List ID.
	// +kubebuilder:validation:Optional
	ListID string `json:"listId,omitempty"`

	// AccountID is the Cloudflare Account ID.
	// +kubebuilder:validation:Optional
	AccountID string `json:"accountId,omitempty"`

	// ItemCount is the number of items in the list.
	// +kubebuilder:validation:Optional
	ItemCount int `json:"itemCount,omitempty"`

	// State indicates the current state.
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
// +kubebuilder:resource:scope=Cluster,shortName=gwlist
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Items",type=integer,JSONPath=`.status.itemCount`
// +kubebuilder:printcolumn:name="ListID",type=string,JSONPath=`.status.listId`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// GatewayList is the Schema for the gatewaylists API.
type GatewayList struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatewayListSpec   `json:"spec,omitempty"`
	Status GatewayListStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GatewayListList contains a list of GatewayList
type GatewayListList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GatewayList `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GatewayList{}, &GatewayListList{})
}

// GetGatewayListName returns the name to use in Cloudflare.
func (g *GatewayList) GetGatewayListName() string {
	if g.Spec.Name != "" {
		return g.Spec.Name
	}
	return g.Name
}
