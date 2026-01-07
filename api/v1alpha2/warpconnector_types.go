/*
Copyright 2025 Adyanth H.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WARPConnectorSpec defines the desired state of WARPConnector
type WARPConnectorSpec struct {
	// Name of the WARP Connector in Cloudflare.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name,omitempty"`

	// Replicas is the number of connector instances.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	Replicas int32 `json:"replicas,omitempty"`

	// Image is the WARP connector container image.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="cloudflare/cloudflared:latest"
	Image string `json:"image,omitempty"`

	// VirtualNetworkRef references the VirtualNetwork for this connector.
	// +kubebuilder:validation:Optional
	VirtualNetworkRef *VirtualNetworkRef `json:"virtualNetworkRef,omitempty"`

	// Routes are the private network routes to advertise.
	// +kubebuilder:validation:Optional
	Routes []WARPConnectorRoute `json:"routes,omitempty"`

	// Resources defines compute resources.
	// +kubebuilder:validation:Optional
	Resources *ResourceRequirements `json:"resources,omitempty"`

	// NodeSelector for pod scheduling.
	// +kubebuilder:validation:Optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations for pod scheduling.
	// +kubebuilder:validation:Optional
	Tolerations []Toleration `json:"tolerations,omitempty"`

	// ServiceAccount to use for the connector pods.
	// +kubebuilder:validation:Optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Cloudflare contains the Cloudflare API credentials.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`
}

// WARPConnectorRoute defines a route to advertise.
type WARPConnectorRoute struct {
	// Network is the CIDR of the network to route.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([0-9]{1,3}\.){3}[0-9]{1,3}/[0-9]{1,2}$`
	Network string `json:"network"`

	// Comment is an optional description.
	// +kubebuilder:validation:Optional
	Comment string `json:"comment,omitempty"`
}

// ResourceRequirements describes compute resources.
type ResourceRequirements struct {
	// Limits describes max allowed resources.
	// +kubebuilder:validation:Optional
	Limits map[string]string `json:"limits,omitempty"`

	// Requests describes minimum required resources.
	// +kubebuilder:validation:Optional
	Requests map[string]string `json:"requests,omitempty"`
}

// Toleration for pod scheduling.
type Toleration struct {
	// Key is the taint key.
	// +kubebuilder:validation:Optional
	Key string `json:"key,omitempty"`

	// Operator represents the relationship.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=Exists;Equal
	Operator string `json:"operator,omitempty"`

	// Value is the taint value.
	// +kubebuilder:validation:Optional
	Value string `json:"value,omitempty"`

	// Effect indicates the taint effect.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=NoSchedule;PreferNoSchedule;NoExecute
	Effect string `json:"effect,omitempty"`

	// TolerationSeconds for NoExecute effect.
	// +kubebuilder:validation:Optional
	TolerationSeconds *int64 `json:"tolerationSeconds,omitempty"`
}

// WARPConnectorStatus defines the observed state
type WARPConnectorStatus struct {
	// ConnectorID is the Cloudflare WARP Connector ID.
	// +kubebuilder:validation:Optional
	ConnectorID string `json:"connectorId,omitempty"`

	// TunnelID is the underlying tunnel ID.
	// +kubebuilder:validation:Optional
	TunnelID string `json:"tunnelId,omitempty"`

	// AccountID is the Cloudflare Account ID.
	// +kubebuilder:validation:Optional
	AccountID string `json:"accountId,omitempty"`

	// ReadyReplicas is the number of ready connector pods.
	// +kubebuilder:validation:Optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// RoutesConfigured is the number of routes configured.
	// +kubebuilder:validation:Optional
	RoutesConfigured int `json:"routesConfigured,omitempty"`

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
// +kubebuilder:resource:shortName=warpconn
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Routes",type=integer,JSONPath=`.status.routesConfigured`
// +kubebuilder:printcolumn:name="ConnectorID",type=string,JSONPath=`.status.connectorId`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// WARPConnector is the Schema for the warpconnectors API.
type WARPConnector struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WARPConnectorSpec   `json:"spec,omitempty"`
	Status WARPConnectorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WARPConnectorList contains a list of WARPConnector
type WARPConnectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WARPConnector `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WARPConnector{}, &WARPConnectorList{})
}

// GetConnectorName returns the name to use in Cloudflare.
func (w *WARPConnector) GetConnectorName() string {
	if w.Spec.Name != "" {
		return w.Spec.Name
	}
	return w.Name
}
