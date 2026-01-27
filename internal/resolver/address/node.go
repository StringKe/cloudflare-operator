// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package address

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// resolveNode extracts addresses from a Kubernetes Node.
//
//nolint:revive // cyclomatic complexity is acceptable for this switch statement
func (r *Resolver) resolveNode(ctx context.Context, ref *v1alpha2.NodeDNSSource) (*ResolveResult, error) {
	node := &corev1.Node{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: ref.Name}, node); err != nil {
		if apierrors.IsNotFound(err) {
			return &ResolveResult{SourceExists: false}, nil
		}
		return nil, err
	}

	result := &ResolveResult{
		ResourceVersion: node.ResourceVersion,
		SourceExists:    true,
		Addresses:       []ResolvedAddress{},
	}

	addressType := ref.AddressType
	if addressType == "" {
		addressType = v1alpha2.NodeAddressExternalIP
	}

	// Map our type to Kubernetes NodeAddressType
	var targetType corev1.NodeAddressType
	switch addressType {
	case v1alpha2.NodeAddressInternalIP:
		targetType = corev1.NodeInternalIP
	case v1alpha2.NodeAddressExternalIP:
		targetType = corev1.NodeExternalIP
	case v1alpha2.NodeAddressHostname:
		targetType = corev1.NodeHostName
	default:
		targetType = corev1.NodeExternalIP
	}

	// Find addresses of the target type
	for _, addr := range node.Status.Addresses {
		if addr.Type == targetType && addr.Address != "" {
			result.Addresses = append(result.Addresses, ParseAddress(addr.Address))
		}
	}

	return result, nil
}
