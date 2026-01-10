// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="TunnelID",type=string,JSONPath=`.status.tunnelId`
// +kubebuilder:deprecatedversion:warning="networking.cloudflare-operator.io/v1alpha1 ClusterTunnel is deprecated, see https://github.com/StringKe/cloudflare-operator/tree/v0.13.0/docs/migration/crd/v1alpha2.md for migrating to v1alpha2"

// ClusterTunnel is the Schema for the clustertunnels API
type ClusterTunnel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TunnelSpec   `json:"spec,omitempty"`
	Status TunnelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterTunnelList contains a list of ClusterTunnel
type ClusterTunnelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterTunnel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterTunnel{}, &ClusterTunnelList{})
}
