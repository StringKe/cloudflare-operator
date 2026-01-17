// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"testing"

	"github.com/cloudflare/cloudflare-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsString(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{
			name:     "valid string",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "nil value",
			input:    nil,
			expected: "",
		},
		{
			name:     "integer value",
			input:    123,
			expected: "",
		},
		{
			name:     "float value",
			input:    123.45,
			expected: "",
		},
		{
			name:     "bool value",
			input:    true,
			expected: "",
		},
		{
			name:     "slice value",
			input:    []string{"a", "b"},
			expected: "",
		},
		{
			name:     "map value",
			input:    map[string]string{"key": "value"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := asString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAsFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected float64
		ok       bool
	}{
		{
			name:     "valid float64",
			input:    123.45,
			expected: 123.45,
			ok:       true,
		},
		{
			name:     "zero float64",
			input:    0.0,
			expected: 0.0,
			ok:       true,
		},
		{
			name:     "negative float64",
			input:    -123.45,
			expected: -123.45,
			ok:       true,
		},
		{
			name:     "nil value",
			input:    nil,
			expected: 0,
			ok:       false,
		},
		{
			name:     "string value",
			input:    "123.45",
			expected: 0,
			ok:       false,
		},
		{
			name:     "integer value",
			input:    123,
			expected: 0,
			ok:       false,
		},
		{
			name:     "bool value",
			input:    true,
			expected: 0,
			ok:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := asFloat64(tt.input)
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestBoolToOnOff(t *testing.T) {
	tests := []struct {
		name     string
		input    *bool
		expected string
	}{
		{
			name:     "nil pointer",
			input:    nil,
			expected: "",
		},
		{
			name:     "true value",
			input:    boolPtr(true),
			expected: "on",
		},
		{
			name:     "false value",
			input:    boolPtr(false),
			expected: "off",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BoolToOnOff(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOnOffToBool(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "on value",
			input:    "on",
			expected: true,
		},
		{
			name:     "off value",
			input:    "off",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "invalid value",
			input:    "yes",
			expected: false,
		},
		{
			name:     "uppercase ON",
			input:    "ON",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := OnOffToBool(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

//nolint:revive // cognitive-complexity: table-driven test with many settings
func TestParseZoneSetting(t *testing.T) {
	tests := []struct {
		name     string
		setting  cloudflare.ZoneSetting
		validate func(t *testing.T, result *ZoneSettings)
	}{
		// SSL/TLS settings
		{
			name:    "ssl setting",
			setting: cloudflare.ZoneSetting{ID: "ssl", Value: "strict"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "strict", result.SSLMode)
			},
		},
		{
			name:    "min_tls_version setting",
			setting: cloudflare.ZoneSetting{ID: "min_tls_version", Value: "1.2"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "1.2", result.MinTLSVersion)
			},
		},
		{
			name:    "tls_1_3 setting",
			setting: cloudflare.ZoneSetting{ID: "tls_1_3", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.TLS13)
			},
		},
		{
			name:    "always_use_https setting",
			setting: cloudflare.ZoneSetting{ID: "always_use_https", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.AlwaysUseHTTPS)
			},
		},
		{
			name:    "automatic_https_rewrites setting",
			setting: cloudflare.ZoneSetting{ID: "automatic_https_rewrites", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.AutomaticHTTPSRewrites)
			},
		},
		{
			name:    "opportunistic_encryption setting",
			setting: cloudflare.ZoneSetting{ID: "opportunistic_encryption", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.OpportunisticEncryption)
			},
		},
		{
			name:    "tls_client_auth setting",
			setting: cloudflare.ZoneSetting{ID: "tls_client_auth", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.TLSClientAuth)
			},
		},

		// Cache settings
		{
			name:    "browser_cache_ttl setting",
			setting: cloudflare.ZoneSetting{ID: "browser_cache_ttl", Value: float64(14400)},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, 14400, result.BrowserCacheTTL)
			},
		},
		{
			name:    "browser_cache_ttl with non-float",
			setting: cloudflare.ZoneSetting{ID: "browser_cache_ttl", Value: "14400"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, 0, result.BrowserCacheTTL)
			},
		},
		{
			name:    "development_mode setting",
			setting: cloudflare.ZoneSetting{ID: "development_mode", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.DevelopmentMode)
			},
		},
		{
			name:    "cache_level setting",
			setting: cloudflare.ZoneSetting{ID: "cache_level", Value: "aggressive"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "aggressive", result.CacheLevel)
			},
		},
		{
			name:    "always_online setting",
			setting: cloudflare.ZoneSetting{ID: "always_online", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.AlwaysOnline)
			},
		},
		{
			name:    "sort_query_string_for_cache setting",
			setting: cloudflare.ZoneSetting{ID: "sort_query_string_for_cache", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.SortQueryString)
			},
		},

		// Security settings
		{
			name:    "security_level setting",
			setting: cloudflare.ZoneSetting{ID: "security_level", Value: "high"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "high", result.SecurityLevel)
			},
		},
		{
			name:    "browser_check setting",
			setting: cloudflare.ZoneSetting{ID: "browser_check", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.BrowserCheck)
			},
		},
		{
			name:    "email_obfuscation setting",
			setting: cloudflare.ZoneSetting{ID: "email_obfuscation", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.EmailObfuscation)
			},
		},
		{
			name:    "server_side_exclude setting",
			setting: cloudflare.ZoneSetting{ID: "server_side_exclude", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.ServerSideExclude)
			},
		},
		{
			name:    "hotlink_protection setting",
			setting: cloudflare.ZoneSetting{ID: "hotlink_protection", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.HotlinkProtection)
			},
		},
		{
			name:    "challenge_ttl setting",
			setting: cloudflare.ZoneSetting{ID: "challenge_ttl", Value: float64(1800)},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, 1800, result.ChallengePassage)
			},
		},
		{
			name:    "waf setting",
			setting: cloudflare.ZoneSetting{ID: "waf", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.WAF)
			},
		},

		// Performance settings
		{
			name:    "brotli setting",
			setting: cloudflare.ZoneSetting{ID: "brotli", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.Brotli)
			},
		},
		{
			name:    "http2 setting",
			setting: cloudflare.ZoneSetting{ID: "http2", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.HTTP2)
			},
		},
		{
			name:    "http3 setting",
			setting: cloudflare.ZoneSetting{ID: "http3", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.HTTP3)
			},
		},
		{
			name:    "0rtt setting",
			setting: cloudflare.ZoneSetting{ID: "0rtt", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.ZeroRTT)
			},
		},
		{
			name: "minify setting",
			setting: cloudflare.ZoneSetting{
				ID: "minify",
				Value: map[string]any{
					"html": "on",
					"css":  "on",
					"js":   "off",
				},
			},
			validate: func(t *testing.T, result *ZoneSettings) {
				require.NotNil(t, result.Minify)
				assert.True(t, result.Minify.HTML)
				assert.True(t, result.Minify.CSS)
				assert.False(t, result.Minify.JS)
			},
		},
		{
			name:    "minify setting with invalid type",
			setting: cloudflare.ZoneSetting{ID: "minify", Value: "invalid"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Nil(t, result.Minify)
			},
		},
		{
			name:    "polish setting",
			setting: cloudflare.ZoneSetting{ID: "polish", Value: "lossy"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "lossy", result.Polish)
			},
		},
		{
			name:    "webp setting",
			setting: cloudflare.ZoneSetting{ID: "webp", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.WebP)
			},
		},
		{
			name:    "mirage setting",
			setting: cloudflare.ZoneSetting{ID: "mirage", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.Mirage)
			},
		},
		{
			name:    "early_hints setting",
			setting: cloudflare.ZoneSetting{ID: "early_hints", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.EarlyHints)
			},
		},
		{
			name:    "rocket_loader setting",
			setting: cloudflare.ZoneSetting{ID: "rocket_loader", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.RocketLoader)
			},
		},
		{
			name:    "prefetch_preload setting",
			setting: cloudflare.ZoneSetting{ID: "prefetch_preload", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.PrefetchPreload)
			},
		},
		{
			name:    "ip_geolocation setting",
			setting: cloudflare.ZoneSetting{ID: "ip_geolocation", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.IPGeolocation)
			},
		},
		{
			name:    "websockets setting",
			setting: cloudflare.ZoneSetting{ID: "websockets", Value: "on"},
			validate: func(t *testing.T, result *ZoneSettings) {
				assert.Equal(t, "on", result.Websockets)
			},
		},

		// Unknown setting (should be ignored)
		{
			name:    "unknown setting",
			setting: cloudflare.ZoneSetting{ID: "unknown_setting", Value: "something"},
			validate: func(t *testing.T, result *ZoneSettings) {
				// Should not panic and result should be empty
				assert.Equal(t, "", result.SSLMode)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ZoneSettings{}
			parseZoneSetting(result, tt.setting)
			tt.validate(t, result)
		})
	}
}

