// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

func createSyncStateWithType(resourceType v1alpha2.SyncResourceType) *v1alpha2.CloudflareSyncState {
	return &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sync-state",
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType: resourceType,
			CloudflareID: "test-id",
			AccountID:    "test-account",
		},
	}
}

func TestPredicateForResourceType_CreateEvent_Matches(t *testing.T) {
	pred := PredicateForResourceType(v1alpha2.SyncResourceDNSRecord)
	syncState := createSyncStateWithType(v1alpha2.SyncResourceDNSRecord)

	e := event.CreateEvent{Object: syncState}

	assert.True(t, pred.Create(e))
}

func TestPredicateForResourceType_CreateEvent_NoMatch(t *testing.T) {
	pred := PredicateForResourceType(v1alpha2.SyncResourceDNSRecord)
	syncState := createSyncStateWithType(v1alpha2.SyncResourceAccessApplication)

	e := event.CreateEvent{Object: syncState}

	assert.False(t, pred.Create(e))
}

func TestPredicateForResourceType_CreateEvent_WrongType(t *testing.T) {
	pred := PredicateForResourceType(v1alpha2.SyncResourceDNSRecord)

	// Use a different object type (not CloudflareSyncState)
	tunnel := &v1alpha2.Tunnel{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
	}

	e := event.CreateEvent{Object: tunnel}

	assert.False(t, pred.Create(e))
}

func TestPredicateForResourceType_UpdateEvent_Matches(t *testing.T) {
	pred := PredicateForResourceType(v1alpha2.SyncResourceTunnelConfiguration)
	oldSyncState := createSyncStateWithType(v1alpha2.SyncResourceTunnelConfiguration)
	newSyncState := createSyncStateWithType(v1alpha2.SyncResourceTunnelConfiguration)

	e := event.UpdateEvent{ObjectOld: oldSyncState, ObjectNew: newSyncState}

	assert.True(t, pred.Update(e))
}

func TestPredicateForResourceType_UpdateEvent_NoMatch(t *testing.T) {
	pred := PredicateForResourceType(v1alpha2.SyncResourceTunnelConfiguration)
	oldSyncState := createSyncStateWithType(v1alpha2.SyncResourceAccessGroup)
	newSyncState := createSyncStateWithType(v1alpha2.SyncResourceAccessGroup)

	e := event.UpdateEvent{ObjectOld: oldSyncState, ObjectNew: newSyncState}

	assert.False(t, pred.Update(e))
}

func TestPredicateForResourceType_UpdateEvent_WrongType(t *testing.T) {
	pred := PredicateForResourceType(v1alpha2.SyncResourceDNSRecord)

	tunnel := &v1alpha2.Tunnel{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
	}

	e := event.UpdateEvent{ObjectOld: tunnel, ObjectNew: tunnel}

	assert.False(t, pred.Update(e))
}

func TestPredicateForResourceType_DeleteEvent_Matches(t *testing.T) {
	pred := PredicateForResourceType(v1alpha2.SyncResourceGatewayRule)
	syncState := createSyncStateWithType(v1alpha2.SyncResourceGatewayRule)

	e := event.DeleteEvent{Object: syncState}

	assert.True(t, pred.Delete(e))
}

func TestPredicateForResourceType_DeleteEvent_NoMatch(t *testing.T) {
	pred := PredicateForResourceType(v1alpha2.SyncResourceGatewayRule)
	syncState := createSyncStateWithType(v1alpha2.SyncResourceR2Bucket)

	e := event.DeleteEvent{Object: syncState}

	assert.False(t, pred.Delete(e))
}

func TestPredicateForResourceType_DeleteEvent_WrongType(t *testing.T) {
	pred := PredicateForResourceType(v1alpha2.SyncResourceDNSRecord)

	tunnel := &v1alpha2.Tunnel{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
	}

	e := event.DeleteEvent{Object: tunnel}

	assert.False(t, pred.Delete(e))
}

func TestPredicateForResourceType_AllResourceTypes(t *testing.T) {
	resourceTypes := []v1alpha2.SyncResourceType{
		v1alpha2.SyncResourceTunnelConfiguration,
		v1alpha2.SyncResourceDNSRecord,
		v1alpha2.SyncResourceAccessApplication,
		v1alpha2.SyncResourceAccessGroup,
		v1alpha2.SyncResourceAccessServiceToken,
		v1alpha2.SyncResourceAccessIdentityProvider,
		v1alpha2.SyncResourceVirtualNetwork,
		v1alpha2.SyncResourceNetworkRoute,
		v1alpha2.SyncResourceR2Bucket,
		v1alpha2.SyncResourceR2BucketDomain,
		v1alpha2.SyncResourceR2BucketNotification,
		v1alpha2.SyncResourceZoneRuleset,
		v1alpha2.SyncResourceTransformRule,
		v1alpha2.SyncResourceRedirectRule,
		v1alpha2.SyncResourceGatewayRule,
		v1alpha2.SyncResourceGatewayList,
		v1alpha2.SyncResourceGatewayConfiguration,
		v1alpha2.SyncResourceOriginCACertificate,
		v1alpha2.SyncResourceCloudflareDomain,
		v1alpha2.SyncResourceDomainRegistration,
		v1alpha2.SyncResourceDevicePostureRule,
		v1alpha2.SyncResourceDeviceSettingsPolicy,
	}

	for _, rt := range resourceTypes {
		t.Run(string(rt), func(t *testing.T) {
			pred := PredicateForResourceType(rt)
			syncState := createSyncStateWithType(rt)

			// Should match the correct type
			createEvent := event.CreateEvent{Object: syncState}
			assert.True(t, pred.Create(createEvent), "Create event should match for %s", rt)

			// Should not match other types
			for _, otherType := range resourceTypes {
				if otherType != rt {
					otherSyncState := createSyncStateWithType(otherType)
					otherEvent := event.CreateEvent{Object: otherSyncState}
					assert.False(t, pred.Create(otherEvent),
						"Create event should not match %s when filtering for %s", otherType, rt)
					break // Just check one other type to keep test fast
				}
			}
		})
	}
}

func TestPredicateForResourceType_GenericEvent(t *testing.T) {
	pred := PredicateForResourceType(v1alpha2.SyncResourceDNSRecord)

	// GenericFunc is not set, so it should use default behavior
	// The default behavior is to return true
	genericE := event.GenericEvent{
		Object: createSyncStateWithType(v1alpha2.SyncResourceDNSRecord),
	}

	// Generic events should pass through by default
	// since we didn't set GenericFunc
	assert.True(t, pred.Generic(genericE))
}
