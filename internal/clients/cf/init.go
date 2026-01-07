/*
Copyright 2025 Adyanth H.

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
)

// NewAPIClientFromDetails creates a new API client from CloudflareDetails.
func NewAPIClientFromDetails(ctx context.Context, k8sClient client.Client, namespace string, details networkingv1alpha2.CloudflareDetails) (*API, error) {
	logger := log.FromContext(ctx)

	// Get the secret containing API credentials
	secret := &corev1.Secret{}
	secretNamespace := namespace
	if secretNamespace == "" {
		secretNamespace = "cloudflare-operator-system"
	}

	if err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      details.Secret,
		Namespace: secretNamespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get secret %s/%s: %w", secretNamespace, details.Secret, err)
	}

	// Extract API credentials from secret
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

	api := &API{
		Log:              logger,
		CloudflareClient: cfClient,
		AccountId:        details.AccountId,
		Domain:           details.Domain,
	}

	return api, nil
}

// NewAPIClientFromSecret creates a new API client from a secret reference.
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
