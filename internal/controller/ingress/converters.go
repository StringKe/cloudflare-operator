// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package ingress

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	networkingv1alpha1 "github.com/StringKe/cloudflare-operator/api/v1alpha1"
	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	tunnelpkg "github.com/StringKe/cloudflare-operator/internal/controller/tunnel"
)

// TunnelInterface is an alias to the shared tunnel.Interface for backward compatibility
type TunnelInterface = tunnelpkg.Interface

// TunnelWrapper is an alias to the shared tunnel.TunnelWrapper for backward compatibility
type TunnelWrapper = tunnelpkg.TunnelWrapper

// ClusterTunnelWrapper is an alias to the shared tunnel.ClusterTunnelWrapper for backward compatibility
type ClusterTunnelWrapper = tunnelpkg.ClusterTunnelWrapper

// buildIngressRules converts Ingresses and TunnelBindings to cloudflared ingress rules
// nolint:staticcheck // TunnelBinding is deprecated but we still support it for backward compatibility
func (r *Reconciler) buildIngressRules(
	ctx context.Context,
	ingresses []*networkingv1.Ingress,
	bindings []networkingv1alpha1.TunnelBinding, //nolint:staticcheck
	config *networkingv1alpha2.TunnelIngressClassConfig,
) []cf.UnvalidatedIngressRule {
	var rules []cf.UnvalidatedIngressRule

	// 1. Process Ingresses (higher priority)
	for _, ing := range ingresses {
		ingressRules := r.convertIngressToRules(ctx, ing, config)
		rules = append(rules, ingressRules...)
	}

	// 2. Process TunnelBindings (backward compatibility)
	for _, binding := range bindings {
		bindingRules := r.convertTunnelBindingToRules(binding)
		rules = append(rules, bindingRules...)
	}

	// 3. Sort rules for deterministic config (by hostname, then path)
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Hostname != rules[j].Hostname {
			return rules[i].Hostname < rules[j].Hostname
		}
		return rules[i].Path < rules[j].Path
	})

	// 4. Add fallback rule
	tunnel, err := r.getTunnel(ctx, config)
	fallbackTarget := "http_status:404"
	if err == nil {
		fallbackTarget = tunnel.GetSpec().FallbackTarget
		if fallbackTarget == "" {
			fallbackTarget = "http_status:404"
		}
	}

	rules = append(rules, cf.UnvalidatedIngressRule{
		Service: fallbackTarget,
	})

	return rules
}

// convertIngressToRules converts a single Ingress to cloudflared rules
// nolint:revive // Cognitive complexity for Ingress to rules conversion
func (r *Reconciler) convertIngressToRules(
	ctx context.Context,
	ing *networkingv1.Ingress,
	config *networkingv1alpha2.TunnelIngressClassConfig,
) []cf.UnvalidatedIngressRule {
	var rules []cf.UnvalidatedIngressRule
	parser := NewAnnotationParser(ing.Annotations)

	// Build a set of TLS hosts for automatic HTTPS detection
	tlsHosts := make(map[string]string) // hostname -> secretName
	for _, tls := range ing.Spec.TLS {
		for _, host := range tls.Hosts {
			tlsHosts[host] = tls.SecretName
		}
	}

	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}

		for _, path := range rule.HTTP.Paths {
			ingressRule := r.buildRuleFromIngressPath(ctx, ing, rule.Host, path, config, parser, tlsHosts)
			rules = append(rules, ingressRule)
		}
	}

	return rules
}

// buildRuleFromIngressPath creates a cloudflared ingress rule from Kubernetes Ingress path
func (r *Reconciler) buildRuleFromIngressPath(
	ctx context.Context,
	ing *networkingv1.Ingress,
	host string,
	path networkingv1.HTTPIngressPath,
	config *networkingv1alpha2.TunnelIngressClassConfig,
	parser *AnnotationParser,
	tlsHosts map[string]string,
) cf.UnvalidatedIngressRule {
	// Determine service target
	target := r.resolveIngressBackend(ctx, ing.Namespace, path.Backend, parser, tlsHosts, host)

	// Build origin request from annotations + defaults
	originRequest := r.buildOriginRequest(parser, config.Spec.DefaultOriginRequest, tlsHosts, host)

	// Convert path
	pathStr := convertPathType(path.Path, path.PathType)

	return cf.UnvalidatedIngressRule{
		Hostname:      host,
		Path:          pathStr,
		Service:       target,
		OriginRequest: originRequest,
	}
}

