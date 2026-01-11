# Istio 服务网格集成

本指南介绍如何将 Cloudflare Operator 与 Istio 服务网格集成，包括 mTLS 配置、常见问题和最佳实践。

## 概述

在启用了 Istio 的 Kubernetes 集群中运行 Cloudflare Tunnel (cloudflared) 时，可能会因 Istio 的自动 mTLS（双向 TLS）功能而遇到 TLS 相关问题。本指南解释了架构原理并提供了无缝集成的解决方案。

## 架构

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           Kubernetes 集群                               │
│                                                                         │
│  ┌─────────────┐     ┌──────────────────┐     ┌───────────────────┐   │
│  │ cloudflared │────▶│  Envoy Sidecar   │────▶│    后端服务       │   │
│  │    Pod      │     │  (istio-proxy)   │     │    Sidecar        │   │
│  └─────────────┘     └──────────────────┘     └───────────────────┘   │
│         │                    │                         │               │
│         │                    │ mTLS (自动)             │               │
│         ▼                    ▼                         ▼               │
│    HTTP 请求            TLS 升级                  TLS 终止             │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### Istio 自动 mTLS 工作原理

1. **源 Pod 有 sidecar**：出站流量被 Envoy 拦截
2. **目标 Pod 检测**：Istio 检查目标 Pod 是否有 sidecar
3. **协议决策**：
   - 目标有 sidecar → 使用 mTLS
   - 目标无 sidecar → 使用明文

## 常见问题：excludeInboundPorts 冲突

### 问题描述

当某个端口被配置在 `excludeInboundPorts` 中（例如用于 Prometheus 抓取），但目标 Pod 仍然有 sidecar 时：

```yaml
# 目标 Pod 注解
annotations:
  traffic.istio.io/excludeInboundPorts: "9091"
```

**会发生什么：**
1. Istio Gateway 检测到目标 Pod 有 sidecar → 发送 mTLS
2. 9091 端口绕过 sidecar → 期望接收明文
3. 结果：`TLS handshake failure` 或 `WRONG_VERSION_NUMBER`

### 流量流向图

```
┌──────────────────────────────────────────────────────────────────────────┐
│                                                                          │
│  cloudflared ──▶ sidecar ──▶ mTLS ──▶ target:9091 ──▶ 失败!             │
│       │              │                     │                             │
│       │              │                     └─ excludeInboundPorts        │
│       │              │                        (期望明文)                 │
│       │              │                                                   │
│       │              └─ 检测到目标有 sidecar                            │
│       │                 → 升级为 mTLS                                   │
│       │                                                                  │
│       └─ 发送 HTTP 请求                                                 │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

## 解决方案

### 方案 1：DestinationRule（推荐）

创建 DestinationRule 禁用特定端口的 mTLS：

```yaml
apiVersion: networking.istio.io/v1
kind: DestinationRule
metadata:
  name: backend-no-mtls-9091
  namespace: app
spec:
  host: my-backend.app.svc.cluster.local
  trafficPolicy:
    portLevelSettings:
    - port:
        number: 9091
      tls:
        mode: DISABLE  # 禁用此端口的 mTLS
```

这告诉 Istio："连接 9091 端口时，不要使用 mTLS。"

### 方案 2：PeerAuthentication 例外

在 PeerAuthentication 中添加端口级别例外：

```yaml
apiVersion: security.istio.io/v1
kind: PeerAuthentication
metadata:
  name: backend-9091-permissive
  namespace: app
spec:
  selector:
    matchLabels:
      app: my-backend
  mtls:
    mode: STRICT  # 其他端口默认
  portLevelMtls:
    9091:
      mode: DISABLE  # 9091 端口允许明文
```

### 方案 3：从 excludeInboundPorts 移除端口

如果该端口可以接受 mTLS，则将其从 `excludeInboundPorts` 移除：

```yaml
# 之前：端口被排除
annotations:
  traffic.istio.io/excludeInboundPorts: "9091,15090"

# 之后：仅排除 metrics 端口
annotations:
  traffic.istio.io/excludeInboundPorts: "15090"
```

这样 Istio 将在 9091 端口上端到端处理 mTLS。

### 方案 4：为 cloudflared 注入 Sidecar（替代方案）

如果 cloudflared 没有 sidecar，它会发送明文。你可以：

```yaml
# cloudflared Deployment
spec:
  template:
    metadata:
      annotations:
        sidecar.istio.io/inject: "true"
        # 确保 sidecar 在 cloudflared 之前启动
        proxy.istio.io/config: '{ "holdApplicationUntilProxyStarts": true }'
```

注入 sidecar 后：
- cloudflared 发送 HTTP 到本地 sidecar
- sidecar 自动升级为 mTLS
- 目标 sidecar 终止 mTLS

**注意：** 如果目标端口使用 `excludeInboundPorts`，这个方案无效。

## 完整示例

### 场景：9091 端口上的 Spring Boot Admin

Spring Boot 应用通常在单独的端口（9091）上暴露管理端点，该端口可能被排除在 Istio 之外以便 Prometheus 抓取。

#### 步骤 1：创建 DestinationRule

```yaml
apiVersion: networking.istio.io/v1
kind: DestinationRule
metadata:
  name: gateway-actuator-no-mtls
  namespace: app
