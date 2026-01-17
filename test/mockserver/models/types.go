// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package models defines the data types for the Cloudflare API mock server.
package models

import (
	"time"
)

// Response is the standard Cloudflare API response wrapper.
type Response[T any] struct {
	Success    bool        `json:"success"`
	Errors     []APIError  `json:"errors"`
	Messages   []string    `json:"messages"`
	Result     T           `json:"result"`
	ResultInfo *ResultInfo `json:"result_info,omitempty"`
}

// APIError represents a Cloudflare API error.
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ResultInfo contains pagination information.
type ResultInfo struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Count      int `json:"count"`
	TotalCount int `json:"total_count"`
}

// Account represents a Cloudflare account.
type Account struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Zone represents a Cloudflare zone.
type Zone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// Tunnel represents a Cloudflare Tunnel.
type Tunnel struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	AccountTag   string     `json:"account_tag"`
	CreatedAt    time.Time  `json:"created_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
	Status       string     `json:"status"`
	RemoteConfig bool       `json:"remote_config"`
	ConfigSrc    string     `json:"config_src"`
	TunnelToken  string     `json:"-"` // Not returned in API, used internally
	TunnelSecret string     `json:"-"` // Not returned in API
}

// TunnelConfiguration represents the tunnel configuration.
type TunnelConfiguration struct {
	TunnelID string                  `json:"tunnel_id"`
	Version  int                     `json:"version"`
	Config   TunnelConfigurationData `json:"config"`
}

// TunnelConfigurationData contains the actual configuration.
type TunnelConfigurationData struct {
	Ingress       []IngressRule        `json:"ingress"`
	WarpRouting   *WarpRoutingConfig   `json:"warp-routing,omitempty"`
	OriginRequest *OriginRequestConfig `json:"originRequest,omitempty"`
}

// IngressRule represents a tunnel ingress rule.
type IngressRule struct {
	Hostname      string               `json:"hostname,omitempty"`
	Path          string               `json:"path,omitempty"`
	Service       string               `json:"service"`
	OriginRequest *OriginRequestConfig `json:"originRequest,omitempty"`
}

// WarpRoutingConfig represents WARP routing configuration.
type WarpRoutingConfig struct {
	Enabled bool `json:"enabled"`
}

// OriginRequestConfig represents origin request configuration.
type OriginRequestConfig struct {
	ConnectTimeout         *Duration `json:"connectTimeout,omitempty"`
	TLSTimeout             *Duration `json:"tlsTimeout,omitempty"`
	TCPKeepAlive           *Duration `json:"tcpKeepAlive,omitempty"`
	KeepAliveTimeout       *Duration `json:"keepAliveTimeout,omitempty"`
	KeepAliveConnections   *int      `json:"keepAliveConnections,omitempty"`
	NoHappyEyeballs        *bool     `json:"noHappyEyeballs,omitempty"`
	HTTPHostHeader         *string   `json:"httpHostHeader,omitempty"`
	OriginServerName       *string   `json:"originServerName,omitempty"`
	CAPool                 *string   `json:"caPool,omitempty"`
	NoTLSVerify            *bool     `json:"noTLSVerify,omitempty"`
	HTTP2Origin            *bool     `json:"http2Origin,omitempty"`
	DisableChunkedEncoding *bool     `json:"disableChunkedEncoding,omitempty"`
	BastionMode            *bool     `json:"bastionMode,omitempty"`
	ProxyAddress           *string   `json:"proxyAddress,omitempty"`
	ProxyPort              *int      `json:"proxyPort,omitempty"`
	ProxyType              *string   `json:"proxyType,omitempty"`
}

// Duration wraps time.Duration for JSON serialization.
type Duration struct {
	time.Duration
}

// DNSRecord represents a DNS record.
type DNSRecord struct {
	ID         string    `json:"id"`
	ZoneID     string    `json:"zone_id"`
	ZoneName   string    `json:"zone_name"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	Content    string    `json:"content"`
	TTL        int       `json:"ttl"`
	Proxied    *bool     `json:"proxied,omitempty"`
	Priority   *int      `json:"priority,omitempty"`
	Comment    string    `json:"comment,omitempty"`
	CreatedOn  time.Time `json:"created_on"`
	ModifiedOn time.Time `json:"modified_on"`
}

