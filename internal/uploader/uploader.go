// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package uploader provides file download and extraction functionality
// for Pages Direct Upload deployments from various sources.
package uploader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

const (
	// MaxDownloadSize is the maximum allowed download size (500MB).
	// This aligns with Cloudflare Pages deployment size limits.
	MaxDownloadSize = 500 * 1024 * 1024

	// MaxFileCount is the maximum number of files allowed in an archive.
	MaxFileCount = 20000

	// MaxFileSize is the maximum size of a single file (100MB).
	MaxFileSize = 100 * 1024 * 1024

	// ContentTypeOctetStream is the default content type for binary files.
	ContentTypeOctetStream = "application/octet-stream"
)

// Uploader defines the interface for fetching deployment files from various sources.
type Uploader interface {
	// Download fetches files from the source and returns a reader.
	// The caller is responsible for closing the reader.
	Download(ctx context.Context) (io.ReadCloser, error)

	// GetContentType returns the expected content type (for archive detection).
	GetContentType() string
}

// FileManifest represents the extracted file manifest for Pages upload.
type FileManifest struct {
	// Files maps relative paths to file content.
	Files map[string][]byte

	// TotalSize is the total size of all files in bytes.
	TotalSize int64

	// FileCount is the number of files.
	FileCount int

	// SourceHash is the SHA-256 hash of the original source package file.
	// Used for tracking and identifying deployments from the same source.
	SourceHash string

	// SourceURL is the URL where the source was fetched from (if applicable).
	SourceURL string
}

// NewUploader creates an Uploader from DirectUploadSource configuration.
func NewUploader(ctx context.Context, k8sClient client.Client, namespace string, source *v1alpha2.DirectUploadSource) (Uploader, error) {
	if source == nil {
		return nil, errors.New("source is nil")
	}

	switch {
	case source.HTTP != nil:
		return NewHTTPUploader(ctx, k8sClient, namespace, source.HTTP)
	case source.S3 != nil:
		return NewS3Uploader(ctx, k8sClient, namespace, source.S3)
	case source.OCI != nil:
		return NewOCIUploader(ctx, k8sClient, namespace, source.OCI)
	default:
		return nil, errors.New("no valid source configured: must specify http, s3, or oci")
	}
}

// ProcessSource downloads, verifies, and extracts files from source.
// This is the main entry point for processing direct upload sources.
//
//nolint:revive // cognitive complexity is acceptable for this orchestration function
func ProcessSource(
	ctx context.Context,
	k8sClient client.Client,
	namespace string,
	source *v1alpha2.DirectUploadSource,
	checksum *v1alpha2.ChecksumConfig,
	archive *v1alpha2.ArchiveConfig,
) (*FileManifest, error) {
	// 1. Create uploader based on source type
	uploader, err := NewUploader(ctx, k8sClient, namespace, source)
	if err != nil {
		return nil, fmt.Errorf("create uploader: %w", err)
	}

	// 2. Download content
	reader, err := uploader.Download(ctx)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer func() { _ = reader.Close() }()

	// 3. Read all content (with size limit)
	data, err := io.ReadAll(io.LimitReader(reader, MaxDownloadSize+1))
	if err != nil {
		return nil, fmt.Errorf("read content: %w", err)
	}

	// Check if we hit the size limit
	if int64(len(data)) > MaxDownloadSize {
		return nil, fmt.Errorf("downloaded content exceeds maximum size of %d bytes", MaxDownloadSize)
	}

	// 4. Calculate source hash (SHA-256 of the raw downloaded data)
	sourceHash := computeSourceHash(data)

	// 5. Verify checksum if configured
	if checksum != nil && checksum.Value != "" {
		if err := VerifyChecksum(data, checksum); err != nil {
			return nil, fmt.Errorf("checksum verification: %w", err)
		}
	}

	// 6. Extract archive
	manifest, err := ExtractArchive(data, archive)
	if err != nil {
		return nil, fmt.Errorf("extract archive: %w", err)
	}

	// 7. Set source metadata
	manifest.SourceHash = sourceHash
	manifest.SourceURL = getSourceURL(source)

	return manifest, nil
}

// computeSourceHash calculates SHA-256 hash of the source data.
func computeSourceHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// getSourceURL extracts the source URL from the source configuration.
func getSourceURL(source *v1alpha2.DirectUploadSource) string {
	if source == nil {
		return ""
	}

	switch {
	case source.HTTP != nil:
		return source.HTTP.URL
	case source.S3 != nil:
		// Format: s3://bucket/key or endpoint/bucket/key
		if source.S3.Endpoint != "" {
			return fmt.Sprintf("%s/%s/%s", source.S3.Endpoint, source.S3.Bucket, source.S3.Key)
		}
		return fmt.Sprintf("s3://%s/%s", source.S3.Bucket, source.S3.Key)
	case source.OCI != nil:
		return fmt.Sprintf("oci://%s", source.OCI.Image)
	default:
		return ""
	}
}
