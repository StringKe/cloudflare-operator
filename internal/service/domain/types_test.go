// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

func TestResourceTypeConstants(t *testing.T) {
	assert.Equal(t, v1alpha2.SyncResourceCloudflareDomain, ResourceTypeCloudflareDomain)
	assert.Equal(t, v1alpha2.SyncResourceOriginCACertificate, ResourceTypeOriginCACertificate)
	assert.Equal(t, v1alpha2.SyncResourceDomainRegistration, ResourceTypeDomainRegistration)
}

func TestPriorityConstants(t *testing.T) {
	assert.Equal(t, 100, PriorityCloudflareDomain)
	assert.Equal(t, 100, PriorityOriginCACertificate)
	assert.Equal(t, 100, PriorityDomainRegistration)
}

func TestSSLConfig(t *testing.T) {
	alwaysHTTPS := true
	autoRewrites := true
	opportunistic := false

	config := SSLConfig{
		Mode:                    "full_strict",
		MinVersion:              "1.2",
		AlwaysUseHTTPS:          &alwaysHTTPS,
		AutomaticHTTPSRewrites:  &autoRewrites,
		OpportunisticEncryption: &opportunistic,
	}

	assert.Equal(t, "full_strict", config.Mode)
	assert.Equal(t, "1.2", config.MinVersion)
	assert.True(t, *config.AlwaysUseHTTPS)
	assert.True(t, *config.AutomaticHTTPSRewrites)
	assert.False(t, *config.OpportunisticEncryption)
}

func TestCacheConfig(t *testing.T) {
	devMode := false
	alwaysOnline := true

	config := CacheConfig{
		Level:           "aggressive",
		BrowserTTL:      86400,
		DevelopmentMode: &devMode,
		AlwaysOnline:    &alwaysOnline,
	}

	assert.Equal(t, "aggressive", config.Level)
	assert.Equal(t, 86400, config.BrowserTTL)
	assert.False(t, *config.DevelopmentMode)
	assert.True(t, *config.AlwaysOnline)
}

func TestSecurityConfig(t *testing.T) {
	browserCheck := true
	emailObfuscation := true
	hotlinkProtection := false

	config := SecurityConfig{
		Level:                 "high",
		BrowserIntegrityCheck: &browserCheck,
		EmailObfuscation:      &emailObfuscation,
		HotlinkProtection:     &hotlinkProtection,
	}

	assert.Equal(t, "high", config.Level)
	assert.True(t, *config.BrowserIntegrityCheck)
	assert.True(t, *config.EmailObfuscation)
	assert.False(t, *config.HotlinkProtection)
}

func TestWAFConfig(t *testing.T) {
	enabled := true

	config := WAFConfig{
		Enabled: &enabled,
		RuleGroups: []WAFRuleGroup{
			{ID: "owasp", Mode: "on"},
			{ID: "cloudflare", Mode: "anomaly"},
		},
	}

	assert.True(t, *config.Enabled)
	assert.Len(t, config.RuleGroups, 2)
	assert.Equal(t, "owasp", config.RuleGroups[0].ID)
	assert.Equal(t, "on", config.RuleGroups[0].Mode)
}

func TestPerformanceConfig(t *testing.T) {
	mirage := true
	brotli := true
	earlyHints := true
	http2 := true
	http3 := true
	zeroRTT := true
	rocketLoader := false

	config := PerformanceConfig{
		Minify: &MinifyConfig{
			HTML: boolPtr(true),
			CSS:  boolPtr(true),
			JS:   boolPtr(true),
		},
		Polish:       "lossless",
		Mirage:       &mirage,
		Brotli:       &brotli,
		EarlyHints:   &earlyHints,
		HTTP2:        &http2,
		HTTP3:        &http3,
		ZeroRTT:      &zeroRTT,
		RocketLoader: &rocketLoader,
	}

	assert.NotNil(t, config.Minify)
	assert.True(t, *config.Minify.HTML)
	assert.Equal(t, "lossless", config.Polish)
	assert.True(t, *config.Mirage)
	assert.True(t, *config.Brotli)
	assert.True(t, *config.HTTP3)
	assert.False(t, *config.RocketLoader)
}

func TestMinifyConfig(t *testing.T) {
	config := MinifyConfig{
		HTML: boolPtr(true),
		CSS:  boolPtr(true),
		JS:   boolPtr(false),
	}

	assert.True(t, *config.HTML)
	assert.True(t, *config.CSS)
	assert.False(t, *config.JS)
}

