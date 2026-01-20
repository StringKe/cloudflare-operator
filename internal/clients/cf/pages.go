// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

//nolint:revive // max-public-structs: Pages API requires multiple struct types for configuration
package cf

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"time"

	"github.com/cloudflare/cloudflare-go"
)

// PagesProjectParams contains parameters for creating or updating a Pages project
type PagesProjectParams struct {
	Name             string
	ProductionBranch string
	Source           *PagesSourceConfig
	BuildConfig      *PagesBuildConfig
	DeploymentConfig *PagesDeploymentConfigs
}

// PagesSourceConfig defines the source configuration
type PagesSourceConfig struct {
	Type   string
	GitHub *PagesGitHubConfig
	GitLab *PagesGitLabConfig
}

// PagesGitHubConfig defines GitHub source configuration
type PagesGitHubConfig struct {
	Owner                        string
	Repo                         string
	ProductionDeploymentsEnabled *bool
	PreviewDeploymentsEnabled    *bool
	PRCommentsEnabled            *bool
	DeploymentsEnabled           *bool
}

// PagesGitLabConfig defines GitLab source configuration
type PagesGitLabConfig struct {
	Owner                        string
	Repo                         string
	ProductionDeploymentsEnabled *bool
	PreviewDeploymentsEnabled    *bool
	DeploymentsEnabled           *bool
}

// PagesBuildConfig defines build configuration
type PagesBuildConfig struct {
	BuildCommand      string
	DestinationDir    string
	RootDir           string
	BuildCaching      *bool
	WebAnalyticsTag   string
	WebAnalyticsToken string
}

// PagesDeploymentConfigs contains preview and production configs
type PagesDeploymentConfigs struct {
	Preview    *PagesDeploymentEnvConfig
	Production *PagesDeploymentEnvConfig
}

// PagesDeploymentEnvConfig defines environment-specific configuration
type PagesDeploymentEnvConfig struct {
	EnvironmentVariables    map[string]PagesEnvVar
	CompatibilityDate       string
	CompatibilityFlags      []string
	D1Bindings              map[string]string // name -> databaseID
	KVBindings              map[string]string // name -> namespaceID
	R2Bindings              map[string]string // name -> bucketName
	ServiceBindings         map[string]PagesServiceBindingConfig
	DurableObjectBindings   map[string]PagesDurableObjectBindingConfig
	QueueBindings           map[string]string // name -> queueName
	AIBindings              []string          // binding names
	VectorizeBindings       map[string]string // name -> indexName
	HyperdriveBindings      map[string]string // name -> configID
	MTLSCertificates        map[string]string // name -> certificateID
	BrowserBinding          string            // binding name
	PlacementMode           string
	UsageModel              string
	FailOpen                *bool
	AlwaysUseLatestCompDate *bool
}

// PagesEnvVar defines an environment variable
type PagesEnvVar struct {
	Value string
	Type  string // "plain_text" or "secret_text"
}

// PagesServiceBindingConfig defines a service binding
type PagesServiceBindingConfig struct {
	Service     string
	Environment string
}

// PagesDurableObjectBindingConfig defines a Durable Object binding
type PagesDurableObjectBindingConfig struct {
	ClassName       string
	ScriptName      string
	EnvironmentName string
}

// PagesProjectResult contains the result of a Pages project operation
type PagesProjectResult struct {
	ID               string
	Name             string
	Subdomain        string
	Domains          []string
	ProductionBranch string
	CreatedOn        time.Time
	Source           *PagesSourceConfig
	BuildConfig      *PagesBuildConfig
	LatestDeployment *PagesDeploymentResult
}

// PagesDeploymentResult contains the result of a Pages deployment operation
type PagesDeploymentResult struct {
	ID               string
	ShortID          string
	ProjectID        string
	ProjectName      string
	Environment      string
	URL              string
	ProductionBranch string
	CreatedOn        time.Time
	ModifiedOn       time.Time
	Stage            string
	StageStatus      string
	Stages           []PagesDeploymentStage
}

// PagesDeploymentStage represents a deployment stage
type PagesDeploymentStage struct {
	Name      string
	StartedOn string
	EndedOn   string
	Status    string
}

