// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExistingTunnel spec needs either a Tunnel Id or a Name to find it on Cloudflare.
type ExistingTunnel struct {
	// +kubebuilder:validation:Optional
	// Existing Tunnel ID to run on. Tunnel ID and Tunnel Name cannot be both empty. If both are provided, ID is used if valid, else falls back to Name.
	Id string `json:"id,omitempty"`

	// +kubebuilder:validation:Optional
	// Existing Tunnel name to run on. Tunnel Name and Tunnel ID cannot be both empty. If both are provided, ID is used if valid, else falls back to Name.
	Name string `json:"name,omitempty"`
}

// NewTunnel spec needs a name to create a Tunnel on Cloudflare.
type NewTunnel struct {
	// +kubebuilder:validation:Required
	// Tunnel name to create on Cloudflare.
	Name string `json:"name,omitempty"`
}

// CloudflareCredentialsRef references a CloudflareCredentials resource
type CloudflareCredentialsRef struct {
	// Name of the CloudflareCredentials resource to use
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// CloudflareDetails spec contains all the necessary parameters needed to connect to the Cloudflare API.
// You can either use credentialsRef to reference a global CloudflareCredentials resource,
// or specify inline credentials using the legacy fields (secret, accountId, etc.)
type CloudflareDetails struct {
	// +kubebuilder:validation:Optional
	// CredentialsRef references a CloudflareCredentials resource for API authentication.
	// When specified, this takes precedence over inline credential fields.
	// This is the recommended way to configure credentials.
	CredentialsRef *CloudflareCredentialsRef `json:"credentialsRef,omitempty"`

	// +kubebuilder:validation:Optional
	// Cloudflare Domain to which this tunnel belongs to.
	// Required if not using credentialsRef with a defaultDomain.
	Domain string `json:"domain,omitempty"`

	// +kubebuilder:validation:Optional
	// Secret containing Cloudflare API key/token (legacy, use credentialsRef instead)
	Secret string `json:"secret,omitempty"`

	// +kubebuilder:validation:Optional
	// Account Name in Cloudflare. AccountName and AccountId cannot be both empty. If both are provided, Account ID is used if valid, else falls back to Account Name.
	AccountName string `json:"accountName,omitempty"`

	// +kubebuilder:validation:Optional
	// Account ID in Cloudflare. AccountId and AccountName cannot be both empty. If both are provided, Account ID is used if valid, else falls back to Account Name.
	AccountId string `json:"accountId,omitempty"`

	// +kubebuilder:validation:Optional
	// Email to use along with API Key for Delete operations for new tunnels only, or as an alternate to API Token
	Email string `json:"email,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=CLOUDFLARE_API_KEY
	// Key in the secret to use for Cloudflare API Key, defaults to CLOUDFLARE_API_KEY. Needs Email also to be provided.
	// For Delete operations for new tunnels only, or as an alternate to API Token
	CLOUDFLARE_API_KEY string `json:"CLOUDFLARE_API_KEY,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=CLOUDFLARE_API_TOKEN
	// Key in the secret to use for Cloudflare API token, defaults to CLOUDFLARE_API_TOKEN
	CLOUDFLARE_API_TOKEN string `json:"CLOUDFLARE_API_TOKEN,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=CLOUDFLARE_TUNNEL_CREDENTIAL_FILE
	// Key in the secret to use as credentials.json for an existing tunnel, defaults to CLOUDFLARE_TUNNEL_CREDENTIAL_FILE
	CLOUDFLARE_TUNNEL_CREDENTIAL_FILE string `json:"CLOUDFLARE_TUNNEL_CREDENTIAL_FILE,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=CLOUDFLARE_TUNNEL_CREDENTIAL_SECRET
	// Key in the secret to use as tunnel secret for an existing tunnel, defaults to CLOUDFLARE_TUNNEL_CREDENTIAL_SECRET
	CLOUDFLARE_TUNNEL_CREDENTIAL_SECRET string `json:"CLOUDFLARE_TUNNEL_CREDENTIAL_SECRET,omitempty"`
}

// TunnelSpec defines the desired state of Tunnel
type TunnelSpec struct {
	// Deployment patch for the cloudflared deployment.
	// Follows https://kubernetes.io/docs/reference/kubectl/generated/kubectl_patch/
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:="{}"
	DeployPatch string `json:"deployPatch,omitempty"`

	// +kubebuilder:default:=false
	// +kubebuilder:validation:Optional
	// NoTlsVerify disables origin TLS certificate checks when the endpoint is HTTPS.
	NoTlsVerify bool `json:"noTlsVerify,omitempty"`

	// +kubebuilder:validation:Optional
	// OriginCaPool speficies the secret with tls.crt (and other certs as needed to be referred in the service annotation) of the Root CA to be trusted when sending traffic to HTTPS endpoints
	OriginCaPool string `json:"originCaPool,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum={"auto","quic","http2"}
	// +kubebuilder:default:="auto"
	// Protocol specifies the protocol to use for the tunnel. Defaults to auto. Options are "auto", "quic" and "http2"
	Protocol string `json:"protocol,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default:="http_status:404"
	// FallbackTarget speficies the target for requests that do not match an ingress. Defaults to http_status:404
	FallbackTarget string `json:"fallbackTarget,omitempty"`

	// +kubebuilder:validation:Required
	// Cloudflare Credentials
	Cloudflare CloudflareDetails `json:"cloudflare,omitempty"`

	// +kubebuilder:validation:Optional
	// Existing tunnel object.
	// ExistingTunnel and NewTunnel cannot be both empty and are mutually exclusive.
	ExistingTunnel *ExistingTunnel `json:"existingTunnel,omitempty"`

	// +kubebuilder:validation:Optional
	// New tunnel object.
	// NewTunnel and ExistingTunnel cannot be both empty and are mutually exclusive.
	NewTunnel *NewTunnel `json:"newTunnel,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=false
	// EnableWarpRouting enables WARP routing for this tunnel, allowing private network
	// access via WARP clients. When enabled, the tunnel can route traffic to private
	// IP ranges defined in NetworkRoute resources.
	EnableWarpRouting bool `json:"enableWarpRouting,omitempty"`
}

// TunnelStatus defines the observed state of Tunnel
type TunnelStatus struct {
	// TunnelId is the Cloudflare tunnel ID
	TunnelId string `json:"tunnelId"`

	// TunnelName is the Cloudflare tunnel name
	TunnelName string `json:"tunnelName"`

	// AccountId is the Cloudflare account ID
	AccountId string `json:"accountId"`

	// ZoneId is the Cloudflare zone ID (optional, for DNS features)
	ZoneId string `json:"zoneId"`

	// State represents the current state of the tunnel
	// +kubebuilder:validation:Enum=pending;creating;active;error;deleting
	State string `json:"state,omitempty"`

	// ObservedGeneration is the generation observed by the controller
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the tunnel's state
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:conversion:hub
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="TunnelID",type=string,JSONPath=`.status.tunnelId`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Tunnel is the Schema for the tunnels API
type Tunnel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TunnelSpec   `json:"spec,omitempty"`
	Status TunnelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TunnelList contains a list of Tunnel
type TunnelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tunnel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Tunnel{}, &TunnelList{})
}
