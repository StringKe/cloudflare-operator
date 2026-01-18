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
	"github.com/StringKe/cloudflare-operator/internal/service"
	dnssvc "github.com/StringKe/cloudflare-operator/internal/service/dns"
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

// reconcileDNSAutomatic registers DNS records via DNS Service (SyncState-based)
// nolint:revive // Cognitive complexity for DNS reconciliation with error aggregation
func (r *Reconciler) reconcileDNSAutomatic(
	ctx context.Context,
	ingress *networkingv1.Ingress,
	hostnames []string,
	config *networkingv1alpha2.TunnelIngressClassConfig,
) error {
	logger := log.FromContext(ctx)

	// Get tunnel
	tunnel, err := r.getTunnel(ctx, config)
	if err != nil {
		return err
	}

	// Get tunnel ID for CNAME target
	tunnelID := tunnel.GetStatus().TunnelId
	if tunnelID == "" {
		return fmt.Errorf("tunnel %s has no tunnel ID", tunnel.GetName())
	}

	accountID := tunnel.GetStatus().AccountId

	// Determine if DNS should be proxied
	proxied := config.IsDNSProxied()
	parser := NewAnnotationParser(ingress.Annotations)
	if p, ok := parser.GetBool(AnnotationDNSProxied); ok {
		proxied = p
	}

	// Get credentials reference from tunnel
	credRef := r.getCredentialsReferenceFromTunnel(tunnel)

	// Resolve Zone IDs for all hostnames using DomainResolver
	hostnameZones, err := r.domainResolver.ResolveMultiple(ctx, hostnames)
	if err != nil {
		logger.Error(err, "Failed to resolve domains for hostnames")
	}

	// Get fallback zone ID from tunnel status
	fallbackZoneID := tunnel.GetStatus().ZoneId
	fallbackDomain := tunnel.GetSpec().Cloudflare.Domain

	// Create DNS Service and register records
	svc := dnssvc.NewService(r.Client)
	var errs []error

	for _, hostname := range hostnames {
		// Determine Zone ID and domain for this hostname
		zoneID := fallbackZoneID
		domainName := fallbackDomain
		if domainInfo, ok := hostnameZones[hostname]; ok && domainInfo != nil {
			zoneID = domainInfo.ZoneID
			domainName = domainInfo.Domain
			logger.V(1).Info("Using CloudflareDomain Zone ID for hostname",
				"hostname", hostname, "domain", domainInfo.Domain, "zoneId", zoneID)
		}

		if zoneID == "" {
			errs = append(errs, fmt.Errorf("no Zone ID found for hostname %s", hostname))
			continue
		}

		// Validate that hostname belongs to the resolved domain
		if domainName != "" && !hostnameBelongsToDomain(hostname, domainName) {
			errs = append(errs, fmt.Errorf("hostname %q does not belong to domain %q: create a CloudflareDomain resource for the correct domain", hostname, domainName))
			continue
		}

		// Build DNS record config
		dnsConfig := dnssvc.DNSRecordConfig{
			Name:    hostname,
			Type:    "CNAME",
			Content: fmt.Sprintf("%s.cfargotunnel.com", tunnelID),
			TTL:     1, // Auto
			Proxied: proxied,
			Comment: fmt.Sprintf("Managed by cloudflare-operator IngressAutomatic: %s/%s", ingress.Namespace, ingress.Name),
		}

		// Register DNS record via Service (will be synced by Sync Controller)
		if err := svc.Register(ctx, dnssvc.RegisterOptions{
			ZoneID:    zoneID,
			AccountID: accountID,
			Source: service.Source{
				Kind:      "IngressAutomatic",
				Namespace: ingress.Namespace,
				Name:      fmt.Sprintf("%s-%s", ingress.Name, sanitizeHostnameForSource(hostname)),
			},
			Config:         dnsConfig,
			CredentialsRef: credRef,
		}); err != nil {
			logger.Error(err, "Failed to register DNS record", "hostname", hostname)
			errs = append(errs, fmt.Errorf("DNS record %s: %w", hostname, err))
		} else {
			logger.Info("DNS record registered to SyncState", "hostname", hostname, "zoneId", zoneID)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to register %d DNS records: %w", len(errs), errors.Join(errs...))
	}
	return nil
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

// cleanupDNSAutomatic unregisters DNS records from SyncState
// nolint:revive // Cognitive complexity for DNS cleanup logic
func (r *Reconciler) cleanupDNSAutomatic(
	ctx context.Context,
	ingress *networkingv1.Ingress,
	hostnames []string,
	_ *networkingv1alpha2.TunnelIngressClassConfig,
) error {
	logger := log.FromContext(ctx)

	// Create DNS Service and unregister records
	svc := dnssvc.NewService(r.Client)
	var errs []error

	for _, hostname := range hostnames {
		source := service.Source{
			Kind:      "IngressAutomatic",
			Namespace: ingress.Namespace,
			Name:      fmt.Sprintf("%s-%s", ingress.Name, sanitizeHostnameForSource(hostname)),
		}

		// Unregister DNS record via Service (Sync Controller will handle deletion)
		if err := svc.Unregister(ctx, "", source); err != nil {
			logger.Error(err, "Failed to unregister DNS record", "hostname", hostname)
			errs = append(errs, fmt.Errorf("unregister DNS record %s: %w", hostname, err))
		} else {
			logger.Info("DNS record unregistered from SyncState", "hostname", hostname)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to cleanup %d DNS records: %w", len(errs), errors.Join(errs...))
	}
	return nil
}

// sanitizeHostnameForSource converts a hostname to a valid Source name component
// (replaces dots with dashes and removes invalid characters)
func sanitizeHostnameForSource(hostname string) string {
	// Replace dots with dashes
	name := strings.ReplaceAll(hostname, ".", "-")
	// Remove trailing dashes
	name = strings.Trim(name, "-")
	// Truncate to avoid overly long names
	if len(name) > 50 {
		name = name[:50]
		name = strings.Trim(name, "-")
	}
	return name
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
