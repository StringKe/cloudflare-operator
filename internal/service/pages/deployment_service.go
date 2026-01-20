// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pages

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// DeploymentService handles Pages deployment configuration registration.
// It implements the ConfigService interface for Pages deployment resources.
type DeploymentService struct {
	*service.BaseService
}

// NewDeploymentService creates a new DeploymentService.
func NewDeploymentService(c client.Client) *DeploymentService {
	return &DeploymentService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers a Pages deployment configuration to SyncState.
// Each PagesDeployment K8s resource has its own SyncState.
//
// SyncState lookup strategy:
// 1. Always try pending ID first (pages-deployment-pending-{namespace}-{name})
// 2. Then try actual deployment ID if provided (for compatibility)
// 3. Only create new SyncState if neither exists
//
// This ensures configuration updates always find the correct SyncState,
// regardless of whether the deployment has been created yet.
//
// Rollback scenario:
// - If action is "rollback" and TargetDeploymentID is set, no file upload needed
// - The Sync Controller will call the rollback API directly
//
//nolint:revive // cognitive complexity is acceptable for this registration logic
func (s *DeploymentService) Register(ctx context.Context, opts DeploymentRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"projectName", opts.ProjectName,
		"deploymentId", opts.DeploymentID,
		"action", opts.Config.Action,
		"accountId", opts.AccountID,
		"source", opts.Source.String(),
	)
	logger.V(1).Info("Registering Pages deployment configuration")

	// Validate required fields
	if opts.ProjectName == "" {
		return errors.New("project name is required")
	}
	if opts.AccountID == "" {
		return errors.New("account ID is required")
	}

	// Use fixed pending ID format for SyncState lookup
	// This ensures we always find the same SyncState regardless of deployment status
	pendingID := fmt.Sprintf("pending-%s-%s", opts.Source.Namespace, opts.Source.Name)

	// Step 1: Try to find existing SyncState with pending ID first
	// This is the primary lookup because SyncState names are based on the initial cloudflareID
	syncState, err := s.GetSyncState(ctx, ResourceTypePagesDeployment, pendingID)
	if err != nil {
		return fmt.Errorf("get syncstate with pending id: %w", err)
	}

	// Step 2: If not found with pending ID, try with actual deployment ID (for compatibility)
	if syncState == nil && opts.DeploymentID != "" && opts.DeploymentID != pendingID {
		syncState, err = s.GetSyncState(ctx, ResourceTypePagesDeployment, opts.DeploymentID)
		if err != nil {
			return fmt.Errorf("get syncstate with deployment id: %w", err)
		}
		if syncState != nil {
			logger.V(1).Info("Found SyncState with deployment ID",
				"syncState", syncState.Name,
				"deploymentId", opts.DeploymentID)
		}
	}

	// Step 3: If still not found, create new SyncState
	if syncState == nil {
		// Determine the initial CloudflareID for new SyncState
		cloudflareID := pendingID

		// For rollback action, use the target deployment ID
		switch {
		case opts.Config.Action == "rollback" && opts.Config.TargetDeploymentID != "":
			cloudflareID = opts.Config.TargetDeploymentID
		case opts.Config.Action == "rollback" && opts.Config.Rollback != nil && opts.Config.Rollback.DeploymentID != "":
			cloudflareID = opts.Config.Rollback.DeploymentID
		}

		logger.Info("Creating new SyncState for Pages deployment",
			"cloudflareId", cloudflareID)

		syncState, err = s.GetOrCreateSyncState(
			ctx,
			ResourceTypePagesDeployment,
			cloudflareID,
			opts.AccountID,
			"", // No zoneID for Pages deployments
			opts.CredentialsRef,
		)
		if err != nil {
			return fmt.Errorf("create syncstate for Pages deployment: %w", err)
		}
	}

	// Update source configuration
	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityPagesDeployment); err != nil {
		return fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("Pages deployment configuration registered successfully",
		"syncState", syncState.Name,
		"projectName", opts.Config.ProjectName,
		"action", opts.Config.Action,
		"cloudflareId", syncState.Spec.CloudflareID)
	return nil
}

