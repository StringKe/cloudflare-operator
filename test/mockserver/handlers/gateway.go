// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package handlers

import (
	"net/http"
	"time"

	"github.com/StringKe/cloudflare-operator/test/mockserver/models"
)

// ---- Gateway Rule Handlers ----

// GatewayRuleCreateRequest represents a gateway rule creation request.
type GatewayRuleCreateRequest struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Precedence   int                    `json:"precedence"`
	Enabled      bool                   `json:"enabled"`
	Action       string                 `json:"action"`
	Filters      []string               `json:"filters"`
	Traffic      string                 `json:"traffic"`
	Identity     string                 `json:"identity"`
	RuleSettings map[string]interface{} `json:"rule_settings"`
}

// CreateGatewayRule handles POST /accounts/{accountId}/gateway/rules.
func (h *Handlers) CreateGatewayRule(w http.ResponseWriter, r *http.Request) {
	req, err := ReadJSON[GatewayRuleCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	now := time.Now()
	rule := &models.GatewayRule{
		ID:           GenerateID(),
		Name:         req.Name,
		Description:  req.Description,
		Precedence:   req.Precedence,
		Enabled:      req.Enabled,
		Action:       req.Action,
		Filters:      req.Filters,
		Traffic:      req.Traffic,
		Identity:     req.Identity,
		RuleSettings: req.RuleSettings,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	h.store.CreateGatewayRule(rule)
	Created(w, rule)
}

// ListGatewayRules handles GET /accounts/{accountId}/gateway/rules.
func (h *Handlers) ListGatewayRules(w http.ResponseWriter, r *http.Request) {
	name := GetQueryParam(r, "name")
	if name != "" {
		rule, ok := h.store.GetGatewayRuleByName(name)
		if !ok {
			Success(w, []*models.GatewayRule{})
			return
		}
		Success(w, []*models.GatewayRule{rule})
		return
	}
	rules := h.store.ListGatewayRules()
	Success(w, rules)
}

// GetGatewayRule handles GET /accounts/{accountId}/gateway/rules/{ruleId}.
func (h *Handlers) GetGatewayRule(w http.ResponseWriter, r *http.Request) {
	ruleID := GetPathParam(r, "ruleId")
	rule, ok := h.store.GetGatewayRule(ruleID)
	if !ok {
		NotFound(w, "gateway rule")
		return
	}
	Success(w, rule)
}

// UpdateGatewayRule handles PUT /accounts/{accountId}/gateway/rules/{ruleId}.
func (h *Handlers) UpdateGatewayRule(w http.ResponseWriter, r *http.Request) {
	ruleID := GetPathParam(r, "ruleId")

	req, err := ReadJSON[GatewayRuleCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if !h.store.UpdateGatewayRule(ruleID, func(rule *models.GatewayRule) {
		if req.Name != "" {
			rule.Name = req.Name
		}
		if req.Description != "" {
			rule.Description = req.Description
		}
		if req.Precedence != 0 {
			rule.Precedence = req.Precedence
		}
		rule.Enabled = req.Enabled
		if req.Action != "" {
			rule.Action = req.Action
		}
		if req.Filters != nil {
			rule.Filters = req.Filters
		}
		if req.Traffic != "" {
			rule.Traffic = req.Traffic
		}
		if req.Identity != "" {
			rule.Identity = req.Identity
		}
		if req.RuleSettings != nil {
			rule.RuleSettings = req.RuleSettings
		}
	}) {
		NotFound(w, "gateway rule")
		return
	}

	rule, _ := h.store.GetGatewayRule(ruleID)
	Success(w, rule)
}

// DeleteGatewayRule handles DELETE /accounts/{accountId}/gateway/rules/{ruleId}.
func (h *Handlers) DeleteGatewayRule(w http.ResponseWriter, r *http.Request) {
	ruleID := GetPathParam(r, "ruleId")
	if !h.store.DeleteGatewayRule(ruleID) {
		NotFound(w, "gateway rule")
		return
	}
	Success(w, struct{}{})
}

// ---- Gateway List Handlers ----

// GatewayListCreateRequest represents a gateway list creation request.
type GatewayListCreateRequest struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Type        string                   `json:"type"`
	Items       []models.GatewayListItem `json:"items"`
}

// CreateGatewayList handles POST /accounts/{accountId}/gateway/lists.
func (h *Handlers) CreateGatewayList(w http.ResponseWriter, r *http.Request) {
	req, err := ReadJSON[GatewayListCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	now := time.Now()
	list := &models.GatewayList{
		ID:          GenerateID(),
		Name:        req.Name,
		Description: req.Description,
		Type:        req.Type,
		Items:       req.Items,
		Count:       len(req.Items),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	h.store.CreateGatewayList(list)
	Created(w, list)
}

// ListGatewayLists handles GET /accounts/{accountId}/gateway/lists.
func (h *Handlers) ListGatewayLists(w http.ResponseWriter, r *http.Request) {
	name := GetQueryParam(r, "name")
	if name != "" {
		list, ok := h.store.GetGatewayListByName(name)
		if !ok {
			Success(w, []*models.GatewayList{})
			return
		}
		Success(w, []*models.GatewayList{list})
		return
	}
	// Would return all lists
	Success(w, []*models.GatewayList{})
}

// GetGatewayList handles GET /accounts/{accountId}/gateway/lists/{listId}.
func (h *Handlers) GetGatewayList(w http.ResponseWriter, r *http.Request) {
	listID := GetPathParam(r, "listId")
	list, ok := h.store.GetGatewayList(listID)
	if !ok {
		NotFound(w, "gateway list")
		return
	}
	Success(w, list)
}

// UpdateGatewayList handles PUT /accounts/{accountId}/gateway/lists/{listId}.
func (h *Handlers) UpdateGatewayList(w http.ResponseWriter, r *http.Request) {
	listID := GetPathParam(r, "listId")

	req, err := ReadJSON[GatewayListCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if !h.store.UpdateGatewayList(listID, func(list *models.GatewayList) {
		if req.Name != "" {
			list.Name = req.Name
		}
		if req.Description != "" {
			list.Description = req.Description
		}
		if req.Type != "" {
			list.Type = req.Type
		}
		if req.Items != nil {
			list.Items = req.Items
			list.Count = len(req.Items)
		}
	}) {
		NotFound(w, "gateway list")
		return
	}

	list, _ := h.store.GetGatewayList(listID)
	Success(w, list)
}

// DeleteGatewayList handles DELETE /accounts/{accountId}/gateway/lists/{listId}.
func (h *Handlers) DeleteGatewayList(w http.ResponseWriter, r *http.Request) {
	listID := GetPathParam(r, "listId")
	if !h.store.DeleteGatewayList(listID) {
		NotFound(w, "gateway list")
		return
	}
	Success(w, struct{}{})
}

// ---- Gateway Configuration Handlers ----

// GetGatewayConfiguration handles GET /accounts/{accountId}/gateway/configuration.
func (h *Handlers) GetGatewayConfiguration(w http.ResponseWriter, r *http.Request) {
	config := h.store.GetGatewayConfiguration()
	Success(w, config)
}

// UpdateGatewayConfiguration handles PUT/PATCH /accounts/{accountId}/gateway/configuration.
func (h *Handlers) UpdateGatewayConfiguration(w http.ResponseWriter, r *http.Request) {
	req, err := ReadJSON[models.GatewayConfiguration](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	h.store.UpdateGatewayConfiguration(req)
	Success(w, req)
}

// ---- Device Posture Rule Handlers ----

// DevicePostureRuleCreateRequest represents a device posture rule creation request.
type DevicePostureRuleCreateRequest struct {
	Name        string                      `json:"name"`
	Description string                      `json:"description"`
	Type        string                      `json:"type"`
	Schedule    string                      `json:"schedule"`
	Expiration  string                      `json:"expiration"`
	Match       []models.DevicePostureMatch `json:"match"`
	Input       map[string]interface{}      `json:"input"`
}

// CreateDevicePostureRule handles POST /accounts/{accountId}/devices/posture.
func (h *Handlers) CreateDevicePostureRule(w http.ResponseWriter, r *http.Request) {
	req, err := ReadJSON[DevicePostureRuleCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	now := time.Now()
	rule := &models.DevicePostureRule{
		ID:          GenerateID(),
		Name:        req.Name,
		Description: req.Description,
		Type:        req.Type,
		Schedule:    req.Schedule,
		Expiration:  req.Expiration,
		Match:       req.Match,
		Input:       req.Input,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	h.store.CreateDevicePostureRule(rule)
	Created(w, rule)
}

// ListDevicePostureRules handles GET /accounts/{accountId}/devices/posture.
func (h *Handlers) ListDevicePostureRules(w http.ResponseWriter, r *http.Request) {
	name := GetQueryParam(r, "name")
	if name != "" {
		rule, ok := h.store.GetDevicePostureRuleByName(name)
		if !ok {
			Success(w, []*models.DevicePostureRule{})
			return
		}
		Success(w, []*models.DevicePostureRule{rule})
		return
	}
	// Would return all rules
	Success(w, []*models.DevicePostureRule{})
}

// GetDevicePostureRule handles GET /accounts/{accountId}/devices/posture/{ruleId}.
func (h *Handlers) GetDevicePostureRule(w http.ResponseWriter, r *http.Request) {
	ruleID := GetPathParam(r, "ruleId")
	rule, ok := h.store.GetDevicePostureRule(ruleID)
	if !ok {
		NotFound(w, "device posture rule")
		return
	}
	Success(w, rule)
}

// UpdateDevicePostureRule handles PUT /accounts/{accountId}/devices/posture/{ruleId}.
func (h *Handlers) UpdateDevicePostureRule(w http.ResponseWriter, r *http.Request) {
	ruleID := GetPathParam(r, "ruleId")

	req, err := ReadJSON[DevicePostureRuleCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if !h.store.UpdateDevicePostureRule(ruleID, func(rule *models.DevicePostureRule) {
		if req.Name != "" {
			rule.Name = req.Name
		}
		if req.Description != "" {
			rule.Description = req.Description
		}
		if req.Type != "" {
			rule.Type = req.Type
		}
		if req.Schedule != "" {
			rule.Schedule = req.Schedule
		}
		if req.Expiration != "" {
			rule.Expiration = req.Expiration
		}
		if req.Match != nil {
			rule.Match = req.Match
		}
		if req.Input != nil {
			rule.Input = req.Input
		}
	}) {
		NotFound(w, "device posture rule")
		return
	}

	rule, _ := h.store.GetDevicePostureRule(ruleID)
	Success(w, rule)
}

// DeleteDevicePostureRule handles DELETE /accounts/{accountId}/devices/posture/{ruleId}.
func (h *Handlers) DeleteDevicePostureRule(w http.ResponseWriter, r *http.Request) {
	ruleID := GetPathParam(r, "ruleId")
	if !h.store.DeleteDevicePostureRule(ruleID) {
		NotFound(w, "device posture rule")
		return
	}
	Success(w, struct{}{})
}

// ---- Split Tunnel Handlers ----

// GetSplitTunnelExclude handles GET /accounts/{accountId}/devices/policy/exclude.
func (h *Handlers) GetSplitTunnelExclude(w http.ResponseWriter, r *http.Request) {
	entries := h.store.GetSplitTunnelExclude()
	Success(w, entries)
}

// UpdateSplitTunnelExclude handles PUT /accounts/{accountId}/devices/policy/exclude.
func (h *Handlers) UpdateSplitTunnelExclude(w http.ResponseWriter, r *http.Request) {
	var entries []models.SplitTunnelEntry
	req, err := ReadJSON[[]models.SplitTunnelEntry](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}
	entries = *req
	h.store.UpdateSplitTunnelExclude(entries)
	Success(w, entries)
}

// GetSplitTunnelInclude handles GET /accounts/{accountId}/devices/policy/include.
func (h *Handlers) GetSplitTunnelInclude(w http.ResponseWriter, r *http.Request) {
	entries := h.store.GetSplitTunnelInclude()
	Success(w, entries)
}

// UpdateSplitTunnelInclude handles PUT /accounts/{accountId}/devices/policy/include.
func (h *Handlers) UpdateSplitTunnelInclude(w http.ResponseWriter, r *http.Request) {
	var entries []models.SplitTunnelEntry
	req, err := ReadJSON[[]models.SplitTunnelEntry](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}
	entries = *req
	h.store.UpdateSplitTunnelInclude(entries)
	Success(w, entries)
}

// ---- Fallback Domain Handlers ----

// GetFallbackDomains handles GET /accounts/{accountId}/devices/policy/fallback_domains.
func (h *Handlers) GetFallbackDomains(w http.ResponseWriter, r *http.Request) {
	entries := h.store.GetFallbackDomains()
	Success(w, entries)
}

// UpdateFallbackDomains handles PUT /accounts/{accountId}/devices/policy/fallback_domains.
func (h *Handlers) UpdateFallbackDomains(w http.ResponseWriter, r *http.Request) {
	var entries []models.FallbackDomainEntry
	req, err := ReadJSON[[]models.FallbackDomainEntry](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}
	entries = *req
	h.store.UpdateFallbackDomains(entries)
	Success(w, entries)
}
