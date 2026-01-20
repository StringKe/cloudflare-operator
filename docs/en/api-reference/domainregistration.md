# DomainRegistration

DomainRegistration is a cluster-scoped resource for managing domain registrations with Cloudflare (Enterprise only).

## Overview

DomainRegistration enables domain registration and management through Cloudflare for enterprise customers.

### Key Features

- Domain registration
- Domain management
- Enterprise feature

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `domain` | string | **Yes** | Domain to register |
| `cloudflare` | CloudflareDetails | **Yes** | API credentials |

## Examples

### Example 1: Register Domain

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: DomainRegistration
metadata:
  name: new-domain
spec:
  domain: "example.com"
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## See Also

- [Cloudflare Registrar](https://developers.cloudflare.com/registrar/)
