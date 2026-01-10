// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package route

import (
	"testing"

	"github.com/stretchr/testify/assert"
	networkingv1 "k8s.io/api/networking/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func pathTypePtr(pt networkingv1.PathType) *networkingv1.PathType {
	return &pt
}

func gatewayPathTypePtr(pt gatewayv1.PathMatchType) *gatewayv1.PathMatchType {
	return &pt
}

func TestConvertIngressPathType(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		pathType *networkingv1.PathType
		want     string
	}{
		{
			name:     "empty path",
			path:     "",
			pathType: nil,
			want:     "",
		},
		{
			name:     "root path",
			path:     "/",
			pathType: nil,
			want:     "",
		},
		{
			name:     "prefix - simple path",
			path:     "/api",
			pathType: pathTypePtr(networkingv1.PathTypePrefix),
			want:     "/api(/.*)?$",
		},
		{
			name:     "prefix - path with trailing slash",
			path:     "/api/",
			pathType: pathTypePtr(networkingv1.PathTypePrefix),
			want:     "/api/.*",
		},
		{
			name:     "exact - simple path",
			path:     "/api/v1/users",
			pathType: pathTypePtr(networkingv1.PathTypeExact),
			want:     "^/api/v1/users$",
		},
		{
			name:     "implementation specific - treated as prefix",
			path:     "/app",
			pathType: pathTypePtr(networkingv1.PathTypeImplementationSpecific),
			want:     "/app(/.*)?$",
		},
		{
			name:     "implementation specific - with trailing slash",
			path:     "/app/",
			pathType: pathTypePtr(networkingv1.PathTypeImplementationSpecific),
			want:     "/app/.*",
		},
		{
			name:     "nil pathType defaults to prefix",
			path:     "/default",
			pathType: nil,
			want:     "/default(/.*)?$",
		},
		{
			name:     "nested path prefix",
			path:     "/api/v1/users",
			pathType: pathTypePtr(networkingv1.PathTypePrefix),
			want:     "/api/v1/users(/.*)?$",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertIngressPathType(tt.path, tt.pathType)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConvertGatewayPathType(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		pathType *gatewayv1.PathMatchType
		want     string
	}{
		{
			name:     "empty path",
			path:     "",
			pathType: nil,
			want:     "",
		},
		{
			name:     "root path",
			path:     "/",
			pathType: nil,
			want:     "",
		},
		{
			name:     "path prefix - simple",
			path:     "/api",
			pathType: gatewayPathTypePtr(gatewayv1.PathMatchPathPrefix),
			want:     "/api(/.*)?$",
		},
		{
			name:     "path prefix - with trailing slash",
			path:     "/api/",
			pathType: gatewayPathTypePtr(gatewayv1.PathMatchPathPrefix),
			want:     "/api/.*",
		},
		{
			name:     "exact match",
			path:     "/api/v1/exact",
			pathType: gatewayPathTypePtr(gatewayv1.PathMatchExact),
			want:     "^/api/v1/exact$",
		},
		{
			name:     "regular expression - used as is",
			path:     "^/api/v[0-9]+/.*",
			pathType: gatewayPathTypePtr(gatewayv1.PathMatchRegularExpression),
			want:     "^/api/v[0-9]+/.*",
		},
		{
			name:     "nil pathType defaults to prefix",
			path:     "/default",
			pathType: nil,
			want:     "/default(/.*)?$",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertGatewayPathType(tt.path, tt.pathType)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestPathMatchingConsistency verifies that Ingress and Gateway path types
// produce consistent results for equivalent path types.
func TestPathMatchingConsistency(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		ingressType networkingv1.PathType
		gatewayType gatewayv1.PathMatchType
	}{
		{
			name:        "prefix paths should match",
			path:        "/api/v1",
			ingressType: networkingv1.PathTypePrefix,
			gatewayType: gatewayv1.PathMatchPathPrefix,
		},
		{
			name:        "exact paths should match",
			path:        "/exact/path",
			ingressType: networkingv1.PathTypeExact,
			gatewayType: gatewayv1.PathMatchExact,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ingressResult := ConvertIngressPathType(tt.path, pathTypePtr(tt.ingressType))
			gatewayResult := ConvertGatewayPathType(tt.path, gatewayPathTypePtr(tt.gatewayType))

			assert.Equal(t, ingressResult, gatewayResult,
				"Ingress and Gateway path conversion should produce same result for equivalent types")
		})
	}
}
