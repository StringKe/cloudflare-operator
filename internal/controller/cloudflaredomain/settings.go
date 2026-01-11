// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cloudflaredomain

import (
	"errors"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

var errZoneIDNotAvailable = errors.New("zone ID not available")

// syncZoneSettings synchronizes zone settings based on the CloudflareDomain spec
//
//nolint:revive // cognitive complexity is acceptable for this orchestration function
func (r *Reconciler) syncZoneSettings(cfClient *cloudflare.API) error {
	if r.domain.Status.ZoneID == "" {
		return errZoneIDNotAvailable
	}

	zoneID := r.domain.Status.ZoneID
	hasSettingsToSync := r.domain.Spec.SSL != nil ||
		r.domain.Spec.Cache != nil ||
		r.domain.Spec.Security != nil ||
		r.domain.Spec.Performance != nil

	if !hasSettingsToSync {
		// No settings configured, skip sync
		return nil
	}

	// Initialize config sync status if nil
	if r.domain.Status.ConfigSyncStatus == nil {
		r.domain.Status.ConfigSyncStatus = &networkingv1alpha2.ConfigSyncStatus{}
	}

	var settings []cloudflare.ZoneSetting
	var syncErrors []error

	// Build SSL settings
	if r.domain.Spec.SSL != nil {
		sslSettings := r.buildSSLSettings()
		settings = append(settings, sslSettings...)
		r.domain.Status.ConfigSyncStatus.SSL = networkingv1alpha2.ConfigSyncStateSyncing
	}

	// Build Cache settings
	if r.domain.Spec.Cache != nil {
		cacheSettings := r.buildCacheSettings()
		settings = append(settings, cacheSettings...)
		r.domain.Status.ConfigSyncStatus.Cache = networkingv1alpha2.ConfigSyncStateSyncing
	}

	// Build Security settings
	if r.domain.Spec.Security != nil {
		securitySettings := r.buildSecuritySettings()
		settings = append(settings, securitySettings...)
		r.domain.Status.ConfigSyncStatus.Security = networkingv1alpha2.ConfigSyncStateSyncing
	}

	// Build Performance settings
	if r.domain.Spec.Performance != nil {
		perfSettings := r.buildPerformanceSettings()
		settings = append(settings, perfSettings...)
		r.domain.Status.ConfigSyncStatus.Performance = networkingv1alpha2.ConfigSyncStateSyncing
	}

	// Apply settings if any
	if len(settings) > 0 {
		r.log.Info("Syncing zone settings", "zoneId", zoneID, "settingsCount", len(settings))

		_, err := cfClient.UpdateZoneSettings(r.ctx, zoneID, settings)
		if err != nil {
			r.domain.Status.ConfigSyncStatus.ErrorMessage = fmt.Sprintf("failed to update zone settings: %v", err)
			return fmt.Errorf("failed to update zone settings: %w", err)
		}

		// Mark all as synced
		now := time.Now().UTC().Format(time.RFC3339)
		r.domain.Status.ConfigSyncStatus.LastSyncTime = &now
		r.domain.Status.ConfigSyncStatus.ErrorMessage = ""

		if r.domain.Spec.SSL != nil {
			r.domain.Status.ConfigSyncStatus.SSL = networkingv1alpha2.ConfigSyncStateSynced
		}
		if r.domain.Spec.Cache != nil {
			r.domain.Status.ConfigSyncStatus.Cache = networkingv1alpha2.ConfigSyncStateSynced
		}
		if r.domain.Spec.Security != nil {
			r.domain.Status.ConfigSyncStatus.Security = networkingv1alpha2.ConfigSyncStateSynced
		}
		if r.domain.Spec.Performance != nil {
			r.domain.Status.ConfigSyncStatus.Performance = networkingv1alpha2.ConfigSyncStateSynced
		}

		r.log.Info("Zone settings synced successfully", "zoneId", zoneID)
	}

	// Return any build errors that occurred
	if len(syncErrors) > 0 {
		return errors.Join(syncErrors...)
	}

	return nil
}

// buildSSLSettings builds SSL/TLS settings for zone update
//
//nolint:revive // cognitive complexity is acceptable for settings builder
func (r *Reconciler) buildSSLSettings() []cloudflare.ZoneSetting {
	ssl := r.domain.Spec.SSL
	if ssl == nil {
		return nil
	}

	var settings []cloudflare.ZoneSetting

	// SSL Mode
	if ssl.Mode != "" {
		mode := string(ssl.Mode)
		// Map full_strict to strict (Cloudflare API uses "strict")
		if mode == "full_strict" {
			mode = "strict"
		}
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "ssl",
			Value: mode,
		})
	}

	// Min TLS Version
	if ssl.MinTLSVersion != "" {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "min_tls_version",
			Value: string(ssl.MinTLSVersion),
		})
	}

	// TLS 1.3
	if ssl.TLS13 != "" {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "tls_1_3",
			Value: string(ssl.TLS13),
		})
	}

	// Always Use HTTPS
	if ssl.AlwaysUseHTTPS != nil {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "always_use_https",
			Value: boolToOnOff(*ssl.AlwaysUseHTTPS),
		})
	}

	// Automatic HTTPS Rewrites
	if ssl.AutomaticHTTPSRewrites != nil {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "automatic_https_rewrites",
			Value: boolToOnOff(*ssl.AutomaticHTTPSRewrites),
		})
	}

	// Opportunistic Encryption
	if ssl.OpportunisticEncryption != nil {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "opportunistic_encryption",
			Value: boolToOnOff(*ssl.OpportunisticEncryption),
		})
	}

	// Authenticated Origin Pull (TLS Client Auth)
	if ssl.AuthenticatedOriginPull != nil {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "tls_client_auth",
			Value: boolToOnOff(ssl.AuthenticatedOriginPull.Enabled),
		})
	}

	return settings
}

