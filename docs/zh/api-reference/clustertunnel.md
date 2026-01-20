# ClusterTunnel

ClusterTunnel 是集群级资源，用于创建和管理可从任何命名空间访问的 Cloudflare Tunnel。它与 Tunnel 共享相同的功能，但在集群级别运行，可以在多个命名空间之间共享隧道。

## 概述

ClusterTunnel 将 Tunnel 的功能扩展到集群范围，允许多个命名空间使用同一个 Cloudflare Tunnel。这非常适合共享基础设施，单个隧道为不同命名空间中的应用程序提供服务。

### 主要特性

| 特性 | 描述 |
|------|------|
| **集群范围访问** | 可从任何命名空间的 Ingress/Gateway 资源访问 |
| **资源共享** | 单个隧道服务多个命名空间 |
| **集中管理** | 在一个地方管理隧道配置 |
| **高可用性** | 与命名空间级 Tunnel 相同的高可用能力 |
| **WARP 路由** | 支持通过 WARP 客户端访问私有网络 |

### 使用场景

- **多租户集群**: 在多个租户命名空间之间共享一个隧道
- **集中化基础设施**: 通过单个集群隧道管理所有外部访问
- **成本优化**: 减少大型集群中所需的隧道数量
- **共享私有网络**: 为整个集群启用 WARP 路由

## ClusterTunnel vs Tunnel

| 方面 | Tunnel | ClusterTunnel |
|------|--------|---------------|
| **作用域** | 命名空间级 | 集群级 |
| **Secret 位置** | 与 Tunnel 相同的命名空间 | `cloudflare-operator-system` 命名空间 |
| **Ingress 绑定** | 仅同一命名空间 | 集群中的任何命名空间 |
| **Gateway 绑定** | 仅同一命名空间 | 集群中的任何命名空间 |
| **使用场景** | 命名空间隔离 | 共享基础设施 |

## Spec