// VirtualNetwork represents a Virtual Network.
type VirtualNetwork struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Comment          string     `json:"comment"`
	IsDefaultNetwork bool       `json:"is_default_network"`
	CreatedAt        time.Time  `json:"created_at"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
}

// TunnelRoute represents a Tunnel Route.
type TunnelRoute struct {
	Network          string    `json:"network"`
	TunnelID         string    `json:"tunnel_id"`
	TunnelName       string    `json:"tunnel_name"`
	VirtualNetworkID string    `json:"virtual_network_id"`
	Comment          string    `json:"comment"`
	CreatedAt        time.Time `json:"created_at"`
}

// AccessApplication represents an Access Application.
type AccessApplication struct {
	ID                      string              `json:"id"`
	Name                    string              `json:"name"`
	Domain                  string              `json:"domain"`
	Type                    string              `json:"type"`
	SessionDuration         string              `json:"session_duration"`
	AutoRedirectToIdentity  bool                `json:"auto_redirect_to_identity"`
	EnableBindingCookie     bool                `json:"enable_binding_cookie"`
	CustomDenyMessage       string              `json:"custom_deny_message"`
	CustomDenyURL           string              `json:"custom_deny_url"`
	SameSiteCookieAttribute string              `json:"same_site_cookie_attribute"`
	LogoURL                 string              `json:"logo_url"`
	SkipInterstitial        bool                `json:"skip_interstitial"`
	AppLauncherVisible      bool                `json:"app_launcher_visible"`
	ServiceAuth401Redirect  bool                `json:"service_auth_401_redirect"`
	CreatedAt               time.Time           `json:"created_at"`
	UpdatedAt               time.Time           `json:"updated_at"`
	AllowedIdps             []string            `json:"allowed_idps,omitempty"`
	Policies                []AccessPolicy      `json:"policies,omitempty"`
	SelfHostedDomains       []string            `json:"self_hosted_domains,omitempty"`
	Destinations            []AccessDestination `json:"destinations,omitempty"`
}

// AccessDestination represents an Access Application destination.
type AccessDestination struct {
	Type string `json:"type"`
	URI  string `json:"uri"`
}

// AccessPolicy represents an Access Policy.
type AccessPolicy struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	Precedence int          `json:"precedence"`
	Decision   string       `json:"decision"`
	Include    []AccessRule `json:"include"`
	Exclude    []AccessRule `json:"exclude,omitempty"`
	Require    []AccessRule `json:"require,omitempty"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
}

// AccessRule represents an Access Policy rule.
type AccessRule struct {
	Email        *EmailRule        `json:"email,omitempty"`
	EmailDomain  *EmailDomainRule  `json:"email_domain,omitempty"`
	Everyone     *EveryoneRule     `json:"everyone,omitempty"`
	Group        *GroupRule        `json:"group,omitempty"`
	ServiceToken *ServiceTokenRule `json:"service_token,omitempty"`
	IPRange      *IPRangeRule      `json:"ip,omitempty"`
	Country      *CountryRule      `json:"geo,omitempty"`
}

// EmailRule matches specific email addresses.
type EmailRule struct {
	Email string `json:"email"`
}

// EmailDomainRule matches email domains.
type EmailDomainRule struct {
	Domain string `json:"domain"`
}

// EveryoneRule matches everyone.
type EveryoneRule struct{}

// GroupRule matches Access Groups.
type GroupRule struct {
	ID string `json:"id"`
}

// ServiceTokenRule matches service tokens.
type ServiceTokenRule struct {
	TokenID string `json:"token_id"`
}

// IPRangeRule matches IP ranges.
type IPRangeRule struct {
	IP string `json:"ip"`
}

// CountryRule matches countries.
type CountryRule struct {
	CountryCode string `json:"country_code"`
}

