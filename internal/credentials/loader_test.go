// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package credentials

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

func TestNewLoader(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	loader := NewLoader(client, logr.Discard())

	assert.NotNil(t, loader)
	assert.NotNil(t, loader.client)
}

func TestLoadFromCredentialsRef_NilRef(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	loader := NewLoader(client, logr.Discard())

	creds, err := loader.LoadFromCredentialsRef(context.Background(), nil)

	assert.Nil(t, creds)
	assert.ErrorIs(t, err, ErrCredentialsRefNil)
}

func TestLoadFromCredentialsRef_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	loader := NewLoader(client, logr.Discard())

	ref := &networkingv1alpha2.CloudflareCredentialsRef{Name: "nonexistent"}
	creds, err := loader.LoadFromCredentialsRef(context.Background(), ref)

	assert.Nil(t, creds)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get CloudflareCredentials")
}

func TestLoadFromCredentialsRef_APIToken(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cf-secret",
			Namespace: "cloudflare-operator-system",
		},
		Data: map[string][]byte{
			"CLOUDFLARE_API_TOKEN": []byte("test-token-12345"),
		},
	}

	cfCreds := &networkingv1alpha2.CloudflareCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-credentials",
		},
		Spec: networkingv1alpha2.CloudflareCredentialsSpec{
			AccountID:     "account-123",
			DefaultDomain: "example.com",
			AuthType:      networkingv1alpha2.AuthTypeAPIToken,
			SecretRef: networkingv1alpha2.SecretReference{
				Name: "cf-secret",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, cfCreds).
		Build()

	loader := NewLoader(client, logr.Discard())

	ref := &networkingv1alpha2.CloudflareCredentialsRef{Name: "my-credentials"}
	creds, err := loader.LoadFromCredentialsRef(context.Background(), ref)

	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "account-123", creds.AccountID)
	assert.Equal(t, "example.com", creds.Domain)
	assert.Equal(t, "test-token-12345", creds.APIToken)
	assert.Equal(t, networkingv1alpha2.AuthTypeAPIToken, creds.AuthType)
}

func TestLoadFromCredentialsRef_GlobalAPIKey(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cf-secret",
			Namespace: "cloudflare-operator-system",
		},
		Data: map[string][]byte{
			"CLOUDFLARE_API_KEY": []byte("global-api-key-xyz"),
			"CLOUDFLARE_EMAIL":   []byte("user@example.com"),
		},
	}

	cfCreds := &networkingv1alpha2.CloudflareCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-credentials",
		},
		Spec: networkingv1alpha2.CloudflareCredentialsSpec{
			AccountID:     "account-456",
			DefaultDomain: "test.com",
			AuthType:      networkingv1alpha2.AuthTypeGlobalAPIKey,
			SecretRef: networkingv1alpha2.SecretReference{
				Name: "cf-secret",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, cfCreds).
		Build()

	loader := NewLoader(client, logr.Discard())

	ref := &networkingv1alpha2.CloudflareCredentialsRef{Name: "my-credentials"}
	creds, err := loader.LoadFromCredentialsRef(context.Background(), ref)

	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "account-456", creds.AccountID)
	assert.Equal(t, "test.com", creds.Domain)
	assert.Equal(t, "global-api-key-xyz", creds.APIKey)
	assert.Equal(t, "user@example.com", creds.Email)
	assert.Equal(t, networkingv1alpha2.AuthTypeGlobalAPIKey, creds.AuthType)
}

