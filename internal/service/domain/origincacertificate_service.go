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

// RequestCreate requests creation of a new Origin CA certificate.
// The actual creation is performed by the OriginCACertificateSyncController.
// Returns the SyncState name that can be watched for completion.
func (s *OriginCACertificateService) RequestCreate(ctx context.Context, opts OriginCACertificateCreateOptions) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"hostnames", opts.Hostnames,
		"source", opts.Source.String(),
	)
	logger.Info("Requesting Origin CA certificate creation")

	// Build lifecycle config
	config := OriginCACertificateLifecycleConfig{
		Action:       OriginCACertificateActionCreate,
		Hostnames:    opts.Hostnames,
		RequestType:  opts.RequestType,
		ValidityDays: opts.ValidityDays,
		CSR:          opts.CSR,
	}

	// Use source name as the SyncState ID placeholder (will be updated after creation)
	syncStateName := fmt.Sprintf("originca-%s-%s", opts.Source.Namespace, opts.Source.Name)

	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeOriginCACertificate,
		syncStateName,
		opts.AccountID,
		opts.ZoneID,
		opts.CredentialsRef,
	)
	if err != nil {
		return "", fmt.Errorf("get/create syncstate for certificate: %w", err)
	}

	if err := s.UpdateSource(ctx, syncState, opts.Source, config, PriorityOriginCACertificate); err != nil {
		return "", fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("Certificate creation request registered", "syncStateName", syncState.Name)
	return syncState.Name, nil
}

// RequestRevoke requests revocation of an existing Origin CA certificate.
// The actual revocation is performed by the OriginCACertificateSyncController.
// Returns the SyncState name that can be watched for completion.
func (s *OriginCACertificateService) RequestRevoke(ctx context.Context, opts OriginCACertificateRevokeOptions) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"certificateId", opts.CertificateID,
		"source", opts.Source.String(),
	)
	logger.Info("Requesting Origin CA certificate revocation")

	// Build lifecycle config
	config := OriginCACertificateLifecycleConfig{
		Action:        OriginCACertificateActionRevoke,
		CertificateID: opts.CertificateID,
	}

	syncStateName := fmt.Sprintf("originca-%s-%s", opts.Source.Namespace, opts.Source.Name)

	syncState, err := s.GetSyncState(ctx, ResourceTypeOriginCACertificate, syncStateName)
	if err != nil {
		return "", fmt.Errorf("get syncstate for certificate: %w", err)
	}

	if syncState == nil {
		// Create new SyncState for revocation
		syncState, err = s.GetOrCreateSyncState(
			ctx,
			ResourceTypeOriginCACertificate,
			syncStateName,
			opts.AccountID,
			opts.ZoneID,
			opts.CredentialsRef,
		)
		if err != nil {
			return "", fmt.Errorf("create syncstate for certificate revocation: %w", err)
		}
	}

	// Reset sync status to pending for new operation
	syncState.Status.SyncStatus = v1alpha2.SyncStatusPending
	if err := s.Client.Status().Update(ctx, syncState); err != nil {
		logger.Error(err, "Failed to reset sync status")
	}

	// Update the source with revoke action
	if err := s.UpdateSource(ctx, syncState, opts.Source, config, PriorityOriginCACertificate); err != nil {
		return "", fmt.Errorf("update source with revoke action: %w", err)
	}

	logger.Info("Certificate revocation request registered", "syncStateName", syncState.Name)
	return syncState.Name, nil
}

