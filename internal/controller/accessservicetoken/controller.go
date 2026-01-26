// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package accessservicetoken provides a controller for managing Cloudflare Access Service Tokens.
// It directly calls Cloudflare API and writes status back to the CRD.
package accessservicetoken

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/controller/common"
)

const (
	finalizerName = "accessservicetoken.networking.cloudflare-operator.io/finalizer"
)

// Reconciler reconciles an AccessServiceToken object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessservicetokens,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessservicetokens/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessservicetokens/finalizers,verbs=update

// Reconcile handles AccessServiceToken reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the AccessServiceToken resource
	token := &networkingv1alpha2.AccessServiceToken{}
	if err := r.Get(ctx, req.NamespacedName, token); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch AccessServiceToken")
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !token.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, token)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, token, finalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Get API client - use resource namespace for credentials resolution
	// AccessServiceToken is now namespaced
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &token.Spec.Cloudflare,
		Namespace:         token.Namespace, // Use resource namespace
		StatusAccountID:   token.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, token, err)
	}

	// Sync token to Cloudflare
	return r.syncToken(ctx, token, apiResult)
}

// handleDeletion handles the deletion of AccessServiceToken.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	token *networkingv1alpha2.AccessServiceToken,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(token, finalizerName) {
		return common.NoRequeue(), nil
	}

	// Remove secret finalizer first
	if err := r.removeSecretFinalizer(ctx, token); err != nil {
		logger.Error(err, "Failed to remove secret finalizer, continuing with deletion")
	}

	// Get API client - use resource namespace for credentials resolution
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &token.Spec.Cloudflare,
		Namespace:         token.Namespace, // Use resource namespace
		StatusAccountID:   token.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client for deletion")
		// Continue with finalizer removal
	} else if token.Status.TokenID != "" {
		// Delete token from Cloudflare
		logger.Info("Deleting Access Service Token from Cloudflare",
			"tokenId", token.Status.TokenID)

		if err := apiResult.API.DeleteAccessServiceToken(ctx, token.Status.TokenID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete Access Service Token from Cloudflare")
				r.Recorder.Event(token, corev1.EventTypeWarning, "DeleteFailed",
					fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
				return common.RequeueShort(), err
			}
			logger.Info("Access Service Token not found in Cloudflare, may have been already deleted")
		}

		r.Recorder.Event(token, corev1.EventTypeNormal, "Deleted",
			"Access Service Token deleted from Cloudflare")
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, token, func() {
		controllerutil.RemoveFinalizer(token, finalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(token, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// syncToken syncs the Access Service Token to Cloudflare.
func (r *Reconciler) syncToken(
	ctx context.Context,
	token *networkingv1alpha2.AccessServiceToken,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Determine token name
	tokenName := token.GetTokenName()
	duration := token.Spec.Duration
	if duration == "" {
		duration = "8760h" // Default to 1 year
	}

	// Check if token already exists by ID
	if token.Status.TokenID != "" {
		// Update existing token
		logger.V(1).Info("Updating Access Service Token in Cloudflare",
			"tokenId", token.Status.TokenID,
			"name", tokenName)

		result, err := apiResult.API.UpdateAccessServiceToken(ctx, token.Status.TokenID, tokenName, duration)
		if err != nil {
			if cf.IsNotFoundError(err) {
				// Token doesn't exist anymore, will create
				logger.Info("Access Service Token not found in Cloudflare, will recreate",
					"tokenId", token.Status.TokenID)
			} else {
				logger.Error(err, "Failed to update Access Service Token")
				return r.updateStatusError(ctx, token, err)
			}
		} else {
			r.Recorder.Event(token, corev1.EventTypeNormal, "Updated",
				fmt.Sprintf("Access Service Token '%s' updated in Cloudflare", tokenName))

			// Note: Update doesn't return client secret, just update status
			return r.updateStatusReady(ctx, token, apiResult.AccountID, result, false)
		}
	}

	// Try to find existing token by name
	existingByName, err := apiResult.API.GetAccessServiceTokenByName(ctx, tokenName)
	if err != nil {
		logger.Error(err, "Failed to search for existing Access Service Token")
		return r.updateStatusError(ctx, token, err)
	}

	if existingByName != nil {
		// Token already exists with this name, adopt it
		logger.Info("Access Service Token already exists with same name, adopting it",
			"tokenId", existingByName.TokenID,
			"name", tokenName)

		// Update the existing token
		result, err := apiResult.API.UpdateAccessServiceToken(ctx, existingByName.TokenID, tokenName, duration)
		if err != nil {
			logger.Error(err, "Failed to update existing Access Service Token")
			return r.updateStatusError(ctx, token, err)
		}

		r.Recorder.Event(token, corev1.EventTypeNormal, "Adopted",
			fmt.Sprintf("Adopted existing Access Service Token '%s'", tokenName))

		// Note: Update doesn't return client secret, so we can't update the secret
		return r.updateStatusReady(ctx, token, apiResult.AccountID, result, false)
	}

	// Create new token
	logger.Info("Creating Access Service Token in Cloudflare",
		"name", tokenName)

	result, err := apiResult.API.CreateAccessServiceToken(ctx, tokenName, duration)
	if err != nil {
		logger.Error(err, "Failed to create Access Service Token")
		return r.updateStatusError(ctx, token, err)
	}

	r.Recorder.Event(token, corev1.EventTypeNormal, "Created",
		fmt.Sprintf("Access Service Token '%s' created in Cloudflare", tokenName))

	// Create/update the secret with client credentials (only on creation)
	if err := r.createOrUpdateSecret(ctx, token, result); err != nil {
		logger.Error(err, "Failed to create/update secret with token credentials")
		r.Recorder.Event(token, corev1.EventTypeWarning, "SecretFailed",
			fmt.Sprintf("Failed to create/update secret: %s", err.Error()))
		// Continue - token was created successfully
	}

	// Ensure secret has finalizer
	if err := r.ensureSecretFinalizer(ctx, token); err != nil {
		logger.Error(err, "Failed to add finalizer to secret")
	}

	return r.updateStatusReady(ctx, token, apiResult.AccountID, result, true)
}

// createOrUpdateSecret creates or updates the K8s secret with token credentials.
// Secret is created in the same namespace as the AccessServiceToken resource.
func (r *Reconciler) createOrUpdateSecret(
	ctx context.Context,
	token *networkingv1alpha2.AccessServiceToken,
	result *cf.AccessServiceTokenResult,
) error {
	secretName := token.Spec.SecretRef.Name
	// Secret is created in the same namespace as the AccessServiceToken resource
	secretNamespace := token.Namespace

	clientIDKey := token.Spec.SecretRef.ClientIDKey
	if clientIDKey == "" {
		clientIDKey = "CF_ACCESS_CLIENT_ID"
	}
	clientSecretKey := token.Spec.SecretRef.ClientSecretKey
	if clientSecretKey == "" {
		clientSecretKey = "CF_ACCESS_CLIENT_SECRET"
	}

	// Check if secret exists
	existingSecret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: secretNamespace}, existingSecret)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	// Prepare secret data
	secretData := map[string][]byte{
		clientIDKey:     []byte(result.ClientID),
		clientSecretKey: []byte(result.ClientSecret),
	}

	if apierrors.IsNotFound(err) {
		// Create new secret
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: secretNamespace,
				Labels: map[string]string{
					"cloudflare-operator.io/managed-by": "accessservicetoken",
					"cloudflare-operator.io/token-name": token.Name,
				},
			},
			Type: corev1.SecretTypeOpaque,
			Data: secretData,
		}
		return r.Create(ctx, secret)
	}

	// Update existing secret
	existingSecret.Data = secretData
	return r.Update(ctx, existingSecret)
}

