// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf_test

import (
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf/mock"
)

func TestCloudflareClientInterface(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock.NewMockCloudflareClient(ctrl)

	// Test that mock implements the interface
	var _ cf.CloudflareClient = mockClient

	t.Log("Mock client successfully implements CloudflareClient interface")
}

func TestCreateTunnel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock.NewMockCloudflareClient(ctrl)

	// Setup expectations
	expectedTunnelID := "test-tunnel-id-12345"
	expectedCreds := `{"AccountTag":"account-123","TunnelID":"test-tunnel-id-12345","TunnelName":"test-tunnel","TunnelSecret":"secret123"}`

	mockClient.EXPECT().
		CreateTunnel().
		Return(expectedTunnelID, expectedCreds, nil).
		Times(1)

	// Execute
	tunnelID, creds, err := mockClient.CreateTunnel()

	// Assert
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if tunnelID != expectedTunnelID {
		t.Errorf("Expected tunnel ID %s, got %s", expectedTunnelID, tunnelID)
	}
	if creds != expectedCreds {
		t.Errorf("Expected creds %s, got %s", expectedCreds, creds)
	}
}

func TestDeleteTunnel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock.NewMockCloudflareClient(ctrl)

	mockClient.EXPECT().
		DeleteTunnel().
		Return(nil).
		Times(1)

	err := mockClient.DeleteTunnel()

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestCreateVirtualNetwork(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock.NewMockCloudflareClient(ctrl)

	params := cf.VirtualNetworkParams{
		Name:             "test-vnet",
		Comment:          "Test virtual network",
		IsDefaultNetwork: false,
	}

	expectedResult := &cf.VirtualNetworkResult{
		ID:               "vnet-12345",
		Name:             "test-vnet",
		Comment:          "Test virtual network",
		IsDefaultNetwork: false,
	}

	mockClient.EXPECT().
		CreateVirtualNetwork(params).
		Return(expectedResult, nil).
		Times(1)

	result, err := mockClient.CreateVirtualNetwork(params)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result.ID != expectedResult.ID {
		t.Errorf("Expected ID %s, got %s", expectedResult.ID, result.ID)
	}
	if result.Name != expectedResult.Name {
		t.Errorf("Expected Name %s, got %s", expectedResult.Name, result.Name)
	}
}

func TestCreateTunnelRoute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock.NewMockCloudflareClient(ctrl)

	params := cf.TunnelRouteParams{
		Network:          "10.0.0.0/8",
		TunnelID:         "tunnel-123",
		VirtualNetworkID: "vnet-123",
		Comment:          "Private network route",
	}

	expectedResult := &cf.TunnelRouteResult{
		Network:          "10.0.0.0/8",
		TunnelID:         "tunnel-123",
		TunnelName:       "my-tunnel",
		VirtualNetworkID: "vnet-123",
		Comment:          "Private network route",
	}

	mockClient.EXPECT().
		CreateTunnelRoute(params).
		Return(expectedResult, nil).
		Times(1)

	result, err := mockClient.CreateTunnelRoute(params)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result.Network != expectedResult.Network {
		t.Errorf("Expected Network %s, got %s", expectedResult.Network, result.Network)
	}
}

func TestDNSOperations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock.NewMockCloudflareClient(ctrl)

	// Test InsertOrUpdateCName
	mockClient.EXPECT().
		InsertOrUpdateCName("app.example.com", "").
		Return("dns-record-123", nil).
		Times(1)

	dnsID, err := mockClient.InsertOrUpdateCName("app.example.com", "")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if dnsID != "dns-record-123" {
		t.Errorf("Expected DNS ID dns-record-123, got %s", dnsID)
	}

	// Test DeleteDNSId
	mockClient.EXPECT().
		DeleteDNSId("app.example.com", "dns-record-123", true).
		Return(nil).
		Times(1)

	err = mockClient.DeleteDNSId("app.example.com", "dns-record-123", true)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestAccessApplication(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock.NewMockCloudflareClient(ctrl)

	params := cf.AccessApplicationParams{
		Name:            "My App",
		Domain:          "app.example.com",
		Type:            "self_hosted",
		SessionDuration: "24h",
	}

	expectedResult := &cf.AccessApplicationResult{
		ID:              "app-123",
		AUD:             "aud-456",
		Name:            "My App",
		Domain:          "app.example.com",
		Type:            "self_hosted",
		SessionDuration: "24h",
	}

	mockClient.EXPECT().
		CreateAccessApplication(params).
		Return(expectedResult, nil).
		Times(1)

	result, err := mockClient.CreateAccessApplication(params)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result.ID != expectedResult.ID {
		t.Errorf("Expected ID %s, got %s", expectedResult.ID, result.ID)
	}
}

func TestGatewayRule(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock.NewMockCloudflareClient(ctrl)

	params := cf.GatewayRuleParams{
		Name:        "Block Malware",
		Description: "Block known malware domains",
		Precedence:  1,
		Enabled:     true,
		Action:      "block",
		Traffic:     "dns",
	}

	expectedResult := &cf.GatewayRuleResult{
		ID:          "rule-123",
		Name:        "Block Malware",
		Description: "Block known malware domains",
		Precedence:  1,
		Enabled:     true,
		Action:      "block",
	}

	mockClient.EXPECT().
		CreateGatewayRule(params).
		Return(expectedResult, nil).
		Times(1)

	result, err := mockClient.CreateGatewayRule(params)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result.ID != expectedResult.ID {
		t.Errorf("Expected ID %s, got %s", expectedResult.ID, result.ID)
	}
}

func TestSplitTunnel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock.NewMockCloudflareClient(ctrl)

	entries := []cf.SplitTunnelEntry{
		{Address: "10.0.0.0/8", Description: "Private network"},
		{Address: "192.168.0.0/16", Description: "Local network"},
	}

	mockClient.EXPECT().
		GetSplitTunnelExclude().
		Return(entries, nil).
		Times(1)

	mockClient.EXPECT().
		UpdateSplitTunnelExclude(entries).
		Return(nil).
		Times(1)

	result, err := mockClient.GetSplitTunnelExclude()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if len(result) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(result))
	}

	err = mockClient.UpdateSplitTunnelExclude(entries)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestWARPConnector(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock.NewMockCloudflareClient(ctrl)

	expectedResult := &cf.WARPConnectorResult{
		ID:          "connector-123",
		TunnelID:    "tunnel-123",
		TunnelToken: "token-abc",
		Name:        "my-connector",
	}

	mockClient.EXPECT().
		CreateWARPConnector("my-connector").
		Return(expectedResult, nil).
		Times(1)

	result, err := mockClient.CreateWARPConnector("my-connector")

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result.ID != expectedResult.ID {
		t.Errorf("Expected ID %s, got %s", expectedResult.ID, result.ID)
	}
}
