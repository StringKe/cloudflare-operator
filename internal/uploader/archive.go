// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package uploader

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// ExtractArchive extracts files from archive data.
func ExtractArchive(data []byte, cfg *v1alpha2.ArchiveConfig) (*FileManifest, error) {
	archiveType := "tar.gz"
	stripComponents := 0
	subPath := ""

	if cfg != nil {
		if cfg.Type != "" {
			archiveType = cfg.Type
		}
		stripComponents = cfg.StripComponents
		subPath = cfg.SubPath
	}

	var manifest *FileManifest
	var err error

	switch archiveType {
	case "tar.gz":
		manifest, err = extractTarGz(data, stripComponents, subPath)
	case "tar":
		manifest, err = extractTar(bytes.NewReader(data), stripComponents, subPath)
	case "zip":
		manifest, err = extractZip(data, stripComponents, subPath)
	case "none":
		// Single file, no extraction needed
		// Use a sensible default filename
		manifest = &FileManifest{
			Files:     map[string][]byte{"index.html": data},
			TotalSize: int64(len(data)),
			FileCount: 1,
		}
	default:
		return nil, fmt.Errorf("unsupported archive type: %s", archiveType)
	}

	if err != nil {
		return nil, err
	}

	return manifest, nil
}

// extractTarGz extracts a gzip-compressed tar archive.
func extractTarGz(data []byte, stripComponents int, subPath string) (*FileManifest, error) {
	gzReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create gzip reader: %w", err)
	}
	defer func() { _ = gzReader.Close() }()

	return extractTar(gzReader, stripComponents, subPath)
}

// extractTar extracts a tar archive.
//
//nolint:revive // cyclomatic complexity is acceptable for this archive extraction function
func extractTar(reader io.Reader, stripComponents int, subPath string) (*FileManifest, error) {
	manifest := &FileManifest{
		Files: make(map[string][]byte),
	}

	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar header: %w", err)
		}

		// Skip directories
		if header.Typeflag == tar.TypeDir {
			continue
		}

		// Only process regular files
		if header.Typeflag != tar.TypeReg {
			continue
		}

		// Process the file path
		name := processFilePath(header.Name, stripComponents, subPath)
		if name == "" {
			continue
		}

		// Check file count limit
		if manifest.FileCount >= MaxFileCount {
			return nil, fmt.Errorf("too many files (max %d)", MaxFileCount)
		}

		// Check file size limit
		if header.Size > MaxFileSize {
			return nil, fmt.Errorf("file too large: %s (%d bytes, max %d)", name, header.Size, MaxFileSize)
		}

		// Read file content
		content, err := io.ReadAll(io.LimitReader(tarReader, MaxFileSize+1))
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", name, err)
		}

		// Check size after reading (in case header was wrong)
		if int64(len(content)) > MaxFileSize {
			return nil, fmt.Errorf("file too large: %s (%d bytes, max %d)", name, len(content), MaxFileSize)
		}

		manifest.Files[name] = content
		manifest.TotalSize += int64(len(content))
		manifest.FileCount++
	}

	if manifest.FileCount == 0 {
		return nil, fmt.Errorf("no files found in archive")
	}

	return manifest, nil
}

// extractZip extracts a ZIP archive.
//
//nolint:revive // cognitive complexity is acceptable for archive extraction
func extractZip(data []byte, stripComponents int, subPath string) (*FileManifest, error) {
	manifest := &FileManifest{
		Files: make(map[string][]byte),
	}

	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("create zip reader: %w", err)
	}

	for _, file := range zipReader.File {
		// Skip directories
		if file.FileInfo().IsDir() {
			continue
		}

		// Process the file path
		name := processFilePath(file.Name, stripComponents, subPath)
		if name == "" {
			continue
		}

		// Check file count limit
		if manifest.FileCount >= MaxFileCount {
			return nil, fmt.Errorf("too many files (max %d)", MaxFileCount)
		}

		// Check file size limit
		if file.UncompressedSize64 > MaxFileSize {
			return nil, fmt.Errorf("file too large: %s (%d bytes, max %d)", name, file.UncompressedSize64, MaxFileSize)
		}

		// Read file content
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open file %s: %w", name, err)
		}

		content, err := io.ReadAll(io.LimitReader(rc, MaxFileSize+1))
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", name, err)
		}

		// Check size after reading
		if int64(len(content)) > MaxFileSize {
			return nil, fmt.Errorf("file too large: %s (%d bytes, max %d)", name, len(content), MaxFileSize)
		}

		manifest.Files[name] = content
		manifest.TotalSize += int64(len(content))
		manifest.FileCount++
	}

	if manifest.FileCount == 0 {
		return nil, fmt.Errorf("no files found in archive")
	}

	return manifest, nil
}

// processFilePath processes a file path with strip components and subPath filtering.
//
//nolint:revive // cognitive complexity is acceptable for path processing
func processFilePath(name string, stripComponents int, subPath string) string {
	// Clean the path
	name = filepath.Clean(name)
	name = strings.TrimPrefix(name, "/")
	name = strings.TrimPrefix(name, "./")

	// Skip hidden files and directories
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") && part != "." {
			return ""
		}
	}

	// Apply strip components
	if stripComponents > 0 {
		parts := strings.Split(name, "/")
		if len(parts) <= stripComponents {
			return ""
		}
		name = strings.Join(parts[stripComponents:], "/")
	}

	// Apply subPath filter
	if subPath != "" {
		subPath = strings.TrimSuffix(subPath, "/")
		if !strings.HasPrefix(name, subPath+"/") && name != subPath {
			return ""
		}
		name = strings.TrimPrefix(name, subPath+"/")
	}

	// Skip empty names
	if name == "" || name == "." {
		return ""
	}

	return name
}
