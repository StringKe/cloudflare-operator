// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesproject

import (
	"context"
	"fmt"

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

// Reconcile ensures Web Analytics is configured according to spec.
// It should be called after the project is successfully synced and subdomain is available.
func (r *WebAnalyticsReconciler) Reconcile(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	apiClient *cf.API,
) error {
	log := r.Log.WithValues("project", project.Name, "namespace", project.Namespace)

	// Check if subdomain is available (required for Web Analytics)
	if project.Status.Subdomain == "" {
		log.V(1).Info("Subdomain not yet available, skipping Web Analytics reconciliation")
		return nil
	}

	// Build the hostname for Web Analytics
	hostname := fmt.Sprintf("%s.pages.dev", project.Status.Subdomain)

	// Check if Web Analytics should be enabled
	enableWebAnalytics := project.Spec.EnableWebAnalytics == nil || *project.Spec.EnableWebAnalytics

	if enableWebAnalytics {
		return r.enableWebAnalytics(ctx, project, apiClient, hostname)
	}

	return r.disableWebAnalytics(ctx, project, apiClient)
}

// enableWebAnalytics enables Web Analytics for the project.
//
//nolint:revive // cognitive complexity acceptable for idempotent enable logic
func (r *WebAnalyticsReconciler) enableWebAnalytics(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	apiClient *cf.API,
	hostname string,
) error {
	log := r.Log.WithValues("hostname", hostname)

	// Check if already enabled with same configuration
	if project.Status.WebAnalytics != nil &&
		project.Status.WebAnalytics.Enabled &&
		project.Status.WebAnalytics.Hostname == hostname &&
		project.Status.WebAnalytics.SiteTag != "" {
		log.V(1).Info("Web Analytics already enabled")
		return nil
	}

	// Check if site already exists
	existingSite, err := apiClient.GetWebAnalyticsSite(ctx, hostname)
	if err != nil {
		log.Error(err, "Failed to check existing Web Analytics site")
		return fmt.Errorf("failed to check existing Web Analytics site: %w", err)
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
				log.Error(err, "Failed to update Web Analytics site")
				return fmt.Errorf("failed to update Web Analytics site: %w", err)
			}
		}
	} else {
		// Create new Web Analytics site
		log.Info("Enabling Web Analytics")
		site, err = apiClient.EnableWebAnalytics(ctx, hostname)
		if err != nil {
			log.Error(err, "Failed to enable Web Analytics")
			r.Recorder.Event(project, corev1.EventTypeWarning, "WebAnalyticsFailed",
				fmt.Sprintf("Failed to enable Web Analytics: %s", cf.SanitizeErrorMessage(err)))
			return fmt.Errorf("failed to enable Web Analytics: %w", err)
		}

		r.Recorder.Event(project, corev1.EventTypeNormal, "WebAnalyticsEnabled",
			fmt.Sprintf("Web Analytics enabled for %s", hostname))
	}

	// Update status
	return r.updateWebAnalyticsStatus(ctx, project, site, hostname)
}

// disableWebAnalytics disables Web Analytics for the project.
func (r *WebAnalyticsReconciler) disableWebAnalytics(
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

	siteTag := project.Status.WebAnalytics.SiteTag
	if siteTag == "" {
		log.V(1).Info("No site tag found, nothing to disable")
		return r.clearWebAnalyticsStatus(ctx, project)
	}

	log.Info("Disabling Web Analytics", "siteTag", siteTag)

	if err := apiClient.DisableWebAnalytics(ctx, siteTag); err != nil {
		log.Error(err, "Failed to disable Web Analytics")
		r.Recorder.Event(project, corev1.EventTypeWarning, "WebAnalyticsDisableFailed",
			fmt.Sprintf("Failed to disable Web Analytics: %s", cf.SanitizeErrorMessage(err)))
		return fmt.Errorf("failed to disable Web Analytics: %w", err)
	}

	r.Recorder.Event(project, corev1.EventTypeNormal, "WebAnalyticsDisabled",
		"Web Analytics disabled")

	return r.clearWebAnalyticsStatus(ctx, project)
}

// Cleanup removes Web Analytics when the project is being deleted.
func (r *WebAnalyticsReconciler) Cleanup(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	apiClient *cf.API,
) error {
	log := r.Log.WithValues("project", project.Name)

	// Check if there's anything to clean up
	if project.Status.WebAnalytics == nil || project.Status.WebAnalytics.SiteTag == "" {
		log.V(1).Info("No Web Analytics to clean up")
		return nil
	}

	siteTag := project.Status.WebAnalytics.SiteTag
	log.Info("Cleaning up Web Analytics", "siteTag", siteTag)

	if err := apiClient.DisableWebAnalytics(ctx, siteTag); err != nil {
		// Log but don't fail deletion
		log.Error(err, "Failed to clean up Web Analytics, continuing with deletion")
	}

	return nil
}

// updateWebAnalyticsStatus updates the Web Analytics status in the project.
func (r *WebAnalyticsReconciler) updateWebAnalyticsStatus(
	ctx context.Context,
	project *networkingv1alpha2.PagesProject,
	site *cf.RUMSite,
	hostname string,
) error {
	now := metav1.Now()

	return controller.UpdateStatusWithConflictRetry(ctx, r.Client, project, func() {
		project.Status.WebAnalytics = &networkingv1alpha2.WebAnalyticsStatus{
			Enabled:     true,
			SiteTag:     site.SiteTag,
			SiteToken:   site.SiteToken,
			Hostname:    hostname,
			AutoInstall: site.AutoInstall,
			LastChecked: &now,
			Message:     "Web Analytics enabled with auto-install",
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
			LastChecked: &now,
			Message:     "Web Analytics disabled",
		}
	})
}
