// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package r2

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

func TestResourceTypeConstants(t *testing.T) {
	assert.Equal(t, v1alpha2.SyncResourceR2Bucket, ResourceTypeR2Bucket)
	assert.Equal(t, v1alpha2.SyncResourceR2BucketDomain, ResourceTypeR2BucketDomain)
	assert.Equal(t, v1alpha2.SyncResourceR2BucketNotification, ResourceTypeR2BucketNotification)
}

func TestPriorityConstants(t *testing.T) {
	assert.Equal(t, 100, PriorityR2Bucket)
	assert.Equal(t, 100, PriorityR2BucketDomain)
	assert.Equal(t, 100, PriorityR2BucketNotification)
}

func TestR2BucketConfig(t *testing.T) {
	config := R2BucketConfig{
		Name:         "my-bucket",
		LocationHint: "wnam",
	}

	assert.Equal(t, "my-bucket", config.Name)
	assert.Equal(t, "wnam", config.LocationHint)
}

func TestR2BucketConfigWithCORS(t *testing.T) {
	maxAge := 3600
	config := R2BucketConfig{
		Name:         "cors-bucket",
		LocationHint: "enam",
		CORS: []v1alpha2.R2CORSRule{
			{
				AllowedOrigins: []string{"https://example.com"},
				AllowedMethods: []string{"GET", "POST"},
				AllowedHeaders: []string{"Content-Type"},
				MaxAgeSeconds:  &maxAge,
			},
		},
	}

	assert.Len(t, config.CORS, 1)
	assert.Contains(t, config.CORS[0].AllowedOrigins, "https://example.com")
	assert.Contains(t, config.CORS[0].AllowedMethods, "GET")
}

func TestR2LifecycleConfig(t *testing.T) {
	config := R2LifecycleConfig{
		Rules: []v1alpha2.R2LifecycleRule{
			{
				ID:      "delete-old",
				Enabled: true,
			},
		},
		DeletionPolicy: "Delete",
	}

	assert.Len(t, config.Rules, 1)
	assert.Equal(t, "delete-old", config.Rules[0].ID)
	assert.Equal(t, "Delete", config.DeletionPolicy)
}

func TestR2BucketConfigWithLifecycle(t *testing.T) {
	config := R2BucketConfig{
		Name: "lifecycle-bucket",
		Lifecycle: &R2LifecycleConfig{
			Rules: []v1alpha2.R2LifecycleRule{
				{
					ID:      "archive-old",
					Enabled: true,
				},
			},
		},
	}

	assert.NotNil(t, config.Lifecycle)
	assert.Len(t, config.Lifecycle.Rules, 1)
}

func TestR2BucketDomainConfig(t *testing.T) {
	publicAccess := true

	config := R2BucketDomainConfig{
		BucketName:         "my-bucket",
		Domain:             "cdn.example.com",
		ZoneID:             "zone-123",
		MinTLS:             "1.2",
		EnablePublicAccess: &publicAccess,
	}

	assert.Equal(t, "my-bucket", config.BucketName)
	assert.Equal(t, "cdn.example.com", config.Domain)
	assert.Equal(t, "zone-123", config.ZoneID)
	assert.Equal(t, "1.2", config.MinTLS)
	assert.True(t, *config.EnablePublicAccess)
}

func TestR2BucketDomainConfigAutoZone(t *testing.T) {
	config := R2BucketDomainConfig{
		BucketName: "my-bucket",
		Domain:     "assets.example.com",
		ZoneID:     "", // Empty for auto-lookup
	}

	assert.Empty(t, config.ZoneID)
	assert.Equal(t, "assets.example.com", config.Domain)
}

func TestR2BucketNotificationConfig(t *testing.T) {
	config := R2BucketNotificationConfig{
		BucketName: "my-bucket",
		QueueName:  "my-queue",
		Rules: []v1alpha2.R2NotificationRule{
			{
				Prefix: "uploads/",
				Suffix: ".jpg",
			},
		},
	}

	assert.Equal(t, "my-bucket", config.BucketName)
	assert.Equal(t, "my-queue", config.QueueName)
	assert.Len(t, config.Rules, 1)
}

