// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package device

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// DeviceSettingsPolicyService manages DeviceSettingsPolicy configurations via CloudflareSyncState.
type DeviceSettingsPolicyService struct {
	*service.BaseService
}

// NewDeviceSettingsPolicyService creates a new DeviceSettingsPolicy service.
func NewDeviceSettingsPolicyService(c client.Client) *DeviceSettingsPolicyService {
	return &DeviceSettingsPolicyService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers a DeviceSettingsPolicy configuration with the SyncState.
func (s *DeviceSettingsPolicyService) Register(ctx context.Context, opts DeviceSettingsPolicyRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"source", opts.Source.String(),
	)
	logger.V(1).Info("Registering DeviceSettingsPolicy configuration")

	// DeviceSettingsPolicy is account-wide, use "account-settings" as ID
	syncStateID := fmt.Sprintf("account-%s", opts.AccountID)
	if opts.AccountID == "" {
		syncStateID = fmt.Sprintf("pending-%s", opts.Source.Name)
	}

	// Get or create SyncState
	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeDeviceSettingsPolicy,
		syncStateID,
		opts.AccountID,
		"", // No zone ID for Device resources
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get or create syncstate: %w", err)
	}

	// Update source in SyncState
	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityDeviceSettingsPolicy); err != nil {
		return fmt.Errorf("update source: %w", err)
	}

	logger.Info("DeviceSettingsPolicy configuration registered", "syncStateId", syncState.Name)
	return nil
}

// Unregister removes a DeviceSettingsPolicy configuration from the SyncState.
//
//nolint:revive // cognitive complexity is acceptable for state lookup logic
func (s *DeviceSettingsPolicyService) Unregister(ctx context.Context, accountID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"accountId", accountID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering DeviceSettingsPolicy from SyncState")

	// Try multiple possible SyncState IDs
	syncStateIDs := []string{
		fmt.Sprintf("account-%s", accountID),
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypeDeviceSettingsPolicy, id)
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

		logger.Info("DeviceSettingsPolicy unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// UpdateStatus updates the K8s DeviceSettingsPolicy resource status based on sync result.
func (s *DeviceSettingsPolicyService) UpdateStatus(
	ctx context.Context,
	policy *v1alpha2.DeviceSettingsPolicy,
	result *DeviceSettingsPolicySyncResult,
) error {
	policy.Status.State = "active"
	policy.Status.AccountID = result.AccountID
	policy.Status.SplitTunnelExcludeCount = result.SplitTunnelExcludeCount
	policy.Status.SplitTunnelIncludeCount = result.SplitTunnelIncludeCount
	policy.Status.FallbackDomainsCount = result.FallbackDomainsCount
	policy.Status.AutoPopulatedRoutesCount = result.AutoPopulatedRoutesCount
	policy.Status.ObservedGeneration = policy.Generation

	return s.Client.Status().Update(ctx, policy)
}
