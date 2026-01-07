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

package accessidentityprovider

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/adyanth/cloudflare-operator/api/v1alpha2"
	"github.com/adyanth/cloudflare-operator/internal/clients/cf"
)

const (
	FinalizerName = "accessidentityprovider.networking.cfargotunnel.com/finalizer"
)

// AccessIdentityProviderReconciler reconciles an AccessIdentityProvider object
type AccessIdentityProviderReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=networking.cfargotunnel.com,resources=accessidentityproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cfargotunnel.com,resources=accessidentityproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cfargotunnel.com,resources=accessidentityproviders/finalizers,verbs=update

func (r *AccessIdentityProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the AccessIdentityProvider instance
	idp := &networkingv1alpha2.AccessIdentityProvider{}
	if err := r.Get(ctx, req.NamespacedName, idp); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Initialize API client
	apiClient, err := cf.NewAPIClientFromDetails(ctx, r.Client, "", idp.Spec.Cloudflare)
	if err != nil {
		logger.Error(err, "Failed to initialize Cloudflare API client")
		return r.updateStatusError(ctx, idp, err)
	}

	// Handle deletion
	if !idp.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, idp, apiClient)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(idp, FinalizerName) {
		controllerutil.AddFinalizer(idp, FinalizerName)
		if err := r.Update(ctx, idp); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile the identity provider
	return r.reconcileIdentityProvider(ctx, idp, apiClient)
}

func (r *AccessIdentityProviderReconciler) handleDeletion(ctx context.Context, idp *networkingv1alpha2.AccessIdentityProvider, apiClient *cf.API) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(idp, FinalizerName) {
		// Delete from Cloudflare
		if idp.Status.ProviderID != "" {
			logger.Info("Deleting Access Identity Provider from Cloudflare", "providerId", idp.Status.ProviderID)
			if err := apiClient.DeleteAccessIdentityProvider(idp.Status.ProviderID); err != nil {
				logger.Error(err, "Failed to delete Access Identity Provider from Cloudflare")
			}
		}

		// Remove finalizer
		controllerutil.RemoveFinalizer(idp, FinalizerName)
		if err := r.Update(ctx, idp); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *AccessIdentityProviderReconciler) reconcileIdentityProvider(ctx context.Context, idp *networkingv1alpha2.AccessIdentityProvider, apiClient *cf.API) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Build identity provider params
	params := cf.AccessIdentityProviderParams{
		Name: idp.GetProviderName(),
		Type: idp.Spec.Type,
	}

	// Build config based on type
	if idp.Spec.Config != nil {
		params.Config = r.buildConfig(idp.Spec.Config)
	}

	var result *cf.AccessIdentityProviderResult
	var err error

	if idp.Status.ProviderID == "" {
		// Create new identity provider
		logger.Info("Creating Access Identity Provider", "name", params.Name, "type", params.Type)
		result, err = apiClient.CreateAccessIdentityProvider(params)
	} else {
		// Update existing identity provider
		logger.Info("Updating Access Identity Provider", "providerId", idp.Status.ProviderID)
		result, err = apiClient.UpdateAccessIdentityProvider(idp.Status.ProviderID, params)
	}

	if err != nil {
		return r.updateStatusError(ctx, idp, err)
	}

	// Update status
	return r.updateStatusSuccess(ctx, idp, result)
}

func (r *AccessIdentityProviderReconciler) buildConfig(config *networkingv1alpha2.IdentityProviderConfig) cloudflare.AccessIdentityProviderConfiguration {
	result := cloudflare.AccessIdentityProviderConfiguration{}

	if config.ClientID != "" {
		result.ClientID = config.ClientID
	}
	if config.ClientSecret != "" {
		result.ClientSecret = config.ClientSecret
	}
	if config.AuthURL != "" {
		result.AuthURL = config.AuthURL
	}
	if config.TokenURL != "" {
		result.TokenURL = config.TokenURL
	}
	if config.CertsURL != "" {
		result.CertsURL = config.CertsURL
	}
	if len(config.IdPPublicCerts) > 0 {
		result.IdpPublicCert = config.IdPPublicCerts[0]
	}
	if config.SSOTargetURL != "" {
		result.SsoTargetURL = config.SSOTargetURL
	}
	if config.SignRequest != nil {
		result.SignRequest = *config.SignRequest
	}
	if config.EmailClaimName != "" {
		result.EmailClaimName = config.EmailClaimName
	}
	if len(config.Claims) > 0 {
		result.Claims = config.Claims
	}
	if len(config.Scopes) > 0 {
		result.Scopes = config.Scopes
	}
	if config.DirectoryID != "" {
		result.DirectoryID = config.DirectoryID
	}
	if config.AppsDomain != "" {
		result.AppsDomain = config.AppsDomain
	}
	if config.PKCEEnabled != nil {
		result.PKCEEnabled = config.PKCEEnabled
	}
	if config.ConditionalAccessEnabled != nil {
		result.ConditionalAccessEnabled = *config.ConditionalAccessEnabled
	}
	if config.SupportGroups != nil {
		result.SupportGroups = *config.SupportGroups
	}
	if config.IssuerURL != "" {
		result.IssuerURL = config.IssuerURL
	}
	if config.EmailAttributeName != "" {
		result.EmailAttributeName = config.EmailAttributeName
	}
	if config.APIToken != "" {
		result.APIToken = config.APIToken
	}
	if config.OktaAccount != "" {
		result.OktaAccount = config.OktaAccount
	}
	if config.OneloginAccount != "" {
		result.OneloginAccount = config.OneloginAccount
	}
	if config.PingEnvID != "" {
		result.PingEnvID = config.PingEnvID
	}
	if config.CentrifyAccount != "" {
		result.CentrifyAccount = config.CentrifyAccount
	}
	if config.CentrifyAppID != "" {
		result.CentrifyAppID = config.CentrifyAppID
	}
	if config.RedirectURL != "" {
		result.RedirectURL = config.RedirectURL
	}

	return result
}

func (r *AccessIdentityProviderReconciler) updateStatusError(ctx context.Context, idp *networkingv1alpha2.AccessIdentityProvider, err error) (ctrl.Result, error) {
	idp.Status.State = "Error"
	meta.SetStatusCondition(&idp.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "ReconcileError",
		Message:            err.Error(),
		LastTransitionTime: metav1.Now(),
	})
	idp.Status.ObservedGeneration = idp.Generation

	if updateErr := r.Status().Update(ctx, idp); updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *AccessIdentityProviderReconciler) updateStatusSuccess(ctx context.Context, idp *networkingv1alpha2.AccessIdentityProvider, result *cf.AccessIdentityProviderResult) (ctrl.Result, error) {
	idp.Status.ProviderID = result.ID
	idp.Status.State = "Ready"
	meta.SetStatusCondition(&idp.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "Access Identity Provider successfully reconciled",
		LastTransitionTime: metav1.Now(),
	})
	idp.Status.ObservedGeneration = idp.Generation

	if err := r.Status().Update(ctx, idp); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *AccessIdentityProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.AccessIdentityProvider{}).
		Complete(r)
}
