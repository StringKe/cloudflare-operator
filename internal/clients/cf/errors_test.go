// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAPIError(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		resource  string
		err       error
		wantMsg   string
	}{
		{
			name:      "with resource",
			operation: "create",
			resource:  "tunnel/my-tunnel",
			err:       errors.New("connection refused"),
			wantMsg:   "create tunnel/my-tunnel: connection refused",
		},
		{
			name:      "without resource",
			operation: "authenticate",
			resource:  "",
			err:       errors.New("invalid token"),
			wantMsg:   "authenticate: invalid token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiErr := NewAPIError(tt.operation, tt.resource, tt.err)
			assert.Equal(t, tt.wantMsg, apiErr.Error())
			assert.ErrorIs(t, apiErr, tt.err)
		})
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "exact ErrResourceNotFound",
			err:  ErrResourceNotFound,
			want: true,
		},
		{
			name: "wrapped ErrResourceNotFound",
			err:  fmt.Errorf("tunnel: %w", ErrResourceNotFound),
			want: true,
		},
		{
			name: "message contains 'not found'",
			err:  errors.New("tunnel not found"),
			want: true,
		},
		{
			name: "message contains 'does not exist'",
			err:  errors.New("resource does not exist"),
			want: true,
		},
		{
			name: "message contains 'no such'",
			err:  errors.New("no such tunnel"),
			want: true,
		},
		{
			name: "message contains '404'",
			err:  errors.New("HTTP 404 error"),
			want: true,
		},
		{
			name: "case insensitive",
			err:  errors.New("Resource NOT FOUND"),
			want: true,
		},
		// Cloudflare Access API specific errors
		{
			name: "access api unknown_application error (11021)",
			err:  errors.New("error from makeRequest: access.api.error.unknown_application (11021)"),
			want: true,
		},
		{
			name: "access api unknown_group error",
			err:  errors.New("access.api.error.unknown_group"),
			want: true,
		},
		{
			name: "access api unknown_policy error",
			err:  errors.New("access.api.error.unknown_policy"),
			want: true,
		},
		{
			name: "access api unknown_identity_provider error",
			err:  errors.New("access.api.error.unknown_identity_provider"),
			want: true,
		},
		{
			name: "access api unknown_service_token error",
			err:  errors.New("access.api.error.unknown_service_token"),
			want: true,
		},
		// Cloudflare Tunnel API specific errors
		{
			name: "tunnel api route not found",
			err:  errors.New("route not found"),
			want: true,
		},
		{
			name: "tunnel api virtual network not found",
			err:  errors.New("virtual network not found"),
			want: true,
		},
		// General Cloudflare API patterns
		{
			name: "resource_not_found pattern",
			err:  errors.New("resource_not_found: the specified resource does not exist"),
			want: true,
		},
		{
			name: "could not find pattern",
			err:  errors.New("could not find the requested tunnel"),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("connection timeout"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNotFoundError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsConflictError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "exact ErrResourceConflict",
			err:  ErrResourceConflict,
			want: true,
		},
		{
			name: "wrapped ErrResourceConflict",
			err:  fmt.Errorf("tunnel: %w", ErrResourceConflict),
			want: true,
		},
		{
			name: "message contains 'already exists'",
			err:  errors.New("tunnel already exists"),
			want: true,
		},
		{
			name: "message contains 'conflict'",
			err:  errors.New("resource conflict"),
			want: true,
		},
		{
			name: "message contains 'duplicate'",
			err:  errors.New("duplicate entry"),
			want: true,
		},
		{
			name: "case insensitive",
			err:  errors.New("ALREADY EXISTS"),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("connection timeout"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsConflictError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsRateLimitError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "exact ErrAPIRateLimited",
			err:  ErrAPIRateLimited,
			want: true,
		},
		{
			name: "wrapped ErrAPIRateLimited",
			err:  fmt.Errorf("api: %w", ErrAPIRateLimited),
			want: true,
		},
		{
			name: "message contains 'rate limit'",
			err:  errors.New("rate limit exceeded"),
			want: true,
		},
		{
			name: "message contains 'too many requests'",
			err:  errors.New("too many requests"),
			want: true,
		},
		{
			name: "message contains '429'",
			err:  errors.New("HTTP 429 error"),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("connection timeout"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRateLimitError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// nolint:dupl // similar test structure is intentional for comprehensive coverage
func TestIsTemporaryError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "exact ErrTemporaryFailure",
			err:  ErrTemporaryFailure,
			want: true,
		},
		{
			name: "rate limit error (is temporary)",
			err:  ErrAPIRateLimited,
			want: true,
		},
		{
			name: "message contains 'timeout'",
			err:  errors.New("connection timeout"),
			want: true,
		},
		{
			name: "message contains 'connection refused'",
			err:  errors.New("connection refused"),
			want: true,
		},
		{
			name: "message contains 'temporary'",
			err:  errors.New("temporary failure"),
			want: true,
		},
		{
			name: "message contains '503'",
			err:  errors.New("HTTP 503 Service Unavailable"),
			want: true,
		},
		{
			name: "message contains '502'",
			err:  errors.New("HTTP 502 Bad Gateway"),
			want: true,
		},
		{
			name: "message contains '504'",
			err:  errors.New("HTTP 504 Gateway Timeout"),
			want: true,
		},
		{
			name: "permanent error",
			err:  errors.New("invalid configuration"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTemporaryError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// nolint:dupl // similar test structure is intentional for comprehensive coverage
func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "exact ErrAuthenticationFailed",
			err:  ErrAuthenticationFailed,
			want: true,
		},
		{
			name: "exact ErrPermissionDenied",
			err:  ErrPermissionDenied,
			want: true,
		},
		{
			name: "message contains 'unauthorized'",
			err:  errors.New("unauthorized access"),
			want: true,
		},
		{
			name: "message contains 'authentication'",
			err:  errors.New("authentication failed"),
			want: true,
		},
		{
			name: "message contains 'permission denied'",
			err:  errors.New("permission denied"),
			want: true,
		},
		{
			name: "message contains 'forbidden'",
			err:  errors.New("forbidden"),
			want: true,
		},
		{
			name: "message contains '401'",
			err:  errors.New("HTTP 401 error"),
			want: true,
		},
		{
			name: "message contains '403'",
			err:  errors.New("HTTP 403 Forbidden"),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("connection timeout"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsAuthError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWrapNotFound(t *testing.T) {
	t.Run("with underlying error", func(t *testing.T) {
		underlying := errors.New("tunnel abc not found in account")
		wrapped := WrapNotFound("tunnel/abc", underlying)

		assert.ErrorIs(t, wrapped, ErrResourceNotFound)
		assert.Contains(t, wrapped.Error(), "tunnel/abc")
		assert.Contains(t, wrapped.Error(), "tunnel abc not found")
	})

	t.Run("without underlying error", func(t *testing.T) {
		wrapped := WrapNotFound("tunnel/xyz", nil)

		assert.ErrorIs(t, wrapped, ErrResourceNotFound)
		assert.Contains(t, wrapped.Error(), "tunnel/xyz")
	})
}

func TestWrapConflict(t *testing.T) {
	t.Run("with underlying error", func(t *testing.T) {
		underlying := errors.New("tunnel already exists")
		wrapped := WrapConflict("tunnel/abc", underlying)

		assert.ErrorIs(t, wrapped, ErrResourceConflict)
		assert.Contains(t, wrapped.Error(), "tunnel/abc")
		assert.Contains(t, wrapped.Error(), "tunnel already exists")
	})

	t.Run("without underlying error", func(t *testing.T) {
		wrapped := WrapConflict("tunnel/xyz", nil)

		assert.ErrorIs(t, wrapped, ErrResourceConflict)
		assert.Contains(t, wrapped.Error(), "tunnel/xyz")
	})
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	assert.Equal(t, 10*time.Second, cfg.BaseDelay)
	assert.Equal(t, 5*time.Minute, cfg.MaxDelay)
	assert.Equal(t, 10, cfg.MaxRetries)
	assert.Equal(t, 0, cfg.RetryCount)
}

func TestGetRequeueDelay(t *testing.T) {
	cfg := DefaultRetryConfig()

	tests := []struct {
		name       string
		err        error
		retryCount int
		wantMin    time.Duration
		wantMax    time.Duration
	}{
		{
			name:       "nil error returns max delay",
			err:        nil,
			retryCount: 0,
			wantMin:    cfg.MaxDelay,
			wantMax:    cfg.MaxDelay,
		},
		{
			name:       "auth error returns max delay",
			err:        ErrAuthenticationFailed,
			retryCount: 0,
			wantMin:    cfg.MaxDelay,
			wantMax:    cfg.MaxDelay,
		},
		{
			name:       "rate limit error uses exponential backoff",
			err:        ErrAPIRateLimited,
			retryCount: 0,
			wantMin:    cfg.BaseDelay,
			wantMax:    cfg.MaxDelay,
		},
		{
			name:       "not found error returns 0",
			err:        ErrResourceNotFound,
			retryCount: 0,
			wantMin:    0,
			wantMax:    0,
		},
		{
			name:       "other error returns base delay",
			err:        errors.New("unknown error"),
			retryCount: 0,
			wantMin:    cfg.BaseDelay,
			wantMax:    cfg.BaseDelay,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg.RetryCount = tt.retryCount
			delay := GetRequeueDelay(tt.err, cfg)
			assert.GreaterOrEqual(t, delay, tt.wantMin)
			assert.LessOrEqual(t, delay, tt.wantMax)
		})
	}
}

func TestGetRequeueDelayExponentialBackoff(t *testing.T) {
	cfg := DefaultRetryConfig()

	// Test exponential backoff for rate limit errors
	delays := make([]time.Duration, 0)
	for i := 0; i < 10; i++ {
		cfg.RetryCount = i
		delay := GetRequeueDelay(ErrAPIRateLimited, cfg)
		delays = append(delays, delay)
	}

	// Verify delays increase (until capped)
	for i := 1; i < len(delays); i++ {
		if delays[i-1] < cfg.MaxDelay {
			assert.GreaterOrEqual(t, delays[i], delays[i-1],
				"Delay should increase or stay same at retry %d", i)
		}
	}

	// Verify max delay is not exceeded
	for _, delay := range delays {
		assert.LessOrEqual(t, delay, cfg.MaxDelay)
	}
}

func TestShouldRetry(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		retryCount int
		maxRetries int
		want       bool
	}{
		{
			name:       "nil error should not retry",
			err:        nil,
			retryCount: 0,
			maxRetries: 10,
			want:       false,
		},
		{
			name:       "max retries exceeded",
			err:        errors.New("some error"),
			retryCount: 10,
			maxRetries: 10,
			want:       false,
		},
		{
			name:       "auth error should not retry",
			err:        ErrAuthenticationFailed,
			retryCount: 0,
			maxRetries: 10,
			want:       false,
		},
		{
			name:       "permission denied should not retry",
			err:        ErrPermissionDenied,
			retryCount: 0,
			maxRetries: 10,
			want:       false,
		},
		{
			name:       "temporary error should retry",
			err:        ErrTemporaryFailure,
			retryCount: 0,
			maxRetries: 10,
			want:       true,
		},
		{
			name:       "rate limit error should retry",
			err:        ErrAPIRateLimited,
			retryCount: 0,
			maxRetries: 10,
			want:       true,
		},
		{
			name:       "unknown error should retry (default)",
			err:        errors.New("unknown error"),
			retryCount: 0,
			maxRetries: 10,
			want:       true,
		},
		{
			name:       "zero max retries means unlimited",
			err:        errors.New("some error"),
			retryCount: 100,
			maxRetries: 0,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldRetry(tt.err, tt.retryCount, tt.maxRetries)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSanitizeErrorMessage(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		want    string
		wantNot []string
	}{
		{
			name: "nil error",
			err:  nil,
			want: "",
		},
		{
			name: "safe error message",
			err:  errors.New("tunnel not found"),
			want: "tunnel not found",
		},
		{
			name:    "message with token",
			err:     errors.New("invalid token: abc123xyz"),
			want:    "operation failed - check operator logs for details",
			wantNot: []string{"abc123xyz", "token"},
		},
		{
			name:    "message with secret",
			err:     errors.New("secret validation failed: my-secret-value"),
			want:    "operation failed - check operator logs for details",
			wantNot: []string{"my-secret-value", "secret"},
		},
		{
			name:    "message with password",
			err:     errors.New("password mismatch: hunter2"),
			want:    "operation failed - check operator logs for details",
			wantNot: []string{"hunter2", "password"},
		},
		{
			name:    "message with credential",
			err:     errors.New("credential expired: cred-123"),
			want:    "operation failed - check operator logs for details",
			wantNot: []string{"cred-123", "credential"},
		},
		{
			name:    "message with api_key",
			err:     errors.New("invalid api_key provided"),
			want:    "operation failed - check operator logs for details",
			wantNot: []string{"api_key"},
		},
		{
			name:    "message with bearer",
			err:     errors.New("Bearer token invalid"),
			want:    "operation failed - check operator logs for details",
			wantNot: []string{"bearer"},
		},
		{
			name:    "message with authorization",
			err:     errors.New("Authorization header missing"),
			want:    "operation failed - check operator logs for details",
			wantNot: []string{"authorization"},
		},
		{
			name: "rate limit error returns specific message",
			err:  errors.New("rate limit exceeded (contains token in impl)"),
			want: "API rate limit exceeded",
		},
		{
			name: "not found error returns specific message",
			err:  errors.New("resource not found (contains secret in impl)"),
			want: "resource not found",
		},
		{
			name: "long message is truncated",
			err:  errors.New(strings.Repeat("a", 1000)),
			want: strings.Repeat("a", 509) + "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeErrorMessage(tt.err)
			assert.Equal(t, tt.want, got)

			for _, notWant := range tt.wantNot {
				assert.NotContains(t, strings.ToLower(got), strings.ToLower(notWant),
					"Sanitized message should not contain %q", notWant)
			}
		})
	}
}

func TestSanitizeErrorMessageMaxLength(t *testing.T) {
	// Create an error message longer than 512 characters
	longMsg := strings.Repeat("x", 600)
	err := errors.New(longMsg)

	sanitized := SanitizeErrorMessage(err)

	// Should be truncated to 512 characters (509 + "...")
	assert.LessOrEqual(t, len(sanitized), 512)
	assert.True(t, strings.HasSuffix(sanitized, "..."))
}

func TestContainsSensitivePattern(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want bool
	}{
		{"empty string", "", false},
		{"normal message", "tunnel created successfully", false},
		{"contains token", "invalid token provided", true},
		{"contains TOKEN uppercase", "invalid TOKEN", true},
		{"contains secret", "secret not found", true},
		{"contains password", "password incorrect", true},
		{"contains credential", "credential expired", true},
		{"contains api_key", "api_key invalid", true},
		{"contains apikey", "apikey required", true},
		{"contains bearer", "bearer token", true},
		{"contains authorization", "authorization failed", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsSensitivePattern(tt.msg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetGenericErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "auth error",
			err:  ErrAuthenticationFailed,
			want: "authentication failed - check credentials",
		},
		{
			name: "rate limit error",
			err:  ErrAPIRateLimited,
			want: "API rate limit exceeded",
		},
		{
			name: "not found error",
			err:  ErrResourceNotFound,
			want: "resource not found",
		},
		{
			name: "other error",
			err:  errors.New("unknown error"),
			want: "operation failed - check operator logs for details",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getGenericErrorMessage(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCalculateExponentialDelay(t *testing.T) {
	baseDelay := 10 * time.Second
	maxDelay := 5 * time.Minute
	maxShift := 6

	tests := []struct {
		retryCount int
		wantDelay  time.Duration
	}{
		{0, 10 * time.Second},  // 10 * 2^0 = 10s
		{1, 20 * time.Second},  // 10 * 2^1 = 20s
		{2, 40 * time.Second},  // 10 * 2^2 = 40s
		{3, 80 * time.Second},  // 10 * 2^3 = 80s
		{4, 160 * time.Second}, // 10 * 2^4 = 160s
		{5, 5 * time.Minute},   // 10 * 2^5 = 320s, capped at 5min
		{6, 5 * time.Minute},   // capped
		{10, 5 * time.Minute},  // capped
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("retry_%d", tt.retryCount), func(t *testing.T) {
			delay := calculateExponentialDelay(baseDelay, maxDelay, tt.retryCount, maxShift)
			assert.Equal(t, tt.wantDelay, delay)
		})
	}
}

func TestErrorVariables(t *testing.T) {
	// Ensure error variables are properly defined
	assert.NotNil(t, ErrResourceNotFound)
	assert.NotNil(t, ErrResourceConflict)
	assert.NotNil(t, ErrMultipleResourcesFound)
	assert.NotNil(t, ErrAPIRateLimited)
	assert.NotNil(t, ErrTemporaryFailure)
	assert.NotNil(t, ErrInvalidConfiguration)
	assert.NotNil(t, ErrAuthenticationFailed)
	assert.NotNil(t, ErrPermissionDenied)

	// Ensure they are distinct
	assert.NotErrorIs(t, ErrResourceNotFound, ErrResourceConflict)
	assert.NotErrorIs(t, ErrAuthenticationFailed, ErrPermissionDenied)
}