func TestVerificationConfig(t *testing.T) {
	config := VerificationConfig{
		Method: "dns",
		DNSRecord: &DNSVerificationRecord{
			Type:  "TXT",
			Name:  "_cf-custom-hostname.example.com",
			Value: "verification-token-123",
		},
	}

	assert.Equal(t, "dns", config.Method)
	assert.NotNil(t, config.DNSRecord)
	assert.Equal(t, "TXT", config.DNSRecord.Type)
}

func TestCloudflareDomainConfig(t *testing.T) {
	config := CloudflareDomainConfig{
		Domain: "example.com",
		SSL: &SSLConfig{
			Mode: "full_strict",
		},
		Cache: &CacheConfig{
			Level: "aggressive",
		},
		Security: &SecurityConfig{
			Level: "high",
		},
		Performance: &PerformanceConfig{
			Polish: "lossy",
		},
	}

	assert.Equal(t, "example.com", config.Domain)
	assert.NotNil(t, config.SSL)
	assert.NotNil(t, config.Cache)
	assert.NotNil(t, config.Security)
	assert.NotNil(t, config.Performance)
}

func TestOriginCACertificateConfig(t *testing.T) {
	config := OriginCACertificateConfig{
		Hostnames:    []string{"*.example.com", "example.com"},
		RequestType:  "origin-rsa",
		ValidityDays: 365,
	}

	assert.Len(t, config.Hostnames, 2)
	assert.Equal(t, "origin-rsa", config.RequestType)
	assert.Equal(t, 365, config.ValidityDays)
}

func TestOriginCACertificateConfigWithCSR(t *testing.T) {
	config := OriginCACertificateConfig{
		Hostnames:    []string{"secure.example.com"},
		RequestType:  "origin-ecc",
		ValidityDays: 730,
		CSR:          "-----BEGIN CERTIFICATE REQUEST-----\n...\n-----END CERTIFICATE REQUEST-----",
	}

	assert.NotEmpty(t, config.CSR)
	assert.Equal(t, "origin-ecc", config.RequestType)
}

func TestCloudflareDomainRegisterOptions(t *testing.T) {
	opts := CloudflareDomainRegisterOptions{
		AccountID: "account-123",
		ZoneID:    "zone-456",
		Source: service.Source{
			Kind: "CloudflareDomain",
			Name: "my-domain",
		},
		Config: CloudflareDomainConfig{
			Domain: "example.com",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "zone-456", opts.ZoneID)
	assert.Equal(t, "CloudflareDomain", opts.Source.Kind)
	assert.Equal(t, "example.com", opts.Config.Domain)
}

func TestOriginCACertificateRegisterOptions(t *testing.T) {
	opts := OriginCACertificateRegisterOptions{
		AccountID:     "account-123",
		ZoneID:        "zone-456",
		CertificateID: "cert-789",
		Source: service.Source{
			Kind:      "OriginCACertificate",
			Namespace: "default",
			Name:      "my-cert",
		},
		Config: OriginCACertificateConfig{
			Hostnames:    []string{"*.example.com"},
			RequestType:  "origin-rsa",
			ValidityDays: 365,
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "cert-789", opts.CertificateID)
	assert.Equal(t, "default", opts.Source.Namespace)
}

func TestCloudflareDomainSyncResult(t *testing.T) {
	result := CloudflareDomainSyncResult{
		ZoneID:   "zone-123",
		ZoneName: "example.com",
		Status:   "active",
	}

	assert.Equal(t, "zone-123", result.ZoneID)
	assert.Equal(t, "example.com", result.ZoneName)
	assert.Equal(t, "active", result.Status)
}

func TestOriginCACertificateSyncResult(t *testing.T) {
	expiresAt := metav1.NewTime(time.Now().Add(365 * 24 * time.Hour))

	result := OriginCACertificateSyncResult{
		CertificateID: "cert-123",
		ExpiresAt:     &expiresAt,
		Certificate:   "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
		PrivateKey:    "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----",
	}

	assert.Equal(t, "cert-123", result.CertificateID)
	assert.NotNil(t, result.ExpiresAt)
	assert.NotEmpty(t, result.Certificate)
	assert.NotEmpty(t, result.PrivateKey)
}

func TestSSLModes(t *testing.T) {
	modes := []string{"off", "flexible", "full", "full_strict"}

	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			config := SSLConfig{Mode: mode}
			assert.Equal(t, mode, config.Mode)
		})
	}
}

func TestSecurityLevels(t *testing.T) {
	levels := []string{"essentially_off", "low", "medium", "high", "under_attack"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			config := SecurityConfig{Level: level}
			assert.Equal(t, level, config.Level)
		})
	}
}

// Helper function
func boolPtr(b bool) *bool {
	return &b
}
