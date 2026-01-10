// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package ingress

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAnnotationConstants(t *testing.T) {
	// Verify annotation prefix
	assert.Equal(t, "cloudflare.com/", AnnotationPrefix)

	// Protocol annotations
	assert.Equal(t, "cloudflare.com/protocol", AnnotationProtocol)
	assert.Equal(t, "cloudflare.com/no-tls-verify", AnnotationNoTLSVerify)
	assert.Equal(t, "cloudflare.com/http2-origin", AnnotationHTTP2Origin)
	assert.Equal(t, "cloudflare.com/ca-pool", AnnotationCAPool)

	// Proxy annotations
	assert.Equal(t, "cloudflare.com/proxy-address", AnnotationProxyAddress)
	assert.Equal(t, "cloudflare.com/proxy-port", AnnotationProxyPort)
	assert.Equal(t, "cloudflare.com/proxy-type", AnnotationProxyType)

	// Connection settings
	assert.Equal(t, "cloudflare.com/connect-timeout", AnnotationConnectTimeout)
	assert.Equal(t, "cloudflare.com/tls-timeout", AnnotationTLSTimeout)
	assert.Equal(t, "cloudflare.com/keep-alive-timeout", AnnotationKeepAliveTimeout)
	assert.Equal(t, "cloudflare.com/keep-alive-connections", AnnotationKeepAliveConnections)

	// Origin header settings
	assert.Equal(t, "cloudflare.com/origin-server-name", AnnotationOriginServerName)
	assert.Equal(t, "cloudflare.com/http-host-header", AnnotationHTTPHostHeader)

	// DNS annotations
	assert.Equal(t, "cloudflare.com/disable-dns", AnnotationDisableDNS)
	assert.Equal(t, "cloudflare.com/dns-proxied", AnnotationDNSProxied)

	// Advanced settings
	assert.Equal(t, "cloudflare.com/disable-chunked-encoding", AnnotationDisableChunkedEncoding)
	assert.Equal(t, "cloudflare.com/bastion-mode", AnnotationBastionMode)
}

func TestNewAnnotationParser(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		wantNil     bool
	}{
		{
			name:        "with annotations",
			annotations: map[string]string{"key": "value"},
			wantNil:     false,
		},
		{
			name:        "nil annotations",
			annotations: nil,
			wantNil:     false,
		},
		{
			name:        "empty annotations",
			annotations: map[string]string{},
			wantNil:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewAnnotationParser(tt.annotations)
			assert.NotNil(t, parser)
		})
	}
}

func TestAnnotationParserGetString(t *testing.T) {
	annotations := map[string]string{
		"cloudflare.com/protocol": "https",
		"cloudflare.com/ca-pool":  "my-ca-secret",
		"empty":                   "",
	}
	parser := NewAnnotationParser(annotations)

	tests := []struct {
		name   string
		key    string
		want   string
		wantOk bool
	}{
		{
			name:   "existing key",
			key:    "cloudflare.com/protocol",
			want:   "https",
			wantOk: true,
		},
		{
			name:   "non-existing key",
			key:    "cloudflare.com/missing",
			want:   "",
			wantOk: false,
		},
		{
			name:   "empty value",
			key:    "empty",
			want:   "",
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parser.GetString(tt.key)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantOk, ok)
		})
	}
}

func TestAnnotationParserGetBool(t *testing.T) {
	annotations := map[string]string{
		"true-key":    "true",
		"false-key":   "false",
		"True-key":    "True",
		"FALSE-key":   "FALSE",
		"one-key":     "1",
		"zero-key":    "0",
		"invalid-key": "invalid",
		"empty-key":   "",
	}
	parser := NewAnnotationParser(annotations)

	tests := []struct {
		name   string
		key    string
		want   bool
		wantOk bool
	}{
		{
			name:   "true string",
			key:    "true-key",
			want:   true,
			wantOk: true,
		},
		{
			name:   "false string",
			key:    "false-key",
			want:   false,
			wantOk: true,
		},
		{
			name:   "True (capitalized)",
			key:    "True-key",
			want:   true,
			wantOk: true,
		},
		{
			name:   "FALSE (all caps)",
			key:    "FALSE-key",
			want:   false,
			wantOk: true,
		},
		{
			name:   "1 as true",
			key:    "one-key",
			want:   true,
			wantOk: true,
		},
		{
			name:   "0 as false",
			key:    "zero-key",
			want:   false,
			wantOk: true,
		},
		{
			name:   "invalid value",
			key:    "invalid-key",
			want:   false,
			wantOk: false,
		},
		{
			name:   "empty value",
			key:    "empty-key",
			want:   false,
			wantOk: false,
		},
		{
			name:   "missing key",
			key:    "missing-key",
			want:   false,
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parser.GetBool(tt.key)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantOk, ok)
		})
	}
}

