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
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

// DefaultRequeueAfter is the default requeue duration after an error
const DefaultRequeueAfter = 30 * time.Second

// DeletionHandler handles the standard deletion flow for resources
type DeletionHandler struct {
	Client        client.Client
	Log           logr.Logger
	Recorder      record.EventRecorder
	FinalizerName string
}

// NewDeletionHandler creates a new DeletionHandler
func NewDeletionHandler(c client.Client, log logr.Logger, recorder record.EventRecorder, finalizerName string) *DeletionHandler {
	return &DeletionHandler{
		Client:        c,
		Log:           log,
		Recorder:      recorder,
		FinalizerName: finalizerName,
	}
}

// HandleDeletion performs the standard deletion workflow:
// 1. Check if finalizer is present
// 2. Execute the delete function (to clean up external resources)
// 3. Remove the finalizer
//
// The deleteFn should handle NotFound errors gracefully (return nil if already deleted).
// Returns (result, requeue, error) where:
// - result is the reconcile result
// - requeue indicates if reconciliation should be requeued
// - error is any error that occurred
//
//nolint:revive // cognitive-complexity is acceptable for deletion handling pattern
func (h *DeletionHandler) HandleDeletion(
	ctx context.Context,
	obj client.Object,
	deleteFn func() error,
) (ctrl.Result, bool, error) {
	// Check if we need to handle deletion
	if !ShouldReconcileDeletion(obj, h.FinalizerName) {
		return ctrl.Result{}, false, nil
	}

	h.Log.Info("Handling deletion")

	// Execute the delete function (clean up external resources)
	if deleteFn != nil {
		if err := deleteFn(); err != nil {
			// Check if the error is NotFound - this means the resource is already deleted
			if !cf.IsNotFoundError(err) {
				h.Log.Error(err, "Failed to delete external resource")
				RecordError(h.Recorder, obj.(runtime.Object), EventReasonDeleteFailed, err)
				return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, true, err
			}
			h.Log.Info("External resource already deleted")
		}
	}

	// Remove the finalizer
	removed, err := RemoveFinalizerSafely(ctx, h.Client, obj, h.FinalizerName)
	if err != nil {
		h.Log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, true, err
	}

	if removed {
		h.Log.Info("Finalizer removed")
		RecordSuccess(h.Recorder, obj.(runtime.Object), EventReasonFinalizerRemoved, "Finalizer removed successfully")
	}

	return ctrl.Result{}, false, nil
}

// HandleDeletionWithMultipleResources handles deletion when multiple external resources need to be cleaned up.
// It aggregates errors and only removes the finalizer if all deletions succeed.
//
//nolint:revive // cognitive-complexity is acceptable for deletion handling pattern
func (h *DeletionHandler) HandleDeletionWithMultipleResources(
	ctx context.Context,
	obj client.Object,
	deleteFns []func() error,
) (ctrl.Result, bool, error) {
	// Check if we need to handle deletion
	if !ShouldReconcileDeletion(obj, h.FinalizerName) {
		return ctrl.Result{}, false, nil
	}

	h.Log.Info("Handling deletion with multiple resources", "count", len(deleteFns))

	// Execute all delete functions, collecting errors
	var errs []error
	for i, deleteFn := range deleteFns {
		if deleteFn == nil {
			continue
		}
		if err := deleteFn(); err != nil {
			if !cf.IsNotFoundError(err) {
				h.Log.Error(err, "Failed to delete external resource", "index", i)
				errs = append(errs, err)
			} else {
				h.Log.V(1).Info("External resource already deleted", "index", i)
			}
		}
	}

	// If any deletion failed, don't remove the finalizer
	if len(errs) > 0 {
		RecordError(h.Recorder, obj.(runtime.Object), EventReasonDeleteFailed, errs[0])
		return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, true, errs[0]
	}

	// All deletions succeeded, remove the finalizer
	removed, err := RemoveFinalizerSafely(ctx, h.Client, obj, h.FinalizerName)
	if err != nil {
		h.Log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, true, err
	}

	if removed {
		h.Log.Info("Finalizer removed")
		RecordSuccess(h.Recorder, obj.(runtime.Object), EventReasonFinalizerRemoved, "Finalizer removed successfully")
	}

	return ctrl.Result{}, false, nil
}

// QuickHandleDeletion is a convenience function for simple deletion scenarios
// It combines HandleDeletion with finalizer checking
func QuickHandleDeletion(
	ctx context.Context,
	c client.Client,
	log logr.Logger,
	recorder record.EventRecorder,
	obj client.Object,
	finalizerName string,
	deleteFn func() error,
) (ctrl.Result, bool, error) {
	handler := NewDeletionHandler(c, log, recorder, finalizerName)
	return handler.HandleDeletion(ctx, obj, deleteFn)
}
