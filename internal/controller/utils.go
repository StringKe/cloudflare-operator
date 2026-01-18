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

	// Tunnel properties labels
	tunnelLabel          = "cloudflare-operator.io/tunnel"
	isClusterTunnelLabel = "cloudflare-operator.io/is-cluster-tunnel"
	tunnelIdLabel        = "cloudflare-operator.io/id"
	tunnelNameLabel      = "cloudflare-operator.io/name"
	tunnelKindLabel      = "cloudflare-operator.io/kind"
	tunnelAppLabel       = "cloudflare-operator.io/app"
	tunnelDomainLabel    = "cloudflare-operator.io/domain"
	tunnelFinalizer      = "cloudflare-operator.io/finalizer"

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
	cloudflareClient, err := CreateCloudflareClientFromCreds(creds)
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

// CreateCloudflareClientFromCreds creates a Cloudflare API client from loaded credentials.
// If CLOUDFLARE_API_BASE_URL environment variable is set, it uses that as the API base URL.
func CreateCloudflareClientFromCreds(creds *credentials.Credentials) (*cloudflare.API, error) {
	// Build options list - add custom base URL if configured
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
		// Fallback: try API Token first, then Global API Key
		if creds.APIToken != "" {
			return cloudflare.NewWithAPIToken(creds.APIToken, opts...)
		} else if creds.APIKey != "" && creds.Email != "" {
			return cloudflare.New(creds.APIKey, creds.Email, opts...)
		}
		return nil, errors.New("no valid API credentials found")
	}
}

// getCloudflareClient returns an initialized *cloudflare.API using either an API Key + Email or an API Token.
// If CLOUDFLARE_API_BASE_URL environment variable is set, it uses that as the API base URL.
//
// Deprecated: Use createCloudflareClientFromCreds instead.
//
//nolint:unused // kept for backward compatibility
func getCloudflareClient(apiKey, apiEmail, apiToken string) (*cloudflare.API, error) {
	// Build options list - add custom base URL if configured
	var opts []cloudflare.Option
	if baseURL := cf.GetAPIBaseURL(); baseURL != "" {
		opts = append(opts, cloudflare.BaseURL(baseURL))
	}

	if apiToken != "" {
		return cloudflare.NewWithAPIToken(apiToken, opts...)
	}
	return cloudflare.New(apiKey, apiEmail, opts...)
}

// CredentialsInfo holds the resolved credentials information needed for SyncState registration.
// This follows the Unified Sync Architecture where Resource Controllers only need
// credential metadata (accountID, credRef) but not the actual API client.
type CredentialsInfo struct {
	// AccountID is the Cloudflare account ID
	AccountID string
	// Domain is the Cloudflare domain
	Domain string
	// ZoneID is the Cloudflare zone ID (if available)
	ZoneID string
	// CredentialsRef is the reference to use for Sync Controller
	CredentialsRef networkingv1alpha2.CredentialsReference
}

// ResolveCredentialsForService resolves credentials information from CloudflareDetails
// without creating a Cloudflare API client.
//
// This function follows the Unified Sync Architecture:
// - Resource Controllers should use this to get accountID and credentialsRef
// - The actual API client creation is deferred to Sync Controllers
//
// Parameters:
//   - ctx: context for the operation
//   - c: Kubernetes client
//   - log: logger
//   - details: CloudflareDetails from the resource spec
//   - namespace: namespace for legacy inline secrets (use OperatorNamespace for cluster-scoped)
//   - statusAccountID: accountID from status (takes precedence if set)
func ResolveCredentialsForService(
	ctx context.Context,
	c client.Client,
	log logr.Logger,
	details networkingv1alpha2.CloudflareDetails,
	namespace string,
	statusAccountID string,
) (*CredentialsInfo, error) {
	// For cluster-scoped resources (empty namespace), use operator namespace for legacy secrets
	secretNamespace := namespace
	if secretNamespace == "" {
		secretNamespace = OperatorNamespace
	}

	// Create credentials loader
	loader := credentials.NewLoader(c, log)

	// Load credentials using the unified loader (no API client creation)
	creds, err := loader.LoadFromCloudflareDetails(ctx, &details, secretNamespace)
	if err != nil {
		log.Error(err, "failed to load credentials")
		return nil, err
	}

	// Build credentials info
	info := &CredentialsInfo{
		AccountID: creds.AccountID,
		Domain:    creds.Domain,
	}

	// Override with spec values if provided
	if details.AccountId != "" {
		info.AccountID = details.AccountId
	}
	if details.Domain != "" {
		info.Domain = details.Domain
	}
	if details.ZoneId != "" {
		info.ZoneID = details.ZoneId
	}

	// Use status accountID if available (already validated)
	if statusAccountID != "" {
		info.AccountID = statusAccountID
	}

	// Build CredentialsRef for SyncState
	info.CredentialsRef = BuildCredentialsRef(details, secretNamespace)

	return info, nil
}

// BuildCredentialsRef builds a CredentialsReference from CloudflareDetails.
// This is used to store the credentials reference in SyncState.
// Note: CredentialsReference only has Name field, so we store the CloudflareCredentials name.
// For legacy inline secrets, we need a fallback mechanism in Sync Controller.
func BuildCredentialsRef(details networkingv1alpha2.CloudflareDetails, _ string) networkingv1alpha2.CredentialsReference {
	// If using new CloudflareCredentials, reference it
	if details.CredentialsRef != nil {
		return networkingv1alpha2.CredentialsReference{
			Name: details.CredentialsRef.Name,
		}
	}

	// For legacy mode (inline secret) or default, use "default" CloudflareCredentials
	// The Sync Controller should handle fallback to inline secret if needed
	return networkingv1alpha2.CredentialsReference{
		Name: "default",
	}
}

// ResolveCredentialsFromRef resolves credentials information from a simple CredentialsReference
// without creating a Cloudflare API client.
//
// This function follows the Unified Sync Architecture for resources that use
// the simplified CredentialsReference (R2Bucket, R2BucketDomain, R2BucketNotification,
// RedirectRule, TransformRule, ZoneRuleset) instead of the full CloudflareDetails.
//
// Parameters:
//   - ctx: context for the operation
//   - c: Kubernetes client
//   - log: logger
//   - credRef: simple CredentialsReference from the resource spec (may be nil for default)
//
// Returns credentials info with AccountID and CredentialsRef for SyncState registration.
func ResolveCredentialsFromRef(
	ctx context.Context,
	c client.Client,
	log logr.Logger,
	credRef *networkingv1alpha2.CredentialsReference,
) (*CredentialsInfo, error) {
	// Create credentials loader
	loader := credentials.NewLoader(c, log)

	var creds *credentials.Credentials
	var err error
	var credentialsName string

	if credRef != nil && credRef.Name != "" {
		// Load from specified CloudflareCredentials
		cfCredRef := &networkingv1alpha2.CloudflareCredentialsRef{Name: credRef.Name}
		creds, err = loader.LoadFromCredentialsRef(ctx, cfCredRef)
		if err != nil {
			log.Error(err, "failed to load credentials from ref", "name", credRef.Name)
			return nil, err
		}
		credentialsName = credRef.Name
	} else {
		// Load from default CloudflareCredentials
		creds, err = loader.LoadDefault(ctx)
		if err != nil {
			log.Error(err, "failed to load default credentials")
			return nil, err
		}
		credentialsName = "default"
	}

	// Build credentials info
	info := &CredentialsInfo{
		AccountID: creds.AccountID,
		Domain:    creds.Domain,
		CredentialsRef: networkingv1alpha2.CredentialsReference{
			Name: credentialsName,
		},
	}

	return info, nil
}
