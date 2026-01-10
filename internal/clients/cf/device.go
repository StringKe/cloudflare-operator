// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"context"

	"github.com/cloudflare/cloudflare-go"
)

// SplitTunnelEntry represents a split tunnel configuration entry.
type SplitTunnelEntry struct {
	Address     string `json:"address,omitempty"`
	Host        string `json:"host,omitempty"`
	Description string `json:"description,omitempty"`
}

// FallbackDomainEntry represents a fallback domain configuration entry.
type FallbackDomainEntry struct {
	Suffix      string   `json:"suffix"`
	Description string   `json:"description,omitempty"`
	DNSServer   []string `json:"dns_server,omitempty"`
}

// GetSplitTunnelExclude retrieves the current split tunnel exclude list.
func (c *API) GetSplitTunnelExclude() ([]SplitTunnelEntry, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()

	routes, err := c.CloudflareClient.ListSplitTunnels(ctx, c.ValidAccountId, "exclude")
	if err != nil {
		c.Log.Error(err, "error listing split tunnel exclude entries")
		return nil, err
	}

	result := make([]SplitTunnelEntry, len(routes))
	for i, r := range routes {
		result[i] = SplitTunnelEntry{
			Address:     r.Address,
			Host:        r.Host,
			Description: r.Description,
		}
	}

	return result, nil
}

// UpdateSplitTunnelExclude updates the split tunnel exclude list.
func (c *API) UpdateSplitTunnelExclude(entries []SplitTunnelEntry) error {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	ctx := context.Background()

	tunnels := make([]cloudflare.SplitTunnel, len(entries))
	for i, e := range entries {
		tunnels[i] = cloudflare.SplitTunnel{
			Address:     e.Address,
			Host:        e.Host,
			Description: e.Description,
		}
	}

	_, err := c.CloudflareClient.UpdateSplitTunnel(ctx, c.ValidAccountId, "exclude", tunnels)
	if err != nil {
		c.Log.Error(err, "error updating split tunnel exclude entries")
		return err
	}

	c.Log.Info("Split tunnel exclude list updated", "count", len(entries))
	return nil
}

// GetSplitTunnelInclude retrieves the current split tunnel include list.
func (c *API) GetSplitTunnelInclude() ([]SplitTunnelEntry, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()

	routes, err := c.CloudflareClient.ListSplitTunnels(ctx, c.ValidAccountId, "include")
	if err != nil {
		c.Log.Error(err, "error listing split tunnel include entries")
		return nil, err
	}

	result := make([]SplitTunnelEntry, len(routes))
	for i, r := range routes {
		result[i] = SplitTunnelEntry{
			Address:     r.Address,
			Host:        r.Host,
			Description: r.Description,
		}
	}

	return result, nil
}

// UpdateSplitTunnelInclude updates the split tunnel include list.
func (c *API) UpdateSplitTunnelInclude(entries []SplitTunnelEntry) error {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	ctx := context.Background()

	tunnels := make([]cloudflare.SplitTunnel, len(entries))
	for i, e := range entries {
		tunnels[i] = cloudflare.SplitTunnel{
			Address:     e.Address,
			Host:        e.Host,
			Description: e.Description,
		}
	}

	_, err := c.CloudflareClient.UpdateSplitTunnel(ctx, c.ValidAccountId, "include", tunnels)
	if err != nil {
		c.Log.Error(err, "error updating split tunnel include entries")
		return err
	}

	c.Log.Info("Split tunnel include list updated", "count", len(entries))
	return nil
}

// GetFallbackDomains retrieves the current fallback domains list.
func (c *API) GetFallbackDomains() ([]FallbackDomainEntry, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()

	domains, err := c.CloudflareClient.ListFallbackDomains(ctx, c.ValidAccountId)
	if err != nil {
		c.Log.Error(err, "error listing fallback domains")
		return nil, err
	}

	result := make([]FallbackDomainEntry, len(domains))
	for i, d := range domains {
		result[i] = FallbackDomainEntry{
			Suffix:      d.Suffix,
			Description: d.Description,
			DNSServer:   d.DNSServer,
		}
	}

	return result, nil
}

// UpdateFallbackDomains updates the fallback domains list.
func (c *API) UpdateFallbackDomains(entries []FallbackDomainEntry) error {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	ctx := context.Background()

	domains := make([]cloudflare.FallbackDomain, len(entries))
	for i, e := range entries {
		domains[i] = cloudflare.FallbackDomain{
			Suffix:      e.Suffix,
			Description: e.Description,
			DNSServer:   e.DNSServer,
		}
	}

	_, err := c.CloudflareClient.UpdateFallbackDomain(ctx, c.ValidAccountId, domains)
	if err != nil {
		c.Log.Error(err, "error updating fallback domains")
		return err
	}

	c.Log.Info("Fallback domains updated", "count", len(entries))
	return nil
}
