// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package warpconnector

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFinalizerName(t *testing.T) {
	assert.NotEmpty(t, FinalizerName)
	assert.Contains(t, FinalizerName, "warp")
}

func TestReconcilerFields(t *testing.T) {
	// Test that the WARPConnectorReconciler struct has the expected fields
	r := &WARPConnectorReconciler{}

	// Verify nil defaults
	assert.Nil(t, r.Client)
	assert.Nil(t, r.Scheme)
}
