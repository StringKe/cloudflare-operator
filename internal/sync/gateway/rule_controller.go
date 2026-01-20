// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package gateway provides Sync Controllers for managing Cloudflare Gateway resources.
package gateway

//nolint:dupl // Similar patterns across resource types are intentional

import (
	"context"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	gatewaysvc "github.com/StringKe/cloudflare-operator/internal/service/gateway"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

const (
	// FinalizerName is the finalizer for Gateway Rule SyncState resources.
	FinalizerName = "gatewayrule.sync.cloudflare-operator.io/finalizer"
)

// RuleController is the Sync Controller for Gateway Rule Configuration.
type RuleController struct {
	*common.BaseSyncController
}

// NewRuleController creates a new GatewayRuleSyncController
func NewRuleController(c client.Client) *RuleController {
	return &RuleController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for Gateway rule.
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *RuleController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "GatewayRuleSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process GatewayRule type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceGatewayRule {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing GatewayRule SyncState",
		"cloudflareId", syncState.Spec.CloudflareID,
		"sources", len(syncState.Spec.Sources))

	// Handle deletion - this is the SINGLE point for Cloudflare API delete calls
	if !syncState.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, syncState)
	}

	// Check if there are any sources - if none, delete from Cloudflare
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, deleting from Cloudflare")
		return r.handleDeletion(ctx, syncState)
	}

	// Add finalizer if not present (with conflict retry)
	if !controllerutil.ContainsFinalizer(syncState, FinalizerName) {
		if err := common.AddFinalizerWithRetry(ctx, r.Client, syncState, FinalizerName); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip if there's a pending debounced request
	if r.Debouncer.IsPending(req.Name) {
		logger.V(1).Info("Skipping reconcile - debounced request pending")
		return ctrl.Result{}, nil
	}

	// Extract Gateway rule configuration from first source
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract Gateway rule configuration")
		if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
	}

	// Compute hash for change detection
	newHash, hashErr := common.ComputeConfigHash(config)
	if hashErr != nil {
		logger.Error(hashErr, "Failed to compute config hash")
		newHash = ""
	}

	if !r.ShouldSync(syncState, newHash) {
		logger.V(1).Info("Configuration unchanged, skipping sync",
			"hash", newHash)
		return ctrl.Result{}, nil
	}

	// Set syncing status
	if err := r.SetSyncStatus(ctx, syncState, v1alpha2.SyncStatusSyncing); err != nil {
		return ctrl.Result{}, err
	}

	// Sync to Cloudflare API
	result, err := r.syncToCloudflare(ctx, syncState, config)
	if err != nil {
		logger.Error(err, "Failed to sync Gateway rule to Cloudflare")
		if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
	}

	// Update success status
	syncResult := &common.SyncResult{
		ConfigHash: newHash,
	}
	if err := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusSynced, syncResult, nil); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Successfully synced Gateway rule to Cloudflare",
		"ruleId", result.RuleID)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts Gateway rule configuration from SyncState sources.
// Gateway rules have 1:1 mapping, so we use the ExtractFirstSourceConfig helper.
func (*RuleController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*gatewaysvc.GatewayRuleConfig, error) {
	return common.ExtractFirstSourceConfig[gatewaysvc.GatewayRuleConfig](syncState)
}

