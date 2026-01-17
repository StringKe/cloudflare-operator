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

// OriginCACertificateService manages OriginCACertificate configurations via CloudflareSyncState.
type OriginCACertificateService struct {
	*service.BaseService
}

// NewOriginCACertificateService creates a new OriginCACertificate service.
func NewOriginCACertificateService(c client.Client) *OriginCACertificateService {
	return &OriginCACertificateService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers an OriginCACertificate configuration with the SyncState.
func (s *OriginCACertificateService) Register(ctx context.Context, opts OriginCACertificateRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"hostnames", opts.Config.Hostnames,
		"source", opts.Source.String(),
	)
	logger.V(1).Info("Registering OriginCACertificate configuration")

	// Generate SyncState ID
	syncStateID := opts.CertificateID
	if syncStateID == "" {
		syncStateID = fmt.Sprintf("pending-%s", opts.Source.Name)
	}

	// Get or create SyncState
	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeOriginCACertificate,
		syncStateID,
		opts.AccountID,
		opts.ZoneID,
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get or create syncstate: %w", err)
	}

	// Update source in SyncState
	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityOriginCACertificate); err != nil {
		return fmt.Errorf("update source: %w", err)
	}

	logger.Info("OriginCACertificate configuration registered", "syncStateId", syncState.Name)
	return nil
}

// Unregister removes a configuration from the SyncState.
//
//nolint:revive // cognitive complexity is acceptable for state lookup logic
func (s *OriginCACertificateService) Unregister(ctx context.Context, certificateID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"certificateId", certificateID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering OriginCACertificate from SyncState")

	// Try multiple possible SyncState IDs
	syncStateIDs := []string{
		certificateID,
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypeOriginCACertificate, id)
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

		logger.Info("OriginCACertificate unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// UpdateCertificateID updates the SyncState to use the actual certificate ID.
func (s *OriginCACertificateService) UpdateCertificateID(ctx context.Context, source service.Source, certificateID, accountID, zoneID string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"certificateId", certificateID,
	)

	pendingID := fmt.Sprintf("pending-%s", source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceTypeOriginCACertificate, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		logger.V(1).Info("No pending SyncState found, OriginCACertificate may already use actual ID")
		return nil
	}

	// Create new SyncState with the actual certificate ID
	newSyncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeOriginCACertificate,
		certificateID,
		accountID,
		zoneID,
		pendingSyncState.Spec.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("create new syncstate with certificate ID: %w", err)
	}

	// Copy sources from pending to new
	newSyncState.Spec.Sources = pendingSyncState.Spec.Sources
	newSyncState.Spec.CloudflareID = certificateID
	if err := s.Client.Update(ctx, newSyncState); err != nil {
		return fmt.Errorf("update new syncstate with sources: %w", err)
	}

	// Delete the pending SyncState
	if err := s.Client.Delete(ctx, pendingSyncState); err != nil {
		logger.Error(err, "Failed to delete pending SyncState", "pendingId", pendingID)
	}

	logger.Info("Updated SyncState to use actual certificate ID",
		"oldId", pendingID,
		"newId", certificateID)
	return nil
}

// UpdateStatus updates the K8s OriginCACertificate resource status based on sync result.
func (s *OriginCACertificateService) UpdateStatus(
	ctx context.Context,
	cert *v1alpha2.OriginCACertificate,
	result *OriginCACertificateSyncResult,
) error {
	cert.Status.CertificateID = result.CertificateID
	cert.Status.ExpiresAt = result.ExpiresAt
	cert.Status.Certificate = result.Certificate
	cert.Status.State = v1alpha2.OriginCACertificateStateReady
	cert.Status.ObservedGeneration = cert.Generation

	return s.Client.Status().Update(ctx, cert)
}
