# TunnelBinding

DEPRECATED: TunnelBinding is a namespaced resource. Please use Ingress or Gateway API instead.

## Overview

TunnelBinding is deprecated. It was used to bind Tunnels to services. Please migrate to standard Kubernetes Ingress or Gateway API resources.

### Alternatives

- Use Kubernetes Ingress with TunnelIngressClassConfig
- Use Kubernetes Gateway API with TunnelGatewayClassConfig
- Use DNSRecord resources for manual DNS management

## See Also

- [Kubernetes Ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/)
- [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/)
