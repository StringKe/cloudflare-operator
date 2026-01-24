// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package tunnelconfig

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller/common"
)

// Reconciler reconciles tunnel configuration stored in ConfigMaps.
// It watches ConfigMaps with the tunnel-config label and syncs
// aggregated configuration to Cloudflare.
type Reconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	Recorder          record.EventRecorder
	APIFactory        *common.APIClientFactory
	OperatorNamespace string
}

// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile handles ConfigMap reconciliation for tunnel configuration.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Only process ConfigMaps in the operator namespace
	if req.Namespace != r.OperatorNamespace {
		return ctrl.Result{}, nil
	}

	// Get the ConfigMap
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, req.NamespacedName, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Unable to fetch ConfigMap")
		return ctrl.Result{}, err
	}

	// Check if this is a tunnel config ConfigMap
	if cm.Labels[ConfigMapLabelType] != ConfigMapTypeValue {
		return ctrl.Result{}, nil
	}

	tunnelID := cm.Labels[ConfigMapLabelTunnelID]
	if tunnelID == "" {
		logger.V(1).Info("ConfigMap missing tunnel ID label, skipping")
		return ctrl.Result{}, nil
	}

	// Parse the configuration
	config, err := ParseConfig(cm)
	if err != nil {
		logger.Error(err, "Failed to parse tunnel config")
		r.Recorder.Event(cm, corev1.EventTypeWarning, "ParseError",
			fmt.Sprintf("Failed to parse config: %v", err))
		return ctrl.Result{}, nil // Don't retry parse errors
	}

	// Check if configuration has changed
	newHash := config.ComputeHash()
	if config.LastHash == newHash && config.SyncStatus == "Synced" {
		logger.V(1).Info("Configuration unchanged, skipping sync",
			"tunnelId", tunnelID, "hash", newHash)
		return ctrl.Result{}, nil
	}

	logger.Info("Syncing tunnel configuration",
		"tunnelId", tunnelID,
		"sources", len(config.Sources),
		"oldHash", config.LastHash,
		"newHash", newHash)

	// Get API client
	apiResult, err := r.getAPIClient(ctx, config)
	if err != nil {
		logger.Error(err, "Failed to get API client")
		r.updateConfigStatus(ctx, cm, config, "Error", fmt.Sprintf("API client error: %v", err))
		return common.RequeueShort(), nil
	}

	// Sync to Cloudflare
	if err := r.syncToCloudflare(ctx, apiResult.API, config); err != nil {
		logger.Error(err, "Failed to sync tunnel configuration")
		r.Recorder.Event(cm, corev1.EventTypeWarning, "SyncFailed",
			fmt.Sprintf("Failed to sync: %s", cf.SanitizeErrorMessage(err)))
		r.updateConfigStatus(ctx, cm, config, "Error", cf.SanitizeErrorMessage(err))
		return common.RequeueShort(), nil
	}

	// Update status
	config.LastHash = newHash
	config.SyncStatus = "Synced"
	now := metav1.Now()
	config.LastSyncTime = &now

	if err := r.updateConfigMap(ctx, cm, config); err != nil {
		logger.Error(err, "Failed to update ConfigMap status")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(cm, corev1.EventTypeNormal, "Synced",
		fmt.Sprintf("Tunnel configuration synced (rules=%d, warp=%v)",
			len(config.AggregateRules()), config.IsWARPRoutingEnabled()))

	logger.Info("Tunnel configuration synced successfully",
		"tunnelId", tunnelID,
		"rules", len(config.AggregateRules()),
		"warpRouting", config.IsWARPRoutingEnabled())

	return ctrl.Result{}, nil
}

// getAPIClient gets the Cloudflare API client for the tunnel configuration.
func (r *Reconciler) getAPIClient(ctx context.Context, config *TunnelConfig) (*common.APIClientResult, error) {
	opts := common.APIClientOptions{
		Namespace:       r.OperatorNamespace,
		StatusAccountID: config.AccountID,
	}

	if config.CredentialsRef != nil && config.CredentialsRef.Name != "" {
		opts.CredentialsRef = &networkingv1alpha2.CredentialsReference{
			Name: config.CredentialsRef.Name,
		}
	}

	return r.APIFactory.GetClient(ctx, opts)
}

// syncToCloudflare syncs the tunnel configuration to Cloudflare.
func (r *Reconciler) syncToCloudflare(ctx context.Context, api *cf.API, config *TunnelConfig) error {
	// Build the tunnel configuration
	tunnelConfig := r.buildCloudflareConfig(config)

	// Update tunnel configuration
	_, err := api.UpdateTunnelConfiguration(ctx, config.TunnelID, tunnelConfig)
	return err
}

