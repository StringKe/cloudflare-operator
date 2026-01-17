// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package gatewayconfiguration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFinalizerName(t *testing.T) {
	assert.NotEmpty(t, FinalizerName)
	assert.Contains(t, FinalizerName, "gatewayconfiguration")
}

func TestReconcilerFields(t *testing.T) {
	// Test that the GatewayConfigurationReconciler struct has the expected fields
	r := &GatewayConfigurationReconciler{}

	// Verify nil defaults
	assert.Nil(t, r.Client)
	assert.Nil(t, r.Scheme)
	assert.Nil(t, r.Recorder)
}
