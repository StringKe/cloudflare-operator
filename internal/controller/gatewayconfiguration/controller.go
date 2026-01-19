// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package gatewayconfiguration

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	"github.com/StringKe/cloudflare-operator/internal/service"
	gatewaysvc "github.com/StringKe/cloudflare-operator/internal/service/gateway"
)

const (
	FinalizerName = "gatewayconfiguration.networking.cloudflare-operator.io/finalizer"
)

// GatewayConfigurationReconciler reconciles a GatewayConfiguration object
type GatewayConfigurationReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Recorder       record.EventRecorder
	gatewayService *gatewaysvc.GatewayConfigurationService
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayconfigurations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayconfigurations/finalizers,verbs=update

func (r *GatewayConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the GatewayConfiguration instance
	config := &networkingv1alpha2.GatewayConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Resolve credentials
	credRef, accountID, err := r.resolveCredentials(ctx, config)
	if err != nil {
		logger.Error(err, "Failed to resolve credentials")
		return r.updateStatusError(ctx, config, err)
	}

	// Handle deletion - for gateway config, we don't delete, just remove finalizer
	if !config.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, config)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(config, FinalizerName) {
		controllerutil.AddFinalizer(config, FinalizerName)
		if err := r.Update(ctx, config); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Register Gateway configuration to SyncState
	return r.registerGatewayConfiguration(ctx, config, accountID, credRef)
}

// resolveCredentials resolves the credentials reference and account ID.
func (r *GatewayConfigurationReconciler) resolveCredentials(
	ctx context.Context,
	config *networkingv1alpha2.GatewayConfiguration,
) (credRef networkingv1alpha2.CredentialsReference, accountID string, err error) {
	// Get credentials reference
	if config.Spec.Cloudflare.CredentialsRef != nil {
		credRef = networkingv1alpha2.CredentialsReference{
			Name: config.Spec.Cloudflare.CredentialsRef.Name,
		}

		// Get account ID from credentials if available
		creds := &networkingv1alpha2.CloudflareCredentials{}
		if err := r.Get(ctx, client.ObjectKey{Name: credRef.Name}, creds); err != nil {
			return credRef, "", fmt.Errorf("get credentials: %w", err)
		}
		accountID = creds.Spec.AccountID
	}

	if credRef.Name == "" {
		return credRef, "", errors.New("credentials reference is required")
	}

	return credRef, accountID, nil
}