// convertPathType converts Kubernetes PathType to cloudflared path regex
// nolint:revive // Cognitive complexity for path conversion logic
func convertPathType(path string, pathType *networkingv1.PathType) string {
	if path == "" || path == "/" {
		return ""
	}

	pt := networkingv1.PathTypePrefix
	if pathType != nil {
		pt = *pathType
	}

	switch pt {
	case networkingv1.PathTypeExact:
		// Exact match
		return "^" + path + "$"
	case networkingv1.PathTypePrefix:
		// Prefix match - cloudflared uses regex
		// /foo should match /foo, /foo/, /foo/bar
		if path[len(path)-1] == '/' {
			return path + ".*"
		}
		return path + "(/.*)?$"
	case networkingv1.PathTypeImplementationSpecific:
		// Treat as prefix
		if path[len(path)-1] == '/' {
			return path + ".*"
		}
		return path + "(/.*)?$"
	default:
		return path
	}
}

// resolveIngressBackend resolves Ingress backend to service URL
func (r *Reconciler) resolveIngressBackend(
	ctx context.Context,
	namespace string,
	backend networkingv1.IngressBackend,
	parser *AnnotationParser,
	tlsHosts map[string]string,
	host string,
) string {
	if backend.Service == nil {
		return "http_status:503"
	}

	// Get port
	port := r.resolveServicePort(ctx, namespace, backend.Service)

	// Determine protocol
	protocol := r.determineProtocol(parser, port, tlsHosts, host)

	return fmt.Sprintf("%s://%s.%s.svc:%s", protocol, backend.Service.Name, namespace, port)
}

// resolveServicePort resolves the port from Ingress backend
// nolint:revive // Cognitive complexity for port resolution logic
func (r *Reconciler) resolveServicePort(ctx context.Context, namespace string, svcBackend *networkingv1.IngressServiceBackend) string {
	logger := log.FromContext(ctx)

	if svcBackend.Port.Number != 0 {
		return fmt.Sprintf("%d", svcBackend.Port.Number)
	}

	if svcBackend.Port.Name != "" {
		// Resolve named port from Service
		svc := &corev1.Service{}
		if err := r.Get(ctx, apitypes.NamespacedName{
			Name:      svcBackend.Name,
			Namespace: namespace,
		}, svc); err != nil {
			logger.Error(err, "Failed to get Service for named port resolution, using default port 80",
				"service", fmt.Sprintf("%s/%s", namespace, svcBackend.Name),
				"portName", svcBackend.Port.Name)
		} else {
			for _, p := range svc.Spec.Ports {
				if p.Name == svcBackend.Port.Name {
					return fmt.Sprintf("%d", p.Port)
				}
			}
			logger.Info("Named port not found in Service, using default port 80",
				"service", fmt.Sprintf("%s/%s", namespace, svcBackend.Name),
				"portName", svcBackend.Port.Name)
		}
	}

	return "80"
}

// determineProtocol determines the protocol based on annotations, port, and TLS
func (*Reconciler) determineProtocol(parser *AnnotationParser, port string, tlsHosts map[string]string, host string) string {
	// 1. Check annotation override
	if protocol, ok := parser.GetString(AnnotationProtocol); ok {
		return protocol
	}

	// 2. Check if host is in TLS hosts
	if _, isTLS := tlsHosts[host]; isTLS {
		return "https"
	}

	// 3. Infer from port
	return inferProtocolFromPort(port)
}

// inferProtocolFromPort determines protocol based on port number
func inferProtocolFromPort(port string) string {
	switch port {
	case "443":
		return "https"
	case "22":
		return "ssh"
	case "3389":
		return "rdp"
	case "139", "445":
		return "smb"
	default:
		return "http"
	}
}

