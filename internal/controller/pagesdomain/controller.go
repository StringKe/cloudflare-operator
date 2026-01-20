// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package pagesdomain implements the L2 Controller for PagesDomain CRD.
// It registers domain configurations to the Core Service for sync.
package pagesdomain

import (
	"context"
	"fmt"
	"time"

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
	"github.com/StringKe/cloudflare-operator/internal/service"
	pagessvc "github.com/StringKe/cloudflare-operator/internal/service/pages"
)

const (
	FinalizerName = "pagesdomain.networking.cloudflare-operator.io/finalizer"
)

// PagesDomainReconciler reconciles a PagesDomain object
type PagesDomainReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	domainService *pagessvc.DomainService
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=pagesdomains,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=pagesdomains/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=pagesdomains/finalizers,verbs=update

//nolint:revive // cognitive complexity is acceptable for this reconcile loop
func (r *PagesDomainReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the PagesDomain instance
	domain := &networkingv1alpha2.PagesDomain{}
	if err := r.Get(ctx, req.NamespacedName, domain); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Resolve credentials
	credInfo, err := r.resolveCredentials(ctx, domain)
	if err != nil {
		logger.Error(err, "Failed to resolve credentials")
		return r.updateStatusError(ctx, domain, err)
	}

	// Resolve project name
	projectName, err := r.resolveProjectName(ctx, domain)
	if err != nil {
		logger.Error(err, "Failed to resolve project name")
		return r.updateStatusError(ctx, domain, err)
	}

	// Handle deletion
	if !domain.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, domain, projectName)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(domain, FinalizerName) {
		controllerutil.AddFinalizer(domain, FinalizerName)
		if err := r.Update(ctx, domain); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Register Pages domain configuration to SyncState
	return r.registerPagesDomain(ctx, domain, projectName, credInfo)
}

// resolveCredentials resolves the credentials for the Pages domain.
func (r *PagesDomainReconciler) resolveCredentials(
	ctx context.Context,
	domain *networkingv1alpha2.PagesDomain,
) (*controller.CredentialsInfo, error) {
	return controller.ResolveCredentialsForService(
		ctx,
		r.Client,
		log.FromContext(ctx),
		domain.Spec.Cloudflare,
		domain.Namespace,
		domain.Status.AccountID,
	)
}

