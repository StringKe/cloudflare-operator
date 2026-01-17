// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package domain provides services for managing Cloudflare Domain configurations.
//
//nolint:revive // max-public-structs is acceptable for API type definitions
package domain

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// Resource Types for SyncState
const (
	// ResourceTypeCloudflareDomain is the SyncState resource type for CloudflareDomain
	ResourceTypeCloudflareDomain = v1alpha2.SyncResourceCloudflareDomain
	// ResourceTypeOriginCACertificate is the SyncState resource type for OriginCACertificate
	ResourceTypeOriginCACertificate = v1alpha2.SyncResourceOriginCACertificate
	// ResourceTypeDomainRegistration is the SyncState resource type for DomainRegistration
	ResourceTypeDomainRegistration = v1alpha2.SyncResourceDomainRegistration

	// Priority constants
	PriorityCloudflareDomain    = 100
	PriorityOriginCACertificate = 100
	PriorityDomainRegistration  = 100
)

// CloudflareDomainConfig contains the configuration for a Cloudflare Domain.
type CloudflareDomainConfig struct {
	// Domain is the domain name
	Domain string `json:"domain"`
	// SSL contains SSL/TLS configuration
	SSL *SSLConfig `json:"ssl,omitempty"`
	// Cache contains cache configuration
	Cache *CacheConfig `json:"cache,omitempty"`
	// Security contains security configuration
	Security *SecurityConfig `json:"security,omitempty"`
	// Performance contains performance configuration
	Performance *PerformanceConfig `json:"performance,omitempty"`
	// Verification contains domain verification settings
	Verification *VerificationConfig `json:"verification,omitempty"`
}

// SSLConfig contains SSL/TLS configuration.
type SSLConfig struct {
	// Mode is the SSL mode (off, flexible, full, full_strict)
	Mode string `json:"mode,omitempty"`
	// MinVersion is the minimum TLS version (1.0, 1.1, 1.2, 1.3)
	MinVersion string `json:"minVersion,omitempty"`
	// AlwaysUseHTTPS enables automatic HTTPS redirect
	AlwaysUseHTTPS *bool `json:"alwaysUseHttps,omitempty"`
	// AutomaticHTTPSRewrites enables automatic HTTPS rewrites
	AutomaticHTTPSRewrites *bool `json:"automaticHttpsRewrites,omitempty"`
	// OpportunisticEncryption enables opportunistic encryption
	OpportunisticEncryption *bool `json:"opportunisticEncryption,omitempty"`
}

// CacheConfig contains cache configuration.
type CacheConfig struct {
	// Level is the cache level (aggressive, basic, simplified)
	Level string `json:"level,omitempty"`
	// BrowserTTL is the browser cache TTL in seconds
	BrowserTTL int `json:"browserTtl,omitempty"`
	// DevelopmentMode enables development mode
	DevelopmentMode *bool `json:"developmentMode,omitempty"`
	// AlwaysOnline enables always online
	AlwaysOnline *bool `json:"alwaysOnline,omitempty"`
}

// SecurityConfig contains security configuration.
type SecurityConfig struct {
	// Level is the security level (essentially_off, low, medium, high, under_attack)
	Level string `json:"level,omitempty"`
	// BrowserIntegrityCheck enables browser integrity check
	BrowserIntegrityCheck *bool `json:"browserIntegrityCheck,omitempty"`
	// EmailObfuscation enables email obfuscation
	EmailObfuscation *bool `json:"emailObfuscation,omitempty"`
	// HotlinkProtection enables hotlink protection
	HotlinkProtection *bool `json:"hotlinkProtection,omitempty"`
	// WAF contains WAF configuration
	WAF *WAFConfig `json:"waf,omitempty"`
}

// WAFConfig contains WAF configuration.
type WAFConfig struct {
	// Enabled enables the WAF
	Enabled *bool `json:"enabled,omitempty"`
	// RuleGroups contains rule group settings
	RuleGroups []WAFRuleGroup `json:"ruleGroups,omitempty"`
}

// WAFRuleGroup contains a WAF rule group configuration.
type WAFRuleGroup struct {
	// ID is the rule group ID
	ID string `json:"id,omitempty"`
	// Mode is the rule group mode (on, off, anomaly, traditional)
	Mode string `json:"mode,omitempty"`
}

