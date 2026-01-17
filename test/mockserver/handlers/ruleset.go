// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package handlers

import (
	"net/http"
	"time"

	"github.com/StringKe/cloudflare-operator/test/mockserver/models"
)

// RulesetCreateRequest represents a ruleset creation request.
type RulesetCreateRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Kind        string         `json:"kind"`
	Phase       string         `json:"phase"`
	Rules       []RulesetRule  `json:"rules"`
}

// RulesetRule represents a single rule in a ruleset.
type RulesetRule struct {
	ID          string                 `json:"id,omitempty"`
	Action      string                 `json:"action"`
	Expression  string                 `json:"expression"`
	Description string                 `json:"description,omitempty"`
	Enabled     *bool                  `json:"enabled,omitempty"`
	ActionParameters map[string]interface{} `json:"action_parameters,omitempty"`
}

// CreateZoneRuleset handles POST /zones/{zoneId}/rulesets.
func (h *Handlers) CreateZoneRuleset(w http.ResponseWriter, r *http.Request) {
	zoneID := GetPathParam(r, "zoneId")

	req, err := ReadJSON[RulesetCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	ruleset := &models.ZoneRuleset{
		ID:          GenerateID(),
		ZoneID:      zoneID,
		Name:        req.Name,
		Description: req.Description,
		Kind:        req.Kind,
		Phase:       req.Phase,
		Rules:       convertToModelRules(req.Rules),
		Version:     "1",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	h.store.CreateZoneRuleset(ruleset)
	Created(w, ruleset)
}

// ListZoneRulesets handles GET /zones/{zoneId}/rulesets.
func (h *Handlers) ListZoneRulesets(w http.ResponseWriter, r *http.Request) {
	zoneID := GetPathParam(r, "zoneId")
	rulesets := h.store.ListZoneRulesets(zoneID)
	Success(w, rulesets)
}

// GetZoneRuleset handles GET /zones/{zoneId}/rulesets/{rulesetId}.
func (h *Handlers) GetZoneRuleset(w http.ResponseWriter, r *http.Request) {
	rulesetID := GetPathParam(r, "rulesetId")
	ruleset, ok := h.store.GetZoneRuleset(rulesetID)
	if !ok {
		NotFound(w, "ruleset")
		return
	}
	Success(w, ruleset)
}

// UpdateZoneRuleset handles PUT /zones/{zoneId}/rulesets/{rulesetId}.
func (h *Handlers) UpdateZoneRuleset(w http.ResponseWriter, r *http.Request) {
	rulesetID := GetPathParam(r, "rulesetId")

	req, err := ReadJSON[RulesetCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if !h.store.UpdateZoneRuleset(rulesetID, func(rs *models.ZoneRuleset) {
		rs.Name = req.Name
		rs.Description = req.Description
		rs.Rules = convertToModelRules(req.Rules)
	}) {
		NotFound(w, "ruleset")
		return
	}

	ruleset, _ := h.store.GetZoneRuleset(rulesetID)
	Success(w, ruleset)
}

// DeleteZoneRuleset handles DELETE /zones/{zoneId}/rulesets/{rulesetId}.
func (h *Handlers) DeleteZoneRuleset(w http.ResponseWriter, r *http.Request) {
	rulesetID := GetPathParam(r, "rulesetId")
	if !h.store.DeleteZoneRuleset(rulesetID) {
		NotFound(w, "ruleset")
		return
	}
	Success(w, struct{}{})
}

// GetZoneRulesetByPhase handles GET /zones/{zoneId}/rulesets/phases/{phase}/entrypoint.
func (h *Handlers) GetZoneRulesetByPhase(w http.ResponseWriter, r *http.Request) {
	zoneID := GetPathParam(r, "zoneId")
	phase := GetPathParam(r, "phase")

	ruleset, ok := h.store.GetZoneRulesetByPhase(zoneID, phase)
	if !ok {
		NotFound(w, "ruleset")
		return
	}
	Success(w, ruleset)
}

// UpdateZoneRulesetByPhase handles PUT /zones/{zoneId}/rulesets/phases/{phase}/entrypoint.
func (h *Handlers) UpdateZoneRulesetByPhase(w http.ResponseWriter, r *http.Request) {
	zoneID := GetPathParam(r, "zoneId")
	phase := GetPathParam(r, "phase")

	req, err := ReadJSON[RulesetCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	// Find or create ruleset for this phase
	ruleset, ok := h.store.GetZoneRulesetByPhase(zoneID, phase)
	if !ok {
		// Create new ruleset for this phase
		ruleset = &models.ZoneRuleset{
			ID:          GenerateID(),
			ZoneID:      zoneID,
			Name:        req.Name,
			Description: req.Description,
			Kind:        "zone",
			Phase:       phase,
			Rules:       convertToModelRules(req.Rules),
			Version:     "1",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		h.store.CreateZoneRuleset(ruleset)
	} else {
		// Update existing ruleset
		h.store.UpdateZoneRuleset(ruleset.ID, func(rs *models.ZoneRuleset) {
			rs.Name = req.Name
			rs.Description = req.Description
			rs.Rules = convertToModelRules(req.Rules)
		})
		ruleset, _ = h.store.GetZoneRuleset(ruleset.ID)
	}

	Success(w, ruleset)
}

// convertToModelRules converts request rules to model rules.
func convertToModelRules(rules []RulesetRule) []models.RulesetRule {
	result := make([]models.RulesetRule, len(rules))
	for i, r := range rules {
		enabled := true
		if r.Enabled != nil {
			enabled = *r.Enabled
		}
		result[i] = models.RulesetRule{
			ID:               r.ID,
			Action:           r.Action,
			Expression:       r.Expression,
			Description:      r.Description,
			Enabled:          enabled,
			ActionParameters: r.ActionParameters,
		}
		if result[i].ID == "" {
			result[i].ID = GenerateID()
		}
	}
	return result
}