func TestR2BucketRegisterOptions(t *testing.T) {
	opts := R2BucketRegisterOptions{
		AccountID:  "account-123",
		BucketName: "existing-bucket",
		Source: service.Source{
			Kind:      "R2Bucket",
			Namespace: "default",
			Name:      "my-bucket",
		},
		Config: R2BucketConfig{
			Name:         "my-bucket",
			LocationHint: "wnam",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "existing-bucket", opts.BucketName)
	assert.Equal(t, "R2Bucket", opts.Source.Kind)
	assert.Equal(t, "wnam", opts.Config.LocationHint)
}

func TestR2BucketRegisterOptionsNew(t *testing.T) {
	opts := R2BucketRegisterOptions{
		AccountID:  "account-123",
		BucketName: "", // Empty for new bucket
		Source: service.Source{
			Kind: "R2Bucket",
			Name: "new-bucket",
		},
		Config: R2BucketConfig{
			Name: "new-bucket",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	assert.Empty(t, opts.BucketName)
	assert.Equal(t, "new-bucket", opts.Config.Name)
}

func TestR2BucketDomainRegisterOptions(t *testing.T) {
	opts := R2BucketDomainRegisterOptions{
		AccountID: "account-123",
		DomainID:  "domain-456",
		ZoneID:    "zone-789",
		Source: service.Source{
			Kind:      "R2BucketDomain",
			Namespace: "default",
			Name:      "my-domain",
		},
		Config: R2BucketDomainConfig{
			BucketName: "my-bucket",
			Domain:     "cdn.example.com",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "domain-456", opts.DomainID)
	assert.Equal(t, "zone-789", opts.ZoneID)
	assert.Equal(t, "R2BucketDomain", opts.Source.Kind)
}

func TestR2BucketNotificationRegisterOptions(t *testing.T) {
	opts := R2BucketNotificationRegisterOptions{
		AccountID: "account-123",
		QueueID:   "queue-456",
		Source: service.Source{
			Kind:      "R2BucketNotification",
			Namespace: "default",
			Name:      "my-notification",
		},
		Config: R2BucketNotificationConfig{
			BucketName: "my-bucket",
			QueueName:  "my-queue",
		},
		CredentialsRef: v1alpha2.CredentialsReference{Name: "cf-creds"},
	}

	assert.Equal(t, "account-123", opts.AccountID)
	assert.Equal(t, "queue-456", opts.QueueID)
	assert.Equal(t, "R2BucketNotification", opts.Source.Kind)
}

func TestSyncResult(t *testing.T) {
	result := SyncResult{
		ID:        "resource-123",
		AccountID: "account-456",
	}

	assert.Equal(t, "resource-123", result.ID)
	assert.Equal(t, "account-456", result.AccountID)
}

func TestR2BucketSyncResult(t *testing.T) {
	result := R2BucketSyncResult{
		SyncResult: SyncResult{
			ID:        "bucket-123",
			AccountID: "account-456",
		},
		BucketName:          "my-bucket",
		Location:            "wnam",
		CreatedAt:           "2024-01-01T00:00:00Z",
		CORSRulesCount:      2,
		LifecycleRulesCount: 3,
	}

	assert.Equal(t, "bucket-123", result.ID)
	assert.Equal(t, "my-bucket", result.BucketName)
	assert.Equal(t, "wnam", result.Location)
	assert.Equal(t, 2, result.CORSRulesCount)
	assert.Equal(t, 3, result.LifecycleRulesCount)
}

func TestR2BucketDomainSyncResult(t *testing.T) {
	result := R2BucketDomainSyncResult{
		SyncResult: SyncResult{
			ID:        "domain-123",
			AccountID: "account-456",
		},
		DomainID:            "domain-123",
		ZoneID:              "zone-789",
		Enabled:             true,
		MinTLS:              "1.2",
		PublicAccessEnabled: true,
		URL:                 "https://cdn.example.com",
		SSLStatus:           "active",
		OwnershipStatus:     "verified",
	}

	assert.Equal(t, "domain-123", result.DomainID)
	assert.Equal(t, "zone-789", result.ZoneID)
	assert.True(t, result.Enabled)
	assert.Equal(t, "1.2", result.MinTLS)
	assert.True(t, result.PublicAccessEnabled)
	assert.Equal(t, "https://cdn.example.com", result.URL)
	assert.Equal(t, "active", result.SSLStatus)
	assert.Equal(t, "verified", result.OwnershipStatus)
}

func TestR2BucketNotificationSyncResult(t *testing.T) {
	result := R2BucketNotificationSyncResult{
		SyncResult: SyncResult{
			ID:        "notification-123",
			AccountID: "account-456",
		},
		QueueID:   "queue-789",
		RuleCount: 5,
	}

	assert.Equal(t, "notification-123", result.ID)
	assert.Equal(t, "queue-789", result.QueueID)
	assert.Equal(t, 5, result.RuleCount)
}

func TestLocationHints(t *testing.T) {
	locations := []string{"wnam", "enam", "weur", "eeur", "apac"}

	for _, loc := range locations {
		t.Run(loc, func(t *testing.T) {
			config := R2BucketConfig{
				Name:         "bucket-" + loc,
				LocationHint: loc,
			}
			assert.Equal(t, loc, config.LocationHint)
		})
	}
}

func TestMinTLSVersions(t *testing.T) {
	versions := []string{"1.0", "1.1", "1.2", "1.3"}

	for _, version := range versions {
		t.Run(version, func(t *testing.T) {
			config := R2BucketDomainConfig{
				BucketName: "test-bucket",
				Domain:     "test.example.com",
				MinTLS:     version,
			}
			assert.Equal(t, version, config.MinTLS)
		})
	}
}