// Unregister removes a Pages deployment's configuration from the SyncState.
// This is called when the PagesDeployment K8s resource is deleted.
//
// Note: This only removes the source from SyncState. The Sync Controller will
// handle the SyncState cleanup, but will NOT delete the Cloudflare deployment
// because active production deployments cannot be deleted.
//
// Lookup order: pending ID first (because SyncState names are based on initial cloudflareID)
//
//nolint:revive // cognitive complexity is acceptable for unregistration with fallback logic
func (s *DeploymentService) Unregister(ctx context.Context, deploymentID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"deploymentId", deploymentID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering Pages deployment from SyncState")

	// Try multiple possible SyncState CloudflareIDs
	// Priority: pending ID first (because SyncState name is based on initial cloudflareID)
	pendingID := fmt.Sprintf("pending-%s-%s", source.Namespace, source.Name)
	syncStateCloudflareIDs := []string{
		pendingID,    // Try pending ID first - this is the most common case
		deploymentID, // Then try actual deployment ID for compatibility
	}

	for _, cloudflareID := range syncStateCloudflareIDs {
		if cloudflareID == "" || cloudflareID == pendingID && deploymentID == pendingID {
			// Skip duplicate lookup if deploymentID equals pendingID
			if cloudflareID == "" {
				continue
			}
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypePagesDeployment, cloudflareID)
		if err != nil {
			logger.V(1).Info("Error getting SyncState", "cloudflareId", cloudflareID, "error", err)
			continue
		}
		if syncState == nil {
			continue
		}

		if err := s.RemoveSource(ctx, syncState, source); err != nil {
			logger.Error(err, "Failed to remove source from SyncState", "cloudflareId", cloudflareID)
			continue
		}

		logger.Info("Pages deployment unregistered from SyncState",
			"syncState", syncState.Name,
			"cloudflareId", cloudflareID)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// DeploymentSyncStatus represents the sync status of a Pages deployment.
type DeploymentSyncStatus struct {
	// IsSynced indicates if the deployment has been synced to Cloudflare
	IsSynced bool
	// DeploymentID is the Cloudflare deployment ID
	DeploymentID string
	// Stage is the current deployment stage
	Stage string
	// URL is the deployment URL
	URL string
	// SyncStateID is the name of the SyncState resource
	SyncStateID string
}

// GetSyncStatus returns the sync status for a Pages deployment.
//
// Lookup order: pending ID first (because SyncState names are based on initial cloudflareID)
//
//nolint:revive // cognitive complexity acceptable for status lookup logic
func (s *DeploymentService) GetSyncStatus(ctx context.Context, source service.Source, knownDeploymentID string) (*DeploymentSyncStatus, error) {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"knownDeploymentId", knownDeploymentID,
	)

	// Try both possible SyncState IDs
	// Priority: pending ID first (because SyncState name is based on initial cloudflareID)
	pendingID := fmt.Sprintf("pending-%s-%s", source.Namespace, source.Name)
	syncStateIDs := []string{
		pendingID,         // Try pending ID first - this is where the SyncState usually is
		knownDeploymentID, // Then try known deployment ID for compatibility
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}
		// Skip duplicate lookup
		if id == knownDeploymentID && knownDeploymentID == pendingID {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypePagesDeployment, id)
		if err != nil {
			logger.V(1).Info("Error getting SyncState", "syncStateId", id, "error", err)
			continue
		}
		if syncState == nil {
			continue
		}

		isSynced := syncState.Status.SyncStatus == v1alpha2.SyncStatusSynced
		deploymentID := syncState.Spec.CloudflareID

		// If the CloudflareID starts with "pending-", it's not a real ID
		if strings.HasPrefix(deploymentID, "pending-") {
			deploymentID = ""
		}

		return &DeploymentSyncStatus{
			IsSynced:     isSynced,
			DeploymentID: deploymentID,
			SyncStateID:  syncState.Name,
		}, nil
	}

	return nil, nil
}