// AccessGroup represents an Access Group.
type AccessGroup struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Include   []AccessRule `json:"include"`
	Exclude   []AccessRule `json:"exclude,omitempty"`
	Require   []AccessRule `json:"require,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// AccessServiceToken represents an Access Service Token.
type AccessServiceToken struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret,omitempty"` // Only returned on creation
	Duration     string    `json:"duration"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AccessIdentityProvider represents an Access Identity Provider.
type AccessIdentityProvider struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	Config    map[string]interface{} `json:"config"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// GatewayRule represents a Gateway Rule.
type GatewayRule struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Precedence   int                    `json:"precedence"`
	Enabled      bool                   `json:"enabled"`
	Action       string                 `json:"action"`
	Filters      []string               `json:"filters"`
	Traffic      string                 `json:"traffic"`
	Identity     string                 `json:"identity"`
	RuleSettings map[string]interface{} `json:"rule_settings,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// GatewayList represents a Gateway List.
type GatewayList struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Type        string            `json:"type"`
	Items       []GatewayListItem `json:"items,omitempty"`
	Count       int               `json:"count"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// GatewayListItem represents an item in a Gateway List.
type GatewayListItem struct {
	Value       string    `json:"value"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// GatewayConfiguration represents the Gateway Configuration.
type GatewayConfiguration struct {
	Settings GatewaySettings `json:"settings"`
}

// GatewaySettings represents Gateway settings.
type GatewaySettings struct {
	BlockPage             *BlockPageSettings             `json:"block_page,omitempty"`
	ActivityLog           *ActivityLogSettings           `json:"activity_log,omitempty"`
	AntiVirus             *AntiVirusSettings             `json:"antivirus,omitempty"`
	TLSDecrypt            *TLSDecryptSettings            `json:"tls_decrypt,omitempty"`
	FIPS                  *FIPSSettings                  `json:"fips,omitempty"`
	ProtocolDetection     *ProtocolDetectionSettings     `json:"protocol_detection,omitempty"`
	ExtendedEmailMatching *ExtendedEmailMatchingSettings `json:"extended_email_matching,omitempty"`
}

// BlockPageSettings represents block page settings.
type BlockPageSettings struct {
	Enabled         bool   `json:"enabled"`
	Name            string `json:"name,omitempty"`
	FooterText      string `json:"footer_text,omitempty"`
	HeaderText      string `json:"header_text,omitempty"`
	LogoPath        string `json:"logo_path,omitempty"`
	BackgroundColor string `json:"background_color,omitempty"`
	SuppressFooter  bool   `json:"suppress_footer,omitempty"`
}

// ActivityLogSettings represents activity log settings.
type ActivityLogSettings struct {
	Enabled bool `json:"enabled"`
}

// AntiVirusSettings represents antivirus settings.
type AntiVirusSettings struct {
	EnabledDownloadPhase bool `json:"enabled_download_phase"`
	EnabledUploadPhase   bool `json:"enabled_upload_phase"`
	FailClosed           bool `json:"fail_closed"`
}

// TLSDecryptSettings represents TLS decrypt settings.
type TLSDecryptSettings struct {
	Enabled bool `json:"enabled"`
}

// FIPSSettings represents FIPS settings.
type FIPSSettings struct {
	TLS bool `json:"tls"`
}

// ProtocolDetectionSettings represents protocol detection settings.
type ProtocolDetectionSettings struct {
	Enabled bool `json:"enabled"`
}

// ExtendedEmailMatchingSettings represents extended email matching settings.
type ExtendedEmailMatchingSettings struct {
	Enabled bool `json:"enabled"`
}

// DevicePostureRule represents a Device Posture Rule.
type DevicePostureRule struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Type        string                 `json:"type"`
	Schedule    string                 `json:"schedule,omitempty"`
	Expiration  string                 `json:"expiration,omitempty"`
	Match       []DevicePostureMatch   `json:"match,omitempty"`
	Input       map[string]interface{} `json:"input,omitempty"`
	CreatedAt   time.Time              `json:"created_at,omitempty"`
	UpdatedAt   time.Time              `json:"updated_at,omitempty"`
}

// DevicePostureMatch represents a device posture match condition.
type DevicePostureMatch struct {
	Platform string `json:"platform"`
}

// DeviceSettingsPolicy represents a Device Settings Policy.
type DeviceSettingsPolicy struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Description         string    `json:"description,omitempty"`
	Precedence          int       `json:"precedence"`
	Match               string    `json:"match,omitempty"`
	Default             bool      `json:"default"`
	Enabled             bool      `json:"enabled"`
	GatewayUniqueID     string    `json:"gateway_unique_id,omitempty"`
	AllowModeSwitch     *bool     `json:"allow_mode_switch,omitempty"`
	AllowUpdates        *bool     `json:"allow_updates,omitempty"`
	AllowedToLeave      *bool     `json:"allowed_to_leave,omitempty"`
	AutoConnect         *int      `json:"auto_connect,omitempty"`
	CaptivePortal       *int      `json:"captive_portal,omitempty"`
	DisableAutoFallback *bool     `json:"disable_auto_fallback,omitempty"`
	ExcludeOfficeIPs    *bool     `json:"exclude_office_ips,omitempty"`
	SupportURL          string    `json:"support_url,omitempty"`
	SwitchLocked        *bool     `json:"switch_locked,omitempty"`
	TunnelProtocol      string    `json:"tunnel_protocol,omitempty"`
	CreatedAt           time.Time `json:"created_at,omitempty"`
	UpdatedAt           time.Time `json:"updated_at,omitempty"`
}

// R2Bucket represents an R2 bucket.
type R2Bucket struct {
	Name         string    `json:"name"`
	CreationDate time.Time `json:"creation_date"`
	Location     string    `json:"location,omitempty"`
}

// WARPConnector represents a WARP Connector.
type WARPConnector struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	AccountID string    `json:"account_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	Token     string    `json:"-"` // Not returned in API
}

