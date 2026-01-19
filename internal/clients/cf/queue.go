// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Queue represents a Cloudflare Queue
type Queue struct {
	ID         string `json:"queue_id"`
	Name       string `json:"queue_name"`
	CreatedOn  string `json:"created_on,omitempty"`
	ModifiedOn string `json:"modified_on,omitempty"`
}

// GetQueueID retrieves the queue ID for a given queue name
func (api *API) GetQueueID(ctx context.Context, queueName string) (string, error) {
	if api.CloudflareClient == nil {
		return "", errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf("/accounts/%s/queues", accountID)
	resp, err := api.CloudflareClient.Raw(ctx, http.MethodGet, endpoint, nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to list queues: %w", err)
	}

	var queues []Queue
	if err := json.Unmarshal(resp.Result, &queues); err != nil {
		return "", fmt.Errorf("failed to parse queues response: %w", err)
	}

	for _, q := range queues {
		if q.Name == queueName {
			return q.ID, nil
		}
	}

	return "", fmt.Errorf("queue not found: %s", queueName)
}

// ListQueues lists all Cloudflare Queues
func (api *API) ListQueues(ctx context.Context) ([]Queue, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf("/accounts/%s/queues", accountID)
	resp, err := api.CloudflareClient.Raw(ctx, http.MethodGet, endpoint, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list queues: %w", err)
	}

	var queues []Queue
	if err := json.Unmarshal(resp.Result, &queues); err != nil {
		return nil, fmt.Errorf("failed to parse queues response: %w", err)
	}

	return queues, nil
}