// getSecretFinalizerName returns a valid RFC 1123 compliant finalizer name for the secret.
func getSecretFinalizerName(tokenName string) string {
	return fmt.Sprintf("accessservicetoken.%s.secret-protection", tokenName)
}

// ensureSecretFinalizer adds a finalizer to the secret to prevent accidental deletion.
func (r *Reconciler) ensureSecretFinalizer(ctx context.Context, token *networkingv1alpha2.AccessServiceToken) error {
	secretName := token.Spec.SecretRef.Name
	// Secret is in the same namespace as the AccessServiceToken
	secretNamespace := token.Namespace

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: secretNamespace}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	finalizerName := getSecretFinalizerName(token.Name)
	if !controllerutil.ContainsFinalizer(secret, finalizerName) {
		controllerutil.AddFinalizer(secret, finalizerName)
		return r.Update(ctx, secret)
	}
	return nil
}

// removeSecretFinalizer removes the finalizer from the secret.
func (r *Reconciler) removeSecretFinalizer(ctx context.Context, token *networkingv1alpha2.AccessServiceToken) error {
	if token.Spec.SecretRef.Name == "" {
		return nil
	}

	secretName := token.Spec.SecretRef.Name
	// Secret is in the same namespace as the AccessServiceToken
	secretNamespace := token.Namespace

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: secretNamespace}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	finalizerName := getSecretFinalizerName(token.Name)
	if controllerutil.ContainsFinalizer(secret, finalizerName) {
		controllerutil.RemoveFinalizer(secret, finalizerName)
		return r.Update(ctx, secret)
	}
	return nil
}

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	token *networkingv1alpha2.AccessServiceToken,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, token, func() {
		token.Status.State = "Error"
		meta.SetStatusCondition(&token.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: token.Generation,
			Reason:             "Error",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		token.Status.ObservedGeneration = token.Generation
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
}

func (r *Reconciler) updateStatusReady(
	ctx context.Context,
	token *networkingv1alpha2.AccessServiceToken,
	accountID string,
	result *cf.AccessServiceTokenResult,
	isNewToken bool,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, token, func() {
		token.Status.AccountID = accountID
		token.Status.TokenID = result.TokenID
		token.Status.ClientID = result.ClientID
		token.Status.ExpiresAt = result.ExpiresAt
		token.Status.CreatedAt = result.CreatedAt
		token.Status.UpdatedAt = result.UpdatedAt
		token.Status.LastSeenAt = result.LastSeenAt
		token.Status.ClientSecretVersion = result.ClientSecretVersion
		token.Status.SecretName = fmt.Sprintf("%s/%s", token.Namespace, token.Spec.SecretRef.Name)
		token.Status.State = "Ready"
		meta.SetStatusCondition(&token.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: token.Generation,
			Reason:             "Synced",
			Message:            "Access Service Token synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		token.Status.ObservedGeneration = token.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("accessservicetoken-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("accessservicetoken"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.AccessServiceToken{}).
		Named("accessservicetoken").
		Complete(r)
}
