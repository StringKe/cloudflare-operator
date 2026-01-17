// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHasFinalizer(t *testing.T) {
	tests := []struct {
		name          string
		finalizers    []string
		finalizerName string
		expected      bool
	}{
		{
			name:          "has finalizer",
			finalizers:    []string{"finalizer-1", "finalizer-2"},
			finalizerName: "finalizer-1",
			expected:      true,
		},
		{
			name:          "does not have finalizer",
			finalizers:    []string{"finalizer-1", "finalizer-2"},
			finalizerName: "finalizer-3",
			expected:      false,
		},
		{
			name:          "empty finalizers",
			finalizers:    []string{},
			finalizerName: "finalizer-1",
			expected:      false,
		},
		{
			name:          "nil finalizers",
			finalizers:    nil,
			finalizerName: "finalizer-1",
			expected:      false,
		},
		{
			name:          "single finalizer match",
			finalizers:    []string{"test-finalizer"},
			finalizerName: "test-finalizer",
			expected:      true,
		},
		{
			name:          "partial match should fail",
			finalizers:    []string{"test-finalizer-full"},
			finalizerName: "test-finalizer",
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "default",
					Finalizers: tt.finalizers,
				},
			}

			result := HasFinalizer(obj, tt.finalizerName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsBeingDeleted(t *testing.T) {
	tests := []struct {
		name              string
		deletionTimestamp *metav1.Time
		expected          bool
	}{
		{
			name:              "not being deleted - nil timestamp",
			deletionTimestamp: nil,
			expected:          false,
		},
		{
			name:              "being deleted - has timestamp",
			deletionTimestamp: &metav1.Time{Time: time.Now()},
			expected:          true,
		},
		{
			name:              "being deleted - past timestamp",
			deletionTimestamp: &metav1.Time{Time: time.Now().Add(-time.Hour)},
			expected:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test",
					Namespace:         "default",
					DeletionTimestamp: tt.deletionTimestamp,
				},
			}

			result := IsBeingDeleted(obj)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShouldReconcileDeletion(t *testing.T) {
	now := metav1.Now()

	tests := []struct {
		name              string
		deletionTimestamp *metav1.Time
		finalizers        []string
		finalizerName     string
		expected          bool
	}{
		{
			name:              "should reconcile - being deleted with finalizer",
			deletionTimestamp: &now,
			finalizers:        []string{"test-finalizer"},
			finalizerName:     "test-finalizer",
			expected:          true,
		},
		{
			name:              "should not reconcile - not being deleted",
			deletionTimestamp: nil,
			finalizers:        []string{"test-finalizer"},
			finalizerName:     "test-finalizer",
			expected:          false,
		},
		{
			name:              "should not reconcile - being deleted without finalizer",
			deletionTimestamp: &now,
			finalizers:        []string{"other-finalizer"},
			finalizerName:     "test-finalizer",
			expected:          false,
		},
		{
			name:              "should not reconcile - no finalizers",
			deletionTimestamp: &now,
			finalizers:        []string{},
			finalizerName:     "test-finalizer",
			expected:          false,
		},
		{
			name:              "should reconcile - multiple finalizers",
			deletionTimestamp: &now,
			finalizers:        []string{"other-finalizer", "test-finalizer", "another-finalizer"},
			finalizerName:     "test-finalizer",
			expected:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test",
					Namespace:         "default",
					DeletionTimestamp: tt.deletionTimestamp,
					Finalizers:        tt.finalizers,
				},
			}

			result := ShouldReconcileDeletion(obj, tt.finalizerName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFinalizerConstants(t *testing.T) {
	// Test that the finalizer prefix is correctly defined
	assert.Equal(t, "cloudflare-operator.io/secret-finalizer-", secretFinalizerPrefix)
	assert.Equal(t, "cloudflare-operator.io/finalizer", tunnelFinalizer)
}
