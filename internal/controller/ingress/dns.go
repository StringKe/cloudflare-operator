// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package ingress

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

// reconcileDNS creates/updates DNS records for Ingress hostnames
func (r *Reconciler) reconcileDNS(ctx context.Context, ingress *networkingv1.Ingress, config *networkingv1alpha2.TunnelIngressClassConfig) error {
	logger := log.FromContext(ctx)

	// Check if DNS is disabled for this Ingress
	parser := NewAnnotationParser(ingress.Annotations)
	if disabled, ok := parser.GetBool(AnnotationDisableDNS); ok && disabled {
		logger.Info("DNS management disabled for this Ingress via annotation")
		return nil
	}

	// Collect all hostnames
	hostnames := r.collectHostnames(ingress)
	if len(hostnames) == 0 {
		logger.Info("No hostnames found in Ingress, skipping DNS reconciliation")
		return nil
	}

	switch config.Spec.DNSManagement {
	case networkingv1alpha2.DNSManagementAutomatic:
		return r.reconcileDNSAutomatic(ctx, ingress, hostnames, config)
	case networkingv1alpha2.DNSManagementDNSRecord:
		return r.reconcileDNSRecords(ctx, ingress, hostnames, config)
	default:
		// Manual - do nothing
		return nil
	}
}

// collectHostnames extracts all unique hostnames from an Ingress
func (*Reconciler) collectHostnames(ingress *networkingv1.Ingress) []string {
	hostnameSet := make(map[string]bool)

	for _, rule := range ingress.Spec.Rules {
		if rule.Host != "" {
			hostnameSet[rule.Host] = true
		}
	}

	// Also collect from TLS spec
	for _, tls := range ingress.Spec.TLS {
		for _, host := range tls.Hosts {
			hostnameSet[host] = true
		}
	}

	hostnames := make([]string, 0, len(hostnameSet))
	for host := range hostnameSet {
		hostnames = append(hostnames, host)
	}

	return hostnames
}

// reconcileDNSAutomatic creates DNSRecord CRDs for automatic DNS management.
// This replaces the old DNS Service (SyncState) approach with direct CRD creation.
func (r *Reconciler) reconcileDNSAutomatic(
	ctx context.Context,
	ingress *networkingv1.Ingress,
	hostnames []string,
	config *networkingv1alpha2.TunnelIngressClassConfig,
) error {
	// Automatic mode now creates DNSRecord CRDs just like DNSRecord mode
	// The DNSRecord controller will handle the actual API calls
	return r.reconcileDNSRecords(ctx, ingress, hostnames, config)
}

