// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"context"
	"crypto/rand"
	"encoding/base64"

	"github.com/cloudflare/cloudflare-go"
)

// WARPConnectorResult contains the result of a WARP Connector operation.
type WARPConnectorResult struct {
	ID          string
	TunnelID    string
	TunnelToken string
	Name        string
}

// WARPConnectorTokenResult contains the tunnel token for a WARP connector.
type WARPConnectorTokenResult struct {
	Token string
}

// CreateWARPConnector creates a new WARP Connector.
func (c *API) CreateWARPConnector(ctx context.Context, name string) (*WARPConnectorResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	// Generate 32 byte random string for tunnel secret
	randSecret := make([]byte, 32)
	if _, err := rand.Read(randSecret); err != nil {
		return nil, err
	}
	tunnelSecret := base64.StdEncoding.EncodeToString(randSecret)

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	params := cloudflare.TunnelCreateParams{
		Name:      name,
		Secret:    tunnelSecret,
		ConfigSrc: "cloudflare", // WARP connectors are cloudflare-managed
	}

	tunnel, err := c.CloudflareClient.CreateTunnel(ctx, rc, params)
	if err != nil {
		c.Log.Error(err, "error creating WARP connector", "name", name)
		return nil, err
	}

	// Get the tunnel token
	tokenResult, err := c.CloudflareClient.GetTunnelToken(ctx, rc, tunnel.ID)
	if err != nil {
		c.Log.Error(err, "error getting tunnel token", "id", tunnel.ID)
		return nil, err
	}

	c.Log.Info("WARP Connector created", "id", tunnel.ID, "name", tunnel.Name)

	return &WARPConnectorResult{
		ID:          tunnel.ID,
		TunnelID:    tunnel.ID,
		TunnelToken: tokenResult,
		Name:        tunnel.Name,
	}, nil
}

// GetWARPConnectorToken retrieves the tunnel token for a WARP connector.
func (c *API) GetWARPConnectorToken(ctx context.Context, connectorID string) (*WARPConnectorTokenResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	token, err := c.CloudflareClient.GetTunnelToken(ctx, rc, connectorID)
	if err != nil {
		c.Log.Error(err, "error getting WARP connector token", "id", connectorID)
		return nil, err
	}

	return &WARPConnectorTokenResult{
		Token: token,
	}, nil
}

// DeleteWARPConnector deletes a WARP Connector.
// This method is idempotent - returns nil if the connector is already deleted.
func (c *API) DeleteWARPConnector(ctx context.Context, connectorID string) error {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	err := c.CloudflareClient.DeleteTunnel(ctx, rc, connectorID)
	if err != nil {
		if IsNotFoundError(err) {
			c.Log.Info("WARP Connector already deleted (not found)", "id", connectorID)
			return nil
		}
		c.Log.Error(err, "error deleting WARP connector", "id", connectorID)
		return err
	}

	c.Log.Info("WARP Connector deleted", "id", connectorID)
	return nil
}

// GatewayConfigurationParams contains parameters for Gateway Configuration.
type GatewayConfigurationParams struct {
	TLSDecrypt        *TLSDecryptSettings
	ActivityLog       *ActivityLogSettings
	AntiVirus         *AntiVirusSettings
	BlockPage         *BlockPageSettings
	BodyScanning      *BodyScanningSettings
	BrowserIsolation  *BrowserIsolationSettings
	FIPS              *FIPSSettings
	ProtocolDetection *ProtocolDetectionSettings
	CustomCertificate *CustomCertificateSettings
}

// TLSDecryptSettings for TLS decryption.
type TLSDecryptSettings struct {
	Enabled bool
}

// ActivityLogSettings for activity logging.
type ActivityLogSettings struct {
	Enabled bool
}

// AntiVirusSettings for AV scanning.
type AntiVirusSettings struct {
	EnabledDownloadPhase bool
	EnabledUploadPhase   bool
	FailClosed           bool
	NotificationSettings *NotificationSettings
}

// NotificationSettings for antivirus notifications.
type NotificationSettings struct {
	Enabled    bool
	Message    string
	SupportURL string
}

// BlockPageSettings for block page customization.
type BlockPageSettings struct {
	Enabled         bool
	Name            string
	FooterText      string
	HeaderText      string
	LogoPath        string
	BackgroundColor string
	MailtoAddress   string
	MailtoSubject   string
	SuppressFooter  *bool
}

// BodyScanningSettings for body scanning.
type BodyScanningSettings struct {
	InspectionMode string
}

// BrowserIsolationSettings for browser isolation.
type BrowserIsolationSettings struct {
	URLBrowserIsolationEnabled bool
	NonIdentityEnabled         bool
}

