// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package common provides shared utilities for controllers in the simplified 3-layer architecture.
//
// # Architecture Overview
//
// The new architecture simplifies the operator from 6 layers to 3 layers:
//
//	Old: CRD → Controller → Service → SyncState → SyncController → CF API
//	New: CRD → Controller → CF API
//
// # Key Components
//
//   - APIClientFactory: Creates and manages Cloudflare API clients
//   - Requeue utilities: Standard intervals and backoff for reconciliation
//   - Re-exports from parent controller package: Status, Finalizer, Event, Deletion utilities
//
// # Usage Pattern
//
// Controllers should follow this pattern:
//
//	func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
//	    obj := &v1alpha2.MyResource{}
//	    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
//	        return ctrl.Result{}, client.IgnoreNotFound(err)
//	    }
//
//	    // Handle deletion
//	    if !obj.DeletionTimestamp.IsZero() {
//	        return r.handleDeletion(ctx, obj)
//	    }
//
//	    // Get API client
//	    apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
//	        CloudflareDetails: &obj.Spec.Cloudflare,
//	        Namespace:         obj.Namespace,
//	        StatusAccountID:   obj.Status.AccountID,
//	    })
//	    if err != nil {
//	        return r.setErrorStatus(ctx, obj, err)
//	    }
//
//	    // Ensure finalizer
//	    if added, err := controller.EnsureFinalizer(ctx, r.Client, obj, FinalizerName); err != nil {
//	        return ctrl.Result{}, err
//	    } else if added {
//	        return ctrl.Result{Requeue: true}, nil
//	    }
//
//	    // Direct API call
//	    result, err := apiResult.API.SomeOperation(ctx, ...)
//	    if err != nil {
//	        return r.setErrorStatus(ctx, obj, err)
//	    }
//
//	    // Direct status write (no intermediate layer!)
//	    return r.setSuccessStatus(ctx, obj, result)
//	}
//
// # Benefits
//
//   - Simpler data flow: One CRD → One controller → Direct API call
//   - No intermediate SyncState: Status written directly to original CRD
//   - Independent polling: Each controller has its own Informer, no interference
//   - Less code: ~13,000 lines removed (service + sync layers)
//   - Easier debugging: Direct data path, no need to correlate multiple resources
package common
