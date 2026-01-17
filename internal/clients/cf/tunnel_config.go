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
		cfg.HTTP2Origin != nil ||
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
	sdk.Http2Origin = local.HTTP2Origin
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
//
// Deprecated: Use MergeAndSync instead to avoid race conditions between controllers.
//
// IMPORTANT: The warpRouting parameter controls WARP routing state:
// - nil: don't change existing warp-routing state (backward compatible)
// - &WarpRoutingConfig{Enabled: true}: explicitly enable warp-routing
// - &WarpRoutingConfig{Enabled: false}: explicitly disable warp-routing
func (c *API) SyncTunnelConfigurationToAPI(tunnelID string, localRules []UnvalidatedIngressRule, warpRouting *WarpRoutingConfig) error {
	// Convert local rules to SDK types
	sdkRules := ConvertLocalRulesToSDK(localRules)

	// Build the configuration
	config := cloudflare.TunnelConfiguration{
		Ingress: sdkRules,
	}

	// Add WarpRouting configuration if specified
	// When warpRouting is not nil, we explicitly set the state (true or false)
	// This allows disabling warp-routing by setting Enabled: false
	if warpRouting != nil {
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

// MergeOptions defines the options for merging tunnel configuration.
// Each controller provides its own configuration fragment, and MergeAndSync
// merges it with the existing remote configuration to avoid race conditions.
type MergeOptions struct {
	// Source identifies the controller/source of this configuration fragment.
	// Used for logging and debugging. Examples: "TunnelBinding/default/my-binding",
	// "Ingress/default/my-ingress", "Gateway/default/my-gateway", "Tunnel/my-tunnel"
	Source string

	// PreviousHostnames contains the hostnames that were previously synced by this source.
	// These will be removed from the remote configuration before adding CurrentRules.
	// This allows proper cleanup when a source's rules change.
	PreviousHostnames []string

	// CurrentRules contains the ingress rules to add to the configuration.
	// These rules will be merged with existing rules from other sources.
	// The last rule should be the catch-all rule (empty hostname with service).
	CurrentRules []UnvalidatedIngressRule

	// WarpRouting controls WARP routing state.
	// - nil: preserve existing warp-routing state (default)
	// - &WarpRoutingConfig{Enabled: true}: explicitly enable warp-routing
	// - &WarpRoutingConfig{Enabled: false}: explicitly disable warp-routing
	WarpRouting *WarpRoutingConfig

	// FallbackTarget is the service URL for the catch-all rule (e.g., "http_status:404").
	// - "": preserve existing fallback target
	// - non-empty: set/override the fallback target
	FallbackTarget string

	// GlobalOriginRequest is the global origin request configuration.
	// - nil: preserve existing global origin request config
	// - non-nil: set/override the global origin request config
	GlobalOriginRequest *OriginRequestConfig
}

// MergeAndSync performs read-merge-write operation to safely update tunnel configuration.
// This method:
// 1. Reads the current configuration from Cloudflare API
// 2. Removes rules owned by this source (based on PreviousHostnames)
// 3. Adds the new rules from CurrentRules
// 4. Preserves rules from other sources
// 5. Writes the merged configuration back to Cloudflare API
//
// This approach prevents race conditions where multiple controllers overwrite
// each other's configurations.
func (c *API) MergeAndSync(tunnelID string, opts MergeOptions) (*MergeSyncResult, error) {
	c.Log.Info("MergeAndSync: starting", "tunnelId", tunnelID, "source", opts.Source,
		"previousHostnames", opts.PreviousHostnames, "currentRulesCount", len(opts.CurrentRules))

	// Step 1: Read current configuration from Cloudflare
	currentConfig, err := c.GetTunnelConfiguration(tunnelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current tunnel configuration: %w", err)
	}

	// Step 2: Merge configuration
	mergedConfig := c.mergeConfiguration(currentConfig, opts)

	// Step 3: Write merged configuration back to Cloudflare
	result, err := c.UpdateTunnelConfiguration(tunnelID, mergedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to update tunnel configuration: %w", err)
	}

	// Build result with synced hostnames
	syncedHostnames := extractHostnames(opts.CurrentRules)
	c.Log.Info("MergeAndSync: completed", "tunnelId", tunnelID, "source", opts.Source,
		"version", result.Version, "syncedHostnames", syncedHostnames)

	return &MergeSyncResult{
		Version:         result.Version,
		SyncedHostnames: syncedHostnames,
	}, nil
}

// MergeSyncResult contains the result of a MergeAndSync operation.
type MergeSyncResult struct {
	// Version is the new configuration version after update.
	Version int `json:"version"`

	// SyncedHostnames contains all hostnames that were synced by this operation.
	// Controllers should store this in their Status for tracking.
	SyncedHostnames []string `json:"syncedHostnames"`
}

// mergeConfiguration merges the current remote configuration with the new options.
func (c *API) mergeConfiguration(current *cloudflare.TunnelConfigurationResult, opts MergeOptions) cloudflare.TunnelConfiguration {
	merged := cloudflare.TunnelConfiguration{}

	// Merge ingress rules
	merged.Ingress = c.mergeIngressRules(current, opts)

	// Merge WarpRouting: use new value if specified, otherwise preserve existing
	merged.WarpRouting = c.mergeWarpRouting(current, opts)

	// Merge OriginRequest: use new value if specified, otherwise preserve existing
	merged.OriginRequest = c.mergeGlobalOriginRequest(current, opts)

	return merged
}

// mergeIngressRules merges ingress rules from multiple sources.
// The algorithm:
// 1. Extract existing rules, excluding rules owned by this source (PreviousHostnames)
// 2. Add new rules from CurrentRules (excluding catch-all)
// 3. Ensure a catch-all rule exists at the end
//
//nolint:revive // cyclomatic complexity is acceptable for rule merging logic
func (c *API) mergeIngressRules(current *cloudflare.TunnelConfigurationResult, opts MergeOptions) []cloudflare.UnvalidatedIngressRule {
	// Build hostname removal set and extract existing fallback
	removeSet := c.buildRemoveSet(opts)
	existingFallback := extractExistingFallback(current)

	// Collect rules from other sources (not in removeSet, not catch-all)
	otherRules := c.collectOtherRules(current, removeSet)

	// Convert and add new rules (excluding catch-all)
	newRules, newFallback := c.convertNewRules(opts)

	// Combine: other rules + new rules + catch-all
	//nolint:gocritic // intentionally creating new slice to avoid modifying otherRules
	merged := make([]cloudflare.UnvalidatedIngressRule, 0, len(otherRules)+len(newRules)+1)
	merged = append(merged, otherRules...)
	merged = append(merged, newRules...)

	// Determine final fallback target and add catch-all rule
	finalFallback := determineFallback(opts.FallbackTarget, newFallback, existingFallback)
	merged = append(merged, cloudflare.UnvalidatedIngressRule{Service: finalFallback})

	c.Log.V(1).Info("mergeIngressRules: merged", "source", opts.Source,
		"otherRulesCount", len(otherRules), "newRulesCount", len(newRules),
		"totalRules", len(merged), "fallback", finalFallback)

	return merged
}

// buildRemoveSet builds a set of hostnames to remove (owned by this source).
func (*API) buildRemoveSet(opts MergeOptions) map[string]bool {
	removeSet := make(map[string]bool)
	for _, h := range opts.PreviousHostnames {
		removeSet[h] = true
	}
	for _, rule := range opts.CurrentRules {
		if rule.Hostname != "" {
			removeSet[rule.Hostname] = true
		}
	}
	return removeSet
}

// extractExistingFallback extracts the existing catch-all rule's service.
func extractExistingFallback(current *cloudflare.TunnelConfigurationResult) string {
	if current == nil || len(current.Config.Ingress) == 0 {
		return ""
	}
	lastRule := current.Config.Ingress[len(current.Config.Ingress)-1]
	if lastRule.Hostname == "" {
		return lastRule.Service
	}
	return ""
}

// collectOtherRules collects rules from other sources (not in removeSet, not catch-all).
func (*API) collectOtherRules(current *cloudflare.TunnelConfigurationResult, removeSet map[string]bool) []cloudflare.UnvalidatedIngressRule {
	if current == nil {
		return nil
	}
	otherRules := make([]cloudflare.UnvalidatedIngressRule, 0, len(current.Config.Ingress))
	for _, rule := range current.Config.Ingress {
		if rule.Hostname == "" || removeSet[rule.Hostname] {
			continue
		}
		otherRules = append(otherRules, rule)
	}
	return otherRules
}

// convertNewRules converts new rules from opts (excluding catch-all) and returns the catch-all service.
func (*API) convertNewRules(opts MergeOptions) ([]cloudflare.UnvalidatedIngressRule, string) {
	newRules := make([]cloudflare.UnvalidatedIngressRule, 0, len(opts.CurrentRules))
	var newFallback string
	for _, rule := range opts.CurrentRules {
		if rule.Hostname == "" {
			newFallback = rule.Service
			continue
		}
		newRules = append(newRules, convertLocalRuleToSDK(rule))
	}
	return newRules, newFallback
}

// determineFallback determines the final fallback target.
// Priority: explicit FallbackTarget > new catch-all from rules > existing fallback > default
func determineFallback(explicit, fromRules, existing string) string {
	if explicit != "" {
		return explicit
	}
	if fromRules != "" {
		return fromRules
	}
	if existing != "" {
		return existing
	}
	return "http_status:404"
}

// convertLocalRuleToSDK converts a single local rule to SDK type.
func convertLocalRuleToSDK(local UnvalidatedIngressRule) cloudflare.UnvalidatedIngressRule {
	sdkRule := cloudflare.UnvalidatedIngressRule{
		Hostname: local.Hostname,
		Path:     local.Path,
		Service:  local.Service,
	}

	if hasOriginRequest(local.OriginRequest) {
		sdkRule.OriginRequest = convertOriginRequest(local.OriginRequest)
	}

	return sdkRule
}

// extractHostnames extracts non-empty hostnames from rules.
func extractHostnames(rules []UnvalidatedIngressRule) []string {
	var hostnames []string
	for _, rule := range rules {
		if rule.Hostname != "" {
			hostnames = append(hostnames, rule.Hostname)
		}
	}
	return hostnames
}

// mergeWarpRouting merges WarpRouting configuration.
// If opts.WarpRouting is specified, use it; otherwise preserve existing.
func (*API) mergeWarpRouting(current *cloudflare.TunnelConfigurationResult, opts MergeOptions) *cloudflare.WarpRoutingConfig {
	if opts.WarpRouting != nil {
		return &cloudflare.WarpRoutingConfig{
			Enabled: opts.WarpRouting.Enabled,
		}
	}

	// Preserve existing
	if current != nil && current.Config.WarpRouting != nil {
		return &cloudflare.WarpRoutingConfig{
			Enabled: current.Config.WarpRouting.Enabled,
		}
	}

	return nil
}

// mergeGlobalOriginRequest merges global OriginRequest configuration.
// If opts.GlobalOriginRequest is specified, use it; otherwise preserve existing.
// Note: TunnelConfiguration.OriginRequest is a value type (not pointer) in cloudflare-go SDK.
func (*API) mergeGlobalOriginRequest(current *cloudflare.TunnelConfigurationResult, opts MergeOptions) cloudflare.OriginRequestConfig {
	if opts.GlobalOriginRequest != nil {
		converted := convertOriginRequest(*opts.GlobalOriginRequest)
		if converted != nil {
			return *converted
		}
	}

	// Preserve existing (OriginRequestConfig is a value type in cloudflare-go SDK)
	if current != nil {
		return current.Config.OriginRequest
	}

	return cloudflare.OriginRequestConfig{}
}
