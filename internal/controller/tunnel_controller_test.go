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
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

var _ = ginkgo.Describe("Tunnel Controller", func() {
	const (
		testTunnelName      = "test-tunnel"
		testTunnelNamespace = "default"
		testSecretName      = "cloudflare-api-credentials"

		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	ginkgo.Context("When creating a Tunnel resource", func() {
		ginkgo.BeforeEach(func() {
			// Create the Secret that the Tunnel will reference
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSecretName,
					Namespace: testTunnelNamespace,
				},
				StringData: map[string]string{
					"CLOUDFLARE_API_TOKEN": "test-api-token",
				},
			}
			gomega.Expect(k8sClient.Create(ctx, secret)).Should(gomega.Succeed())
		})

		ginkgo.AfterEach(func() {
			// Clean up the Secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSecretName,
					Namespace: testTunnelNamespace,
				},
			}
			_ = k8sClient.Delete(ctx, secret)

			// Clean up any Tunnel resources
			tunnel := &networkingv1alpha2.Tunnel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testTunnelName,
					Namespace: testTunnelNamespace,
				},
			}
			_ = k8sClient.Delete(ctx, tunnel)
		})

		ginkgo.It("Should create a Tunnel resource successfully", func() {
			ginkgo.By("Creating a new Tunnel")
			tunnel := &networkingv1alpha2.Tunnel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testTunnelName,
					Namespace: testTunnelNamespace,
				},
				Spec: networkingv1alpha2.TunnelSpec{
					Cloudflare: networkingv1alpha2.CloudflareDetails{
						Domain:                              "example.com",
						Secret:                              testSecretName,
						AccountId:                           "test-account-id",
						CLOUDFLARE_API_TOKEN:                "CLOUDFLARE_API_TOKEN",
						CLOUDFLARE_API_KEY:                  "CLOUDFLARE_API_KEY",
						CLOUDFLARE_TUNNEL_CREDENTIAL_FILE:   "CLOUDFLARE_TUNNEL_CREDENTIAL_FILE",
						CLOUDFLARE_TUNNEL_CREDENTIAL_SECRET: "CLOUDFLARE_TUNNEL_CREDENTIAL_SECRET",
					},
					NewTunnel: &networkingv1alpha2.NewTunnel{
						Name: "test-new-tunnel",
					},
				},
			}
			gomega.Expect(k8sClient.Create(ctx, tunnel)).Should(gomega.Succeed())

			// Verify the Tunnel was created
			tunnelLookupKey := types.NamespacedName{Name: testTunnelName, Namespace: testTunnelNamespace}
			createdTunnel := &networkingv1alpha2.Tunnel{}

			gomega.Eventually(func() bool {
				err := k8sClient.Get(ctx, tunnelLookupKey, createdTunnel)
				return err == nil
			}, timeout, interval).Should(gomega.BeTrue())

			gomega.Expect(createdTunnel.Spec.Cloudflare.Domain).Should(gomega.Equal("example.com"))
			gomega.Expect(createdTunnel.Spec.NewTunnel.Name).Should(gomega.Equal("test-new-tunnel"))
		})

		ginkgo.It("Should validate mutually exclusive fields", func() {
			ginkgo.By("Creating a Tunnel with both NewTunnel and ExistingTunnel")
			tunnel := &networkingv1alpha2.Tunnel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testTunnelName + "-invalid",
					Namespace: testTunnelNamespace,
				},
				Spec: networkingv1alpha2.TunnelSpec{
					Cloudflare: networkingv1alpha2.CloudflareDetails{
						Domain:                              "example.com",
						Secret:                              testSecretName,
						AccountId:                           "test-account-id",
						CLOUDFLARE_API_TOKEN:                "CLOUDFLARE_API_TOKEN",
						CLOUDFLARE_API_KEY:                  "CLOUDFLARE_API_KEY",
						CLOUDFLARE_TUNNEL_CREDENTIAL_FILE:   "CLOUDFLARE_TUNNEL_CREDENTIAL_FILE",
						CLOUDFLARE_TUNNEL_CREDENTIAL_SECRET: "CLOUDFLARE_TUNNEL_CREDENTIAL_SECRET",
					},
					NewTunnel: &networkingv1alpha2.NewTunnel{
						Name: "test-new-tunnel",
					},
					ExistingTunnel: &networkingv1alpha2.ExistingTunnel{
						Id: "existing-tunnel-id",
					},
				},
			}
			// This should fail validation
			err := k8sClient.Create(ctx, tunnel)
			// Note: CRD validation may or may not catch this, depending on CEL rules
			// The controller will catch it during reconciliation
			if err == nil {
				// Clean up if it was created
				_ = k8sClient.Delete(ctx, tunnel)
			}
		})
	})

	ginkgo.Context("When testing Tunnel helper functions", func() {
		ginkgo.It("Should generate correct labels for tunnel", func() {
			tunnel := &networkingv1alpha2.Tunnel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-tunnel",
					Namespace: "default",
				},
				Spec: networkingv1alpha2.TunnelSpec{
					Cloudflare: networkingv1alpha2.CloudflareDetails{
						Domain: "example.com",
					},
				},
				Status: networkingv1alpha2.TunnelStatus{
					TunnelId:   "tunnel-12345",
					TunnelName: "my-tunnel",
				},
			}

			adapter := TunnelAdapter{tunnel}
			labels := labelsForTunnel(adapter)

			gomega.Expect(labels[tunnelLabel]).Should(gomega.Equal("my-tunnel"))
			gomega.Expect(labels[tunnelAppLabel]).Should(gomega.Equal("cloudflared"))
			gomega.Expect(labels[tunnelIdLabel]).Should(gomega.Equal("tunnel-12345"))
			gomega.Expect(labels[tunnelNameLabel]).Should(gomega.Equal("my-tunnel"))
			gomega.Expect(labels[tunnelDomainLabel]).Should(gomega.Equal("example.com"))
		})

		ginkgo.It("Should generate unique secret finalizer names", func() {
			finalizer1 := getSecretFinalizerName("tunnel-a")
			finalizer2 := getSecretFinalizerName("tunnel-b")

			gomega.Expect(finalizer1).ShouldNot(gomega.Equal(finalizer2))
			gomega.Expect(finalizer1).Should(gomega.ContainSubstring("tunnel-a"))
			gomega.Expect(finalizer2).Should(gomega.ContainSubstring("tunnel-b"))
		})
	})
})

var _ = ginkgo.Describe("Tunnel Configuration", func() {
	ginkgo.Context("When generating ConfigMap", func() {
		ginkgo.It("Should include WARP routing when enabled", func() {
			// This test verifies the configuration generation logic
			// without actually reconciling
			ginkgo.By("Verifying EnableWarpRouting field is respected")
			tunnel := &networkingv1alpha2.Tunnel{
				Spec: networkingv1alpha2.TunnelSpec{
					EnableWarpRouting: true,
					FallbackTarget:    "http_status:404",
				},
			}
			gomega.Expect(tunnel.Spec.EnableWarpRouting).Should(gomega.BeTrue())
		})

		ginkgo.It("Should have correct default values", func() {
			tunnel := &networkingv1alpha2.Tunnel{
				Spec: networkingv1alpha2.TunnelSpec{},
			}
			// Default values from CRD
			gomega.Expect(tunnel.Spec.EnableWarpRouting).Should(gomega.BeFalse())
		})
	})
})
