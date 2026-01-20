# TunnelIngressClassConfig

TunnelIngressClassConfig 是一个集群作用域的资源，用于配置 Kubernetes Ingress 与 Cloudflare Tunnel 的集成。

## 概述

TunnelIngressClassConfig 在使用 Kubernetes Ingress 资源与 Cloudflare Tunnel 时启用自动 DNS 管理。

### 主要特性

- Ingress 集成
- 自动 DNS 管理
- DNS TXT 记录管理

## 规范

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `cloudflare` | CloudflareDetails | **是** | API 凭证 |

## 示例

### 示例 1：Ingress 配置

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: TunnelIngressClassConfig
metadata:
  name: tunnel-ingress-config
spec:
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## 另请参阅

- [Kubernetes Ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/)
