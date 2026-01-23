// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package common provides base infrastructure for Sync Controllers.
// Sync Controllers are responsible for:
// - Watching CloudflareSyncState resources
// - Aggregating configuration from multiple sources
// - Debouncing rapid changes
// - Syncing to Cloudflare API with incremental detection
// - Updating SyncState status
package common

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

const (
	// MaxConflictRetries is the maximum number of retries for conflict errors
	MaxConflictRetries = 5
	// ConflictRetryDelay is the delay between retries
	ConflictRetryDelay = 100 * time.Millisecond
)

// SyncResult contains the result of a successful sync operation
type SyncResult struct {
	// ConfigVersion is the version returned by Cloudflare after update
	ConfigVersion int
	// ConfigHash is the hash of the synced configuration
	ConfigHash string
}

// SyncError wraps an error with additional sync context
type SyncError struct {
	Op      string // Operation that failed (e.g., "aggregate", "sync", "updateStatus")
	Err     error  // Underlying error
	Retries int    // Number of retries attempted
}

func (e *SyncError) Error() string {
	return fmt.Sprintf("%s: %v (retries: %d)", e.Op, e.Err, e.Retries)
}

func (e *SyncError) Unwrap() error {
	return e.Err
}

// BaseSyncController provides common functionality for all Sync Controllers.
// Each resource type (Tunnel, DNS, Access, etc.) extends this with specific
// aggregation and sync logic.
type BaseSyncController struct {
	Client    client.Client
	Debouncer *Debouncer
}

// NewBaseSyncController creates a new BaseSyncController
func NewBaseSyncController(c client.Client) *BaseSyncController {
	return &BaseSyncController{
		Client:    c,
		Debouncer: NewDebouncer(DefaultDebounceDelay),
	}
}

// NewBaseSyncControllerWithDelay creates a BaseSyncController with custom debounce delay
func NewBaseSyncControllerWithDelay(c client.Client, delay time.Duration) *BaseSyncController {
	return &BaseSyncController{
		Client:    c,
		Debouncer: NewDebouncer(delay),
	}
}

// GetSyncState retrieves a CloudflareSyncState by name
func (c *BaseSyncController) GetSyncState(ctx context.Context, name string) (*v1alpha2.CloudflareSyncState, error) {
	syncState := &v1alpha2.CloudflareSyncState{}
	if err := c.Client.Get(ctx, client.ObjectKey{Name: name}, syncState); err != nil {
		return nil, err
	}
	return syncState, nil
}

// ShouldSync determines if a sync is needed by comparing config hashes.
// Returns true if the configuration has changed since the last sync.
func (*BaseSyncController) ShouldSync(syncState *v1alpha2.CloudflareSyncState, newHash string) bool {
	return HashChanged(syncState.Status.ConfigHash, newHash)
}

// SetSyncStatus updates the SyncState status to the specified state.
// This is a convenience method for setting just the status without result.
func (c *BaseSyncController) SetSyncStatus(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	status v1alpha2.SyncStatus,
) error {
	return c.UpdateSyncStatus(ctx, syncState, status, nil, nil)
}

// UpdateSyncStatus updates the SyncState status with the sync result.
// It handles both success and error cases, setting appropriate conditions.
// Uses conflict retry to handle concurrent updates safely.
func (c *BaseSyncController) UpdateSyncStatus(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	status v1alpha2.SyncStatus,
	result *SyncResult,
	syncErr error,
) error {
	logger := log.FromContext(ctx)

	// Use conflict retry for status updates
	err := UpdateStatusWithConflictRetry(ctx, c.Client, syncState, func() {
		// Update sync status
		syncState.Status.SyncStatus = status

		// Update result fields if provided
		if result != nil {
			syncState.Status.ConfigVersion = result.ConfigVersion
			syncState.Status.ConfigHash = result.ConfigHash
			now := metav1.Now()
			syncState.Status.LastSyncTime = &now
		}

		// Update conditions based on status
		if syncErr != nil {
			syncState.Status.Error = syncErr.Error()
			meta.SetStatusCondition(&syncState.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "SyncFailed",
				Message:            syncErr.Error(),
				ObservedGeneration: syncState.Generation,
				LastTransitionTime: metav1.Now(),
			})
		} else {
			c.setStatusConditionByStatus(syncState, status)
		}
	})

	if err != nil {
		logger.Error(err, "Failed to update SyncState status",
			"name", syncState.Name,
			"status", status)
		return fmt.Errorf("update status: %w", err)
	}

	return nil
}

