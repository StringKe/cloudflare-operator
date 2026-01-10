// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package route

import (
	"context"
	"sort"

	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

// Builder is an interface for building cloudflared ingress rules.
// Different resource types (Ingress, HTTPRoute, TCPRoute, TunnelBinding)
// implement this interface to convert their rules to cloudflared format.
type Builder interface {
	// Build generates cloudflared ingress rules from the resource.
	// Returns a slice of rules (can be empty if resource has no routes).
	Build(ctx context.Context) ([]cf.UnvalidatedIngressRule, error)
}

// Aggregator aggregates rules from multiple builders and adds a fallback rule.
type Aggregator struct {
	// Builders are the rule builders to aggregate
	Builders []Builder
	// FallbackTarget is the target for the catch-all fallback rule
	// Default is "http_status:404"
	FallbackTarget string
}

// NewAggregator creates a new rule aggregator with the given fallback target.
func NewAggregator(fallbackTarget string) *Aggregator {
	if fallbackTarget == "" {
		fallbackTarget = "http_status:404"
	}
	return &Aggregator{
		Builders:       make([]Builder, 0),
		FallbackTarget: fallbackTarget,
	}
}

// Add adds a builder to the aggregator.
func (a *Aggregator) Add(b Builder) {
	a.Builders = append(a.Builders, b)
}

// AddAll adds multiple builders to the aggregator.
func (a *Aggregator) AddAll(builders ...Builder) {
	a.Builders = append(a.Builders, builders...)
}

// Build aggregates rules from all builders, sorts them, and adds a fallback rule.
// Rules are sorted by hostname, then by path for deterministic configuration.
func (a *Aggregator) Build(ctx context.Context) ([]cf.UnvalidatedIngressRule, error) {
	var allRules []cf.UnvalidatedIngressRule

	// Collect rules from all builders
	for _, builder := range a.Builders {
		rules, err := builder.Build(ctx)
		if err != nil {
			return nil, err
		}
		allRules = append(allRules, rules...)
	}

	// Sort rules for deterministic config (by hostname, then path)
	sort.Slice(allRules, func(i, j int) bool {
		if allRules[i].Hostname != allRules[j].Hostname {
			return allRules[i].Hostname < allRules[j].Hostname
		}
		return allRules[i].Path < allRules[j].Path
	})

	// Add fallback rule
	allRules = append(allRules, cf.UnvalidatedIngressRule{
		Service: a.FallbackTarget,
	})

	return allRules, nil
}

// BuildWithoutFallback aggregates rules from all builders without adding a fallback rule.
// This is useful when the caller wants to add a custom fallback or combine with other rules.
func (a *Aggregator) BuildWithoutFallback(ctx context.Context) ([]cf.UnvalidatedIngressRule, error) {
	var allRules []cf.UnvalidatedIngressRule

	for _, builder := range a.Builders {
		rules, err := builder.Build(ctx)
		if err != nil {
			return nil, err
		}
		allRules = append(allRules, rules...)
	}

	// Sort rules for deterministic config
	sort.Slice(allRules, func(i, j int) bool {
		if allRules[i].Hostname != allRules[j].Hostname {
			return allRules[i].Hostname < allRules[j].Hostname
		}
		return allRules[i].Path < allRules[j].Path
	})

	return allRules, nil
}
