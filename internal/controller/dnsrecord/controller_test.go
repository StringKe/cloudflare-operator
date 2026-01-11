// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package dnsrecord

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

//nolint:revive // cognitive-complexity: table-driven test with many test cases
func TestBuildRecordData(t *testing.T) {
	reconciler := &DNSRecordReconciler{}

	tests := []struct {
		name     string
		data     *networkingv1alpha2.DNSRecordData
		wantNil  bool
		validate func(t *testing.T, result *cf.DNSRecordDataParams)
	}{
		{
			name:    "nil data returns nil",
			data:    nil,
			wantNil: true,
		},
		{
			name: "SRV record data",
			data: &networkingv1alpha2.DNSRecordData{
				Service: "_http",
				Proto:   "_tcp",
				Weight:  10,
				Port:    80,
				Target:  "server.example.com",
			},
			validate: func(t *testing.T, result *cf.DNSRecordDataParams) {
				require.NotNil(t, result)
				assert.Equal(t, "_http", result.Service)
				assert.Equal(t, "_tcp", result.Proto)
				assert.Equal(t, 10, result.Weight)
				assert.Equal(t, 80, result.Port)
				assert.Equal(t, "server.example.com", result.Target)
			},
		},
		{
			name: "CAA record data",
			data: &networkingv1alpha2.DNSRecordData{
				Flags: 0,
				Tag:   "issue",
				Value: "letsencrypt.org",
			},
			validate: func(t *testing.T, result *cf.DNSRecordDataParams) {
				require.NotNil(t, result)
				assert.Equal(t, 0, result.Flags)
				assert.Equal(t, "issue", result.Tag)
				assert.Equal(t, "letsencrypt.org", result.Value)
			},
		},
		{
			name: "CAA record data with non-zero flags",
			data: &networkingv1alpha2.DNSRecordData{
				Flags: 128,
				Tag:   "issuewild",
				Value: "example.com",
			},
			validate: func(t *testing.T, result *cf.DNSRecordDataParams) {
				require.NotNil(t, result)
				assert.Equal(t, 128, result.Flags)
				assert.Equal(t, "issuewild", result.Tag)
			},
		},
		{
			name: "CERT record data",
			data: &networkingv1alpha2.DNSRecordData{
				Algorithm:   8,
				Certificate: "BASE64CERTDATA",
				KeyTag:      12345,
			},
			validate: func(t *testing.T, result *cf.DNSRecordDataParams) {
				require.NotNil(t, result)
				assert.Equal(t, 8, result.Algorithm)
				assert.Equal(t, "BASE64CERTDATA", result.Certificate)
				assert.Equal(t, 12345, result.KeyTag)
			},
		},
		{
			name: "SSHFP record data",
			data: &networkingv1alpha2.DNSRecordData{
				Algorithm:    2,
				MatchingType: 1,
			},
			validate: func(t *testing.T, result *cf.DNSRecordDataParams) {
				require.NotNil(t, result)
				assert.Equal(t, 2, result.Algorithm)
				assert.Equal(t, 1, result.MatchingType)
			},
		},
		{
			name: "TLSA record data",
			data: &networkingv1alpha2.DNSRecordData{
				Usage:        3,
				Selector:     1,
				MatchingType: 1,
				Certificate:  "TLSACERTHASH",
			},
			validate: func(t *testing.T, result *cf.DNSRecordDataParams) {
				require.NotNil(t, result)
				assert.Equal(t, 3, result.Usage)
				assert.Equal(t, 1, result.Selector)
				assert.Equal(t, 1, result.MatchingType)
				assert.Equal(t, "TLSACERTHASH", result.Certificate)
			},
		},
		{
			name: "LOC record data",
			data: &networkingv1alpha2.DNSRecordData{
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
			validate: func(t *testing.T, result *cf.DNSRecordDataParams) {
				require.NotNil(t, result)
				assert.Equal(t, 37, result.LatDegrees)
				assert.Equal(t, 46, result.LatMinutes)
				assert.Equal(t, "26.424", result.LatSeconds)
				assert.Equal(t, "N", result.LatDirection)
				assert.Equal(t, 122, result.LongDegrees)
				assert.Equal(t, 25, result.LongMinutes)
				assert.Equal(t, "9.132", result.LongSeconds)
				assert.Equal(t, "W", result.LongDirection)
				assert.Equal(t, "10.00m", result.Altitude)
				assert.Equal(t, "10.00m", result.Size)
				assert.Equal(t, "10.00m", result.PrecisionHorz)
				assert.Equal(t, "10.00m", result.PrecisionVert)
			},
		},
		{
			name: "URI record data",
			data: &networkingv1alpha2.DNSRecordData{
				Weight:     10,
				Target:     "https://example.com/resource",
				ContentURI: "https://example.com/resource",
			},
			validate: func(t *testing.T, result *cf.DNSRecordDataParams) {
				require.NotNil(t, result)
				assert.Equal(t, 10, result.Weight)
				assert.Equal(t, "https://example.com/resource", result.Target)
				assert.Equal(t, "https://example.com/resource", result.ContentURI)
			},
		},
		{
			name: "empty data struct",
			data: &networkingv1alpha2.DNSRecordData{},
			validate: func(t *testing.T, result *cf.DNSRecordDataParams) {
				require.NotNil(t, result)
				// All fields should be zero values
				assert.Empty(t, result.Service)
				assert.Empty(t, result.Proto)
				assert.Equal(t, 0, result.Weight)
			},
		},
		{
			name: "partial SRV data",
			data: &networkingv1alpha2.DNSRecordData{
				Service: "_sip",
				Proto:   "_udp",
				Target:  "sipserver.example.com",
			},
			validate: func(t *testing.T, result *cf.DNSRecordDataParams) {
				require.NotNil(t, result)
				assert.Equal(t, "_sip", result.Service)
				assert.Equal(t, "_udp", result.Proto)
				assert.Equal(t, "sipserver.example.com", result.Target)
				assert.Equal(t, 0, result.Weight)
				assert.Equal(t, 0, result.Port)
			},
		},
		{
			name: "full record data with all fields",
			data: &networkingv1alpha2.DNSRecordData{
				// SRV fields
				Service: "_https",
				Proto:   "_tcp",
				Weight:  5,
				Port:    443,
				Target:  "web.example.com",
				// CAA fields
				Flags: 1,
				Tag:   "iodef",
				Value: "mailto:admin@example.com",
				// CERT fields
				Algorithm:   13,
				Certificate: "FULLCERTDATA",
				KeyTag:      54321,
				// TLSA fields
				Usage:        2,
				Selector:     0,
				MatchingType: 2,
				// LOC fields
				LatDegrees:    51,
				LatMinutes:    30,
				LatSeconds:    "0.000",
				LatDirection:  "N",
				LongDegrees:   0,
				LongMinutes:   7,
				LongSeconds:   "0.000",
				LongDirection: "W",
				Altitude:      "100.00m",
				Size:          "1.00m",
				PrecisionHorz: "1.00m",
				PrecisionVert: "1.00m",
				// URI field
				ContentURI: "https://api.example.com/v1",
			},
			validate: func(t *testing.T, result *cf.DNSRecordDataParams) {
				require.NotNil(t, result)
				// Verify all fields are mapped correctly
				assert.Equal(t, "_https", result.Service)
				assert.Equal(t, 443, result.Port)
				assert.Equal(t, 1, result.Flags)
				assert.Equal(t, 13, result.Algorithm)
				assert.Equal(t, 51, result.LatDegrees)
				assert.Equal(t, "https://api.example.com/v1", result.ContentURI)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reconciler.buildRecordData(tt.data)

			if tt.wantNil {
				assert.Nil(t, result)
				return
			}

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}
