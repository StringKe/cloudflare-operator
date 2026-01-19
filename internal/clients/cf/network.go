// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"context"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
)

// VirtualNetworkParams contains parameters for creating or updating a Virtual Network.
type VirtualNetworkParams struct {
	Name             string
	Comment          string
	IsDefaultNetwork bool
}

// VirtualNetworkResult contains the result of a Virtual Network operation.
type VirtualNetworkResult struct {
	ID               string
	Name             string
	Comment          string
	IsDefaultNetwork bool
	DeletedAt        *string
}

// TunnelRouteParams contains parameters for creating a Tunnel Route.
type TunnelRouteParams struct {
	Network          string // CIDR notation
	TunnelID         string
	VirtualNetworkID string
	Comment          string
}

// TunnelRouteResult contains the result of a Tunnel Route operation.
type TunnelRouteResult struct {
	Network          string
	TunnelID         string
	TunnelName       string
	VirtualNetworkID string
	Comment          string
}

// CreateVirtualNetwork creates a new Virtual Network in Cloudflare.
func (c *API) CreateVirtualNetwork(ctx context.Context, params VirtualNetworkParams) (*VirtualNetworkResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	createParams := cloudflare.TunnelVirtualNetworkCreateParams{
		Name:      params.Name,
		Comment:   params.Comment,
		IsDefault: params.IsDefaultNetwork,
	}

	vnet, err := c.CloudflareClient.CreateTunnelVirtualNetwork(ctx, rc, createParams)
	if err != nil {
		c.Log.Error(err, "error creating virtual network", "name", params.Name)
		return nil, err
	}

	c.Log.Info("Virtual Network created successfully", "id", vnet.ID, "name", vnet.Name)

	return &VirtualNetworkResult{
		ID:               vnet.ID,
		Name:             vnet.Name,
		Comment:          vnet.Comment,
		IsDefaultNetwork: vnet.IsDefaultNetwork,
	}, nil
}

// GetVirtualNetwork retrieves a Virtual Network by ID.
func (c *API) GetVirtualNetwork(ctx context.Context, virtualNetworkID string) (*VirtualNetworkResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	// List all virtual networks and find by ID
	params := cloudflare.TunnelVirtualNetworksListParams{
		ID: virtualNetworkID,
	}

	vnets, err := c.CloudflareClient.ListTunnelVirtualNetworks(ctx, rc, params)
	if err != nil {
		c.Log.Error(err, "error listing virtual networks", "id", virtualNetworkID)
		return nil, err
	}

	if len(vnets) == 0 {
		return nil, fmt.Errorf("virtual network not found: %s", virtualNetworkID)
	}

	vnet := vnets[0]
	var deletedAt *string
	if vnet.DeletedAt != nil {
		deletedStr := vnet.DeletedAt.String()
		deletedAt = &deletedStr
	}

	return &VirtualNetworkResult{
		ID:               vnet.ID,
		Name:             vnet.Name,
		Comment:          vnet.Comment,
		IsDefaultNetwork: vnet.IsDefaultNetwork,
		DeletedAt:        deletedAt,
	}, nil
}

// GetVirtualNetworkByName retrieves a Virtual Network by name.
func (c *API) GetVirtualNetworkByName(ctx context.Context, name string) (*VirtualNetworkResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	params := cloudflare.TunnelVirtualNetworksListParams{
		Name: name,
	}

	vnets, err := c.CloudflareClient.ListTunnelVirtualNetworks(ctx, rc, params)
	if err != nil {
		c.Log.Error(err, "error listing virtual networks by name", "name", name)
		return nil, err
	}

	if len(vnets) == 0 {
		return nil, fmt.Errorf("virtual network not found: %s", name)
	}

	vnet := vnets[0]
	return &VirtualNetworkResult{
		ID:               vnet.ID,
		Name:             vnet.Name,
		Comment:          vnet.Comment,
		IsDefaultNetwork: vnet.IsDefaultNetwork,
	}, nil
}

