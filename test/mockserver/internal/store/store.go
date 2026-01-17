// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package store provides in-memory storage for the mock Cloudflare API server.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/StringKe/cloudflare-operator/test/mockserver/models"
)

// Store provides thread-safe in-memory storage for mock data.
type Store struct {
	mu sync.RWMutex

	// Core entities
	accounts map[string]*models.Account
	zones    map[string]*models.Zone

	// Tunnel resources
	tunnels              map[string]*models.Tunnel              // tunnelID -> Tunnel
	tunnelConfigurations map[string]*models.TunnelConfiguration // tunnelID -> Configuration
	virtualNetworks      map[string]*models.VirtualNetwork      // vnetID -> VirtualNetwork
	tunnelRoutes         map[string]*models.TunnelRoute         // network:vnetID -> TunnelRoute

	// DNS resources
	dnsRecords map[string]*models.DNSRecord // recordID -> DNSRecord

	// Access resources
	accessApplications      map[string]*models.AccessApplication      // appID -> AccessApplication
	accessPolicies          map[string]map[string]*models.AccessPolicy // appID -> policyID -> AccessPolicy
	accessGroups            map[string]*models.AccessGroup            // groupID -> AccessGroup
	accessServiceTokens     map[string]*models.AccessServiceToken     // tokenID -> AccessServiceToken
	accessIdentityProviders map[string]*models.AccessIdentityProvider // idpID -> AccessIdentityProvider

	// Gateway resources
	gatewayRules         map[string]*models.GatewayRule         // ruleID -> GatewayRule
	gatewayLists         map[string]*models.GatewayList         // listID -> GatewayList
	gatewayConfiguration *models.GatewayConfiguration

	// Device resources
	devicePostureRules     map[string]*models.DevicePostureRule     // ruleID -> DevicePostureRule
	deviceSettingsPolicies map[string]*models.DeviceSettingsPolicy  // policyID -> DeviceSettingsPolicy

	// R2 resources
	r2Buckets          map[string]*models.R2Bucket // bucketName -> R2Bucket
	r2BucketLifecycle  map[string]interface{}      // bucketName -> lifecycle rules

	// Zone Rulesets
	zoneRulesets map[string]*models.ZoneRuleset // rulesetID -> ZoneRuleset

	// WARP Connector
	warpConnectors map[string]*models.WARPConnector // connectorID -> WARPConnector

	// Split Tunnel and Fallback Domains
	splitTunnelExclude []models.SplitTunnelEntry
	splitTunnelInclude []models.SplitTunnelEntry
	fallbackDomains    []models.FallbackDomainEntry

	// Counters for generating IDs
	idCounter int64
}

// NewStore creates a new Store with initialized maps and default data.
func NewStore() *Store {
	s := &Store{
		accounts:                make(map[string]*models.Account),
		zones:                   make(map[string]*models.Zone),
		tunnels:                 make(map[string]*models.Tunnel),
		tunnelConfigurations:    make(map[string]*models.TunnelConfiguration),
		virtualNetworks:         make(map[string]*models.VirtualNetwork),
		tunnelRoutes:            make(map[string]*models.TunnelRoute),
		dnsRecords:              make(map[string]*models.DNSRecord),
		accessApplications:      make(map[string]*models.AccessApplication),
		accessPolicies:          make(map[string]map[string]*models.AccessPolicy),
		accessGroups:            make(map[string]*models.AccessGroup),
		accessServiceTokens:     make(map[string]*models.AccessServiceToken),
		accessIdentityProviders: make(map[string]*models.AccessIdentityProvider),
		gatewayRules:            make(map[string]*models.GatewayRule),
		gatewayLists:            make(map[string]*models.GatewayList),
		devicePostureRules:      make(map[string]*models.DevicePostureRule),
		deviceSettingsPolicies:  make(map[string]*models.DeviceSettingsPolicy),
		r2Buckets:               make(map[string]*models.R2Bucket),
		r2BucketLifecycle:       make(map[string]interface{}),
		zoneRulesets:            make(map[string]*models.ZoneRuleset),
		warpConnectors:          make(map[string]*models.WARPConnector),
		splitTunnelExclude:      []models.SplitTunnelEntry{},
		splitTunnelInclude:      []models.SplitTunnelEntry{},
		fallbackDomains:         []models.FallbackDomainEntry{},
		idCounter:               1000,
	}

	// Initialize with default data
	s.initDefaults()
	return s
}