// PerformanceConfig contains performance configuration.
type PerformanceConfig struct {
	// Minify contains minification settings
	Minify *MinifyConfig `json:"minify,omitempty"`
	// Polish is the image optimization setting (lossy, lossless, off)
	Polish string `json:"polish,omitempty"`
	// Mirage enables Mirage (image optimization for mobile)
	Mirage *bool `json:"mirage,omitempty"`
	// Brotli enables Brotli compression
	Brotli *bool `json:"brotli,omitempty"`
	// EarlyHints enables Early Hints
	EarlyHints *bool `json:"earlyHints,omitempty"`
	// HTTP2 enables HTTP/2
	HTTP2 *bool `json:"http2,omitempty"`
	// HTTP3 enables HTTP/3
	HTTP3 *bool `json:"http3,omitempty"`
	// ZeroRTT enables 0-RTT Connection Resumption
	ZeroRTT *bool `json:"zeroRtt,omitempty"`
	// RocketLoader enables Rocket Loader
	RocketLoader *bool `json:"rocketLoader,omitempty"`
}

// MinifyConfig contains minification settings.
type MinifyConfig struct {
	// HTML enables HTML minification
	HTML *bool `json:"html,omitempty"`
	// CSS enables CSS minification
	CSS *bool `json:"css,omitempty"`
	// JS enables JavaScript minification
	JS *bool `json:"js,omitempty"`
}

// VerificationConfig contains domain verification settings.
type VerificationConfig struct {
	// Method is the verification method (dns, http)
	Method string `json:"method,omitempty"`
	// DNSRecord contains DNS verification settings
	DNSRecord *DNSVerificationRecord `json:"dnsRecord,omitempty"`
}

// DNSVerificationRecord contains DNS verification record details.
type DNSVerificationRecord struct {
	// Type is the DNS record type (TXT, CNAME)
	Type string `json:"type,omitempty"`
	// Name is the record name
	Name string `json:"name,omitempty"`
	// Value is the record value
	Value string `json:"value,omitempty"`
}

// OriginCACertificateConfig contains the configuration for an Origin CA Certificate.
type OriginCACertificateConfig struct {
	// Hostnames is the list of hostnames to cover
	Hostnames []string `json:"hostnames"`
	// RequestType is the certificate request type (origin-rsa, origin-ecc)
	RequestType string `json:"requestType,omitempty"`
	// ValidityDays is the certificate validity in days
	ValidityDays int `json:"validityDays,omitempty"`
	// CSR is the Certificate Signing Request (if provided)
	CSR string `json:"csr,omitempty"`
}

// CloudflareDomainRegisterOptions contains options for registering a CloudflareDomain.
type CloudflareDomainRegisterOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// ZoneID is the Cloudflare zone ID
	ZoneID string
	// Source is the K8s resource source
	Source service.Source
	// Config is the domain configuration
	Config CloudflareDomainConfig
	// CredentialsRef references the CloudflareCredentials resource
	CredentialsRef v1alpha2.CredentialsReference
}

// OriginCACertificateRegisterOptions contains options for registering an OriginCACertificate.
type OriginCACertificateRegisterOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// ZoneID is the Cloudflare zone ID
	ZoneID string
	// CertificateID is the existing certificate ID (empty for new)
	CertificateID string
	// Source is the K8s resource source
	Source service.Source
	// Config is the certificate configuration
	Config OriginCACertificateConfig
	// CredentialsRef references the CloudflareCredentials resource
	CredentialsRef v1alpha2.CredentialsReference
}

// CloudflareDomainSyncResult contains CloudflareDomain-specific sync result.
type CloudflareDomainSyncResult struct {
	// ZoneID is the Cloudflare zone ID
	ZoneID string
	// ZoneName is the zone name
	ZoneName string
	// Status is the domain status (maps to CloudflareDomainState)
	Status string
}

// OriginCACertificateSyncResult contains OriginCACertificate-specific sync result.
type OriginCACertificateSyncResult struct {
	// CertificateID is the certificate ID
	CertificateID string
	// ExpiresAt is the expiration time
	ExpiresAt *metav1.Time
	// Certificate is the certificate PEM
	Certificate string
	// PrivateKey is the private key PEM (only on creation)
	PrivateKey string
}
