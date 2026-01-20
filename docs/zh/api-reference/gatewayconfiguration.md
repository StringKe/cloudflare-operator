# Ugatewayconfiguratio

gatewayconfiguration 是集群作用域的资源（或命名空间作用域）。

## 概述

此资源管理 Cloudflare 中的相应功能。

### 主要特性

- 功能管理
- 配置控制
- 规则应用

## 规范

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
|  | string | **是** | 资源名称 |
|  | CloudflareDetails | **是** | API 凭证 |

## 示例

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: Ugatewayconfiguration
metadata:
  name: example
spec:
  name: "Example"
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## 相关资源

- 参考相关文档

## 另请参阅

- Cloudflare 文档
