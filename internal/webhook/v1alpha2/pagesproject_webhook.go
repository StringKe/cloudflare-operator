// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	ctrl "sigs.k8s.io/controller-runtime"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// SetupPagesProjectWebhookWithManager registers the webhook for PagesProject in the manager.
func SetupPagesProjectWebhookWithManager(mgr ctrl.Manager) error {
	return (&networkingv1alpha2.PagesProject{}).SetupWebhookWithManager(mgr)
}
