// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// SyncResourceType defines the type of Cloudflare resource being synced
// +kubebuilder:validation:Enum=TunnelConfiguration;TunnelLifecycle;DNSRecord;AccessApplication;AccessGroup;AccessPolicy;AccessServiceToken;AccessIdentityProvider;VirtualNetwork;NetworkRoute;PrivateService;R2Bucket;R2BucketDomain;R2BucketNotification;ZoneRuleset;TransformRule;RedirectRule;GatewayRule;GatewayList;GatewayConfiguration;OriginCACertificate;CloudflareDomain;DomainRegistration;DevicePostureRule;DeviceSettingsPolicy;WARPConnector;PagesProject;PagesDomain;PagesDeployment
type SyncResourceType string

const (
	// SyncResourceTunnelConfiguration represents Cloudflare Tunnel ingress configuration
	SyncResourceTunnelConfiguration SyncResourceType = "TunnelConfiguration"
	// SyncResourceTunnelLifecycle represents Cloudflare Tunnel lifecycle (create/delete) operations
	SyncResourceTunnelLifecycle SyncResourceType = "TunnelLifecycle"
	// SyncResourceDNSRecord represents a Cloudflare DNS record
	SyncResourceDNSRecord SyncResourceType = "DNSRecord"
	// SyncResourceAccessApplication represents a Cloudflare Access application
	SyncResourceAccessApplication SyncResourceType = "AccessApplication"
	// SyncResourceAccessGroup represents a Cloudflare Access group
	SyncResourceAccessGroup SyncResourceType = "AccessGroup"
	// SyncResourceAccessPolicy represents a Cloudflare reusable Access policy
	SyncResourceAccessPolicy SyncResourceType = "AccessPolicy"
	// SyncResourceAccessServiceToken represents a Cloudflare Access service token
	SyncResourceAccessServiceToken SyncResourceType = "AccessServiceToken"
	// SyncResourceAccessIdentityProvider represents a Cloudflare Access identity provider
	SyncResourceAccessIdentityProvider SyncResourceType = "AccessIdentityProvider"
	// SyncResourceVirtualNetwork represents a Cloudflare virtual network
	SyncResourceVirtualNetwork SyncResourceType = "VirtualNetwork"
	// SyncResourceNetworkRoute represents a Cloudflare network route
	SyncResourceNetworkRoute SyncResourceType = "NetworkRoute"
	// SyncResourcePrivateService represents a Cloudflare tunnel route for a K8s service
	SyncResourcePrivateService SyncResourceType = "PrivateService"
	// SyncResourceR2Bucket represents a Cloudflare R2 bucket
	SyncResourceR2Bucket SyncResourceType = "R2Bucket"
	// SyncResourceR2BucketDomain represents a Cloudflare R2 bucket custom domain
	SyncResourceR2BucketDomain SyncResourceType = "R2BucketDomain"
	// SyncResourceR2BucketNotification represents a Cloudflare R2 bucket notification
	SyncResourceR2BucketNotification SyncResourceType = "R2BucketNotification"
	// SyncResourceZoneRuleset represents a Cloudflare zone ruleset
	SyncResourceZoneRuleset SyncResourceType = "ZoneRuleset"
	// SyncResourceTransformRule represents a Cloudflare transform rule
	SyncResourceTransformRule SyncResourceType = "TransformRule"
	// SyncResourceRedirectRule represents a Cloudflare redirect rule
	SyncResourceRedirectRule SyncResourceType = "RedirectRule"
	// SyncResourceGatewayRule represents a Cloudflare Gateway rule
	SyncResourceGatewayRule SyncResourceType = "GatewayRule"
	// SyncResourceGatewayList represents a Cloudflare Gateway list
	SyncResourceGatewayList SyncResourceType = "GatewayList"
	// SyncResourceGatewayConfiguration represents a Cloudflare Gateway configuration
	SyncResourceGatewayConfiguration SyncResourceType = "GatewayConfiguration"
	// SyncResourceOriginCACertificate represents a Cloudflare Origin CA certificate
	SyncResourceOriginCACertificate SyncResourceType = "OriginCACertificate"
	// SyncResourceCloudflareDomain represents a Cloudflare domain/zone configuration
	SyncResourceCloudflareDomain SyncResourceType = "CloudflareDomain"
	// SyncResourceDomainRegistration represents a Cloudflare domain registration
	SyncResourceDomainRegistration SyncResourceType = "DomainRegistration"
	// SyncResourceDevicePostureRule represents a Cloudflare device posture rule
	SyncResourceDevicePostureRule SyncResourceType = "DevicePostureRule"
	// SyncResourceDeviceSettingsPolicy represents a Cloudflare device settings policy
	SyncResourceDeviceSettingsPolicy SyncResourceType = "DeviceSettingsPolicy"
	// SyncResourceWARPConnector represents a Cloudflare WARP connector
	SyncResourceWARPConnector SyncResourceType = "WARPConnector"
	// SyncResourcePagesProject represents a Cloudflare Pages project
	SyncResourcePagesProject SyncResourceType = "PagesProject"
	// SyncResourcePagesDomain represents a Cloudflare Pages custom domain
	SyncResourcePagesDomain SyncResourceType = "PagesDomain"
	// SyncResourcePagesDeployment represents a Cloudflare Pages deployment
	SyncResourcePagesDeployment SyncResourceType = "PagesDeployment"
)

// SyncStatus represents the synchronization status
// +kubebuilder:validation:Enum=Pending;Syncing;Synced;Error
type SyncStatus string