// SplitTunnelEntry represents a split tunnel entry.
type SplitTunnelEntry struct {
	Address     string `json:"address,omitempty"`
	Host        string `json:"host,omitempty"`
	Description string `json:"description,omitempty"`
}

// FallbackDomainEntry represents a fallback domain entry.
type FallbackDomainEntry struct {
	Suffix      string   `json:"suffix"`
	Description string   `json:"description,omitempty"`
	DNSServer   []string `json:"dns_server,omitempty"`
}

// ZoneRuleset represents a zone ruleset.
type ZoneRuleset struct {
	ID          string        `json:"id"`
	ZoneID      string        `json:"zone_id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Kind        string        `json:"kind"`
	Phase       string        `json:"phase"`
	Rules       []RulesetRule `json:"rules"`
	Version     string        `json:"version"`
	CreatedAt   time.Time     `json:"created_at,omitempty"`
	UpdatedAt   time.Time     `json:"updated_at,omitempty"`
}

// RulesetRule represents a rule within a ruleset.
type RulesetRule struct {
	ID               string                 `json:"id"`
	Action           string                 `json:"action"`
	Expression       string                 `json:"expression"`
	Description      string                 `json:"description,omitempty"`
	Enabled          bool                   `json:"enabled"`
	ActionParameters map[string]interface{} `json:"action_parameters,omitempty"`
}

// TransformRule represents a URL transform rule.
type TransformRule struct {
	ID           string                 `json:"id"`
	ZoneID       string                 `json:"zone_id"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description,omitempty"`
	Enabled      bool                   `json:"enabled"`
	Expression   string                 `json:"expression"`
	Action       string                 `json:"action"`
	ActionParams map[string]interface{} `json:"action_parameters,omitempty"`
	CreatedAt    time.Time              `json:"created_at,omitempty"`
	UpdatedAt    time.Time              `json:"updated_at,omitempty"`
}

// RedirectRule represents a redirect rule.
type RedirectRule struct {
	ID                  string    `json:"id"`
	ZoneID              string    `json:"zone_id"`
	Name                string    `json:"name"`
	Description         string    `json:"description,omitempty"`
	Enabled             bool      `json:"enabled"`
	Expression          string    `json:"expression"`
	TargetURL           string    `json:"target_url"`
	StatusCode          int       `json:"status_code"`
	PreserveQueryString bool      `json:"preserve_query_string"`
	CreatedAt           time.Time `json:"created_at,omitempty"`
	UpdatedAt           time.Time `json:"updated_at,omitempty"`
}

// OriginCACertificate represents an Origin CA certificate.
type OriginCACertificate struct {
	ID              string    `json:"id"`
	Certificate     string    `json:"certificate"`
	Hostnames       []string  `json:"hostnames"`
	ExpiresOn       time.Time `json:"expires_on"`
	RequestType     string    `json:"request_type"`
	RequestValidity int       `json:"requested_validity"`
	CSR             string    `json:"csr,omitempty"`
	CreatedAt       time.Time `json:"created_at,omitempty"`
}

// ZoneSettings represents zone settings.
type ZoneSettings struct {
	ZoneID   string                 `json:"zone_id"`
	Settings map[string]interface{} `json:"settings"`
}
