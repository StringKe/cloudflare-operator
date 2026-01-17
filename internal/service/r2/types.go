// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package r2 provides services for managing Cloudflare R2 resource configurations.
//
//nolint:revive // max-public-structs is acceptable for comprehensive R2 API types
package r2

import (
	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// Resource Types for SyncState - use constants from v1alpha2
const (
	// ResourceTypeR2Bucket is the SyncState resource type for R2Bucket
	ResourceTypeR2Bucket = v1alpha2.SyncResourceR2Bucket
	// ResourceTypeR2BucketDomain is the SyncState resource type for R2BucketDomain
	ResourceTypeR2BucketDomain = v1alpha2.SyncResourceR2BucketDomain
	// ResourceTypeR2BucketNotification is the SyncState resource type for R2BucketNotification
	ResourceTypeR2BucketNotification = v1alpha2.SyncResourceR2BucketNotification

	// Priority constants
	PriorityR2Bucket             = 100
	PriorityR2BucketDomain       = 100
	PriorityR2BucketNotification = 100
)

// R2BucketConfig contains the configuration for an R2 Bucket.
type R2BucketConfig struct {
	// Name is the bucket name in Cloudflare
	Name string `json:"name"`
	// LocationHint is the preferred geographic location
	LocationHint string `json:"locationHint,omitempty"`
	// CORS contains CORS rules for the bucket
	CORS []v1alpha2.R2CORSRule `json:"cors,omitempty"`
	// Lifecycle contains object lifecycle rules
	Lifecycle *R2LifecycleConfig `json:"lifecycle,omitempty"`
}

// R2LifecycleConfig wraps the lifecycle rules configuration.
type R2LifecycleConfig struct {
	// Rules contains the lifecycle rules
	Rules []v1alpha2.R2LifecycleRule `json:"rules,omitempty"`
	// DeletionPolicy determines behavior on K8s resource deletion
	DeletionPolicy string `json:"deletionPolicy,omitempty"`
}

// R2BucketDomainConfig contains the configuration for an R2 custom domain.
type R2BucketDomainConfig struct {
	// BucketName is the target R2 bucket
	BucketName string `json:"bucketName"`
	// Domain is the custom domain FQDN
	Domain string `json:"domain"`
	// ZoneID is the Cloudflare zone ID (can be empty for auto-lookup)
	ZoneID string `json:"zoneId,omitempty"`
	// MinTLS is the minimum TLS version
	MinTLS string `json:"minTls,omitempty"`
	// EnablePublicAccess enables public bucket access
	EnablePublicAccess *bool `json:"enablePublicAccess,omitempty"`
}

// R2BucketNotificationConfig contains the configuration for R2 event notifications.
type R2BucketNotificationConfig struct {
	// BucketName is the target R2 bucket
	BucketName string `json:"bucketName"`
	// QueueName is the Cloudflare Queue name
	QueueName string `json:"queueName"`
	// Rules defines notification rules
	Rules []v1alpha2.R2NotificationRule `json:"rules"`
}

// R2BucketRegisterOptions contains options for registering an R2Bucket.
type R2BucketRegisterOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// BucketName is the existing bucket name (empty for new)
	BucketName string
	// Source is the K8s resource source
	Source service.Source
	// Config is the bucket configuration
	Config R2BucketConfig
	// CredentialsRef references the CloudflareCredentials resource
	CredentialsRef v1alpha2.CredentialsReference
}

// R2BucketDomainRegisterOptions contains options for registering an R2BucketDomain.
type R2BucketDomainRegisterOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// DomainID is the existing domain configuration ID (empty for new)
	DomainID string
	// ZoneID is the Cloudflare zone ID
	ZoneID string
	// Source is the K8s resource source
	Source service.Source
	// Config is the domain configuration
	Config R2BucketDomainConfig
	// CredentialsRef references the CloudflareCredentials resource
	CredentialsRef v1alpha2.CredentialsReference
}

// R2BucketNotificationRegisterOptions contains options for registering an R2BucketNotification.
type R2BucketNotificationRegisterOptions struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// QueueID is the resolved Cloudflare Queue ID
	QueueID string
	// Source is the K8s resource source
	Source service.Source
	// Config is the notification configuration
	Config R2BucketNotificationConfig
	// CredentialsRef references the CloudflareCredentials resource
	CredentialsRef v1alpha2.CredentialsReference
}

// SyncResult contains the result of a sync operation.
type SyncResult struct {
	// ID is the Cloudflare resource ID
	ID string
	// AccountID is the Cloudflare account ID
	AccountID string
}

// R2BucketSyncResult contains R2Bucket-specific sync result.
type R2BucketSyncResult struct {
	SyncResult
	// BucketName is the actual bucket name
	BucketName string
	// Location is the bucket location
	Location string
	// CreatedAt is the creation timestamp
	CreatedAt string
	// CORSRulesCount is the number of CORS rules
	CORSRulesCount int
	// LifecycleRulesCount is the number of lifecycle rules
	LifecycleRulesCount int
}

// R2BucketDomainSyncResult contains R2BucketDomain-specific sync result.
type R2BucketDomainSyncResult struct {
	SyncResult
	// DomainID is the domain configuration ID
	DomainID string
	// ZoneID is the Cloudflare zone ID
	ZoneID string
	// Enabled indicates if the domain is enabled
	Enabled bool
	// MinTLS is the configured TLS version
	MinTLS string
	// PublicAccessEnabled indicates public access status
	PublicAccessEnabled bool
	// URL is the full HTTPS URL
	URL string
	// SSLStatus is the SSL certificate status
	SSLStatus string
	// OwnershipStatus is the domain ownership status
	OwnershipStatus string
}

// R2BucketNotificationSyncResult contains R2BucketNotification-specific sync result.
type R2BucketNotificationSyncResult struct {
	SyncResult
	// QueueID is the resolved Queue ID
	QueueID string
	// RuleCount is the number of notification rules
	RuleCount int
}
