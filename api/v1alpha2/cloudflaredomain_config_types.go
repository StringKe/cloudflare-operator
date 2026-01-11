// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

// SSLMode represents the SSL/TLS encryption mode
// +kubebuilder:validation:Enum=off;flexible;full;strict;full_strict
type SSLMode string

const (
	SSLModeOff        SSLMode = "off"
	SSLModeFlexible   SSLMode = "flexible"
	SSLModeFull       SSLMode = "full"
	SSLModeStrict     SSLMode = "strict"
	SSLModeFullStrict SSLMode = "full_strict"
)

// TLSVersion represents supported TLS versions
// +kubebuilder:validation:Enum="1.0";"1.1";"1.2";"1.3"
type TLSVersion string

const (
	TLSVersion10 TLSVersion = "1.0"
	TLSVersion11 TLSVersion = "1.1"
	TLSVersion12 TLSVersion = "1.2"
	TLSVersion13 TLSVersion = "1.3"
)

// FeatureToggle represents on/off toggle settings
// +kubebuilder:validation:Enum=on;off
type FeatureToggle string

const (
	FeatureOn  FeatureToggle = "on"
	FeatureOff FeatureToggle = "off"
)

// CacheLevel represents cache level settings
// +kubebuilder:validation:Enum=bypass;basic;simplified;aggressive
type CacheLevel string

const (
	CacheLevelBypass     CacheLevel = "bypass"
	CacheLevelBasic      CacheLevel = "basic"
	CacheLevelSimplified CacheLevel = "simplified"
	CacheLevelAggressive CacheLevel = "aggressive"
)

// SecurityLevel represents security level settings
// +kubebuilder:validation:Enum=off;essentially_off;low;medium;high;under_attack
type SecurityLevel string

const (
	SecurityLevelOff            SecurityLevel = "off"
	SecurityLevelEssentiallyOff SecurityLevel = "essentially_off"
	SecurityLevelLow            SecurityLevel = "low"
	SecurityLevelMedium         SecurityLevel = "medium"
	SecurityLevelHigh           SecurityLevel = "high"
	SecurityLevelUnderAttack    SecurityLevel = "under_attack"
)

// PolishMode represents image optimization mode
// +kubebuilder:validation:Enum=off;lossless;lossy
type PolishMode string

const (
	PolishModeOff      PolishMode = "off"
	PolishModeLossless PolishMode = "lossless"
	PolishModeLossy    PolishMode = "lossy"
)

// TieredCacheTopology represents tiered cache topology
// +kubebuilder:validation:Enum=smart;generic
type TieredCacheTopology string

const (
	TieredCacheSmart   TieredCacheTopology = "smart"
	TieredCacheGeneric TieredCacheTopology = "generic"
)

// ============================================================
// SSL/TLS Configuration
// ============================================================

// AuthenticatedOriginPullConfig configures client certificate authentication
type AuthenticatedOriginPullConfig struct {
	// Enabled enables authenticated origin pulls (mTLS)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// CertificateSecretRef references a Secret containing custom client certificate
	// If not specified, Cloudflare's default certificate will be used
	// +kubebuilder:validation:Optional
	CertificateSecretRef *SecretReference `json:"certificateSecretRef,omitempty"`
}

