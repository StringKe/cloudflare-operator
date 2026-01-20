# CloudflareDomain

CloudflareDomain is a cluster-scoped resource that manages comprehensive domain configuration in Cloudflare.

## Overview

CloudflareDomain configures zone settings including SSL/TLS, caching, security, WAF, and other domain-level features.

### Key Features

- Zone configuration
- SSL/TLS settings
- Caching policies
- Security options
- WAF rules

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `domain` | string | **Yes** | Domain name |
| `settings` | DomainSettings | No | Domain settings |
| `cloudflare` | CloudflareDetails | **Yes** | API credentials |

## Examples

### Example 1: Domain Configuration

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: CloudflareDomain
metadata:
  name: example-domain
spec:
  domain: "example.com"
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## See Also

- [Cloudflare Zones](https://developers.cloudflare.com/dns/)