// setStatusConditionByStatus sets the Ready condition based on the sync status
func (*BaseSyncController) setStatusConditionByStatus(syncState *v1alpha2.CloudflareSyncState, status v1alpha2.SyncStatus) {
	switch status {
	case v1alpha2.SyncStatusSynced:
		syncState.Status.Error = ""
		syncState.Status.RetryCount = 0
		syncState.Status.FailureReason = ""
		syncState.Status.ErrorCategory = ""
		syncState.Status.FailedAt = nil
		meta.SetStatusCondition(&syncState.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "Synced",
			Message:            "Configuration synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
	case v1alpha2.SyncStatusSyncing:
		meta.SetStatusCondition(&syncState.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "Syncing",
			Message:            "Sync in progress",
			LastTransitionTime: metav1.Now(),
		})
	case v1alpha2.SyncStatusPending:
		meta.SetStatusCondition(&syncState.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "Pending",
			Message:            "Waiting for sync",
			LastTransitionTime: metav1.Now(),
		})
	case v1alpha2.SyncStatusFailed:
		// Failed status is handled by transitionToFailed, but include here for completeness
		meta.SetStatusCondition(&syncState.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "Failed",
			Message:            "Sync failed permanently",
			LastTransitionTime: metav1.Now(),
		})
	}
}

// StoreAggregatedConfig stores the aggregated configuration in the SyncState status.
// This is useful for debugging and observability.
func (*BaseSyncController) StoreAggregatedConfig(
	syncState *v1alpha2.CloudflareSyncState,
	config interface{},
) error {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal aggregated config: %w", err)
	}

	syncState.Status.AggregatedConfig = &runtime.RawExtension{Raw: configJSON}
	return nil
}

// ParseSourceConfig parses a source's configuration into the target struct.
// This is a helper for Sync Controllers to extract typed configuration.
func ParseSourceConfig[T any](source *v1alpha2.ConfigSource) (*T, error) {
	if source == nil || source.Config.Raw == nil {
		return nil, nil
	}

	var config T
	if err := json.Unmarshal(source.Config.Raw, &config); err != nil {
		return nil, fmt.Errorf("unmarshal source config from %s: %w", source.Ref.String(), err)
	}

	return &config, nil
}

// FilterSourcesByKind returns sources matching the specified kinds.
func FilterSourcesByKind(sources []v1alpha2.ConfigSource, kinds ...string) []v1alpha2.ConfigSource {
	kindSet := make(map[string]bool)
	for _, k := range kinds {
		kindSet[k] = true
	}

	var filtered []v1alpha2.ConfigSource
	for _, source := range sources {
		if kindSet[source.Ref.Kind] {
			filtered = append(filtered, source)
		}
	}
	return filtered
}

// RequeueAfterError returns a requeue duration appropriate for the error type.
// Uses error-type-specific delays:
//   - Rate limit errors: 60 seconds (base for exponential backoff)
//   - Temporary errors (timeout, 502, 503, 504): 10 seconds (base for exponential backoff)
//   - Authentication errors: 0 (permanent error, no retry)
//   - Not found errors: 0 (permanent error, no retry)
//   - Validation errors: 0 (permanent error, no retry)
//   - Other errors: 30 seconds (default for unknown errors)
func RequeueAfterError(err error) time.Duration {
	if err == nil {
		return 0
	}

	switch {
	case cf.IsRateLimitError(err):
		// Rate limit: longer delay to allow quota recovery
		return 60 * time.Second
	case cf.IsTemporaryError(err):
		// Temporary errors: shorter delay for quick recovery
		return 10 * time.Second
	case cf.IsAuthError(err), cf.IsNotFoundError(err), cf.IsValidationError(err):
		// Permanent errors: no retry needed, requires user intervention
		return 0
	default:
		// Default delay for unknown errors
		return 30 * time.Second
	}
}

// RequeueAfterSuccess returns a requeue duration for periodic refresh.
// Most syncs don't need periodic refresh (they're event-driven), so this returns 0.
func RequeueAfterSuccess() time.Duration {
	return 0
}

