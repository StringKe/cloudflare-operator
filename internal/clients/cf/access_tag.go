// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"context"

	"github.com/cloudflare/cloudflare-go"
)

// AccessTagResult is the operator-facing view of a Cloudflare Access tag.
type AccessTagResult struct {
	Name     string
	AppCount int
}

// findAccessTag returns the converted tag matching name, or nil if none match.
// Separated from the API call so the match/convert logic is unit-testable.
func findAccessTag(tags []cloudflare.AccessTag, name string) *AccessTagResult {
	for _, t := range tags {
		if t.Name == name {
			return &AccessTagResult{Name: t.Name, AppCount: t.AppCount}
		}
	}
	return nil
}

// GetAccessTag finds an Access tag by name. It returns nil (without error) when
// no tag matches, mirroring the other "find existing" helpers in this package.
func (c *API) GetAccessTag(ctx context.Context, name string) (*AccessTagResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	tags, err := c.CloudflareClient.ListAccessTags(ctx, rc, cloudflare.ListAccessTagsParams{})
	if err != nil {
		c.Log.Error(err, "error listing access tags")
		return nil, err
	}

	return findAccessTag(tags, name), nil
}

// CreateAccessTag creates an account-level Access tag with the given name.
func (c *API) CreateAccessTag(ctx context.Context, name string) (*AccessTagResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	tag, err := c.CloudflareClient.CreateAccessTag(ctx, rc, cloudflare.CreateAccessTagParams{Name: name})
	if err != nil {
		c.Log.Error(err, "error creating access tag", "name", name)
		return nil, err
	}

	return &AccessTagResult{Name: tag.Name, AppCount: tag.AppCount}, nil
}

// DeleteAccessTag deletes an account-level Access tag by name.
func (c *API) DeleteAccessTag(ctx context.Context, name string) error {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	if err := c.CloudflareClient.DeleteAccessTag(ctx, rc, name); err != nil {
		c.Log.Error(err, "error deleting access tag", "name", name)
		return err
	}

	return nil
}
