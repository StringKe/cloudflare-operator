// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

//nolint:revive // max-public-structs: test file requires multiple test types
package common

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// AggTestCfg is a simple config for testing aggregation
type AggTestCfg struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

func TestAggregator_Aggregate(t *testing.T) {
	tests := []struct {
		name          string
		syncState     *v1alpha2.CloudflareSyncState
		wantCount     int
		wantSourceRef int
	}{
		{
			name: "empty sources",
			syncState: &v1alpha2.CloudflareSyncState{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: v1alpha2.CloudflareSyncStateSpec{
					Sources: []v1alpha2.ConfigSource{},
				},
			},
			wantCount:     0,
			wantSourceRef: 0,
		},
		{
			name: "single source",
			syncState: &v1alpha2.CloudflareSyncState{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: v1alpha2.CloudflareSyncStateSpec{
					Sources: []v1alpha2.ConfigSource{
						{
							Ref: v1alpha2.SourceReference{
								Kind:      "TestResource",
								Namespace: "default",
								Name:      "test-1",
							},
							Priority: 100,
							Config:   mustMarshal(AggTestCfg{Name: "config1", Values: []string{"a", "b"}}),
						},
					},
				},
			},
			wantCount:     1,
			wantSourceRef: 1,
		},
		{
			name: "multiple sources sorted by priority",
			syncState: &v1alpha2.CloudflareSyncState{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: v1alpha2.CloudflareSyncStateSpec{
					Sources: []v1alpha2.ConfigSource{
						{
							Ref: v1alpha2.SourceReference{
								Kind:      "TestResource",
								Namespace: "default",
								Name:      "low-priority",
							},
							Priority: 200,
							Config:   mustMarshal(AggTestCfg{Name: "low", Values: []string{"c"}}),
						},
						{
							Ref: v1alpha2.SourceReference{
								Kind:      "TestResource",
								Namespace: "default",
								Name:      "high-priority",
							},
							Priority: 50,
							Config:   mustMarshal(AggTestCfg{Name: "high", Values: []string{"a"}}),
						},
						{
							Ref: v1alpha2.SourceReference{
								Kind:      "TestResource",
								Namespace: "default",
								Name:      "medium-priority",
							},
							Priority: 100,
							Config:   mustMarshal(AggTestCfg{Name: "medium", Values: []string{"b"}}),
						},
					},
				},
			},
			wantCount:     3,
			wantSourceRef: 3,
		},
		{
			name: "skip nil config",
			syncState: &v1alpha2.CloudflareSyncState{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: v1alpha2.CloudflareSyncStateSpec{
					Sources: []v1alpha2.ConfigSource{
						{
							Ref: v1alpha2.SourceReference{
								Kind:      "TestResource",
								Namespace: "default",
								Name:      "valid",
							},
							Priority: 100,
							Config:   mustMarshal(AggTestCfg{Name: "valid", Values: []string{"a"}}),
						},
						{
							Ref: v1alpha2.SourceReference{
								Kind:      "TestResource",
								Namespace: "default",
								Name:      "nil-config",
							},
							Priority: 200,
							Config:   runtime.RawExtension{Raw: nil},
						},
					},
				},
			},
			wantCount:     1,
			wantSourceRef: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aggregator := &Aggregator[AggTestCfg]{
				MergeFunc: func(aggregated *AggTestCfg, source *AggTestCfg, _ v1alpha2.SourceReference, _ int) *AggTestCfg {
					if aggregated.Name == "" {
						aggregated.Name = source.Name
					}
					aggregated.Values = append(aggregated.Values, source.Values...)
					return aggregated
				},
			}

			result, err := aggregator.Aggregate(tt.syncState)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCount, result.SourceCount)
			assert.Equal(t, tt.wantSourceRef, len(result.SourceRefs))
		})
	}
}

