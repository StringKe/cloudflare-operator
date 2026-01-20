# R2BucketDomain

R2BucketDomain 是一个命名空间作用域的资源，用于为 Cloudflare R2 桶配置自定义域名。

## 概述

R2BucketDomain 允许您通过自定义域名使用 Cloudflare 的 CDN 和安全功能来提供 R2 桶内容。配置 CNAME 或完整域名委托以通过您自己的域名提供 R2 对象。

### 主要特性

- R2 桶的自定义域名配置
- CDN 集成以加快内容交付
- Cloudflare 边缘的 SSL/TLS 终止
- 访问日志和安全选项

## 规范

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `domain` | string | **是** | 桶的自定义域名 |
| `bucketRef` | BucketRef | **是** | R2Bucket 资源的引用 |
| `cloudflare` | CloudflareDetails | **是** | Cloudflare API 凭证 |

## 示例

### 示例 1：R2 桶的自定义域名

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

## 前置条件

- 已创建 R2Bucket 资源
- Cloudflare DNS 管理的域名
- 有效的 API 凭证

## 相关资源

- [R2Bucket](r2bucket.md) - 存储桶
- [R2BucketNotification](r2bucketnotification.md) - 事件通知

## 另请参阅

- [Cloudflare R2 自定义域名](https://developers.cloudflare.com/r2/data-access/domain-setup/)
