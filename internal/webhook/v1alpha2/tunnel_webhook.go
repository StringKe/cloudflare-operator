// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// nolint:unused
// log is for logging in this package.
var tunnellog = logf.Log.WithName("tunnel-resource")

// SetupTunnelWebhookWithManager registers the webhook for Tunnel in the manager.
func SetupTunnelWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&networkingv1alpha2.Tunnel{}).
		Complete()
}
