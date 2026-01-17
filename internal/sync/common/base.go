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

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
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
func (c *BaseSyncController) UpdateSyncStatus(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	status v1alpha2.SyncStatus,
	result *SyncResult,
	syncErr error,
) error {
	logger := log.FromContext(ctx)

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
			LastTransitionTime: metav1.Now(),
		})
	} else {
		c.setStatusConditionByStatus(syncState, status)
	}

	// Perform status update
	if err := c.Client.Status().Update(ctx, syncState); err != nil {
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
// Permanent errors get longer delays, transient errors get shorter delays.
func RequeueAfterError(_ error) time.Duration {
	// Default requeue delay for most errors
	// TODO: implement error-type-specific delays
	return 30 * time.Second
}

// RequeueAfterSuccess returns a requeue duration for periodic refresh.
// Most syncs don't need periodic refresh (they're event-driven), so this returns 0.
func RequeueAfterSuccess() time.Duration {
	return 0
}
