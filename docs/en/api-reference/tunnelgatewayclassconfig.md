# TunnelGatewayClassConfig

TunnelGatewayClassConfig is a cluster-scoped resource that configures Kubernetes Gateway API integration with Cloudflare Tunnels.

## Overview

TunnelGatewayClassConfig enables automatic DNS management when using Kubernetes Gateway API resources with Cloudflare Tunnel.

### Key Features

- Gateway API integration
- Automatic DNS management
- Modern networking APIs

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `cloudflare` | CloudflareDetails | **Yes** | API credentials |

## Examples

### Example 1: Gateway Configuration

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: TunnelGatewayClassConfig
metadata:
  name: tunnel-gateway-config
spec:
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## See Also

- [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/)
