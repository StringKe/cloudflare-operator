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
	FinalizerName = "gatewayconfiguration.networking.cfargotunnel.com/finalizer"
)

// GatewayConfigurationReconciler reconciles a GatewayConfiguration object
type GatewayConfigurationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=networking.cfargotunnel.com,resources=gatewayconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cfargotunnel.com,resources=gatewayconfigurations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cfargotunnel.com,resources=gatewayconfigurations/finalizers,verbs=update

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
	return cf.NewAPIClientFromDetails(ctx, r.Client, "", config.Spec.Cloudflare)
}

func (r *GatewayConfigurationReconciler) handleDeletion(ctx context.Context, config *networkingv1alpha2.GatewayConfiguration) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(config, FinalizerName) {
		// Gateway configuration is account-level, we don't delete it
		// Just remove the finalizer

		controllerutil.RemoveFinalizer(config, FinalizerName)
		if err := r.Update(ctx, config); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *GatewayConfigurationReconciler) reconcileGatewayConfiguration(ctx context.Context, config *networkingv1alpha2.GatewayConfiguration, apiClient *cf.API) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Build gateway configuration params
	params := r.buildConfigParams(config.Spec.Settings)

	// Update gateway configuration (always an update, not create)
	logger.Info("Updating Gateway Configuration")
	result, err := apiClient.UpdateGatewayConfiguration(params)
	if err != nil {
		return r.updateStatusError(ctx, config, err)
	}

	// Update status
	return r.updateStatusSuccess(ctx, config, result)
}

func (r *GatewayConfigurationReconciler) buildConfigParams(settings networkingv1alpha2.GatewaySettings) cf.GatewayConfigurationParams {
	params := cf.GatewayConfigurationParams{
		Settings: make(map[string]interface{}),
	}

	if settings.TLSDecrypt != nil {
		params.Settings["tls_decrypt"] = map[string]interface{}{
			"enabled": settings.TLSDecrypt.Enabled,
		}
	}

	if settings.ActivityLog != nil {
		params.Settings["activity_log"] = map[string]interface{}{
			"enabled": settings.ActivityLog.Enabled,
		}
	}

	if settings.AntiVirus != nil {
		avMap := map[string]interface{}{
			"enabled": settings.AntiVirus.Enabled,
		}
		if settings.AntiVirus.EnabledDownloadPhase {
			avMap["enabled_download_phase"] = true
		}
		if settings.AntiVirus.EnabledUploadPhase {
			avMap["enabled_upload_phase"] = true
		}
		if settings.AntiVirus.FailClosed {
			avMap["fail_closed"] = true
		}
		if settings.AntiVirus.NotificationSettings != nil {
			avMap["notification_settings"] = map[string]interface{}{
				"enabled":     settings.AntiVirus.NotificationSettings.Enabled,
				"msg":         settings.AntiVirus.NotificationSettings.Message,
				"support_url": settings.AntiVirus.NotificationSettings.SupportURL,
			}
		}
		params.Settings["antivirus"] = avMap
	}

	if settings.BlockPage != nil {
		bpMap := map[string]interface{}{
			"enabled": settings.BlockPage.Enabled,
		}
		if settings.BlockPage.Name != "" {
			bpMap["name"] = settings.BlockPage.Name
		}
		if settings.BlockPage.FooterText != "" {
			bpMap["footer_text"] = settings.BlockPage.FooterText
		}
		if settings.BlockPage.HeaderText != "" {
			bpMap["header_text"] = settings.BlockPage.HeaderText
		}
		if settings.BlockPage.LogoPath != "" {
			bpMap["logo_path"] = settings.BlockPage.LogoPath
		}
		if settings.BlockPage.BackgroundColor != "" {
			bpMap["background_color"] = settings.BlockPage.BackgroundColor
		}
		if settings.BlockPage.MailtoAddress != "" {
			bpMap["mailto_address"] = settings.BlockPage.MailtoAddress
		}
		if settings.BlockPage.MailtoSubject != "" {
			bpMap["mailto_subject"] = settings.BlockPage.MailtoSubject
		}
		if settings.BlockPage.SuppressFooter {
			bpMap["suppress_footer"] = true
		}
		params.Settings["block_page"] = bpMap
	}

	if settings.BodyScanning != nil {
		params.Settings["body_scanning"] = map[string]interface{}{
			"inspection_mode": settings.BodyScanning.InspectionMode,
		}
	}

	if settings.BrowserIsolation != nil {
		biMap := make(map[string]interface{})
		if settings.BrowserIsolation.URLBrowserIsolationEnabled {
			biMap["url_browser_isolation_enabled"] = true
		}
		if settings.BrowserIsolation.NonIdentityEnabled {
			biMap["non_identity_enabled"] = true
		}
		params.Settings["browser_isolation"] = biMap
	}

	if settings.FIPS != nil {
		params.Settings["fips"] = map[string]interface{}{
			"tls": settings.FIPS.TLS,
		}
	}

	if settings.ProtocolDetection != nil {
		params.Settings["protocol_detection"] = map[string]interface{}{
			"enabled": settings.ProtocolDetection.Enabled,
		}
	}

	if settings.CustomCertificate != nil {
		ccMap := map[string]interface{}{
			"enabled": settings.CustomCertificate.Enabled,
		}
		if settings.CustomCertificate.ID != "" {
			ccMap["id"] = settings.CustomCertificate.ID
		}
		params.Settings["custom_certificate"] = ccMap
	}

	if settings.NonIdentityBrowserIsolation != nil {
		params.Settings["non_identity_browser_isolation"] = map[string]interface{}{
			"enabled": settings.NonIdentityBrowserIsolation.Enabled,
		}
	}

	return params
}

func (r *GatewayConfigurationReconciler) updateStatusError(ctx context.Context, config *networkingv1alpha2.GatewayConfiguration, err error) (ctrl.Result, error) {
	config.Status.State = "Error"
	meta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "ReconcileError",
		Message:            err.Error(),
		LastTransitionTime: metav1.Now(),
	})
	config.Status.ObservedGeneration = config.Generation

	if updateErr := r.Status().Update(ctx, config); updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GatewayConfigurationReconciler) updateStatusSuccess(ctx context.Context, config *networkingv1alpha2.GatewayConfiguration, result *cf.GatewayConfigurationResult) (ctrl.Result, error) {
	config.Status.AccountID = result.AccountID
	config.Status.State = "Ready"
	meta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "Gateway Configuration successfully reconciled",
		LastTransitionTime: metav1.Now(),
	})
	config.Status.ObservedGeneration = config.Generation

	if err := r.Status().Update(ctx, config); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *GatewayConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.GatewayConfiguration{}).
		Complete(r)
}
