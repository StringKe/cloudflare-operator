// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha1

// CloudflareCredentials specifies how to authenticate with the Cloudflare API.
type CloudflareCredentials struct {
	// SecretRef references a Secret containing API credentials.
	// The Secret should contain either:
	// - CLOUDFLARE_API_TOKEN: An API Token with appropriate permissions
	// - CLOUDFLARE_API_KEY and CLOUDFLARE_API_EMAIL: Global API Key and email
	// +kubebuilder:validation:Required
	SecretRef SecretRef `json:"secretRef"`

	// APITokenKey is the key in the Secret containing the API Token.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=CLOUDFLARE_API_TOKEN
	APITokenKey string `json:"apiTokenKey,omitempty"`

	// APIKeyKey is the key in the Secret containing the API Key.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=CLOUDFLARE_API_KEY
	APIKeyKey string `json:"apiKeyKey,omitempty"`

	// APIEmailKey is the key in the Secret containing the account email (for API Key auth).
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=CLOUDFLARE_API_EMAIL
	APIEmailKey string `json:"apiEmailKey,omitempty"`
}

// CloudflareAccountRef references a Cloudflare account.
type CloudflareAccountRef struct {
	// Credentials for accessing the Cloudflare API.
	// +kubebuilder:validation:Required
	Credentials CloudflareCredentials `json:"credentials"`

	// AccountID is the Cloudflare Account ID.
	// If both AccountID and AccountName are provided, AccountID takes precedence.
	// +kubebuilder:validation:Optional
	AccountID string `json:"accountId,omitempty"`

	// AccountName is the Cloudflare Account Name.
	// Used as a fallback if AccountID is not provided.
	// +kubebuilder:validation:Optional
	AccountName string `json:"accountName,omitempty"`
}

// CloudflareZoneRef references a Cloudflare zone (domain).
type CloudflareZoneRef struct {
	// ZoneID is the Cloudflare Zone ID.
	// If both ZoneID and Domain are provided, ZoneID takes precedence.
	// +kubebuilder:validation:Optional
	ZoneID string `json:"zoneId,omitempty"`

	// Domain is the Cloudflare Zone domain name.
	// Used as a fallback if ZoneID is not provided.
	// +kubebuilder:validation:Optional
	Domain string `json:"domain,omitempty"`
}

// CloudflareRef provides a unified reference to Cloudflare credentials, account, and optionally zone.
// Use this for resources that need full Cloudflare API access.
type CloudflareRef struct {
	// Credentials for accessing the Cloudflare API.
	// +kubebuilder:validation:Required
	Credentials CloudflareCredentials `json:"credentials"`

	// Account references the Cloudflare account.
	// +kubebuilder:validation:Required
	Account CloudflareAccountIdentifier `json:"account"`

	// Zone references the Cloudflare zone (optional, only for zone-scoped resources).
	// +kubebuilder:validation:Optional
	Zone *CloudflareZoneIdentifier `json:"zone,omitempty"`
}

// CloudflareAccountIdentifier identifies a Cloudflare account.
type CloudflareAccountIdentifier struct {
	// ID is the Cloudflare Account ID.
	// If both ID and Name are provided, ID takes precedence.
	// +kubebuilder:validation:Optional
	ID string `json:"id,omitempty"`

	// Name is the Cloudflare Account Name.
	// Used as a fallback if ID is not provided.
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`
}

// CloudflareZoneIdentifier identifies a Cloudflare zone.
type CloudflareZoneIdentifier struct {
	// ID is the Cloudflare Zone ID.
	// If both ID and Name are provided, ID takes precedence.
	// +kubebuilder:validation:Optional
	ID string `json:"id,omitempty"`

	// Name is the Cloudflare Zone domain name.
	// Used as a fallback if ID is not provided.
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`
}

// TunnelSource specifies where to get or create a tunnel.
type TunnelSource struct {
	// ExistingTunnel references an existing tunnel by ID or name.
	// Mutually exclusive with NewTunnel.
	// +kubebuilder:validation:Optional
	ExistingTunnel *ExistingTunnelRef `json:"existingTunnel,omitempty"`

	// NewTunnel creates a new tunnel with the given name.
	// Mutually exclusive with ExistingTunnel.
	// +kubebuilder:validation:Optional
	NewTunnel *NewTunnelSpec `json:"newTunnel,omitempty"`

	// CredentialSecretRef references a Secret containing tunnel credentials.
	// Required for ExistingTunnel, optional for NewTunnel (will be created).
	// +kubebuilder:validation:Optional
	CredentialSecretRef *SecretKeyRef `json:"credentialSecretRef,omitempty"`
}

// ExistingTunnelRef references an existing Cloudflare tunnel.
type ExistingTunnelRef struct {
	// ID is the Cloudflare Tunnel ID.
	// If both ID and Name are provided, ID takes precedence.
	// +kubebuilder:validation:Optional
	ID string `json:"id,omitempty"`

	// Name is the Cloudflare Tunnel name.
	// Used as a fallback if ID is not provided.
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`
}

// NewTunnelSpec specifies parameters for creating a new tunnel.
type NewTunnelSpec struct {
	// Name for the new tunnel.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// WARPRoutingConfig specifies WARP routing configuration for a tunnel.
type WARPRoutingConfig struct {
	// Enabled enables or disables WARP routing for the tunnel.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`
}

// SplitTunnelEntry represents an entry in the split tunnel configuration.
type SplitTunnelEntry struct {
	// Address is the IP address or CIDR range.
	// +kubebuilder:validation:Optional
	Address string `json:"address,omitempty"`

	// Host is the hostname or domain.
	// +kubebuilder:validation:Optional
	Host string `json:"host,omitempty"`

	// Description for the entry.
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`
}

// FallbackDomainEntry represents an entry in the local domain fallback configuration.
type FallbackDomainEntry struct {
	// Suffix is the domain suffix (e.g., "internal.company.com").
	// +kubebuilder:validation:Required
	Suffix string `json:"suffix"`

	// DNSServer is the DNS server to use for this domain.
	// +kubebuilder:validation:Optional
	DNSServer []string `json:"dnsServer,omitempty"`

	// Description for the entry.
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`
}
