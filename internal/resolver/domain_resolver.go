// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package resolver provides hostname to CloudflareDomain resolution using longest suffix match.
package resolver

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// DomainInfo contains resolved domain information for DNS operations
type DomainInfo struct {
	// Domain is the domain name
	Domain string
	// ZoneID is the Cloudflare Zone ID
	ZoneID string
	// AccountID is the Cloudflare Account ID
	AccountID string
	// CredentialsRef is the reference to CloudflareCredentials (optional)
	CredentialsRef *networkingv1alpha2.CredentialsReference
	// CloudflareDomainName is the name of the CloudflareDomain resource
	CloudflareDomainName string
}

// DomainResolver resolves hostnames to CloudflareDomain resources using longest suffix match.
// It provides Zone ID lookup for DNS operations across all controllers.
type DomainResolver struct {
	client client.Client
	log    logr.Logger

	// Cache for CloudflareDomain list
	mu      sync.RWMutex
	domains []networkingv1alpha2.CloudflareDomain
}

// NewDomainResolver creates a new DomainResolver
func NewDomainResolver(client client.Client, log logr.Logger) *DomainResolver {
	return &DomainResolver{
		client: client,
		log:    log.WithName("domain-resolver"),
	}
}

// Resolve finds the best matching CloudflareDomain for a hostname.
// It uses longest suffix match: for "api.staging.example.com":
// - "example.com" matches (suffix)
// - "staging.example.com" matches better (longer suffix)
//
// Returns nil if no matching domain is found.
func (r *DomainResolver) Resolve(ctx context.Context, hostname string) (*DomainInfo, error) {
	// Refresh domain cache
	if err := r.refreshCache(ctx); err != nil {
		return nil, fmt.Errorf("failed to refresh domain cache: %w", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Find best matching domain (longest suffix match)
	var bestMatch *networkingv1alpha2.CloudflareDomain
	var bestMatchLen int
	var defaultDomain *networkingv1alpha2.CloudflareDomain

	for i := range r.domains {
		domain := &r.domains[i]

		// Track default domain as fallback
		if domain.Spec.IsDefault {
			defaultDomain = domain
		}

		// Skip domains that don't have a valid ZoneID
		// We allow Verifying state if ZoneID is already set (periodic re-verification)
		// This prevents race conditions during CloudflareDomain reconciliation
		if domain.Status.ZoneID == "" {
			continue
		}

		// Check if hostname matches this domain
		domainName := domain.Spec.Domain

		// Exact match
		if hostname == domainName {
			bestMatch = domain
			break // Exact match is the best
		}

		// Suffix match: hostname ends with ".domain"
		suffix := "." + domainName
		if strings.HasSuffix(hostname, suffix) {
			if len(domainName) > bestMatchLen {
				bestMatch = domain
				bestMatchLen = len(domainName)
			}
		}
	}

	// Use default domain as fallback if no match found
	if bestMatch == nil && defaultDomain != nil && defaultDomain.Status.ZoneID != "" {
		r.log.V(1).Info("Using default domain as fallback",
			"hostname", hostname,
			"domain", defaultDomain.Spec.Domain)
		bestMatch = defaultDomain
	}

	if bestMatch == nil {
		return nil, nil
	}

	r.log.V(1).Info("Resolved hostname to domain",
		"hostname", hostname,
		"domain", bestMatch.Spec.Domain,
		"zoneId", bestMatch.Status.ZoneID)

	return &DomainInfo{
		Domain:               bestMatch.Spec.Domain,
		ZoneID:               bestMatch.Status.ZoneID,
		AccountID:            bestMatch.Status.AccountID,
		CredentialsRef:       bestMatch.Spec.CredentialsRef,
		CloudflareDomainName: bestMatch.Name,
	}, nil
}

// ResolveMultiple resolves multiple hostnames and returns a map of hostname to DomainInfo.
// This is useful for batch operations like Ingress reconciliation.
func (r *DomainResolver) ResolveMultiple(ctx context.Context, hostnames []string) (map[string]*DomainInfo, error) {
	// Refresh domain cache once for all hostnames
	if err := r.refreshCache(ctx); err != nil {
		return nil, fmt.Errorf("failed to refresh domain cache: %w", err)
	}

	result := make(map[string]*DomainInfo, len(hostnames))
	for _, hostname := range hostnames {
		info, err := r.resolveFromCache(hostname)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve hostname %s: %w", hostname, err)
		}
		if info != nil {
			result[hostname] = info
		}
	}

	return result, nil
}

// resolveFromCache resolves a hostname using cached domain data (no cache refresh)
func (r *DomainResolver) resolveFromCache(hostname string) (*DomainInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var bestMatch *networkingv1alpha2.CloudflareDomain
	var bestMatchLen int
	var defaultDomain *networkingv1alpha2.CloudflareDomain

	for i := range r.domains {
		domain := &r.domains[i]

		if domain.Spec.IsDefault {
			defaultDomain = domain
		}

		// Skip domains without a valid ZoneID
		if domain.Status.ZoneID == "" {
			continue
		}

		domainName := domain.Spec.Domain

		if hostname == domainName {
			bestMatch = domain
			break
		}

		suffix := "." + domainName
		if strings.HasSuffix(hostname, suffix) {
			if len(domainName) > bestMatchLen {
				bestMatch = domain
				bestMatchLen = len(domainName)
			}
		}
	}

	if bestMatch == nil && defaultDomain != nil && defaultDomain.Status.ZoneID != "" {
		bestMatch = defaultDomain
	}

	if bestMatch == nil {
		return nil, nil
	}

	return &DomainInfo{
		Domain:               bestMatch.Spec.Domain,
		ZoneID:               bestMatch.Status.ZoneID,
		AccountID:            bestMatch.Status.AccountID,
		CredentialsRef:       bestMatch.Spec.CredentialsRef,
		CloudflareDomainName: bestMatch.Name,
	}, nil
}

// GetZoneID is a convenience method to get just the Zone ID for a hostname.
// Returns empty string if no matching domain is found.
func (r *DomainResolver) GetZoneID(ctx context.Context, hostname string) (string, error) {
	info, err := r.Resolve(ctx, hostname)
	if err != nil {
		return "", err
	}
	if info == nil {
		return "", nil
	}
	return info.ZoneID, nil
}

// MustResolve resolves a hostname and returns an error if no matching domain is found.
func (r *DomainResolver) MustResolve(ctx context.Context, hostname string) (*DomainInfo, error) {
	info, err := r.Resolve(ctx, hostname)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, fmt.Errorf("no CloudflareDomain found for hostname %q, please create a CloudflareDomain resource for this domain", hostname)
	}
	return info, nil
}

