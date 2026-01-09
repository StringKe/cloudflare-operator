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

package gatewayconfiguration

import (
	"context"
	"fmt"
	"time"

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
	FinalizerName = "gatewayconfiguration.networking.cloudflare-operator.io/finalizer"
)

// GatewayConfigurationReconciler reconciles a GatewayConfiguration object
type GatewayConfigurationReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayconfigurations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewayconfigurations/finalizers,verbs=update

func (r *GatewayConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the GatewayConfiguration instance
	config := &networkingv1alpha2.GatewayConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Initialize API client
	apiClient, err := r.initAPIClient(ctx, config)
	if err != nil {
		logger.Error(err, "Failed to initialize Cloudflare API client")
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

	// Reconcile the gateway configuration
	return r.reconcileGatewayConfiguration(ctx, config, apiClient)
}

func (r *GatewayConfigurationReconciler) initAPIClient(ctx context.Context, config *networkingv1alpha2.GatewayConfiguration) (*cf.API, error) {
	// GatewayConfiguration is cluster-scoped, use operator namespace for legacy inline secrets
	return cf.NewAPIClientFromDetails(ctx, r.Client, controller.OperatorNamespace, config.Spec.Cloudflare)
}

func (r *GatewayConfigurationReconciler) handleDeletion(ctx context.Context, config *networkingv1alpha2.GatewayConfiguration) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(config, FinalizerName) {
		// Gateway configuration is account-level, we don't delete it
		// Just remove the finalizer

		// P0 FIX: Remove finalizer with retry logic to handle conflicts
		if err := controller.UpdateWithConflictRetry(ctx, r.Client, config, func() {
			controllerutil.RemoveFinalizer(config, FinalizerName)
		}); err != nil {
			logger.Error(err, "failed to remove finalizer")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(config, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")
	}

	return ctrl.Result{}, nil
}

func (r *GatewayConfigurationReconciler) reconcileGatewayConfiguration(ctx context.Context, config *networkingv1alpha2.GatewayConfiguration, apiClient *cf.API) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Build gateway configuration params
	params := r.buildConfigParams(config.Spec.Settings)

	// Update gateway configuration (always an update, not create)
	logger.Info("Updating Gateway Configuration")
	r.Recorder.Event(config, corev1.EventTypeNormal, "Updating", "Updating Gateway Configuration in Cloudflare")
	result, err := apiClient.UpdateGatewayConfiguration(params)
	if err != nil {
		r.Recorder.Event(config, corev1.EventTypeWarning, controller.EventReasonUpdateFailed,
			fmt.Sprintf("Failed to update Gateway Configuration: %s", cf.SanitizeErrorMessage(err)))
		return r.updateStatusError(ctx, config, err)
	}
	r.Recorder.Event(config, corev1.EventTypeNormal, controller.EventReasonUpdated, "Updated Gateway Configuration")

	// Update status
	return r.updateStatusSuccess(ctx, config, result)
}

func (r *GatewayConfigurationReconciler) buildConfigParams(settings networkingv1alpha2.GatewaySettings) cf.GatewayConfigurationParams {
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
			FooterText:      settings.BlockPage.FooterText,
			HeaderText:      settings.BlockPage.HeaderText,
			LogoPath:        settings.BlockPage.LogoPath,
			BackgroundColor: settings.BlockPage.BackgroundColor,
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

	return params
}

func (r *GatewayConfigurationReconciler) updateStatusError(ctx context.Context, config *networkingv1alpha2.GatewayConfiguration, err error) (ctrl.Result, error) {
	// P0 FIX: Use UpdateStatusWithConflictRetry for status updates
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

func (r *GatewayConfigurationReconciler) updateStatusSuccess(ctx context.Context, config *networkingv1alpha2.GatewayConfiguration, result *cf.GatewayConfigurationResult) (ctrl.Result, error) {
	// P0 FIX: Use UpdateStatusWithConflictRetry for status updates
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, config, func() {
		config.Status.AccountID = result.AccountID
		config.Status.State = "Ready"
		meta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: config.Generation,
			Reason:             "Reconciled",
			Message:            "Gateway Configuration successfully reconciled",
			LastTransitionTime: metav1.Now(),
		})
		config.Status.ObservedGeneration = config.Generation
	})
	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *GatewayConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("gatewayconfiguration-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.GatewayConfiguration{}).
		Complete(r)
}
