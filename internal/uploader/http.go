// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package uploader

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

const (
	// DefaultHTTPTimeout is the default timeout for HTTP requests.
	DefaultHTTPTimeout = 5 * time.Minute
)

// HTTPUploader downloads files from HTTP/HTTPS URLs.
type HTTPUploader struct {
	url                string
	headers            map[string]string
	timeout            time.Duration
	insecureSkipVerify bool
}

// NewHTTPUploader creates a new HTTP uploader from configuration.
//
//nolint:revive // cognitive complexity is acceptable for this configuration function
func NewHTTPUploader(ctx context.Context, k8sClient client.Client, namespace string, config *v1alpha2.HTTPSource) (*HTTPUploader, error) {
	if config == nil {
		return nil, errors.New("HTTP source config is nil")
	}

	if config.URL == "" {
		return nil, errors.New("HTTP URL is required")
	}

	uploader := &HTTPUploader{
		url:                config.URL,
		headers:            make(map[string]string),
		timeout:            DefaultHTTPTimeout,
		insecureSkipVerify: config.InsecureSkipVerify,
	}

	// Copy inline headers
	for k, v := range config.Headers {
		uploader.headers[k] = v
	}

	// Load headers from secret if specified
	if config.HeadersSecretRef != nil {
		secret := &corev1.Secret{}
		if err := k8sClient.Get(ctx, client.ObjectKey{
			Namespace: namespace,
			Name:      config.HeadersSecretRef.Name,
		}, secret); err != nil {
			return nil, fmt.Errorf("get headers secret %q: %w", config.HeadersSecretRef.Name, err)
		}

		for k, v := range secret.Data {
			uploader.headers[k] = string(v)
		}
	}

	// Parse timeout if specified
	if config.Timeout != nil && config.Timeout.Duration > 0 {
		uploader.timeout = config.Timeout.Duration
	}

	return uploader, nil
}

// Download fetches the file from the HTTP URL.
func (u *HTTPUploader) Download(ctx context.Context) (io.ReadCloser, error) {
	// Create HTTP client with configured timeout and TLS settings
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: u.insecureSkipVerify, //nolint:gosec // User explicitly requested this
		},
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   u.timeout,
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Add headers
	for k, v := range u.headers {
		req.Header.Set(k, v)
	}

	// Execute request
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("unexpected HTTP status: %d %s", resp.StatusCode, resp.Status)
	}

	return resp.Body, nil
}

// GetContentType returns the expected content type.
func (*HTTPUploader) GetContentType() string {
	return ContentTypeOctetStream
}