// buildOriginRequest creates OriginRequestConfig from annotations and defaults
// nolint:gocyclo,revive // This function has many conditional branches for each annotation, which is expected
func (*Reconciler) buildOriginRequest(
	parser *AnnotationParser,
	defaults *networkingv1alpha2.OriginRequestSpec,
	_ map[string]string,
	_ string,
) cf.OriginRequestConfig {
	config := cf.OriginRequestConfig{}

	// Apply defaults first
	if defaults != nil {
		config.NoTLSVerify = &defaults.NoTLSVerify
		config.Http2Origin = &defaults.HTTP2Origin

		if defaults.CAPool != "" {
			caPath := fmt.Sprintf("/etc/cloudflared/certs/%s", defaults.CAPool)
			config.CAPool = &caPath
		}

		if defaults.ConnectTimeout != "" {
			if d, err := time.ParseDuration(defaults.ConnectTimeout); err == nil {
				config.ConnectTimeout = &d
			}
		}

		if defaults.TLSTimeout != "" {
			if d, err := time.ParseDuration(defaults.TLSTimeout); err == nil {
				config.TLSTimeout = &d
			}
		}

		if defaults.KeepAliveTimeout != "" {
			if d, err := time.ParseDuration(defaults.KeepAliveTimeout); err == nil {
				config.KeepAliveTimeout = &d
			}
		}

		if defaults.KeepAliveConnections != nil {
			config.KeepAliveConnections = defaults.KeepAliveConnections
		}

		if defaults.OriginServerName != "" {
			config.OriginServerName = &defaults.OriginServerName
		}

		if defaults.HTTPHostHeader != "" {
			config.HTTPHostHeader = &defaults.HTTPHostHeader
		}

		if defaults.ProxyAddress != "" {
			config.ProxyAddress = &defaults.ProxyAddress
		}

		if defaults.ProxyPort != nil {
			port := uint(*defaults.ProxyPort)
			config.ProxyPort = &port
		}

		if defaults.ProxyType != "" {
			config.ProxyType = &defaults.ProxyType
		}

		if defaults.DisableChunkedEncoding != nil {
			config.DisableChunkedEncoding = defaults.DisableChunkedEncoding
		}

		if defaults.BastionMode != nil {
			config.BastionMode = defaults.BastionMode
		}
	}

	// Override with annotations
	config.NoTLSVerify = parser.GetBoolPtr(AnnotationNoTLSVerify)
	if config.NoTLSVerify == nil && defaults != nil {
		config.NoTLSVerify = &defaults.NoTLSVerify
	}

	config.Http2Origin = parser.GetBoolPtr(AnnotationHTTP2Origin)
	if config.Http2Origin == nil && defaults != nil {
		config.Http2Origin = &defaults.HTTP2Origin
	}

	if caPool, ok := parser.GetString(AnnotationCAPool); ok {
		caPath := fmt.Sprintf("/etc/cloudflared/certs/%s", caPool)
		config.CAPool = &caPath
	}

	if d, ok := parser.GetDuration(AnnotationConnectTimeout); ok {
		config.ConnectTimeout = &d
	}

	if d, ok := parser.GetDuration(AnnotationTLSTimeout); ok {
		config.TLSTimeout = &d
	}

	if d, ok := parser.GetDuration(AnnotationKeepAliveTimeout); ok {
		config.KeepAliveTimeout = &d
	}

	if n, ok := parser.GetInt(AnnotationKeepAliveConnections); ok {
		config.KeepAliveConnections = &n
	}

	if v, ok := parser.GetString(AnnotationOriginServerName); ok {
		config.OriginServerName = &v
	}

	if v, ok := parser.GetString(AnnotationHTTPHostHeader); ok {
		config.HTTPHostHeader = &v
	}

	if v, ok := parser.GetString(AnnotationProxyAddress); ok {
		config.ProxyAddress = &v
	}

	if port, ok := parser.GetUint16(AnnotationProxyPort); ok {
		p := uint(port)
		config.ProxyPort = &p
	}

	if v, ok := parser.GetString(AnnotationProxyType); ok {
		config.ProxyType = &v
	}

	config.DisableChunkedEncoding = parser.GetBoolPtr(AnnotationDisableChunkedEncoding)
	config.BastionMode = parser.GetBoolPtr(AnnotationBastionMode)

	return config
}

