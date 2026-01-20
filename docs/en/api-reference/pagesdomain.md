# PagesDomain

PagesDomain is a namespaced resource that configures custom domains for Cloudflare Pages projects.

## Overview

PagesDomain allows you to serve Pages projects through custom domains. Configure DNS settings and SSL/TLS options directly from Kubernetes.

### Key Features

- Custom domain configuration
- Automatic SSL certificates
- DNS management integration
- Environment-specific domains
- Subdomain support

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `domain` | string | **Yes** | Custom domain |
| `projectRef` | ProjectRef | **Yes** | Reference to PagesProject |
| `environment` | string | No | Production or preview |
| `cloudflare` | CloudflareDetails | **Yes** | API credentials |

## Examples

### Example 1: Custom Domain

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDomain
metadata:
  name: app-domain
  namespace: production
spec:
  domain: "app.example.com"
  projectRef:
    name: my-app
  environment: "production"
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## Prerequisites

- PagesProject created
- Domain managed by Cloudflare
- Valid API credentials

## Related Resources

- [PagesProject](pagesproject.md) - The project
- [PagesDeployment](pagesdeployment.md) - Project deployment

## See Also

- [Cloudflare Pages Custom Domains](https://developers.cloudflare.com/pages/platform/custom-domains/)