// UpdateVirtualNetwork updates an existing Virtual Network.
func (c *API) UpdateVirtualNetwork(ctx context.Context, virtualNetworkID string, params VirtualNetworkParams) (*VirtualNetworkResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	updateParams := cloudflare.TunnelVirtualNetworkUpdateParams{
		VnetID:           virtualNetworkID,
		Name:             params.Name,
		Comment:          params.Comment,
		IsDefaultNetwork: &params.IsDefaultNetwork,
	}

	vnet, err := c.CloudflareClient.UpdateTunnelVirtualNetwork(ctx, rc, updateParams)
	if err != nil {
		c.Log.Error(err, "error updating virtual network", "id", virtualNetworkID, "name", params.Name)
		return nil, err
	}

	c.Log.Info("Virtual Network updated successfully", "id", vnet.ID, "name", vnet.Name)

	return &VirtualNetworkResult{
		ID:               vnet.ID,
		Name:             vnet.Name,
		Comment:          vnet.Comment,
		IsDefaultNetwork: vnet.IsDefaultNetwork,
	}, nil
}

// DeleteVirtualNetwork deletes a Virtual Network.
// This method is idempotent - returns nil if the virtual network is already deleted.
func (c *API) DeleteVirtualNetwork(ctx context.Context, virtualNetworkID string) error {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	err := c.CloudflareClient.DeleteTunnelVirtualNetwork(ctx, rc, virtualNetworkID)
	if err != nil {
		if IsNotFoundError(err) {
			c.Log.Info("Virtual Network already deleted (not found)", "id", virtualNetworkID)
			return nil
		}
		c.Log.Error(err, "error deleting virtual network", "id", virtualNetworkID)
		return err
	}

	c.Log.Info("Virtual Network deleted successfully", "id", virtualNetworkID)
	return nil
}

// GetDefaultVirtualNetwork retrieves the default Virtual Network for the account.
// Every Cloudflare Zero Trust account has a default Virtual Network.
func (c *API) GetDefaultVirtualNetwork(ctx context.Context) (*VirtualNetworkResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	// List all virtual networks and find the default one
	isDefault := true
	params := cloudflare.TunnelVirtualNetworksListParams{
		IsDefault: &isDefault,
	}

	vnets, err := c.CloudflareClient.ListTunnelVirtualNetworks(ctx, rc, params)
	if err != nil {
		c.Log.Error(err, "error listing virtual networks for default")
		return nil, err
	}

	for _, vnet := range vnets {
		if vnet.IsDefaultNetwork {
			return &VirtualNetworkResult{
				ID:               vnet.ID,
				Name:             vnet.Name,
				Comment:          vnet.Comment,
				IsDefaultNetwork: true,
			}, nil
		}
	}

	return nil, fmt.Errorf("no default virtual network found for account %s", c.ValidAccountId)
}

// ListVirtualNetworks lists all Virtual Networks for the account.
func (c *API) ListVirtualNetworks(ctx context.Context) ([]VirtualNetworkResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	vnets, err := c.CloudflareClient.ListTunnelVirtualNetworks(ctx, rc, cloudflare.TunnelVirtualNetworksListParams{})
	if err != nil {
		c.Log.Error(err, "error listing virtual networks")
		return nil, err
	}

	results := make([]VirtualNetworkResult, 0, len(vnets))
	for _, vnet := range vnets {
		if vnet.DeletedAt != nil {
			continue // Skip deleted VNets
		}
		results = append(results, VirtualNetworkResult{
			ID:               vnet.ID,
			Name:             vnet.Name,
			Comment:          vnet.Comment,
			IsDefaultNetwork: vnet.IsDefaultNetwork,
		})
	}

	return results, nil
}

// CreateTunnelRoute creates a new Tunnel Route for private network access.
func (c *API) CreateTunnelRoute(ctx context.Context, params TunnelRouteParams) (*TunnelRouteResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	createParams := cloudflare.TunnelRoutesCreateParams{
		TunnelID:         params.TunnelID,
		Network:          params.Network,
		VirtualNetworkID: params.VirtualNetworkID,
		Comment:          params.Comment,
	}

	route, err := c.CloudflareClient.CreateTunnelRoute(ctx, rc, createParams)
	if err != nil {
		c.Log.Error(err, "error creating tunnel route", "network", params.Network, "tunnelId", params.TunnelID)
		return nil, err
	}

	c.Log.Info("Tunnel Route created successfully", "network", route.Network, "tunnelId", route.TunnelID)

	return &TunnelRouteResult{
		Network:          route.Network,
		TunnelID:         route.TunnelID,
		TunnelName:       route.TunnelName,
		VirtualNetworkID: route.VirtualNetworkID,
		Comment:          route.Comment,
	}, nil
}