// nolint:dupl // similar test structure is intentional for comprehensive coverage
func TestLoadFromCredentialsRef_CustomSecretKeys(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-secret",
			Namespace: "cloudflare-operator-system",
		},
		Data: map[string][]byte{
			"my-custom-token": []byte("custom-token-value"),
		},
	}

	cfCreds := &networkingv1alpha2.CloudflareCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name: "custom-creds",
		},
		Spec: networkingv1alpha2.CloudflareCredentialsSpec{
			AccountID: "account-789",
			AuthType:  networkingv1alpha2.AuthTypeAPIToken,
			SecretRef: networkingv1alpha2.SecretReference{
				Name:        "custom-secret",
				APITokenKey: "my-custom-token",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, cfCreds).
		Build()

	loader := NewLoader(client, logr.Discard())

	ref := &networkingv1alpha2.CloudflareCredentialsRef{Name: "custom-creds"}
	creds, err := loader.LoadFromCredentialsRef(context.Background(), ref)

	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "custom-token-value", creds.APIToken)
}

func TestLoadFromCredentialsRef_SecretNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)

	cfCreds := &networkingv1alpha2.CloudflareCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name: "creds-without-secret",
		},
		Spec: networkingv1alpha2.CloudflareCredentialsSpec{
			AccountID: "account-123",
			AuthType:  networkingv1alpha2.AuthTypeAPIToken,
			SecretRef: networkingv1alpha2.SecretReference{
				Name: "nonexistent-secret",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cfCreds).
		Build()

	loader := NewLoader(client, logr.Discard())

	ref := &networkingv1alpha2.CloudflareCredentialsRef{Name: "creds-without-secret"}
	creds, err := loader.LoadFromCredentialsRef(context.Background(), ref)

	assert.Nil(t, creds)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get secret")
}

func TestLoadFromCredentialsRef_MissingAPIToken(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "empty-secret",
			Namespace: "cloudflare-operator-system",
		},
		Data: map[string][]byte{},
	}

	cfCreds := &networkingv1alpha2.CloudflareCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name: "creds-empty-secret",
		},
		Spec: networkingv1alpha2.CloudflareCredentialsSpec{
			AccountID: "account-123",
			AuthType:  networkingv1alpha2.AuthTypeAPIToken,
			SecretRef: networkingv1alpha2.SecretReference{
				Name: "empty-secret",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, cfCreds).
		Build()

	loader := NewLoader(client, logr.Discard())

	ref := &networkingv1alpha2.CloudflareCredentialsRef{Name: "creds-empty-secret"}
	creds, err := loader.LoadFromCredentialsRef(context.Background(), ref)

	assert.Nil(t, creds)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API token not found")
}

// nolint:dupl // similar test structure is intentional for comprehensive coverage
func TestLoadFromCredentialsRef_MissingGlobalAPIKey(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "partial-secret",
			Namespace: "cloudflare-operator-system",
		},
		Data: map[string][]byte{
			"CLOUDFLARE_EMAIL": []byte("user@example.com"),
		},
	}

	cfCreds := &networkingv1alpha2.CloudflareCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name: "creds-partial",
		},
		Spec: networkingv1alpha2.CloudflareCredentialsSpec{
			AccountID: "account-123",
			AuthType:  networkingv1alpha2.AuthTypeGlobalAPIKey,
			SecretRef: networkingv1alpha2.SecretReference{
				Name: "partial-secret",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, cfCreds).
		Build()

	loader := NewLoader(client, logr.Discard())

	ref := &networkingv1alpha2.CloudflareCredentialsRef{Name: "creds-partial"}
	creds, err := loader.LoadFromCredentialsRef(context.Background(), ref)

	assert.Nil(t, creds)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API key not found")
}

// nolint:dupl // similar test structure is intentional for comprehensive coverage
func TestLoadFromCredentialsRef_MissingEmail(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "key-only-secret",
			Namespace: "cloudflare-operator-system",
		},
		Data: map[string][]byte{
			"CLOUDFLARE_API_KEY": []byte("global-key"),
		},
	}

	cfCreds := &networkingv1alpha2.CloudflareCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name: "creds-no-email",
		},
		Spec: networkingv1alpha2.CloudflareCredentialsSpec{
			AccountID: "account-123",
			AuthType:  networkingv1alpha2.AuthTypeGlobalAPIKey,
			SecretRef: networkingv1alpha2.SecretReference{
				Name: "key-only-secret",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, cfCreds).
		Build()

	loader := NewLoader(client, logr.Discard())

	ref := &networkingv1alpha2.CloudflareCredentialsRef{Name: "creds-no-email"}
	creds, err := loader.LoadFromCredentialsRef(context.Background(), ref)

	assert.Nil(t, creds)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "email not found")
}

