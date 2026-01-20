# GatewayConfiguration

GatewayConfiguration is a cluster-scoped resource that configures global Cloudflare Gateway settings.

## Overview

GatewayConfiguration manages account-level Gateway settings including logging, certificate inspection, and global policies.

### Key Features

- Global gateway settings
- Logging configuration
- Policy defaults
- Certificate options

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `logging` | LoggingConfig | No | Logging settings |
| `inspection` | bool | No | Certificate inspection |
| `cloudflare` | CloudflareDetails | **Yes** | API credentials |

## Examples

### Example 1: Gateway Logging

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: GatewayConfiguration
metadata:
  name: gateway-config
spec:
  logging:
    enabled: true
    level: "standard"
  inspection: true
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## See Also

- [Cloudflare Gateway Configuration](https://developers.cloudflare.com/cloudflare-one/policies/gateway/)
