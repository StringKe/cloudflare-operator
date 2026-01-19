// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// R2CustomDomain represents a custom domain attached to an R2 bucket
type R2CustomDomain struct {
	Domain   string         `json:"domain"`
	Enabled  bool           `json:"enabled"`
	Status   R2DomainStatus `json:"status"`
	MinTLS   string         `json:"minTLS,omitempty"`
	ZoneID   string         `json:"zoneId,omitempty"`
	ZoneName string         `json:"zoneName,omitempty"`
}

// R2DomainStatus represents the status of an R2 custom domain
type R2DomainStatus struct {
	Ownership string `json:"ownership,omitempty"`
	SSL       string `json:"ssl,omitempty"`
}

// R2CustomDomainParams contains parameters for attaching a custom domain
type R2CustomDomainParams struct {
	Domain  string `json:"domain"`
	ZoneID  string `json:"zoneId,omitempty"`
	MinTLS  string `json:"minTLS,omitempty"`
	Enabled bool   `json:"enabled"`
}

// r2CustomDomainResponse is the API response for custom domain operations
type r2CustomDomainResponse struct {
	Result   R2CustomDomain `json:"result"`
	Success  bool           `json:"success"`
	Errors   []apiError     `json:"errors"`
	Messages []string       `json:"messages"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// AttachR2CustomDomain attaches a custom domain to an R2 bucket
func (api *API) AttachR2CustomDomain(
	ctx context.Context, bucketName string, params R2CustomDomainParams,
) (*R2CustomDomain, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf("/accounts/%s/r2/buckets/%s/domains/custom", accountID, bucketName)

	resp, err := api.CloudflareClient.Raw(ctx, http.MethodPost, endpoint, params, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to attach custom domain: %w", err)
	}

	var result r2CustomDomainResponse
	if err := json.Unmarshal(resp.Result, &result.Result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result.Result, nil
}

// GetR2CustomDomain retrieves a custom domain configuration for an R2 bucket
func (api *API) GetR2CustomDomain(
	ctx context.Context, bucketName, domain string,
) (*R2CustomDomain, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf(
		"/accounts/%s/r2/buckets/%s/domains/custom/%s",
		accountID, bucketName, domain,
	)

	resp, err := api.CloudflareClient.Raw(ctx, http.MethodGet, endpoint, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get custom domain: %w", err)
	}

	var result R2CustomDomain
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// ListR2CustomDomains lists all custom domains for an R2 bucket
func (api *API) ListR2CustomDomains(
	ctx context.Context, bucketName string,
) ([]R2CustomDomain, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf("/accounts/%s/r2/buckets/%s/domains/custom", accountID, bucketName)

	resp, err := api.CloudflareClient.Raw(ctx, http.MethodGet, endpoint, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list custom domains: %w", err)
	}

	var result []R2CustomDomain
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// UpdateR2CustomDomain updates the settings for a custom domain
func (api *API) UpdateR2CustomDomain(
	ctx context.Context, bucketName, domain string, params R2CustomDomainParams,
) (*R2CustomDomain, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf(
		"/accounts/%s/r2/buckets/%s/domains/custom/%s",
		accountID, bucketName, domain,
	)

	resp, err := api.CloudflareClient.Raw(ctx, http.MethodPut, endpoint, params, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to update custom domain: %w", err)
	}

	var result R2CustomDomain
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// DeleteR2CustomDomain removes a custom domain from an R2 bucket.
// This method is idempotent - returns nil if the custom domain is already deleted.
func (api *API) DeleteR2CustomDomain(ctx context.Context, bucketName, domain string) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf(
		"/accounts/%s/r2/buckets/%s/domains/custom/%s",
		accountID, bucketName, domain,
	)

	if _, err := api.CloudflareClient.Raw(ctx, http.MethodDelete, endpoint, nil, nil); err != nil {
		if IsNotFoundError(err) {
			api.Log.Info("R2 Custom domain already deleted (not found)", "bucket", bucketName, "domain", domain)
			return nil
		}
		return fmt.Errorf("failed to delete custom domain: %w", err)
	}

	api.Log.Info("R2 Custom domain deleted", "bucket", bucketName, "domain", domain)
	return nil
}

// EnableR2PublicAccess enables public access for an R2 bucket via managed domain
func (api *API) EnableR2PublicAccess(ctx context.Context, bucketName string, enabled bool) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf("/accounts/%s/r2/buckets/%s/domains/managed", accountID, bucketName)

	body := map[string]bool{"enabled": enabled}
	if _, err := api.CloudflareClient.Raw(ctx, http.MethodPut, endpoint, body, nil); err != nil {
		return fmt.Errorf("failed to update public access: %w", err)
	}

	return nil
}
