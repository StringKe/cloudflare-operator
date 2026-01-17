// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package dns provides the DNSService for managing Cloudflare DNS record configuration.
// Unlike TunnelConfigService which aggregates multiple sources, DNSService manages
// individual DNS records with a 1:1 mapping between K8s DNSRecord and Cloudflare DNS record.
package dns

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

const (
	// ResourceType is the SyncState resource type for DNS records
	ResourceType = v1alpha2.SyncResourceDNSRecord

	// PriorityDNSRecord is the default priority for DNS record configuration
	PriorityDNSRecord = 100
)

// Service handles DNS record configuration registration.
// It implements the ConfigService interface for DNS record resources.
type Service struct {
	*service.BaseService
}

// NewService creates a new DNSService
func NewService(c client.Client) *Service {
	return &Service{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers a DNS record configuration to SyncState.
// Each DNSRecord K8s resource has its own SyncState, keyed by a generated ID
// (namespace/name) until the Cloudflare record ID is known.
func (s *Service) Register(ctx context.Context, opts RegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"zoneId", opts.ZoneID,
		"source", opts.Source.String(),
		"recordName", opts.Config.Name,
	)
	logger.V(1).Info("Registering DNS record configuration")

	// Generate SyncState ID:
	// - If RecordID is known (existing record), use it
	// - Otherwise, use a placeholder based on source (namespace/name)
	syncStateID := opts.RecordID
	if syncStateID == "" {
		syncStateID = fmt.Sprintf("pending-%s-%s", opts.Source.Namespace, opts.Source.Name)
	}

	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceType,
		syncStateID,
		opts.AccountID,
		opts.ZoneID,
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get/create syncstate for DNS record: %w", err)
	}

	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityDNSRecord); err != nil {
		return fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("DNS record configuration registered successfully",
		"syncState", syncState.Name,
		"recordName", opts.Config.Name,
		"recordType", opts.Config.Type)
	return nil
}

// Unregister removes a DNS record's configuration from the SyncState.
// This is called when the DNSRecord K8s resource is deleted.
//
//nolint:revive // cognitive complexity is acceptable for unregistration with fallback logic
func (s *Service) Unregister(ctx context.Context, recordID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"recordId", recordID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering DNS record from SyncState")

	// Try both possible SyncState IDs
	// 1. The actual record ID (if record was created)
	// 2. The pending placeholder (if record was never created)
	syncStateIDs := []string{
		recordID,
		fmt.Sprintf("pending-%s-%s", source.Namespace, source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceType, id)
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

		logger.Info("DNS record unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// UpdateRecordID updates the SyncState to use the actual Cloudflare record ID
// after the record is created. This migrates from the pending placeholder.
func (s *Service) UpdateRecordID(ctx context.Context, source service.Source, newRecordID string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"newRecordId", newRecordID,
	)

	pendingID := fmt.Sprintf("pending-%s-%s", source.Namespace, source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceType, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		// No pending state, might already be using real ID
		logger.V(1).Info("No pending SyncState found, record may already use actual ID")
		return nil
	}

	// Update the SyncState name to use the real record ID
	// This requires creating a new SyncState with the correct ID and deleting the old one
	newSyncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceType,
		newRecordID,
		pendingSyncState.Spec.AccountID,
		pendingSyncState.Spec.ZoneID,
		pendingSyncState.Spec.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("create new syncstate with record ID: %w", err)
	}

	// Copy sources from pending to new
	newSyncState.Spec.Sources = pendingSyncState.Spec.Sources
	if err := s.Client.Update(ctx, newSyncState); err != nil {
		return fmt.Errorf("update new syncstate with sources: %w", err)
	}

	// Delete the pending SyncState
	if err := s.Client.Delete(ctx, pendingSyncState); err != nil {
		logger.Error(err, "Failed to delete pending SyncState", "pendingId", pendingID)
		// Non-fatal - the pending state will be orphaned but won't cause issues
	}

	logger.Info("Updated SyncState to use actual record ID",
		"oldId", pendingID,
		"newId", newRecordID)
	return nil
}