// initDefaults sets up default test data.
func (s *Store) initDefaults() {
	// Default account
	s.accounts["test-account-id"] = &models.Account{
		ID:   "test-account-id",
		Name: "Test Account",
	}

	// Default zones
	s.zones["test-zone-id"] = &models.Zone{
		ID:     "test-zone-id",
		Name:   "example.com",
		Status: "active",
	}
	s.zones["test-zone-id-2"] = &models.Zone{
		ID:     "test-zone-id-2",
		Name:   "test.com",
		Status: "active",
	}

	// Default gateway configuration
	s.gatewayConfiguration = &models.GatewayConfiguration{
		Settings: models.GatewaySettings{
			ActivityLog: &models.ActivityLogSettings{Enabled: true},
		},
	}
}

// Reset clears all data and reinitializes defaults.
func (s *Store) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tunnels = make(map[string]*models.Tunnel)
	s.tunnelConfigurations = make(map[string]*models.TunnelConfiguration)
	s.virtualNetworks = make(map[string]*models.VirtualNetwork)
	s.tunnelRoutes = make(map[string]*models.TunnelRoute)
	s.dnsRecords = make(map[string]*models.DNSRecord)
	s.accessApplications = make(map[string]*models.AccessApplication)
	s.accessPolicies = make(map[string]map[string]*models.AccessPolicy)
	s.accessGroups = make(map[string]*models.AccessGroup)
	s.accessServiceTokens = make(map[string]*models.AccessServiceToken)
	s.accessIdentityProviders = make(map[string]*models.AccessIdentityProvider)
	s.gatewayRules = make(map[string]*models.GatewayRule)
	s.gatewayLists = make(map[string]*models.GatewayList)
	s.devicePostureRules = make(map[string]*models.DevicePostureRule)
	s.deviceSettingsPolicies = make(map[string]*models.DeviceSettingsPolicy)
	s.r2Buckets = make(map[string]*models.R2Bucket)
	s.r2BucketLifecycle = make(map[string]interface{})
	s.zoneRulesets = make(map[string]*models.ZoneRuleset)
	s.warpConnectors = make(map[string]*models.WARPConnector)
	s.splitTunnelExclude = []models.SplitTunnelEntry{}
	s.splitTunnelInclude = []models.SplitTunnelEntry{}
	s.fallbackDomains = []models.FallbackDomainEntry{}
	s.idCounter = 1000

	s.initDefaults()
}

// GenerateID generates a unique ID.
func (s *Store) GenerateID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idCounter++
	return generateUUID()
}

// ---- Account Operations ----

// GetAccount retrieves an account by ID.
func (s *Store) GetAccount(id string) (*models.Account, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	acc, ok := s.accounts[id]
	return acc, ok
}

// GetAccountByName retrieves an account by name.
func (s *Store) GetAccountByName(name string) (*models.Account, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, acc := range s.accounts {
		if acc.Name == name {
			return acc, true
		}
	}
	return nil, false
}

// ---- Zone Operations ----

// GetZone retrieves a zone by ID.
func (s *Store) GetZone(id string) (*models.Zone, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	zone, ok := s.zones[id]
	return zone, ok
}

// GetZoneByName retrieves a zone by name.
func (s *Store) GetZoneByName(name string) (*models.Zone, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, zone := range s.zones {
		if zone.Name == name {
			return zone, true
		}
	}
	return nil, false
}

// ListZones returns all zones.
func (s *Store) ListZones() []*models.Zone {
	s.mu.RLock()
	defer s.mu.RUnlock()
	zones := make([]*models.Zone, 0, len(s.zones))
	for _, zone := range s.zones {
		zones = append(zones, zone)
	}
	return zones
}

