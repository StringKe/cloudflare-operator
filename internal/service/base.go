// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// BaseService provides common functionality for all Core Services.
// It handles CloudflareSyncState CRD operations including:
// - Creating new SyncState resources
// - Adding/updating/removing configuration sources
// - Handling optimistic locking conflicts
type BaseService struct {
	Client client.Client
}

// NewBaseService creates a new BaseService
func NewBaseService(c client.Client) *BaseService {
	return &BaseService{Client: c}
}

// GetOrCreateSyncState retrieves an existing CloudflareSyncState or creates a new one.
// The SyncState is uniquely identified by resourceType and cloudflareID.
//
//nolint:revive // cognitive complexity is acceptable for this initialization function
func (s *BaseService) GetOrCreateSyncState(
	ctx context.Context,
	resourceType v1alpha2.SyncResourceType,
	cloudflareID, accountID, zoneID string,
	credRef v1alpha2.CredentialsReference,
) (*v1alpha2.CloudflareSyncState, error) {
	logger := log.FromContext(ctx)
	name := SyncStateName(resourceType, cloudflareID)

	syncState := &v1alpha2.CloudflareSyncState{}
	err := s.Client.Get(ctx, types.NamespacedName{Name: name}, syncState)

	if err == nil {
		return syncState, nil
	}

	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("get syncstate %s: %w", name, err)
	}

	// Create new SyncState
	logger.Info("Creating new CloudflareSyncState",
		"name", name,
		"resourceType", resourceType,
		"cloudflareId", cloudflareID)

	syncState = &v1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"cloudflare-operator.io/resource-type": string(resourceType),
				"cloudflare-operator.io/cloudflare-id": sanitizeLabelValue(cloudflareID),
			},
		},
		Spec: v1alpha2.CloudflareSyncStateSpec{
			ResourceType:   resourceType,
			CloudflareID:   cloudflareID,
			AccountID:      accountID,
			ZoneID:         zoneID,
			CredentialsRef: credRef,
			Sources:        []v1alpha2.ConfigSource{},
		},
	}

	if err := s.Client.Create(ctx, syncState); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Race condition: another controller created it, fetch and return
			if err := s.Client.Get(ctx, types.NamespacedName{Name: name}, syncState); err != nil {
				return nil, fmt.Errorf("get syncstate after create conflict: %w", err)
			}
			return syncState, nil
		}
		return nil, fmt.Errorf("create syncstate %s: %w", name, err)
	}

	return syncState, nil
}

// UpdateSource adds or updates a source's configuration in the SyncState.
// This uses optimistic locking via resourceVersion to handle concurrent updates.
// On conflict, it re-fetches the resource and re-applies only this source's change,
// preserving changes made by other controllers.
func (s *BaseService) UpdateSource(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	source Source,
	config interface{},
	priority int,
) error {
	logger := log.FromContext(ctx)

	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	sourceRef := source.ToReference()
	sourceStr := sourceRef.String()

	// Define the operation to apply/re-apply on conflict
	applySourceUpdate := func(state *v1alpha2.CloudflareSyncState) {
		now := metav1.Now()
		found := false
		for i := range state.Spec.Sources {
			if state.Spec.Sources[i].Ref.String() == sourceStr {
				state.Spec.Sources[i].Config = runtime.RawExtension{Raw: configJSON}
				state.Spec.Sources[i].Priority = priority
				state.Spec.Sources[i].LastUpdated = now
				found = true
				break
			}
		}
		if !found {
			state.Spec.Sources = append(state.Spec.Sources, v1alpha2.ConfigSource{
				Ref:         sourceRef,
				Config:      runtime.RawExtension{Raw: configJSON},
				Priority:    priority,
				LastUpdated: now,
			})
		}
	}

	// Apply the update to current state
	applySourceUpdate(syncState)
	logger.V(1).Info("Updating source in SyncState",
		"syncState", syncState.Name,
		"source", sourceStr,
		"totalSources", len(syncState.Spec.Sources))

	// Update with conflict retry, re-applying operation on conflict
	return s.updateWithRetryFunc(ctx, syncState, applySourceUpdate)
}

