// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Error types for Cloudflare API operations
var (
	// ErrResourceNotFound indicates the requested resource was not found
	ErrResourceNotFound = errors.New("resource not found")

	// ErrResourceConflict indicates the resource is already managed by another K8s object
	ErrResourceConflict = errors.New("resource already managed by another object")

	// ErrMultipleResourcesFound indicates multiple resources matched when only one was expected
	ErrMultipleResourcesFound = errors.New("multiple resources found")

	// ErrAPIRateLimited indicates the API rate limit was exceeded
	ErrAPIRateLimited = errors.New("API rate limit exceeded")

	// ErrTemporaryFailure indicates a temporary failure that should be retried
	ErrTemporaryFailure = errors.New("temporary failure")

	// ErrInvalidConfiguration indicates invalid configuration
	ErrInvalidConfiguration = errors.New("invalid configuration")

	// ErrAuthenticationFailed indicates authentication failed
	ErrAuthenticationFailed = errors.New("authentication failed")

	// ErrPermissionDenied indicates permission was denied
	ErrPermissionDenied = errors.New("permission denied")

	// ErrInvalidTunnelID indicates tunnel ID is missing or invalid
	ErrInvalidTunnelID = errors.New("invalid or missing tunnel ID")

	// ErrInvalidZoneID indicates zone ID is missing or invalid
	ErrInvalidZoneID = errors.New("invalid or missing zone ID")
)

// APIError wraps a Cloudflare API error with additional context
type APIError struct {
	Operation string
	Resource  string
	Err       error
}

func (e *APIError) Error() string {
	if e.Resource != "" {
		return fmt.Sprintf("%s %s: %v", e.Operation, e.Resource, e.Err)
	}
	return fmt.Sprintf("%s: %v", e.Operation, e.Err)
}

func (e *APIError) Unwrap() error {
	return e.Err
}

// NewAPIError creates a new APIError
func NewAPIError(operation, resource string, err error) *APIError {
	return &APIError{
		Operation: operation,
		Resource:  resource,
		Err:       err,
	}
}

// IsNotFoundError checks if the error indicates a resource was not found
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrResourceNotFound) {
		return true
	}
	// Check for common "not found" patterns in error messages
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "does not exist") ||
		strings.Contains(errStr, "no such") ||
		strings.Contains(errStr, "404") ||
		// Cloudflare Access API specific "not found" errors
		strings.Contains(errStr, "unknown_application") || // 11021
		strings.Contains(errStr, "unknown_group") || // Access group not found
		strings.Contains(errStr, "unknown_policy") || // Access policy not found
		strings.Contains(errStr, "unknown_identity_provider") || // IdP not found
		strings.Contains(errStr, "unknown_service_token") || // Service token not found
		// Cloudflare Tunnel API specific errors
		strings.Contains(errStr, "tunnel not found") ||
		strings.Contains(errStr, "route not found") ||
		strings.Contains(errStr, "virtual network not found") ||
		// General Cloudflare API patterns
		strings.Contains(errStr, "resource_not_found") ||
		strings.Contains(errStr, "could not find")
}

// IsConflictError checks if the error indicates a resource conflict
func IsConflictError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrResourceConflict) {
		return true
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "already exists") ||
		strings.Contains(errStr, "conflict") ||
		strings.Contains(errStr, "duplicate")
}

// IsRateLimitError checks if the error indicates rate limiting
func IsRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrAPIRateLimited) {
		return true
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "too many requests") ||
		strings.Contains(errStr, "429")
}

// IsTemporaryError checks if the error is temporary and should be retried
func IsTemporaryError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrTemporaryFailure) {
		return true
	}
	if IsRateLimitError(err) {
		return true
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "temporary") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "504")
}

// IsAuthError checks if the error indicates an authentication/authorization failure
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrAuthenticationFailed) || errors.Is(err, ErrPermissionDenied) {
		return true
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "unauthorized") ||
		strings.Contains(errStr, "authentication") ||
		strings.Contains(errStr, "permission denied") ||
		strings.Contains(errStr, "forbidden") ||
		strings.Contains(errStr, "401") ||
		strings.Contains(errStr, "403")
}

// WrapNotFound wraps an error as a not found error
func WrapNotFound(resource string, err error) error {
	if err == nil {
		return fmt.Errorf("%s: %w", resource, ErrResourceNotFound)
	}
	return fmt.Errorf("%s: %w: %v", resource, ErrResourceNotFound, err)
}