// ---- Tunnel Operations ----

// CreateTunnel creates a new tunnel.
func (s *Store) CreateTunnel(tunnel *models.Tunnel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tunnels[tunnel.ID] = tunnel
	// Initialize empty configuration
	s.tunnelConfigurations[tunnel.ID] = &models.TunnelConfiguration{
		TunnelID: tunnel.ID,
		Version:  1,
		Config: models.TunnelConfigurationData{
			Ingress: []models.IngressRule{{Service: "http_status:404"}},
		},
	}
}

// GetTunnel retrieves a tunnel by ID.
func (s *Store) GetTunnel(id string) (*models.Tunnel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tunnel, ok := s.tunnels[id]
	return tunnel, ok
}

// GetTunnelByName retrieves a tunnel by name.
func (s *Store) GetTunnelByName(accountID, name string) (*models.Tunnel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, tunnel := range s.tunnels {
		if tunnel.AccountTag == accountID && tunnel.Name == name && tunnel.DeletedAt == nil {
			return tunnel, true
		}
	}
	return nil, false
}

// ListTunnels returns all tunnels for an account.
func (s *Store) ListTunnels(accountID string) []*models.Tunnel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tunnels := make([]*models.Tunnel, 0)
	for _, tunnel := range s.tunnels {
		if tunnel.AccountTag == accountID && tunnel.DeletedAt == nil {
			tunnels = append(tunnels, tunnel)
		}
	}
	return tunnels
}

// DeleteTunnel soft-deletes a tunnel.
func (s *Store) DeleteTunnel(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	tunnel, ok := s.tunnels[id]
	if !ok {
		return false
	}
	now := time.Now()
	tunnel.DeletedAt = &now
	return true
}

// GetTunnelConfiguration retrieves tunnel configuration.
func (s *Store) GetTunnelConfiguration(tunnelID string) (*models.TunnelConfiguration, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	config, ok := s.tunnelConfigurations[tunnelID]
	return config, ok
}

// UpdateTunnelConfiguration updates tunnel configuration.
func (s *Store) UpdateTunnelConfiguration(tunnelID string, config models.TunnelConfigurationData) (*models.TunnelConfiguration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.tunnelConfigurations[tunnelID]
	if !ok {
		return nil, false
	}
	existing.Version++
	existing.Config = config
	return existing, true
}

// ---- Virtual Network Operations ----

// CreateVirtualNetwork creates a new virtual network.
func (s *Store) CreateVirtualNetwork(vnet *models.VirtualNetwork) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.virtualNetworks[vnet.ID] = vnet
}

// GetVirtualNetwork retrieves a virtual network by ID.
func (s *Store) GetVirtualNetwork(id string) (*models.VirtualNetwork, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	vnet, ok := s.virtualNetworks[id]
	return vnet, ok
}

// GetVirtualNetworkByName retrieves a virtual network by name.
func (s *Store) GetVirtualNetworkByName(name string) (*models.VirtualNetwork, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, vnet := range s.virtualNetworks {
		if vnet.Name == name && vnet.DeletedAt == nil {
			return vnet, true
		}
	}
	return nil, false
}

// ListVirtualNetworks returns all virtual networks.
func (s *Store) ListVirtualNetworks() []*models.VirtualNetwork {
	s.mu.RLock()
	defer s.mu.RUnlock()
	vnets := make([]*models.VirtualNetwork, 0, len(s.virtualNetworks))
	for _, vnet := range s.virtualNetworks {
		if vnet.DeletedAt == nil {
			vnets = append(vnets, vnet)
		}
	}
	return vnets
}

// UpdateVirtualNetwork updates a virtual network.
func (s *Store) UpdateVirtualNetwork(id string, update func(*models.VirtualNetwork)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	vnet, ok := s.virtualNetworks[id]
	if !ok {
		return false
	}
	update(vnet)
	return true
}

