// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package domain

//nolint:dupl // Similar patterns across resource types are intentional

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// CloudflareDomainService manages CloudflareDomain configurations via CloudflareSyncState.
type CloudflareDomainService struct {
	*service.BaseService
}

// NewCloudflareDomainService creates a new CloudflareDomain service.
func NewCloudflareDomainService(c client.Client) *CloudflareDomainService {
	return &CloudflareDomainService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers a CloudflareDomain configuration with the SyncState.
func (s *CloudflareDomainService) Register(ctx context.Context, opts CloudflareDomainRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"domain", opts.Config.Domain,
		"source", opts.Source.String(),
	)
	logger.V(1).Info("Registering CloudflareDomain configuration")

	// Generate SyncState ID using zone ID or domain name
	syncStateID := opts.ZoneID
	if syncStateID == "" {
		syncStateID = fmt.Sprintf("pending-%s", opts.Source.Name)
	}

	// Get or create SyncState
	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeCloudflareDomain,
		syncStateID,
		opts.AccountID,
		opts.ZoneID,
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get or create syncstate: %w", err)
	}

	// Update source in SyncState
	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityCloudflareDomain); err != nil {
		return fmt.Errorf("update source: %w", err)
	}

	logger.Info("CloudflareDomain configuration registered", "syncStateId", syncState.Name)
	return nil
}

// Unregister removes a CloudflareDomain configuration from the SyncState.
//
//nolint:revive // cognitive complexity is acceptable for state lookup logic
func (s *CloudflareDomainService) Unregister(ctx context.Context, zoneID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"zoneId", zoneID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering CloudflareDomain from SyncState")

	// Try multiple possible SyncState IDs
	syncStateIDs := []string{
		zoneID,
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypeCloudflareDomain, id)
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

		logger.Info("CloudflareDomain unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// UpdateZoneID updates the SyncState to use the actual zone ID.
func (s *CloudflareDomainService) UpdateZoneID(ctx context.Context, source service.Source, zoneID, accountID string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"zoneId", zoneID,
	)

	pendingID := fmt.Sprintf("pending-%s", source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceTypeCloudflareDomain, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		logger.V(1).Info("No pending SyncState found, CloudflareDomain may already use actual ID")
		return nil
	}

	// Create new SyncState with the actual zone ID
	newSyncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeCloudflareDomain,
		zoneID,
		accountID,
		zoneID,
		pendingSyncState.Spec.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("create new syncstate with zone ID: %w", err)
	}

	// Copy sources from pending to new
	newSyncState.Spec.Sources = pendingSyncState.Spec.Sources
	newSyncState.Spec.CloudflareID = zoneID
	if err := s.Client.Update(ctx, newSyncState); err != nil {
		return fmt.Errorf("update new syncstate with sources: %w", err)
	}

	// Delete the pending SyncState
	if err := s.Client.Delete(ctx, pendingSyncState); err != nil {
		logger.Error(err, "Failed to delete pending SyncState", "pendingId", pendingID)
	}

	logger.Info("Updated SyncState to use actual zone ID",
		"oldId", pendingID,
		"newId", zoneID)
	return nil
}

// UpdateStatus updates the K8s CloudflareDomain resource status based on sync result.
func (s *CloudflareDomainService) UpdateStatus(
	ctx context.Context,
	domain *v1alpha2.CloudflareDomain,
	result *CloudflareDomainSyncResult,
) error {
	domain.Status.ZoneID = result.ZoneID
	domain.Status.ZoneName = result.ZoneName
	domain.Status.State = v1alpha2.CloudflareDomainState(result.Status)
	domain.Status.ObservedGeneration = domain.Generation

	return s.Client.Status().Update(ctx, domain)
}