// RequestRenew requests renewal of an existing Origin CA certificate.
// The actual renewal is performed by the OriginCACertificateSyncController.
// Returns the SyncState name that can be watched for completion.
func (s *OriginCACertificateService) RequestRenew(ctx context.Context, opts OriginCACertificateRenewOptions) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"certificateId", opts.CertificateID,
		"hostnames", opts.Hostnames,
		"source", opts.Source.String(),
	)
	logger.Info("Requesting Origin CA certificate renewal")

	// Build lifecycle config
	config := OriginCACertificateLifecycleConfig{
		Action:        OriginCACertificateActionRenew,
		CertificateID: opts.CertificateID,
		Hostnames:     opts.Hostnames,
		RequestType:   opts.RequestType,
		ValidityDays:  opts.ValidityDays,
		CSR:           opts.CSR,
	}

	syncStateName := fmt.Sprintf("originca-%s-%s", opts.Source.Namespace, opts.Source.Name)

	syncState, err := s.GetSyncState(ctx, ResourceTypeOriginCACertificate, syncStateName)
	if err != nil {
		return "", fmt.Errorf("get syncstate for certificate: %w", err)
	}

	if syncState == nil {
		syncState, err = s.GetOrCreateSyncState(
			ctx,
			ResourceTypeOriginCACertificate,
			syncStateName,
			opts.AccountID,
			opts.ZoneID,
			opts.CredentialsRef,
		)
		if err != nil {
			return "", fmt.Errorf("create syncstate for certificate renewal: %w", err)
		}
	}

	// Reset sync status to pending for new operation
	syncState.Status.SyncStatus = v1alpha2.SyncStatusPending
	if err := s.Client.Status().Update(ctx, syncState); err != nil {
		logger.Error(err, "Failed to reset sync status")
	}

	if err := s.UpdateSource(ctx, syncState, opts.Source, config, PriorityOriginCACertificate); err != nil {
		return "", fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("Certificate renewal request registered", "syncStateName", syncState.Name)
	return syncState.Name, nil
}

// GetLifecycleResult retrieves the result of a lifecycle operation from SyncState.
// Returns nil if the operation hasn't completed yet.
func (s *OriginCACertificateService) GetLifecycleResult(ctx context.Context, namespace, name string) (*OriginCACertificateSyncResult, error) {
	syncStateName := fmt.Sprintf("originca-%s-%s", namespace, name)
	syncState, err := s.GetSyncState(ctx, ResourceTypeOriginCACertificate, syncStateName)
	if err != nil {
		return nil, fmt.Errorf("get syncstate: %w", err)
	}

	if syncState == nil {
		return nil, nil
	}

	// Check if operation completed
	if syncState.Status.SyncStatus != v1alpha2.SyncStatusSynced {
		return nil, nil
	}

	// Extract result from ResultData
	if syncState.Status.ResultData == nil {
		return nil, nil
	}

	result := &OriginCACertificateSyncResult{
		CertificateID: syncState.Status.ResultData[ResultKeyOriginCACertificateID],
		Certificate:   syncState.Status.ResultData[ResultKeyOriginCACertificate],
	}

	return result, nil
}

// IsLifecycleCompleted checks if the lifecycle operation has completed.
func (s *OriginCACertificateService) IsLifecycleCompleted(ctx context.Context, namespace, name string) (bool, error) {
	syncStateName := fmt.Sprintf("originca-%s-%s", namespace, name)
	syncState, err := s.GetSyncState(ctx, ResourceTypeOriginCACertificate, syncStateName)
	if err != nil {
		return false, err
	}

	if syncState == nil {
		return false, nil
	}

	return syncState.Status.SyncStatus == v1alpha2.SyncStatusSynced, nil
}

// GetLifecycleError returns the error message if the lifecycle operation failed.
func (s *OriginCACertificateService) GetLifecycleError(ctx context.Context, namespace, name string) (string, error) {
	syncStateName := fmt.Sprintf("originca-%s-%s", namespace, name)
	syncState, err := s.GetSyncState(ctx, ResourceTypeOriginCACertificate, syncStateName)
	if err != nil {
		return "", err
	}

	if syncState == nil {
		return "", nil
	}

	if syncState.Status.SyncStatus == v1alpha2.SyncStatusError {
		return syncState.Status.Error, nil
	}

	return "", nil
}

// CleanupSyncState removes the SyncState for a certificate after successful deletion.
func (s *OriginCACertificateService) CleanupSyncState(ctx context.Context, namespace, name string) error {
	syncStateName := fmt.Sprintf("originca-%s-%s", namespace, name)
	syncState, err := s.GetSyncState(ctx, ResourceTypeOriginCACertificate, syncStateName)
	if err != nil {
		return err
	}

	if syncState == nil {
		return nil
	}

	return s.Client.Delete(ctx, syncState)
}
