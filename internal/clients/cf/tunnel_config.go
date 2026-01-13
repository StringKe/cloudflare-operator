// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"context"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
)

// GetTunnelConfiguration retrieves the Tunnel configuration from Cloudflare API.
// This returns the remotely-managed tunnel configuration including public hostnames.
func (c *API) GetTunnelConfiguration(tunnelID string) (*cloudflare.TunnelConfigurationResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error in getting account ID")
		return nil, err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	result, err := c.CloudflareClient.GetTunnelConfiguration(ctx, rc, tunnelID)
	if err != nil {
		c.Log.Error(err, "error getting tunnel configuration", "tunnelId", tunnelID)
		return nil, err
	}

	c.Log.V(1).Info("Got tunnel configuration", "tunnelId", tunnelID, "version", result.Version)
	return &result, nil
}

// UpdateTunnelConfiguration updates the Tunnel configuration in Cloudflare API.
// This syncs the local ingress rules to Cloudflare, making domains available
// for Access Applications validation.
func (c *API) UpdateTunnelConfiguration(tunnelID string, config cloudflare.TunnelConfiguration) (*cloudflare.TunnelConfigurationResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error in getting account ID")
		return nil, err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	params := cloudflare.TunnelConfigurationParams{
		TunnelID: tunnelID,
		Config:   config,
	}

	result, err := c.CloudflareClient.UpdateTunnelConfiguration(ctx, rc, params)
	if err != nil {
		c.Log.Error(err, "error updating tunnel configuration", "tunnelId", tunnelID)
		return nil, err
	}

	c.Log.Info("Tunnel configuration updated", "tunnelId", tunnelID, "version", result.Version, "ingressCount", len(config.Ingress))
	return &result, nil
}

// ConvertLocalRulesToSDK converts local UnvalidatedIngressRule to cloudflare-go SDK types.
// This is necessary because:
// - Local types use time.Duration for timeouts
// - SDK types use cloudflare.TunnelDuration
// - Local OriginRequestConfig is a value, SDK uses a pointer
func ConvertLocalRulesToSDK(localRules []UnvalidatedIngressRule) []cloudflare.UnvalidatedIngressRule {
	sdkRules := make([]cloudflare.UnvalidatedIngressRule, 0, len(localRules))

	for _, local := range localRules {
		sdkRule := cloudflare.UnvalidatedIngressRule{
			Hostname: local.Hostname,
			Path:     local.Path,
			Service:  local.Service,
		}

		// Convert OriginRequestConfig if it has any non-zero values
		if hasOriginRequest(local.OriginRequest) {
			sdkRule.OriginRequest = convertOriginRequest(local.OriginRequest)
		}

		sdkRules = append(sdkRules, sdkRule)
	}

	return sdkRules
}

// hasOriginRequest checks if OriginRequestConfig has any non-zero values
// nolint:revive // cyclomatic complexity is acceptable for simple field presence check
func hasOriginRequest(cfg OriginRequestConfig) bool {
	return cfg.ConnectTimeout != nil ||
		cfg.TLSTimeout != nil ||
		cfg.TCPKeepAlive != nil ||
		cfg.NoHappyEyeballs != nil ||
		cfg.KeepAliveConnections != nil ||
		cfg.KeepAliveTimeout != nil ||
		cfg.HTTPHostHeader != nil ||
		cfg.OriginServerName != nil ||
		cfg.CAPool != nil ||
		cfg.NoTLSVerify != nil ||
		cfg.Http2Origin != nil ||
		cfg.DisableChunkedEncoding != nil ||
		cfg.BastionMode != nil ||
		cfg.ProxyAddress != nil ||
		cfg.ProxyPort != nil ||
		cfg.ProxyType != nil ||
		len(cfg.IPRules) > 0
}

// convertOriginRequest converts local OriginRequestConfig to SDK type
// nolint:revive // cognitive complexity is acceptable for simple field mapping
func convertOriginRequest(local OriginRequestConfig) *cloudflare.OriginRequestConfig {
	sdk := &cloudflare.OriginRequestConfig{}

	// Convert duration fields (time.Duration -> TunnelDuration)
	if local.ConnectTimeout != nil {
		sdk.ConnectTimeout = &cloudflare.TunnelDuration{Duration: *local.ConnectTimeout}
	}
	if local.TLSTimeout != nil {
		sdk.TLSTimeout = &cloudflare.TunnelDuration{Duration: *local.TLSTimeout}
	}
	if local.TCPKeepAlive != nil {
		sdk.TCPKeepAlive = &cloudflare.TunnelDuration{Duration: *local.TCPKeepAlive}
	}
	if local.KeepAliveTimeout != nil {
		sdk.KeepAliveTimeout = &cloudflare.TunnelDuration{Duration: *local.KeepAliveTimeout}
	}

	// Copy pointer fields directly
	sdk.NoHappyEyeballs = local.NoHappyEyeballs
	sdk.KeepAliveConnections = local.KeepAliveConnections
	sdk.HTTPHostHeader = local.HTTPHostHeader
	sdk.OriginServerName = local.OriginServerName
	sdk.CAPool = local.CAPool
	sdk.NoTLSVerify = local.NoTLSVerify
	sdk.Http2Origin = local.Http2Origin
	sdk.DisableChunkedEncoding = local.DisableChunkedEncoding
	sdk.BastionMode = local.BastionMode
	sdk.ProxyAddress = local.ProxyAddress
	sdk.ProxyPort = local.ProxyPort
	sdk.ProxyType = local.ProxyType

	// Convert IP rules
	if len(local.IPRules) > 0 {
		sdk.IPRules = make([]cloudflare.IngressIPRule, 0, len(local.IPRules))
		for _, rule := range local.IPRules {
			sdkRule := cloudflare.IngressIPRule{
				Prefix: rule.Prefix,
				Allow:  rule.Allow,
			}
			if len(rule.Ports) > 0 {
				sdkRule.Ports = rule.Ports
			}
			sdk.IPRules = append(sdk.IPRules, sdkRule)
		}
	}

	return sdk
}

// SyncTunnelConfigurationToAPI syncs the local ingress rules to Cloudflare API.
// This is a convenience method that combines type conversion and API call.
func (c *API) SyncTunnelConfigurationToAPI(tunnelID string, localRules []UnvalidatedIngressRule, warpRouting *WarpRoutingConfig) error {
	// Convert local rules to SDK types
	sdkRules := ConvertLocalRulesToSDK(localRules)

	// Build the configuration
	config := cloudflare.TunnelConfiguration{
		Ingress: sdkRules,
	}

	// Add WarpRouting if configured
	if warpRouting != nil && warpRouting.Enabled {
		config.WarpRouting = &cloudflare.WarpRoutingConfig{
			Enabled: warpRouting.Enabled,
		}
	}

	// Update the configuration
	_, err := c.UpdateTunnelConfiguration(tunnelID, config)
	if err != nil {
		return fmt.Errorf("failed to sync tunnel configuration to API: %w", err)
	}

	return nil
}
