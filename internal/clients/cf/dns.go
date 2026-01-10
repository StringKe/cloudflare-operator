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

// InZone DNS Operations
// These methods allow specifying Zone ID directly instead of relying on c.ValidZoneId.
// This enables multi-zone support via DomainResolver.

// GetDNSCNameIdInZone returns the ID of the CNAME record for the given fqdn in the specified zone.
// Returns empty string and nil error if the record does not exist (this is not an error condition).
// Returns empty string and error if there was an actual API error or multiple records found.
func (c *API) GetDNSCNameIdInZone(zoneID, fqdn string) (string, error) {
	ctx := context.Background()
	rc := cloudflare.ZoneIdentifier(zoneID)
	params := cloudflare.ListDNSRecordsParams{
		Type: "CNAME",
		Name: fqdn,
	}
	records, _, err := c.CloudflareClient.ListDNSRecords(ctx, rc, params)
	if err != nil {
		c.Log.Error(err, "error listing DNS records in zone", "zoneId", zoneID, "fqdn", fqdn)
		return "", err
	}

	switch len(records) {
	case 0:
		c.Log.V(1).Info("no DNS record found for fqdn in zone", "zoneId", zoneID, "fqdn", fqdn)
		return "", nil
	case 1:
		return records[0].ID, nil
	default:
		err := ErrMultipleResourcesFound
		c.Log.Error(err, "multiple CNAME records found for fqdn in zone", "zoneId", zoneID, "fqdn", fqdn, "count", len(records))
		return "", err
	}
}

// GetDNSRecordIdInZone returns the ID of a DNS record of the given type for the fqdn in the specified zone.
// Returns empty string and nil error if the record does not exist.
func (c *API) GetDNSRecordIdInZone(zoneID, fqdn, recordType string) (string, error) {
	ctx := context.Background()
	rc := cloudflare.ZoneIdentifier(zoneID)
	params := cloudflare.ListDNSRecordsParams{
		Type: recordType,
		Name: fqdn,
	}
	records, _, err := c.CloudflareClient.ListDNSRecords(ctx, rc, params)
	if err != nil {
		c.Log.Error(err, "error listing DNS records in zone", "zoneId", zoneID, "fqdn", fqdn, "type", recordType)
		return "", err
	}

	switch len(records) {
	case 0:
		c.Log.V(1).Info("no DNS record found for fqdn in zone", "zoneId", zoneID, "fqdn", fqdn, "type", recordType)
		return "", nil
	case 1:
		return records[0].ID, nil
	default:
		err := ErrMultipleResourcesFound
		c.Log.Error(err, "multiple records found for fqdn in zone", "zoneId", zoneID, "fqdn", fqdn, "type", recordType, "count", len(records))
		return "", err
	}
}

// CreateDNSRecordInZone creates a new DNS record in the specified zone.
func (c *API) CreateDNSRecordInZone(zoneID string, params DNSRecordParams) (*DNSRecordResult, error) {
	ctx := context.Background()
	rc := cloudflare.ZoneIdentifier(zoneID)

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
		c.Log.Error(err, "error creating DNS record in zone", "zoneId", zoneID, "name", params.Name)
		return nil, err
	}

	c.Log.Info("DNS Record created in zone", "zoneId", zoneID, "id", record.ID, "name", record.Name)

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

// UpdateDNSRecordInZone updates an existing DNS record in the specified zone.
func (c *API) UpdateDNSRecordInZone(zoneID, recordID string, params DNSRecordParams) (*DNSRecordResult, error) {
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
		c.Log.Error(err, "error updating DNS record in zone", "zoneId", zoneID, "id", recordID)
		return nil, err
	}

	c.Log.Info("DNS Record updated in zone", "zoneId", zoneID, "id", record.ID, "name", record.Name)

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

// DeleteDNSRecordInZone deletes a DNS record in the specified zone.
func (c *API) DeleteDNSRecordInZone(zoneID, recordID string) error {
	ctx := context.Background()
	rc := cloudflare.ZoneIdentifier(zoneID)

	err := c.CloudflareClient.DeleteDNSRecord(ctx, rc, recordID)
	if err != nil {
		c.Log.Error(err, "error deleting DNS record in zone", "zoneId", zoneID, "id", recordID)
		return err
	}

	c.Log.Info("DNS Record deleted from zone", "zoneId", zoneID, "id", recordID)
	return nil
}

// InsertOrUpdateCNameInZone upserts DNS CNAME record for the given FQDN to point to the tunnel in the specified zone.
// If tunnelID is empty, it uses c.ValidTunnelId.
func (c *API) InsertOrUpdateCNameInZone(zoneID, fqdn, dnsId, tunnelID string, proxied bool) (string, error) {
	if tunnelID == "" {
		tunnelID = c.ValidTunnelId
	}
	if tunnelID == "" {
		return "", ErrInvalidTunnelId
	}

	ctx := context.Background()
	rc := cloudflare.ZoneIdentifier(zoneID)
	target := tunnelID + ".cfargotunnel.com"

	if dnsId != "" {
		c.Log.Info("Updating existing CNAME record in zone", "zoneId", zoneID, "fqdn", fqdn, "dnsId", dnsId)
		updateParams := cloudflare.UpdateDNSRecordParams{
			ID:      dnsId,
			Type:    "CNAME",
			Name:    fqdn,
			Content: target,
			Comment: stringPtr("Managed by cloudflare-operator"),
			TTL:     1,
			Proxied: &proxied,
		}
		_, err := c.CloudflareClient.UpdateDNSRecord(ctx, rc, updateParams)
		if err != nil {
			c.Log.Error(err, "error updating DNS record in zone", "zoneId", zoneID, "fqdn", fqdn)
			return "", err
		}
		c.Log.Info("DNS record updated in zone", "zoneId", zoneID, "fqdn", fqdn)
		return dnsId, nil
	}

	c.Log.Info("Creating CNAME record in zone", "zoneId", zoneID, "fqdn", fqdn)
	createParams := cloudflare.CreateDNSRecordParams{
		Type:    "CNAME",
		Name:    fqdn,
		Content: target,
		Comment: "Managed by cloudflare-operator",
		TTL:     1,
		Proxied: &proxied,
	}
	resp, err := c.CloudflareClient.CreateDNSRecord(ctx, rc, createParams)
	if err != nil {
		c.Log.Error(err, "error creating DNS record in zone", "zoneId", zoneID, "fqdn", fqdn)
		return "", err
	}
	c.Log.Info("DNS record created in zone", "zoneId", zoneID, "fqdn", fqdn)
	return resp.ID, nil
}

// stringPtr returns a pointer to a string
func stringPtr(s string) *string {
	return &s
}
