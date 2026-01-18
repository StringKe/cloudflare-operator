// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package warp

import (
	"context"
	"encoding/json"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

// ConnectorService provides operations for WARP connector lifecycle management.
// It manages SyncState resources for creating, updating, and deleting WARP connectors.
type ConnectorService struct {
	*service.BaseService
}

// NewConnectorService creates a new ConnectorService
func NewConnectorService(c client.Client) *ConnectorService {
	return &ConnectorService{
		BaseService: service.NewBaseService(c),
	}
}

// RequestCreate requests creation of a new WARP connector via SyncState
func (s *ConnectorService) RequestCreate(ctx context.Context, opts CreateConnectorOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"operation", "CreateConnector",
		"connectorName", opts.ConnectorName,
		"source", opts.Source.String(),
	)
	logger.Info("Requesting WARP connector creation")

	// Build lifecycle config
	routes := make([]RouteConfig, len(opts.Routes))
	copy(routes, opts.Routes)

	config := ConnectorLifecycleConfig{
		Action:           ConnectorActionCreate,
		ConnectorName:    opts.ConnectorName,
		VirtualNetworkID: opts.VirtualNetworkID,
		Routes:           routes,
	}

	syncStateName := fmt.Sprintf("warpconnector-%s", opts.ConnectorName)

	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ConnectorResourceType,
		syncStateName,
		opts.AccountID,
		"",
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get or create syncstate: %w", err)
	}

	// Create source
	source := service.Source{
		Kind:      opts.Source.Kind,
		Namespace: opts.Source.Namespace,
		Name:      opts.Source.Name,
	}

	// Update source with config and priority
	return s.UpdateSource(ctx, syncState, source, config, service.PriorityDefault)
}

// RequestDelete requests deletion of an existing WARP connector via SyncState
func (s *ConnectorService) RequestDelete(ctx context.Context, opts DeleteConnectorOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"operation", "DeleteConnector",
		"connectorName", opts.ConnectorName,
		"connectorId", opts.ConnectorID,
		"source", opts.Source.String(),
	)
	logger.Info("Requesting WARP connector deletion")

	// Build lifecycle config
	routes := make([]RouteConfig, len(opts.Routes))
	copy(routes, opts.Routes)

	config := ConnectorLifecycleConfig{
		Action:           ConnectorActionDelete,
		ConnectorName:    opts.ConnectorName,
		ConnectorID:      opts.ConnectorID,
		TunnelID:         opts.TunnelID,
		VirtualNetworkID: opts.VirtualNetworkID,
		Routes:           routes,
	}

	syncStateName := fmt.Sprintf("warpconnector-%s", opts.ConnectorName)

	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ConnectorResourceType,
		syncStateName,
		opts.AccountID,
		opts.ConnectorID,
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get or create syncstate: %w", err)
	}

	// Create source
	source := service.Source{
		Kind:      opts.Source.Kind,
		Namespace: opts.Source.Namespace,
		Name:      opts.Source.Name,
	}

	// Update source with delete action config
	return s.UpdateSource(ctx, syncState, source, config, service.PriorityDefault)
}

// RequestUpdate requests updating routes for an existing WARP connector via SyncState
func (s *ConnectorService) RequestUpdate(ctx context.Context, opts UpdateConnectorOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"operation", "UpdateConnector",
		"connectorName", opts.ConnectorName,
		"connectorId", opts.ConnectorID,
		"source", opts.Source.String(),
	)
	logger.Info("Requesting WARP connector routes update")

	// Build lifecycle config
	routes := make([]RouteConfig, len(opts.Routes))
	copy(routes, opts.Routes)

	config := ConnectorLifecycleConfig{
		Action:           ConnectorActionUpdate,
		ConnectorName:    opts.ConnectorName,
		ConnectorID:      opts.ConnectorID,
		TunnelID:         opts.TunnelID,
		VirtualNetworkID: opts.VirtualNetworkID,
		Routes:           routes,
	}

	syncStateName := fmt.Sprintf("warpconnector-%s", opts.ConnectorName)

	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ConnectorResourceType,
		syncStateName,
		opts.AccountID,
		opts.ConnectorID,
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get or create syncstate: %w", err)
	}

	// Create source
	source := service.Source{
		Kind:      opts.Source.Kind,
		Namespace: opts.Source.Namespace,
		Name:      opts.Source.Name,
	}

	// Update source
	return s.UpdateSource(ctx, syncState, source, config, service.PriorityDefault)
}

// Unregister removes the source from the SyncState
func (s *ConnectorService) Unregister(ctx context.Context, connectorName string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"operation", "UnregisterConnector",
		"connectorName", connectorName,
		"source", source.String(),
	)
	logger.Info("Unregistering WARP connector source")

	syncStateName := fmt.Sprintf("warpconnector-%s", connectorName)

	// Get the SyncState first
	syncState, err := s.BaseService.GetSyncState(ctx, ConnectorResourceType, syncStateName)
	if err != nil {
		// If not found, nothing to unregister
		if client.IgnoreNotFound(err) == nil {
			logger.V(1).Info("SyncState not found, nothing to unregister")
			return nil
		}
		return fmt.Errorf("get syncstate: %w", err)
	}

	// Remove the source
	return s.RemoveSource(ctx, syncState, source)
}

// GetSyncState returns the current SyncState for a WARP connector
func (s *ConnectorService) GetSyncState(ctx context.Context, connectorName string) (*v1alpha2.CloudflareSyncState, error) {
	syncStateName := fmt.Sprintf("warpconnector-%s", connectorName)
	return s.BaseService.GetSyncState(ctx, ConnectorResourceType, syncStateName)
}

// ParseLifecycleConfig parses the lifecycle configuration from raw JSON
func ParseLifecycleConfig(raw []byte) (*ConnectorLifecycleConfig, error) {
	var config ConnectorLifecycleConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, fmt.Errorf("unmarshal lifecycle config: %w", err)
	}
	return &config, nil
}
