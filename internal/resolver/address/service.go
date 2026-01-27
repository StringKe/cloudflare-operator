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

// resolveService extracts addresses from a Kubernetes Service.
//
//nolint:revive // cyclomatic complexity is acceptable for this switch statement
func (r *Resolver) resolveService(ctx context.Context, ref *v1alpha2.ServiceDNSSource, defaultNamespace string) (*ResolveResult, error) {
	ns := ref.Namespace
	if ns == "" {
		ns = defaultNamespace
	}

	svc := &corev1.Service{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ns}, svc); err != nil {
		if apierrors.IsNotFound(err) {
			return &ResolveResult{SourceExists: false}, nil
		}
		return nil, err
	}

	result := &ResolveResult{
		ResourceVersion: svc.ResourceVersion,
		SourceExists:    true,
		Addresses:       []ResolvedAddress{},
	}

	addressType := ref.AddressType
	if addressType == "" {
		addressType = v1alpha2.ServiceAddressLoadBalancerIP
	}

	switch addressType {
	case v1alpha2.ServiceAddressLoadBalancerIP:
		for _, ingress := range svc.Status.LoadBalancer.Ingress {
			if ingress.IP != "" {
				result.Addresses = append(result.Addresses, ParseAddress(ingress.IP))
			}
		}

	case v1alpha2.ServiceAddressLoadBalancerHostname:
		for _, ingress := range svc.Status.LoadBalancer.Ingress {
			if ingress.Hostname != "" {
				result.Addresses = append(result.Addresses, ParseAddress(ingress.Hostname))
			}
		}

	case v1alpha2.ServiceAddressExternalIP:
		for _, ip := range svc.Spec.ExternalIPs {
			result.Addresses = append(result.Addresses, ParseAddress(ip))
		}

	case v1alpha2.ServiceAddressExternalName:
		if svc.Spec.Type == corev1.ServiceTypeExternalName && svc.Spec.ExternalName != "" {
			result.Addresses = append(result.Addresses, ParseAddress(svc.Spec.ExternalName))
		}

	case v1alpha2.ServiceAddressClusterIP:
		if svc.Spec.ClusterIP != "" && svc.Spec.ClusterIP != "None" {
			result.Addresses = append(result.Addresses, ParseAddress(svc.Spec.ClusterIP))
		}
	}

	return result, nil
}