// resolveProjectName resolves the Cloudflare project name from the ProjectRef.
//
//nolint:revive // cognitive complexity is acceptable for resolution logic
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
	projectName string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(domain, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Check deletion policy
	if domain.Spec.DeletionPolicy == "Orphan" {
		logger.Info("Orphan deletion policy, skipping Cloudflare deletion")
	} else {
		// Unregister from SyncState - this triggers Sync Controller to delete from Cloudflare
		source := service.Source{
			Kind:      "PagesDomain",
			Namespace: domain.Namespace,
			Name:      domain.Name,
		}

		logger.Info("Unregistering Pages domain from SyncState",
			"projectName", projectName,
			"domain", domain.Spec.Domain,
			"source", fmt.Sprintf("%s/%s", domain.Namespace, domain.Name))

		if err := r.domainService.Unregister(ctx, projectName, domain.Spec.Domain, source); err != nil {
			logger.Error(err, "Failed to unregister Pages domain from SyncState")
			r.Recorder.Event(domain, corev1.EventTypeWarning, "UnregisterFailed",
				fmt.Sprintf("Failed to unregister from SyncState: %s", err.Error()))
			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}

		r.Recorder.Event(domain, corev1.EventTypeNormal, "Unregistered",
			"Unregistered from SyncState, Sync Controller will delete from Cloudflare")
	}

	// Remove finalizer with retry logic
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, domain, func() {
		controllerutil.RemoveFinalizer(domain, FinalizerName)
	}); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(domain, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// registerPagesDomain registers the Pages domain configuration to SyncState.
func (r *PagesDomainReconciler) registerPagesDomain(
	ctx context.Context,
	domain *networkingv1alpha2.PagesDomain,
	projectName string,
	credInfo *controller.CredentialsInfo,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Create source reference
	source := service.Source{
		Kind:      "PagesDomain",
		Namespace: domain.Namespace,
		Name:      domain.Name,
	}

	// Build Pages domain configuration
	config := pagessvc.PagesDomainConfig{
		Domain:           domain.Spec.Domain,
		ProjectName:      projectName,
		AutoConfigureDNS: domain.Spec.AutoConfigureDNS,
	}

	// Register to SyncState
	opts := pagessvc.DomainRegisterOptions{
		DomainName:     domain.Spec.Domain,
		ProjectName:    projectName,
		AccountID:      credInfo.AccountID,
		Source:         source,
		Config:         config,
		CredentialsRef: credInfo.CredentialsRef,
	}

	if err := r.domainService.Register(ctx, opts); err != nil {
		logger.Error(err, "Failed to register Pages domain configuration")
		r.Recorder.Event(domain, corev1.EventTypeWarning, "RegisterFailed",
			fmt.Sprintf("Failed to register Pages domain: %s", err.Error()))
		return r.updateStatusError(ctx, domain, err)
	}

	r.Recorder.Event(domain, corev1.EventTypeNormal, "Registered",
		fmt.Sprintf("Registered Pages Domain '%s' configuration to SyncState", domain.Spec.Domain))

	// Check if already synced
	syncStatus, err := r.domainService.GetSyncStatus(ctx, projectName, domain.Spec.Domain)
	if err != nil {
		logger.Error(err, "Failed to get sync status")
		return r.updateStatusPending(ctx, domain, projectName, credInfo.AccountID)
	}

	if syncStatus != nil && syncStatus.IsSynced {
		// Already synced, update status to Active
		return r.updateStatusActive(ctx, domain, projectName, credInfo.AccountID, syncStatus.DomainID, syncStatus.Status)
	}

	// Update status to Pending - actual sync happens via PagesSyncController
	return r.updateStatusPending(ctx, domain, projectName, credInfo.AccountID)
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

func (r *PagesDomainReconciler) updateStatusError(
	ctx context.Context,
	domain *networkingv1alpha2.PagesDomain,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, domain, func() {
		domain.Status.State = networkingv1alpha2.PagesDomainStateError
		meta.SetStatusCondition(&domain.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: domain.Generation,
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		domain.Status.ObservedGeneration = domain.Generation
	})

	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *PagesDomainReconciler) updateStatusPending(
	ctx context.Context,
	domain *networkingv1alpha2.PagesDomain,
	projectName, accountID string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, domain, func() {
		if domain.Status.AccountID == "" {
			domain.Status.AccountID = accountID
		}
		domain.Status.ProjectName = projectName
		domain.Status.State = networkingv1alpha2.PagesDomainStatePending
		meta.SetStatusCondition(&domain.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: domain.Generation,
			Reason:             "Pending",
			Message:            "Pages domain configuration registered, waiting for sync",
			LastTransitionTime: metav1.Now(),
		})
		domain.Status.ObservedGeneration = domain.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	// Requeue to check for sync completion
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *PagesDomainReconciler) updateStatusActive(
	ctx context.Context,
	domain *networkingv1alpha2.PagesDomain,
	projectName, accountID, domainID, status string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, domain, func() {
		domain.Status.AccountID = accountID
		domain.Status.ProjectName = projectName
		domain.Status.DomainID = domainID
		domain.Status.Status = status
		domain.Status.State = networkingv1alpha2.PagesDomainStateActive
		meta.SetStatusCondition(&domain.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: domain.Generation,
			Reason:             "Synced",
			Message:            "Pages domain synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		domain.Status.ObservedGeneration = domain.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *PagesDomainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("pagesdomain-controller")

	// Initialize DomainService
	r.domainService = pagessvc.NewDomainService(mgr.GetClient())

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.PagesDomain{}).
		Watches(&networkingv1alpha2.PagesProject{},
			handler.EnqueueRequestsFromMapFunc(r.findDomainsForProject)).
		Complete(r)
}
