// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package tunnelconfig provides types and utilities for managing tunnel configuration
// via ConfigMaps. This replaces the SyncState-based approach with a simpler,
// more robust ConfigMap-based aggregation system.
package tunnelconfig

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ConfigMapLabelTunnelID is the label key for tunnel ID.
	ConfigMapLabelTunnelID = "cloudflare-operator.io/tunnel-id"
	// ConfigMapLabelType is the label key for config type.
	ConfigMapLabelType = "cloudflare-operator.io/type"
	// ConfigMapTypeValue is the label value for tunnel config type.
	ConfigMapTypeValue = "tunnel-config"
	// ConfigDataKey is the key in ConfigMap.Data for the config JSON.
	ConfigDataKey = "config.json"

	// SourceKindTunnel represents a Tunnel source.
	SourceKindTunnel = "Tunnel"
	// SourceKindClusterTunnel represents a ClusterTunnel source.
	SourceKindClusterTunnel = "ClusterTunnel"
	// SourceKindIngress represents an Ingress source.
	SourceKindIngress = "Ingress"
	// SourceKindTunnelBinding represents a TunnelBinding source.
	SourceKindTunnelBinding = "TunnelBinding"
	// SourceKindHTTPRoute represents an HTTPRoute source.
	SourceKindHTTPRoute = "HTTPRoute"

	// PriorityTunnelSettings is the priority for Tunnel/ClusterTunnel settings (highest).
	PriorityTunnelSettings = 10
	// PriorityBinding is the priority for TunnelBinding rules.
	PriorityBinding = 50
	// PriorityIngress is the priority for Ingress rules.
	PriorityIngress = 100
	// PriorityGateway is the priority for Gateway API rules.
	PriorityGateway = 100
)

// TunnelConfig represents the aggregated tunnel configuration stored in a ConfigMap.
type TunnelConfig struct {
	// TunnelID is the Cloudflare tunnel ID.
	TunnelID string `json:"tunnelId"`

	// AccountID is the Cloudflare account ID.
	AccountID string `json:"accountId"`

	// TunnelName is the human-readable tunnel name.
	TunnelName string `json:"tunnelName,omitempty"`

	// WARPRouting contains WARP routing settings.
	WARPRouting *WARPRoutingConfig `json:"warpRouting,omitempty"`

	// Sources contains configuration from each source (Tunnel, Ingress, etc.).
	Sources map[string]*SourceConfig `json:"sources"`

	// LastHash is the hash of the last synced configuration.
	LastHash string `json:"lastHash,omitempty"`

	// SyncStatus is the current sync status.
	SyncStatus string `json:"syncStatus,omitempty"`

	// LastSyncTime is the last time the configuration was synced.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// CredentialsRef references the credentials to use for this tunnel.
	CredentialsRef *CredentialsRef `json:"credentialsRef,omitempty"`
}

// CredentialsRef references Cloudflare credentials.
type CredentialsRef struct {
	// Name is the CloudflareCredentials resource name.
	Name string `json:"name,omitempty"`
}

// WARPRoutingConfig contains WARP routing settings.
type WARPRoutingConfig struct {
	// Enabled indicates if WARP routing is enabled.
	Enabled bool `json:"enabled"`
}

// SourceConfig contains configuration from a single source.
type SourceConfig struct {
	// Kind is the source kind (Tunnel, Ingress, TunnelBinding, etc.).
	Kind string `json:"kind"`

	// Namespace is the source namespace (empty for cluster-scoped).
	Namespace string `json:"namespace,omitempty"`

	// Name is the source name.
	Name string `json:"name"`

	// Generation is the source's generation when this config was captured.
	Generation int64 `json:"generation,omitempty"`

	// Settings contains tunnel-level settings (from Tunnel/ClusterTunnel).
	Settings *TunnelSettings `json:"settings,omitempty"`

	// Rules contains ingress rules (from Ingress/TunnelBinding/HTTPRoute).
	Rules []IngressRule `json:"rules,omitempty"`

	// UpdatedAt is when this source was last updated.
	UpdatedAt *metav1.Time `json:"updatedAt,omitempty"`
}

