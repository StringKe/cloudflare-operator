// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package testutil

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/StringKe/cloudflare-operator/api/v1alpha1"
	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// TunnelBuilder builds Tunnel resources for testing.
type TunnelBuilder struct {
	tunnel *v1alpha2.Tunnel
}

// NewTunnelBuilder creates a new TunnelBuilder.
func NewTunnelBuilder(name, namespace string) *TunnelBuilder {
	return &TunnelBuilder{
		tunnel: &v1alpha2.Tunnel{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "networking.cloudflare-operator.io/v1alpha2",
				Kind:       "Tunnel",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				UID:       types.UID(fmt.Sprintf("tunnel-%s-%d", name, time.Now().UnixNano())),
			},
			Spec: v1alpha2.TunnelSpec{
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: "default-credentials",
					},
				},
			},
		},
	}
}

// WithCredentialsRef sets the credentials reference.
func (b *TunnelBuilder) WithCredentialsRef(name string) *TunnelBuilder {
	b.tunnel.Spec.Cloudflare.CredentialsRef = &v1alpha2.CloudflareCredentialsRef{Name: name}
	return b
}

// WithCloudflareID sets the Cloudflare tunnel ID.
func (b *TunnelBuilder) WithCloudflareID(id string) *TunnelBuilder {
	b.tunnel.Status.TunnelId = id
	return b
}

// WithFinalizer adds a finalizer.
func (b *TunnelBuilder) WithFinalizer(name string) *TunnelBuilder {
	b.tunnel.Finalizers = append(b.tunnel.Finalizers, name)
	return b
}

// WithDeletionTimestamp marks the resource for deletion.
func (b *TunnelBuilder) WithDeletionTimestamp() *TunnelBuilder {
	now := metav1.Now()
	b.tunnel.DeletionTimestamp = &now
	return b
}

// WithDomain sets the Cloudflare domain.
func (b *TunnelBuilder) WithDomain(domain string) *TunnelBuilder {
	b.tunnel.Spec.Cloudflare.Domain = domain
	return b
}

// Build returns the constructed Tunnel.
func (b *TunnelBuilder) Build() *v1alpha2.Tunnel {
	return b.tunnel.DeepCopy()
}

// ClusterTunnelBuilder builds ClusterTunnel resources for testing.
type ClusterTunnelBuilder struct {
	tunnel *v1alpha2.ClusterTunnel
}

// NewClusterTunnelBuilder creates a new ClusterTunnelBuilder.
func NewClusterTunnelBuilder(name string) *ClusterTunnelBuilder {
	return &ClusterTunnelBuilder{
		tunnel: &v1alpha2.ClusterTunnel{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "networking.cloudflare-operator.io/v1alpha2",
				Kind:       "ClusterTunnel",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				UID:  types.UID(fmt.Sprintf("clustertunnel-%s-%d", name, time.Now().UnixNano())),
			},
			Spec: v1alpha2.TunnelSpec{
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: "default-credentials",
					},
				},
			},
		},
	}
}

// WithCredentialsRef sets the credentials reference.
func (b *ClusterTunnelBuilder) WithCredentialsRef(name string) *ClusterTunnelBuilder {
	b.tunnel.Spec.Cloudflare.CredentialsRef = &v1alpha2.CloudflareCredentialsRef{Name: name}
	return b
}

// WithCloudflareID sets the Cloudflare tunnel ID.
func (b *ClusterTunnelBuilder) WithCloudflareID(id string) *ClusterTunnelBuilder {
	b.tunnel.Status.TunnelId = id
	return b
}

// WithFinalizer adds a finalizer.
func (b *ClusterTunnelBuilder) WithFinalizer(name string) *ClusterTunnelBuilder {
	b.tunnel.Finalizers = append(b.tunnel.Finalizers, name)
	return b
}

// WithDeletionTimestamp marks the resource for deletion.
func (b *ClusterTunnelBuilder) WithDeletionTimestamp() *ClusterTunnelBuilder {
	now := metav1.Now()
	b.tunnel.DeletionTimestamp = &now
	return b
}

