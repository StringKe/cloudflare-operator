// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package tunnelconfig

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Writer provides methods for writing to tunnel configuration ConfigMaps.
type Writer struct {
	client            client.Client
	operatorNamespace string
}

// NewWriter creates a new Writer.
func NewWriter(c client.Client, operatorNamespace string) *Writer {
	return &Writer{
		client:            c,
		operatorNamespace: operatorNamespace,
	}
}

// WriteSourceConfig writes a source configuration to the tunnel's ConfigMap.
// If the ConfigMap doesn't exist, it will be created.
func (w *Writer) WriteSourceConfig(
	ctx context.Context,
	tunnelID string,
	accountID string,
	source *SourceConfig,
	owner metav1.Object,
	ownerGVK metav1.GroupVersionKind,
) error {
	logger := log.FromContext(ctx)
	sourceKey := source.GetSourceKey()

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Get or create ConfigMap
		cm, config, err := w.getOrCreateConfigMap(ctx, tunnelID, accountID, owner, ownerGVK)
		if err != nil {
			return err
		}

		// Update source
		now := metav1.Now()
		source.UpdatedAt = &now
		config.Sources[sourceKey] = source

		// Ensure account ID is set
		if config.AccountID == "" && accountID != "" {
			config.AccountID = accountID
		}

		// Serialize and update
		data, err := config.ToConfigMapData()
		if err != nil {
			return fmt.Errorf("failed to serialize config: %w", err)
		}

		cm.Data = data

		if cm.ResourceVersion == "" {
			// Create new ConfigMap
			logger.Info("Creating tunnel config ConfigMap",
				"configMap", cm.Name,
				"tunnelId", tunnelID,
				"source", sourceKey)
			return w.client.Create(ctx, cm)
		}

		// Update existing ConfigMap
		logger.V(1).Info("Updating tunnel config ConfigMap",
			"configMap", cm.Name,
			"tunnelId", tunnelID,
			"source", sourceKey)
		return w.client.Update(ctx, cm)
	})
}

// RemoveSourceConfig removes a source configuration from the tunnel's ConfigMap.
func (w *Writer) RemoveSourceConfig(
	ctx context.Context,
	tunnelID string,
	sourceKey string,
) error {
	logger := log.FromContext(ctx)

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Get ConfigMap
		cm := &corev1.ConfigMap{}
		err := w.client.Get(ctx, types.NamespacedName{
			Name:      ConfigMapName(tunnelID),
			Namespace: w.operatorNamespace,
		}, cm)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// ConfigMap doesn't exist, nothing to remove
				return nil
			}
			return err
		}

		// Parse config
		config, err := ParseConfig(cm)
		if err != nil {
			return err
		}

		// Remove source
		if _, exists := config.Sources[sourceKey]; !exists {
			// Source doesn't exist, nothing to remove
			return nil
		}

		delete(config.Sources, sourceKey)

		logger.Info("Removing source from tunnel config",
			"configMap", cm.Name,
			"tunnelId", tunnelID,
			"source", sourceKey,
			"remainingSources", len(config.Sources))

		// Serialize and update
		data, err := config.ToConfigMapData()
		if err != nil {
			return fmt.Errorf("failed to serialize config: %w", err)
		}

		cm.Data = data
		return w.client.Update(ctx, cm)
	})
}

// SetTunnelSettings sets tunnel-level settings (from Tunnel/ClusterTunnel).
func (w *Writer) SetTunnelSettings(
	ctx context.Context,
	tunnelID string,
	accountID string,
	tunnelName string,
	settings *TunnelSettings,
	credentialsRef *CredentialsRef,
	source *SourceConfig,
	owner metav1.Object,
	ownerGVK metav1.GroupVersionKind,
) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Get or create ConfigMap
		cm, config, err := w.getOrCreateConfigMap(ctx, tunnelID, accountID, owner, ownerGVK)
		if err != nil {
			return err
		}

		// Update tunnel-level settings
		config.TunnelID = tunnelID
		config.AccountID = accountID
		config.TunnelName = tunnelName
		config.CredentialsRef = credentialsRef

		// Update WARP routing from settings
		if settings != nil && settings.WARPRouting {
			config.WARPRouting = &WARPRoutingConfig{Enabled: true}
		}

		// Update source
		if source != nil {
			now := metav1.Now()
			source.UpdatedAt = &now
			source.Settings = settings
			config.Sources[source.GetSourceKey()] = source
		}

		// Serialize and update
		data, err := config.ToConfigMapData()
		if err != nil {
			return fmt.Errorf("failed to serialize config: %w", err)
		}

		cm.Data = data

		if cm.ResourceVersion == "" {
			return w.client.Create(ctx, cm)
		}
		return w.client.Update(ctx, cm)
	})
}

// getOrCreateConfigMap gets an existing ConfigMap or creates a new one.
func (w *Writer) getOrCreateConfigMap(
	ctx context.Context,
	tunnelID string,
	accountID string,
	owner metav1.Object,
	ownerGVK metav1.GroupVersionKind,
) (*corev1.ConfigMap, *TunnelConfig, error) {
	cmName := ConfigMapName(tunnelID)

	// Try to get existing ConfigMap
	cm := &corev1.ConfigMap{}
	err := w.client.Get(ctx, types.NamespacedName{
		Name:      cmName,
		Namespace: w.operatorNamespace,
	}, cm)

	if err == nil {
		// Parse existing config
		config, parseErr := ParseConfig(cm)
		if parseErr != nil {
			return nil, nil, parseErr
		}
		return cm, config, nil
	}

	if !apierrors.IsNotFound(err) {
		return nil, nil, err
	}

	// Create new ConfigMap
	cm = NewConfigMap(w.operatorNamespace, tunnelID, owner, ownerGVK)

	// Initialize config
	config := &TunnelConfig{
		TunnelID:  tunnelID,
		AccountID: accountID,
		Sources:   make(map[string]*SourceConfig),
	}

	return cm, config, nil
}

// GetTunnelConfig gets the current tunnel configuration.
func (w *Writer) GetTunnelConfig(ctx context.Context, tunnelID string) (*TunnelConfig, error) {
	cm := &corev1.ConfigMap{}
	err := w.client.Get(ctx, types.NamespacedName{
		Name:      ConfigMapName(tunnelID),
		Namespace: w.operatorNamespace,
	}, cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return ParseConfig(cm)
}

// DeleteConfigMap deletes the tunnel configuration ConfigMap.
func (w *Writer) DeleteConfigMap(ctx context.Context, tunnelID string) error {
	cm := &corev1.ConfigMap{}
	err := w.client.Get(ctx, types.NamespacedName{
		Name:      ConfigMapName(tunnelID),
		Namespace: w.operatorNamespace,
	}, cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return w.client.Delete(ctx, cm)
}
