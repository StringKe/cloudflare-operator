// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package pagesdomain implements the Controller for PagesDomain CRD.
// It directly manages custom domains for Cloudflare Pages projects.
package pagesdomain

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/controller/common"
)

const (
	FinalizerName = "pagesdomain.networking.cloudflare-operator.io/finalizer"
)

// PagesDomainReconciler reconciles a PagesDomain object.
// It directly calls Cloudflare API and writes status back to the CRD.
type PagesDomainReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=pagesdomains,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=pagesdomains/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=pagesdomains/finalizers,verbs=update

func (r *PagesDomainReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the PagesDomain instance
	domain := &networkingv1alpha2.PagesDomain{}
	if err := r.Get(ctx, req.NamespacedName, domain); err != nil {
		if errors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !domain.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, domain)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, domain, FinalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &domain.Spec.Cloudflare,
		Namespace:         domain.Namespace,
		StatusAccountID:   domain.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.setErrorStatus(ctx, domain, err)
	}

	// Resolve project name
	projectName, err := r.resolveProjectName(ctx, domain)
	if err != nil {
		logger.Error(err, "Failed to resolve project name")
		return r.setErrorStatus(ctx, domain, err)
	}

	// Check if domain exists in Cloudflare
	existingDomain, err := apiResult.API.GetPagesDomain(ctx, projectName, domain.Spec.Domain)
	if err != nil && !cf.IsNotFoundError(err) {
		logger.Error(err, "Failed to get Pages domain from Cloudflare")
		return r.setErrorStatus(ctx, domain, err)
	}

	if existingDomain != nil {
		// Domain exists, check if we need to update
		return r.handleExistingDomain(ctx, domain, apiResult, projectName, existingDomain)
	}

	// Domain doesn't exist, create it
	return r.handleCreateDomain(ctx, domain, apiResult, projectName)
}

// resolveProjectName resolves the Cloudflare project name from the ProjectRef.
func (r *PagesDomainReconciler) resolveProjectName(
	ctx context.Context,
	domain *networkingv1alpha2.PagesDomain,
) (string, error) {
	// Priority 1: CloudflareID/CloudflareName directly specified
	if domain.Spec.ProjectRef.CloudflareID != "" {
		return domain.Spec.ProjectRef.CloudflareID, nil
	}
	if domain.Spec.ProjectRef.CloudflareName != "" {
		return domain.Spec.ProjectRef.CloudflareName, nil
	}

	// Priority 2: Reference to PagesProject K8s resource
	if domain.Spec.ProjectRef.Name != "" {
		project := &networkingv1alpha2.PagesProject{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: domain.Namespace,
			Name:      domain.Spec.ProjectRef.Name,
		}, project); err != nil {
			return "", fmt.Errorf("failed to get referenced PagesProject %s: %w",
				domain.Spec.ProjectRef.Name, err)
		}

		// Get project name from the PagesProject spec
		if project.Spec.Name != "" {
			return project.Spec.Name, nil
		}
		return project.Name, nil
	}

	return "", fmt.Errorf("project reference is required: specify name, cloudflareId, or cloudflareName")
}

