// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/StringKe/cloudflare-operator/test/mockserver/internal/store"
	"github.com/StringKe/cloudflare-operator/test/mockserver/models"
)

// Handlers provides HTTP handlers for the mock API.
type Handlers struct {
	store *store.Store
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(s *store.Store) *Handlers {
	return &Handlers{store: s}
}

// ---- Account Handlers ----

// ListAccounts handles GET /accounts.
func (h *Handlers) ListAccounts(w http.ResponseWriter, r *http.Request) {
	name := GetQueryParam(r, "name")
	if name != "" {
		acc, ok := h.store.GetAccountByName(name)
		if !ok {
			Success(w, []*models.Account{})
			return
		}
		Success(w, []*models.Account{acc})
		return
	}
	// Return default account
	acc, _ := h.store.GetAccount("test-account-id")
	Success(w, []*models.Account{acc})
}

// GetAccount handles GET /accounts/{accountId}.
func (h *Handlers) GetAccount(w http.ResponseWriter, r *http.Request) {
	accountID := GetPathParam(r, "accountId")
	acc, ok := h.store.GetAccount(accountID)
	if !ok {
		NotFound(w, "account")
		return
	}
	Success(w, acc)
}

// ---- Zone Handlers ----

// ListZones handles GET /zones.
func (h *Handlers) ListZones(w http.ResponseWriter, r *http.Request) {
	name := GetQueryParam(r, "name")
	if name != "" {
		zone, ok := h.store.GetZoneByName(name)
		if !ok {
			Success(w, []*models.Zone{})
			return
		}
		Success(w, []*models.Zone{zone})
		return
	}
	zones := h.store.ListZones()
	Success(w, zones)
}

// GetZone handles GET /zones/{zoneId}.
func (h *Handlers) GetZone(w http.ResponseWriter, r *http.Request) {
	zoneID := GetPathParam(r, "zoneId")
	zone, ok := h.store.GetZone(zoneID)
	if !ok {
		NotFound(w, "zone")
		return
	}
	Success(w, zone)
}

// ---- Tunnel Handlers ----

// TunnelCreateRequest represents a tunnel creation request.
type TunnelCreateRequest struct {
	Name      string `json:"name"`
	Secret    string `json:"tunnel_secret"`
	ConfigSrc string `json:"config_src"`
}

// CreateTunnel handles POST /accounts/{accountId}/cfd_tunnel.
func (h *Handlers) CreateTunnel(w http.ResponseWriter, r *http.Request) {
	accountID := GetPathParam(r, "accountId")

	req, err := ReadJSON[TunnelCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	// Check if tunnel with name already exists
	if _, ok := h.store.GetTunnelByName(accountID, req.Name); ok {
		Conflict(w, "tunnel with this name already exists")
		return
	}

	tunnel := &models.Tunnel{
		ID:           GenerateID(),
		Name:         req.Name,
		AccountTag:   accountID,
		CreatedAt:    time.Now(),
		Status:       "inactive",
		RemoteConfig: req.ConfigSrc == "cloudflare",
		ConfigSrc:    req.ConfigSrc,
		TunnelSecret: req.Secret,
	}

	h.store.CreateTunnel(tunnel)
	Created(w, tunnel)
}

// ListTunnels handles GET /accounts/{accountId}/cfd_tunnel.
func (h *Handlers) ListTunnels(w http.ResponseWriter, r *http.Request) {
	accountID := GetPathParam(r, "accountId")
	name := GetQueryParam(r, "name")

	if name != "" {
		tunnel, ok := h.store.GetTunnelByName(accountID, name)
		if !ok {
			Success(w, []*models.Tunnel{})
			return
		}
		Success(w, []*models.Tunnel{tunnel})
		return
	}

	tunnels := h.store.ListTunnels(accountID)
	Success(w, tunnels)
}

// GetTunnel handles GET /accounts/{accountId}/cfd_tunnel/{tunnelId}.
func (h *Handlers) GetTunnel(w http.ResponseWriter, r *http.Request) {
	tunnelID := GetPathParam(r, "tunnelId")
	tunnel, ok := h.store.GetTunnel(tunnelID)
	if !ok {
		NotFound(w, "tunnel")
		return
	}
	Success(w, tunnel)
}

// DeleteTunnel handles DELETE /accounts/{accountId}/cfd_tunnel/{tunnelId}.
func (h *Handlers) DeleteTunnel(w http.ResponseWriter, r *http.Request) {
	tunnelID := GetPathParam(r, "tunnelId")
	if !h.store.DeleteTunnel(tunnelID) {
		NotFound(w, "tunnel")
		return
	}
	Success(w, struct{}{})
}

// CleanupTunnelConnections handles DELETE /accounts/{accountId}/cfd_tunnel/{tunnelId}/connections.
func (h *Handlers) CleanupTunnelConnections(w http.ResponseWriter, r *http.Request) {
	tunnelID := GetPathParam(r, "tunnelId")
	_, ok := h.store.GetTunnel(tunnelID)
	if !ok {
		NotFound(w, "tunnel")
		return
	}
	Success(w, struct{}{})
}

// GetTunnelToken handles GET /accounts/{accountId}/cfd_tunnel/{tunnelId}/token.
func (h *Handlers) GetTunnelToken(w http.ResponseWriter, r *http.Request) {
	tunnelID := GetPathParam(r, "tunnelId")
	accountID := GetPathParam(r, "accountId")

	tunnel, ok := h.store.GetTunnel(tunnelID)
	if !ok {
		NotFound(w, "tunnel")
		return
	}

	// Generate a token similar to real Cloudflare token
	tokenData := map[string]interface{}{
		"a": accountID,
		"t": tunnelID,
		"s": tunnel.TunnelSecret,
	}
	tokenJSON, _ := json.Marshal(tokenData)
	token := base64.StdEncoding.EncodeToString(tokenJSON)

	Success(w, token)
}

// ---- Tunnel Configuration Handlers ----

// GetTunnelConfiguration handles GET /accounts/{accountId}/cfd_tunnel/{tunnelId}/configurations.
func (h *Handlers) GetTunnelConfiguration(w http.ResponseWriter, r *http.Request) {
	tunnelID := GetPathParam(r, "tunnelId")
	config, ok := h.store.GetTunnelConfiguration(tunnelID)
	if !ok {
		NotFound(w, "tunnel configuration")
		return
	}
	Success(w, config)
}

// TunnelConfigurationUpdateRequest represents a tunnel configuration update request.
type TunnelConfigurationUpdateRequest struct {
	Config models.TunnelConfigurationData `json:"config"`
}

// UpdateTunnelConfiguration handles PUT /accounts/{accountId}/cfd_tunnel/{tunnelId}/configurations.
func (h *Handlers) UpdateTunnelConfiguration(w http.ResponseWriter, r *http.Request) {
	tunnelID := GetPathParam(r, "tunnelId")

	req, err := ReadJSON[TunnelConfigurationUpdateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	config, ok := h.store.UpdateTunnelConfiguration(tunnelID, req.Config)
	if !ok {
		NotFound(w, "tunnel")
		return
	}
	Success(w, config)
}

// ---- Virtual Network Handlers ----

// VirtualNetworkCreateRequest represents a virtual network creation request.
type VirtualNetworkCreateRequest struct {
	Name      string `json:"name"`
	Comment   string `json:"comment"`
	IsDefault bool   `json:"is_default"`
}

// CreateVirtualNetwork handles POST /accounts/{accountId}/teamnet/virtual_networks.
func (h *Handlers) CreateVirtualNetwork(w http.ResponseWriter, r *http.Request) {
	req, err := ReadJSON[VirtualNetworkCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	// Check for existing network with same name
	if _, ok := h.store.GetVirtualNetworkByName(req.Name); ok {
		Conflict(w, "virtual network with this name already exists")
		return
	}

	vnet := &models.VirtualNetwork{
		ID:               GenerateID(),
		Name:             req.Name,
		Comment:          req.Comment,
		IsDefaultNetwork: req.IsDefault,
		CreatedAt:        time.Now(),
	}

	h.store.CreateVirtualNetwork(vnet)
	Created(w, vnet)
}

// ListVirtualNetworks handles GET /accounts/{accountId}/teamnet/virtual_networks.
func (h *Handlers) ListVirtualNetworks(w http.ResponseWriter, r *http.Request) {
	id := GetQueryParam(r, "id")
	name := GetQueryParam(r, "name")

	if id != "" {
		vnet, ok := h.store.GetVirtualNetwork(id)
		if !ok {
			Success(w, []*models.VirtualNetwork{})
			return
		}
		Success(w, []*models.VirtualNetwork{vnet})
		return
	}

	if name != "" {
		vnet, ok := h.store.GetVirtualNetworkByName(name)
		if !ok {
			Success(w, []*models.VirtualNetwork{})
			return
		}
		Success(w, []*models.VirtualNetwork{vnet})
		return
	}

	vnets := h.store.ListVirtualNetworks()
	Success(w, vnets)
}

// GetVirtualNetwork handles GET /accounts/{accountId}/teamnet/virtual_networks/{vnetId}.
func (h *Handlers) GetVirtualNetwork(w http.ResponseWriter, r *http.Request) {
	vnetID := GetPathParam(r, "vnetId")
	vnet, ok := h.store.GetVirtualNetwork(vnetID)
	if !ok {
		NotFound(w, "virtual network")
		return
	}
	Success(w, vnet)
}

// VirtualNetworkUpdateRequest represents a virtual network update request.
type VirtualNetworkUpdateRequest struct {
	Name      string `json:"name"`
	Comment   string `json:"comment"`
	IsDefault *bool  `json:"is_default_network"`
}

// UpdateVirtualNetwork handles PATCH /accounts/{accountId}/teamnet/virtual_networks/{vnetId}.
func (h *Handlers) UpdateVirtualNetwork(w http.ResponseWriter, r *http.Request) {
	vnetID := GetPathParam(r, "vnetId")

	req, err := ReadJSON[VirtualNetworkUpdateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if !h.store.UpdateVirtualNetwork(vnetID, func(vnet *models.VirtualNetwork) {
		if req.Name != "" {
			vnet.Name = req.Name
		}
		if req.Comment != "" {
			vnet.Comment = req.Comment
		}
		if req.IsDefault != nil {
			vnet.IsDefaultNetwork = *req.IsDefault
		}
	}) {
		NotFound(w, "virtual network")
		return
	}

	vnet, _ := h.store.GetVirtualNetwork(vnetID)
	Success(w, vnet)
}

// DeleteVirtualNetwork handles DELETE /accounts/{accountId}/teamnet/virtual_networks/{vnetId}.
func (h *Handlers) DeleteVirtualNetwork(w http.ResponseWriter, r *http.Request) {
	vnetID := GetPathParam(r, "vnetId")
	if !h.store.DeleteVirtualNetwork(vnetID) {
		NotFound(w, "virtual network")
		return
	}
	Success(w, struct{}{})
}

// ---- Tunnel Route Handlers ----

// TunnelRouteCreateRequest represents a tunnel route creation request.
type TunnelRouteCreateRequest struct {
	Network          string `json:"network"`
	TunnelID         string `json:"tunnel_id"`
	VirtualNetworkID string `json:"virtual_network_id"`
	Comment          string `json:"comment"`
}

// CreateTunnelRoute handles POST /accounts/{accountId}/teamnet/routes.
func (h *Handlers) CreateTunnelRoute(w http.ResponseWriter, r *http.Request) {
	req, err := ReadJSON[TunnelRouteCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	// Get tunnel name
	tunnelName := ""
	if tunnel, ok := h.store.GetTunnel(req.TunnelID); ok {
		tunnelName = tunnel.Name
	}

	route := &models.TunnelRoute{
		Network:          req.Network,
		TunnelID:         req.TunnelID,
		TunnelName:       tunnelName,
		VirtualNetworkID: req.VirtualNetworkID,
		Comment:          req.Comment,
		CreatedAt:        time.Now(),
	}

	h.store.CreateTunnelRoute(route)
	Created(w, route)
}

// ListTunnelRoutes handles GET /accounts/{accountId}/teamnet/routes.
func (h *Handlers) ListTunnelRoutes(w http.ResponseWriter, r *http.Request) {
	tunnelID := GetQueryParam(r, "tunnel_id")
	vnetID := GetQueryParam(r, "virtual_network_id")

	routes := h.store.ListTunnelRoutes(tunnelID, vnetID)
	Success(w, routes)
}

// UpdateTunnelRoute handles PATCH /accounts/{accountId}/teamnet/routes/network/{network}.
func (h *Handlers) UpdateTunnelRoute(w http.ResponseWriter, r *http.Request) {
	network := GetPathParam(r, "network")
	// URL decode the network parameter
	decodedNetwork, _ := url.PathUnescape(network)

	req, err := ReadJSON[TunnelRouteCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	vnetID := GetQueryParam(r, "virtual_network_id")
	if vnetID == "" {
		vnetID = req.VirtualNetworkID
	}

	if !h.store.UpdateTunnelRoute(decodedNetwork, vnetID, func(route *models.TunnelRoute) {
		if req.TunnelID != "" {
			route.TunnelID = req.TunnelID
		}
		if req.Comment != "" {
			route.Comment = req.Comment
		}
	}) {
		NotFound(w, "tunnel route")
		return
	}

	route, _ := h.store.GetTunnelRoute(decodedNetwork, vnetID)
	Success(w, route)
}

// DeleteTunnelRoute handles DELETE /accounts/{accountId}/teamnet/routes/network/{network}.
func (h *Handlers) DeleteTunnelRoute(w http.ResponseWriter, r *http.Request) {
	network := GetPathParam(r, "network")
	decodedNetwork, _ := url.PathUnescape(network)
	vnetID := GetQueryParam(r, "virtual_network_id")

	if !h.store.DeleteTunnelRoute(decodedNetwork, vnetID) {
		NotFound(w, "tunnel route")
		return
	}
	Success(w, struct{}{})
}
