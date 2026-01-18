// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package cloudflaredomain implements the controller for CloudflareDomain resources.
package cloudflaredomain

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"github.com/go-logr/logr"
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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	cfclient "github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

const (
	finalizerName = "cloudflare.com/domain-finalizer"
)

// Reconciler reconciles a CloudflareDomain object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Internal state
	ctx    context.Context
	log    logr.Logger
	domain *networkingv1alpha2.CloudflareDomain
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=cloudflaredomains,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=cloudflaredomains/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=cloudflaredomains/finalizers,verbs=update

// Reconcile handles CloudflareDomain reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrl.LoggerFrom(ctx)

	// Get the CloudflareDomain resource
	r.domain = &networkingv1alpha2.CloudflareDomain{}
	if err := r.Get(ctx, req.NamespacedName, r.domain); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch CloudflareDomain")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !r.domain.DeletionTimestamp.IsZero() {
		return r.handleDeletion()
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.domain, finalizerName) {
		controllerutil.AddFinalizer(r.domain, finalizerName)
		if err := r.Update(ctx, r.domain); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Only set Verifying state if this is a new domain or was in error state
	// This prevents race conditions where DomainResolver skips the domain
	// during periodic re-verification
	needsFullVerification := r.domain.Status.ZoneID == "" ||
		r.domain.Status.State == networkingv1alpha2.CloudflareDomainStateError ||
		r.domain.Status.State == networkingv1alpha2.CloudflareDomainStatePending ||
		r.domain.Status.State == ""

	if needsFullVerification {
		r.updateState(networkingv1alpha2.CloudflareDomainStateVerifying, "Verifying domain with Cloudflare API")
	}

	// Get credentials
	creds, err := r.getCredentials()
	if err != nil {
		r.updateState(networkingv1alpha2.CloudflareDomainStateError, fmt.Sprintf("Failed to get credentials: %v", err))
		r.Recorder.Event(r.domain, corev1.EventTypeWarning, controller.EventReasonAPIError, err.Error())
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Verify domain and get zone info
	if err := r.verifyDomain(creds); err != nil {
		r.updateState(networkingv1alpha2.CloudflareDomainStateError, fmt.Sprintf("Failed to verify domain: %v", err))
		r.Recorder.Event(r.domain, corev1.EventTypeWarning, controller.EventReasonAPIError, err.Error())
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	// Check for duplicate default
	if r.domain.Spec.IsDefault {
		if err := r.ensureSingleDefault(); err != nil {
			r.updateState(networkingv1alpha2.CloudflareDomainStateError, err.Error())
			r.Recorder.Event(r.domain, corev1.EventTypeWarning, "DuplicateDefault", err.Error())
			return ctrl.Result{}, nil
		}
	}

	// Sync zone settings if configured
	if r.domain.Spec.SSL != nil || r.domain.Spec.Cache != nil ||
		r.domain.Spec.Security != nil || r.domain.Spec.Performance != nil {
		cfClient, err := r.createCloudflareClient(creds)
		if err != nil {
			r.updateState(networkingv1alpha2.CloudflareDomainStateError, fmt.Sprintf("Failed to create Cloudflare client: %v", err))
			r.Recorder.Event(r.domain, corev1.EventTypeWarning, controller.EventReasonAPIError, err.Error())
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		if err := r.syncZoneSettings(cfClient); err != nil {
			r.log.Error(err, "Failed to sync zone settings")
			r.Recorder.Event(r.domain, corev1.EventTypeWarning, "SettingsSyncFailed", err.Error())
			// Don't fail the entire reconciliation, just log the error
			// Settings sync is best-effort
		} else {
			r.Recorder.Event(r.domain, corev1.EventTypeNormal, "SettingsSynced", "Zone settings synchronized successfully")
		}
	}

	// Update status to Ready
	r.updateState(networkingv1alpha2.CloudflareDomainStateReady, "Domain verified successfully")
	r.Recorder.Event(r.domain, corev1.EventTypeNormal, controller.EventReasonReconciled, "Domain verified successfully")

	// Requeue periodically to re-verify
	return ctrl.Result{RequeueAfter: time.Hour}, nil
}

// handleDeletion handles the deletion of CloudflareDomain
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(r.domain, finalizerName) {
		// Nothing to clean up on Cloudflare side for domain mapping

		// Remove finalizer with conflict retry
		if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.domain, func() {
			controllerutil.RemoveFinalizer(r.domain, finalizerName)
		}); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// getCredentials retrieves the CloudflareCredentials to use
func (r *Reconciler) getCredentials() (*networkingv1alpha2.CloudflareCredentials, error) {
	// If credentialsRef is specified, use it
	if r.domain.Spec.CredentialsRef != nil {
		creds := &networkingv1alpha2.CloudflareCredentials{}
		if err := r.Get(r.ctx, types.NamespacedName{Name: r.domain.Spec.CredentialsRef.Name}, creds); err != nil {
			return nil, fmt.Errorf("failed to get CloudflareCredentials '%s': %w", r.domain.Spec.CredentialsRef.Name, err)
		}
		return creds, nil
	}

	// Otherwise, find the default CloudflareCredentials
	credsList := &networkingv1alpha2.CloudflareCredentialsList{}
	if err := r.List(r.ctx, credsList); err != nil {
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
func (r *Reconciler) verifyDomain(creds *networkingv1alpha2.CloudflareCredentials) error {
	// If ZoneID is manually specified, use it directly
	if r.domain.Spec.ZoneID != "" {
		r.domain.Status.ZoneID = r.domain.Spec.ZoneID
		r.log.Info("Using manually specified Zone ID", "zoneId", r.domain.Spec.ZoneID)
		return r.fetchZoneDetails(creds, r.domain.Spec.ZoneID)
	}

	// Create Cloudflare client
	cfClient, err := r.createCloudflareClient(creds)
	if err != nil {
		return err
	}

	// Look up zone by name
	zones, err := cfClient.ListZonesContext(r.ctx, cloudflare.WithZoneFilters(r.domain.Spec.Domain, creds.Spec.AccountID, ""))
	if err != nil {
		return fmt.Errorf("failed to list zones: %w", err)
	}

	if len(zones.Result) == 0 {
		return fmt.Errorf("zone not found for domain '%s' in account '%s'", r.domain.Spec.Domain, creds.Spec.AccountID)
	}

	zone := zones.Result[0]

	// Update status with zone information
	r.domain.Status.ZoneID = zone.ID
	r.domain.Status.ZoneName = zone.Name
	r.domain.Status.AccountID = zone.Account.ID
	r.domain.Status.NameServers = zone.NameServers
	r.domain.Status.ZoneStatus = zone.Status

	now := metav1.Now()
	r.domain.Status.LastVerifiedTime = &now

	r.log.Info("Domain verified", "domain", r.domain.Spec.Domain, "zoneId", zone.ID, "zoneStatus", zone.Status)

	return nil
}

// fetchZoneDetails fetches zone details for a given zone ID
func (r *Reconciler) fetchZoneDetails(creds *networkingv1alpha2.CloudflareCredentials, zoneID string) error {
	cfClient, err := r.createCloudflareClient(creds)
	if err != nil {
		return err
	}

	zone, err := cfClient.ZoneDetails(r.ctx, zoneID)
	if err != nil {
		return fmt.Errorf("failed to get zone details for ID '%s': %w", zoneID, err)
	}

	// Update status with zone information
	r.domain.Status.ZoneID = zone.ID
	r.domain.Status.ZoneName = zone.Name
	r.domain.Status.AccountID = zone.Account.ID
	r.domain.Status.NameServers = zone.NameServers
	r.domain.Status.ZoneStatus = zone.Status

	now := metav1.Now()
	r.domain.Status.LastVerifiedTime = &now

	return nil
}

// createCloudflareClient creates a Cloudflare API client from credentials
func (r *Reconciler) createCloudflareClient(creds *networkingv1alpha2.CloudflareCredentials) (*cloudflare.API, error) {
	// Get the secret
	secret := &corev1.Secret{}
	secretNamespace := creds.Spec.SecretRef.Namespace
	if secretNamespace == "" {
		secretNamespace = "cloudflare-operator-system"
	}

	if err := r.Get(r.ctx, types.NamespacedName{
		Name:      creds.Spec.SecretRef.Name,
		Namespace: secretNamespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	// Create Cloudflare client based on auth type
	var cfClient *cloudflare.API
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
		cfClient, err = cloudflare.NewWithAPIToken(token, opts...)

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
		cfClient, err = cloudflare.New(apiKey, email, opts...)

	default:
		return nil, fmt.Errorf("unknown auth type: %s", creds.Spec.AuthType)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create Cloudflare client: %w", err)
	}

	return cfClient, nil
}

// ensureSingleDefault ensures only one CloudflareDomain is marked as default
func (r *Reconciler) ensureSingleDefault() error {
	domainList := &networkingv1alpha2.CloudflareDomainList{}
	if err := r.List(r.ctx, domainList); err != nil {
		return fmt.Errorf("failed to list CloudflareDomains: %w", err)
	}

	for _, domain := range domainList.Items {
		if domain.Name != r.domain.Name && domain.Spec.IsDefault {
			return fmt.Errorf("another CloudflareDomain '%s' is already marked as default", domain.Name)
		}
	}

	return nil
}

// updateState updates the state and status of the CloudflareDomain
func (r *Reconciler) updateState(state networkingv1alpha2.CloudflareDomainState, message string) {
	r.domain.Status.State = state
	r.domain.Status.Message = message
	r.domain.Status.ObservedGeneration = r.domain.Generation

	// Update conditions
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		ObservedGeneration: r.domain.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(state),
		Message:            message,
	}

	if state == networkingv1alpha2.CloudflareDomainStateReady {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "Verified"
	}

	// Use helper to set condition
	controller.SetCondition(&r.domain.Status.Conditions, condition.Type, condition.Status, condition.Reason, condition.Message)

	if err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.domain, func() {
		r.domain.Status.State = state
		r.domain.Status.Message = message
		r.domain.Status.ObservedGeneration = r.domain.Generation
		controller.SetCondition(&r.domain.Status.Conditions, condition.Type, condition.Status, condition.Reason, condition.Message)
	}); err != nil {
		r.log.Error(err, "failed to update status")
	}
}

// findDomainsForCredentials returns CloudflareDomains that reference the given credentials
func (r *Reconciler) findDomainsForCredentials(ctx context.Context, obj client.Object) []reconcile.Request {
	creds, ok := obj.(*networkingv1alpha2.CloudflareCredentials)
	if !ok {
		return nil
	}

	// List all CloudflareDomains
	domainList := &networkingv1alpha2.CloudflareDomainList{}
	if err := r.List(ctx, domainList); err != nil {
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
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.CloudflareDomain{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findDomainsForCredentials)).
		Named("cloudflareDomain").
		Complete(r)
}
