// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package domain provides Sync Controllers for domain-related Cloudflare resources.
//
//nolint:revive // max-public-structs is acceptable for this package
package domain

import (
	"context"
	"encoding/json"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	domainsvc "github.com/StringKe/cloudflare-operator/internal/service/domain"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

const (
	// CloudflareDomainFinalizerName is the finalizer for CloudflareDomain SyncState resources.
	CloudflareDomainFinalizerName = "cloudflaredomain.sync.cloudflare-operator.io/finalizer"
)

// Zone setting values for boolean on/off settings
const (
	zoneSettingOn  = "on"
	zoneSettingOff = "off"
)

// boolToOnOff converts a boolean to "on" or "off" string for zone settings
func boolToOnOff(b bool) string {
	if b {
		return zoneSettingOn
	}
	return zoneSettingOff
}

// CloudflareDomainController is the Sync Controller for CloudflareDomain resources.
// It watches CloudflareSyncState resources of type CloudflareDomain and
// syncs zone settings to Cloudflare API.
type CloudflareDomainController struct {
	*common.BaseSyncController
}

// NewCloudflareDomainController creates a new CloudflareDomainController
func NewCloudflareDomainController(c client.Client) *CloudflareDomainController {
	return &CloudflareDomainController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for CloudflareDomain.
//
//nolint:revive // cognitive complexity is acceptable for this reconciliation loop
func (r *CloudflareDomainController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "CloudflareDomainSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState resource
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Verify this is a CloudflareDomain type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceCloudflareDomain {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing CloudflareDomain SyncState",
		"cloudflareId", syncState.Spec.CloudflareID,
		"sources", len(syncState.Spec.Sources))

	// Handle deletion - this is the SINGLE point for cleanup
	// Note: CloudflareDomain doesn't "delete" zone settings - we just clean up the SyncState
	if !syncState.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, syncState)
	}

	// Check if there are any sources - if none, clean up
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, cleaning up")
		return r.handleDeletion(ctx, syncState)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(syncState, CloudflareDomainFinalizerName) {
		controllerutil.AddFinalizer(syncState, CloudflareDomainFinalizerName)
		if err := r.Client.Update(ctx, syncState); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip if there's a pending debounced request
	if r.Debouncer.IsPending(req.Name) {
		logger.V(1).Info("Skipping reconcile - debounced request pending")
		return ctrl.Result{}, nil
	}

	// Get domain config from sources
	config, err := r.getDomainConfig(syncState)
	if err != nil {
		return r.handleError(ctx, syncState, fmt.Errorf("get domain config: %w", err))
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

	// Set status to Syncing
	if err := r.SetSyncStatus(ctx, syncState, v1alpha2.SyncStatusSyncing); err != nil {
		logger.Error(err, "Failed to set syncing status")
	}

	// Check if zone ID is available
	zoneID := syncState.Spec.ZoneID
	if zoneID == "" {
		logger.Info("Zone ID not available, skipping sync")
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(nil)}, nil
	}

	logger.Info("Syncing CloudflareDomain settings",
		"domain", config.Domain,
		"zoneId", zoneID)

	// Create Cloudflare API client
	cfAPI, err := r.createAPIClient(ctx, syncState)
	if err != nil {
		return r.handleError(ctx, syncState, fmt.Errorf("create API client: %w", err))
	}

	// Sync zone settings
	if err := r.syncZoneSettings(ctx, cfAPI, zoneID, config); err != nil {
		return r.handleError(ctx, syncState, fmt.Errorf("sync zone settings: %w", err))
	}

	// Update status with success
	syncResult := &common.SyncResult{
		ConfigHash: newHash,
	}
	if err := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusSynced, syncResult, nil); err != nil {
		logger.Error(err, "Failed to update success status")
		return ctrl.Result{}, err
	}

	logger.Info("CloudflareDomain settings synced successfully",
		"domain", config.Domain,
		"zoneId", zoneID)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// getDomainConfig extracts the domain configuration from SyncState sources.
// Assumes sources is non-empty (caller should check beforehand).
func (*CloudflareDomainController) getDomainConfig(syncState *v1alpha2.CloudflareSyncState) (*domainsvc.CloudflareDomainConfig, error) {
	// Use the highest priority source
	source := syncState.Spec.Sources[0]
	var config domainsvc.CloudflareDomainConfig
	if err := json.Unmarshal(source.Config.Raw, &config); err != nil {
		return nil, fmt.Errorf("unmarshal domain config: %w", err)
	}
	return &config, nil
}

// handleDeletion handles the deletion of CloudflareDomain from Kubernetes.
// Note: Zone settings are NOT deleted from Cloudflare - they persist on the zone.
// This method only cleans up the SyncState resource.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller cleans up SyncState
func (r *CloudflareDomainController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, CloudflareDomainFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Note: We intentionally do NOT reset zone settings on Cloudflare
	// Zone settings are persistent configurations that should remain even after
	// the CloudflareDomain resource is deleted from Kubernetes.
	// Users who want to reset settings should modify them before deletion.
	logger.Info("CloudflareDomain deleted - zone settings will be preserved on Cloudflare",
		"zoneId", syncState.Spec.ZoneID)

	// Remove finalizer
	controllerutil.RemoveFinalizer(syncState, CloudflareDomainFinalizerName)
	if err := r.Client.Update(ctx, syncState); err != nil {
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

// createAPIClient creates a Cloudflare API client from the SyncState credentials
func (r *CloudflareDomainController) createAPIClient(ctx context.Context, syncState *v1alpha2.CloudflareSyncState) (*cf.API, error) {
	credRef := &v1alpha2.CloudflareCredentialsRef{
		Name: syncState.Spec.CredentialsRef.Name,
	}
	return cf.NewAPIClientFromCredentialsRef(ctx, r.Client, credRef)
}

// syncZoneSettings syncs the zone settings to Cloudflare
//
//nolint:revive // cognitive complexity is acceptable for settings sync
func (r *CloudflareDomainController) syncZoneSettings(
	ctx context.Context,
	cfAPI *cf.API,
	zoneID string,
	config *domainsvc.CloudflareDomainConfig,
) error {
	logger := log.FromContext(ctx)

	// Sync SSL settings
	if config.SSL != nil {
		if err := r.syncSSLSettings(ctx, cfAPI, zoneID, config.SSL); err != nil {
			logger.Error(err, "Failed to sync SSL settings")
			return fmt.Errorf("sync SSL settings: %w", err)
		}
	}

	// Sync Cache settings
	if config.Cache != nil {
		if err := r.syncCacheSettings(ctx, cfAPI, zoneID, config.Cache); err != nil {
			logger.Error(err, "Failed to sync Cache settings")
			return fmt.Errorf("sync Cache settings: %w", err)
		}
	}

	// Sync Security settings
	if config.Security != nil {
		if err := r.syncSecuritySettings(ctx, cfAPI, zoneID, config.Security); err != nil {
			logger.Error(err, "Failed to sync Security settings")
			return fmt.Errorf("sync Security settings: %w", err)
		}
	}

	// Sync Performance settings
	if config.Performance != nil {
		if err := r.syncPerformanceSettings(ctx, cfAPI, zoneID, config.Performance); err != nil {
			logger.Error(err, "Failed to sync Performance settings")
			return fmt.Errorf("sync Performance settings: %w", err)
		}
	}

	return nil
}

// syncSSLSettings syncs SSL/TLS settings to Cloudflare
//
//nolint:revive // cognitive complexity is acceptable for syncing multiple settings
func (*CloudflareDomainController) syncSSLSettings(
	ctx context.Context,
	cfAPI *cf.API,
	zoneID string,
	ssl *domainsvc.SSLConfig,
) error {
	logger := log.FromContext(ctx)

	if ssl.Mode != "" {
		logger.V(1).Info("Setting SSL mode", "mode", ssl.Mode)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "ssl", ssl.Mode); err != nil {
			return fmt.Errorf("set SSL mode: %w", err)
		}
	}

	if ssl.MinVersion != "" {
		logger.V(1).Info("Setting min TLS version", "version", ssl.MinVersion)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "min_tls_version", ssl.MinVersion); err != nil {
			return fmt.Errorf("set min TLS version: %w", err)
		}
	}

	if ssl.AlwaysUseHTTPS != nil {
		logger.V(1).Info("Setting always use HTTPS", "enabled", *ssl.AlwaysUseHTTPS)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "always_use_https", boolToOnOff(*ssl.AlwaysUseHTTPS)); err != nil {
			return fmt.Errorf("set always use HTTPS: %w", err)
		}
	}

	if ssl.AutomaticHTTPSRewrites != nil {
		logger.V(1).Info("Setting automatic HTTPS rewrites", "enabled", *ssl.AutomaticHTTPSRewrites)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "automatic_https_rewrites", boolToOnOff(*ssl.AutomaticHTTPSRewrites)); err != nil {
			return fmt.Errorf("set automatic HTTPS rewrites: %w", err)
		}
	}

	if ssl.OpportunisticEncryption != nil {
		logger.V(1).Info("Setting opportunistic encryption", "enabled", *ssl.OpportunisticEncryption)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "opportunistic_encryption", boolToOnOff(*ssl.OpportunisticEncryption)); err != nil {
			return fmt.Errorf("set opportunistic encryption: %w", err)
		}
	}

	return nil
}

