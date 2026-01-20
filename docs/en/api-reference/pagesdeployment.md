# PagesDeployment

PagesDeployment is a namespaced resource that manages Cloudflare Pages project deployments with direct upload and intelligent rollback.

## Overview

PagesDeployment enables you to deploy your application directly to Cloudflare Pages from Kubernetes. It supports upload of build artifacts, automatic rollback to previous versions, and integration with CI/CD pipelines.

### Key Features

- Direct artifact upload to Pages
- Automatic rollback on errors
- Build manifest support
- Multiple deployment strategies
- Status tracking and history

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `projectRef` | ProjectRef | **Yes** | Reference to PagesProject |
| `source` | DeploymentSource | **Yes** | Deployment source |
| `rollbackOnError` | bool | No | Auto-rollback on failures |
| `cloudflare` | CloudflareDetails | **Yes** | API credentials |

## Examples

### Example 1: Deploy Build Artifacts

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: app-deployment
  namespace: production
spec:
  projectRef:
    name: my-app
  source:
    path: "/build"
    format: "service-bindings"
  rollbackOnError: true
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## Prerequisites

- PagesProject resource created
- Build artifacts ready
- Valid API credentials

## Related Resources

- [PagesProject](pagesproject.md) - The project
- [PagesDomain](pagesdomain.md) - Custom domain

## See Also

- [Cloudflare Pages Deployments](https://developers.cloudflare.com/pages/deployments/)
