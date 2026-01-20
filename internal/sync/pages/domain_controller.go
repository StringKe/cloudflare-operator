// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pages

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/service"
	dnssvc "github.com/StringKe/cloudflare-operator/internal/service/dns"
	pagessvc "github.com/StringKe/cloudflare-operator/internal/service/pages"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

const (
	// DomainFinalizerName is the finalizer for Pages Domain SyncState resources.
	DomainFinalizerName = "pages-domain.sync.cloudflare-operator.io/finalizer"
)

// DomainSyncController is the Sync Controller for Pages Domain Configuration.
// It watches CloudflareSyncState resources of type PagesDomain,
// extracts the configuration, and syncs to Cloudflare API.
// This is the SINGLE point that calls Cloudflare Pages Domain API.
type DomainSyncController struct {
	*common.BaseSyncController
}

// NewDomainSyncController creates a new DomainSyncController
func NewDomainSyncController(c client.Client) *DomainSyncController {
	return &DomainSyncController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for Pages domain.
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *DomainSyncController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "PagesDomainSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process PagesDomain type
	if syncState.Spec.ResourceType != v1alpha2.SyncResourcePagesDomain {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing Pages Domain SyncState",
		"cloudflareId", syncState.Spec.CloudflareID,
		"sources", len(syncState.Spec.Sources))

	// Handle deletion
	if !syncState.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, syncState)
	}

	// Check if there are any sources - if none, delete from Cloudflare
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources in SyncState, deleting from Cloudflare")
		return r.handleDeletion(ctx, syncState)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(syncState, DomainFinalizerName) {
		if err := common.AddFinalizerWithRetry(ctx, r.Client, syncState, DomainFinalizerName); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip if there's a pending debounced request
	if r.Debouncer.IsPending(req.Name) {
		logger.V(1).Info("Skipping reconcile - debounced request pending")
		return ctrl.Result{}, nil
	}

	// Extract Pages domain configuration from first source
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract Pages domain configuration")
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
	if err := r.syncToCloudflare(ctx, syncState, config); err != nil {
		logger.Error(err, "Failed to sync Pages domain to Cloudflare")
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

	logger.Info("Successfully synced Pages domain to Cloudflare",
		"domain", config.Domain,
		"projectName", config.ProjectName)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts Pages domain configuration from SyncState sources.
func (*DomainSyncController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*pagessvc.PagesDomainConfig, error) {
	return common.ExtractFirstSourceConfig[pagessvc.PagesDomainConfig](syncState)
}

// syncToCloudflare syncs the Pages domain configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *DomainSyncController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *pagessvc.PagesDomainConfig,
) error {
	logger := log.FromContext(ctx)

	// Create API client using common helper
	apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
	if err != nil {
		return err
	}

	// Check if this is a new domain or existing
	cloudflareID := syncState.Spec.CloudflareID

	// Determine DNS configuration mode
	autoConfigureDNS := config.AutoConfigureDNS == nil || *config.AutoConfigureDNS

	if common.IsPendingID(cloudflareID) {
		// Add new domain
		logger.Info("Adding new Pages domain",
			"domain", config.Domain,
			"projectName", config.ProjectName,
			"autoConfigureDNS", autoConfigureDNS)

		result, err := apiClient.AddPagesDomain(ctx, config.ProjectName, config.Domain)
		if err != nil {
			// Check if domain already exists - adopt it
			if cf.IsConflictError(err) {
				logger.Info("Pages domain already exists, attempting to adopt",
					"domain", config.Domain,
					"projectName", config.ProjectName)

				existingDomain, getErr := apiClient.GetPagesDomain(ctx, config.ProjectName, config.Domain)
				if getErr != nil {
					return fmt.Errorf("domain exists but failed to get for adoption: %w", getErr)
				}

				// Adopt the existing domain by updating CloudflareID
				if err := common.UpdateCloudflareID(ctx, r.Client, syncState, existingDomain.ID); err != nil {
					return err
				}

				logger.Info("Adopted existing Pages domain",
					"domainId", existingDomain.ID,
					"domain", existingDomain.Name,
					"status", existingDomain.Status)

				// Configure DNS if autoConfigureDNS is enabled
				if autoConfigureDNS {
					if err := r.configureDNS(ctx, syncState, config); err != nil {
						logger.Error(err, "Failed to configure DNS for adopted domain")
						// Non-fatal, domain was added successfully
					}
				}
				return nil
			}
			return fmt.Errorf("add Pages domain: %w", err)
		}

		// Update SyncState with actual domain ID (must succeed)
		if err := common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID); err != nil {
			return err
		}

		logger.Info("Added Pages domain",
			"domainId", result.ID,
			"domain", result.Name,
			"status", result.Status)

		// Configure DNS if autoConfigureDNS is enabled
		if autoConfigureDNS {
			if err := r.configureDNS(ctx, syncState, config); err != nil {
				logger.Error(err, "Failed to configure DNS for new domain")
				// Non-fatal, domain was added successfully
			}
		}
	} else {
		// Check if domain exists (domains can only be added/deleted, not updated)
		logger.Info("Verifying Pages domain exists",
			"domain", config.Domain,
			"projectName", config.ProjectName)

		_, err := apiClient.GetPagesDomain(ctx, config.ProjectName, config.Domain)
		if err != nil {
			if cf.IsNotFoundError(err) {
				// Domain was deleted externally, recreate it
				logger.Info("Pages domain not found, recreating",
					"domain", config.Domain)
				result, addErr := apiClient.AddPagesDomain(ctx, config.ProjectName, config.Domain)
				if addErr != nil {
					// Handle race condition where domain was recreated externally
					if cf.IsConflictError(addErr) {
						logger.Info("Pages domain was recreated externally, re-verifying")
						existingDomain, getErr := apiClient.GetPagesDomain(ctx, config.ProjectName, config.Domain)
						if getErr != nil {
							return fmt.Errorf("failed to get recreated domain: %w", getErr)
						}
						if updateErr := common.UpdateCloudflareID(ctx, r.Client, syncState, existingDomain.ID); updateErr != nil {
							logger.Error(updateErr, "Failed to update CloudflareID after external recreation")
						}
						// Configure DNS for externally recreated domain
						if autoConfigureDNS {
							if err := r.configureDNS(ctx, syncState, config); err != nil {
								logger.Error(err, "Failed to configure DNS for recreated domain")
							}
						}
						return nil
					}
					return fmt.Errorf("recreate Pages domain: %w", addErr)
				}

				// Update SyncState with new domain ID
				if updateErr := common.UpdateCloudflareID(ctx, r.Client, syncState, result.ID); updateErr != nil {
					logger.Error(updateErr, "Failed to update CloudflareID after recreating")
				}

				// Configure DNS for recreated domain
				if autoConfigureDNS {
					if err := r.configureDNS(ctx, syncState, config); err != nil {
						logger.Error(err, "Failed to configure DNS for recreated domain")
					}
				}
			} else {
				return fmt.Errorf("get Pages domain: %w", err)
			}
		} else {
			// Domain exists - ensure DNS is configured (retry if previous attempt failed)
			if autoConfigureDNS {
				if err := r.ensureDNSConfigured(ctx, syncState, config); err != nil {
					logger.Error(err, "Failed to ensure DNS configured for existing domain")
					// Non-fatal, domain exists
				}
			}
		}

		logger.Info("Pages domain verified",
			"domain", config.Domain)
	}

	return nil
}

