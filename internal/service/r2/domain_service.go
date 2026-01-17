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

// DomainService manages R2BucketDomain configurations via CloudflareSyncState.
type DomainService struct {
	*service.BaseService
}

// NewDomainService creates a new R2BucketDomain service.
func NewDomainService(c client.Client) *DomainService {
	return &DomainService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers an R2BucketDomain configuration with the SyncState.
func (s *DomainService) Register(ctx context.Context, opts R2BucketDomainRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"domain", opts.Config.Domain,
		"bucketName", opts.Config.BucketName,
		"source", opts.Source.String(),
	)
	logger.V(1).Info("Registering R2BucketDomain configuration")

	// Generate SyncState ID using domain as unique identifier
	syncStateID := opts.DomainID
	if syncStateID == "" {
		syncStateID = fmt.Sprintf("pending-%s", opts.Source.Name)
	}

	// Get or create SyncState
	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeR2BucketDomain,
		syncStateID,
		opts.AccountID,
		opts.ZoneID,
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get or create syncstate: %w", err)
	}

	// Update source in SyncState
	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityR2BucketDomain); err != nil {
		return fmt.Errorf("update source: %w", err)
	}

	logger.Info("R2BucketDomain configuration registered", "syncStateId", syncState.Name)
	return nil
}

// Unregister removes a configuration from the SyncState.
//
//nolint:revive // cognitive complexity is acceptable for state lookup logic
func (s *DomainService) Unregister(ctx context.Context, domainID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"domainId", domainID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering R2BucketDomain from SyncState")

	// Try multiple possible SyncState IDs
	syncStateIDs := []string{
		domainID,
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypeR2BucketDomain, id)
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

		logger.Info("R2BucketDomain unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// UpdateDomainID updates the SyncState to use the actual domain ID
// after the domain is configured.
func (s *DomainService) UpdateDomainID(ctx context.Context, source service.Source, domainID, zoneID string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"domainId", domainID,
	)

	pendingID := fmt.Sprintf("pending-%s", source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceTypeR2BucketDomain, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		logger.V(1).Info("No pending SyncState found, R2BucketDomain may already use actual ID")
		return nil
	}

	// Create new SyncState with the actual domain ID
	newSyncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeR2BucketDomain,
		domainID,
		pendingSyncState.Spec.AccountID,
		zoneID,
		pendingSyncState.Spec.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("create new syncstate with domain ID: %w", err)
	}

	// Copy sources from pending to new
	newSyncState.Spec.Sources = pendingSyncState.Spec.Sources
	newSyncState.Spec.CloudflareID = domainID
	if err := s.Client.Update(ctx, newSyncState); err != nil {
		return fmt.Errorf("update new syncstate with sources: %w", err)
	}

	// Delete the pending SyncState
	if err := s.Client.Delete(ctx, pendingSyncState); err != nil {
		logger.Error(err, "Failed to delete pending SyncState", "pendingId", pendingID)
	}

	logger.Info("Updated SyncState to use actual domain ID",
		"oldId", pendingID,
		"newId", domainID)
	return nil
}

// UpdateStatus updates the K8s R2BucketDomain resource status based on sync result.
func (s *DomainService) UpdateStatus(
	ctx context.Context,
	domain *v1alpha2.R2BucketDomain,
	result *R2BucketDomainSyncResult,
) error {
	domain.Status.State = "Active"
	domain.Status.DomainID = result.DomainID
	domain.Status.ZoneID = result.ZoneID
	domain.Status.Enabled = result.Enabled
	domain.Status.MinTLS = result.MinTLS
	domain.Status.PublicAccessEnabled = result.PublicAccessEnabled
	domain.Status.URL = result.URL
	domain.Status.ObservedGeneration = domain.Generation

	return s.Client.Status().Update(ctx, domain)
}