// WrapConflict wraps an error as a conflict error
func WrapConflict(resource string, err error) error {
	if err == nil {
		return fmt.Errorf("%s: %w", resource, ErrResourceConflict)
	}
	return fmt.Errorf("%s: %w: %v", resource, ErrResourceConflict, err)
}

// RetryConfig holds configuration for retry behavior
type RetryConfig struct {
	// BaseDelay is the initial delay before retry
	BaseDelay time.Duration
	// MaxDelay is the maximum delay between retries
	MaxDelay time.Duration
	// MaxRetries is the maximum number of retries (0 = no limit)
	MaxRetries int
	// RetryCount tracks the current retry count (for exponential backoff)
	RetryCount int
}

// DefaultRetryConfig returns a default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		BaseDelay:  10 * time.Second,
		MaxDelay:   5 * time.Minute,
		MaxRetries: 10,
	}
}

// calculateExponentialDelay computes exponential backoff delay capped at maxDelay
func calculateExponentialDelay(baseDelay, maxDelay time.Duration, retryCount, maxShift int) time.Duration {
	delay := baseDelay * time.Duration(1<<min(retryCount, maxShift))
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}

// GetRequeueDelay calculates the appropriate requeue delay based on error type
// Uses exponential backoff for temporary errors
func GetRequeueDelay(err error, cfg RetryConfig) time.Duration {
	switch {
	case err == nil, IsAuthError(err):
		return cfg.MaxDelay
	case IsRateLimitError(err):
		return calculateExponentialDelay(cfg.BaseDelay, cfg.MaxDelay, cfg.RetryCount, 6)
	case IsTemporaryError(err):
		return calculateExponentialDelay(cfg.BaseDelay, cfg.MaxDelay, cfg.RetryCount, 4)
	case IsNotFoundError(err):
		return 0
	default:
		return cfg.BaseDelay
	}
}

// ShouldRetry determines if an operation should be retried based on error type and retry count
func ShouldRetry(err error, retryCount int, maxRetries int) bool {
	if err == nil {
		return false
	}
	if maxRetries > 0 && retryCount >= maxRetries {
		return false
	}
	// Auth errors should not be retried
	if IsAuthError(err) {
		return false
	}
	// Temporary errors and rate limits should be retried
	if IsTemporaryError(err) || IsRateLimitError(err) {
		return true
	}
	return true // Default to retry
}

// containsSensitivePattern checks if the message contains any sensitive patterns
func containsSensitivePattern(msg string) bool {
	sensitivePatterns := []string{
		"token", "secret", "password", "credential", "api_key", "apikey",
		"bearer", "authorization",
	}
	lowerMsg := strings.ToLower(msg)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(lowerMsg, pattern) {
			return true
		}
	}
	return false
}

// getGenericErrorMessage returns a generic error message based on error type
func getGenericErrorMessage(err error) string {
	switch {
	case IsAuthError(err):
		return "authentication failed - check credentials"
	case IsRateLimitError(err):
		return "API rate limit exceeded"
	case IsNotFoundError(err):
		return "resource not found"
	default:
		return "operation failed - check operator logs for details"
	}
}

// IsDomainNotInDestinationsError checks if the error indicates the domain is not
// included in tunnel destinations. This error (code 12130) occurs when trying to
// create an AccessApplication for a domain that hasn't been synced to the tunnel yet.
// This is typically a temporary condition that resolves when the Ingress controller
// syncs the tunnel configuration.
func IsDomainNotInDestinationsError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "domain not included in destinations") ||
		strings.Contains(errStr, "12130") ||
		strings.Contains(errStr, "not included in destinations")
}

// IsUnknownApplicationError checks if the error indicates the application ID
// stored in status no longer exists in Cloudflare. This can happen if the
// application was deleted manually from Cloudflare dashboard.
func IsUnknownApplicationError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "unknown_application") ||
		strings.Contains(errStr, "11021")
}

// IsAccessApplicationRecoverableError checks if an Access Application error
// is recoverable through retry. Domain not in destinations errors are recoverable
// because the Ingress controller may not have synced the tunnel configuration yet.
func IsAccessApplicationRecoverableError(err error) bool {
	return IsDomainNotInDestinationsError(err) || IsTemporaryError(err) || IsRateLimitError(err)
}

// SanitizeErrorMessage removes potentially sensitive information from error messages
// before storing them in Status conditions
func SanitizeErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()

	// Truncate long error messages
	const maxLen = 512
	if len(msg) > maxLen {
		msg = msg[:maxLen-3] + "..."
	}

	// Check for sensitive patterns and return generic message if found
	if containsSensitivePattern(msg) {
		return getGenericErrorMessage(err)
	}

	return msg
}
