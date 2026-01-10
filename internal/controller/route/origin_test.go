// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package route

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

func TestNewOriginRequestBuilder(t *testing.T) {
	builder := NewOriginRequestBuilder()

	require.NotNil(t, builder)
	config := builder.Build()
	assert.Nil(t, config.NoTLSVerify)
	assert.Nil(t, config.Http2Origin)
	assert.Nil(t, config.CAPool)
}

func TestOriginRequestBuilder_WithDefaults_Nil(t *testing.T) {
	builder := NewOriginRequestBuilder()

	result := builder.WithDefaults(nil)

	assert.Same(t, builder, result) // Returns self for chaining
	config := builder.Build()
	assert.Nil(t, config.NoTLSVerify)
}

func TestOriginRequestBuilder_WithDefaults_AllFields(t *testing.T) {
	builder := NewOriginRequestBuilder()

	keepAliveConns := 100
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
		KeepAliveConnections:   &keepAliveConns,
		OriginServerName:       "origin.example.com",
		HTTPHostHeader:         "custom-host.example.com",
		ProxyAddress:           "127.0.0.1",
		ProxyPort:              &proxyPort,
		ProxyType:              "socks5",
		DisableChunkedEncoding: &disableChunked,
		BastionMode:            &bastionMode,
	}

	builder.WithDefaults(defaults)
	config := builder.Build()

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

	require.NotNil(t, config.DisableChunkedEncoding)
	assert.True(t, *config.DisableChunkedEncoding)

	require.NotNil(t, config.BastionMode)
	assert.False(t, *config.BastionMode)
}

func TestOriginRequestBuilder_WithDefaults_PartialFields(t *testing.T) {
	builder := NewOriginRequestBuilder()

	defaults := &networkingv1alpha2.OriginRequestSpec{
		NoTLSVerify:      true,
		HTTP2Origin:      false,
		CAPool:           "", // Empty - should not set
		OriginServerName: "partial.example.com",
	}

	builder.WithDefaults(defaults)
	config := builder.Build()

	require.NotNil(t, config.NoTLSVerify)
	assert.True(t, *config.NoTLSVerify)

	require.NotNil(t, config.Http2Origin)
	assert.False(t, *config.Http2Origin)

	assert.Nil(t, config.CAPool)

	require.NotNil(t, config.OriginServerName)
	assert.Equal(t, "partial.example.com", *config.OriginServerName)
}

func TestOriginRequestBuilder_WithDefaults_InvalidDuration(t *testing.T) {
	builder := NewOriginRequestBuilder()

	defaults := &networkingv1alpha2.OriginRequestSpec{
		ConnectTimeout: "invalid-duration",
		TLSTimeout:     "also-invalid",
	}

	builder.WithDefaults(defaults)
	config := builder.Build()

	// Invalid durations should be silently ignored
	assert.Nil(t, config.ConnectTimeout)
	assert.Nil(t, config.TLSTimeout)
}

func TestOriginRequestBuilder_SetNoTLSVerify(t *testing.T) {
	builder := NewOriginRequestBuilder()

	// Nil should not change config
	builder.SetNoTLSVerify(nil)
	assert.Nil(t, builder.Build().NoTLSVerify)

	// True value
	trueVal := true
	builder.SetNoTLSVerify(&trueVal)
	require.NotNil(t, builder.Build().NoTLSVerify)
	assert.True(t, *builder.Build().NoTLSVerify)

	// False value
	falseVal := false
	builder.SetNoTLSVerify(&falseVal)
	require.NotNil(t, builder.Build().NoTLSVerify)
	assert.False(t, *builder.Build().NoTLSVerify)
}

func TestOriginRequestBuilder_SetHTTP2Origin(t *testing.T) {
	builder := NewOriginRequestBuilder()

	builder.SetHTTP2Origin(nil)
	assert.Nil(t, builder.Build().Http2Origin)

	trueVal := true
	builder.SetHTTP2Origin(&trueVal)
	require.NotNil(t, builder.Build().Http2Origin)
	assert.True(t, *builder.Build().Http2Origin)
}

func TestOriginRequestBuilder_SetCAPool(t *testing.T) {
	builder := NewOriginRequestBuilder()

	// Empty should not set
	builder.SetCAPool("")
	assert.Nil(t, builder.Build().CAPool)

	// Non-empty value
	builder.SetCAPool("my-ca.pem")
	require.NotNil(t, builder.Build().CAPool)
	assert.Equal(t, "/etc/cloudflared/certs/my-ca.pem", *builder.Build().CAPool)
}

func TestOriginRequestBuilder_SetConnectTimeout(t *testing.T) {
	builder := NewOriginRequestBuilder()

	builder.SetConnectTimeout(nil)
	assert.Nil(t, builder.Build().ConnectTimeout)

	d := 45 * time.Second
	builder.SetConnectTimeout(&d)
	require.NotNil(t, builder.Build().ConnectTimeout)
	assert.Equal(t, 45*time.Second, *builder.Build().ConnectTimeout)
}

func TestOriginRequestBuilder_SetTLSTimeout(t *testing.T) {
	builder := NewOriginRequestBuilder()

	builder.SetTLSTimeout(nil)
	assert.Nil(t, builder.Build().TLSTimeout)

	d := 15 * time.Second
	builder.SetTLSTimeout(&d)
	require.NotNil(t, builder.Build().TLSTimeout)
	assert.Equal(t, 15*time.Second, *builder.Build().TLSTimeout)
}

