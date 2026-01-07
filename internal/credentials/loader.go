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

// Package credentials provides utilities for loading Cloudflare API credentials
// from various sources including CloudflareCredentials resources and Kubernetes secrets.
package credentials

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// ErrCredentialsRefNil is returned when credentialsRef is nil
var ErrCredentialsRefNil = errors.New("credentialsRef is nil")

// ErrNoDefaultCredentials is returned when no default CloudflareCredentials is found
var ErrNoDefaultCredentials = errors.New("no default CloudflareCredentials found")

// ErrNoValidCredentials is returned when no valid credentials are found in secret
var ErrNoValidCredentials = errors.New("no valid credentials found in secret")

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
		return nil, ErrCredentialsRefNil
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

	for i := range credsList.Items {
		if credsList.Items[i].Spec.IsDefault {
			return l.loadFromCloudflareCredentials(ctx, &credsList.Items[i])
		}
	}

	return nil, ErrNoDefaultCredentials
}

// LoadFromCloudflareDetails loads credentials from legacy CloudflareDetails
// This maintains backwards compatibility with existing resources
func (l *Loader) LoadFromCloudflareDetails(ctx context.Context, details *networkingv1alpha2.CloudflareDetails, namespace string) (*Credentials, error) {
	// If credentialsRef is specified, use it
	if details.CredentialsRef != nil {
		return l.loadFromCredentialsRefWithDomain(ctx, details.CredentialsRef, details.Domain)
	}

	// Legacy mode: load from inline secret reference
	if details.Secret == "" {
		return l.loadDefaultWithDomain(ctx, details.Domain)
	}

	// Load from inline secret (legacy mode)
	return l.loadFromInlineSecret(ctx, details, namespace)
}

// loadFromCredentialsRefWithDomain loads credentials and overrides domain if specified
func (l *Loader) loadFromCredentialsRefWithDomain(ctx context.Context, ref *networkingv1alpha2.CloudflareCredentialsRef, domain string) (*Credentials, error) {
	creds, err := l.LoadFromCredentialsRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	if domain != "" {
		creds.Domain = domain
	}
	return creds, nil
}

// loadDefaultWithDomain loads default credentials and overrides domain if specified
func (l *Loader) loadDefaultWithDomain(ctx context.Context, domain string) (*Credentials, error) {
	creds, err := l.LoadDefault(ctx)
	if err != nil {
		return nil, fmt.Errorf("no credentials specified and no default found: %w", err)
	}
	if domain != "" {
		creds.Domain = domain
	}
	return creds, nil
}

// loadFromCloudflareCredentials loads credentials from a CloudflareCredentials resource
func (l *Loader) loadFromCloudflareCredentials(ctx context.Context, creds *networkingv1alpha2.CloudflareCredentials) (*Credentials, error) {
	secret, err := l.getCredentialsSecret(ctx, creds)
	if err != nil {
		return nil, err
	}

	result := &Credentials{
		AccountID: creds.Spec.AccountID,
		Domain:    creds.Spec.DefaultDomain,
		AuthType:  creds.Spec.AuthType,
	}

	return l.extractCredentialsFromSecret(secret, creds.Spec.SecretRef, creds.Spec.AuthType, result)
}

// getCredentialsSecret retrieves the secret for CloudflareCredentials
func (l *Loader) getCredentialsSecret(ctx context.Context, creds *networkingv1alpha2.CloudflareCredentials) (*corev1.Secret, error) {
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

	return secret, nil
}

// extractCredentialsFromSecret extracts credentials from secret based on auth type
func (l *Loader) extractCredentialsFromSecret(secret *corev1.Secret, secretRef networkingv1alpha2.SecretReference, authType networkingv1alpha2.CloudflareAuthType, result *Credentials) (*Credentials, error) {
	switch authType {
	case networkingv1alpha2.AuthTypeAPIToken:
		return l.extractAPIToken(secret, secretRef, result)
	case networkingv1alpha2.AuthTypeGlobalAPIKey:
		return l.extractGlobalAPIKey(secret, secretRef, result)
	default:
		return nil, fmt.Errorf("unknown auth type: %s", authType)
	}
}

// extractAPIToken extracts API token from secret
func (l *Loader) extractAPIToken(secret *corev1.Secret, secretRef networkingv1alpha2.SecretReference, result *Credentials) (*Credentials, error) {
	tokenKey := secretRef.APITokenKey
	if tokenKey == "" {
		tokenKey = "CLOUDFLARE_API_TOKEN"
	}
	result.APIToken = string(secret.Data[tokenKey])
	if result.APIToken == "" {
		return nil, fmt.Errorf("API token not found in secret (key: %s)", tokenKey)
	}
	return result, nil
}

// extractGlobalAPIKey extracts Global API Key and email from secret
func (l *Loader) extractGlobalAPIKey(secret *corev1.Secret, secretRef networkingv1alpha2.SecretReference, result *Credentials) (*Credentials, error) {
	keyKey := secretRef.APIKeyKey
	if keyKey == "" {
		keyKey = "CLOUDFLARE_API_KEY"
	}
	emailKey := secretRef.EmailKey
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
	if creds := l.tryAPIToken(secret, details, result); creds != nil {
		return creds, nil
	}

	// Fall back to Global API Key
	if creds := l.tryGlobalAPIKey(secret, details, result); creds != nil {
		return creds, nil
	}

	return nil, ErrNoValidCredentials
}

// tryAPIToken attempts to load API token from secret
func (l *Loader) tryAPIToken(secret *corev1.Secret, details *networkingv1alpha2.CloudflareDetails, result *Credentials) *Credentials {
	tokenKey := details.CLOUDFLARE_API_TOKEN
	if tokenKey == "" {
		tokenKey = "CLOUDFLARE_API_TOKEN"
	}
	result.APIToken = string(secret.Data[tokenKey])

	if result.APIToken != "" {
		result.AuthType = networkingv1alpha2.AuthTypeAPIToken
		return result
	}
	return nil
}

// tryGlobalAPIKey attempts to load Global API Key from secret
func (l *Loader) tryGlobalAPIKey(secret *corev1.Secret, details *networkingv1alpha2.CloudflareDetails, result *Credentials) *Credentials {
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
		return result
	}
	return nil
}