func TestAnnotationParserGetBoolPtr(t *testing.T) {
	annotations := map[string]string{
		"true-key":  "true",
		"false-key": "false",
	}
	parser := NewAnnotationParser(annotations)

	tests := []struct {
		name    string
		key     string
		wantNil bool
		want    bool
	}{
		{
			name:    "true value returns pointer",
			key:     "true-key",
			wantNil: false,
			want:    true,
		},
		{
			name:    "false value returns pointer",
			key:     "false-key",
			wantNil: false,
			want:    false,
		},
		{
			name:    "missing key returns nil",
			key:     "missing-key",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parser.GetBoolPtr(tt.key)
			if tt.wantNil {
				assert.Nil(t, got)
			} else {
				assert.NotNil(t, got)
				assert.Equal(t, tt.want, *got)
			}
		})
	}
}

func TestAnnotationParserGetInt(t *testing.T) {
	annotations := map[string]string{
		"valid-int":    "42",
		"negative-int": "-10",
		"zero":         "0",
		"invalid":      "not-a-number",
		"float":        "3.14",
		"empty":        "",
	}
	parser := NewAnnotationParser(annotations)

	tests := []struct {
		name   string
		key    string
		want   int
		wantOk bool
	}{
		{
			name:   "valid integer",
			key:    "valid-int",
			want:   42,
			wantOk: true,
		},
		{
			name:   "negative integer",
			key:    "negative-int",
			want:   -10,
			wantOk: true,
		},
		{
			name:   "zero",
			key:    "zero",
			want:   0,
			wantOk: true,
		},
		{
			name:   "invalid string",
			key:    "invalid",
			want:   0,
			wantOk: false,
		},
		{
			name:   "float string",
			key:    "float",
			want:   0,
			wantOk: false,
		},
		{
			name:   "empty string",
			key:    "empty",
			want:   0,
			wantOk: false,
		},
		{
			name:   "missing key",
			key:    "missing",
			want:   0,
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parser.GetInt(tt.key)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantOk, ok)
		})
	}
}

func TestAnnotationParserGetUint16(t *testing.T) {
	annotations := map[string]string{
		"valid":    "8080",
		"max":      "65535",
		"zero":     "0",
		"too-big":  "70000",
		"invalid":  "not-a-number",
		"negative": "-1",
	}
	parser := NewAnnotationParser(annotations)

	tests := []struct {
		name   string
		key    string
		want   uint16
		wantOk bool
	}{
		{
			name:   "valid port",
			key:    "valid",
			want:   8080,
			wantOk: true,
		},
		{
			name:   "max uint16",
			key:    "max",
			want:   65535,
			wantOk: true,
		},
		{
			name:   "zero",
			key:    "zero",
			want:   0,
			wantOk: true,
		},
		{
			name:   "too big for uint16",
			key:    "too-big",
			want:   0,
			wantOk: false,
		},
		{
			name:   "invalid string",
			key:    "invalid",
			want:   0,
			wantOk: false,
		},
		{
			name:   "negative number",
			key:    "negative",
			want:   0,
			wantOk: false,
		},
		{
			name:   "missing key",
			key:    "missing",
			want:   0,
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parser.GetUint16(tt.key)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantOk, ok)
		})
	}
}

