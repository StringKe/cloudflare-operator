// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
)

// RegistrarDomainInfo contains information about a registered domain
type RegistrarDomainInfo struct {
	ID                string
	Available         bool
	SupportedTLD      bool
	CanRegister       bool
	CurrentRegistrar  string
	ExpiresAt         time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
	RegistryStatuses  string
	Locked            bool
	TransferInStatus  string // Combined transfer status
	CanCancelTransfer bool
	RegistrantContact *RegistrantContactInfo
}

// RegistrantContactInfo contains registrant contact information
type RegistrantContactInfo struct {
	ID           string
	FirstName    string
	LastName     string
	Organization string
	Address      string
	Address2     string
	City         string
	State        string
	Zip          string
	Country      string
	Phone        string
	Email        string
	Fax          string
}

// RegistrarDomainConfig contains domain configuration
type RegistrarDomainConfig struct {
	NameServers []string
	Privacy     bool
	Locked      bool
	AutoRenew   bool
}

const transferStepComplete = "complete"

// getTransferStatus returns a human-readable transfer status
//
//nolint:revive // cognitive complexity is acceptable for transfer status determination
func getTransferStatus(transferIn cloudflare.RegistrarTransferIn) string {
	// Check transfer steps in order
	if step, ok := checkTransferStep(transferIn.UnlockDomain, "unlock_domain"); ok {
		return step
	}
	if step, ok := checkTransferStep(transferIn.DisablePrivacy, "disable_privacy"); ok {
		return step
	}
	if step, ok := checkTransferStep(transferIn.EnterAuthCode, "enter_auth_code"); ok {
		return step
	}
	if step, ok := checkTransferStep(transferIn.ApproveTransfer, "approve_transfer"); ok {
		return step
	}
	if step, ok := checkTransferStep(transferIn.AcceptFoa, "accept_foa"); ok {
		return step
	}
	// All steps complete or empty
	if transferIn.CanCancelTransfer {
		return "in_progress"
	}
	return ""
}

// checkTransferStep checks if a transfer step is incomplete
func checkTransferStep(status, stepName string) (string, bool) {
	if status != "" && status != transferStepComplete {
		return stepName + ":" + status, true
	}
	return "", false
}

// GetRegistrarDomain retrieves information about a registered domain
func (api *API) GetRegistrarDomain(ctx context.Context, domainName string) (*RegistrarDomainInfo, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	domain, err := api.CloudflareClient.RegistrarDomain(ctx, accountID, domainName)
	if err != nil {
		return nil, fmt.Errorf("failed to get registrar domain: %w", err)
	}

	info := &RegistrarDomainInfo{
		ID:                domain.ID,
		Available:         domain.Available,
		SupportedTLD:      domain.SupportedTLD,
		CanRegister:       domain.CanRegister,
		CurrentRegistrar:  domain.CurrentRegistrar,
		ExpiresAt:         domain.ExpiresAt,
		CreatedAt:         domain.CreatedAt,
		UpdatedAt:         domain.UpdatedAt,
		RegistryStatuses:  domain.RegistryStatuses,
		Locked:            domain.Locked,
		TransferInStatus:  getTransferStatus(domain.TransferIn),
		CanCancelTransfer: domain.TransferIn.CanCancelTransfer,
	}

	if domain.RegistrantContact.ID != "" {
		info.RegistrantContact = &RegistrantContactInfo{
			ID:           domain.RegistrantContact.ID,
			FirstName:    domain.RegistrantContact.FirstName,
			LastName:     domain.RegistrantContact.LastName,
			Organization: domain.RegistrantContact.Organization,
			Address:      domain.RegistrantContact.Address,
			Address2:     domain.RegistrantContact.Address2,
			City:         domain.RegistrantContact.City,
			State:        domain.RegistrantContact.State,
			Zip:          domain.RegistrantContact.Zip,
			Country:      domain.RegistrantContact.Country,
			Phone:        domain.RegistrantContact.Phone,
			Email:        domain.RegistrantContact.Email,
			Fax:          domain.RegistrantContact.Fax,
		}
	}

	return info, nil
}

// ListRegistrarDomains lists all domains registered with Cloudflare Registrar
func (api *API) ListRegistrarDomains(ctx context.Context) ([]RegistrarDomainInfo, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	domains, err := api.CloudflareClient.RegistrarDomains(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to list registrar domains: %w", err)
	}

	results := make([]RegistrarDomainInfo, len(domains))
	for i, domain := range domains {
		results[i] = RegistrarDomainInfo{
			ID:                domain.ID,
			Available:         domain.Available,
			SupportedTLD:      domain.SupportedTLD,
			CanRegister:       domain.CanRegister,
			CurrentRegistrar:  domain.CurrentRegistrar,
			ExpiresAt:         domain.ExpiresAt,
			CreatedAt:         domain.CreatedAt,
			UpdatedAt:         domain.UpdatedAt,
			RegistryStatuses:  domain.RegistryStatuses,
			Locked:            domain.Locked,
			TransferInStatus:  getTransferStatus(domain.TransferIn),
			CanCancelTransfer: domain.TransferIn.CanCancelTransfer,
		}
	}

	return results, nil
}

// UpdateRegistrarDomain updates domain configuration
func (api *API) UpdateRegistrarDomain(
	ctx context.Context, domainName string, config RegistrarDomainConfig,
) (*RegistrarDomainInfo, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	cfConfig := cloudflare.RegistrarDomainConfiguration{
		NameServers: config.NameServers,
		Privacy:     config.Privacy,
		Locked:      config.Locked,
		AutoRenew:   config.AutoRenew,
	}

	domain, err := api.CloudflareClient.UpdateRegistrarDomain(ctx, accountID, domainName, cfConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to update registrar domain: %w", err)
	}

	info := &RegistrarDomainInfo{
		ID:                domain.ID,
		Available:         domain.Available,
		SupportedTLD:      domain.SupportedTLD,
		CanRegister:       domain.CanRegister,
		CurrentRegistrar:  domain.CurrentRegistrar,
		ExpiresAt:         domain.ExpiresAt,
		CreatedAt:         domain.CreatedAt,
		UpdatedAt:         domain.UpdatedAt,
		RegistryStatuses:  domain.RegistryStatuses,
		Locked:            domain.Locked,
		TransferInStatus:  getTransferStatus(domain.TransferIn),
		CanCancelTransfer: domain.TransferIn.CanCancelTransfer,
	}

	return info, nil
}

// InitiateRegistrarTransfer initiates a domain transfer to Cloudflare
func (api *API) InitiateRegistrarTransfer(ctx context.Context, domainName string) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}

	_, err = api.CloudflareClient.TransferRegistrarDomain(ctx, accountID, domainName)
	if err != nil {
		return fmt.Errorf("failed to initiate domain transfer: %w", err)
	}

	return nil
}

// CancelRegistrarTransfer cancels a pending domain transfer
func (api *API) CancelRegistrarTransfer(ctx context.Context, domainName string) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}

	_, err = api.CloudflareClient.CancelRegistrarDomainTransfer(ctx, accountID, domainName)
	if err != nil {
		return fmt.Errorf("failed to cancel domain transfer: %w", err)
	}

	return nil
}
