// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package controller

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

// CRDChecker provides methods to check if CRDs exist in the cluster
type CRDChecker struct {
	discoveryClient discovery.DiscoveryInterface
}

// NewCRDChecker creates a new CRDChecker using the provided REST config
func NewCRDChecker(config *rest.Config) (*CRDChecker, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}
	return &CRDChecker{discoveryClient: dc}, nil
}

// HasGVK checks if a specific GroupVersionKind is available in the cluster
func (c *CRDChecker) HasGVK(gvk schema.GroupVersionKind) bool {
	resourceList, err := c.discoveryClient.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err != nil {
		return false
	}

	for _, resource := range resourceList.APIResources {
		if resource.Kind == gvk.Kind {
			return true
		}
	}
	return false
}

// HasGatewayAPI checks if Gateway API CRDs are installed
// It checks for the core Gateway and GatewayClass types
func (c *CRDChecker) HasGatewayAPI() bool {
	// Check for GatewayClass and Gateway from gateway.networking.k8s.io/v1
	gatewayClassGVK := schema.GroupVersionKind{
		Group:   "gateway.networking.k8s.io",
		Version: "v1",
		Kind:    "GatewayClass",
	}

	gatewayGVK := schema.GroupVersionKind{
		Group:   "gateway.networking.k8s.io",
		Version: "v1",
		Kind:    "Gateway",
	}

	return c.HasGVK(gatewayClassGVK) && c.HasGVK(gatewayGVK)
}

// HasHTTPRoute checks if HTTPRoute CRD is installed
func (c *CRDChecker) HasHTTPRoute() bool {
	httpRouteGVK := schema.GroupVersionKind{
		Group:   "gateway.networking.k8s.io",
		Version: "v1",
		Kind:    "HTTPRoute",
	}
	return c.HasGVK(httpRouteGVK)
}

// HasTCPRoute checks if TCPRoute CRD is installed (alpha2)
func (c *CRDChecker) HasTCPRoute() bool {
	tcpRouteGVK := schema.GroupVersionKind{
		Group:   "gateway.networking.k8s.io",
		Version: "v1alpha2",
		Kind:    "TCPRoute",
	}
	return c.HasGVK(tcpRouteGVK)
}

// HasUDPRoute checks if UDPRoute CRD is installed (alpha2)
func (c *CRDChecker) HasUDPRoute() bool {
	udpRouteGVK := schema.GroupVersionKind{
		Group:   "gateway.networking.k8s.io",
		Version: "v1alpha2",
		Kind:    "UDPRoute",
	}
	return c.HasGVK(udpRouteGVK)
}

// GatewayAPIStatus contains the status of Gateway API CRDs
type GatewayAPIStatus struct {
	// GatewayClassAvailable indicates if GatewayClass CRD is available
	GatewayClassAvailable bool
	// GatewayAvailable indicates if Gateway CRD is available
	GatewayAvailable bool
	// HTTPRouteAvailable indicates if HTTPRoute CRD is available
	HTTPRouteAvailable bool
	// TCPRouteAvailable indicates if TCPRoute CRD is available
	TCPRouteAvailable bool
	// UDPRouteAvailable indicates if UDPRoute CRD is available
	UDPRouteAvailable bool
}

// GetGatewayAPIStatus returns the detailed status of Gateway API CRDs
func (c *CRDChecker) GetGatewayAPIStatus() GatewayAPIStatus {
	return GatewayAPIStatus{
		GatewayClassAvailable: c.HasGVK(schema.GroupVersionKind{
			Group:   "gateway.networking.k8s.io",
			Version: "v1",
			Kind:    "GatewayClass",
		}),
		GatewayAvailable: c.HasGVK(schema.GroupVersionKind{
			Group:   "gateway.networking.k8s.io",
			Version: "v1",
			Kind:    "Gateway",
		}),
		HTTPRouteAvailable: c.HasHTTPRoute(),
		TCPRouteAvailable:  c.HasTCPRoute(),
		UDPRouteAvailable:  c.HasUDPRoute(),
	}
}

// IsComplete returns true if all core Gateway API CRDs are available
func (s GatewayAPIStatus) IsComplete() bool {
	return s.GatewayClassAvailable && s.GatewayAvailable && s.HTTPRouteAvailable
}

// CoreAvailable returns true if the core Gateway API CRDs are available
// (GatewayClass and Gateway, which are the minimum requirement)
func (s GatewayAPIStatus) CoreAvailable() bool {
	return s.GatewayClassAvailable && s.GatewayAvailable
}
