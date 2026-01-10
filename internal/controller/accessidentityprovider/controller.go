// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package accessidentityprovider

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
)

const (
	FinalizerName = "accessidentityprovider.networking.cloudflare-operator.io/finalizer"
)

// AccessIdentityProviderReconciler reconciles an AccessIdentityProvider object
type AccessIdentityProviderReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessidentityproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessidentityproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessidentityproviders/finalizers,verbs=update

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
	// AccessIdentityProvider is cluster-scoped, use operator namespace for legacy inline secrets
	apiClient, err := cf.NewAPIClientFromDetails(ctx, r.Client, controller.OperatorNamespace, idp.Spec.Cloudflare)
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
				// P0 FIX: Check if resource already deleted
				if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to delete Access Identity Provider from Cloudflare")
					r.Recorder.Event(idp, corev1.EventTypeWarning, controller.EventReasonDeleteFailed,
						fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
					return ctrl.Result{RequeueAfter: 30 * time.Second}, err
				}
				logger.Info("Access Identity Provider already deleted from Cloudflare")
				r.Recorder.Event(idp, corev1.EventTypeNormal, "AlreadyDeleted", "Access Identity Provider was already deleted from Cloudflare")
			} else {
				r.Recorder.Event(idp, corev1.EventTypeNormal, controller.EventReasonDeleted, "Deleted from Cloudflare")
			}
		}

		// P0 FIX: Remove finalizer with retry logic to handle conflicts
		if err := controller.UpdateWithConflictRetry(ctx, r.Client, idp, func() {
			controllerutil.RemoveFinalizer(idp, FinalizerName)
		}); err != nil {
			logger.Error(err, "failed to remove finalizer")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(idp, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")
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
		r.Recorder.Event(idp, corev1.EventTypeNormal, "Creating",
			fmt.Sprintf("Creating Access Identity Provider '%s' (type: %s) in Cloudflare", params.Name, params.Type))
		result, err = apiClient.CreateAccessIdentityProvider(params)
		if err != nil {
			r.Recorder.Event(idp, corev1.EventTypeWarning, controller.EventReasonCreateFailed,
				fmt.Sprintf("Failed to create Access Identity Provider: %s", cf.SanitizeErrorMessage(err)))
			return r.updateStatusError(ctx, idp, err)
		}
		r.Recorder.Event(idp, corev1.EventTypeNormal, controller.EventReasonCreated,
			fmt.Sprintf("Created Access Identity Provider with ID '%s'", result.ID))
	} else {
		// Update existing identity provider
		logger.Info("Updating Access Identity Provider", "providerId", idp.Status.ProviderID)
		r.Recorder.Event(idp, corev1.EventTypeNormal, "Updating",
			fmt.Sprintf("Updating Access Identity Provider '%s' in Cloudflare", idp.Status.ProviderID))
		result, err = apiClient.UpdateAccessIdentityProvider(idp.Status.ProviderID, params)
		if err != nil {
			r.Recorder.Event(idp, corev1.EventTypeWarning, controller.EventReasonUpdateFailed,
				fmt.Sprintf("Failed to update Access Identity Provider: %s", cf.SanitizeErrorMessage(err)))
			return r.updateStatusError(ctx, idp, err)
		}
		r.Recorder.Event(idp, corev1.EventTypeNormal, controller.EventReasonUpdated,
			fmt.Sprintf("Updated Access Identity Provider '%s'", result.ID))
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
	// P0 FIX: Use UpdateStatusWithConflictRetry for status updates
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, idp, func() {
		idp.Status.State = "Error"
		meta.SetStatusCondition(&idp.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: idp.Generation,
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		idp.Status.ObservedGeneration = idp.Generation
	})
	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *AccessIdentityProviderReconciler) updateStatusSuccess(ctx context.Context, idp *networkingv1alpha2.AccessIdentityProvider, result *cf.AccessIdentityProviderResult) (ctrl.Result, error) {
	// P0 FIX: Use UpdateStatusWithConflictRetry for status updates
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, idp, func() {
		idp.Status.ProviderID = result.ID
		idp.Status.State = "Ready"
		meta.SetStatusCondition(&idp.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: idp.Generation,
			Reason:             "Reconciled",
			Message:            "Access Identity Provider successfully reconciled",
			LastTransitionTime: metav1.Now(),
		})
		idp.Status.ObservedGeneration = idp.Generation
	})
	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *AccessIdentityProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("accessidentityprovider-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.AccessIdentityProvider{}).
		Complete(r)
}