// refreshCache refreshes the CloudflareDomain cache from the API server
func (r *DomainResolver) refreshCache(ctx context.Context) error {
	domainList := &networkingv1alpha2.CloudflareDomainList{}
	if err := r.client.List(ctx, domainList); err != nil {
		return fmt.Errorf("failed to list CloudflareDomains: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.domains = domainList.Items

	r.log.V(1).Info("Refreshed CloudflareDomain cache", "count", len(r.domains))
	return nil
}

// InvalidateCache invalidates the domain cache, forcing a refresh on next Resolve call
func (r *DomainResolver) InvalidateCache() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.domains = nil
}

// ListDomains returns all CloudflareDomain resources (refreshes cache if needed)
func (r *DomainResolver) ListDomains(ctx context.Context) ([]networkingv1alpha2.CloudflareDomain, error) {
	if err := r.refreshCache(ctx); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to avoid data races
	result := make([]networkingv1alpha2.CloudflareDomain, len(r.domains))
	copy(result, r.domains)
	return result, nil
}

// GetDefaultDomain returns the default CloudflareDomain if one exists
func (r *DomainResolver) GetDefaultDomain(ctx context.Context) (*DomainInfo, error) {
	if err := r.refreshCache(ctx); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for i := range r.domains {
		domain := &r.domains[i]
		if domain.Spec.IsDefault && domain.Status.ZoneID != "" {
			return &DomainInfo{
				Domain:               domain.Spec.Domain,
				ZoneID:               domain.Status.ZoneID,
				AccountID:            domain.Status.AccountID,
				CredentialsRef:       domain.Spec.CredentialsRef,
				CloudflareDomainName: domain.Name,
			}, nil
		}
	}

	return nil, nil
}
