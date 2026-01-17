// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConditionsAccessor is an interface for resources that have conditions.
type ConditionsAccessor interface {
	GetConditions() []metav1.Condition
}

// AssertConditionTrue asserts that a condition with the given type is True.
func AssertConditionTrue(t *testing.T, conditions []metav1.Condition, conditionType string) {
	t.Helper()
	cond := findCondition(conditions, conditionType)
	require.NotNil(t, cond, "Condition %s should exist", conditionType)
	assert.Equal(t, metav1.ConditionTrue, cond.Status, "Condition %s should be True", conditionType)
}

// AssertConditionFalse asserts that a condition with the given type is False.
func AssertConditionFalse(t *testing.T, conditions []metav1.Condition, conditionType string) {
	t.Helper()
	cond := findCondition(conditions, conditionType)
	require.NotNil(t, cond, "Condition %s should exist", conditionType)
	assert.Equal(t, metav1.ConditionFalse, cond.Status, "Condition %s should be False", conditionType)
}

// AssertConditionWithReason asserts that a condition has the expected reason.
func AssertConditionWithReason(t *testing.T, conditions []metav1.Condition, conditionType, expectedReason string) {
	t.Helper()
	cond := findCondition(conditions, conditionType)
	require.NotNil(t, cond, "Condition %s should exist", conditionType)
	assert.Equal(t, expectedReason, cond.Reason, "Condition %s should have reason %s", conditionType, expectedReason)
}

// AssertConditionWithMessage asserts that a condition message contains the expected substring.
func AssertConditionWithMessage(t *testing.T, conditions []metav1.Condition, conditionType, expectedMessage string) {
	t.Helper()
	cond := findCondition(conditions, conditionType)
	require.NotNil(t, cond, "Condition %s should exist", conditionType)
	assert.Contains(t, cond.Message, expectedMessage, "Condition %s message should contain %s", conditionType, expectedMessage)
}

// AssertNoCondition asserts that a condition with the given type does not exist.
func AssertNoCondition(t *testing.T, conditions []metav1.Condition, conditionType string) {
	t.Helper()
	cond := findCondition(conditions, conditionType)
	assert.Nil(t, cond, "Condition %s should not exist", conditionType)
}

// AssertHasFinalizer asserts that the object has the given finalizer.
func AssertHasFinalizer(t *testing.T, finalizers []string, finalizerName string) {
	t.Helper()
	for _, f := range finalizers {
		if f == finalizerName {
			return
		}
	}
	t.Errorf("Expected finalizer %s not found in %v", finalizerName, finalizers)
}

// AssertNoFinalizer asserts that the object does not have the given finalizer.
func AssertNoFinalizer(t *testing.T, finalizers []string, finalizerName string) {
	t.Helper()
	for _, f := range finalizers {
		if f == finalizerName {
			t.Errorf("Unexpected finalizer %s found in %v", finalizerName, finalizers)
			return
		}
	}
}

// AssertResourceReady asserts that the Ready condition is True.
func AssertResourceReady(t *testing.T, conditions []metav1.Condition) {
	t.Helper()
	AssertConditionTrue(t, conditions, "Ready")
}

// AssertResourceNotReady asserts that the Ready condition is False.
func AssertResourceNotReady(t *testing.T, conditions []metav1.Condition) {
	t.Helper()
	AssertConditionFalse(t, conditions, "Ready")
}

// findCondition finds a condition by type.
func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

// RequireEventually is a helper that runs require.Eventually with common defaults.
func RequireEventually(t *testing.T, condition func() bool, msgAndArgs ...interface{}) {
	t.Helper()
	require.Eventually(t, condition, testTimeout, testInterval, msgAndArgs...)
}

// AssertEventually is a helper that runs assert.Eventually with common defaults.
func AssertEventually(t *testing.T, condition func() bool, msgAndArgs ...interface{}) bool {
	t.Helper()
	return assert.Eventually(t, condition, testTimeout, testInterval, msgAndArgs...)
}
