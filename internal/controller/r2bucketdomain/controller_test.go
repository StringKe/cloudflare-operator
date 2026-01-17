// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package r2bucketdomain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFinalizerName(t *testing.T) {
	assert.NotEmpty(t, finalizerName)
	assert.Contains(t, finalizerName, "r2")
}

func TestReconcilerFields(t *testing.T) {
	// Test that the Reconciler struct has the expected fields
	r := &Reconciler{}

	// Verify nil defaults
	assert.Nil(t, r.Client)
	assert.Nil(t, r.Scheme)
	assert.Nil(t, r.Recorder)
}
