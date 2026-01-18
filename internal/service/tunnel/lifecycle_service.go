// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package tunnel

import (
	"context"
	"encoding/json"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

const (
	// LifecycleResourceType is the SyncState resource type for tunnel lifecycle
	LifecycleResourceType = v1alpha2.SyncResourceTunnelLifecycle
)

// LifecycleService handles Tunnel lifecycle operations through SyncState.
// It provides methods to request tunnel creation, deletion, and adoption,
// which are then processed by the TunnelLifecycleSyncController.
type LifecycleService struct {
	*service.BaseService
}

// NewLifecycleService creates a new TunnelLifecycleService
func NewLifecycleService(c client.Client) *LifecycleService {
	return &LifecycleService{
		BaseService: service.NewBaseService(c),
	}
}

// RequestCreate requests creation of a new tunnel.
// The actual creation is performed by TunnelLifecycleSyncController.
// Returns the SyncState name that can be watched for completion.
func (s *LifecycleService) RequestCreate(ctx context.Context, opts CreateTunnelOptions) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"tunnelName", opts.TunnelName,
		"source", opts.Source.String(),
	)
	logger.Info("Requesting tunnel creation")

	// Build lifecycle config
	config := LifecycleConfig{
		Action:     LifecycleActionCreate,
		TunnelName: opts.TunnelName,
		ConfigSrc:  opts.ConfigSrc,
	}

	// Use tunnel name as the CloudflareID placeholder (will be updated after creation)
	syncStateName := fmt.Sprintf("tunnel-lifecycle-%s", opts.TunnelName)

	syncState, err := s.GetOrCreateSyncState(
		ctx,
		LifecycleResourceType,
		syncStateName, // Use name-based ID for pending tunnels
		opts.AccountID,
		"", // No zone ID for tunnels
		opts.CredentialsRef,
	)
	if err != nil {
		return "", fmt.Errorf("get/create syncstate for tunnel %s: %w", opts.TunnelName, err)
	}

	if err := s.UpdateSource(ctx, syncState, opts.Source, config, PriorityTunnelSettings); err != nil {
		return "", fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("Tunnel creation request registered", "syncStateName", syncState.Name)
	return syncState.Name, nil
}

// RequestDelete requests deletion of an existing tunnel.
// The actual deletion is performed by TunnelLifecycleSyncController.
// Returns the SyncState name that can be watched for completion.
func (s *LifecycleService) RequestDelete(ctx context.Context, opts DeleteTunnelOptions) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"tunnelId", opts.TunnelID,
		"tunnelName", opts.TunnelName,
		"source", opts.Source.String(),
	)
	logger.Info("Requesting tunnel deletion")

	// Build lifecycle config
	config := LifecycleConfig{
		Action:     LifecycleActionDelete,
		TunnelID:   opts.TunnelID,
		TunnelName: opts.TunnelName,
	}

	syncStateName := fmt.Sprintf("tunnel-lifecycle-%s", opts.TunnelName)

	syncState, err := s.GetSyncState(ctx, LifecycleResourceType, syncStateName)
	if err != nil {
		return "", fmt.Errorf("get syncstate for tunnel %s: %w", opts.TunnelName, err)
	}

	if syncState == nil {
		// Create new SyncState for deletion
		syncState, err = s.GetOrCreateSyncState(
			ctx,
			LifecycleResourceType,
			syncStateName,
			opts.AccountID,
			"",
			opts.CredentialsRef,
		)
		if err != nil {
			return "", fmt.Errorf("create syncstate for tunnel deletion: %w", err)
		}
	}

	// Update the source with delete action
	if err := s.UpdateSource(ctx, syncState, opts.Source, config, PriorityTunnelSettings); err != nil {
		return "", fmt.Errorf("update source with delete action: %w", err)
	}

	logger.Info("Tunnel deletion request registered", "syncStateName", syncState.Name)
	return syncState.Name, nil
}

