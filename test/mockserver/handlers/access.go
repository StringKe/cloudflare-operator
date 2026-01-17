// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package handlers

import (
	"net/http"
	"time"

	"github.com/StringKe/cloudflare-operator/test/mockserver/models"
)

// ---- Access Application Handlers ----

// AccessApplicationCreateRequest represents an access application creation request.
type AccessApplicationCreateRequest struct {
	Name                    string                      `json:"name"`
	Domain                  string                      `json:"domain"`
	Type                    string                      `json:"type"`
	SessionDuration         string                      `json:"session_duration"`
	AutoRedirectToIdentity  bool                        `json:"auto_redirect_to_identity"`
	EnableBindingCookie     bool                        `json:"enable_binding_cookie"`
	CustomDenyMessage       string                      `json:"custom_deny_message"`
	CustomDenyURL           string                      `json:"custom_deny_url"`
	SameSiteCookieAttribute string                      `json:"same_site_cookie_attribute"`
	LogoURL                 string                      `json:"logo_url"`
	SkipInterstitial        bool                        `json:"skip_interstitial"`
	AppLauncherVisible      bool                        `json:"app_launcher_visible"`
	ServiceAuth401Redirect  bool                        `json:"service_auth_401_redirect"`
	AllowedIdps             []string                    `json:"allowed_idps"`
	SelfHostedDomains       []string                    `json:"self_hosted_domains"`
	Destinations            []models.AccessDestination  `json:"destinations"`
	Policies                []AccessPolicyCreateRequest `json:"policies"`
}