// PagesDomainResult contains the result of a Pages domain operation
type PagesDomainResult struct {
	ID               string
	Name             string
	Status           string
	ZoneTag          string
	ValidationMethod string
	ValidationStatus string
	CreatedOn        time.Time
}

// PagesDeploymentLogsResult contains deployment logs
type PagesDeploymentLogsResult struct {
	Total            int
	IncludesContents bool
	Data             []PagesDeploymentLogEntry
}

// PagesDeploymentLogEntry represents a log entry
type PagesDeploymentLogEntry struct {
	Timestamp time.Time
	Message   string
}

// CreatePagesProject creates a new Pages project
func (api *API) CreatePagesProject(ctx context.Context, params PagesProjectParams) (*PagesProjectResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	rc := cloudflare.AccountIdentifier(accountID)
	createParams := convertToPagesProjectParams(params)

	project, err := api.CloudflareClient.CreatePagesProject(ctx, rc, createParams)
	if err != nil {
		return nil, fmt.Errorf("failed to create Pages project: %w", err)
	}

	return convertFromPagesProject(project), nil
}

// GetPagesProject retrieves a Pages project by name
func (api *API) GetPagesProject(ctx context.Context, projectName string) (*PagesProjectResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	rc := cloudflare.AccountIdentifier(accountID)
	project, err := api.CloudflareClient.GetPagesProject(ctx, rc, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get Pages project: %w", err)
	}

	return convertFromPagesProject(project), nil
}

// UpdatePagesProject updates an existing Pages project
func (api *API) UpdatePagesProject(ctx context.Context, projectName string, params PagesProjectParams) (*PagesProjectResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	// Build update params
	updateParams := cloudflare.UpdatePagesProjectParams{
		Name:             params.Name,
		ProductionBranch: params.ProductionBranch,
	}

	if params.BuildConfig != nil {
		updateParams.BuildConfig = cloudflare.PagesProjectBuildConfig{
			BuildCommand:      params.BuildConfig.BuildCommand,
			DestinationDir:    params.BuildConfig.DestinationDir,
			RootDir:           params.BuildConfig.RootDir,
			BuildCaching:      params.BuildConfig.BuildCaching,
			WebAnalyticsTag:   params.BuildConfig.WebAnalyticsTag,
			WebAnalyticsToken: params.BuildConfig.WebAnalyticsToken,
		}
	}

	if params.DeploymentConfig != nil {
		updateParams.DeploymentConfigs = convertToDeploymentConfigs(params.DeploymentConfig)
	}

	if params.Source != nil {
		updateParams.Source = convertToPagesSource(params.Source)
	}

	// Use Raw API since UpdatePagesProject may not be directly available
	endpoint := fmt.Sprintf("/accounts/%s/pages/projects/%s", accountID, projectName)
	resp, err := api.CloudflareClient.Raw(ctx, "PATCH", endpoint, updateParams, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to update Pages project: %w", err)
	}

	var project cloudflare.PagesProject
	if err := jsonUnmarshal(resp.Result, &project); err != nil {
		return nil, fmt.Errorf("failed to parse update response: %w", err)
	}

	return convertFromPagesProject(project), nil
}

// DeletePagesProject deletes a Pages project
func (api *API) DeletePagesProject(ctx context.Context, projectName string) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}

	rc := cloudflare.AccountIdentifier(accountID)
	if err := api.CloudflareClient.DeletePagesProject(ctx, rc, projectName); err != nil {
		if IsNotFoundError(err) {
			api.Log.Info("Pages project already deleted (not found)", "project", projectName)
			return nil
		}
		return fmt.Errorf("failed to delete Pages project: %w", err)
	}

	api.Log.Info("Pages project deleted", "project", projectName)
	return nil
}

// ListPagesProjects lists all Pages projects
func (api *API) ListPagesProjects(ctx context.Context) ([]PagesProjectResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	rc := cloudflare.AccountIdentifier(accountID)
	projects, _, err := api.CloudflareClient.ListPagesProjects(ctx, rc, cloudflare.ListPagesProjectsParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list Pages projects: %w", err)
	}

	results := make([]PagesProjectResult, len(projects))
	for i, p := range projects {
		results[i] = *convertFromPagesProject(p)
	}

	return results, nil
}