// syncCacheSettings syncs cache settings to Cloudflare
//
//nolint:revive // cognitive complexity is acceptable for syncing multiple settings
func (*CloudflareDomainController) syncCacheSettings(
	ctx context.Context,
	cfAPI *cf.API,
	zoneID string,
	cache *domainsvc.CacheConfig,
) error {
	logger := log.FromContext(ctx)

	if cache.Level != "" {
		logger.V(1).Info("Setting cache level", "level", cache.Level)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "cache_level", cache.Level); err != nil {
			return fmt.Errorf("set cache level: %w", err)
		}
	}

	if cache.BrowserTTL > 0 {
		logger.V(1).Info("Setting browser TTL", "ttl", cache.BrowserTTL)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "browser_cache_ttl", cache.BrowserTTL); err != nil {
			return fmt.Errorf("set browser TTL: %w", err)
		}
	}

	if cache.DevelopmentMode != nil {
		logger.V(1).Info("Setting development mode", "enabled", *cache.DevelopmentMode)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "development_mode", boolToOnOff(*cache.DevelopmentMode)); err != nil {
			return fmt.Errorf("set development mode: %w", err)
		}
	}

	if cache.AlwaysOnline != nil {
		logger.V(1).Info("Setting always online", "enabled", *cache.AlwaysOnline)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "always_online", boolToOnOff(*cache.AlwaysOnline)); err != nil {
			return fmt.Errorf("set always online: %w", err)
		}
	}

	return nil
}

