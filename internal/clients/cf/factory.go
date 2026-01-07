/*
Copyright 2025 Adyanth H.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
func (*DefaultClientFactory) NewClient(config ClientConfig) (CloudflareClient, error) {
	var cfClient *cloudflare.API
	var err error

	switch {
	case config.APIToken != "":
		cfClient, err = cloudflare.NewWithAPIToken(config.APIToken)
	case config.APIKey != "" && config.Email != "":
		cfClient, err = cloudflare.New(config.APIKey, config.Email)
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