// PurgePagesProjectBuildCache purges the build cache for a Pages project
func (api *API) PurgePagesProjectBuildCache(ctx context.Context, projectName string) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}

	endpoint := fmt.Sprintf("/accounts/%s/pages/projects/%s/purge_build_cache", accountID, projectName)
	if _, err := api.CloudflareClient.Raw(ctx, "POST", endpoint, nil, nil); err != nil {
		return fmt.Errorf("failed to purge build cache: %w", err)
	}

	api.Log.Info("Pages build cache purged", "project", projectName)
	return nil
}

// AddPagesDomain adds a custom domain to a Pages project
func (api *API) AddPagesDomain(ctx context.Context, projectName, domain string) (*PagesDomainResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	params := cloudflare.PagesDomainParameters{
		AccountID:   accountID,
		ProjectName: projectName,
		DomainName:  domain,
	}

	domainResult, err := api.CloudflareClient.PagesAddDomain(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to add Pages domain: %w", err)
	}

	return convertFromPagesDomain(domainResult), nil
}

// GetPagesDomain gets a custom domain from a Pages project
func (api *API) GetPagesDomain(ctx context.Context, projectName, domain string) (*PagesDomainResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	params := cloudflare.PagesDomainParameters{
		AccountID:   accountID,
		ProjectName: projectName,
		DomainName:  domain,
	}

	domainResult, err := api.CloudflareClient.GetPagesDomain(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get Pages domain: %w", err)
	}

	return convertFromPagesDomain(domainResult), nil
}

// DeletePagesDomain removes a custom domain from a Pages project
func (api *API) DeletePagesDomain(ctx context.Context, projectName, domain string) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}

	params := cloudflare.PagesDomainParameters{
		AccountID:   accountID,
		ProjectName: projectName,
		DomainName:  domain,
	}

	if err := api.CloudflareClient.PagesDeleteDomain(ctx, params); err != nil {
		if IsNotFoundError(err) {
			api.Log.Info("Pages domain already deleted (not found)", "project", projectName, "domain", domain)
			return nil
		}
		return fmt.Errorf("failed to delete Pages domain: %w", err)
	}

	api.Log.Info("Pages domain deleted", "project", projectName, "domain", domain)
	return nil
}

// PatchPagesDomain updates a custom domain on a Pages project
func (api *API) PatchPagesDomain(ctx context.Context, projectName, domain string) (*PagesDomainResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	params := cloudflare.PagesDomainParameters{
		AccountID:   accountID,
		ProjectName: projectName,
		DomainName:  domain,
	}

	domainResult, err := api.CloudflareClient.PagesPatchDomain(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to patch Pages domain: %w", err)
	}

	return convertFromPagesDomain(domainResult), nil
}

// ListPagesDomains lists all custom domains for a Pages project
func (api *API) ListPagesDomains(ctx context.Context, projectName string) ([]PagesDomainResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	params := cloudflare.PagesDomainsParameters{
		AccountID:   accountID,
		ProjectName: projectName,
	}

	domains, err := api.CloudflareClient.GetPagesDomains(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list Pages domains: %w", err)
	}

	results := make([]PagesDomainResult, len(domains))
	for i, d := range domains {
		results[i] = *convertFromPagesDomain(d)
	}

	return results, nil
}

// CreatePagesDeployment creates a new deployment for a Pages project
func (api *API) CreatePagesDeployment(ctx context.Context, projectName string, branch string) (*PagesDeploymentResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	rc := cloudflare.AccountIdentifier(accountID)
	params := cloudflare.CreatePagesDeploymentParams{
		ProjectName: projectName,
		Branch:      branch,
	}

	deployment, err := api.CloudflareClient.CreatePagesDeployment(ctx, rc, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create Pages deployment: %w", err)
	}

	return convertFromPagesDeployment(deployment), nil
}

// GetPagesDeployment gets a deployment from a Pages project
func (api *API) GetPagesDeployment(ctx context.Context, projectName, deploymentID string) (*PagesDeploymentResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	rc := cloudflare.AccountIdentifier(accountID)
	deployment, err := api.CloudflareClient.GetPagesDeploymentInfo(ctx, rc, projectName, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get Pages deployment: %w", err)
	}

	return convertFromPagesDeployment(deployment), nil
}

