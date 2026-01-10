// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition types for Cloudflare resources
const (
	// ConditionTypeReady indicates whether the resource is ready
	ConditionTypeReady = "Ready"

	// ConditionTypeSynced indicates whether the resource is synced with Cloudflare
	ConditionTypeSynced = "Synced"

	// ConditionTypeDegraded indicates whether the resource is in a degraded state
	ConditionTypeDegraded = "Degraded"
)

// Condition reasons
const (
	// ReasonReconciling indicates the resource is being reconciled
	ReasonReconciling = "Reconciling"

	// ReasonReconciled indicates the resource was successfully reconciled
	ReasonReconciled = "Reconciled"

	// ReasonFailed indicates the reconciliation failed
	ReasonFailed = "Failed"

	// ReasonNotFound indicates a referenced resource was not found
	ReasonNotFound = "NotFound"

	// ReasonInvalidConfig indicates the configuration is invalid
	ReasonInvalidConfig = "InvalidConfig"

	// ReasonAPIError indicates an error from the Cloudflare API
	ReasonAPIError = "APIError"
)

// SecretRef references a Secret in a specific namespace.
type SecretRef struct {
	// Name of the Secret.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the Secret. If empty, defaults to the namespace of the resource.
	// +kubebuilder:validation:Optional
	Namespace string `json:"namespace,omitempty"`
}

// SecretKeyRef references a key within a Secret.
type SecretKeyRef struct {
	// Name of the Secret.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Key within the Secret.
	// +kubebuilder:validation:Required
	Key string `json:"key"`

	// Namespace of the Secret. If empty, defaults to the namespace of the resource.
	// +kubebuilder:validation:Optional
	Namespace string `json:"namespace,omitempty"`
}

// CommonStatus contains common status fields for all Cloudflare resources.
type CommonStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// +kubebuilder:validation:Optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	// +kubebuilder:validation:Optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastReconcileTime is the last time the resource was reconciled.
	// +kubebuilder:validation:Optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`
}

// TunnelReference references a Tunnel or ClusterTunnel resource.
type TunnelReference struct {
	// Kind of the tunnel resource (Tunnel or ClusterTunnel).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Tunnel;ClusterTunnel
	// +kubebuilder:default=ClusterTunnel
	Kind string `json:"kind"`

	// Name of the Tunnel or ClusterTunnel resource.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the Tunnel resource. Only applicable when Kind is Tunnel.
	// If empty, defaults to the namespace of the referencing resource.
	// +kubebuilder:validation:Optional
	Namespace string `json:"namespace,omitempty"`
}

// VirtualNetworkReference references a VirtualNetwork resource.
type VirtualNetworkReference struct {
	// Name of the VirtualNetwork resource.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// ServiceReference references a Kubernetes Service.
type ServiceReference struct {
	// Name of the Service.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the Service. If empty, defaults to the namespace of the referencing resource.
	// +kubebuilder:validation:Optional
	Namespace string `json:"namespace,omitempty"`

	// Port of the Service to use.
	// +kubebuilder:validation:Optional
	Port *int32 `json:"port,omitempty"`
}

// SetCondition sets or updates a condition on the CommonStatus.
func (s *CommonStatus) SetCondition(condition metav1.Condition) {
	for i, existing := range s.Conditions {
		if existing.Type == condition.Type {
			// Update existing condition if changed
			if existing.Status != condition.Status ||
				existing.Reason != condition.Reason ||
				existing.Message != condition.Message {
				s.Conditions[i] = condition
			}
			return
		}
	}
	// Add new condition
	s.Conditions = append(s.Conditions, condition)
}

// GetCondition returns the condition with the given type, or nil if not found.
func (s *CommonStatus) GetCondition(conditionType string) *metav1.Condition {
	for i := range s.Conditions {
		if s.Conditions[i].Type == conditionType {
			return &s.Conditions[i]
		}
	}
	return nil
}

// IsReady returns true if the Ready condition is True.
func (s *CommonStatus) IsReady() bool {
	cond := s.GetCondition(ConditionTypeReady)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

// NewCondition creates a new Condition with the given parameters.
func NewCondition(conditionType string, status metav1.ConditionStatus, reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// ReadyCondition creates a Ready condition.
func ReadyCondition(status metav1.ConditionStatus, reason, message string) metav1.Condition {
	return NewCondition(ConditionTypeReady, status, reason, message)
}

// SyncedCondition creates a Synced condition.
func SyncedCondition(status metav1.ConditionStatus, reason, message string) metav1.Condition {
	return NewCondition(ConditionTypeSynced, status, reason, message)
}
