// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package handlers

import (
	"net/http"
	"time"

	"github.com/StringKe/cloudflare-operator/test/mockserver/models"
)

// DNSRecordCreateRequest represents a DNS record creation request.
type DNSRecordCreateRequest struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Proxied  *bool  `json:"proxied"`
	Priority *int   `json:"priority"`
	Comment  string `json:"comment"`
}

// CreateDNSRecord handles POST /zones/{zoneId}/dns_records.
func (h *Handlers) CreateDNSRecord(w http.ResponseWriter, r *http.Request) {
	zoneID := GetPathParam(r, "zoneId")

	zone, ok := h.store.GetZone(zoneID)
	if !ok {
		NotFound(w, "zone")
		return
	}

	req, err := ReadJSON[DNSRecordCreateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	now := time.Now()
	record := &models.DNSRecord{
		ID:         GenerateID(),
		ZoneID:     zoneID,
		ZoneName:   zone.Name,
		Name:       req.Name,
		Type:       req.Type,
		Content:    req.Content,
		TTL:        req.TTL,
		Proxied:    req.Proxied,
		Priority:   req.Priority,
		Comment:    req.Comment,
		CreatedOn:  now,
		ModifiedOn: now,
	}

	h.store.CreateDNSRecord(record)
	Created(w, record)
}

// ListDNSRecords handles GET /zones/{zoneId}/dns_records.
func (h *Handlers) ListDNSRecords(w http.ResponseWriter, r *http.Request) {
	zoneID := GetPathParam(r, "zoneId")
	recordType := GetQueryParam(r, "type")
	name := GetQueryParam(r, "name")

	records := h.store.ListDNSRecords(zoneID, recordType, name)
	Success(w, records)
}

// GetDNSRecord handles GET /zones/{zoneId}/dns_records/{recordId}.
func (h *Handlers) GetDNSRecord(w http.ResponseWriter, r *http.Request) {
	recordID := GetPathParam(r, "recordId")
	record, ok := h.store.GetDNSRecord(recordID)
	if !ok {
		NotFound(w, "dns record")
		return
	}
	Success(w, record)
}

// DNSRecordUpdateRequest represents a DNS record update request.
type DNSRecordUpdateRequest struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Proxied  *bool  `json:"proxied"`
	Priority *int   `json:"priority"`
	Comment  string `json:"comment"`
}

// UpdateDNSRecord handles PUT/PATCH /zones/{zoneId}/dns_records/{recordId}.
func (h *Handlers) UpdateDNSRecord(w http.ResponseWriter, r *http.Request) {
	recordID := GetPathParam(r, "recordId")

	req, err := ReadJSON[DNSRecordUpdateRequest](r)
	if err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if !h.store.UpdateDNSRecord(recordID, func(record *models.DNSRecord) {
		if req.Name != "" {
			record.Name = req.Name
		}
		if req.Type != "" {
			record.Type = req.Type
		}
		if req.Content != "" {
			record.Content = req.Content
		}
		if req.TTL != 0 {
			record.TTL = req.TTL
		}
		if req.Proxied != nil {
			record.Proxied = req.Proxied
		}
		if req.Priority != nil {
			record.Priority = req.Priority
		}
		if req.Comment != "" {
			record.Comment = req.Comment
		}
	}) {
		NotFound(w, "dns record")
		return
	}

	record, _ := h.store.GetDNSRecord(recordID)
	Success(w, record)
}

// DeleteDNSRecord handles DELETE /zones/{zoneId}/dns_records/{recordId}.
func (h *Handlers) DeleteDNSRecord(w http.ResponseWriter, r *http.Request) {
	recordID := GetPathParam(r, "recordId")
	if !h.store.DeleteDNSRecord(recordID) {
		NotFound(w, "dns record")
		return
	}
	Success(w, struct {
		ID string `json:"id"`
	}{ID: recordID})
}