// FIPSSettings for FIPS compliance.
type FIPSSettings struct {
	TLS bool
}

// ProtocolDetectionSettings for protocol detection.
type ProtocolDetectionSettings struct {
	Enabled bool
}

// CustomCertificateSettings for custom CA.
type CustomCertificateSettings struct {
	Enabled bool
	ID      string
}

// GatewayConfigurationResult contains the result of a Gateway Configuration operation.
type GatewayConfigurationResult struct {
	AccountID string
}

// buildTeamsAccountSettings constructs Teams account settings from params.
//
//nolint:revive // function extracts settings building logic to reduce main function complexity
func buildTeamsAccountSettings(params GatewayConfigurationParams) cloudflare.TeamsAccountSettings {
	settings := cloudflare.TeamsAccountSettings{}

	if params.TLSDecrypt != nil {
		settings.TLSDecrypt = &cloudflare.TeamsTLSDecrypt{
			Enabled: params.TLSDecrypt.Enabled,
		}
	}

	if params.ActivityLog != nil {
		settings.ActivityLog = &cloudflare.TeamsActivityLog{
			Enabled: params.ActivityLog.Enabled,
		}
	}

	if params.AntiVirus != nil {
		settings.Antivirus = buildAntiVirusSettings(params.AntiVirus)
	}

	if params.BlockPage != nil {
		settings.BlockPage = &cloudflare.TeamsBlockPage{
			Enabled:         &params.BlockPage.Enabled,
			Name:            params.BlockPage.Name,
			FooterText:      params.BlockPage.FooterText,
			HeaderText:      params.BlockPage.HeaderText,
			LogoPath:        params.BlockPage.LogoPath,
			BackgroundColor: params.BlockPage.BackgroundColor,
			MailtoAddress:   params.BlockPage.MailtoAddress,
			MailtoSubject:   params.BlockPage.MailtoSubject,
			SuppressFooter:  params.BlockPage.SuppressFooter,
		}
	}

	if params.BodyScanning != nil {
		settings.BodyScanning = &cloudflare.TeamsBodyScanning{
			InspectionMode: params.BodyScanning.InspectionMode,
		}
	}

	if params.BrowserIsolation != nil {
		settings.BrowserIsolation = &cloudflare.BrowserIsolation{
			UrlBrowserIsolationEnabled: &params.BrowserIsolation.URLBrowserIsolationEnabled,
			NonIdentityEnabled:         &params.BrowserIsolation.NonIdentityEnabled,
		}
	}

	if params.FIPS != nil {
		settings.FIPS = &cloudflare.TeamsFIPS{
			TLS: params.FIPS.TLS,
		}
	}

	if params.ProtocolDetection != nil {
		settings.ProtocolDetection = &cloudflare.TeamsProtocolDetection{
			Enabled: params.ProtocolDetection.Enabled,
		}
	}

	if params.CustomCertificate != nil {
		settings.CustomCertificate = &cloudflare.TeamsCustomCertificate{
			Enabled: &params.CustomCertificate.Enabled,
			ID:      params.CustomCertificate.ID,
		}
	}

	return settings
}

// buildAntiVirusSettings constructs Teams antivirus settings.
func buildAntiVirusSettings(params *AntiVirusSettings) *cloudflare.TeamsAntivirus {
	av := &cloudflare.TeamsAntivirus{
		EnabledDownloadPhase: params.EnabledDownloadPhase,
		EnabledUploadPhase:   params.EnabledUploadPhase,
		FailClosed:           params.FailClosed,
	}
	if params.NotificationSettings != nil {
		av.NotificationSettings = &cloudflare.TeamsNotificationSettings{
			Enabled:    &params.NotificationSettings.Enabled,
			Message:    params.NotificationSettings.Message,
			SupportURL: params.NotificationSettings.SupportURL,
		}
	}
	return av
}

// UpdateGatewayConfiguration updates the Gateway configuration for an account.
func (c *API) UpdateGatewayConfiguration(
	ctx context.Context,
	params GatewayConfigurationParams,
) (*GatewayConfigurationResult, error) {
	if _, err := c.GetAccountId(ctx); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	// Build the final configuration
	settings := cloudflare.TeamsConfiguration{
		Settings: buildTeamsAccountSettings(params),
	}

	// Update the configuration
	_, err := c.CloudflareClient.TeamsAccountUpdateConfiguration(ctx, c.ValidAccountId, settings)
	if err != nil {
		c.Log.Error(err, "error updating gateway configuration")
		return nil, err
	}

	c.Log.Info("Gateway Configuration updated", "accountId", c.ValidAccountId)

	return &GatewayConfigurationResult{
		AccountID: c.ValidAccountId,
	}, nil
}
