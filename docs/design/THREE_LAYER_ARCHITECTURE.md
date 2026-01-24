# 三层同步架构设计

## 文档信息

| 项目 | 内容 |
|------|------|
| 版本 | v2.0 |
| 状态 | ✅ 已实现 |
| 作者 | Cloudflare Operator Team |
| 日期 | 2026-01-24 (v0.34.0+) |

## 概述

本文档描述 Cloudflare Operator 的新三层同步架构，该架构从旧的六层 SyncState 架构简化而来。

## 架构演进

### 旧架构 (六层 SyncState)

```
L1 CRD → L2 Controller → L3 Service → L4 SyncState → L5 Sync Controller → L6 CF API
```

**问题**：
- 29 个 Sync Controller 共享 SyncState Informer，事件互相干扰
- 状态回写需要多层传递，延迟高
- Spec.Sources + Status 并发写入同一对象导致冲突
- 代码复杂，数据流难以追踪

### 新架构 (三层)

```
L1 CRD → L2 Controller → L3 CF API
```

**优势**：
- 每个 CRD 独立 Controller + Informer
- 状态直接写回 CRD.Status
- 单层写入，无竞争
- 代码简洁，数据流清晰

---

## 架构图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           三层同步架构                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 1: K8s CRD                                                      ║ │
│  ║                                                                       ║ │
│  ║  ┌───────────────────────────────────────────────────────────────┐   ║ │
│  ║  │ 1:1 资源                                                       │   ║ │
│  ║  │ DNSRecord, AccessApplication, R2Bucket, PagesDeployment...    │   ║ │
│  ║  └───────────────────────────────────────────────────────────────┘   ║ │
│  ║                                                                       ║ │
│  ║  ┌───────────────────────────────────────────────────────────────┐   ║ │
│  ║  │ 聚合资源                                                       │   ║ │
│  ║  │ Tunnel, ClusterTunnel, Ingress, TunnelBinding, HTTPRoute      │   ║ │
│  ║  └───────────────────────────────────────────────────────────────┘   ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    ▼                                        │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 2: Controllers                                                  ║ │
│  ║                                                                       ║ │
│  ║  ┌─────────────────────────────┐ ┌─────────────────────────────────┐ ║ │
│  ║  │ 1:1 Controllers             │ │ TunnelConfig Controller          │ ║ │
│  ║  │                             │ │                                  │ ║ │
│  ║  │ • 直接调用 CF API           │ │ • 监听 ConfigMap 变化            │ ║ │
│  ║  │ • 直接写回 CRD.Status       │ │ • 聚合多 source 配置             │ ║ │
│  ║  │ • 独立 Informer            │ │ • 单次 API 调用                  │ ║ │
│  ║  └─────────────────────────────┘ └─────────────────────────────────┘ ║ │
│  ║                                                                       ║ │
│  ║  ┌─────────────────────────────────────────────────────────────────┐ ║ │
│  ║  │ 聚合资源 Controllers (写入 ConfigMap)                            │ ║ │
│  ║  │                                                                  │ ║ │
│  ║  │ • Ingress Controller       → ConfigMap                           │ ║ │
│  ║  │ • TunnelBinding Controller → ConfigMap                           │ ║ │
│  ║  │ • Gateway Controller       → ConfigMap                           │ ║ │
│  ║  │ • Tunnel/ClusterTunnel     → ConfigMap                           │ ║ │
│  ║  └─────────────────────────────────────────────────────────────────┘ ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    ▼                                        │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 3: Cloudflare API Client                                        ║ │
│  ║                                                                       ║ │
│  ║  • 连接池管理                                                         ║ │
│  ║  • 速率限制 (Token Bucket)                                            ║ │
│  ║  • 自动重试 (指数退避)                                                 ║ │
│  ║  • 错误分类和包装                                                      ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 资源处理策略

### 1:1 资源 (直接同步)

一个 K8s 资源对应一个 Cloudflare 资源，Controller 直接调用 API。

