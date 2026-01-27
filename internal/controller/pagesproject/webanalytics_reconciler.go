// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesproject

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

// WebAnalyticsReconciler handles Web Analytics (RUM) configuration for PagesProject.
type WebAnalyticsReconciler struct {
	client.Client
	Recorder record.EventRecorder
	Log      logr.Logger
}

// NewWebAnalyticsReconciler creates a new WebAnalyticsReconciler.
func NewWebAnalyticsReconciler(k8sClient client.Client, recorder record.EventRecorder, log logr.Logger) *WebAnalyticsReconciler {
	return &WebAnalyticsReconciler{
		Client:   k8sClient,
		Recorder: recorder,
		Log:      log.WithName("webanalytics"),
	}
}

// Reconcile ensures Web Analytics is configured for all hostnames according to spec.
// It should be called after the project is successfully synced and subdomain is available.
func (r *WebAnalyticsReconciler) Reconcile(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	apiClient *cf.API,
) error {
	log := r.Log.WithValues("project", project.Name, "namespace", project.Namespace)

	// Collect all hostnames that need Web Analytics
	hostnames := r.collectHostnames(project)
	if len(hostnames) == 0 {
		log.V(1).Info("No hostnames available, skipping Web Analytics reconciliation")
		return nil
	}

	// Check if Web Analytics should be enabled
	enableWebAnalytics := project.Spec.EnableWebAnalytics == nil || *project.Spec.EnableWebAnalytics

	if enableWebAnalytics {
		return r.enableAllSites(ctx, project, apiClient, hostnames)
	}
	return r.disableAllSites(ctx, project, apiClient)
}

// collectHostnames gathers all hostnames that need Web Analytics.
// Returns *.pages.dev hostname plus all custom domains.
func (*WebAnalyticsReconciler) collectHostnames(project *networkingv1alpha2.PagesProject) []string {
	var hostnames []string

	// 1. Add *.pages.dev hostname (fix: check for existing suffix to avoid double concatenation)
	if project.Status.Subdomain != "" {
		hostname := project.Status.Subdomain
		// Fix: Only append .pages.dev if not already present
		if !strings.HasSuffix(hostname, ".pages.dev") {
			hostname = fmt.Sprintf("%s.pages.dev", hostname)
		}
		hostnames = append(hostnames, hostname)
	}

	// 2. Add all custom domains from status
	hostnames = append(hostnames, project.Status.Domains...)

	return hostnames
}

