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

var _ = Describe("Tunnel Webhook", func() {
	var (
		v1alpha1Tunnel *networkingv1alpha1.Tunnel
		v1alpha2Tunnel *networkingv1alpha2.Tunnel
	)

	BeforeEach(func() {
		v1alpha1Tunnel = &networkingv1alpha1.Tunnel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-tunnel",
				Namespace: "default",
			},
			Spec: networkingv1alpha1.TunnelSpec{
				Size:           2,
				Image:          "cloudflare/cloudflared:latest",
				NoTlsVerify:    true,
				Protocol:       "auto",
				FallbackTarget: "http_status:404",
				Cloudflare: networkingv1alpha1.CloudflareDetails{
					Domain:    "example.com",
					Secret:    "cf-secret",
					AccountId: "account-123",
				},
				NewTunnel: networkingv1alpha1.NewTunnel{
					Name: "my-tunnel",
				},
			},
			Status: networkingv1alpha1.TunnelStatus{
				TunnelId:   "tunnel-123",
				TunnelName: "my-tunnel",
				AccountId:  "account-123",
				ZoneId:     "zone-456",
			},
		}

		v1alpha2Tunnel = &networkingv1alpha2.Tunnel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-tunnel",
				Namespace: "default",
			},
			Spec: networkingv1alpha2.TunnelSpec{
				DeployPatch:    "{}",
				NoTlsVerify:    true,
				Protocol:       "auto",
				FallbackTarget: "http_status:404",
				Cloudflare: networkingv1alpha2.CloudflareDetails{
					Domain:    "example.com",
					Secret:    "cf-secret",
					AccountId: "account-123",
				},
				NewTunnel: &networkingv1alpha2.NewTunnel{
					Name: "my-tunnel",
				},
			},
			Status: networkingv1alpha2.TunnelStatus{
				TunnelId:   "tunnel-123",
				TunnelName: "my-tunnel",
				AccountId:  "account-123",
				ZoneId:     "zone-456",
			},
		}
	})

	AfterEach(func() {
		// Cleanup is handled by the test framework
	})

	Context("When converting Tunnel from v1alpha1 to v1alpha2", func() {
		It("Should convert basic fields correctly", func() {
			convertedTunnel := &networkingv1alpha2.Tunnel{}
			err := v1alpha1Tunnel.ConvertTo(convertedTunnel)
			Expect(err).NotTo(HaveOccurred())

			// Verify metadata
			Expect(convertedTunnel.Name).To(Equal(v1alpha1Tunnel.Name))
			Expect(convertedTunnel.Namespace).To(Equal(v1alpha1Tunnel.Namespace))

			// Verify spec fields
			Expect(convertedTunnel.Spec.NoTlsVerify).To(Equal(v1alpha1Tunnel.Spec.NoTlsVerify))
			Expect(convertedTunnel.Spec.Protocol).To(Equal(v1alpha1Tunnel.Spec.Protocol))
			Expect(convertedTunnel.Spec.FallbackTarget).To(Equal(v1alpha1Tunnel.Spec.FallbackTarget))

			// Verify cloudflare details
			Expect(convertedTunnel.Spec.Cloudflare.Domain).To(Equal(v1alpha1Tunnel.Spec.Cloudflare.Domain))
			Expect(convertedTunnel.Spec.Cloudflare.Secret).To(Equal(v1alpha1Tunnel.Spec.Cloudflare.Secret))
			Expect(convertedTunnel.Spec.Cloudflare.AccountId).To(Equal(v1alpha1Tunnel.Spec.Cloudflare.AccountId))

			// Verify tunnel name
			Expect(convertedTunnel.Spec.NewTunnel).NotTo(BeNil())
			Expect(convertedTunnel.Spec.NewTunnel.Name).To(Equal(v1alpha1Tunnel.Spec.NewTunnel.Name))

			// Verify status
			Expect(convertedTunnel.Status.TunnelId).To(Equal(v1alpha1Tunnel.Status.TunnelId))
			Expect(convertedTunnel.Status.TunnelName).To(Equal(v1alpha1Tunnel.Status.TunnelName))
			Expect(convertedTunnel.Status.AccountId).To(Equal(v1alpha1Tunnel.Status.AccountId))
			Expect(convertedTunnel.Status.ZoneId).To(Equal(v1alpha1Tunnel.Status.ZoneId))
		})

		It("Should convert size and image to deployPatch", func() {
			convertedTunnel := &networkingv1alpha2.Tunnel{}
			err := v1alpha1Tunnel.ConvertTo(convertedTunnel)
			Expect(err).NotTo(HaveOccurred())

			// DeployPatch should contain size (replicas) and image
			Expect(convertedTunnel.Spec.DeployPatch).NotTo(BeEmpty())
			Expect(convertedTunnel.Spec.DeployPatch).To(ContainSubstring("replicas"))
			Expect(convertedTunnel.Spec.DeployPatch).To(ContainSubstring("cloudflared"))
		})
	})

	Context("When converting Tunnel from v1alpha2 to v1alpha1", func() {
		It("Should convert basic fields correctly", func() {
			convertedTunnel := &networkingv1alpha1.Tunnel{}
			err := convertedTunnel.ConvertFrom(v1alpha2Tunnel)
			Expect(err).NotTo(HaveOccurred())

			// Verify metadata
			Expect(convertedTunnel.Name).To(Equal(v1alpha2Tunnel.Name))
			Expect(convertedTunnel.Namespace).To(Equal(v1alpha2Tunnel.Namespace))

			// Verify spec fields
			Expect(convertedTunnel.Spec.NoTlsVerify).To(Equal(v1alpha2Tunnel.Spec.NoTlsVerify))
			Expect(convertedTunnel.Spec.Protocol).To(Equal(v1alpha2Tunnel.Spec.Protocol))
			Expect(convertedTunnel.Spec.FallbackTarget).To(Equal(v1alpha2Tunnel.Spec.FallbackTarget))

			// Verify cloudflare details
			Expect(convertedTunnel.Spec.Cloudflare.Domain).To(Equal(v1alpha2Tunnel.Spec.Cloudflare.Domain))
			Expect(convertedTunnel.Spec.Cloudflare.Secret).To(Equal(v1alpha2Tunnel.Spec.Cloudflare.Secret))
			Expect(convertedTunnel.Spec.Cloudflare.AccountId).To(Equal(v1alpha2Tunnel.Spec.Cloudflare.AccountId))

			// Verify tunnel name
			Expect(convertedTunnel.Spec.NewTunnel.Name).To(Equal(v1alpha2Tunnel.Spec.NewTunnel.Name))

			// Verify status
			Expect(convertedTunnel.Status.TunnelId).To(Equal(v1alpha2Tunnel.Status.TunnelId))
			Expect(convertedTunnel.Status.TunnelName).To(Equal(v1alpha2Tunnel.Status.TunnelName))
			Expect(convertedTunnel.Status.AccountId).To(Equal(v1alpha2Tunnel.Status.AccountId))
			Expect(convertedTunnel.Status.ZoneId).To(Equal(v1alpha2Tunnel.Status.ZoneId))
		})
	})

	Context("When handling edge cases", func() {
		It("Should handle empty optional fields", func() {
			minimalTunnel := &networkingv1alpha1.Tunnel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-tunnel",
					Namespace: "default",
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

			convertedTunnel := &networkingv1alpha2.Tunnel{}
			err := minimalTunnel.ConvertTo(convertedTunnel)
			Expect(err).NotTo(HaveOccurred())
			Expect(convertedTunnel.Spec.NewTunnel).NotTo(BeNil())
		})

		It("Should handle existing tunnel reference", func() {
			existingTunnel := &networkingv1alpha1.Tunnel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-ref-tunnel",
					Namespace: "default",
				},
				Spec: networkingv1alpha1.TunnelSpec{
					Cloudflare: networkingv1alpha1.CloudflareDetails{
						Domain:    "example.com",
						Secret:    "cf-secret",
						AccountId: "account-123",
					},
					ExistingTunnel: networkingv1alpha1.ExistingTunnel{
						Id:   "existing-id-123",
						Name: "existing-name",
					},
				},
			}

			convertedTunnel := &networkingv1alpha2.Tunnel{}
			err := existingTunnel.ConvertTo(convertedTunnel)
			Expect(err).NotTo(HaveOccurred())
			Expect(convertedTunnel.Spec.ExistingTunnel).NotTo(BeNil())
			Expect(convertedTunnel.Spec.ExistingTunnel.Id).To(Equal("existing-id-123"))
			Expect(convertedTunnel.Spec.ExistingTunnel.Name).To(Equal("existing-name"))
		})
	})
})