func (r *GatewayConfigurationReconciler) handleDeletion(ctx context.Context, config *networkingv1alpha2.GatewayConfiguration) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(config, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Gateway configuration is account-level, we don't delete it from Cloudflare
	// Just unregister from SyncState and remove the finalizer

	// Unregister from SyncState
	source := service.Source{
		Kind:      "GatewayConfiguration",
		Namespace: "",
		Name:      config.Name,
	}
	if err := r.gatewayService.Unregister(ctx, config.Status.AccountID, source); err != nil {
		logger.Error(err, "Failed to unregister Gateway configuration from SyncState")
		// Non-fatal - continue with finalizer removal
	}

	// Remove finalizer with retry logic
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, config, func() {
		controllerutil.RemoveFinalizer(config, FinalizerName)
	}); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(config, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// registerGatewayConfiguration registers the Gateway configuration to SyncState.
func (r *GatewayConfigurationReconciler) registerGatewayConfiguration(
	ctx context.Context,
	config *networkingv1alpha2.GatewayConfiguration,
	accountID string,
	credRef networkingv1alpha2.CredentialsReference,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Build Gateway configuration
	svcConfig := r.buildConfigParams(config.Spec.Settings)

	// Create source reference
	source := service.Source{
		Kind:      "GatewayConfiguration",
		Namespace: "",
		Name:      config.Name,
	}

	// Register to SyncState
	opts := gatewaysvc.GatewayConfigurationRegisterOptions{
		AccountID:      accountID,
		Source:         source,
		Config:         svcConfig,
		CredentialsRef: credRef,
	}

	if err := r.gatewayService.Register(ctx, opts); err != nil {
		logger.Error(err, "Failed to register Gateway configuration")
		r.Recorder.Event(config, corev1.EventTypeWarning, "RegisterFailed",
			fmt.Sprintf("Failed to register Gateway configuration: %s", err.Error()))
		return r.updateStatusError(ctx, config, err)
	}

	r.Recorder.Event(config, corev1.EventTypeNormal, "Registered",
		"Registered Gateway Configuration to SyncState")

	// Update status to Pending - actual sync happens via GatewaySyncController
	return r.updateStatusPending(ctx, config, accountID)
}

//nolint:revive // cognitive complexity is acceptable for config construction
func (*GatewayConfigurationReconciler) buildConfigParams(settings networkingv1alpha2.GatewaySettings) gatewaysvc.GatewayConfigurationConfig {
	config := gatewaysvc.GatewayConfigurationConfig{}

	if settings.TLSDecrypt != nil {
		config.TLSDecrypt = &gatewaysvc.TLSDecryptSettings{
			Enabled: settings.TLSDecrypt.Enabled,
		}
	}

	if settings.ActivityLog != nil {
		config.ActivityLog = &gatewaysvc.ActivityLogSettings{
			Enabled: settings.ActivityLog.Enabled,
		}
	}

	if settings.AntiVirus != nil {
		av := &gatewaysvc.AntiVirusSettings{
			EnabledDownloadPhase: settings.AntiVirus.EnabledDownloadPhase,
			EnabledUploadPhase:   settings.AntiVirus.EnabledUploadPhase,
			FailClosed:           settings.AntiVirus.FailClosed,
		}
		if settings.AntiVirus.NotificationSettings != nil {
			av.NotificationSettings = &gatewaysvc.NotificationSettings{
				Enabled:    settings.AntiVirus.NotificationSettings.Enabled,
				Message:    settings.AntiVirus.NotificationSettings.Message,
				SupportURL: settings.AntiVirus.NotificationSettings.SupportURL,
			}
		}
		config.AntiVirus = av
	}

	if settings.BlockPage != nil {
		config.BlockPage = &gatewaysvc.BlockPageSettings{
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
		config.BodyScanning = &gatewaysvc.BodyScanningSettings{
			InspectionMode: settings.BodyScanning.InspectionMode,
		}
	}

	if settings.BrowserIsolation != nil {
		config.BrowserIsolation = &gatewaysvc.BrowserIsolationSettings{
			URLBrowserIsolationEnabled: settings.BrowserIsolation.URLBrowserIsolationEnabled,
			NonIdentityEnabled:         settings.BrowserIsolation.NonIdentityEnabled,
		}
	}

	if settings.FIPS != nil {
		config.FIPS = &gatewaysvc.FIPSSettings{
			TLS: settings.FIPS.TLS,
		}
	}

	if settings.ProtocolDetection != nil {
		config.ProtocolDetection = &gatewaysvc.ProtocolDetectionSettings{
			Enabled: settings.ProtocolDetection.Enabled,
		}
	}

	if settings.CustomCertificate != nil {
		config.CustomCertificate = &gatewaysvc.CustomCertificateSettings{
			Enabled: settings.CustomCertificate.Enabled,
			ID:      settings.CustomCertificate.ID,
		}
	}

	// Merge deprecated NonIdentityBrowserIsolation into BrowserIsolation.NonIdentityEnabled
	// This maintains backward compatibility while the field is deprecated.
	// See: BrowserIsolation.NonIdentityEnabled is the canonical field for this setting.
	if settings.NonIdentityBrowserIsolation != nil && settings.NonIdentityBrowserIsolation.Enabled {
		if config.BrowserIsolation == nil {
			config.BrowserIsolation = &gatewaysvc.BrowserIsolationSettings{}
		}
		config.BrowserIsolation.NonIdentityEnabled = true
	}

	return config
}

func (r *GatewayConfigurationReconciler) updateStatusError(ctx context.Context, config *networkingv1alpha2.GatewayConfiguration, err error) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, config, func() {
		config.Status.State = "Error"
		meta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: config.Generation,
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		config.Status.ObservedGeneration = config.Generation
	})
	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GatewayConfigurationReconciler) updateStatusPending(
	ctx context.Context,
	config *networkingv1alpha2.GatewayConfiguration,
	accountID string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, config, func() {
		if config.Status.AccountID == "" {
			config.Status.AccountID = accountID
		}
		config.Status.State = "Pending"
		meta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: config.Generation,
			Reason:             "Pending",
			Message:            "Gateway configuration registered, waiting for sync",
			LastTransitionTime: metav1.Now(),
		})
		config.Status.ObservedGeneration = config.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	// Requeue to check for sync completion
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *GatewayConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("gatewayconfiguration-controller")

	// Initialize GatewayConfigurationService
	r.gatewayService = gatewaysvc.NewGatewayConfigurationService(mgr.GetClient())

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.GatewayConfiguration{}).
		Complete(r)
}
