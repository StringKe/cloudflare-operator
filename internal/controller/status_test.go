// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package controller

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNewStatusUpdater(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	updater := NewStatusUpdater(fakeClient)

	assert.NotNil(t, updater)
	assert.Equal(t, DefaultMaxRetries, updater.MaxRetries)
	assert.Equal(t, DefaultRetryDelay, updater.RetryDelay)
}

func TestDefaultConstants(t *testing.T) {
	assert.Equal(t, 5, DefaultMaxRetries)
	assert.Equal(t, 100*time.Millisecond, DefaultRetryDelay)
}

func TestSetCondition(t *testing.T) {
	tests := []struct {
		name          string
		conditionType string
		status        metav1.ConditionStatus
		reason        string
		message       string
	}{
		{
			name:          "ready true",
			conditionType: "Ready",
			status:        metav1.ConditionTrue,
			reason:        "Reconciled",
			message:       "Resource is ready",
		},
		{
			name:          "ready false",
			conditionType: "Ready",
			status:        metav1.ConditionFalse,
			reason:        "Error",
			message:       "Failed to reconcile",
		},
		{
			name:          "ready unknown",
			conditionType: "Ready",
			status:        metav1.ConditionUnknown,
			reason:        "Progressing",
			message:       "Reconciliation in progress",
		},
		{
			name:          "custom condition",
			conditionType: "Available",
			status:        metav1.ConditionTrue,
			reason:        "TunnelReady",
			message:       "Tunnel is available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var conditions []metav1.Condition

			SetCondition(&conditions, tt.conditionType, tt.status, tt.reason, tt.message)

			require.Len(t, conditions, 1)
			assert.Equal(t, tt.conditionType, conditions[0].Type)
			assert.Equal(t, tt.status, conditions[0].Status)
			assert.Equal(t, tt.reason, conditions[0].Reason)
			assert.Equal(t, tt.message, conditions[0].Message)
			assert.False(t, conditions[0].LastTransitionTime.IsZero())
		})
	}
}

func TestSetConditionUpdatesExisting(t *testing.T) {
	conditions := []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "Error",
			Message:            "Old error",
			LastTransitionTime: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
		},
	}

	SetCondition(&conditions, "Ready", metav1.ConditionTrue, "Reconciled", "Success")

	require.Len(t, conditions, 1)
	assert.Equal(t, metav1.ConditionTrue, conditions[0].Status)
	assert.Equal(t, "Reconciled", conditions[0].Reason)
	assert.Equal(t, "Success", conditions[0].Message)
}

func TestSetReadyCondition(t *testing.T) {
	var conditions []metav1.Condition

	SetReadyCondition(&conditions, metav1.ConditionTrue, "Ready", "All systems go")

	require.Len(t, conditions, 1)
	assert.Equal(t, "Ready", conditions[0].Type)
	assert.Equal(t, metav1.ConditionTrue, conditions[0].Status)
}

func TestSetErrorCondition(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantMessage string
	}{
		{
			name:        "with error",
			err:         errors.New("something went wrong"),
			wantMessage: "something went wrong",
		},
		{
			name:        "nil error",
			err:         nil,
			wantMessage: "Unknown error",
		},
		{
			name:        "long error message is truncated",
			err:         errors.New(strings.Repeat("x", 2000)),
			wantMessage: strings.Repeat("x", 1021) + "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var conditions []metav1.Condition

			SetErrorCondition(&conditions, tt.err)

			require.Len(t, conditions, 1)
			assert.Equal(t, "Ready", conditions[0].Type)
			assert.Equal(t, metav1.ConditionFalse, conditions[0].Status)
			assert.Equal(t, "ReconcileError", conditions[0].Reason)
			assert.Equal(t, tt.wantMessage, conditions[0].Message)
		})
	}
}

func TestSetSuccessCondition(t *testing.T) {
	var conditions []metav1.Condition

	SetSuccessCondition(&conditions, "Resource reconciled successfully")

	require.Len(t, conditions, 1)
	assert.Equal(t, "Ready", conditions[0].Type)
	assert.Equal(t, metav1.ConditionTrue, conditions[0].Status)
	assert.Equal(t, "Reconciled", conditions[0].Reason)
	assert.Equal(t, "Resource reconciled successfully", conditions[0].Message)
}

func TestStateConstants(t *testing.T) {
	assert.Equal(t, "Pending", StatePending)
	assert.Equal(t, "Creating", StateCreating)
	assert.Equal(t, "Active", StateActive)
	assert.Equal(t, "Ready", StateReady)
	assert.Equal(t, "Error", StateError)
	assert.Equal(t, "Deleting", StateDeleting)
	assert.Equal(t, "Warning", StateWarning)
}

func TestIsTerminalState(t *testing.T) {
	tests := []struct {
		state string
		want  bool
	}{
		{StatePending, false},
		{StateCreating, false},
		{StateActive, true},
		{StateReady, true},
		{StateError, true},
		{StateDeleting, false},
		{StateWarning, false},
		{"Unknown", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got := IsTerminalState(tt.state)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMultipleConditions(t *testing.T) {
	var conditions []metav1.Condition

	// Set multiple conditions
	SetCondition(&conditions, "Ready", metav1.ConditionTrue, "Reconciled", "Ready")
	SetCondition(&conditions, "Available", metav1.ConditionTrue, "Available", "Available")
	SetCondition(&conditions, "Progressing", metav1.ConditionFalse, "Done", "Not progressing")

	assert.Len(t, conditions, 3)

	// Find each condition
	var ready, available, progressing *metav1.Condition
	for i := range conditions {
		switch conditions[i].Type {
		case "Ready":
			ready = &conditions[i]
		case "Available":
			available = &conditions[i]
		case "Progressing":
			progressing = &conditions[i]
		}
	}

	require.NotNil(t, ready)
	require.NotNil(t, available)
	require.NotNil(t, progressing)

	assert.Equal(t, metav1.ConditionTrue, ready.Status)
	assert.Equal(t, metav1.ConditionTrue, available.Status)
	assert.Equal(t, metav1.ConditionFalse, progressing.Status)
}

func TestSetConditionPreservesOtherConditions(t *testing.T) {
	conditions := []metav1.Condition{
		{
			Type:   "Available",
			Status: metav1.ConditionTrue,
			Reason: "Available",
		},
		{
			Type:   "Ready",
			Status: metav1.ConditionFalse,
			Reason: "NotReady",
		},
	}

	// Update only Ready condition
	SetCondition(&conditions, "Ready", metav1.ConditionTrue, "Ready", "Now ready")

	assert.Len(t, conditions, 2)

	// Available should be unchanged
	for _, c := range conditions {
		if c.Type == "Available" {
			assert.Equal(t, metav1.ConditionTrue, c.Status)
			assert.Equal(t, "Available", c.Reason)
		}
	}
}

func TestStatusUpdaterWithRetry(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Create a ConfigMap to update
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"key": "value",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	updater := NewStatusUpdater(fakeClient)
	ctx := context.Background()

	// Update should succeed
	err := updater.UpdateWithRetry(ctx, cm, func() {
		cm.Data["key"] = "new-value"
	})

	assert.NoError(t, err)

	// Verify update
	updated := &corev1.ConfigMap{}
	err = fakeClient.Get(ctx, client.ObjectKeyFromObject(cm), updated)
	require.NoError(t, err)
	assert.Equal(t, "new-value", updated.Data["key"])
}
