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
	// TLS13 enables TLS 1.3 (on, off)
	TLS13 string `json:"tls13,omitempty"`
	// AlwaysUseHTTPS enables automatic HTTPS redirect
	AlwaysUseHTTPS *bool `json:"alwaysUseHttps,omitempty"`
	// AutomaticHTTPSRewrites enables automatic HTTPS rewrites
	AutomaticHTTPSRewrites *bool `json:"automaticHttpsRewrites,omitempty"`
	// OpportunisticEncryption enables opportunistic encryption
	OpportunisticEncryption *bool `json:"opportunisticEncryption,omitempty"`
	// AuthenticatedOriginPull configures mTLS between Cloudflare and origin
	AuthenticatedOriginPull *AuthenticatedOriginPullConfig `json:"authenticatedOriginPull,omitempty"`
}

// AuthenticatedOriginPullConfig configures client certificate authentication.
type AuthenticatedOriginPullConfig struct {
	// Enabled enables authenticated origin pulls (mTLS)
	Enabled bool `json:"enabled,omitempty"`
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
	// TieredCache configures tiered caching
	TieredCache *TieredCacheConfig `json:"tieredCache,omitempty"`
	// CacheReserve configures persistent cache storage
	CacheReserve *CacheReserveConfig `json:"cacheReserve,omitempty"`
	// CacheByDeviceType caches content separately for mobile/desktop
	CacheByDeviceType *bool `json:"cacheByDeviceType,omitempty"`
	// SortQueryStringForCache treats query strings with same parameters
	// but different order as the same for caching purposes
	SortQueryStringForCache *bool `json:"sortQueryStringForCache,omitempty"`
}

// TieredCacheConfig configures tiered caching.
type TieredCacheConfig struct {
	// Enabled enables tiered caching
	Enabled bool `json:"enabled,omitempty"`
	// Topology sets the tiered cache topology (smart, generic)
	Topology string `json:"topology,omitempty"`
}

// CacheReserveConfig configures Cache Reserve (persistent cache).
type CacheReserveConfig struct {
	// Enabled enables Cache Reserve
	Enabled bool `json:"enabled,omitempty"`
}

// SecurityConfig contains security configuration.
type SecurityConfig struct {
	// Level is the security level (essentially_off, low, medium, high, under_attack)
	Level string `json:"level,omitempty"`
	// BrowserIntegrityCheck enables browser integrity check
	BrowserIntegrityCheck *bool `json:"browserIntegrityCheck,omitempty"`
	// EmailObfuscation enables email obfuscation
	EmailObfuscation *bool `json:"emailObfuscation,omitempty"`
	// ServerSideExclude enables server-side excludes
	ServerSideExclude *bool `json:"serverSideExclude,omitempty"`
	// HotlinkProtection enables hotlink protection
	HotlinkProtection *bool `json:"hotlinkProtection,omitempty"`
	// ChallengePassage sets how long a visitor can access the site
	// after completing a challenge (in seconds)
	ChallengePassage *int `json:"challengePassage,omitempty"`
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
	// WebP enables WebP image conversion
	WebP *bool `json:"webp,omitempty"`
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
	// PrefetchPreload enables prefetch and preload
	PrefetchPreload *bool `json:"prefetchPreload,omitempty"`
	// IPGeolocation adds visitor's country to request headers
	IPGeolocation *bool `json:"ipGeolocation,omitempty"`
	// Websockets enables WebSocket support
	Websockets *bool `json:"websockets,omitempty"`
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

// OriginCACertificateAction defines the action to perform on a certificate.
type OriginCACertificateAction string

const (
	// OriginCACertificateActionCreate creates a new certificate
	OriginCACertificateActionCreate OriginCACertificateAction = "create"
	// OriginCACertificateActionRevoke revokes an existing certificate
	OriginCACertificateActionRevoke OriginCACertificateAction = "revoke"
	// OriginCACertificateActionRenew renews an existing certificate
	OriginCACertificateActionRenew OriginCACertificateAction = "renew"
)

// OriginCACertificateLifecycleConfig contains lifecycle operation configuration.
type OriginCACertificateLifecycleConfig struct {
	// Action is the lifecycle operation to perform
	Action OriginCACertificateAction `json:"action"`
	// CertificateID is the existing certificate ID (for revoke/renew)
	CertificateID string `json:"certificateId,omitempty"`
	// Hostnames is the list of hostnames to cover (for create/renew)
	Hostnames []string `json:"hostnames,omitempty"`
	// RequestType is the certificate request type (origin-rsa, origin-ecc)
	RequestType string `json:"requestType,omitempty"`
	// ValidityDays is the certificate validity in days
	ValidityDays int `json:"validityDays,omitempty"`
	// CSR is the Certificate Signing Request
	CSR string `json:"csr,omitempty"`
}

// Result data keys for OriginCACertificate SyncState.
const (
	ResultKeyOriginCACertificateID = "certificateId"
	ResultKeyOriginCACertificate   = "certificate"
	ResultKeyOriginCAExpiresAt     = "expiresAt"
	ResultKeyOriginCARequestType   = "requestType"
	ResultKeyOriginCAHostnames     = "hostnames"
)

// OriginCACertificateCreateOptions contains options for creating an Origin CA certificate.
type OriginCACertificateCreateOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// ZoneID is the Cloudflare zone ID
	ZoneID string
	// Source identifies the K8s resource
	Source service.Source
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
	// Hostnames is the list of hostnames to cover
	Hostnames []string
	// RequestType is the certificate request type (origin-rsa, origin-ecc)
	RequestType string
	// ValidityDays is the certificate validity in days
	ValidityDays int
	// CSR is the Certificate Signing Request
	CSR string
}

