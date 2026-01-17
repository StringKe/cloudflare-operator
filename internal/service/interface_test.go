// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

func TestSource_String_Namespaced(t *testing.T) {
	source := Source{
		Kind:      "DNSRecord",
		Namespace: "test-ns",
		Name:      "my-record",
	}

	result := source.String()
	assert.Equal(t, "DNSRecord/test-ns/my-record", result)
}

func TestSource_String_ClusterScoped(t *testing.T) {
	source := Source{
		Kind:      "ClusterTunnel",
		Namespace: "",
		Name:      "my-tunnel",
	}

	result := source.String()
	assert.Equal(t, "ClusterTunnel/my-tunnel", result)
}

func TestSource_ToReference(t *testing.T) {
	source := Source{
		Kind:      "Ingress",
		Namespace: "prod",
		Name:      "my-ingress",
	}

	ref := source.ToReference()

	assert.Equal(t, "Ingress", ref.Kind)
	assert.Equal(t, "prod", ref.Namespace)
	assert.Equal(t, "my-ingress", ref.Name)
}

func TestFromReference(t *testing.T) {
	ref := v1alpha2.SourceReference{
		Kind:      "TunnelBinding",
		Namespace: "staging",
		Name:      "my-binding",
	}

	source := FromReference(ref)

	assert.Equal(t, "TunnelBinding", source.Kind)
	assert.Equal(t, "staging", source.Namespace)
	assert.Equal(t, "my-binding", source.Name)
}

func TestFromReference_ClusterScoped(t *testing.T) {
	ref := v1alpha2.SourceReference{
		Kind: "ClusterTunnel",
		Name: "my-tunnel",
	}

	source := FromReference(ref)

	assert.Equal(t, "ClusterTunnel", source.Kind)
	assert.Empty(t, source.Namespace)
	assert.Equal(t, "my-tunnel", source.Name)
}

func TestSource_Roundtrip(t *testing.T) {
	original := Source{
		Kind:      "NetworkRoute",
		Namespace: "system",
		Name:      "route-1",
	}

	ref := original.ToReference()
	restored := FromReference(ref)

	assert.Equal(t, original.Kind, restored.Kind)
	assert.Equal(t, original.Namespace, restored.Namespace)
	assert.Equal(t, original.Name, restored.Name)
	assert.Equal(t, original.String(), restored.String())
}

func TestPriorityConstants(t *testing.T) {
	// Verify priority hierarchy
	assert.Less(t, PriorityTunnel, PriorityBinding, "Tunnel should have higher priority than Binding")
	assert.Less(t, PriorityBinding, PriorityDefault, "Binding should have higher priority than Default")
}

func TestStateConstants(t *testing.T) {
	assert.Equal(t, "Ready", StateReady)
}

func TestRegisterOptions(t *testing.T) {
	opts := RegisterOptions{
		ResourceType: v1alpha2.SyncResourceDNSRecord,
		CloudflareID: "record-123",
		AccountID:    "account-456",
		ZoneID:       "zone-789",
		Source: Source{
			Kind:      "DNSRecord",
			Namespace: "default",
			Name:      "my-record",
		},
		Config:   map[string]string{"name": "test.example.com"},
		Priority: PriorityDefault,
		CredentialsRef: v1alpha2.CredentialsReference{
			Name: "my-creds",
		},
	}

	assert.Equal(t, v1alpha2.SyncResourceDNSRecord, opts.ResourceType)
	assert.Equal(t, "record-123", opts.CloudflareID)
	assert.Equal(t, 100, opts.Priority)
}

func TestUnregisterOptions(t *testing.T) {
	opts := UnregisterOptions{
		ResourceType: v1alpha2.SyncResourceAccessApplication,
		CloudflareID: "app-123",
		Source: Source{
			Kind: "AccessApplication",
			Name: "my-app",
		},
	}

	assert.Equal(t, v1alpha2.SyncResourceAccessApplication, opts.ResourceType)
	assert.Equal(t, "app-123", opts.CloudflareID)
	assert.Equal(t, "AccessApplication/my-app", opts.Source.String())
}
