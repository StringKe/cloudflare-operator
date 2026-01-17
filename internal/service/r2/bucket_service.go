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

// BucketService manages R2Bucket configurations via CloudflareSyncState.
type BucketService struct {
	*service.BaseService
}

// NewBucketService creates a new R2Bucket service.
func NewBucketService(c client.Client) *BucketService {
	return &BucketService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers an R2Bucket configuration with the SyncState.
func (s *BucketService) Register(ctx context.Context, opts R2BucketRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"bucketName", opts.Config.Name,
		"source", opts.Source.String(),
	)
	logger.V(1).Info("Registering R2Bucket configuration")

	// Generate SyncState ID
	syncStateID := opts.BucketName
	if syncStateID == "" {
		syncStateID = fmt.Sprintf("pending-%s", opts.Source.Name)
	}

	// Get or create SyncState
	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeR2Bucket,
		syncStateID,
		opts.AccountID,
		"", // No zone ID for R2
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get or create syncstate: %w", err)
	}

	// Update source in SyncState
	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityR2Bucket); err != nil {
		return fmt.Errorf("update source: %w", err)
	}

	logger.Info("R2Bucket configuration registered", "syncStateId", syncState.Name)
	return nil
}

// Unregister removes a configuration from the SyncState.
//
//nolint:revive // cognitive complexity is acceptable for state lookup logic
func (s *BucketService) Unregister(ctx context.Context, bucketName string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"bucketName", bucketName,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering R2Bucket from SyncState")

	// Try multiple possible SyncState IDs
	syncStateIDs := []string{
		bucketName,
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypeR2Bucket, id)
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

		logger.Info("R2Bucket unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// UpdateBucketName updates the SyncState to use the actual bucket name
// after the bucket is created.
func (s *BucketService) UpdateBucketName(ctx context.Context, source service.Source, bucketName string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"bucketName", bucketName,
	)

	pendingID := fmt.Sprintf("pending-%s", source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceTypeR2Bucket, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		logger.V(1).Info("No pending SyncState found, R2Bucket may already use actual name")
		return nil
	}

	// Create new SyncState with the actual bucket name
	newSyncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeR2Bucket,
		bucketName,
		pendingSyncState.Spec.AccountID,
		"",
		pendingSyncState.Spec.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("create new syncstate with bucket name: %w", err)
	}

	// Copy sources from pending to new
	newSyncState.Spec.Sources = pendingSyncState.Spec.Sources
	newSyncState.Spec.CloudflareID = bucketName
	if err := s.Client.Update(ctx, newSyncState); err != nil {
		return fmt.Errorf("update new syncstate with sources: %w", err)
	}

	// Delete the pending SyncState
	if err := s.Client.Delete(ctx, pendingSyncState); err != nil {
		logger.Error(err, "Failed to delete pending SyncState", "pendingId", pendingID)
		// Non-fatal - the pending state will be orphaned but won't cause issues
	}

	logger.Info("Updated SyncState to use actual bucket name",
		"oldId", pendingID,
		"newId", bucketName)
	return nil
}

// UpdateStatus updates the K8s R2Bucket resource status based on sync result.
func (s *BucketService) UpdateStatus(
	ctx context.Context,
	bucket *v1alpha2.R2Bucket,
	result *R2BucketSyncResult,
) error {
	bucket.Status.State = service.StateReady
	bucket.Status.BucketName = result.BucketName
	bucket.Status.Location = result.Location
	bucket.Status.CORSRulesCount = result.CORSRulesCount
	bucket.Status.LifecycleRulesCount = result.LifecycleRulesCount
	bucket.Status.ObservedGeneration = bucket.Generation

	return s.Client.Status().Update(ctx, bucket)
}
