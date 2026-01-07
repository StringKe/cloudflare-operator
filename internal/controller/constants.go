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

package controller

// New cloudflare.com API Group constants
// These will be used for new CRDs (VirtualNetwork, NetworkRoute, etc.)
const (
	// Label and annotation prefixes for the new cloudflare.com API group
	LabelPrefix      = "cloudflare.com/"
	AnnotationPrefix = "cloudflare.com/"

	// Finalizer for cloudflare.com resources
	FinalizerCloudflare = "cloudflare.com/finalizer"

	// Legacy prefixes for backward compatibility with cfargotunnel.com resources
	LegacyLabelPrefix      = "cfargotunnel.com/"
	LegacyAnnotationPrefix = "cfargotunnel.com/"
	LegacyFinalizer        = "cfargotunnel.com/finalizer"
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

	// Failure events
	EventReasonCreateFailed    = "CreateFailed"
	EventReasonUpdateFailed    = "UpdateFailed"
	EventReasonDeleteFailed    = "DeleteFailed"
	EventReasonSyncFailed      = "SyncFailed"
	EventReasonReconcileFailed = "ReconcileFailed"
	EventReasonAPIError        = "APIError"
	EventReasonInvalidConfig   = "InvalidConfig"
	EventReasonDependencyError = "DependencyError"
)
