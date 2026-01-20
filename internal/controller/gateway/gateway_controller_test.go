// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

// TestExtractHostnamesFromRules tests the extraction of hostnames from ingress rules.
func TestExtractHostnamesFromRules(t *testing.T) {
	r := &GatewayReconciler{}

	tests := []struct {
		name  string
		rules []cf.UnvalidatedIngressRule
		want  []string
	}{
		{
			name:  "empty rules",
			rules: []cf.UnvalidatedIngressRule{},
			want:  []string{},
		},
		{
			name: "single hostname",
			rules: []cf.UnvalidatedIngressRule{
				{Hostname: "app.example.com"},
			},
			want: []string{"app.example.com"},
		},
		{
			name: "multiple hostnames",
			rules: []cf.UnvalidatedIngressRule{
				{Hostname: "app.example.com"},
				{Hostname: "api.example.com"},
				{Hostname: "web.example.com"},
			},
			want: []string{"api.example.com", "app.example.com", "web.example.com"}, // sorted
		},
		{
			name: "duplicate hostnames",
			rules: []cf.UnvalidatedIngressRule{
				{Hostname: "app.example.com"},
				{Hostname: "app.example.com"},
				{Hostname: "api.example.com"},
			},
			want: []string{"api.example.com", "app.example.com"}, // deduplicated
		},
		{
			name: "wildcard hostname",
			rules: []cf.UnvalidatedIngressRule{
				{Hostname: "*.example.com"},
				{Hostname: "app.example.com"},
			},
			want: []string{"*.example.com", "app.example.com"}, // both kept
		},
		{
			name: "skip empty hostname (catch-all)",
			rules: []cf.UnvalidatedIngressRule{
				{Hostname: "app.example.com"},
				{Hostname: ""}, // catch-all rule
				{Hostname: "api.example.com"},
			},
			want: []string{"api.example.com", "app.example.com"},
		},
		{
			name: "only catch-all rule",
			rules: []cf.UnvalidatedIngressRule{
				{Hostname: ""},
			},
			want: []string{},
		},
		{
			name: "mixed with service field",
			rules: []cf.UnvalidatedIngressRule{
				{Hostname: "app.example.com", Service: "http://svc1:80"},
				{Hostname: "api.example.com", Service: "http://svc2:8080"},
			},
			want: []string{"api.example.com", "app.example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.extractHostnamesFromRules(tt.rules)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestSanitizeHostnameForK8s tests the hostname sanitization for K8s resource names.
func TestSanitizeHostnameForK8s(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		want     string
	}{
		{
			name:     "simple hostname",
			hostname: "app.example.com",
			want:     "app-example-com",
		},
		{
			name:     "subdomain",
			hostname: "api.staging.example.com",
			want:     "api-staging-example-com",
		},
		{
			name:     "wildcard hostname",
			hostname: "*.example.com",
			want:     "example-com",
		},
		{
			name:     "single label",
			hostname: "localhost",
			want:     "localhost",
		},
		{
			name:     "empty hostname",
			hostname: "",
			want:     "",
		},
		{
			name:     "long hostname truncated",
			hostname: "very-long-subdomain.another-long-part.yet-another-subdomain.example.com",
			want:     "very-long-subdomain-another-long-part-yet-another-",
		},
		{
			name:     "hostname with leading dot",
			hostname: ".example.com",
			want:     "example-com",
		},
		{
			name:     "wildcard with subdomain",
			hostname: "*.api.example.com",
			want:     "api-example-com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeHostnameForK8s(tt.hostname)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBoolPtr tests the boolPtr helper function.
func TestBoolPtr(t *testing.T) {
	truePtr := boolPtr(true)
	falsePtr := boolPtr(false)

	assert.NotNil(t, truePtr)
	assert.NotNil(t, falsePtr)
	assert.True(t, *truePtr)
	assert.False(t, *falsePtr)
}
