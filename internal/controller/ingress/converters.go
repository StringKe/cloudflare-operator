// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package ingress

import (
	"context"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha1 "github.com/StringKe/cloudflare-operator/api/v1alpha1"
	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
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
	// Determine service target with multi-level protocol detection
	target := r.resolveIngressBackend(ctx, ing.Namespace, path.Backend, parser, config)

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

// ServiceInfo contains Service information for protocol detection
type ServiceInfo struct {
	Name        string
	Namespace   string
	Port        string
	Annotations map[string]string
	AppProtocol *string
	PortName    string
}

// resolveIngressBackend resolves Ingress backend to service URL
func (r *Reconciler) resolveIngressBackend(
	ctx context.Context,
	namespace string,
	backend networkingv1.IngressBackend,
	parser *AnnotationParser,
	config *networkingv1alpha2.TunnelIngressClassConfig,
) string {
	if backend.Service == nil {
		return "http_status:503"
	}

	// Get Service info including port, annotations, appProtocol
	svcInfo := r.getServiceInfo(ctx, namespace, backend.Service)

	// Determine protocol using multi-level detection
	protocol := r.determineProtocol(parser, svcInfo, config)

	return fmt.Sprintf("%s://%s.%s.svc:%s", protocol, backend.Service.Name, namespace, svcInfo.Port)
}

// getServiceInfo retrieves Service information for protocol detection
// nolint:revive // cognitive complexity is acceptable for port resolution logic
func (r *Reconciler) getServiceInfo(ctx context.Context, namespace string, svcBackend *networkingv1.IngressServiceBackend) ServiceInfo {
	logger := log.FromContext(ctx)

	info := ServiceInfo{
		Name:      svcBackend.Name,
		Namespace: namespace,
		Port:      "80",
	}

	// Try to get Service for additional info
	svc := &corev1.Service{}
	if err := r.Get(ctx, apitypes.NamespacedName{
		Name:      svcBackend.Name,
		Namespace: namespace,
	}, svc); err != nil {
		logger.V(1).Info("Failed to get Service, using defaults",
			"service", fmt.Sprintf("%s/%s", namespace, svcBackend.Name),
			"error", err.Error())
		// Still try to resolve port from backend spec
		if svcBackend.Port.Number != 0 {
			info.Port = fmt.Sprintf("%d", svcBackend.Port.Number)
		}
		return info
	}

	// Get Service annotations
	info.Annotations = svc.Annotations

	// Resolve port and get port info
	if svcBackend.Port.Number != 0 {
		info.Port = fmt.Sprintf("%d", svcBackend.Port.Number)
		// Find matching port for appProtocol and name
		for _, p := range svc.Spec.Ports {
			if p.Port == svcBackend.Port.Number {
				info.AppProtocol = p.AppProtocol
				info.PortName = p.Name
				break
			}
		}
	} else if svcBackend.Port.Name != "" {
		// Resolve named port
		for _, p := range svc.Spec.Ports {
			if p.Name == svcBackend.Port.Name {
				info.Port = fmt.Sprintf("%d", p.Port)
				info.AppProtocol = p.AppProtocol
				info.PortName = p.Name
				break
			}
		}
	} else if len(svc.Spec.Ports) > 0 {
		// Use first port if no port specified
		info.Port = fmt.Sprintf("%d", svc.Spec.Ports[0].Port)
		info.AppProtocol = svc.Spec.Ports[0].AppProtocol
		info.PortName = svc.Spec.Ports[0].Name
	}

	return info
}

// determineProtocol determines the protocol using multi-level detection.
// Priority (highest to lowest):
// 1. Ingress annotation: cloudflare.com/protocol
// 2. Ingress annotation: cloudflare.com/protocol-{port} (port-specific)
// 3. Service annotation: cloudflare.com/protocol
// 4. Service port appProtocol field (Kubernetes native)
// 5. Service port name (http, https, grpc, h2c, etc.)
// 6. TunnelIngressClassConfig defaultProtocol
// 7. Port number inference (443→https, 22→ssh, others→http)
//
// nolint:revive // cyclomatic complexity is acceptable for multi-level detection
func (*Reconciler) determineProtocol(
	ingressParser *AnnotationParser,
	svcInfo ServiceInfo,
	config *networkingv1alpha2.TunnelIngressClassConfig,
) string {
	// 1. Check Ingress annotation: cloudflare.com/protocol
	if protocol, ok := ingressParser.GetString(AnnotationProtocol); ok && protocol != "" {
		return protocol
	}

	// 2. Check Ingress annotation: cloudflare.com/protocol-{port}
	portSpecificKey := AnnotationProtocolPrefix + svcInfo.Port
	if protocol, ok := ingressParser.GetString(portSpecificKey); ok && protocol != "" {
		return protocol
	}

	// 3. Check Service annotation: cloudflare.com/protocol
	if svcInfo.Annotations != nil {
		if protocol, ok := svcInfo.Annotations[AnnotationProtocol]; ok && protocol != "" {
			return protocol
		}
	}

	// 4. Check Service port appProtocol field (Kubernetes native)
	if svcInfo.AppProtocol != nil && *svcInfo.AppProtocol != "" {
		return inferProtocolFromAppProtocol(*svcInfo.AppProtocol)
	}

	// 5. Check Service port name
	if svcInfo.PortName != "" {
		if protocol := inferProtocolFromPortName(svcInfo.PortName); protocol != "" {
			return protocol
		}
	}

	// 6. Check TunnelIngressClassConfig defaultProtocol
	if config != nil && config.Spec.DefaultProtocol != "" {
		return string(config.Spec.DefaultProtocol)
	}

	// 7. Fall back to port number inference
	return inferProtocolFromPort(svcInfo.Port)
}

// inferProtocolFromAppProtocol converts Kubernetes appProtocol to tunnel protocol
// nolint:goconst // protocol strings in switch cases don't need to be constants
func inferProtocolFromAppProtocol(appProtocol string) string {
	// Handle kubernetes.io/h2c and similar prefixes
	switch appProtocol {
	case "kubernetes.io/h2c", "h2c":
		return "h2mux" // HTTP/2 cleartext
	case "kubernetes.io/ws", "ws":
		return "ws"
	case "kubernetes.io/wss", "wss":
		return "wss"
	case "http", "HTTP":
		return "http"
	case "https", "HTTPS":
		return "https"
	case "tcp", "TCP":
		return "tcp"
	case "udp", "UDP":
		return "udp"
	case "grpc", "GRPC":
		return "http" // gRPC over HTTP/2
	default:
		// If it looks like a protocol, use it directly
		return appProtocol
	}
}

// inferProtocolFromPortName determines protocol based on port name
// nolint:goconst,revive // protocol strings in switch cases don't need to be constants; cyclomatic complexity is acceptable
func inferProtocolFromPortName(portName string) string {
	switch portName {
	case "http", "http-web", "http-api", "web":
		return "http"
	case "https", "https-web", "https-api", "secure":
		return "https"
	case "grpc", "grpc-web":
		return "http" // gRPC uses HTTP/2
	case "h2c", "http2":
		return "h2mux"
	case "ws", "websocket":
		return "ws"
	case "wss", "websocket-secure":
		return "wss"
	case "ssh":
		return "ssh"
	case "rdp":
		return "rdp"
	case "smb":
		return "smb"
	case "tcp":
		return "tcp"
	case "udp":
		return "udp"
	default:
		return "" // Not recognized, continue to next level
	}
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
		config.HTTP2Origin = &defaults.HTTP2Origin

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

	config.HTTP2Origin = parser.GetBoolPtr(AnnotationHTTP2Origin)
	if config.HTTP2Origin == nil && defaults != nil {
		config.HTTP2Origin = &defaults.HTTP2Origin
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
		originRequest.HTTP2Origin = &subject.Spec.HTTP2Origin

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
