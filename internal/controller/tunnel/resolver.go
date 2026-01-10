// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package tunnel

import (
	"context"
	"fmt"

	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1alpha1 "github.com/StringKe/cloudflare-operator/api/v1alpha1"
	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// Resolver resolves tunnel references to concrete tunnel resources.
// It handles both Tunnel (namespaced) and ClusterTunnel (cluster-scoped) resources.
type Resolver struct {
	client.Client
	OperatorNamespace string
}

// NewResolver creates a new tunnel resolver.
func NewResolver(c client.Client, operatorNamespace string) *Resolver {
	return &Resolver{
		Client:            c,
		OperatorNamespace: operatorNamespace,
	}
}

// Resolve resolves a TunnelReference to a concrete Interface.
// For Tunnel kind, it uses the provided defaultNamespace or the namespace from the reference.
// For ClusterTunnel kind, the namespace is the operator namespace.
func (r *Resolver) Resolve(ctx context.Context, ref networkingv1alpha2.TunnelReference, defaultNamespace string) (Interface, error) {
	switch ref.Kind {
	case "Tunnel":
		return r.resolveTunnel(ctx, ref, defaultNamespace)
	case "ClusterTunnel":
		return r.resolveClusterTunnel(ctx, ref)
	default:
		return nil, fmt.Errorf("invalid tunnel kind: %s (expected Tunnel or ClusterTunnel)", ref.Kind)
	}
}

// resolveTunnel resolves a Tunnel reference
func (r *Resolver) resolveTunnel(ctx context.Context, ref networkingv1alpha2.TunnelReference, defaultNamespace string) (Interface, error) {
	namespace := ref.Namespace
	if namespace == "" {
		namespace = defaultNamespace
	}
	if namespace == "" {
		return nil, fmt.Errorf("tunnel %s: namespace is required for Tunnel kind", ref.Name)
	}

	tunnel := &networkingv1alpha2.Tunnel{}
	if err := r.Get(ctx, apitypes.NamespacedName{
		Name:      ref.Name,
		Namespace: namespace,
	}, tunnel); err != nil {
		return nil, fmt.Errorf("tunnel %s/%s not found: %w", namespace, ref.Name, err)
	}

	return &TunnelWrapper{Tunnel: tunnel}, nil
}

// resolveClusterTunnel resolves a ClusterTunnel reference
func (r *Resolver) resolveClusterTunnel(ctx context.Context, ref networkingv1alpha2.TunnelReference) (Interface, error) {
	clusterTunnel := &networkingv1alpha2.ClusterTunnel{}
	if err := r.Get(ctx, apitypes.NamespacedName{Name: ref.Name}, clusterTunnel); err != nil {
		return nil, fmt.Errorf("clustertunnel %s not found: %w", ref.Name, err)
	}

	return &ClusterTunnelWrapper{
		ClusterTunnel:     clusterTunnel,
		OperatorNamespace: r.OperatorNamespace,
	}, nil
}

// ResolveFromTunnelRef resolves from v1alpha1.TunnelRef for backward compatibility.
// This is used by TunnelBinding and other legacy resources.
// Note: v1alpha1.TunnelRef doesn't have a Namespace field, so for Tunnel kind,
// we use the binding's namespace.
func (r *Resolver) ResolveFromTunnelRef(ctx context.Context, ref networkingv1alpha1.TunnelRef, bindingNamespace string) (Interface, error) {
	// Convert v1alpha1.TunnelRef to v1alpha2.TunnelReference
	// v1alpha1.TunnelRef doesn't have Namespace, so we use bindingNamespace for Tunnel kind
	tunnelRef := networkingv1alpha2.TunnelReference{
		Kind: ref.Kind,
		Name: ref.Name,
		// Namespace is not in v1alpha1.TunnelRef, use bindingNamespace as default
	}

	return r.Resolve(ctx, tunnelRef, bindingNamespace)
}

// ResolveForIngressClassConfig resolves a tunnel from TunnelIngressClassConfig.
// It uses the config's GetTunnelNamespace() method for namespace resolution.
func (r *Resolver) ResolveForIngressClassConfig(ctx context.Context, config *networkingv1alpha2.TunnelIngressClassConfig) (Interface, error) {
	ref := config.Spec.TunnelRef
	defaultNamespace := config.GetTunnelNamespace()

	return r.Resolve(ctx, ref, defaultNamespace)
}