const (
	// SyncStatusPending indicates the sync is pending
	SyncStatusPending SyncStatus = "Pending"
	// SyncStatusSyncing indicates the sync is in progress
	SyncStatusSyncing SyncStatus = "Syncing"
	// SyncStatusSynced indicates the sync completed successfully
	SyncStatusSynced SyncStatus = "Synced"
	// SyncStatusError indicates the sync failed
	SyncStatusError SyncStatus = "Error"
)

// SourceReference identifies a Kubernetes resource that contributes configuration
type SourceReference struct {
	// Kind is the resource kind (e.g., "Tunnel", "Ingress", "TunnelBinding")
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`

	// Namespace is the resource namespace (empty for cluster-scoped resources)
	// +kubebuilder:validation:Optional
	Namespace string `json:"namespace,omitempty"`

	// Name is the resource name
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// String returns the string representation of SourceReference
func (r SourceReference) String() string {
	if r.Namespace == "" {
		return r.Kind + "/" + r.Name
	}
	return r.Kind + "/" + r.Namespace + "/" + r.Name
}

// ConfigSource represents configuration contributed by a single Kubernetes resource
type ConfigSource struct {
	// Ref identifies the source Kubernetes resource
	// +kubebuilder:validation:Required
	Ref SourceReference `json:"ref"`

	// Config contains the configuration contributed by this source
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Required
	Config runtime.RawExtension `json:"config"`

	// Priority determines the order when resolving conflicts (lower number = higher priority)
	// Tunnel/ClusterTunnel settings use priority 10, rules use priority 100 by default
	// +kubebuilder:default=100
	Priority int `json:"priority,omitempty"`

	// LastUpdated is the timestamp when this source was last updated
	// +kubebuilder:validation:Optional
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`
}

// CloudflareSyncStateSpec defines the desired state of CloudflareSyncState
type CloudflareSyncStateSpec struct {
	// ResourceType is the type of Cloudflare resource being synced
	// +kubebuilder:validation:Required
	ResourceType SyncResourceType `json:"resourceType"`

	// CloudflareID is the Cloudflare resource identifier (e.g., tunnel ID, record ID)
	// +kubebuilder:validation:Required
	CloudflareID string `json:"cloudflareId"`

	// AccountID is the Cloudflare Account ID
	// +kubebuilder:validation:Required
	AccountID string `json:"accountId"`

	// ZoneID is the Cloudflare Zone ID (required for DNS and zone-level resources)
	// +kubebuilder:validation:Optional
	ZoneID string `json:"zoneId,omitempty"`

	// CredentialsRef references a CloudflareCredentials resource for API access
	// +kubebuilder:validation:Required
	CredentialsRef CredentialsReference `json:"credentialsRef"`

	// Sources contains configuration from each contributing Kubernetes resource
	// Multiple sources are aggregated by the Sync Controller before syncing to Cloudflare
	// +kubebuilder:validation:Optional
	Sources []ConfigSource `json:"sources,omitempty"`
}

// CloudflareSyncStateStatus defines the observed state of CloudflareSyncState
type CloudflareSyncStateStatus struct {
	// SyncStatus indicates the current synchronization state
	// +kubebuilder:validation:Optional
	SyncStatus SyncStatus `json:"syncStatus,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync to Cloudflare
	// +kubebuilder:validation:Optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// ConfigVersion is the Cloudflare configuration version after the last sync
	// +kubebuilder:validation:Optional
	ConfigVersion int `json:"configVersion,omitempty"`

	// ConfigHash is the SHA256 hash of the aggregated configuration
	// Used for incremental sync detection - sync is skipped if hash is unchanged
	// +kubebuilder:validation:Optional
	ConfigHash string `json:"configHash,omitempty"`

	// AggregatedConfig contains the merged configuration from all sources
	// Stored for debugging and observability purposes
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Optional
	AggregatedConfig *runtime.RawExtension `json:"aggregatedConfig,omitempty"`

	// Error contains the last error message if SyncStatus is Error
	// +kubebuilder:validation:Optional
	Error string `json:"error,omitempty"`

	// ResultData contains resource-specific output data from the sync operation
	// For TunnelLifecycle: tunnelId, tunnelToken, credentials (base64)
	// For OriginCACertificate: certificate (PEM)
	// +kubebuilder:validation:Optional
	ResultData map[string]string `json:"resultData,omitempty"`

	// Conditions represent the latest available observations
	// +kubebuilder:validation:Optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cfss;syncstate
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.resourceType`
// +kubebuilder:printcolumn:name="CloudflareID",type=string,JSONPath=`.spec.cloudflareId`,priority=1
// +kubebuilder:printcolumn:name="Sources",type=integer,JSONPath=`.spec.sources`,priority=1
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.syncStatus`
// +kubebuilder:printcolumn:name="Version",type=integer,JSONPath=`.status.configVersion`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// CloudflareSyncState stores the aggregated configuration state for a Cloudflare resource.
// It acts as a shared state store between multiple K8s resource controllers and the
// Sync Controller that performs the actual Cloudflare API synchronization.
//
// This resource is managed internally by the operator and should not be created or
// modified by users directly. It enables:
// - Multi-controller coordination without race conditions
// - Multi-instance operator deployments with K8s optimistic locking
// - Efficient batching and deduplication of API calls
// - Observable sync state via kubectl
type CloudflareSyncState struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudflareSyncStateSpec   `json:"spec,omitempty"`
	Status CloudflareSyncStateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudflareSyncStateList contains a list of CloudflareSyncState
type CloudflareSyncStateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudflareSyncState `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudflareSyncState{}, &CloudflareSyncStateList{})
}
