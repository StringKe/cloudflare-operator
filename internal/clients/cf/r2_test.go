// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestR2RulesRequest(t *testing.T) {
	t.Run("CORS rules request", func(t *testing.T) {
		rules := []R2CORSRule{
			{
				ID:             "cors-1",
				AllowedOrigins: []string{"https://example.com"},
				AllowedMethods: []string{"GET", "POST"},
				AllowedHeaders: []string{"Content-Type"},
				ExposeHeaders:  []string{"X-Custom-Header"},
				MaxAgeSeconds:  intPtr(3600),
			},
		}

		request := r2RulesRequest[R2CORSRule]{Rules: rules}

		// Verify JSON serialization
		data, err := json.Marshal(request)
		require.NoError(t, err)

		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Contains(t, parsed, "rules")
		rulesArr, ok := parsed["rules"].([]interface{})
		require.True(t, ok, "rules should be []interface{}")
		assert.Len(t, rulesArr, 1)
	})

	t.Run("lifecycle rules request", func(t *testing.T) {
		days := 30
		rules := []R2LifecycleRule{
			{
				ID:      "lifecycle-1",
				Enabled: true,
				Prefix:  "logs/",
				Expiration: &R2LifecycleExpiration{
					Days: &days,
				},
			},
		}

		request := r2RulesRequest[R2LifecycleRule]{Rules: rules}

		data, err := json.Marshal(request)
		require.NoError(t, err)

		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Contains(t, parsed, "rules")
	})

	t.Run("notification rules request", func(t *testing.T) {
		rules := []R2NotificationRule{
			{
				RuleID:      "notif-1",
				Prefix:      "uploads/",
				Suffix:      ".jpg",
				EventTypes:  []string{"object:create"},
				Description: "Image upload notification",
			},
		}

		request := r2RulesRequest[R2NotificationRule]{Rules: rules}

		data, err := json.Marshal(request)
		require.NoError(t, err)

		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Contains(t, parsed, "rules")
	})
}

func TestR2CORSRule(t *testing.T) {
	tests := []struct {
		name string
		rule R2CORSRule
	}{
		{
			name: "minimal CORS rule",
			rule: R2CORSRule{
				AllowedOrigins: []string{"*"},
				AllowedMethods: []string{"GET"},
			},
		},
		{
			name: "full CORS rule",
			rule: R2CORSRule{
				ID:             "cors-full",
				AllowedOrigins: []string{"https://example.com", "https://app.example.com"},
				AllowedMethods: []string{"GET", "POST", "PUT", "DELETE"},
				AllowedHeaders: []string{"Content-Type", "Authorization", "X-Custom-Header"},
				ExposeHeaders:  []string{"X-Response-Header", "ETag"},
				MaxAgeSeconds:  intPtr(7200),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.rule)
			require.NoError(t, err)

			var parsed R2CORSRule
			err = json.Unmarshal(data, &parsed)
			require.NoError(t, err)

			assert.Equal(t, tt.rule.AllowedOrigins, parsed.AllowedOrigins)
			assert.Equal(t, tt.rule.AllowedMethods, parsed.AllowedMethods)
		})
	}
}

func TestR2LifecycleRule(t *testing.T) {
	tests := []struct {
		name string
		rule R2LifecycleRule
	}{
		{
			name: "expiration by days",
			rule: R2LifecycleRule{
				ID:      "expire-30",
				Enabled: true,
				Prefix:  "temp/",
				Expiration: &R2LifecycleExpiration{
					Days: intPtr(30),
				},
			},
		},
		{
			name: "expiration by date",
			rule: R2LifecycleRule{
				ID:      "expire-date",
				Enabled: true,
				Prefix:  "archive/",
				Expiration: &R2LifecycleExpiration{
					Date: "2025-12-31",
				},
			},
		},
		{
			name: "abort incomplete multipart",
			rule: R2LifecycleRule{
				ID:      "abort-multipart",
				Enabled: true,
				AbortIncompleteMultipartUpload: &R2LifecycleAbortUpload{
					DaysAfterInitiation: 7,
				},
			},
		},
		{
			name: "combined rule",
			rule: R2LifecycleRule{
				ID:      "combined",
				Enabled: true,
				Prefix:  "uploads/",
				Expiration: &R2LifecycleExpiration{
					Days: intPtr(90),
				},
				AbortIncompleteMultipartUpload: &R2LifecycleAbortUpload{
					DaysAfterInitiation: 1,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.rule)
			require.NoError(t, err)

			var parsed R2LifecycleRule
			err = json.Unmarshal(data, &parsed)
			require.NoError(t, err)

			assert.Equal(t, tt.rule.ID, parsed.ID)
			assert.Equal(t, tt.rule.Enabled, parsed.Enabled)
		})
	}
}

func TestR2NotificationRule(t *testing.T) {
	tests := []struct {
		name string
		rule R2NotificationRule
	}{
		{
			name: "object create notification",
			rule: R2NotificationRule{
				RuleID:      "create-notif",
				EventTypes:  []string{"object:create"},
				Description: "Notify on object creation",
			},
		},
		{
			name: "filtered notification",
			rule: R2NotificationRule{
				RuleID:      "filtered-notif",
				Prefix:      "images/",
				Suffix:      ".png",
				EventTypes:  []string{"object:create", "object:delete"},
				Description: "Notify on PNG image changes",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.rule)
			require.NoError(t, err)

			var parsed R2NotificationRule
			err = json.Unmarshal(data, &parsed)
			require.NoError(t, err)

			assert.Equal(t, tt.rule.RuleID, parsed.RuleID)
			assert.Equal(t, tt.rule.EventTypes, parsed.EventTypes)
		})
	}
}

func TestR2BucketParams(t *testing.T) {
	params := R2BucketParams{
		Name:         "my-bucket",
		LocationHint: "wnam",
	}

	assert.Equal(t, "my-bucket", params.Name)
	assert.Equal(t, "wnam", params.LocationHint)
}

func TestR2LifecycleExpiration(t *testing.T) {
	t.Run("by days", func(t *testing.T) {
		days := 30
		exp := R2LifecycleExpiration{Days: &days}

		data, err := json.Marshal(exp)
		require.NoError(t, err)

		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, float64(30), parsed["days"])
	})

	t.Run("by date", func(t *testing.T) {
		exp := R2LifecycleExpiration{Date: "2025-12-31"}

		data, err := json.Marshal(exp)
		require.NoError(t, err)

		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, "2025-12-31", parsed["date"])
	})
}

func TestR2LifecycleAbortUpload(t *testing.T) {
	abort := R2LifecycleAbortUpload{
		DaysAfterInitiation: 7,
	}

	data, err := json.Marshal(abort)
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, float64(7), parsed["daysAfterInitiation"])
}

// Helper function
func intPtr(i int) *int {
	return &i
}
