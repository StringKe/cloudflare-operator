// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
)

// ZoneSettings represents a collection of zone settings
type ZoneSettings struct {
	// SSL/TLS settings
	SSLMode                 string `json:"ssl,omitempty"`
	MinTLSVersion           string `json:"min_tls_version,omitempty"`
	TLS13                   string `json:"tls_1_3,omitempty"`
	AlwaysUseHTTPS          string `json:"always_use_https,omitempty"`
	AutomaticHTTPSRewrites  string `json:"automatic_https_rewrites,omitempty"`
	OpportunisticEncryption string `json:"opportunistic_encryption,omitempty"`
	TLSClientAuth           string `json:"tls_client_auth,omitempty"`

	// Cache settings
	BrowserCacheTTL int    `json:"browser_cache_ttl,omitempty"`
	DevelopmentMode string `json:"development_mode,omitempty"`
	CacheLevel      string `json:"cache_level,omitempty"`
	AlwaysOnline    string `json:"always_online,omitempty"`
	SortQueryString string `json:"sort_query_string_for_cache,omitempty"`

	// Security settings
	SecurityLevel     string `json:"security_level,omitempty"`
	BrowserCheck      string `json:"browser_check,omitempty"`
	EmailObfuscation  string `json:"email_obfuscation,omitempty"`
	ServerSideExclude string `json:"server_side_exclude,omitempty"`
	HotlinkProtection string `json:"hotlink_protection,omitempty"`
	ChallengePassage  int    `json:"challenge_ttl,omitempty"`
	WAF               string `json:"waf,omitempty"`

	// Performance settings
	Brotli          string          `json:"brotli,omitempty"`
	HTTP2           string          `json:"http2,omitempty"`
	HTTP3           string          `json:"http3,omitempty"`
	ZeroRTT         string          `json:"0rtt,omitempty"`
	Minify          *MinifySettings `json:"minify,omitempty"`
	Polish          string          `json:"polish,omitempty"`
	WebP            string          `json:"webp,omitempty"`
	Mirage          string          `json:"mirage,omitempty"`
	EarlyHints      string          `json:"early_hints,omitempty"`
	RocketLoader    string          `json:"rocket_loader,omitempty"`
	PrefetchPreload string          `json:"prefetch_preload,omitempty"`
	IPGeolocation   string          `json:"ip_geolocation,omitempty"`
	Websockets      string          `json:"websockets,omitempty"`
}

// MinifySettings represents minification settings
type MinifySettings struct {
	HTML bool `json:"html"`
	CSS  bool `json:"css"`
	JS   bool `json:"js"`
}

var errClientNotInitialized = errors.New("cloudflare client not initialized")

// asString safely converts interface value to string
func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// asFloat64 safely converts interface value to float64
func asFloat64(v any) (float64, bool) {
	if f, ok := v.(float64); ok {
		return f, true
	}
	return 0, false
}

// GetZoneSettings retrieves all zone settings
func (api *API) GetZoneSettings(ctx context.Context, zoneID string) (*ZoneSettings, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	settings, err := api.CloudflareClient.ZoneSettings(ctx, zoneID)
	if err != nil {
		return nil, fmt.Errorf("failed to get zone settings: %w", err)
	}

	result := &ZoneSettings{}
	for _, setting := range settings.Result {
		parseZoneSetting(result, setting)
	}

	return result, nil
}

