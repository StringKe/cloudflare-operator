// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// R2BucketState represents the state of the R2 bucket
// +kubebuilder:validation:Enum=Pending;Creating;Ready;Deleting;Error
type R2BucketState string

const (
	// R2BucketStatePending means the bucket is waiting to be created
	R2BucketStatePending R2BucketState = "Pending"
	// R2BucketStateCreating means the bucket is being created
	R2BucketStateCreating R2BucketState = "Creating"
	// R2BucketStateReady means the bucket is created and ready
	R2BucketStateReady R2BucketState = "Ready"
	// R2BucketStateDeleting means the bucket is being deleted
	R2BucketStateDeleting R2BucketState = "Deleting"
	// R2BucketStateError means there was an error with the bucket
	R2BucketStateError R2BucketState = "Error"
)

// R2LocationHint specifies the location hint for the bucket
// +kubebuilder:validation:Enum=apac;eeur;enam;weur;wnam
type R2LocationHint string

const (
	// R2LocationAPAC is Asia-Pacific
	R2LocationAPAC R2LocationHint = "apac"
	// R2LocationEEUR is Eastern Europe
	R2LocationEEUR R2LocationHint = "eeur"
	// R2LocationENAM is Eastern North America
	R2LocationENAM R2LocationHint = "enam"
	// R2LocationWEUR is Western Europe
	R2LocationWEUR R2LocationHint = "weur"
	// R2LocationWNAM is Western North America
	R2LocationWNAM R2LocationHint = "wnam"
)

// R2CORSRule defines a CORS rule for the bucket
type R2CORSRule struct {
	// ID is an optional identifier for the rule
	// +kubebuilder:validation:Optional
	ID string `json:"id,omitempty"`

	// AllowedOrigins is a list of origins that are allowed
	// Use "*" to allow all origins
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	AllowedOrigins []string `json:"allowedOrigins"`

	// AllowedMethods is a list of HTTP methods that are allowed
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	AllowedMethods []string `json:"allowedMethods"`

	// AllowedHeaders is a list of headers that are allowed in requests
	// +kubebuilder:validation:Optional
	AllowedHeaders []string `json:"allowedHeaders,omitempty"`

	// ExposeHeaders is a list of headers that can be exposed to the browser
	// +kubebuilder:validation:Optional
	ExposeHeaders []string `json:"exposeHeaders,omitempty"`

	// MaxAgeSeconds is the number of seconds the browser can cache the preflight response
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	MaxAgeSeconds *int `json:"maxAgeSeconds,omitempty"`
}

// R2LifecycleRule defines a lifecycle rule for the bucket
type R2LifecycleRule struct {
	// ID is a unique identifier for the rule
	// +kubebuilder:validation:Required
	ID string `json:"id"`

	// Enabled indicates if this rule is active
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Prefix limits the rule to objects with this key prefix
	// +kubebuilder:validation:Optional
	Prefix string `json:"prefix,omitempty"`

	// Expiration defines when objects should be deleted
	// +kubebuilder:validation:Optional
	Expiration *R2LifecycleExpiration `json:"expiration,omitempty"`

	// AbortIncompleteMultipartUpload defines when to abort incomplete multipart uploads
	// +kubebuilder:validation:Optional
	AbortIncompleteMultipartUpload *R2LifecycleAbortUpload `json:"abortIncompleteMultipartUpload,omitempty"`
}

// R2LifecycleExpiration defines expiration settings
type R2LifecycleExpiration struct {
	// Days is the number of days after object creation when the object expires
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	Days *int `json:"days,omitempty"`

	// Date is the specific date when objects expire (ISO 8601 format)
	// +kubebuilder:validation:Optional
	Date string `json:"date,omitempty"`
}

// R2LifecycleAbortUpload defines abort incomplete upload settings
type R2LifecycleAbortUpload struct {
	// DaysAfterInitiation is the number of days after which incomplete uploads are aborted
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	DaysAfterInitiation int `json:"daysAfterInitiation"`
}

// R2BucketSpec defines the desired state of R2Bucket
type R2BucketSpec struct {
	// Name is the name of the R2 bucket in Cloudflare
	// If not specified, defaults to the Kubernetes resource name
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`
	Name string `json:"name,omitempty"`

	// LocationHint specifies the preferred location for the bucket
	// Cloudflare will attempt to place the bucket in this location,
	// but may use a different location if unavailable
	// +kubebuilder:validation:Optional
	LocationHint R2LocationHint `json:"locationHint,omitempty"`

	// CORS defines the Cross-Origin Resource Sharing rules for the bucket
	// +kubebuilder:validation:Optional
	CORS []R2CORSRule `json:"cors,omitempty"`

	// Lifecycle defines the object lifecycle rules for the bucket
	// +kubebuilder:validation:Optional
	Lifecycle []R2LifecycleRule `json:"lifecycle,omitempty"`

	// CredentialsRef references a CloudflareCredentials resource
	// If not specified, the default CloudflareCredentials will be used
	// +kubebuilder:validation:Optional
	CredentialsRef *CredentialsReference `json:"credentialsRef,omitempty"`

	// DeletionPolicy specifies what happens when the Kubernetes resource is deleted
	// Delete: The R2 bucket will be deleted from Cloudflare
	// Orphan: The R2 bucket will be left in Cloudflare
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=Delete;Orphan
	// +kubebuilder:default=Delete
	DeletionPolicy string `json:"deletionPolicy,omitempty"`
}

// R2BucketStatus defines the observed state of R2Bucket
type R2BucketStatus struct {
	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// State represents the current state of the bucket
	// +optional
	State R2BucketState `json:"state,omitempty"`

	// BucketName is the actual name of the bucket in Cloudflare
	// +optional
	BucketName string `json:"bucketName,omitempty"`

	// Location is the actual location where the bucket was created
	// +optional
	Location string `json:"location,omitempty"`

	// CreatedAt is the time the bucket was created in Cloudflare
	// +optional
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// CORSRulesCount is the number of CORS rules configured
	// +optional
	CORSRulesCount int `json:"corsRulesCount,omitempty"`

	// LifecycleRulesCount is the number of lifecycle rules configured
	// +optional
	LifecycleRulesCount int `json:"lifecycleRulesCount,omitempty"`

	// Message provides additional information about the current state
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cfr2;r2bucket
// +kubebuilder:printcolumn:name="Bucket",type=string,JSONPath=`.status.bucketName`
// +kubebuilder:printcolumn:name="Location",type=string,JSONPath=`.status.location`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// R2Bucket manages a Cloudflare R2 storage bucket.
// R2 is Cloudflare's S3-compatible object storage service.
//
// The controller creates and manages R2 buckets in your Cloudflare account.
type R2Bucket struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   R2BucketSpec   `json:"spec,omitempty"`
	Status R2BucketStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// R2BucketList contains a list of R2Bucket
type R2BucketList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []R2Bucket `json:"items"`
}

func init() {
	SchemeBuilder.Register(&R2Bucket{}, &R2BucketList{})
}
