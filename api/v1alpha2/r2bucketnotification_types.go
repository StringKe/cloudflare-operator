// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// R2NotificationState represents the state of the notification rule
// +kubebuilder:validation:Enum=Pending;Active;Error
type R2NotificationState string

const (
	// R2NotificationStatePending means the notification is waiting to be configured
	R2NotificationStatePending R2NotificationState = "Pending"
	// R2NotificationStateActive means the notification is active
	R2NotificationStateActive R2NotificationState = "Active"
	// R2NotificationStateError means there was an error configuring the notification
	R2NotificationStateError R2NotificationState = "Error"
)

// R2EventType represents the type of R2 event to notify on
// +kubebuilder:validation:Enum=object-create;object-delete
type R2EventType string

const (
	// R2EventTypeObjectCreate triggers on object creation
	R2EventTypeObjectCreate R2EventType = "object-create"
	// R2EventTypeObjectDelete triggers on object deletion
	R2EventTypeObjectDelete R2EventType = "object-delete"
)

// R2NotificationRule defines a notification rule
type R2NotificationRule struct {
	// EventTypes is the list of event types to notify on
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	EventTypes []R2EventType `json:"eventTypes"`

	// Prefix filters events to objects with keys starting with this prefix
	// +kubebuilder:validation:Optional
	Prefix string `json:"prefix,omitempty"`

	// Suffix filters events to objects with keys ending with this suffix
	// +kubebuilder:validation:Optional
	Suffix string `json:"suffix,omitempty"`

	// Description is a human-readable description of this rule
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`
}

// R2BucketNotificationSpec defines the desired state of R2BucketNotification
type R2BucketNotificationSpec struct {
	// BucketName is the name of the R2 bucket to configure notifications for
	// +kubebuilder:validation:Required
	BucketName string `json:"bucketName"`

	// QueueName is the name of the Cloudflare Queue to send notifications to
	// +kubebuilder:validation:Required
	QueueName string `json:"queueName"`

	// Rules defines the notification rules
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Rules []R2NotificationRule `json:"rules"`

	// CredentialsRef references a CloudflareCredentials resource
	// If not specified, the default CloudflareCredentials will be used
	// +kubebuilder:validation:Optional
	CredentialsRef *CredentialsReference `json:"credentialsRef,omitempty"`
}

// R2BucketNotificationStatus defines the observed state of R2BucketNotification
type R2BucketNotificationStatus struct {
	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// State represents the current state of the notification
	// +optional
	State R2NotificationState `json:"state,omitempty"`

	// Message provides additional information about the current state
	// +optional
	Message string `json:"message,omitempty"`

	// QueueID is the Cloudflare Queue ID
	// +optional
	QueueID string `json:"queueId,omitempty"`

	// RuleCount is the number of notification rules configured
	// +optional
	RuleCount int `json:"ruleCount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cfr2n;r2notify
// +kubebuilder:printcolumn:name="Bucket",type=string,JSONPath=`.spec.bucketName`
// +kubebuilder:printcolumn:name="Queue",type=string,JSONPath=`.spec.queueName`
// +kubebuilder:printcolumn:name="Rules",type=integer,JSONPath=`.status.ruleCount`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// R2BucketNotification configures event notifications for an R2 bucket.
// Events are sent to a Cloudflare Queue when objects are created or deleted.
type R2BucketNotification struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   R2BucketNotificationSpec   `json:"spec,omitempty"`
	Status R2BucketNotificationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// R2BucketNotificationList contains a list of R2BucketNotification
type R2BucketNotificationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []R2BucketNotification `json:"items"`
}

func init() {
	SchemeBuilder.Register(&R2BucketNotification{}, &R2BucketNotificationList{})
}