| CRD | Cloudflare 资源 |
|-----|-----------------|
| DNSRecord | DNS Record |
| AccessApplication | Access Application |
| AccessGroup | Access Group |
| AccessPolicy | Access Policy |
| AccessServiceToken | Access Service Token |
| AccessIdentityProvider | Access Identity Provider |
| VirtualNetwork | Virtual Network |
| NetworkRoute | Network Route |
| PrivateService | Private Service |
| R2Bucket | R2 Bucket |
| R2BucketDomain | R2 Custom Domain |
| R2BucketNotification | R2 Event Notification |
| PagesProject | Pages Project |
| PagesDomain | Pages Custom Domain |
| PagesDeployment | Pages Deployment |
| GatewayRule | Gateway Rule |
| GatewayList | Gateway List |
| GatewayConfiguration | Gateway Configuration |
| DevicePostureRule | Device Posture Rule |
| DeviceSettingsPolicy | Device Settings Policy |
| ZoneRuleset | Zone Ruleset |
| TransformRule | Transform Rule |
| RedirectRule | Redirect Rule |
| OriginCACertificate | Origin CA Certificate |
| CloudflareDomain | Zone |
| DomainRegistration | Domain Registration |
| WARPConnector | WARP Connector |

**数据流**：

```
DNSRecord CRD → DNSRecord Controller → CF DNS API → DNSRecord.Status
                       ↑_______________________________↓
                              直接回写
```

### 聚合资源 (ConfigMap)

多个 K8s 资源对应一个 Cloudflare Tunnel Configuration。

| CRD | 贡献内容 |
|-----|----------|
| Tunnel/ClusterTunnel | warpRouting, fallback, originRequest |
| Ingress | hostname → service 规则 |
| TunnelBinding | 额外路由规则 |
| HTTPRoute | Gateway API 路由 |

**数据流**：

```
Tunnel CRD ─────────┐
Ingress CRD ────────┼──► ConfigMap ──► TunnelConfig Controller ──► CF Tunnel API
TunnelBinding CRD ──┤
HTTPRoute CRD ──────┘
```

---

## ConfigMap 聚合方案

### ConfigMap 结构

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tunnel-config-{tunnelID}
  namespace: cloudflare-operator-system
  labels:
    cloudflare-operator.io/tunnel-id: {tunnelID}
    cloudflare-operator.io/type: tunnel-config
  ownerReferences:
    - apiVersion: networking.cloudflare-operator.io/v1alpha2
      kind: ClusterTunnel  # 或 Tunnel
      name: {tunnelName}
      uid: {tunnelUID}
data:
  config.json: |
    {
      "tunnelId": "{tunnelID}",
      "accountId": "{accountID}",
      "credentialsRef": {
        "name": "cloudflare-credentials",
        "namespace": "cloudflare-operator-system"
      },
      "lastHash": "sha256:...",
      "sources": {
        "ClusterTunnel/{tunnelName}": {
          "settings": {
            "warpRouting": true,
            "fallbackTarget": "http_status:404"
          },
          "priority": 10
        },
        "Ingress/default/web-app": {
          "rules": [
            {
              "hostname": "app.example.com",
              "service": "http://web-app.default.svc:80"
            }
          ],
          "priority": 100
        }
      }
    }