// buildCloudflareConfig builds the Cloudflare tunnel configuration from our config.
func (*Reconciler) buildCloudflareConfig(config *TunnelConfig) cloudflare.TunnelConfiguration {
	cfConfig := cloudflare.TunnelConfiguration{
		Ingress: []cloudflare.UnvalidatedIngressRule{},
	}

	// Set WARP routing
	if config.IsWARPRoutingEnabled() {
		cfConfig.WarpRouting = &cloudflare.WarpRoutingConfig{
			Enabled: true,
		}
	}

	// Set origin request defaults
	if defaults := config.GetOriginRequestDefaults(); defaults != nil {
		if converted := convertOriginRequest(defaults); converted != nil {
			cfConfig.OriginRequest = *converted
		}
	}

	// Add all ingress rules
	rules := config.AggregateRules()
	for _, rule := range rules {
		cfRule := cloudflare.UnvalidatedIngressRule{
			Hostname: rule.Hostname,
			Path:     rule.Path,
			Service:  rule.Service,
		}

		if rule.OriginRequest != nil {
			cfRule.OriginRequest = convertOriginRequest(rule.OriginRequest)
		}

		cfConfig.Ingress = append(cfConfig.Ingress, cfRule)
	}

	// Add catch-all rule if we have any rules
	if len(cfConfig.Ingress) > 0 {
		cfConfig.Ingress = append(cfConfig.Ingress, cloudflare.UnvalidatedIngressRule{
			Service: "http_status:404",
		})
	}

	return cfConfig
}

// convertOriginRequest converts our OriginRequestConfig to Cloudflare's format.
//
//nolint:revive // cognitive complexity is acceptable for field mapping
func convertOriginRequest(req *OriginRequestConfig) *cloudflare.OriginRequestConfig {
	if req == nil {
		return nil
	}

	cfReq := &cloudflare.OriginRequestConfig{}

	// Set string pointer fields only if non-empty
	if req.HTTPHostHeader != "" {
		cfReq.HTTPHostHeader = &req.HTTPHostHeader
	}
	if req.OriginServerName != "" {
		cfReq.OriginServerName = &req.OriginServerName
	}
	if req.ProxyAddress != "" {
		cfReq.ProxyAddress = &req.ProxyAddress
	}
	if req.ProxyType != "" {
		cfReq.ProxyType = &req.ProxyType
	}

	// Set bool pointer fields
	cfReq.NoTLSVerify = &req.NoTLSVerify
	cfReq.DisableChunkedEncoding = &req.DisableChunkedEncoding
	cfReq.BastionMode = &req.BastionMode
	cfReq.Http2Origin = &req.HTTP2Origin
	cfReq.NoHappyEyeballs = &req.NoHappyEyeballs

	// Convert duration fields (string -> TunnelDuration pointer)
	if req.ConnectTimeout != "" {
		if d, err := time.ParseDuration(req.ConnectTimeout); err == nil {
			cfReq.ConnectTimeout = &cloudflare.TunnelDuration{Duration: d}
		}
	}
	if req.TLSTimeout != "" {
		if d, err := time.ParseDuration(req.TLSTimeout); err == nil {
			cfReq.TLSTimeout = &cloudflare.TunnelDuration{Duration: d}
		}
	}
	if req.TCPKeepAlive != "" {
		if d, err := time.ParseDuration(req.TCPKeepAlive); err == nil {
			cfReq.TCPKeepAlive = &cloudflare.TunnelDuration{Duration: d}
		}
	}
	if req.KeepAliveTimeout != "" {
		if d, err := time.ParseDuration(req.KeepAliveTimeout); err == nil {
			cfReq.KeepAliveTimeout = &cloudflare.TunnelDuration{Duration: d}
		}
	}

	if req.KeepAliveConnections != 0 {
		cfReq.KeepAliveConnections = &req.KeepAliveConnections
	}
	if req.ProxyPort != 0 {
		port := uint(req.ProxyPort)
		cfReq.ProxyPort = &port
	}

	// Convert IP rules
	if len(req.IPRules) > 0 {
		cfReq.IPRules = make([]cloudflare.IngressIPRule, len(req.IPRules))
		for i, rule := range req.IPRules {
			cfReq.IPRules[i] = cloudflare.IngressIPRule{
				Prefix: &rule.Prefix,
				Allow:  rule.Allow,
				Ports:  rule.Ports,
			}
		}
	}

	// Convert Access config
	if req.Access != nil {
		cfReq.Access = &cloudflare.AccessConfig{
			Required: req.Access.Required,
			TeamName: req.Access.TeamName,
			AudTag:   req.Access.AudTag,
		}
	}

	return cfReq
}

// updateConfigStatus updates the ConfigMap with status information.
func (r *Reconciler) updateConfigStatus(ctx context.Context, cm *corev1.ConfigMap, config *TunnelConfig, status, message string) {
	config.SyncStatus = status
	now := metav1.Now()
	config.LastSyncTime = &now

	if err := r.updateConfigMap(ctx, cm, config); err != nil {
		log.FromContext(ctx).Error(err, "Failed to update config status")
	}
}

// updateConfigMap updates the ConfigMap with new configuration.
func (r *Reconciler) updateConfigMap(ctx context.Context, cm *corev1.ConfigMap, config *TunnelConfig) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Get fresh copy
		fresh := &corev1.ConfigMap{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(cm), fresh); err != nil {
			return err
		}

		// Update data
		data, err := config.ToConfigMapData()
		if err != nil {
			return err
		}
		fresh.Data = data

		return r.Update(ctx, fresh)
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("tunnelconfig-controller")
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("tunnelconfig"))

	// Only watch ConfigMaps with our label
	labelPredicate, err := predicate.LabelSelectorPredicate(metav1.LabelSelector{
		MatchLabels: map[string]string{
			ConfigMapLabelType: ConfigMapTypeValue,
		},
	})
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}, builder.WithPredicates(labelPredicate)).
		Named("tunnelconfig").
		Complete(r)
}