// DeletePagesDeployment deletes a deployment from a Pages project
func (api *API) DeletePagesDeployment(ctx context.Context, projectName, deploymentID string) error {
	if api.CloudflareClient == nil {
		return errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}

	rc := cloudflare.AccountIdentifier(accountID)
	params := cloudflare.DeletePagesDeploymentParams{
		ProjectName:  projectName,
		DeploymentID: deploymentID,
	}

	if err := api.CloudflareClient.DeletePagesDeployment(ctx, rc, params); err != nil {
		if IsNotFoundError(err) {
			api.Log.Info("Pages deployment already deleted (not found)", "project", projectName, "deployment", deploymentID)
			return nil
		}
		return fmt.Errorf("failed to delete Pages deployment: %w", err)
	}

	api.Log.Info("Pages deployment deleted", "project", projectName, "deployment", deploymentID)
	return nil
}

// ListPagesDeployments lists all deployments for a Pages project
func (api *API) ListPagesDeployments(ctx context.Context, projectName string) ([]PagesDeploymentResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	rc := cloudflare.AccountIdentifier(accountID)
	params := cloudflare.ListPagesDeploymentsParams{
		ProjectName: projectName,
	}

	deployments, _, err := api.CloudflareClient.ListPagesDeployments(ctx, rc, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list Pages deployments: %w", err)
	}

	results := make([]PagesDeploymentResult, len(deployments))
	for i, d := range deployments {
		results[i] = *convertFromPagesDeployment(d)
	}

	return results, nil
}

// RetryPagesDeployment retries a failed deployment
func (api *API) RetryPagesDeployment(ctx context.Context, projectName, deploymentID string) (*PagesDeploymentResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	rc := cloudflare.AccountIdentifier(accountID)
	deployment, err := api.CloudflareClient.RetryPagesDeployment(ctx, rc, projectName, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to retry Pages deployment: %w", err)
	}

	return convertFromPagesDeployment(deployment), nil
}

// RollbackPagesDeployment rolls back to a previous deployment
func (api *API) RollbackPagesDeployment(ctx context.Context, projectName, deploymentID string) (*PagesDeploymentResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	rc := cloudflare.AccountIdentifier(accountID)
	deployment, err := api.CloudflareClient.RollbackPagesDeployment(ctx, rc, projectName, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to rollback Pages deployment: %w", err)
	}

	return convertFromPagesDeployment(deployment), nil
}

// GetPagesDeploymentLogs gets the logs for a deployment
func (api *API) GetPagesDeploymentLogs(ctx context.Context, projectName, deploymentID string) (*PagesDeploymentLogsResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	rc := cloudflare.AccountIdentifier(accountID)
	params := cloudflare.GetPagesDeploymentLogsParams{
		ProjectName:  projectName,
		DeploymentID: deploymentID,
	}

	logs, err := api.CloudflareClient.GetPagesDeploymentLogs(ctx, rc, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get Pages deployment logs: %w", err)
	}

	result := &PagesDeploymentLogsResult{
		Total:            logs.Total,
		IncludesContents: logs.IncludesContainerLogs,
	}

	for _, entry := range logs.Data {
		var ts time.Time
		if entry.Timestamp != nil {
			ts = *entry.Timestamp
		}
		result.Data = append(result.Data, PagesDeploymentLogEntry{
			Timestamp: ts,
			Message:   entry.Line,
		})
	}

	return result, nil
}

// Helper functions for converting between SDK types and our types

func convertToPagesProjectParams(params PagesProjectParams) cloudflare.CreatePagesProjectParams {
	result := cloudflare.CreatePagesProjectParams{
		Name:             params.Name,
		ProductionBranch: params.ProductionBranch,
	}

	if params.BuildConfig != nil {
		result.BuildConfig = cloudflare.PagesProjectBuildConfig{
			BuildCommand:      params.BuildConfig.BuildCommand,
			DestinationDir:    params.BuildConfig.DestinationDir,
			RootDir:           params.BuildConfig.RootDir,
			BuildCaching:      params.BuildConfig.BuildCaching,
			WebAnalyticsTag:   params.BuildConfig.WebAnalyticsTag,
			WebAnalyticsToken: params.BuildConfig.WebAnalyticsToken,
		}
	}

	if params.DeploymentConfig != nil {
		result.DeploymentConfigs = convertToDeploymentConfigs(params.DeploymentConfig)
	}

	if params.Source != nil {
		result.Source = convertToPagesSource(params.Source)
	}

	return result
}

