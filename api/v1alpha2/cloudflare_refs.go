// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

// ============================================================================
// Unified Reference Types
// ============================================================================
// These types provide a consistent way to reference Cloudflare resources.
// All reference types support three modes:
// 1. K8s resource name - References a K8s CRD that manages the Cloudflare resource
// 2. cloudflareId - Direct UUID reference to an existing Cloudflare resource
// 3. cloudflareName - Display name lookup via Cloudflare API
//
// CEL validation ensures exactly one of the three fields is set.

// AccessIdentityProviderRefV2 references an AccessIdentityProvider.
// Supports K8s name, Cloudflare UUID, or Cloudflare display name.
//
// +kubebuilder:validation:XValidation:rule="(has(self.name) ? 1 : 0) + (has(self.cloudflareId) ? 1 : 0) + (has(self.cloudflareName) ? 1 : 0) == 1",message="exactly one of name, cloudflareId, or cloudflareName must be set"
type AccessIdentityProviderRefV2 struct {
	// Name is the K8s AccessIdentityProvider resource name.
	// The controller will look up the CRD and use its status.providerID.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name,omitempty"`

	// CloudflareID is the Cloudflare IdP UUID.
	// Use this to directly reference a Cloudflare-managed IdP
	// without creating a corresponding K8s AccessIdentityProvider resource.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
	CloudflareID string `json:"cloudflareId,omitempty"`

	// CloudflareName is the display name of the IdP in Cloudflare.
	// The controller will resolve this name to an ID via the Cloudflare API.
	// Use this when you want to reference an IdP by name
	// (e.g., IdPs created via Terraform or the Cloudflare dashboard).
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	CloudflareName string `json:"cloudflareName,omitempty"`
}

// ReusableGroupRef references an AccessGroup.
// Supports K8s name, Cloudflare UUID, or Cloudflare display name.
//
// +kubebuilder:validation:XValidation:rule="(has(self.name) ? 1 : 0) + (has(self.cloudflareId) ? 1 : 0) + (has(self.cloudflareName) ? 1 : 0) == 1",message="exactly one of name, cloudflareId, or cloudflareName must be set"
type ReusableGroupRef struct {
	// Name is the K8s AccessGroup resource name.
	// The controller will look up the CRD and use its status.groupId.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name,omitempty"`

	// CloudflareID is the Cloudflare Group UUID.
	// Use this to directly reference a Cloudflare-managed group
	// without creating a corresponding K8s AccessGroup resource.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
	CloudflareID string `json:"cloudflareId,omitempty"`

	// CloudflareName is the display name of the group in Cloudflare.
	// The controller will resolve this name to an ID via the Cloudflare API.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	CloudflareName string `json:"cloudflareName,omitempty"`
}

// VirtualNetworkRef references a VirtualNetwork.
// Supports K8s name, Cloudflare UUID, or Cloudflare display name.
//
// +kubebuilder:validation:XValidation:rule="(has(self.name) ? 1 : 0) + (has(self.cloudflareId) ? 1 : 0) + (has(self.cloudflareName) ? 1 : 0) == 1",message="exactly one of name, cloudflareId, or cloudflareName must be set"
type VirtualNetworkRef struct {
	// Name is the K8s VirtualNetwork resource name.
	// The controller will look up the CRD and use its status.virtualNetworkId.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name,omitempty"`

	// CloudflareID is the Cloudflare VNet UUID.
	// Use this to directly reference a Cloudflare-managed VNet
	// without creating a corresponding K8s VirtualNetwork resource.
	// +kubebuilder:validation:Optional
	CloudflareID string `json:"cloudflareId,omitempty"`

	// CloudflareName is the display name of the VNet in Cloudflare.
	// The controller will resolve this name to an ID via the Cloudflare API.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	CloudflareName string `json:"cloudflareName,omitempty"`
}
