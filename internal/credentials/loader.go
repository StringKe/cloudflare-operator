/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package credentials

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// Credentials holds the resolved Cloudflare API credentials
type Credentials struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// Domain is the default domain
	Domain string
	// APIToken is the API token (if using API token auth)
	APIToken string
	// APIKey is the Global API Key (if using Global API Key auth)
	APIKey string
	// Email is the email for Global API Key auth
	Email string
	// AuthType is the authentication type used
	AuthType networkingv1alpha2.CloudflareAuthType
}

// Loader loads Cloudflare credentials from various sources
type Loader struct {
	client client.Client
	log    logr.Logger
}

// NewLoader creates a new credential loader
func NewLoader(c client.Client, log logr.Logger) *Loader {
	return &Loader{
		client: c,
		log:    log,
	}
}

// LoadFromCredentialsRef loads credentials from a CloudflareCredentials resource
func (l *Loader) LoadFromCredentialsRef(ctx context.Context, ref *networkingv1alpha2.CloudflareCredentialsRef) (*Credentials, error) {
	if ref == nil {
		return nil, fmt.Errorf("credentialsRef is nil")
	}

	// Get the CloudflareCredentials resource
	creds := &networkingv1alpha2.CloudflareCredentials{}
	if err := l.client.Get(ctx, types.NamespacedName{Name: ref.Name}, creds); err != nil {
		return nil, fmt.Errorf("failed to get CloudflareCredentials %s: %w", ref.Name, err)
	}

	return l.loadFromCloudflareCredentials(ctx, creds)
}

// LoadDefault loads credentials from the default CloudflareCredentials resource
func (l *Loader) LoadDefault(ctx context.Context) (*Credentials, error) {
	// List all CloudflareCredentials and find the default one
	credsList := &networkingv1alpha2.CloudflareCredentialsList{}
	if err := l.client.List(ctx, credsList); err != nil {
		return nil, fmt.Errorf("failed to list CloudflareCredentials: %w", err)
	}

	for _, creds := range credsList.Items {
		if creds.Spec.IsDefault {
			return l.loadFromCloudflareCredentials(ctx, &creds)
		}
	}

	return nil, fmt.Errorf("no default CloudflareCredentials found")
}

// LoadFromCloudflareDetails loads credentials from legacy CloudflareDetails
// This maintains backwards compatibility with existing resources
func (l *Loader) LoadFromCloudflareDetails(ctx context.Context, details *networkingv1alpha2.CloudflareDetails, namespace string) (*Credentials, error) {
	// If credentialsRef is specified, use it
	if details.CredentialsRef != nil {
		creds, err := l.LoadFromCredentialsRef(ctx, details.CredentialsRef)
		if err != nil {
			return nil, err
		}
		// Override domain if specified in details
		if details.Domain != "" {
			creds.Domain = details.Domain
		}
		return creds, nil
	}

	// Legacy mode: load from inline secret reference
	if details.Secret == "" {
		// Try to load default credentials
		creds, err := l.LoadDefault(ctx)
		if err != nil {
			return nil, fmt.Errorf("no credentials specified and no default found: %w", err)
		}
		// Override domain if specified in details
		if details.Domain != "" {
			creds.Domain = details.Domain
		}
		return creds, nil
	}

	// Load from inline secret (legacy mode)
	return l.loadFromInlineSecret(ctx, details, namespace)
}

// loadFromCloudflareCredentials loads credentials from a CloudflareCredentials resource
func (l *Loader) loadFromCloudflareCredentials(ctx context.Context, creds *networkingv1alpha2.CloudflareCredentials) (*Credentials, error) {
	// Get the secret
	secret := &corev1.Secret{}
	secretNamespace := creds.Spec.SecretRef.Namespace
	if secretNamespace == "" {
		secretNamespace = "cloudflare-operator-system"
	}

	if err := l.client.Get(ctx, types.NamespacedName{
		Name:      creds.Spec.SecretRef.Name,
		Namespace: secretNamespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get secret %s/%s: %w", secretNamespace, creds.Spec.SecretRef.Name, err)
	}

	result := &Credentials{
		AccountID: creds.Spec.AccountID,
		Domain:    creds.Spec.DefaultDomain,
		AuthType:  creds.Spec.AuthType,
	}

	switch creds.Spec.AuthType {
	case networkingv1alpha2.AuthTypeAPIToken:
		tokenKey := creds.Spec.SecretRef.APITokenKey
		if tokenKey == "" {
			tokenKey = "CLOUDFLARE_API_TOKEN"
		}
		result.APIToken = string(secret.Data[tokenKey])
		if result.APIToken == "" {
			return nil, fmt.Errorf("API token not found in secret (key: %s)", tokenKey)
		}

	case networkingv1alpha2.AuthTypeGlobalAPIKey:
		keyKey := creds.Spec.SecretRef.APIKeyKey
		if keyKey == "" {
			keyKey = "CLOUDFLARE_API_KEY"
		}
		emailKey := creds.Spec.SecretRef.EmailKey
		if emailKey == "" {
			emailKey = "CLOUDFLARE_EMAIL"
		}

		result.APIKey = string(secret.Data[keyKey])
		result.Email = string(secret.Data[emailKey])

		if result.APIKey == "" {
			return nil, fmt.Errorf("API key not found in secret (key: %s)", keyKey)
		}
		if result.Email == "" {
			return nil, fmt.Errorf("email not found in secret (key: %s)", emailKey)
		}

	default:
		return nil, fmt.Errorf("unknown auth type: %s", creds.Spec.AuthType)
	}

	return result, nil
}

// loadFromInlineSecret loads credentials from an inline secret reference (legacy mode)
func (l *Loader) loadFromInlineSecret(ctx context.Context, details *networkingv1alpha2.CloudflareDetails, namespace string) (*Credentials, error) {
	secret := &corev1.Secret{}
	if err := l.client.Get(ctx, types.NamespacedName{
		Name:      details.Secret,
		Namespace: namespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get secret %s/%s: %w", namespace, details.Secret, err)
	}

	result := &Credentials{
		AccountID: details.AccountId,
		Domain:    details.Domain,
	}

	// Try API Token first
	tokenKey := details.CLOUDFLARE_API_TOKEN
	if tokenKey == "" {
		tokenKey = "CLOUDFLARE_API_TOKEN"
	}
	result.APIToken = string(secret.Data[tokenKey])

	if result.APIToken != "" {
		result.AuthType = networkingv1alpha2.AuthTypeAPIToken
		return result, nil
	}

	// Fall back to Global API Key
	keyKey := details.CLOUDFLARE_API_KEY
	if keyKey == "" {
		keyKey = "CLOUDFLARE_API_KEY"
	}
	result.APIKey = string(secret.Data[keyKey])
	result.Email = details.Email

	// Try to get email from secret if not in details
	if result.Email == "" {
		result.Email = string(secret.Data["CLOUDFLARE_EMAIL"])
	}
	if result.Email == "" {
		result.Email = string(secret.Data["CLOUDFLARE_API_EMAIL"])
	}

	if result.APIKey != "" && result.Email != "" {
		result.AuthType = networkingv1alpha2.AuthTypeGlobalAPIKey
		return result, nil
	}

	return nil, fmt.Errorf("no valid credentials found in secret")
}
