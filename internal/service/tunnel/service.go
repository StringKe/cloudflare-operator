// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package tunnel provides the TunnelConfigService for managing Cloudflare Tunnel configuration.
// It aggregates configuration from multiple sources (Tunnel, ClusterTunnel, TunnelBinding,
// Ingress, Gateway) into a single CloudflareSyncState for synchronized updates.
package tunnel

import (
	"context"
	"errors"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/service"
)

const (
	// ResourceType is the SyncState resource type for tunnel configuration
	ResourceType = v1alpha2.SyncResourceTunnelConfiguration

	// PriorityTunnelSettings is the priority for Tunnel/ClusterTunnel settings (highest)
	PriorityTunnelSettings = 10

	// PriorityBinding is the priority for TunnelBinding rules
	PriorityBinding = 50

	// PriorityIngress is the priority for Ingress rules
	PriorityIngress = 100

	// PriorityGateway is the priority for Gateway API rules
	PriorityGateway = 100
)

// Service handles Tunnel configuration registration.
// It implements the ConfigService interface for TunnelConfiguration resources.
type Service struct {
	*service.BaseService
}

// NewService creates a new TunnelConfigService
func NewService(c client.Client) *Service {
	return &Service{
		BaseService: service.NewBaseService(c),
	}
}

// RegisterSettings registers tunnel settings from a Tunnel or ClusterTunnel controller.
// Settings include warp routing, fallback target, and global origin request config.
// These have the highest priority and override settings from other sources.
func (s *Service) RegisterSettings(ctx context.Context, opts RegisterSettingsOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"tunnelID", opts.TunnelID,
		"source", opts.Source.String(),
	)
	logger.V(1).Info("Registering tunnel settings")

	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceType,
		opts.TunnelID,
		opts.AccountID,
		"", // No zone ID for tunnel configuration
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get/create syncstate for tunnel %s: %w", opts.TunnelID, err)
	}

	config := TunnelConfig{
		Settings: &opts.Settings,
	}

	if err := s.UpdateSource(ctx, syncState, opts.Source, config, PriorityTunnelSettings); err != nil {
		return fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("Tunnel settings registered successfully")
	return nil
}

// RegisterRules registers ingress rules from an Ingress, TunnelBinding, or Gateway controller.
// Rules from multiple sources are aggregated by the SyncController before syncing to Cloudflare.
func (s *Service) RegisterRules(ctx context.Context, opts RegisterRulesOptions) error {
	logger := log.FromContext(ctx).WithValues(
		"tunnelID", opts.TunnelID,
		"source", opts.Source.String(),
		"ruleCount", len(opts.Rules),
	)
	logger.V(1).Info("Registering ingress rules")

	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceType,
		opts.TunnelID,
		opts.AccountID,
		"", // No zone ID for tunnel configuration
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get/create syncstate for tunnel %s: %w", opts.TunnelID, err)
	}

	config := TunnelConfig{
		Rules: opts.Rules,
	}

	// Use provided priority or default
	priority := opts.Priority
	if priority == 0 {
		priority = service.PriorityDefault
	}

	if err := s.UpdateSource(ctx, syncState, opts.Source, config, priority); err != nil {
		return fmt.Errorf("update source in syncstate: %w", err)
	}

	logger.Info("Ingress rules registered successfully",
		"hostnames", extractHostnames(opts.Rules))
	return nil
}

// Unregister removes a source's configuration from the SyncState.
// This is called when the source K8s resource is deleted.
// If no sources remain, the SyncState is also deleted.
func (s *Service) Unregister(ctx context.Context, tunnelID string, source service.Source) error {
	logger := log.FromContext(ctx).WithValues(
		"tunnelID", tunnelID,
		"source", source.String(),
	)
	logger.V(1).Info("Unregistering source from tunnel configuration")

	syncState, err := s.GetSyncState(ctx, ResourceType, tunnelID)
	if err != nil {
		return fmt.Errorf("get syncstate for tunnel %s: %w", tunnelID, err)
	}

	if syncState == nil {
		logger.V(1).Info("SyncState not found, nothing to unregister")
		return nil
	}

	if err := s.RemoveSource(ctx, syncState, source); err != nil {
		return fmt.Errorf("remove source from syncstate: %w", err)
	}

	logger.Info("Source unregistered from tunnel configuration")
	return nil
}

// Register implements the ConfigService interface.
// It routes to RegisterSettings or RegisterRules based on the config type.
func (s *Service) Register(ctx context.Context, opts service.RegisterOptions) error {
	config, ok := opts.Config.(TunnelConfig)
	if !ok {
		return errors.New("invalid config type: expected TunnelConfig")
	}

	syncState, err := s.GetOrCreateSyncState(
		ctx,
		ResourceType,
		opts.CloudflareID,
		opts.AccountID,
		opts.ZoneID,
		opts.CredentialsRef,
	)
	if err != nil {
		return fmt.Errorf("get/create syncstate: %w", err)
	}

	return s.UpdateSource(ctx, syncState, opts.Source, config, opts.Priority)
}

// Unregister implements the ConfigService interface.
func (s *Service) UnregisterConfig(ctx context.Context, opts service.UnregisterOptions) error {
	return s.Unregister(ctx, opts.CloudflareID, opts.Source)
}

// extractHostnames extracts unique hostnames from ingress rules
func extractHostnames(rules []IngressRule) []string {
	seen := make(map[string]bool)
	var hostnames []string

	for _, rule := range rules {
		if rule.Hostname != "" && !seen[rule.Hostname] {
			seen[rule.Hostname] = true
			hostnames = append(hostnames, rule.Hostname)
		}
	}

	return hostnames
}
