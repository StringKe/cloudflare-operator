// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package uploader

import (
	"crypto/md5" //nolint:gosec // MD5 is supported for legacy compatibility
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// Checksum algorithm constants
const (
	AlgorithmSHA256 = "sha256"
	AlgorithmSHA512 = "sha512"
	AlgorithmMD5    = "md5"
)

// VerifyChecksum verifies the checksum of data against expected value.
func VerifyChecksum(data []byte, cfg *v1alpha2.ChecksumConfig) error {
	if cfg == nil || cfg.Value == "" {
		return nil
	}

	algorithm := cfg.Algorithm
	if algorithm == "" {
		algorithm = AlgorithmSHA256
	}

	var hasher hash.Hash
	switch strings.ToLower(algorithm) {
	case AlgorithmSHA256:
		hasher = sha256.New()
	case AlgorithmSHA512:
		hasher = sha512.New()
	case AlgorithmMD5:
		hasher = md5.New() //nolint:gosec // MD5 is supported for legacy compatibility
	default:
		return fmt.Errorf("unsupported checksum algorithm: %s", algorithm)
	}

	_, _ = hasher.Write(data)
	computed := hex.EncodeToString(hasher.Sum(nil))

	// Compare checksums (case-insensitive)
	expected := strings.ToLower(cfg.Value)
	if computed != expected {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, computed)
	}

	return nil
}

// ComputeChecksum computes a checksum for the given data.
func ComputeChecksum(data []byte, algorithm string) (string, error) {
	if algorithm == "" {
		algorithm = AlgorithmSHA256
	}

	var hasher hash.Hash
	switch strings.ToLower(algorithm) {
	case AlgorithmSHA256:
		hasher = sha256.New()
	case AlgorithmSHA512:
		hasher = sha512.New()
	case AlgorithmMD5:
		hasher = md5.New() //nolint:gosec // MD5 is supported for legacy compatibility
	default:
		return "", fmt.Errorf("unsupported checksum algorithm: %s", algorithm)
	}

	_, _ = hasher.Write(data)
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
