// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkingv1alpha1 "github.com/StringKe/cloudflare-operator/api/v1alpha1"
	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

var _ = Describe("ClusterTunnel Webhook", func() {
	var (
		v1alpha1ClusterTunnel *networkingv1alpha1.ClusterTunnel
		v1alpha2ClusterTunnel *networkingv1alpha2.ClusterTunnel
	)

	BeforeEach(func() {
		v1alpha1ClusterTunnel = &networkingv1alpha1.ClusterTunnel{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cluster-tunnel",
			},
			Spec: networkingv1alpha1.TunnelSpec{
				Size:           3,
				Image:          "cloudflare/cloudflared:latest",
				NoTlsVerify:    false,
				Protocol:       "quic",
				FallbackTarget: "http_status:503",
				Cloudflare: networkingv1alpha1.CloudflareDetails{
					Domain:    "cluster.example.com",
					Secret:    "cluster-cf-secret",
					AccountId: "cluster-account-123",
				},
				NewTunnel: networkingv1alpha1.NewTunnel{
					Name: "cluster-tunnel",
				},
			},
			Status: networkingv1alpha1.TunnelStatus{
				TunnelId:   "cluster-tunnel-123",
				TunnelName: "cluster-tunnel",
				AccountId:  "cluster-account-123",
				ZoneId:     "cluster-zone-456",
			},
		}

		v1alpha2ClusterTunnel = &networkingv1alpha2.ClusterTunnel{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cluster-tunnel",
			},
			Spec: networkingv1alpha2.TunnelSpec{
				DeployPatch:    "{}",
				NoTlsVerify:    false,
				Protocol:       "quic",
				FallbackTarget: "http_status:503",
				Cloudflare: networkingv1alpha2.CloudflareDetails{
					Domain:    "cluster.example.com",
					Secret:    "cluster-cf-secret",
					AccountId: "cluster-account-123",
				},
				NewTunnel: &networkingv1alpha2.NewTunnel{
					Name: "cluster-tunnel",
				},
			},
			Status: networkingv1alpha2.TunnelStatus{
				TunnelId:   "cluster-tunnel-123",
				TunnelName: "cluster-tunnel",
				AccountId:  "cluster-account-123",
				ZoneId:     "cluster-zone-456",
			},
		}
	})

	AfterEach(func() {
		// Cleanup is handled by the test framework
	})

	Context("When converting ClusterTunnel from v1alpha1 to v1alpha2", func() {
		It("Should convert basic fields correctly", func() {
			convertedTunnel := &networkingv1alpha2.ClusterTunnel{}
			err := v1alpha1ClusterTunnel.ConvertTo(convertedTunnel)
			Expect(err).NotTo(HaveOccurred())

			// Verify metadata (ClusterTunnel is cluster-scoped, no namespace)
			Expect(convertedTunnel.Name).To(Equal(v1alpha1ClusterTunnel.Name))

			// Verify spec fields
			Expect(convertedTunnel.Spec.NoTlsVerify).To(Equal(v1alpha1ClusterTunnel.Spec.NoTlsVerify))
			Expect(convertedTunnel.Spec.Protocol).To(Equal(v1alpha1ClusterTunnel.Spec.Protocol))
			Expect(convertedTunnel.Spec.FallbackTarget).To(Equal(v1alpha1ClusterTunnel.Spec.FallbackTarget))

			// Verify cloudflare details
			Expect(convertedTunnel.Spec.Cloudflare.Domain).To(Equal(v1alpha1ClusterTunnel.Spec.Cloudflare.Domain))
			Expect(convertedTunnel.Spec.Cloudflare.Secret).To(Equal(v1alpha1ClusterTunnel.Spec.Cloudflare.Secret))
			Expect(convertedTunnel.Spec.Cloudflare.AccountId).To(Equal(v1alpha1ClusterTunnel.Spec.Cloudflare.AccountId))

			// Verify tunnel name
			Expect(convertedTunnel.Spec.NewTunnel).NotTo(BeNil())
			Expect(convertedTunnel.Spec.NewTunnel.Name).To(Equal(v1alpha1ClusterTunnel.Spec.NewTunnel.Name))

			// Verify status
			Expect(convertedTunnel.Status.TunnelId).To(Equal(v1alpha1ClusterTunnel.Status.TunnelId))
			Expect(convertedTunnel.Status.TunnelName).To(Equal(v1alpha1ClusterTunnel.Status.TunnelName))
			Expect(convertedTunnel.Status.AccountId).To(Equal(v1alpha1ClusterTunnel.Status.AccountId))
			Expect(convertedTunnel.Status.ZoneId).To(Equal(v1alpha1ClusterTunnel.Status.ZoneId))
		})

		It("Should convert size and image to deployPatch", func() {
			convertedTunnel := &networkingv1alpha2.ClusterTunnel{}
			err := v1alpha1ClusterTunnel.ConvertTo(convertedTunnel)
			Expect(err).NotTo(HaveOccurred())

			// DeployPatch should contain size (replicas) and image
			Expect(convertedTunnel.Spec.DeployPatch).NotTo(BeEmpty())
			Expect(convertedTunnel.Spec.DeployPatch).To(ContainSubstring("replicas"))
			Expect(convertedTunnel.Spec.DeployPatch).To(ContainSubstring("cloudflared"))
		})
	})

	Context("When converting ClusterTunnel from v1alpha2 to v1alpha1", func() {
		It("Should convert basic fields correctly", func() {
			convertedTunnel := &networkingv1alpha1.ClusterTunnel{}
			err := convertedTunnel.ConvertFrom(v1alpha2ClusterTunnel)
			Expect(err).NotTo(HaveOccurred())

			// Verify metadata
			Expect(convertedTunnel.Name).To(Equal(v1alpha2ClusterTunnel.Name))

			// Verify spec fields
			Expect(convertedTunnel.Spec.NoTlsVerify).To(Equal(v1alpha2ClusterTunnel.Spec.NoTlsVerify))
			Expect(convertedTunnel.Spec.Protocol).To(Equal(v1alpha2ClusterTunnel.Spec.Protocol))
			Expect(convertedTunnel.Spec.FallbackTarget).To(Equal(v1alpha2ClusterTunnel.Spec.FallbackTarget))

			// Verify cloudflare details
			Expect(convertedTunnel.Spec.Cloudflare.Domain).To(Equal(v1alpha2ClusterTunnel.Spec.Cloudflare.Domain))
			Expect(convertedTunnel.Spec.Cloudflare.Secret).To(Equal(v1alpha2ClusterTunnel.Spec.Cloudflare.Secret))
			Expect(convertedTunnel.Spec.Cloudflare.AccountId).To(Equal(v1alpha2ClusterTunnel.Spec.Cloudflare.AccountId))

			// Verify tunnel name
			Expect(convertedTunnel.Spec.NewTunnel.Name).To(Equal(v1alpha2ClusterTunnel.Spec.NewTunnel.Name))

			// Verify status
			Expect(convertedTunnel.Status.TunnelId).To(Equal(v1alpha2ClusterTunnel.Status.TunnelId))
			Expect(convertedTunnel.Status.TunnelName).To(Equal(v1alpha2ClusterTunnel.Status.TunnelName))
			Expect(convertedTunnel.Status.AccountId).To(Equal(v1alpha2ClusterTunnel.Status.AccountId))
			Expect(convertedTunnel.Status.ZoneId).To(Equal(v1alpha2ClusterTunnel.Status.ZoneId))
		})
	})

	Context("When handling edge cases", func() {
		It("Should handle empty optional fields", func() {
			minimalTunnel := &networkingv1alpha1.ClusterTunnel{
				ObjectMeta: metav1.ObjectMeta{
					Name: "minimal-cluster-tunnel",
				},
				Spec: networkingv1alpha1.TunnelSpec{
					Cloudflare: networkingv1alpha1.CloudflareDetails{
						Domain:    "example.com",
						Secret:    "cf-secret",
						AccountId: "account-123",
					},
					NewTunnel: networkingv1alpha1.NewTunnel{
						Name: "minimal",
					},
				},
			}

			convertedTunnel := &networkingv1alpha2.ClusterTunnel{}
			err := minimalTunnel.ConvertTo(convertedTunnel)
			Expect(err).NotTo(HaveOccurred())
			Expect(convertedTunnel.Spec.NewTunnel).NotTo(BeNil())
		})

		It("Should handle existing tunnel reference", func() {
			existingTunnel := &networkingv1alpha1.ClusterTunnel{
				ObjectMeta: metav1.ObjectMeta{
					Name: "existing-cluster-tunnel",
				},
				Spec: networkingv1alpha1.TunnelSpec{
					Cloudflare: networkingv1alpha1.CloudflareDetails{
						Domain:    "example.com",
						Secret:    "cf-secret",
						AccountId: "account-123",
					},
					ExistingTunnel: networkingv1alpha1.ExistingTunnel{
						Id:   "existing-cluster-id",
						Name: "existing-cluster-name",
					},
				},
			}

			convertedTunnel := &networkingv1alpha2.ClusterTunnel{}
			err := existingTunnel.ConvertTo(convertedTunnel)
			Expect(err).NotTo(HaveOccurred())
			Expect(convertedTunnel.Spec.ExistingTunnel).NotTo(BeNil())
			Expect(convertedTunnel.Spec.ExistingTunnel.Id).To(Equal("existing-cluster-id"))
			Expect(convertedTunnel.Spec.ExistingTunnel.Name).To(Equal("existing-cluster-name"))
		})
	})
})
