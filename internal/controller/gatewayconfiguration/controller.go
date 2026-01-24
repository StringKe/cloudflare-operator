// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package gatewayconfiguration provides a controller for managing Cloudflare Gateway Configuration.
// It directly calls Cloudflare API and writes status back to the CRD.
package gatewayconfiguration

import (
	"context"
	"fmt"

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
	finalizerName = "gatewayconfiguration.networking.cloudflare-operator.io/finalizer"
)

// Reconciler reconciles a GatewayConfiguration object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayconfigurations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayconfigurations/finalizers,verbs=update

// Reconcile handles GatewayConfiguration reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the GatewayConfiguration resource
	config := &networkingv1alpha2.GatewayConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch GatewayConfiguration")
		return common.NoRequeue(), err
	}

	// Handle deletion - Gateway config is account-level, don't delete from CF
	if !config.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, config)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, config, finalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Get API client
	// GatewayConfiguration is cluster-scoped, use operator namespace for legacy inline secrets
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &config.Spec.Cloudflare,
		Namespace:         common.OperatorNamespace,
		StatusAccountID:   config.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, config, err)
	}

	// Sync Gateway configuration to Cloudflare
	return r.syncGatewayConfiguration(ctx, config, apiResult)
}

// handleDeletion handles the deletion of GatewayConfiguration.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	config *networkingv1alpha2.GatewayConfiguration,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(config, finalizerName) {
		return common.NoRequeue(), nil
	}

	// Gateway configuration is account-level, we don't delete it from Cloudflare
	// Just remove the finalizer
	logger.Info("Removing finalizer for GatewayConfiguration (account-level config not deleted from CF)")

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, config, func() {
		controllerutil.RemoveFinalizer(config, finalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(config, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// syncGatewayConfiguration syncs the Gateway Configuration to Cloudflare.
func (r *Reconciler) syncGatewayConfiguration(
	ctx context.Context,
	config *networkingv1alpha2.GatewayConfiguration,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Build params from spec
	params := r.buildParams(config.Spec.Settings)

	// Update Gateway configuration in Cloudflare
	logger.V(1).Info("Updating Gateway Configuration in Cloudflare")

	result, err := apiResult.API.UpdateGatewayConfiguration(ctx, params)
	if err != nil {
		logger.Error(err, "Failed to update Gateway Configuration")
		return r.updateStatusError(ctx, config, err)
	}

	r.Recorder.Event(config, corev1.EventTypeNormal, "Updated",
		"Gateway Configuration updated in Cloudflare")

	return r.updateStatusReady(ctx, config, result.AccountID)
}

// buildParams builds the GatewayConfigurationParams from the GatewaySettings.
//
//nolint:revive // cognitive complexity is acceptable for config construction
func (r *Reconciler) buildParams(settings networkingv1alpha2.GatewaySettings) cf.GatewayConfigurationParams {
	params := cf.GatewayConfigurationParams{}

	if settings.TLSDecrypt != nil {
		params.TLSDecrypt = &cf.TLSDecryptSettings{
			Enabled: settings.TLSDecrypt.Enabled,
		}
	}

	if settings.ActivityLog != nil {
		params.ActivityLog = &cf.ActivityLogSettings{
			Enabled: settings.ActivityLog.Enabled,
		}
	}

	if settings.AntiVirus != nil {
		av := &cf.AntiVirusSettings{
			EnabledDownloadPhase: settings.AntiVirus.EnabledDownloadPhase,
			EnabledUploadPhase:   settings.AntiVirus.EnabledUploadPhase,
			FailClosed:           settings.AntiVirus.FailClosed,
		}
		if settings.AntiVirus.NotificationSettings != nil {
			av.NotificationSettings = &cf.NotificationSettings{
				Enabled:    settings.AntiVirus.NotificationSettings.Enabled,
				Message:    settings.AntiVirus.NotificationSettings.Message,
				SupportURL: settings.AntiVirus.NotificationSettings.SupportURL,
			}
		}
		params.AntiVirus = av
	}

	if settings.BlockPage != nil {
		params.BlockPage = &cf.BlockPageSettings{
			Enabled:         settings.BlockPage.Enabled,
			Name:            settings.BlockPage.Name,
			FooterText:      settings.BlockPage.FooterText,
			HeaderText:      settings.BlockPage.HeaderText,
			LogoPath:        settings.BlockPage.LogoPath,
			BackgroundColor: settings.BlockPage.BackgroundColor,
			MailtoAddress:   settings.BlockPage.MailtoAddress,
			MailtoSubject:   settings.BlockPage.MailtoSubject,
			SuppressFooter:  &settings.BlockPage.SuppressFooter,
		}
	}

	if settings.BodyScanning != nil {
		params.BodyScanning = &cf.BodyScanningSettings{
			InspectionMode: settings.BodyScanning.InspectionMode,
		}
	}

	if settings.BrowserIsolation != nil {
		params.BrowserIsolation = &cf.BrowserIsolationSettings{
			URLBrowserIsolationEnabled: settings.BrowserIsolation.URLBrowserIsolationEnabled,
			NonIdentityEnabled:         settings.BrowserIsolation.NonIdentityEnabled,
		}
	}

	if settings.FIPS != nil {
		params.FIPS = &cf.FIPSSettings{
			TLS: settings.FIPS.TLS,
		}
	}

	if settings.ProtocolDetection != nil {
		params.ProtocolDetection = &cf.ProtocolDetectionSettings{
			Enabled: settings.ProtocolDetection.Enabled,
		}
	}

	if settings.CustomCertificate != nil {
		params.CustomCertificate = &cf.CustomCertificateSettings{
			Enabled: settings.CustomCertificate.Enabled,
			ID:      settings.CustomCertificate.ID,
		}
	}

	// Merge deprecated NonIdentityBrowserIsolation into BrowserIsolation.NonIdentityEnabled
	// This maintains backward compatibility while the field is deprecated.
	if settings.NonIdentityBrowserIsolation != nil && settings.NonIdentityBrowserIsolation.Enabled {
		if params.BrowserIsolation == nil {
			params.BrowserIsolation = &cf.BrowserIsolationSettings{}
		}
		params.BrowserIsolation.NonIdentityEnabled = true
	}

	return params
}

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	config *networkingv1alpha2.GatewayConfiguration,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, config, func() {
		config.Status.State = "Error"
		meta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: config.Generation,
			Reason:             "Error",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		config.Status.ObservedGeneration = config.Generation
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
}

func (r *Reconciler) updateStatusReady(
	ctx context.Context,
	config *networkingv1alpha2.GatewayConfiguration,
	accountID string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, config, func() {
		config.Status.AccountID = accountID
		config.Status.State = "Ready"
		meta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: config.Generation,
			Reason:             "Synced",
			Message:            "Gateway Configuration synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		config.Status.ObservedGeneration = config.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("gatewayconfiguration-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("gatewayconfiguration"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.GatewayConfiguration{}).
		Named("gatewayconfiguration").
		Complete(r)
}