// TunnelSettings contains tunnel-level settings.
type TunnelSettings struct {
	// WARPRouting indicates if WARP routing should be enabled.
	WARPRouting bool `json:"warpRouting,omitempty"`

	// OriginRequest contains default origin request settings.
	OriginRequest *OriginRequestConfig `json:"originRequest,omitempty"`
}

// OriginRequestConfig contains origin request settings.
type OriginRequestConfig struct {
	// ConnectTimeout is the timeout for connecting to the origin.
	ConnectTimeout string `json:"connectTimeout,omitempty"`

	// TLSTimeout is the timeout for TLS handshake.
	TLSTimeout string `json:"tlsTimeout,omitempty"`

	// TCPKeepAlive is the TCP keep-alive interval.
	TCPKeepAlive string `json:"tcpKeepAlive,omitempty"`

	// NoHappyEyeballs disables Happy Eyeballs.
	NoHappyEyeballs bool `json:"noHappyEyeballs,omitempty"`

	// KeepAliveConnections is the number of keep-alive connections.
	KeepAliveConnections int `json:"keepAliveConnections,omitempty"`

	// KeepAliveTimeout is the keep-alive timeout.
	KeepAliveTimeout string `json:"keepAliveTimeout,omitempty"`

	// HTTPHostHeader overrides the HTTP Host header.
	HTTPHostHeader string `json:"httpHostHeader,omitempty"`

	// OriginServerName is the TLS server name for the origin.
	OriginServerName string `json:"originServerName,omitempty"`

	// NoTLSVerify disables TLS verification.
	NoTLSVerify bool `json:"noTLSVerify,omitempty"`

	// DisableChunkedEncoding disables chunked transfer encoding.
	DisableChunkedEncoding bool `json:"disableChunkedEncoding,omitempty"`

	// BastionMode enables bastion mode.
	BastionMode bool `json:"bastionMode,omitempty"`

	// ProxyAddress is the address of the proxy.
	ProxyAddress string `json:"proxyAddress,omitempty"`

	// ProxyPort is the port of the proxy.
	ProxyPort int `json:"proxyPort,omitempty"`

	// ProxyType is the type of proxy.
	ProxyType string `json:"proxyType,omitempty"`

	// IPRules contains IP access rules.
	IPRules []IPRule `json:"ipRules,omitempty"`

	// HTTP2Origin enables HTTP/2 to the origin.
	HTTP2Origin bool `json:"http2Origin,omitempty"`

	// Access contains Access settings.
	Access *AccessConfig `json:"access,omitempty"`
}

// IPRule defines an IP access rule.
type IPRule struct {
	// Prefix is the IP prefix (CIDR).
	Prefix string `json:"prefix"`

	// Allow indicates if this prefix is allowed.
	Allow bool `json:"allow"`

	// Ports is the list of ports (optional).
	Ports []int `json:"ports,omitempty"`
}

// AccessConfig contains Cloudflare Access settings.
type AccessConfig struct {
	// Required indicates if Access is required.
	Required bool `json:"required,omitempty"`

	// TeamName is the Access team name.
	TeamName string `json:"teamName,omitempty"`

	// AudTag is the Access audience tag.
	AudTag []string `json:"audTag,omitempty"`
}

// IngressRule defines a tunnel ingress rule.
type IngressRule struct {
	// Hostname is the hostname to match.
	Hostname string `json:"hostname,omitempty"`

	// Path is the path to match (optional).
	Path string `json:"path,omitempty"`

	// Service is the backend service URL.
	Service string `json:"service"`

	// OriginRequest contains rule-specific origin request settings.
	OriginRequest *OriginRequestConfig `json:"originRequest,omitempty"`

	// Priority is the rule priority (lower = higher priority).
	Priority int `json:"priority,omitempty"`
}

// SourceKey returns a unique key for a source.
func SourceKey(kind, namespace, name string) string {
	if namespace == "" {
		return fmt.Sprintf("%s/%s", kind, name)
	}
	return fmt.Sprintf("%s/%s/%s", kind, namespace, name)
}

// GetSourceKey returns the source key for this config.
func (s *SourceConfig) GetSourceKey() string {
	return SourceKey(s.Kind, s.Namespace, s.Name)
}