// DeleteVirtualNetwork soft-deletes a virtual network.
func (s *Store) DeleteVirtualNetwork(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	vnet, ok := s.virtualNetworks[id]
	if !ok {
		return false
	}
	now := time.Now()
	vnet.DeletedAt = &now
	return true
}

// ---- Tunnel Route Operations ----

func routeKey(network, vnetID string) string {
	return network + ":" + vnetID
}

// CreateTunnelRoute creates a new tunnel route.
func (s *Store) CreateTunnelRoute(route *models.TunnelRoute) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := routeKey(route.Network, route.VirtualNetworkID)
	s.tunnelRoutes[key] = route
}

// GetTunnelRoute retrieves a tunnel route.
func (s *Store) GetTunnelRoute(network, vnetID string) (*models.TunnelRoute, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := routeKey(network, vnetID)
	route, ok := s.tunnelRoutes[key]
	return route, ok
}

// ListTunnelRoutes returns routes filtered by tunnel ID or virtual network ID.
func (s *Store) ListTunnelRoutes(tunnelID, vnetID string) []*models.TunnelRoute {
	s.mu.RLock()
	defer s.mu.RUnlock()
	routes := make([]*models.TunnelRoute, 0)
	for _, route := range s.tunnelRoutes {
		if (tunnelID == "" || route.TunnelID == tunnelID) &&
			(vnetID == "" || route.VirtualNetworkID == vnetID) {
			routes = append(routes, route)
		}
	}
	return routes
}

// UpdateTunnelRoute updates a tunnel route.
func (s *Store) UpdateTunnelRoute(network, vnetID string, update func(*models.TunnelRoute)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := routeKey(network, vnetID)
	route, ok := s.tunnelRoutes[key]
	if !ok {
		return false
	}
	update(route)
	return true
}

// DeleteTunnelRoute deletes a tunnel route.
func (s *Store) DeleteTunnelRoute(network, vnetID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := routeKey(network, vnetID)
	if _, ok := s.tunnelRoutes[key]; !ok {
		return false
	}
	delete(s.tunnelRoutes, key)
	return true
}

// ---- DNS Record Operations ----

// CreateDNSRecord creates a new DNS record.
func (s *Store) CreateDNSRecord(record *models.DNSRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dnsRecords[record.ID] = record
}

// GetDNSRecord retrieves a DNS record by ID.
func (s *Store) GetDNSRecord(id string) (*models.DNSRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.dnsRecords[id]
	return record, ok
}

// ListDNSRecords returns DNS records filtered by zone, type, and name.
func (s *Store) ListDNSRecords(zoneID, recordType, name string) []*models.DNSRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	records := make([]*models.DNSRecord, 0)
	for _, record := range s.dnsRecords {
		if (zoneID == "" || record.ZoneID == zoneID) &&
			(recordType == "" || record.Type == recordType) &&
			(name == "" || record.Name == name) {
			records = append(records, record)
		}
	}
	return records
}

// UpdateDNSRecord updates a DNS record.
func (s *Store) UpdateDNSRecord(id string, update func(*models.DNSRecord)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.dnsRecords[id]
	if !ok {
		return false
	}
	update(record)
	record.ModifiedOn = time.Now()
	return true
}

// DeleteDNSRecord deletes a DNS record.
func (s *Store) DeleteDNSRecord(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.dnsRecords[id]; !ok {
		return false
	}
	delete(s.dnsRecords, id)
	return true
}

// ---- Access Application Operations ----

// CreateAccessApplication creates a new access application.
func (s *Store) CreateAccessApplication(app *models.AccessApplication) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accessApplications[app.ID] = app
	s.accessPolicies[app.ID] = make(map[string]*models.AccessPolicy)
}

// GetAccessApplication retrieves an access application by ID.
func (s *Store) GetAccessApplication(id string) (*models.AccessApplication, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	app, ok := s.accessApplications[id]
	return app, ok
}

// GetAccessApplicationByName retrieves an access application by name.
func (s *Store) GetAccessApplicationByName(name string) (*models.AccessApplication, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, app := range s.accessApplications {
		if app.Name == name {
			return app, true
		}
	}
	return nil, false
}

