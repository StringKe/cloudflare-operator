// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package configmap provides utilities for managing cloudflared ConfigMaps
// and triggering Deployment restarts when configuration changes.
package configmap

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

const (
	// ConfigKeyName is the key in the ConfigMap data for the cloudflared config
	ConfigKeyName = "config.yaml"
	// ChecksumAnnotation is the annotation key used to track config changes
	ChecksumAnnotation = "cloudflare-operator.io/checksum"
)

// Manager handles ConfigMap operations for cloudflared tunnels.
type Manager struct {
	client.Client
}

// NewManager creates a new ConfigMap manager.
func NewManager(c client.Client) *Manager {
	return &Manager{Client: c}
}

// TunnelInfo contains the basic tunnel information needed for ConfigMap operations.
type TunnelInfo interface {
	GetName() string
	GetNamespace() string
}

// UpdateIngressRules updates the cloudflared ConfigMap with new ingress rules.
// Returns true if the ConfigMap was actually updated, false if unchanged.
// nolint:revive // Cognitive complexity for ConfigMap update logic
func (m *Manager) UpdateIngressRules(
	ctx context.Context,
	tunnel TunnelInfo,
	rules []cf.UnvalidatedIngressRule,
) (bool, error) {
	logger := log.FromContext(ctx)

	// Get ConfigMap
	cm := &corev1.ConfigMap{}
	if err := m.Get(ctx, apitypes.NamespacedName{
		Name:      tunnel.GetName(),
		Namespace: tunnel.GetNamespace(),
	}, cm); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("ConfigMap not found, tunnel may not be ready yet", "tunnel", tunnel.GetName())
			return false, fmt.Errorf("ConfigMap %s/%s not found, tunnel may not be ready", tunnel.GetNamespace(), tunnel.GetName())
		}
		return false, err
	}

	// Parse existing config
	existingConfig := &cf.Configuration{}
	if configStr, ok := cm.Data[ConfigKeyName]; ok {
		if err := yaml.Unmarshal([]byte(configStr), existingConfig); err != nil {
			return false, fmt.Errorf("failed to parse existing config: %w", err)
		}
	}

	// Update ingress rules
	existingConfig.Ingress = rules

	// Marshal back to YAML
	configBytes, err := yaml.Marshal(existingConfig)
	if err != nil {
		return false, fmt.Errorf("failed to marshal config: %w", err)
	}

	configStr := string(configBytes)

	// Check if config actually changed
	if cm.Data[ConfigKeyName] == configStr {
		logger.Info("ConfigMap unchanged, skipping update")
		return false, nil
	}

	// Update ConfigMap with retry
	if err := controller.UpdateWithConflictRetry(ctx, m.Client, cm, func() {
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
		cm.Data[ConfigKeyName] = configStr
	}); err != nil {
		return false, fmt.Errorf("failed to update ConfigMap: %w", err)
	}

	logger.Info("ConfigMap updated", "tunnel", tunnel.GetName())
	return true, nil
}

// TriggerDeploymentRestart updates the Deployment annotation to trigger a rolling restart.
// This is typically called after updating the ConfigMap to ensure pods pick up the new config.
func (m *Manager) TriggerDeploymentRestart(ctx context.Context, tunnel TunnelInfo, configStr string) error {
	logger := log.FromContext(ctx)

	// Calculate config checksum
	checksum := CalculateChecksum(configStr)

	// Get Deployment
	deployment := &appsv1.Deployment{}
	if err := m.Get(ctx, apitypes.NamespacedName{
		Name:      tunnel.GetName(),
		Namespace: tunnel.GetNamespace(),
	}, deployment); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Deployment not found, skipping restart trigger")
			return nil
		}
		return err
	}

	// Check if checksum is already set
	if deployment.Spec.Template.Annotations != nil {
		if deployment.Spec.Template.Annotations[ChecksumAnnotation] == checksum {
			logger.Info("Deployment checksum unchanged, skipping restart")
			return nil
		}
	}

	// Update Deployment annotation
	if err := controller.UpdateWithConflictRetry(ctx, m.Client, deployment, func() {
		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = make(map[string]string)
		}
		deployment.Spec.Template.Annotations[ChecksumAnnotation] = checksum
	}); err != nil {
		return err
	}

	logger.Info("Deployment restart triggered", "tunnel", tunnel.GetName(), "checksum", checksum)
	return nil
}

// UpdateAndRestart updates the ConfigMap and triggers a Deployment restart if changed.
// This is a convenience method that combines UpdateIngressRules and TriggerDeploymentRestart.
// nolint:revive // Cognitive complexity for ConfigMap update and restart logic
func (m *Manager) UpdateAndRestart(
	ctx context.Context,
	tunnel TunnelInfo,
	rules []cf.UnvalidatedIngressRule,
) error {
	logger := log.FromContext(ctx)

	// Get ConfigMap
	cm := &corev1.ConfigMap{}
	if err := m.Get(ctx, apitypes.NamespacedName{
		Name:      tunnel.GetName(),
		Namespace: tunnel.GetNamespace(),
	}, cm); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("ConfigMap not found, tunnel may not be ready yet", "tunnel", tunnel.GetName())
			return fmt.Errorf("ConfigMap %s/%s not found, tunnel may not be ready", tunnel.GetNamespace(), tunnel.GetName())
		}
		return err
	}

	// Parse existing config
	existingConfig := &cf.Configuration{}
	if configStr, ok := cm.Data[ConfigKeyName]; ok {
		if err := yaml.Unmarshal([]byte(configStr), existingConfig); err != nil {
			return fmt.Errorf("failed to parse existing config: %w", err)
		}
	}

	// Update ingress rules
	existingConfig.Ingress = rules

	// Marshal back to YAML
	configBytes, err := yaml.Marshal(existingConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configStr := string(configBytes)

	// Check if config actually changed
	if cm.Data[ConfigKeyName] == configStr {
		logger.Info("ConfigMap unchanged, skipping update")
		return nil
	}

	// Update ConfigMap with retry
	if err := controller.UpdateWithConflictRetry(ctx, m.Client, cm, func() {
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
		cm.Data[ConfigKeyName] = configStr
	}); err != nil {
		return fmt.Errorf("failed to update ConfigMap: %w", err)
	}

	logger.Info("ConfigMap updated", "tunnel", tunnel.GetName())

	// Trigger Deployment restart
	if err := m.TriggerDeploymentRestart(ctx, tunnel, configStr); err != nil {
		logger.Error(err, "Failed to trigger deployment restart")
		// Don't fail - ConfigMap is updated, pod will eventually pick up changes
	}

	return nil
}

// CalculateChecksum calculates MD5 checksum of the config string.
func CalculateChecksum(configStr string) string {
	hash := md5.Sum([]byte(configStr))
	return hex.EncodeToString(hash[:])
}

// GetCurrentConfig retrieves the current cloudflared configuration from ConfigMap.
func (m *Manager) GetCurrentConfig(ctx context.Context, tunnel TunnelInfo) (*cf.Configuration, error) {
	cm := &corev1.ConfigMap{}
	if err := m.Get(ctx, apitypes.NamespacedName{
		Name:      tunnel.GetName(),
		Namespace: tunnel.GetNamespace(),
	}, cm); err != nil {
		return nil, err
	}

	config := &cf.Configuration{}
	if configStr, ok := cm.Data[ConfigKeyName]; ok {
		if err := yaml.Unmarshal([]byte(configStr), config); err != nil {
			return nil, fmt.Errorf("failed to parse config: %w", err)
		}
	}

	return config, nil
}
