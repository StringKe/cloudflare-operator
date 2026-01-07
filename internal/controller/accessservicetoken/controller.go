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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/adyanth/cloudflare-operator/api/v1alpha2"
	"github.com/adyanth/cloudflare-operator/internal/clients/cf"
)

const (
	FinalizerName = "accessservicetoken.networking.cloudflare-operator.io/finalizer"
)

// AccessServiceTokenReconciler reconciles an AccessServiceToken object
type AccessServiceTokenReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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
				logger.Error(err, "Failed to delete Access Service Token from Cloudflare")
			}
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
		// Create new service token
		logger.Info("Creating Access Service Token", "name", tokenName)
		result, err = apiClient.CreateAccessServiceToken(tokenName, duration)
		if err != nil {
			return r.updateStatusError(ctx, token, err)
		}

		// Store the client secret in a Kubernetes secret (only available on creation)
		if token.Spec.SecretRef.Name != "" && result.ClientSecret != "" {
			if err := r.createOrUpdateSecret(ctx, token, result); err != nil {
				return r.updateStatusError(ctx, token, err)
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

	// Update status
	return r.updateStatusSuccess(ctx, token, result)
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
	token.Status.State = "Error"
	meta.SetStatusCondition(&token.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "ReconcileError",
		Message:            err.Error(),
		LastTransitionTime: metav1.Now(),
	})
	token.Status.ObservedGeneration = token.Generation

	if updateErr := r.Status().Update(ctx, token); updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *AccessServiceTokenReconciler) updateStatusSuccess(ctx context.Context, token *networkingv1alpha2.AccessServiceToken, result *cf.AccessServiceTokenResult) (ctrl.Result, error) {
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

	if err := r.Status().Update(ctx, token); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *AccessServiceTokenReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.AccessServiceToken{}).
		Complete(r)
}
