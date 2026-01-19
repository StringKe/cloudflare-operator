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

// ProjectService handles Pages project configuration registration.
// It implements the ConfigService interface for Pages project resources.
type ProjectService struct {
	*service.BaseService
}

// NewProjectService creates a new ProjectService.
func NewProjectService(c client.Client) *ProjectService {
	return &ProjectService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers a Pages project configuration to SyncState.
// Each PagesProject K8s resource has its own SyncState, keyed by the project name.
func (s *ProjectService) Register(ctx context.Context, opts ProjectRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"projectName", opts.ProjectName,
		"accountId", opts.AccountID,
		"source", opts.Source.String(),
	)
	logger.V(1).Info("Registering Pages project configuration")

	// Validate required fields
	if opts.ProjectName == "" {
		return errors.New("project name is required")
	}
	if opts.AccountID == "" {
		return errors.New("account ID is required")
	}

	// Get or create SyncState
	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypePagesProject,
		opts.ProjectName,
		opts.AccountID,
		"", // No zoneID for Pages projects
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get/create syncstate for Pages project: %w", err)
	}

	// Update source configuration
	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityPagesProject); err != nil {
		return fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("Pages project configuration registered successfully",
		"syncState", syncState.Name,
		"projectName", opts.Config.Name)
	return nil
}

// Unregister removes a Pages project's configuration from the SyncState.
// This is called when the PagesProject K8s resource is deleted.
func (s *ProjectService) Unregister(ctx context.Context, projectName string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"projectName", projectName,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering Pages project from SyncState")

	syncState, err := s.GetSyncState(ctx, ResourceTypePagesProject, projectName)
	if err != nil {
		return fmt.Errorf("get syncstate for Pages project: %w", err)
	}
	if syncState == nil {
		logger.V(1).Info("No SyncState found for Pages project")
		return nil
	}

	if err := s.RemoveSource(ctx, syncState, source); err != nil {
		return fmt.Errorf("remove source from syncstate: %w", err)
	}

	logger.Info("Pages project unregistered from SyncState")
	return nil
}

// ProjectSyncStatus represents the sync status of a Pages project.
type ProjectSyncStatus struct {
	// IsSynced indicates if the project has been synced to Cloudflare
	IsSynced bool
	// ProjectName is the Cloudflare project name
	ProjectName string
	// Subdomain is the *.pages.dev subdomain
	Subdomain string
	// SyncStateID is the name of the SyncState resource
	SyncStateID string
}

// GetSyncStatus returns the sync status for a Pages project.
func (s *ProjectService) GetSyncStatus(ctx context.Context, projectName string) (*ProjectSyncStatus, error) {
	logger := log.FromContext(ctx).WithValues(
		"projectName", projectName,
	)

	syncState, err := s.GetSyncState(ctx, ResourceTypePagesProject, projectName)
	if err != nil {
		return nil, fmt.Errorf("get syncstate: %w", err)
	}
	if syncState == nil {
		logger.V(1).Info("No SyncState found for Pages project")
		return nil, nil
	}

	isSynced := syncState.Status.SyncStatus == v1alpha2.SyncStatusSynced

	return &ProjectSyncStatus{
		IsSynced:    isSynced,
		ProjectName: syncState.Spec.CloudflareID,
		SyncStateID: syncState.Name,
	}, nil
}
