# R2Bucket

R2Bucket 是一个命名空间作用域的资源，用于创建和管理具有生命周期规则和配置的 Cloudflare R2 对象存储桶。

## 概述

R2Bucket 从 Kubernetes 直接创建和管理 Cloudflare R2 对象存储桶。R2 是 Cloudflare 的 S3 兼容对象存储服务。操作员处理桶的创建、配置，并可以应用生命周期规则来自动管理对象保留和过期。

### 主要特性

| 特性 | 描述 |
|------|------|
| **S3 兼容** | 完整的 AWS S3 API 兼容性 |
| **生命周期管理** | 自动保留和过期规则 |
| **桶配置** | 从 Kubernetes 管理桶设置 |
| **访问控制** | 配置桶可见性和 CORS |
| **自动清理** | 资源删除期间删除桶数据 |

### 使用场景

- **对象存储**：使用 Kubernetes 原生的 R2 桶管理
- **数据存档**：为数据存档实现生命周期策略
- **静态资源**：存储带有 CDN 交付的静态内容
- **备份存储**：应用程序的集中备份存储
- **多租户**：为不同应用程序创建单独的桶

## 规范

### 主要字段

| 字段 | 类型 | 必需 | 默认值 | 描述 |
|------|------|------|--------|------|
| `bucketName` | string | 否 | 资源名称 | R2 桶的名称 |
| `lifecycleRules` | []LifecycleRule | 否 | - | 桶生命周期规则 |
| `cloudflare` | CloudflareDetails | **是** | - | Cloudflare API 凭证 |

### LifecycleRule

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `id` | string | **是** | 规则 ID（在桶内唯一） |
| `enabled` | bool | **是** | 启用/禁用规则 |
| `prefix` | string | 否 | 要匹配的对象键前缀 |
| `expiration` | *int | 否 | 对象过期前的天数 |
| `noncurrentVersionExpiration` | *int | 否 | 旧版本过期前的天数 |

## 状态

| 字段 | 类型 | 描述 |
|------|------|------|
| `bucketId` | string | Cloudflare R2 桶 ID |
| `bucketName` | string | Cloudflare 中的桶名称 |
| `accountId` | string | Cloudflare 账户 ID |
| `state` | string | 当前状态 |
| `endpoint` | string | R2 桶端点 URL |
| `conditions` | []metav1.Condition | 最新观察 |

## 示例

### 示例 1：基本 R2 桶

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: R2Bucket
metadata:
  name: app-storage
  namespace: production
spec:
  bucketName: "app-storage-prod"
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

### 示例 2：具有生命周期规则的桶

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: R2Bucket
metadata:
  name: logs-archive
  namespace: production
spec:
  bucketName: "logs-archive"
  lifecycleRules:
    - id: "delete-old-logs"
      enabled: true
      prefix: "logs/"
      expiration: 90
    - id: "archive-data"
      enabled: true
      prefix: "data/"
      expiration: 365
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

### 示例 3：多层存储

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: R2Bucket
metadata:
  name: backups
  namespace: production
spec:
  bucketName: "database-backups"
  lifecycleRules:
    - id: "daily-cleanup"
      enabled: true
      prefix: "daily/"
      expiration: 30
    - id: "monthly-retain"
      enabled: true
      prefix: "monthly/"
      expiration: 365
    - id: "yearly-archive"
      enabled: true
      prefix: "yearly/"
      expiration: 2555
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## 前置条件

- 启用了 R2 的 Cloudflare 账户
- 具有 R2 权限的有效 API 凭证
- 唯一的桶名称（在所有 R2 用户中全局唯一）

## 限制

- 桶名称必须在全局范围内唯一
- 桶删除会删除所有对象
- 生命周期规则异步应用
- 创建后无法重命名桶
- 暂不支持对象保留策略

## 相关资源

- [R2BucketDomain](r2bucketdomain.md) - 桶的自定义域名
- [R2BucketNotification](r2bucketnotification.md) - 事件通知
- [CloudflareCredentials](cloudflarecredentials.md) - API 凭证

## 另请参阅

- [Cloudflare R2 文档](https://developers.cloudflare.com/r2/)
- [S3 API 兼容性](https://developers.cloudflare.com/r2/data-access/s3-api/)
