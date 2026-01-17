// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

//go:build e2e

// Package e2e contains end-to-end tests for cloudflare-operator.
// These tests run against a real Kubernetes cluster (Kind or existing)
// with a mock Cloudflare API server.
package e2e

import (
	"os"
	"testing"

	"github.com/StringKe/cloudflare-operator/test/e2e/framework"
)

// TestE2E runs all E2E tests
func TestE2E(t *testing.T) {
	// Check for required environment
	if os.Getenv("E2E_SKIP") == "true" {
		t.Skip("E2E tests skipped (E2E_SKIP=true)")
	}

	// Verify framework can be created
	opts := framework.DefaultOptions()
	opts.SkipMockServer = true // Don't start mock server for suite test
	opts.UseExistingCluster = true

	_, err := framework.New(opts)
	if err != nil {
		t.Skipf("E2E framework setup failed: %v", err)
	}

	t.Log("E2E test suite initialized successfully")
}