// SSLConfig defines SSL/TLS settings for a domain
type SSLConfig struct {
	// Mode sets the SSL/TLS encryption mode
	// - off: No encryption (not recommended)
	// - flexible: Encrypts traffic between browser and Cloudflare only
	// - full: Encrypts end-to-end, using a self-signed cert on the origin
	// - strict/full_strict: Encrypts end-to-end, requires valid origin cert
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=full
	Mode SSLMode `json:"mode,omitempty"`

	// MinTLSVersion sets the minimum TLS version
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="1.2"
	MinTLSVersion TLSVersion `json:"minTLSVersion,omitempty"`

	// TLS13 enables TLS 1.3 support
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=on
	TLS13 FeatureToggle `json:"tls13,omitempty"`

	// AlwaysUseHTTPS redirects all HTTP requests to HTTPS
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	AlwaysUseHTTPS *bool `json:"alwaysUseHttps,omitempty"`

	// AutomaticHTTPSRewrites rewrites HTTP links to HTTPS in HTML content
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	AutomaticHTTPSRewrites *bool `json:"automaticHttpsRewrites,omitempty"`

	// OpportunisticEncryption enables opportunistic encryption
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	OpportunisticEncryption *bool `json:"opportunisticEncryption,omitempty"`

	// AuthenticatedOriginPull configures mTLS between Cloudflare and origin
	// +kubebuilder:validation:Optional
	AuthenticatedOriginPull *AuthenticatedOriginPullConfig `json:"authenticatedOriginPull,omitempty"`
}

// ============================================================
// Cache Configuration
// ============================================================

// TieredCacheConfig configures tiered caching
type TieredCacheConfig struct {
	// Enabled enables tiered caching
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Topology sets the tiered cache topology
	// - smart: Dynamically selects the best upper tier
	// - generic: Uses regional hub data centers
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=smart
	Topology TieredCacheTopology `json:"topology,omitempty"`
}

// CacheReserveConfig configures Cache Reserve (persistent cache)
type CacheReserveConfig struct {
	// Enabled enables Cache Reserve
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`
}

// CacheConfig defines caching settings for a domain
type CacheConfig struct {
	// BrowserTTL sets the browser cache TTL in seconds
	// Minimum: 0 (respect origin), Maximum: 31536000 (1 year)
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=31536000
	BrowserTTL *int `json:"browserTTL,omitempty"`

	// DevelopmentMode temporarily bypasses cache for development
	// Automatically disables after 3 hours
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	DevelopmentMode bool `json:"developmentMode,omitempty"`

	// CacheLevel sets the cache level
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=aggressive
	CacheLevel CacheLevel `json:"cacheLevel,omitempty"`

	// TieredCache configures tiered caching
	// +kubebuilder:validation:Optional
	TieredCache *TieredCacheConfig `json:"tieredCache,omitempty"`

	// CacheReserve configures persistent cache storage
	// +kubebuilder:validation:Optional
	CacheReserve *CacheReserveConfig `json:"cacheReserve,omitempty"`

	// AlwaysOnline serves stale content when origin is unavailable
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	AlwaysOnline *bool `json:"alwaysOnline,omitempty"`

	// CacheByDeviceType caches content separately for mobile/desktop
	// Requires Enterprise plan
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	CacheByDeviceType bool `json:"cacheByDeviceType,omitempty"`

	// SortQueryStringForCache treats query strings with same parameters
	// but different order as the same for caching purposes
	// Requires Enterprise plan
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	SortQueryStringForCache bool `json:"sortQueryStringForCache,omitempty"`
}

// ============================================================
// Security Configuration
// ============================================================

// WAFConfig configures Web Application Firewall
type WAFConfig struct {
	// Enabled enables WAF
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`
}

// SecurityConfig defines security settings for a domain
type SecurityConfig struct {
	// Level sets the security level
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=medium
	Level SecurityLevel `json:"level,omitempty"`

	// BrowserCheck enables browser integrity check
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	BrowserCheck *bool `json:"browserCheck,omitempty"`

	// EmailObfuscation hides email addresses from bots
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	EmailObfuscation *bool `json:"emailObfuscation,omitempty"`

	// ServerSideExclude enables server-side excludes
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	ServerSideExclude *bool `json:"serverSideExclude,omitempty"`

	// HotlinkProtection prevents hotlinking of images
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	HotlinkProtection bool `json:"hotlinkProtection,omitempty"`

	// ChallengePassage sets how long a visitor can access the site
	// after completing a challenge (in seconds)
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=300
	// +kubebuilder:validation:Maximum=31536000
	// +kubebuilder:default=1800
	ChallengePassage *int `json:"challengePassage,omitempty"`

	// WAF configures Web Application Firewall
	// +kubebuilder:validation:Optional
	WAF *WAFConfig `json:"waf,omitempty"`
}

