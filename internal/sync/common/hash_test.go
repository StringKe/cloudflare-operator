// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeConfigHash_SimpleStruct(t *testing.T) {
	config := struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		Count   int    `json:"count"`
	}{
		Name:    "test",
		Enabled: true,
		Count:   42,
	}

	hash, err := ComputeConfigHash(config)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.Len(t, hash, 64) // SHA256 = 32 bytes = 64 hex chars

	// Same input should produce same hash
	hash2, err := ComputeConfigHash(config)
	require.NoError(t, err)
	assert.Equal(t, hash, hash2)
}

func TestComputeConfigHash_DifferentValues(t *testing.T) {
	config1 := map[string]string{"key": "value1"}
	config2 := map[string]string{"key": "value2"}

	hash1, err := ComputeConfigHash(config1)
	require.NoError(t, err)

	hash2, err := ComputeConfigHash(config2)
	require.NoError(t, err)

	assert.NotEqual(t, hash1, hash2)
}

func TestComputeConfigHash_NilValue(t *testing.T) {
	hash, err := ComputeConfigHash(nil)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestComputeConfigHash_EmptyMap(t *testing.T) {
	hash, err := ComputeConfigHash(map[string]string{})
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestComputeConfigHash_EmptySlice(t *testing.T) {
	hash, err := ComputeConfigHash([]string{})
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestComputeConfigHash_NestedStruct(t *testing.T) {
	config := struct {
		Outer struct {
			Inner struct {
				Value string `json:"value"`
			} `json:"inner"`
		} `json:"outer"`
	}{}
	config.Outer.Inner.Value = "deep"

	hash, err := ComputeConfigHash(config)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestComputeConfigHashDeterministic_MapOrdering(t *testing.T) {
	// Create two maps with same content but potentially different iteration order
	// Go map iteration order is random, so we create multiple maps to test
	map1 := map[string]interface{}{
		"zebra":    1,
		"apple":    2,
		"banana":   3,
		"cherry":   4,
		"date":     5,
		"elephant": 6,
	}

	map2 := map[string]interface{}{
		"date":     5,
		"banana":   3,
		"zebra":    1,
		"apple":    2,
		"elephant": 6,
		"cherry":   4,
	}

	hash1, err := ComputeConfigHashDeterministic(map1)
	require.NoError(t, err)

	hash2, err := ComputeConfigHashDeterministic(map2)
	require.NoError(t, err)

	assert.Equal(t, hash1, hash2, "Same map content should produce same hash regardless of key order")
}

func TestComputeConfigHashDeterministic_NestedMaps(t *testing.T) {
	config := map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"zebra": "last",
				"alpha": "first",
			},
		},
		"other": []interface{}{
			map[string]interface{}{
				"z": 1,
				"a": 2,
			},
		},
	}

	hash1, err := ComputeConfigHashDeterministic(config)
	require.NoError(t, err)

	hash2, err := ComputeConfigHashDeterministic(config)
	require.NoError(t, err)

	assert.Equal(t, hash1, hash2)
}

func TestComputeConfigHashDeterministic_SliceWithMaps(t *testing.T) {
	config := []interface{}{
		map[string]interface{}{"b": 2, "a": 1},
		map[string]interface{}{"d": 4, "c": 3},
	}

	hash1, err := ComputeConfigHashDeterministic(config)
	require.NoError(t, err)

	hash2, err := ComputeConfigHashDeterministic(config)
	require.NoError(t, err)

	assert.Equal(t, hash1, hash2)
}

func TestHashChanged_EmptyPrevious(t *testing.T) {
	assert.True(t, HashChanged("", "abc123"))
}

func TestHashChanged_SameHash(t *testing.T) {
	assert.False(t, HashChanged("abc123", "abc123"))
}

func TestHashChanged_DifferentHash(t *testing.T) {
	assert.True(t, HashChanged("abc123", "xyz789"))
}

func TestHashChanged_BothEmpty(t *testing.T) {
	assert.True(t, HashChanged("", ""))
}

func TestMustComputeHash_Success(t *testing.T) {
	config := map[string]string{"key": "value"}
	hash := MustComputeHash(config)
	assert.NotEmpty(t, hash)
}

func TestMustComputeHash_ComplexStruct(t *testing.T) {
	config := struct {
		Name    string            `json:"name"`
		Tags    []string          `json:"tags"`
		Options map[string]int    `json:"options"`
		Nested  map[string]string `json:"nested"`
	}{
		Name: "test-resource",
		Tags: []string{"tag1", "tag2", "tag3"},
		Options: map[string]int{
			"retries": 3,
			"timeout": 30,
		},
		Nested: map[string]string{
			"inner": "value",
		},
	}

	hash := MustComputeHash(config)
	assert.NotEmpty(t, hash)
	assert.Len(t, hash, 64)
}

func TestComputeConfigHash_Stability(t *testing.T) {
	// Same input should always produce same hash
	config := struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}{
		Name: "test",
		Age:  25,
	}

	// Regular hash should be stable
	hash1, err := ComputeConfigHash(config)
	require.NoError(t, err)

	hash2, err := ComputeConfigHash(config)
	require.NoError(t, err)

	assert.Equal(t, hash1, hash2, "Same config should produce same hash")

	// Deterministic hash should also be stable
	hash3, err := ComputeConfigHashDeterministic(config)
	require.NoError(t, err)

	hash4, err := ComputeConfigHashDeterministic(config)
	require.NoError(t, err)

	assert.Equal(t, hash3, hash4, "Deterministic hash should be stable")
}

func TestMarshalSorted_PrimitiveTypes(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
	}{
		{"string", "hello"},
		{"int", 42},
		{"float", 3.14},
		{"bool", true},
		{"nil", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := marshalSorted(tt.input)
			require.NoError(t, err)
			assert.NotEmpty(t, result)
		})
	}
}

func TestMarshalSortedMap_EmptyMap(t *testing.T) {
	result, err := marshalSortedMap(map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, []byte("{}"), result)
}

func TestMarshalSortedSlice_EmptySlice(t *testing.T) {
	result, err := marshalSortedSlice([]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, []byte("[]"), result)
}

func TestMarshalSortedSlice_MixedTypes(t *testing.T) {
	input := []interface{}{
		"string",
		123,
		true,
		nil,
		map[string]interface{}{"key": "value"},
	}

	result, err := marshalSortedSlice(input)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.Contains(t, string(result), `"string"`)
	assert.Contains(t, string(result), "123")
	assert.Contains(t, string(result), "true")
	assert.Contains(t, string(result), "null")
}
