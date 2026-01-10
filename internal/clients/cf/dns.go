// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"context"

	"github.com/cloudflare/cloudflare-go"
)

// DNSRecordParams contains parameters for creating/updating a DNS record.
type DNSRecordParams struct {
	Name     string
	Type     string
	Content  string
	TTL      int
	Proxied  bool
	Priority *int
	Comment  string
	Tags     []string
	Data     map[string]interface{}
}

// DNSRecordResult contains the result of a DNS record operation.
type DNSRecordResult struct {
	ID      string
	ZoneID  string
	Name    string
	Type    string
	Content string
	TTL     int
	Proxied bool
}

// CreateDNSRecord creates a new DNS record.
func (c *API) CreateDNSRecord(params DNSRecordParams) (*DNSRecordResult, error) {
	if _, err := c.GetZoneId(); err != nil {
		c.Log.Error(err, "error getting zone ID")
		return nil, err
	}

	ctx := context.Background()
	rc := cloudflare.ZoneIdentifier(c.ValidZoneId)

	createParams := cloudflare.CreateDNSRecordParams{
		Name:    params.Name,
		Type:    params.Type,
		Content: params.Content,
		TTL:     params.TTL,
		Comment: params.Comment,
	}

	if params.Type == "A" || params.Type == "AAAA" || params.Type == "CNAME" {
		createParams.Proxied = &params.Proxied
	}

	if params.Priority != nil {
		priority := uint16(*params.Priority)
		createParams.Priority = &priority
	}

	if params.Data != nil {
		createParams.Data = params.Data
	}

	record, err := c.CloudflareClient.CreateDNSRecord(ctx, rc, createParams)
	if err != nil {
		c.Log.Error(err, "error creating DNS record", "name", params.Name)
		return nil, err
	}

	c.Log.Info("DNS Record created", "id", record.ID, "name", record.Name)

	proxied := false
	if record.Proxied != nil {
		proxied = *record.Proxied
	}

	return &DNSRecordResult{
		ID:      record.ID,
		ZoneID:  c.ValidZoneId,
		Name:    record.Name,
		Type:    record.Type,
		Content: record.Content,
		TTL:     record.TTL,
		Proxied: proxied,
	}, nil
}

// GetDNSRecord retrieves a DNS record by ID.
func (c *API) GetDNSRecord(zoneID, recordID string) (*DNSRecordResult, error) {
	ctx := context.Background()
	rc := cloudflare.ZoneIdentifier(zoneID)

	record, err := c.CloudflareClient.GetDNSRecord(ctx, rc, recordID)
	if err != nil {
		c.Log.Error(err, "error getting DNS record", "id", recordID)
		return nil, err
	}

	proxied := false
	if record.Proxied != nil {
		proxied = *record.Proxied
	}

	return &DNSRecordResult{
		ID:      record.ID,
		ZoneID:  zoneID,
		Name:    record.Name,
		Type:    record.Type,
		Content: record.Content,
		TTL:     record.TTL,
		Proxied: proxied,
	}, nil
}

// UpdateDNSRecord updates an existing DNS record.
func (c *API) UpdateDNSRecord(zoneID, recordID string, params DNSRecordParams) (*DNSRecordResult, error) {
	ctx := context.Background()
	rc := cloudflare.ZoneIdentifier(zoneID)

	updateParams := cloudflare.UpdateDNSRecordParams{
		ID:      recordID,
		Name:    params.Name,
		Type:    params.Type,
		Content: params.Content,
		TTL:     params.TTL,
		Comment: &params.Comment,
	}

	if params.Type == "A" || params.Type == "AAAA" || params.Type == "CNAME" {
		updateParams.Proxied = &params.Proxied
	}

	if params.Priority != nil {
		priority := uint16(*params.Priority)
		updateParams.Priority = &priority
	}

	if params.Data != nil {
		updateParams.Data = params.Data
	}

	record, err := c.CloudflareClient.UpdateDNSRecord(ctx, rc, updateParams)
	if err != nil {
		c.Log.Error(err, "error updating DNS record", "id", recordID)
		return nil, err
	}

	c.Log.Info("DNS Record updated", "id", record.ID, "name", record.Name)

	proxied := false
	if record.Proxied != nil {
		proxied = *record.Proxied
	}

	return &DNSRecordResult{
		ID:      record.ID,
		ZoneID:  zoneID,
		Name:    record.Name,
		Type:    record.Type,
		Content: record.Content,
		TTL:     record.TTL,
		Proxied: proxied,
	}, nil
}

// DeleteDNSRecord deletes a DNS record.
func (c *API) DeleteDNSRecord(zoneID, recordID string) error {
	ctx := context.Background()
	rc := cloudflare.ZoneIdentifier(zoneID)

	err := c.CloudflareClient.DeleteDNSRecord(ctx, rc, recordID)
	if err != nil {
		c.Log.Error(err, "error deleting DNS record", "id", recordID)
		return err
	}

	c.Log.Info("DNS Record deleted", "id", recordID)
	return nil
}
