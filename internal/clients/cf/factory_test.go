// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf_test

import (
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"go.uber.org/mock/gomock"

	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf/mock"
)

func TestDefaultClientFactory_NewClient_WithAPIToken(t *testing.T) {
	factory := cf.NewDefaultClientFactory()

	config := cf.ClientConfig{
		Log:         logr.Discard(),
		APIToken:    "test-api-token",
		AccountID:   "account-123",
		AccountName: "Test Account",
		Domain:      "example.com",
	}

	client, err := factory.NewClient(config)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if client == nil {
		t.Error("Expected client to be non-nil")
	}
}

func TestDefaultClientFactory_NewClient_WithAPIKey(t *testing.T) {
	factory := cf.NewDefaultClientFactory()

	config := cf.ClientConfig{
		Log:         logr.Discard(),
		APIKey:      "test-api-key",
		Email:       "test@example.com",
		AccountID:   "account-123",
		AccountName: "Test Account",
		Domain:      "example.com",
	}

	client, err := factory.NewClient(config)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if client == nil {
		t.Error("Expected client to be non-nil")
	}
}

func TestDefaultClientFactory_NewClient_NoCredentials(t *testing.T) {
	factory := cf.NewDefaultClientFactory()

	config := cf.ClientConfig{
		Log:         logr.Discard(),
		AccountID:   "account-123",
		AccountName: "Test Account",
		Domain:      "example.com",
	}

	client, err := factory.NewClient(config)

	if err == nil {
		t.Error("Expected error for missing credentials")
	}
	if client != nil {
		t.Error("Expected client to be nil when error occurs")
	}
	if !errors.Is(err, cf.ErrNoCredentials) {
		t.Errorf("Expected ErrNoCredentials, got %v", err)
	}
}

func TestMockClientFactory(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	factory := mock.NewMockClientFactory(ctrl)
	mockClient := factory.GetMockClient()

	// Setup expectations
	mockClient.EXPECT().
		GetAccountId().
		Return("mock-account-id", nil).
		AnyTimes()

	config := cf.ClientConfig{
		Log:       logr.Discard(),
		APIToken:  "test-token",
		AccountID: "account-123",
	}

	client, err := factory.NewClient(config)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if client == nil {
		t.Error("Expected client to be non-nil")
	}

	// Verify the mock client works
	accountID, err := client.GetAccountId()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if accountID != "mock-account-id" {
		t.Errorf("Expected mock-account-id, got %s", accountID)
	}
}

func TestMockClientFactoryWithError(t *testing.T) {
	expectedError := errors.New("factory error")
	factory := &mock.MockClientFactoryWithError{
		Err: expectedError,
	}

	config := cf.ClientConfig{
		Log:      logr.Discard(),
		APIToken: "test-token",
	}

	client, err := factory.NewClient(config)

	if err == nil {
		t.Error("Expected error from factory")
	}
	if !errors.Is(err, expectedError) {
		t.Errorf("Expected %v, got %v", expectedError, err)
	}
	if client != nil {
		t.Error("Expected client to be nil")
	}
}

func TestGlobalDefaultFactory(t *testing.T) {
	// Get the default factory
	factory := cf.GetDefaultFactory()
	if factory == nil {
		t.Error("Expected default factory to be non-nil")
	}

	// Create a mock factory
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFactory := mock.NewMockClientFactory(ctrl)

	// Set the mock factory as default
	cf.SetDefaultFactory(mockFactory)

	// Verify it was set
	if cf.GetDefaultFactory() != mockFactory {
		t.Error("Expected mock factory to be set as default")
	}

	// Reset to default
	cf.ResetDefaultFactory()

	// Verify it was reset
	if cf.GetDefaultFactory() == mockFactory {
		t.Error("Expected default factory to be reset")
	}
}

func TestMockClientFactoryWithCustomClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	customMock := mock.NewMockCloudflareClient(ctrl)

	factory := &mock.MockClientFactoryWithCustomClient{
		Client: customMock,
	}

	config := cf.ClientConfig{
		Log:      logr.Discard(),
		APIToken: "test-token",
	}

	client, err := factory.NewClient(config)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if client != customMock {
		t.Error("Expected custom mock client")
	}
}
