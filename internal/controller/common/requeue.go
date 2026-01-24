// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package common

import (
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

// Standard requeue intervals for controllers.
const (
	// RequeueIntervalShort is used for quick retries after transient errors.
	RequeueIntervalShort = 10 * time.Second

	// RequeueIntervalMedium is used for polling in-progress operations.
	RequeueIntervalMedium = 30 * time.Second

	// RequeueIntervalLong is used for rate limit backoff or long-running operations.
	RequeueIntervalLong = 1 * time.Minute

	// RequeueIntervalVeryLong is used for permanent errors that might recover.
	RequeueIntervalVeryLong = 5 * time.Minute

	// PollingIntervalDeployment is used for polling deployment status.
	PollingIntervalDeployment = 30 * time.Second

	// PollingIntervalDNS is used for polling DNS propagation.
	PollingIntervalDNS = 1 * time.Minute
)

// RequeueResult returns a ctrl.Result for requeuing after the specified duration.
func RequeueResult(after time.Duration) ctrl.Result {
	return ctrl.Result{RequeueAfter: after}
}

// RequeueShort returns a result for quick retry.
func RequeueShort() ctrl.Result {
	return ctrl.Result{RequeueAfter: RequeueIntervalShort}
}

// RequeueMedium returns a result for medium-term retry.
func RequeueMedium() ctrl.Result {
	return ctrl.Result{RequeueAfter: RequeueIntervalMedium}
}

// RequeueLong returns a result for long-term retry.
func RequeueLong() ctrl.Result {
	return ctrl.Result{RequeueAfter: RequeueIntervalLong}
}

// RequeueVeryLong returns a result for very long retry.
func RequeueVeryLong() ctrl.Result {
	return ctrl.Result{RequeueAfter: RequeueIntervalVeryLong}
}

// NoRequeue returns a result indicating no requeue needed.
func NoRequeue() ctrl.Result {
	return ctrl.Result{}
}

// RequeueForError returns an appropriate requeue result based on the error type.
// It uses exponential backoff for transient errors.
func RequeueForError(err error, retryCount int) ctrl.Result {
	if err == nil {
		return NoRequeue()
	}

	cfg := cf.RetryConfig{
		BaseDelay:  RequeueIntervalShort,
		MaxDelay:   RequeueIntervalVeryLong,
		MaxRetries: 10,
		RetryCount: retryCount,
	}

	delay := cf.GetRequeueDelay(err, cfg)
	if delay == 0 {
		// NotFound errors don't need requeue
		return NoRequeue()
	}

	return ctrl.Result{RequeueAfter: delay}
}

// ShouldRequeueForError returns true if the error warrants a requeue.
// Permanent errors (NotFound, Auth, Validation) return false.
func ShouldRequeueForError(err error) bool {
	if err == nil {
		return false
	}
	return !cf.IsPermanentError(err)
}

// RequeueWithBackoff returns a result with exponential backoff delay.
func RequeueWithBackoff(baseDelay time.Duration, retryCount int, maxDelay time.Duration) ctrl.Result {
	delay := baseDelay * time.Duration(1<<min(retryCount, 6))
	if delay > maxDelay {
		delay = maxDelay
	}
	return ctrl.Result{RequeueAfter: delay}
}

// IsInProgress checks if the stage indicates an in-progress deployment.
// This is used to determine if polling should continue.
func IsInProgress(stage string) bool {
	switch stage {
	case "queued", "initialize", "clone_repo", "build", "deploy", "pending", "building", "deploying":
		return true
	default:
		return false
	}
}

// IsTerminal checks if the stage indicates a terminal state.
// Terminal states don't require further polling.
func IsTerminal(stage string) bool {
	switch stage {
	case "success", "active", "failure", "failed", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

// IsSuccess checks if the stage indicates a successful completion.
func IsSuccess(stage string) bool {
	return stage == "success" || stage == "active"
}

// IsFailure checks if the stage indicates a failure.
func IsFailure(stage string) bool {
	return stage == "failure" || stage == "failed" || stage == "cancelled" || stage == "canceled"
}