// handleDeletion handles the deletion of Pages domain from Cloudflare.
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps
func (r *DomainSyncController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, DomainFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Get the Cloudflare domain ID
	cloudflareID := syncState.Spec.CloudflareID

	// Skip if pending ID (domain was never created)
	if common.IsPendingID(cloudflareID) {
		logger.Info("Skipping deletion - Pages domain was never created",
			"cloudflareId", cloudflareID)
	} else if cloudflareID != "" {
		// Extract config to get project name and domain
		config, err := r.extractConfig(syncState)
		if err != nil {
			logger.Error(err, "Failed to extract config for deletion")
			// Continue to remove finalizer even if we can't extract config
		} else {
			// Cleanup DNS record first (if autoConfigureDNS was enabled)
			r.cleanupDNS(ctx, syncState, config)

			// Create API client
			apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
			if err != nil {
				logger.Error(err, "Failed to create API client for deletion")
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}

			logger.Info("Deleting Pages domain from Cloudflare",
				"domain", config.Domain,
				"projectName", config.ProjectName)

			if err := apiClient.DeletePagesDomain(ctx, config.ProjectName, config.Domain); err != nil {
				if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to delete Pages domain from Cloudflare")
					if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
						logger.Error(statusErr, "Failed to update error status")
					}
					return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
				}
				logger.Info("Pages domain already deleted from Cloudflare")
			} else {
				logger.Info("Successfully deleted Pages domain from Cloudflare",
					"domain", config.Domain)
			}
		}
	}

	// Remove finalizer
	if err := common.RemoveFinalizerWithRetry(ctx, r.Client, syncState, DomainFinalizerName); err != nil {
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

// ensureDNSConfigured checks if DNS is already configured, and creates it if not.
// This is used to retry DNS configuration for existing domains.
func (r *DomainSyncController) ensureDNSConfigured(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *pagessvc.PagesDomainConfig,
) error {
	logger := log.FromContext(ctx)

	// Check if we have Zone ID for DNS configuration
	zoneID := syncState.Spec.ZoneID
	if zoneID == "" {
		logger.V(1).Info("No Zone ID available, skipping DNS check",
			"domain", config.Domain)
		return nil
	}

	// Build source name (must match what's used in configureDNS)
	sourceName := fmt.Sprintf("pagesdomain-%s", sanitizeDomainForSource(config.Domain))

	// Check if DNS SyncState already exists
	svc := dnssvc.NewService(r.Client)
	syncStatus, err := svc.GetSyncStatus(ctx, service.Source{
		Kind:      "PagesDomainAutomatic",
		Namespace: syncState.Namespace,
		Name:      sourceName,
	}, "")
	if err != nil {
		logger.V(1).Info("Failed to check DNS sync status", "error", err)
		// Continue to try configuring
	}

	if syncStatus != nil && syncStatus.IsSynced {
		logger.V(1).Info("DNS already configured for Pages domain",
			"domain", config.Domain,
			"dnsRecordId", syncStatus.RecordID)
		return nil
	}

	// DNS not configured or sync failed, configure it
	logger.Info("DNS not yet configured for Pages domain, configuring now",
		"domain", config.Domain,
		"zoneId", zoneID)

	return r.configureDNS(ctx, syncState, config)
}

// configureDNS creates a CNAME DNS record pointing to the Pages project.
func (r *DomainSyncController) configureDNS(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *pagessvc.PagesDomainConfig,
) error {
	logger := log.FromContext(ctx)

	// Check if we have Zone ID for DNS configuration
	zoneID := syncState.Spec.ZoneID
	if zoneID == "" {
		logger.Info("No Zone ID available, skipping automatic DNS configuration",
			"domain", config.Domain,
			"hint", "DNS must be configured manually or zone is not managed by this account")
		return nil
	}

	// Build CNAME target: {project-name}.pages.dev
	cnameTarget := fmt.Sprintf("%s.pages.dev", config.ProjectName)

	// Create DNS record config
	dnsConfig := dnssvc.DNSRecordConfig{
		Name:    config.Domain,
		Type:    "CNAME",
		Content: cnameTarget,
		TTL:     1, // Auto
		Proxied: true,
		Comment: fmt.Sprintf("Managed by cloudflare-operator PagesDomain: %s", config.Domain),
	}

	// Build source for DNS record
	source := service.Source{
		Kind:      "PagesDomainAutomatic",
		Namespace: syncState.Namespace,
		Name:      fmt.Sprintf("pagesdomain-%s", sanitizeDomainForSource(config.Domain)),
	}

	// Register DNS record via DNS Service
	svc := dnssvc.NewService(r.Client)
	if err := svc.Register(ctx, dnssvc.RegisterOptions{
		ZoneID:         zoneID,
		AccountID:      syncState.Spec.AccountID,
		Source:         source,
		Config:         dnsConfig,
		CredentialsRef: syncState.Spec.CredentialsRef,
	}); err != nil {
		return fmt.Errorf("register DNS record: %w", err)
	}

	logger.Info("DNS record registered for Pages domain",
		"domain", config.Domain,
		"cname", cnameTarget,
		"zoneId", zoneID)

	return nil
}

// cleanupDNS removes the automatically created DNS record for a Pages domain.
func (r *DomainSyncController) cleanupDNS(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *pagessvc.PagesDomainConfig,
) {
	logger := log.FromContext(ctx)

	// Check if autoConfigureDNS was enabled
	autoConfigureDNS := config.AutoConfigureDNS == nil || *config.AutoConfigureDNS
	if !autoConfigureDNS {
		return
	}

	// Check if we have Zone ID
	zoneID := syncState.Spec.ZoneID
	if zoneID == "" {
		return
	}

	// Build source for DNS record (must match what was used in configureDNS)
	source := service.Source{
		Kind:      "PagesDomainAutomatic",
		Namespace: syncState.Namespace,
		Name:      fmt.Sprintf("pagesdomain-%s", sanitizeDomainForSource(config.Domain)),
	}

	// Unregister DNS record via DNS Service
	svc := dnssvc.NewService(r.Client)
	if err := svc.Unregister(ctx, "", source); err != nil {
		logger.Error(err, "Failed to unregister DNS record", "domain", config.Domain)
		// Non-fatal, continue with domain deletion
	} else {
		logger.Info("DNS record unregistered for Pages domain", "domain", config.Domain)
	}
}

// sanitizeDomainForSource converts a domain name to a valid Kubernetes resource name part.
func sanitizeDomainForSource(domain string) string {
	// Replace dots with dashes for Kubernetes compatibility
	result := ""
	for _, c := range domain {
		if c == '.' {
			result += "-"
		} else {
			result += string(c)
		}
	}
	return result
}

// SetupWithManager sets up the controller with the Manager.
func (r *DomainSyncController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("pages-domain-sync").
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(v1alpha2.SyncResourcePagesDomain)).
		Complete(r)
}