// syncSecuritySettings syncs security settings to Cloudflare
//
//nolint:revive // cognitive complexity is acceptable for syncing multiple settings
func (*CloudflareDomainController) syncSecuritySettings(
	ctx context.Context,
	cfAPI *cf.API,
	zoneID string,
	security *domainsvc.SecurityConfig,
) error {
	logger := log.FromContext(ctx)

	if security.Level != "" {
		logger.V(1).Info("Setting security level", "level", security.Level)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "security_level", security.Level); err != nil {
			return fmt.Errorf("set security level: %w", err)
		}
	}

	if security.BrowserIntegrityCheck != nil {
		logger.V(1).Info("Setting browser integrity check", "enabled", *security.BrowserIntegrityCheck)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "browser_check", boolToOnOff(*security.BrowserIntegrityCheck)); err != nil {
			return fmt.Errorf("set browser integrity check: %w", err)
		}
	}

	if security.EmailObfuscation != nil {
		logger.V(1).Info("Setting email obfuscation", "enabled", *security.EmailObfuscation)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "email_obfuscation", boolToOnOff(*security.EmailObfuscation)); err != nil {
			return fmt.Errorf("set email obfuscation: %w", err)
		}
	}

	if security.HotlinkProtection != nil {
		logger.V(1).Info("Setting hotlink protection", "enabled", *security.HotlinkProtection)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "hotlink_protection", boolToOnOff(*security.HotlinkProtection)); err != nil {
			return fmt.Errorf("set hotlink protection: %w", err)
		}
	}

	// WAF settings require separate API calls
	if security.WAF != nil && security.WAF.Enabled != nil {
		logger.V(1).Info("Setting WAF", "enabled", *security.WAF.Enabled)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "waf", boolToOnOff(*security.WAF.Enabled)); err != nil {
			return fmt.Errorf("set WAF: %w", err)
		}
	}

	return nil
}

