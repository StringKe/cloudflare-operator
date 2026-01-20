// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pages

import (
	"context"
	"errors"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// DomainService handles Pages custom domain configuration registration.
// It implements the ConfigService interface for Pages domain resources.
type DomainService struct {
	*service.BaseService
}

// NewDomainService creates a new DomainService.
func NewDomainService(c client.Client) *DomainService {
	return &DomainService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers a Pages domain configuration to SyncState.
// Each PagesDomain K8s resource has its own SyncState, keyed by domain name.
func (s *DomainService) Register(ctx context.Context, opts DomainRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"domain", opts.DomainName,
		"projectName", opts.ProjectName,
		"accountId", opts.AccountID,
		"source", opts.Source.String(),
	)
	logger.V(1).Info("Registering Pages domain configuration")

	// Validate required fields
	if opts.DomainName == "" {
		return errors.New("domain name is required")
	}
	if opts.ProjectName == "" {
		return errors.New("project name is required")
	}
	if opts.AccountID == "" {
		return errors.New("account ID is required")
	}

	// Use domain name as CloudflareID (unique per account)
	syncStateID := fmt.Sprintf("%s-%s", opts.ProjectName, opts.DomainName)

	// Get or create SyncState with optional ZoneID for DNS auto-configuration
	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypePagesDomain,
		syncStateID,
		opts.AccountID,
		opts.ZoneID, // Pass ZoneID if provided for DNS auto-configuration
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get/create syncstate for Pages domain: %w", err)
	}

	// Update source configuration
	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityPagesDomain); err != nil {
		return fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("Pages domain configuration registered successfully",
		"syncState", syncState.Name,
		"domain", opts.Config.Domain)
	return nil
}

// Unregister removes a Pages domain's configuration from the SyncState.
// This is called when the PagesDomain K8s resource is deleted.
func (s *DomainService) Unregister(ctx context.Context, projectName, domainName string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"projectName", projectName,
		"domain", domainName,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering Pages domain from SyncState")

	syncStateID := fmt.Sprintf("%s-%s", projectName, domainName)

	syncState, err := s.GetSyncState(ctx, ResourceTypePagesDomain, syncStateID)
	if err != nil {
		return fmt.Errorf("get syncstate for Pages domain: %w", err)
	}
	if syncState == nil {
		logger.V(1).Info("No SyncState found for Pages domain")
		return nil
	}

	if err := s.RemoveSource(ctx, syncState, source); err != nil {
		return fmt.Errorf("remove source from syncstate: %w", err)
	}

	logger.Info("Pages domain unregistered from SyncState")
	return nil
}

// DomainSyncStatus represents the sync status of a Pages domain.
type DomainSyncStatus struct {
	// IsSynced indicates if the domain has been synced to Cloudflare
	IsSynced bool
	// DomainID is the Cloudflare domain ID
	DomainID string
	// Status is the domain status (active, pending, etc.)
	Status string
	// SyncStateID is the name of the SyncState resource
	SyncStateID string
}

// GetSyncStatus returns the sync status for a Pages domain.
func (s *DomainService) GetSyncStatus(ctx context.Context, projectName, domainName string) (*DomainSyncStatus, error) {
	logger := log.FromContext(ctx).WithValues(
		"projectName", projectName,
		"domain", domainName,
	)

	syncStateID := fmt.Sprintf("%s-%s", projectName, domainName)

	syncState, err := s.GetSyncState(ctx, ResourceTypePagesDomain, syncStateID)
	if err != nil {
		return nil, fmt.Errorf("get syncstate: %w", err)
	}
	if syncState == nil {
		logger.V(1).Info("No SyncState found for Pages domain")
		return nil, nil
	}

	isSynced := syncState.Status.SyncStatus == v1alpha2.SyncStatusSynced

	return &DomainSyncStatus{
		IsSynced:    isSynced,
		SyncStateID: syncState.Name,
	}, nil
}
