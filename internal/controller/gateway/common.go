// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package gateway implements Kubernetes Gateway API controllers for cloudflared tunnels.
// This includes controllers for GatewayClass, Gateway, HTTPRoute, TCPRoute, and UDPRoute.
package gateway

import (
	"context"
	"fmt"

	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

const (
	// ControllerName is the name used to identify this controller in GatewayClass
	ControllerName = "cloudflare-operator.io/gateway-controller"

	// FinalizerName is the finalizer used by Gateway API controllers
	FinalizerName = "gateway.cloudflare-operator.io/finalizer"

	// ParametersGroup is the API group for TunnelGatewayClassConfig
	ParametersGroup = "networking.cloudflare-operator.io"

	// ParametersKind is the kind for TunnelGatewayClassConfig
	ParametersKind = "TunnelGatewayClassConfig"

	// KindService is the Kubernetes Service kind
	KindService = "Service"

	// KindGateway is the Gateway API Gateway kind
	KindGateway = "Gateway"

	// Annotations used by Gateway API resources
	AnnotationPrefix = "cloudflare-operator.io/"

	// AnnotationDisableDNS disables DNS management for a specific route
	AnnotationDisableDNS = AnnotationPrefix + "disable-dns"

	// AnnotationDNSProxied overrides the proxied setting for DNS records
	AnnotationDNSProxied = AnnotationPrefix + "dns-proxied"

	// AnnotationProtocol overrides the protocol detection
	AnnotationProtocol = AnnotationPrefix + "protocol"
)

// GatewayClassAccepted condition reasons
const (
	ReasonAccepted        = "Accepted"
	ReasonInvalidParams   = "InvalidParameters"
	ReasonParamsNotFound  = "ParametersNotFound"
	ReasonUnsupportedKind = "UnsupportedKind"
)

// GetTunnelGatewayClassConfig retrieves the TunnelGatewayClassConfig referenced by a GatewayClass.
// Returns nil if the GatewayClass doesn't have parametersRef or if it's not a TunnelGatewayClassConfig.
func GetTunnelGatewayClassConfig(
	ctx context.Context,
	c client.Client,
	gatewayClass *gatewayv1.GatewayClass,
) (*networkingv1alpha2.TunnelGatewayClassConfig, error) {
	params := gatewayClass.Spec.ParametersRef
	if params == nil {
		return nil, fmt.Errorf("GatewayClass %s has no parametersRef", gatewayClass.Name)
	}

	// Validate group and kind
	if string(params.Group) != ParametersGroup {
		return nil, fmt.Errorf("GatewayClass %s parametersRef has invalid group: %s (expected %s)",
			gatewayClass.Name, params.Group, ParametersGroup)
	}
	if string(params.Kind) != ParametersKind {
		return nil, fmt.Errorf("GatewayClass %s parametersRef has invalid kind: %s (expected %s)",
			gatewayClass.Name, params.Kind, ParametersKind)
	}

	// Get namespace - use default namespace if not specified
	namespace := ""
	if params.Namespace != nil {
		namespace = string(*params.Namespace)
	}
	if namespace == "" {
		return nil, fmt.Errorf("GatewayClass %s parametersRef must specify namespace for TunnelGatewayClassConfig",
			gatewayClass.Name)
	}

	// Get the config
	config := &networkingv1alpha2.TunnelGatewayClassConfig{}
	if err := c.Get(ctx, apitypes.NamespacedName{
		Name:      params.Name,
		Namespace: namespace,
	}, config); err != nil {
		return nil, fmt.Errorf("failed to get TunnelGatewayClassConfig %s/%s: %w",
			namespace, params.Name, err)
	}

	return config, nil
}

// GetGatewayClass retrieves a GatewayClass by name.
func GetGatewayClass(ctx context.Context, c client.Client, name string) (*gatewayv1.GatewayClass, error) {
	gatewayClass := &gatewayv1.GatewayClass{}
	if err := c.Get(ctx, apitypes.NamespacedName{Name: name}, gatewayClass); err != nil {
		return nil, err
	}
	return gatewayClass, nil
}

// IsOurGatewayClass returns true if the GatewayClass is managed by this controller.
func IsOurGatewayClass(gatewayClass *gatewayv1.GatewayClass) bool {
	return string(gatewayClass.Spec.ControllerName) == ControllerName
}

// GetGatewayClassForGateway retrieves the GatewayClass for a Gateway.
func GetGatewayClassForGateway(ctx context.Context, c client.Client, gateway *gatewayv1.Gateway) (*gatewayv1.GatewayClass, error) {
	return GetGatewayClass(ctx, c, string(gateway.Spec.GatewayClassName))
}

// IsGatewayManagedByUs returns true if the Gateway's GatewayClass is managed by this controller.
func IsGatewayManagedByUs(ctx context.Context, c client.Client, gateway *gatewayv1.Gateway) (bool, error) {
	gatewayClass, err := GetGatewayClassForGateway(ctx, c, gateway)
	if err != nil {
		return false, err
	}
	return IsOurGatewayClass(gatewayClass), nil
}
