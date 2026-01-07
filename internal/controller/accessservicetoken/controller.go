/*
Copyright 2025 Adyanth H.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package accessservicetoken

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
)

const (
	FinalizerName = "accessservicetoken.networking.cloudflare-operator.io/finalizer"
)

// AccessServiceTokenReconciler reconciles an AccessServiceToken object
type AccessServiceTokenReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessservicetokens,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessservicetokens/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessservicetokens/finalizers,verbs=update

func (r *AccessServiceTokenReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the AccessServiceToken instance
	token := &networkingv1alpha2.AccessServiceToken{}
	if err := r.Get(ctx, req.NamespacedName, token); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Initialize API client
	apiClient, err := cf.NewAPIClientFromDetails(ctx, r.Client, token.Namespace, token.Spec.Cloudflare)
	if err != nil {
		logger.Error(err, "Failed to initialize Cloudflare API client")
		return r.updateStatusError(ctx, token, err)
	}

	// Handle deletion
	if !token.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, token, apiClient)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(token, FinalizerName) {
		controllerutil.AddFinalizer(token, FinalizerName)
		if err := r.Update(ctx, token); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile the service token
	return r.reconcileServiceToken(ctx, token, apiClient)
}

func (r *AccessServiceTokenReconciler) handleDeletion(ctx context.Context, token *networkingv1alpha2.AccessServiceToken, apiClient *cf.API) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(token, FinalizerName) {
		// Delete from Cloudflare
		if token.Status.TokenID != "" {
			logger.Info("Deleting Access Service Token from Cloudflare", "tokenId", token.Status.TokenID)
			if err := apiClient.DeleteAccessServiceToken(token.Status.TokenID); err != nil {
				// P0 FIX: Check if resource is already deleted (NotFound error)
				if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to delete Access Service Token from Cloudflare")
					r.Recorder.Event(token, corev1.EventTypeWarning, "DeleteFailed",
						fmt.Sprintf("Failed to delete from Cloudflare: %v", cf.SanitizeErrorMessage(err)))
					return ctrl.Result{RequeueAfter: 30 * time.Second}, err
				}
				logger.Info("Access Service Token already deleted from Cloudflare", "tokenId", token.Status.TokenID)
				r.Recorder.Event(token, corev1.EventTypeNormal, "AlreadyDeleted", "Token was already deleted from Cloudflare")
			} else {
				r.Recorder.Event(token, corev1.EventTypeNormal, "Deleted", "Deleted from Cloudflare")
			}
		}

		// Remove secret finalizer before removing token finalizer
		if err := r.removeSecretFinalizer(ctx, token); err != nil {
			logger.Error(err, "Failed to remove secret finalizer, continuing with deletion")
			// Don't block on this - the secret might have been deleted already
		}

		// Remove finalizer
		controllerutil.RemoveFinalizer(token, FinalizerName)
		if err := r.Update(ctx, token); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *AccessServiceTokenReconciler) reconcileServiceToken(ctx context.Context, token *networkingv1alpha2.AccessServiceToken, apiClient *cf.API) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	tokenName := token.GetTokenName()
	duration := token.Spec.Duration
	if duration == "" {
		duration = "8760h" // Default to 1 year
	}

	var result *cf.AccessServiceTokenResult
	var err error

	if token.Status.TokenID == "" {
		// Try to find existing token by name first (adoption)
		existingToken, err := apiClient.GetAccessServiceTokenByName(tokenName)
		if err == nil && existingToken != nil {
			// Found existing token - attempt to adopt
			logger.Info("Found existing Access Service Token, adopting", "tokenId", existingToken.TokenID, "name", tokenName)

			// Check if we have a secret with credentials
			if token.Spec.SecretRef.Name != "" {
				secretExists, hasCredentials := r.checkSecretExists(ctx, token)
				if !secretExists || !hasCredentials {
					// Secret doesn't exist or is missing credentials - warn user
					warnMsg := "Adopting existing token but secret is missing. " +
						"ClientSecret cannot be recovered. Create the secret manually or delete the token."
					logger.Info(warnMsg)
					r.Recorder.Event(token, corev1.EventTypeWarning, "SecretMissing", warnMsg)
					// Continue with adoption but set a warning condition
					token.Status.TokenID = existingToken.TokenID
					token.Status.ClientID = existingToken.ClientID
					token.Status.AccountID = existingToken.AccountID
					token.Status.ExpiresAt = existingToken.ExpiresAt
					return r.updateStatusWithWarning(ctx, token, "Adopted token but secret is missing")
				}
			}

			// Adopt the token - update with our settings
			result, err = apiClient.UpdateAccessServiceToken(existingToken.TokenID, tokenName, duration)
			if err != nil {
				return r.updateStatusError(ctx, token, err)
			}
			r.Recorder.Event(token, corev1.EventTypeNormal, "Adopted", fmt.Sprintf("Adopted existing token: %s", existingToken.TokenID))
		} else {
			// Create new service token
			logger.Info("Creating Access Service Token", "name", tokenName)
			result, err = apiClient.CreateAccessServiceToken(tokenName, duration)
			if err != nil {
				return r.updateStatusError(ctx, token, err)
			}

			// Store the client secret in a Kubernetes secret (only available on creation)
			if token.Spec.SecretRef.Name != "" && result.ClientSecret != "" {
				if err := r.createOrUpdateSecret(ctx, token, result); err != nil {
					// Critical: Secret creation failed but token was created
					// We should warn strongly because the secret cannot be recovered
					logger.Error(err, "CRITICAL: Failed to create secret for token. ClientSecret cannot be recovered!")
					r.Recorder.Event(token, corev1.EventTypeWarning, "SecretCreateFailed",
						"CRITICAL: Token created but secret creation failed. ClientSecret is lost and cannot be recovered from Cloudflare.")
					return r.updateStatusError(ctx, token, fmt.Errorf("failed to create secret: %w (ClientSecret is lost)", err))
				}
				r.Recorder.Event(token, corev1.EventTypeNormal, "SecretCreated", "Secret created with client credentials")
			}
		}
	} else {
		// Update existing service token
		logger.Info("Updating Access Service Token", "tokenId", token.Status.TokenID)
		result, err = apiClient.UpdateAccessServiceToken(token.Status.TokenID, tokenName, duration)
		if err != nil {
			return r.updateStatusError(ctx, token, err)
		}
	}

	// Ensure secret has finalizer to prevent accidental deletion
	if token.Spec.SecretRef.Name != "" {
		if err := r.ensureSecretFinalizer(ctx, token); err != nil {
			logger.Error(err, "Failed to add finalizer to secret")
			// Don't fail reconciliation for this
		}
	}

	// Update status
	return r.updateStatusSuccess(ctx, token, result)
}

// checkSecretExists checks if the secret exists and has the required credentials
func (r *AccessServiceTokenReconciler) checkSecretExists(
	ctx context.Context, token *networkingv1alpha2.AccessServiceToken,
) (exists bool, hasCredentials bool) {
	secretName := token.Spec.SecretRef.Name
	secretNamespace := token.Spec.SecretRef.Namespace
	if secretNamespace == "" {
		secretNamespace = token.Namespace
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: secretNamespace}, secret); err != nil {
		return false, false
	}

	_, hasClientID := secret.Data["CF_ACCESS_CLIENT_ID"]
	_, hasClientSecret := secret.Data["CF_ACCESS_CLIENT_SECRET"]

	return true, hasClientID && hasClientSecret
}

// ensureSecretFinalizer adds a finalizer to the secret to prevent accidental deletion
func (r *AccessServiceTokenReconciler) ensureSecretFinalizer(ctx context.Context, token *networkingv1alpha2.AccessServiceToken) error {
	secretName := token.Spec.SecretRef.Name
	secretNamespace := token.Spec.SecretRef.Namespace
	if secretNamespace == "" {
		secretNamespace = token.Namespace
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: secretNamespace}, secret); err != nil {
		if errors.IsNotFound(err) {
			return nil // Secret doesn't exist yet
		}
		return err
	}

	finalizerName := fmt.Sprintf("accessservicetoken.%s.%s/secret-protection", token.Namespace, token.Name)
	if !controllerutil.ContainsFinalizer(secret, finalizerName) {
		controllerutil.AddFinalizer(secret, finalizerName)
		return r.Update(ctx, secret)
	}
	return nil
}

// removeSecretFinalizer removes the finalizer from the secret
func (r *AccessServiceTokenReconciler) removeSecretFinalizer(ctx context.Context, token *networkingv1alpha2.AccessServiceToken) error {
	if token.Spec.SecretRef.Name == "" {
		return nil
	}

	secretName := token.Spec.SecretRef.Name
	secretNamespace := token.Spec.SecretRef.Namespace
	if secretNamespace == "" {
		secretNamespace = token.Namespace
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: secretNamespace}, secret); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	finalizerName := fmt.Sprintf("accessservicetoken.%s.%s/secret-protection", token.Namespace, token.Name)
	if controllerutil.ContainsFinalizer(secret, finalizerName) {
		controllerutil.RemoveFinalizer(secret, finalizerName)
		return r.Update(ctx, secret)
	}
	return nil
}

func (r *AccessServiceTokenReconciler) updateStatusWithWarning(
	ctx context.Context, token *networkingv1alpha2.AccessServiceToken, warning string,
) (ctrl.Result, error) {
	// Use retry logic for status updates to handle conflicts
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, token, func() {
		token.Status.State = "Warning"
		meta.SetStatusCondition(&token.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "AdoptedWithWarning",
			Message:            warning,
			LastTransitionTime: metav1.Now(),
		})
		token.Status.ObservedGeneration = token.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *AccessServiceTokenReconciler) createOrUpdateSecret(ctx context.Context, token *networkingv1alpha2.AccessServiceToken, result *cf.AccessServiceTokenResult) error {
	secretName := token.Spec.SecretRef.Name
	secretNamespace := token.Spec.SecretRef.Namespace
	if secretNamespace == "" {
		secretNamespace = token.Namespace
	}

	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: secretNamespace}, secret)

	if errors.IsNotFound(err) {
		// Create new secret
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: secretNamespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "cloudflare-operator",
					"app.kubernetes.io/name":       token.Name,
				},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"CF_ACCESS_CLIENT_ID":     []byte(result.ClientID),
				"CF_ACCESS_CLIENT_SECRET": []byte(result.ClientSecret),
			},
		}
		return r.Create(ctx, secret)
	} else if err != nil {
		return err
	}

	// Update existing secret
	secret.Data = map[string][]byte{
		"CF_ACCESS_CLIENT_ID":     []byte(result.ClientID),
		"CF_ACCESS_CLIENT_SECRET": []byte(result.ClientSecret),
	}
	return r.Update(ctx, secret)
}

func (r *AccessServiceTokenReconciler) updateStatusError(ctx context.Context, token *networkingv1alpha2.AccessServiceToken, err error) (ctrl.Result, error) {
	// Use retry logic for status updates to handle conflicts
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, token, func() {
		token.Status.State = "Error"
		meta.SetStatusCondition(&token.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		token.Status.ObservedGeneration = token.Generation
	})

	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *AccessServiceTokenReconciler) updateStatusSuccess(ctx context.Context, token *networkingv1alpha2.AccessServiceToken, result *cf.AccessServiceTokenResult) (ctrl.Result, error) {
	// Use retry logic for status updates to handle conflicts
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, token, func() {
		token.Status.TokenID = result.TokenID
		token.Status.ClientID = result.ClientID
		token.Status.AccountID = result.AccountID
		if result.ExpiresAt != "" {
			token.Status.ExpiresAt = result.ExpiresAt
		}
		token.Status.State = "Ready"
		meta.SetStatusCondition(&token.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "Reconciled",
			Message:            "Access Service Token successfully reconciled",
			LastTransitionTime: metav1.Now(),
		})
		token.Status.ObservedGeneration = token.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *AccessServiceTokenReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("accessservicetoken-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.AccessServiceToken{}).
		Complete(r)
}