// WithDomain sets the Cloudflare domain.
func (b *ClusterTunnelBuilder) WithDomain(domain string) *ClusterTunnelBuilder {
	b.tunnel.Spec.Cloudflare.Domain = domain
	return b
}

// Build returns the constructed ClusterTunnel.
func (b *ClusterTunnelBuilder) Build() *v1alpha2.ClusterTunnel {
	return b.tunnel.DeepCopy()
}

// DNSRecordBuilder builds DNSRecord resources for testing.
type DNSRecordBuilder struct {
	record *v1alpha2.DNSRecord
}

// NewDNSRecordBuilder creates a new DNSRecordBuilder.
func NewDNSRecordBuilder(name, namespace string) *DNSRecordBuilder {
	return &DNSRecordBuilder{
		record: &v1alpha2.DNSRecord{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "networking.cloudflare-operator.io/v1alpha2",
				Kind:       "DNSRecord",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				UID:       types.UID(fmt.Sprintf("dnsrecord-%s-%d", name, time.Now().UnixNano())),
			},
			Spec: v1alpha2.DNSRecordSpec{
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: "default-credentials",
					},
				},
			},
		},
	}
}

// WithType sets the record type.
func (b *DNSRecordBuilder) WithType(recordType string) *DNSRecordBuilder {
	b.record.Spec.Type = recordType
	return b
}

// WithName sets the DNS name.
func (b *DNSRecordBuilder) WithName(name string) *DNSRecordBuilder {
	b.record.Spec.Name = name
	return b
}

// WithContent sets the record content.
func (b *DNSRecordBuilder) WithContent(content string) *DNSRecordBuilder {
	b.record.Spec.Content = content
	return b
}

// WithProxied sets whether the record is proxied.
func (b *DNSRecordBuilder) WithProxied(proxied bool) *DNSRecordBuilder {
	b.record.Spec.Proxied = proxied
	return b
}

// WithTTL sets the TTL.
func (b *DNSRecordBuilder) WithTTL(ttl int) *DNSRecordBuilder {
	b.record.Spec.TTL = ttl
	return b
}

// WithCredentialsRef sets the credentials reference.
func (b *DNSRecordBuilder) WithCredentialsRef(name string) *DNSRecordBuilder {
	b.record.Spec.Cloudflare.CredentialsRef = &v1alpha2.CloudflareCredentialsRef{Name: name}
	return b
}

// WithFinalizer adds a finalizer.
func (b *DNSRecordBuilder) WithFinalizer(name string) *DNSRecordBuilder {
	b.record.Finalizers = append(b.record.Finalizers, name)
	return b
}

// WithDeletionTimestamp marks the resource for deletion.
func (b *DNSRecordBuilder) WithDeletionTimestamp() *DNSRecordBuilder {
	now := metav1.Now()
	b.record.DeletionTimestamp = &now
	return b
}

// Build returns the constructed DNSRecord.
func (b *DNSRecordBuilder) Build() *v1alpha2.DNSRecord {
	return b.record.DeepCopy()
}

// VirtualNetworkBuilder builds VirtualNetwork resources for testing.
type VirtualNetworkBuilder struct {
	vnet *v1alpha2.VirtualNetwork
}

// NewVirtualNetworkBuilder creates a new VirtualNetworkBuilder.
func NewVirtualNetworkBuilder(name string) *VirtualNetworkBuilder {
	return &VirtualNetworkBuilder{
		vnet: &v1alpha2.VirtualNetwork{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "networking.cloudflare-operator.io/v1alpha2",
				Kind:       "VirtualNetwork",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				UID:  types.UID(fmt.Sprintf("vnet-%s-%d", name, time.Now().UnixNano())),
			},
			Spec: v1alpha2.VirtualNetworkSpec{
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: "default-credentials",
					},
				},
			},
		},
	}
}

// WithComment sets the comment.
func (b *VirtualNetworkBuilder) WithComment(comment string) *VirtualNetworkBuilder {
	b.vnet.Spec.Comment = comment
	return b
}

