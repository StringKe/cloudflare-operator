package cf

import (
	"time"

	"github.com/cloudflare/cloudflare-go"
)

// TunnelConfigurationResult is an alias for cloudflare.TunnelConfigurationResult
// for use in the controller package without direct cloudflare-go imports.
type TunnelConfigurationResult = cloudflare.TunnelConfigurationResult

// Configuration is a cloudflared configuration yaml model
// https://github.com/cloudflare/cloudflared/blob/master/config/configuration.go
// Note: Both yaml and json tags are required because sigs.k8s.io/yaml uses
// json.Marshal internally, which only recognizes json tags.
type Configuration struct {
	TunnelID      string                   `yaml:"tunnel" json:"tunnel"`
	Ingress       []UnvalidatedIngressRule `yaml:"ingress,omitempty" json:"ingress,omitempty"`
	WarpRouting   WarpRoutingConfig        `yaml:"warp-routing,omitempty" json:"warp-routing,omitempty"`
	OriginRequest OriginRequestConfig      `yaml:"originRequest,omitempty" json:"originRequest,omitempty"`
	SourceFile    string                   `yaml:"credentials-file" json:"credentials-file"`
	Metrics       string                   `yaml:"metrics,omitempty" json:"metrics,omitempty"`
	NoAutoUpdate  bool                     `yaml:"no-autoupdate,omitempty" json:"no-autoupdate,omitempty"`
}

// UnvalidatedIngressRule is a cloudflared ingress entry model
type UnvalidatedIngressRule struct {
	Hostname      string              `yaml:"hostname,omitempty" json:"hostname,omitempty"`
	Path          string              `yaml:"path,omitempty" json:"path,omitempty"`
	Service       string              `yaml:"service" json:"service"`
	OriginRequest OriginRequestConfig `yaml:"originRequest,omitempty" json:"originRequest,omitempty"`
}

// WarpRoutingConfig is a cloudflared warp routing model
type WarpRoutingConfig struct {
	Enabled bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

// OriginRequestConfig is a cloudflared origin request configuration model
type OriginRequestConfig struct {
	// HTTP proxy timeout for establishing a new connection
	ConnectTimeout *time.Duration `yaml:"connectTimeout,omitempty" json:"connectTimeout,omitempty"`
	// HTTP proxy timeout for completing a TLS handshake
	TLSTimeout *time.Duration `yaml:"tlsTimeout,omitempty" json:"tlsTimeout,omitempty"`
	// HTTP proxy TCP keepalive duration
	TCPKeepAlive *time.Duration `yaml:"tcpKeepAlive,omitempty" json:"tcpKeepAlive,omitempty"`
	// HTTP proxy should disable "happy eyeballs" for IPv4/v6 fallback
	NoHappyEyeballs *bool `yaml:"noHappyEyeballs,omitempty" json:"noHappyEyeballs,omitempty"`
	// HTTP proxy maximum keepalive connection pool size
	KeepAliveConnections *int `yaml:"keepAliveConnections,omitempty" json:"keepAliveConnections,omitempty"`
	// HTTP proxy timeout for closing an idle connection
	KeepAliveTimeout *time.Duration `yaml:"keepAliveTimeout,omitempty" json:"keepAliveTimeout,omitempty"`
	// Sets the HTTP Host header for the local webserver.
	HTTPHostHeader *string `yaml:"httpHostHeader,omitempty" json:"httpHostHeader,omitempty"`
	// Hostname on the origin server certificate.
	OriginServerName *string `yaml:"originServerName,omitempty" json:"originServerName,omitempty"`
	// Path to the CA for the certificate of your origin.
	// This option should be used only if your certificate is not signed by Cloudflare.
	CAPool *string `yaml:"caPool,omitempty" json:"caPool,omitempty"`
	// Disables TLS verification of the certificate presented by your origin.
	// Will allow any certificate from the origin to be accepted.
	// Note: The connection from your machine to Cloudflare's Edge is still encrypted.
	NoTLSVerify *bool `yaml:"noTLSVerify,omitempty" json:"noTLSVerify,omitempty"`
	// Attempt to connect to origin using HTTP2. Origin must be configured as https.
	HTTP2Origin *bool `yaml:"http2Origin,omitempty" json:"http2Origin,omitempty"`
	// Disables chunked transfer encoding.
	// Useful if you are running a WSGI server.
	DisableChunkedEncoding *bool `yaml:"disableChunkedEncoding,omitempty" json:"disableChunkedEncoding,omitempty"`
	// Runs as jump host
	BastionMode *bool `yaml:"bastionMode,omitempty" json:"bastionMode,omitempty"`
	// Listen address for the proxy.
	ProxyAddress *string `yaml:"proxyAddress,omitempty" json:"proxyAddress,omitempty"`
	// Listen port for the proxy.
	ProxyPort *uint `yaml:"proxyPort,omitempty" json:"proxyPort,omitempty"`
	// Valid options are 'socks' or empty.
	ProxyType *string `yaml:"proxyType,omitempty" json:"proxyType,omitempty"`
	// IP rules for the proxy service
	IPRules []IngressIPRule `yaml:"ipRules,omitempty" json:"ipRules,omitempty"`
}

// IngressIPRule is a cloudflared origin ingress IP rule config model
type IngressIPRule struct {
	Prefix *string `yaml:"prefix,omitempty" json:"prefix,omitempty"`
	Ports  []int   `yaml:"ports,omitempty" json:"ports,omitempty"`
	Allow  bool    `yaml:"allow,omitempty" json:"allow,omitempty"`
}
