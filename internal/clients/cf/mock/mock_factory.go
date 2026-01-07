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
