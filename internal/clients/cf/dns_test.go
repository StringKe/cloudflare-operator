// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:revive // cognitive-complexity: table-driven test with many test cases
func TestConvertDataToMap(t *testing.T) {
	tests := []struct {
		name     string
		data     *DNSRecordDataParams
		wantNil  bool
		validate func(t *testing.T, result map[string]interface{})
	}{
		{
			name:    "nil data",
			data:    nil,
			wantNil: true,
		},
		{
			name:    "empty data",
			data:    &DNSRecordDataParams{},
			wantNil: true,
		},
		{
			name: "SRV record data",
			data: &DNSRecordDataParams{
				Service: "_http",
				Proto:   "_tcp",
				Weight:  10,
				Port:    80,
				Target:  "server.example.com",
			},
			validate: func(t *testing.T, result map[string]interface{}) {
				assert.Equal(t, "_http", result["service"])
				assert.Equal(t, "_tcp", result["proto"])
				assert.Equal(t, 10, result["weight"])
				assert.Equal(t, 80, result["port"])
				assert.Equal(t, "server.example.com", result["target"])
			},
		},
		{
			name: "CAA record data",
			data: &DNSRecordDataParams{
				Flags: 0,
				Tag:   "issue",
				Value: "letsencrypt.org",
			},
			validate: func(t *testing.T, result map[string]interface{}) {
				// Flags=0 should not be included (zero value)
				_, hasFlags := result["flags"]
				assert.False(t, hasFlags)
				assert.Equal(t, "issue", result["tag"])
				assert.Equal(t, "letsencrypt.org", result["value"])
			},
		},
		{
			name: "CAA record data with non-zero flags",
			data: &DNSRecordDataParams{
				Flags: 128,
				Tag:   "issuewild",
				Value: "example.com",
			},
			validate: func(t *testing.T, result map[string]interface{}) {
				assert.Equal(t, 128, result["flags"])
				assert.Equal(t, "issuewild", result["tag"])
				assert.Equal(t, "example.com", result["value"])
			},
		},
		{
			name: "CERT record data",
			data: &DNSRecordDataParams{
				Algorithm:   8,
				Certificate: "BASE64CERTDATA",
				KeyTag:      12345,
			},
			validate: func(t *testing.T, result map[string]interface{}) {
				assert.Equal(t, 8, result["algorithm"])
				assert.Equal(t, "BASE64CERTDATA", result["certificate"])
				assert.Equal(t, 12345, result["key_tag"])
			},
		},
		{
			name: "SSHFP record data",
			data: &DNSRecordDataParams{
				Algorithm:    1,
				MatchingType: 1,
			},
			validate: func(t *testing.T, result map[string]interface{}) {
				assert.Equal(t, 1, result["algorithm"])
				assert.Equal(t, 1, result["matching_type"])
			},
		},
		{
			name: "TLSA record data",
			data: &DNSRecordDataParams{
				Usage:        3,
				Selector:     1,
				MatchingType: 1,
				Certificate:  "TLSACERTHASH",
			},
			validate: func(t *testing.T, result map[string]interface{}) {
				assert.Equal(t, 3, result["usage"])
				assert.Equal(t, 1, result["selector"])
				assert.Equal(t, 1, result["matching_type"])
				assert.Equal(t, "TLSACERTHASH", result["certificate"])
			},
		},
		{
			name: "LOC record data - full",
			data: &DNSRecordDataParams{
				LatDegrees:    37,
				LatMinutes:    46,
				LatSeconds:    "26.424",
				LatDirection:  "N",
				LongDegrees:   122,
				LongMinutes:   25,
				LongSeconds:   "9.132",
				LongDirection: "W",
				Altitude:      "10.00m",
				Size:          "10.00m",
				PrecisionHorz: "10.00m",
				PrecisionVert: "10.00m",
			},
			validate: func(t *testing.T, result map[string]interface{}) {
				assert.Equal(t, 37, result["lat_degrees"])
				assert.Equal(t, 46, result["lat_minutes"])
				assert.Equal(t, "26.424", result["lat_seconds"])
				assert.Equal(t, "N", result["lat_direction"])
				assert.Equal(t, 122, result["long_degrees"])
				assert.Equal(t, 25, result["long_minutes"])
				assert.Equal(t, "9.132", result["long_seconds"])
				assert.Equal(t, "W", result["long_direction"])
				assert.Equal(t, "10.00m", result["altitude"])
				assert.Equal(t, "10.00m", result["size"])
				assert.Equal(t, "10.00m", result["precision_horz"])
				assert.Equal(t, "10.00m", result["precision_vert"])
			},
		},
		{
			name: "URI record data",
			data: &DNSRecordDataParams{
				Weight:     10,
				Target:     "https://example.com/resource",
				ContentURI: "https://example.com/resource",
			},
			validate: func(t *testing.T, result map[string]interface{}) {
				assert.Equal(t, 10, result["weight"])
				assert.Equal(t, "https://example.com/resource", result["target"])
				assert.Equal(t, "https://example.com/resource", result["content"])
			},
		},
		{
			name: "partial SRV data - only required fields",
			data: &DNSRecordDataParams{
				Service: "_sip",
				Proto:   "_udp",
				Target:  "sipserver.example.com",
			},
			validate: func(t *testing.T, result map[string]interface{}) {
				assert.Equal(t, "_sip", result["service"])
				assert.Equal(t, "_udp", result["proto"])
				assert.Equal(t, "sipserver.example.com", result["target"])
				// Weight and Port are 0, should not be included
				_, hasWeight := result["weight"]
				_, hasPort := result["port"]
				assert.False(t, hasWeight)
				assert.False(t, hasPort)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertDataToMap(tt.data)

			if tt.wantNil {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestDNSRecordParams(t *testing.T) {
	priority := 10

	tests := []struct {
		name   string
		params DNSRecordParams
	}{
		{
			name: "A record",
			params: DNSRecordParams{
				Name:    "www.example.com",
				Type:    "A",
				Content: "192.0.2.1",
				TTL:     300,
				Proxied: true,
				Comment: "Web server",
			},
		},
		{
			name: "AAAA record",
			params: DNSRecordParams{
				Name:    "ipv6.example.com",
				Type:    "AAAA",
				Content: "2001:db8::1",
				TTL:     300,
				Proxied: true,
			},
		},
		{
			name: "CNAME record",
			params: DNSRecordParams{
				Name:    "alias.example.com",
				Type:    "CNAME",
				Content: "www.example.com",
				TTL:     300,
				Proxied: false,
			},
		},
		{
			name: "MX record with priority",
			params: DNSRecordParams{
				Name:     "example.com",
				Type:     "MX",
				Content:  "mail.example.com",
				TTL:      3600,
				Priority: &priority,
			},
		},
		{
			name: "TXT record",
			params: DNSRecordParams{
				Name:    "example.com",
				Type:    "TXT",
				Content: "v=spf1 include:_spf.example.com ~all",
				TTL:     3600,
			},
		},
		{
			name: "SRV record with data",
			params: DNSRecordParams{
				Name:     "_http._tcp.example.com",
				Type:     "SRV",
				Priority: &priority,
				Data: &DNSRecordDataParams{
					Service: "_http",
					Proto:   "_tcp",
					Weight:  5,
					Port:    80,
					Target:  "server.example.com",
				},
			},
		},
		{
			name: "CAA record with data",
			params: DNSRecordParams{
				Name: "example.com",
				Type: "CAA",
				TTL:  3600,
				Data: &DNSRecordDataParams{
					Flags: 0,
					Tag:   "issue",
					Value: "letsencrypt.org",
				},
			},
		},
		{
			name: "record with tags",
			params: DNSRecordParams{
				Name:    "tagged.example.com",
				Type:    "A",
				Content: "192.0.2.2",
				TTL:     300,
				Tags:    []string{"production", "web"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.params.Name)
			assert.NotEmpty(t, tt.params.Type)
		})
	}
}

func TestDNSRecordDataParamsAllFields(t *testing.T) {
	data := DNSRecordDataParams{
		// SRV fields
		Service: "_http",
		Proto:   "_tcp",
		Weight:  10,
		Port:    80,
		Target:  "server.example.com",

		// CAA fields
		Flags: 128,
		Tag:   "issue",
		Value: "ca.example.com",

		// CERT/SSHFP/TLSA fields
		Algorithm:    8,
		Certificate:  "CERTDATA",
		KeyTag:       12345,
		Usage:        3,
		Selector:     1,
		MatchingType: 1,

		// LOC fields
		LatDegrees:    37,
		LatMinutes:    46,
		LatSeconds:    "26.424",
		LatDirection:  "N",
		LongDegrees:   122,
		LongMinutes:   25,
		LongSeconds:   "9.132",
		LongDirection: "W",
		Altitude:      "10.00m",
		Size:          "10.00m",
		PrecisionHorz: "10.00m",
		PrecisionVert: "10.00m",

		// URI field
		ContentURI: "https://example.com/resource",
	}

	result := convertDataToMap(&data)
	require.NotNil(t, result)

	// Verify all non-zero fields are present
	expectedKeys := []string{
		"service", "proto", "weight", "port", "target",
		"flags", "tag", "value",
		"algorithm", "certificate", "key_tag", "usage", "selector", "matching_type",
		"lat_degrees", "lat_minutes", "lat_seconds", "lat_direction",
		"long_degrees", "long_minutes", "long_seconds", "long_direction",
		"altitude", "size", "precision_horz", "precision_vert",
		"content",
	}

	for _, key := range expectedKeys {
		assert.Contains(t, result, key, "Result should contain key %s", key)
	}
}

func TestDNSRecordResult(t *testing.T) {
	result := DNSRecordResult{
		ID:      "record-123",
		ZoneID:  "zone-456",
		Name:    "www.example.com",
		Type:    "A",
		Content: "192.0.2.1",
		TTL:     300,
		Proxied: true,
	}

	assert.Equal(t, "record-123", result.ID)
	assert.Equal(t, "zone-456", result.ZoneID)
	assert.Equal(t, "www.example.com", result.Name)
	assert.Equal(t, "A", result.Type)
	assert.Equal(t, "192.0.2.1", result.Content)
	assert.Equal(t, 300, result.TTL)
	assert.True(t, result.Proxied)
}