ClusterTunnel 使用与 [Tunnel](tunnel.md#spec) 相同的 spec。详细的字段描述请参见 Tunnel 文档。

### 主要字段

| 字段 | 类型 | 必需 | 默认值 | 描述 |
|------|------|------|--------|------|
| `newTunnel` | *NewTunnel | 否 | - | 创建新隧道（与 `existingTunnel` 互斥） |
| `existingTunnel` | *ExistingTunnel | 否 | - | 使用现有隧道（与 `newTunnel` 互斥） |
| `cloudflare` | CloudflareDetails | **是** | - | Cloudflare API 凭证和配置 |
| `enableWarpRouting` | bool | 否 | `false` | 启用 WARP 路由以实现私有网络访问 |
| `protocol` | string | 否 | `"auto"` | 隧道协议：`"auto"`、`"quic"` 或 `"http2"` |
| `fallbackTarget` | string | 否 | `"http_status:404"` | 无匹配入口规则时的默认响应 |
| `deployPatch` | string | 否 | `"{}"` | 用于自定义 cloudflared Deployment 的 JSON patch |

### 重要提示：Secret 位置

对于 ClusterTunnel，Cloudflare API 凭证 Secret **必须**位于 `cloudflare-operator-system` 命名空间（或 Operator 安装的命名空间）。

## Status

与 [Tunnel Status](tunnel.md#status) 相同。

| 字段 | 类型 | 描述 |
|------|------|------|
| `tunnelId` | string | Cloudflare 隧道 UUID |
| `tunnelName` | string | Cloudflare 隧道名称 |
| `accountId` | string | Cloudflare 账户 ID |
| `state` | string | 当前状态：`pending`、`creating`、`active`、`error`、`deleting` |
| `configVersion` | int | 当前隧道配置版本 |
| `conditions` | []Condition | 标准 Kubernetes 条件 |

## 示例

### 基础集群隧道

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: ClusterTunnel
metadata:
  name: shared-cluster-tunnel
spec:
  newTunnel:
    name: shared-k8s-tunnel

  cloudflare:
    accountId: "<your-account-id>"
    domain: example.com
    # Secret 必须在 cloudflare-operator-system 命名空间中
    secret: cloudflare-api-credentials
```

### 高可用集群隧道

具有 Pod 反亲和性的多副本：

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: ClusterTunnel
metadata:
  name: ha-cluster-tunnel
spec:
  newTunnel:
    name: ha-k8s-tunnel

  # 3 个副本，带 Pod 反亲和性
  deployPatch: |
    {
      "spec": {
        "replicas": 3,
        "template": {
          "spec": {
            "affinity": {
              "podAntiAffinity": {
                "preferredDuringSchedulingIgnoredDuringExecution": [
                  {
                    "weight": 100,
                    "podAffinityTerm": {
                      "labelSelector": {
                        "matchLabels": {
                          "app.kubernetes.io/name": "cloudflared"
                        }
                      },
                      "topologyKey": "kubernetes.io/hostname"
                    }
                  }
                ]
              }
            }
          }
        }
      }
    }

  cloudflare:
    accountId: "<your-account-id>"
    domain: example.com
    secret: cloudflare-api-credentials
```

### 启用 WARP 路由的集群隧道

用于集群范围的私有网络访问：

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: ClusterTunnel
metadata:
  name: private-network-tunnel
spec:
  newTunnel:
    name: cluster-private-tunnel

  # 为所有命名空间启用 WARP 路由
  enableWarpRouting: true

  cloudflare:
    accountId: "<your-account-id>"
    domain: example.com
    secret: cloudflare-api-credentials
```

### 从 Ingress 使用 ClusterTunnel（不同命名空间）

```yaml
# 集群级的 ClusterTunnel
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: ClusterTunnel
metadata:
  name: shared-tunnel
spec:
  newTunnel:
    name: shared-k8s-tunnel
  cloudflare:
    accountId: "<your-account-id>"
    domain: example.com
    secret: cloudflare-api-credentials
---
# app-namespace-1 中的 Ingress
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app1-ingress
  namespace: app-namespace-1
  annotations:
    # 引用 ClusterTunnel
    cf-operator.io/tunnel: "shared-tunnel"
spec:
  ingressClassName: cloudflare-tunnel
  rules:
    - host: app1.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: app1-service
                port:
                  number: 80
---
# app-namespace-2 中的 Ingress
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app2-ingress
  namespace: app-namespace-2
  annotations:
    # 引用同一个 ClusterTunnel
    cf-operator.io/tunnel: "shared-tunnel"
spec:
  ingressClassName: cloudflare-tunnel
  rules:
    - host: app2.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: app2-service
                port:
                  number: 80
```

### 使用 ClusterTunnel 与 Gateway API

```yaml
# ClusterTunnel
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: ClusterTunnel
metadata:
  name: gateway-tunnel
spec:
  newTunnel:
    name: gateway-k8s-tunnel
  cloudflare:
    accountId: "<your-account-id>"
    domain: example.com
    secret: cloudflare-api-credentials
---
# 任何命名空间中的 HTTPRoute
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: api-route
  namespace: api-namespace
  annotations:
    cf-operator.io/tunnel: "gateway-tunnel"
spec:
  parentRefs:
    - name: cloudflare-gateway
      kind: Gateway
  hostnames:
    - "api.example.com"
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: api-service
          port: 8080
```

## 前置条件

1. **集群管理员访问权限**: ClusterTunnel 是集群级的，需要管理员权限
2. **Cloudflare 账户**: 已启用 Zero Trust 的活跃 Cloudflare 账户
3. **Operator 命名空间中的 API 凭证**: Secret 必须存在于 `cloudflare-operator-system` 中

### 在 Operator 命名空间中创建凭证 Secret

```bash
kubectl create secret generic cloudflare-api-credentials \
  --from-literal=CLOUDFLARE_API_TOKEN="your-token-here" \
  -n cloudflare-operator-system
```

或通过 YAML：

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cloudflare-api-credentials
  namespace: cloudflare-operator-system
type: Opaque
stringData:
  CLOUDFLARE_API_TOKEN: "your-api-token-here"
```

## 限制

- **Secret 位置**: 凭证 Secret 必须在 Operator 的命名空间（`cloudflare-operator-system`）中
- **需要集群管理员**: 创建 ClusterTunnel 需要 cluster-admin 权限
- **单点配置**: 对 ClusterTunnel 的更改会影响所有使用它的命名空间
- **名称唯一性**: ClusterTunnel 名称在集群范围内必须唯一

## 安全考虑

- **命名空间隔离**: 虽然 ClusterTunnel 可从所有命名空间访问，但考虑使用 RBAC 控制哪些命名空间可以引用它
- **集中化凭证**: Operator 命名空间中的 API 凭证更加敏感 - 确保适当的访问控制
- **配置更改**: ClusterTunnel 配置的更新会影响所有依赖的 Ingress/Gateway 资源

## 相关资源

- [Tunnel](tunnel.md) - 用于单命名空间使用的命名空间级隧道
- [DNSRecord](dnsrecord.md) - 管理隧道端点的 DNS 记录
- [Ingress 集成](../guides/ingress-integration.md) - 将 Ingress 与 ClusterTunnel 配合使用
- [Gateway API 集成](../guides/gateway-api-integration.md) - 将 Gateway API 与 ClusterTunnel 配合使用
- [NetworkRoute](networkroute.md) - 通过隧道路由私有 IP 范围

## 另请参阅

- [示例](../../../examples/01-basic/tunnel/)
- [Cloudflare Tunnel 文档](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/)
