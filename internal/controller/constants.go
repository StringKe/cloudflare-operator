// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package controller

// Operator namespace constant
// Cluster-scoped resources should use this namespace for loading secrets when using legacy inline secret references.
const (
	// OperatorNamespace is the namespace where the operator is deployed.
	// Used for cluster-scoped resources to locate secrets.
	OperatorNamespace = "cloudflare-operator-system"
)

// New cloudflare.com API Group constants
// These will be used for new CRDs (VirtualNetwork, NetworkRoute, etc.)
const (
	// Label and annotation prefixes for the new cloudflare.com API group
	LabelPrefix      = "cloudflare.com/"
	AnnotationPrefix = "cloudflare.com/"

	// Finalizer for cloudflare.com resources
	FinalizerCloudflare = "cloudflare.com/finalizer"

	// Legacy prefixes for backward compatibility with cloudflare-operator.io resources
	LegacyLabelPrefix      = "cloudflare-operator.io/"
	LegacyAnnotationPrefix = "cloudflare-operator.io/"
	LegacyFinalizer        = "cloudflare-operator.io/finalizer"
)

// VirtualNetwork controller constants
const (
	VirtualNetworkFinalizer = "cloudflare.com/virtualnetwork-finalizer"

	// Labels for VirtualNetwork
	LabelVirtualNetworkName = LabelPrefix + "virtualnetwork-name"
	LabelVirtualNetworkID   = LabelPrefix + "virtualnetwork-id"
)

// NetworkRoute controller constants
const (
	NetworkRouteFinalizer = "cloudflare.com/networkroute-finalizer"

	// Labels for NetworkRoute
	LabelNetworkRouteNetwork = LabelPrefix + "networkroute-network"
	LabelNetworkRouteTunnel  = LabelPrefix + "networkroute-tunnel"
)

// PrivateService controller constants
const (
	PrivateServiceFinalizer = "cloudflare.com/privateservice-finalizer"

	// Labels for PrivateService
	LabelPrivateServiceName = LabelPrefix + "privateservice-name"
)

// DeviceSettingsPolicy controller constants
const (
	DeviceSettingsPolicyFinalizer = "cloudflare.com/devicesettingspolicy-finalizer"

	// Labels for DeviceSettingsPolicy
	LabelDeviceSettingsPolicyName = LabelPrefix + "devicesettingspolicy-name"
)

// Annotations used across controllers
const (
	// AnnotationLastAppliedConfig stores the last applied configuration for drift detection
	AnnotationLastAppliedConfig = AnnotationPrefix + "last-applied-configuration"

	// AnnotationManagedBy indicates the controller managing the resource
	AnnotationManagedBy = AnnotationPrefix + "managed-by"

	// AnnotationManagedByValue is the value for AnnotationManagedBy
	AnnotationManagedByValue = "cloudflare-operator"
)

// Controller names for logging and events
const (
	ControllerNameVirtualNetwork       = "VirtualNetwork"
	ControllerNameNetworkRoute         = "NetworkRoute"
	ControllerNamePrivateService       = "PrivateService"
	ControllerNameDeviceSettingsPolicy = "DeviceSettingsPolicy"
)

// Event reasons
const (
	// Success events
	EventReasonCreated          = "Created"
	EventReasonUpdated          = "Updated"
	EventReasonDeleted          = "Deleted"
	EventReasonSynced           = "Synced"
	EventReasonReconciled       = "Reconciled"
	EventReasonFinalizerSet     = "FinalizerSet"
	EventReasonFinalizerRemoved = "FinalizerRemoved"
	EventReasonAdopted          = "Adopted"

	// Failure events
	EventReasonCreateFailed     = "CreateFailed"
	EventReasonUpdateFailed     = "UpdateFailed"
	EventReasonDeleteFailed     = "DeleteFailed"
	EventReasonSyncFailed       = "SyncFailed"
	EventReasonReconcileFailed  = "ReconcileFailed"
	EventReasonAPIError         = "APIError"
	EventReasonInvalidConfig    = "InvalidConfig"
	EventReasonDependencyError  = "DependencyError"
	EventReasonAdoptionConflict = "AdoptionConflict"
)

// Management tracking constants
// These are used to track which K8s resource manages a Cloudflare resource,
// preventing adoption race conditions where multiple K8s resources try to
// manage the same Cloudflare resource.
const (
	// ManagementMarkerPrefix is the prefix for management markers in comments
	// Format: [managed:kind/namespace/name] or [managed:kind/name] for cluster-scoped
	ManagementMarkerPrefix = "[managed:"
	ManagementMarkerSuffix = "]"
)