// buildCacheSettings builds cache settings for zone update
func (r *Reconciler) buildCacheSettings() []cloudflare.ZoneSetting {
	cache := r.domain.Spec.Cache
	if cache == nil {
		return nil
	}

	var settings []cloudflare.ZoneSetting

	// Browser Cache TTL
	if cache.BrowserTTL != nil {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "browser_cache_ttl",
			Value: *cache.BrowserTTL,
		})
	}

	// Development Mode
	settings = append(settings, cloudflare.ZoneSetting{
		ID:    "development_mode",
		Value: boolToOnOff(cache.DevelopmentMode),
	})

	// Cache Level
	if cache.CacheLevel != "" {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "cache_level",
			Value: string(cache.CacheLevel),
		})
	}

	// Always Online
	if cache.AlwaysOnline != nil {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "always_online",
			Value: boolToOnOff(*cache.AlwaysOnline),
		})
	}

	// Sort Query String for Cache (Enterprise only)
	if cache.SortQueryStringForCache {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "sort_query_string_for_cache",
			Value: "on",
		})
	}

	return settings
}

// buildSecuritySettings builds security settings for zone update
//
//nolint:revive // cognitive complexity is acceptable for settings builder
func (r *Reconciler) buildSecuritySettings() []cloudflare.ZoneSetting {
	security := r.domain.Spec.Security
	if security == nil {
		return nil
	}

	var settings []cloudflare.ZoneSetting

	// Security Level
	if security.Level != "" {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "security_level",
			Value: string(security.Level),
		})
	}

	// Browser Check
	if security.BrowserCheck != nil {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "browser_check",
			Value: boolToOnOff(*security.BrowserCheck),
		})
	}

	// Email Obfuscation
	if security.EmailObfuscation != nil {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "email_obfuscation",
			Value: boolToOnOff(*security.EmailObfuscation),
		})
	}

	// Server Side Exclude
	if security.ServerSideExclude != nil {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "server_side_exclude",
			Value: boolToOnOff(*security.ServerSideExclude),
		})
	}

	// Hotlink Protection
	settings = append(settings, cloudflare.ZoneSetting{
		ID:    "hotlink_protection",
		Value: boolToOnOff(security.HotlinkProtection),
	})

	// Challenge TTL
	if security.ChallengePassage != nil {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "challenge_ttl",
			Value: *security.ChallengePassage,
		})
	}

	// WAF
	if security.WAF != nil && security.WAF.Enabled {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "waf",
			Value: "on",
		})
	}

	return settings
}

// buildPerformanceSettings builds performance settings for zone update
//
//nolint:revive // cognitive complexity is acceptable for settings builder
func (r *Reconciler) buildPerformanceSettings() []cloudflare.ZoneSetting {
	perf := r.domain.Spec.Performance
	if perf == nil {
		return nil
	}

	var settings []cloudflare.ZoneSetting

	// Brotli
	if perf.Brotli != nil {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "brotli",
			Value: boolToOnOff(*perf.Brotli),
		})
	}

	// HTTP/2
	if perf.HTTP2 != nil {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "http2",
			Value: boolToOnOff(*perf.HTTP2),
		})
	}

	// HTTP/3
	if perf.HTTP3 != nil {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "http3",
			Value: boolToOnOff(*perf.HTTP3),
		})
	}

	// 0-RTT
	if perf.ZeroRTT != nil {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "0rtt",
			Value: boolToOnOff(*perf.ZeroRTT),
		})
	}

	// Minify
	if perf.Minify != nil {
		settings = append(settings, cloudflare.ZoneSetting{
			ID: "minify",
			Value: map[string]string{
				"html": boolToOnOff(perf.Minify.HTML),
				"css":  boolToOnOff(perf.Minify.CSS),
				"js":   boolToOnOff(perf.Minify.JavaScript),
			},
		})
	}

	// Polish (image optimization)
	if perf.Polish != "" {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "polish",
			Value: string(perf.Polish),
		})
	}

	// WebP
	settings = append(settings, cloudflare.ZoneSetting{
		ID:    "webp",
		Value: boolToOnOff(perf.WebP),
	})

	// Mirage
	settings = append(settings, cloudflare.ZoneSetting{
		ID:    "mirage",
		Value: boolToOnOff(perf.Mirage),
	})

	// Early Hints
	if perf.EarlyHints != nil {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "early_hints",
			Value: boolToOnOff(*perf.EarlyHints),
		})
	}

	// Rocket Loader
	settings = append(settings, cloudflare.ZoneSetting{
		ID:    "rocket_loader",
		Value: boolToOnOff(perf.RocketLoader),
	})

	// Prefetch Preload
	if perf.PrefetchPreload != nil {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "prefetch_preload",
			Value: boolToOnOff(*perf.PrefetchPreload),
		})
	}

	// IP Geolocation
	if perf.IPGeolocation != nil {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "ip_geolocation",
			Value: boolToOnOff(*perf.IPGeolocation),
		})
	}

	// Websockets
	if perf.Websockets != nil {
		settings = append(settings, cloudflare.ZoneSetting{
			ID:    "websockets",
			Value: boolToOnOff(*perf.Websockets),
		})
	}

	return settings
}

// boolToOnOff converts a bool to "on"/"off" string
//
//nolint:revive // flag-parameter is intentional for this helper
func boolToOnOff(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}
