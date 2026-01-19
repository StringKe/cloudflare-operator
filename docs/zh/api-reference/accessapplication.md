# AccessApplication

AccessApplication 是一个集群级资源，表示 Cloudflare Access 应用。它通过 Zero Trust 认证保护 Web 应用、SSH 端点、VNC 和其他资源。

## 概述

AccessApplication 支持两种定义访问策略的模式：

| 模式 | 说明 | 使用场景 |
|------|------|----------|
| **组引用模式** | 引用现有的 AccessGroup 资源 | 简单设置、可复用的组 |
| **内联规则模式** | 直接定义 include/exclude/require 规则 | 快速设置、应用专属规则 |

## Spec

| 字段 | 类型 | 必需 | 默认值 | 说明 |
|------|------|------|--------|------|
| `name` | string | 否 | K8s 资源名称 | Cloudflare 中的应用名称 |
| `domain` | string | **是** | - | 应用的主域名/URL |
| `type` | string | **是** | `self_hosted` | 应用类型（见下表） |
| `sessionDuration` | string | 否 | `24h` | 重新认证前的会话持续时间 |
| `policies` | []AccessPolicyRef | 否 | - | 访问策略（见策略模式） |
| `reusablePolicyRefs` | []ReusablePolicyRef | 否 | - | 可复用 Access Policy 引用 |
| `cloudflare` | CloudflareDetails | **是** | - | Cloudflare API 凭证 |

### 应用类型

| 类型 | 说明 |
|------|------|
| `self_hosted` | 自托管 Web 应用 |
| `saas` | SaaS 应用（SAML/OIDC） |
| `ssh` | SSH 端点 |
| `vnc` | VNC 端点 |
| `app_launcher` | 应用启动器 |
| `warp` | WARP 客户端 |
| `biso` | 浏览器隔离 |
| `bookmark` | 书签 |
| `dash_sso` | Dashboard SSO |
| `infrastructure` | 基础设施应用 |

### 其他 Spec 字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `selfHostedDomains` | []string | 应用的附加域名 |
| `destinations` | []AccessDestination | 公有/私有目标配置 |
| `allowedIdps` | []string | 允许的身份提供商 ID |
| `allowedIdpRefs` | []AccessIdentityProviderRef | AccessIdentityProvider 资源引用 |
| `autoRedirectToIdentity` | bool | 自动重定向到身份提供商 |
| `appLauncherVisible` | bool | 在应用启动器中显示 |
| `skipInterstitial` | bool | 跳过过渡页面 |
| `logoUrl` | string | 应用 Logo URL |
| `customDenyMessage` | string | 自定义拒绝消息 |
| `customDenyUrl` | string | 自定义拒绝 URL |
| `corsHeaders` | AccessApplicationCorsHeaders | CORS 配置 |
| `saasApp` | SaasApplicationConfig | SaaS 应用配置（type=saas 时） |
| `tags` | []string | 自定义标签 |

## 策略模式

### 模式 1：组引用模式（简单）

引用现有的 AccessGroup 资源或 Cloudflare Access Groups：

```yaml
spec:
  policies:
    # 选项 1：通过名称引用 K8s AccessGroup
    - name: accessgroup-employees
      decision: allow
      precedence: 1

    # 选项 2：通过 UUID 引用 Cloudflare Access Group
    - groupId: "12345678-1234-1234-1234-123456789abc"
      decision: allow
      precedence: 2

    # 选项 3：通过显示名称引用 Cloudflare Access Group
    - cloudflareGroupName: "Infrastructure Users"
      decision: allow
      precedence: 3
```

### 模式 2：内联规则模式（高级）

直接在策略中定义 include/exclude/require 规则：

```yaml
spec:
  policies:
    - policyName: "允许公司员工"
      decision: allow
      precedence: 1
      # Include: 匹配任意规则的用户将被授予访问权限（或逻辑）
      include:
        - email:
            email: "admin@example.com"
        - emailDomain:
            domain: "example.com"
      # Exclude: 匹配任意规则的用户将被拒绝（非逻辑）
      exclude:
        - email:
            email: "contractor@example.com"
      # Require: 用户必须匹配所有规则（与逻辑）
      require:
        - geo:
            country: ["US", "CA"]
```

### AccessPolicyRef 字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | K8s AccessGroup 资源名称（组引用模式） |
| `groupId` | string | Cloudflare Access Group UUID（组引用模式） |
| `cloudflareGroupName` | string | Cloudflare Access Group 显示名称（组引用模式） |
| `include` | []AccessGroupRule | Include 规则（内联规则模式） |
| `exclude` | []AccessGroupRule | Exclude 规则（内联规则模式） |
| `require` | []AccessGroupRule | Require 规则（内联规则模式） |
| `decision` | string | 策略决策：`allow`、`deny`、`bypass`、`non_identity` |
| `precedence` | int | 评估顺序（数字越小优先级越高） |
| `policyName` | string | Cloudflare 中的策略名称 |
| `sessionDuration` | string | 此策略的会话持续时间覆盖 |

### 支持的规则类型

以下规则类型可用于 `include`、`exclude` 和 `require` 数组：

