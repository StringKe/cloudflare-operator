# RedirectRule

RedirectRule is a namespaced resource that creates HTTP redirects at the edge.

## Overview

RedirectRule enables you to redirect requests to different URLs based on patterns, fully at Cloudflare's edge without reaching your origin.

### Key Features

- URL redirects
- Pattern matching
- HTTP status control
- Preserve paths and parameters

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `pattern` | string | **Yes** | URL pattern |
| `destination` | string | **Yes** | Redirect destination |
| `statusCode` | int | No | HTTP status code |
| `cloudflare` | CloudflareDetails | **Yes** | API credentials |

## Examples

### Example 1: Redirect HTTP to HTTPS

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: RedirectRule
metadata:
  name: https-redirect
  namespace: production
spec:
  pattern: "http://example.com/*"
  destination: "https://example.com/$1"
  statusCode: 301
  cloudflare:
    accountId: "1234567890abcdef"
    domain: "example.com"
    credentialsRef:
      name: production
```

## See Also

- [Cloudflare Redirect Rules](https://developers.cloudflare.com/rules/redirect/)
