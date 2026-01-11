// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

//nolint:revive // max-public-structs: R2 API requires multiple struct types for configuration
package cf

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
)

// R2BucketParams contains parameters for creating an R2 bucket
type R2BucketParams struct {
	Name         string
	LocationHint string
}

// R2BucketResult contains the result of an R2 bucket operation
type R2BucketResult struct {
	Name         string
	Location     string
	CreationDate time.Time
}

// CreateR2Bucket creates a new R2 bucket
func (api *API) CreateR2Bucket(ctx context.Context, params R2BucketParams) (*R2BucketResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId()
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	rc := cloudflare.AccountIdentifier(accountID)
	bucket, err := api.CloudflareClient.CreateR2Bucket(ctx, rc, cloudflare.CreateR2BucketParameters{
		Name:         params.Name,
		LocationHint: params.LocationHint,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create R2 bucket: %w", err)
	}

	result := &R2BucketResult{
		Name:     bucket.Name,
		Location: bucket.Location,
	}
	if bucket.CreationDate != nil {
		result.CreationDate = *bucket.CreationDate
	}

	return result, nil
}

// GetR2Bucket retrieves an R2 bucket by name
func (api *API) GetR2Bucket(ctx context.Context, bucketName string) (*R2BucketResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId()
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	rc := cloudflare.AccountIdentifier(accountID)
	bucket, err := api.CloudflareClient.GetR2Bucket(ctx, rc, bucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to get R2 bucket: %w", err)
	}

	result := &R2BucketResult{
		Name:     bucket.Name,
		Location: bucket.Location,
	}
	if bucket.CreationDate != nil {
		result.CreationDate = *bucket.CreationDate
	}

	return result, nil
}

// ListR2Buckets lists all R2 buckets
func (api *API) ListR2Buckets(ctx context.Context) ([]R2BucketResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId()
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	rc := cloudflare.AccountIdentifier(accountID)
	buckets, err := api.CloudflareClient.ListR2Buckets(ctx, rc, cloudflare.ListR2BucketsParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list R2 buckets: %w", err)
	}

	results := make([]R2BucketResult, len(buckets))
	for i, bucket := range buckets {
		results[i] = R2BucketResult{
			Name:     bucket.Name,
			Location: bucket.Location,
		}
		if bucket.CreationDate != nil {
			results[i].CreationDate = *bucket.CreationDate
		}
	}

	return results, nil
}

// DeleteR2Bucket deletes an R2 bucket
func (api *API) DeleteR2Bucket(ctx context.Context, bucketName string) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	accountID, err := api.GetAccountId()
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}

	rc := cloudflare.AccountIdentifier(accountID)
	if err := api.CloudflareClient.DeleteR2Bucket(ctx, rc, bucketName); err != nil {
		return fmt.Errorf("failed to delete R2 bucket: %w", err)
	}

	return nil
}

// R2CORSRule represents a CORS rule for an R2 bucket
type R2CORSRule struct {
	ID             string   `json:"id,omitempty"`
	AllowedOrigins []string `json:"allowedOrigins"`
	AllowedMethods []string `json:"allowedMethods"`
	AllowedHeaders []string `json:"allowedHeaders,omitempty"`
	ExposeHeaders  []string `json:"exposeHeaders,omitempty"`
	MaxAgeSeconds  *int     `json:"maxAgeSeconds,omitempty"`
}

// R2LifecycleRule represents a lifecycle rule for an R2 bucket
type R2LifecycleRule struct {
	ID                             string                  `json:"id"`
	Enabled                        bool                    `json:"enabled"`
	Prefix                         string                  `json:"prefix,omitempty"`
	Expiration                     *R2LifecycleExpiration  `json:"expiration,omitempty"`
	AbortIncompleteMultipartUpload *R2LifecycleAbortUpload `json:"abortIncompleteMultipartUpload,omitempty"`
}

// R2LifecycleExpiration represents expiration settings
type R2LifecycleExpiration struct {
	Days *int   `json:"days,omitempty"`
	Date string `json:"date,omitempty"`
}

// R2LifecycleAbortUpload represents abort incomplete upload settings
type R2LifecycleAbortUpload struct {
	DaysAfterInitiation int `json:"daysAfterInitiation"`
}

// R2NotificationRule represents a notification rule
type R2NotificationRule struct {
	RuleID      string   `json:"ruleId,omitempty"`
	Prefix      string   `json:"prefix,omitempty"`
	Suffix      string   `json:"suffix,omitempty"`
	EventTypes  []string `json:"eventType"`
	Description string   `json:"description,omitempty"`
}

// GetR2CORS retrieves the CORS configuration for an R2 bucket
func (api *API) GetR2CORS(ctx context.Context, bucketName string) ([]R2CORSRule, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId()
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf("/accounts/%s/r2/buckets/%s/cors", accountID, bucketName)
	resp, err := api.CloudflareClient.Raw(ctx, "GET", endpoint, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get CORS: %w", err)
	}

	var result struct {
		Rules []R2CORSRule `json:"rules"`
	}
	if err := jsonUnmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse CORS response: %w", err)
	}

	return result.Rules, nil
}