func TestLoadDefault_Found(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-secret",
			Namespace: "cloudflare-operator-system",
		},
		Data: map[string][]byte{
			"CLOUDFLARE_API_TOKEN": []byte("default-token"),
		},
	}

	defaultCreds := &networkingv1alpha2.CloudflareCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default-credentials",
		},
		Spec: networkingv1alpha2.CloudflareCredentialsSpec{
			AccountID:     "default-account",
			DefaultDomain: "default.example.com",
			AuthType:      networkingv1alpha2.AuthTypeAPIToken,
			IsDefault:     true,
			SecretRef: networkingv1alpha2.SecretReference{
				Name: "default-secret",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, defaultCreds).
		Build()

	loader := NewLoader(client, logr.Discard())

	creds, err := loader.LoadDefault(context.Background())

	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "default-account", creds.AccountID)
	assert.Equal(t, "default.example.com", creds.Domain)
	assert.Equal(t, "default-token", creds.APIToken)
}

func TestLoadDefault_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)

	// Create non-default credentials
	nonDefaultCreds := &networkingv1alpha2.CloudflareCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name: "non-default",
		},
		Spec: networkingv1alpha2.CloudflareCredentialsSpec{
			AccountID: "some-account",
			AuthType:  networkingv1alpha2.AuthTypeAPIToken,
			IsDefault: false,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nonDefaultCreds).
		Build()

	loader := NewLoader(client, logr.Discard())

	creds, err := loader.LoadDefault(context.Background())

	assert.Nil(t, creds)
	assert.ErrorIs(t, err, ErrNoDefaultCredentials)
}

func TestLoadFromCloudflareDetails_WithCredentialsRef(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ref-secret",
			Namespace: "cloudflare-operator-system",
		},
		Data: map[string][]byte{
			"CLOUDFLARE_API_TOKEN": []byte("ref-token"),
		},
	}

	cfCreds := &networkingv1alpha2.CloudflareCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ref-credentials",
		},
		Spec: networkingv1alpha2.CloudflareCredentialsSpec{
			AccountID: "ref-account",
			AuthType:  networkingv1alpha2.AuthTypeAPIToken,
			SecretRef: networkingv1alpha2.SecretReference{
				Name: "ref-secret",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, cfCreds).
		Build()

	loader := NewLoader(client, logr.Discard())

	details := &networkingv1alpha2.CloudflareDetails{
		Domain: "override.example.com",
		CredentialsRef: &networkingv1alpha2.CloudflareCredentialsRef{
			Name: "ref-credentials",
		},
	}

	creds, err := loader.LoadFromCloudflareDetails(context.Background(), details, "test-namespace")

	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "override.example.com", creds.Domain) // Domain should be overridden
	assert.Equal(t, "ref-token", creds.APIToken)
}

func TestLoadFromCloudflareDetails_LegacyInlineSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-secret",
			Namespace: "test-namespace",
		},
		Data: map[string][]byte{
			"CLOUDFLARE_API_TOKEN": []byte("legacy-token"),
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	loader := NewLoader(client, logr.Discard())

	details := &networkingv1alpha2.CloudflareDetails{
		Secret:    "legacy-secret",
		AccountId: "legacy-account",
		Domain:    "legacy.example.com",
	}

	creds, err := loader.LoadFromCloudflareDetails(context.Background(), details, "test-namespace")

	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "legacy-account", creds.AccountID)
	assert.Equal(t, "legacy.example.com", creds.Domain)
	assert.Equal(t, "legacy-token", creds.APIToken)
	assert.Equal(t, networkingv1alpha2.AuthTypeAPIToken, creds.AuthType)
}

