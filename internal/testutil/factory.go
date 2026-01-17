// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package testutil provides testing utilities for the cloudflare-operator.
package testutil

import (
	"context"
	"net/http"
	"time"

	"github.com/cloudflare/cloudflare-go"
)

// MockAPIClientConfig holds configuration for creating mock API clients.
type MockAPIClientConfig struct {
	// BaseURL is the mock server URL
	BaseURL string
	// AccountID is the test account ID
	AccountID string
	// ZoneID is the test zone ID
	ZoneID string
}

// DefaultMockConfig returns a default mock configuration.
func DefaultMockConfig() *MockAPIClientConfig {
	return &MockAPIClientConfig{
		BaseURL:   "http://localhost:8787",
		AccountID: "test-account-id",
		ZoneID:    "test-zone-id",
	}
}

// NewMockCloudflareAPI creates a new Cloudflare API client pointing to the mock server.
func NewMockCloudflareAPI(config *MockAPIClientConfig) (*cloudflare.API, error) {
	if config == nil {
		config = DefaultMockConfig()
	}

	// Create HTTP client with custom transport
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create API client with mock server URL
	api, err := cloudflare.New(
		"test-api-key",
		"test@example.com",
		cloudflare.HTTPClient(httpClient),
		cloudflare.BaseURL(config.BaseURL),
	)
	if err != nil {
		return nil, err
	}

	return api, nil
}

// NewMockCloudflareAPIWithToken creates a new Cloudflare API client using API token.
func NewMockCloudflareAPIWithToken(config *MockAPIClientConfig) (*cloudflare.API, error) {
	if config == nil {
		config = DefaultMockConfig()
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	api, err := cloudflare.NewWithAPIToken(
		"test-api-token",
		cloudflare.HTTPClient(httpClient),
		cloudflare.BaseURL(config.BaseURL),
	)
	if err != nil {
		return nil, err
	}

	return api, nil
}

// ResourceContext provides context for creating test resources.
type ResourceContext struct {
	Ctx       context.Context
	AccountID string
	ZoneID    string
	API       *cloudflare.API
}

// NewResourceContext creates a new resource context for testing.
func NewResourceContext(api *cloudflare.API, config *MockAPIClientConfig) *ResourceContext {
	if config == nil {
		config = DefaultMockConfig()
	}
	return &ResourceContext{
		Ctx:       context.Background(),
		AccountID: config.AccountID,
		ZoneID:    config.ZoneID,
		API:       api,
	}
}
