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

// AccessApplicationParams contains parameters for creating/updating an Access Application.
type AccessApplicationParams struct {
	Name                     string
	Domain                   string
	Type                     string // self_hosted, saas, ssh, vnc, app_launcher, warp, biso, bookmark, dash_sso
	SessionDuration          string
	AllowedIdps              []string
	AutoRedirectToIdentity   *bool
	EnableBindingCookie      *bool
	HttpOnlyCookieAttribute  *bool
	SameSiteCookieAttribute  string
	LogoURL                  string
	SkipInterstitial         *bool
	AppLauncherVisible       *bool
	ServiceAuth401Redirect   *bool
	CustomDenyMessage        string
	CustomDenyURL            string
	AllowAuthenticateViaWarp *bool
	Tags                     []string
}

// AccessApplicationResult contains the result of an Access Application operation.
type AccessApplicationResult struct {
	ID                     string
	AUD                    string
	Name                   string
	Domain                 string
	Type                   string
	SessionDuration        string
	AllowedIdps            []string
	AutoRedirectToIdentity bool
}

// CreateAccessApplication creates a new Access Application.
func (c *API) CreateAccessApplication(params AccessApplicationParams) (*AccessApplicationResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	createParams := cloudflare.CreateAccessApplicationParams{
		Name:                    params.Name,
		Domain:                  params.Domain,
		Type:                    cloudflare.AccessApplicationType(params.Type),
		SessionDuration:         params.SessionDuration,
		AllowedIdps:             params.AllowedIdps,
		AutoRedirectToIdentity:  params.AutoRedirectToIdentity,
		EnableBindingCookie:     params.EnableBindingCookie,
		HttpOnlyCookieAttribute: params.HttpOnlyCookieAttribute,
		SameSiteCookieAttribute: params.SameSiteCookieAttribute,
		LogoURL:                 params.LogoURL,
		SkipInterstitial:        params.SkipInterstitial,
		AppLauncherVisible:      params.AppLauncherVisible,
		ServiceAuth401Redirect:  params.ServiceAuth401Redirect,
		CustomDenyMessage:       params.CustomDenyMessage,
		CustomDenyURL:           params.CustomDenyURL,
	}

	if params.AllowAuthenticateViaWarp != nil {
		createParams.AllowAuthenticateViaWarp = params.AllowAuthenticateViaWarp
	}
	if len(params.Tags) > 0 {
		createParams.Tags = params.Tags
	}

	app, err := c.CloudflareClient.CreateAccessApplication(ctx, rc, createParams)
	if err != nil {
		c.Log.Error(err, "error creating access application", "name", params.Name)
		return nil, err
	}

	c.Log.Info("Access Application created", "id", app.ID, "name", app.Name)

	autoRedirect := false
	if app.AutoRedirectToIdentity != nil {
		autoRedirect = *app.AutoRedirectToIdentity
	}

	return &AccessApplicationResult{
		ID:                     app.ID,
		AUD:                    app.AUD,
		Name:                   app.Name,
		Domain:                 app.Domain,
		Type:                   string(app.Type),
		SessionDuration:        app.SessionDuration,
		AllowedIdps:            app.AllowedIdps,
		AutoRedirectToIdentity: autoRedirect,
	}, nil
}

// GetAccessApplication retrieves an Access Application by ID.
func (c *API) GetAccessApplication(applicationID string) (*AccessApplicationResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	app, err := c.CloudflareClient.GetAccessApplication(ctx, rc, applicationID)
	if err != nil {
		c.Log.Error(err, "error getting access application", "id", applicationID)
		return nil, err
	}

	autoRedirect := false
	if app.AutoRedirectToIdentity != nil {
		autoRedirect = *app.AutoRedirectToIdentity
	}

	return &AccessApplicationResult{
		ID:                     app.ID,
		AUD:                    app.AUD,
		Name:                   app.Name,
		Domain:                 app.Domain,
		Type:                   string(app.Type),
		SessionDuration:        app.SessionDuration,
		AllowedIdps:            app.AllowedIdps,
		AutoRedirectToIdentity: autoRedirect,
	}, nil
}

