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

const (
	// boolTrueString is the string representation of boolean true for ResultData comparisons.
	boolTrueString = "true"
)

// DomainRegistrationService manages DomainRegistration configurations via CloudflareSyncState.
type DomainRegistrationService struct {
	*service.BaseService
}

// NewDomainRegistrationService creates a new DomainRegistration service.
func NewDomainRegistrationService(c client.Client) *DomainRegistrationService {
	return &DomainRegistrationService{
		BaseService: service.NewBaseService(c),
	}
}

// RequestSync requests a sync of domain registration information from Cloudflare.
// Returns the SyncState name that can be watched for completion.
func (s *DomainRegistrationService) RequestSync(ctx context.Context, opts DomainRegistrationRegisterOptions) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"domainName", opts.DomainName,
		"source", opts.Source.String(),
	)
	logger.Info("Requesting domain registration sync")

	// Build lifecycle config
	config := DomainRegistrationLifecycleConfig{
		Action:     DomainRegistrationActionSync,
		DomainName: opts.DomainName,
	}

	// Include configuration if present
	if opts.Configuration != nil {
		config.Configuration = opts.Configuration
		config.Action = DomainRegistrationActionUpdate
	}

	// Use domain name as the SyncState ID
	syncStateName := fmt.Sprintf("domainreg-%s", opts.DomainName)

	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeDomainRegistration,
		syncStateName,
		opts.AccountID,
		"", // No zone ID for domain registration
		opts.CredentialsRef,
	)
	if err != nil {
		return "", fmt.Errorf("get/create syncstate for domain: %w", err)
	}

	if err := s.UpdateSource(ctx, syncState, opts.Source, config, PriorityDomainRegistration); err != nil {
		return "", fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("Domain registration sync request registered", "syncStateName", syncState.Name)
	return syncState.Name, nil
}

// Unregister removes a domain registration configuration from the SyncState.
func (s *DomainRegistrationService) Unregister(ctx context.Context, domainName string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"domainName", domainName,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering DomainRegistration from SyncState")

	syncStateName := fmt.Sprintf("domainreg-%s", domainName)

	syncState, err := s.GetSyncState(ctx, ResourceTypeDomainRegistration, syncStateName)
	if err != nil {
		logger.V(1).Info("Error getting SyncState", "syncStateName", syncStateName, "error", err)
		return nil
	}
	if syncState == nil {
		return nil
	}

	if err := s.RemoveSource(ctx, syncState, source); err != nil {
		logger.Error(err, "Failed to remove source from SyncState", "syncStateName", syncStateName)
		return err
	}

	logger.Info("DomainRegistration unregistered from SyncState", "syncStateName", syncStateName)
	return nil
}

// GetLifecycleResult retrieves the result of a sync operation from SyncState.
// Returns nil if the operation hasn't completed yet.
func (s *DomainRegistrationService) GetLifecycleResult(ctx context.Context, domainName string) (*DomainRegistrationSyncResult, error) {
	syncStateName := fmt.Sprintf("domainreg-%s", domainName)
	syncState, err := s.GetSyncState(ctx, ResourceTypeDomainRegistration, syncStateName)
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

	result := &DomainRegistrationSyncResult{
		DomainID:         syncState.Status.ResultData[ResultKeyDomainID],
		CurrentRegistrar: syncState.Status.ResultData[ResultKeyCurrentRegistrar],
		TransferInStatus: syncState.Status.ResultData[ResultKeyTransferInStatus],
		Locked:           syncState.Status.ResultData[ResultKeyDomainLocked] == boolTrueString,
		AutoRenew:        syncState.Status.ResultData[ResultKeyDomainAutoRenew] == boolTrueString,
		Privacy:          syncState.Status.ResultData[ResultKeyDomainPrivacy] == boolTrueString,
	}

	return result, nil
}

// IsLifecycleCompleted checks if the sync operation has completed.
func (s *DomainRegistrationService) IsLifecycleCompleted(ctx context.Context, domainName string) (bool, error) {
	syncStateName := fmt.Sprintf("domainreg-%s", domainName)
	syncState, err := s.GetSyncState(ctx, ResourceTypeDomainRegistration, syncStateName)
	if err != nil {
		return false, err
	}

	if syncState == nil {
		return false, nil
	}

	return syncState.Status.SyncStatus == v1alpha2.SyncStatusSynced, nil
}

// GetLifecycleError returns the error message if the sync operation failed.
func (s *DomainRegistrationService) GetLifecycleError(ctx context.Context, domainName string) (string, error) {
	syncStateName := fmt.Sprintf("domainreg-%s", domainName)
	syncState, err := s.GetSyncState(ctx, ResourceTypeDomainRegistration, syncStateName)
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

// CleanupSyncState removes the SyncState for a domain registration after successful deletion.
func (s *DomainRegistrationService) CleanupSyncState(ctx context.Context, domainName string) error {
	syncStateName := fmt.Sprintf("domainreg-%s", domainName)
	syncState, err := s.GetSyncState(ctx, ResourceTypeDomainRegistration, syncStateName)
	if err != nil {
		return err
	}

	if syncState == nil {
		return nil
	}

	return s.Client.Delete(ctx, syncState)
}
