// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package cloudflaredomain provides a controller for managing CloudflareDomain resources.
// It verifies domains with Cloudflare API and writes status back to the CRD.
package cloudflaredomain

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	cfclient "github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

const (
	finalizerName = "cloudflare.com/domain-finalizer"
)

// Reconciler reconciles a CloudflareDomain object.
// It verifies domains with Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=cloudflaredomains,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=cloudflaredomains/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=cloudflaredomains/finalizers,verbs=update
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=cloudflarecredentials,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile handles CloudflareDomain reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the CloudflareDomain resource
	domain := &networkingv1alpha2.CloudflareDomain{}
	if err := r.Get(ctx, req.NamespacedName, domain); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Unable to fetch CloudflareDomain")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !domain.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, domain)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, domain, finalizerName); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Only set Verifying state if this is a new domain or was in error state
	needsFullVerification := domain.Status.ZoneID == "" ||
		domain.Status.State == networkingv1alpha2.CloudflareDomainStateError ||
		domain.Status.State == networkingv1alpha2.CloudflareDomainStatePending ||
		domain.Status.State == ""

	if needsFullVerification {
		r.updateState(ctx, domain, networkingv1alpha2.CloudflareDomainStateVerifying, "Verifying domain with Cloudflare API")
	}

	// Get credentials
	creds, err := r.getCredentials(ctx, domain)
	if err != nil {
		r.updateState(ctx, domain, networkingv1alpha2.CloudflareDomainStateError, fmt.Sprintf("Failed to get credentials: %v", err))
		r.Recorder.Event(domain, corev1.EventTypeWarning, controller.EventReasonAPIError, err.Error())
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Verify domain and get zone info
	if err := r.verifyDomain(ctx, domain, creds); err != nil {
		r.updateState(ctx, domain, networkingv1alpha2.CloudflareDomainStateError, fmt.Sprintf("Failed to verify domain: %v", err))
		r.Recorder.Event(domain, corev1.EventTypeWarning, controller.EventReasonAPIError, err.Error())
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	// Check for duplicate default
	if domain.Spec.IsDefault {
		if err := r.ensureSingleDefault(ctx, domain); err != nil {
			r.updateState(ctx, domain, networkingv1alpha2.CloudflareDomainStateError, err.Error())
			r.Recorder.Event(domain, corev1.EventTypeWarning, "DuplicateDefault", err.Error())
			return ctrl.Result{}, nil
		}
	}

	// Update status to Ready
	r.updateState(ctx, domain, networkingv1alpha2.CloudflareDomainStateReady, "Domain verified successfully")
	r.Recorder.Event(domain, corev1.EventTypeNormal, controller.EventReasonReconciled, "Domain verified successfully")

	// Requeue periodically to re-verify
	return ctrl.Result{RequeueAfter: time.Hour}, nil
}

// handleDeletion handles the deletion of CloudflareDomain
func (r *Reconciler) handleDeletion(ctx context.Context, domain *networkingv1alpha2.CloudflareDomain) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(domain, finalizerName) {
		return ctrl.Result{}, nil
	}

	// CloudflareDomain is a verification resource - we don't delete zones from Cloudflare
	logger.Info("Removing finalizer for CloudflareDomain (zones are not deleted from CF)")

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, domain, func() {
		controllerutil.RemoveFinalizer(domain, finalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(domain, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// getCredentials retrieves the CloudflareCredentials to use
func (r *Reconciler) getCredentials(ctx context.Context, domain *networkingv1alpha2.CloudflareDomain) (*networkingv1alpha2.CloudflareCredentials, error) {
	// If credentialsRef is specified, use it
	if domain.Spec.CredentialsRef != nil {
		creds := &networkingv1alpha2.CloudflareCredentials{}
		if err := r.Get(ctx, types.NamespacedName{Name: domain.Spec.CredentialsRef.Name}, creds); err != nil {
			return nil, fmt.Errorf("failed to get CloudflareCredentials '%s': %w", domain.Spec.CredentialsRef.Name, err)
		}
		return creds, nil
	}

	// Otherwise, find the default CloudflareCredentials
	credsList := &networkingv1alpha2.CloudflareCredentialsList{}
	if err := r.List(ctx, credsList); err != nil {
		return nil, fmt.Errorf("failed to list CloudflareCredentials: %w", err)
	}

	for i := range credsList.Items {
		if credsList.Items[i].Spec.IsDefault {
			return &credsList.Items[i], nil
		}
	}

	return nil, errors.New("no default CloudflareCredentials found, please create one with isDefault=true or specify credentialsRef")
}

// verifyDomain verifies the domain exists in Cloudflare and retrieves zone information
func (r *Reconciler) verifyDomain(ctx context.Context, domain *networkingv1alpha2.CloudflareDomain, creds *networkingv1alpha2.CloudflareCredentials) error {
	logger := log.FromContext(ctx)

	// If ZoneID is manually specified, use it directly
	if domain.Spec.ZoneID != "" {
		domain.Status.ZoneID = domain.Spec.ZoneID
		logger.Info("Using manually specified Zone ID", "zoneId", domain.Spec.ZoneID)
		return r.fetchZoneDetails(ctx, domain, creds, domain.Spec.ZoneID)
	}

	// Create Cloudflare client
	cfClient, err := r.createCloudflareClient(ctx, creds)
	if err != nil {
		return err
	}

	// Look up zone by name
	zones, err := cfClient.ListZonesContext(ctx, cloudflare.WithZoneFilters(domain.Spec.Domain, creds.Spec.AccountID, ""))
	if err != nil {
		return fmt.Errorf("failed to list zones: %w", err)
	}

	if len(zones.Result) == 0 {
		return fmt.Errorf("zone not found for domain '%s' in account '%s'", domain.Spec.Domain, creds.Spec.AccountID)
	}

	zone := zones.Result[0]

	// Update status with zone information
	domain.Status.ZoneID = zone.ID
	domain.Status.ZoneName = zone.Name
	domain.Status.AccountID = zone.Account.ID
	domain.Status.NameServers = zone.NameServers
	domain.Status.ZoneStatus = zone.Status

	now := metav1.Now()
	domain.Status.LastVerifiedTime = &now

	logger.Info("Domain verified", "domain", domain.Spec.Domain, "zoneId", zone.ID, "zoneStatus", zone.Status)

	return nil
}

// fetchZoneDetails fetches zone details for a given zone ID
func (r *Reconciler) fetchZoneDetails(ctx context.Context, domain *networkingv1alpha2.CloudflareDomain, creds *networkingv1alpha2.CloudflareCredentials, zoneID string) error {
	cfClient, err := r.createCloudflareClient(ctx, creds)
	if err != nil {
		return err
	}

	zone, err := cfClient.ZoneDetails(ctx, zoneID)
	if err != nil {
		return fmt.Errorf("failed to get zone details for ID '%s': %w", zoneID, err)
	}

	// Update status with zone information
	domain.Status.ZoneID = zone.ID
	domain.Status.ZoneName = zone.Name
	domain.Status.AccountID = zone.Account.ID
	domain.Status.NameServers = zone.NameServers
	domain.Status.ZoneStatus = zone.Status

	now := metav1.Now()
	domain.Status.LastVerifiedTime = &now

	return nil
}

// createCloudflareClient creates a Cloudflare API client from credentials
func (r *Reconciler) createCloudflareClient(ctx context.Context, creds *networkingv1alpha2.CloudflareCredentials) (*cloudflare.API, error) {
	// Get the secret
	secret := &corev1.Secret{}
	secretNamespace := creds.Spec.SecretRef.Namespace
	if secretNamespace == "" {
		secretNamespace = "cloudflare-operator-system"
	}

	if err := r.Get(ctx, types.NamespacedName{
		Name:      creds.Spec.SecretRef.Name,
		Namespace: secretNamespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	// Create Cloudflare client based on auth type
	var cfAPI *cloudflare.API
	var err error

	// Build options list - add custom base URL if configured
	var opts []cloudflare.Option
	if baseURL := cfclient.GetAPIBaseURL(); baseURL != "" {
		opts = append(opts, cloudflare.BaseURL(baseURL))
	}

	switch creds.Spec.AuthType {
	case networkingv1alpha2.AuthTypeAPIToken:
		tokenKey := creds.Spec.SecretRef.APITokenKey
		if tokenKey == "" {
			tokenKey = "CLOUDFLARE_API_TOKEN"
		}
		token := string(secret.Data[tokenKey])
		if token == "" {
			return nil, fmt.Errorf("API token not found in secret (key: %s)", tokenKey)
		}
		cfAPI, err = cloudflare.NewWithAPIToken(token, opts...)

	case networkingv1alpha2.AuthTypeGlobalAPIKey:
		keyKey := creds.Spec.SecretRef.APIKeyKey
		if keyKey == "" {
			keyKey = "CLOUDFLARE_API_KEY"
		}
		emailKey := creds.Spec.SecretRef.EmailKey
		if emailKey == "" {
			emailKey = "CLOUDFLARE_EMAIL"
		}

		apiKey := string(secret.Data[keyKey])
		email := string(secret.Data[emailKey])

		if apiKey == "" {
			return nil, fmt.Errorf("API key not found in secret (key: %s)", keyKey)
		}
		if email == "" {
			return nil, fmt.Errorf("email not found in secret (key: %s)", emailKey)
		}
		cfAPI, err = cloudflare.New(apiKey, email, opts...)

	default:
		return nil, fmt.Errorf("unknown auth type: %s", creds.Spec.AuthType)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create Cloudflare client: %w", err)
	}

	return cfAPI, nil
}

// ensureSingleDefault ensures only one CloudflareDomain is marked as default
func (r *Reconciler) ensureSingleDefault(ctx context.Context, domain *networkingv1alpha2.CloudflareDomain) error {
	domainList := &networkingv1alpha2.CloudflareDomainList{}
	if err := r.List(ctx, domainList); err != nil {
		return fmt.Errorf("failed to list CloudflareDomains: %w", err)
	}

	for _, d := range domainList.Items {
		if d.Name != domain.Name && d.Spec.IsDefault {
			return fmt.Errorf("another CloudflareDomain '%s' is already marked as default", d.Name)
		}
	}

	return nil
}

// updateState updates the state and status of the CloudflareDomain
func (r *Reconciler) updateState(ctx context.Context, domain *networkingv1alpha2.CloudflareDomain, state networkingv1alpha2.CloudflareDomainState, message string) {
	logger := log.FromContext(ctx)

	// Update conditions
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		ObservedGeneration: domain.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(state),
		Message:            message,
	}

	if state == networkingv1alpha2.CloudflareDomainStateReady {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "Verified"
	}

	if err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, domain, func() {
		domain.Status.State = state
		domain.Status.Message = message
		domain.Status.ObservedGeneration = domain.Generation
		controller.SetCondition(&domain.Status.Conditions, condition.Type, condition.Status, condition.Reason, condition.Message)
	}); err != nil {
		logger.Error(err, "failed to update status")
	}
}

// findDomainsForCredentials returns CloudflareDomains that reference the given credentials
func (r *Reconciler) findDomainsForCredentials(ctx context.Context, obj client.Object) []reconcile.Request {
	creds, ok := obj.(*networkingv1alpha2.CloudflareCredentials)
	if !ok {
		return nil
	}
	logger := log.FromContext(ctx)

	// List all CloudflareDomains
	domainList := &networkingv1alpha2.CloudflareDomainList{}
	if err := r.List(ctx, domainList); err != nil {
		logger.Error(err, "Failed to list CloudflareDomains for credentials watch")
		return nil
	}

	var requests []reconcile.Request
	for _, domain := range domainList.Items {
		// Check if this domain references the credentials
		if domain.Spec.CredentialsRef != nil && domain.Spec.CredentialsRef.Name == creds.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: domain.Name},
			})
		}

		// Also check if credentials is default and domain has no credentialsRef
		if creds.Spec.IsDefault && domain.Spec.CredentialsRef == nil {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: domain.Name},
			})
		}
	}

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("cloudflaredomain-controller")

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.CloudflareDomain{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findDomainsForCredentials)).
		Named("cloudflaredomain").
		Complete(r)
}