// ListAccessApplications returns all access applications.
func (s *Store) ListAccessApplications() []*models.AccessApplication {
	s.mu.RLock()
	defer s.mu.RUnlock()
	apps := make([]*models.AccessApplication, 0, len(s.accessApplications))
	for _, app := range s.accessApplications {
		apps = append(apps, app)
	}
	return apps
}

// UpdateAccessApplication updates an access application.
func (s *Store) UpdateAccessApplication(id string, update func(*models.AccessApplication)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	app, ok := s.accessApplications[id]
	if !ok {
		return false
	}
	update(app)
	app.UpdatedAt = time.Now()
	return true
}

// DeleteAccessApplication deletes an access application.
func (s *Store) DeleteAccessApplication(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.accessApplications[id]; !ok {
		return false
	}
	delete(s.accessApplications, id)
	delete(s.accessPolicies, id)
	return true
}

// ---- Access Policy Operations ----

// CreateAccessPolicy creates a new access policy.
func (s *Store) CreateAccessPolicy(appID string, policy *models.AccessPolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.accessPolicies[appID]; !ok {
		s.accessPolicies[appID] = make(map[string]*models.AccessPolicy)
	}
	s.accessPolicies[appID][policy.ID] = policy
}

// GetAccessPolicy retrieves an access policy.
func (s *Store) GetAccessPolicy(appID, policyID string) (*models.AccessPolicy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	policies, ok := s.accessPolicies[appID]
	if !ok {
		return nil, false
	}
	policy, ok := policies[policyID]
	return policy, ok
}

// ListAccessPolicies returns all policies for an application.
func (s *Store) ListAccessPolicies(appID string) []*models.AccessPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	policies, ok := s.accessPolicies[appID]
	if !ok {
		return nil
	}
	result := make([]*models.AccessPolicy, 0, len(policies))
	for _, p := range policies {
		result = append(result, p)
	}
	return result
}

// UpdateAccessPolicy updates an access policy.
func (s *Store) UpdateAccessPolicy(appID, policyID string, update func(*models.AccessPolicy)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	policies, ok := s.accessPolicies[appID]
	if !ok {
		return false
	}
	policy, ok := policies[policyID]
	if !ok {
		return false
	}
	update(policy)
	policy.UpdatedAt = time.Now()
	return true
}

// DeleteAccessPolicy deletes an access policy.
func (s *Store) DeleteAccessPolicy(appID, policyID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	policies, ok := s.accessPolicies[appID]
	if !ok {
		return false
	}
	if _, ok := policies[policyID]; !ok {
		return false
	}
	delete(policies, policyID)
	return true
}

// ---- Access Group Operations ----

// CreateAccessGroup creates a new access group.
func (s *Store) CreateAccessGroup(group *models.AccessGroup) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accessGroups[group.ID] = group
}

// GetAccessGroup retrieves an access group by ID.
func (s *Store) GetAccessGroup(id string) (*models.AccessGroup, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	group, ok := s.accessGroups[id]
	return group, ok
}

// GetAccessGroupByName retrieves an access group by name.
func (s *Store) GetAccessGroupByName(name string) (*models.AccessGroup, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, group := range s.accessGroups {
		if group.Name == name {
			return group, true
		}
	}
	return nil, false
}

// ListAccessGroups returns all access groups.
func (s *Store) ListAccessGroups() []*models.AccessGroup {
	s.mu.RLock()
	defer s.mu.RUnlock()
	groups := make([]*models.AccessGroup, 0, len(s.accessGroups))
	for _, group := range s.accessGroups {
		groups = append(groups, group)
	}
	return groups
}

// UpdateAccessGroup updates an access group.
func (s *Store) UpdateAccessGroup(id string, update func(*models.AccessGroup)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	group, ok := s.accessGroups[id]
	if !ok {
		return false
	}
	update(group)
	group.UpdatedAt = time.Now()
	return true
}

