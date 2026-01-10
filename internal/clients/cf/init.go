// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"context"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/credentials"
)

// NewAPIClientFromDetails creates a new API client from CloudflareDetails.
// This function supports both the new CloudflareCredentials reference and legacy inline secrets.
// Priority order:
//  1. credentialsRef (if specified) - references a CloudflareCredentials resource
//  2. inline secret (if specified) - legacy mode for backwards compatibility
//  3. default CloudflareCredentials (if no credentials specified)
func NewAPIClientFromDetails(ctx context.Context, k8sClient client.Client, namespace string, details networkingv1alpha2.CloudflareDetails) (*API, error) {
	logger := log.FromContext(ctx)

	// Create credentials loader
	loader := credentials.NewLoader(k8sClient, logger)

	// Load credentials using the loader (handles all modes)
	creds, err := loader.LoadFromCloudflareDetails(ctx, &details, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to load credentials: %w", err)
	}

	// Create Cloudflare client based on auth type
	cfClient, err := createCloudflareClient(creds)
	if err != nil {
		return nil, err
	}

	api := &API{
		Log:              logger,
		CloudflareClient: cfClient,
		AccountId:        creds.AccountID,
		Domain:           creds.Domain,
	}

	// Override domain if specified in details
	if details.Domain != "" {
		api.Domain = details.Domain
	}

	return api, nil
}

// NewAPIClientFromCredentialsRef creates a new API client from a CloudflareCredentials reference.
func NewAPIClientFromCredentialsRef(ctx context.Context, k8sClient client.Client, ref *networkingv1alpha2.CloudflareCredentialsRef) (*API, error) {
	logger := log.FromContext(ctx)

	// Create credentials loader
	loader := credentials.NewLoader(k8sClient, logger)

	// Load credentials from the reference
	creds, err := loader.LoadFromCredentialsRef(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("failed to load credentials from ref: %w", err)
	}

	// Create Cloudflare client based on auth type
	cfClient, err := createCloudflareClient(creds)
	if err != nil {
		return nil, err
	}

	return &API{
		Log:              logger,
		CloudflareClient: cfClient,
		AccountId:        creds.AccountID,
		Domain:           creds.Domain,
	}, nil
}

// NewAPIClientFromDefaultCredentials creates a new API client using the default CloudflareCredentials.
func NewAPIClientFromDefaultCredentials(ctx context.Context, k8sClient client.Client) (*API, error) {
	logger := log.FromContext(ctx)

	// Create credentials loader
	loader := credentials.NewLoader(k8sClient, logger)

	// Load default credentials
	creds, err := loader.LoadDefault(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load default credentials: %w", err)
	}

	// Create Cloudflare client based on auth type
	cfClient, err := createCloudflareClient(creds)
	if err != nil {
		return nil, err
	}

	return &API{
		Log:              logger,
		CloudflareClient: cfClient,
		AccountId:        creds.AccountID,
		Domain:           creds.Domain,
	}, nil
}

// NewAPIClientFromSecret creates a new API client from a secret reference.
// This is a legacy function maintained for backwards compatibility.
func NewAPIClientFromSecret(ctx context.Context, k8sClient client.Client, secretName, namespace string, log logr.Logger) (*API, error) {
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: namespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get secret %s/%s: %w", namespace, secretName, err)
	}

	apiToken := string(secret.Data["CLOUDFLARE_API_TOKEN"])
	apiKey := string(secret.Data["CLOUDFLARE_API_KEY"])
	apiEmail := string(secret.Data["CLOUDFLARE_API_EMAIL"])

	var cfClient *cloudflare.API
	var err error

	if apiToken != "" {
		cfClient, err = cloudflare.NewWithAPIToken(apiToken)
	} else if apiKey != "" && apiEmail != "" {
		cfClient, err = cloudflare.New(apiKey, apiEmail)
	} else {
		return nil, fmt.Errorf("no valid API credentials found in secret")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create Cloudflare client: %w", err)
	}

	return &API{
		Log:              log,
		CloudflareClient: cfClient,
	}, nil
}

// createCloudflareClient creates a Cloudflare API client from loaded credentials.
func createCloudflareClient(creds *credentials.Credentials) (*cloudflare.API, error) {
	var cfClient *cloudflare.API
	var err error

	switch creds.AuthType {
	case networkingv1alpha2.AuthTypeAPIToken:
		cfClient, err = cloudflare.NewWithAPIToken(creds.APIToken)
	case networkingv1alpha2.AuthTypeGlobalAPIKey:
		cfClient, err = cloudflare.New(creds.APIKey, creds.Email)
	default:
		// Fallback: try API Token first, then Global API Key
		if creds.APIToken != "" {
			cfClient, err = cloudflare.NewWithAPIToken(creds.APIToken)
		} else if creds.APIKey != "" && creds.Email != "" {
			cfClient, err = cloudflare.New(creds.APIKey, creds.Email)
		} else {
			return nil, fmt.Errorf("no valid API credentials found")
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create Cloudflare client: %w", err)
	}

	return cfClient, nil
}