func convertToPagesSource(source *PagesSourceConfig) *cloudflare.PagesProjectSource {
	if source == nil {
		return nil
	}

	result := &cloudflare.PagesProjectSource{
		Type: source.Type,
	}

	if source.GitHub != nil {
		cfg := &cloudflare.PagesProjectSourceConfig{
			Owner:    source.GitHub.Owner,
			RepoName: source.GitHub.Repo,
		}
		if source.GitHub.ProductionDeploymentsEnabled != nil {
			cfg.ProductionDeploymentsEnabled = *source.GitHub.ProductionDeploymentsEnabled
		}
		if source.GitHub.PRCommentsEnabled != nil {
			cfg.PRCommentsEnabled = *source.GitHub.PRCommentsEnabled
		}
		if source.GitHub.DeploymentsEnabled != nil {
			cfg.DeploymentsEnabled = *source.GitHub.DeploymentsEnabled
		}
		// Map PreviewDeploymentsEnabled to PreviewDeploymentSetting
		if source.GitHub.PreviewDeploymentsEnabled != nil {
			if *source.GitHub.PreviewDeploymentsEnabled {
				cfg.PreviewDeploymentSetting = cloudflare.PagesPreviewAllBranches
			} else {
				cfg.PreviewDeploymentSetting = cloudflare.PagesPreviewNoBranches
			}
		}
		result.Config = cfg
	} else if source.GitLab != nil {
		cfg := &cloudflare.PagesProjectSourceConfig{
			Owner:    source.GitLab.Owner,
			RepoName: source.GitLab.Repo,
		}
		if source.GitLab.ProductionDeploymentsEnabled != nil {
			cfg.ProductionDeploymentsEnabled = *source.GitLab.ProductionDeploymentsEnabled
		}
		if source.GitLab.DeploymentsEnabled != nil {
			cfg.DeploymentsEnabled = *source.GitLab.DeploymentsEnabled
		}
		// Map PreviewDeploymentsEnabled to PreviewDeploymentSetting
		if source.GitLab.PreviewDeploymentsEnabled != nil {
			if *source.GitLab.PreviewDeploymentsEnabled {
				cfg.PreviewDeploymentSetting = cloudflare.PagesPreviewAllBranches
			} else {
				cfg.PreviewDeploymentSetting = cloudflare.PagesPreviewNoBranches
			}
		}
		result.Config = cfg
	}

	return result
}

func convertToDeploymentConfigs(config *PagesDeploymentConfigs) cloudflare.PagesProjectDeploymentConfigs {
	result := cloudflare.PagesProjectDeploymentConfigs{}

	if config.Preview != nil {
		result.Preview = convertToDeploymentEnvConfig(config.Preview)
	}

	if config.Production != nil {
		result.Production = convertToDeploymentEnvConfig(config.Production)
	}

	return result
}

