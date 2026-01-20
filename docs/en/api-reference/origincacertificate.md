# OriginCACertificate

OriginCACertificate is a namespaced resource that manages Cloudflare Origin CA certificates for origin authentication.

## Overview

OriginCACertificate creates and manages Cloudflare Origin CA certificates that prove traffic genuinely came from Cloudflare's network.

### Key Features

- Origin authentication
- Automatic certificate generation
- Certificate storage in Kubernetes Secrets
- Certificate rotation support

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `hostname` | string | **Yes** | Hostname for certificate |
| `secretRef` | SecretRef | **Yes** | Secret for cert storage |
| `cloudflare` | CloudflareDetails | **Yes** | API credentials |

## Examples

### Example 1: Origin CA Certificate

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: OriginCACertificate
metadata:
  name: origin-cert
  namespace: production
spec:
  hostname: "*.example.com"
  secretRef:
    name: origin-ca-cert
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## See Also

- [Cloudflare Origin CA](https://developers.cloudflare.com/ssl/origin-configuration/origin-ca/)