func TestZoneSettingsStruct(t *testing.T) {
	settings := ZoneSettings{
		// SSL/TLS
		SSLMode:                 "strict",
		MinTLSVersion:           "1.2",
		TLS13:                   "on",
		AlwaysUseHTTPS:          "on",
		AutomaticHTTPSRewrites:  "on",
		OpportunisticEncryption: "on",
		TLSClientAuth:           "on",

		// Cache
		BrowserCacheTTL: 14400,
		DevelopmentMode: "off",
		CacheLevel:      "aggressive",
		AlwaysOnline:    "on",
		SortQueryString: "on",

		// Security
		SecurityLevel:     "high",
		BrowserCheck:      "on",
		EmailObfuscation:  "on",
		ServerSideExclude: "on",
		HotlinkProtection: "on",
		ChallengePassage:  1800,
		WAF:               "on",

		// Performance
		Brotli:          "on",
		HTTP2:           "on",
		HTTP3:           "on",
		ZeroRTT:         "on",
		Minify:          &MinifySettings{HTML: true, CSS: true, JS: true},
		Polish:          "lossy",
		WebP:            "on",
		Mirage:          "on",
		EarlyHints:      "on",
		RocketLoader:    "on",
		PrefetchPreload: "on",
		IPGeolocation:   "on",
		Websockets:      "on",
	}

	// Verify all fields are set correctly
	assert.Equal(t, "strict", settings.SSLMode)
	assert.Equal(t, "1.2", settings.MinTLSVersion)
	assert.Equal(t, 14400, settings.BrowserCacheTTL)
	assert.Equal(t, "high", settings.SecurityLevel)
	assert.Equal(t, 1800, settings.ChallengePassage)
	assert.NotNil(t, settings.Minify)
	assert.True(t, settings.Minify.HTML)
	assert.True(t, settings.Minify.CSS)
	assert.True(t, settings.Minify.JS)
}

func TestMinifySettings(t *testing.T) {
	tests := []struct {
		name     string
		settings MinifySettings
	}{
		{
			name: "all enabled",
			settings: MinifySettings{
				HTML: true,
				CSS:  true,
				JS:   true,
			},
		},
		{
			name: "all disabled",
			settings: MinifySettings{
				HTML: false,
				CSS:  false,
				JS:   false,
			},
		},
		{
			name: "mixed settings",
			settings: MinifySettings{
				HTML: true,
				CSS:  false,
				JS:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify struct can be created without issues
			assert.NotNil(t, &tt.settings)
		})
	}
}