func TestAggregator_AggregateWithFilter(t *testing.T) {
	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			Sources: []v1alpha2.ConfigSource{
				{
					Ref: v1alpha2.SourceReference{
						Kind:      "TypeA",
						Namespace: "default",
						Name:      "resource-a",
					},
					Priority: 100,
					Config:   mustMarshal(AggTestCfg{Name: "a", Values: []string{"a"}}),
				},
				{
					Ref: v1alpha2.SourceReference{
						Kind:      "TypeB",
						Namespace: "default",
						Name:      "resource-b",
					},
					Priority: 200,
					Config:   mustMarshal(AggTestCfg{Name: "b", Values: []string{"b"}}),
				},
				{
					Ref: v1alpha2.SourceReference{
						Kind:      "TypeA",
						Namespace: "production",
						Name:      "resource-a2",
					},
					Priority: 150,
					Config:   mustMarshal(AggTestCfg{Name: "a2", Values: []string{"a2"}}),
				},
			},
		},
	}

	aggregator := &Aggregator[AggTestCfg]{
		MergeFunc: func(aggregated *AggTestCfg, source *AggTestCfg, _ v1alpha2.SourceReference, _ int) *AggTestCfg {
			aggregated.Values = append(aggregated.Values, source.Values...)
			return aggregated
		},
	}

	// Filter by TypeA only
	result, err := aggregator.AggregateWithFilter(syncState, "TypeA")
	require.NoError(t, err)
	assert.Equal(t, 2, result.SourceCount)

	// Filter by TypeB only
	result, err = aggregator.AggregateWithFilter(syncState, "TypeB")
	require.NoError(t, err)
	assert.Equal(t, 1, result.SourceCount)

	// Filter by both types
	result, err = aggregator.AggregateWithFilter(syncState, "TypeA", "TypeB")
	require.NoError(t, err)
	assert.Equal(t, 3, result.SourceCount)

	// Filter by non-existent type
	result, err = aggregator.AggregateWithFilter(syncState, "TypeC")
	require.NoError(t, err)
	assert.Equal(t, 0, result.SourceCount)
}

func TestEntryAggregator_Aggregate(t *testing.T) {
	type TestEntry struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}

	type TestEntryConfig struct {
		Entries []TestEntry `json:"entries"`
	}

	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			Sources: []v1alpha2.ConfigSource{
				{
					Ref: v1alpha2.SourceReference{
						Kind:      "TestResource",
						Namespace: "default",
						Name:      "source-1",
					},
					Priority: 100,
					Config: mustMarshal(TestEntryConfig{
						Entries: []TestEntry{
							{Key: "key1", Value: "value1"},
							{Key: "key2", Value: "value2"},
						},
					}),
				},
				{
					Ref: v1alpha2.SourceReference{
						Kind:      "TestResource",
						Namespace: "default",
						Name:      "source-2",
					},
					Priority: 200,
					Config: mustMarshal(TestEntryConfig{
						Entries: []TestEntry{
							{Key: "key3", Value: "value3"},
						},
					}),
				},
			},
		},
	}

	aggregator := &EntryAggregator[TestEntryConfig]{
		ExtractEntries: func(config *TestEntryConfig) []Entry {
			entries := make([]Entry, len(config.Entries))
			for i, e := range config.Entries {
				entries[i] = Entry{Data: e}
			}
			return entries
		},
	}

	result, err := aggregator.Aggregate(syncState)
	require.NoError(t, err)

	assert.Equal(t, 3, len(result.Entries))
	assert.Equal(t, 2, result.SourceCount)
	assert.Equal(t, 2, len(result.ByOwner["TestResource/default/source-1"]))
	assert.Equal(t, 1, len(result.ByOwner["TestResource/default/source-2"]))
}

func TestEntryAggregator_Deduplication(t *testing.T) {
	type TestEntry struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}

	type TestEntryConfig struct {
		Entries []TestEntry `json:"entries"`
	}

	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			Sources: []v1alpha2.ConfigSource{
				{
					Ref: v1alpha2.SourceReference{
						Kind:      "TestResource",
						Namespace: "default",
						Name:      "high-priority",
					},
					Priority: 50, // Higher priority (lower number)
					Config: mustMarshal(TestEntryConfig{
						Entries: []TestEntry{
							{Key: "duplicate-key", Value: "high-priority-value"},
						},
					}),
				},
				{
					Ref: v1alpha2.SourceReference{
						Kind:      "TestResource",
						Namespace: "default",
						Name:      "low-priority",
					},
					Priority: 200, // Lower priority (higher number)
					Config: mustMarshal(TestEntryConfig{
						Entries: []TestEntry{
							{Key: "duplicate-key", Value: "low-priority-value"}, // Same key
							{Key: "unique-key", Value: "unique-value"},
						},
					}),
				},
			},
		},
	}

	aggregator := &EntryAggregator[TestEntryConfig]{
		ExtractEntries: func(config *TestEntryConfig) []Entry {
			entries := make([]Entry, len(config.Entries))
			for i, e := range config.Entries {
				entries[i] = Entry{Data: e}
			}
			return entries
		},
		EntryKey: func(entry Entry) string {
			return entry.Data.(TestEntry).Key
		},
	}

	result, err := aggregator.Aggregate(syncState)
	require.NoError(t, err)

	// Should have 2 entries: high-priority duplicate-key and unique-key
	assert.Equal(t, 2, len(result.Entries))

	// First entry should be from high-priority source
	firstEntry, ok := result.Entries[0].Data.(TestEntry)
	require.True(t, ok, "expected TestEntry type")
	assert.Equal(t, "duplicate-key", firstEntry.Key)
	assert.Equal(t, "high-priority-value", firstEntry.Value)
}

