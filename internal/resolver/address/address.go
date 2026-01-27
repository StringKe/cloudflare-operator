// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package address provides utilities for resolving addresses from Kubernetes resources.
package address

import (
	"context"
	"errors"
	"net"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// ResolvedAddress represents a single resolved address.
type ResolvedAddress struct {
	// Value is the IP address or hostname.
	Value string
	// IsIPv4 indicates if this is an IPv4 address.
	IsIPv4 bool
	// IsIPv6 indicates if this is an IPv6 address.
	IsIPv6 bool
	// IsHostname indicates if this is a hostname (not an IP).
	IsHostname bool
}

// ResolveResult contains the result of address resolution.
type ResolveResult struct {
	// Addresses contains all resolved addresses.
	Addresses []ResolvedAddress
	// ResourceVersion is the source resource's resourceVersion.
	ResourceVersion string
	// SourceExists indicates whether the source resource exists.
	SourceExists bool
}

// Resolver resolves addresses from Kubernetes resources.
type Resolver struct {
	client client.Client
}

// NewResolver creates a new address resolver.
func NewResolver(c client.Client) *Resolver {
	return &Resolver{client: c}
}

// Resolve resolves addresses from the given source reference.
// The defaultNamespace is used when the source doesn't specify a namespace.
func (r *Resolver) Resolve(ctx context.Context, sourceRef *v1alpha2.DNSRecordSourceRef, defaultNamespace string) (*ResolveResult, error) {
	if sourceRef == nil {
		return nil, errors.New("sourceRef is nil")
	}

	// Validate exactly one source is specified
	count := sourceRef.CountSources()
	if count == 0 {
		return nil, errors.New("no source specified in sourceRef")
	}
	if count > 1 {
		return nil, errors.New("multiple sources specified in sourceRef, exactly one is required")
	}

	switch {
	case sourceRef.Service != nil:
		return r.resolveService(ctx, sourceRef.Service, defaultNamespace)
	case sourceRef.Ingress != nil:
		return r.resolveIngress(ctx, sourceRef.Ingress, defaultNamespace)
	case sourceRef.HTTPRoute != nil:
		return r.resolveHTTPRoute(ctx, sourceRef.HTTPRoute, defaultNamespace)
	case sourceRef.Gateway != nil:
		return r.resolveGateway(ctx, sourceRef.Gateway, defaultNamespace)
	case sourceRef.Node != nil:
		return r.resolveNode(ctx, sourceRef.Node)
	default:
		return nil, errors.New("unknown source type in sourceRef")
	}
}

// ParseAddress parses an address string and returns a ResolvedAddress.
func ParseAddress(addr string) ResolvedAddress {
	ip := net.ParseIP(addr)
	if ip == nil {
		// Not a valid IP, treat as hostname
		return ResolvedAddress{
			Value:      addr,
			IsHostname: true,
		}
	}

	// Check if IPv4 or IPv6
	if ip.To4() != nil {
		return ResolvedAddress{
			Value:  addr,
			IsIPv4: true,
		}
	}

	return ResolvedAddress{
		Value:  addr,
		IsIPv6: true,
	}
}

// DetermineRecordType determines the DNS record type based on the address.
func DetermineRecordType(addr ResolvedAddress) string {
	if addr.IsIPv4 {
		return "A"
	}
	if addr.IsIPv6 {
		return "AAAA"
	}
	// Hostname -> CNAME
	return "CNAME"
}

// SelectAddresses applies the selection policy to choose addresses.
//
//nolint:revive // cyclomatic complexity is acceptable for this switch statement
func SelectAddresses(addresses []ResolvedAddress, policy v1alpha2.AddressSelectionPolicy) []ResolvedAddress {
	if len(addresses) == 0 {
		return nil
	}

	switch policy {
	case v1alpha2.AddressSelectionFirst, "":
		return addresses[:1]

	case v1alpha2.AddressSelectionAll:
		return addresses

	case v1alpha2.AddressSelectionPreferIPv4:
		// Find first IPv4 address
		for _, addr := range addresses {
			if addr.IsIPv4 {
				return []ResolvedAddress{addr}
			}
		}
		// Fall back to first address
		return addresses[:1]

	case v1alpha2.AddressSelectionPreferIPv6:
		// Find first IPv6 address
		for _, addr := range addresses {
			if addr.IsIPv6 {
				return []ResolvedAddress{addr}
			}
		}
		// Fall back to first address
		return addresses[:1]

	default:
		return addresses[:1]
	}
}

// AddressesToStrings converts ResolvedAddresses to a string slice.
func AddressesToStrings(addresses []ResolvedAddress) []string {
	result := make([]string, len(addresses))
	for i, addr := range addresses {
		result[i] = addr.Value
	}
	return result
}
