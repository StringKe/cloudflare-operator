// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesdeployment

import (
	"strings"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// Condition types for PagesDeployment
const (
	// ConditionTypeCloudflareResourceExists indicates whether the Cloudflare deployment exists.
	// When False, the deployment was likely cleaned by Cloudflare's retention policy.
	ConditionTypeCloudflareResourceExists = "CloudflareResourceExists"

	// ConditionTypeSpecChangeIgnored indicates that spec changes were ignored for a terminal deployment.
	ConditionTypeSpecChangeIgnored = "SpecChangeIgnored"

	// ConditionTypeDeploymentRecovered indicates that the deployment was recovered by commit hash.
	ConditionTypeDeploymentRecovered = "DeploymentRecovered"
)

// IsTerminalState checks if the deployment is in a terminal state.
// Terminal states represent completed deployments that should not be automatically recreated.
// Once in a terminal state, the deployment becomes immutable (a historical record).
func IsTerminalState(state networkingv1alpha2.PagesDeploymentState) bool {
	switch state {
	case networkingv1alpha2.PagesDeploymentStateSucceeded,
		networkingv1alpha2.PagesDeploymentStateFailed,
		networkingv1alpha2.PagesDeploymentStateCancelled:
		return true
	default:
		return false
	}
}

// IsInProgressState checks if the deployment is actively being processed.
// In-progress deployments can be recreated if the Cloudflare deployment is lost.
func IsInProgressState(state networkingv1alpha2.PagesDeploymentState) bool {
	switch state {
	case networkingv1alpha2.PagesDeploymentStatePending,
		networkingv1alpha2.PagesDeploymentStateQueued,
		networkingv1alpha2.PagesDeploymentStateBuilding,
		networkingv1alpha2.PagesDeploymentStateDeploying,
		networkingv1alpha2.PagesDeploymentStateRetrying,
		networkingv1alpha2.PagesDeploymentStateRollingBack,
		"": // Initial state before first reconciliation
		return true
	default:
		return false
	}
}

// ExtractCommitHash extracts the commit hash from the deployment spec.
// Returns empty string if no commit hash is specified.
func ExtractCommitHash(deployment *networkingv1alpha2.PagesDeployment) string {
	if deployment.Spec.Source == nil {
		return ""
	}

	switch deployment.Spec.Source.Type {
	case networkingv1alpha2.PagesDeploymentSourceTypeGit:
		if deployment.Spec.Source.Git != nil {
			return normalizeCommitHash(deployment.Spec.Source.Git.CommitSha)
		}
	case networkingv1alpha2.PagesDeploymentSourceTypeDirectUpload:
		if deployment.Spec.Source.DirectUpload != nil &&
			deployment.Spec.Source.DirectUpload.DeploymentMetadata != nil {
			return normalizeCommitHash(deployment.Spec.Source.DirectUpload.DeploymentMetadata.CommitHash)
		}
	default:
		// Unknown source type, no commit hash
		return ""
	}

	return ""
}

// normalizeCommitHash normalizes a commit hash for comparison.
// Converts to lowercase and trims whitespace.
func normalizeCommitHash(hash string) string {
	return strings.ToLower(strings.TrimSpace(hash))
}

// CommitHashMatches checks if two commit hashes match.
// Supports both full hash and short hash (prefix) matching.
// At least 7 characters are required for short hash matching.
//
//nolint:revive // cognitive complexity acceptable for hash matching logic
func CommitHashMatches(hash1, hash2 string) bool {
	if hash1 == "" || hash2 == "" {
		return false
	}

	h1 := normalizeCommitHash(hash1)
	h2 := normalizeCommitHash(hash2)

	// Exact match or prefix match (for short hashes)
	minLen := 7
	return h1 == h2 ||
		(len(h1) >= minLen && len(h2) >= minLen &&
			(strings.HasPrefix(h1, h2) || strings.HasPrefix(h2, h1)))
}