func TestAggregatedEntries_RemoveOwner(t *testing.T) {
	owner1 := v1alpha2.SourceReference{
		Kind:      "TestResource",
		Namespace: "default",
		Name:      "source-1",
	}
	owner2 := v1alpha2.SourceReference{
		Kind:      "TestResource",
		Namespace: "default",
		Name:      "source-2",
	}

	entries := &AggregatedEntries{
		Entries: []Entry{
			{Data: "entry1", Owner: owner1, Priority: 100},
			{Data: "entry2", Owner: owner1, Priority: 100},
			{Data: "entry3", Owner: owner2, Priority: 200},
		},
		ByOwner: map[string][]Entry{
			owner1.String(): {{Data: "entry1", Owner: owner1}, {Data: "entry2", Owner: owner1}},
			owner2.String(): {{Data: "entry3", Owner: owner2}},
		},
	}

	remaining := entries.RemoveOwner(owner1)

	assert.Equal(t, 1, len(remaining))
	assert.Equal(t, "entry3", remaining[0].Data)
}

func TestAggregatedEntries_GetOwnerEntries(t *testing.T) {
	owner := v1alpha2.SourceReference{
		Kind:      "TestResource",
		Namespace: "default",
		Name:      "source-1",
	}

	entries := &AggregatedEntries{
		Entries: []Entry{},
		ByOwner: map[string][]Entry{
			owner.String(): {
				{Data: "entry1", Owner: owner},
				{Data: "entry2", Owner: owner},
			},
		},
	}

	ownerEntries := entries.GetOwnerEntries(owner)
	assert.Equal(t, 2, len(ownerEntries))
}

func TestOwnershipMarker(t *testing.T) {
	tests := []struct {
		name      string
		ref       v1alpha2.SourceReference
		wantStr   string
		checkDesc string
		wantOwned bool
	}{
		{
			name: "basic marker",
			ref: v1alpha2.SourceReference{
				Kind:      "ZoneRuleset",
				Namespace: "default",
				Name:      "my-ruleset",
			},
			wantStr:   "managed-by:ZoneRuleset/default/my-ruleset",
			checkDesc: "Test description [managed-by:ZoneRuleset/default/my-ruleset]",
			wantOwned: true,
		},
		{
			name: "different owner",
			ref: v1alpha2.SourceReference{
				Kind:      "ZoneRuleset",
				Namespace: "default",
				Name:      "my-ruleset",
			},
			wantStr:   "managed-by:ZoneRuleset/default/my-ruleset",
			checkDesc: "Test description [managed-by:TransformRule/default/other]",
			wantOwned: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			marker := NewOwnershipMarker(tt.ref)
			assert.Equal(t, tt.wantStr, marker.String())
			assert.Equal(t, tt.wantOwned, marker.IsOwnedBy(tt.checkDesc))
		})
	}
}

