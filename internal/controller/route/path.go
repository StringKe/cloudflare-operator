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
