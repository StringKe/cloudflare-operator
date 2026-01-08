/*
Copyright 2025 Adyanth H.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
func (c *API) CreateWARPConnector(name string) (*WARPConnectorResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	// Generate 32 byte random string for tunnel secret
	randSecret := make([]byte, 32)
	if _, err := rand.Read(randSecret); err != nil {
		return nil, err
	}
	tunnelSecret := base64.StdEncoding.EncodeToString(randSecret)

	ctx := context.Background()
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
func (c *API) GetWARPConnectorToken(connectorID string) (*WARPConnectorTokenResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()
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
func (c *API) DeleteWARPConnector(connectorID string) error {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return err
	}

	ctx := context.Background()
	rc := cloudflare.AccountIdentifier(c.ValidAccountId)

	err := c.CloudflareClient.DeleteTunnel(ctx, rc, connectorID)
	if err != nil {
		c.Log.Error(err, "error deleting WARP connector", "id", connectorID)
		return err
	}

	c.Log.Info("WARP Connector deleted", "id", connectorID)
	return nil
}

// GatewayConfigurationParams contains parameters for Gateway Configuration.
type GatewayConfigurationParams struct {
	Settings map[string]interface{}
}

// GatewayConfigurationResult contains the result of a Gateway Configuration operation.
type GatewayConfigurationResult struct {
	AccountID string
}

// UpdateGatewayConfiguration updates the Gateway configuration for an account.
func (c *API) UpdateGatewayConfiguration(params GatewayConfigurationParams) (*GatewayConfigurationResult, error) {
	if _, err := c.GetAccountId(); err != nil {
		c.Log.Error(err, "error getting account ID")
		return nil, err
	}

	ctx := context.Background()

	// Build the gateway settings from params
	accountSettings := cloudflare.TeamsAccountSettings{}

	// Parse TLS Decrypt
	if tlsDecrypt, ok := params.Settings["tls_decrypt"].(map[string]any); ok {
		if enabled, ok := tlsDecrypt["enabled"].(bool); ok {
			accountSettings.TLSDecrypt = &cloudflare.TeamsTLSDecrypt{
				Enabled: enabled,
			}
		}
	}

	// Parse Activity Log
	if activityLog, ok := params.Settings["activity_log"].(map[string]any); ok {
		if enabled, ok := activityLog["enabled"].(bool); ok {
			accountSettings.ActivityLog = &cloudflare.TeamsActivityLog{
				Enabled: enabled,
			}
		}
	}

	// Parse AntiVirus
	if antivirus, ok := params.Settings["antivirus"].(map[string]any); ok {
		av := &cloudflare.TeamsAntivirus{}
		if v, ok := antivirus["enabled_download_phase"].(bool); ok {
			av.EnabledDownloadPhase = v
		}
		if v, ok := antivirus["enabled_upload_phase"].(bool); ok {
			av.EnabledUploadPhase = v
		}
		if v, ok := antivirus["fail_closed"].(bool); ok {
			av.FailClosed = v
		}
		if ns, ok := antivirus["notification_settings"].(map[string]any); ok {
			av.NotificationSettings = &cloudflare.TeamsNotificationSettings{}
			if v, ok := ns["enabled"].(bool); ok {
				av.NotificationSettings.Enabled = &v
			}
			if v, ok := ns["msg"].(string); ok {
				av.NotificationSettings.Message = v
			}
			if v, ok := ns["support_url"].(string); ok {
				av.NotificationSettings.SupportURL = v
			}
		}
		accountSettings.Antivirus = av
	}

	// Parse Block Page
	if blockPage, ok := params.Settings["block_page"].(map[string]any); ok {
		bp := &cloudflare.TeamsBlockPage{}
		if v, ok := blockPage["enabled"].(bool); ok {
			bp.Enabled = &v
		}
		if v, ok := blockPage["footer_text"].(string); ok {
			bp.FooterText = v
		}
		if v, ok := blockPage["header_text"].(string); ok {
			bp.HeaderText = v
		}
		if v, ok := blockPage["logo_path"].(string); ok {
			bp.LogoPath = v
		}
		if v, ok := blockPage["background_color"].(string); ok {
			bp.BackgroundColor = v
		}
		accountSettings.BlockPage = bp
	}

	// Parse Body Scanning
	if bodyScanning, ok := params.Settings["body_scanning"].(map[string]any); ok {
		bs := &cloudflare.TeamsBodyScanning{}
		if v, ok := bodyScanning["inspection_mode"].(string); ok {
			bs.InspectionMode = v
		}
		accountSettings.BodyScanning = bs
	}

	// Parse Browser Isolation
	if browserIsolation, ok := params.Settings["browser_isolation"].(map[string]any); ok {
		bi := &cloudflare.BrowserIsolation{}
		if v, ok := browserIsolation["url_browser_isolation_enabled"].(bool); ok {
			bi.UrlBrowserIsolationEnabled = &v
		}
		if v, ok := browserIsolation["non_identity_enabled"].(bool); ok {
			bi.NonIdentityEnabled = &v
		}
		accountSettings.BrowserIsolation = bi
	}

	// Parse FIPS
	if fips, ok := params.Settings["fips"].(map[string]any); ok {
		f := &cloudflare.TeamsFIPS{}
		if v, ok := fips["tls"].(bool); ok {
			f.TLS = v
		}
		accountSettings.FIPS = f
	}

	// Parse Protocol Detection
	if protocolDetection, ok := params.Settings["protocol_detection"].(map[string]any); ok {
		pd := &cloudflare.TeamsProtocolDetection{}
		if v, ok := protocolDetection["enabled"].(bool); ok {
			pd.Enabled = v
		}
		accountSettings.ProtocolDetection = pd
	}

	// Parse Custom Certificate
	if customCertificate, ok := params.Settings["custom_certificate"].(map[string]any); ok {
		cc := &cloudflare.TeamsCustomCertificate{}
		if v, ok := customCertificate["enabled"].(bool); ok {
			cc.Enabled = &v
		}
		if v, ok := customCertificate["id"].(string); ok {
			cc.ID = v
		}
		accountSettings.CustomCertificate = cc
	}

	// Build the final configuration
	settings := cloudflare.TeamsConfiguration{
		Settings: accountSettings,
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