// RequestAdopt requests adoption of an existing tunnel.
// The actual adoption (fetching credentials/token) is performed by TunnelLifecycleSyncController.
// Returns the SyncState name that can be watched for completion.
func (s *LifecycleService) RequestAdopt(ctx context.Context, opts AdoptTunnelOptions) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"tunnelId", opts.TunnelID,
		"tunnelName", opts.TunnelName,
		"source", opts.Source.String(),
	)
	logger.Info("Requesting tunnel adoption")

	// Build lifecycle config
	config := LifecycleConfig{
		Action:           LifecycleActionAdopt,
		TunnelID:         opts.TunnelID,
		TunnelName:       opts.TunnelName,
		ExistingTunnelID: opts.TunnelID,
	}

	syncStateName := fmt.Sprintf("tunnel-lifecycle-%s", opts.TunnelName)

	syncState, err := s.GetOrCreateSyncState(
		ctx,
		LifecycleResourceType,
		syncStateName,
		opts.AccountID,
		"",
		opts.CredentialsRef,
	)
	if err != nil {
		return "", fmt.Errorf("get/create syncstate for tunnel %s: %w", opts.TunnelName, err)
	}

	if err := s.UpdateSource(ctx, syncState, opts.Source, config, PriorityTunnelSettings); err != nil {
		return "", fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("Tunnel adoption request registered", "syncStateName", syncState.Name)
	return syncState.Name, nil
}

// GetLifecycleResult retrieves the result of a lifecycle operation from SyncState.
// Returns nil if the operation hasn't completed yet.
func (s *LifecycleService) GetLifecycleResult(ctx context.Context, tunnelName string) (*LifecycleResult, error) {
	syncStateName := fmt.Sprintf("tunnel-lifecycle-%s", tunnelName)
	syncState, err := s.GetSyncState(ctx, LifecycleResourceType, syncStateName)
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

	result := &LifecycleResult{
		TunnelID:    syncState.Status.ResultData[ResultKeyTunnelID],
		TunnelName:  syncState.Status.ResultData[ResultKeyTunnelName],
		TunnelToken: syncState.Status.ResultData[ResultKeyTunnelToken],
		Credentials: syncState.Status.ResultData[ResultKeyCredentials],
		AccountTag:  syncState.Status.ResultData[ResultKeyAccountTag],
	}

	return result, nil
}

// GetSyncStateName returns the SyncState name for a tunnel
func GetSyncStateName(tunnelName string) string {
	return fmt.Sprintf("tunnel-lifecycle-%s", tunnelName)
}

// IsLifecycleCompleted checks if the lifecycle operation has completed
func (s *LifecycleService) IsLifecycleCompleted(ctx context.Context, tunnelName string) (bool, error) {
	syncStateName := GetSyncStateName(tunnelName)
	syncState, err := s.GetSyncState(ctx, LifecycleResourceType, syncStateName)
	if err != nil {
		return false, err
	}

	if syncState == nil {
		return false, nil
	}

	return syncState.Status.SyncStatus == v1alpha2.SyncStatusSynced, nil
}

// GetLifecycleError returns the error message if the lifecycle operation failed
func (s *LifecycleService) GetLifecycleError(ctx context.Context, tunnelName string) (string, error) {
	syncStateName := GetSyncStateName(tunnelName)
	syncState, err := s.GetSyncState(ctx, LifecycleResourceType, syncStateName)
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

// CleanupSyncState removes the SyncState for a tunnel after successful deletion
func (s *LifecycleService) CleanupSyncState(ctx context.Context, tunnelName string) error {
	syncStateName := GetSyncStateName(tunnelName)
	syncState, err := s.GetSyncState(ctx, LifecycleResourceType, syncStateName)
	if err != nil {
		return err
	}

	if syncState == nil {
		return nil
	}

	return s.Client.Delete(ctx, syncState)
}

// ParseLifecycleConfig parses the lifecycle configuration from raw JSON
func ParseLifecycleConfig(raw []byte) (*LifecycleConfig, error) {
	var config LifecycleConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, fmt.Errorf("unmarshal lifecycle config: %w", err)
	}
	return &config, nil
}