// UpdateWithConflictRetry updates a SyncState with retry on conflict.
// This should be used for all SyncState modifications including Finalizer operations.
// After successful update, it re-fetches the object to ensure the caller has the latest resourceVersion.
//
//nolint:revive // cognitive complexity is acceptable for retry logic
func UpdateWithConflictRetry(ctx context.Context, c client.Client, syncState *v1alpha2.CloudflareSyncState, updateFn func()) error {
	var lastErr error
	for i := 0; i < MaxConflictRetries; i++ {
		if i > 0 {
			// Re-fetch the object to get the latest ResourceVersion
			if err := c.Get(ctx, client.ObjectKeyFromObject(syncState), syncState); err != nil {
				return fmt.Errorf("failed to get latest SyncState version: %w", err)
			}
			time.Sleep(ConflictRetryDelay)
		}
		updateFn()
		err := c.Update(ctx, syncState)
		if err == nil {
			// CRITICAL: Re-fetch after successful update to get the latest resourceVersion
			// This ensures subsequent operations in the same reconcile loop won't fail
			// with "the object has been modified" error
			if fetchErr := c.Get(ctx, client.ObjectKeyFromObject(syncState), syncState); fetchErr != nil {
				// Log but don't fail - the update was successful
				log.FromContext(ctx).V(1).Info("Warning: failed to re-fetch after update",
					"error", fetchErr)
			}
			return nil
		}
		if !apierrors.IsConflict(err) {
			return err
		}
		lastErr = err
	}
	return fmt.Errorf("update SyncState failed after %d retries: %w", MaxConflictRetries, lastErr)
}

// AddFinalizerWithRetry adds a finalizer to a SyncState with conflict retry.
func AddFinalizerWithRetry(ctx context.Context, c client.Client, syncState *v1alpha2.CloudflareSyncState, finalizerName string) error {
	return UpdateWithConflictRetry(ctx, c, syncState, func() {
		controllerutil.AddFinalizer(syncState, finalizerName)
	})
}

// RemoveFinalizerWithRetry removes a finalizer from a SyncState with conflict retry.
func RemoveFinalizerWithRetry(ctx context.Context, c client.Client, syncState *v1alpha2.CloudflareSyncState, finalizerName string) error {
	return UpdateWithConflictRetry(ctx, c, syncState, func() {
		controllerutil.RemoveFinalizer(syncState, finalizerName)
	})
}

// UpdateStatusWithConflictRetry updates a SyncState status with retry on conflict.
// After successful update, it re-fetches the object to ensure the caller has the latest resourceVersion.
// This prevents subsequent updates from failing due to stale resourceVersion.
//
//nolint:revive // cognitive complexity is acceptable for retry logic
func UpdateStatusWithConflictRetry(ctx context.Context, c client.Client, syncState *v1alpha2.CloudflareSyncState, updateFn func()) error {
	var lastErr error
	for i := 0; i < MaxConflictRetries; i++ {
		if i > 0 {
			// Re-fetch the object to get the latest ResourceVersion
			if err := c.Get(ctx, client.ObjectKeyFromObject(syncState), syncState); err != nil {
				return fmt.Errorf("failed to get latest SyncState version: %w", err)
			}
			time.Sleep(ConflictRetryDelay)
		}
		updateFn()
		err := c.Status().Update(ctx, syncState)
		if err == nil {
			// CRITICAL: Re-fetch after successful update to get the latest resourceVersion
			// This ensures subsequent operations in the same reconcile loop won't fail
			// with "the object has been modified" error
			if fetchErr := c.Get(ctx, client.ObjectKeyFromObject(syncState), syncState); fetchErr != nil {
				// Log but don't fail - the update was successful
				log.FromContext(ctx).V(1).Info("Warning: failed to re-fetch after status update",
					"error", fetchErr)
			}
			return nil
		}
		if !apierrors.IsConflict(err) {
			return err
		}
		lastErr = err
	}
	return fmt.Errorf("update SyncState status failed after %d retries: %w", MaxConflictRetries, lastErr)
}

// ============================================================================
// Unified Error Handling for SyncState
// ============================================================================

const (
	// DefaultMaxRetries is the default maximum number of retry attempts
	DefaultMaxRetries = 5
	// BaseRetryDelay is the base delay for exponential backoff
	BaseRetryDelay = 10 * time.Second
	// MaxRetryDelay is the maximum delay for exponential backoff
	MaxRetryDelay = 5 * time.Minute
)

