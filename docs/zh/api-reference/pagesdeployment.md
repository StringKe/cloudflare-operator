# PagesDeployment

PagesDeployment 是一个命名空间作用域的资源，用于管理具有直接上传和智能回滚的 Cloudflare Pages 项目部署。

## 概述

PagesDeployment 使您能够从 Kubernetes 直接将应用程序部署到 Cloudflare Pages。它支持构建工件上传、自动回滚到先前版本以及与 CI/CD 管道的集成。

### 主要特性

- 直接上传工件到 Pages
- 错误时自动回滚
- 构建清单支持
- 多个部署策略
- 状态跟踪和历史记录

## 规范

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `projectRef` | ProjectRef | **是** | PagesProject 的引用 |
| `source` | DeploymentSource | **是** | 部署源 |
| `rollbackOnError` | bool | 否 | 失败时自动回滚 |
| `cloudflare` | CloudflareDetails | **是** | API 凭证 |

## 示例

### 示例 1：部署构建工件

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: app-deployment
  namespace: production
spec:
  projectRef:
    name: my-app
  source:
    path: "/build"
    format: "service-bindings"
  rollbackOnError: true
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## 前置条件

- PagesProject 资源已创建
- 构建工件已准备
- 有效的 API 凭证

## 相关资源

- [PagesProject](pagesproject.md) - 项目
- [PagesDomain](pagesdomain.md) - 自定义域名

## 另请参阅

- [Cloudflare Pages 部署](https://developers.cloudflare.com/pages/deployments/)
