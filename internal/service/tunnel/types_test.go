// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package tunnel

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

func TestIngressRule(t *testing.T) {
	rule := IngressRule{
		Hostname: "app.example.com",
		Path:     "/api/*",
		Service:  "http://backend.default.svc:8080",
	}

	assert.Equal(t, "app.example.com", rule.Hostname)
	assert.Equal(t, "/api/*", rule.Path)
	assert.Equal(t, "http://backend.default.svc:8080", rule.Service)
}

func TestIngressRuleWithOriginRequest(t *testing.T) {
	connectTimeout := 30 * time.Second
	noTLSVerify := true
	httpHostHeader := "internal.example.com"

	rule := IngressRule{
		Hostname: "app.example.com",
		Service:  "https://secure-backend.default.svc:443",
		OriginRequest: &OriginRequestConfig{
			ConnectTimeout: &connectTimeout,
			NoTLSVerify:    &noTLSVerify,
			HTTPHostHeader: &httpHostHeader,
		},
	}

	assert.NotNil(t, rule.OriginRequest)
	assert.Equal(t, 30*time.Second, *rule.OriginRequest.ConnectTimeout)
	assert.True(t, *rule.OriginRequest.NoTLSVerify)
	assert.Equal(t, "internal.example.com", *rule.OriginRequest.HTTPHostHeader)
}

func TestOriginRequestConfig(t *testing.T) {
	connectTimeout := 10 * time.Second
	tlsTimeout := 15 * time.Second
	keepAliveTimeout := 60 * time.Second
	keepAliveConnections := 100
	noHappyEyeballs := false
	http2Origin := true
	bastionMode := false

	config := OriginRequestConfig{
		ConnectTimeout:       &connectTimeout,
		TLSTimeout:           &tlsTimeout,
		KeepAliveTimeout:     &keepAliveTimeout,
		KeepAliveConnections: &keepAliveConnections,
		NoHappyEyeballs:      &noHappyEyeballs,
		HTTP2Origin:          &http2Origin,
		BastionMode:          &bastionMode,
	}

	assert.Equal(t, 10*time.Second, *config.ConnectTimeout)
	assert.Equal(t, 15*time.Second, *config.TLSTimeout)
	assert.Equal(t, 60*time.Second, *config.KeepAliveTimeout)
	assert.Equal(t, 100, *config.KeepAliveConnections)
	assert.False(t, *config.NoHappyEyeballs)
	assert.True(t, *config.HTTP2Origin)
	assert.False(t, *config.BastionMode)
}

func TestOriginRequestConfigProxy(t *testing.T) {
	proxyAddress := "127.0.0.1"
	proxyPort := uint(1080)
	proxyType := "socks"

	config := OriginRequestConfig{
		ProxyAddress: &proxyAddress,
		ProxyPort:    &proxyPort,
		ProxyType:    &proxyType,
	}

	assert.Equal(t, "127.0.0.1", *config.ProxyAddress)
	assert.Equal(t, uint(1080), *config.ProxyPort)
	assert.Equal(t, "socks", *config.ProxyType)
}

func TestWarpRoutingConfig(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{
			name:    "enabled",
			enabled: true,
		},
		{
			name:    "disabled",
			enabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := WarpRoutingConfig{
				Enabled: tt.enabled,
			}
			assert.Equal(t, tt.enabled, config.Enabled)
		})
	}
}

func TestTunnelSettings(t *testing.T) {
	settings := TunnelSettings{
		WarpRouting: &WarpRoutingConfig{
			Enabled: true,
		},
		FallbackTarget: "http_status:404",
	}

	assert.NotNil(t, settings.WarpRouting)
	assert.True(t, settings.WarpRouting.Enabled)
	assert.Equal(t, "http_status:404", settings.FallbackTarget)
}

func TestTunnelSettingsWithGlobalOriginRequest(t *testing.T) {
	connectTimeout := 30 * time.Second

	settings := TunnelSettings{
		GlobalOriginRequest: &OriginRequestConfig{
			ConnectTimeout: &connectTimeout,
		},
	}

	assert.NotNil(t, settings.GlobalOriginRequest)
	assert.Equal(t, 30*time.Second, *settings.GlobalOriginRequest.ConnectTimeout)
}

func TestTunnelConfig(t *testing.T) {
	config := TunnelConfig{
		Settings: &TunnelSettings{
			WarpRouting: &WarpRoutingConfig{
				Enabled: true,
			},
		},
		Rules: []IngressRule{
			{
				Hostname: "app1.example.com",
				Service:  "http://app1.default.svc:80",
			},
			{
				Hostname: "app2.example.com",
				Service:  "http://app2.default.svc:80",
			},
		},
	}

	assert.NotNil(t, config.Settings)
	assert.True(t, config.Settings.WarpRouting.Enabled)
	assert.Len(t, config.Rules, 2)
}

func TestRegisterSettingsOptions(t *testing.T) {
	opts := RegisterSettingsOptions{
		TunnelID:  "tunnel-123",
		AccountID: "account-456",
		Source: service.Source{
			Kind:      "Tunnel",
			Namespace: "default",
			Name:      "my-tunnel",
		},
		Settings: TunnelSettings{
			FallbackTarget: "http_status:503",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "creds"},
	}

	assert.Equal(t, "tunnel-123", opts.TunnelID)
	assert.Equal(t, "account-456", opts.AccountID)
	assert.Equal(t, "Tunnel", opts.Source.Kind)
	assert.Equal(t, "default", opts.Source.Namespace)
	assert.Equal(t, "my-tunnel", opts.Source.Name)
	assert.Equal(t, "http_status:503", opts.Settings.FallbackTarget)
}

func TestRegisterRulesOptions(t *testing.T) {
	opts := RegisterRulesOptions{
		TunnelID:  "tunnel-123",
		AccountID: "account-456",
		Source: service.Source{
			Kind:      "Ingress",
			Namespace: "production",
			Name:      "my-ingress",
		},
		Rules: []IngressRule{
			{
				Hostname: "web.example.com",
				Service:  "http://web.production.svc:80",
			},
		},
		Priority:       100,
		CredentialsRef: v1alpha2.CredentialsReference{Name: "creds"},
	}

	assert.Equal(t, "tunnel-123", opts.TunnelID)
	assert.Equal(t, "account-456", opts.AccountID)
	assert.Equal(t, "Ingress", opts.Source.Kind)
	assert.Len(t, opts.Rules, 1)
	assert.Equal(t, 100, opts.Priority)
}

func TestEmptyTunnelConfig(t *testing.T) {
	config := TunnelConfig{}

	assert.Nil(t, config.Settings)
	assert.Empty(t, config.Rules)
}

func TestMultipleIngressRules(t *testing.T) {
	rules := []IngressRule{
		{
			Hostname: "api.example.com",
			Path:     "/v1/*",
			Service:  "http://api-v1.default.svc:8080",
		},
		{
			Hostname: "api.example.com",
			Path:     "/v2/*",
			Service:  "http://api-v2.default.svc:8080",
		},
		{
			Hostname: "web.example.com",
			Service:  "http://web.default.svc:80",
		},
	}

	assert.Len(t, rules, 3)
	assert.Equal(t, "/v1/*", rules[0].Path)
	assert.Equal(t, "/v2/*", rules[1].Path)
	assert.Empty(t, rules[2].Path)
}
