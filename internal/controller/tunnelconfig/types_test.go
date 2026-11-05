// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package tunnelconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAggregateRulesOrdersByPathSpecificity(t *testing.T) {
	config := &TunnelConfig{
		Sources: map[string]*SourceConfig{
			"Ingress/test/example": {
				Kind:      SourceKindIngress,
				Namespace: "test",
				Name:      "example",
				Rules: []IngressRule{
					{Hostname: "example.com", Path: "", Service: "http://root"},
					{Hostname: "example.com", Path: "/api", Service: "http://api"},
					{Hostname: "example.com", Path: "/api/v2", Service: "http://api-v2"},
					{Hostname: "example.com", Path: "/static", Service: "http://static"},
					{Hostname: "other.example.com", Path: "", Service: "http://other"},
				},
			},
		},
	}

	got := config.AggregateRules()
	want := []string{
		"example.com/api/v2",
		"example.com/static",
		"example.com/api",
		"example.com",
		"other.example.com",
	}

	assert.Len(t, got, len(want))
	for i := range want {
		assert.Equal(t, want[i], got[i].Hostname+got[i].Path)
	}
}
