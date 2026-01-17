// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package handlers

import (
	"net/http"
	"time"

	"github.com/StringKe/cloudflare-operator/test/mockserver/models"
)

// R2BucketCreateRequest represents an R2 bucket creation request.
type R2BucketCreateRequest struct {
	Name     string `json:"name"`
	Location string `json:"locationHint,omitempty"`
}

// CreateR2Bucket handles PUT /accounts/{accountId}/r2/buckets.
func (h *Handlers) CreateR2Bucket(w http.ResponseWriter, r *http.Request) {
	req, err := ReadJSON[R2BucketCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	// Check for existing bucket
	if _, ok := h.store.GetR2Bucket(req.Name); ok {
		Conflict(w, "bucket already exists")
		return
	}

	bucket := &models.R2Bucket{
		Name:         req.Name,
		CreationDate: time.Now(),
		Location:     req.Location,
	}

	h.store.CreateR2Bucket(bucket)
	Created(w, bucket)
}

// ListR2Buckets handles GET /accounts/{accountId}/r2/buckets.
func (h *Handlers) ListR2Buckets(w http.ResponseWriter, r *http.Request) {
	namePrefix := GetQueryParam(r, "name_prefix")
	buckets := h.store.ListR2Buckets(namePrefix)
	Success(w, map[string]interface{}{
		"buckets": buckets,
	})
}

// GetR2Bucket handles GET /accounts/{accountId}/r2/buckets/{bucketName}.
func (h *Handlers) GetR2Bucket(w http.ResponseWriter, r *http.Request) {
	bucketName := GetPathParam(r, "bucketName")
	bucket, ok := h.store.GetR2Bucket(bucketName)
	if !ok {
		NotFound(w, "bucket")
		return
	}
	Success(w, bucket)
}

// DeleteR2Bucket handles DELETE /accounts/{accountId}/r2/buckets/{bucketName}.
func (h *Handlers) DeleteR2Bucket(w http.ResponseWriter, r *http.Request) {
	bucketName := GetPathParam(r, "bucketName")
	if !h.store.DeleteR2Bucket(bucketName) {
		NotFound(w, "bucket")
		return
	}
	Success(w, struct{}{})
}

// R2BucketLifecycleRules represents lifecycle rules for an R2 bucket.
type R2BucketLifecycleRules struct {
	Rules []R2LifecycleRule `json:"rules"`
}

// R2LifecycleRule represents a single lifecycle rule.
type R2LifecycleRule struct {
	ID         string `json:"id"`
	Enabled    bool   `json:"enabled"`
	Conditions struct {
		Prefix string `json:"prefix,omitempty"`
	} `json:"conditions,omitempty"`
	Actions struct {
		DeleteAfterDays int `json:"deleteAfterDays,omitempty"`
	} `json:"actions,omitempty"`
}

// GetR2BucketLifecycle handles GET /accounts/{accountId}/r2/buckets/{bucketName}/lifecycle.
func (h *Handlers) GetR2BucketLifecycle(w http.ResponseWriter, r *http.Request) {
	bucketName := GetPathParam(r, "bucketName")
	if _, ok := h.store.GetR2Bucket(bucketName); !ok {
		NotFound(w, "bucket")
		return
	}

	lifecycle := h.store.GetR2BucketLifecycle(bucketName)
	Success(w, lifecycle)
}

// UpdateR2BucketLifecycle handles PUT /accounts/{accountId}/r2/buckets/{bucketName}/lifecycle.
func (h *Handlers) UpdateR2BucketLifecycle(w http.ResponseWriter, r *http.Request) {
	bucketName := GetPathParam(r, "bucketName")
	if _, ok := h.store.GetR2Bucket(bucketName); !ok {
		NotFound(w, "bucket")
		return
	}

	req, err := ReadJSON[R2BucketLifecycleRules](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	h.store.UpdateR2BucketLifecycle(bucketName, req)
	Success(w, req)
}
