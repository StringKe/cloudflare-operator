# Core Service 统一同步架构设计

## 1. 问题背景

### 1.1 当前架构的问题

当前 Operator 中存在**多个 Controller 直接调用 Cloudflare API** 的问题：

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          当前架构（有竞态问题）                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Tunnel Controller ──────┐                                                  │
│  TunnelBinding Ctrl ─────┼──► 直接调用 Cloudflare API ──► 互相覆盖配置       │
│  Ingress Controller ─────┤                                                  │
│  Gateway Controller ─────┘                                                  │
│                                                                             │
│  问题：                                                                      │
│  1. 竞态条件：多个 Controller 同时更新同一个 Cloudflare 资源                  │
│  2. 配置覆盖：后执行的 Controller 覆盖先执行的配置                            │
│  3. 多实例部署时问题更严重                                                    │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 1.2 具体案例：Tunnel Configuration 竞态

```
时间线：
T0: Tunnel Controller 同步 (warp-routing: true, ingress: [catch-all])
T1: Ingress Controller 同步 (warp-routing: false, ingress: [app.example.com])
     → 完全覆盖 T0 的配置，warp-routing 被重置为 false
```

## 2. 解决方案：统一同步架构

### 2.1 架构概览

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          统一同步架构                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    K8S Resources (用户创建)                          │   │
│  │  Tunnel │ TunnelBinding │ Ingress │ HTTPRoute │ DNSRecord │ ...    │   │
│  └────────────────────────────┬────────────────────────────────────────┘   │
│                               │                                            │
│                               ▼                                            │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    Controllers (各自的)                              │   │
│  │  - 监听自己负责的 K8S 资源                                            │   │
│  │  - 构建配置片段                                                       │   │
│  │  - 调用 Core Service 注册配置                                         │   │
│  │  - ❌ 不再直接调用 Cloudflare API                                     │   │
│  └────────────────────────────┬────────────────────────────────────────┘   │
│                               │                                            │
│                               ▼                                            │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    Core Service                                      │   │
│  │                                                                      │   │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐      │   │
│  │  │ TunnelConfig    │  │ DNSConfig       │  │ AccessConfig    │      │   │
│  │  │ (聚合 ingress)   │  │ (聚合 DNS)      │  │ (聚合 Access)   │      │   │
│  │  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘      │   │
│  │           │                    │                    │               │   │
│  │           └────────────────────┼────────────────────┘               │   │
│  │                                │                                    │   │
│  │                    ┌───────────▼───────────┐                        │   │
│  │                    │     Sync Manager      │                        │   │
│  │                    │  - 防抖动             │                        │   │
│  │                    │  - 批量处理           │                        │   │
│  │                    │  - 错误重试           │                        │   │
│  │                    └───────────┬───────────┘                        │   │
│  └────────────────────────────────┼────────────────────────────────────┘   │
│                                   │                                        │
│                                   ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    Cloudflare API                                    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 核心组件

#### 2.2.1 CloudflareSyncState CRD

内部 CRD，用于存储聚合后的配置状态，支持多实例部署：

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: CloudflareSyncState
metadata:
  name: tunnel-<tunnel-id>
  labels:
    cloudflare-operator.io/resource-type: TunnelConfiguration
spec:
  # Cloudflare 资源标识
  resourceType: TunnelConfiguration
  cloudflareId: "<tunnel-id>"

  # 来自各个 Controller 的配置片段
  sources:
    - ref:
        kind: ClusterTunnel
        name: my-tunnel
      config:
        warpRouting: true
        fallbackTarget: "http_status:404"

    - ref:
        kind: Ingress
        namespace: default
        name: my-app
      config:
        rules:
          - hostname: app.example.com
            service: http://my-app-svc:80

    - ref:
        kind: TunnelBinding
        namespace: default
        name: my-binding
      config:
        rules:
          - hostname: api.example.com
            service: http://my-api-svc:8080

status:
  # 聚合后的完整配置
  aggregatedConfig:
    ingress:
      - hostname: app.example.com
        service: http://my-app-svc:80
      - hostname: api.example.com
        service: http://my-api-svc:8080
      - service: http_status:404  # catch-all
    warpRouting:
      enabled: true

  # 同步状态
  lastSyncTime: "2026-01-16T14:00:00Z"
  syncStatus: Synced
  configHash: "sha256:abc123..."

  conditions:
    - type: Ready
      status: "True"
      reason: Synced
      message: "Configuration synced to Cloudflare"
```

#### 2.2.2 Core Service 接口

```go
// internal/service/core/interface.go

package core

import (
    "context"
)

// SyncService 是统一同步服务的接口
type SyncService interface {
    // RegisterConfig 注册配置片段
    // - resourceType: 资源类型（如 "TunnelConfiguration"）
    // - cloudflareId: Cloudflare 资源 ID（如 tunnel ID）
    // - source: 配置来源（如 {Kind: "Ingress", Namespace: "default", Name: "my-app"}）
    // - config: 配置内容
    RegisterConfig(ctx context.Context, resourceType string, cloudflareId string, source Source, config interface{}) error

    // UnregisterConfig 注销配置片段（资源删除时）
    UnregisterConfig(ctx context.Context, resourceType string, cloudflareId string, source Source) error

    // TriggerSync 触发同步（可选，也可以自动同步）
    TriggerSync(ctx context.Context, resourceType string, cloudflareId string) error
}