func TestAnnotationParserGetDuration(t *testing.T) {
	annotations := map[string]string{
		"seconds":      "30s",
		"minutes":      "5m",
		"hours":        "2h",
		"milliseconds": "100ms",
		"combined":     "1h30m",
		"invalid":      "not-a-duration",
		"empty":        "",
		"just-number":  "30",
	}
	parser := NewAnnotationParser(annotations)

	tests := []struct {
		name   string
		key    string
		want   time.Duration
		wantOk bool
	}{
		{
			name:   "seconds",
			key:    "seconds",
			want:   30 * time.Second,
			wantOk: true,
		},
		{
			name:   "minutes",
			key:    "minutes",
			want:   5 * time.Minute,
			wantOk: true,
		},
		{
			name:   "hours",
			key:    "hours",
			want:   2 * time.Hour,
			wantOk: true,
		},
		{
			name:   "milliseconds",
			key:    "milliseconds",
			want:   100 * time.Millisecond,
			wantOk: true,
		},
		{
			name:   "combined",
			key:    "combined",
			want:   1*time.Hour + 30*time.Minute,
			wantOk: true,
		},
		{
			name:   "invalid string",
			key:    "invalid",
			want:   0,
			wantOk: false,
		},
		{
			name:   "empty string",
			key:    "empty",
			want:   0,
			wantOk: false,
		},
		{
			name:   "number without unit",
			key:    "just-number",
			want:   0,
			wantOk: false,
		},
		{
			name:   "missing key",
			key:    "missing",
			want:   0,
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parser.GetDuration(tt.key)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantOk, ok)
		})
	}
}

// TestAnnotationParserRealWorldScenarios tests realistic usage patterns
func TestAnnotationParserRealWorldScenarios(t *testing.T) {
	t.Run("typical ingress annotations", func(t *testing.T) {
		annotations := map[string]string{
			AnnotationProtocol:       "https",
			AnnotationNoTLSVerify:    "true",
			AnnotationConnectTimeout: "30s",
			AnnotationDNSProxied:     "true",
		}
		parser := NewAnnotationParser(annotations)

		protocol, ok := parser.GetString(AnnotationProtocol)
		assert.True(t, ok)
		assert.Equal(t, "https", protocol)

		noTLSVerify := parser.GetBoolPtr(AnnotationNoTLSVerify)
		assert.NotNil(t, noTLSVerify)
		assert.True(t, *noTLSVerify)

		timeout, ok := parser.GetDuration(AnnotationConnectTimeout)
		assert.True(t, ok)
		assert.Equal(t, 30*time.Second, timeout)

		proxied, ok := parser.GetBool(AnnotationDNSProxied)
		assert.True(t, ok)
		assert.True(t, proxied)
	})

	t.Run("bastion mode configuration", func(t *testing.T) {
		annotations := map[string]string{
			AnnotationBastionMode:  "true",
			AnnotationProxyAddress: "0.0.0.0",
			AnnotationProxyPort:    "1080",
			AnnotationProxyType:    "socks",
		}
		parser := NewAnnotationParser(annotations)

		bastionMode := parser.GetBoolPtr(AnnotationBastionMode)
		assert.NotNil(t, bastionMode)
		assert.True(t, *bastionMode)

		proxyAddr, ok := parser.GetString(AnnotationProxyAddress)
		assert.True(t, ok)
		assert.Equal(t, "0.0.0.0", proxyAddr)

		proxyPort, ok := parser.GetUint16(AnnotationProxyPort)
		assert.True(t, ok)
		assert.Equal(t, uint16(1080), proxyPort)

		proxyType, ok := parser.GetString(AnnotationProxyType)
		assert.True(t, ok)
		assert.Equal(t, "socks", proxyType)
	})

	t.Run("connection tuning", func(t *testing.T) {
		annotations := map[string]string{
			AnnotationConnectTimeout:       "30s",
			AnnotationTLSTimeout:           "10s",
			AnnotationKeepAliveTimeout:     "90s",
			AnnotationKeepAliveConnections: "100",
		}
		parser := NewAnnotationParser(annotations)

		connectTimeout, ok := parser.GetDuration(AnnotationConnectTimeout)
		assert.True(t, ok)
		assert.Equal(t, 30*time.Second, connectTimeout)

		tlsTimeout, ok := parser.GetDuration(AnnotationTLSTimeout)
		assert.True(t, ok)
		assert.Equal(t, 10*time.Second, tlsTimeout)

		keepAliveTimeout, ok := parser.GetDuration(AnnotationKeepAliveTimeout)
		assert.True(t, ok)
		assert.Equal(t, 90*time.Second, keepAliveTimeout)

		keepAliveConns, ok := parser.GetInt(AnnotationKeepAliveConnections)
		assert.True(t, ok)
		assert.Equal(t, 100, keepAliveConns)
	})
}
