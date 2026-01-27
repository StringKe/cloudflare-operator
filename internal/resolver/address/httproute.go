// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package address

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// resolveHTTPRoute extracts addresses from a Gateway API HTTPRoute.
// It resolves the parent Gateway(s) to get the actual addresses.
//
//nolint:revive // cognitive complexity is acceptable for this function
func (r *Resolver) resolveHTTPRoute(ctx context.Context, ref *v1alpha2.HTTPRouteDNSSource, defaultNamespace string) (*ResolveResult, error) {
	ns := ref.Namespace
	if ns == "" {
		ns = defaultNamespace
	}

	httpRoute := &gatewayv1.HTTPRoute{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ns}, httpRoute); err != nil {
		if apierrors.IsNotFound(err) {
			return &ResolveResult{SourceExists: false}, nil
		}
		return nil, err
	}

	result := &ResolveResult{
		ResourceVersion: httpRoute.ResourceVersion,
		SourceExists:    true,
		Addresses:       []ResolvedAddress{},
	}

	// HTTPRoute doesn't have addresses directly, we need to look up parent Gateways
	for _, parentRef := range httpRoute.Spec.ParentRefs {
		// Only handle Gateway parents
		if parentRef.Group != nil && *parentRef.Group != gatewayv1.GroupName {
			continue
		}
		if parentRef.Kind != nil && *parentRef.Kind != "Gateway" {
			continue
		}

		// Determine gateway namespace
		gwNamespace := ns
		if parentRef.Namespace != nil {
			gwNamespace = string(*parentRef.Namespace)
		}

		gateway := &gatewayv1.Gateway{}
		if err := r.client.Get(ctx, types.NamespacedName{
			Name:      string(parentRef.Name),
			Namespace: gwNamespace,
		}, gateway); err != nil {
			if apierrors.IsNotFound(err) {
				continue // Skip missing gateways
			}
			return nil, fmt.Errorf("failed to get parent Gateway %s/%s: %w", gwNamespace, parentRef.Name, err)
		}

		// Extract addresses from Gateway status
		for _, addr := range gateway.Status.Addresses {
			if addr.Value != "" {
				result.Addresses = append(result.Addresses, ParseAddress(addr.Value))
			}
		}
	}

	return result, nil
}