// UpdateAccessApplication updates an existing Access Application.
func (c *API) UpdateAccessApplication(applicationID string, params AccessApplicationParams) (*AccessApplicationResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	updateParams := cloudflare.UpdateAccessApplicationParams{
		ID:                      applicationID,
		Name:                    params.Name,
		Domain:                  params.Domain,
		Type:                    cloudflare.AccessApplicationType(params.Type),
		SessionDuration:         params.SessionDuration,
		AllowedIdps:             params.AllowedIdps,
		AutoRedirectToIdentity:  params.AutoRedirectToIdentity,
		EnableBindingCookie:     params.EnableBindingCookie,
		HttpOnlyCookieAttribute: params.HttpOnlyCookieAttribute,
		SameSiteCookieAttribute: params.SameSiteCookieAttribute,
		LogoURL:                 params.LogoURL,
		SkipInterstitial:        params.SkipInterstitial,
		AppLauncherVisible:      params.AppLauncherVisible,
		ServiceAuth401Redirect:  params.ServiceAuth401Redirect,
		CustomDenyMessage:       params.CustomDenyMessage,
		CustomDenyURL:           params.CustomDenyURL,
	}

	if params.AllowAuthenticateViaWarp != nil {
		updateParams.AllowAuthenticateViaWarp = params.AllowAuthenticateViaWarp
	}
	if len(params.Tags) > 0 {
		updateParams.Tags = params.Tags
	}

	app, err := c.CloudflareClient.UpdateAccessApplication(ctx, rc, updateParams)
	if err != nil {
		c.Log.Error(err, "error updating access application", "id", applicationID)
		return nil, err
	}

	c.Log.Info("Access Application updated", "id", app.ID, "name", app.Name)

	autoRedirect := false
	if app.AutoRedirectToIdentity != nil {
		autoRedirect = *app.AutoRedirectToIdentity
	}

	return &AccessApplicationResult{
		ID:                     app.ID,
		AUD:                    app.AUD,
		Name:                   app.Name,
		Domain:                 app.Domain,
		Type:                   string(app.Type),
		SessionDuration:        app.SessionDuration,
		AllowedIdps:            app.AllowedIdps,
		AutoRedirectToIdentity: autoRedirect,
	}, nil
}

// DeleteAccessApplication deletes an Access Application.
func (c *API) DeleteAccessApplication(applicationID string) error {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	err := c.CloudflareClient.DeleteAccessApplication(ctx, rc, applicationID)
	if err != nil {
		c.Log.Error(err, "error deleting access application", "id", applicationID)
		return err
	}

	c.Log.Info("Access Application deleted", "id", applicationID)
	return nil
}

// AccessGroupParams contains parameters for creating/updating an Access Group.
type AccessGroupParams struct {
	Name    string
	Include []interface{}
	Exclude []interface{}
	Require []interface{}
}

// AccessGroupResult contains the result of an Access Group operation.
type AccessGroupResult struct {
	ID   string
	Name string
}

// CreateAccessGroup creates a new Access Group.
func (c *API) CreateAccessGroup(params AccessGroupParams) (*AccessGroupResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	createParams := cloudflare.CreateAccessGroupParams{
		Name:    params.Name,
		Include: params.Include,
		Exclude: params.Exclude,
		Require: params.Require,
	}

	group, err := c.CloudflareClient.CreateAccessGroup(ctx, rc, createParams)
	if err != nil {
		c.Log.Error(err, "error creating access group", "name", params.Name)
		return nil, err
	}

	c.Log.Info("Access Group created", "id", group.ID, "name", group.Name)

	return &AccessGroupResult{
		ID:   group.ID,
		Name: group.Name,
	}, nil
}

// GetAccessGroup retrieves an Access Group by ID.
func (c *API) GetAccessGroup(groupID string) (*AccessGroupResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	group, err := c.CloudflareClient.GetAccessGroup(ctx, rc, groupID)
	if err != nil {
		c.Log.Error(err, "error getting access group", "id", groupID)
		return nil, err
	}

	return &AccessGroupResult{
		ID:   group.ID,
		Name: group.Name,
	}, nil
}

// UpdateAccessGroup updates an existing Access Group.
func (c *API) UpdateAccessGroup(groupID string, params AccessGroupParams) (*AccessGroupResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	updateParams := cloudflare.UpdateAccessGroupParams{
		ID:      groupID,
		Name:    params.Name,
		Include: params.Include,
		Exclude: params.Exclude,
		Require: params.Require,
	}

	group, err := c.CloudflareClient.UpdateAccessGroup(ctx, rc, updateParams)
	if err != nil {
		c.Log.Error(err, "error updating access group", "id", groupID)
		return nil, err
	}

	c.Log.Info("Access Group updated", "id", group.ID, "name", group.Name)

	return &AccessGroupResult{
		ID:   group.ID,
		Name: group.Name,
	}, nil
}

