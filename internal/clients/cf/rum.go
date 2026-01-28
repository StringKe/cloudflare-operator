// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// RUMSite represents a Web Analytics site configuration.
type RUMSite struct {
	// SiteTag is the unique identifier for the site
	SiteTag string `json:"site_tag,omitempty"`
	// SiteToken is the token used for tracking
	SiteToken string `json:"site_token,omitempty"`
	// Host is the hostname being tracked
	Host string `json:"host,omitempty"`
	// ZoneTag is the zone ID if applicable
	ZoneTag string `json:"zone_tag,omitempty"`
	// AutoInstall enables automatic script injection
	AutoInstall bool `json:"auto_install"`
	// Ruleset contains rule configuration
	Ruleset *RUMRuleset `json:"ruleset,omitempty"`
}

// RUMRuleset contains ruleset configuration.
type RUMRuleset struct {
	ID       string    `json:"id,omitempty"`
	ZoneTag  string    `json:"zone_tag,omitempty"`
	ZoneName string    `json:"zone_name,omitempty"`
	Enabled  bool      `json:"enabled"`
	Rules    []RUMRule `json:"rules,omitempty"`
}

// RUMRule represents a Web Analytics rule.
type RUMRule struct {
	ID          string   `json:"id,omitempty"`
	Host        string   `json:"host,omitempty"`
	Paths       []string `json:"paths,omitempty"`
	IsPageviews bool     `json:"is_pageveiws,omitempty"`
	Inclusive   bool     `json:"inclusive,omitempty"`
	IsPaused    bool     `json:"is_paused,omitempty"`
	Priority    int      `json:"priority,omitempty"`
	CreatedAt   string   `json:"created,omitempty"`
}

// EnableWebAnalytics enables Web Analytics for a hostname.
// For Pages projects, use the *.pages.dev hostname or custom domain.
//
// Note: auto_install is only supported for custom domains proxied through Cloudflare.
// For *.pages.dev domains, auto_install must be false as Pages has built-in injection.
func (api *API) EnableWebAnalytics(ctx context.Context, hostname string) (*RUMSite, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf("/accounts/%s/rum/site_info", accountID)

	// auto_install is only valid for custom domains with Cloudflare proxy.
	// For *.pages.dev domains, auto_install must be false (error 10022 otherwise).
	// Pages projects have built-in Web Analytics injection, so auto_install is not needed.
	autoInstall := !strings.HasSuffix(hostname, ".pages.dev")

	params := map[string]interface{}{
		"host":         hostname,
		"auto_install": autoInstall,
	}

	resp, err := api.CloudflareClient.Raw(ctx, "POST", endpoint, params, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Web Analytics site: %w", err)
	}

	var site RUMSite
	if err := json.Unmarshal(resp.Result, &site); err != nil {
		return nil, fmt.Errorf("failed to parse site: %w", err)
	}

	api.Log.Info("Web Analytics enabled", "hostname", hostname, "siteTag", site.SiteTag, "autoInstall", autoInstall)
	return &site, nil
}

// GetWebAnalyticsSite gets a Web Analytics site by hostname.
func (api *API) GetWebAnalyticsSite(ctx context.Context, hostname string) (*RUMSite, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	// List all sites and find by hostname
	endpoint := fmt.Sprintf("/accounts/%s/rum/site_info/list", accountID)

	resp, err := api.CloudflareClient.Raw(ctx, "GET", endpoint, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list Web Analytics sites: %w", err)
	}

	var sites []RUMSite
	if err := json.Unmarshal(resp.Result, &sites); err != nil {
		return nil, fmt.Errorf("failed to parse sites: %w", err)
	}

	for _, site := range sites {
		if site.Host == hostname {
			return &site, nil
		}
	}

	return nil, nil // Not found
}

// UpdateWebAnalyticsSite updates a Web Analytics site configuration.
func (api *API) UpdateWebAnalyticsSite(ctx context.Context, siteTag string, autoInstall bool) (*RUMSite, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf("/accounts/%s/rum/site_info/%s", accountID, siteTag)

	params := map[string]interface{}{
		"auto_install": autoInstall,
	}

	resp, err := api.CloudflareClient.Raw(ctx, "PUT", endpoint, params, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to update Web Analytics site: %w", err)
	}

	var site RUMSite
	if err := json.Unmarshal(resp.Result, &site); err != nil {
		return nil, fmt.Errorf("failed to parse site: %w", err)
	}

	api.Log.Info("Web Analytics site updated", "siteTag", siteTag, "autoInstall", autoInstall)
	return &site, nil
}

// DisableWebAnalytics disables Web Analytics for a site.
func (api *API) DisableWebAnalytics(ctx context.Context, siteTag string) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf("/accounts/%s/rum/site_info/%s", accountID, siteTag)

	_, err = api.CloudflareClient.Raw(ctx, "DELETE", endpoint, nil, nil)
	if err != nil {
		if IsNotFoundError(err) {
			api.Log.Info("Web Analytics site already deleted", "siteTag", siteTag)
			return nil
		}
		return fmt.Errorf("failed to delete Web Analytics site: %w", err)
	}

	api.Log.Info("Web Analytics disabled", "siteTag", siteTag)
	return nil
}
