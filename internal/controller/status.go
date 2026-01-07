/*
Copyright 2025 Adyanth H.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultMaxRetries is the default number of retries for status updates
	DefaultMaxRetries = 5

	// DefaultRetryDelay is the default delay between retries
	DefaultRetryDelay = 100 * time.Millisecond
)

// StatusUpdater provides utilities for updating resource status with retry logic
type StatusUpdater struct {
	Client     client.Client
	MaxRetries int
	RetryDelay time.Duration
}

// NewStatusUpdater creates a new StatusUpdater with default settings
func NewStatusUpdater(c client.Client) *StatusUpdater {
	return &StatusUpdater{
		Client:     c,
		MaxRetries: DefaultMaxRetries,
		RetryDelay: DefaultRetryDelay,
	}
}

// refetchObject re-fetches the object to get the latest ResourceVersion
func (u *StatusUpdater) refetchObject(ctx context.Context, obj client.Object) error {
	if err := u.Client.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		return fmt.Errorf("failed to get latest object version: %w", err)
	}
	time.Sleep(u.RetryDelay)
	return nil
}

// retryLoop executes fn with retry on conflict, refetching obj between attempts
//
//nolint:revive // retry loop complexity is inherent to the logic
func (u *StatusUpdater) retryLoop(ctx context.Context, obj client.Object, fn func() error) error {
	var lastErr error
	for i := 0; i < u.MaxRetries; i++ {
		if i > 0 {
			if err := u.refetchObject(ctx, obj); err != nil {
				return err
			}
		}
		err := fn()
		if err == nil {
			return nil
		}
		if !apierrors.IsConflict(err) {
			return err
		}
		lastErr = err
	}
	return fmt.Errorf("operation failed after %d retries: %w", u.MaxRetries, lastErr)
}

// UpdateStatusWithRetry updates the status of an object with retry on conflict
func (u *StatusUpdater) UpdateStatusWithRetry(ctx context.Context, obj client.Object, updateFn func()) error {
	return u.retryLoop(ctx, obj, func() error {
		updateFn()
		return u.Client.Status().Update(ctx, obj)
	})
}

// UpdateWithRetry updates an object with retry on conflict
func (u *StatusUpdater) UpdateWithRetry(ctx context.Context, obj client.Object, updateFn func()) error {
	return u.retryLoop(ctx, obj, func() error {
		updateFn()
		return u.Client.Update(ctx, obj)
	})
}

// SetCondition is a helper to set a condition on a resource
// It handles the common pattern of setting conditions with proper timestamps
func SetCondition(conditions *[]metav1.Condition, conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// SetReadyCondition is a shorthand for setting the Ready condition
func SetReadyCondition(conditions *[]metav1.Condition, status metav1.ConditionStatus, reason, message string) {
	SetCondition(conditions, "Ready", status, reason, message)
}

// SetErrorCondition sets the Ready condition to False with an error reason
func SetErrorCondition(conditions *[]metav1.Condition, err error) {
	message := "Unknown error"
	if err != nil {
		message = err.Error()
		// Truncate long error messages
		if len(message) > 1024 {
			message = message[:1021] + "..."
		}
	}
	SetCondition(conditions, "Ready", metav1.ConditionFalse, "ReconcileError", message)
}

// SetSuccessCondition sets the Ready condition to True
func SetSuccessCondition(conditions *[]metav1.Condition, message string) {
	SetCondition(conditions, "Ready", metav1.ConditionTrue, "Reconciled", message)
}

// State constants for consistent state management across controllers
const (
	StatePending  = "Pending"
	StateCreating = "Creating"
	StateActive   = "Active"
	StateReady    = "Ready"
	StateError    = "Error"
	StateDeleting = "Deleting"
	StateWarning  = "Warning"
)

// IsTerminalState returns true if the state is a terminal state
func IsTerminalState(state string) bool {
	return state == StateActive || state == StateReady || state == StateError
}

// RetryOnConflict retries a function that may return a conflict error
// This is useful for status updates where optimistic locking may fail
func RetryOnConflict(ctx context.Context, c client.Client, obj client.Object, fn func() error) error {
	updater := &StatusUpdater{Client: c, MaxRetries: DefaultMaxRetries, RetryDelay: DefaultRetryDelay}
	return updater.retryLoop(ctx, obj, fn)
}

// UpdateStatusWithConflictRetry is a convenience function that updates status with retry on conflict
func UpdateStatusWithConflictRetry(ctx context.Context, c client.Client, obj client.Object, updateFn func()) error {
	return RetryOnConflict(ctx, c, obj, func() error {
		updateFn()
		return c.Status().Update(ctx, obj)
	})
}

// UpdateWithConflictRetry is a convenience function that updates object with retry on conflict
func UpdateWithConflictRetry(ctx context.Context, c client.Client, obj client.Object, updateFn func()) error {
	return RetryOnConflict(ctx, c, obj, func() error {
		updateFn()
		return c.Update(ctx, obj)
	})
}
