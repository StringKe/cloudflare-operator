# TunnelGatewayClassConfig

TunnelGatewayClassConfig 是一个集群作用域的资源，用于配置 Kubernetes Gateway API 与 Cloudflare Tunnel 的集成。

## 概述

TunnelGatewayClassConfig 在使用 Kubernetes Gateway API 资源与 Cloudflare Tunnel 时启用自动 DNS 管理。

### 主要特性

- Gateway API 集成
- 自动 DNS 管理
- 现代网络 API

## 规范

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `cloudflare` | CloudflareDetails | **是** | API 凭证 |

## 示例

### 示例 1：Gateway 配置

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: TunnelGatewayClassConfig
metadata:
  name: tunnel-gateway-config
spec:
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## 另请参阅

- [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/)