| 规则类型 | 说明 | 示例 |
|----------|------|------|
| `email` | 匹配特定邮箱 | `email: { email: "user@example.com" }` |
| `emailDomain` | 匹配邮箱域 | `emailDomain: { domain: "example.com" }` |
| `everyone` | 匹配所有用户 | `everyone: {}` |
| `group` | 匹配 IdP 组 | `group: { id: "group-id" }` |
| `ipRanges` | 匹配 IP 范围 | `ipRanges: { ranges: ["10.0.0.0/8"] }` |
| `geo` | 匹配国家代码 | `geo: { country: ["US", "CA"] }` |
| `anyValidServiceToken` | 匹配任何有效服务令牌 | `anyValidServiceToken: {}` |
| `serviceToken` | 匹配特定服务令牌 | `serviceToken: { tokenId: "token-id" }` |
| `certificate` | 匹配客户端证书 | `certificate: {}` |
| `commonName` | 匹配证书 CN | `commonName: { commonName: "*.example.com" }` |
| `loginMethod` | 匹配登录方法 | `loginMethod: { id: "method-id" }` |
| `devicePosture` | 匹配设备态势 | `devicePosture: { integrationUid: "uid" }` |
| `warp` | 匹配 WARP 客户端 | `warp: {}` |
| `gsuite` | 匹配 Google Workspace | `gsuite: { identityProviderId: "id", email: "user@example.com" }` |
| `github` | 匹配 GitHub 组织 | `github: { identityProviderId: "id", name: "org-name" }` |
| `okta` | 匹配 Okta 组 | `okta: { identityProviderId: "id", name: "group-name" }` |
| `azure` | 匹配 Azure AD 组 | `azure: { identityProviderId: "id", id: "group-id" }` |
| `saml` | 匹配 SAML 属性 | `saml: { attributeName: "role", attributeValue: "admin" }` |
| `authContext` | 匹配认证上下文 | `authContext: { id: "context-id", acId: "ac-id", identityProviderId: "id" }` |
| `externalEvaluation` | 外部评估 | `externalEvaluation: { evaluateUrl: "https://..." }` |
| `groupId` | 通过 ID 匹配 Access Group | `groupId: { id: "group-id" }` |
| `emailList` | 匹配邮箱列表 | `emailList: { id: "list-id" }` |
| `ipList` | 匹配 IP 列表 | `ipList: { id: "list-id" }` |

## Status

| 字段 | 类型 | 说明 |
|------|------|------|
| `applicationId` | string | Cloudflare 应用 ID |
| `aud` | string | 应用受众（AUD）标签 |
| `accountId` | string | Cloudflare 账户 ID |
| `domain` | string | 配置的域名 |
| `selfHostedDomains` | []string | 所有配置的域名 |
| `state` | string | 当前状态 |
| `resolvedPolicies` | []ResolvedPolicyStatus | 已解析的策略信息 |
| `conditions` | []Condition | 标准 Kubernetes 条件 |

## 示例

### 使用组引用的基本应用

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessApplication
metadata:
  name: internal-dashboard
spec:
  name: 内部仪表盘
  domain: dashboard.example.com
  type: self_hosted
  sessionDuration: 24h
  appLauncherVisible: true

  policies:
    - name: employees
      decision: allow
      precedence: 1

  cloudflare:
    accountId: "<account-id>"
    domain: example.com
    secret: cloudflare-credentials
```

### 使用内联邮箱规则的应用

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessApplication
metadata:
  name: team-wiki
spec:
  name: 团队 Wiki
  domain: wiki.example.com
  type: self_hosted

  policies:
    - policyName: "允许团队成员"
      decision: allow
      precedence: 1
      include:
        - emailDomain:
            domain: "example.com"
      exclude:
        - email:
            email: "contractor@example.com"

  cloudflare:
    accountId: "<account-id>"
    domain: example.com
    secret: cloudflare-credentials
```

### 使用 Require 规则的应用（多条件）

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessApplication
metadata:
  name: admin-panel
spec:
  name: 管理面板
  domain: admin.example.com
  type: self_hosted
  sessionDuration: 1h

  policies:
    - policyName: "管理员访问 - 仅美国"
      decision: allow
      precedence: 1
      include:
        - emailDomain:
            domain: "example.com"
      require:
        - geo:
            country: ["US"]

  cloudflare:
    accountId: "<account-id>"
    domain: example.com
    secret: cloudflare-credentials
```

### 公开 API（绕过认证）

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessApplication
metadata:
  name: public-api
spec:
  name: 公开 API
  domain: api.example.com
  type: self_hosted

  policies:
    - policyName: "允许所有人"
      decision: bypass
      precedence: 1
      include:
        - everyone: {}

  cloudflare:
    accountId: "<account-id>"
    domain: example.com
    secret: cloudflare-credentials
```

### 服务令牌访问（M2M）

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessApplication
metadata:
  name: api-service
spec:
  name: API 服务
  domain: api-internal.example.com
  type: self_hosted

  policies:
    - policyName: "服务令牌访问"
      decision: non_identity
      precedence: 1
      include:
        - anyValidServiceToken: {}
    - policyName: "人类访问"
      decision: allow
      precedence: 2
      include:
        - emailDomain:
            domain: "example.com"

  cloudflare:
    accountId: "<account-id>"
    domain: example.com
    secret: cloudflare-credentials
```

## 相关资源

- [AccessGroup](accessgroup.md) - 定义可复用的访问组
- [AccessServiceToken](accessservicetoken.md) - 创建 M2M 认证的服务令牌
- [AccessIdentityProvider](accessidentityprovider.md) - 配置身份提供商

## 参见

- [示例](../../../examples/03-zero-trust/access-application/)
- [Cloudflare Access 文档](https://developers.cloudflare.com/cloudflare-one/policies/access/)
