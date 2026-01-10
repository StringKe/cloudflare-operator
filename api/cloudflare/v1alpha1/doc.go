// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package v1alpha1 contains shared API types for Cloudflare Zero Trust resources.
// These types are used across multiple CRDs to provide consistent interfaces
// for Cloudflare credentials, resource references, and status reporting.
//
// Key types include:
//   - CloudflareRef: Unified reference to Cloudflare credentials and account
//   - CloudflareCredentials: API authentication configuration
//   - CommonStatus: Standard status fields with conditions
//   - TunnelReference: Reference to Tunnel/ClusterTunnel resources
//   - VirtualNetworkReference: Reference to VirtualNetwork resources
//
// +kubebuilder:object:generate=true
// +groupName=cloudflare.com
package v1alpha1
