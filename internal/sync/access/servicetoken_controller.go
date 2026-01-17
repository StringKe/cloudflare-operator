// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package access

//nolint:dupl // Similar patterns across resource types are intentional

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	accesssvc "github.com/StringKe/cloudflare-operator/internal/service/access"
	"github.com/StringKe/cloudflare-operator/internal/sync/common"
)

const (
	// ServiceTokenFinalizerName is the finalizer for AccessServiceToken SyncState resources.
	ServiceTokenFinalizerName = "accessservicetoken.sync.cloudflare-operator.io/finalizer"
)

// ServiceTokenController is the Sync Controller for AccessServiceToken Configuration.
type ServiceTokenController struct {
	*common.BaseSyncController
}

// NewServiceTokenController creates a new AccessServiceToken sync controller.
func NewServiceTokenController(c client.Client) *ServiceTokenController {
	return &ServiceTokenController{
		BaseSyncController: common.NewBaseSyncController(c),
	}
}

// Reconcile processes a CloudflareSyncState resource for AccessServiceToken.
//
//nolint:revive // cognitive complexity is acceptable for this central reconciliation loop
func (r *ServiceTokenController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("controller", "AccessServiceTokenSync", "name", req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Get the SyncState
	syncState, err := r.GetSyncState(ctx, req.Name)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Only process AccessServiceToken type
	if syncState.Spec.ResourceType != accesssvc.ResourceTypeAccessServiceToken {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Processing AccessServiceToken SyncState",
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

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(syncState, ServiceTokenFinalizerName) {
		controllerutil.AddFinalizer(syncState, ServiceTokenFinalizerName)
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

	// Extract AccessServiceToken configuration from first source
	config, err := r.extractConfig(syncState)
	if err != nil {
		logger.Error(err, "Failed to extract AccessServiceToken configuration")
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
		logger.V(1).Info("Configuration unchanged, skipping sync", "hash", newHash)
		return ctrl.Result{}, nil
	}

	// Set syncing status
	if err := r.SetSyncStatus(ctx, syncState, v1alpha2.SyncStatusSyncing); err != nil {
		return ctrl.Result{}, err
	}

	// Sync to Cloudflare API
	result, err := r.syncToCloudflare(ctx, syncState, config)
	if err != nil {
		logger.Error(err, "Failed to sync AccessServiceToken to Cloudflare")
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

	logger.Info("Successfully synced AccessServiceToken to Cloudflare",
		"tokenId", result.ID,
		"name", config.Name)

	return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}

// extractConfig extracts AccessServiceToken configuration from SyncState sources.
// AccessServiceToken has 1:1 mapping, so we use the ExtractFirstSourceConfig helper.
func (*ServiceTokenController) extractConfig(syncState *v1alpha2.CloudflareSyncState) (*accesssvc.AccessServiceTokenConfig, error) {
	return common.ExtractFirstSourceConfig[accesssvc.AccessServiceTokenConfig](syncState)
}

// syncToCloudflare syncs the AccessServiceToken configuration to Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for API sync logic
func (r *ServiceTokenController) syncToCloudflare(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	config *accesssvc.AccessServiceTokenConfig,
) (*accesssvc.AccessServiceTokenSyncResult, error) {
	logger := log.FromContext(ctx)

	// Create API client using common helper
	apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
	if err != nil {
		return nil, err
	}

	// Validate and set account ID
	accountID, err := common.RequireAccountID(syncState)
	if err != nil {
		return nil, err
	}
	apiClient.ValidAccountId = accountID

	// Get duration with default
	duration := config.Duration
	if duration == "" {
		duration = "8760h" // Default to 1 year
	}

	// Check if this is an existing token or new (pending)
	cloudflareID := syncState.Spec.CloudflareID
	var result *cf.AccessServiceTokenResult

	if common.IsPendingID(cloudflareID) {
		// Try to find existing token by name first (adoption)
		existingToken, findErr := apiClient.GetAccessServiceTokenByName(config.Name)
		if findErr == nil && existingToken != nil {
			// Found existing token - adopt it
			logger.Info("Found existing AccessServiceToken, adopting",
				"tokenId", existingToken.TokenID,
				"name", config.Name)

			result, err = apiClient.UpdateAccessServiceToken(existingToken.TokenID, config.Name, duration)
			if err != nil {
				return nil, fmt.Errorf("adopt AccessServiceToken: %w", err)
			}
		} else {
			// Create new token
			logger.Info("Creating new AccessServiceToken",
				"name", config.Name)

			result, err = apiClient.CreateAccessServiceToken(config.Name, duration)
			if err != nil {
				return nil, fmt.Errorf("create AccessServiceToken: %w", err)
			}
		}

		// Update SyncState CloudflareID with the actual token ID
		common.UpdateCloudflareID(ctx, r.Client, syncState, result.TokenID)

		logger.Info("Created/Adopted AccessServiceToken",
			"tokenId", result.TokenID,
			"clientId", result.ClientID)
	} else {
		// Update existing token
		logger.Info("Updating AccessServiceToken",
			"tokenId", cloudflareID,
			"name", config.Name)

		result, err = apiClient.UpdateAccessServiceToken(cloudflareID, config.Name, duration)
		if err != nil {
			// Check if token was deleted externally
			if common.HandleNotFoundOnUpdate(err) {
				logger.Info("AccessServiceToken not found, recreating",
					"tokenId", cloudflareID)
				result, err = apiClient.CreateAccessServiceToken(config.Name, duration)
				if err != nil {
					return nil, fmt.Errorf("recreate AccessServiceToken: %w", err)
				}

				// Update SyncState CloudflareID if ID changed
				if result.TokenID != cloudflareID {
					common.UpdateCloudflareID(ctx, r.Client, syncState, result.TokenID)
				}
			} else {
				return nil, fmt.Errorf("update AccessServiceToken: %w", err)
			}
		}

		logger.Info("Updated AccessServiceToken",
			"tokenId", result.TokenID)
	}

	syncResult := &accesssvc.AccessServiceTokenSyncResult{
		SyncResult: accesssvc.SyncResult{
			ID:        result.TokenID,
			AccountID: result.AccountID,
		},
		ClientID:            result.ClientID,
		ClientSecret:        result.ClientSecret,
		ExpiresAt:           result.ExpiresAt,
		CreatedAt:           result.CreatedAt,
		UpdatedAt:           result.UpdatedAt,
		LastSeenAt:          result.LastSeenAt,
		ClientSecretVersion: fmt.Sprintf("%d", result.ClientSecretVersion),
	}

	// Create or update secret if SecretRef is specified and we have ClientSecret
	if config.SecretRef != nil && config.SecretRef.Name != "" && result.ClientSecret != "" {
		if err := r.createOrUpdateSecret(ctx, config.SecretRef, syncResult); err != nil {
			// Critical: Secret creation failed but token was created
			// We should warn strongly because the secret cannot be recovered
			logger.Error(err, "CRITICAL: Failed to create secret for token. ClientSecret cannot be recovered!")
			return nil, fmt.Errorf("create secret: %w (ClientSecret is lost)", err)
		}
		logger.Info("Created/Updated secret with service token credentials",
			"secret", config.SecretRef.Name,
			"namespace", config.SecretRef.Namespace)
	}

	return syncResult, nil
}

// createOrUpdateSecret creates or updates the K8s secret with token credentials.
func (r *ServiceTokenController) createOrUpdateSecret(
	ctx context.Context,
	secretRef *accesssvc.SecretReference,
	result *accesssvc.AccessServiceTokenSyncResult,
) error {
	secretName := secretRef.Name
	secretNamespace := secretRef.Namespace
	if secretNamespace == "" {
		secretNamespace = "cloudflare-operator-system" // Default namespace for cluster-scoped resources
	}

	secret := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: secretNamespace}, secret)

	if apierrors.IsNotFound(err) {
		// Create new secret
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: secretNamespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "cloudflare-operator",
				},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"CF_ACCESS_CLIENT_ID":     []byte(result.ClientID),
				"CF_ACCESS_CLIENT_SECRET": []byte(result.ClientSecret),
			},
		}
		return r.Client.Create(ctx, secret)
	} else if err != nil {
		return err
	}

	// Update existing secret
	secret.Data = map[string][]byte{
		"CF_ACCESS_CLIENT_ID":     []byte(result.ClientID),
		"CF_ACCESS_CLIENT_SECRET": []byte(result.ClientSecret),
	}
	return r.Client.Update(ctx, secret)
}