// SyncErrorResult contains the result of error handling
type SyncErrorResult struct {
	// ShouldRequeue indicates whether the reconcile should requeue
	ShouldRequeue bool
	// RequeueAfter is the duration to wait before requeuing (0 = no requeue)
	RequeueAfter time.Duration
	// IsFailed indicates whether the SyncState entered Failed state
	IsFailed bool
}

// HandleSyncError handles an error from sync operations using the unified error handling approach.
// It classifies the error, updates retry count, and transitions to Failed state when appropriate.
//
// Error handling strategy:
//   - Permanent errors (NotFound, Auth, Validation): Immediate transition to Failed
//   - Transient errors: Increment retry count, use exponential backoff
//   - Max retries exceeded: Transition to Failed
//   - Unknown errors: Treat as transient with limited retries
//
// Returns SyncErrorResult indicating how the reconciler should proceed.
//
//nolint:revive // cognitive complexity is acceptable for unified error handling
func (c *BaseSyncController) HandleSyncError(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	syncErr error,
) (*SyncErrorResult, error) {
	logger := log.FromContext(ctx)

	if syncErr == nil {
		return &SyncErrorResult{ShouldRequeue: false}, nil
	}

	// Classify the error
	category := cf.ClassifyError(syncErr)
	failureReason := cf.GetFailureReason(syncErr)

	logger.V(1).Info("Handling sync error",
		"error", syncErr.Error(),
		"category", category,
		"failureReason", failureReason,
		"currentRetryCount", syncState.Status.RetryCount)

	// Determine max retries (use default if not set)
	maxRetries := syncState.Status.MaxRetries
	if maxRetries == 0 {
		maxRetries = DefaultMaxRetries
	}

	switch category {
	case cf.ErrorCategoryPermanent:
		// Permanent error: immediate transition to Failed
		return c.transitionToFailed(ctx, syncState, syncErr, string(failureReason), string(category))

	case cf.ErrorCategoryTransient:
		// Transient error: increment retry count, check max retries
		newRetryCount := syncState.Status.RetryCount + 1

		if newRetryCount >= maxRetries {
			// Max retries exceeded: transition to Failed
			logger.Info("Max retries exceeded, transitioning to Failed",
				"retryCount", newRetryCount,
				"maxRetries", maxRetries)
			return c.transitionToFailed(ctx, syncState, syncErr, string(cf.FailureReasonMaxRetriesExceeded), string(category))
		}

		// Update status with incremented retry count
		requeueDelay := calculateExponentialBackoff(newRetryCount)
		if err := c.updateErrorStatus(ctx, syncState, syncErr, newRetryCount, string(category)); err != nil {
			return nil, err
		}

		logger.Info("Transient error, will retry",
			"retryCount", newRetryCount,
			"maxRetries", maxRetries,
			"requeueAfter", requeueDelay)

		return &SyncErrorResult{
			ShouldRequeue: true,
			RequeueAfter:  requeueDelay,
			IsFailed:      false,
		}, nil

	default: // ErrorCategoryUnknown
		// Unknown error: treat as transient with limited retries
		newRetryCount := syncState.Status.RetryCount + 1

		if newRetryCount >= maxRetries {
			logger.Info("Max retries exceeded for unknown error, transitioning to Failed",
				"retryCount", newRetryCount,
				"maxRetries", maxRetries)
			return c.transitionToFailed(ctx, syncState, syncErr, string(cf.FailureReasonMaxRetriesExceeded), string(category))
		}

		requeueDelay := calculateExponentialBackoff(newRetryCount)
		if err := c.updateErrorStatus(ctx, syncState, syncErr, newRetryCount, string(category)); err != nil {
			return nil, err
		}

		logger.Info("Unknown error, will retry",
			"retryCount", newRetryCount,
			"maxRetries", maxRetries,
			"requeueAfter", requeueDelay)

		return &SyncErrorResult{
			ShouldRequeue: true,
			RequeueAfter:  requeueDelay,
			IsFailed:      false,
		}, nil
	}
}

