# PagesDomain

PagesDomain 是一个命名空间作用域的资源，用于为 Cloudflare Pages 项目配置自定义域名。

## 概述

PagesDomain 允许您通过自定义域名提供 Pages 项目。直接从 Kubernetes 配置 DNS 设置和 SSL/TLS 选项。

### 主要特性

- 自定义域名配置
- 自动 SSL 证书
- DNS 管理集成
- 特定于环境的域名
- 子域名支持

## 规范

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `domain` | string | **是** | 自定义域名 |
| `projectRef` | ProjectRef | **是** | PagesProject 的引用 |
| `environment` | string | 否 | 生产或预览 |
| `cloudflare` | CloudflareDetails | **是** | API 凭证 |

## 示例

### 示例 1：自定义域名

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDomain
metadata:
  name: app-domain
  namespace: production
spec:
  domain: "app.example.com"
  projectRef:
    name: my-app
  environment: "production"
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## 前置条件

- PagesProject 已创建
- Cloudflare 管理的域名
- 有效的 API 凭证

## 相关资源

- [PagesProject](pagesproject.md) - 项目
- [PagesDeployment](pagesdeployment.md) - 项目部署

## 另请参阅

- [Cloudflare Pages 自定义域名](https://developers.cloudflare.com/pages/platform/custom-domains/)
