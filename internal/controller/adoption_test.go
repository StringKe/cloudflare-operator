// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package controller

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

const testResourceID = "resource-id"

func TestAdoptionAnnotation(t *testing.T) {
	assert.Equal(t, "cloudflare-operator.io/managed-by", AdoptionAnnotation)
}

func TestNewAdoptionChecker(t *testing.T) {
	tests := []struct {
		name              string
		namespace         string
		resourceName      string
		expectedManagedBy string
	}{
		{
			name:              "namespaced resource",
			namespace:         "default",
			resourceName:      "my-resource",
			expectedManagedBy: "default/my-resource",
		},
		{
			name:              "cluster-scoped resource",
			namespace:         "",
			resourceName:      "my-cluster-resource",
			expectedManagedBy: "my-cluster-resource",
		},
		{
			name:              "different namespace",
			namespace:         "production",
			resourceName:      "prod-resource",
			expectedManagedBy: "production/prod-resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewAdoptionChecker(tt.namespace, tt.resourceName)

			assert.NotNil(t, checker)
			assert.Equal(t, tt.expectedManagedBy, checker.ManagedByValue)
		})
	}
}

func TestAdoptionResultIsAvailable(t *testing.T) {
	tests := []struct {
		name     string
		result   AdoptionResult
		expected bool
	}{
		{
			name: "not found - available",
			result: AdoptionResult{
				Found:    false,
				CanAdopt: true,
			},
			expected: true,
		},
		{
			name: "found and can adopt - available",
			result: AdoptionResult{
				Found:    true,
				CanAdopt: true,
			},
			expected: true,
		},
		{
			name: "found but cannot adopt - not available",
			result: AdoptionResult{
				Found:    true,
				CanAdopt: false,
			},
			expected: false,
		},
		{
			name: "not found and cannot adopt - available (Found is false)",
			result: AdoptionResult{
				Found:    false,
				CanAdopt: false,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.result.IsAvailable())
		})
	}
}

func TestAdoptionResultFields(t *testing.T) {
	result := AdoptionResult{
		Found:      true,
		CanAdopt:   false,
		ExistingID: "resource-123",
		ManagedBy:  "other-namespace/other-resource",
		Error:      nil,
	}

	assert.True(t, result.Found)
	assert.False(t, result.CanAdopt)
	assert.Equal(t, "resource-123", result.ExistingID)
	assert.Equal(t, "other-namespace/other-resource", result.ManagedBy)
	assert.Nil(t, result.Error)
}

func TestAdoptionCheckerCheckByName(t *testing.T) {
	tests := []struct {
		name       string
		namespace  string
		resName    string
		lookupFn   func(name string) (id string, managedBy string, err error)
		wantFound  bool
		wantAdopt  bool
		wantID     string
		wantMgr    string
		wantErrNil bool
	}{
		{
			name:      "resource not found",
			namespace: "default",
			resName:   "my-resource",
			lookupFn: func(_ string) (string, string, error) {
				return "", "", nil
			},
			wantFound:  false,
			wantAdopt:  true,
			wantID:     "",
			wantMgr:    "",
			wantErrNil: true,
		},
		{
			name:      "resource found with no manager",
			namespace: "default",
			resName:   "my-resource",
			lookupFn: func(_ string) (string, string, error) {
				return testResourceID, "", nil
			},
			wantFound:  true,
			wantAdopt:  true,
			wantID:     testResourceID,
			wantMgr:    "",
			wantErrNil: true,
		},
		{
			name:      "resource found managed by same",
			namespace: "default",
			resName:   "my-resource",
			lookupFn: func(_ string) (string, string, error) {
				return testResourceID, "default/my-resource", nil
			},
			wantFound:  true,
			wantAdopt:  true,
			wantID:     testResourceID,
			wantMgr:    "default/my-resource",
			wantErrNil: true,
		},
		{
			name:      "resource found managed by other",
			namespace: "default",
			resName:   "my-resource",
			lookupFn: func(_ string) (string, string, error) {
				return testResourceID, "other-namespace/other-resource", nil
			},
			wantFound:  true,
			wantAdopt:  false,
			wantID:     testResourceID,
			wantMgr:    "other-namespace/other-resource",
			wantErrNil: true,
		},
		{
			name:      "not found error",
			namespace: "default",
			resName:   "my-resource",
			lookupFn: func(_ string) (string, string, error) {
				return "", "", cf.ErrResourceNotFound
			},
			wantFound:  false,
			wantAdopt:  true,
			wantID:     "",
			wantMgr:    "",
			wantErrNil: true,
		},
		{
			name:      "other error",
			namespace: "default",
			resName:   "my-resource",
			lookupFn: func(_ string) (string, string, error) {
				return "", "", errors.New("connection failed")
			},
			wantFound:  false,
			wantAdopt:  false,
			wantID:     "",
			wantMgr:    "",
			wantErrNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewAdoptionChecker(tt.namespace, tt.resName)
			result := checker.CheckByName("test-name", tt.lookupFn)

			assert.Equal(t, tt.wantFound, result.Found)
			assert.Equal(t, tt.wantAdopt, result.CanAdopt)
			assert.Equal(t, tt.wantID, result.ExistingID)
			assert.Equal(t, tt.wantMgr, result.ManagedBy)
			if tt.wantErrNil {
				assert.Nil(t, result.Error)
			} else {
				assert.NotNil(t, result.Error)
			}
		})
	}
}