spec:
  host: sg-cloud-gateway.app.svc.cluster.local
  trafficPolicy:
    portLevelSettings:
    - port:
        number: 9091
      tls:
        mode: DISABLE
```

#### 步骤 2：创建带协议注解的 Ingress

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: spring-boot-admin
  namespace: app
  annotations:
    # 明确指定 9091 端口使用 HTTP 协议
    cloudflare.com/protocol-9091: http
spec:
  ingressClassName: cloudflare-tunnel
  rules:
  - host: admin.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: sg-cloud-gateway
            port:
              number: 9091
```

#### 步骤 3：配置 TunnelIngressClassConfig（可选）

在 IngressClass 级别设置默认协议：

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: TunnelIngressClassConfig
metadata:
  name: cloudflare-tunnel-config
  namespace: cloudflare-operator-system
spec:
  tunnelRef:
    kind: ClusterTunnel
    name: main-tunnel
  defaultProtocol: http  # 所有后端默认使用 HTTP
  defaultOriginRequest:
    connectTimeout: "30s"
    keepAliveTimeout: "90s"
```

## 协议检测优先级

Cloudflare Operator 使用 7 级优先级系统进行协议检测：

| 优先级 | 来源 | 示例 |
|--------|------|------|
| 1 | Ingress 注解：`cloudflare.com/protocol` | `https` |
| 2 | Ingress 注解：`cloudflare.com/protocol-{port}` | `cloudflare.com/protocol-9091: http` |
| 3 | Service 注解：`cloudflare.com/protocol` | `http` |
| 4 | Service 端口 `appProtocol` 字段 | `kubernetes.io/h2c` |
| 5 | Service 端口名称 | `http`、`https`、`grpc` |
| 6 | TunnelIngressClassConfig `defaultProtocol` | `http` |
| 7 | 端口号推断 | `443` → `https`，其他 → `http` |

## 故障排查

### 检查 cloudflared 是否有 Sidecar

```bash
kubectl get pod -l app=<tunnel-name> -o jsonpath='{.items[0].spec.containers[*].name}'
# 如果输出包含 "istio-proxy"，说明有 sidecar
```

### 检查生成的 Tunnel 配置

```bash
kubectl get cm <tunnel-name> -n <namespace> -o yaml | grep -A30 "ingress:"
# 验证服务 URL 使用正确的协议（http:// vs https://）
```

### 检查 Istio mTLS 状态

```bash
# 检查 PeerAuthentication
kubectl get peerauthentication -A

# 检查 DestinationRule
kubectl get destinationrule -A

# 检查是否正在使用 mTLS（从源 pod）
istioctl x describe pod <source-pod>
```

### 常见错误信息

| 错误 | 原因 | 解决方案 |
|------|------|----------|
| `WRONG_VERSION_NUMBER` | mTLS 到明文端口 | 添加 DestinationRule，设置 `tls.mode: DISABLE` |
| `upstream_reset_before_response_started` | 连接被终止 | 检查 excludeInboundPorts 和 DestinationRule |
| `TLS handshake failure` | 协议不匹配 | 使用 `cloudflare.com/protocol-{port}` 注解 |
| `503 UC` | 上游连接失败 | 验证服务存在且端口正确 |

### 使用 istioctl 调试

```bash
# 分析配置
istioctl analyze -n <namespace>

# 检查代理配置
istioctl proxy-config cluster <pod-name> -n <namespace>

# 检查监听器配置
istioctl proxy-config listener <pod-name> -n <namespace>
```

## 最佳实践

### 1. 端口配置保持一致

确保 `excludeInboundPorts` 和 DestinationRule 配置一致：

```yaml
# 如果你排除了入站端口...
traffic.istio.io/excludeInboundPorts: "9091"

# ...也要禁用到该端口的出站 mTLS
trafficPolicy:
  portLevelSettings:
  - port:
      number: 9091
    tls:
      mode: DISABLE
```

### 2. 使用显式协议注解

不要依赖推断；明确指定协议：

```yaml
annotations:
  cloudflare.com/protocol-8080: http    # 主服务
  cloudflare.com/protocol-9091: http    # Actuator
  cloudflare.com/protocol-443: https    # 外部 HTTPS
```

### 3. 关注点分离

- **Istio**：管理服务网格 mTLS
- **Cloudflare Operator**：管理隧道入口规则
- **保持配置独立但对齐**

### 4. 监控和告警

为 TLS 相关错误设置监控：

```yaml
# Prometheus 告警示例
- alert: CloudflareTunnelTLSErrors
  expr: rate(cloudflared_tunnel_request_errors_total{error=~".*tls.*"}[5m]) > 0
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: Cloudflare Tunnel 检测到 TLS 错误
```

## 参考资料

- [Istio mTLS 迁移指南](https://istio.io/latest/docs/tasks/security/authentication/mtls-migration/)
- [Istio DestinationRule 参考](https://istio.io/latest/docs/reference/config/networking/destination-rule/)
- [Istio PeerAuthentication 参考](https://istio.io/latest/docs/reference/config/security/peer_authentication/)
- [Cloudflare Tunnel Kubernetes 指南](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/deploy-tunnels/deployment-guides/kubernetes/)
