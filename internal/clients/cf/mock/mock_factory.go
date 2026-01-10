// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package mock provides mock implementations for testing Cloudflare client operations.
package mock

import (
	"go.uber.org/mock/gomock"

	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

// MockClientFactory is a factory that creates mock CloudflareClient instances.
type MockClientFactory struct {
	ctrl       *gomock.Controller
	mockClient *MockCloudflareClient
}

// NewMockClientFactory creates a new MockClientFactory.
func NewMockClientFactory(ctrl *gomock.Controller) *MockClientFactory {
	return &MockClientFactory{
		ctrl:       ctrl,
		mockClient: NewMockCloudflareClient(ctrl),
	}
}

// NewClient returns the mock client.
func (f *MockClientFactory) NewClient(_ cf.ClientConfig) (cf.CloudflareClient, error) {
	return f.mockClient, nil
}

// GetMockClient returns the underlying mock client for setting up expectations.
func (f *MockClientFactory) GetMockClient() *MockCloudflareClient {
	return f.mockClient
}

// MockClientFactoryWithError is a factory that returns an error.
type MockClientFactoryWithError struct {
	Err error
}

// NewClient returns the configured error.
func (f *MockClientFactoryWithError) NewClient(_ cf.ClientConfig) (cf.CloudflareClient, error) {
	return nil, f.Err
}

// MockClientFactoryWithCustomClient is a factory that returns a custom mock client.
type MockClientFactoryWithCustomClient struct {
	Client cf.CloudflareClient
}

// NewClient returns the custom client.
func (f *MockClientFactoryWithCustomClient) NewClient(_ cf.ClientConfig) (cf.CloudflareClient, error) {
	return f.Client, nil
}