// CreateAccessApplication handles POST /accounts/{accountId}/access/apps.
func (h *Handlers) CreateAccessApplication(w http.ResponseWriter, r *http.Request) {
	req, err := ReadJSON[AccessApplicationCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	now := time.Now()
	app := &models.AccessApplication{
		ID:                      GenerateID(),
		Name:                    req.Name,
		Domain:                  req.Domain,
		Type:                    req.Type,
		SessionDuration:         req.SessionDuration,
		AutoRedirectToIdentity:  req.AutoRedirectToIdentity,
		EnableBindingCookie:     req.EnableBindingCookie,
		CustomDenyMessage:       req.CustomDenyMessage,
		CustomDenyURL:           req.CustomDenyURL,
		SameSiteCookieAttribute: req.SameSiteCookieAttribute,
		LogoURL:                 req.LogoURL,
		SkipInterstitial:        req.SkipInterstitial,
		AppLauncherVisible:      req.AppLauncherVisible,
		ServiceAuth401Redirect:  req.ServiceAuth401Redirect,
		AllowedIdps:             req.AllowedIdps,
		SelfHostedDomains:       req.SelfHostedDomains,
		Destinations:            req.Destinations,
		CreatedAt:               now,
		UpdatedAt:               now,
	}

	h.store.CreateAccessApplication(app)

	// Create policies if provided
	for _, policyReq := range req.Policies {
		policy := &models.AccessPolicy{
			ID:         GenerateID(),
			Name:       policyReq.Name,
			Precedence: policyReq.Precedence,
			Decision:   policyReq.Decision,
			Include:    policyReq.Include,
			Exclude:    policyReq.Exclude,
			Require:    policyReq.Require,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		h.store.CreateAccessPolicy(app.ID, policy)
		app.Policies = append(app.Policies, *policy)
	}

	Created(w, app)
}

// ListAccessApplications handles GET /accounts/{accountId}/access/apps.
func (h *Handlers) ListAccessApplications(w http.ResponseWriter, r *http.Request) {
	name := GetQueryParam(r, "name")
	if name != "" {
		app, ok := h.store.GetAccessApplicationByName(name)
		if !ok {
			Success(w, []*models.AccessApplication{})
			return
		}
		Success(w, []*models.AccessApplication{app})
		return
	}
	apps := h.store.ListAccessApplications()
	Success(w, apps)
}

// GetAccessApplication handles GET /accounts/{accountId}/access/apps/{appId}.
func (h *Handlers) GetAccessApplication(w http.ResponseWriter, r *http.Request) {
	appID := GetPathParam(r, "appId")
	app, ok := h.store.GetAccessApplication(appID)
	if !ok {
		NotFound(w, "access application")
		return
	}
	// Attach policies
	app.Policies = make([]models.AccessPolicy, 0)
	for _, p := range h.store.ListAccessPolicies(appID) {
		app.Policies = append(app.Policies, *p)
	}
	Success(w, app)
}

// UpdateAccessApplication handles PUT /accounts/{accountId}/access/apps/{appId}.
func (h *Handlers) UpdateAccessApplication(w http.ResponseWriter, r *http.Request) {
	appID := GetPathParam(r, "appId")

	req, err := ReadJSON[AccessApplicationCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if !h.store.UpdateAccessApplication(appID, func(app *models.AccessApplication) {
		if req.Name != "" {
			app.Name = req.Name
		}
		if req.Domain != "" {
			app.Domain = req.Domain
		}
		if req.Type != "" {
			app.Type = req.Type
		}
		if req.SessionDuration != "" {
			app.SessionDuration = req.SessionDuration
		}
		app.AutoRedirectToIdentity = req.AutoRedirectToIdentity
		app.EnableBindingCookie = req.EnableBindingCookie
		app.CustomDenyMessage = req.CustomDenyMessage
		app.CustomDenyURL = req.CustomDenyURL
		app.SameSiteCookieAttribute = req.SameSiteCookieAttribute
		app.LogoURL = req.LogoURL
		app.SkipInterstitial = req.SkipInterstitial
		app.AppLauncherVisible = req.AppLauncherVisible
		app.ServiceAuth401Redirect = req.ServiceAuth401Redirect
		if req.AllowedIdps != nil {
			app.AllowedIdps = req.AllowedIdps
		}
		if req.SelfHostedDomains != nil {
			app.SelfHostedDomains = req.SelfHostedDomains
		}
		if req.Destinations != nil {
			app.Destinations = req.Destinations
		}
	}) {
		NotFound(w, "access application")
		return
	}

	app, _ := h.store.GetAccessApplication(appID)
	Success(w, app)
}

// DeleteAccessApplication handles DELETE /accounts/{accountId}/access/apps/{appId}.
func (h *Handlers) DeleteAccessApplication(w http.ResponseWriter, r *http.Request) {
	appID := GetPathParam(r, "appId")
	if !h.store.DeleteAccessApplication(appID) {
		NotFound(w, "access application")
		return
	}
	Success(w, struct{}{})
}

// ---- Access Policy Handlers ----

// AccessPolicyCreateRequest represents an access policy creation request.
type AccessPolicyCreateRequest struct {
	Name       string              `json:"name"`
	Precedence int                 `json:"precedence"`
	Decision   string              `json:"decision"`
	Include    []models.AccessRule `json:"include"`
	Exclude    []models.AccessRule `json:"exclude"`
	Require    []models.AccessRule `json:"require"`
}

// CreateAccessPolicy handles POST /accounts/{accountId}/access/apps/{appId}/policies.
func (h *Handlers) CreateAccessPolicy(w http.ResponseWriter, r *http.Request) {
	appID := GetPathParam(r, "appId")

	if _, ok := h.store.GetAccessApplication(appID); !ok {
		NotFound(w, "access application")
		return
	}

	req, err := ReadJSON[AccessPolicyCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	now := time.Now()
	policy := &models.AccessPolicy{
		ID:         GenerateID(),
		Name:       req.Name,
		Precedence: req.Precedence,
		Decision:   req.Decision,
		Include:    req.Include,
		Exclude:    req.Exclude,
		Require:    req.Require,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	h.store.CreateAccessPolicy(appID, policy)
	Created(w, policy)
}

// ListAccessPolicies handles GET /accounts/{accountId}/access/apps/{appId}/policies.
func (h *Handlers) ListAccessPolicies(w http.ResponseWriter, r *http.Request) {
	appID := GetPathParam(r, "appId")
	policies := h.store.ListAccessPolicies(appID)
	Success(w, policies)
}

// GetAccessPolicy handles GET /accounts/{accountId}/access/apps/{appId}/policies/{policyId}.
func (h *Handlers) GetAccessPolicy(w http.ResponseWriter, r *http.Request) {
	appID := GetPathParam(r, "appId")
	policyID := GetPathParam(r, "policyId")
	policy, ok := h.store.GetAccessPolicy(appID, policyID)
	if !ok {
		NotFound(w, "access policy")
		return
	}
	Success(w, policy)
}

// UpdateAccessPolicy handles PUT /accounts/{accountId}/access/apps/{appId}/policies/{policyId}.
func (h *Handlers) UpdateAccessPolicy(w http.ResponseWriter, r *http.Request) {
	appID := GetPathParam(r, "appId")
	policyID := GetPathParam(r, "policyId")

	req, err := ReadJSON[AccessPolicyCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if !h.store.UpdateAccessPolicy(appID, policyID, func(policy *models.AccessPolicy) {
		if req.Name != "" {
			policy.Name = req.Name
		}
		if req.Precedence != 0 {
			policy.Precedence = req.Precedence
		}
		if req.Decision != "" {
			policy.Decision = req.Decision
		}
		if req.Include != nil {
			policy.Include = req.Include
		}
		if req.Exclude != nil {
			policy.Exclude = req.Exclude
		}
		if req.Require != nil {
			policy.Require = req.Require
		}
	}) {
		NotFound(w, "access policy")
		return
	}

	policy, _ := h.store.GetAccessPolicy(appID, policyID)
	Success(w, policy)
}

// DeleteAccessPolicy handles DELETE /accounts/{accountId}/access/apps/{appId}/policies/{policyId}.
func (h *Handlers) DeleteAccessPolicy(w http.ResponseWriter, r *http.Request) {
	appID := GetPathParam(r, "appId")
	policyID := GetPathParam(r, "policyId")
	if !h.store.DeleteAccessPolicy(appID, policyID) {
		NotFound(w, "access policy")
		return
	}
	Success(w, struct{}{})
}

// ---- Access Group Handlers ----

// AccessGroupCreateRequest represents an access group creation request.
type AccessGroupCreateRequest struct {
	Name    string              `json:"name"`
	Include []models.AccessRule `json:"include"`
	Exclude []models.AccessRule `json:"exclude"`
	Require []models.AccessRule `json:"require"`
}

// CreateAccessGroup handles POST /accounts/{accountId}/access/groups.
func (h *Handlers) CreateAccessGroup(w http.ResponseWriter, r *http.Request) {
	req, err := ReadJSON[AccessGroupCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	now := time.Now()
	group := &models.AccessGroup{
		ID:        GenerateID(),
		Name:      req.Name,
		Include:   req.Include,
		Exclude:   req.Exclude,
		Require:   req.Require,
		CreatedAt: now,
		UpdatedAt: now,
	}

	h.store.CreateAccessGroup(group)
	Created(w, group)
}

// ListAccessGroups handles GET /accounts/{accountId}/access/groups.
func (h *Handlers) ListAccessGroups(w http.ResponseWriter, r *http.Request) {
	name := GetQueryParam(r, "name")
	if name != "" {
		group, ok := h.store.GetAccessGroupByName(name)
		if !ok {
			Success(w, []*models.AccessGroup{})
			return
		}
		Success(w, []*models.AccessGroup{group})
		return
	}
	groups := h.store.ListAccessGroups()
	Success(w, groups)
}

// GetAccessGroup handles GET /accounts/{accountId}/access/groups/{groupId}.
func (h *Handlers) GetAccessGroup(w http.ResponseWriter, r *http.Request) {
	groupID := GetPathParam(r, "groupId")
	group, ok := h.store.GetAccessGroup(groupID)
	if !ok {
		NotFound(w, "access group")
		return
	}
	Success(w, group)
}

// UpdateAccessGroup handles PUT /accounts/{accountId}/access/groups/{groupId}.
func (h *Handlers) UpdateAccessGroup(w http.ResponseWriter, r *http.Request) {
	groupID := GetPathParam(r, "groupId")

	req, err := ReadJSON[AccessGroupCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if !h.store.UpdateAccessGroup(groupID, func(group *models.AccessGroup) {
		if req.Name != "" {
			group.Name = req.Name
		}
		if req.Include != nil {
			group.Include = req.Include
		}
		if req.Exclude != nil {
			group.Exclude = req.Exclude
		}
		if req.Require != nil {
			group.Require = req.Require
		}
	}) {
		NotFound(w, "access group")
		return
	}

	group, _ := h.store.GetAccessGroup(groupID)
	Success(w, group)
}

// DeleteAccessGroup handles DELETE /accounts/{accountId}/access/groups/{groupId}.
func (h *Handlers) DeleteAccessGroup(w http.ResponseWriter, r *http.Request) {
	groupID := GetPathParam(r, "groupId")
	if !h.store.DeleteAccessGroup(groupID) {
		NotFound(w, "access group")
		return
	}
	Success(w, struct{}{})
}

// ---- Access Service Token Handlers ----

// AccessServiceTokenCreateRequest represents a service token creation request.
type AccessServiceTokenCreateRequest struct {
	Name     string `json:"name"`
	Duration string `json:"duration"`
}

// CreateAccessServiceToken handles POST /accounts/{accountId}/access/service_tokens.
func (h *Handlers) CreateAccessServiceToken(w http.ResponseWriter, r *http.Request) {
	req, err := ReadJSON[AccessServiceTokenCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	now := time.Now()
	token := &models.AccessServiceToken{
		ID:           GenerateID(),
		Name:         req.Name,
		ClientID:     GenerateID() + ".access",
		ClientSecret: GenerateToken(32),
		Duration:     req.Duration,
		ExpiresAt:    now.AddDate(1, 0, 0), // Default 1 year
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	h.store.CreateAccessServiceToken(token)
	Created(w, token)
}

// ListAccessServiceTokens handles GET /accounts/{accountId}/access/service_tokens.
func (h *Handlers) ListAccessServiceTokens(w http.ResponseWriter, r *http.Request) {
	name := GetQueryParam(r, "name")
	if name != "" {
		token, ok := h.store.GetAccessServiceTokenByName(name)
		if !ok {
			Success(w, []*models.AccessServiceToken{})
			return
		}
		// Don't return client_secret in list
		token.ClientSecret = ""
		Success(w, []*models.AccessServiceToken{token})
		return
	}
	// Would return all tokens but omit secrets
	Success(w, []*models.AccessServiceToken{})
}

// GetAccessServiceToken handles GET /accounts/{accountId}/access/service_tokens/{tokenId}.
func (h *Handlers) GetAccessServiceToken(w http.ResponseWriter, r *http.Request) {
	tokenID := GetPathParam(r, "tokenId")
	token, ok := h.store.GetAccessServiceToken(tokenID)
	if !ok {
		NotFound(w, "access service token")
		return
	}
	// Don't return client_secret
	token.ClientSecret = ""
	Success(w, token)
}

// UpdateAccessServiceToken handles PUT /accounts/{accountId}/access/service_tokens/{tokenId}.
func (h *Handlers) UpdateAccessServiceToken(w http.ResponseWriter, r *http.Request) {
	tokenID := GetPathParam(r, "tokenId")

	req, err := ReadJSON[AccessServiceTokenCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if !h.store.UpdateAccessServiceToken(tokenID, func(token *models.AccessServiceToken) {
		if req.Name != "" {
			token.Name = req.Name
		}
		if req.Duration != "" {
			token.Duration = req.Duration
		}
	}) {
		NotFound(w, "access service token")
		return
	}

	token, _ := h.store.GetAccessServiceToken(tokenID)
	token.ClientSecret = ""
	Success(w, token)
}

// RefreshAccessServiceToken handles POST /accounts/{accountId}/access/service_tokens/{tokenId}/refresh.
func (h *Handlers) RefreshAccessServiceToken(w http.ResponseWriter, r *http.Request) {
	tokenID := GetPathParam(r, "tokenId")

	if !h.store.UpdateAccessServiceToken(tokenID, func(token *models.AccessServiceToken) {
		token.ExpiresAt = time.Now().AddDate(1, 0, 0)
		token.ClientSecret = GenerateToken(32)
	}) {
		NotFound(w, "access service token")
		return
	}

	token, _ := h.store.GetAccessServiceToken(tokenID)
	Success(w, token)
}

// DeleteAccessServiceToken handles DELETE /accounts/{accountId}/access/service_tokens/{tokenId}.
func (h *Handlers) DeleteAccessServiceToken(w http.ResponseWriter, r *http.Request) {
	tokenID := GetPathParam(r, "tokenId")
	if !h.store.DeleteAccessServiceToken(tokenID) {
		NotFound(w, "access service token")
		return
	}
	Success(w, struct{}{})
}

// ---- Access Identity Provider Handlers ----

// AccessIdentityProviderCreateRequest represents an IdP creation request.
type AccessIdentityProviderCreateRequest struct {
	Name   string                 `json:"name"`
	Type   string                 `json:"type"`
	Config map[string]interface{} `json:"config"`
}

// CreateAccessIdentityProvider handles POST /accounts/{accountId}/access/identity_providers.
func (h *Handlers) CreateAccessIdentityProvider(w http.ResponseWriter, r *http.Request) {
	req, err := ReadJSON[AccessIdentityProviderCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	now := time.Now()
	idp := &models.AccessIdentityProvider{
		ID:        GenerateID(),
		Name:      req.Name,
		Type:      req.Type,
		Config:    req.Config,
		CreatedAt: now,
		UpdatedAt: now,
	}

	h.store.CreateAccessIdentityProvider(idp)
	Created(w, idp)
}

// ListAccessIdentityProviders handles GET /accounts/{accountId}/access/identity_providers.
func (h *Handlers) ListAccessIdentityProviders(w http.ResponseWriter, r *http.Request) {
	name := GetQueryParam(r, "name")
	if name != "" {
		idp, ok := h.store.GetAccessIdentityProviderByName(name)
		if !ok {
			Success(w, []*models.AccessIdentityProvider{})
			return
		}
		Success(w, []*models.AccessIdentityProvider{idp})
		return
	}
	// Would return all IdPs
	Success(w, []*models.AccessIdentityProvider{})
}

// GetAccessIdentityProvider handles GET /accounts/{accountId}/access/identity_providers/{idpId}.
func (h *Handlers) GetAccessIdentityProvider(w http.ResponseWriter, r *http.Request) {
	idpID := GetPathParam(r, "idpId")
	idp, ok := h.store.GetAccessIdentityProvider(idpID)
	if !ok {
		NotFound(w, "access identity provider")
		return
	}
	Success(w, idp)
}

// UpdateAccessIdentityProvider handles PUT /accounts/{accountId}/access/identity_providers/{idpId}.
func (h *Handlers) UpdateAccessIdentityProvider(w http.ResponseWriter, r *http.Request) {
	idpID := GetPathParam(r, "idpId")

	req, err := ReadJSON[AccessIdentityProviderCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if !h.store.UpdateAccessIdentityProvider(idpID, func(idp *models.AccessIdentityProvider) {
		if req.Name != "" {
			idp.Name = req.Name
		}
		if req.Type != "" {
			idp.Type = req.Type
		}
		if req.Config != nil {
			idp.Config = req.Config
		}
	}) {
		NotFound(w, "access identity provider")
		return
	}

	idp, _ := h.store.GetAccessIdentityProvider(idpID)
	Success(w, idp)
}

// DeleteAccessIdentityProvider handles DELETE /accounts/{accountId}/access/identity_providers/{idpId}.
func (h *Handlers) DeleteAccessIdentityProvider(w http.ResponseWriter, r *http.Request) {
	idpID := GetPathParam(r, "idpId")
	if !h.store.DeleteAccessIdentityProvider(idpID) {
		NotFound(w, "access identity provider")
		return
	}
	Success(w, struct{}{})
}
