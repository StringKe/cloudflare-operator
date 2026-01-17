// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package tunnel provides the TunnelConfigSyncController and aggregation logic
// for Cloudflare Tunnel configuration management.
package tunnel

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	tunnelsvc "github.com/StringKe/cloudflare-operator/internal/service/tunnel"
)

// AggregatedConfig represents the final merged Tunnel configuration.
// This is the format that will be sent to the Cloudflare API.
type AggregatedConfig struct {
	// WarpRouting controls WARP routing (from highest priority source)
	WarpRouting *tunnelsvc.WarpRoutingConfig `json:"warp-routing,omitempty"`
	// Ingress contains all merged ingress rules (sorted by specificity)
	Ingress []tunnelsvc.IngressRule `json:"ingress"`
	// OriginRequest contains global origin request settings
	OriginRequest *tunnelsvc.OriginRequestConfig `json:"originRequest,omitempty"`
}

// Aggregate merges all sources in a SyncState into a single AggregatedConfig.
// The algorithm:
// 1. Sort sources by priority (lower number = higher priority)
// 2. Apply settings from highest priority source
// 3. Collect all rules from all sources
// 4. Sort rules by specificity (more specific paths first)
// 5. Add catch-all rule at the end
//
//nolint:revive // cognitive complexity is acceptable for this core aggregation algorithm
func Aggregate(syncState *v1alpha2.CloudflareSyncState) (*AggregatedConfig, error) {
	result := &AggregatedConfig{
		Ingress: []tunnelsvc.IngressRule{},
	}

	// Sort sources by priority (lower number = higher priority)
	sources := make([]v1alpha2.ConfigSource, len(syncState.Spec.Sources))
	copy(sources, syncState.Spec.Sources)
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Priority < sources[j].Priority
	})

	var fallbackTarget string

	// Process each source
	for _, source := range sources {
		var config tunnelsvc.TunnelConfig
		if source.Config.Raw == nil {
			continue
		}
		if err := json.Unmarshal(source.Config.Raw, &config); err != nil {
			// Log warning but continue - don't fail entire aggregation for one bad source
			continue
		}

		// Apply settings from highest priority source (first one wins)
		if config.Settings != nil {
			if result.WarpRouting == nil && config.Settings.WarpRouting != nil {
				result.WarpRouting = config.Settings.WarpRouting
			}
			if result.OriginRequest == nil && config.Settings.GlobalOriginRequest != nil {
				result.OriginRequest = config.Settings.GlobalOriginRequest
			}
			if fallbackTarget == "" && config.Settings.FallbackTarget != "" {
				fallbackTarget = config.Settings.FallbackTarget
			}
		}

		// Collect rules from all sources
		result.Ingress = append(result.Ingress, config.Rules...)
	}

	// Sort rules by specificity:
	// 1. Rules with longer paths come first
	// 2. Rules with paths come before rules without paths
	// 3. Alphabetical order for same specificity
	sortRulesBySpecificity(result.Ingress)

	// Add catch-all rule at the end
	if fallbackTarget == "" {
		fallbackTarget = "http_status:404"
	}
	result.Ingress = append(result.Ingress, tunnelsvc.IngressRule{
		Service: fallbackTarget,
	})

	return result, nil
}

// sortRulesBySpecificity sorts rules so more specific ones come first.
// Cloudflare evaluates rules in order, so more specific rules must be first.
//
//nolint:revive // cognitive complexity is acceptable for this sorting logic
func sortRulesBySpecificity(rules []tunnelsvc.IngressRule) {
	sort.SliceStable(rules, func(i, j int) bool {
		// Rules with paths are more specific than rules without
		if rules[i].Path != "" && rules[j].Path == "" {
			return true
		}
		if rules[i].Path == "" && rules[j].Path != "" {
			return false
		}

		// Longer paths are more specific
		if len(rules[i].Path) != len(rules[j].Path) {
			return len(rules[i].Path) > len(rules[j].Path)
		}

		// Same path length - sort by hostname for deterministic order
		if rules[i].Hostname != rules[j].Hostname {
			return rules[i].Hostname < rules[j].Hostname
		}

		// Same hostname - sort by path
		return rules[i].Path < rules[j].Path
	})
}

// ExtractHostnames extracts all unique hostnames from the aggregated config.
// This is useful for DNS record management and status reporting.
func ExtractHostnames(config *AggregatedConfig) []string {
	if config == nil {
		return nil
	}

	seen := make(map[string]bool)
	var hostnames []string

	for _, rule := range config.Ingress {
		if rule.Hostname != "" && !seen[rule.Hostname] {
			seen[rule.Hostname] = true
			hostnames = append(hostnames, rule.Hostname)
		}
	}

	// Sort for deterministic output
	sort.Strings(hostnames)
	return hostnames
}

// GetSourceCount returns the number of sources in the SyncState.
func GetSourceCount(syncState *v1alpha2.CloudflareSyncState) int {
	if syncState == nil {
		return 0
	}
	return len(syncState.Spec.Sources)
}

// GetRuleCount returns the total number of rules across all sources.
func GetRuleCount(syncState *v1alpha2.CloudflareSyncState) int {
	if syncState == nil {
		return 0
	}

	count := 0
	for _, source := range syncState.Spec.Sources {
		var config tunnelsvc.TunnelConfig
		if source.Config.Raw == nil {
			continue
		}
		if err := json.Unmarshal(source.Config.Raw, &config); err != nil {
			continue
		}
		count += len(config.Rules)
	}
	return count
}

// ValidateAggregatedConfig validates the aggregated configuration.
// Returns an error if the configuration is invalid.
func ValidateAggregatedConfig(config *AggregatedConfig) error {
	if config == nil {
		return errors.New("config is nil")
	}

	// Must have at least one rule (the catch-all)
	if len(config.Ingress) == 0 {
		return errors.New("ingress rules are empty")
	}

	// Last rule must be catch-all (no hostname)
	lastRule := config.Ingress[len(config.Ingress)-1]
	if lastRule.Hostname != "" {
		return errors.New("last ingress rule must be catch-all (no hostname)")
	}

	// Catch-all must have a service
	if lastRule.Service == "" {
		return errors.New("catch-all rule must have a service")
	}

	// All non-catch-all rules must have hostname or service
	for i, rule := range config.Ingress[:len(config.Ingress)-1] {
		if rule.Service == "" {
			return fmt.Errorf("rule %d has no service", i)
		}
	}

	return nil
}
