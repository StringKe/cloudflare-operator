# OriginCACertificate

OriginCACertificate 是一个命名空间作用域的资源，用于管理 Cloudflare Origin CA 证书以进行源站身份验证。

## 概述

OriginCACertificate 创建和管理 Cloudflare Origin CA 证书，证明流量确实来自 Cloudflare 的网络。

### 主要特性

- 源站身份验证
- 自动证书生成
- 证书存储在 Kubernetes Secret 中
- 证书轮换支持

## 规范

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `hostname` | string | **是** | 证书的主机名 |
| `secretRef` | SecretRef | **是** | 证书存储的 Secret |
| `cloudflare` | CloudflareDetails | **是** | API 凭证 |

## 示例

### 示例 1：Origin CA 证书

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: OriginCACertificate
metadata:
  name: origin-cert
  namespace: production
spec:
  hostname: "*.example.com"
  secretRef:
    name: origin-ca-cert
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## 另请参阅

- [Cloudflare Origin CA](https://developers.cloudflare.com/ssl/origin-configuration/origin-ca/)