// convertTunnelBindingToRules converts a TunnelBinding to cloudflared rules (backward compatibility)
// nolint:revive,staticcheck // Cognitive complexity for TunnelBinding conversion; TunnelBinding is deprecated but still supported
func (*Reconciler) convertTunnelBindingToRules(binding networkingv1alpha1.TunnelBinding) []cf.UnvalidatedIngressRule { //nolint:staticcheck
	rules := make([]cf.UnvalidatedIngressRule, 0, len(binding.Subjects))

	for i, subject := range binding.Subjects {
		if i >= len(binding.Status.Services) {
			continue
		}

		svcStatus := binding.Status.Services[i]

		// Build target
		target := svcStatus.Target
		if subject.Spec.Target != "" {
			target = subject.Spec.Target
		}

		// Build origin request
		originRequest := cf.OriginRequestConfig{}
		originRequest.NoTLSVerify = &subject.Spec.NoTlsVerify
		originRequest.Http2Origin = &subject.Spec.Http2Origin

		if subject.Spec.ProxyAddress != "" {
			originRequest.ProxyAddress = &subject.Spec.ProxyAddress
		}
		if subject.Spec.ProxyPort != 0 {
			port := subject.Spec.ProxyPort
			originRequest.ProxyPort = &port
		}
		if subject.Spec.ProxyType != "" {
			originRequest.ProxyType = &subject.Spec.ProxyType
		}
		if subject.Spec.CaPool != "" {
			caPath := fmt.Sprintf("/etc/cloudflared/certs/%s", subject.Spec.CaPool)
			originRequest.CAPool = &caPath
		}

		rules = append(rules, cf.UnvalidatedIngressRule{
			Hostname:      svcStatus.Hostname,
			Path:          subject.Spec.Path,
			Service:       target,
			OriginRequest: originRequest,
		})
	}

	return rules
}

// updateTunnelConfigMap updates the tunnel's ConfigMap with new ingress rules
// nolint:revive // Cognitive complexity for ConfigMap update logic
func (r *Reconciler) updateTunnelConfigMap(
	ctx context.Context,
	tunnel TunnelInterface,
	rules []cf.UnvalidatedIngressRule,
	_ *networkingv1alpha2.TunnelIngressClassConfig,
) error {
	logger := log.FromContext(ctx)

	// Get ConfigMap
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, apitypes.NamespacedName{
		Name:      tunnel.GetName(),
		Namespace: tunnel.GetNamespace(),
	}, cm); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("ConfigMap not found, tunnel may not be ready yet", "tunnel", tunnel.GetName())
			return fmt.Errorf("ConfigMap %s/%s not found, tunnel may not be ready", tunnel.GetNamespace(), tunnel.GetName())
		}
		return err
	}

	// Parse existing config
	existingConfig := &cf.Configuration{}
	if configStr, ok := cm.Data["config.yaml"]; ok {
		if err := yaml.Unmarshal([]byte(configStr), existingConfig); err != nil {
			return fmt.Errorf("failed to parse existing config: %w", err)
		}
	}

	// Update ingress rules
	existingConfig.Ingress = rules

	// Marshal back to YAML
	configBytes, err := yaml.Marshal(existingConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configStr := string(configBytes)

	// Check if config actually changed
	if cm.Data["config.yaml"] == configStr {
		logger.Info("ConfigMap unchanged, skipping update")
		return nil
	}

	// Update ConfigMap with retry
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, cm, func() {
		cm.Data["config.yaml"] = configStr
	}); err != nil {
		return fmt.Errorf("failed to update ConfigMap: %w", err)
	}

	logger.Info("ConfigMap updated", "tunnel", tunnel.GetName())

	// Update Deployment annotation to trigger pod restart
	if err := r.triggerDeploymentRestart(ctx, tunnel, configStr); err != nil {
		logger.Error(err, "Failed to trigger deployment restart")
		// Don't fail - ConfigMap is updated, pod will eventually pick up changes
	}

	return nil
}

// triggerDeploymentRestart updates the Deployment annotation to trigger a rolling restart
func (r *Reconciler) triggerDeploymentRestart(ctx context.Context, tunnel TunnelInterface, configStr string) error {
	logger := log.FromContext(ctx)

	// Calculate config checksum
	hash := md5.Sum([]byte(configStr))
	checksum := hex.EncodeToString(hash[:])

	// Get Deployment
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, apitypes.NamespacedName{
		Name:      tunnel.GetName(),
		Namespace: tunnel.GetNamespace(),
	}, deployment); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Deployment not found, skipping restart trigger")
			return nil
		}
		return err
	}

	// Check if checksum is already set
	const checksumAnnotation = "cloudflare-operator.io/checksum"
	if deployment.Spec.Template.Annotations != nil {
		if deployment.Spec.Template.Annotations[checksumAnnotation] == checksum {
			logger.Info("Deployment checksum unchanged, skipping restart")
			return nil
		}
	}

	// Update Deployment annotation
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, deployment, func() {
		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = make(map[string]string)
		}
		deployment.Spec.Template.Annotations[checksumAnnotation] = checksum
	}); err != nil {
		return err
	}

	logger.Info("Deployment restart triggered", "tunnel", tunnel.GetName(), "checksum", checksum)
	return nil
}
