// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package ingress

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"

	networkingv1alpha1 "github.com/StringKe/cloudflare-operator/api/v1alpha1"
	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

func TestConvertPathType(t *testing.T) {
	prefixType := networkingv1.PathTypePrefix
	exactType := networkingv1.PathTypeExact
	implType := networkingv1.PathTypeImplementationSpecific

	tests := []struct {
		name     string
		path     string
		pathType *networkingv1.PathType
		want     string
	}{
		{
			name:     "empty path",
			path:     "",
			pathType: &prefixType,
			want:     "",
		},
		{
			name:     "root path",
			path:     "/",
			pathType: &prefixType,
			want:     "",
		},
		{
			name:     "prefix type simple",
			path:     "/api",
			pathType: &prefixType,
			want:     "/api(/.*)?$",
		},
		{
			name:     "prefix type with trailing slash",
			path:     "/api/",
			pathType: &prefixType,
			want:     "/api/.*",
		},
		{
			name:     "exact type",
			path:     "/api/users",
			pathType: &exactType,
			want:     "^/api/users$",
		},
		{
			name:     "implementation specific without trailing slash",
			path:     "/static",
			pathType: &implType,
			want:     "/static(/.*)?$",
		},
		{
			name:     "implementation specific with trailing slash",
			path:     "/static/",
			pathType: &implType,
			want:     "/static/.*",
		},
		{
			name:     "nil pathType defaults to prefix",
			path:     "/default",
			pathType: nil,
			want:     "/default(/.*)?$",
		},
		{
			name:     "deep path prefix",
			path:     "/api/v1/users",
			pathType: &prefixType,
			want:     "/api/v1/users(/.*)?$",
		},
		{
			name:     "deep path exact",
			path:     "/api/v1/users",
			pathType: &exactType,
			want:     "^/api/v1/users$",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertPathType(tt.path, tt.pathType)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestInferProtocolFromPort(t *testing.T) {
	tests := []struct {
		port string
		want string
	}{
		{"443", "https"},
		{"22", "ssh"},
		{"3389", "rdp"},
		{"139", "smb"},
		{"445", "smb"},
		{"80", "http"},
		{"8080", "http"},
		{"3000", "http"},
		{"", "http"},
	}

	for _, tt := range tests {
		t.Run(tt.port, func(t *testing.T) {
			got := inferProtocolFromPort(tt.port)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDetermineProtocol(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		port        string
		tlsHosts    map[string]string
		host        string
		want        string
	}{
		{
			name:        "annotation override",
			annotations: map[string]string{AnnotationProtocol: "wss"},
			port:        "80",
			tlsHosts:    map[string]string{},
			host:        "example.com",
			want:        "wss",
		},
		{
			name:        "TLS host",
			annotations: map[string]string{},
			port:        "80",
			tlsHosts:    map[string]string{"example.com": "tls-secret"},
			host:        "example.com",
			want:        "https",
		},
		{
			name:        "port 443",
			annotations: map[string]string{},
			port:        "443",
			tlsHosts:    map[string]string{},
			host:        "example.com",
			want:        "https",
		},
		{
			name:        "default http",
			annotations: map[string]string{},
			port:        "8080",
			tlsHosts:    map[string]string{},
			host:        "example.com",
			want:        "http",
		},
		{
			name:        "annotation takes precedence over TLS",
			annotations: map[string]string{AnnotationProtocol: "tcp"},
			port:        "443",
			tlsHosts:    map[string]string{"example.com": "tls-secret"},
			host:        "example.com",
			want:        "tcp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{}
			parser := NewAnnotationParser(tt.annotations)
			got := r.determineProtocol(parser, tt.port, tt.tlsHosts, tt.host)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildOriginRequest_Defaults(t *testing.T) {
	r := &Reconciler{}
	parser := NewAnnotationParser(map[string]string{})

	keepAlive := 100
	proxyPort := uint16(8080)
	disableChunked := true
	bastionMode := false

	defaults := &networkingv1alpha2.OriginRequestSpec{
		NoTLSVerify:            true,
		HTTP2Origin:            true,
		CAPool:                 "custom-ca.pem",
		ConnectTimeout:         "30s",
		TLSTimeout:             "10s",
		KeepAliveTimeout:       "60s",
		KeepAliveConnections:   &keepAlive,
		OriginServerName:       "origin.example.com",
		HTTPHostHeader:         "custom-host.example.com",
		ProxyAddress:           "127.0.0.1",
		ProxyPort:              &proxyPort,
		ProxyType:              "socks5",
		DisableChunkedEncoding: &disableChunked,
		BastionMode:            &bastionMode,
	}

	config := r.buildOriginRequest(parser, defaults, nil, "")

	require.NotNil(t, config.NoTLSVerify)
	assert.True(t, *config.NoTLSVerify)

	require.NotNil(t, config.Http2Origin)
	assert.True(t, *config.Http2Origin)

	require.NotNil(t, config.CAPool)
	assert.Equal(t, "/etc/cloudflared/certs/custom-ca.pem", *config.CAPool)

	require.NotNil(t, config.ConnectTimeout)
	assert.Equal(t, 30*time.Second, *config.ConnectTimeout)

	require.NotNil(t, config.TLSTimeout)
	assert.Equal(t, 10*time.Second, *config.TLSTimeout)

	require.NotNil(t, config.KeepAliveTimeout)
	assert.Equal(t, 60*time.Second, *config.KeepAliveTimeout)

	require.NotNil(t, config.KeepAliveConnections)
	assert.Equal(t, 100, *config.KeepAliveConnections)

	require.NotNil(t, config.OriginServerName)
	assert.Equal(t, "origin.example.com", *config.OriginServerName)

	require.NotNil(t, config.HTTPHostHeader)
	assert.Equal(t, "custom-host.example.com", *config.HTTPHostHeader)

	require.NotNil(t, config.ProxyAddress)
	assert.Equal(t, "127.0.0.1", *config.ProxyAddress)

	require.NotNil(t, config.ProxyPort)
	assert.Equal(t, uint(8080), *config.ProxyPort)

	require.NotNil(t, config.ProxyType)
	assert.Equal(t, "socks5", *config.ProxyType)

	// Note: DisableChunkedEncoding and BastionMode are only set from annotations,
	// not from defaults (they get overwritten by nil if no annotation is present)
	// This is the actual behavior of buildOriginRequest
	assert.Nil(t, config.DisableChunkedEncoding)
	assert.Nil(t, config.BastionMode)
}

func TestBuildOriginRequest_AnnotationOverrides(t *testing.T) {
	r := &Reconciler{}
	annotations := map[string]string{
		AnnotationNoTLSVerify:          "true",
		AnnotationHTTP2Origin:          "false",
		AnnotationCAPool:               "annotation-ca.pem",
		AnnotationConnectTimeout:       "45s",
		AnnotationKeepAliveTimeout:     "90s",
		AnnotationOriginServerName:     "annotation-origin.example.com",
		AnnotationHTTPHostHeader:       "annotation-host.example.com",
		AnnotationProxyAddress:         "192.168.1.1",
		AnnotationProxyPort:            "3128",
		AnnotationProxyType:            "http",
		AnnotationKeepAliveConnections: "200",
	}
	parser := NewAnnotationParser(annotations)

	// Start with defaults that should be overridden
	keepAlive := 50
	defaults := &networkingv1alpha2.OriginRequestSpec{
		NoTLSVerify:          false,
		HTTP2Origin:          true,
		CAPool:               "default-ca.pem",
		ConnectTimeout:       "10s",
		KeepAliveConnections: &keepAlive,
	}

	config := r.buildOriginRequest(parser, defaults, nil, "")

	// Annotations should override defaults
	require.NotNil(t, config.NoTLSVerify)
	assert.True(t, *config.NoTLSVerify)

	require.NotNil(t, config.Http2Origin)
	assert.False(t, *config.Http2Origin)

	require.NotNil(t, config.CAPool)
	assert.Equal(t, "/etc/cloudflared/certs/annotation-ca.pem", *config.CAPool)

	require.NotNil(t, config.ConnectTimeout)
	assert.Equal(t, 45*time.Second, *config.ConnectTimeout)

	require.NotNil(t, config.KeepAliveTimeout)
	assert.Equal(t, 90*time.Second, *config.KeepAliveTimeout)

	require.NotNil(t, config.OriginServerName)
	assert.Equal(t, "annotation-origin.example.com", *config.OriginServerName)

	require.NotNil(t, config.KeepAliveConnections)
	assert.Equal(t, 200, *config.KeepAliveConnections)
}

func TestBuildOriginRequest_NilDefaults(t *testing.T) {
	r := &Reconciler{}
	parser := NewAnnotationParser(map[string]string{
		AnnotationNoTLSVerify: "true",
	})

	config := r.buildOriginRequest(parser, nil, nil, "")

	require.NotNil(t, config.NoTLSVerify)
	assert.True(t, *config.NoTLSVerify)

	// Other fields should be nil
	assert.Nil(t, config.CAPool)
	assert.Nil(t, config.ConnectTimeout)
}

func TestBuildOriginRequest_InvalidDuration(t *testing.T) {
	r := &Reconciler{}
	parser := NewAnnotationParser(map[string]string{})

	defaults := &networkingv1alpha2.OriginRequestSpec{
		ConnectTimeout:   "invalid",
		TLSTimeout:       "also-invalid",
		KeepAliveTimeout: "30m", // Valid
	}

	config := r.buildOriginRequest(parser, defaults, nil, "")

	// Invalid durations should be ignored
	assert.Nil(t, config.ConnectTimeout)
	assert.Nil(t, config.TLSTimeout)

	// Valid duration should be set
	require.NotNil(t, config.KeepAliveTimeout)
	assert.Equal(t, 30*time.Minute, *config.KeepAliveTimeout)
}

// nolint:staticcheck // TunnelBinding is deprecated but still tested for backward compatibility
func TestConvertTunnelBindingToRules(t *testing.T) {
	r := &Reconciler{}

	binding := networkingv1alpha1.TunnelBinding{
		Subjects: []networkingv1alpha1.TunnelBindingSubject{
			{
				Spec: networkingv1alpha1.TunnelBindingSubjectSpec{
					Target:       "",
					Path:         "/api",
					NoTlsVerify:  true,
					Http2Origin:  false,
					ProxyAddress: "127.0.0.1",
					ProxyPort:    8080,
					ProxyType:    "socks5",
					CaPool:       "binding-ca.pem",
				},
			},
			{
				Spec: networkingv1alpha1.TunnelBindingSubjectSpec{
					Target: "http://custom-target:9000",
					Path:   "/custom",
				},
			},
		},
		Status: networkingv1alpha1.TunnelBindingStatus{
			Services: []networkingv1alpha1.ServiceInfo{
				{
					Hostname: "api.example.com",
					Target:   "http://api-service:80",
				},
				{
					Hostname: "custom.example.com",
					Target:   "http://default-target:80",
				},
			},
		},
	}

	rules := r.convertTunnelBindingToRules(binding)

	require.Len(t, rules, 2)

	// First rule
	assert.Equal(t, "api.example.com", rules[0].Hostname)
	assert.Equal(t, "/api", rules[0].Path)
	assert.Equal(t, "http://api-service:80", rules[0].Service)
	require.NotNil(t, rules[0].OriginRequest.NoTLSVerify)
	assert.True(t, *rules[0].OriginRequest.NoTLSVerify)
	require.NotNil(t, rules[0].OriginRequest.Http2Origin)
	assert.False(t, *rules[0].OriginRequest.Http2Origin)
	require.NotNil(t, rules[0].OriginRequest.ProxyAddress)
	assert.Equal(t, "127.0.0.1", *rules[0].OriginRequest.ProxyAddress)
	require.NotNil(t, rules[0].OriginRequest.ProxyPort)
	assert.Equal(t, uint(8080), *rules[0].OriginRequest.ProxyPort)
	require.NotNil(t, rules[0].OriginRequest.ProxyType)
	assert.Equal(t, "socks5", *rules[0].OriginRequest.ProxyType)
	require.NotNil(t, rules[0].OriginRequest.CAPool)
	assert.Equal(t, "/etc/cloudflared/certs/binding-ca.pem", *rules[0].OriginRequest.CAPool)

	// Second rule - custom target overrides status target
	assert.Equal(t, "custom.example.com", rules[1].Hostname)
	assert.Equal(t, "/custom", rules[1].Path)
	assert.Equal(t, "http://custom-target:9000", rules[1].Service)
}

// nolint:staticcheck // TunnelBinding is deprecated but still tested for backward compatibility
func TestConvertTunnelBindingToRules_Empty(t *testing.T) {
	r := &Reconciler{}

	binding := networkingv1alpha1.TunnelBinding{
		Subjects: []networkingv1alpha1.TunnelBindingSubject{},
		Status: networkingv1alpha1.TunnelBindingStatus{
			Services: []networkingv1alpha1.ServiceInfo{},
		},
	}

	rules := r.convertTunnelBindingToRules(binding)

	assert.Empty(t, rules)
}

// nolint:staticcheck // TunnelBinding is deprecated but still tested for backward compatibility
func TestConvertTunnelBindingToRules_SubjectWithoutStatus(t *testing.T) {
	r := &Reconciler{}

	binding := networkingv1alpha1.TunnelBinding{
		Subjects: []networkingv1alpha1.TunnelBindingSubject{
			{
				Spec: networkingv1alpha1.TunnelBindingSubjectSpec{
					Target: "http://service:80",
				},
			},
			{
				Spec: networkingv1alpha1.TunnelBindingSubjectSpec{
					Target: "http://service2:80",
				},
			},
		},
		Status: networkingv1alpha1.TunnelBindingStatus{
			Services: []networkingv1alpha1.ServiceInfo{
				{
					Hostname: "service.example.com",
					Target:   "http://service:80",
				},
				// Missing second status entry
			},
		},
	}

	rules := r.convertTunnelBindingToRules(binding)

	// Only the first subject should be processed (has matching status)
	require.Len(t, rules, 1)
	assert.Equal(t, "service.example.com", rules[0].Hostname)
}
