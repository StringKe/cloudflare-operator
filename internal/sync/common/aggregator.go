// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

//nolint:revive // max-public-structs: aggregation framework requires multiple public types for flexibility
package common

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// Aggregator provides common aggregation logic for Sync Controllers.
// It supports merging configurations from multiple sources with priority ordering.
type Aggregator[T any] struct {
	// MergeFunc defines how to merge a source config into the aggregated result.
	// It receives the current aggregated result and a source config, and returns
	// the updated aggregated result.
	MergeFunc func(aggregated *T, source *T, sourceRef v1alpha2.SourceReference, priority int) *T

	// FinalizeFunc is called after all sources are merged to perform any
	// final processing (e.g., sorting, deduplication, adding defaults).
	// Optional - if nil, no finalization is performed.
	FinalizeFunc func(aggregated *T) *T
}

// AggregatedResult wraps the aggregated configuration with metadata
type AggregatedResult[T any] struct {
	// Config is the merged configuration
	Config *T
	// SourceCount is the number of sources that contributed to this config
	SourceCount int
	// SourceRefs contains references to all contributing sources
	SourceRefs []v1alpha2.SourceReference
}

// Aggregate merges all sources in a SyncState using the configured merge function.
// Sources are processed in priority order (lower priority number = higher precedence).
//
// Algorithm:
// 1. Sort sources by priority (ascending)
// 2. Initialize empty aggregated result
// 3. For each source, parse config and call MergeFunc
// 4. Call FinalizeFunc if configured
// 5. Return aggregated result with metadata
func (a *Aggregator[T]) Aggregate(syncState *v1alpha2.CloudflareSyncState) (*AggregatedResult[T], error) {
	if len(syncState.Spec.Sources) == 0 {
		return &AggregatedResult[T]{
			Config:      new(T),
			SourceCount: 0,
			SourceRefs:  nil,
		}, nil
	}

	// Sort sources by priority (lower number = higher priority)
	sources := make([]v1alpha2.ConfigSource, len(syncState.Spec.Sources))
	copy(sources, syncState.Spec.Sources)
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Priority < sources[j].Priority
	})

	// Initialize aggregated result
	var aggregated T
	result := &AggregatedResult[T]{
		Config:      &aggregated,
		SourceRefs:  make([]v1alpha2.SourceReference, 0, len(sources)),
		SourceCount: 0,
	}

	// Process each source
	for _, source := range sources {
		if source.Config.Raw == nil {
			continue
		}

		// Parse source config
		var config T
		if err := json.Unmarshal(source.Config.Raw, &config); err != nil {
			// Log warning but continue - don't fail entire aggregation for one bad source
			continue
		}

		// Merge into aggregated result
		if a.MergeFunc != nil {
			result.Config = a.MergeFunc(result.Config, &config, source.Ref, source.Priority)
		}

		result.SourceRefs = append(result.SourceRefs, source.Ref)
		result.SourceCount++
	}

	// Finalize if configured
	if a.FinalizeFunc != nil {
		result.Config = a.FinalizeFunc(result.Config)
	}

	return result, nil
}

// AggregateWithFilter aggregates only sources matching the specified kinds.
func (a *Aggregator[T]) AggregateWithFilter(
	syncState *v1alpha2.CloudflareSyncState,
	kinds ...string,
) (*AggregatedResult[T], error) {
	// Create a filtered SyncState
	filtered := syncState.DeepCopy()
	filtered.Spec.Sources = FilterSourcesByKind(syncState.Spec.Sources, kinds...)
	return a.Aggregate(filtered)
}

// EntryAggregator provides aggregation for list-based configurations
// (e.g., split tunnel entries, ruleset rules, DNS records).
// It tracks ownership of each entry to support incremental deletion.
type EntryAggregator[T any] struct {
	// ExtractEntries extracts entries from a source config
	ExtractEntries func(config *T) []Entry

	// SetEntries sets entries in the aggregated config
	SetEntries func(config *T, entries []Entry)

	// EntryKey returns a unique key for deduplication (optional)
	// If nil, no deduplication is performed
	EntryKey func(entry Entry) string
}

// Entry represents a single entry with ownership tracking
type Entry struct {
	// Data is the actual entry data (will be type-asserted by the caller)
	Data interface{}
	// Owner identifies the source that contributed this entry
	Owner v1alpha2.SourceReference
	// Priority from the source
	Priority int
}

// AggregatedEntries contains the merged entries with ownership information
type AggregatedEntries struct {
	// Entries is the list of merged entries
	Entries []Entry
	// ByOwner groups entries by their owner for efficient lookup
	ByOwner map[string][]Entry
	// SourceCount is the number of sources that contributed entries
	SourceCount int
}

// Aggregate merges entries from all sources, tracking ownership of each entry.
func (a *EntryAggregator[T]) Aggregate(syncState *v1alpha2.CloudflareSyncState) (*AggregatedEntries, error) {
	result := &AggregatedEntries{
		Entries: make([]Entry, 0),
		ByOwner: make(map[string][]Entry),
	}

	if len(syncState.Spec.Sources) == 0 {
		return result, nil
	}

	// Sort sources by priority
	sources := make([]v1alpha2.ConfigSource, len(syncState.Spec.Sources))
	copy(sources, syncState.Spec.Sources)
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Priority < sources[j].Priority
	})

	// Track seen keys for deduplication
	seenKeys := make(map[string]bool)

	// Process each source
	for _, source := range sources {
		if source.Config.Raw == nil {
			continue
		}

		// Parse source config
		var config T
		if err := json.Unmarshal(source.Config.Raw, &config); err != nil {
			continue
		}

		// Extract entries from this source
		entries := a.ExtractEntries(&config)
		if len(entries) == 0 {
			continue
		}

		ownerKey := source.Ref.String()
		result.SourceCount++

		for _, entry := range entries {
			// Add ownership info
			entry.Owner = source.Ref
			entry.Priority = source.Priority

			// Deduplicate if key function is provided
			if a.EntryKey != nil {
				key := a.EntryKey(entry)
				if seenKeys[key] {
					continue // Skip duplicate (first one wins due to priority sort)
				}
				seenKeys[key] = true
			}

			result.Entries = append(result.Entries, entry)

			// Group by owner
			if result.ByOwner[ownerKey] == nil {
				result.ByOwner[ownerKey] = make([]Entry, 0)
			}
			result.ByOwner[ownerKey] = append(result.ByOwner[ownerKey], entry)
		}
	}

	return result, nil
}

