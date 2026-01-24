# 资源冲突解决方案

## 概述

本文档定义了 Cloudflare Operator 中多个 K8s 资源管理同一 Cloudflare 设置时的冲突检测、预防和解决机制。

## 资源分类

### 1:1 资源 (直接同步)

大部分资源是 1:1 映射关系，一个 K8s CRD 对应一个 Cloudflare 资源。这些资源直接调用 Cloudflare API，无需聚合。

| CRD | Cloudflare 资源 | 冲突处理 |
|-----|-----------------|----------|
| DNSRecord | DNS Record | 名称唯一 |
| AccessApplication | Access Application | ID 唯一 |
| AccessGroup | Access Group | ID 唯一 |
| VirtualNetwork | Virtual Network | 名称唯一 |
| NetworkRoute | Network Route | 网络段唯一 |
| R2Bucket | R2 Bucket | 名称唯一 |
| GatewayRule | Gateway Rule | ID 唯一 |
| ... | ... | ... |

**冲突预防**: 通过所有权标记 (Comment 字段) 检测资源是否被其他来源管理。

### 聚合资源 (ConfigMap)

Tunnel 配置需要聚合多个来源，使用 ConfigMap 存储配置片段。

| 来源 | 贡献内容 |
|------|----------|
| Tunnel/ClusterTunnel | warpRouting, fallback, originRequest |
| Ingress | hostname → service 规则 |
| TunnelBinding | 额外路由规则 |
| HTTPRoute | Gateway API 路由 |

---

## 聚合模式架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Tunnel 配置聚合架构                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐     │
│  │   Tunnel    │   │   Ingress   │   │TunnelBinding│   │  HTTPRoute  │     │
│  │  (CRD)      │   │  (CRD)      │   │   (CRD)     │   │   (CRD)     │     │
│  └──────┬──────┘   └──────┬──────┘   └──────┬──────┘   └──────┬──────┘     │
│         │                 │                 │                 │             │
│         ▼                 ▼                 ▼                 ▼             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    各自的 Controller                                 │   │
│  │  1. 解析 Spec                                                       │   │
│  │  2. 构建配置片段                                                     │   │
│  │  3. 写入 ConfigMap (RetryOnConflict)                                │   │
│  └──────────────────────────────┬──────────────────────────────────────┘   │
│                                 │                                           │
│                                 ▼                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    ConfigMap (聚合存储)                              │   │
│  │  ┌─────────────────────────────────────────────────────────────┐    │   │
│  │  │ metadata:                                                    │    │   │
│  │  │   name: tunnel-config-{tunnelID}                             │    │   │
│  │  │   ownerReferences: Tunnel/ClusterTunnel                      │    │   │
│  │  │ data:                                                        │    │   │
│  │  │   config.json:                                               │    │   │
│  │  │     sources:                                                 │    │   │
│  │  │       "ClusterTunnel/my-tunnel": {priority: 10, ...}         │    │   │
│  │  │       "Ingress/default/app": {priority: 100, ...}            │    │   │
│  │  └─────────────────────────────────────────────────────────────┘    │   │
│  └──────────────────────────────┬──────────────────────────────────────┘   │
│                                 │                                           │
│                                 ▼                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    TunnelConfig Controller                          │   │
│  │  1. Watch ConfigMap 变化                                            │   │
│  │  2. 聚合所有 sources (按优先级)                                      │   │
│  │  3. 计算 Hash 检测变化                                               │   │
│  │  4. 调用 Cloudflare Tunnel API                                      │   │
│  └──────────────────────────────┬──────────────────────────────────────┘   │
│                                 │                                           │
│                                 ▼                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    Cloudflare Tunnel Configuration API              │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 优先级机制

### 优先级定义

```go
const (
    PriorityTunnelSettings = 10   // Tunnel/ClusterTunnel 设置 (最高)
    PriorityBinding        = 50   // TunnelBinding
    PriorityIngress        = 100  // Ingress
    PriorityGateway        = 100  // Gateway API
)
```

### 优先级规则

1. **数字越小优先级越高**
2. **高优先级的 warpRouting/fallback 设置生效**
3. **规则按优先级排序后聚合**
4. **同优先级规则按 hostname 排序**

---

## 所有权标记

### 1:1 资源所有权

使用 Cloudflare 资源的 `comment` 或 `description` 字段标记所有权：

```
格式: [managed-by:Kind/Namespace/Name]
示例: [managed-by:DNSRecord/default/my-record]
```

### 实现

