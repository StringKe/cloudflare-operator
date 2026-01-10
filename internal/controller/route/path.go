// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package route provides shared utilities for building cloudflared ingress rules
// from various Kubernetes resources (Ingress, Gateway API routes, TunnelBinding).
package route

import (
	networkingv1 "k8s.io/api/networking/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ConvertIngressPathType converts Kubernetes Ingress PathType to cloudflared path regex.
// Cloudflared uses regex for path matching, so we need to convert the Kubernetes path types.
// nolint:revive // Cognitive complexity for path conversion logic
func ConvertIngressPathType(path string, pathType *networkingv1.PathType) string {
	if path == "" || path == "/" {
		return ""
	}

	pt := networkingv1.PathTypePrefix
	if pathType != nil {
		pt = *pathType
	}

	switch pt {
	case networkingv1.PathTypeExact:
		// Exact match
		return "^" + path + "$"
	case networkingv1.PathTypePrefix:
		// Prefix match - cloudflared uses regex
		// /foo should match /foo, /foo/, /foo/bar
		if path[len(path)-1] == '/' {
			return path + ".*"
		}
		return path + "(/.*)?$"
	case networkingv1.PathTypeImplementationSpecific:
		// Treat as prefix
		if path[len(path)-1] == '/' {
			return path + ".*"
		}
		return path + "(/.*)?$"
	default:
		return path
	}
}

// ConvertGatewayPathType converts Gateway API PathMatchType to cloudflared path regex.
// This supports the Gateway API's path matching semantics.
func ConvertGatewayPathType(path string, pathType *gatewayv1.PathMatchType) string {
	if path == "" || path == "/" {
		return ""
	}

	pt := gatewayv1.PathMatchPathPrefix
	if pathType != nil {
		pt = *pathType
	}

	switch pt {
	case gatewayv1.PathMatchExact:
		// Exact match
		return "^" + path + "$"
	case gatewayv1.PathMatchPathPrefix:
		// Prefix match
		if path[len(path)-1] == '/' {
			return path + ".*"
		}
		return path + "(/.*)?$"
	case gatewayv1.PathMatchRegularExpression:
		// Already a regex, use as-is
		return path
	default:
		return path
	}
}
