// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package accessapplication

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDomainMatchesHosts(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		hosts  []string
		want   bool
	}{
		{
			name:   "empty domain",
			domain: "",
			hosts:  []string{"example.com"},
			want:   false,
		},
		{
			name:   "empty hosts",
			domain: "app.example.com",
			hosts:  []string{},
			want:   false,
		},
		{
			name:   "exact match",
			domain: "app.example.com",
			hosts:  []string{"app.example.com"},
			want:   true,
		},
		{
			name:   "exact match case insensitive",
			domain: "APP.EXAMPLE.COM",
			hosts:  []string{"app.example.com"},
			want:   true,
		},
		{
			name:   "no match",
			domain: "app.example.com",
			hosts:  []string{"other.example.com"},
			want:   false,
		},
		{
			name:   "wildcard host matches subdomain",
			domain: "app.example.com",
			hosts:  []string{"*.example.com"},
			want:   true,
		},
		{
			name:   "wildcard host matches base domain",
			domain: "example.com",
			hosts:  []string{"*.example.com"},
			want:   true,
		},
		{
			name:   "wildcard host does not match different domain",
			domain: "app.other.com",
			hosts:  []string{"*.example.com"},
			want:   false,
		},
		{
			name:   "wildcard domain matches host subdomain",
			domain: "*.example.com",
			hosts:  []string{"app.example.com"},
			want:   true,
		},
		{
			name:   "wildcard domain matches base host",
			domain: "*.example.com",
			hosts:  []string{"example.com"},
			want:   true,
		},
		{
			name:   "wildcard domain does not match different host",
			domain: "*.example.com",
			hosts:  []string{"app.other.com"},
			want:   false,
		},
		{
			name:   "deep subdomain match with wildcard",
			domain: "deep.sub.example.com",
			hosts:  []string{"*.example.com"},
			want:   true,
		},
		{
			name:   "match in multiple hosts",
			domain: "app.example.com",
			hosts:  []string{"other.com", "app.example.com", "test.com"},
			want:   true,
		},
		{
			name:   "real world example - sba domain",
			domain: "sba.test.1xe.mip.sup-any.com",
			hosts:  []string{"sba.test.1xe.mip.sup-any.com"},
			want:   true,
		},
		{
			name:   "real world example - gateway domain",
			domain: "gateway.test.1xe.mip.sup-any.com",
			hosts:  []string{"gateway.test.1xe.mip.sup-any.com", "other.example.com"},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domainMatchesHosts(tt.domain, tt.hosts)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAnyDomainMatchesHosts(t *testing.T) {
	tests := []struct {
		name    string
		domains []string
		hosts   []string
		want    bool
	}{
		{
			name:    "empty domains",
			domains: []string{},
			hosts:   []string{"example.com"},
			want:    false,
		},
		{
			name:    "empty hosts",
			domains: []string{"app.example.com"},
			hosts:   []string{},
			want:    false,
		},
		{
			name:    "first domain matches",
			domains: []string{"app.example.com", "other.com"},
			hosts:   []string{"app.example.com"},
			want:    true,
		},
		{
			name:    "second domain matches",
			domains: []string{"other.com", "app.example.com"},
			hosts:   []string{"app.example.com"},
			want:    true,
		},
		{
			name:    "no domains match",
			domains: []string{"foo.com", "bar.com"},
			hosts:   []string{"example.com"},
			want:    false,
		},
		{
			name:    "wildcard in domains matches",
			domains: []string{"*.example.com"},
			hosts:   []string{"app.example.com"},
			want:    true,
		},
		{
			name:    "real world selfHostedDomains",
			domains: []string{"sba.test.1xe.mip.sup-any.com", "sba.test.1xe.mip.sup-game.com"},
			hosts:   []string{"sba.test.1xe.mip.sup-any.com"},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := anyDomainMatchesHosts(tt.domains, tt.hosts)
			assert.Equal(t, tt.want, got)
		})
	}
}