```

### 优先级

| 来源 | 优先级 | 说明 |
|------|--------|------|
| Tunnel/ClusterTunnel | 10 | 最高，全局设置 |
| TunnelBinding | 50 | 显式绑定 |
| Ingress | 100 | 标准 Kubernetes Ingress |
| HTTPRoute | 100 | Gateway API |

### 聚合逻辑

1. **warpRouting**: 取优先级最高的设置
2. **fallbackTarget**: 取优先级最高的设置
3. **rules**: 按优先级排序，同优先级按 hostname 排序

---

## TunnelConfig Controller

### 职责

1. 监听 ConfigMap 变化（按 label 过滤）
2. 解析 sources，聚合配置
3. 计算 Hash，检测变化
4. 调用 Cloudflare Tunnel Configuration API
5. 更新 ConfigMap 状态

### 代码位置

```
internal/controller/tunnelconfig/
├── controller.go   # 主 Reconciler
├── writer.go       # ConfigMap 读写工具
└── types.go        # 配置类型定义
```

### 关键实现

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. 获取 ConfigMap
    configMap := &corev1.ConfigMap{}
    if err := r.Get(ctx, req.NamespacedName, configMap); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. 解析配置
    config, err := ParseTunnelConfig(configMap)
    if err != nil {
        return ctrl.Result{}, err
    }

    // 3. 检测变化 (Hash 比较)
    newHash := ComputeConfigHash(config)
    if config.LastHash == newHash {
        return ctrl.Result{}, nil
    }

    // 4. 聚合规则
    tunnelConfig := r.aggregateConfig(config)

    // 5. 同步到 Cloudflare
    if err := r.syncToCloudflare(ctx, config.TunnelID, tunnelConfig); err != nil {
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    // 6. 更新 Hash
    config.LastHash = newHash
    return ctrl.Result{}, r.updateConfigMap(ctx, configMap, config)
}
```

---

## 异步操作 (Tunnel 生命周期)

Tunnel 的创建和删除是异步操作，仍使用 SyncState + Lifecycle Controller。

### 为什么保留 SyncState？

1. Tunnel 创建后需要等待 Cloudflare 分配 ID
2. 删除需要先清理所有相关资源
3. 这些操作需要状态机管理

### 数据流

```
Tunnel CRD → Lifecycle Service → SyncState CRD → Lifecycle Controller → CF Tunnel API
                                                        ↓
                                              Tunnel CRD.Status (回写)
```

---

## 代码结构

```
internal/
├── controller/
│   ├── {resource}/              # 1:1 资源 Controller
│   │   └── controller.go        # 直接调用 CF API
│   │
│   ├── tunnelconfig/            # Tunnel 配置聚合
│   │   ├── controller.go        # 监听 ConfigMap
│   │   ├── writer.go            # ConfigMap 读写
│   │   └── types.go             # 类型定义
│   │
│   ├── ingress/                 # 写入 ConfigMap
│   │   ├── controller.go
│   │   └── dns.go               # DNS 自动管理
│   │
│   └── gateway/                 # 写入 ConfigMap
│       └── gateway_controller.go
│
├── sync/tunnel/                 # Tunnel 生命周期
│   └── lifecycle_controller.go
│
├── service/tunnel/              # Tunnel 生命周期服务
│   └── lifecycle_service.go
│
└── clients/cf/                  # Cloudflare API Client
```

---

## 迁移说明

### 从六层架构迁移

1. **删除的目录**：
   - `internal/service/dns/` - DNS 服务层
   - `internal/sync/dns/` - DNS 同步控制器

2. **保留的组件**：
   - `internal/sync/tunnel/lifecycle_controller.go` - Tunnel 生命周期
   - `internal/service/tunnel/lifecycle_service.go` - Tunnel 生命周期服务

3. **新增的组件**：
   - `internal/controller/tunnelconfig/` - ConfigMap 聚合

### 兼容性

- 用户 CRD 完全兼容，无需修改
- ConfigMap 是内部资源，用户无需感知
- SyncState 仅用于 Tunnel 生命周期

---

## 验收标准

### 功能验收

- [x] 1:1 资源直接同步
- [x] Tunnel 配置聚合
- [x] DNS 自动管理 (使用 DNSRecord CRD)
- [x] 状态直接回写到 CRD.Status
- [x] ConfigMap OwnerReference 自动清理

### 性能验收

- [x] 轮询稳定 (RequeueAfter 不被干扰)
- [x] 无 "object has been modified" 错误
- [x] API 调用数量不增加

### 代码质量

- [x] 删除冗余中间层
- [x] 数据流清晰可追踪
- [x] 每个 CRD 独立 Controller