func TestOwnershipMarker_AppendToDescription(t *testing.T) {
	marker := NewOwnershipMarker(v1alpha2.SourceReference{
		Kind:      "ZoneRuleset",
		Namespace: "default",
		Name:      "my-ruleset",
	})

	tests := []struct {
		name        string
		description string
		want        string
	}{
		{
			name:        "empty description",
			description: "",
			want:        "[managed-by:ZoneRuleset/default/my-ruleset]",
		},
		{
			name:        "with description",
			description: "My rule",
			want:        "My rule [managed-by:ZoneRuleset/default/my-ruleset]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := marker.AppendToDescription(tt.description)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestIsManagedByOperator(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        bool
	}{
		{
			name:        "managed by operator",
			description: "My rule [managed-by:ZoneRuleset/default/test]",
			want:        true,
		},
		{
			name:        "only marker",
			description: "[managed-by:ZoneRuleset/default/test]",
			want:        true,
		},
		{
			name:        "not managed",
			description: "External rule created by admin",
			want:        false,
		},
		{
			name:        "empty description",
			description: "",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsManagedByOperator(tt.description)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestSettingsAggregator_Aggregate(t *testing.T) {
	type TestSettings struct {
		Settings map[string]interface{} `json:"settings"`
	}

	syncState := &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			Sources: []v1alpha2.ConfigSource{
				{
					Ref: v1alpha2.SourceReference{
						Kind:      "TestResource",
						Namespace: "default",
						Name:      "high-priority",
					},
					Priority: 50,
					Config: mustMarshal(TestSettings{
						Settings: map[string]interface{}{
							"key1": "value1-high",
							"key2": "value2-high",
						},
					}),
				},
				{
					Ref: v1alpha2.SourceReference{
						Kind:      "TestResource",
						Namespace: "default",
						Name:      "low-priority",
					},
					Priority: 200,
					Config: mustMarshal(TestSettings{
						Settings: map[string]interface{}{
							"key1": "value1-low", // Should be ignored (lower priority)
							"key3": "value3-low", // Should be included
						},
					}),
				},
			},
		},
	}

	aggregator := &SettingsAggregator[TestSettings]{
		GetSettings: func(config *TestSettings) map[string]interface{} {
			return config.Settings
		},
	}

	result, err := aggregator.Aggregate(syncState)
	require.NoError(t, err)

	// key1 should be from high-priority
	assert.Equal(t, "value1-high", result.Settings["key1"].Value)
	assert.Equal(t, "high-priority", result.Settings["key1"].Owner.Name)

	// key2 should be from high-priority
	assert.Equal(t, "value2-high", result.Settings["key2"].Value)

	// key3 should be from low-priority
	assert.Equal(t, "value3-low", result.Settings["key3"].Value)
	assert.Equal(t, "low-priority", result.Settings["key3"].Owner.Name)
}

func TestSettingOwnership_GetSettingsWithoutOwner(t *testing.T) {
	owner1 := v1alpha2.SourceReference{
		Kind:      "TestResource",
		Namespace: "default",
		Name:      "source-1",
	}
	owner2 := v1alpha2.SourceReference{
		Kind:      "TestResource",
		Namespace: "default",
		Name:      "source-2",
	}

	ownership := &SettingOwnership{
		Settings: map[string]SettingValue{
			"key1": {Value: "value1", Owner: owner1, Priority: 100},
			"key2": {Value: "value2", Owner: owner1, Priority: 100},
			"key3": {Value: "value3", Owner: owner2, Priority: 200},
		},
	}

	remaining := ownership.GetSettingsWithoutOwner(owner1)

	assert.Equal(t, 1, len(remaining))
	assert.Equal(t, "value3", remaining["key3"])
}

func TestFilterSourcesByKind(t *testing.T) {
	sources := []v1alpha2.ConfigSource{
		{Ref: v1alpha2.SourceReference{Kind: "TypeA", Namespace: "default", Name: "a1"}},
		{Ref: v1alpha2.SourceReference{Kind: "TypeB", Namespace: "default", Name: "b1"}},
		{Ref: v1alpha2.SourceReference{Kind: "TypeA", Namespace: "default", Name: "a2"}},
		{Ref: v1alpha2.SourceReference{Kind: "TypeC", Namespace: "default", Name: "c1"}},
	}

	// Filter single kind
	result := FilterSourcesByKind(sources, "TypeA")
	assert.Equal(t, 2, len(result))

	// Filter multiple kinds
	result = FilterSourcesByKind(sources, "TypeA", "TypeB")
	assert.Equal(t, 3, len(result))

	// Filter non-existent kind
	result = FilterSourcesByKind(sources, "TypeD")
	assert.Equal(t, 0, len(result))
}

// mustMarshal marshals config to runtime.RawExtension, panics on error
func mustMarshal(v interface{}) runtime.RawExtension {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return runtime.RawExtension{Raw: data}
}