// ComputeHash computes a hash of the configuration for change detection.
func (c *TunnelConfig) ComputeHash() string {
	// Create a copy without status fields
	copy := &TunnelConfig{
		TunnelID:       c.TunnelID,
		AccountID:      c.AccountID,
		TunnelName:     c.TunnelName,
		WARPRouting:    c.WARPRouting,
		Sources:        c.Sources,
		CredentialsRef: c.CredentialsRef,
	}

	data, _ := json.Marshal(copy)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash[:8])
}

// AggregateRules aggregates all rules from all sources into a single list.
// Rules are sorted by priority and then by hostname.
func (c *TunnelConfig) AggregateRules() []IngressRule {
	var allRules []IngressRule

	for _, source := range c.Sources {
		allRules = append(allRules, source.Rules...)
	}

	// Sort by priority (lower first), then by hostname
	sort.Slice(allRules, func(i, j int) bool {
		if allRules[i].Priority != allRules[j].Priority {
			return allRules[i].Priority < allRules[j].Priority
		}
		return allRules[i].Hostname < allRules[j].Hostname
	})

	return allRules
}

// IsWARPRoutingEnabled checks if any source has WARP routing enabled.
func (c *TunnelConfig) IsWARPRoutingEnabled() bool {
	if c.WARPRouting != nil && c.WARPRouting.Enabled {
		return true
	}

	for _, source := range c.Sources {
		if source.Settings != nil && source.Settings.WARPRouting {
			return true
		}
	}

	return false
}

// GetOriginRequestDefaults returns the default origin request settings from tunnel sources.
func (c *TunnelConfig) GetOriginRequestDefaults() *OriginRequestConfig {
	// Look for settings from Tunnel or ClusterTunnel sources
	for _, source := range c.Sources {
		if (source.Kind == SourceKindTunnel || source.Kind == SourceKindClusterTunnel) &&
			source.Settings != nil && source.Settings.OriginRequest != nil {
			return source.Settings.OriginRequest
		}
	}
	return nil
}

// ConfigMapName returns the ConfigMap name for a tunnel.
func ConfigMapName(tunnelID string) string {
	return fmt.Sprintf("tunnel-config-%s", tunnelID)
}

// ParseConfig parses a TunnelConfig from a ConfigMap.
func ParseConfig(cm *corev1.ConfigMap) (*TunnelConfig, error) {
	if cm == nil {
		return nil, fmt.Errorf("ConfigMap is nil")
	}

	data, ok := cm.Data[ConfigDataKey]
	if !ok {
		// Return empty config if no data
		return &TunnelConfig{
			Sources: make(map[string]*SourceConfig),
		}, nil
	}

	var config TunnelConfig
	if err := json.Unmarshal([]byte(data), &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Ensure Sources map is initialized
	if config.Sources == nil {
		config.Sources = make(map[string]*SourceConfig)
	}

	return &config, nil
}

// ToConfigMapData serializes the TunnelConfig to ConfigMap data.
func (c *TunnelConfig) ToConfigMapData() (map[string]string, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	return map[string]string{
		ConfigDataKey: string(data),
	}, nil
}

// NewConfigMap creates a new ConfigMap for a tunnel configuration.
func NewConfigMap(namespace, tunnelID string, owner metav1.Object, ownerGVK metav1.GroupVersionKind) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName(tunnelID),
			Namespace: namespace,
			Labels: map[string]string{
				ConfigMapLabelTunnelID: tunnelID,
				ConfigMapLabelType:     ConfigMapTypeValue,
			},
		},
		Data: map[string]string{},
	}

	// Set owner reference if provided
	if owner != nil {
		// Build API version string from group and version
		apiVersion := ownerGVK.Version
		if ownerGVK.Group != "" {
			apiVersion = ownerGVK.Group + "/" + ownerGVK.Version
		}
		cm.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         apiVersion,
				Kind:               ownerGVK.Kind,
				Name:               owner.GetName(),
				UID:                owner.GetUID(),
				Controller:         boolPtr(true),
				BlockOwnerDeletion: boolPtr(true),
			},
		}
	}

	return cm
}

func boolPtr(b bool) *bool {
	return &b
}