func TestLoadFromCloudflareDetails_LegacyGlobalAPIKey(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-key-secret",
			Namespace: "test-namespace",
		},
		Data: map[string][]byte{
			"CLOUDFLARE_API_KEY": []byte("legacy-api-key"),
			"CLOUDFLARE_EMAIL":   []byte("legacy@example.com"),
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	loader := NewLoader(client, logr.Discard())

	details := &networkingv1alpha2.CloudflareDetails{
		Secret:    "legacy-key-secret",
		AccountId: "legacy-account",
		Domain:    "legacy.example.com",
	}

	creds, err := loader.LoadFromCloudflareDetails(context.Background(), details, "test-namespace")

	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "legacy-api-key", creds.APIKey)
	assert.Equal(t, "legacy@example.com", creds.Email)
	assert.Equal(t, networkingv1alpha2.AuthTypeGlobalAPIKey, creds.AuthType)
}

func TestLoadFromCloudflareDetails_LegacyEmailFromDetails(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "key-only",
			Namespace: "test-namespace",
		},
		Data: map[string][]byte{
			"CLOUDFLARE_API_KEY": []byte("api-key-value"),
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	loader := NewLoader(client, logr.Discard())

	details := &networkingv1alpha2.CloudflareDetails{
		Secret: "key-only",
		Email:  "from-details@example.com",
	}

	creds, err := loader.LoadFromCloudflareDetails(context.Background(), details, "test-namespace")

	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "api-key-value", creds.APIKey)
	assert.Equal(t, "from-details@example.com", creds.Email)
}

func TestLoadFromCloudflareDetails_NoValidCredentials(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "empty-secret",
			Namespace: "test-namespace",
		},
		Data: map[string][]byte{},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	loader := NewLoader(client, logr.Discard())

	details := &networkingv1alpha2.CloudflareDetails{
		Secret: "empty-secret",
	}

	creds, err := loader.LoadFromCloudflareDetails(context.Background(), details, "test-namespace")

	assert.Nil(t, creds)
	assert.ErrorIs(t, err, ErrNoValidCredentials)
}

func TestLoadFromCloudflareDetails_FallbackToDefault(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-secret",
			Namespace: "cloudflare-operator-system",
		},
		Data: map[string][]byte{
			"CLOUDFLARE_API_TOKEN": []byte("default-fallback-token"),
		},
	}

	defaultCreds := &networkingv1alpha2.CloudflareCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default-fallback",
		},
		Spec: networkingv1alpha2.CloudflareCredentialsSpec{
			AccountID:     "default-fallback-account",
			DefaultDomain: "default-fallback.example.com",
			AuthType:      networkingv1alpha2.AuthTypeAPIToken,
			IsDefault:     true,
			SecretRef: networkingv1alpha2.SecretReference{
				Name: "default-secret",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, defaultCreds).
		Build()

	loader := NewLoader(client, logr.Discard())

	// Details without secret or credentialsRef should fall back to default
	details := &networkingv1alpha2.CloudflareDetails{
		Domain: "override-domain.example.com",
	}

	creds, err := loader.LoadFromCloudflareDetails(context.Background(), details, "test-namespace")

	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "override-domain.example.com", creds.Domain)
	assert.Equal(t, "default-fallback-token", creds.APIToken)
}

func TestLoadFromCloudflareDetails_CustomSecretKeys(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-keys-secret",
			Namespace: "test-namespace",
		},
		Data: map[string][]byte{
			"MY_TOKEN_KEY": []byte("custom-key-token"),
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	loader := NewLoader(client, logr.Discard())

	details := &networkingv1alpha2.CloudflareDetails{
		Secret:               "custom-keys-secret",
		CLOUDFLARE_API_TOKEN: "MY_TOKEN_KEY",
	}

	creds, err := loader.LoadFromCloudflareDetails(context.Background(), details, "test-namespace")

	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "custom-key-token", creds.APIToken)
}