// RemoveSource removes a source from the SyncState.
// If no sources remain after removal, the SyncState is deleted.
// On conflict, it re-fetches the resource and re-applies only this source's removal,
// preserving changes made by other controllers.
//
//nolint:revive // cognitive complexity is acceptable for this cleanup function
func (s *BaseService) RemoveSource(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	source Source,
) error {
	logger := log.FromContext(ctx)
	sourceStr := source.String()

	// Define the operation to apply/re-apply on conflict
	applySourceRemoval := func(state *v1alpha2.CloudflareSyncState) {
		newSources := make([]v1alpha2.ConfigSource, 0, len(state.Spec.Sources))
		for _, src := range state.Spec.Sources {
			if src.Ref.String() != sourceStr {
				newSources = append(newSources, src)
			}
		}
		state.Spec.Sources = newSources
	}

	// Apply the removal to current state
	applySourceRemoval(syncState)

	// If no sources remain, delete the SyncState
	if len(syncState.Spec.Sources) == 0 {
		logger.Info("No sources remaining, deleting CloudflareSyncState",
			"name", syncState.Name)
		if err := s.Client.Delete(ctx, syncState); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("delete syncstate %s: %w", syncState.Name, err)
		}
		return nil
	}

	logger.V(1).Info("Removed source from SyncState",
		"syncState", syncState.Name,
		"source", sourceStr,
		"remainingSources", len(syncState.Spec.Sources))

	// Update with conflict retry, re-applying operation on conflict
	return s.updateWithRetryFunc(ctx, syncState, applySourceRemoval)
}

// GetSyncState retrieves a CloudflareSyncState by resourceType and cloudflareID.
// Returns nil if not found (not an error).
func (s *BaseService) GetSyncState(
	ctx context.Context,
	resourceType v1alpha2.SyncResourceType,
	cloudflareID string,
) (*v1alpha2.CloudflareSyncState, error) {
	name := SyncStateName(resourceType, cloudflareID)
	syncState := &v1alpha2.CloudflareSyncState{}

	if err := s.Client.Get(ctx, types.NamespacedName{Name: name}, syncState); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get syncstate %s: %w", name, err)
	}

	return syncState, nil
}

// updateWithRetryFunc performs an update with automatic conflict retry.
// On conflict, it re-fetches the resource and re-applies the operation function,
// ensuring that changes from other controllers are preserved.
//
//nolint:revive // cognitive complexity is acceptable for retry logic
func (s *BaseService) updateWithRetryFunc(
	ctx context.Context,
	syncState *v1alpha2.CloudflareSyncState,
	applyOp func(*v1alpha2.CloudflareSyncState),
) error {
	const maxRetries = 5

	for i := 0; i < maxRetries; i++ {
		if err := s.Client.Update(ctx, syncState); err != nil {
			if apierrors.IsConflict(err) && i < maxRetries-1 {
				// Re-fetch the latest version
				fresh := &v1alpha2.CloudflareSyncState{}
				if err := s.Client.Get(ctx, types.NamespacedName{Name: syncState.Name}, fresh); err != nil {
					return fmt.Errorf("refetch syncstate on conflict: %w", err)
				}
				// Re-apply the operation to the fresh resource
				// This preserves changes made by other controllers
				applyOp(fresh)
				syncState = fresh
				continue
			}
			return fmt.Errorf("update syncstate %s: %w", syncState.Name, err)
		}
		return nil
	}

	return fmt.Errorf("update syncstate %s: max retries exceeded", syncState.Name)
}

// SyncStateName generates a consistent name for CloudflareSyncState resources.
// Format: {resource-type}-{cloudflare-id}
func SyncStateName(resourceType v1alpha2.SyncResourceType, cloudflareID string) string {
	return fmt.Sprintf("%s-%s", toKebabCase(string(resourceType)), sanitizeName(cloudflareID))
}

// toKebabCase converts CamelCase to kebab-case
//
//nolint:revive // WriteRune error is always nil for valid runes
func toKebabCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			_, _ = result.WriteRune('-')
		}
		_, _ = result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

// sanitizeName ensures the name is valid for Kubernetes resource names
//
//nolint:revive // WriteRune error is always nil for valid runes
func sanitizeName(s string) string {
	// Replace invalid characters with dashes
	var result strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			_, _ = result.WriteRune(r)
		} else {
			_, _ = result.WriteRune('-')
		}
	}

	// Trim leading/trailing dashes and limit length
	name := strings.Trim(result.String(), "-")
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

// sanitizeLabelValue ensures the value is valid for Kubernetes label values.
// Label values must be 63 characters or less and match regex: ^[a-z0-9A-Z]?([a-z0-9A-Z-_.]*[a-z0-9A-Z])?$
//
//nolint:revive // WriteRune error is always nil for valid runes
func sanitizeLabelValue(s string) string {
	// Replace invalid characters with dashes (similar to sanitizeName)
	var result strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			_, _ = result.WriteRune(r)
		} else {
			_, _ = result.WriteRune('-')
		}
	}

	sanitized := result.String()

	// Truncate to 63 characters
	if len(sanitized) > 63 {
		sanitized = sanitized[:63]
	}

	// Trim leading/trailing invalid characters for label values
	return strings.Trim(sanitized, "-_.")
}
