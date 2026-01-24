// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package common provides shared utilities for controllers in the simplified 3-layer architecture.
// Controllers can directly call Cloudflare API using the APIClientFactory.
package common

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/credentials"
)

// APIClientFactory creates and caches Cloudflare API clients.
// It provides a simple interface for controllers to get API clients
// without dealing with credentials resolution complexity.
type APIClientFactory struct {
	client client.Client
	log    logr.Logger
	// Note: cache and mutex removed as clients are not currently cached
	// Future optimization: add caching with proper expiration
}

// NewAPIClientFactory creates a new APIClientFactory.
func NewAPIClientFactory(c client.Client, log logr.Logger) *APIClientFactory {
	return &APIClientFactory{
		client: c,
		log:    log.WithName("api-client-factory"),
	}
}

// APIClientOptions contains options for getting an API client.
type APIClientOptions struct {
	// CloudflareDetails contains the Cloudflare configuration from the resource spec.
	// Used for resources with full CloudflareDetails (Tunnel, PagesProject, etc.)
	CloudflareDetails *networkingv1alpha2.CloudflareDetails

	// CredentialsRef is a simple reference to CloudflareCredentials.
	// Used for resources with simplified credentials (R2Bucket, etc.)
	CredentialsRef *networkingv1alpha2.CredentialsReference

	// Namespace is the namespace for resolving legacy inline secrets.
	// For cluster-scoped resources, leave empty or use OperatorNamespace.
	Namespace string

	// AccountID override from status (already validated).
	StatusAccountID string

	// ZoneID override from status (already validated).
	StatusZoneID string
}

// APIClientResult contains the API client and resolved metadata.
type APIClientResult struct {
	// API is the Cloudflare API client.
	API *cf.API

	// AccountID is the resolved Cloudflare account ID.
	AccountID string

	// ZoneID is the resolved Cloudflare zone ID (may be empty).
	ZoneID string

	// Domain is the resolved Cloudflare domain (may be empty).
	Domain string

	// CredentialsName is the name of the CloudflareCredentials used.
	CredentialsName string
}

// GetClient returns a Cloudflare API client based on the provided options.
// It resolves credentials and creates or reuses a cached client.
func (f *APIClientFactory) GetClient(ctx context.Context, opts APIClientOptions) (*APIClientResult, error) {
	loader := credentials.NewLoader(f.client, f.log)

	var creds *credentials.Credentials
	var credentialsName string
	var err error

	// Resolve credentials based on what's provided
	if opts.CloudflareDetails != nil && opts.CloudflareDetails.CredentialsRef != nil {
		// New mode: CloudflareCredentials reference
		credentialsName = opts.CloudflareDetails.CredentialsRef.Name
		creds, err = loader.LoadFromCredentialsRef(ctx, opts.CloudflareDetails.CredentialsRef)
	} else if opts.CloudflareDetails != nil && opts.CloudflareDetails.Secret != "" {
		// Legacy mode: inline secret
		namespace := opts.Namespace
		if namespace == "" {
			namespace = OperatorNamespace
		}
		creds, err = loader.LoadFromCloudflareDetails(ctx, opts.CloudflareDetails, namespace)
		credentialsName = "inline-" + opts.CloudflareDetails.Secret
	} else if opts.CredentialsRef != nil && opts.CredentialsRef.Name != "" {
		// Simple credentials reference
		credentialsName = opts.CredentialsRef.Name
		cfCredRef := &networkingv1alpha2.CloudflareCredentialsRef{Name: opts.CredentialsRef.Name}
		creds, err = loader.LoadFromCredentialsRef(ctx, cfCredRef)
	} else {
		// Default credentials
		credentialsName = "default"
		creds, err = loader.LoadDefault(ctx)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load credentials: %w", err)
	}

	// Create Cloudflare client
	cloudflareClient, err := createCloudflareClient(creds)
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloudflare client: %w", err)
	}

	// Build API struct
	api := &cf.API{
		Log:              f.log,
		CloudflareClient: cloudflareClient,
		AccountId:        creds.AccountID,
		Domain:           creds.Domain,
	}

	// Apply overrides from CloudflareDetails
	if opts.CloudflareDetails != nil {
		if opts.CloudflareDetails.AccountId != "" {
			api.AccountId = opts.CloudflareDetails.AccountId
		}
		if opts.CloudflareDetails.AccountName != "" {
			api.AccountName = opts.CloudflareDetails.AccountName
		}
		if opts.CloudflareDetails.Domain != "" {
			api.Domain = opts.CloudflareDetails.Domain
		}
	}

	// Apply status overrides (already validated)
	if opts.StatusAccountID != "" {
		api.ValidAccountId = opts.StatusAccountID
	}
	if opts.StatusZoneID != "" {
		api.ValidZoneId = opts.StatusZoneID
	}

	// Store auth info for direct API calls if needed
	switch creds.AuthType {
	case networkingv1alpha2.AuthTypeAPIToken:
		api.APIToken = creds.APIToken
	case networkingv1alpha2.AuthTypeGlobalAPIKey:
		api.APIKey = creds.APIKey
		api.APIEmail = creds.Email
	default:
		if creds.APIToken != "" {
			api.APIToken = creds.APIToken
		} else if creds.APIKey != "" {
			api.APIKey = creds.APIKey
			api.APIEmail = creds.Email
		}
	}

	result := &APIClientResult{
		API:             api,
		AccountID:       api.AccountId,
		Domain:          api.Domain,
		CredentialsName: credentialsName,
	}

	// Try to resolve zone ID if not already set
	if opts.CloudflareDetails != nil && opts.CloudflareDetails.ZoneId != "" {
		result.ZoneID = opts.CloudflareDetails.ZoneId
	}

	return result, nil
}

// GetClientForCredentials returns a Cloudflare API client for the given CloudflareCredentials name.
// This is a simplified version for resources that only need credentials name.
func (f *APIClientFactory) GetClientForCredentials(ctx context.Context, credentialsName string) (*APIClientResult, error) {
	if credentialsName == "" {
		credentialsName = "default"
	}

	return f.GetClient(ctx, APIClientOptions{
		CredentialsRef: &networkingv1alpha2.CredentialsReference{Name: credentialsName},
	})
}

// createCloudflareClient creates a Cloudflare API client from loaded credentials.
func createCloudflareClient(creds *credentials.Credentials) (*cloudflare.API, error) {
	var opts []cloudflare.Option
	if baseURL := cf.GetAPIBaseURL(); baseURL != "" {
		opts = append(opts, cloudflare.BaseURL(baseURL))
	}

	switch creds.AuthType {
	case networkingv1alpha2.AuthTypeAPIToken:
		return cloudflare.NewWithAPIToken(creds.APIToken, opts...)
	case networkingv1alpha2.AuthTypeGlobalAPIKey:
		return cloudflare.New(creds.APIKey, creds.Email, opts...)
	default:
		if creds.APIToken != "" {
			return cloudflare.NewWithAPIToken(creds.APIToken, opts...)
		} else if creds.APIKey != "" && creds.Email != "" {
			return cloudflare.New(creds.APIKey, creds.Email, opts...)
		}
		return nil, errors.New("no valid API credentials found")
	}
}

// OperatorNamespace is the namespace where the operator runs.
// This is used for cluster-scoped resources that need to look up secrets.
var OperatorNamespace = "cloudflare-operator-system"

// SetOperatorNamespace sets the operator namespace.
// This should be called during operator initialization.
func SetOperatorNamespace(ns string) {
	if ns != "" {
		OperatorNamespace = ns
	}
}