// GetTunnelRoute retrieves a Tunnel Route by network CIDR and virtual network ID.
func (c *API) GetTunnelRoute(ctx context.Context, network, virtualNetworkID string) (*TunnelRouteResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	params := cloudflare.TunnelRoutesListParams{
		VirtualNetworkID: virtualNetworkID,
	}

	routes, err := c.CloudflareClient.ListTunnelRoutes(ctx, rc, params)
	if err != nil {
		c.Log.Error(err, "error listing tunnel routes", "virtualNetworkId", virtualNetworkID)
		return nil, err
	}

	for _, route := range routes {
		if route.Network == network {
			return &TunnelRouteResult{
				Network:          route.Network,
				TunnelID:         route.TunnelID,
				TunnelName:       route.TunnelName,
				VirtualNetworkID: route.VirtualNetworkID,
				Comment:          route.Comment,
			}, nil
		}
	}

	return nil, fmt.Errorf("tunnel route not found for network %s in virtual network %s", network, virtualNetworkID)
}

// GetTunnelRouteByNetwork retrieves a Tunnel Route by network CIDR across all Virtual Networks.
// This is useful when you don't know which VNet the route is in.
// Returns the first matching route found.
func (c *API) GetTunnelRouteByNetwork(ctx context.Context, network string) (*TunnelRouteResult, error) {
	routes, err := c.ListTunnelRoutesByNetwork(ctx, network)
	if err != nil {
		return nil, err
	}

	if len(routes) == 0 {
		return nil, fmt.Errorf("tunnel route not found for network %s", network)
	}

	return &routes[0], nil
}

// ListTunnelRoutesByNetwork lists all Tunnel Routes for a given network CIDR across all Virtual Networks.
// This searches all VNets and returns all routes matching the network CIDR.
func (c *API) ListTunnelRoutesByNetwork(ctx context.Context, network string) ([]TunnelRouteResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	// Use NetworkSubset to find exact match or containing routes
	// Empty VirtualNetworkID means search across all VNets
	params := cloudflare.TunnelRoutesListParams{}

	routes, err := c.CloudflareClient.ListTunnelRoutes(ctx, rc, params)
	if err != nil {
		c.Log.Error(err, "error listing tunnel routes for network search", "network", network)
		return nil, err
	}

	results := make([]TunnelRouteResult, 0)
	for _, route := range routes {
		if route.Network == network {
			results = append(results, TunnelRouteResult{
				Network:          route.Network,
				TunnelID:         route.TunnelID,
				TunnelName:       route.TunnelName,
				VirtualNetworkID: route.VirtualNetworkID,
				Comment:          route.Comment,
			})
		}
	}

	return results, nil
}

// UpdateTunnelRoute updates an existing Tunnel Route.
func (c *API) UpdateTunnelRoute(ctx context.Context, network string, params TunnelRouteParams) (*TunnelRouteResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	updateParams := cloudflare.TunnelRoutesUpdateParams{
		TunnelID:         params.TunnelID,
		Network:          params.Network,
		VirtualNetworkID: params.VirtualNetworkID,
		Comment:          params.Comment,
	}

	route, err := c.CloudflareClient.UpdateTunnelRoute(ctx, rc, updateParams)
	if err != nil {
		c.Log.Error(err, "error updating tunnel route", "network", network)
		return nil, err
	}

	c.Log.Info("Tunnel Route updated successfully", "network", route.Network)

	return &TunnelRouteResult{
		Network:          route.Network,
		TunnelID:         route.TunnelID,
		TunnelName:       route.TunnelName,
		VirtualNetworkID: route.VirtualNetworkID,
		Comment:          route.Comment,
	}, nil
}

// DeleteTunnelRoute deletes a Tunnel Route.
// This method is idempotent - returns nil if the route is already deleted.
func (c *API) DeleteTunnelRoute(ctx context.Context, network, virtualNetworkID string) error {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	params := cloudflare.TunnelRoutesDeleteParams{
		Network:          network,
		VirtualNetworkID: virtualNetworkID,
	}

	err := c.CloudflareClient.DeleteTunnelRoute(ctx, rc, params)
	if err != nil {
		if IsNotFoundError(err) {
			c.Log.Info("Tunnel Route already deleted (not found)", "network", network, "virtualNetworkId", virtualNetworkID)
			return nil
		}
		c.Log.Error(err, "error deleting tunnel route", "network", network, "virtualNetworkId", virtualNetworkID)
		return err
	}

	c.Log.Info("Tunnel Route deleted successfully", "network", network)
	return nil
}