// DeleteAccessGroup deletes an Access Group.
func (c *API) DeleteAccessGroup(groupID string) error {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	err := c.CloudflareClient.DeleteAccessGroup(ctx, rc, groupID)
	if err != nil {
		c.Log.Error(err, "error deleting access group", "id", groupID)
		return err
	}

	c.Log.Info("Access Group deleted", "id", groupID)
	return nil
}

// AccessIdentityProviderParams contains parameters for an Access Identity Provider.
type AccessIdentityProviderParams struct {
	Name       string
	Type       string
	Config     cloudflare.AccessIdentityProviderConfiguration
	ScimConfig cloudflare.AccessIdentityProviderScimConfiguration
}

// AccessIdentityProviderResult contains the result of an Access Identity Provider operation.
type AccessIdentityProviderResult struct {
	ID   string
	Name string
	Type string
}

// CreateAccessIdentityProvider creates a new Access Identity Provider.
func (c *API) CreateAccessIdentityProvider(params AccessIdentityProviderParams) (*AccessIdentityProviderResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	createParams := cloudflare.CreateAccessIdentityProviderParams{
		Name:   params.Name,
		Type:   params.Type,
		Config: params.Config,
	}

	idp, err := c.CloudflareClient.CreateAccessIdentityProvider(ctx, rc, createParams)
	if err != nil {
		c.Log.Error(err, "error creating access identity provider", "name", params.Name)
		return nil, err
	}

	c.Log.Info("Access Identity Provider created", "id", idp.ID, "name", idp.Name)

	return &AccessIdentityProviderResult{
		ID:   idp.ID,
		Name: idp.Name,
		Type: idp.Type,
	}, nil
}

// GetAccessIdentityProvider retrieves an Access Identity Provider by ID.
func (c *API) GetAccessIdentityProvider(idpID string) (*AccessIdentityProviderResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	idp, err := c.CloudflareClient.GetAccessIdentityProvider(ctx, rc, idpID)
	if err != nil {
		c.Log.Error(err, "error getting access identity provider", "id", idpID)
		return nil, err
	}

	return &AccessIdentityProviderResult{
		ID:   idp.ID,
		Name: idp.Name,
		Type: idp.Type,
	}, nil
}

// UpdateAccessIdentityProvider updates an existing Access Identity Provider.
func (c *API) UpdateAccessIdentityProvider(idpID string, params AccessIdentityProviderParams) (*AccessIdentityProviderResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	updateParams := cloudflare.UpdateAccessIdentityProviderParams{
		ID:     idpID,
		Name:   params.Name,
		Type:   params.Type,
		Config: params.Config,
	}

	idp, err := c.CloudflareClient.UpdateAccessIdentityProvider(ctx, rc, updateParams)
	if err != nil {
		c.Log.Error(err, "error updating access identity provider", "id", idpID)
		return nil, err
	}

	c.Log.Info("Access Identity Provider updated", "id", idp.ID, "name", idp.Name)

	return &AccessIdentityProviderResult{
		ID:   idp.ID,
		Name: idp.Name,
		Type: idp.Type,
	}, nil
}

// DeleteAccessIdentityProvider deletes an Access Identity Provider.
func (c *API) DeleteAccessIdentityProvider(idpID string) error {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	_, err := c.CloudflareClient.DeleteAccessIdentityProvider(ctx, rc, idpID)
	if err != nil {
		c.Log.Error(err, "error deleting access identity provider", "id", idpID)
		return err
	}

	c.Log.Info("Access Identity Provider deleted", "id", idpID)
	return nil
}

// AccessServiceTokenResult contains the result of an Access Service Token operation.
type AccessServiceTokenResult struct {
	ID           string
	TokenID      string
	Name         string
	ClientID     string
	ClientSecret string
	AccountID    string
	ExpiresAt    string
}