func convertToDeploymentEnvConfig(config *PagesDeploymentEnvConfig) cloudflare.PagesProjectDeploymentConfigEnvironment {
	result := cloudflare.PagesProjectDeploymentConfigEnvironment{
		CompatibilityDate:  config.CompatibilityDate,
		CompatibilityFlags: config.CompatibilityFlags,
	}

	// Convert environment variables
	if len(config.EnvironmentVariables) > 0 {
		result.EnvVars = make(cloudflare.EnvironmentVariableMap)
		for name, envVar := range config.EnvironmentVariables {
			result.EnvVars[name] = &cloudflare.EnvironmentVariable{
				Value: envVar.Value,
				Type:  cloudflare.EnvVarType(envVar.Type),
			}
		}
	}

	// Convert D1 bindings
	if len(config.D1Bindings) > 0 {
		result.D1Databases = make(cloudflare.D1BindingMap)
		for name, dbID := range config.D1Bindings {
			result.D1Databases[name] = &cloudflare.D1Binding{ID: dbID}
		}
	}

	// Convert KV bindings
	if len(config.KVBindings) > 0 {
		result.KvNamespaces = make(cloudflare.NamespaceBindingMap)
		for name, nsID := range config.KVBindings {
			result.KvNamespaces[name] = &cloudflare.NamespaceBindingValue{Value: nsID}
		}
	}

	// Convert R2 bindings
	if len(config.R2Bindings) > 0 {
		result.R2Bindings = make(cloudflare.R2BindingMap)
		for name, bucket := range config.R2Bindings {
			result.R2Bindings[name] = &cloudflare.R2BindingValue{Name: bucket}
		}
	}

	// Convert service bindings
	if len(config.ServiceBindings) > 0 {
		result.ServiceBindings = make(cloudflare.ServiceBindingMap)
		for name, binding := range config.ServiceBindings {
			result.ServiceBindings[name] = &cloudflare.ServiceBinding{
				Service:     binding.Service,
				Environment: binding.Environment,
			}
		}
	}

	// Set placement if configured
	if config.PlacementMode != "" {
		result.Placement = &cloudflare.Placement{
			Mode: cloudflare.PlacementMode(config.PlacementMode),
		}
	}

	// Set usage model
	if config.UsageModel != "" {
		result.UsageModel = cloudflare.UsageModel(config.UsageModel)
	}

	// Handle optional bool fields
	if config.FailOpen != nil {
		result.FailOpen = *config.FailOpen
	}
	if config.AlwaysUseLatestCompDate != nil {
		result.AlwaysUseLatestCompatibilityDate = *config.AlwaysUseLatestCompDate
	}

	return result
}

func convertFromPagesProject(project cloudflare.PagesProject) *PagesProjectResult {
	result := &PagesProjectResult{
		ID:               project.ID,
		Name:             project.Name,
		Subdomain:        project.SubDomain,
		Domains:          project.Domains,
		ProductionBranch: project.ProductionBranch,
	}

	if project.CreatedOn != nil {
		result.CreatedOn = *project.CreatedOn
	}

	// Convert source
	if project.Source != nil {
		result.Source = &PagesSourceConfig{
			Type: project.Source.Type,
		}
		if project.Source.Config != nil {
			// Map PreviewDeploymentSetting to PreviewDeploymentsEnabled
			previewEnabled := project.Source.Config.PreviewDeploymentSetting == cloudflare.PagesPreviewAllBranches ||
				project.Source.Config.PreviewDeploymentSetting == cloudflare.PagesPreviewCustomBranches

			switch project.Source.Type {
			case "github":
				prodEnabled := project.Source.Config.ProductionDeploymentsEnabled
				prCommentsEnabled := project.Source.Config.PRCommentsEnabled
				deploymentsEnabled := project.Source.Config.DeploymentsEnabled
				result.Source.GitHub = &PagesGitHubConfig{
					Owner:                        project.Source.Config.Owner,
					Repo:                         project.Source.Config.RepoName,
					ProductionDeploymentsEnabled: &prodEnabled,
					PreviewDeploymentsEnabled:    &previewEnabled,
					PRCommentsEnabled:            &prCommentsEnabled,
					DeploymentsEnabled:           &deploymentsEnabled,
				}
			case "gitlab":
				prodEnabled := project.Source.Config.ProductionDeploymentsEnabled
				deploymentsEnabled := project.Source.Config.DeploymentsEnabled
				result.Source.GitLab = &PagesGitLabConfig{
					Owner:                        project.Source.Config.Owner,
					Repo:                         project.Source.Config.RepoName,
					ProductionDeploymentsEnabled: &prodEnabled,
					PreviewDeploymentsEnabled:    &previewEnabled,
					DeploymentsEnabled:           &deploymentsEnabled,
				}
			}
		}
	}

	// Convert build config
	result.BuildConfig = &PagesBuildConfig{
		BuildCommand:      project.BuildConfig.BuildCommand,
		DestinationDir:    project.BuildConfig.DestinationDir,
		RootDir:           project.BuildConfig.RootDir,
		BuildCaching:      project.BuildConfig.BuildCaching,
		WebAnalyticsTag:   project.BuildConfig.WebAnalyticsTag,
		WebAnalyticsToken: project.BuildConfig.WebAnalyticsToken,
	}

	// Convert latest deployment
	if project.LatestDeployment.ID != "" {
		result.LatestDeployment = convertFromPagesDeployment(project.LatestDeployment)
	}

	return result
}

