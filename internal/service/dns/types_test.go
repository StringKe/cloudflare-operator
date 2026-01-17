// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package dns

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

func TestDNSRecordConfig(t *testing.T) {
	priority := 10
	config := DNSRecordConfig{
		Name:     "www.example.com",
		Type:     "A",
		Content:  "192.0.2.1",
		TTL:      300,
		Proxied:  true,
		Priority: &priority,
		Comment:  "Main website",
		Tags:     []string{"production", "web"},
	}

	assert.Equal(t, "www.example.com", config.Name)
	assert.Equal(t, "A", config.Type)
	assert.Equal(t, "192.0.2.1", config.Content)
	assert.Equal(t, 300, config.TTL)
	assert.True(t, config.Proxied)
	assert.Equal(t, 10, *config.Priority)
	assert.Equal(t, "Main website", config.Comment)
	assert.Len(t, config.Tags, 2)
}

func TestDNSRecordConfigTypes(t *testing.T) {
	tests := []struct {
		name    string
		config  DNSRecordConfig
		recType string
	}{
		{
			name: "A record",
			config: DNSRecordConfig{
				Name:    "www.example.com",
				Type:    "A",
				Content: "192.0.2.1",
				Proxied: true,
			},
			recType: "A",
		},
		{
			name: "AAAA record",
			config: DNSRecordConfig{
				Name:    "www.example.com",
				Type:    "AAAA",
				Content: "2001:db8::1",
				Proxied: true,
			},
			recType: "AAAA",
		},
		{
			name: "CNAME record",
			config: DNSRecordConfig{
				Name:    "alias.example.com",
				Type:    "CNAME",
				Content: "www.example.com",
			},
			recType: "CNAME",
		},
		{
			name: "TXT record",
			config: DNSRecordConfig{
				Name:    "example.com",
				Type:    "TXT",
				Content: "v=spf1 include:_spf.example.com ~all",
			},
			recType: "TXT",
		},
		{
			name: "MX record",
			config: DNSRecordConfig{
				Name:     "example.com",
				Type:     "MX",
				Content:  "mail.example.com",
				Priority: intPtr(10),
			},
			recType: "MX",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.recType, tt.config.Type)
			assert.NotEmpty(t, tt.config.Name)
		})
	}
}

