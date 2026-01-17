// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package testutil

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/StringKe/cloudflare-operator/test/mockserver"
)

// MockServerTestEnv combines a mock Cloudflare server with a test environment.
type MockServerTestEnv struct {
	MockServer *mockserver.Server
	TestEnv    *TestEnv
	APIConfig  *MockAPIClientConfig
}

// MockServerTestEnvOptions configures the mock server test environment.
type MockServerTestEnvOptions struct {
	MockServerPort int
	TestEnvOptions *TestEnvOptions
}

// DefaultMockServerTestEnvOptions returns default options.
func DefaultMockServerTestEnvOptions() *MockServerTestEnvOptions {
	return &MockServerTestEnvOptions{
		MockServerPort: 8787,
		TestEnvOptions: DefaultTestEnvOptions(),
	}
}

// NewMockServerTestEnv creates a new test environment with a mock Cloudflare server.
func NewMockServerTestEnv(opts *MockServerTestEnvOptions) (*MockServerTestEnv, error) {
	if opts == nil {
		opts = DefaultMockServerTestEnvOptions()
	}

	// Create and start mock server
	mockServer := mockserver.NewServer(mockserver.WithPort(opts.MockServerPort))

	if err := mockServer.StartAsync(); err != nil {
		return nil, fmt.Errorf("failed to start mock server: %w", err)
	}

	// Create API config
	apiConfig := &MockAPIClientConfig{
		BaseURL:   mockServer.URL(),
		AccountID: DefaultAccountID,
		ZoneID:    DefaultZoneID,
	}

	// Create test environment
	testEnv, err := NewTestEnv(opts.TestEnvOptions)
	if err != nil {
		mockServer.Stop(context.Background())
		return nil, fmt.Errorf("failed to create test environment: %w", err)
	}

	return &MockServerTestEnv{
		MockServer: mockServer,
		TestEnv:    testEnv,
		APIConfig:  apiConfig,
	}, nil
}

// Stop stops both the mock server and test environment.
func (m *MockServerTestEnv) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var errs []error

	if m.MockServer != nil {
		if err := m.MockServer.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop mock server: %w", err))
		}
	}

	if m.TestEnv != nil {
		if err := m.TestEnv.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop test environment: %w", err))
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// Reset resets the mock server state.
func (m *MockServerTestEnv) Reset() {
	if m.MockServer != nil {
		m.MockServer.Reset()
	}
}

// WaitForMockServer waits for the mock server to be ready.
func WaitForMockServer(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("mock server not ready within %v", timeout)
}
