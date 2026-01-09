package controller

import (
	"context"
	"errors"

	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/credentials"

	"github.com/cloudflare/cloudflare-go"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

const (
	tunnelProtoHTTP  = "http"
	tunnelProtoHTTPS = "https"
	tunnelProtoRDP   = "rdp"
	tunnelProtoSMB   = "smb"
	tunnelProtoSSH   = "ssh"
	tunnelProtoTCP   = "tcp"
	tunnelProtoUDP   = "udp"

	// Checksum of the config, used to restart pods in the deployment
	tunnelConfigChecksum = "cloudflare-operator.io/checksum"

	// Tunnel properties labels
	tunnelLabel          = "cloudflare-operator.io/tunnel"
	isClusterTunnelLabel = "cloudflare-operator.io/is-cluster-tunnel"
	tunnelIdLabel        = "cloudflare-operator.io/id"
	tunnelNameLabel      = "cloudflare-operator.io/name"
	tunnelKindLabel      = "cloudflare-operator.io/kind"
	tunnelAppLabel       = "cloudflare-operator.io/app"
	tunnelDomainLabel    = "cloudflare-operator.io/domain"
	tunnelFinalizer      = "cloudflare-operator.io/finalizer"
	configmapKey         = "config.yaml"

	// Annotation for tracking previous hostnames (PR #166 fix)
	tunnelPreviousHostnamesAnnotation = "cloudflare-operator.io/previous-hostnames"

	// Secret finalizer prefix - append tunnel name for multi-tunnel support (PR #158 fix)
	secretFinalizerPrefix = "cloudflare-operator.io/secret-finalizer-"
)

var tunnelValidProtoMap = map[string]bool{
	tunnelProtoHTTP:  true,
	tunnelProtoHTTPS: true,
	tunnelProtoRDP:   true,
	tunnelProtoSMB:   true,
	tunnelProtoSSH:   true,
	tunnelProtoTCP:   true,
	tunnelProtoUDP:   true,
}

// getAPIDetails loads Cloudflare API credentials and returns an API client.
// It supports both new CloudflareCredentials references and legacy inline secrets.
// For tunnel controllers, it also returns the secret for tunnel credential file access.
//
// For cluster-scoped resources (ClusterTunnel), pass empty string as namespace.
// The function will default to OperatorNamespace for legacy inline secret lookups.
func getAPIDetails(
	ctx context.Context,
	c client.Client,
	log logr.Logger,
	tunnelSpec networkingv1alpha2.TunnelSpec,
	tunnelStatus networkingv1alpha2.TunnelStatus,
	namespace string,
) (
	*cf.API,
	*corev1.Secret,
	error,
) {
	// For cluster-scoped resources (empty namespace), use operator namespace for legacy secrets
	secretNamespace := namespace
	if secretNamespace == "" {
		secretNamespace = OperatorNamespace
	}

	// Create credentials loader
	loader := credentials.NewLoader(c, log)

	// Load credentials using the unified loader
	creds, err := loader.LoadFromCloudflareDetails(ctx, &tunnelSpec.Cloudflare, secretNamespace)
	if err != nil {
		log.Error(err, "failed to load credentials")
		return nil, nil, err
	}

	// Create Cloudflare client based on auth type
	cloudflareClient, err := createCloudflareClientFromCreds(creds)
	if err != nil {
		log.Error(err, "error initializing cloudflare api client")
		return nil, nil, err
	}

	// Build API struct
	cfAPI := &cf.API{
		Log:              log,
		AccountName:      tunnelSpec.Cloudflare.AccountName,
		AccountId:        creds.AccountID,
		Domain:           creds.Domain,
		ValidAccountId:   tunnelStatus.AccountId,
		ValidTunnelId:    tunnelStatus.TunnelId,
		ValidTunnelName:  tunnelStatus.TunnelName,
		ValidZoneId:      tunnelStatus.ZoneId,
		CloudflareClient: cloudflareClient,
	}

	// Override with spec values if provided
	if tunnelSpec.Cloudflare.AccountId != "" {
		cfAPI.AccountId = tunnelSpec.Cloudflare.AccountId
	}
	if tunnelSpec.Cloudflare.Domain != "" {
		cfAPI.Domain = tunnelSpec.Cloudflare.Domain
	}

	// Get the tunnel credential secret (needed for existing tunnel credentials)
	// This is the secret that contains CLOUDFLARE_TUNNEL_CREDENTIAL_SECRET/FILE
	cfSecret := &corev1.Secret{}
	if tunnelSpec.Cloudflare.Secret != "" {
		// Legacy mode: secret is specified in spec
		// Use secretNamespace which defaults to OperatorNamespace for cluster-scoped resources
		if err := c.Get(ctx, apitypes.NamespacedName{Name: tunnelSpec.Cloudflare.Secret, Namespace: secretNamespace}, cfSecret); err != nil {
			log.V(1).Info("tunnel credential secret not found, this is fine for new tunnels", "secret", tunnelSpec.Cloudflare.Secret)
			// Don't return error here - secret might not be needed for new tunnels
		}
	} else if tunnelSpec.Cloudflare.CredentialsRef != nil {
		// New mode: get the secret from CloudflareCredentials
		credsResource := &networkingv1alpha2.CloudflareCredentials{}
		if err := c.Get(ctx, apitypes.NamespacedName{Name: tunnelSpec.Cloudflare.CredentialsRef.Name}, credsResource); err == nil {
			credsSecretNamespace := credsResource.Spec.SecretRef.Namespace
			if credsSecretNamespace == "" {
				credsSecretNamespace = OperatorNamespace
			}
			if err := c.Get(ctx, apitypes.NamespacedName{
				Name:      credsResource.Spec.SecretRef.Name,
				Namespace: credsSecretNamespace,
			}, cfSecret); err != nil {
				log.V(1).Info("credentials secret not found", "secret", credsResource.Spec.SecretRef.Name)
			}
		}
	}

	return cfAPI, cfSecret, nil
}

// createCloudflareClientFromCreds creates a Cloudflare API client from loaded credentials.
func createCloudflareClientFromCreds(creds *credentials.Credentials) (*cloudflare.API, error) {
	switch creds.AuthType {
	case networkingv1alpha2.AuthTypeAPIToken:
		return cloudflare.NewWithAPIToken(creds.APIToken)
	case networkingv1alpha2.AuthTypeGlobalAPIKey:
		return cloudflare.New(creds.APIKey, creds.Email)
	default:
		// Fallback: try API Token first, then Global API Key
		if creds.APIToken != "" {
			return cloudflare.NewWithAPIToken(creds.APIToken)
		} else if creds.APIKey != "" && creds.Email != "" {
			return cloudflare.New(creds.APIKey, creds.Email)
		}
		return nil, errors.New("no valid API credentials found")
	}
}

// getCloudflareClient returns an initialized *cloudflare.API using either an API Key + Email or an API Token
// Deprecated: Use createCloudflareClientFromCreds instead
func getCloudflareClient(apiKey, apiEmail, apiToken string) (*cloudflare.API, error) {
	if apiToken != "" {
		return cloudflare.NewWithAPIToken(apiToken)
	}
	return cloudflare.New(apiKey, apiEmail)
}
