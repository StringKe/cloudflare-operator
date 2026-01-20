# AccessIdentityProvider

AccessIdentityProvider 是一个集群作用域的资源，为 Cloudflare Zero Trust Access 配置身份提供商，支持 OAuth、OIDC、SAML 和其他身份验证方法。

## 概述

AccessIdentityProvider 资源代表 Cloudflare 账户中配置的身份提供商 (IdP)。这些提供商在用户可以访问受保护的应用程序之前进行身份验证。操作员支持多种提供商类型，包括 Google Workspace、Microsoft Azure AD、Okta、Auth0、OIDC 提供商等。

### 主要特性

| 特性 | 描述 |
|------|------|
| **多种提供商类型** | Google、Azure AD、Okta、Auth0、OIDC、SAML 等 |
| **OAuth/OIDC 支持** | 标准 OAuth 2.0 和 OpenID Connect |
| **安全凭证** | 从 Kubernetes Secret 引用凭证 |
| **SCIM 配置** | 通过 SCIM 协议进行用户/组同步 |
| **自定义端点** | 支持自定义 OIDC 和 SAML 端点 |

### 使用场景

- **企业身份验证**：与公司身份系统集成
- **多提供商设置**：配置多个身份源
- **用户同步**：通过 SCIM 协议同步用户
- **自定义 OIDC**：与自定义 OAuth 提供商集成
- **单点登录**：跨应用程序集中身份验证

## 规范

### 主要字段

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `type` | string | **是** | 提供商类型（google、azureAd、okta、auth0、oidc、saml 等） |
| `name` | string | 否 | 提供商的显示名称 |
| `config` | *IdentityProviderConfig | 否 | 提供商特定的配置 |
| `configSecretRef` | *SecretKeySelector | 否 | 敏感配置的 Secret 引用 |
| `scimConfig` | *IdentityProviderScimConfig | 否 | SCIM 配置 |
| `cloudflare` | CloudflareDetails | **是** | Cloudflare API 凭证 |

## 状态

| 字段 | 类型 | 描述 |
|------|------|------|
| `providerId` | string | Cloudflare 提供商 ID |
| `accountId` | string | Cloudflare 账户 ID |
| `state` | string | 当前状态 |
| `conditions` | []metav1.Condition | 最新观察 |

## 示例

### 示例 1：Google Workspace

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

### 示例 2：Microsoft Azure AD

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

### 示例 3：具有 SCIM 的自定义 OIDC 提供商

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

## 前置条件

- Cloudflare Zero Trust 订阅
- 有效的身份提供商账户/订阅
- OAuth 客户端凭证（如果使用 OAuth/OIDC）
- 包含敏感配置的 Kubernetes Secret

## 限制

- 创建后无法更改提供商类型
- SCIM 配置需要提供商支持
- 自定义 OIDC 端点必须公开可访问
- 某些提供商类型需要企业计划

## 相关资源

- [AccessApplication](accessapplication.md) - 使用此提供商的应用程序
- [AccessPolicy](accesspolicy.md) - 引用此提供商的策略
- [AccessGroup](accessgroup.md) - 来自此提供商的组

## 另请参阅

- [Cloudflare Access 身份提供商](https://developers.cloudflare.com/cloudflare-one/identity/idp-integration/)
- [OAuth 2.0 规范](https://oauth.net/2/)
- [OpenID Connect](https://openid.net/connect/)