// DeleteAccessGroup deletes an access group.
func (s *Store) DeleteAccessGroup(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.accessGroups[id]; !ok {
		return false
	}
	delete(s.accessGroups, id)
	return true
}

// ---- Access Service Token Operations ----

// CreateAccessServiceToken creates a new access service token.
func (s *Store) CreateAccessServiceToken(token *models.AccessServiceToken) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accessServiceTokens[token.ID] = token
}

// GetAccessServiceToken retrieves an access service token by ID.
func (s *Store) GetAccessServiceToken(id string) (*models.AccessServiceToken, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	token, ok := s.accessServiceTokens[id]
	return token, ok
}

// GetAccessServiceTokenByName retrieves an access service token by name.
func (s *Store) GetAccessServiceTokenByName(name string) (*models.AccessServiceToken, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, token := range s.accessServiceTokens {
		if token.Name == name {
			return token, true
		}
	}
	return nil, false
}

// UpdateAccessServiceToken updates an access service token.
func (s *Store) UpdateAccessServiceToken(id string, update func(*models.AccessServiceToken)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, ok := s.accessServiceTokens[id]
	if !ok {
		return false
	}
	update(token)
	token.UpdatedAt = time.Now()
	return true
}

// DeleteAccessServiceToken deletes an access service token.
func (s *Store) DeleteAccessServiceToken(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.accessServiceTokens[id]; !ok {
		return false
	}
	delete(s.accessServiceTokens, id)
	return true
}

// ---- Access Identity Provider Operations ----

// CreateAccessIdentityProvider creates a new access identity provider.
func (s *Store) CreateAccessIdentityProvider(idp *models.AccessIdentityProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accessIdentityProviders[idp.ID] = idp
}

// GetAccessIdentityProvider retrieves an access identity provider by ID.
func (s *Store) GetAccessIdentityProvider(id string) (*models.AccessIdentityProvider, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idp, ok := s.accessIdentityProviders[id]
	return idp, ok
}

// GetAccessIdentityProviderByName retrieves an access identity provider by name.
func (s *Store) GetAccessIdentityProviderByName(name string) (*models.AccessIdentityProvider, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, idp := range s.accessIdentityProviders {
		if idp.Name == name {
			return idp, true
		}
	}
	return nil, false
}

// UpdateAccessIdentityProvider updates an access identity provider.
func (s *Store) UpdateAccessIdentityProvider(id string, update func(*models.AccessIdentityProvider)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	idp, ok := s.accessIdentityProviders[id]
	if !ok {
		return false
	}
	update(idp)
	idp.UpdatedAt = time.Now()
	return true
}

// DeleteAccessIdentityProvider deletes an access identity provider.
func (s *Store) DeleteAccessIdentityProvider(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.accessIdentityProviders[id]; !ok {
		return false
	}
	delete(s.accessIdentityProviders, id)
	return true
}

// ---- Gateway Rule Operations ----

// CreateGatewayRule creates a new gateway rule.
func (s *Store) CreateGatewayRule(rule *models.GatewayRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gatewayRules[rule.ID] = rule
}

// GetGatewayRule retrieves a gateway rule by ID.
func (s *Store) GetGatewayRule(id string) (*models.GatewayRule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rule, ok := s.gatewayRules[id]
	return rule, ok
}

// GetGatewayRuleByName retrieves a gateway rule by name.
func (s *Store) GetGatewayRuleByName(name string) (*models.GatewayRule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, rule := range s.gatewayRules {
		if rule.Name == name {
			return rule, true
		}
	}
	return nil, false
}

// ListGatewayRules returns all gateway rules.
func (s *Store) ListGatewayRules() []*models.GatewayRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rules := make([]*models.GatewayRule, 0, len(s.gatewayRules))
	for _, rule := range s.gatewayRules {
		rules = append(rules, rule)
	}
	return rules
}

// UpdateGatewayRule updates a gateway rule.
func (s *Store) UpdateGatewayRule(id string, update func(*models.GatewayRule)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	rule, ok := s.gatewayRules[id]
	if !ok {
		return false
	}
	update(rule)
	rule.UpdatedAt = time.Now()
	return true
}