func TestOriginRequestBuilder_SetKeepAliveTimeout(t *testing.T) {
	builder := NewOriginRequestBuilder()

	builder.SetKeepAliveTimeout(nil)
	assert.Nil(t, builder.Build().KeepAliveTimeout)

	d := 120 * time.Second
	builder.SetKeepAliveTimeout(&d)
	require.NotNil(t, builder.Build().KeepAliveTimeout)
	assert.Equal(t, 120*time.Second, *builder.Build().KeepAliveTimeout)
}

func TestOriginRequestBuilder_SetKeepAliveConnections(t *testing.T) {
	builder := NewOriginRequestBuilder()

	builder.SetKeepAliveConnections(nil)
	assert.Nil(t, builder.Build().KeepAliveConnections)

	n := 50
	builder.SetKeepAliveConnections(&n)
	require.NotNil(t, builder.Build().KeepAliveConnections)
	assert.Equal(t, 50, *builder.Build().KeepAliveConnections)
}

func TestOriginRequestBuilder_SetOriginServerName(t *testing.T) {
	builder := NewOriginRequestBuilder()

	builder.SetOriginServerName("")
	assert.Nil(t, builder.Build().OriginServerName)

	builder.SetOriginServerName("origin.example.com")
	require.NotNil(t, builder.Build().OriginServerName)
	assert.Equal(t, "origin.example.com", *builder.Build().OriginServerName)
}

func TestOriginRequestBuilder_SetHTTPHostHeader(t *testing.T) {
	builder := NewOriginRequestBuilder()

	builder.SetHTTPHostHeader("")
	assert.Nil(t, builder.Build().HTTPHostHeader)

	builder.SetHTTPHostHeader("custom-host.example.com")
	require.NotNil(t, builder.Build().HTTPHostHeader)
	assert.Equal(t, "custom-host.example.com", *builder.Build().HTTPHostHeader)
}

func TestOriginRequestBuilder_SetProxyAddress(t *testing.T) {
	builder := NewOriginRequestBuilder()

	builder.SetProxyAddress("")
	assert.Nil(t, builder.Build().ProxyAddress)

	builder.SetProxyAddress("192.168.1.1")
	require.NotNil(t, builder.Build().ProxyAddress)
	assert.Equal(t, "192.168.1.1", *builder.Build().ProxyAddress)
}

func TestOriginRequestBuilder_SetProxyPort(t *testing.T) {
	builder := NewOriginRequestBuilder()

	// 0 should not set
	builder.SetProxyPort(0)
	assert.Nil(t, builder.Build().ProxyPort)

	builder.SetProxyPort(3128)
	require.NotNil(t, builder.Build().ProxyPort)
	assert.Equal(t, uint(3128), *builder.Build().ProxyPort)
}

func TestOriginRequestBuilder_SetProxyType(t *testing.T) {
	builder := NewOriginRequestBuilder()

	builder.SetProxyType("")
	assert.Nil(t, builder.Build().ProxyType)

	builder.SetProxyType("socks5")
	require.NotNil(t, builder.Build().ProxyType)
	assert.Equal(t, "socks5", *builder.Build().ProxyType)
}

func TestOriginRequestBuilder_SetDisableChunkedEncoding(t *testing.T) {
	builder := NewOriginRequestBuilder()

	builder.SetDisableChunkedEncoding(nil)
	assert.Nil(t, builder.Build().DisableChunkedEncoding)

	trueVal := true
	builder.SetDisableChunkedEncoding(&trueVal)
	require.NotNil(t, builder.Build().DisableChunkedEncoding)
	assert.True(t, *builder.Build().DisableChunkedEncoding)
}

func TestOriginRequestBuilder_SetBastionMode(t *testing.T) {
	builder := NewOriginRequestBuilder()

	builder.SetBastionMode(nil)
	assert.Nil(t, builder.Build().BastionMode)

	trueVal := true
	builder.SetBastionMode(&trueVal)
	require.NotNil(t, builder.Build().BastionMode)
	assert.True(t, *builder.Build().BastionMode)
}

func TestOriginRequestBuilder_Chaining(t *testing.T) {
	trueVal := true
	d := 30 * time.Second

	config := NewOriginRequestBuilder().
		SetNoTLSVerify(&trueVal).
		SetHTTP2Origin(&trueVal).
		SetCAPool("chain-ca.pem").
		SetConnectTimeout(&d).
		SetOriginServerName("chain.example.com").
		SetProxyPort(8080).
		Build()

	require.NotNil(t, config.NoTLSVerify)
	assert.True(t, *config.NoTLSVerify)

	require.NotNil(t, config.Http2Origin)
	assert.True(t, *config.Http2Origin)

	require.NotNil(t, config.CAPool)
	assert.Contains(t, *config.CAPool, "chain-ca.pem")

	require.NotNil(t, config.ConnectTimeout)
	assert.Equal(t, 30*time.Second, *config.ConnectTimeout)

	require.NotNil(t, config.OriginServerName)
	assert.Equal(t, "chain.example.com", *config.OriginServerName)

	require.NotNil(t, config.ProxyPort)
	assert.Equal(t, uint(8080), *config.ProxyPort)
}
