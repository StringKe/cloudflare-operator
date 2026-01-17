// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package testutil

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/StringKe/cloudflare-operator/api/v1alpha1"
	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// TestNamespace is the default namespace for tests.
const TestNamespace = "test-namespace"

// SystemNamespace is the operator system namespace.
const SystemNamespace = "cloudflare-operator-system"

// DefaultAccountID is the default test account ID.
const DefaultAccountID = "test-account-id"

// DefaultZoneID is the default test zone ID.
const DefaultZoneID = "test-zone-id"

// DefaultZoneName is the default test zone name.
const DefaultZoneName = "example.com"

// Fixtures provides pre-built test resources.
type Fixtures struct {
	Namespace string
}

// NewFixtures creates a new Fixtures instance.
func NewFixtures() *Fixtures {
	return &Fixtures{
		Namespace: TestNamespace,
	}
}

// WithNamespace sets the namespace for fixtures.
func (f *Fixtures) WithNamespace(ns string) *Fixtures {
	f.Namespace = ns
	return f
}

// TestNamespaceObj returns a test namespace object.
func (f *Fixtures) TestNamespaceObj() *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: f.Namespace,
		},
	}
}

// SystemNamespaceObj returns the system namespace object.
func (f *Fixtures) SystemNamespaceObj() *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: SystemNamespace,
		},
	}
}

// DefaultCredentials returns default CloudflareCredentials for testing.
func (f *Fixtures) DefaultCredentials() *v1alpha2.CloudflareCredentials {
	return NewCloudflareCredentialsBuilder("default-credentials").
		WithAccountID(DefaultAccountID).
		WithAPIToken("cloudflare-api-token", "token").
		Build()
}

// DefaultCredentialsSecret returns the secret for default credentials.
func (f *Fixtures) DefaultCredentialsSecret() *corev1.Secret {
	return NewSecretBuilder("cloudflare-api-token", SystemNamespace).
		WithStringData("token", "test-api-token").
		Build()
}

// SimpleTunnel returns a simple Tunnel resource.
func (f *Fixtures) SimpleTunnel(name string) *v1alpha2.Tunnel {
	return NewTunnelBuilder(name, f.Namespace).
		WithCredentialsRef("default-credentials").
		WithDomain(DefaultZoneName).
		Build()
}

// SimpleClusterTunnel returns a simple ClusterTunnel resource.
func (f *Fixtures) SimpleClusterTunnel(name string) *v1alpha2.ClusterTunnel {
	return NewClusterTunnelBuilder(name).
		WithCredentialsRef("default-credentials").
		WithDomain(DefaultZoneName).
		Build()
}

// SimpleDNSRecord returns a simple DNS record.
func (f *Fixtures) SimpleDNSRecord(name, dnsName, content string) *v1alpha2.DNSRecord {
	return NewDNSRecordBuilder(name, f.Namespace).
		WithType("A").
		WithName(dnsName).
		WithContent(content).
		WithProxied(true).
		WithCredentialsRef("default-credentials").
		Build()
}

// CNAMERecord returns a CNAME DNS record pointing to a tunnel.
func (f *Fixtures) CNAMERecord(name, hostname, tunnelID string) *v1alpha2.DNSRecord {
	return NewDNSRecordBuilder(name, f.Namespace).
		WithType("CNAME").
		WithName(hostname).
		WithContent(tunnelID + ".cfargotunnel.com").
		WithProxied(true).
		WithCredentialsRef("default-credentials").
		Build()
}

// SimpleVirtualNetwork returns a simple VirtualNetwork resource.
func (f *Fixtures) SimpleVirtualNetwork(name string) *v1alpha2.VirtualNetwork {
	return NewVirtualNetworkBuilder(name).
		WithComment("Test virtual network").
		WithCredentialsRef("default-credentials").
		Build()
}

// DefaultVirtualNetwork returns a default VirtualNetwork resource.
func (f *Fixtures) DefaultVirtualNetwork(name string) *v1alpha2.VirtualNetwork {
	return NewVirtualNetworkBuilder(name).
		WithComment("Default virtual network").
		WithIsDefault(true).
		WithCredentialsRef("default-credentials").
		Build()
}

// SimpleNetworkRoute returns a simple NetworkRoute resource.
func (f *Fixtures) SimpleNetworkRoute(name, network, tunnelName string) *v1alpha2.NetworkRoute {
	return NewNetworkRouteBuilder(name).
		WithNetwork(network).
		WithTunnelRef(tunnelName).
		WithComment("Test network route").
		WithCredentialsRef("default-credentials").
		Build()
}

// NetworkRouteWithVNet returns a NetworkRoute with virtual network reference.
func (f *Fixtures) NetworkRouteWithVNet(name, network, tunnelName, vnetName string) *v1alpha2.NetworkRoute {
	return NewNetworkRouteBuilder(name).
		WithNetwork(network).
		WithTunnelRef(tunnelName).
		WithVirtualNetworkRef(vnetName).
		WithComment("Test network route with vnet").
		WithCredentialsRef("default-credentials").
		Build()
}

// SimpleAccessApplication returns a simple AccessApplication resource.
func (f *Fixtures) SimpleAccessApplication(name, domain string) *v1alpha2.AccessApplication {
	return NewAccessApplicationBuilder(name).
		WithDomain(domain).
		WithType("self_hosted").
		WithSessionDuration("24h").
		WithCredentialsRef("default-credentials").
		Build()
}

// SimpleTunnelBinding returns a simple TunnelBinding resource (v1alpha1).
func (f *Fixtures) SimpleTunnelBinding(name, tunnelName, serviceName, fqdn string) *v1alpha1.TunnelBinding {
	return NewTunnelBindingBuilder(name, f.Namespace).
		WithTunnelRef(tunnelName, "Tunnel").
		WithSubject(serviceName, fqdn).
		Build()
}

// Service returns a Kubernetes Service for testing.
func (f *Fixtures) Service(name string, port int32) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: f.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port: port,
				},
			},
		},
	}
}

// ServiceWithSelector returns a Kubernetes Service with selector for testing.
func (f *Fixtures) ServiceWithSelector(name string, port int32, selector map[string]string) *corev1.Service {
	svc := f.Service(name, port)
	svc.Spec.Selector = selector
	return svc
}
