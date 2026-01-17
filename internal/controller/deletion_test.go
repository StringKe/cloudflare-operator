// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package controller

import (
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
)

func TestDefaultRequeueAfter(t *testing.T) {
	assert.Equal(t, 30*time.Second, DefaultRequeueAfter)
}

func TestNewDeletionHandler(t *testing.T) {
	log := logr.Discard()
	finalizerName := "test-finalizer"

	handler := NewDeletionHandler(nil, log, nil, finalizerName)

	assert.NotNil(t, handler)
	assert.Equal(t, finalizerName, handler.FinalizerName)
	assert.Nil(t, handler.Client)
	assert.Nil(t, handler.Recorder)
}

func TestDeletionHandlerFields(t *testing.T) {
	tests := []struct {
		name          string
		finalizerName string
	}{
		{
			name:          "simple finalizer",
			finalizerName: "test-finalizer",
		},
		{
			name:          "namespaced finalizer",
			finalizerName: "cloudflare-operator.io/finalizer",
		},
		{
			name:          "empty finalizer",
			finalizerName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logr.Discard()
			handler := NewDeletionHandler(nil, log, nil, tt.finalizerName)

			assert.Equal(t, tt.finalizerName, handler.FinalizerName)
		})
	}
}

func TestDeletionHandlerStruct(t *testing.T) {
	handler := DeletionHandler{
		FinalizerName: "test-finalizer",
	}

	assert.Equal(t, "test-finalizer", handler.FinalizerName)
	assert.Nil(t, handler.Client)
	assert.Nil(t, handler.Recorder)
}
