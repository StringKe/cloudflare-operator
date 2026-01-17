// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package common

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// ComputeConfigHash computes a SHA256 hash of the given configuration.
// The hash is used to detect changes and avoid unnecessary API calls.
// Returns a hex-encoded string of the hash.
func ComputeConfigHash(config interface{}) (string, error) {
	// Sort maps to ensure consistent hashing
	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshal config for hash: %w", err)
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// ComputeConfigHashDeterministic computes a deterministic hash by normalizing the JSON.
// This handles cases where map iteration order differs.
func ComputeConfigHashDeterministic(config interface{}) (string, error) {
	// First marshal to JSON, then unmarshal into interface{} to normalize
	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}

	var normalized interface{}
	if err := json.Unmarshal(data, &normalized); err != nil {
		return "", fmt.Errorf("unmarshal config: %w", err)
	}

	// Sort and re-marshal for deterministic output
	sortedData, err := marshalSorted(normalized)
	if err != nil {
		return "", fmt.Errorf("sort config: %w", err)
	}

	hash := sha256.Sum256(sortedData)
	return hex.EncodeToString(hash[:]), nil
}

// marshalSorted marshals an interface with sorted map keys
func marshalSorted(v interface{}) ([]byte, error) {
	switch vv := v.(type) {
	case map[string]interface{}:
		return marshalSortedMap(vv)
	case []interface{}:
		return marshalSortedSlice(vv)
	default:
		return json.Marshal(v)
	}
}

//nolint:revive // cognitive complexity is acceptable for this marshalling function
func marshalSortedMap(m map[string]interface{}) ([]byte, error) {
	// Sort keys
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build sorted JSON manually
	result := []byte("{")
	for i, k := range keys {
		if i > 0 {
			result = append(result, ',')
		}

		keyBytes, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		result = append(result, keyBytes...)
		result = append(result, ':')

		valBytes, err := marshalSorted(m[k])
		if err != nil {
			return nil, err
		}
		result = append(result, valBytes...)
	}
	result = append(result, '}')

	return result, nil
}

func marshalSortedSlice(s []interface{}) ([]byte, error) {
	result := []byte("[")
	for i, v := range s {
		if i > 0 {
			result = append(result, ',')
		}

		valBytes, err := marshalSorted(v)
		if err != nil {
			return nil, err
		}
		result = append(result, valBytes...)
	}
	result = append(result, ']')

	return result, nil
}

// HashChanged compares two hashes and returns true if they differ.
// An empty previous hash always indicates a change.
func HashChanged(previous, current string) bool {
	if previous == "" {
		return true
	}
	return previous != current
}

// MustComputeHash computes a hash, panicking on error.
// Only use this for known-good configurations during initialization.
func MustComputeHash(config interface{}) string {
	hash, err := ComputeConfigHash(config)
	if err != nil {
		panic(fmt.Sprintf("failed to compute hash: %v", err))
	}
	return hash
}
