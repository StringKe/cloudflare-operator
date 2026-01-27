// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package address

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// resolveGateway extracts addresses from a Gateway API Gateway.
//
//nolint:revive // cognitive complexity is acceptable for this function
func (r *Resolver) resolveGateway(ctx context.Context, ref *v1alpha2.GatewayDNSSource, defaultNamespace string) (*ResolveResult, error) {
	ns := ref.Namespace
	if ns == "" {
		ns = defaultNamespace
	}

	gateway := &gatewayv1.Gateway{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ns}, gateway); err != nil {
		if apierrors.IsNotFound(err) {
			return &ResolveResult{SourceExists: false}, nil
		}
		return nil, err
	}

	result := &ResolveResult{
		ResourceVersion: gateway.ResourceVersion,
		SourceExists:    true,
		Addresses:       []ResolvedAddress{},
	}

	// Extract addresses from Gateway status
	for _, addr := range gateway.Status.Addresses {
		if addr.Value != "" {
			result.Addresses = append(result.Addresses, ParseAddress(addr.Value))
		}
	}

	return result, nil
}