// Source 标识配置来源
type Source struct {
    Kind      string `json:"kind"`
    Namespace string `json:"namespace,omitempty"`
    Name      string `json:"name"`
}

// TunnelConfigService 是 Tunnel 配置的专用服务
type TunnelConfigService interface {
    // SetWarpRouting 设置 warp-routing（来自 Tunnel Controller）
    SetWarpRouting(ctx context.Context, tunnelId string, enabled bool) error

    // SetFallbackTarget 设置 fallback target
    SetFallbackTarget(ctx context.Context, tunnelId string, target string) error

    // RegisterIngressRules 注册 ingress 规则
    RegisterIngressRules(ctx context.Context, tunnelId string, source Source, rules []IngressRule) error

    // UnregisterIngressRules 注销 ingress 规则
    UnregisterIngressRules(ctx context.Context, tunnelId string, source Source) error
}

// IngressRule 表示一条 ingress 规则
type IngressRule struct {
    Hostname      string            `json:"hostname,omitempty"`
    Path          string            `json:"path,omitempty"`
    Service       string            `json:"service"`
    OriginRequest *OriginRequest    `json:"originRequest,omitempty"`
}
```

#### 2.2.3 SyncState Controller

```go
// internal/controller/syncstate/controller.go

// SyncStateReconciler 负责将聚合后的配置同步到 Cloudflare
type SyncStateReconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder

    // Cloudflare API 客户端工厂
    CFClientFactory cf.ClientFactory
}