func convertFromPagesDomain(domain cloudflare.PagesDomain) *PagesDomainResult {
	result := &PagesDomainResult{
		ID:      domain.ID,
		Name:    domain.Name,
		Status:  domain.Status,
		ZoneTag: domain.ZoneTag,
	}

	if domain.CreatedOn != nil {
		result.CreatedOn = *domain.CreatedOn
	}

	return result
}

func convertFromPagesDeployment(deployment cloudflare.PagesProjectDeployment) *PagesDeploymentResult {
	result := &PagesDeploymentResult{
		ID:               deployment.ID,
		ShortID:          deployment.ShortID,
		ProjectID:        deployment.ProjectID,
		ProjectName:      deployment.ProjectName,
		Environment:      deployment.Environment,
		URL:              deployment.URL,
		ProductionBranch: deployment.ProductionBranch,
		Stage:            deployment.LatestStage.Name,
		StageStatus:      deployment.LatestStage.Status,
	}

	if deployment.CreatedOn != nil {
		result.CreatedOn = *deployment.CreatedOn
	}
	if deployment.ModifiedOn != nil {
		result.ModifiedOn = *deployment.ModifiedOn
	}

	// Convert stages
	for _, stage := range deployment.Stages {
		s := PagesDeploymentStage{
			Name:   stage.Name,
			Status: stage.Status,
		}
		if stage.StartedOn != nil {
			s.StartedOn = stage.StartedOn.Format(time.RFC3339)
		}
		if stage.EndedOn != nil {
			s.EndedOn = stage.EndedOn.Format(time.RFC3339)
		}
		result.Stages = append(result.Stages, s)
	}

	return result
}

// =============================================================================
// Direct Upload API
// =============================================================================

// PagesDirectUploadResult contains the result of a direct upload deployment.
type PagesDirectUploadResult struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Stage string `json:"stage"`
}

// CreatePagesDirectUploadDeployment creates a deployment via direct upload.
// This uses the Pages Direct Upload API with multipart/form-data format.
// The API expects: manifest (JSON object mapping file paths to hashes) + file contents.
//
//nolint:revive // cognitive complexity is acceptable for multi-step upload process
func (api *API) CreatePagesDirectUploadDeployment(
	ctx context.Context,
	projectName string,
	files map[string][]byte,
) (*PagesDirectUploadResult, error) {
	if api.CloudflareClient == nil {
		return nil, errClientNotInitialized
	}

	accountID, err := api.GetAccountId(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID: %w", err)
	}

	api.Log.Info("Creating direct upload deployment",
		"project", projectName,
		"fileCount", len(files))

	// Build manifest: map of file paths to their SHA256 hashes
	manifest := make(map[string]string, len(files))
	for path, content := range files {
		hash := sha256.Sum256(content)
		// Ensure path starts with /
		key := path
		if len(key) == 0 || key[0] != '/' {
			key = "/" + key
		}
		manifest[key] = hex.EncodeToString(hash[:])
	}

	// Create multipart form data
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add manifest field as JSON
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}
	if err := writer.WriteField("manifest", string(manifestJSON)); err != nil {
		return nil, fmt.Errorf("write manifest field: %w", err)
	}

	// Add each file as a form file
	for path, content := range files {
		// Ensure path starts with /
		key := path
		if len(key) == 0 || key[0] != '/' {
			key = "/" + key
		}

		// Create form file with the path as field name
		part, err := writer.CreateFormFile(key, filepath.Base(path))
		if err != nil {
			return nil, fmt.Errorf("create form file %s: %w", path, err)
		}
		if _, err := part.Write(content); err != nil {
			return nil, fmt.Errorf("write file content %s: %w", path, err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	// Send request to Cloudflare API
	endpoint := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/pages/projects/%s/deployments",
		accountID, projectName)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	if api.APIToken != "" {
		req.Header.Set("Authorization", "Bearer "+api.APIToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create deployment failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var response struct {
		Result cloudflare.PagesProjectDeployment `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	api.Log.Info("Created direct upload deployment",
		"project", projectName,
		"deploymentId", response.Result.ID,
		"url", response.Result.URL)

	return &PagesDirectUploadResult{
		ID:    response.Result.ID,
		URL:   response.Result.URL,
		Stage: response.Result.LatestStage.Name,
	}, nil
}
