# DomainRegistration

DomainRegistration 是一个集群作用域的资源，用于使用 Cloudflare 管理域名注册（仅限企业）。

## 概述

DomainRegistration 使企业客户能够通过 Cloudflare 进行域名注册和管理。

### 主要特性

- 域名注册
- 域名管理
- 企业功能

## 规范

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `domain` | string | **是** | 要注册的域名 |
| `cloudflare` | CloudflareDetails | **是** | API 凭证 |

## 示例

### 示例 1：注册域名

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: DomainRegistration
metadata:
  name: new-domain
spec:
  domain: "example.com"
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## 另请参阅

- [Cloudflare 注册商](https://developers.cloudflare.com/registrar/)
