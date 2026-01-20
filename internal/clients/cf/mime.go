// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"mime"
	"path/filepath"
	"strings"
)

// Common MIME types that may not be registered in the system.
// This mirrors Wrangler's behavior for consistent file serving.
var additionalMIMETypes = map[string]string{
	// Web essentials
	".html": "text/html",
	".htm":  "text/html",
	".css":  "text/css",
	".js":   "application/javascript",
	".mjs":  "application/javascript",
	".json": "application/json",
	".xml":  "application/xml",

	// Images
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".svg":  "image/svg+xml",
	".ico":  "image/x-icon",
	".webp": "image/webp",
	".avif": "image/avif",

	// Fonts
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".ttf":   "font/ttf",
	".otf":   "font/otf",
	".eot":   "application/vnd.ms-fontobject",

	// Documents
	".pdf": "application/pdf",
	".txt": "text/plain",
	".md":  "text/markdown",

	// Data formats
	".csv":  "text/csv",
	".yaml": "text/yaml",
	".yml":  "text/yaml",
	".toml": "text/toml",

	// Web app manifest and service worker
	".webmanifest": "application/manifest+json",
	".map":         "application/json",

	// Video
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".ogg":  "video/ogg",

	// Audio
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".flac": "audio/flac",

	// Archives (typically not served directly, but included for completeness)
	".zip": "application/zip",
	".gz":  "application/gzip",
	".tar": "application/x-tar",

	// WebAssembly
	".wasm": "application/wasm",
}

// GetContentType determines the MIME type for a file based on its extension.
// This function mirrors Wrangler's getContentType behavior:
// 1. Tries to get MIME type from file extension
// 2. For text/* types, adds charset=utf-8 if not present
// 3. Returns "application/octet-stream" for unknown types
//
// Reference: https://github.com/cloudflare/workers-sdk/blob/main/packages/workers-shared/utils/helpers.ts
func GetContentType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == "" {
		return "application/octet-stream"
	}

	// First try our additional types (takes precedence for consistency)
	if contentType, ok := additionalMIMETypes[ext]; ok {
		return addCharsetIfNeeded(contentType)
	}

	// Fall back to system MIME type database
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		return "application/octet-stream"
	}

	return addCharsetIfNeeded(contentType)
}

// addCharsetIfNeeded adds charset=utf-8 to text/* MIME types if not already present.
// This matches Wrangler's behavior for consistent text file encoding.
func addCharsetIfNeeded(contentType string) string {
	if strings.HasPrefix(contentType, "text/") && !strings.Contains(contentType, "charset") {
		return contentType + "; charset=utf-8"
	}
	return contentType
}

// GetContentTypeOrNull returns the MIME type or "application/null" for unknown types.
// "application/null" is a special value that tells Cloudflare to not set a Content-Type header.
// This is useful when you want Cloudflare to auto-detect the content type.
func GetContentTypeOrNull(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == "" {
		return "application/null"
	}

	// First try our additional types
	if contentType, ok := additionalMIMETypes[ext]; ok {
		return addCharsetIfNeeded(contentType)
	}

	// Fall back to system MIME type database
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		return "application/null"
	}

	return addCharsetIfNeeded(contentType)
}
