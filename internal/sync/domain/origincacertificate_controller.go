// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	// OriginCACertificateFinalizerName is the finalizer for OriginCACertificate SyncState resources.
	OriginCACertificateFinalizerName = "origincacertificate.sync.cloudflare-operator.io/finalizer"
)

// OriginCACertificateController is the Sync Controller for OriginCACertificate operations.
// It watches CloudflareSyncState resources of type OriginCACertificate and
// performs the actual Cloudflare API calls for certificate creation and revocation.
type OriginCACertificateController struct {
	*common.BaseSyncController
}

// NewOriginCACertificateController creates a new OriginCACertificateSyncController.
func NewOriginCACertificateController(c client.Client) *OriginCACertificateController {
	return &OriginCACertificateController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for OriginCACertificate operations.
//
//nolint:revive // cognitive complexity is acceptable for this reconciliation loop
func (r *OriginCACertificateController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "OriginCACertificateSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState resource
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Verify this is an OriginCACertificate type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourceOriginCACertificate {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing OriginCACertificate SyncState",
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
	if !controllerutil.ContainsFinalizer(syncState, OriginCACertificateFinalizerName) {
		if err := common.AddFinalizerWithRetry(ctx, r.Client, syncState, OriginCACertificateFinalizerName); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip if already synced (for lifecycle operations that are one-time)
	if syncState.Status.SyncStatus == v1alpha2.SyncStatusSynced {
		logger.V(1).Info("Certificate operation already completed, skipping")
		return ctrl.Result{}, nil
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

	logger.Info("Processing OriginCACertificate operation",
		"action", config.Action,
		"hostnames", config.Hostnames,
		"certificateId", config.CertificateID)

	// Create Cloudflare API client
	cfAPI, err := r.createAPIClient(ctx, syncState)
	if err != nil {
		return r.handleError(ctx, syncState, fmt.Errorf("create API client: %w", err))
	}

	// Execute lifecycle operation
	var result *domainsvc.OriginCACertificateSyncResult
	switch config.Action {
	case domainsvc.OriginCACertificateActionCreate:
		result, err = r.createCertificate(ctx, cfAPI, config)
	case domainsvc.OriginCACertificateActionRevoke:
		err = r.revokeCertificate(ctx, cfAPI, config)
		if err == nil {
			// For revoke, we don't have a result to store
			result = &domainsvc.OriginCACertificateSyncResult{
				CertificateID: config.CertificateID,
			}
		}
	case domainsvc.OriginCACertificateActionRenew:
		result, err = r.renewCertificate(ctx, cfAPI, config)
	default:
		err = fmt.Errorf("unknown certificate action: %s", config.Action)
	}

	if err != nil {
		return r.handleError(ctx, syncState, fmt.Errorf("%s certificate: %w", config.Action, err))
	}

	// Update status with success
	if err := r.updateSuccessStatus(ctx, syncState, result); err != nil {
		logger.Error(err, "Failed to update success status")
		return ctrl.Result{}, err
	}

	logger.Info("OriginCACertificate operation completed successfully",
		"action", config.Action,
		"certificateId", result.CertificateID)

	return ctrl.Result{}, nil
}

// getLifecycleConfig extracts the lifecycle configuration from SyncState sources.
func (*OriginCACertificateController) getLifecycleConfig(
	syncState *v1alpha2.CloudflareSyncState,
) (*domainsvc.OriginCACertificateLifecycleConfig, error) {
	if len(syncState.Spec.Sources) == 0 {
		return nil, errors.New("no sources found in syncstate")
	}

	// Use the highest priority source (lowest priority number)
	source := syncState.Spec.Sources[0]

	var config domainsvc.OriginCACertificateLifecycleConfig
	if err := json.Unmarshal(source.Config.Raw, &config); err != nil {
		return nil, fmt.Errorf("unmarshal lifecycle config: %w", err)
	}
	return &config, nil
}

// createAPIClient creates a Cloudflare API client from the SyncState credentials.
func (r *OriginCACertificateController) createAPIClient(ctx context.Context, syncState *v1alpha2.CloudflareSyncState) (*cf.API, error) {
	credRef := &v1alpha2.CloudflareCredentialsRef{
		Name: syncState.Spec.CredentialsRef.Name,
	}
	return cf.NewAPIClientFromCredentialsRef(ctx, r.Client, credRef)
}

// createCertificate creates a new Origin CA certificate via Cloudflare API.
func (*OriginCACertificateController) createCertificate(
	ctx context.Context,
	cfAPI *cf.API,
	config *domainsvc.OriginCACertificateLifecycleConfig,
) (*domainsvc.OriginCACertificateSyncResult, error) {
	logger := log.FromContext(ctx)

	logger.Info("Creating Origin CA certificate", "hostnames", config.Hostnames)

	// Set defaults
	requestType := config.RequestType
	if requestType == "" {
		requestType = "origin-rsa"
	}
	validityDays := config.ValidityDays
	if validityDays == 0 {
		validityDays = 5475 // Default 15 years
	}

	// Create certificate
	cert, err := cfAPI.CreateOriginCACertificate(ctx, cf.OriginCACertificateParams{
		Hostnames:       config.Hostnames,
		RequestType:     requestType,
		RequestValidity: validityDays,
		CSR:             config.CSR,
	})
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	logger.Info("Origin CA certificate created successfully",
		"certificateId", cert.ID,
		"expiresOn", cert.ExpiresOn)

	return &domainsvc.OriginCACertificateSyncResult{
		CertificateID: cert.ID,
		Certificate:   cert.Certificate,
	}, nil
}

// revokeCertificate revokes an existing Origin CA certificate via Cloudflare API.
func (*OriginCACertificateController) revokeCertificate(
	ctx context.Context,
	cfAPI *cf.API,
	config *domainsvc.OriginCACertificateLifecycleConfig,
) error {
	logger := log.FromContext(ctx)

	if config.CertificateID == "" {
		logger.Info("No certificate ID to revoke, skipping")
		return nil
	}

	logger.Info("Revoking Origin CA certificate", "certificateId", config.CertificateID)

	if err := cfAPI.RevokeOriginCACertificate(ctx, config.CertificateID); err != nil {
		if cf.IsNotFoundError(err) {
			logger.Info("Certificate already revoked or deleted", "certificateId", config.CertificateID)
			return nil
		}
		return fmt.Errorf("revoke certificate: %w", err)
	}

	logger.Info("Origin CA certificate revoked successfully", "certificateId", config.CertificateID)
	return nil
}

// renewCertificate renews an existing Origin CA certificate.
// This revokes the old certificate and creates a new one.
func (r *OriginCACertificateController) renewCertificate(
	ctx context.Context,
	cfAPI *cf.API,
	config *domainsvc.OriginCACertificateLifecycleConfig,
) (*domainsvc.OriginCACertificateSyncResult, error) {
	logger := log.FromContext(ctx)

	logger.Info("Renewing Origin CA certificate",
		"oldCertificateId", config.CertificateID,
		"hostnames", config.Hostnames)

	// Revoke old certificate first
	if config.CertificateID != "" {
		if err := cfAPI.RevokeOriginCACertificate(ctx, config.CertificateID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to revoke old certificate, continuing with renewal",
					"certificateId", config.CertificateID)
			}
		}
	}

	// Create new certificate
	return r.createCertificate(ctx, cfAPI, config)
}

// handleError updates the SyncState status with an error and returns appropriate result.
func (r *OriginCACertificateController) handleError(
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
func (r *OriginCACertificateController) updateSuccessStatus(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	result *domainsvc.OriginCACertificateSyncResult,
) error {
	// Build result data map
	resultData := make(map[string]string)
	if result != nil {
		resultData[domainsvc.ResultKeyOriginCACertificateID] = result.CertificateID
		if result.Certificate != "" {
			resultData[domainsvc.ResultKeyOriginCACertificate] = result.Certificate
		}
		if result.ExpiresAt != nil {
			resultData[domainsvc.ResultKeyOriginCAExpiresAt] = result.ExpiresAt.Format(time.RFC3339)
		}
	}

	// Update CloudflareID with actual certificate ID (with conflict retry)
	if result != nil && result.CertificateID != "" {
		if err := common.UpdateCloudflareID(ctx, r.Client, syncState, result.CertificateID); err != nil {
			return fmt.Errorf("update syncstate CloudflareID: %w", err)
		}
	}

	syncResult := &common.SyncResult{
		ConfigHash: "", // Lifecycle operations don't use config hash
	}

	// Update status with result data (with conflict retry built into UpdateSyncStatus)
	syncState.Status.ResultData = resultData
	if err := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusSynced, syncResult, nil); err != nil {
		return fmt.Errorf("update syncstate status: %w", err)
	}

	return nil
}

// handleDeletion handles the deletion of OriginCACertificate from Cloudflare.
// This revokes the certificate when the SyncState is deleted.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller revokes from Cloudflare
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps
func (r *OriginCACertificateController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, OriginCACertificateFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Revoke certificate from Cloudflare
	certificateID := syncState.Spec.CloudflareID
	if certificateID != "" && !common.IsPendingID(certificateID) {
		cfAPI, err := r.createAPIClient(ctx, syncState)
		if err != nil {
			logger.Error(err, "Failed to create API client for deletion")
			return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
		}

		logger.Info("Revoking Origin CA certificate from Cloudflare",
			"certificateId", certificateID)

		if err := cfAPI.RevokeOriginCACertificate(ctx, certificateID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to revoke certificate")
				if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
					logger.Error(statusErr, "Failed to update error status")
				}
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}
			logger.Info("Certificate already revoked or not found")
		} else {
			logger.Info("Successfully revoked Origin CA certificate from Cloudflare")
		}
	}

	// Remove finalizer (with conflict retry)
	if err := common.RemoveFinalizerWithRetry(ctx, r.Client, syncState, OriginCACertificateFinalizerName); err != nil {
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

// isOriginCACertificateSyncState checks if the object is an OriginCACertificate SyncState.
func isOriginCACertificateSyncState(obj client.Object) bool {
	ss, ok := obj.(*v1alpha2.CloudflareSyncState)
	return ok && ss.Spec.ResourceType == v1alpha2.SyncResourceOriginCACertificate
}

// SetupWithManager sets up the controller with the Manager.
func (r *OriginCACertificateController) SetupWithManager(mgr ctrl.Manager) error {
	// Predicate to only watch OriginCACertificate type SyncStates
	certPredicate := predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return isOriginCACertificateSyncState(e.Object) },
		UpdateFunc:  func(e event.UpdateEvent) bool { return isOriginCACertificateSyncState(e.ObjectNew) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return isOriginCACertificateSyncState(e.Object) },
		GenericFunc: func(e event.GenericEvent) bool { return isOriginCACertificateSyncState(e.Object) },
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("origincacertificate-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(certPredicate).
		Complete(r)
}

// ParseLifecycleConfig parses the lifecycle configuration from raw JSON.
func ParseLifecycleConfig(raw []byte) (*domainsvc.OriginCACertificateLifecycleConfig, error) {
	var config domainsvc.OriginCACertificateLifecycleConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, fmt.Errorf("unmarshal lifecycle config: %w", err)
	}
	return &config, nil
}

// GetResultFromSyncState extracts the certificate result from SyncState ResultData.
func GetResultFromSyncState(syncState *v1alpha2.CloudflareSyncState) *domainsvc.OriginCACertificateSyncResult {
	if syncState == nil || syncState.Status.ResultData == nil {
		return nil
	}

	result := &domainsvc.OriginCACertificateSyncResult{
		CertificateID: syncState.Status.ResultData[domainsvc.ResultKeyOriginCACertificateID],
		Certificate:   syncState.Status.ResultData[domainsvc.ResultKeyOriginCACertificate],
	}

	// Parse hostnames if present
	if hostnamesStr, ok := syncState.Status.ResultData[domainsvc.ResultKeyOriginCAHostnames]; ok && hostnamesStr != "" {
		_ = strings.Split(hostnamesStr, ",") // Available if needed
	}

	return result
}
