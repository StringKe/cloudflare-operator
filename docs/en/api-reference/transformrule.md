# TransformRule

TransformRule is a namespaced resource that modifies HTTP requests and responses at the edge.

## Overview

TransformRule enables you to manipulate HTTP headers and request/response bodies at Cloudflare's edge before reaching your origin.

### Key Features

- Header manipulation
- URL rewriting
- Body transformation
- Request/response modification

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | **Yes** | Rule name |
| `pattern` | string | **Yes** | URL pattern |
| `transforms` | []Transform | No | Transformations to apply |
| `cloudflare` | CloudflareDetails | **Yes** | API credentials |

## Examples

### Example 1: Add Security Headers

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: TransformRule
metadata:
  name: add-security-headers
  namespace: production
spec:
  name: "Add Security Headers"
  pattern: "*example.com/*"
  cloudflare:
    accountId: "1234567890abcdef"
    domain: "example.com"
    credentialsRef:
      name: production
```

## See Also

- [Cloudflare Transform Rules](https://developers.cloudflare.com/rules/transform/)
