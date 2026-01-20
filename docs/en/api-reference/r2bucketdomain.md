# R2BucketDomain

R2BucketDomain is a namespaced resource that configures custom domains for Cloudflare R2 buckets.

## Overview

R2BucketDomain allows you to serve R2 bucket contents via a custom domain with Cloudflare's CDN and security features. Configure CNAME or full domain delegation to serve R2 objects through your own domain.

### Key Features

- Custom domain configuration for R2 buckets
- CDN integration for fast content delivery
- SSL/TLS termination at Cloudflare edge
- Access logging and security options

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `domain` | string | **Yes** | Custom domain for the bucket |
| `bucketRef` | BucketRef | **Yes** | Reference to R2Bucket resource |
| `cloudflare` | CloudflareDetails | **Yes** | Cloudflare API credentials |

## Examples

### Example 1: Custom Domain for R2 Bucket

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: R2BucketDomain
metadata:
  name: assets-domain
  namespace: production
spec:
  domain: "assets.example.com"
  bucketRef:
    name: app-storage
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## Prerequisites

- R2Bucket resource created
- Domain managed by Cloudflare DNS
- Valid API credentials

## Related Resources

- [R2Bucket](r2bucket.md) - The storage bucket
- [R2BucketNotification](r2bucketnotification.md) - Event notifications

## See Also

- [Cloudflare R2 Custom Domains](https://developers.cloudflare.com/r2/data-access/domain-setup/)
