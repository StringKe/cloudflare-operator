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

//go:generate mockgen -destination=mock/mock_client.go -package=mock github.com/StringKe/cloudflare-operator/internal/clients/cf CloudflareClient

package cf

// CloudflareClient defines the interface for interacting with the Cloudflare API.
// This interface enables dependency injection and mocking for unit tests.
// The interface is intentionally large to cover all Cloudflare API operations.
//
//nolint:interfacebloat // Cloudflare API requires many methods for full coverage
type CloudflareClient interface {
	// Tunnel operations
	CreateTunnel() (string, string, error)
	DeleteTunnel() error
	ValidateAll() error
	GetAccountId() (string, error)
	GetTunnelId() (string, error)
	GetTunnelCreds(tunnelSecret string) (string, error)
	GetZoneId() (string, error)

	// DNS operations (api.go - CNAME/TXT for tunnels)
	InsertOrUpdateCName(fqdn, dnsID string) (string, error)
	DeleteDNSId(fqdn, dnsID string, created bool) error
	GetDNSCNameId(fqdn string) (string, error)
	GetManagedDnsTxt(fqdn string) (string, DnsManagedRecordTxt, bool, error)
	InsertOrUpdateTXT(fqdn, txtID, dnsID string) error

	// DNS operations (dns.go - Generic DNS records)
	CreateDNSRecord(params DNSRecordParams) (*DNSRecordResult, error)
	GetDNSRecord(zoneID, recordID string) (*DNSRecordResult, error)
	UpdateDNSRecord(zoneID, recordID string, params DNSRecordParams) (*DNSRecordResult, error)
	DeleteDNSRecord(zoneID, recordID string) error

	// Virtual Network operations
	CreateVirtualNetwork(params VirtualNetworkParams) (*VirtualNetworkResult, error)
	GetVirtualNetwork(virtualNetworkID string) (*VirtualNetworkResult, error)
	GetVirtualNetworkByName(name string) (*VirtualNetworkResult, error)
	UpdateVirtualNetwork(virtualNetworkID string, params VirtualNetworkParams) (*VirtualNetworkResult, error)
	DeleteVirtualNetwork(virtualNetworkID string) error

	// Tunnel Route operations
	CreateTunnelRoute(params TunnelRouteParams) (*TunnelRouteResult, error)
	GetTunnelRoute(network, virtualNetworkID string) (*TunnelRouteResult, error)
	UpdateTunnelRoute(network string, params TunnelRouteParams) (*TunnelRouteResult, error)
	DeleteTunnelRoute(network, virtualNetworkID string) error

	// Access Application operations
	CreateAccessApplication(params AccessApplicationParams) (*AccessApplicationResult, error)
	GetAccessApplication(applicationID string) (*AccessApplicationResult, error)
	UpdateAccessApplication(applicationID string, params AccessApplicationParams) (*AccessApplicationResult, error)
	DeleteAccessApplication(applicationID string) error
	ListAccessApplicationsByName(name string) (*AccessApplicationResult, error)

	// Access Policy operations
	CreateAccessPolicy(params AccessPolicyParams) (*AccessPolicyResult, error)
	GetAccessPolicy(applicationID, policyID string) (*AccessPolicyResult, error)
	UpdateAccessPolicy(policyID string, params AccessPolicyParams) (*AccessPolicyResult, error)
	DeleteAccessPolicy(applicationID, policyID string) error
	ListAccessPolicies(applicationID string) ([]AccessPolicyResult, error)

	// Access Group operations
	CreateAccessGroup(params AccessGroupParams) (*AccessGroupResult, error)
	GetAccessGroup(groupID string) (*AccessGroupResult, error)
	UpdateAccessGroup(groupID string, params AccessGroupParams) (*AccessGroupResult, error)
	DeleteAccessGroup(groupID string) error
	ListAccessGroupsByName(name string) (*AccessGroupResult, error)

	// Access Identity Provider operations
	CreateAccessIdentityProvider(params AccessIdentityProviderParams) (*AccessIdentityProviderResult, error)
	GetAccessIdentityProvider(idpID string) (*AccessIdentityProviderResult, error)
	UpdateAccessIdentityProvider(idpID string, params AccessIdentityProviderParams) (*AccessIdentityProviderResult, error)
	DeleteAccessIdentityProvider(idpID string) error
	ListAccessIdentityProvidersByName(name string) (*AccessIdentityProviderResult, error)

	// Access Service Token operations
	GetAccessServiceTokenByName(name string) (*AccessServiceTokenResult, error)
	CreateAccessServiceToken(name string, duration string) (*AccessServiceTokenResult, error)
	UpdateAccessServiceToken(tokenID string, name string, duration string) (*AccessServiceTokenResult, error)
	RefreshAccessServiceToken(tokenID string) (*AccessServiceTokenResult, error)
	DeleteAccessServiceToken(tokenID string) error

	// Device Posture Rule operations
	CreateDevicePostureRule(params DevicePostureRuleParams) (*DevicePostureRuleResult, error)
	GetDevicePostureRule(ruleID string) (*DevicePostureRuleResult, error)
	UpdateDevicePostureRule(ruleID string, params DevicePostureRuleParams) (*DevicePostureRuleResult, error)
	DeleteDevicePostureRule(ruleID string) error
	ListDevicePostureRulesByName(name string) (*DevicePostureRuleResult, error)

	// Gateway Rule operations
	CreateGatewayRule(params GatewayRuleParams) (*GatewayRuleResult, error)
	GetGatewayRule(ruleID string) (*GatewayRuleResult, error)
	UpdateGatewayRule(ruleID string, params GatewayRuleParams) (*GatewayRuleResult, error)
	DeleteGatewayRule(ruleID string) error
	ListGatewayRulesByName(name string) (*GatewayRuleResult, error)

	// Gateway List operations
	CreateGatewayList(params GatewayListParams) (*GatewayListResult, error)
	GetGatewayList(listID string) (*GatewayListResult, error)
	UpdateGatewayList(listID string, params GatewayListParams) (*GatewayListResult, error)
	DeleteGatewayList(listID string) error
	ListGatewayListsByName(name string) (*GatewayListResult, error)

	// Split Tunnel operations
	GetSplitTunnelExclude() ([]SplitTunnelEntry, error)
	UpdateSplitTunnelExclude(entries []SplitTunnelEntry) error
	GetSplitTunnelInclude() ([]SplitTunnelEntry, error)
	UpdateSplitTunnelInclude(entries []SplitTunnelEntry) error

	// Fallback Domain operations
	GetFallbackDomains() ([]FallbackDomainEntry, error)
	UpdateFallbackDomains(entries []FallbackDomainEntry) error

	// WARP Connector operations
	CreateWARPConnector(name string) (*WARPConnectorResult, error)
	GetWARPConnectorToken(connectorID string) (*WARPConnectorTokenResult, error)
	DeleteWARPConnector(connectorID string) error

	// Gateway Configuration operations
	UpdateGatewayConfiguration(params GatewayConfigurationParams) (*GatewayConfigurationResult, error)
}

// Ensure API implements CloudflareClient
var _ CloudflareClient = (*API)(nil)