// WithIsDefault sets whether this is the default network.
func (b *VirtualNetworkBuilder) WithIsDefault(isDefault bool) *VirtualNetworkBuilder {
	b.vnet.Spec.IsDefaultNetwork = isDefault
	return b
}

// WithCredentialsRef sets the credentials reference.
func (b *VirtualNetworkBuilder) WithCredentialsRef(name string) *VirtualNetworkBuilder {
	b.vnet.Spec.Cloudflare.CredentialsRef = &v1alpha2.CloudflareCredentialsRef{Name: name}
	return b
}

// WithFinalizer adds a finalizer.
func (b *VirtualNetworkBuilder) WithFinalizer(name string) *VirtualNetworkBuilder {
	b.vnet.Finalizers = append(b.vnet.Finalizers, name)
	return b
}

// WithCloudflareID sets the Cloudflare virtual network ID.
func (b *VirtualNetworkBuilder) WithCloudflareID(id string) *VirtualNetworkBuilder {
	b.vnet.Status.VirtualNetworkId = id
	return b
}

// WithDeletionTimestamp marks the resource for deletion.
func (b *VirtualNetworkBuilder) WithDeletionTimestamp() *VirtualNetworkBuilder {
	now := metav1.Now()
	b.vnet.DeletionTimestamp = &now
	return b
}

// Build returns the constructed VirtualNetwork.
func (b *VirtualNetworkBuilder) Build() *v1alpha2.VirtualNetwork {
	return b.vnet.DeepCopy()
}

// NetworkRouteBuilder builds NetworkRoute resources for testing.
type NetworkRouteBuilder struct {
	route *v1alpha2.NetworkRoute
}

// NewNetworkRouteBuilder creates a new NetworkRouteBuilder.
func NewNetworkRouteBuilder(name string) *NetworkRouteBuilder {
	return &NetworkRouteBuilder{
		route: &v1alpha2.NetworkRoute{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "networking.cloudflare-operator.io/v1alpha2",
				Kind:       "NetworkRoute",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				UID:  types.UID(fmt.Sprintf("networkroute-%s-%d", name, time.Now().UnixNano())),
			},
			Spec: v1alpha2.NetworkRouteSpec{
				TunnelRef: v1alpha2.TunnelRef{
					Kind: "ClusterTunnel",
					Name: "default-tunnel",
				},
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: "default-credentials",
					},
				},
			},
		},
	}
}

// WithNetwork sets the CIDR network.
func (b *NetworkRouteBuilder) WithNetwork(network string) *NetworkRouteBuilder {
	b.route.Spec.Network = network
	return b
}

// WithComment sets the comment.
func (b *NetworkRouteBuilder) WithComment(comment string) *NetworkRouteBuilder {
	b.route.Spec.Comment = comment
	return b
}

// WithTunnelRef sets the tunnel reference.
func (b *NetworkRouteBuilder) WithTunnelRef(name string) *NetworkRouteBuilder {
	b.route.Spec.TunnelRef = v1alpha2.TunnelRef{
		Name: name,
		Kind: "ClusterTunnel",
	}
	return b
}

// WithTunnelRefKind sets the tunnel reference with kind.
func (b *NetworkRouteBuilder) WithTunnelRefKind(name, kind string) *NetworkRouteBuilder {
	b.route.Spec.TunnelRef = v1alpha2.TunnelRef{
		Name: name,
		Kind: kind,
	}
	return b
}

// WithVirtualNetworkRef sets the virtual network reference.
func (b *NetworkRouteBuilder) WithVirtualNetworkRef(name string) *NetworkRouteBuilder {
	b.route.Spec.VirtualNetworkRef = &v1alpha2.VirtualNetworkRef{
		Name: name,
	}
	return b
}

// WithCredentialsRef sets the credentials reference.
func (b *NetworkRouteBuilder) WithCredentialsRef(name string) *NetworkRouteBuilder {
	b.route.Spec.Cloudflare.CredentialsRef = &v1alpha2.CloudflareCredentialsRef{Name: name}
	return b
}

// WithFinalizer adds a finalizer.
func (b *NetworkRouteBuilder) WithFinalizer(name string) *NetworkRouteBuilder {
	b.route.Finalizers = append(b.route.Finalizers, name)
	return b
}

