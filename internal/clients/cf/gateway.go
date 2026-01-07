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

package cf

import (
	"context"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
)

// GatewayRuleParams contains parameters for a Gateway Rule.
type GatewayRuleParams struct {
	Name          string
	Description   string
	Precedence    int
	Enabled       bool
	Action        string
	Filters       []cloudflare.TeamsFilterType
	Traffic       string
	Identity      string
	DevicePosture string
	RuleSettings  map[string]interface{}
}

// GatewayRuleResult contains the result of a Gateway Rule operation.
type GatewayRuleResult struct {
	ID          string
	Name        string
	Description string
	Precedence  int
	Enabled     bool
	Action      string
}

// CreateGatewayRule creates a new Gateway Rule.
func (c *API) CreateGatewayRule(params GatewayRuleParams) (*GatewayRuleResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()

	rule := cloudflare.TeamsRule{
		Name:          params.Name,
		Description:   params.Description,
		Precedence:    uint64(params.Precedence),
		Enabled:       params.Enabled,
		Action:        cloudflare.TeamsGatewayAction(params.Action),
		Filters:       params.Filters,
		Traffic:       params.Traffic,
		Identity:      params.Identity,
		DevicePosture: params.DevicePosture,
	}

	result, err := c.CloudflareClient.TeamsCreateRule(ctx, c.ValidAccountId, rule)
	if err != nil {
		c.Log.Error(err, "error creating gateway rule", "name", params.Name)
		return nil, err
	}

	c.Log.Info("Gateway Rule created", "id", result.ID, "name", result.Name)

	return &GatewayRuleResult{
		ID:          result.ID,
		Name:        result.Name,
		Description: result.Description,
		Precedence:  int(result.Precedence),
		Enabled:     result.Enabled,
		Action:      string(result.Action),
	}, nil
}

// GetGatewayRule retrieves a Gateway Rule by ID.
func (c *API) GetGatewayRule(ruleID string) (*GatewayRuleResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()

	rule, err := c.CloudflareClient.TeamsRule(ctx, c.ValidAccountId, ruleID)
	if err != nil {
		c.Log.Error(err, "error getting gateway rule", "id", ruleID)
		return nil, err
	}

	return &GatewayRuleResult{
		ID:          rule.ID,
		Name:        rule.Name,
		Description: rule.Description,
		Precedence:  int(rule.Precedence),
		Enabled:     rule.Enabled,
		Action:      string(rule.Action),
	}, nil
}

// UpdateGatewayRule updates an existing Gateway Rule.
func (c *API) UpdateGatewayRule(ruleID string, params GatewayRuleParams) (*GatewayRuleResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()

	rule := cloudflare.TeamsRule{
		ID:            ruleID,
		Name:          params.Name,
		Description:   params.Description,
		Precedence:    uint64(params.Precedence),
		Enabled:       params.Enabled,
		Action:        cloudflare.TeamsGatewayAction(params.Action),
		Filters:       params.Filters,
		Traffic:       params.Traffic,
		Identity:      params.Identity,
		DevicePosture: params.DevicePosture,
	}

	result, err := c.CloudflareClient.TeamsUpdateRule(ctx, c.ValidAccountId, ruleID, rule)
	if err != nil {
		c.Log.Error(err, "error updating gateway rule", "id", ruleID)
		return nil, err
	}

	c.Log.Info("Gateway Rule updated", "id", result.ID, "name", result.Name)

	return &GatewayRuleResult{
		ID:          result.ID,
		Name:        result.Name,
		Description: result.Description,
		Precedence:  int(result.Precedence),
		Enabled:     result.Enabled,
		Action:      string(result.Action),
	}, nil
}

// DeleteGatewayRule deletes a Gateway Rule.
func (c *API) DeleteGatewayRule(ruleID string) error {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	ctx := context.Background()

	err := c.CloudflareClient.TeamsDeleteRule(ctx, c.ValidAccountId, ruleID)
	if err != nil {
		c.Log.Error(err, "error deleting gateway rule", "id", ruleID)
		return err
	}

	c.Log.Info("Gateway Rule deleted", "id", ruleID)
	return nil
}

// GatewayListParams contains parameters for a Gateway List.
type GatewayListParams struct {
	Name        string
	Description string
	Type        string // SERIAL, URL, DOMAIN, EMAIL, IP
	Items       []string
}

// GatewayListItem represents an item in a Gateway List.
type GatewayListItem struct {
	Value       string
	Description string
}

// GatewayListResult contains the result of a Gateway List operation.
type GatewayListResult struct {
	ID          string
	Name        string
	Description string
	Type        string
	Count       int
	AccountID   string
}

