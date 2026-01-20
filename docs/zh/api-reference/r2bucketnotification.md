# R2BucketNotification

R2BucketNotification 是一个命名空间作用域的资源，用于为 Cloudflare R2 桶配置事件通知。

## 概述

R2BucketNotification 使您能够在对象上传、删除或修改时接收 R2 桶的通知。通知可以发送到事件平台，如 AWS SQS、Azure Queue Storage 或 JSON 端点。

### 主要特性

- 对象生命周期事件通知
- 支持多种目标类型
- 按对象前缀过滤事件
- 交付失败时自动重试

## 规范

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `bucketRef` | BucketRef | **是** | R2Bucket 的引用 |
| `destination` | NotificationDestination | **是** | 事件目标 |
| `events` | []string | **是** | 要通知的事件 |
| `cloudflare` | CloudflareDetails | **是** | Cloudflare API 凭证 |

## 示例

### 示例 1：SQS 通知

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: R2BucketNotification
metadata:
  name: bucket-sqs-notifications
  namespace: production
spec:
  bucketRef:
    name: app-storage
  destination:
    queue:
      queueName: "r2-events"
      queueRegion: "us-east-1"
  events:
    - "object-created"
    - "object-deleted"
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## 前置条件

- R2Bucket 资源存在
- 目标服务已配置
- 有效凭证

## 相关资源

- [R2Bucket](r2bucket.md) - 存储桶
- [R2BucketDomain](r2bucketdomain.md) - 自定义域名

## 另请参阅

- [Cloudflare R2 通知](https://developers.cloudflare.com/r2/notifications/)