func TestLoadFromCloudflareDetails_AlternateEmailKey(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "alt-email-secret",
			Namespace: "test-namespace",
		},
		Data: map[string][]byte{
			"CLOUDFLARE_API_KEY":   []byte("api-key"),
			"CLOUDFLARE_API_EMAIL": []byte("alt-email@example.com"),
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	loader := NewLoader(client, logr.Discard())

	details := &networkingv1alpha2.CloudflareDetails{
		Secret: "alt-email-secret",
	}

	creds, err := loader.LoadFromCloudflareDetails(context.Background(), details, "test-namespace")

	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "api-key", creds.APIKey)
	assert.Equal(t, "alt-email@example.com", creds.Email)
}

// nolint:dupl // similar test structure is intentional for comprehensive coverage
func TestSecretInCustomNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-ns-secret",
			Namespace: "custom-namespace",
		},
		Data: map[string][]byte{
			"CLOUDFLARE_API_TOKEN": []byte("custom-ns-token"),
		},
	}

	cfCreds := &networkingv1alpha2.CloudflareCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name: "custom-ns-credentials",
		},
		Spec: networkingv1alpha2.CloudflareCredentialsSpec{
			AccountID: "custom-ns-account",
			AuthType:  networkingv1alpha2.AuthTypeAPIToken,
			SecretRef: networkingv1alpha2.SecretReference{
				Name:      "custom-ns-secret",
				Namespace: "custom-namespace",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, cfCreds).
		Build()

	loader := NewLoader(client, logr.Discard())

	ref := &networkingv1alpha2.CloudflareCredentialsRef{Name: "custom-ns-credentials"}
	creds, err := loader.LoadFromCredentialsRef(context.Background(), ref)

	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "custom-ns-token", creds.APIToken)
}

func TestUnknownAuthType(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1alpha2.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unknown-auth-secret",
			Namespace: "cloudflare-operator-system",
		},
		Data: map[string][]byte{
			"CLOUDFLARE_API_TOKEN": []byte("token"),
		},
	}

	cfCreds := &networkingv1alpha2.CloudflareCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name: "unknown-auth-creds",
		},
		Spec: networkingv1alpha2.CloudflareCredentialsSpec{
			AccountID: "account",
			AuthType:  "InvalidAuthType",
			SecretRef: networkingv1alpha2.SecretReference{
				Name: "unknown-auth-secret",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, cfCreds).
		Build()

	loader := NewLoader(client, logr.Discard())

	ref := &networkingv1alpha2.CloudflareCredentialsRef{Name: "unknown-auth-creds"}
	creds, err := loader.LoadFromCredentialsRef(context.Background(), ref)

	assert.Nil(t, creds)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown auth type")
}

func TestCredentialsStruct(t *testing.T) {
	creds := &Credentials{
		AccountID: "acc-123",
		Domain:    "example.com",
		APIToken:  "token-xyz",
		APIKey:    "",
		Email:     "",
		AuthType:  networkingv1alpha2.AuthTypeAPIToken,
	}

	assert.Equal(t, "acc-123", creds.AccountID)
	assert.Equal(t, "example.com", creds.Domain)
	assert.Equal(t, "token-xyz", creds.APIToken)
	assert.Empty(t, creds.APIKey)
	assert.Empty(t, creds.Email)
}

func TestErrorVariables(t *testing.T) {
	assert.Equal(t, "credentialsRef is nil", ErrCredentialsRefNil.Error())
	assert.Equal(t, "no default CloudflareCredentials found", ErrNoDefaultCredentials.Error())
	assert.Equal(t, "no valid credentials found in secret", ErrNoValidCredentials.Error())
}