```go
// internal/controller/management.go

// NewManagementInfo 创建管理信息
func NewManagementInfo(obj client.Object, kind string) ManagementInfo

// BuildManagedComment 构建带管理标记的注释
func BuildManagedComment(info ManagementInfo, userComment string) string

// GetConflictingManager 检测冲突的管理者
func GetConflictingManager(comment string, info ManagementInfo) *ManagementInfo
```

### 冲突检测示例

```go
// 创建/更新前检测冲突
existing, _ := apiClient.GetDNSRecord(recordID)
mgmtInfo := controller.NewManagementInfo(obj, "DNSRecord")

if conflict := controller.GetConflictingManager(existing.Comment, mgmtInfo); conflict != nil {
    return fmt.Errorf("record managed by %s/%s/%s",
        conflict.Kind, conflict.Namespace, conflict.Name)
}
```

---

## ConfigMap 并发控制

### 写入冲突处理

使用 Kubernetes 的乐观锁机制处理并发写入：

```go
// internal/controller/tunnelconfig/writer.go

func (w *Writer) WriteSourceConfig(ctx context.Context, tunnelID, sourceKey string, config *SourceConfig) error {
    return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
        // 1. 读取当前 ConfigMap
        configMap, err := w.getOrCreateConfigMap(ctx, tunnelID)
        if err != nil {
            return err
        }

        // 2. 更新 source 配置
        tunnelConfig, _ := ParseTunnelConfig(configMap)
        tunnelConfig.Sources[sourceKey] = config

        // 3. 写回 ConfigMap (可能因 resourceVersion 冲突失败)
        return w.updateConfigMap(ctx, configMap, tunnelConfig)
    })
}
```

### 删除时清理

```go
func (w *Writer) RemoveSourceConfig(ctx context.Context, tunnelID, sourceKey string) error {
    return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
        configMap, err := w.getConfigMap(ctx, tunnelID)
        if err != nil {
            return client.IgnoreNotFound(err)
        }

        tunnelConfig, _ := ParseTunnelConfig(configMap)
        delete(tunnelConfig.Sources, sourceKey)

        if len(tunnelConfig.Sources) == 0 {
            // ConfigMap 会通过 OwnerReference 自动删除
            return nil
        }

        return w.updateConfigMap(ctx, configMap, tunnelConfig)
    })
}
```

---

## 测试验证场景

### 场景 1: 多个 Ingress 配置同一 Tunnel

```yaml
# Ingress A
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app-a
spec:
  ingressClassName: cloudflare
  rules:
  - host: app-a.example.com
    http:
      paths:
      - path: /
        backend:
          service:
            name: app-a
            port:
              number: 80

# Ingress B
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app-b
spec:
  ingressClassName: cloudflare
  rules:
  - host: app-b.example.com
    http:
      paths:
      - path: /
        backend:
          service:
            name: app-b
            port:
              number: 80
```

**预期行为**:
1. 两个 Ingress 的规则都写入同一 ConfigMap
2. TunnelConfig Controller 聚合后同步到 Cloudflare
3. 删除 Ingress A 后，Ingress B 的规则仍然存在

### 场景 2: Tunnel 设置覆盖 Ingress

```yaml
# ClusterTunnel 启用 WARP Routing
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: ClusterTunnel
metadata:
  name: main-tunnel
spec:
  enableWarpRouting: true

# Ingress 配置规则
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web-app
spec:
  ingressClassName: cloudflare
  rules:
  - host: web.example.com
    # ...
```

**预期行为**:
1. ClusterTunnel 的 warpRouting 设置优先级 (10) 高于 Ingress (100)
2. 最终配置包含 `warpRouting: true` 和 Ingress 规则
3. 删除 ClusterTunnel 后 warpRouting 设置消失，但 Ingress 规则保留

### 场景 3: DNSRecord 冲突检测

```yaml
# DNSRecord A 管理 api.example.com
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: DNSRecord
metadata:
  name: api-record
  namespace: default
spec:
  name: api.example.com
  type: CNAME
  content: tunnel.cfargotunnel.com
```

**预期行为**:
1. 创建时在 Cloudflare 记录 comment 中添加 `[managed-by:DNSRecord/default/api-record]`
2. 如果存在未被此资源管理的同名记录，报错冲突
3. 删除 CRD 时删除 Cloudflare 记录

---

## 相关代码

| 文件 | 说明 |
|------|------|
| `internal/controller/tunnelconfig/writer.go` | ConfigMap 读写工具 |
| `internal/controller/tunnelconfig/types.go` | 配置类型和优先级 |
| `internal/controller/tunnelconfig/controller.go` | 聚合同步控制器 |
| `internal/controller/management.go` | 所有权标记工具 |
