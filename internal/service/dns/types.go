// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package dns provides types and service for DNS record configuration management.
package dns

import (
	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// DNSRecordConfig represents a single DNS record configuration.
// Each DNSRecord K8s resource contributes one DNSRecordConfig to its SyncState.
type DNSRecordConfig struct {
	// Name is the DNS record name (e.g., "api.example.com")
	Name string `json:"name"`
	// Type is the DNS record type (A, AAAA, CNAME, TXT, MX, etc.)
	Type string `json:"type"`
	// Content is the DNS record content (e.g., IP address, CNAME target)
	Content string `json:"content"`
	// TTL is the time-to-live in seconds (1 = automatic)
	TTL int `json:"ttl,omitempty"`
	// Proxied indicates if the record is proxied through Cloudflare
	Proxied bool `json:"proxied,omitempty"`
	// Priority is used for MX and SRV records
	Priority *int `json:"priority,omitempty"`
	// Comment is a user-provided comment for the DNS record
	Comment string `json:"comment,omitempty"`
	// Tags are user-provided tags for the DNS record (Enterprise only)
	Tags []string `json:"tags,omitempty"`
	// Data contains additional record-type-specific data (SRV, CAA, etc.)
	Data *DNSRecordData `json:"data,omitempty"`
}

// DNSRecordData contains record-type-specific data fields.
// These match the API types for simplicity.
type DNSRecordData struct {
	// SRV record data
	Service string `json:"service,omitempty"`
	Proto   string `json:"proto,omitempty"`
	Weight  int    `json:"weight,omitempty"`
	Port    int    `json:"port,omitempty"`
	Target  string `json:"target,omitempty"`

	// CAA record data
	Flags int    `json:"flags,omitempty"`
	Tag   string `json:"tag,omitempty"`
	Value string `json:"value,omitempty"`

	// CERT/SSHFP/TLSA record data
	Algorithm    int    `json:"algorithm,omitempty"`
	Certificate  string `json:"certificate,omitempty"`
	KeyTag       int    `json:"keyTag,omitempty"`
	Usage        int    `json:"usage,omitempty"`
	Selector     int    `json:"selector,omitempty"`
	MatchingType int    `json:"matchingType,omitempty"`

	// LOC record data
	LatDegrees    int    `json:"latDegrees,omitempty"`
	LatMinutes    int    `json:"latMinutes,omitempty"`
	LatSeconds    string `json:"latSeconds,omitempty"`
	LatDirection  string `json:"latDirection,omitempty"`
	LongDegrees   int    `json:"longDegrees,omitempty"`
	LongMinutes   int    `json:"longMinutes,omitempty"`
	LongSeconds   string `json:"longSeconds,omitempty"`
	LongDirection string `json:"longDirection,omitempty"`
	Altitude      string `json:"altitude,omitempty"`
	Size          string `json:"size,omitempty"`
	PrecisionHorz string `json:"precisionHorz,omitempty"`
	PrecisionVert string `json:"precisionVert,omitempty"`

	// URI record data
	ContentURI string `json:"content,omitempty"`
}

// RegisterOptions contains options for registering a DNS record configuration.
type RegisterOptions struct {
	// ZoneID is the Cloudflare Zone ID
	ZoneID string
	// AccountID is the Cloudflare Account ID
	AccountID string
	// RecordID is the Cloudflare DNS record ID (if already created)
	// Used as the CloudflareID in SyncState for existing records
	// For new records, a placeholder is used until the record is created
	RecordID string
	// Source identifies the K8s resource contributing this configuration
	Source service.Source
	// Config contains the DNS record configuration
	Config DNSRecordConfig
	// CredentialsRef references the CloudflareCredentials to use
	CredentialsRef v1alpha2.CredentialsReference
}

// SyncResult contains the result of a successful DNS record sync operation.
type SyncResult struct {
	// RecordID is the Cloudflare DNS record ID after sync
	RecordID string
	// ZoneID is the Cloudflare Zone ID
	ZoneID string
	// FQDN is the fully-qualified domain name
	FQDN string
}