// syncToCloudflare syncs the Gateway rule configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *RuleController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *gatewaysvc.GatewayRuleConfig,
) (*gatewaysvc.GatewayRuleSyncResult, error) {
	logger := log.FromContext(ctx)

	// Create API client using common helper
	apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
	if err != nil {
		return nil, err
	}

	// Validate account ID is present
	accountID, err := common.RequireAccountID(syncState)
	if err != nil {
		return nil, err
	}

	// Build Gateway rule params
	params := cf.GatewayRuleParams{
		Name:          config.Name,
		Description:   config.Description,
		Action:        config.Action,
		Enabled:       config.Enabled,
		Precedence:    config.Priority,
		Identity:      config.Identity,
		DevicePosture: config.DevicePosture,
	}

	// Build filters/traffic expression
	if len(config.Filters) > 0 {
		params.Filters = make([]cloudflare.TeamsFilterType, len(config.Filters))
		for i, f := range config.Filters {
			params.Filters[i] = cloudflare.TeamsFilterType(f.Type)
		}
		if config.Filters[0].Expression != "" {
			params.Traffic = config.Filters[0].Expression
		}
	}

	// Build schedule
	if config.Schedule != nil {
		params.Schedule = &cf.GatewayRuleScheduleParams{
			TimeZone: config.Schedule.TimeZone,
			Mon:      config.Schedule.Mon,
			Tue:      config.Schedule.Tue,
			Wed:      config.Schedule.Wed,
			Thu:      config.Schedule.Thu,
			Fri:      config.Schedule.Fri,
			Sat:      config.Schedule.Sat,
			Sun:      config.Schedule.Sun,
		}
	}

	// Build expiration
	if config.Expiration != nil {
		params.Expiration = &cf.GatewayRuleExpirationParams{
			ExpiresAt: config.Expiration.ExpiresAt,
			Duration:  config.Expiration.Duration,
		}
	}

	// Build rule settings
	if config.RuleSettings != nil {
		params.RuleSettings = r.convertRuleSettings(config.RuleSettings)
	}

	// Check if this is an existing rule or new
	cloudflareID := syncState.Spec.CloudflareID
	var result *cf.GatewayRuleResult

	if common.IsPendingID(cloudflareID) {
		// Create new rule
		logger.Info("Creating new Gateway rule",
			"name", config.Name,
			"action", config.Action)

		result, err = apiClient.CreateGatewayRule(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("create Gateway rule: %w", err)
		}

		// Update SyncState with actual rule ID (must succeed)
		if err := common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID); err != nil {
			return nil, err
		}

		logger.Info("Created Gateway rule", "ruleId", result.ID)
	} else {
		// Update existing rule
		logger.Info("Updating Gateway rule",
			"ruleId", cloudflareID,
			"name", config.Name)

		result, err = apiClient.UpdateGatewayRule(ctx, cloudflareID, params)
		if err != nil {
			if common.HandleNotFoundOnUpdate(err) {
				// Rule deleted externally, recreate it
				logger.Info("Gateway rule not found, recreating", "ruleId", cloudflareID)
				result, err = apiClient.CreateGatewayRule(ctx, params)
				if err != nil {
					return nil, fmt.Errorf("recreate Gateway rule: %w", err)
				}

				if updateErr := common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID); updateErr != nil {
					logger.Error(updateErr, "Failed to update CloudflareID after recreating")
				}
			} else {
				return nil, fmt.Errorf("update Gateway rule: %w", err)
			}
		}

		logger.Info("Updated Gateway rule", "ruleId", result.ID)
	}

	return &gatewaysvc.GatewayRuleSyncResult{
		RuleID:    result.ID,
		AccountID: accountID,
	}, nil
}

