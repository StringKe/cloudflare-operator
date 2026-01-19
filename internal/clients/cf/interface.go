// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

//go:generate mockgen -destination=mock/mock_client.go -package=mock github.com/StringKe/cloudflare-operator/internal/clients/cf CloudflareClient

package cf

import "context"

// CloudflareClient defines the interface for interacting with the Cloudflare API.
// This interface enables dependency injection and mocking for unit tests.
// The interface is intentionally large to cover all Cloudflare API operations.
//
//nolint:interfacebloat // Cloudflare API requires many methods for full coverage
type CloudflareClient interface {
	// Tunnel operations
	CreateTunnel(ctx context.Context) (string, string, error)
	DeleteTunnel(ctx context.Context) error
	ValidateAll(ctx context.Context) error
	GetAccountId(ctx context.Context) (string, error)
	GetTunnelId(ctx context.Context) (string, error)
	GetTunnelCreds(ctx context.Context, tunnelSecret string) (string, error)
	GetZoneId(ctx context.Context) (string, error)

	// DNS operations (api.go - CNAME/TXT for tunnels)
	InsertOrUpdateCName(ctx context.Context, fqdn, dnsID string) (string, error)
	DeleteDNSId(ctx context.Context, fqdn, dnsID string, created bool) error
	GetDNSCNameId(ctx context.Context, fqdn string) (string, error)
	GetManagedDnsTxt(ctx context.Context, fqdn string) (string, DnsManagedRecordTxt, bool, error)
	InsertOrUpdateTXT(ctx context.Context, fqdn, txtID, dnsID string) error

	// DNS operations (dns.go - Generic DNS records)
	CreateDNSRecord(ctx context.Context, params DNSRecordParams) (*DNSRecordResult, error)
	GetDNSRecord(ctx context.Context, zoneID, recordID string) (*DNSRecordResult, error)
	UpdateDNSRecord(ctx context.Context, zoneID, recordID string, params DNSRecordParams) (*DNSRecordResult, error)
	DeleteDNSRecord(ctx context.Context, zoneID, recordID string) error

	// Virtual Network operations
	CreateVirtualNetwork(ctx context.Context, params VirtualNetworkParams) (*VirtualNetworkResult, error)
	GetVirtualNetwork(ctx context.Context, virtualNetworkID string) (*VirtualNetworkResult, error)
	GetVirtualNetworkByName(ctx context.Context, name string) (*VirtualNetworkResult, error)
	UpdateVirtualNetwork(ctx context.Context, virtualNetworkID string, params VirtualNetworkParams) (*VirtualNetworkResult, error)
	DeleteVirtualNetwork(ctx context.Context, virtualNetworkID string) error

	// Tunnel Route operations
	CreateTunnelRoute(ctx context.Context, params TunnelRouteParams) (*TunnelRouteResult, error)
	GetTunnelRoute(ctx context.Context, network, virtualNetworkID string) (*TunnelRouteResult, error)
	UpdateTunnelRoute(ctx context.Context, network string, params TunnelRouteParams) (*TunnelRouteResult, error)
	DeleteTunnelRoute(ctx context.Context, network, virtualNetworkID string) error

	// Access Application operations
	CreateAccessApplication(ctx context.Context, params AccessApplicationParams) (*AccessApplicationResult, error)
	GetAccessApplication(ctx context.Context, applicationID string) (*AccessApplicationResult, error)
	UpdateAccessApplication(ctx context.Context, applicationID string, params AccessApplicationParams) (*AccessApplicationResult, error)
	DeleteAccessApplication(ctx context.Context, applicationID string) error
	ListAccessApplicationsByName(ctx context.Context, name string) (*AccessApplicationResult, error)

	// Access Policy operations
	CreateAccessPolicy(ctx context.Context, params AccessPolicyParams) (*AccessPolicyResult, error)
	GetAccessPolicy(ctx context.Context, applicationID, policyID string) (*AccessPolicyResult, error)
	UpdateAccessPolicy(ctx context.Context, policyID string, params AccessPolicyParams) (*AccessPolicyResult, error)
	DeleteAccessPolicy(ctx context.Context, applicationID, policyID string) error
	ListAccessPolicies(ctx context.Context, applicationID string) ([]AccessPolicyResult, error)

	// Access Group operations
	CreateAccessGroup(ctx context.Context, params AccessGroupParams) (*AccessGroupResult, error)
	GetAccessGroup(ctx context.Context, groupID string) (*AccessGroupResult, error)
	UpdateAccessGroup(ctx context.Context, groupID string, params AccessGroupParams) (*AccessGroupResult, error)
	DeleteAccessGroup(ctx context.Context, groupID string) error
	ListAccessGroupsByName(ctx context.Context, name string) (*AccessGroupResult, error)

	// Access Identity Provider operations
	CreateAccessIdentityProvider(ctx context.Context, params AccessIdentityProviderParams) (*AccessIdentityProviderResult, error)
	GetAccessIdentityProvider(ctx context.Context, idpID string) (*AccessIdentityProviderResult, error)
	UpdateAccessIdentityProvider(ctx context.Context, idpID string, params AccessIdentityProviderParams) (*AccessIdentityProviderResult, error)
	DeleteAccessIdentityProvider(ctx context.Context, idpID string) error
	ListAccessIdentityProvidersByName(ctx context.Context, name string) (*AccessIdentityProviderResult, error)

	// Access Service Token operations
	GetAccessServiceTokenByName(ctx context.Context, name string) (*AccessServiceTokenResult, error)
	CreateAccessServiceToken(ctx context.Context, name string, duration string) (*AccessServiceTokenResult, error)
	UpdateAccessServiceToken(ctx context.Context, tokenID string, name string, duration string) (*AccessServiceTokenResult, error)
	RefreshAccessServiceToken(ctx context.Context, tokenID string) (*AccessServiceTokenResult, error)
	DeleteAccessServiceToken(ctx context.Context, tokenID string) error

	// Device Posture Rule operations
	CreateDevicePostureRule(ctx context.Context, params DevicePostureRuleParams) (*DevicePostureRuleResult, error)
	GetDevicePostureRule(ctx context.Context, ruleID string) (*DevicePostureRuleResult, error)
	UpdateDevicePostureRule(ctx context.Context, ruleID string, params DevicePostureRuleParams) (*DevicePostureRuleResult, error)
	DeleteDevicePostureRule(ctx context.Context, ruleID string) error
	ListDevicePostureRulesByName(ctx context.Context, name string) (*DevicePostureRuleResult, error)

	// Gateway Rule operations
	CreateGatewayRule(ctx context.Context, params GatewayRuleParams) (*GatewayRuleResult, error)
	GetGatewayRule(ctx context.Context, ruleID string) (*GatewayRuleResult, error)
	UpdateGatewayRule(ctx context.Context, ruleID string, params GatewayRuleParams) (*GatewayRuleResult, error)
	DeleteGatewayRule(ctx context.Context, ruleID string) error
	ListGatewayRulesByName(ctx context.Context, name string) (*GatewayRuleResult, error)

	// Gateway List operations
	CreateGatewayList(ctx context.Context, params GatewayListParams) (*GatewayListResult, error)
	GetGatewayList(ctx context.Context, listID string) (*GatewayListResult, error)
	UpdateGatewayList(ctx context.Context, listID string, params GatewayListParams) (*GatewayListResult, error)
	DeleteGatewayList(ctx context.Context, listID string) error
	ListGatewayListsByName(ctx context.Context, name string) (*GatewayListResult, error)

	// Split Tunnel operations
	GetSplitTunnelExclude(ctx context.Context) ([]SplitTunnelEntry, error)
	UpdateSplitTunnelExclude(ctx context.Context, entries []SplitTunnelEntry) error
	GetSplitTunnelInclude(ctx context.Context) ([]SplitTunnelEntry, error)
	UpdateSplitTunnelInclude(ctx context.Context, entries []SplitTunnelEntry) error

	// Fallback Domain operations
	GetFallbackDomains(ctx context.Context) ([]FallbackDomainEntry, error)
	UpdateFallbackDomains(ctx context.Context, entries []FallbackDomainEntry) error

	// WARP Connector operations
	CreateWARPConnector(ctx context.Context, name string) (*WARPConnectorResult, error)
	GetWARPConnectorToken(ctx context.Context, connectorID string) (*WARPConnectorTokenResult, error)
	DeleteWARPConnector(ctx context.Context, connectorID string) error

	// Gateway Configuration operations
	UpdateGatewayConfiguration(ctx context.Context, params GatewayConfigurationParams) (*GatewayConfigurationResult, error)

	// Pages Project operations
	CreatePagesProject(ctx context.Context, params PagesProjectParams) (*PagesProjectResult, error)
	GetPagesProject(ctx context.Context, projectName string) (*PagesProjectResult, error)
	UpdatePagesProject(ctx context.Context, projectName string, params PagesProjectParams) (*PagesProjectResult, error)
	DeletePagesProject(ctx context.Context, projectName string) error
	ListPagesProjects(ctx context.Context) ([]PagesProjectResult, error)
	PurgePagesProjectBuildCache(ctx context.Context, projectName string) error

	// Pages Domain operations
	AddPagesDomain(ctx context.Context, projectName, domain string) (*PagesDomainResult, error)
	GetPagesDomain(ctx context.Context, projectName, domain string) (*PagesDomainResult, error)
	DeletePagesDomain(ctx context.Context, projectName, domain string) error
	PatchPagesDomain(ctx context.Context, projectName, domain string) (*PagesDomainResult, error)
	ListPagesDomains(ctx context.Context, projectName string) ([]PagesDomainResult, error)

	// Pages Deployment operations
	CreatePagesDeployment(ctx context.Context, projectName, branch string) (*PagesDeploymentResult, error)
	GetPagesDeployment(ctx context.Context, projectName, deploymentID string) (*PagesDeploymentResult, error)
	DeletePagesDeployment(ctx context.Context, projectName, deploymentID string) error
	ListPagesDeployments(ctx context.Context, projectName string) ([]PagesDeploymentResult, error)
	RetryPagesDeployment(ctx context.Context, projectName, deploymentID string) (*PagesDeploymentResult, error)
	RollbackPagesDeployment(ctx context.Context, projectName, deploymentID string) (*PagesDeploymentResult, error)
	GetPagesDeploymentLogs(ctx context.Context, projectName, deploymentID string) (*PagesDeploymentLogsResult, error)
}

// Ensure API implements CloudflareClient
var _ CloudflareClient = (*API)(nil)