// handleDeletion handles the deletion of a PagesDomain.
func (r *PagesDomainReconciler) handleDeletion(
	ctx context.Context,
	domain *networkingv1alpha2.PagesDomain,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(domain, FinalizerName) {
		return common.NoRequeue(), nil
	}

	// Check deletion policy
	if domain.Spec.DeletionPolicy == "Orphan" {
		logger.Info("Orphan deletion policy, skipping Cloudflare deletion")
	} else {
		// Get API client
		apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
			CloudflareDetails: &domain.Spec.Cloudflare,
			Namespace:         domain.Namespace,
			StatusAccountID:   domain.Status.AccountID,
		})
		if err != nil {
			logger.Error(err, "Failed to get API client for deletion")
			// Continue with finalizer removal even if API client fails
		} else {
			// Resolve project name
			projectName, err := r.resolveProjectName(ctx, domain)
			if err != nil {
				logger.Error(err, "Failed to resolve project name for deletion")
				// Continue with finalizer removal
			} else {
				// Delete domain from Cloudflare
				logger.Info("Deleting Pages domain from Cloudflare",
					"projectName", projectName,
					"domain", domain.Spec.Domain)

				if err := apiResult.API.DeletePagesDomain(ctx, projectName, domain.Spec.Domain); err != nil {
					if !cf.IsNotFoundError(err) {
						logger.Error(err, "Failed to delete Pages domain from Cloudflare")
						r.Recorder.Event(domain, corev1.EventTypeWarning, "DeleteFailed",
							fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
						return common.RequeueShort(), err
					}
					// NotFound is fine, domain already deleted
					logger.Info("Pages domain not found in Cloudflare, may have been already deleted")
				}

				r.Recorder.Event(domain, corev1.EventTypeNormal, "Deleted",
					"Pages domain deleted from Cloudflare")
			}
		}
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, domain, func() {
		controllerutil.RemoveFinalizer(domain, FinalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(domain, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// handleExistingDomain handles the case when the domain already exists in Cloudflare.
func (r *PagesDomainReconciler) handleExistingDomain(
	ctx context.Context,
	domain *networkingv1alpha2.PagesDomain,
	apiResult *common.APIClientResult,
	projectName string,
	existingDomain *cf.PagesDomainResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Pages domain exists in Cloudflare",
		"projectName", projectName,
		"domain", domain.Spec.Domain,
		"status", existingDomain.Status)

	// Try to auto-configure DNS if domain is pending
	r.autoConfigureDNSRecord(ctx, domain, apiResult, projectName, existingDomain)

	// Update status from Cloudflare
	return r.setSuccessStatus(ctx, domain, apiResult.AccountID, projectName, existingDomain)
}

// handleCreateDomain handles the creation of a new Pages domain.
func (r *PagesDomainReconciler) handleCreateDomain(
	ctx context.Context,
	domain *networkingv1alpha2.PagesDomain,
	apiResult *common.APIClientResult,
	projectName string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Creating Pages domain in Cloudflare",
		"projectName", projectName,
		"domain", domain.Spec.Domain)

	// Add domain to Cloudflare
	result, err := apiResult.API.AddPagesDomain(ctx, projectName, domain.Spec.Domain)
	if err != nil {
		logger.Error(err, "Failed to add Pages domain to Cloudflare")
		r.Recorder.Event(domain, corev1.EventTypeWarning, "CreateFailed",
			fmt.Sprintf("Failed to add domain to Cloudflare: %s", cf.SanitizeErrorMessage(err)))
		return r.setErrorStatus(ctx, domain, err)
	}

	r.Recorder.Event(domain, corev1.EventTypeNormal, "Created",
		fmt.Sprintf("Pages domain '%s' added to project '%s'", domain.Spec.Domain, projectName))

	// Try to auto-configure DNS if domain is pending
	r.autoConfigureDNSRecord(ctx, domain, apiResult, projectName, result)

	// Update status
	return r.setSuccessStatus(ctx, domain, apiResult.AccountID, projectName, result)
}

// setSuccessStatus updates the domain status with success.
func (r *PagesDomainReconciler) setSuccessStatus(
	ctx context.Context,
	domain *networkingv1alpha2.PagesDomain,
	accountID, projectName string,
	result *cf.PagesDomainResult,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, domain, func() {
		domain.Status.AccountID = accountID
		domain.Status.ProjectName = projectName
		domain.Status.DomainID = result.ID
		domain.Status.Status = result.Status
		domain.Status.ZoneID = result.ZoneTag
		domain.Status.ValidationMethod = result.ValidationMethod
		domain.Status.ValidationStatus = result.ValidationStatus

		// Map Cloudflare status to State
		switch result.Status {
		case "active":
			domain.Status.State = networkingv1alpha2.PagesDomainStateActive
		case "pending":
			domain.Status.State = networkingv1alpha2.PagesDomainStatePending
		case "verifying":
			domain.Status.State = networkingv1alpha2.PagesDomainStateVerifying
		case "moved":
			domain.Status.State = networkingv1alpha2.PagesDomainStateMoved
		case "deleting":
			domain.Status.State = networkingv1alpha2.PagesDomainStateDeleting
		default:
			domain.Status.State = networkingv1alpha2.PagesDomainStatePending
		}

		// Set Ready condition based on status
		if result.Status == "active" {
			meta.SetStatusCondition(&domain.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				ObservedGeneration: domain.Generation,
				Reason:             "Active",
				Message:            "Pages domain is active and serving traffic",
				LastTransitionTime: metav1.Now(),
			})
		} else {
			meta.SetStatusCondition(&domain.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				ObservedGeneration: domain.Generation,
				Reason:             "Pending",
				Message:            fmt.Sprintf("Pages domain is %s", result.Status),
				LastTransitionTime: metav1.Now(),
			})
		}
		domain.Status.ObservedGeneration = domain.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	// If domain is not yet active, poll for status updates
	if result.Status != "active" {
		return common.RequeueMedium(), nil
	}

	return common.NoRequeue(), nil
}

// autoConfigureDNSRecord automatically creates a CNAME DNS record if autoConfigureDNS is enabled.
// This is called when the domain status is pending and requires DNS validation.
//
//nolint:revive // cyclomatic complexity is acceptable for this linear DNS configuration logic
func (r *PagesDomainReconciler) autoConfigureDNSRecord(
	ctx context.Context,
	domain *networkingv1alpha2.PagesDomain,
	apiResult *common.APIClientResult,
	projectName string,
	result *cf.PagesDomainResult,
) {
	logger := log.FromContext(ctx)

	// Check if autoConfigureDNS is disabled
	if domain.Spec.AutoConfigureDNS != nil && !*domain.Spec.AutoConfigureDNS {
		logger.V(1).Info("autoConfigureDNS is disabled, skipping DNS auto-configuration")
		return
	}

	// Only proceed if domain is pending
	if result.Status != "pending" {
		logger.V(1).Info("Domain is not pending, skipping DNS auto-configuration",
			"status", result.Status)
		return
	}

	// Resolve ZoneID
	zoneID := r.resolveZoneID(ctx, domain, apiResult)
	if zoneID == "" {
		logger.Info("ZoneID not found, DNS auto-configuration not possible. " +
			"Set spec.zoneID or ensure the domain zone is in the same Cloudflare account.")
		r.Recorder.Event(domain, corev1.EventTypeWarning, "DNSAutoConfigSkipped",
			"ZoneID not found. DNS must be configured manually.")
		return
	}

	// Build CNAME target: <project>.pages.dev
	cnameTarget := fmt.Sprintf("%s.pages.dev", projectName)

	// Check if CNAME record already exists
	existingRecordID, err := apiResult.API.GetDNSRecordIDInZone(ctx, zoneID, domain.Spec.Domain, cf.DNSRecordTypeCNAME)
	if err != nil {
		logger.Error(err, "Failed to check existing DNS record")
		r.Recorder.Event(domain, corev1.EventTypeWarning, "DNSAutoConfigFailed",
			fmt.Sprintf("Failed to check existing DNS record: %s", cf.SanitizeErrorMessage(err)))
		return
	}

	if existingRecordID != "" {
		// Record exists, update it
		logger.Info("Updating existing CNAME record for Pages domain",
			"domain", domain.Spec.Domain,
			"target", cnameTarget,
			"zoneID", zoneID,
			"recordID", existingRecordID)

		_, err = apiResult.API.UpdateDNSRecordInZone(ctx, zoneID, existingRecordID, cf.DNSRecordParams{
			Name:    domain.Spec.Domain,
			Type:    cf.DNSRecordTypeCNAME,
			Content: cnameTarget,
			TTL:     1, // Auto TTL
			Proxied: true,
			Comment: "Managed by cloudflare-operator for PagesDomain",
		})
		if err != nil {
			logger.Error(err, "Failed to update CNAME record")
			r.Recorder.Event(domain, corev1.EventTypeWarning, "DNSAutoConfigFailed",
				fmt.Sprintf("Failed to update CNAME record: %s", cf.SanitizeErrorMessage(err)))
			return
		}

		r.Recorder.Event(domain, corev1.EventTypeNormal, "DNSRecordUpdated",
			fmt.Sprintf("CNAME record updated: %s → %s", domain.Spec.Domain, cnameTarget))
	} else {
		// Create new CNAME record
		logger.Info("Creating CNAME record for Pages domain",
			"domain", domain.Spec.Domain,
			"target", cnameTarget,
			"zoneID", zoneID)

		_, err = apiResult.API.CreateDNSRecordInZone(ctx, zoneID, cf.DNSRecordParams{
			Name:    domain.Spec.Domain,
			Type:    cf.DNSRecordTypeCNAME,
			Content: cnameTarget,
			TTL:     1, // Auto TTL
			Proxied: true,
			Comment: "Managed by cloudflare-operator for PagesDomain",
		})
		if err != nil {
			// Check if record already exists (race condition)
			if strings.Contains(err.Error(), "already exists") {
				logger.Info("CNAME record already exists, skipping creation")
				return
			}
			logger.Error(err, "Failed to create CNAME record")
			r.Recorder.Event(domain, corev1.EventTypeWarning, "DNSAutoConfigFailed",
				fmt.Sprintf("Failed to create CNAME record: %s", cf.SanitizeErrorMessage(err)))
			return
		}

		r.Recorder.Event(domain, corev1.EventTypeNormal, "DNSRecordCreated",
			fmt.Sprintf("CNAME record created: %s → %s", domain.Spec.Domain, cnameTarget))
	}
}

// resolveZoneID resolves the ZoneID for DNS auto-configuration.
// Priority: spec.zoneID > status.zoneID > API lookup by domain name.
// Returns empty string if zone cannot be resolved (not an error, just means auto-config is not possible).
func (*PagesDomainReconciler) resolveZoneID(
	ctx context.Context,
	domain *networkingv1alpha2.PagesDomain,
	apiResult *common.APIClientResult,
) string {
	logger := log.FromContext(ctx)

	// Priority 1: Spec.ZoneID explicitly set
	if domain.Spec.ZoneID != "" {
		logger.V(1).Info("Using ZoneID from spec", "zoneID", domain.Spec.ZoneID)
		return domain.Spec.ZoneID
	}

	// Priority 2: Status.ZoneID from Cloudflare response (zone_tag)
	if domain.Status.ZoneID != "" {
		logger.V(1).Info("Using ZoneID from status", "zoneID", domain.Status.ZoneID)
		return domain.Status.ZoneID
	}

	// Priority 3: Query Cloudflare API by domain name
	logger.V(1).Info("Querying ZoneID from Cloudflare API", "domain", domain.Spec.Domain)
	zoneID, zoneName, err := apiResult.API.GetZoneIDForDomain(ctx, domain.Spec.Domain)
	if err != nil {
		// Zone not found is not a fatal error, just means auto-config is not possible
		logger.V(1).Info("Zone not found for domain", "domain", domain.Spec.Domain, "error", err)
		return ""
	}

	logger.Info("Resolved ZoneID from Cloudflare API",
		"domain", domain.Spec.Domain,
		"zoneID", zoneID,
		"zoneName", zoneName)
	return zoneID
}

// setErrorStatus updates the domain status with an error.
func (r *PagesDomainReconciler) setErrorStatus(
	ctx context.Context,
	domain *networkingv1alpha2.PagesDomain,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, domain, func() {
		domain.Status.State = networkingv1alpha2.PagesDomainStateError
		domain.Status.Message = cf.SanitizeErrorMessage(err)
		meta.SetStatusCondition(&domain.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: domain.Generation,
			Reason:             "Error",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		domain.Status.ObservedGeneration = domain.Generation
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
}

// findDomainsForProject returns PagesDomains that may need reconciliation when a PagesProject changes.
func (r *PagesDomainReconciler) findDomainsForProject(ctx context.Context, obj client.Object) []reconcile.Request {
	project, ok := obj.(*networkingv1alpha2.PagesProject)
	if !ok {
		return nil
	}

	// List all PagesDomains in the same namespace
	domainList := &networkingv1alpha2.PagesDomainList{}
	if err := r.List(ctx, domainList, client.InNamespace(project.Namespace)); err != nil {
		return nil
	}

	projectName := project.Name
	if project.Spec.Name != "" {
		projectName = project.Spec.Name
	}

	var requests []reconcile.Request
	for _, domain := range domainList.Items {
		// Check if this domain references the project
		if domain.Spec.ProjectRef.Name == project.Name ||
			domain.Spec.ProjectRef.CloudflareID == projectName ||
			domain.Spec.ProjectRef.CloudflareName == projectName {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&domain),
			})
		}
	}

	return requests
}

func (r *PagesDomainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("pagesdomain-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("pagesdomain"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.PagesDomain{}).
		Watches(&networkingv1alpha2.PagesProject{},
			handler.EnqueueRequestsFromMapFunc(r.findDomainsForProject)).
		Complete(r)
}