// OriginCACertificateRevokeOptions contains options for revoking an Origin CA certificate.
type OriginCACertificateRevokeOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// ZoneID is the Cloudflare zone ID
	ZoneID string
	// Source identifies the K8s resource
	Source service.Source
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
	// CertificateID is the ID of the certificate to revoke
	CertificateID string
}

// OriginCACertificateRenewOptions contains options for renewing an Origin CA certificate.
type OriginCACertificateRenewOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// ZoneID is the Cloudflare zone ID
	ZoneID string
	// Source identifies the K8s resource
	Source service.Source
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
	// CertificateID is the existing certificate ID to revoke
	CertificateID string
	// Hostnames is the list of hostnames to cover
	Hostnames []string
	// RequestType is the certificate request type (origin-rsa, origin-ecc)
	RequestType string
	// ValidityDays is the certificate validity in days
	ValidityDays int
	// CSR is the Certificate Signing Request
	CSR string
}

// DomainRegistration Types

// DomainRegistrationAction defines the action to perform on a domain registration.
type DomainRegistrationAction string

const (
	// DomainRegistrationActionSync syncs domain information from Cloudflare
	DomainRegistrationActionSync DomainRegistrationAction = "sync"
	// DomainRegistrationActionUpdate updates domain configuration in Cloudflare
	DomainRegistrationActionUpdate DomainRegistrationAction = "update"
)

// DomainRegistrationLifecycleConfig contains lifecycle operation configuration for domain registration.
type DomainRegistrationLifecycleConfig struct {
	// Action is the lifecycle operation to perform
	Action DomainRegistrationAction `json:"action"`
	// DomainName is the domain name to manage
	DomainName string `json:"domainName"`
	// Configuration is the optional domain configuration to apply
	Configuration *DomainRegistrationConfiguration `json:"configuration,omitempty"`
}

// DomainRegistrationConfiguration contains domain registration configuration settings.
type DomainRegistrationConfiguration struct {
	// AutoRenew enables auto-renewal
	AutoRenew bool `json:"autoRenew,omitempty"`
	// Privacy enables WHOIS privacy
	Privacy bool `json:"privacy,omitempty"`
	// Locked enables registrar lock
	Locked bool `json:"locked,omitempty"`
	// NameServers is the list of name servers
	NameServers []string `json:"nameServers,omitempty"`
}

// DomainRegistrationSyncResult contains the result of a domain registration sync.
type DomainRegistrationSyncResult struct {
	// DomainID is the domain ID
	DomainID string
	// CurrentRegistrar is the current registrar
	CurrentRegistrar string
	// RegistryStatuses are the registry statuses (comma-separated string from Cloudflare)
	RegistryStatuses string
	// Locked indicates if the domain is locked
	Locked bool
	// TransferInStatus is the transfer in status
	TransferInStatus string
	// ExpiresAt is the expiration time
	ExpiresAt metav1.Time
	// CreatedAt is the creation time
	CreatedAt metav1.Time
	// AutoRenew indicates if auto-renewal is enabled
	AutoRenew bool
	// Privacy indicates if WHOIS privacy is enabled
	Privacy bool
}

// Result data keys for DomainRegistration SyncState.
const (
	ResultKeyDomainID         = "domainId"
	ResultKeyCurrentRegistrar = "currentRegistrar"
	ResultKeyRegistryStatuses = "registryStatuses"
	ResultKeyDomainLocked     = "locked"
	ResultKeyTransferInStatus = "transferInStatus"
	ResultKeyDomainExpiresAt  = "expiresAt"
	ResultKeyDomainCreatedAt  = "createdAt"
	ResultKeyDomainAutoRenew  = "autoRenew"
	ResultKeyDomainPrivacy    = "privacy"
)

// DomainRegistrationRegisterOptions contains options for registering a DomainRegistration.
type DomainRegistrationRegisterOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// Source identifies the K8s resource
	Source service.Source
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
	// DomainName is the domain name to manage
	DomainName string
	// Configuration is the optional domain configuration to apply
	Configuration *DomainRegistrationConfiguration
}