// SetR2CORS sets the CORS configuration for an R2 bucket
func (api *API) SetR2CORS(ctx context.Context, bucketName string, rules []R2CORSRule) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	accountID, err := api.GetAccountId()
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf("/accounts/%s/r2/buckets/%s/cors", accountID, bucketName)
	body := map[string]interface{}{"rules": rules}
	if _, err := api.CloudflareClient.Raw(ctx, "PUT", endpoint, body, nil); err != nil {
		return fmt.Errorf("failed to set CORS: %w", err)
	}

	return nil
}

// DeleteR2CORS deletes the CORS configuration for an R2 bucket
func (api *API) DeleteR2CORS(ctx context.Context, bucketName string) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	accountID, err := api.GetAccountId()
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf("/accounts/%s/r2/buckets/%s/cors", accountID, bucketName)
	if _, err := api.CloudflareClient.Raw(ctx, "DELETE", endpoint, nil, nil); err != nil {
		return fmt.Errorf("failed to delete CORS: %w", err)
	}

	return nil
}

// GetR2Lifecycle retrieves the lifecycle rules for an R2 bucket
func (api *API) GetR2Lifecycle(ctx context.Context, bucketName string) ([]R2LifecycleRule, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId()
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf("/accounts/%s/r2/buckets/%s/lifecycle", accountID, bucketName)
	resp, err := api.CloudflareClient.Raw(ctx, "GET", endpoint, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get lifecycle: %w", err)
	}

	var result struct {
		Rules []R2LifecycleRule `json:"rules"`
	}
	if err := jsonUnmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse lifecycle response: %w", err)
	}

	return result.Rules, nil
}

// SetR2Lifecycle sets the lifecycle rules for an R2 bucket
func (api *API) SetR2Lifecycle(ctx context.Context, bucketName string, rules []R2LifecycleRule) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	accountID, err := api.GetAccountId()
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf("/accounts/%s/r2/buckets/%s/lifecycle", accountID, bucketName)
	body := map[string]interface{}{"rules": rules}
	if _, err := api.CloudflareClient.Raw(ctx, "PUT", endpoint, body, nil); err != nil {
		return fmt.Errorf("failed to set lifecycle: %w", err)
	}

	return nil
}

// DeleteR2Lifecycle deletes the lifecycle rules for an R2 bucket
func (api *API) DeleteR2Lifecycle(ctx context.Context, bucketName string) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	accountID, err := api.GetAccountId()
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf("/accounts/%s/r2/buckets/%s/lifecycle", accountID, bucketName)
	if _, err := api.CloudflareClient.Raw(ctx, "DELETE", endpoint, nil, nil); err != nil {
		return fmt.Errorf("failed to delete lifecycle: %w", err)
	}

	return nil
}

// GetR2Notifications retrieves the notification rules for an R2 bucket
func (api *API) GetR2Notifications(
	ctx context.Context, bucketName string,
) ([]R2NotificationRule, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId()
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf("/accounts/%s/event_notifications/r2/%s/configuration", accountID, bucketName)
	resp, err := api.CloudflareClient.Raw(ctx, "GET", endpoint, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get notifications: %w", err)
	}

	var result struct {
		Queues map[string][]R2NotificationRule `json:"queues"`
	}
	if err := jsonUnmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse notifications response: %w", err)
	}

	// Flatten all rules from all queues
	var rules []R2NotificationRule
	for _, queueRules := range result.Queues {
		rules = append(rules, queueRules...)
	}

	return rules, nil
}

// SetR2Notification creates or updates a notification rule for an R2 bucket
func (api *API) SetR2Notification(
	ctx context.Context, bucketName, queueID string, rules []R2NotificationRule,
) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	accountID, err := api.GetAccountId()
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf(
		"/accounts/%s/event_notifications/r2/%s/configuration/queues/%s",
		accountID, bucketName, queueID,
	)
	body := map[string]interface{}{"rules": rules}
	if _, err := api.CloudflareClient.Raw(ctx, "PUT", endpoint, body, nil); err != nil {
		return fmt.Errorf("failed to set notification: %w", err)
	}

	return nil
}

// DeleteR2Notification deletes notification rules for an R2 bucket and queue
func (api *API) DeleteR2Notification(ctx context.Context, bucketName, queueID string) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	accountID, err := api.GetAccountId()
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf(
		"/accounts/%s/event_notifications/r2/%s/configuration/queues/%s",
		accountID, bucketName, queueID,
	)
	if _, err := api.CloudflareClient.Raw(ctx, "DELETE", endpoint, nil, nil); err != nil {
		return fmt.Errorf("failed to delete notification: %w", err)
	}

	return nil
}

// jsonUnmarshal is a helper to unmarshal JSON
func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
