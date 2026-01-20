# AccessServiceToken

AccessServiceToken is a namespaced resource that creates and manages Cloudflare Access Service Tokens for machine-to-machine authentication without human interaction.

## Overview

Service Tokens provide long-lived credentials for non-interactive (machine-to-machine) authentication to protected Cloudflare Zero Trust applications. Unlike user authentication, Service Tokens authenticate applications or services that need programmatic access. The operator automatically creates the token in Cloudflare and stores credentials in a Kubernetes Secret.

### Key Features

| Feature | Description |
|---------|-------------|
| **Machine-to-Machine Auth** | Enable service-to-service authentication |
| **Long-Lived Credentials** | Persistent access without user interaction |
| **Automatic Storage** | Credentials stored in Kubernetes Secrets |
| **Token Metadata** | Track creation and usage information |
| **Expiration Tracking** | Monitor token expiration dates |

### Use Cases

- **Service Authentication**: Enable microservices to authenticate with each other
- **CI/CD Integration**: Authenticate deployment pipelines with protected resources
- **API Access**: Provide programmatic access to internal APIs
- **Service Account**: Create accounts for automated systems
- **Cross-Service Communication**: Enable services to call protected endpoints

## Spec

### Main Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | No | Resource name | Display name for the service token |
| `secretRef` | ServiceTokenSecretRef | **Yes** | - | Secret location for storing credentials |
| `cloudflare` | CloudflareDetails | **Yes** | - | Cloudflare API credentials |

### ServiceTokenSecretRef

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | **Yes** | - | Name of Secret to create/update |
| `namespace` | string | **Yes** | - | Namespace for the Secret |
| `clientIdKey` | string | No | `CF_ACCESS_CLIENT_ID` | Key for Client ID |
| `clientSecretKey` | string | No | `CF_ACCESS_CLIENT_SECRET` | Key for Client Secret |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `tokenId` | string | Cloudflare Service Token ID |
| `clientId` | string | Service Token Client ID |
| `accountId` | string | Cloudflare Account ID |
| `expiresAt` | string | Token expiration time |
| `createdAt` | string | Creation time |
| `updatedAt` | string | Last update time |
| `lastSeenAt` | string | Last usage time |
| `conditions` | []metav1.Condition | Latest observations |

## Examples

### Example 1: Basic Service Token

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessServiceToken
metadata:
  name: api-service-token
  namespace: production
spec:
  name: "API Service Account"
  secretRef:
    name: api-service-creds
    namespace: production
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

### Example 2: Service Token with Custom Keys

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessServiceToken
metadata:
  name: worker-credentials
  namespace: workers
spec:
  name: "Worker Service Token"
  secretRef:
    name: worker-creds
    namespace: workers
    clientIdKey: WORKER_CLIENT_ID
    clientSecretKey: WORKER_CLIENT_SECRET
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

### Example 3: CI/CD Integration

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessServiceToken
metadata:
  name: ci-cd-token
  namespace: ci-cd
spec:
  name: "CI/CD Pipeline Token"
  secretRef:
    name: cicd-cf-credentials
    namespace: ci-cd
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
---
# Usage in GitHub Actions
apiVersion: v1
kind: ConfigMap
metadata:
  name: ci-cd-config
  namespace: ci-cd
data:
  deploy.sh: |
    #!/bin/bash
    CLIENT_ID=$(kubectl get secret cicd-cf-credentials -o jsonpath='{.data.CF_ACCESS_CLIENT_ID}' | base64 -d)
    CLIENT_SECRET=$(kubectl get secret cicd-cf-credentials -o jsonpath='{.data.CF_ACCESS_CLIENT_SECRET}' | base64 -d)
    # Use credentials to authenticate with protected services
```

## Prerequisites

- Cloudflare Zero Trust subscription
- Valid Cloudflare API credentials
- Kubernetes Namespace where Secret will be created
- Protected Access Application to authenticate against

## Limitations

- Service Token cannot be retrieved after creation
- Credentials are stored in plain text in Kubernetes Secret
- Only one token per AccessServiceToken resource
- Token metadata is read-only
- Cannot update token after creation (must delete and recreate)

## Related Resources

- [AccessApplication](accessapplication.md) - Applications this token can access
- [AccessPolicy](accesspolicy.md) - Policies controlling token access
- [CloudflareCredentials](cloudflarecredentials.md) - API credentials for operator

## See Also

- [Cloudflare Service Tokens](https://developers.cloudflare.com/cloudflare-one/identity/service-tokens/)
- [Machine-to-Machine Authentication](https://developers.cloudflare.com/cloudflare-one/identity/)
- [Kubernetes Secrets](https://kubernetes.io/docs/concepts/configuration/secret/)