// WithDeletionTimestamp marks the resource for deletion.
func (b *NetworkRouteBuilder) WithDeletionTimestamp() *NetworkRouteBuilder {
	now := metav1.Now()
	b.route.DeletionTimestamp = &now
	return b
}

// Build returns the constructed NetworkRoute.
func (b *NetworkRouteBuilder) Build() *v1alpha2.NetworkRoute {
	return b.route.DeepCopy()
}

// AccessApplicationBuilder builds AccessApplication resources for testing.
type AccessApplicationBuilder struct {
	app *v1alpha2.AccessApplication
}

// NewAccessApplicationBuilder creates a new AccessApplicationBuilder.
func NewAccessApplicationBuilder(name string) *AccessApplicationBuilder {
	return &AccessApplicationBuilder{
		app: &v1alpha2.AccessApplication{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "networking.cloudflare-operator.io/v1alpha2",
				Kind:       "AccessApplication",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				UID:  types.UID(fmt.Sprintf("accessapp-%s-%d", name, time.Now().UnixNano())),
			},
			Spec: v1alpha2.AccessApplicationSpec{
				Type: "self_hosted",
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: "default-credentials",
					},
				},
			},
		},
	}
}

// WithDomain sets the application domain.
func (b *AccessApplicationBuilder) WithDomain(domain string) *AccessApplicationBuilder {
	b.app.Spec.Domain = domain
	return b
}

// WithType sets the application type.
func (b *AccessApplicationBuilder) WithType(appType string) *AccessApplicationBuilder {
	b.app.Spec.Type = appType
	return b
}

// WithSessionDuration sets the session duration.
func (b *AccessApplicationBuilder) WithSessionDuration(duration string) *AccessApplicationBuilder {
	b.app.Spec.SessionDuration = duration
	return b
}

// WithCredentialsRef sets the credentials reference.
func (b *AccessApplicationBuilder) WithCredentialsRef(name string) *AccessApplicationBuilder {
	b.app.Spec.Cloudflare.CredentialsRef = &v1alpha2.CloudflareCredentialsRef{Name: name}
	return b
}

// WithFinalizer adds a finalizer.
func (b *AccessApplicationBuilder) WithFinalizer(name string) *AccessApplicationBuilder {
	b.app.Finalizers = append(b.app.Finalizers, name)
	return b
}

// WithDeletionTimestamp marks the resource for deletion.
func (b *AccessApplicationBuilder) WithDeletionTimestamp() *AccessApplicationBuilder {
	now := metav1.Now()
	b.app.DeletionTimestamp = &now
	return b
}

// Build returns the constructed AccessApplication.
func (b *AccessApplicationBuilder) Build() *v1alpha2.AccessApplication {
	return b.app.DeepCopy()
}

// TunnelBindingBuilder builds TunnelBinding resources for testing (v1alpha1).
type TunnelBindingBuilder struct {
	binding *v1alpha1.TunnelBinding
}

// NewTunnelBindingBuilder creates a new TunnelBindingBuilder.
func NewTunnelBindingBuilder(name, namespace string) *TunnelBindingBuilder {
	return &TunnelBindingBuilder{
		binding: &v1alpha1.TunnelBinding{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "networking.cloudflare-operator.io/v1alpha1",
				Kind:       "TunnelBinding",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				UID:       types.UID(fmt.Sprintf("tunnelbinding-%s-%d", name, time.Now().UnixNano())),
			},
			TunnelRef: v1alpha1.TunnelRef{
				Kind: "Tunnel",
				Name: "default-tunnel",
			},
			Subjects: []v1alpha1.TunnelBindingSubject{},
		},
	}
}

// WithTunnelRef sets the tunnel reference.
func (b *TunnelBindingBuilder) WithTunnelRef(name, kind string) *TunnelBindingBuilder {
	b.binding.TunnelRef = v1alpha1.TunnelRef{
		Name: name,
		Kind: kind,
	}
	return b
}