// RemoveOwner returns entries without those owned by the specified source.
// This is used during deletion to remove only the deleted source's entries.
func (a *AggregatedEntries) RemoveOwner(owner v1alpha2.SourceReference) []Entry {
	ownerKey := owner.String()
	remaining := make([]Entry, 0, len(a.Entries))

	for _, entry := range a.Entries {
		if entry.Owner.String() != ownerKey {
			remaining = append(remaining, entry)
		}
	}

	return remaining
}

// GetOwnerEntries returns entries owned by the specified source.
func (a *AggregatedEntries) GetOwnerEntries(owner v1alpha2.SourceReference) []Entry {
	return a.ByOwner[owner.String()]
}

// OwnershipMarker provides utilities for embedding ownership information
// in Cloudflare resource descriptions for tracking purposes.
type OwnershipMarker struct {
	Kind      string
	Namespace string
	Name      string
}

// NewOwnershipMarker creates a marker from a SourceReference
func NewOwnershipMarker(ref v1alpha2.SourceReference) OwnershipMarker {
	return OwnershipMarker{
		Kind:      ref.Kind,
		Namespace: ref.Namespace,
		Name:      ref.Name,
	}
}

// String returns the marker in format "managed-by:Kind/Namespace/Name"
func (m OwnershipMarker) String() string {
	return fmt.Sprintf("managed-by:%s/%s/%s", m.Kind, m.Namespace, m.Name)
}

// AppendToDescription appends the ownership marker to a description
func (m OwnershipMarker) AppendToDescription(description string) string {
	marker := m.String()
	if description == "" {
		return fmt.Sprintf("[%s]", marker)
	}
	return fmt.Sprintf("%s [%s]", description, marker)
}

// IsOwnedBy checks if a description contains the ownership marker
func (m OwnershipMarker) IsOwnedBy(description string) bool {
	return containsOwnershipMarker(description, m.String())
}

// containsOwnershipMarker checks if description contains the marker
func containsOwnershipMarker(description, marker string) bool {
	// Check for [marker] format
	if len(description) >= len(marker)+2 {
		for i := 0; i <= len(description)-len(marker)-2; i++ {
			if description[i] == '[' {
				endIdx := i + 1 + len(marker)
				if endIdx < len(description) && description[endIdx] == ']' {
					if description[i+1:endIdx] == marker {
						return true
					}
				}
			}
		}
	}
	return false
}

// IsManagedByOperator checks if a description indicates operator management
func IsManagedByOperator(description string) bool {
	return len(description) > 0 && containsSubstring(description, "managed-by:")
}

// containsSubstring is a simple substring check
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// SettingsAggregator provides aggregation for key-value settings
// where higher priority sources override lower priority ones.
type SettingsAggregator[T any] struct {
	// GetSettings extracts settings map from config
	GetSettings func(config *T) map[string]interface{}

	// ApplySettings applies merged settings to config
	ApplySettings func(config *T, settings map[string]interface{})
}

// SettingOwnership tracks which source owns each setting
type SettingOwnership struct {
	// Settings maps setting key to its value and owner
	Settings map[string]SettingValue
}

// SettingValue contains a setting value with ownership
type SettingValue struct {
	Value    interface{}
	Owner    v1alpha2.SourceReference
	Priority int
}

// Aggregate merges settings from all sources, with higher priority sources winning.
func (a *SettingsAggregator[T]) Aggregate(syncState *v1alpha2.CloudflareSyncState) (*SettingOwnership, error) {
	result := &SettingOwnership{
		Settings: make(map[string]SettingValue),
	}

	if len(syncState.Spec.Sources) == 0 {
		return result, nil
	}

	// Sort sources by priority (lower = higher precedence)
	sources := make([]v1alpha2.ConfigSource, len(syncState.Spec.Sources))
	copy(sources, syncState.Spec.Sources)
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Priority < sources[j].Priority
	})

	// Process each source - first one wins for each setting
	for _, source := range sources {
		if source.Config.Raw == nil {
			continue
		}

		var config T
		if err := json.Unmarshal(source.Config.Raw, &config); err != nil {
			continue
		}

		settings := a.GetSettings(&config)
		for key, value := range settings {
			// Only set if not already set (first wins)
			if _, exists := result.Settings[key]; !exists {
				result.Settings[key] = SettingValue{
					Value:    value,
					Owner:    source.Ref,
					Priority: source.Priority,
				}
			}
		}
	}

	return result, nil
}

// GetSettingsWithoutOwner returns settings not owned by the specified source.
// Used during deletion to preserve other sources' settings.
func (o *SettingOwnership) GetSettingsWithoutOwner(owner v1alpha2.SourceReference) map[string]interface{} {
	ownerKey := owner.String()
	result := make(map[string]interface{})

	for key, value := range o.Settings {
		if value.Owner.String() != ownerKey {
			result[key] = value.Value
		}
	}

	return result
}