// enableAllSites enables Web Analytics for all specified hostnames.
func (r *WebAnalyticsReconciler) enableAllSites(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	apiClient *cf.API,
	hostnames []string,
) error {
	log := r.Log.WithValues("project", project.Name)
	sites := make([]networkingv1alpha2.WebAnalyticsSiteStatus, 0, len(hostnames))
	var errs []error

	for _, hostname := range hostnames {
		site, err := r.enableSite(ctx, project, apiClient, hostname)
		if err != nil {
			log.Error(err, "Failed to enable Web Analytics for hostname", "hostname", hostname)
			errs = append(errs, fmt.Errorf("hostname %s: %w", hostname, err))
			continue
		}
		sites = append(sites, *site)
	}

	// Update status with all sites
	if updateErr := r.updateMultiSiteStatus(ctx, project, sites); updateErr != nil {
		errs = append(errs, fmt.Errorf("update status: %w", updateErr))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// enableSite enables Web Analytics for a single hostname.
//
//nolint:revive // cognitive complexity acceptable for idempotent enable logic
func (r *WebAnalyticsReconciler) enableSite(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	apiClient *cf.API,
	hostname string,
) (*networkingv1alpha2.WebAnalyticsSiteStatus, error) {
	log := r.Log.WithValues("hostname", hostname)

	// Check if site already exists
	existingSite, err := apiClient.GetWebAnalyticsSite(ctx, hostname)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing site: %w", err)
	}

	var site *cf.RUMSite
	if existingSite != nil {
		log.V(1).Info("Web Analytics site already exists", "siteTag", existingSite.SiteTag)
		site = existingSite

		// Ensure auto_install is enabled
		if !existingSite.AutoInstall {
			log.Info("Updating Web Analytics site to enable auto_install")
			site, err = apiClient.UpdateWebAnalyticsSite(ctx, existingSite.SiteTag, true)
			if err != nil {
				return nil, fmt.Errorf("failed to update site: %w", err)
			}
		}
	} else {
		// Create new Web Analytics site
		log.Info("Enabling Web Analytics", "hostname", hostname)
		site, err = apiClient.EnableWebAnalytics(ctx, hostname)
		if err != nil {
			r.Recorder.Event(project, corev1.EventTypeWarning, "WebAnalyticsFailed",
				fmt.Sprintf("Failed to enable Web Analytics for %s: %s", hostname, cf.SanitizeErrorMessage(err)))
			return nil, fmt.Errorf("failed to enable Web Analytics: %w", err)
		}
		r.Recorder.Event(project, corev1.EventTypeNormal, "WebAnalyticsEnabled",
			fmt.Sprintf("Web Analytics enabled for %s", hostname))
	}

	return &networkingv1alpha2.WebAnalyticsSiteStatus{
		Hostname:    hostname,
		SiteTag:     site.SiteTag,
		SiteToken:   site.SiteToken,
		AutoInstall: site.AutoInstall,
		Enabled:     true,
	}, nil
}

// disableAllSites disables Web Analytics for the project.
//
//nolint:revive // cognitive complexity acceptable for multi-site disable with backward compatibility
func (r *WebAnalyticsReconciler) disableAllSites(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	apiClient *cf.API,
) error {
	log := r.Log

	// Check if there's anything to disable
	if project.Status.WebAnalytics == nil || !project.Status.WebAnalytics.Enabled {
		log.V(1).Info("Web Analytics already disabled")
		return nil
	}

	var errs []error

	// Disable all sites from the Sites list
	for _, site := range project.Status.WebAnalytics.Sites {
		if site.SiteTag == "" {
			continue
		}
		log.Info("Disabling Web Analytics", "hostname", site.Hostname, "siteTag", site.SiteTag)
		if err := apiClient.DisableWebAnalytics(ctx, site.SiteTag); err != nil {
			errs = append(errs, fmt.Errorf("disable site %s: %w", site.Hostname, err))
		}
	}

	// Also clean up legacy single site if present (backward compatibility)
	if project.Status.WebAnalytics.SiteTag != "" {
		// Check if it's not already in the Sites list
		found := false
		for _, site := range project.Status.WebAnalytics.Sites {
			if site.SiteTag == project.Status.WebAnalytics.SiteTag {
				found = true
				break
			}
		}
		if !found {
			log.Info("Disabling legacy Web Analytics site", "siteTag", project.Status.WebAnalytics.SiteTag)
			if err := apiClient.DisableWebAnalytics(ctx, project.Status.WebAnalytics.SiteTag); err != nil {
				errs = append(errs, fmt.Errorf("disable legacy site: %w", err))
			}
		}
	}

	if len(errs) > 0 {
		r.Recorder.Event(project, corev1.EventTypeWarning, "WebAnalyticsDisableFailed",
			fmt.Sprintf("Failed to disable some Web Analytics sites: %v", errs))
	} else {
		r.Recorder.Event(project, corev1.EventTypeNormal, "WebAnalyticsDisabled",
			"Web Analytics disabled for all sites")
	}

	// Clear the status
	clearErr := r.clearWebAnalyticsStatus(ctx, project)
	if clearErr != nil {
		errs = append(errs, clearErr)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Cleanup removes Web Analytics when the project is being deleted.
func (r *WebAnalyticsReconciler) Cleanup(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	apiClient *cf.API,
) error {
	log := r.Log.WithValues("project", project.Name)

	if project.Status.WebAnalytics == nil {
		log.V(1).Info("No Web Analytics to clean up")
		return nil
	}

	var errs []error

	// Clean up all sites from the Sites list
	for _, site := range project.Status.WebAnalytics.Sites {
		if site.SiteTag == "" {
			continue
		}
		log.Info("Cleaning up Web Analytics site", "hostname", site.Hostname, "siteTag", site.SiteTag)
		if err := apiClient.DisableWebAnalytics(ctx, site.SiteTag); err != nil {
			// Log but don't fail deletion
			log.Error(err, "Failed to clean up Web Analytics site", "hostname", site.Hostname)
			errs = append(errs, fmt.Errorf("cleanup site %s: %w", site.Hostname, err))
		}
	}

	// Also clean up legacy single site if present (backward compatibility)
	if project.Status.WebAnalytics.SiteTag != "" {
		// Check if it's not already in the Sites list
		found := false
		for _, site := range project.Status.WebAnalytics.Sites {
			if site.SiteTag == project.Status.WebAnalytics.SiteTag {
				found = true
				break
			}
		}
		if !found {
			log.Info("Cleaning up legacy Web Analytics site", "siteTag", project.Status.WebAnalytics.SiteTag)
			if err := apiClient.DisableWebAnalytics(ctx, project.Status.WebAnalytics.SiteTag); err != nil {
				log.Error(err, "Failed to clean up legacy Web Analytics site")
			}
		}
	}

	// Log errors but don't fail deletion
	if len(errs) > 0 {
		log.Error(errors.Join(errs...), "Some Web Analytics sites failed to clean up, continuing with deletion")
	}

	return nil
}

// updateMultiSiteStatus updates the Web Analytics status with multiple sites.
//
//nolint:revive // cognitive complexity acceptable for multi-site status update
func (r *WebAnalyticsReconciler) updateMultiSiteStatus(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	sites []networkingv1alpha2.WebAnalyticsSiteStatus,
) error {
	now := metav1.Now()

	// Build message
	var message string
	switch len(sites) {
	case 0:
		message = "No Web Analytics sites enabled"
	case 1:
		message = fmt.Sprintf("Web Analytics enabled for %s", sites[0].Hostname)
	default:
		message = fmt.Sprintf("Web Analytics enabled for %d sites", len(sites))
	}

	return controller.UpdateStatusWithConflictRetry(ctx, r.Client, project, func() {
		// Determine if any site is enabled
		anyEnabled := false
		for _, site := range sites {
			if site.Enabled {
				anyEnabled = true
				break
			}
		}

		project.Status.WebAnalytics = &networkingv1alpha2.WebAnalyticsStatus{
			Enabled:     anyEnabled,
			Sites:       sites,
			LastChecked: &now,
			Message:     message,
		}

		// Set legacy fields for backward compatibility (use first site)
		if len(sites) > 0 {
			project.Status.WebAnalytics.SiteTag = sites[0].SiteTag
			project.Status.WebAnalytics.SiteToken = sites[0].SiteToken
			project.Status.WebAnalytics.Hostname = sites[0].Hostname
			project.Status.WebAnalytics.AutoInstall = sites[0].AutoInstall
		}
	})
}

// clearWebAnalyticsStatus clears the Web Analytics status.
func (r *WebAnalyticsReconciler) clearWebAnalyticsStatus(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
) error {
	now := metav1.Now()

	return controller.UpdateStatusWithConflictRetry(ctx, r.Client, project, func() {
		project.Status.WebAnalytics = &networkingv1alpha2.WebAnalyticsStatus{
			Enabled:     false,
			Sites:       nil,
			LastChecked: &now,
			Message:     "Web Analytics disabled",
		}
	})
}
