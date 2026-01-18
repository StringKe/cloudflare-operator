// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package mockserver

import (
	"net/http"

	"github.com/StringKe/cloudflare-operator/test/mockserver/handlers"
)

// apiPrefix is the Cloudflare API path prefix.
const apiPrefix = "/client/v4"

// registerRoutes registers all API routes.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Health check
	mux.HandleFunc("GET /health", s.handleHealth)

	// Admin endpoints for testing
	mux.HandleFunc("POST /admin/reset", s.handleReset)
	mux.HandleFunc("GET /admin/requests", s.handleGetRequests)

	// Create handlers
	h := handlers.NewHandlers(s.store)

	// ---- Account Routes ----
	mux.HandleFunc("GET "+apiPrefix+"/accounts", h.ListAccounts)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}", h.GetAccount)

	// ---- Zone Routes ----
	mux.HandleFunc("GET "+apiPrefix+"/zones", h.ListZones)
	mux.HandleFunc("GET "+apiPrefix+"/zones/{zoneId}", h.GetZone)

	// ---- Tunnel Routes ----
	mux.HandleFunc("POST "+apiPrefix+"/accounts/{accountId}/cfd_tunnel", h.CreateTunnel)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/cfd_tunnel", h.ListTunnels)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/cfd_tunnel/{tunnelId}", h.GetTunnel)
	mux.HandleFunc("DELETE "+apiPrefix+"/accounts/{accountId}/cfd_tunnel/{tunnelId}", h.DeleteTunnel)
	mux.HandleFunc("DELETE "+apiPrefix+"/accounts/{accountId}/cfd_tunnel/{tunnelId}/connections", h.CleanupTunnelConnections)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/cfd_tunnel/{tunnelId}/token", h.GetTunnelToken)

	// ---- Tunnel Configuration Routes ----
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/cfd_tunnel/{tunnelId}/configurations", h.GetTunnelConfiguration)
	mux.HandleFunc("PUT "+apiPrefix+"/accounts/{accountId}/cfd_tunnel/{tunnelId}/configurations", h.UpdateTunnelConfiguration)

	// ---- Virtual Network Routes ----
	mux.HandleFunc("POST "+apiPrefix+"/accounts/{accountId}/teamnet/virtual_networks", h.CreateVirtualNetwork)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/teamnet/virtual_networks", h.ListVirtualNetworks)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/teamnet/virtual_networks/{vnetId}", h.GetVirtualNetwork)
	mux.HandleFunc("PATCH "+apiPrefix+"/accounts/{accountId}/teamnet/virtual_networks/{vnetId}", h.UpdateVirtualNetwork)
	mux.HandleFunc("DELETE "+apiPrefix+"/accounts/{accountId}/teamnet/virtual_networks/{vnetId}", h.DeleteVirtualNetwork)

	// ---- Tunnel Route Routes ----
	// Note: cloudflare-go SDK uses /network/{network} path for CreateTunnelRoute
	mux.HandleFunc("POST "+apiPrefix+"/accounts/{accountId}/teamnet/routes/network/{network}", h.CreateTunnelRoute)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/teamnet/routes", h.ListTunnelRoutes)
	mux.HandleFunc("PATCH "+apiPrefix+"/accounts/{accountId}/teamnet/routes/network/{network}", h.UpdateTunnelRoute)
	mux.HandleFunc("DELETE "+apiPrefix+"/accounts/{accountId}/teamnet/routes/network/{network}", h.DeleteTunnelRoute)

	// ---- DNS Record Routes ----
	mux.HandleFunc("POST "+apiPrefix+"/zones/{zoneId}/dns_records", h.CreateDNSRecord)
	mux.HandleFunc("GET "+apiPrefix+"/zones/{zoneId}/dns_records", h.ListDNSRecords)
	mux.HandleFunc("GET "+apiPrefix+"/zones/{zoneId}/dns_records/{recordId}", h.GetDNSRecord)
	mux.HandleFunc("PUT "+apiPrefix+"/zones/{zoneId}/dns_records/{recordId}", h.UpdateDNSRecord)
	mux.HandleFunc("PATCH "+apiPrefix+"/zones/{zoneId}/dns_records/{recordId}", h.UpdateDNSRecord)
	mux.HandleFunc("DELETE "+apiPrefix+"/zones/{zoneId}/dns_records/{recordId}", h.DeleteDNSRecord)

	// ---- Access Application Routes ----
	mux.HandleFunc("POST "+apiPrefix+"/accounts/{accountId}/access/apps", h.CreateAccessApplication)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/access/apps", h.ListAccessApplications)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/access/apps/{appId}", h.GetAccessApplication)
	mux.HandleFunc("PUT "+apiPrefix+"/accounts/{accountId}/access/apps/{appId}", h.UpdateAccessApplication)
	mux.HandleFunc("DELETE "+apiPrefix+"/accounts/{accountId}/access/apps/{appId}", h.DeleteAccessApplication)

	// ---- Access Policy Routes ----
	mux.HandleFunc("POST "+apiPrefix+"/accounts/{accountId}/access/apps/{appId}/policies", h.CreateAccessPolicy)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/access/apps/{appId}/policies", h.ListAccessPolicies)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/access/apps/{appId}/policies/{policyId}", h.GetAccessPolicy)
	mux.HandleFunc("PUT "+apiPrefix+"/accounts/{accountId}/access/apps/{appId}/policies/{policyId}", h.UpdateAccessPolicy)
	mux.HandleFunc("DELETE "+apiPrefix+"/accounts/{accountId}/access/apps/{appId}/policies/{policyId}", h.DeleteAccessPolicy)

	// ---- Access Group Routes ----
	mux.HandleFunc("POST "+apiPrefix+"/accounts/{accountId}/access/groups", h.CreateAccessGroup)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/access/groups", h.ListAccessGroups)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/access/groups/{groupId}", h.GetAccessGroup)
	mux.HandleFunc("PUT "+apiPrefix+"/accounts/{accountId}/access/groups/{groupId}", h.UpdateAccessGroup)
	mux.HandleFunc("DELETE "+apiPrefix+"/accounts/{accountId}/access/groups/{groupId}", h.DeleteAccessGroup)

	// ---- Access Service Token Routes ----
	mux.HandleFunc("POST "+apiPrefix+"/accounts/{accountId}/access/service_tokens", h.CreateAccessServiceToken)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/access/service_tokens", h.ListAccessServiceTokens)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/access/service_tokens/{tokenId}", h.GetAccessServiceToken)
	mux.HandleFunc("PUT "+apiPrefix+"/accounts/{accountId}/access/service_tokens/{tokenId}", h.UpdateAccessServiceToken)
	mux.HandleFunc("POST "+apiPrefix+"/accounts/{accountId}/access/service_tokens/{tokenId}/refresh", h.RefreshAccessServiceToken)
	mux.HandleFunc("DELETE "+apiPrefix+"/accounts/{accountId}/access/service_tokens/{tokenId}", h.DeleteAccessServiceToken)

	// ---- Access Identity Provider Routes ----
	mux.HandleFunc("POST "+apiPrefix+"/accounts/{accountId}/access/identity_providers", h.CreateAccessIdentityProvider)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/access/identity_providers", h.ListAccessIdentityProviders)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/access/identity_providers/{idpId}", h.GetAccessIdentityProvider)
	mux.HandleFunc("PUT "+apiPrefix+"/accounts/{accountId}/access/identity_providers/{idpId}", h.UpdateAccessIdentityProvider)
	mux.HandleFunc("DELETE "+apiPrefix+"/accounts/{accountId}/access/identity_providers/{idpId}", h.DeleteAccessIdentityProvider)

	// ---- Gateway Rule Routes ----
	mux.HandleFunc("POST "+apiPrefix+"/accounts/{accountId}/gateway/rules", h.CreateGatewayRule)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/gateway/rules", h.ListGatewayRules)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/gateway/rules/{ruleId}", h.GetGatewayRule)
	mux.HandleFunc("PUT "+apiPrefix+"/accounts/{accountId}/gateway/rules/{ruleId}", h.UpdateGatewayRule)
	mux.HandleFunc("DELETE "+apiPrefix+"/accounts/{accountId}/gateway/rules/{ruleId}", h.DeleteGatewayRule)

	// ---- Gateway List Routes ----
	mux.HandleFunc("POST "+apiPrefix+"/accounts/{accountId}/gateway/lists", h.CreateGatewayList)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/gateway/lists", h.ListGatewayLists)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/gateway/lists/{listId}", h.GetGatewayList)
	mux.HandleFunc("PUT "+apiPrefix+"/accounts/{accountId}/gateway/lists/{listId}", h.UpdateGatewayList)
	mux.HandleFunc("DELETE "+apiPrefix+"/accounts/{accountId}/gateway/lists/{listId}", h.DeleteGatewayList)

	// ---- Gateway Configuration Routes ----
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/gateway/configuration", h.GetGatewayConfiguration)
	mux.HandleFunc("PUT "+apiPrefix+"/accounts/{accountId}/gateway/configuration", h.UpdateGatewayConfiguration)
	mux.HandleFunc("PATCH "+apiPrefix+"/accounts/{accountId}/gateway/configuration", h.UpdateGatewayConfiguration)

	// ---- Device Posture Rule Routes ----
	mux.HandleFunc("POST "+apiPrefix+"/accounts/{accountId}/devices/posture", h.CreateDevicePostureRule)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/devices/posture", h.ListDevicePostureRules)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/devices/posture/{ruleId}", h.GetDevicePostureRule)
	mux.HandleFunc("PUT "+apiPrefix+"/accounts/{accountId}/devices/posture/{ruleId}", h.UpdateDevicePostureRule)
	mux.HandleFunc("DELETE "+apiPrefix+"/accounts/{accountId}/devices/posture/{ruleId}", h.DeleteDevicePostureRule)

	// ---- Split Tunnel Routes ----
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/devices/policy/exclude", h.GetSplitTunnelExclude)
	mux.HandleFunc("PUT "+apiPrefix+"/accounts/{accountId}/devices/policy/exclude", h.UpdateSplitTunnelExclude)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/devices/policy/include", h.GetSplitTunnelInclude)
	mux.HandleFunc("PUT "+apiPrefix+"/accounts/{accountId}/devices/policy/include", h.UpdateSplitTunnelInclude)

	// ---- Fallback Domain Routes ----
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/devices/policy/fallback_domains", h.GetFallbackDomains)
	mux.HandleFunc("PUT "+apiPrefix+"/accounts/{accountId}/devices/policy/fallback_domains", h.UpdateFallbackDomains)

	// ---- R2 Bucket Routes ----
	mux.HandleFunc("PUT "+apiPrefix+"/accounts/{accountId}/r2/buckets", h.CreateR2Bucket)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/r2/buckets", h.ListR2Buckets)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/r2/buckets/{bucketName}", h.GetR2Bucket)
	mux.HandleFunc("DELETE "+apiPrefix+"/accounts/{accountId}/r2/buckets/{bucketName}", h.DeleteR2Bucket)
	mux.HandleFunc("GET "+apiPrefix+"/accounts/{accountId}/r2/buckets/{bucketName}/lifecycle", h.GetR2BucketLifecycle)
	mux.HandleFunc("PUT "+apiPrefix+"/accounts/{accountId}/r2/buckets/{bucketName}/lifecycle", h.UpdateR2BucketLifecycle)

	// ---- Zone Ruleset Routes ----
	mux.HandleFunc("POST "+apiPrefix+"/zones/{zoneId}/rulesets", h.CreateZoneRuleset)
	mux.HandleFunc("GET "+apiPrefix+"/zones/{zoneId}/rulesets", h.ListZoneRulesets)
	mux.HandleFunc("GET "+apiPrefix+"/zones/{zoneId}/rulesets/{rulesetId}", h.GetZoneRuleset)
	mux.HandleFunc("PUT "+apiPrefix+"/zones/{zoneId}/rulesets/{rulesetId}", h.UpdateZoneRuleset)
	mux.HandleFunc("DELETE "+apiPrefix+"/zones/{zoneId}/rulesets/{rulesetId}", h.DeleteZoneRuleset)
	mux.HandleFunc("GET "+apiPrefix+"/zones/{zoneId}/rulesets/phases/{phase}/entrypoint", h.GetZoneRulesetByPhase)
	mux.HandleFunc("PUT "+apiPrefix+"/zones/{zoneId}/rulesets/phases/{phase}/entrypoint", h.UpdateZoneRulesetByPhase)
}

// handleHealth handles health check requests.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// handleReset handles reset requests.
func (s *Server) handleReset(w http.ResponseWriter, _ *http.Request) {
	s.Reset()
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"success":true}`))
}

// handleGetRequests returns the request log.
func (s *Server) handleGetRequests(w http.ResponseWriter, _ *http.Request) {
	log := s.GetRequestLog()
	writeSuccess(w, log)
}
