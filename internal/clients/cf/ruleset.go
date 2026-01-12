// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
)

// RulesetResult contains the result of a ruleset operation
type RulesetResult struct {
	ID          string
	Name        string
	Description string
	Kind        string
	Phase       string
	Version     string
	LastUpdated time.Time
	Rules       []cloudflare.RulesetRule
}

// GetEntrypointRuleset gets the entrypoint ruleset for a zone and phase
func (api *API) GetEntrypointRuleset(ctx context.Context, zoneID, phase string) (*RulesetResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	rc := cloudflare.ZoneIdentifier(zoneID)
	ruleset, err := api.CloudflareClient.GetEntrypointRuleset(ctx, rc, phase)
	if err != nil {
		return nil, fmt.Errorf("failed to get entrypoint ruleset: %w", err)
	}

	result := &RulesetResult{
		ID:          ruleset.ID,
		Name:        ruleset.Name,
		Description: ruleset.Description,
		Kind:        ruleset.Kind,
		Phase:       ruleset.Phase,
		Rules:       ruleset.Rules,
	}
	if ruleset.Version != nil {
		result.Version = *ruleset.Version
	}
	if ruleset.LastUpdated != nil {
		result.LastUpdated = *ruleset.LastUpdated
	}

	return result, nil
}

// UpdateEntrypointRuleset updates the entrypoint ruleset for a zone and phase
func (api *API) UpdateEntrypointRuleset(
	ctx context.Context, zoneID, phase, description string, rules []cloudflare.RulesetRule,
) (*RulesetResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	rc := cloudflare.ZoneIdentifier(zoneID)
	ruleset, err := api.CloudflareClient.UpdateEntrypointRuleset(ctx, rc, cloudflare.UpdateEntrypointRulesetParams{
		Phase:       phase,
		Description: description,
		Rules:       rules,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update entrypoint ruleset: %w", err)
	}

	result := &RulesetResult{
		ID:          ruleset.ID,
		Name:        ruleset.Name,
		Description: ruleset.Description,
		Kind:        ruleset.Kind,
		Phase:       ruleset.Phase,
		Rules:       ruleset.Rules,
	}
	if ruleset.Version != nil {
		result.Version = *ruleset.Version
	}
	if ruleset.LastUpdated != nil {
		result.LastUpdated = *ruleset.LastUpdated
	}

	return result, nil
}

// GetRuleset gets a ruleset by ID
func (api *API) GetRuleset(ctx context.Context, zoneID, rulesetID string) (*RulesetResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	rc := cloudflare.ZoneIdentifier(zoneID)
	ruleset, err := api.CloudflareClient.GetRuleset(ctx, rc, rulesetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ruleset: %w", err)
	}

	result := &RulesetResult{
		ID:          ruleset.ID,
		Name:        ruleset.Name,
		Description: ruleset.Description,
		Kind:        ruleset.Kind,
		Phase:       ruleset.Phase,
		Rules:       ruleset.Rules,
	}
	if ruleset.Version != nil {
		result.Version = *ruleset.Version
	}
	if ruleset.LastUpdated != nil {
		result.LastUpdated = *ruleset.LastUpdated
	}

	return result, nil
}

// ListRulesets lists all rulesets for a zone
func (api *API) ListRulesets(ctx context.Context, zoneID string) ([]RulesetResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	rc := cloudflare.ZoneIdentifier(zoneID)
	rulesets, err := api.CloudflareClient.ListRulesets(ctx, rc, cloudflare.ListRulesetsParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list rulesets: %w", err)
	}

	results := make([]RulesetResult, len(rulesets))
	for i, rs := range rulesets {
		results[i] = RulesetResult{
			ID:          rs.ID,
			Name:        rs.Name,
			Description: rs.Description,
			Kind:        rs.Kind,
			Phase:       rs.Phase,
			Rules:       rs.Rules,
		}
		if rs.Version != nil {
			results[i].Version = *rs.Version
		}
		if rs.LastUpdated != nil {
			results[i].LastUpdated = *rs.LastUpdated
		}
	}

	return results, nil
}

// DeleteRuleset deletes a ruleset.
// This method is idempotent - returns nil if the ruleset is already deleted.
func (api *API) DeleteRuleset(ctx context.Context, zoneID, rulesetID string) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	rc := cloudflare.ZoneIdentifier(zoneID)
	if err := api.CloudflareClient.DeleteRuleset(ctx, rc, rulesetID); err != nil {
		if IsNotFoundError(err) {
			api.Log.Info("Ruleset already deleted (not found)", "zoneId", zoneID, "rulesetId", rulesetID)
			return nil
		}
		return fmt.Errorf("failed to delete ruleset: %w", err)
	}

	api.Log.Info("Ruleset deleted", "zoneId", zoneID, "rulesetId", rulesetID)
	return nil
}