func TestAdoptionCheckerConflictError(t *testing.T) {
	checker := NewAdoptionChecker("default", "my-resource")

	err := checker.ConflictError("DNSRecord", "www.example.com", "other/resource")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DNSRecord")
	assert.Contains(t, err.Error(), "www.example.com")
	assert.Contains(t, err.Error(), "other/resource")
	assert.ErrorIs(t, err, cf.ErrResourceConflict)
}

func TestFormatManagedByValue(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		resName   string
		expected  string
	}{
		{
			name:      "with namespace",
			namespace: "default",
			resName:   "my-resource",
			expected:  "default/my-resource",
		},
		{
			name:      "without namespace",
			namespace: "",
			resName:   "cluster-resource",
			expected:  "cluster-resource",
		},
		{
			name:      "different namespace",
			namespace: "kube-system",
			resName:   "system-resource",
			expected:  "kube-system/system-resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatManagedByValue(tt.namespace, tt.resName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAdoptionResultWithError(t *testing.T) {
	testErr := errors.New("test error")
	result := AdoptionResult{
		Error: testErr,
	}

	assert.Equal(t, testErr, result.Error)
	// IsAvailable should return true if not found (even with error)
	assert.True(t, result.IsAvailable())
}

func TestClusterScopedAdoption(t *testing.T) {
	// Test adoption for cluster-scoped resources (no namespace)
	checker := NewAdoptionChecker("", "my-cluster-tunnel")

	assert.Equal(t, "my-cluster-tunnel", checker.ManagedByValue)

	// Lookup function returns resource managed by same
	result := checker.CheckByName("my-cluster-tunnel", func(_ string) (string, string, error) {
		return "tunnel-id", "my-cluster-tunnel", nil
	})

	assert.True(t, result.Found)
	assert.True(t, result.CanAdopt)
	assert.Equal(t, "tunnel-id", result.ExistingID)
}

func TestNamespacedAdoption(t *testing.T) {
	// Test adoption for namespaced resources
	checker := NewAdoptionChecker("production", "my-tunnel")

	assert.Equal(t, "production/my-tunnel", checker.ManagedByValue)

	// Lookup function returns resource managed by different namespace
	result := checker.CheckByName("my-tunnel", func(_ string) (string, string, error) {
		return "tunnel-id", "staging/my-tunnel", nil
	})

	assert.True(t, result.Found)
	assert.False(t, result.CanAdopt) // Different namespace
	assert.Equal(t, "staging/my-tunnel", result.ManagedBy)
}