// ============================================================
// Performance Configuration
// ============================================================

// MinifyConfig configures code minification
type MinifyConfig struct {
	// HTML enables HTML minification
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	HTML bool `json:"html,omitempty"`

	// CSS enables CSS minification
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	CSS bool `json:"css,omitempty"`

	// JavaScript enables JavaScript minification
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	JavaScript bool `json:"javascript,omitempty"`
}

// PerformanceConfig defines performance settings for a domain
type PerformanceConfig struct {
	// Brotli enables Brotli compression
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Brotli *bool `json:"brotli,omitempty"`

	// HTTP2 enables HTTP/2 support
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	HTTP2 *bool `json:"http2,omitempty"`

	// HTTP3 enables HTTP/3 (QUIC) support
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	HTTP3 *bool `json:"http3,omitempty"`

	// ZeroRTT enables 0-RTT Connection Resumption
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	ZeroRTT *bool `json:"zeroRTT,omitempty"`

	// Minify configures code minification
	// +kubebuilder:validation:Optional
	Minify *MinifyConfig `json:"minify,omitempty"`

	// Polish configures image optimization mode
	// Requires Pro plan or higher
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=off
	Polish PolishMode `json:"polish,omitempty"`

	// WebP enables WebP image conversion
	// Requires Pro plan or higher
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	WebP bool `json:"webp,omitempty"`

	// Mirage enables mobile image optimization
	// Requires Pro plan or higher
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Mirage bool `json:"mirage,omitempty"`

	// EarlyHints enables 103 Early Hints
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	EarlyHints *bool `json:"earlyHints,omitempty"`

	// RocketLoader optimizes JavaScript loading
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	RocketLoader bool `json:"rocketLoader,omitempty"`

	// PrefetchPreload enables prefetch and preload
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	PrefetchPreload *bool `json:"prefetchPreload,omitempty"`

	// IPGeolocation adds visitor's country to request headers
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	IPGeolocation *bool `json:"ipGeolocation,omitempty"`

	// Websockets enables WebSocket support
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Websockets *bool `json:"websockets,omitempty"`
}

// ============================================================
// Config Sync Status
// ============================================================

// ConfigSyncState represents the sync state of a configuration section
// +kubebuilder:validation:Enum=Synced;Syncing;Error;Unknown
type ConfigSyncState string

const (
	ConfigSyncStateSynced  ConfigSyncState = "Synced"
	ConfigSyncStateSyncing ConfigSyncState = "Syncing"
	ConfigSyncStateError   ConfigSyncState = "Error"
	ConfigSyncStateUnknown ConfigSyncState = "Unknown"
)

// ConfigSyncStatus represents the sync status of all configuration sections
type ConfigSyncStatus struct {
	// SSL sync status
	// +kubebuilder:validation:Optional
	SSL ConfigSyncState `json:"ssl,omitempty"`

	// Cache sync status
	// +kubebuilder:validation:Optional
	Cache ConfigSyncState `json:"cache,omitempty"`

	// Security sync status
	// +kubebuilder:validation:Optional
	Security ConfigSyncState `json:"security,omitempty"`

	// Performance sync status
	// +kubebuilder:validation:Optional
	Performance ConfigSyncState `json:"performance,omitempty"`

	// LastSyncTime is the last time any configuration was synced
	// +kubebuilder:validation:Optional
	LastSyncTime *string `json:"lastSyncTime,omitempty"`

	// ErrorMessage contains error details if any sync failed
	// +kubebuilder:validation:Optional
	ErrorMessage string `json:"errorMessage,omitempty"`
}