// CreateAccessServiceToken creates a new Access Service Token.
func (c *API) CreateAccessServiceToken(name string, duration string) (*AccessServiceTokenResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	createParams := cloudflare.CreateAccessServiceTokenParams{
		Name:     name,
		Duration: duration,
	}

	token, err := c.CloudflareClient.CreateAccessServiceToken(ctx, rc, createParams)
	if err != nil {
		c.Log.Error(err, "error creating access service token", "name", name)
		return nil, err
	}

	c.Log.Info("Access Service Token created", "id", token.ID, "name", token.Name)

	expiresAt := ""
	if token.ExpiresAt != nil {
		expiresAt = token.ExpiresAt.String()
	}

	return &AccessServiceTokenResult{
		ID:           token.ID,
		TokenID:      token.ID,
		Name:         token.Name,
		ClientID:     token.ClientID,
		ClientSecret: token.ClientSecret,
		AccountID:    c.ValidAccountId,
		ExpiresAt:    expiresAt,
	}, nil
}

// UpdateAccessServiceToken updates an existing Access Service Token.
func (c *API) UpdateAccessServiceToken(tokenID string, name string, duration string) (*AccessServiceTokenResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	updateParams := cloudflare.UpdateAccessServiceTokenParams{
		UUID:     tokenID,
		Name:     name,
		Duration: duration,
	}

	token, err := c.CloudflareClient.UpdateAccessServiceToken(ctx, rc, updateParams)
	if err != nil {
		c.Log.Error(err, "error updating access service token", "id", tokenID)
		return nil, err
	}

	c.Log.Info("Access Service Token updated", "id", token.ID, "name", token.Name)

	expiresAt := ""
	if token.ExpiresAt != nil {
		expiresAt = token.ExpiresAt.String()
	}

	return &AccessServiceTokenResult{
		ID:           token.ID,
		TokenID:      token.ID,
		Name:         token.Name,
		ClientID:     token.ClientID,
		ClientSecret: "", // ClientSecret not returned on update
		AccountID:    c.ValidAccountId,
		ExpiresAt:    expiresAt,
	}, nil
}

// RefreshAccessServiceToken refreshes an Access Service Token, generating a new client secret.
func (c *API) RefreshAccessServiceToken(tokenID string) (*AccessServiceTokenResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	token, err := c.CloudflareClient.RefreshAccessServiceToken(ctx, rc, tokenID)
	if err != nil {
		c.Log.Error(err, "error refreshing access service token", "id", tokenID)
		return nil, err
	}

	c.Log.Info("Access Service Token refreshed", "id", token.ID, "name", token.Name)

	expiresAt := ""
	if token.ExpiresAt != nil {
		expiresAt = token.ExpiresAt.String()
	}

	return &AccessServiceTokenResult{
		ID:           token.ID,
		TokenID:      token.ID,
		Name:         token.Name,
		ClientID:     token.ClientID,
		ClientSecret: "", // ClientSecret returned separately
		AccountID:    c.ValidAccountId,
		ExpiresAt:    expiresAt,
	}, nil
}

// DeleteAccessServiceToken deletes an Access Service Token.
func (c *API) DeleteAccessServiceToken(tokenID string) error {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	_, err := c.CloudflareClient.DeleteAccessServiceToken(ctx, rc, tokenID)
	if err != nil {
		c.Log.Error(err, "error deleting access service token", "id", tokenID)
		return err
	}

	c.Log.Info("Access Service Token deleted", "id", tokenID)
	return nil
}

// DevicePostureRuleParams contains parameters for a Device Posture Rule.
type DevicePostureRuleParams struct {
	Name        string
	Type        string
	Description string
	Schedule    string
	Expiration  string
	Match       []map[string]interface{}
	Input       map[string]interface{}
}

// DevicePostureRuleResult contains the result of a Device Posture Rule operation.
type DevicePostureRuleResult struct {
	ID          string
	Name        string
	Type        string
	Description string
	AccountID   string
}

// CreateDevicePostureRule creates a new Device Posture Rule.
func (c *API) CreateDevicePostureRule(params DevicePostureRuleParams) (*DevicePostureRuleResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()

	// Convert match to DevicePostureRuleMatch
	var match []cloudflare.DevicePostureRuleMatch
	for _, m := range params.Match {
		platform, _ := m["platform"].(string)
		match = append(match, cloudflare.DevicePostureRuleMatch{
			Platform: platform,
		})
	}

	// Convert input to DevicePostureRuleInput
	input := cloudflare.DevicePostureRuleInput{}
	if id, ok := params.Input["id"].(string); ok {
		input.ID = id
	}

	rule := cloudflare.DevicePostureRule{
		Name:        params.Name,
		Type:        params.Type,
		Description: params.Description,
		Schedule:    params.Schedule,
		Expiration:  params.Expiration,
		Match:       match,
		Input:       input,
	}

	result, err := c.CloudflareClient.CreateDevicePostureRule(ctx, c.ValidAccountId, rule)
	if err != nil {
		c.Log.Error(err, "error creating device posture rule", "name", params.Name)
		return nil, err
	}

	c.Log.Info("Device Posture Rule created", "id", result.ID, "name", result.Name)

	return &DevicePostureRuleResult{
		ID:          result.ID,
		Name:        result.Name,
		Type:        result.Type,
		Description: result.Description,
		AccountID:   c.ValidAccountId,
	}, nil
}

