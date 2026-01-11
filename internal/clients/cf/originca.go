// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
)

// OriginCACertificateParams contains parameters for creating an Origin CA certificate
type OriginCACertificateParams struct {
	Hostnames       []string
	RequestType     string // "origin-rsa" or "origin-ecc"
	RequestValidity int    // days: 7, 30, 90, 365, 730, 1095, 5475
	CSR             string
}

// OriginCACertificateResult contains the result of an Origin CA certificate operation
type OriginCACertificateResult struct {
	ID          string
	Certificate string
	Hostnames   []string
	ExpiresOn   time.Time
	RequestType string
	CSR         string
}

// CreateOriginCACertificate creates a new Origin CA certificate
func (api *API) CreateOriginCACertificate(ctx context.Context, params OriginCACertificateParams) (*OriginCACertificateResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	cert, err := api.CloudflareClient.CreateOriginCACertificate(ctx, cloudflare.CreateOriginCertificateParams{
		Hostnames:       params.Hostnames,
		RequestType:     params.RequestType,
		RequestValidity: params.RequestValidity,
		CSR:             params.CSR,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create origin CA certificate: %w", err)
	}

	return &OriginCACertificateResult{
		ID:          cert.ID,
		Certificate: cert.Certificate,
		Hostnames:   cert.Hostnames,
		ExpiresOn:   cert.ExpiresOn,
		RequestType: cert.RequestType,
		CSR:         cert.CSR,
	}, nil
}

// GetOriginCACertificate retrieves an Origin CA certificate by ID
func (api *API) GetOriginCACertificate(ctx context.Context, certificateID string) (*OriginCACertificateResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	cert, err := api.CloudflareClient.GetOriginCACertificate(ctx, certificateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get origin CA certificate: %w", err)
	}

	return &OriginCACertificateResult{
		ID:          cert.ID,
		Certificate: cert.Certificate,
		Hostnames:   cert.Hostnames,
		ExpiresOn:   cert.ExpiresOn,
		RequestType: cert.RequestType,
		CSR:         cert.CSR,
	}, nil
}

// ListOriginCACertificates lists Origin CA certificates for a zone
func (api *API) ListOriginCACertificates(ctx context.Context, zoneID string) ([]OriginCACertificateResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	certs, err := api.CloudflareClient.ListOriginCACertificates(ctx, cloudflare.ListOriginCertificatesParams{
		ZoneID: zoneID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list origin CA certificates: %w", err)
	}

	results := make([]OriginCACertificateResult, len(certs))
	for i, cert := range certs {
		results[i] = OriginCACertificateResult{
			ID:          cert.ID,
			Certificate: cert.Certificate,
			Hostnames:   cert.Hostnames,
			ExpiresOn:   cert.ExpiresOn,
			RequestType: cert.RequestType,
			CSR:         cert.CSR,
		}
	}

	return results, nil
}

// RevokeOriginCACertificate revokes an Origin CA certificate
func (api *API) RevokeOriginCACertificate(ctx context.Context, certificateID string) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	_, err := api.CloudflareClient.RevokeOriginCACertificate(ctx, certificateID)
	if err != nil {
		return fmt.Errorf("failed to revoke origin CA certificate: %w", err)
	}

	return nil
}