// DeleteGatewayRule deletes a gateway rule.
func (s *Store) DeleteGatewayRule(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.gatewayRules[id]; !ok {
		return false
	}
	delete(s.gatewayRules, id)
	return true
}

// ---- Gateway List Operations ----

// CreateGatewayList creates a new gateway list.
func (s *Store) CreateGatewayList(list *models.GatewayList) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gatewayLists[list.ID] = list
}

// GetGatewayList retrieves a gateway list by ID.
func (s *Store) GetGatewayList(id string) (*models.GatewayList, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list, ok := s.gatewayLists[id]
	return list, ok
}

// GetGatewayListByName retrieves a gateway list by name.
func (s *Store) GetGatewayListByName(name string) (*models.GatewayList, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, list := range s.gatewayLists {
		if list.Name == name {
			return list, true
		}
	}
	return nil, false
}

// UpdateGatewayList updates a gateway list.
func (s *Store) UpdateGatewayList(id string, update func(*models.GatewayList)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, ok := s.gatewayLists[id]
	if !ok {
		return false
	}
	update(list)
	list.UpdatedAt = time.Now()
	return true
}

// DeleteGatewayList deletes a gateway list.
func (s *Store) DeleteGatewayList(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.gatewayLists[id]; !ok {
		return false
	}
	delete(s.gatewayLists, id)
	return true
}

// ---- Gateway Configuration Operations ----

// GetGatewayConfiguration retrieves the gateway configuration.
func (s *Store) GetGatewayConfiguration() *models.GatewayConfiguration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.gatewayConfiguration
}

// UpdateGatewayConfiguration updates the gateway configuration.
func (s *Store) UpdateGatewayConfiguration(config *models.GatewayConfiguration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gatewayConfiguration = config
}

// ---- Device Posture Rule Operations ----

// CreateDevicePostureRule creates a new device posture rule.
func (s *Store) CreateDevicePostureRule(rule *models.DevicePostureRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devicePostureRules[rule.ID] = rule
}

// GetDevicePostureRule retrieves a device posture rule by ID.
func (s *Store) GetDevicePostureRule(id string) (*models.DevicePostureRule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rule, ok := s.devicePostureRules[id]
	return rule, ok
}

// GetDevicePostureRuleByName retrieves a device posture rule by name.
func (s *Store) GetDevicePostureRuleByName(name string) (*models.DevicePostureRule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, rule := range s.devicePostureRules {
		if rule.Name == name {
			return rule, true
		}
	}
	return nil, false
}

// UpdateDevicePostureRule updates a device posture rule.
func (s *Store) UpdateDevicePostureRule(id string, update func(*models.DevicePostureRule)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	rule, ok := s.devicePostureRules[id]
	if !ok {
		return false
	}
	update(rule)
	rule.UpdatedAt = time.Now()
	return true
}

// DeleteDevicePostureRule deletes a device posture rule.
func (s *Store) DeleteDevicePostureRule(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devicePostureRules[id]; !ok {
		return false
	}
	delete(s.devicePostureRules, id)
	return true
}

// ---- Split Tunnel Operations ----

// GetSplitTunnelExclude returns split tunnel exclude entries.
func (s *Store) GetSplitTunnelExclude() []models.SplitTunnelEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.splitTunnelExclude
}

// UpdateSplitTunnelExclude updates split tunnel exclude entries.
func (s *Store) UpdateSplitTunnelExclude(entries []models.SplitTunnelEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.splitTunnelExclude = entries
}

// GetSplitTunnelInclude returns split tunnel include entries.
func (s *Store) GetSplitTunnelInclude() []models.SplitTunnelEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.splitTunnelInclude
}

// UpdateSplitTunnelInclude updates split tunnel include entries.
func (s *Store) UpdateSplitTunnelInclude(entries []models.SplitTunnelEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.splitTunnelInclude = entries
}

// ---- Fallback Domain Operations ----