// reconcileDNSRecords creates DNSRecord CRDs for each hostname
// nolint:revive // Cognitive complexity for DNSRecord creation
func (r *Reconciler) reconcileDNSRecords(ctx context.Context, ingress *networkingv1.Ingress, hostnames []string, config *networkingv1alpha2.TunnelIngressClassConfig) error {
	logger := log.FromContext(ctx)

	// Get tunnel
	tunnel, err := r.getTunnel(ctx, config)
	if err != nil {
		return err
	}

	tunnelID := tunnel.GetStatus().TunnelId
	if tunnelID == "" {
		return fmt.Errorf("tunnel %s has no tunnel ID", tunnel.GetName())
	}

	// Determine if DNS should be proxied
	proxied := config.IsDNSProxied()
	parser := NewAnnotationParser(ingress.Annotations)
	if p, ok := parser.GetBool(AnnotationDNSProxied); ok {
		proxied = p
	}

	// Get Cloudflare details from tunnel
	cloudflare := tunnel.GetSpec().Cloudflare

	// Create DNSRecords with error aggregation
	var errs []error
	for _, hostname := range hostnames {
		dnsRecord := &networkingv1alpha2.DNSRecord{
			ObjectMeta: metav1.ObjectMeta{
				Name:      r.sanitizeDNSRecordName(hostname, ingress),
				Namespace: ingress.Namespace,
				Labels: map[string]string{
					ManagedByAnnotation:           ManagedByValue,
					"cloudflare.com/ingress-name": ingress.Name,
				},
			},
			Spec: networkingv1alpha2.DNSRecordSpec{
				Name:       hostname,
				Type:       "CNAME",
				Content:    fmt.Sprintf("%s.cfargotunnel.com", tunnelID),
				TTL:        1, // Auto
				Proxied:    proxied,
				Cloudflare: cloudflare,
			},
		}

		// Set owner reference for garbage collection
		if err := ctrl.SetControllerReference(ingress, dnsRecord, r.Scheme); err != nil {
			logger.Error(err, "Failed to set owner reference for DNSRecord", "hostname", hostname)
			errs = append(errs, fmt.Errorf("set owner ref for %s: %w", hostname, err))
			continue
		}

		// Create or update
		if err := r.createOrUpdateDNSRecord(ctx, dnsRecord); err != nil {
			logger.Error(err, "Failed to create/update DNSRecord", "hostname", hostname)
			errs = append(errs, fmt.Errorf("create/update DNSRecord %s: %w", hostname, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to create %d DNSRecords: %w", len(errs), errors.Join(errs...))
	}
	return nil
}

// sanitizeDNSRecordName creates a valid Kubernetes resource name from hostname
func (*Reconciler) sanitizeDNSRecordName(hostname string, ingress *networkingv1.Ingress) string {
	// Replace dots with dashes
	name := strings.ReplaceAll(hostname, ".", "-")

	// Add ingress name prefix to avoid conflicts
	name = fmt.Sprintf("%s-%s", ingress.Name, name)

	// Remove invalid characters
	reg := regexp.MustCompile(`[^a-z0-9-]`)
	name = reg.ReplaceAllString(strings.ToLower(name), "")

	// Trim leading/trailing dashes
	name = strings.Trim(name, "-")

	// Truncate to 63 characters (Kubernetes limit)
	if len(name) > 63 {
		name = name[:63]
		name = strings.Trim(name, "-")
	}

	return name
}

// createOrUpdateDNSRecord creates or updates a DNSRecord CRD
func (r *Reconciler) createOrUpdateDNSRecord(ctx context.Context, dnsRecord *networkingv1alpha2.DNSRecord) error {
	logger := log.FromContext(ctx)

	// Try to get existing
	existing := &networkingv1alpha2.DNSRecord{}
	err := r.Get(ctx, apitypes.NamespacedName{
		Name:      dnsRecord.Name,
		Namespace: dnsRecord.Namespace,
	}, existing)

	if err != nil {
		if apierrors.IsNotFound(err) {
			// Create new
			if err := r.Create(ctx, dnsRecord); err != nil {
				return fmt.Errorf("failed to create DNSRecord: %w", err)
			}
			logger.Info("DNSRecord created", "name", dnsRecord.Name, "hostname", dnsRecord.Spec.Name)
			return nil
		}
		return err
	}

	// Update existing
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, existing, func() {
		existing.Spec = dnsRecord.Spec
		existing.Labels = dnsRecord.Labels
	}); err != nil {
		return fmt.Errorf("failed to update DNSRecord: %w", err)
	}

	logger.Info("DNSRecord updated", "name", dnsRecord.Name, "hostname", dnsRecord.Spec.Name)
	return nil
}

// cleanupDNS cleans up DNS records for a deleted Ingress
func (r *Reconciler) cleanupDNS(ctx context.Context, ingress *networkingv1.Ingress, config *networkingv1alpha2.TunnelIngressClassConfig) error {
	logger := log.FromContext(ctx)

	hostnames := r.collectHostnames(ingress)
	if len(hostnames) == 0 {
		return nil
	}

	switch config.Spec.DNSManagement {
	case networkingv1alpha2.DNSManagementAutomatic:
		return r.cleanupDNSAutomatic(ctx, ingress, hostnames, config)
	case networkingv1alpha2.DNSManagementDNSRecord:
		// DNSRecords are cleaned up automatically via owner reference
		logger.Info("DNSRecords will be garbage collected via owner reference")
		return nil
	default:
		return nil
	}
}

// cleanupDNSAutomatic cleanup for automatic DNS mode.
// Since automatic mode now uses DNSRecord CRDs (with owner references),
// cleanup is handled automatically via garbage collection.
func (r *Reconciler) cleanupDNSAutomatic(
	ctx context.Context,
	_ *networkingv1.Ingress,
	_ []string,
	_ *networkingv1alpha2.TunnelIngressClassConfig,
) error {
	logger := log.FromContext(ctx)
	// DNSRecords are cleaned up automatically via owner reference
	logger.Info("DNSRecords will be garbage collected via owner reference")
	return nil
}

// hostnameBelongsToDomain checks if a hostname belongs to a domain.
// For example:
//   - "api.example.com" belongs to "example.com" → true
//   - "api.staging.example.com" belongs to "example.com" → true
//   - "example.com" belongs to "example.com" → true
//   - "_acm.api.test.example.com." belongs to "example.com" → true (trailing dot)
//   - "api.other.com" does NOT belong to "example.com" → false
func hostnameBelongsToDomain(hostname, domain string) bool {
	// Normalize: remove trailing dots (FQDN format)
	hostname = strings.TrimSuffix(hostname, ".")
	domain = strings.TrimSuffix(domain, ".")

	// Exact match
	if hostname == domain {
		return true
	}
	// Suffix match: hostname must end with ".domain"
	return strings.HasSuffix(hostname, "."+domain)
}
