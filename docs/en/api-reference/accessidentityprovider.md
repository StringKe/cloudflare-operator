# AccessIdentityProvider

AccessIdentityProvider is a cluster-scoped resource that configures identity providers for Cloudflare Zero Trust Access, supporting OAuth, OIDC, SAML, and other authentication methods.

## Overview

AccessIdentityProvider resources represent configured identity providers (IdPs) in your Cloudflare account. These providers authenticate users before they can access protected applications. The operator supports multiple provider types including Google Workspace, Microsoft Azure AD, Okta, Auth0, OIDC providers, and more.

### Key Features

| Feature | Description |
|---------|-------------|
| **Multiple Provider Types** | Google, Azure AD, Okta, Auth0, OIDC, SAML, etc. |
| **OAuth/OIDC Support** | Standard OAuth 2.0 and OpenID Connect |
| **Secure Credentials** | Reference credentials from Kubernetes Secrets |
| **SCIM Provisioning** | User/group synchronization via SCIM |
| **Custom Endpoints** | Support for custom OIDC and SAML endpoints |

### Use Cases

- **Enterprise Authentication**: Integrate with corporate identity systems
- **Multi-Provider Setup**: Configure multiple identity sources
- **User Synchronization**: Sync users via SCIM protocol
- **Custom OIDC**: Integrate with custom OAuth providers
- **Single Sign-On**: Centralize authentication across applications

## Spec

### Main Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | **Yes** | Provider type (google, azureAd, okta, auth0, oidc, saml, etc.) |
| `name` | string | No | Display name for the provider |
| `config` | *IdentityProviderConfig | No | Provider-specific configuration |
| `configSecretRef` | *SecretKeySelector | No | Secret reference for sensitive config |
| `scimConfig` | *IdentityProviderScimConfig | No | SCIM provisioning configuration |
| `cloudflare` | CloudflareDetails | **Yes** | Cloudflare API credentials |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `providerId` | string | Cloudflare provider ID |
| `accountId` | string | Cloudflare Account ID |
| `state` | string | Current state |
| `conditions` | []metav1.Condition | Latest observations |

## Examples

### Example 1: Google Workspace

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessIdentityProvider
metadata:
  name: google-workspace
spec:
  type: google
  name: "Google Workspace"
  config:
    appsDomain: "example.com"
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

### Example 2: Microsoft Azure AD

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: azure-credentials
  namespace: cloudflare-operator-system
type: Opaque
stringData:
  CLIENT_ID: "your-client-id"
  CLIENT_SECRET: "your-client-secret"
---
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessIdentityProvider
metadata:
  name: azure-ad
spec:
  type: azureAd
  name: "Azure AD"
  config:
    appsDomain: "tenant.onmicrosoft.com"
  configSecretRef:
    name: azure-credentials
    namespace: cloudflare-operator-system
    key: CLIENT_ID
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

### Example 3: Custom OIDC Provider with SCIM

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessIdentityProvider
metadata:
  name: custom-oidc
spec:
  type: oidc
  name: "Custom OIDC Provider"
  config:
    clientId: "oidc-client-id"
    authUrl: "https://idp.example.com/oauth/authorize"
    tokenUrl: "https://idp.example.com/oauth/token"
    certsUrl: "https://idp.example.com/oauth/jwks"
  scimConfig:
    enabled: true
    userDeprovision: true
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## Prerequisites

- Cloudflare Zero Trust subscription
- Valid identity provider account/subscription
- OAuth client credentials (if using OAuth/OIDC)
- Kubernetes Secret with sensitive configuration

## Limitations

- Provider type cannot be changed after creation
- SCIM provisioning requires provider support
- Custom OIDC endpoints must be publicly accessible
- Some provider types require enterprise plan

## Related Resources

- [AccessApplication](accessapplication.md) - Applications using this provider
- [AccessPolicy](accesspolicy.md) - Policies referencing this provider
- [AccessGroup](accessgroup.md) - Groups from this provider

## See Also

- [Cloudflare Access Identity Providers](https://developers.cloudflare.com/cloudflare-one/identity/idp-integration/)
- [OAuth 2.0 Specification](https://oauth.net/2/)
- [OpenID Connect](https://openid.net/connect/)
