// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

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
	// DomainRegistrationFinalizerName is the finalizer for DomainRegistration SyncState resources.
	DomainRegistrationFinalizerName = "domainregistration.sync.cloudflare-operator.io/finalizer"
)

// DomainRegistrationController is the Sync Controller for DomainRegistration operations.
// It watches CloudflareSyncState resources of type DomainRegistration and
// performs the actual Cloudflare API calls for domain sync and configuration updates.
type DomainRegistrationController struct {
	*common.BaseSyncController
}

// NewDomainRegistrationController creates a new DomainRegistrationSyncController.
func NewDomainRegistrationController(c client.Client) *DomainRegistrationController {
	return &DomainRegistrationController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for DomainRegistration operations.
//
//nolint:revive // cognitive complexity is acceptable for this reconciliation loop
func (r *DomainRegistrationController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "DomainRegistrationSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState resource
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Verify this is a DomainRegistration type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceDomainRegistration {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing DomainRegistration SyncState",
		"cloudflareId", syncState.Spec.CloudflareID,
		"sources", len(syncState.Spec.Sources))

	// Handle deletion - this is the SINGLE point for cleanup
	if !syncState.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, syncState)
	}

	// Check if there are any sources - if none, clean up
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, cleaning up")
		return r.handleDeletion(ctx, syncState)
	}

	// Add finalizer if not present (with conflict retry)
	if !controllerutil.ContainsFinalizer(syncState, DomainRegistrationFinalizerName) {
		if err := common.AddFinalizerWithRetry(ctx, r.Client, syncState, DomainRegistrationFinalizerName); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip if already synced (for periodic sync, we re-sync after an hour)
	if syncState.Status.SyncStatus == v1alpha2.SyncStatusSynced {
		lastSync := syncState.Status.LastSyncTime
		if lastSync != nil && time.Since(lastSync.Time) < 55*time.Minute {
			logger.V(1).Info("Domain registration recently synced, skipping")
			return ctrl.Result{RequeueAfter: 1 * time.Hour}, nil
		}
	}

	// Skip if there's a pending debounced request
	if r.Debouncer.IsPending(req.Name) {
		logger.V(1).Info("Skipping reconcile - debounced request pending")
		return ctrl.Result{}, nil
	}

	// Set status to Syncing
	if err := r.SetSyncStatus(ctx, syncState, v1alpha2.SyncStatusSyncing); err != nil {
		logger.Error(err, "Failed to set syncing status")
	}

	// Get lifecycle config from sources
	config, err := r.getLifecycleConfig(syncState)
	if err != nil {
		return r.handleError(ctx, syncState, fmt.Errorf("get lifecycle config: %w", err))
	}

	logger.Info("Processing DomainRegistration operation",
		"action", config.Action,
		"domainName", config.DomainName)

	// Create Cloudflare API client
	cfAPI, err := r.createAPIClient(ctx, syncState)
	if err != nil {
		return r.handleError(ctx, syncState, fmt.Errorf("create API client: %w", err))
	}

	// Execute sync operation
	result, err := r.syncDomain(ctx, cfAPI, config)
	if err != nil {
		return r.handleError(ctx, syncState, fmt.Errorf("%s domain: %w", config.Action, err))
	}

	// Update status with success
	if err := r.updateSuccessStatus(ctx, syncState, result); err != nil {
		logger.Error(err, "Failed to update success status")
		return ctrl.Result{}, err
	}

	logger.Info("DomainRegistration operation completed successfully",
		"action", config.Action,
		"domainId", result.DomainID)

	// Requeue periodically to monitor domain status
	return ctrl.Result{RequeueAfter: 1 * time.Hour}, nil
}

// getLifecycleConfig extracts the lifecycle configuration from SyncState sources.
func (*DomainRegistrationController) getLifecycleConfig(
	syncState *v1alpha2.CloudflareSyncState,
) (*domainsvc.DomainRegistrationLifecycleConfig, error) {
	if len(syncState.Spec.Sources) == 0 {
		return nil, errors.New("no sources found in syncstate")
	}

	// Use the highest priority source (lowest priority number)
	source := syncState.Spec.Sources[0]

	var config domainsvc.DomainRegistrationLifecycleConfig
	if err := json.Unmarshal(source.Config.Raw, &config); err != nil {
		return nil, fmt.Errorf("unmarshal lifecycle config: %w", err)
	}
	return &config, nil
}

