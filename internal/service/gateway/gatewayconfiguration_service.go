// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package gateway

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// GatewayConfigurationService manages GatewayConfiguration via CloudflareSyncState.
type GatewayConfigurationService struct {
	*service.BaseService
}

// NewGatewayConfigurationService creates a new GatewayConfiguration service.
func NewGatewayConfigurationService(c client.Client) *GatewayConfigurationService {
	return &GatewayConfigurationService{
		BaseService: service.NewBaseService(c),
	}
}

// Register registers a GatewayConfiguration with the SyncState.
func (s *GatewayConfigurationService) Register(ctx context.Context, opts GatewayConfigurationRegisterOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"source", opts.Source.String(),
	)
	logger.V(1).Info("Registering GatewayConfiguration")

	// GatewayConfiguration is account-wide, use "account-config" as ID
	// In a multi-configuration scenario, each config would need a unique ID
	syncStateID := fmt.Sprintf("account-%s", opts.AccountID)
	if opts.AccountID == "" {
		syncStateID = fmt.Sprintf("pending-%s", opts.Source.Name)
	}

	// Get or create SyncState
	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceTypeGatewayConfiguration,
		syncStateID,
		opts.AccountID,
		"", // No zone ID for Gateway configuration
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get or create syncstate: %w", err)
	}

	// Update source in SyncState
	if err := s.UpdateSource(ctx, syncState, opts.Source, opts.Config, PriorityGatewayConfiguration); err != nil {
		return fmt.Errorf("update source: %w", err)
	}

	logger.Info("GatewayConfiguration registered", "syncStateId", syncState.Name)
	return nil
}

// Unregister removes a GatewayConfiguration from the SyncState.
//
//nolint:revive // cognitive complexity is acceptable for state lookup logic
func (s *GatewayConfigurationService) Unregister(ctx context.Context, accountID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"accountId", accountID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering GatewayConfiguration from SyncState")

	// Try multiple possible SyncState IDs
	syncStateIDs := []string{
		fmt.Sprintf("account-%s", accountID),
		fmt.Sprintf("pending-%s", source.Name),
	}

	for _, id := range syncStateIDs {
		if id == "" {
			continue
		}

		syncState, err := s.GetSyncState(ctx, ResourceTypeGatewayConfiguration, id)
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

		logger.Info("GatewayConfiguration unregistered from SyncState", "syncStateId", id)
		return nil
	}

	logger.V(1).Info("No SyncState found to unregister")
	return nil
}

// UpdateStatus updates the K8s GatewayConfiguration resource status based on sync result.
func (s *GatewayConfigurationService) UpdateStatus(
	ctx context.Context,
	config *v1alpha2.GatewayConfiguration,
	result *GatewayConfigurationSyncResult,
) error {
	config.Status.State = service.StateReady
	config.Status.AccountID = result.AccountID
	config.Status.ObservedGeneration = config.Generation

	return s.Client.Status().Update(ctx, config)
}
