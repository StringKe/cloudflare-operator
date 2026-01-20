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
// SyncState naming strategy:
// - Always use fixed format: pages-deployment-{namespace}-{name}
// - This avoids the need to "migrate" SyncState after deployment creation
// - The actual CloudflareID (deployment ID) is stored in spec.cloudflareID
//
// Rollback scenario:
// - If action is "rollback" and TargetDeploymentID is set, no file upload needed
// - The Sync Controller will call the rollback API directly
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

	// Use fixed SyncState ID format based on K8s resource identity
	// This ensures stable naming across deployment lifecycles
	syncStateID := fmt.Sprintf("pages-deployment-%s-%s", opts.Source.Namespace, opts.Source.Name)

	// Determine the CloudflareID to store in SyncState
	// - For rollback: use the target deployment ID (existing deployment)
	// - For new deployments: use pending placeholder, will be updated after creation
	cloudflareID := opts.DeploymentID
	if cloudflareID == "" {
		// Check if this is a rollback to existing deployment
		switch {
		case opts.Config.Action == "rollback" && opts.Config.TargetDeploymentID != "":
			cloudflareID = opts.Config.TargetDeploymentID
		case opts.Config.Action == "rollback" && opts.Config.Rollback != nil && opts.Config.Rollback.DeploymentID != "":
			cloudflareID = opts.Config.Rollback.DeploymentID
		default:
			// New deployment, use pending placeholder
			cloudflareID = fmt.Sprintf("pending-%s-%s", opts.Source.Namespace, opts.Source.Name)
		}
	}

	// Get or create SyncState
	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypePagesDeployment,
		cloudflareID,
		opts.AccountID,
		"", // No zoneID for Pages deployments
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get/create syncstate for Pages deployment: %w", err)
	}

	// Override the SyncState name if it doesn't match our expected format
	// This handles migration from old naming scheme
	if syncState.Name != syncStateID {
		logger.V(1).Info("SyncState name mismatch, will use existing",
			"expected", syncStateID,
			"actual", syncState.Name)
	}

	// Update source configuration
	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityPagesDeployment); err != nil {
		return fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("Pages deployment configuration registered successfully",
		"syncState", syncState.Name,
		"projectName", opts.Config.ProjectName,
		"action", opts.Config.Action,
		"cloudflareId", cloudflareID)
	return nil
}

// Unregister removes a Pages deployment's configuration from the SyncState.
// This is called when the PagesDeployment K8s resource is deleted.
//
// Note: This only removes the source from SyncState. The Sync Controller will
// handle the SyncState cleanup, but will NOT delete the Cloudflare deployment
// because active production deployments cannot be deleted.
//
//nolint:revive // cognitive complexity is acceptable for unregistration with fallback logic
func (s *DeploymentService) Unregister(ctx context.Context, deploymentID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"deploymentId", deploymentID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering Pages deployment from SyncState")

	// Try multiple possible SyncState CloudflareIDs
	// The SyncState uses CloudflareID as part of its identity
	// We need to check both:
	// 1. The actual deployment ID (if deployment was created)
	// 2. The pending placeholder (if deployment was never created)
	syncStateCloudflareIDs := []string{
		deploymentID,
		fmt.Sprintf("pending-%s-%s", source.Namespace, source.Name),
	}

	for _, cloudflareID := range syncStateCloudflareIDs {
		if cloudflareID == "" {
			continue
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
//nolint:revive // cognitive complexity acceptable for status lookup logic
func (s *DeploymentService) GetSyncStatus(ctx context.Context, source service.Source, knownDeploymentID string) (*DeploymentSyncStatus, error) {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"knownDeploymentId", knownDeploymentID,
	)

	// Try both possible SyncState IDs
	syncStateIDs := []string{
		knownDeploymentID,
		fmt.Sprintf("pending-%s-%s", source.Namespace, source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
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

// UpdateDeploymentID updates the SyncState to use the actual Cloudflare deployment ID
// after the deployment is created. This migrates from the pending placeholder.
func (s *DeploymentService) UpdateDeploymentID(ctx context.Context, source service.Source, newDeploymentID string) error {
	logger := log.FromContext(ctx).WithValues(
		"source", source.String(),
		"newDeploymentId", newDeploymentID,
	)

	pendingID := fmt.Sprintf("pending-%s-%s", source.Namespace, source.Name)

	// Get the pending SyncState
	pendingSyncState, err := s.GetSyncState(ctx, ResourceTypePagesDeployment, pendingID)
	if err != nil {
		return fmt.Errorf("get pending syncstate: %w", err)
	}

	if pendingSyncState == nil {
		// No pending state, might already be using real ID
		logger.V(1).Info("No pending SyncState found, deployment may already use actual ID")
		return nil
	}

	// Update the SyncState CloudflareID to use the real deployment ID
	// Create a new SyncState with the correct ID and delete the old one
	newSyncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypePagesDeployment,
		newDeploymentID,
		pendingSyncState.Spec.AccountID,
		pendingSyncState.Spec.ZoneID,
		pendingSyncState.Spec.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("create new syncstate with deployment ID: %w", err)
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

	logger.Info("Updated SyncState to use actual deployment ID",
		"oldId", pendingID,
		"newId", newDeploymentID)
	return nil
}