// parseZoneSetting parses a single zone setting into ZoneSettings struct
//
//nolint:gocyclo,revive // switch over all zone settings is inherently complex
func parseZoneSetting(result *ZoneSettings, setting cloudflare.ZoneSetting) {
	switch setting.ID {
	// SSL/TLS
	case "ssl":
		result.SSLMode = asString(setting.Value)
	case "min_tls_version":
		result.MinTLSVersion = asString(setting.Value)
	case "tls_1_3":
		result.TLS13 = asString(setting.Value)
	case "always_use_https":
		result.AlwaysUseHTTPS = asString(setting.Value)
	case "automatic_https_rewrites":
		result.AutomaticHTTPSRewrites = asString(setting.Value)
	case "opportunistic_encryption":
		result.OpportunisticEncryption = asString(setting.Value)
	case "tls_client_auth":
		result.TLSClientAuth = asString(setting.Value)

	// Cache
	case "browser_cache_ttl":
		if v, ok := asFloat64(setting.Value); ok {
			result.BrowserCacheTTL = int(v)
		}
	case "development_mode":
		result.DevelopmentMode = asString(setting.Value)
	case "cache_level":
		result.CacheLevel = asString(setting.Value)
	case "always_online":
		result.AlwaysOnline = asString(setting.Value)
	case "sort_query_string_for_cache":
		result.SortQueryString = asString(setting.Value)

	// Security
	case "security_level":
		result.SecurityLevel = asString(setting.Value)
	case "browser_check":
		result.BrowserCheck = asString(setting.Value)
	case "email_obfuscation":
		result.EmailObfuscation = asString(setting.Value)
	case "server_side_exclude":
		result.ServerSideExclude = asString(setting.Value)
	case "hotlink_protection":
		result.HotlinkProtection = asString(setting.Value)
	case "challenge_ttl":
		if v, ok := asFloat64(setting.Value); ok {
			result.ChallengePassage = int(v)
		}
	case "waf":
		result.WAF = asString(setting.Value)

	// Performance
	case "brotli":
		result.Brotli = asString(setting.Value)
	case "http2":
		result.HTTP2 = asString(setting.Value)
	case "http3":
		result.HTTP3 = asString(setting.Value)
	case "0rtt":
		result.ZeroRTT = asString(setting.Value)
	case "minify":
		if m, ok := setting.Value.(map[string]any); ok {
			result.Minify = &MinifySettings{
				HTML: m["html"] == "on",
				CSS:  m["css"] == "on",
				JS:   m["js"] == "on",
			}
		}
	case "polish":
		result.Polish = asString(setting.Value)
	case "webp":
		result.WebP = asString(setting.Value)
	case "mirage":
		result.Mirage = asString(setting.Value)
	case "early_hints":
		result.EarlyHints = asString(setting.Value)
	case "rocket_loader":
		result.RocketLoader = asString(setting.Value)
	case "prefetch_preload":
		result.PrefetchPreload = asString(setting.Value)
	case "ip_geolocation":
		result.IPGeolocation = asString(setting.Value)
	case "websockets":
		result.Websockets = asString(setting.Value)
	}
}

// UpdateZoneSetting updates a single zone setting
func (api *API) UpdateZoneSetting(ctx context.Context, zoneID, settingName string, value any) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	// cloudflare-go v0.116.0 uses new API signature with ResourceContainer
	rc := cloudflare.ZoneIdentifier(zoneID)
	params := cloudflare.UpdateZoneSettingParams{
		Name:  settingName,
		Value: value,
	}

	_, err := api.CloudflareClient.UpdateZoneSetting(ctx, rc, params)
	if err != nil {
		return fmt.Errorf("failed to update zone setting %s: %w", settingName, err)
	}

	return nil
}

// UpdateZoneSettings updates multiple zone settings
func (api *API) UpdateZoneSettings(ctx context.Context, zoneID string, settings []cloudflare.ZoneSetting) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	_, err := api.CloudflareClient.UpdateZoneSettings(ctx, zoneID, settings)
	if err != nil {
		return fmt.Errorf("failed to update zone settings: %w", err)
	}

	return nil
}

// BoolToOnOff converts a bool pointer to "on"/"off" string
func BoolToOnOff(b *bool) string {
	if b == nil {
		return ""
	}
	if *b {
		return "on"
	}
	return "off"
}

// OnOffToBool converts "on"/"off" string to bool
func OnOffToBool(s string) bool {
	return s == "on"
}
