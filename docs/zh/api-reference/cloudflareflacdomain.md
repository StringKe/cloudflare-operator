# CloudflareDomain

CloudflareDomain 是一个集群作用域的资源，用于管理 Cloudflare 中的综合域名配置。

## 概述

CloudflareDomain 配置区域设置，包括 SSL/TLS、缓存、安全、WAF 和其他域级功能。

### 主要特性

- 区域配置
- SSL/TLS 设置
- 缓存策略
- 安全选项
- WAF 规则

## 规范

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `domain` | string | **是** | 域名 |
| `settings` | DomainSettings | 否 | 域名设置 |
| `cloudflare` | CloudflareDetails | **是** | API 凭证 |

## 示例

### 示例 1：域名配置

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: CloudflareDomain
metadata:
  name: example-domain
spec:
  domain: "example.com"
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## 另请参阅

- [Cloudflare 区域](https://developers.cloudflare.com/dns/)