// transitionToFailed transitions the SyncState to Failed status
func (c *BaseSyncController) transitionToFailed(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	syncErr error,
	failureReason string,
	errorCategory string,
) (*SyncErrorResult, error) {
	logger := log.FromContext(ctx)

	now := metav1.Now()
	err := UpdateStatusWithConflictRetry(ctx, c.Client, syncState, func() {
		syncState.Status.SyncStatus = v1alpha2.SyncStatusFailed
		syncState.Status.Error = cf.SanitizeErrorMessage(syncErr)
		syncState.Status.FailedAt = &now
		syncState.Status.FailureReason = failureReason
		syncState.Status.ErrorCategory = errorCategory
		syncState.Status.ObservedGeneration = syncState.Generation

		meta.SetStatusCondition(&syncState.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "Failed",
			Message:            fmt.Sprintf("Sync failed permanently: %s", failureReason),
			ObservedGeneration: syncState.Generation,
			LastTransitionTime: now,
		})
	})

	if err != nil {
		logger.Error(err, "Failed to update SyncState to Failed status")
		return nil, err
	}

	logger.Info("Transitioned to Failed state",
		"failureReason", failureReason,
		"error", syncErr.Error())

	return &SyncErrorResult{
		ShouldRequeue: false,
		RequeueAfter:  0,
		IsFailed:      true,
	}, nil
}

// updateErrorStatus updates the SyncState status for a transient error
func (c *BaseSyncController) updateErrorStatus(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	syncErr error,
	retryCount int,
	errorCategory string,
) error {
	return UpdateStatusWithConflictRetry(ctx, c.Client, syncState, func() {
		syncState.Status.SyncStatus = v1alpha2.SyncStatusError
		syncState.Status.Error = cf.SanitizeErrorMessage(syncErr)
		syncState.Status.RetryCount = retryCount
		syncState.Status.ErrorCategory = errorCategory
		syncState.Status.ObservedGeneration = syncState.Generation

		meta.SetStatusCondition(&syncState.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "SyncError",
			Message:            fmt.Sprintf("Sync failed (retry %d): %s", retryCount, cf.SanitizeErrorMessage(syncErr)),
			ObservedGeneration: syncState.Generation,
			LastTransitionTime: metav1.Now(),
		})
	})
}

// ShouldResetFromFailed checks if the SyncState should be reset from Failed state.
// This happens when the Spec generation changes, indicating the user has made changes.
func ShouldResetFromFailed(syncState *v1alpha2.CloudflareSyncState) bool {
	if syncState.Status.SyncStatus != v1alpha2.SyncStatusFailed {
		return false
	}
	// Check if Spec generation has changed since we entered Failed state
	return syncState.Generation > syncState.Status.ObservedGeneration
}

// ResetFromFailed resets a SyncState from Failed status back to Pending.
// This should be called when Spec changes are detected on a Failed SyncState.
func (c *BaseSyncController) ResetFromFailed(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) error {
	logger := log.FromContext(ctx)

	if syncState.Status.SyncStatus != v1alpha2.SyncStatusFailed {
		return nil
	}

	logger.Info("Resetting from Failed state due to Spec change",
		"previousFailureReason", syncState.Status.FailureReason,
		"previousGeneration", syncState.Status.ObservedGeneration,
		"currentGeneration", syncState.Generation)

	return UpdateStatusWithConflictRetry(ctx, c.Client, syncState, func() {
		syncState.Status.SyncStatus = v1alpha2.SyncStatusPending
		syncState.Status.Error = ""
		syncState.Status.RetryCount = 0
		syncState.Status.FailedAt = nil
		syncState.Status.FailureReason = ""
		syncState.Status.ErrorCategory = ""
		syncState.Status.ObservedGeneration = syncState.Generation

		meta.SetStatusCondition(&syncState.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "Pending",
			Message:            "Reset from Failed state due to Spec change",
			ObservedGeneration: syncState.Generation,
			LastTransitionTime: metav1.Now(),
		})
	})
}

// IsFailed checks if a SyncState is in Failed status
func IsFailed(syncState *v1alpha2.CloudflareSyncState) bool {
	return syncState.Status.SyncStatus == v1alpha2.SyncStatusFailed
}

// calculateExponentialBackoff computes exponential backoff delay
// Formula: min(maxDelay, baseDelay * 2^retryCount)
// Sequence: 10s, 20s, 40s, 80s, 160s (capped at 5min)
func calculateExponentialBackoff(retryCount int) time.Duration {
	if retryCount < 0 {
		retryCount = 0
	}

	// Cap the shift to prevent overflow
	const maxShift = 5
	shift := retryCount
	if shift > maxShift {
		shift = maxShift
	}

	delay := BaseRetryDelay * time.Duration(1<<shift)
	if delay > MaxRetryDelay {
		return MaxRetryDelay
	}
	return delay
}
