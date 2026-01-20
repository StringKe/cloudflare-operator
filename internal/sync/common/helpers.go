// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package common

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

// OperatorNamespace is the namespace where the operator is deployed.
// This is used for storing cluster-scoped resource secrets.
const OperatorNamespace = "cloudflare-operator-system"

// PendingIDPrefix is the prefix used for Cloudflare IDs that haven't been created yet.
// When a SyncState is first created, the CloudflareID is set to "pending-<resource-name>"
// to indicate that the resource needs to be created in Cloudflare.
const PendingIDPrefix = "pending-"

// IsPendingID checks if the given Cloudflare ID indicates a resource that hasn't
// been created yet. Resources start with a pending ID and are updated with the
// real Cloudflare ID after successful creation.
func IsPendingID(cloudflareID string) bool {
	return len(cloudflareID) > len(PendingIDPrefix) &&
		strings.HasPrefix(cloudflareID, PendingIDPrefix)
}

// GeneratePendingID creates a pending ID for a new resource.
// The resource name is appended to distinguish between multiple pending resources.
func GeneratePendingID(resourceName string) string {
	return PendingIDPrefix + resourceName
}

// ExtractFirstSourceConfig extracts configuration from the first source in a SyncState.
// This is the standard pattern for 1:1 mapping resources (DNS, Gateway, Device, etc.)
// where each Kubernetes resource maps to exactly one Cloudflare resource.
//
// Returns an error if:
// - No sources are present
// - JSON unmarshaling fails
// - Configuration is empty/nil
func ExtractFirstSourceConfig[T any](syncState *v1alpha2.CloudflareSyncState) (*T, error) {
	if len(syncState.Spec.Sources) == 0 {
		return nil, errors.New("no sources in SyncState")
	}

	// 1:1 mapping resources use the first source
	source := syncState.Spec.Sources[0]

	config, err := ParseSourceConfig[T](&source)
	if err != nil {
		return nil, fmt.Errorf("parse config from source %s: %w", source.Ref.String(), err)
	}

	if config == nil {
		return nil, fmt.Errorf("empty config from source %s", source.Ref.String())
	}

	return config, nil
}

// CreateAPIClient creates a Cloudflare API client from the credentials reference
// in a SyncState. This is the standard pattern used by all Sync Controllers.
func CreateAPIClient(
	ctx context.Context,
	c client.Client,
	syncState *v1alpha2.CloudflareSyncState,
) (*cf.API, error) {
	if syncState.Spec.CredentialsRef.Name == "" {
		return nil, errors.New("credentials reference name is empty")
	}

	credRef := &v1alpha2.CloudflareCredentialsRef{
		Name: syncState.Spec.CredentialsRef.Name,
	}

	apiClient, err := cf.NewAPIClientFromCredentialsRef(ctx, c, credRef)
	if err != nil {
		return nil, fmt.Errorf("create API client from credentials %s: %w",
			credRef.Name, err)
	}

	return apiClient, nil
}

// UpdateCloudflareID updates the CloudflareID in the SyncState spec.
// This is called after successfully creating a resource in Cloudflare to record
// the actual ID. Returns an error if the update fails - callers should handle
// this as a fatal error to ensure the CloudflareID is persisted before
// updating the sync status.
func UpdateCloudflareID(
	ctx context.Context,
	c client.Client,
	syncState *v1alpha2.CloudflareSyncState,
	newID string,
) error {
	logger := log.FromContext(ctx)

	syncState.Spec.CloudflareID = newID
	if err := c.Update(ctx, syncState); err != nil {
		logger.Error(err, "Failed to update SyncState with Cloudflare ID",
			"name", syncState.Name,
			"cloudflareId", newID)
		return fmt.Errorf("update SyncState CloudflareID to %s: %w", newID, err)
	}

	logger.V(1).Info("Updated SyncState CloudflareID",
		"name", syncState.Name,
		"cloudflareId", newID)
	return nil
}

// RequireAccountID validates that the SyncState has an AccountID specified.
// This is required for account-scoped resources (Gateway, Device, Access, etc.).
func RequireAccountID(syncState *v1alpha2.CloudflareSyncState) (string, error) {
	if syncState.Spec.AccountID == "" {
		return "", errors.New("account ID not specified in SyncState")
	}
	return syncState.Spec.AccountID, nil
}

// RequireZoneID validates that the SyncState has a ZoneID specified.
// This is required for zone-scoped resources (DNS, Ruleset, etc.).
func RequireZoneID(syncState *v1alpha2.CloudflareSyncState) (string, error) {
	if syncState.Spec.ZoneID == "" {
		return "", errors.New("zone ID not specified in SyncState")
	}
	return syncState.Spec.ZoneID, nil
}

// HandleNotFoundOnUpdate is a helper for the common pattern where an update fails
// because the resource was deleted externally. It returns true if the error
// indicates the resource was not found, allowing the caller to recreate it.
func HandleNotFoundOnUpdate(err error) bool {
	return cf.IsNotFoundError(err)
}