func (r *SyncStateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. 获取 SyncState 资源
    syncState := &v1alpha2.CloudflareSyncState{}
    if err := r.Get(ctx, req.NamespacedName, syncState); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. 聚合所有来源的配置
    aggregatedConfig := r.aggregateConfig(syncState)

    // 3. 检查是否需要同步（配置是否变化）
    configHash := r.computeHash(aggregatedConfig)
    if configHash == syncState.Status.ConfigHash {
        return ctrl.Result{}, nil  // 配置未变化，无需同步
    }

    // 4. 同步到 Cloudflare
    if err := r.syncToCloudflare(ctx, syncState, aggregatedConfig); err != nil {
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    // 5. 更新状态
    syncState.Status.AggregatedConfig = aggregatedConfig
    syncState.Status.ConfigHash = configHash
    syncState.Status.LastSyncTime = metav1.Now()
    syncState.Status.SyncStatus = "Synced"

    return ctrl.Result{}, r.Status().Update(ctx, syncState)
}
```

### 2.3 Controller 改造

#### 2.3.1 Tunnel Controller

```go
// 修改后的 Tunnel Controller
func (r *TunnelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ... 创建/管理 Tunnel 的逻辑保持不变 ...

    // 不再直接调用 Cloudflare API 同步配置
    // 而是通过 Core Service 注册配置

    // 设置 warp-routing
    if err := r.TunnelConfigService.SetWarpRouting(ctx, tunnelID, spec.EnableWarpRouting); err != nil {
        return ctrl.Result{}, err
    }

    // 设置 fallback target
    if err := r.TunnelConfigService.SetFallbackTarget(ctx, tunnelID, spec.FallbackTarget); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}
```

#### 2.3.2 Ingress Controller

```go
// 修改后的 Ingress Controller
func (r *IngressReconciler) reconcileIngress(ctx context.Context, ingress *networkingv1.Ingress) error {
    // 构建 ingress 规则
    rules := r.buildIngressRules(ingress)

    // 不再直接调用 Cloudflare API
    // 而是注册到 Core Service
    source := core.Source{
        Kind:      "Ingress",
        Namespace: ingress.Namespace,
        Name:      ingress.Name,
    }

    return r.TunnelConfigService.RegisterIngressRules(ctx, tunnelID, source, rules)
}

func (r *IngressReconciler) handleDeletion(ctx context.Context, ingress *networkingv1.Ingress) error {
    source := core.Source{
        Kind:      "Ingress",
        Namespace: ingress.Namespace,
        Name:      ingress.Name,
    }

    return r.TunnelConfigService.UnregisterIngressRules(ctx, tunnelID, source)
}
```

### 2.4 多实例支持

#### 2.4.1 状态持久化

- 所有配置状态存储在 CloudflareSyncState CRD 中
- 不依赖内存状态，Operator 重启后可恢复

#### 2.4.2 并发控制

- 使用 K8s resourceVersion 进行乐观锁控制
- 并发更新时自动重试

#### 2.4.3 Leader Election

- controller-runtime 已支持 Leader Election
- 只有 Leader 的 SyncState Controller 会同步到 Cloudflare

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          多实例部署                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐             │
│  │ Operator Pod 1  │  │ Operator Pod 2  │  │ Operator Pod 3  │             │
│  │ (Leader)        │  │ (Standby)       │  │ (Standby)       │             │
│  │                 │  │                 │  │                 │             │
│  │ ┌─────────────┐ │  │ ┌─────────────┐ │  │ ┌─────────────┐ │             │
│  │ │ Controllers │ │  │ │ Controllers │ │  │ │ Controllers │ │             │
│  │ │ (all)       │ │  │ │ (all)       │ │  │ │ (all)       │ │             │
│  │ └──────┬──────┘ │  │ └──────┬──────┘ │  │ └──────┬──────┘ │             │
│  │        │        │  │        │        │  │        │        │             │
│  │        ▼        │  │        ▼        │  │        ▼        │             │
│  │ ┌─────────────┐ │  │ ┌─────────────┐ │  │ ┌─────────────┐ │             │
│  │ │Core Service │ │  │ │Core Service │ │  │ │Core Service │ │             │
│  │ │ (active)    │ │  │ │ (active)    │ │  │ │ (active)    │ │             │
│  │ └──────┬──────┘ │  │ └──────┬──────┘ │  │ └──────┬──────┘ │             │
│  └────────┼────────┘  └────────┼────────┘  └────────┼────────┘             │
│           │                    │                    │                      │
│           └────────────────────┼────────────────────┘                      │
│                                │                                           │
│                                ▼                                           │
│           ┌─────────────────────────────────────────┐                      │
│           │        CloudflareSyncState CRD          │                      │
│           │      (K8s 乐观锁处理并发更新)             │                      │
│           └─────────────────────┬───────────────────┘                      │
│                                 │                                          │
│                                 ▼                                          │
│           ┌─────────────────────────────────────────┐                      │
│           │      SyncState Controller (Leader)      │                      │
│           │        只有 Leader 同步到 CF             │                      │
│           └─────────────────────┬───────────────────┘                      │
│                                 │                                          │
│                                 ▼                                          │
│           ┌─────────────────────────────────────────┐                      │
│           │            Cloudflare API               │                      │
│           └─────────────────────────────────────────┘                      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.5 适用范围

| 资源类型 | 是否需要聚合 | SyncState 类型 |
|----------|-------------|----------------|
| Tunnel Configuration | ✅ 是 | `TunnelConfiguration` |
| DNS Records | ❌ 否（1:1） | `DNSRecord` |
| Access Application | ❌ 否（1:1） | `AccessApplication` |
| Network Route | ❌ 否（1:1） | `NetworkRoute` |
| Gateway Rules | 视情况 | `GatewayRules` |

对于 1:1 映射的资源，仍然通过 Core Service 同步，但不需要聚合逻辑。

## 3. 文件结构

```
internal/
├── service/                        # Core Service 层
│   ├── core/
│   │   ├── interface.go            # 通用接口定义
│   │   ├── service.go              # 通用实现
│   │   └── types.go                # 类型定义
│   │
│   ├── tunnel/                     # Tunnel 配置服务
│   │   ├── service.go              # TunnelConfigService 实现
│   │   └── aggregator.go           # 规则聚合逻辑
│   │
│   ├── dns/                        # DNS 配置服务
│   │   └── service.go
│   │
│   └── access/                     # Access 配置服务
│       └── service.go
│
├── controller/
│   ├── syncstate/                  # SyncState Controller
│   │   ├── controller.go           # 同步到 Cloudflare
│   │   ├── tunnel_sync.go          # Tunnel 配置同步逻辑
│   │   ├── dns_sync.go             # DNS 同步逻辑
│   │   └── access_sync.go          # Access 同步逻辑
│   │
│   ├── tunnel_controller.go        # 修改：使用 Service
│   ├── clustertunnel_controller.go # 修改：使用 Service
│   ├── tunnelbinding_controller.go # 修改：使用 Service
│   ├── ingress/controller.go       # 修改：使用 Service
│   ├── gateway/gateway_controller.go # 修改：使用 Service
│   └── ...
│
└── api/v1alpha2/
    └── cloudflaresyncstate_types.go  # SyncState CRD 定义
```

## 4. 实施计划

### Phase 1: 基础架构
1. 定义 CloudflareSyncState CRD
2. 实现 Core Service 接口
3. 实现 SyncState Controller

### Phase 2: Tunnel 配置迁移
1. 实现 TunnelConfigService
2. 修改 Tunnel/ClusterTunnel Controller
3. 修改 TunnelBinding Controller
4. 修改 Ingress Controller
5. 修改 Gateway Controller

### Phase 3: 其他资源迁移
1. 迁移 DNS 相关资源
2. 迁移 Access 相关资源
3. 迁移其他资源

### Phase 4: 测试和文档
1. 单元测试
2. 集成测试
3. E2E 测试
4. 更新文档

## 5. 兼容性考虑

- 保持 API 向后兼容
- 现有 CRD 不变
- CloudflareSyncState 是内部资源，用户无需感知