// createAPIClient creates a Cloudflare API client from the SyncState credentials.
func (r *DomainRegistrationController) createAPIClient(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (*cf.API, error) {
	credRef := &v1alpha2.CloudflareCredentialsRef{
		Name: syncState.Spec.CredentialsRef.Name,
	}
	return cf.NewAPIClientFromCredentialsRef(ctx, r.Client, credRef)
}

// syncDomain syncs domain registration information from Cloudflare.
//
//nolint:revive // cognitive complexity is acceptable for sync logic
func (*DomainRegistrationController) syncDomain(
	ctx context.Context,
	cfAPI *cf.API,
	config *domainsvc.DomainRegistrationLifecycleConfig,
) (*domainsvc.DomainRegistrationSyncResult, error) {
	logger := log.FromContext(ctx)

	logger.Info("Syncing domain registration", "domainName", config.DomainName)

	// Get domain information from Cloudflare
	domainInfo, err := cfAPI.GetRegistrarDomain(ctx, config.DomainName)
	if err != nil {
		return nil, fmt.Errorf("get registrar domain: %w", err)
	}

	result := &domainsvc.DomainRegistrationSyncResult{
		DomainID:         domainInfo.ID,
		CurrentRegistrar: domainInfo.CurrentRegistrar,
		RegistryStatuses: domainInfo.RegistryStatuses,
		Locked:           domainInfo.Locked,
		TransferInStatus: domainInfo.TransferInStatus,
	}

	if !domainInfo.ExpiresAt.IsZero() {
		result.ExpiresAt.Time = domainInfo.ExpiresAt
	}
	if !domainInfo.CreatedAt.IsZero() {
		result.CreatedAt.Time = domainInfo.CreatedAt
	}

	// Update configuration if present
	if config.Configuration != nil && config.Action == domainsvc.DomainRegistrationActionUpdate {
		logger.Info("Updating domain configuration", "domainName", config.DomainName)

		cfConfig := cf.RegistrarDomainConfig{
			AutoRenew: config.Configuration.AutoRenew,
			Privacy:   config.Configuration.Privacy,
			Locked:    config.Configuration.Locked,
		}
		if len(config.Configuration.NameServers) > 0 {
			cfConfig.NameServers = config.Configuration.NameServers
		}

		updatedInfo, err := cfAPI.UpdateRegistrarDomain(ctx, config.DomainName, cfConfig)
		if err != nil {
			return nil, fmt.Errorf("update registrar domain: %w", err)
		}

		result.Locked = updatedInfo.Locked
		result.AutoRenew = config.Configuration.AutoRenew
		result.Privacy = config.Configuration.Privacy
	}

	logger.Info("Domain registration synced successfully",
		"domainId", result.DomainID,
		"currentRegistrar", result.CurrentRegistrar)

	return result, nil
}

// handleError updates the SyncState status with an error and returns appropriate result.
func (r *DomainRegistrationController) handleError(
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

// updateSuccessStatus updates the SyncState status with success and result data.
//
//nolint:revive // cognitive complexity is acceptable for building result data
func (r *DomainRegistrationController) updateSuccessStatus(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	result *domainsvc.DomainRegistrationSyncResult,
) error {
	// Build result data map
	resultData := make(map[string]string)
	if result != nil {
		resultData[domainsvc.ResultKeyDomainID] = result.DomainID
		resultData[domainsvc.ResultKeyCurrentRegistrar] = result.CurrentRegistrar
		resultData[domainsvc.ResultKeyDomainLocked] = strconv.FormatBool(result.Locked)
		resultData[domainsvc.ResultKeyTransferInStatus] = result.TransferInStatus
		resultData[domainsvc.ResultKeyDomainAutoRenew] = strconv.FormatBool(result.AutoRenew)
		resultData[domainsvc.ResultKeyDomainPrivacy] = strconv.FormatBool(result.Privacy)

		if result.RegistryStatuses != "" {
			resultData[domainsvc.ResultKeyRegistryStatuses] = result.RegistryStatuses
		}
		if !result.ExpiresAt.IsZero() {
			resultData[domainsvc.ResultKeyDomainExpiresAt] = result.ExpiresAt.Format(time.RFC3339)
		}
		if !result.CreatedAt.IsZero() {
			resultData[domainsvc.ResultKeyDomainCreatedAt] = result.CreatedAt.Format(time.RFC3339)
		}
	}

	// Update CloudflareID with domain ID (with conflict retry)
	if result != nil && result.DomainID != "" {
		if err := common.UpdateCloudflareID(ctx, r.Client, syncState, result.DomainID); err != nil {
			return fmt.Errorf("update syncstate CloudflareID: %w", err)
		}
	}

	syncResult := &common.SyncResult{
		ConfigHash: "", // Domain registration doesn't use config hash
	}

	// Update status with result data (with conflict retry built into UpdateSyncStatus)
	syncState.Status.ResultData = resultData
	if err := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusSynced, syncResult, nil); err != nil {
		return fmt.Errorf("update syncstate status: %w", err)
	}

	return nil
}

// handleDeletion handles the deletion of DomainRegistration SyncState.
// Note: Domain registrations cannot be deleted from Cloudflare - they can only be transferred.
// This method only cleans up the SyncState resource.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller cleans up SyncState
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps
func (r *DomainRegistrationController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, DomainRegistrationFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Note: Domain registrations cannot be deleted from Cloudflare.
	// They can only be transferred to another registrar.
	// We only clean up the SyncState resource here.
	logger.Info("DomainRegistration SyncState being deleted - domain registration will be preserved on Cloudflare",
		"domainId", syncState.Spec.CloudflareID)

	// Remove finalizer (with conflict retry)
	if err := common.RemoveFinalizerWithRetry(ctx, r.Client, syncState, DomainRegistrationFinalizerName); err != nil {
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

// isDomainRegistrationSyncState checks if the object is a DomainRegistration SyncState.
func isDomainRegistrationSyncState(obj client.Object) bool {
	ss, ok := obj.(*v1alpha2.CloudflareSyncState)
	return ok && ss.Spec.ResourceType == v1alpha2.SyncResourceDomainRegistration
}

// SetupWithManager sets up the controller with the Manager.
func (r *DomainRegistrationController) SetupWithManager(mgr ctrl.Manager) error {
	// Predicate to only watch DomainRegistration type SyncStates
	domainRegPredicate := predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return isDomainRegistrationSyncState(e.Object) },
		UpdateFunc:  func(e event.UpdateEvent) bool { return isDomainRegistrationSyncState(e.ObjectNew) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return isDomainRegistrationSyncState(e.Object) },
		GenericFunc: func(e event.GenericEvent) bool { return isDomainRegistrationSyncState(e.Object) },
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("domainregistration-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(domainRegPredicate).
		Complete(r)
}