func TestDNSRecordData(t *testing.T) {
	tests := []struct {
		name string
		data DNSRecordData
	}{
		{
			name: "SRV data",
			data: DNSRecordData{
				Service: "_http",
				Proto:   "_tcp",
				Weight:  10,
				Port:    80,
				Target:  "server.example.com",
			},
		},
		{
			name: "CAA data",
			data: DNSRecordData{
				Flags: 0,
				Tag:   "issue",
				Value: "letsencrypt.org",
			},
		},
		{
			name: "CERT data",
			data: DNSRecordData{
				Algorithm:   8,
				Certificate: "BASE64CERTDATA",
				KeyTag:      12345,
			},
		},
		{
			name: "SSHFP data",
			data: DNSRecordData{
				Algorithm:    1,
				MatchingType: 1,
			},
		},
		{
			name: "TLSA data",
			data: DNSRecordData{
				Usage:        3,
				Selector:     1,
				MatchingType: 1,
				Certificate:  "TLSACERTHASH",
			},
		},
		{
			name: "LOC data",
			data: DNSRecordData{
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
		},
		{
			name: "URI data",
			data: DNSRecordData{
				Weight:     10,
				Target:     "https://example.com/resource",
				ContentURI: "https://example.com/resource",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify the struct can be created without issues
			assert.NotNil(t, &tt.data)
		})
	}
}

func TestDNSRecordDataSRV(t *testing.T) {
	data := DNSRecordData{
		Service: "_sip",
		Proto:   "_udp",
		Weight:  5,
		Port:    5060,
		Target:  "sipserver.example.com",
	}

	assert.Equal(t, "_sip", data.Service)
	assert.Equal(t, "_udp", data.Proto)
	assert.Equal(t, 5, data.Weight)
	assert.Equal(t, 5060, data.Port)
	assert.Equal(t, "sipserver.example.com", data.Target)
}

func TestDNSRecordDataCAA(t *testing.T) {
	data := DNSRecordData{
		Flags: 128,
		Tag:   "issuewild",
		Value: "example.com",
	}

	assert.Equal(t, 128, data.Flags)
	assert.Equal(t, "issuewild", data.Tag)
	assert.Equal(t, "example.com", data.Value)
}

func TestDNSRecordDataLOC(t *testing.T) {
	data := DNSRecordData{
		LatDegrees:    37,
		LatMinutes:    46,
		LatSeconds:    "26.424",
		LatDirection:  "N",
		LongDegrees:   122,
		LongMinutes:   25,
		LongSeconds:   "9.132",
		LongDirection: "W",
		Altitude:      "0.00m",
		Size:          "1.00m",
		PrecisionHorz: "10000.00m",
		PrecisionVert: "10.00m",
	}

	assert.Equal(t, 37, data.LatDegrees)
	assert.Equal(t, "N", data.LatDirection)
	assert.Equal(t, 122, data.LongDegrees)
	assert.Equal(t, "W", data.LongDirection)
}

func TestRegisterOptions(t *testing.T) {
	opts := RegisterOptions{
		ZoneID:    "zone-123",
		AccountID: "account-456",
		RecordID:  "record-789",
		Source: service.Source{
			Kind:      "DNSRecord",
			Namespace: "default",
			Name:      "www-record",
		},
		Config: DNSRecordConfig{
			Name:    "www.example.com",
			Type:    "A",
			Content: "192.0.2.1",
			Proxied: true,
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	assert.Equal(t, "zone-123", opts.ZoneID)
	assert.Equal(t, "account-456", opts.AccountID)
	assert.Equal(t, "record-789", opts.RecordID)
	assert.Equal(t, "DNSRecord", opts.Source.Kind)
	assert.Equal(t, "default", opts.Source.Namespace)
	assert.Equal(t, "www-record", opts.Source.Name)
	assert.Equal(t, "www.example.com", opts.Config.Name)
	assert.Equal(t, "A", opts.Config.Type)
}

func TestRegisterOptionsWithoutRecordID(t *testing.T) {
	opts := RegisterOptions{
		ZoneID:    "zone-123",
		AccountID: "account-456",
		Source: service.Source{
			Kind:      "DNSRecord",
			Namespace: "default",
			Name:      "new-record",
		},
		Config: DNSRecordConfig{
			Name:    "new.example.com",
			Type:    "CNAME",
			Content: "www.example.com",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	// RecordID should be empty for new records
	assert.Empty(t, opts.RecordID)
	assert.NotEmpty(t, opts.ZoneID)
}

func TestSyncResult(t *testing.T) {
	result := SyncResult{
		RecordID: "record-123",
		ZoneID:   "zone-456",
		FQDN:     "www.example.com",
	}

	assert.Equal(t, "record-123", result.RecordID)
	assert.Equal(t, "zone-456", result.ZoneID)
	assert.Equal(t, "www.example.com", result.FQDN)
}

func TestDNSRecordConfigWithData(t *testing.T) {
	config := DNSRecordConfig{
		Name:    "_http._tcp.example.com",
		Type:    "SRV",
		Comment: "HTTP service record",
		Data: &DNSRecordData{
			Service: "_http",
			Proto:   "_tcp",
			Weight:  10,
			Port:    80,
			Target:  "www.example.com",
		},
	}

	assert.NotNil(t, config.Data)
	assert.Equal(t, "_http", config.Data.Service)
	assert.Equal(t, 80, config.Data.Port)
}

func TestEmptyDNSRecordData(t *testing.T) {
	data := DNSRecordData{}

	assert.Empty(t, data.Service)
	assert.Empty(t, data.Proto)
	assert.Equal(t, 0, data.Weight)
	assert.Equal(t, 0, data.Port)
	assert.Empty(t, data.Target)
}

// Helper function
func intPtr(i int) *int {
	return &i
}
