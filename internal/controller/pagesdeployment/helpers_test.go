// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesdeployment

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

func TestIsTerminalState(t *testing.T) {
	tests := []struct {
		name     string
		state    networkingv1alpha2.PagesDeploymentState
		expected bool
	}{
		{
			name:     "Succeeded is terminal",
			state:    networkingv1alpha2.PagesDeploymentStateSucceeded,
			expected: true,
		},
		{
			name:     "Failed is terminal",
			state:    networkingv1alpha2.PagesDeploymentStateFailed,
			expected: true,
		},
		{
			name:     "Cancelled is terminal",
			state:    networkingv1alpha2.PagesDeploymentStateCancelled,
			expected: true,
		},
		{
			name:     "Pending is not terminal",
			state:    networkingv1alpha2.PagesDeploymentStatePending,
			expected: false,
		},
		{
			name:     "Queued is not terminal",
			state:    networkingv1alpha2.PagesDeploymentStateQueued,
			expected: false,
		},
		{
			name:     "Building is not terminal",
			state:    networkingv1alpha2.PagesDeploymentStateBuilding,
			expected: false,
		},
		{
			name:     "Deploying is not terminal",
			state:    networkingv1alpha2.PagesDeploymentStateDeploying,
			expected: false,
		},
		{
			name:     "Empty string is not terminal",
			state:    "",
			expected: false,
		},
		{
			name:     "Retrying is not terminal",
			state:    networkingv1alpha2.PagesDeploymentStateRetrying,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsTerminalState(tt.state)
			if result != tt.expected {
				t.Errorf("IsTerminalState(%q) = %v, expected %v", tt.state, result, tt.expected)
			}
		})
	}
}

func TestIsInProgressState(t *testing.T) {
	tests := []struct {
		name     string
		state    networkingv1alpha2.PagesDeploymentState
		expected bool
	}{
		{
			name:     "Pending is in progress",
			state:    networkingv1alpha2.PagesDeploymentStatePending,
			expected: true,
		},
		{
			name:     "Building is in progress",
			state:    networkingv1alpha2.PagesDeploymentStateBuilding,
			expected: true,
		},
		{
			name:     "Empty string is in progress (initial state)",
			state:    "",
			expected: true,
		},
		{
			name:     "Succeeded is not in progress",
			state:    networkingv1alpha2.PagesDeploymentStateSucceeded,
			expected: false,
		},
		{
			name:     "Failed is not in progress",
			state:    networkingv1alpha2.PagesDeploymentStateFailed,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsInProgressState(tt.state)
			if result != tt.expected {
				t.Errorf("IsInProgressState(%q) = %v, expected %v", tt.state, result, tt.expected)
			}
		})
	}
}

