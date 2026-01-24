// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package accessidentityprovider provides a controller for managing Cloudflare Access Identity Providers.
// It directly calls Cloudflare API and writes status back to the CRD.
package accessidentityprovider

import (
	"context"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	finalizerName = "accessidentityprovider.networking.cloudflare-operator.io/finalizer"
)

// Reconciler reconciles an AccessIdentityProvider object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessidentityproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessidentityproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessidentityproviders/finalizers,verbs=update

// Reconcile handles AccessIdentityProvider reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the AccessIdentityProvider resource
	idp := &networkingv1alpha2.AccessIdentityProvider{}
	if err := r.Get(ctx, req.NamespacedName, idp); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch AccessIdentityProvider")
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !idp.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, idp)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, idp, finalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Get API client
	// AccessIdentityProvider is cluster-scoped, use operator namespace for legacy inline secrets
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &idp.Spec.Cloudflare,
		Namespace:         common.OperatorNamespace,
		StatusAccountID:   idp.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, idp, err)
	}

	// Sync identity provider to Cloudflare
	return r.syncIdentityProvider(ctx, idp, apiResult)
}

// handleDeletion handles the deletion of AccessIdentityProvider.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	idp *networkingv1alpha2.AccessIdentityProvider,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(idp, finalizerName) {
		return common.NoRequeue(), nil
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &idp.Spec.Cloudflare,
		Namespace:         common.OperatorNamespace,
		StatusAccountID:   idp.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client for deletion")
		// Continue with finalizer removal
	} else if idp.Status.ProviderID != "" {
		// Delete identity provider from Cloudflare
		logger.Info("Deleting Access Identity Provider from Cloudflare",
			"providerId", idp.Status.ProviderID)

		if err := apiResult.API.DeleteAccessIdentityProvider(ctx, idp.Status.ProviderID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete Access Identity Provider from Cloudflare")
				r.Recorder.Event(idp, corev1.EventTypeWarning, "DeleteFailed",
					fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
				return common.RequeueShort(), err
			}
			logger.Info("Access Identity Provider not found in Cloudflare, may have been already deleted")
		}

		r.Recorder.Event(idp, corev1.EventTypeNormal, "Deleted",
			"Access Identity Provider deleted from Cloudflare")
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, idp, func() {
		controllerutil.RemoveFinalizer(idp, finalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(idp, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// syncIdentityProvider syncs the Access Identity Provider to Cloudflare.
func (r *Reconciler) syncIdentityProvider(
	ctx context.Context,
	idp *networkingv1alpha2.AccessIdentityProvider,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Determine provider name
	providerName := idp.GetProviderName()

	// Build params
	params := r.buildParams(idp, providerName)

	// Check if provider already exists by ID
	if idp.Status.ProviderID != "" {
		existing, err := apiResult.API.GetAccessIdentityProvider(ctx, idp.Status.ProviderID)
		if err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to get Access Identity Provider from Cloudflare")
				return r.updateStatusError(ctx, idp, err)
			}
			// Provider doesn't exist, will create
			logger.Info("Access Identity Provider not found in Cloudflare, will recreate",
				"providerId", idp.Status.ProviderID)
		} else {
			// Provider exists, update it
			logger.V(1).Info("Updating Access Identity Provider in Cloudflare",
				"providerId", existing.ID,
				"name", providerName)

			result, err := apiResult.API.UpdateAccessIdentityProvider(ctx, existing.ID, params)
			if err != nil {
				logger.Error(err, "Failed to update Access Identity Provider")
				return r.updateStatusError(ctx, idp, err)
			}

			r.Recorder.Event(idp, corev1.EventTypeNormal, "Updated",
				fmt.Sprintf("Access Identity Provider '%s' updated in Cloudflare", providerName))

			return r.updateStatusReady(ctx, idp, apiResult.AccountID, result.ID)
		}
	}

	// Try to find existing provider by name
	existingByName, err := apiResult.API.ListAccessIdentityProvidersByName(ctx, providerName)
	if err != nil && !cf.IsNotFoundError(err) {
		logger.Error(err, "Failed to search for existing Access Identity Provider")
		return r.updateStatusError(ctx, idp, err)
	}

	if existingByName != nil {
		// Provider already exists with this name, adopt it
		logger.Info("Access Identity Provider already exists with same name, adopting it",
			"providerId", existingByName.ID,
			"name", providerName)

		// Update the existing provider
		result, err := apiResult.API.UpdateAccessIdentityProvider(ctx, existingByName.ID, params)
		if err != nil {
			logger.Error(err, "Failed to update existing Access Identity Provider")
			return r.updateStatusError(ctx, idp, err)
		}

		r.Recorder.Event(idp, corev1.EventTypeNormal, "Adopted",
			fmt.Sprintf("Adopted existing Access Identity Provider '%s'", providerName))

		return r.updateStatusReady(ctx, idp, apiResult.AccountID, result.ID)
	}

	// Create new provider
	logger.Info("Creating Access Identity Provider in Cloudflare",
		"name", providerName)

	result, err := apiResult.API.CreateAccessIdentityProvider(ctx, params)
	if err != nil {
		logger.Error(err, "Failed to create Access Identity Provider")
		return r.updateStatusError(ctx, idp, err)
	}

	r.Recorder.Event(idp, corev1.EventTypeNormal, "Created",
		fmt.Sprintf("Access Identity Provider '%s' created in Cloudflare", providerName))

	return r.updateStatusReady(ctx, idp, apiResult.AccountID, result.ID)
}

// buildParams builds the AccessIdentityProviderParams from the AccessIdentityProvider spec.
func (r *Reconciler) buildParams(idp *networkingv1alpha2.AccessIdentityProvider, providerName string) cf.AccessIdentityProviderParams {
	params := cf.AccessIdentityProviderParams{
		Name: providerName,
		Type: idp.Spec.Type,
	}

	// Convert config
	if idp.Spec.Config != nil {
		params.Config = convertConfigToCF(idp.Spec.Config)
	}

	// Convert SCIM config
	if idp.Spec.ScimConfig != nil {
		params.ScimConfig = convertScimConfigToCF(idp.Spec.ScimConfig)
	}

	return params
}

// convertConfigToCF converts IdentityProviderConfig to cloudflare.AccessIdentityProviderConfiguration.
func convertConfigToCF(config *networkingv1alpha2.IdentityProviderConfig) cloudflare.AccessIdentityProviderConfiguration {
	result := cloudflare.AccessIdentityProviderConfiguration{
		ClientID:                  config.ClientID,
		ClientSecret:              config.ClientSecret,
		AppsDomain:                config.AppsDomain,
		AuthURL:                   config.AuthURL,
		TokenURL:                  config.TokenURL,
		CertsURL:                  config.CertsURL,
		Scopes:                    config.Scopes,
		Attributes:                config.Attributes,
		IdpPublicCert:             config.IdPPublicCert,
		IssuerURL:                 config.IssuerURL,
		SsoTargetURL:              config.SSOTargetURL,
		EmailClaimName:            config.EmailClaimName,
		DirectoryID:               config.DirectoryID,
		PKCEEnabled:               config.PKCEEnabled,
		Claims:                    config.Claims,
		EmailAttributeName:        config.EmailAttributeName,
		APIToken:                  config.APIToken,
		OktaAccount:               config.OktaAccount,
		OktaAuthorizationServerID: config.OktaAuthorizationServerID,
		OneloginAccount:           config.OneloginAccount,
		PingEnvID:                 config.PingEnvID,
		CentrifyAccount:           config.CentrifyAccount,
		CentrifyAppID:             config.CentrifyAppID,
		RedirectURL:               config.RedirectURL,
	}

	// Handle bool pointer to bool conversions
	if config.SignRequest != nil {
		result.SignRequest = *config.SignRequest
	}
	if config.SupportGroups != nil {
		result.SupportGroups = *config.SupportGroups
	}
	if config.ConditionalAccessEnabled != nil {
		result.ConditionalAccessEnabled = *config.ConditionalAccessEnabled
	}

	return result
}

// convertScimConfigToCF converts IdentityProviderScimConfig to cloudflare.AccessIdentityProviderScimConfiguration.
func convertScimConfigToCF(scimConfig *networkingv1alpha2.IdentityProviderScimConfig) cloudflare.AccessIdentityProviderScimConfiguration {
	result := cloudflare.AccessIdentityProviderScimConfiguration{
		Secret:                 scimConfig.Secret,
		IdentityUpdateBehavior: scimConfig.IdentityUpdateBehavior,
	}

	// Handle bool pointer to bool conversions
	if scimConfig.Enabled != nil {
		result.Enabled = *scimConfig.Enabled
	}
	if scimConfig.UserDeprovision != nil {
		result.UserDeprovision = *scimConfig.UserDeprovision
	}
	if scimConfig.SeatDeprovision != nil {
		result.SeatDeprovision = *scimConfig.SeatDeprovision
	}
	if scimConfig.GroupMemberDeprovision != nil {
		result.GroupMemberDeprovision = *scimConfig.GroupMemberDeprovision
	}

	return result
}

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	idp *networkingv1alpha2.AccessIdentityProvider,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, idp, func() {
		idp.Status.State = "Error"
		meta.SetStatusCondition(&idp.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: idp.Generation,
			Reason:             "Error",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		idp.Status.ObservedGeneration = idp.Generation
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
}

func (r *Reconciler) updateStatusReady(
	ctx context.Context,
	idp *networkingv1alpha2.AccessIdentityProvider,
	accountID, providerID string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, idp, func() {
		idp.Status.AccountID = accountID
		idp.Status.ProviderID = providerID
		idp.Status.State = "Ready"
		meta.SetStatusCondition(&idp.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: idp.Generation,
			Reason:             "Synced",
			Message:            "Access Identity Provider synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		idp.Status.ObservedGeneration = idp.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("accessidentityprovider-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("accessidentityprovider"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.AccessIdentityProvider{}).
		Named("accessidentityprovider").
		Complete(r)
}
