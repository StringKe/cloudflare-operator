// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package dns provides the DNS Sync Controller for managing Cloudflare DNS records.
// Unlike TunnelConfigSyncController which aggregates multiple sources,
// DNSSyncController handles individual DNS records with a 1:1 mapping.
package dns

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	dnssvc "github.com/StringKe/cloudflare-operator/internal/service/dns"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

// Controller is the Sync Controller for DNS Record Configuration.
// It watches CloudflareSyncState resources of type DNSRecord,
// extracts the configuration, and syncs to Cloudflare API.
type Controller struct {
	*common.BaseSyncController
}

// NewController creates a new DNSSyncController
func NewController(c client.Client) *Controller {
	return &Controller{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for DNS record.
// The reconciliation flow:
// 1. Get the SyncState resource
// 2. Check for debounce
// 3. Extract DNS record configuration
// 4. Compute hash for change detection
// 5. If changed, sync to Cloudflare API
// 6. Update SyncState status
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "DNSSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process DNSRecord type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceDNSRecord {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing DNS SyncState",
		"cloudflareId", syncState.Spec.CloudflareID,
		"sources", len(syncState.Spec.Sources))

	// Skip if there's a pending debounced request
	if r.Debouncer.IsPending(req.Name) {
		logger.V(1).Info("Skipping reconcile - debounced request pending")
		return ctrl.Result{}, nil
	}

	// Check if there are any sources
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, marking as synced (no-op)")
		if err := r.SetSyncStatus(ctx, syncState, v1alpha2.SyncStatusSynced); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Extract DNS record configuration from first source
	// (DNS records have 1:1 mapping, so there should only be one source)
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract DNS configuration")
		if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
	}

	// Compute hash for change detection
	newHash, hashErr := common.ComputeConfigHash(config)
	if hashErr != nil {
		logger.Error(hashErr, "Failed to compute config hash")
		newHash = "" // Force sync if hash fails
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
		logger.Error(err, "Failed to sync DNS record to Cloudflare")
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

	logger.Info("Successfully synced DNS record to Cloudflare",
		"recordId", result.RecordID,
		"fqdn", result.FQDN)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts DNS record configuration from SyncState sources.
// DNS records have 1:1 mapping, so we use the ExtractFirstSourceConfig helper.
func (*Controller) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*dnssvc.DNSRecordConfig, error) {
	return common.ExtractFirstSourceConfig[dnssvc.DNSRecordConfig](syncState)
}

// syncToCloudflare syncs the DNS record configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *Controller) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *dnssvc.DNSRecordConfig,
) (*dnssvc.SyncResult, error) {
	logger := log.FromContext(ctx)

	// Create API client using common helper
	apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
	if err != nil {
		return nil, err
	}

	// Validate zone ID is present
	zoneID, err := common.RequireZoneID(syncState)
	if err != nil {
		return nil, err
	}

	// Build DNS record params
	params := cf.DNSRecordParams{
		Name:    config.Name,
		Type:    config.Type,
		Content: config.Content,
		TTL:     config.TTL,
		Proxied: config.Proxied,
		Comment: config.Comment,
		Tags:    config.Tags,
	}

	// Handle Priority
	if config.Priority != nil {
		params.Priority = config.Priority
	}

	// Handle Data
	if config.Data != nil {
		params.Data = convertRecordData(config.Data)
	}

	// Check if this is a new record (pending) or existing (has real Cloudflare ID)
	cloudflareID := syncState.Spec.CloudflareID
	var result *cf.DNSRecordResult

	if common.IsPendingID(cloudflareID) {
		// Create new record
		logger.Info("Creating new DNS record",
			"name", config.Name,
			"type", config.Type,
			"zoneId", zoneID)

		result, err = apiClient.CreateDNSRecordInZone(zoneID, params)
		if err != nil {
			return nil, fmt.Errorf("create DNS record: %w", err)
		}

		// Update SyncState with actual record ID
		common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID)

		logger.Info("Created DNS record",
			"recordId", result.ID,
			"fqdn", result.Name)
	} else {
		// Update existing record
		logger.Info("Updating DNS record",
			"recordId", cloudflareID,
			"name", config.Name,
			"zoneId", zoneID)

		result, err = apiClient.UpdateDNSRecordInZone(zoneID, cloudflareID, params)
		if err != nil {
			// Check if record was deleted externally
			if common.HandleNotFoundOnUpdate(err) {
				// Record deleted externally, recreate it
				logger.Info("DNS record not found, recreating",
					"recordId", cloudflareID)
				result, err = apiClient.CreateDNSRecordInZone(zoneID, params)
				if err != nil {
					return nil, fmt.Errorf("recreate DNS record: %w", err)
				}

				// Update SyncState with new record ID
				common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID)
			} else {
				return nil, fmt.Errorf("update DNS record: %w", err)
			}
		}

		logger.Info("Updated DNS record",
			"recordId", result.ID,
			"fqdn", result.Name)
	}

	return &dnssvc.SyncResult{
		RecordID: result.ID,
		ZoneID:   result.ZoneID,
		FQDN:     result.Name,
	}, nil
}

// convertRecordData converts dnssvc.DNSRecordData to cf.DNSRecordDataParams.
func convertRecordData(data *dnssvc.DNSRecordData) *cf.DNSRecordDataParams {
	if data == nil {
		return nil
	}

	return &cf.DNSRecordDataParams{
		// SRV record data
		Service: data.Service,
		Proto:   data.Proto,
		Weight:  data.Weight,
		Port:    data.Port,
		Target:  data.Target,

		// CAA record data
		Flags: data.Flags,
		Tag:   data.Tag,
		Value: data.Value,

		// CERT/SSHFP/TLSA record data
		Algorithm:    data.Algorithm,
		Certificate:  data.Certificate,
		KeyTag:       data.KeyTag,
		Usage:        data.Usage,
		Selector:     data.Selector,
		MatchingType: data.MatchingType,

		// LOC record data
		LatDegrees:    data.LatDegrees,
		LatMinutes:    data.LatMinutes,
		LatSeconds:    data.LatSeconds,
		LatDirection:  data.LatDirection,
		LongDegrees:   data.LongDegrees,
		LongMinutes:   data.LongMinutes,
		LongSeconds:   data.LongSeconds,
		LongDirection: data.LongDirection,
		Altitude:      data.Altitude,
		Size:          data.Size,
		PrecisionHorz: data.PrecisionHorz,
		PrecisionVert: data.PrecisionVert,

		// URI record data
		ContentURI: data.ContentURI,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourceDNSRecord)).
		Complete(r)
}
