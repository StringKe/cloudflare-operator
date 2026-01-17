// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package r2

//nolint:dupl // Similar patterns across resource types are intentional

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// NotificationService manages R2BucketNotification configurations via CloudflareSyncState.
type NotificationService struct {
	*service.BaseService
}

// NewNotificationService creates a new R2BucketNotification service.
func NewNotificationService(c client.Client) *NotificationService {
	return &NotificationService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers an R2BucketNotification configuration with the SyncState.
func (s *NotificationService) Register(ctx context.Context, opts R2BucketNotificationRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"bucketName", opts.Config.BucketName,
		"queueName", opts.Config.QueueName,
		"source", opts.Source.String(),
	)
	logger.V(1).Info("Registering R2BucketNotification configuration")

	// Generate SyncState ID using bucket+queue combination
	syncStateID := opts.QueueID
	if syncStateID == "" {
		syncStateID = fmt.Sprintf("pending-%s", opts.Source.Name)
	}

	// Get or create SyncState
	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeR2BucketNotification,
		syncStateID,
		opts.AccountID,
		"", // No zone ID for R2 notifications
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get or create syncstate: %w", err)
	}

	// Update source in SyncState
	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityR2BucketNotification); err != nil {
		return fmt.Errorf("update source: %w", err)
	}

	logger.Info("R2BucketNotification configuration registered", "syncStateId", syncState.Name)
	return nil
}

// Unregister removes a configuration from the SyncState.
//
//nolint:revive // cognitive complexity is acceptable for state lookup logic
func (s *NotificationService) Unregister(ctx context.Context, queueID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"queueId", queueID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering R2BucketNotification from SyncState")

	// Try multiple possible SyncState IDs
	syncStateIDs := []string{
		queueID,
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypeR2BucketNotification, id)
		if err != nil {
			logger.V(1).Info("Error getting SyncState", "syncStateId", id, "error", err)
			continue
		}
		if syncState == nil {
			continue
		}

		if err := s.RemoveSource(ctx, syncState, source); err != nil {
			logger.Error(err, "Failed to remove source from SyncState", "syncStateId", id)
			continue
		}

		logger.Info("R2BucketNotification unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// UpdateQueueID updates the SyncState to use the actual queue ID
// after the queue is resolved.
func (s *NotificationService) UpdateQueueID(ctx context.Context, source service.Source, queueID string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"queueId", queueID,
	)

	pendingID := fmt.Sprintf("pending-%s", source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceTypeR2BucketNotification, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		logger.V(1).Info("No pending SyncState found, R2BucketNotification may already use actual ID")
		return nil
	}

	// Create new SyncState with the actual queue ID
	newSyncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeR2BucketNotification,
		queueID,
		pendingSyncState.Spec.AccountID,
		"",
		pendingSyncState.Spec.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("create new syncstate with queue ID: %w", err)
	}

	// Copy sources from pending to new
	newSyncState.Spec.Sources = pendingSyncState.Spec.Sources
	newSyncState.Spec.CloudflareID = queueID
	if err := s.Client.Update(ctx, newSyncState); err != nil {
		return fmt.Errorf("update new syncstate with sources: %w", err)
	}

	// Delete the pending SyncState
	if err := s.Client.Delete(ctx, pendingSyncState); err != nil {
		logger.Error(err, "Failed to delete pending SyncState", "pendingId", pendingID)
	}

	logger.Info("Updated SyncState to use actual queue ID",
		"oldId", pendingID,
		"newId", queueID)
	return nil
}

// UpdateStatus updates the K8s R2BucketNotification resource status based on sync result.
func (s *NotificationService) UpdateStatus(
	ctx context.Context,
	notification *v1alpha2.R2BucketNotification,
	result *R2BucketNotificationSyncResult,
) error {
	notification.Status.State = "Active"
	notification.Status.QueueID = result.QueueID
	notification.Status.RuleCount = result.RuleCount
	notification.Status.ObservedGeneration = notification.Generation

	return s.Client.Status().Update(ctx, notification)
}
