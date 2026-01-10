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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// EnsureFinalizer ensures the finalizer is present on the object.
// It uses UpdateWithConflictRetry to handle concurrent modifications.
// Returns true if the finalizer was added (object was updated), false if already present.
func EnsureFinalizer(ctx context.Context, c client.Client, obj client.Object, finalizerName string) (bool, error) {
	if controllerutil.ContainsFinalizer(obj, finalizerName) {
		return false, nil
	}

	err := UpdateWithConflictRetry(ctx, c, obj, func() {
		controllerutil.AddFinalizer(obj, finalizerName)
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// RemoveFinalizerSafely removes the finalizer from the object safely.
// It uses UpdateWithConflictRetry to handle concurrent modifications.
// Returns true if the finalizer was removed (object was updated), false if not present.
func RemoveFinalizerSafely(ctx context.Context, c client.Client, obj client.Object, finalizerName string) (bool, error) {
	if !controllerutil.ContainsFinalizer(obj, finalizerName) {
		return false, nil
	}

	err := UpdateWithConflictRetry(ctx, c, obj, func() {
		controllerutil.RemoveFinalizer(obj, finalizerName)
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// HasFinalizer checks if the object has the specified finalizer
func HasFinalizer(obj client.Object, finalizerName string) bool {
	return controllerutil.ContainsFinalizer(obj, finalizerName)
}

// IsBeingDeleted checks if the object is being deleted (has deletion timestamp)
func IsBeingDeleted(obj client.Object) bool {
	return obj.GetDeletionTimestamp() != nil
}

// ShouldReconcileDeletion returns true if the object is being deleted and has the finalizer
func ShouldReconcileDeletion(obj client.Object, finalizerName string) bool {
	return IsBeingDeleted(obj) && HasFinalizer(obj, finalizerName)
}
