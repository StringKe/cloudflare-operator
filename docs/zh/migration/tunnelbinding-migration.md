# TunnelBinding 迁移指南

本指南说明如何从已废弃的 `TunnelBinding` 资源迁移到推荐的替代方案：使用 `TunnelIngressClassConfig` 的 Kubernetes Ingress 或 Gateway API。

## 为什么要迁移？

`TunnelBinding` 已废弃，原因如下：

1. **非标准 API**：TunnelBinding 使用自定义 API，无法与 Kubernetes 生态系统工具集成
2. **功能有限**：缺少高级路由、TLS 配置和中间件支持
3. **统一架构**：Operator 正在迁移到统一的同步架构，Ingress 和 Gateway API 提供标准接口
4. **更好的可观测性**：Ingress 和 Gateway API 资源得到监控工具的更好支持

## 迁移选项

| 功能 | TunnelBinding | Ingress | Gateway API |
|------|--------------|---------|-------------|
| HTTP/HTTPS 路由 | 是 | 是 | 是 |
| TCP/UDP 服务 | 有限 | 否 | 是 (TCPRoute/UDPRoute) |
| 基于路径的路由 | 基本正则 | 是 | 是 |
| 基于 Header 的路由 | 否 | 有限 | 是 |
| 多集群 | 否 | 否 | 是 |
| DNS 管理 | 自动 | 可配置 | 可配置 |
| Access 集成 | 手动 | 注解 | 注解 |

### 推荐的迁移路径

- **HTTP/HTTPS 服务**：使用带有 `TunnelIngressClassConfig` 的 **Ingress**
- **TCP/UDP 服务**：使用带有 `TunnelGatewayClassConfig` 的 **Gateway API**
- **复杂路由**：使用带有 HTTPRoute/TCPRoute/UDPRoute 的 **Gateway API**

## 前提条件

1. Operator 版本 0.20.0 或更高
2. 已安装 `TunnelIngressClassConfig` CRD
3. 如使用 Gateway API：已安装 Gateway API CRD

## 自动迁移工具

我们提供了一个迁移脚本来帮助转换 TunnelBinding 资源：

```bash
# 下载迁移脚本
curl -O https://raw.githubusercontent.com/StringKe/cloudflare-operator/main/scripts/migrate-tunnelbinding.sh
chmod +x migrate-tunnelbinding.sh

# 运行迁移（默认为 dry-run 模式）
./migrate-tunnelbinding.sh <namespace> <output-directory>

# 示例
./migrate-tunnelbinding.sh default ./migration-output
```

脚本生成：
- 用于隧道配置的 `TunnelIngressClassConfig`
- 用于 Kubernetes 集成的 `IngressClass`
- 每个 TunnelBinding subject 对应的 `Ingress` 资源

## 手动迁移步骤

### 步骤 1：识别 TunnelBinding 资源

```bash
# 列出所有 TunnelBinding
kubectl get tunnelbinding -A

# 导出特定的 TunnelBinding
kubectl get tunnelbinding <name> -n <namespace> -o yaml
```

### 步骤 2：创建 TunnelIngressClassConfig

创建引用你的隧道的 `TunnelIngressClassConfig`：

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: TunnelIngressClassConfig
metadata:
  name: my-tunnel-ingress
spec:
  tunnelRef:
    kind: ClusterTunnel  # 或 Tunnel
    name: my-tunnel
  dnsManagement: Automatic  # Automatic, DNSRecord 或 Manual
  dnsProxied: true
```

### 步骤 3：创建 IngressClass

```yaml
apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata:
  name: my-tunnel-ingress
spec:
  controller: cloudflare-operator.io/tunnel-ingress-controller
  parameters:
    apiGroup: networking.cloudflare-operator.io
    kind: TunnelIngressClassConfig
    name: my-tunnel-ingress
```

### 步骤 4：将 TunnelBinding Subjects 转换为 Ingress

**之前 (TunnelBinding)：**

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha1
kind: TunnelBinding
metadata:
  name: my-binding
  namespace: default
subjects:
  - kind: Service
    name: my-service
    spec:
      fqdn: app.example.com
      protocol: https
      noTlsVerify: true
      path: /api/.*
tunnelRef:
  kind: ClusterTunnel
  name: my-tunnel
```

**之后 (Ingress)：**

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-service-ingress
  namespace: default
  annotations:
    cloudflare-operator.io/origin-request-protocol: "https"
    cloudflare-operator.io/origin-request-no-tls-verify: "true"