// CreateGatewayList creates a new Gateway List.
func (c *API) CreateGatewayList(params GatewayListParams) (*GatewayListResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()

	// Convert items to TeamsListItems
	items := make([]cloudflare.TeamsListItem, len(params.Items))
	for i, item := range params.Items {
		items[i] = cloudflare.TeamsListItem{Value: item}
	}

	createParams := cloudflare.CreateTeamsListParams{
		Name:        params.Name,
		Description: params.Description,
		Type:        params.Type,
		Items:       items,
	}

	result, err := c.CloudflareClient.CreateTeamsList(ctx, cloudflare.AccountIdentifier(c.ValidAccountId), createParams)
	if err != nil {
		c.Log.Error(err, "error creating gateway list", "name", params.Name)
		return nil, err
	}

	c.Log.Info("Gateway List created", "id", result.ID, "name", result.Name)

	return &GatewayListResult{
		ID:          result.ID,
		Name:        result.Name,
		Description: result.Description,
		Type:        result.Type,
		Count:       int(result.Count),
		AccountID:   c.ValidAccountId,
	}, nil
}

// GetGatewayList retrieves a Gateway List by ID.
func (c *API) GetGatewayList(listID string) (*GatewayListResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()

	list, err := c.CloudflareClient.GetTeamsList(ctx, cloudflare.AccountIdentifier(c.ValidAccountId), listID)
	if err != nil {
		c.Log.Error(err, "error getting gateway list", "id", listID)
		return nil, err
	}

	return &GatewayListResult{
		ID:          list.ID,
		Name:        list.Name,
		Description: list.Description,
		Type:        list.Type,
		Count:       int(list.Count),
		AccountID:   c.ValidAccountId,
	}, nil
}

// UpdateGatewayList updates an existing Gateway List.
func (c *API) UpdateGatewayList(listID string, params GatewayListParams) (*GatewayListResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()

	updateParams := cloudflare.UpdateTeamsListParams{
		ID:          listID,
		Name:        params.Name,
		Description: params.Description,
	}

	result, err := c.CloudflareClient.UpdateTeamsList(ctx, cloudflare.AccountIdentifier(c.ValidAccountId), updateParams)
	if err != nil {
		c.Log.Error(err, "error updating gateway list", "id", listID)
		return nil, err
	}

	c.Log.Info("Gateway List updated", "id", result.ID, "name", result.Name)

	return &GatewayListResult{
		ID:          result.ID,
		Name:        result.Name,
		Description: result.Description,
		Type:        result.Type,
		Count:       int(result.Count),
		AccountID:   c.ValidAccountId,
	}, nil
}

// DeleteGatewayList deletes a Gateway List.
func (c *API) DeleteGatewayList(listID string) error {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	ctx := context.Background()

	err := c.CloudflareClient.DeleteTeamsList(ctx, cloudflare.AccountIdentifier(c.ValidAccountId), listID)
	if err != nil {
		c.Log.Error(err, "error deleting gateway list", "id", listID)
		return err
	}

	c.Log.Info("Gateway List deleted", "id", listID)
	return nil
}

// ListGatewayRulesByName finds a Gateway Rule by name.
func (c *API) ListGatewayRulesByName(name string) (*GatewayRuleResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()

	rules, err := c.CloudflareClient.TeamsRules(ctx, c.ValidAccountId)
	if err != nil {
		c.Log.Error(err, "error listing gateway rules")
		return nil, err
	}

	for _, rule := range rules {
		if rule.Name == name {
			return &GatewayRuleResult{
				ID:          rule.ID,
				Name:        rule.Name,
				Description: rule.Description,
				Precedence:  int(rule.Precedence),
				Enabled:     rule.Enabled,
				Action:      string(rule.Action),
			}, nil
		}
	}

	return nil, fmt.Errorf("gateway rule not found: %s", name)
}

// ListGatewayListsByName finds a Gateway List by name.
func (c *API) ListGatewayListsByName(name string) (*GatewayListResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()

	lists, _, err := c.CloudflareClient.ListTeamsLists(ctx, cloudflare.AccountIdentifier(c.ValidAccountId), cloudflare.ListTeamListsParams{})
	if err != nil {
		c.Log.Error(err, "error listing gateway lists")
		return nil, err
	}

	for _, list := range lists {
		if list.Name == name {
			return &GatewayListResult{
				ID:          list.ID,
				Name:        list.Name,
				Description: list.Description,
				Type:        list.Type,
				Count:       int(list.Count),
				AccountID:   c.ValidAccountId,
			}, nil
		}
	}

	return nil, fmt.Errorf("gateway list not found: %s", name)
}
