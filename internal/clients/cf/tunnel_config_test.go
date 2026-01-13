// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"testing"
	"time"

	"github.com/cloudflare/cloudflare-go"
)

func TestConvertLocalRulesToSDK(t *testing.T) {
	tests := []struct {
		name       string
		localRules []UnvalidatedIngressRule
		wantCount  int
	}{
		{
			name:       "empty rules",
			localRules: []UnvalidatedIngressRule{},
			wantCount:  0,
		},
		{
			name: "simple rule without origin request",
			localRules: []UnvalidatedIngressRule{
				{
					Hostname: "example.com",
					Service:  "http://backend:80",
				},
			},
			wantCount: 1,
		},
		{
			name: "rule with path",
			localRules: []UnvalidatedIngressRule{
				{
					Hostname: "api.example.com",
					Path:     "/v1/*",
					Service:  "http://api:8080",
				},
			},
			wantCount: 1,
		},
		{
			name: "multiple rules",
			localRules: []UnvalidatedIngressRule{
				{Hostname: "app1.example.com", Service: "http://app1:80"},
				{Hostname: "app2.example.com", Service: "http://app2:80"},
				{Service: "http_status:404"}, // fallback
			},
			wantCount: 3,
		},
		{
			name: "rule with origin request",
			localRules: []UnvalidatedIngressRule{
				{
					Hostname: "secure.example.com",
					Service:  "https://backend:443",
					OriginRequest: OriginRequestConfig{
						NoTLSVerify: boolPtr(true),
						Http2Origin: boolPtr(true),
					},
				},
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertLocalRulesToSDK(tt.localRules)
			if len(got) != tt.wantCount {
				t.Errorf("ConvertLocalRulesToSDK() returned %d rules, want %d", len(got), tt.wantCount)
			}

			// Verify basic field mapping
			for i, rule := range got {
				if rule.Hostname != tt.localRules[i].Hostname {
					t.Errorf("rule[%d].Hostname = %q, want %q", i, rule.Hostname, tt.localRules[i].Hostname)
				}
				if rule.Path != tt.localRules[i].Path {
					t.Errorf("rule[%d].Path = %q, want %q", i, rule.Path, tt.localRules[i].Path)
				}
				if rule.Service != tt.localRules[i].Service {
					t.Errorf("rule[%d].Service = %q, want %q", i, rule.Service, tt.localRules[i].Service)
				}
			}
		})
	}
}

func TestConvertOriginRequest(t *testing.T) {
	timeout := 30 * time.Second

	tests := []struct {
		name  string
		local OriginRequestConfig
		check func(t *testing.T, sdk *cloudflare.OriginRequestConfig)
	}{
		{
			name:  "empty config",
			local: OriginRequestConfig{},
			check: func(t *testing.T, sdk *cloudflare.OriginRequestConfig) {
				if sdk.NoTLSVerify != nil {
					t.Error("expected NoTLSVerify to be nil")
				}
			},
		},
		{
			name: "with NoTLSVerify",
			local: OriginRequestConfig{
				NoTLSVerify: boolPtr(true),
			},
			check: func(t *testing.T, sdk *cloudflare.OriginRequestConfig) {
				if sdk.NoTLSVerify == nil || !*sdk.NoTLSVerify {
					t.Error("expected NoTLSVerify to be true")
				}
			},
		},
		{
			name: "with timeout duration",
			local: OriginRequestConfig{
				ConnectTimeout: &timeout,
			},
			check: func(t *testing.T, sdk *cloudflare.OriginRequestConfig) {
				if sdk.ConnectTimeout == nil {
					t.Error("expected ConnectTimeout to be set")
					return
				}
				if sdk.ConnectTimeout.Duration != timeout {
					t.Errorf("ConnectTimeout = %v, want %v", sdk.ConnectTimeout.Duration, timeout)
				}
			},
		},
		{
			name: "with HTTP2Origin",
			local: OriginRequestConfig{
				Http2Origin: boolPtr(true),
			},
			check: func(t *testing.T, sdk *cloudflare.OriginRequestConfig) {
				if sdk.Http2Origin == nil || !*sdk.Http2Origin {
					t.Error("expected Http2Origin to be true")
				}
			},
		},
		{
			name: "with CAPool",
			local: OriginRequestConfig{
				CAPool: stringPtr("/path/to/ca.crt"),
			},
			check: func(t *testing.T, sdk *cloudflare.OriginRequestConfig) {
				if sdk.CAPool == nil || *sdk.CAPool != "/path/to/ca.crt" {
					t.Errorf("CAPool = %v, want /path/to/ca.crt", sdk.CAPool)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sdk := convertOriginRequest(tt.local)
			tt.check(t, sdk)
		})
	}
}

func TestHasOriginRequest(t *testing.T) {
	tests := []struct {
		name string
		cfg  OriginRequestConfig
		want bool
	}{
		{
			name: "empty config",
			cfg:  OriginRequestConfig{},
			want: false,
		},
		{
			name: "with NoTLSVerify",
			cfg:  OriginRequestConfig{NoTLSVerify: boolPtr(true)},
			want: true,
		},
		{
			name: "with ConnectTimeout",
			cfg:  OriginRequestConfig{ConnectTimeout: durationPtr(30 * time.Second)},
			want: true,
		},
		{
			name: "with IPRules",
			cfg: OriginRequestConfig{
				IPRules: []IngressIPRule{{Allow: true}},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasOriginRequest(tt.cfg); got != tt.want {
				t.Errorf("hasOriginRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper function for duration pointer (boolPtr and stringPtr are defined in other test files)
func durationPtr(d time.Duration) *time.Duration {
	return &d
}
