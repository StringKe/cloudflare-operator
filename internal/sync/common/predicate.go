// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package common

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// PredicateForResourceType creates a predicate that filters CloudflareSyncState
// events by resource type. This is the standard predicate used by all Sync Controllers
// to ensure they only process SyncStates of their specific type.
//
// Usage:
//
//	ctrl.NewControllerManagedBy(mgr).
//	    For(&v1alpha2.CloudflareSyncState{}).
//	    WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourceDNSRecord)).
//	    Complete(r)
func PredicateForResourceType(resourceType v1alpha2.SyncResourceType) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			syncState, ok := e.Object.(*v1alpha2.CloudflareSyncState)
			if !ok {
				return false
			}
			return syncState.Spec.ResourceType == resourceType
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			syncState, ok := e.ObjectNew.(*v1alpha2.CloudflareSyncState)
			if !ok {
				return false
			}
			return syncState.Spec.ResourceType == resourceType
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			syncState, ok := e.Object.(*v1alpha2.CloudflareSyncState)
			if !ok {
				return false
			}
			return syncState.Spec.ResourceType == resourceType
		},
	}
}