// syncPerformanceSettings syncs performance settings to Cloudflare
//
//nolint:gocyclo,revive // complexity is acceptable for syncing multiple zone settings
func (*CloudflareDomainController) syncPerformanceSettings(
	ctx context.Context,
	cfAPI *cf.API,
	zoneID string,
	perf *domainsvc.PerformanceConfig,
) error {
	logger := log.FromContext(ctx)

	if perf.Minify != nil {
		// Build minify value as a map
		minify := map[string]string{
			"html": zoneSettingOff,
			"css":  zoneSettingOff,
			"js":   zoneSettingOff,
		}
		if perf.Minify.HTML != nil && *perf.Minify.HTML {
			minify["html"] = zoneSettingOn
		}
		if perf.Minify.CSS != nil && *perf.Minify.CSS {
			minify["css"] = zoneSettingOn
		}
		if perf.Minify.JS != nil && *perf.Minify.JS {
			minify["js"] = zoneSettingOn
		}
		logger.V(1).Info("Setting minify", "html", minify["html"], "css", minify["css"], "js", minify["js"])
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "minify", minify); err != nil {
			return fmt.Errorf("set minify: %w", err)
		}
	}

	if perf.Polish != "" {
		logger.V(1).Info("Setting Polish", "value", perf.Polish)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "polish", perf.Polish); err != nil {
			return fmt.Errorf("set Polish: %w", err)
		}
	}

	if perf.Mirage != nil {
		logger.V(1).Info("Setting Mirage", "enabled", *perf.Mirage)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "mirage", boolToOnOff(*perf.Mirage)); err != nil {
			return fmt.Errorf("set Mirage: %w", err)
		}
	}

	if perf.Brotli != nil {
		logger.V(1).Info("Setting Brotli", "enabled", *perf.Brotli)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "brotli", boolToOnOff(*perf.Brotli)); err != nil {
			return fmt.Errorf("set Brotli: %w", err)
		}
	}

	if perf.EarlyHints != nil {
		logger.V(1).Info("Setting Early Hints", "enabled", *perf.EarlyHints)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "early_hints", boolToOnOff(*perf.EarlyHints)); err != nil {
			return fmt.Errorf("set Early Hints: %w", err)
		}
	}

	if perf.HTTP2 != nil {
		logger.V(1).Info("Setting HTTP/2", "enabled", *perf.HTTP2)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "http2", boolToOnOff(*perf.HTTP2)); err != nil {
			return fmt.Errorf("set HTTP/2: %w", err)
		}
	}

	if perf.HTTP3 != nil {
		logger.V(1).Info("Setting HTTP/3", "enabled", *perf.HTTP3)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "http3", boolToOnOff(*perf.HTTP3)); err != nil {
			return fmt.Errorf("set HTTP/3: %w", err)
		}
	}

	if perf.ZeroRTT != nil {
		logger.V(1).Info("Setting 0-RTT", "enabled", *perf.ZeroRTT)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "0rtt", boolToOnOff(*perf.ZeroRTT)); err != nil {
			return fmt.Errorf("set 0-RTT: %w", err)
		}
	}

	if perf.RocketLoader != nil {
		logger.V(1).Info("Setting Rocket Loader", "enabled", *perf.RocketLoader)
		if err := cfAPI.UpdateZoneSetting(ctx, zoneID, "rocket_loader", boolToOnOff(*perf.RocketLoader)); err != nil {
			return fmt.Errorf("set Rocket Loader: %w", err)
		}
	}

	return nil
}

// handleError updates the SyncState status with an error
func (r *CloudflareDomainController) handleError(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	err error,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if updateErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); updateErr != nil {
		logger.Error(updateErr, "Failed to update error status")
	}

	return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, err
}

// isCloudflareDomainSyncState checks if the object is a CloudflareDomain SyncState
func isCloudflareDomainSyncState(obj client.Object) bool {
	ss, ok := obj.(*v1alpha2.CloudflareSyncState)
	return ok && ss.Spec.ResourceType == v1alpha2.SyncResourceCloudflareDomain
}

// SetupWithManager sets up the controller with the Manager.
func (r *CloudflareDomainController) SetupWithManager(mgr ctrl.Manager) error {
	// Predicate to only watch CloudflareDomain type SyncStates
	domainPredicate := predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return isCloudflareDomainSyncState(e.Object) },
		UpdateFunc:  func(e event.UpdateEvent) bool { return isCloudflareDomainSyncState(e.ObjectNew) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return isCloudflareDomainSyncState(e.Object) },
		GenericFunc: func(e event.GenericEvent) bool { return isCloudflareDomainSyncState(e.Object) },
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("cloudflaredomain-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(domainPredicate).
		Complete(r)
}
