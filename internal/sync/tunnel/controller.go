// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package tunnel

import (
	"context"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	tunnelsvc "github.com/StringKe/cloudflare-operator/internal/service/tunnel"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=cloudflaresyncstates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=cloudflaresyncstates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=cloudflaresyncstates/finalizers,verbs=update

// Controller is the Sync Controller for Tunnel Configuration.
// It watches CloudflareSyncState resources of type TunnelConfiguration,
// aggregates configuration from multiple sources, and syncs to Cloudflare API.
type Controller struct {
	*common.BaseSyncController
}

// NewController creates a new TunnelConfigSyncController
func NewController(c client.Client) *Controller {
	return &Controller{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for tunnel configuration.
// The reconciliation flow:
// 1. Get the SyncState resource
// 2. Debounce rapid changes
// 3. Aggregate configuration from all sources
// 4. Compute hash for change detection
// 5. If changed, sync to Cloudflare API
// 6. Update SyncState status
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "TunnelConfigSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState resource
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Verify this is a TunnelConfiguration type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceTunnelConfiguration {
		logger.V(1).Info("Skipping non-tunnel SyncState",
			"resourceType", syncState.Spec.ResourceType)
		return ctrl.Result{}, nil
	}

	// Skip if there's a pending debounced request (will be reconciled later)
	if r.Debouncer.IsPending(req.Name) {
		logger.V(1).Info("Skipping reconcile - debounced request pending")
		return ctrl.Result{}, nil
	}

	// Set status to Syncing
	if err := r.SetSyncStatus(ctx, syncState, v1alpha2.SyncStatusSyncing); err != nil {
		logger.Error(err, "Failed to set syncing status")
	}

	// Aggregate configuration from all sources
	aggregatedConfig, err := Aggregate(syncState)
	if err != nil {
		syncErr := fmt.Errorf("aggregate configuration: %w", err)
		if updateErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, syncErr); updateErr != nil {
			logger.Error(updateErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, syncErr
	}

	// Store aggregated config in status for debugging
	if err := r.StoreAggregatedConfig(syncState, aggregatedConfig); err != nil {
		logger.Error(err, "Failed to store aggregated config")
	}

	// Validate the aggregated configuration
	if err := ValidateAggregatedConfig(aggregatedConfig); err != nil {
		syncErr := fmt.Errorf("validate configuration: %w", err)
		if updateErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, syncErr); updateErr != nil {
			logger.Error(updateErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, syncErr
	}

	// Compute hash for change detection
	configHash, err := common.ComputeConfigHashDeterministic(aggregatedConfig)
	if err != nil {
		syncErr := fmt.Errorf("compute config hash: %w", err)
		if updateErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, syncErr); updateErr != nil {
			logger.Error(updateErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, syncErr
	}

	// Check if configuration has changed
	if !r.ShouldSync(syncState, configHash) {
		logger.V(1).Info("Configuration unchanged, skipping sync", "hash", configHash)
		return ctrl.Result{}, nil
	}

	logger.Info("Configuration changed, syncing to Cloudflare",
		"tunnelId", syncState.Spec.CloudflareID,
		"previousHash", syncState.Status.ConfigHash,
		"newHash", configHash,
		"sourceCount", len(syncState.Spec.Sources),
		"ruleCount", len(aggregatedConfig.Ingress))

	// Create Cloudflare API client
	cfAPI, err := r.createAPIClient(ctx, syncState)
	if err != nil {
		syncErr := fmt.Errorf("create API client: %w", err)
		if updateErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, syncErr); updateErr != nil {
			logger.Error(updateErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, syncErr
	}

	// Sync to Cloudflare API
	result, err := r.syncToCloudflare(ctx, cfAPI, syncState.Spec.CloudflareID, aggregatedConfig)
	if err != nil {
		syncErr := fmt.Errorf("sync to Cloudflare: %w", err)
		if updateErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, syncErr); updateErr != nil {
			logger.Error(updateErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, syncErr
	}

	// Update status with success
	syncResult := &common.SyncResult{
		ConfigVersion: result.Version,
		ConfigHash:    configHash,
	}
	if err := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusSynced, syncResult, nil); err != nil {
		logger.Error(err, "Failed to update success status")
		return ctrl.Result{}, err
	}

	logger.Info("Tunnel configuration synced successfully",
		"tunnelId", syncState.Spec.CloudflareID,
		"version", result.Version,
		"hostnames", ExtractHostnames(aggregatedConfig))

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// createAPIClient creates a Cloudflare API client from the SyncState credentials
func (r *Controller) createAPIClient(ctx context.Context, syncState *v1alpha2.CloudflareSyncState) (*cf.API, error) {
	credRef := &v1alpha2.CloudflareCredentialsRef{
		Name: syncState.Spec.CredentialsRef.Name,
	}

	return cf.NewAPIClientFromCredentialsRef(ctx, r.Client, credRef)
}

// syncToCloudflare syncs the aggregated configuration to Cloudflare API
func (r *Controller) syncToCloudflare(
	ctx context.Context,
	cfAPI *cf.API,
	tunnelID string,
	config *AggregatedConfig,
) (*SyncToCloudflareResult, error) {
	logger := log.FromContext(ctx)

	// Convert aggregated config to Cloudflare API format
	cfConfig := r.buildCloudflareConfig(config)

	logger.V(1).Info("Updating tunnel configuration",
		"tunnelId", tunnelID,
		"ingressCount", len(cfConfig.Ingress))

	// Call Cloudflare API to update configuration
	result, err := cfAPI.UpdateTunnelConfiguration(tunnelID, cfConfig)
	if err != nil {
		return nil, fmt.Errorf("update tunnel configuration: %w", err)
	}

	return &SyncToCloudflareResult{
		Version: result.Version,
	}, nil
}

// buildCloudflareConfig converts AggregatedConfig to cloudflare.TunnelConfiguration
func (r *Controller) buildCloudflareConfig(config *AggregatedConfig) cloudflare.TunnelConfiguration {
	cfConfig := cloudflare.TunnelConfiguration{
		Ingress: make([]cloudflare.UnvalidatedIngressRule, 0, len(config.Ingress)),
	}

	// Convert ingress rules
	for _, rule := range config.Ingress {
		cfRule := cloudflare.UnvalidatedIngressRule{
			Hostname: rule.Hostname,
			Path:     rule.Path,
			Service:  rule.Service,
		}

		// Convert origin request config if present
		if rule.OriginRequest != nil {
			cfRule.OriginRequest = r.convertOriginRequest(rule.OriginRequest)
		}

		cfConfig.Ingress = append(cfConfig.Ingress, cfRule)
	}

	// Convert WarpRouting if present
	if config.WarpRouting != nil {
		cfConfig.WarpRouting = &cloudflare.WarpRoutingConfig{
			Enabled: config.WarpRouting.Enabled,
		}
	}

	// Convert global OriginRequest if present
	if config.OriginRequest != nil {
		converted := r.convertOriginRequest(config.OriginRequest)
		if converted != nil {
			cfConfig.OriginRequest = *converted
		}
	}

	return cfConfig
}

// convertOriginRequest converts internal OriginRequestConfig to Cloudflare SDK type
func (*Controller) convertOriginRequest(config *tunnelsvc.OriginRequestConfig) *cloudflare.OriginRequestConfig {
	if config == nil {
		return nil
	}

	cfConfig := &cloudflare.OriginRequestConfig{}

	// Convert duration fields
	if config.ConnectTimeout != nil {
		cfConfig.ConnectTimeout = &cloudflare.TunnelDuration{Duration: *config.ConnectTimeout}
	}
	if config.TLSTimeout != nil {
		cfConfig.TLSTimeout = &cloudflare.TunnelDuration{Duration: *config.TLSTimeout}
	}
	if config.TCPKeepAlive != nil {
		cfConfig.TCPKeepAlive = &cloudflare.TunnelDuration{Duration: *config.TCPKeepAlive}
	}
	if config.KeepAliveTimeout != nil {
		cfConfig.KeepAliveTimeout = &cloudflare.TunnelDuration{Duration: *config.KeepAliveTimeout}
	}

	// Copy pointer fields directly
	cfConfig.NoHappyEyeballs = config.NoHappyEyeballs
	cfConfig.KeepAliveConnections = config.KeepAliveConnections
	cfConfig.HTTPHostHeader = config.HTTPHostHeader
	cfConfig.OriginServerName = config.OriginServerName
	cfConfig.CAPool = config.CAPool
	cfConfig.NoTLSVerify = config.NoTLSVerify
	cfConfig.Http2Origin = config.HTTP2Origin
	cfConfig.DisableChunkedEncoding = config.DisableChunkedEncoding
	cfConfig.BastionMode = config.BastionMode
	cfConfig.ProxyAddress = config.ProxyAddress
	cfConfig.ProxyPort = config.ProxyPort
	cfConfig.ProxyType = config.ProxyType

	return cfConfig
}

// SyncToCloudflareResult contains the result of syncing to Cloudflare API
type SyncToCloudflareResult struct {
	// Version is the configuration version returned by Cloudflare
	Version int
}

// isTunnelConfigSyncState checks if the object is a TunnelConfiguration SyncState
func isTunnelConfigSyncState(obj client.Object) bool {
	ss, ok := obj.(*v1alpha2.CloudflareSyncState)
	return ok && ss.Spec.ResourceType == v1alpha2.SyncResourceTunnelConfiguration
}

// SetupWithManager sets up the controller with the Manager.
//
//nolint:revive // cognitive complexity is acceptable for setup function
func (r *Controller) SetupWithManager(mgr ctrl.Manager) error {
	// Predicate to only watch TunnelConfiguration type SyncStates
	tunnelConfigPredicate := predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return isTunnelConfigSyncState(e.Object) },
		UpdateFunc:  func(e event.UpdateEvent) bool { return isTunnelConfigSyncState(e.ObjectNew) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return isTunnelConfigSyncState(e.Object) },
		GenericFunc: func(e event.GenericEvent) bool { return isTunnelConfigSyncState(e.Object) },
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("tunnel-config-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(tunnelConfigPredicate).
		Complete(r)
}
