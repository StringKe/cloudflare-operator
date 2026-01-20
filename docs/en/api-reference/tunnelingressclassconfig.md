# TunnelIngressClassConfig

TunnelIngressClassConfig is a cluster-scoped resource that configures Kubernetes Ingress integration with Cloudflare Tunnels.

## Overview

TunnelIngressClassConfig enables automatic DNS management when using Kubernetes Ingress resources with Cloudflare Tunnel.

### Key Features

- Ingress integration
- Automatic DNS management
- DNS TXT record management

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `cloudflare` | CloudflareDetails | **Yes** | API credentials |

## Examples

### Example 1: Ingress Configuration

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: TunnelIngressClassConfig
metadata:
  name: tunnel-ingress-config
spec:
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## See Also

- [Kubernetes Ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/)
