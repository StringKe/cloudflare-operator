# GatewayList

GatewayList is a cluster-scoped resource that defines reusable lists for Gateway rules.

## Overview

GatewayList enables you to create lists of domains or IP addresses that can be referenced by multiple Gateway rules.

### Key Features

- Reusable domain lists
- Centralized list management
- Multiple list types
- Easy maintenance

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | **Yes** | List name |
| `type` | string | **Yes** | List type: domains, ips |
| `items` | []string | **Yes** | List items |
| `cloudflare` | CloudflareDetails | **Yes** | API credentials |

## Examples

### Example 1: Domain List

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: GatewayList
metadata:
  name: blocked-domains
spec:
  name: "Blocked Domains"
  type: "domains"
  items:
    - "malware.com"
    - "phishing.com"
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## Related Resources

- [GatewayRule](gatewayrule.md) - Rules using this list

## See Also

- [Cloudflare Gateway Lists](https://developers.cloudflare.com/cloudflare-one/policies/gateway/)
