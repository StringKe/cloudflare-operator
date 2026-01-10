// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package tunnel provides shared tunnel resolution and management utilities
// for controllers that work with Tunnel and ClusterTunnel resources.
package tunnel

import (
	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// Interface provides a common interface for Tunnel and ClusterTunnel.
// This allows controllers to work with both resource types uniformly.
type Interface interface {
	GetName() string
	GetNamespace() string
	GetSpec() networkingv1alpha2.TunnelSpec
	GetStatus() networkingv1alpha2.TunnelStatus
}

// TunnelWrapper wraps a Tunnel to implement Interface
type TunnelWrapper struct {
	Tunnel *networkingv1alpha2.Tunnel
}

// GetName returns the tunnel name
func (w *TunnelWrapper) GetName() string {
	return w.Tunnel.Name
}

// GetNamespace returns the tunnel namespace
func (w *TunnelWrapper) GetNamespace() string {
	return w.Tunnel.Namespace
}

// GetSpec returns the tunnel spec
func (w *TunnelWrapper) GetSpec() networkingv1alpha2.TunnelSpec {
	return w.Tunnel.Spec
}

// GetStatus returns the tunnel status
func (w *TunnelWrapper) GetStatus() networkingv1alpha2.TunnelStatus {
	return w.Tunnel.Status
}

// ClusterTunnelWrapper wraps a ClusterTunnel to implement Interface
type ClusterTunnelWrapper struct {
	ClusterTunnel     *networkingv1alpha2.ClusterTunnel
	OperatorNamespace string
}

// GetName returns the cluster tunnel name
func (w *ClusterTunnelWrapper) GetName() string {
	return w.ClusterTunnel.Name
}

// GetNamespace returns the operator namespace (cluster-scoped resources use operator namespace)
func (w *ClusterTunnelWrapper) GetNamespace() string {
	return w.OperatorNamespace
}

// GetSpec returns the cluster tunnel spec
func (w *ClusterTunnelWrapper) GetSpec() networkingv1alpha2.TunnelSpec {
	return w.ClusterTunnel.Spec
}

// GetStatus returns the cluster tunnel status
func (w *ClusterTunnelWrapper) GetStatus() networkingv1alpha2.TunnelStatus {
	return w.ClusterTunnel.Status
}
