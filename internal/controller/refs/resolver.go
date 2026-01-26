// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package refs provides unified reference resolution for Cloudflare resources.
// It supports resolving references by K8s name, Cloudflare UUID, or Cloudflare display name.
package refs

import (
	"context"
	"errors"
	"fmt"

	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

// Resolver resolves Cloudflare resource references.
// It supports three resolution modes:
// 1. K8s CRD name - looks up the CRD and extracts the Cloudflare ID from status
// 2. Direct Cloudflare ID - uses the provided UUID directly
// 3. Cloudflare display name - queries the Cloudflare API to resolve name to ID
type Resolver struct {
	client client.Client
	api    *cf.API
}

// NewResolver creates a new reference resolver.
func NewResolver(c client.Client, api *cf.API) *Resolver {
	return &Resolver{
		client: c,
		api:    api,
	}
}

// ResolveIdentityProvider resolves an AccessIdentityProviderRefV2 to a Cloudflare IdP ID.
// Resolution priority: cloudflareId > name > cloudflareName
//
//nolint:revive // cognitive complexity is acceptable for this linear resolution logic
func (r *Resolver) ResolveIdentityProvider(ctx context.Context, ref *networkingv1alpha2.AccessIdentityProviderRefV2) (string, error) {
	if ref == nil {
		return "", errors.New("nil identity provider reference")
	}

	// Priority 1: Direct Cloudflare ID
	if ref.CloudflareID != "" {
		return ref.CloudflareID, nil
	}

	// Priority 2: K8s AccessIdentityProvider name
	if ref.Name != "" {
		idp := &networkingv1alpha2.AccessIdentityProvider{}
		if err := r.client.Get(ctx, apitypes.NamespacedName{Name: ref.Name}, idp); err != nil {
			return "", fmt.Errorf("AccessIdentityProvider %q not found: %w", ref.Name, err)
		}
		if idp.Status.ProviderID == "" {
			return "", fmt.Errorf("AccessIdentityProvider %q not ready (no ProviderID in status)", ref.Name)
		}
		return idp.Status.ProviderID, nil
	}

	// Priority 3: Cloudflare display name lookup
	if ref.CloudflareName != "" {
		result, err := r.api.GetAccessIdentityProviderByName(ctx, ref.CloudflareName)
		if err != nil {
			return "", fmt.Errorf("failed to find IdP by name %q: %w", ref.CloudflareName, err)
		}
		if result == nil {
			return "", fmt.Errorf("IdP %q not found in Cloudflare", ref.CloudflareName)
		}
		return result.ID, nil
	}

	return "", errors.New("invalid identity provider ref: must specify name, cloudflareId, or cloudflareName")
}

// ResolveGroup resolves a ReusableGroupRef to a Cloudflare Access Group ID.
// Resolution priority: cloudflareId > name > cloudflareName
//
//nolint:revive // cognitive complexity is acceptable for this linear resolution logic
func (r *Resolver) ResolveGroup(ctx context.Context, ref *networkingv1alpha2.ReusableGroupRef) (string, error) {
	if ref == nil {
		return "", errors.New("nil group reference")
	}

	// Priority 1: Direct Cloudflare ID
	if ref.CloudflareID != "" {
		return ref.CloudflareID, nil
	}

	// Priority 2: K8s AccessGroup name
	if ref.Name != "" {
		group := &networkingv1alpha2.AccessGroup{}
		if err := r.client.Get(ctx, apitypes.NamespacedName{Name: ref.Name}, group); err != nil {
			return "", fmt.Errorf("AccessGroup %q not found: %w", ref.Name, err)
		}
		if group.Status.GroupID == "" {
			return "", fmt.Errorf("AccessGroup %q not ready (no GroupID in status)", ref.Name)
		}
		return group.Status.GroupID, nil
	}

	// Priority 3: Cloudflare display name lookup
	if ref.CloudflareName != "" {
		result, err := r.api.GetAccessGroupByName(ctx, ref.CloudflareName)
		if err != nil {
			return "", fmt.Errorf("failed to find group by name %q: %w", ref.CloudflareName, err)
		}
		if result == nil {
			return "", fmt.Errorf("group %q not found in Cloudflare", ref.CloudflareName)
		}
		return result.ID, nil
	}

	return "", errors.New("invalid group ref: must specify name, cloudflareId, or cloudflareName")
}

// ResolveVirtualNetwork resolves a VirtualNetworkRef to a Cloudflare VNet ID.
// Resolution priority: cloudflareId > name > cloudflareName
//
//nolint:revive // cognitive complexity is acceptable for this linear resolution logic
func (r *Resolver) ResolveVirtualNetwork(ctx context.Context, ref *networkingv1alpha2.VirtualNetworkRef) (string, error) {
	if ref == nil {
		return "", errors.New("nil virtual network reference")
	}

	// Priority 1: Direct Cloudflare ID
	if ref.CloudflareID != "" {
		return ref.CloudflareID, nil
	}

	// Priority 2: K8s VirtualNetwork name
	if ref.Name != "" {
		vnet := &networkingv1alpha2.VirtualNetwork{}
		if err := r.client.Get(ctx, apitypes.NamespacedName{Name: ref.Name}, vnet); err != nil {
			return "", fmt.Errorf("VirtualNetwork %q not found: %w", ref.Name, err)
		}
		if vnet.Status.VirtualNetworkId == "" {
			return "", fmt.Errorf("VirtualNetwork %q not ready (no VirtualNetworkId in status)", ref.Name)
		}
		return vnet.Status.VirtualNetworkId, nil
	}

	// Priority 3: Cloudflare display name lookup
	if ref.CloudflareName != "" {
		result, err := r.api.GetVirtualNetworkByName(ctx, ref.CloudflareName)
		if err != nil {
			return "", fmt.Errorf("failed to find VNet by name %q: %w", ref.CloudflareName, err)
		}
		if result == nil {
			return "", fmt.Errorf("VNet %q not found in Cloudflare", ref.CloudflareName)
		}
		return result.ID, nil
	}

	return "", errors.New("invalid virtual network ref: must specify name, cloudflareId, or cloudflareName")
}

// ResolveAllIdentityProviders resolves all IdP references to Cloudflare IdP IDs.
// It handles deduplication automatically.
//
//nolint:revive,prealloc // cognitive complexity is acceptable for this aggregation logic
func (r *Resolver) ResolveAllIdentityProviders(
	ctx context.Context,
	directIDs []string,
	refs []networkingv1alpha2.AccessIdentityProviderRefV2,
) ([]string, []error) {
	seen := make(map[string]bool)
	var result []string
	var errs []error

	// Add direct IDs first
	for _, id := range directIDs {
		if id != "" && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}

	// Resolve refs
	for i, ref := range refs {
		id, err := r.ResolveIdentityProvider(ctx, &ref)
		if err != nil {
			errs = append(errs, fmt.Errorf("IdP ref at index %d: %w", i, err))
			continue
		}
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}

	return result, errs
}

// ResolveAllGroups resolves all group references to Cloudflare Group IDs.
//
//nolint:prealloc // result size depends on runtime resolution success
func (r *Resolver) ResolveAllGroups(
	ctx context.Context,
	refs []networkingv1alpha2.ReusableGroupRef,
) ([]string, []error) {
	var result []string
	var errs []error

	for i, ref := range refs {
		id, err := r.ResolveGroup(ctx, &ref)
		if err != nil {
			errs = append(errs, fmt.Errorf("group ref at index %d: %w", i, err))
			continue
		}
		result = append(result, id)
	}

	return result, errs
}
