// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package address

import (
	"context"

	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// resolveIngress extracts addresses from a Kubernetes Ingress.
//
//nolint:revive // cognitive complexity is acceptable for this function
func (r *Resolver) resolveIngress(ctx context.Context, ref *v1alpha2.IngressDNSSource, defaultNamespace string) (*ResolveResult, error) {
	ns := ref.Namespace
	if ns == "" {
		ns = defaultNamespace
	}

	ingress := &networkingv1.Ingress{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ns}, ingress); err != nil {
		if apierrors.IsNotFound(err) {
			return &ResolveResult{SourceExists: false}, nil
		}
		return nil, err
	}

	result := &ResolveResult{
		ResourceVersion: ingress.ResourceVersion,
		SourceExists:    true,
		Addresses:       []ResolvedAddress{},
	}

	// Extract addresses from LoadBalancer status
	for _, lbIngress := range ingress.Status.LoadBalancer.Ingress {
		if lbIngress.IP != "" {
			result.Addresses = append(result.Addresses, ParseAddress(lbIngress.IP))
		}
		if lbIngress.Hostname != "" {
			result.Addresses = append(result.Addresses, ParseAddress(lbIngress.Hostname))
		}
	}

	return result, nil
}