func TestExtractCommitHash(t *testing.T) {
	tests := []struct {
		name       string
		deployment *networkingv1alpha2.PagesDeployment
		expected   string
	}{
		{
			name: "Git source with commit SHA",
			deployment: &networkingv1alpha2.PagesDeployment{
				Spec: networkingv1alpha2.PagesDeploymentSpec{
					Source: &networkingv1alpha2.PagesDeploymentSourceSpec{
						Type: networkingv1alpha2.PagesDeploymentSourceTypeGit,
						Git: &networkingv1alpha2.PagesGitSourceSpec{
							CommitSha: "abc123def456",
						},
					},
				},
			},
			expected: "abc123def456",
		},
		{
			name: "Git source with uppercase commit SHA (normalized to lowercase)",
			deployment: &networkingv1alpha2.PagesDeployment{
				Spec: networkingv1alpha2.PagesDeploymentSpec{
					Source: &networkingv1alpha2.PagesDeploymentSourceSpec{
						Type: networkingv1alpha2.PagesDeploymentSourceTypeGit,
						Git: &networkingv1alpha2.PagesGitSourceSpec{
							CommitSha: "ABC123DEF456",
						},
					},
				},
			},
			expected: "abc123def456",
		},
		{
			name: "Direct upload with commit hash in metadata",
			deployment: &networkingv1alpha2.PagesDeployment{
				Spec: networkingv1alpha2.PagesDeploymentSpec{
					Source: &networkingv1alpha2.PagesDeploymentSourceSpec{
						Type: networkingv1alpha2.PagesDeploymentSourceTypeDirectUpload,
						DirectUpload: &networkingv1alpha2.PagesDirectUploadSourceSpec{
							DeploymentMetadata: &networkingv1alpha2.DeploymentTriggerMetadata{
								CommitHash: "def789ghi012",
							},
						},
					},
				},
			},
			expected: "def789ghi012",
		},
		{
			name: "No source",
			deployment: &networkingv1alpha2.PagesDeployment{
				Spec: networkingv1alpha2.PagesDeploymentSpec{},
			},
			expected: "",
		},
		{
			name: "Git source without commit SHA",
			deployment: &networkingv1alpha2.PagesDeployment{
				Spec: networkingv1alpha2.PagesDeploymentSpec{
					Source: &networkingv1alpha2.PagesDeploymentSourceSpec{
						Type: networkingv1alpha2.PagesDeploymentSourceTypeGit,
						Git: &networkingv1alpha2.PagesGitSourceSpec{
							Branch: "main",
						},
					},
				},
			},
			expected: "",
		},
		{
			name: "Direct upload without deployment metadata",
			deployment: &networkingv1alpha2.PagesDeployment{
				Spec: networkingv1alpha2.PagesDeploymentSpec{
					Source: &networkingv1alpha2.PagesDeploymentSourceSpec{
						Type:         networkingv1alpha2.PagesDeploymentSourceTypeDirectUpload,
						DirectUpload: &networkingv1alpha2.PagesDirectUploadSourceSpec{},
					},
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractCommitHash(tt.deployment)
			if result != tt.expected {
				t.Errorf("ExtractCommitHash() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestCommitHashMatches(t *testing.T) {
	tests := []struct {
		name     string
		hash1    string
		hash2    string
		expected bool
	}{
		{
			name:     "Exact match",
			hash1:    "abc123def456789012345678901234567890abcd",
			hash2:    "abc123def456789012345678901234567890abcd",
			expected: true,
		},
		{
			name:     "Case insensitive match",
			hash1:    "ABC123DEF456",
			hash2:    "abc123def456",
			expected: true,
		},
		{
			name:     "Short hash prefix match (7 chars)",
			hash1:    "abc123d",
			hash2:    "abc123def456789012345678901234567890abcd",
			expected: true,
		},
		{
			name:     "Short hash prefix match (reverse)",
			hash1:    "abc123def456789012345678901234567890abcd",
			hash2:    "abc123d",
			expected: true,
		},
		{
			name:     "Too short for prefix match (6 chars)",
			hash1:    "abc123",
			hash2:    "abc123def456789012345678901234567890abcd",
			expected: false,
		},
		{
			name:     "No match",
			hash1:    "abc123def456",
			hash2:    "xyz789ghi012",
			expected: false,
		},
		{
			name:     "Empty hash1",
			hash1:    "",
			hash2:    "abc123def456",
			expected: false,
		},
		{
			name:     "Empty hash2",
			hash1:    "abc123def456",
			hash2:    "",
			expected: false,
		},
		{
			name:     "Both empty",
			hash1:    "",
			hash2:    "",
			expected: false,
		},
		{
			name:     "Whitespace handling",
			hash1:    "  abc123def456  ",
			hash2:    "abc123def456",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CommitHashMatches(tt.hash1, tt.hash2)
			if result != tt.expected {
				t.Errorf("CommitHashMatches(%q, %q) = %v, expected %v", tt.hash1, tt.hash2, result, tt.expected)
			}
		})
	}
}

func TestExtractCommitHash_AllDeploymentModes(t *testing.T) {
	tests := []struct {
		name       string
		deployment *networkingv1alpha2.PagesDeployment
		expected   string
		desc       string
	}{
		{
			name: "Git with commitSha",
			deployment: &networkingv1alpha2.PagesDeployment{
				Spec: networkingv1alpha2.PagesDeploymentSpec{
					Source: &networkingv1alpha2.PagesDeploymentSourceSpec{
						Type: networkingv1alpha2.PagesDeploymentSourceTypeGit,
						Git: &networkingv1alpha2.PagesGitSourceSpec{
							Branch:    "main",
							CommitSha: "abc123def456",
						},
					},
				},
			},
			expected: "abc123def456",
			desc:     "Should extract commitSha from git source",
		},
		{
			name: "Git with branch only (no commitSha)",
			deployment: &networkingv1alpha2.PagesDeployment{
				Spec: networkingv1alpha2.PagesDeploymentSpec{
					Source: &networkingv1alpha2.PagesDeploymentSourceSpec{
						Type: networkingv1alpha2.PagesDeploymentSourceTypeGit,
						Git: &networkingv1alpha2.PagesGitSourceSpec{
							Branch: "feature-branch",
						},
					},
				},
			},
			expected: "",
			desc:     "Should return empty for git without commitSha (idempotency check skipped)",
		},
		{
			name: "DirectUpload with commitHash in metadata",
			deployment: &networkingv1alpha2.PagesDeployment{
				Spec: networkingv1alpha2.PagesDeploymentSpec{
					Source: &networkingv1alpha2.PagesDeploymentSourceSpec{
						Type: networkingv1alpha2.PagesDeploymentSourceTypeDirectUpload,
						DirectUpload: &networkingv1alpha2.PagesDirectUploadSourceSpec{
							Source: &networkingv1alpha2.DirectUploadSource{
								HTTP: &networkingv1alpha2.HTTPSource{URL: "https://example.com/files.tar.gz"},
							},
							DeploymentMetadata: &networkingv1alpha2.DeploymentTriggerMetadata{
								CommitHash: "def789ghi012",
							},
						},
					},
				},
			},
			expected: "def789ghi012",
			desc:     "Should extract commitHash from directUpload metadata",
		},
		{
			name: "DirectUpload without commitHash",
			deployment: &networkingv1alpha2.PagesDeployment{
				Spec: networkingv1alpha2.PagesDeploymentSpec{
					Source: &networkingv1alpha2.PagesDeploymentSourceSpec{
						Type: networkingv1alpha2.PagesDeploymentSourceTypeDirectUpload,
						DirectUpload: &networkingv1alpha2.PagesDirectUploadSourceSpec{
							Source: &networkingv1alpha2.DirectUploadSource{
								HTTP: &networkingv1alpha2.HTTPSource{URL: "https://example.com/files.tar.gz"},
							},
						},
					},
				},
			},
			expected: "",
			desc:     "Should return empty for directUpload without metadata (idempotency check skipped)",
		},
		{
			name: "Rollback mode (action=rollback)",
			deployment: &networkingv1alpha2.PagesDeployment{
				Spec: networkingv1alpha2.PagesDeploymentSpec{
					Action:             networkingv1alpha2.PagesDeploymentActionRollback,
					TargetDeploymentID: "previous-deployment-id",
				},
			},
			expected: "",
			desc:     "Should return empty for rollback mode (uses targetDeploymentID, not commit hash)",
		},
		{
			name: "Legacy Git mode (branch field only)",
			deployment: &networkingv1alpha2.PagesDeployment{
				Spec: networkingv1alpha2.PagesDeploymentSpec{
					Branch: "legacy-branch",
				},
			},
			expected: "",
			desc:     "Should return empty for legacy mode (no Source defined)",
		},
		{
			name: "Empty spec",
			deployment: &networkingv1alpha2.PagesDeployment{
				Spec: networkingv1alpha2.PagesDeploymentSpec{},
			},
			expected: "",
			desc:     "Should return empty for empty spec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractCommitHash(tt.deployment)
			if result != tt.expected {
				t.Errorf("%s\nExtractCommitHash() = %q, expected %q", tt.desc, result, tt.expected)
			}
		})
	}
}

func TestNeedsNewDeployment_TerminalStateProtection(t *testing.T) {
	r := &PagesDeploymentReconciler{}

	tests := []struct {
		name       string
		deployment *networkingv1alpha2.PagesDeployment
		expected   bool
	}{
		{
			name: "Succeeded state without force-redeploy should not need new deployment",
			deployment: &networkingv1alpha2.PagesDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
				Status: networkingv1alpha2.PagesDeploymentStatus{
					State:              networkingv1alpha2.PagesDeploymentStateSucceeded,
					DeploymentID:       "existing-id",
					ObservedGeneration: 1, // Spec changed but should be ignored
				},
			},
			expected: false,
		},
		{
			name: "Succeeded state with force-redeploy should need new deployment",
			deployment: &networkingv1alpha2.PagesDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
					Annotations: map[string]string{
						AnnotationForceRedeploy:     "timestamp-123",
						AnnotationLastForceRedeploy: "timestamp-122", // Different value
					},
				},
				Status: networkingv1alpha2.PagesDeploymentStatus{
					State:              networkingv1alpha2.PagesDeploymentStateSucceeded,
					DeploymentID:       "existing-id",
					ObservedGeneration: 1,
				},
			},
			expected: true,
		},
		{
			name: "Pending state without deployment ID should need new deployment",
			deployment: &networkingv1alpha2.PagesDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Status: networkingv1alpha2.PagesDeploymentStatus{
					State:              networkingv1alpha2.PagesDeploymentStatePending,
					DeploymentID:       "",
					ObservedGeneration: 0,
				},
			},
			expected: true,
		},
		{
			name: "Building state with deployment ID should not need new deployment",
			deployment: &networkingv1alpha2.PagesDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Status: networkingv1alpha2.PagesDeploymentStatus{
					State:              networkingv1alpha2.PagesDeploymentStateBuilding,
					DeploymentID:       "existing-id",
					ObservedGeneration: 1,
				},
			},
			expected: false,
		},
		{
			name: "Failed state with same force-redeploy should not need new deployment (unless auto-retry)",
			deployment: &networkingv1alpha2.PagesDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						AnnotationForceRedeploy:     "same-value",
						AnnotationLastForceRedeploy: "same-value",
						AnnotationAutoRetry:         "false", // Disable auto-retry
					},
				},
				Status: networkingv1alpha2.PagesDeploymentStatus{
					State:              networkingv1alpha2.PagesDeploymentStateFailed,
					DeploymentID:       "existing-id",
					ObservedGeneration: 1,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.needsNewDeployment(tt.deployment)
			if result != tt.expected {
				t.Errorf("needsNewDeployment() = %v, expected %v", result, tt.expected)
			}
		})
	}
}