// handleDeletion handles the deletion of AccessServiceToken from Cloudflare.
// This is the SINGLE point for Cloudflare AccessServiceToken deletion in the system.
// Following Unified Sync Architecture:
// Resource Controller unregisters → SyncState updated → Sync Controller deletes from Cloudflare
//
//nolint:revive // cognitive complexity unavoidable: deletion logic requires multiple cleanup steps and error handling
func (r *ServiceTokenController) handleDeletion(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(syncState, ServiceTokenFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Get the Cloudflare token ID
	cloudflareID := syncState.Spec.CloudflareID

	// Skip if pending ID (token was never created)
	if common.IsPendingID(cloudflareID) {
		logger.Info("Skipping deletion - AccessServiceToken was never created",
			"cloudflareId", cloudflareID)
	} else if cloudflareID != "" {
		// Create API client
		apiClient, err := common.CreateAPIClient(ctx, r.Client, syncState)
		if err != nil {
			logger.Error(err, "Failed to create API client for deletion")
			return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
		}

		// Set account ID
		if syncState.Spec.AccountID != "" {
			apiClient.ValidAccountId = syncState.Spec.AccountID
		}

		logger.Info("Deleting AccessServiceToken from Cloudflare",
			"tokenId", cloudflareID)

		if err := apiClient.DeleteAccessServiceToken(cloudflareID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete AccessServiceToken from Cloudflare")
				if statusErr := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err); statusErr != nil {
					logger.Error(statusErr, "Failed to update error status")
				}
				return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
			}
			logger.Info("AccessServiceToken already deleted from Cloudflare")
		} else {
			logger.Info("Successfully deleted AccessServiceToken from Cloudflare",
				"tokenId", cloudflareID)
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(syncState, ServiceTokenFinalizerName)
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

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceTokenController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.CloudflareSyncState{}).
		WithEventFilter(common.PredicateForResourceType(accesssvc.ResourceTypeAccessServiceToken)).
		Complete(r)
}
