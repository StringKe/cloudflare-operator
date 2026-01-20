# GatewayRule

GatewayRule is a cluster-scoped resource that defines routing rules for Cloudflare Gateway DNS filtering and security policies.

## Overview

GatewayRule creates DNS filtering and security policies in Cloudflare Gateway. Rules can block, allow, or redirect DNS queries based on domain patterns and other criteria.

### Key Features

- DNS filtering rules
- Security policy enforcement
- Traffic routing control
- Pattern-based matching
- Multiple action types

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `pattern` | string | **Yes** | Domain pattern to match |
| `action` | string | **Yes** | Action: block, allow, redirect |
| `priority` | int | No | Rule priority |
| `cloudflare` | CloudflareDetails | **Yes** | API credentials |

## Examples

### Example 1: Block Malware Domains

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: GatewayRule
metadata:
  name: block-malware
spec:
  pattern: "*.malware-domain.com"
  action: "block"
  priority: 10
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## Related Resources

- [GatewayList](gatewaylist.md) - Rule lists
- [GatewayConfiguration](gatewayconfiguration.md) - Gateway settings

## See Also

- [Cloudflare Gateway DNS](https://developers.cloudflare.com/cloudflare-one/policies/gateway/dns-policies/)
