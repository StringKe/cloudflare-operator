// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cloudflarecredentials

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	cfclient "github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

const (
	finalizerName = "cloudflare.com/credentials-finalizer"
)

// Reconciler reconciles a CloudflareCredentials object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Internal state
	ctx   context.Context
	log   logr.Logger
	creds *networkingv1alpha2.CloudflareCredentials
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=cloudflarecredentials,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=cloudflarecredentials/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=cloudflarecredentials/finalizers,verbs=update

// Reconcile handles CloudflareCredentials reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrl.LoggerFrom(ctx)

	// Get the CloudflareCredentials resource
	r.creds = &networkingv1alpha2.CloudflareCredentials{}
	if err := r.Get(ctx, req.NamespacedName, r.creds); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch CloudflareCredentials")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !r.creds.DeletionTimestamp.IsZero() {
		return r.handleDeletion()
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.creds, finalizerName) {
		controllerutil.AddFinalizer(r.creds, finalizerName)
		if err := r.Update(ctx, r.creds); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Validate credentials
	if err := r.validateCredentials(); err != nil {
		r.updateStatus("Error", false, err.Error())
		r.Recorder.Event(r.creds, corev1.EventTypeWarning, controller.EventReasonAPIError, err.Error())
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	// Check for duplicate default
	if r.creds.Spec.IsDefault {
		if err := r.ensureSingleDefault(); err != nil {
			r.updateStatus("Error", false, err.Error())
			r.Recorder.Event(r.creds, corev1.EventTypeWarning, "DuplicateDefault", err.Error())
			return ctrl.Result{}, nil
		}
	}

	// Update status
	r.updateStatus("Ready", true, "Credentials validated successfully")
	r.Recorder.Event(r.creds, corev1.EventTypeNormal, controller.EventReasonReconciled, "Credentials validated successfully")

	// Requeue periodically to re-validate
	return ctrl.Result{RequeueAfter: time.Hour}, nil
}

// handleDeletion handles the deletion of CloudflareCredentials
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(r.creds, finalizerName) {
		// Nothing to clean up on Cloudflare side for credentials

		// Remove finalizer
		controllerutil.RemoveFinalizer(r.creds, finalizerName)
		if err := r.Update(r.ctx, r.creds); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// validateCredentials validates the credentials against Cloudflare API
func (r *Reconciler) validateCredentials() error {
	// Get the secret
	secret := &corev1.Secret{}
	secretNamespace := r.creds.Spec.SecretRef.Namespace
	if secretNamespace == "" {
		secretNamespace = "cloudflare-operator-system"
	}

	if err := r.Get(r.ctx, types.NamespacedName{
		Name:      r.creds.Spec.SecretRef.Name,
		Namespace: secretNamespace,
	}, secret); err != nil {
		return fmt.Errorf("failed to get secret: %w", err)
	}

	// Create Cloudflare client based on auth type
	var cfClient *cloudflare.API
	var err error

	// Build options list - add custom base URL if configured
	var opts []cloudflare.Option
	if baseURL := cfclient.GetAPIBaseURL(); baseURL != "" {
		opts = append(opts, cloudflare.BaseURL(baseURL))
	}

	switch r.creds.Spec.AuthType {
	case networkingv1alpha2.AuthTypeAPIToken:
		tokenKey := r.creds.Spec.SecretRef.APITokenKey
		if tokenKey == "" {
			tokenKey = "CLOUDFLARE_API_TOKEN"
		}
		token := string(secret.Data[tokenKey])
		if token == "" {
			return fmt.Errorf("API token not found in secret (key: %s)", tokenKey)
		}
		cfClient, err = cloudflare.NewWithAPIToken(token, opts...)

	case networkingv1alpha2.AuthTypeGlobalAPIKey:
		keyKey := r.creds.Spec.SecretRef.APIKeyKey
		if keyKey == "" {
			keyKey = "CLOUDFLARE_API_KEY"
		}
		emailKey := r.creds.Spec.SecretRef.EmailKey
		if emailKey == "" {
			emailKey = "CLOUDFLARE_EMAIL"
		}

		apiKey := string(secret.Data[keyKey])
		email := string(secret.Data[emailKey])

		if apiKey == "" {
			return fmt.Errorf("API key not found in secret (key: %s)", keyKey)
		}
		if email == "" {
			return fmt.Errorf("email not found in secret (key: %s)", emailKey)
		}
		cfClient, err = cloudflare.New(apiKey, email, opts...)

	default:
		return fmt.Errorf("unknown auth type: %s", r.creds.Spec.AuthType)
	}

	if err != nil {
		return fmt.Errorf("failed to create Cloudflare client: %w", err)
	}

	// Validate by fetching account details
	account, _, err := cfClient.Account(r.ctx, r.creds.Spec.AccountID)
	if err != nil {
		return fmt.Errorf("failed to validate account: %w", err)
	}

	// Store account name in status
	r.creds.Status.AccountName = account.Name

	return nil
}

// ensureSingleDefault ensures only one CloudflareCredentials is marked as default
func (r *Reconciler) ensureSingleDefault() error {
	credsList := &networkingv1alpha2.CloudflareCredentialsList{}
	if err := r.List(r.ctx, credsList); err != nil {
		return fmt.Errorf("failed to list CloudflareCredentials: %w", err)
	}

	for _, creds := range credsList.Items {
		if creds.Name != r.creds.Name && creds.Spec.IsDefault {
			return fmt.Errorf("another CloudflareCredentials '%s' is already marked as default", creds.Name)
		}
	}

	return nil
}

// updateStatus updates the status of the CloudflareCredentials
func (r *Reconciler) updateStatus(state string, validated bool, message string) {
	r.creds.Status.State = state
	r.creds.Status.Validated = validated
	r.creds.Status.ObservedGeneration = r.creds.Generation

	if validated {
		now := metav1.Now()
		r.creds.Status.LastValidatedTime = &now
	}

	// Update conditions
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		ObservedGeneration: r.creds.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             "ValidationFailed",
		Message:            message,
	}

	if validated {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "Validated"
	}

	// Update or add condition
	found := false
	for i, c := range r.creds.Status.Conditions {
		if c.Type == condition.Type {
			r.creds.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		r.creds.Status.Conditions = append(r.creds.Status.Conditions, condition)
	}

	if err := r.Status().Update(r.ctx, r.creds); err != nil {
		r.log.Error(err, "failed to update status")
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.CloudflareCredentials{}).
		Named("cloudflarecredentials").
		Complete(r)
}