// WithSubject adds a subject.
func (b *TunnelBindingBuilder) WithSubject(serviceName, fqdn string) *TunnelBindingBuilder {
	b.binding.Subjects = append(b.binding.Subjects, v1alpha1.TunnelBindingSubject{
		Kind: "Service",
		Name: serviceName,
		Spec: v1alpha1.TunnelBindingSubjectSpec{
			Fqdn: fqdn,
		},
	})
	return b
}

// WithFinalizer adds a finalizer.
func (b *TunnelBindingBuilder) WithFinalizer(name string) *TunnelBindingBuilder {
	b.binding.Finalizers = append(b.binding.Finalizers, name)
	return b
}

// WithDeletionTimestamp marks the resource for deletion.
func (b *TunnelBindingBuilder) WithDeletionTimestamp() *TunnelBindingBuilder {
	now := metav1.Now()
	b.binding.DeletionTimestamp = &now
	return b
}

// Build returns the constructed TunnelBinding.
func (b *TunnelBindingBuilder) Build() *v1alpha1.TunnelBinding {
	return b.binding.DeepCopy()
}

// SecretBuilder builds Secret resources for testing.
type SecretBuilder struct {
	secret *corev1.Secret
}

// NewSecretBuilder creates a new SecretBuilder.
func NewSecretBuilder(name, namespace string) *SecretBuilder {
	return &SecretBuilder{
		secret: &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Secret",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Data: make(map[string][]byte),
		},
	}
}

// WithData adds data to the secret.
func (b *SecretBuilder) WithData(key string, value []byte) *SecretBuilder {
	b.secret.Data[key] = value
	return b
}

// WithStringData adds string data to the secret.
func (b *SecretBuilder) WithStringData(key, value string) *SecretBuilder {
	b.secret.Data[key] = []byte(value)
	return b
}

// WithType sets the secret type.
func (b *SecretBuilder) WithType(secretType corev1.SecretType) *SecretBuilder {
	b.secret.Type = secretType
	return b
}

// Build returns the constructed Secret.
func (b *SecretBuilder) Build() *corev1.Secret {
	return b.secret.DeepCopy()
}

// CloudflareCredentialsBuilder builds CloudflareCredentials resources for testing.
type CloudflareCredentialsBuilder struct {
	creds *v1alpha2.CloudflareCredentials
}

// NewCloudflareCredentialsBuilder creates a new CloudflareCredentialsBuilder.
func NewCloudflareCredentialsBuilder(name string) *CloudflareCredentialsBuilder {
	return &CloudflareCredentialsBuilder{
		creds: &v1alpha2.CloudflareCredentials{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "networking.cloudflare-operator.io/v1alpha2",
				Kind:       "CloudflareCredentials",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				UID:  types.UID(fmt.Sprintf("creds-%s-%d", name, time.Now().UnixNano())),
			},
			Spec: v1alpha2.CloudflareCredentialsSpec{},
		},
	}
}

// WithAccountID sets the account ID.
func (b *CloudflareCredentialsBuilder) WithAccountID(accountID string) *CloudflareCredentialsBuilder {
	b.creds.Spec.AccountID = accountID
	return b
}

// WithSecretRef sets the secret reference for API credentials.
func (b *CloudflareCredentialsBuilder) WithSecretRef(secretName, namespace string) *CloudflareCredentialsBuilder {
	b.creds.Spec.SecretRef = v1alpha2.SecretReference{
		Name:      secretName,
		Namespace: namespace,
	}
	return b
}

// WithAPIToken is a convenience method that sets up secret ref and auth type for API token.
func (b *CloudflareCredentialsBuilder) WithAPIToken(secretName, secretKey string) *CloudflareCredentialsBuilder {
	b.creds.Spec.AuthType = v1alpha2.AuthTypeAPIToken
	b.creds.Spec.SecretRef = v1alpha2.SecretReference{
		Name:        secretName,
		Namespace:   SystemNamespace,
		APITokenKey: secretKey,
	}
	return b
}

// Build returns the constructed CloudflareCredentials.
func (b *CloudflareCredentialsBuilder) Build() *v1alpha2.CloudflareCredentials {
	return b.creds.DeepCopy()
}
