# AccessServiceToken

AccessServiceToken 是一个命名空间作用域的资源，为不需要人工交互的机器对机器身份验证创建和管理 Cloudflare Access 服务令牌。

## 概述

服务令牌为不交互式（机器对机器）身份验证提供长期凭证以访问受保护的 Cloudflare Zero Trust 应用程序。与用户身份验证不同，服务令牌对需要编程式访问的应用程序或服务进行身份验证。操作员会自动在 Cloudflare 中创建令牌并将凭证存储在 Kubernetes Secret 中。

### 主要特性

| 特性 | 描述 |
|------|------|
| **机器对机器身份验证** | 启用服务间身份验证 |
| **长期凭证** | 无需用户交互的持久访问 |
| **自动存储** | 凭证存储在 Kubernetes Secret 中 |
| **令牌元数据** | 跟踪创建和使用信息 |
| **过期时间跟踪** | 监控令牌过期日期 |

### 使用场景

- **服务身份验证**：启用微服务相互身份验证
- **CI/CD 集成**：使用受保护资源对部署管道进行身份验证
- **API 访问**：提供对内部 API 的编程式访问
- **服务账户**：为自动化系统创建账户
- **跨服务通信**：使服务能够调用受保护的端点

## 规范

### 主要字段

| 字段 | 类型 | 必需 | 默认值 | 描述 |
|------|------|------|--------|------|
| `name` | string | 否 | 资源名称 | 服务令牌的显示名称 |
| `secretRef` | ServiceTokenSecretRef | **是** | - | 用于存储凭证的 Secret 位置 |
| `cloudflare` | CloudflareDetails | **是** | - | Cloudflare API 凭证 |

### ServiceTokenSecretRef

| 字段 | 类型 | 必需 | 默认值 | 描述 |
|------|------|------|--------|------|
| `name` | string | **是** | - | 要创建/更新的 Secret 名称 |
| `namespace` | string | **是** | - | Secret 的命名空间 |
| `clientIdKey` | string | 否 | `CF_ACCESS_CLIENT_ID` | 客户端 ID 的密钥 |
| `clientSecretKey` | string | 否 | `CF_ACCESS_CLIENT_SECRET` | 客户端密钥的密钥 |

## 状态

| 字段 | 类型 | 描述 |
|------|------|------|
| `tokenId` | string | Cloudflare 服务令牌 ID |
| `clientId` | string | 服务令牌客户端 ID |
| `accountId` | string | Cloudflare 账户 ID |
| `expiresAt` | string | 令牌过期时间 |
| `createdAt` | string | 创建时间 |
| `updatedAt` | string | 最后更新时间 |
| `lastSeenAt` | string | 最后使用时间 |
| `conditions` | []metav1.Condition | 最新观察 |

## 示例

### 示例 1：基本服务令牌

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

### 示例 2：具有自定义密钥的服务令牌

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

### 示例 3：CI/CD 集成

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
# 在 GitHub Actions 中的使用
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
    # 使用凭证对受保护的服务进行身份验证
```

## 前置条件

- Cloudflare Zero Trust 订阅
- 有效的 Cloudflare API 凭证
- 将创建 Secret 的 Kubernetes 命名空间
- 令牌可以用来进行身份验证的受保护的 Access 应用程序

## 限制

- 创建后无法检索服务令牌
- 凭证以纯文本格式存储在 Kubernetes Secret 中
- 每个 AccessServiceToken 资源只能有一个令牌
- 令牌元数据是只读的
- 创建后无法更新令牌（必须删除并重新创建）

## 相关资源

- [AccessApplication](accessapplication.md) - 此令牌可以访问的应用程序
- [AccessPolicy](accesspolicy.md) - 控制令牌访问的策略
- [CloudflareCredentials](cloudflarecredentials.md) - 操作员的 API 凭证

## 另请参阅

- [Cloudflare 服务令牌](https://developers.cloudflare.com/cloudflare-one/identity/service-tokens/)
- [机器对机器身份验证](https://developers.cloudflare.com/cloudflare-one/identity/)
- [Kubernetes Secrets](https://kubernetes.io/docs/concepts/configuration/secret/)