// GetDevicePostureRule retrieves a Device Posture Rule by ID.
func (c *API) GetDevicePostureRule(ruleID string) (*DevicePostureRuleResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()

	rule, err := c.CloudflareClient.DevicePostureRule(ctx, c.ValidAccountId, ruleID)
	if err != nil {
		c.Log.Error(err, "error getting device posture rule", "id", ruleID)
		return nil, err
	}

	return &DevicePostureRuleResult{
		ID:          rule.ID,
		Name:        rule.Name,
		Type:        rule.Type,
		Description: rule.Description,
		AccountID:   c.ValidAccountId,
	}, nil
}

// UpdateDevicePostureRule updates an existing Device Posture Rule.
func (c *API) UpdateDevicePostureRule(ruleID string, params DevicePostureRuleParams) (*DevicePostureRuleResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()

	// Convert match to DevicePostureRuleMatch
	var match []cloudflare.DevicePostureRuleMatch
	for _, m := range params.Match {
		platform, _ := m["platform"].(string)
		match = append(match, cloudflare.DevicePostureRuleMatch{
			Platform: platform,
		})
	}

	// Convert input to DevicePostureRuleInput
	input := cloudflare.DevicePostureRuleInput{}
	if id, ok := params.Input["id"].(string); ok {
		input.ID = id
	}

	rule := cloudflare.DevicePostureRule{
		ID:          ruleID,
		Name:        params.Name,
		Type:        params.Type,
		Description: params.Description,
		Schedule:    params.Schedule,
		Expiration:  params.Expiration,
		Match:       match,
		Input:       input,
	}

	result, err := c.CloudflareClient.UpdateDevicePostureRule(ctx, c.ValidAccountId, rule)
	if err != nil {
		c.Log.Error(err, "error updating device posture rule", "id", ruleID)
		return nil, err
	}

	c.Log.Info("Device Posture Rule updated", "id", result.ID, "name", result.Name)

	return &DevicePostureRuleResult{
		ID:          result.ID,
		Name:        result.Name,
		Type:        result.Type,
		Description: result.Description,
		AccountID:   c.ValidAccountId,
	}, nil
}

// DeleteDevicePostureRule deletes a Device Posture Rule.
func (c *API) DeleteDevicePostureRule(ruleID string) error {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	ctx := context.Background()

	err := c.CloudflareClient.DeleteDevicePostureRule(ctx, c.ValidAccountId, ruleID)
	if err != nil {
		c.Log.Error(err, "error deleting device posture rule", "id", ruleID)
		return err
	}

	c.Log.Info("Device Posture Rule deleted", "id", ruleID)
	return nil
}

// ListAccessApplicationsByName finds an Access Application by name.
func (c *API) ListAccessApplicationsByName(name string) (*AccessApplicationResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	apps, _, err := c.CloudflareClient.ListAccessApplications(ctx, rc, cloudflare.ListAccessApplicationsParams{})
	if err != nil {
		c.Log.Error(err, "error listing access applications")
		return nil, err
	}

	for _, app := range apps {
		if app.Name == name {
			autoRedirect := false
			if app.AutoRedirectToIdentity != nil {
				autoRedirect = *app.AutoRedirectToIdentity
			}
			return &AccessApplicationResult{
				ID:                     app.ID,
				AUD:                    app.AUD,
				Name:                   app.Name,
				Domain:                 app.Domain,
				Type:                   string(app.Type),
				SessionDuration:        app.SessionDuration,
				AllowedIdps:            app.AllowedIdps,
				AutoRedirectToIdentity: autoRedirect,
			}, nil
		}
	}

	return nil, fmt.Errorf("access application not found: %s", name)
}
