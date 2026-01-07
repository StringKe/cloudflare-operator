/*
Copyright 2025 Adyanth H.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