// convertRuleSettings converts service rule settings to CF API params.
//
//nolint:revive // cognitive complexity is acceptable for settings conversion logic
func (*RuleController) convertRuleSettings(settings *gatewaysvc.GatewayRuleSettings) *cf.GatewayRuleSettingsParams {
	if settings == nil {
		return nil
	}

	params := &cf.GatewayRuleSettingsParams{
		BlockPageEnabled:                settings.BlockPageEnabled,
		BlockReason:                     settings.BlockReason,
		OverrideHost:                    settings.OverrideHost,
		OverrideIPs:                     settings.OverrideIPs,
		InsecureDisableDNSSECValidation: settings.InsecureDisableDNSSECValidation,
		AddHeaders:                      settings.AddHeaders,
		UntrustedCertAction:             settings.UntrustedCertificateAction,
		ResolveDNSThroughCloudflare:     settings.ResolveDNSThroughCloudflare,
		AllowChildBypass:                settings.AllowChildBypass,
		BypassParentRule:                settings.BypassParentRule,
		IgnoreCNAMECategoryMatches:      settings.IgnoreCNAMECategoryMatches,
		IPCategories:                    settings.IPCategories,
		IPIndicatorFeeds:                settings.IPIndicatorFeeds,
	}

	if settings.L4Override != nil {
		params.L4Override = &cf.GatewayL4OverrideParams{
			IP:   settings.L4Override.IP,
			Port: settings.L4Override.Port,
		}
	}

	if settings.BISOAdminControls != nil {
		params.BISOAdminControls = &cf.GatewayBISOAdminControlsParams{
			DisablePrinting:             settings.BISOAdminControls.DisablePrinting,
			DisableCopyPaste:            settings.BISOAdminControls.DisableCopyPaste,
			DisableDownload:             settings.BISOAdminControls.DisableDownload,
			DisableUpload:               settings.BISOAdminControls.DisableUpload,
			DisableKeyboard:             settings.BISOAdminControls.DisableKeyboard,
			DisableClipboardRedirection: settings.BISOAdminControls.DisableClipboardRedirect,
		}
	}

	if settings.CheckSession != nil {
		params.CheckSession = &cf.GatewayCheckSessionParams{
			Enforce:  settings.CheckSession.Enforce,
			Duration: settings.CheckSession.Duration,
		}
	}

	if settings.Egress != nil {
		params.Egress = &cf.GatewayEgressParams{
			IPv4:         settings.Egress.Ipv4,
			IPv6:         settings.Egress.Ipv6,
			IPv4Fallback: settings.Egress.Ipv4Fallback,
		}
	}

	if settings.PayloadLog != nil {
		params.PayloadLog = &cf.GatewayPayloadLogParams{
			Enabled: settings.PayloadLog.Enabled,
		}
	}

	if settings.AuditSSH != nil {
		params.AuditSSH = &cf.GatewayAuditSSHParams{
			CommandLogging: settings.AuditSSH.CommandLogging,
		}
	}

	if settings.NotificationSettings != nil {
		params.NotificationSettings = &cf.GatewayNotificationSettingsParams{
			Enabled:    settings.NotificationSettings.Enabled,
			Message:    settings.NotificationSettings.Message,
			SupportURL: settings.NotificationSettings.SupportURL,
		}
	}

	if settings.DNSResolvers != nil {
		params.DNSResolvers = &cf.GatewayDNSResolversParams{}
		for _, resolver := range settings.DNSResolvers.Ipv4 {
			params.DNSResolvers.IPv4 = append(params.DNSResolvers.IPv4, cf.GatewayDNSResolverEntryParams{
				IP:                         resolver.IP,
				Port:                       resolver.Port,
				VNetID:                     resolver.VNetID,
				RouteThroughPrivateNetwork: resolver.RouteThroughPrivateNetwork,
			})
		}
		for _, resolver := range settings.DNSResolvers.Ipv6 {
			params.DNSResolvers.IPv6 = append(params.DNSResolvers.IPv6, cf.GatewayDNSResolverEntryParams{
				IP:                         resolver.IP,
				Port:                       resolver.Port,
				VNetID:                     resolver.VNetID,
				RouteThroughPrivateNetwork: resolver.RouteThroughPrivateNetwork,
			})
		}
	}

	if settings.ResolveDNSInternally != nil {
		params.ResolveDNSInternally = &cf.GatewayResolveDNSInternallyParams{
			ViewID:   settings.ResolveDNSInternally.ViewID,
			Fallback: settings.ResolveDNSInternally.Fallback,
		}
	}

	if settings.Quarantine != nil {
		params.Quarantine = &cf.GatewayQuarantineParams{
			FileTypes: settings.Quarantine.FileTypes,
		}
	}

	return params
}

// handleDeletion handles the deletion of Gateway Rule from Cloudflare.
// This is the SINGLE point for Cloudflare Gateway Rule deletion in the system.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller deletes from Cloudflare
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps
func (r *RuleController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Get the Cloudflare rule ID
	cloudflareID := syncState.Spec.CloudflareID

	// Skip if pending ID (rule was never created)
	if common.IsPendingID(cloudflareID) {
		logger.Info("Skipping deletion - Gateway Rule was never created",
			"cloudflareId", cloudflareID)
	} else if cloudflareID != "" {
		// Delete from Cloudflare
		apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
		if err != nil {
			logger.Error(err, "Failed to create API client for deletion")
			return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
		}

		logger.Info("Deleting Gateway Rule from Cloudflare",
			"ruleId", cloudflareID)

		if err := apiClient.DeleteGatewayRule(ctx, cloudflareID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete Gateway Rule from Cloudflare")
				if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
					logger.Error(statusErr, "Failed to update error status")
				}
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}
			logger.Info("Gateway Rule already deleted from Cloudflare")
		} else {
			logger.Info("Successfully deleted Gateway Rule from Cloudflare",
				"ruleId", cloudflareID)
		}
	}

	// Remove finalizer (with conflict retry)
	if err := common.RemoveFinalizerWithRetry(ctx, r.Client, syncState, FinalizerName); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	// If sources are empty (not a deletion timestamp trigger), delete the SyncState itself
	if syncState.DeletionTimestamp.IsZero() && len(syncState.Spec.Sources) == 0 {
		logger.Info("Deleting orphaned SyncState")
		if err := r.Client.Delete(ctx, syncState); err != nil {
			if client.IgnoreNotFound(err) != nil {
				logger.Error(err, "Failed to delete SyncState")
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RuleController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("gateway-rule-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourceGatewayRule)).
		Complete(r)
}