// ListTunnelRoutesByTunnelID lists all Tunnel Routes associated with a specific Tunnel.
// This is used to clean up routes before deleting a tunnel.
func (c *API) ListTunnelRoutesByTunnelID(ctx context.Context, tunnelID string) ([]TunnelRouteResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	params := cloudflare.TunnelRoutesListParams{
		TunnelID: tunnelID,
	}

	routes, err := c.CloudflareClient.ListTunnelRoutes(ctx, rc, params)
	if err != nil {
		c.Log.Error(err, "error listing tunnel routes by tunnel ID", "tunnelId", tunnelID)
		return nil, err
	}

	results := make([]TunnelRouteResult, 0, len(routes))
	for _, route := range routes {
		results = append(results, TunnelRouteResult{
			Network:          route.Network,
			TunnelID:         route.TunnelID,
			TunnelName:       route.TunnelName,
			VirtualNetworkID: route.VirtualNetworkID,
			Comment:          route.Comment,
		})
	}

	return results, nil
}

// ListTunnelRoutesByVirtualNetworkID lists all Tunnel Routes associated with a specific Virtual Network.
// This is used to clean up routes before deleting a virtual network.
func (c *API) ListTunnelRoutesByVirtualNetworkID(ctx context.Context, virtualNetworkID string) ([]TunnelRouteResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	params := cloudflare.TunnelRoutesListParams{
		VirtualNetworkID: virtualNetworkID,
	}

	routes, err := c.CloudflareClient.ListTunnelRoutes(ctx, rc, params)
	if err != nil {
		c.Log.Error(err, "error listing tunnel routes by virtual network ID", "virtualNetworkId", virtualNetworkID)
		return nil, err
	}

	results := make([]TunnelRouteResult, 0, len(routes))
	for _, route := range routes {
		results = append(results, TunnelRouteResult{
			Network:          route.Network,
			TunnelID:         route.TunnelID,
			TunnelName:       route.TunnelName,
			VirtualNetworkID: route.VirtualNetworkID,
			Comment:          route.Comment,
		})
	}

	return results, nil
}

// DeleteTunnelRoutesByTunnelID deletes all routes associated with a tunnel.
// Returns the number of routes deleted and any error encountered.
//
//nolint:revive // cognitive complexity is acceptable for this cleanup function
func (c *API) DeleteTunnelRoutesByTunnelID(ctx context.Context, tunnelID string) (int, error) {
	routes, err := c.ListTunnelRoutesByTunnelID(ctx, tunnelID)
	if err != nil {
		return 0, err
	}

	deletedCount := 0
	for _, route := range routes {
		if err := c.DeleteTunnelRoute(ctx, route.Network, route.VirtualNetworkID); err != nil {
			if !IsNotFoundError(err) {
				c.Log.Error(err, "error deleting tunnel route during cleanup",
					"network", route.Network, "tunnelId", tunnelID)
				return deletedCount, err
			}
			// Route already deleted, continue
		}
		deletedCount++
	}

	if deletedCount > 0 {
		c.Log.Info("Deleted tunnel routes", "tunnelId", tunnelID, "count", deletedCount)
	}

	return deletedCount, nil
}

// DeleteTunnelRoutesByVirtualNetworkID deletes all routes associated with a virtual network.
// Returns the number of routes deleted and any error encountered.
//
//nolint:revive // cognitive complexity is acceptable for this cleanup function
func (c *API) DeleteTunnelRoutesByVirtualNetworkID(ctx context.Context, virtualNetworkID string) (int, error) {
	routes, err := c.ListTunnelRoutesByVirtualNetworkID(ctx, virtualNetworkID)
	if err != nil {
		return 0, err
	}

	deletedCount := 0
	for _, route := range routes {
		if err := c.DeleteTunnelRoute(ctx, route.Network, route.VirtualNetworkID); err != nil {
			if !IsNotFoundError(err) {
				c.Log.Error(err, "error deleting tunnel route during cleanup",
					"network", route.Network, "virtualNetworkId", virtualNetworkID)
				return deletedCount, err
			}
			// Route already deleted, continue
		}
		deletedCount++
	}

	if deletedCount > 0 {
		c.Log.Info("Deleted tunnel routes", "virtualNetworkId", virtualNetworkID, "count", deletedCount)
	}

	return deletedCount, nil
}
