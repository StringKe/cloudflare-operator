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
//   - Rate limit errors: 60-120 seconds (exponential backoff)
//   - Temporary errors (timeout, 502, 503, 504): 10-60 seconds (exponential backoff)
//   - Authentication errors: 5 minutes (needs manual intervention)
//   - Not found errors: 0 (no retry needed)
//   - Other errors: 30 seconds (default)
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
	case cf.IsAuthError(err):
		// Auth errors: longer delay, likely needs manual intervention
		return 5 * time.Minute
	case cf.IsNotFoundError(err):
		// Not found: no automatic retry, resource may need to be recreated
		return 30 * time.Second
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
			return nil
		}
		if !apierrors.IsConflict(err) {
			return err
		}
		lastErr = err
	}
	return fmt.Errorf("update SyncState status failed after %d retries: %w", MaxConflictRetries, lastErr)
}