spec:
  ingressClassName: my-tunnel-ingress
  rules:
  - host: app.example.com
    http:
      paths:
      - path: /api
        pathType: Prefix
        backend:
          service:
            name: my-service
            port:
              number: 443
```

### 步骤 5：验证并清理

1. **验证 Ingress 正常工作：**
   ```bash
   kubectl get ingress -n <namespace>
   kubectl describe ingress <name> -n <namespace>
   ```

2. **检查 DNS 记录：**
   ```bash
   kubectl get dnsrecord -n <namespace>
   ```

3. **删除 TunnelBinding：**
   ```bash
   # 警告：这将删除 TunnelBinding 管理的 DNS 记录
   kubectl delete tunnelbinding <name> -n <namespace>
   ```

## Origin Request 配置映射

下表将 TunnelBinding spec 字段映射到 Ingress 注解：

| TunnelBinding 字段 | Ingress 注解 |
|-------------------|-------------|
| `spec.protocol` | `cloudflare-operator.io/origin-request-protocol` |
| `spec.noTlsVerify` | `cloudflare-operator.io/origin-request-no-tls-verify` |
| `spec.http2Origin` | `cloudflare-operator.io/origin-request-http2-origin` |
| `spec.caPool` | `cloudflare-operator.io/origin-request-ca-pool` |
| `spec.proxyAddress` | `cloudflare-operator.io/origin-request-proxy-address` |
| `spec.proxyPort` | `cloudflare-operator.io/origin-request-proxy-port` |
| `spec.proxyType` | `cloudflare-operator.io/origin-request-proxy-type` |

## Gateway API 迁移

对于 TCP/UDP 服务或高级路由，使用 Gateway API：

### 步骤 1：创建 TunnelGatewayClassConfig

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: TunnelGatewayClassConfig
metadata:
  name: my-tunnel-gateway
spec:
  tunnelRef:
    kind: ClusterTunnel
    name: my-tunnel
  dnsManagement: Automatic
```

### 步骤 2：创建 GatewayClass 和 Gateway

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: cloudflare-tunnel
spec:
  controllerName: cloudflare-operator.io/tunnel-gateway-controller
  parametersRef:
    group: networking.cloudflare-operator.io
    kind: TunnelGatewayClassConfig
    name: my-tunnel-gateway
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: my-gateway
  namespace: default
spec:
  gatewayClassName: cloudflare-tunnel
  listeners:
  - name: http
    protocol: HTTP
    port: 80
```

### 步骤 3：创建 Route

**HTTPRoute：**

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-http-route
  namespace: default
spec:
  parentRefs:
  - name: my-gateway
  hostnames:
  - "app.example.com"
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /api
    backendRefs:
    - name: my-service
      port: 80
```

**TCPRoute（用于 TCP 服务）：**

```yaml
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: TCPRoute
metadata:
  name: my-tcp-route
  namespace: default
spec:
  parentRefs:
  - name: my-gateway
  rules:
  - backendRefs:
    - name: my-tcp-service
      port: 5432
```

## 故障排除

### DNS 记录未创建

1. 检查 `TunnelIngressClassConfig` DNS 设置：
   ```bash
   kubectl describe tunnelingressclassconfig <name>
   ```

2. 验证隧道/凭证中的 Zone ID：
   ```bash
   kubectl get secret <credentials> -n <namespace> -o yaml
   ```

### 服务无法访问

1. 检查隧道配置：
   ```bash
   kubectl get cloudflaresyncstate -A | grep tunnel
   ```

2. 验证 Ingress 状态：
   ```bash
   kubectl describe ingress <name> -n <namespace>
   ```

### Origin 连接错误

1. 验证 origin request 注解是否正确
2. 检查服务是否在集群内可访问：
   ```bash
   kubectl run test --rm -it --image=curlimages/curl -- curl http://<service>.<namespace>.svc
   ```

## 常见问题

**问：我可以同时运行 TunnelBinding 和 Ingress 吗？**
答：可以，但要避免在两者中配置相同的主机名以防止冲突。

**问：删除 TunnelBinding 会影响我的 DNS 记录吗？**
答：会的，TunnelBinding 会创建 DNS TXT 记录来进行所有权追踪。删除它会移除这些记录。先创建 Ingress 资源以确保连续性。

**问：如何在不停机的情况下迁移？**
答：
1. 为相同的主机名创建 Ingress 资源
2. 等待 DNS 传播并验证访问
3. 删除 TunnelBinding

**问：Access Applications 怎么办？**
答：Access Applications 与 TunnelBinding 和 Ingress 都能配合使用。Access 配置不需要修改。