// GetFallbackDomains returns fallback domain entries.
func (s *Store) GetFallbackDomains() []models.FallbackDomainEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fallbackDomains
}

// UpdateFallbackDomains updates fallback domain entries.
func (s *Store) UpdateFallbackDomains(entries []models.FallbackDomainEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fallbackDomains = entries
}

// ---- R2 Bucket Operations ----

// CreateR2Bucket creates a new R2 bucket.
func (s *Store) CreateR2Bucket(bucket *models.R2Bucket) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.r2Buckets[bucket.Name] = bucket
}

// GetR2Bucket retrieves an R2 bucket by name.
func (s *Store) GetR2Bucket(name string) (*models.R2Bucket, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	bucket, ok := s.r2Buckets[name]
	return bucket, ok
}

// ListR2Buckets returns all R2 buckets, optionally filtered by name prefix.
func (s *Store) ListR2Buckets(namePrefix string) []*models.R2Bucket {
	s.mu.RLock()
	defer s.mu.RUnlock()
	buckets := make([]*models.R2Bucket, 0, len(s.r2Buckets))
	for _, bucket := range s.r2Buckets {
		if namePrefix == "" || len(bucket.Name) >= len(namePrefix) && bucket.Name[:len(namePrefix)] == namePrefix {
			buckets = append(buckets, bucket)
		}
	}
	return buckets
}

// DeleteR2Bucket deletes an R2 bucket.
func (s *Store) DeleteR2Bucket(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.r2Buckets[name]; !ok {
		return false
	}
	delete(s.r2Buckets, name)
	delete(s.r2BucketLifecycle, name)
	return true
}

// GetR2BucketLifecycle retrieves lifecycle rules for an R2 bucket.
func (s *Store) GetR2BucketLifecycle(bucketName string) interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if lifecycle, ok := s.r2BucketLifecycle[bucketName]; ok {
		return lifecycle
	}
	return map[string]interface{}{"rules": []interface{}{}}
}

// UpdateR2BucketLifecycle updates lifecycle rules for an R2 bucket.
func (s *Store) UpdateR2BucketLifecycle(bucketName string, lifecycle interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.r2BucketLifecycle[bucketName] = lifecycle
}

// ---- Zone Ruleset Operations ----

// CreateZoneRuleset creates a new zone ruleset.
func (s *Store) CreateZoneRuleset(ruleset *models.ZoneRuleset) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.zoneRulesets[ruleset.ID] = ruleset
}

// GetZoneRuleset retrieves a zone ruleset by ID.
func (s *Store) GetZoneRuleset(id string) (*models.ZoneRuleset, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ruleset, ok := s.zoneRulesets[id]
	return ruleset, ok
}

// GetZoneRulesetByPhase retrieves a zone ruleset by zone ID and phase.
func (s *Store) GetZoneRulesetByPhase(zoneID, phase string) (*models.ZoneRuleset, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, rs := range s.zoneRulesets {
		if rs.ZoneID == zoneID && rs.Phase == phase {
			return rs, true
		}
	}
	return nil, false
}

// ListZoneRulesets returns all rulesets for a zone.
func (s *Store) ListZoneRulesets(zoneID string) []*models.ZoneRuleset {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rulesets := make([]*models.ZoneRuleset, 0)
	for _, rs := range s.zoneRulesets {
		if zoneID == "" || rs.ZoneID == zoneID {
			rulesets = append(rulesets, rs)
		}
	}
	return rulesets
}

// UpdateZoneRuleset updates a zone ruleset.
func (s *Store) UpdateZoneRuleset(id string, update func(*models.ZoneRuleset)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	ruleset, ok := s.zoneRulesets[id]
	if !ok {
		return false
	}
	update(ruleset)
	ruleset.UpdatedAt = time.Now()
	return true
}

// DeleteZoneRuleset deletes a zone ruleset.
func (s *Store) DeleteZoneRuleset(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.zoneRulesets[id]; !ok {
		return false
	}
	delete(s.zoneRulesets, id)
	return true
}

// generateUUID generates a random UUID-like string.
func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b[:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:])
}
