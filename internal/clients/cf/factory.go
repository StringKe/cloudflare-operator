// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"errors"

	"github.com/cloudflare/cloudflare-go"
	"github.com/go-logr/logr"
)

// ErrNoCredentials is returned when no API credentials are provided.
var ErrNoCredentials = errors.New("no API credentials provided: either APIToken or (APIKey + Email) required")

// ClientFactory creates CloudflareClient instances.
// This interface enables dependency injection for testing.
type ClientFactory interface {
	// NewClient creates a new CloudflareClient with the given configuration.
	NewClient(config ClientConfig) (CloudflareClient, error)
}

// ClientConfig contains configuration for creating a CloudflareClient.
type ClientConfig struct {
	Log         logr.Logger
	APIToken    string
	APIKey      string
	Email       string
	AccountID   string
	AccountName string
	Domain      string
	TunnelID    string
	TunnelName  string
}

// DefaultClientFactory creates real CloudflareClient instances.
type DefaultClientFactory struct{}

// NewClient creates a new CloudflareClient using the real Cloudflare API.
// If CLOUDFLARE_API_BASE_URL environment variable is set, it uses that as the API base URL.
func (*DefaultClientFactory) NewClient(config ClientConfig) (CloudflareClient, error) {
	var cfClient *cloudflare.API
	var err error

	// Build options list - add custom base URL if configured
	var opts []cloudflare.Option
	if baseURL := GetAPIBaseURL(); baseURL != "" {
		opts = append(opts, cloudflare.BaseURL(baseURL))
	}

	switch {
	case config.APIToken != "":
		cfClient, err = cloudflare.NewWithAPIToken(config.APIToken, opts...)
	case config.APIKey != "" && config.Email != "":
		cfClient, err = cloudflare.New(config.APIKey, config.Email, opts...)
	default:
		return nil, ErrNoCredentials
	}

	if err != nil {
		return nil, err
	}

	return &API{
		Log:              config.Log,
		AccountId:        config.AccountID,
		AccountName:      config.AccountName,
		Domain:           config.Domain,
		TunnelId:         config.TunnelID,
		TunnelName:       config.TunnelName,
		CloudflareClient: cfClient,
	}, nil
}

// NewDefaultClientFactory creates a new DefaultClientFactory.
func NewDefaultClientFactory() ClientFactory {
	return &DefaultClientFactory{}
}

// Global default factory instance
var defaultFactory ClientFactory = &DefaultClientFactory{}

// GetDefaultFactory returns the default ClientFactory.
func GetDefaultFactory() ClientFactory {
	return defaultFactory
}

// SetDefaultFactory sets the default ClientFactory (useful for testing).
func SetDefaultFactory(factory ClientFactory) {
	defaultFactory = factory
}

// ResetDefaultFactory resets the default ClientFactory to the real implementation.
func ResetDefaultFactory() {
	defaultFactory = &DefaultClientFactory{}
}
