# Cloudflare Operator 统一同步架构重构

## 文档信息

| 项目 | 内容 |
|------|------|
| 版本 | v1.0 |
| 状态 | ✅ 已实现 (Phase 1-5 完成，Resource Controller 迁移进行中) |
| 作者 | Cloudflare Operator Team |
| 日期 | 2026-01-16 |
| 更新 | 2026-01-17 |

## 实现进度

| Phase | 描述 | 状态 |
|-------|------|------|
| Phase 1 | CloudflareSyncState CRD 设计与实现 | ✅ 完成 |
| Phase 2 | BaseService 基础设施实现 | ✅ 完成 |
| Phase 3 | Sync Controller 通用工具 (Debouncer, Hash, Predicate) | ✅ 完成 |
| Phase 4 | 所有资源的 Core Service 实现 | ✅ 完成 |
| Phase 5 | 所有资源的 Sync Controller 实现 | ✅ 完成 |
| Phase 6 | Resource Controller 迁移 | ⚠️ 进行中 (20/25 待迁移) |
| Phase 7 | E2E 测试和文档 | ⚠️ 进行中 |

---

## 1. 背景与问题

### 1.1 当前架构

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        当前架构（存在严重问题）                           │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  每个 Controller 直接调用 Cloudflare API：                               │
│                                                                         │
│  ┌──────────────────┐     ┌──────────────────┐                         │
│  │ Tunnel Controller│────►│ Tunnel API       │                         │
│  └──────────────────┘     └──────────────────┘                         │
│  ┌──────────────────┐     ┌──────────────────┐                         │
│  │TunnelBinding Ctrl│────►│ Tunnel Config API│◄── 竞态！               │
│  └──────────────────┘     └──────────────────┘                         │
│  ┌──────────────────┐            ▲                                     │
│  │ Ingress Controller│───────────┘                                     │
│  └──────────────────┘                                                  │
│  ┌──────────────────┐            ▲                                     │
│  │ Gateway Controller│───────────┘                                     │
│  └──────────────────┘                                                  │
│  ┌──────────────────┐     ┌──────────────────┐                         │
│  │ DNSRecord Ctrl   │────►│ DNS API          │                         │
│  └──────────────────┘     └──────────────────┘                         │
│  ┌──────────────────┐     ┌──────────────────┐                         │
│  │ AccessApp Ctrl   │────►│ Access API       │                         │
│  └──────────────────┘     └──────────────────┘                         │
│  ... (30 个 Controller，每个 500-1000 行代码)                           │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 1.2 核心问题

| 问题 | 描述 | 影响 |
|------|------|------|
| **竞态条件** | 多个 Controller 同时更新同一 Cloudflare 资源 | 配置丢失、状态不一致 |
| **多实例不安全** | 每个 Pod 独立调用 API，无协调机制 | 配置覆盖、重复操作 |
| **代码臃肿** | 每个 Controller 500-1000 行，职责混杂 | 难以维护、难以测试 |
| **逻辑重复** | API 调用、错误处理、重试逻辑分散 | 代码重复、不一致 |
| **无防抖** | 每次资源变更都立即调用 API | API 限流、性能差 |

### 1.3 具体案例：Tunnel Configuration 竞态

```
时间线：
T0: ClusterTunnel 创建，Tunnel Controller 同步配置
    → warp-routing: true, fallback: http_status:404, ingress: []

T1: 用户创建 Ingress，Ingress Controller 同步配置
    → 读取当前配置，添加规则，写回
    → warp-routing: true, ingress: [app.example.com]

T2: 用户创建 TunnelBinding，TunnelBinding Controller 同步配置
    → 如果在 T1 完成前读取，会丢失 Ingress 的规则！
    → warp-routing: true, ingress: [api.example.com]  ← app.example.com 丢失！

T3: Tunnel Controller 检测到变化，重新同步
    → warp-routing: true, ingress: []  ← 所有规则丢失！
```

---

## 2. 目标架构

### 2.1 设计原则

1. **单一同步点**：只有 Sync Controller 调用 Cloudflare API
2. **CRD 共享状态**：使用 K8s CRD 作为分布式状态存储
3. **职责分离**：每个组件只做一件事
4. **多实例安全**：利用 K8s resourceVersion 乐观锁
5. **高效同步**：防抖、批量、增量

### 2.2 六层架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           统一六层架构                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 1: K8s Resources (用户创建和管理)                                ║ │
│  ║                                                                       ║ │
│  ║  Tunnel │ ClusterTunnel │ TunnelBinding │ Ingress │ HTTPRoute        ║ │
│  ║  DNSRecord │ AccessApplication │ AccessGroup │ VirtualNetwork        ║ │
│  ║  NetworkRoute │ R2Bucket │ ZoneRuleset │ ... (30 个 CRD)             ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    ▼                                        │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 2: Resource Controllers (轻量级，每个 100-150 行)                ║ │
│  ║                                                                       ║ │
│  ║  职责：                                                                ║ │
│  ║  ✓ Watch K8s 资源变化                                                 ║ │
│  ║  ✓ 验证 Spec 合法性                                                   ║ │
│  ║  ✓ 解析引用（credentialsRef, tunnelRef 等）                           ║ │
│  ║  ✓ 构建配置对象                                                       ║ │
│  ║  ✓ 调用 Core Service 注册/注销配置                                    ║ │
│  ║  ✗ 不直接调用 Cloudflare API                                          ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    ▼                                        │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 3: Core Services (业务逻辑层)                                    ║ │
│  ║                                                                       ║ │
│  ║  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐    ║ │
│  ║  │TunnelConfig │ │    DNS      │ │   Access    │ │   Network   │    ║ │
│  ║  │  Service    │ │  Service    │ │  Service    │ │  Service    │    ║ │
│  ║  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘    ║ │
│  ║  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐    ║ │
│  ║  │   Gateway   │ │     R2      │ │   Ruleset   │ │   Domain    │    ║ │
│  ║  │  Service    │ │  Service    │ │  Service    │ │  Service    │    ║ │
│  ║  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘    ║ │
│  ║                                                                       ║ │
│  ║  职责：                                                                ║ │
│  ║  ✓ 接收配置注册/注销请求                                              ║ │
│  ║  ✓ 验证业务规则                                                       ║ │
│  ║  ✓ 创建/更新 SyncState CRD                                            ║ │
│  ║  ✓ 处理资源间依赖关系                                                 ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    ▼                                        │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 4: SyncState CRD (共享状态存储)                                  ║ │
│  ║                                                                       ║ │
│  ║  CloudflareSyncState - 按 Cloudflare 资源类型和 ID 组织：              ║ │
│  ║                                                                       ║ │
│  ║  ┌─────────────────────────────────────────────────────────────────┐ ║ │
│  ║  │ resourceType: TunnelConfiguration                               │ ║ │
│  ║  │ cloudflareId: tunnel-abc123                                     │ ║ │
│  ║  │ sources:                                                        │ ║ │
│  ║  │   - kind: ClusterTunnel, name: my-tunnel                        │ ║ │
│  ║  │     config: {warpRouting: true, fallback: ...}                  │ ║ │
│  ║  │   - kind: Ingress, namespace: default, name: my-app             │ ║ │
│  ║  │     config: {rules: [...]}                                      │ ║ │
│  ║  │   - kind: TunnelBinding, namespace: default, name: my-binding   │ ║ │
│  ║  │     config: {rules: [...]}                                      │ ║ │
│  ║  └─────────────────────────────────────────────────────────────────┘ ║ │
│  ║  ┌─────────────────────────────────────────────────────────────────┐ ║ │
│  ║  │ resourceType: DNSRecord                                         │ ║ │
│  ║  │ cloudflareId: dns-record-xxx                                    │ ║ │
│  ║  │ sources:                                                        │ ║ │
│  ║  │   - kind: DNSRecord, namespace: default, name: my-record        │ ║ │
│  ║  │     config: {type: CNAME, name: app, content: ...}              │ ║ │
│  ║  └─────────────────────────────────────────────────────────────────┘ ║ │
│  ║                                                                       ║ │
│  ║  特点：                                                                ║ │
│  ║  ✓ K8s 原生存储（etcd）                                               ║ │
│  ║  ✓ resourceVersion 乐观锁                                             ║ │
│  ║  ✓ 多实例安全                                                         ║ │
│  ║  ✓ 状态持久化                                                         ║ │
│  ║  ✓ kubectl 可观测                                                     ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    ▼                                        │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 5: Sync Controllers (同步控制器)                                 ║ │
│  ║                                                                       ║ │
│  ║  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐    ║ │
│  ║  │TunnelConfig │ │    DNS      │ │   Access    │ │   Network   │    ║ │
│  ║  │    Sync     │ │    Sync     │ │    Sync     │ │    Sync     │    ║ │
│  ║  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘    ║ │
│  ║                                                                       ║ │
│  ║  职责：                                                                ║ │
│  ║  ✓ Watch SyncState CRD 变化                                           ║ │
│  ║  ✓ 聚合多个 sources 的配置（如需要）                                   ║ │
│  ║  ✓ 防抖（500ms 合并多次变更）                                         ║ │
│  ║  ✓ 增量检测（Hash 比较，无变化不同步）                                 ║ │
│  ║  ✓ 调用 Cloudflare API 同步                                           ║ │
│  ║  ✓ 更新 SyncState Status                                              ║ │
│  ║  ✓ 错误处理和重试                                                     ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    │ ✅ 唯一 API 调用点                     │
│                                    ▼                                        │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 6: Cloudflare API Client                                        ║ │
│  ║                                                                       ║ │
│  ║  统一的 API 客户端，包含：                                             ║ │
│  ║  ✓ 连接池管理                                                         ║ │
│  ║  ✓ 速率限制（Token Bucket）                                           ║ │
│  ║  ✓ 自动重试（指数退避）                                               ║ │
│  ║  ✓ 错误分类和包装                                                     ║ │
│  ║  ✓ 请求/响应日志                                                      ║ │
│  ║  ✓ Metrics 埋点                                                       ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    ▼                                        │
│  ┌───────────────────────────────────────────────────────────────────────┐ │
│  │                         Cloudflare API                                │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.3 数据流示例

#### 2.3.1 创建 Ingress 的数据流

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ 用户创建 Ingress                                                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 用户: kubectl apply -f ingress.yaml                                     │
│     ┌─────────────────────────────────────┐                                │
│     │ apiVersion: networking.k8s.io/v1    │                                │
│     │ kind: Ingress                       │                                │
│     │ metadata:                           │                                │
│     │   name: my-app                      │                                │
│     │   annotations:                      │                                │
│     │     ingressClassName: cloudflare    │                                │
│     │ spec:                               │                                │
│     │   rules:                            │                                │
│     │   - host: app.example.com           │                                │
│     │     http:                           │                                │
│     │       paths:                        │                                │
│     │       - path: /                     │                                │
│     │         backend:                    │                                │
│     │           service:                  │                                │
│     │             name: my-app-svc        │                                │
│     │             port: 80                │                                │
│     └─────────────────────────────────────┘                                │
│                          │                                                  │
│                          ▼                                                  │
│  2. Ingress Controller 收到事件                                             │
│     - 验证 IngressClass 是否为 cloudflare                                   │
│     - 解析 TunnelIngressClassConfig，获取 TunnelID                          │
│     - 构建 IngressRule 配置对象                                             │
│                          │                                                  │
│                          ▼                                                  │
│  3. 调用 TunnelConfigService.RegisterRules()                                │
│     ┌─────────────────────────────────────┐                                │
│     │ source: {                           │                                │
│     │   kind: "Ingress",                  │                                │
│     │   namespace: "default",             │                                │
│     │   name: "my-app"                    │                                │
│     │ }                                   │                                │
│     │ tunnelId: "abc123"                  │                                │
│     │ rules: [{                           │                                │
│     │   hostname: "app.example.com",      │                                │
│     │   path: "/",                        │                                │
│     │   service: "http://my-app-svc:80"   │                                │
│     │ }]                                  │                                │
│     └─────────────────────────────────────┘                                │
│                          │                                                  │
│                          ▼                                                  │
│  4. TunnelConfigService 创建/更新 SyncState CRD                             │
│     ┌─────────────────────────────────────┐                                │
│     │ apiVersion: ...cloudflare.../v1alpha2│                               │
│     │ kind: CloudflareSyncState           │                                │
│     │ metadata:                           │                                │
│     │   name: tunnel-abc123               │                                │
│     │ spec:                               │                                │
│     │   resourceType: TunnelConfiguration │                                │
│     │   cloudflareId: abc123              │                                │
│     │   sources:                          │                                │
│     │   - ref: {kind: Ingress, ...}       │                                │
│     │     config:                         │                                │
│     │       rules: [...]                  │                                │
│     │     lastUpdated: "2026-01-16T..."   │                                │
│     └─────────────────────────────────────┘                                │
│                          │                                                  │
│                          ▼                                                  │
│  5. TunnelConfigSyncController 收到 SyncState 变更事件                       │
│     - 防抖：等待 500ms，合并多次变更                                         │
│     - 聚合：收集所有 sources 的 rules                                       │
│     - Hash：计算配置 Hash，检测是否变化                                      │
│                          │                                                  │
│                          ▼                                                  │
│  6. 调用 Cloudflare API（单次调用）                                          │
│     PUT /accounts/{account_id}/cfd_tunnel/{tunnel_id}/configurations        │
│     ┌─────────────────────────────────────┐                                │
│     │ {                                   │                                │
│     │   "config": {                       │                                │
│     │     "warp-routing": {"enabled":true},│                               │
│     │     "ingress": [                    │                                │
│     │       {"hostname":"app.example.com",│                                │
│     │        "service":"http://..."},     │                                │
│     │       {"service":"http_status:404"} │                                │
│     │     ]                               │                                │
│     │   }                                 │                                │
│     │ }                                   │                                │
│     └─────────────────────────────────────┘                                │
│                          │                                                  │
│                          ▼                                                  │
│  7. 更新 SyncState Status                                                   │
│     ┌─────────────────────────────────────┐                                │
│     │ status:                             │                                │
│     │   syncStatus: Synced                │                                │
│     │   lastSyncTime: "2026-01-16T..."    │                                │
│     │   configVersion: 42                 │                                │
│     │   configHash: "sha256:..."          │                                │
│     └─────────────────────────────────────┘                                │
│                          │                                                  │
│                          ▼                                                  │
│  8. Ingress Controller 更新 Ingress Status（可选）                           │
│     - 从 SyncState 读取同步状态                                             │
│     - 更新 Ingress 的 Conditions                                            │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### 2.3.2 多实例部署数据流

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ 多实例安全：3 个 Operator Pod 同时运行                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐             │
│  │   Pod 1         │  │   Pod 2         │  │   Pod 3         │             │
│  │                 │  │                 │  │                 │             │
│  │ Resource Ctrl   │  │ Resource Ctrl   │  │ Resource Ctrl   │             │
│  │ (all active)    │  │ (all active)    │  │ (all active)    │             │
│  │       │         │  │       │         │  │       │         │             │
│  │       ▼         │  │       ▼         │  │       ▼         │             │
│  │ Core Service    │  │ Core Service    │  │ Core Service    │             │
│  │ (all active)    │  │ (all active)    │  │ (all active)    │             │
│  │       │         │  │       │         │  │       │         │             │
│  └───────┼─────────┘  └───────┼─────────┘  └───────┼─────────┘             │
│          │                    │                    │                        │
│          └────────────────────┼────────────────────┘                        │
│                               │                                             │
│                               ▼                                             │
│          ┌─────────────────────────────────────────┐                       │
│          │         SyncState CRD (K8s etcd)        │                       │
│          │                                         │                       │
│          │  并发写入通过 resourceVersion 控制：     │                       │
│          │  - Pod 1 更新 source A                  │                       │
│          │  - Pod 2 更新 source B                  │                       │
│          │  - 冲突时自动重试                        │                       │
│          └─────────────────────┬───────────────────┘                       │
│                                │                                            │
│                                ▼                                            │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐             │
│  │   Pod 1         │  │   Pod 2         │  │   Pod 3         │             │
│  │                 │  │                 │  │                 │             │
│  │ Sync Controller │  │ Sync Controller │  │ Sync Controller │             │
│  │ (Leader ✓)      │  │ (Standby)       │  │ (Standby)       │             │
│  │       │         │  │                 │  │                 │             │
│  └───────┼─────────┘  └─────────────────┘  └─────────────────┘             │
│          │                                                                  │
│          │ 只有 Leader 调用 Cloudflare API                                  │
│          ▼                                                                  │
│  ┌─────────────────────────────────────────┐                               │
│  │            Cloudflare API               │                               │
│  └─────────────────────────────────────────┘                               │
│                                                                             │
│  关键点：                                                                    │
│  1. Resource Controller 和 Core Service 所有实例都活跃                      │
│  2. SyncState CRD 通过 K8s 乐观锁处理并发                                   │
│  3. Sync Controller 使用 Leader Election，只有 Leader 同步                  │
│  4. Leader 切换时状态已在 CRD 中，无需重建                                   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. 资源分类与处理策略

### 3.1 资源分类

| 类别 | 特点 | K8s CRD | Cloudflare 资源 | 处理策略 |
|------|------|---------|-----------------|----------|
| **聚合型** | 多个 K8s 资源 → 一个 CF 资源 | Tunnel, Ingress, TunnelBinding, Gateway | Tunnel Configuration | 聚合后同步 |
| **一对一型** | 一个 K8s 资源 → 一个 CF 资源 | DNSRecord, AccessApplication, R2Bucket | 对应 CF 资源 | 直接同步 |
| **依赖型** | 资源间有依赖关系 | AccessApplication → AccessGroup | Access App/Group | 按依赖顺序同步 |
| **组合型** | 可能需要聚合 | ZoneRuleset, TransformRule | Zone Rulesets | 按 Zone 聚合 |

### 3.2 详细资源映射

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           资源映射关系                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  聚合型资源：                                                                │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ SyncState: TunnelConfiguration                                      │   │
│  │                                                                     │   │
│  │   ┌─────────────┐                                                   │   │
│  │   │ Tunnel/     │──┐                                                │   │
│  │   │ ClusterTunnel│  │    warpRouting, fallback, globalOriginRequest │   │
│  │   └─────────────┘  │                                                │   │
│  │   ┌─────────────┐  │                                                │   │
│  │   │ TunnelBinding│──┼───► 聚合 ───► Tunnel Configuration API        │   │
│  │   └─────────────┘  │         ingress rules                          │   │
│  │   ┌─────────────┐  │                                                │   │
│  │   │ Ingress     │──┤                                                │   │
│  │   └─────────────┘  │                                                │   │
│  │   ┌─────────────┐  │                                                │   │
│  │   │ HTTPRoute   │──┘                                                │   │
│  │   └─────────────┘                                                   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  一对一型资源：                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ DNSRecord        ───► SyncState: DNSRecord      ───► DNS API        │   │
│  │ VirtualNetwork   ───► SyncState: VirtualNetwork ───► Network API    │   │
│  │ NetworkRoute     ───► SyncState: NetworkRoute   ───► Network API    │   │
│  │ R2Bucket         ───► SyncState: R2Bucket       ───► R2 API         │   │
│  │ R2BucketDomain   ───► SyncState: R2BucketDomain ───► R2 API         │   │
│  │ OriginCACert     ───► SyncState: OriginCACert   ───► SSL API        │   │
│  │ CloudflareDomain ───► SyncState: CloudflareDomain ─► Zone API       │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  依赖型资源：                                                                │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ AccessGroup          ───► SyncState ───► Access Group API           │   │
│  │      ▲                                                              │   │
│  │      │ 依赖                                                         │   │
│  │      │                                                              │   │
│  │ AccessApplication    ───► SyncState ───► Access Application API     │   │
│  │ AccessServiceToken   ───► SyncState ───► Access Service Token API   │   │
│  │ AccessIdentityProvider ─► SyncState ───► Access IdP API             │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  组合型资源：                                                                │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ SyncState: ZoneRuleset (按 Zone + Phase 聚合)                       │   │
│  │                                                                     │   │
│  │   ┌─────────────┐                                                   │   │
│  │   │ ZoneRuleset │──┐                                                │   │
│  │   └─────────────┘  │                                                │   │
│  │   ┌─────────────┐  ├───► 聚合 ───► Ruleset API                      │   │
│  │   │TransformRule│──┤                                                │   │
│  │   └─────────────┘  │                                                │   │
│  │   ┌─────────────┐  │                                                │   │
│  │   │RedirectRule │──┘                                                │   │
│  │   └─────────────┘                                                   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  Gateway 相关（待定，根据 Gateway API 实现情况）：                            │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ GatewayRule          ───► SyncState ───► Gateway Rules API          │   │
│  │ GatewayList          ───► SyncState ───► Gateway Lists API          │   │
│  │ GatewayConfiguration ───► SyncState ───► Gateway Config API         │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 4. CRD 设计

### 4.1 CloudflareSyncState CRD

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: cloudflaresyncstates.networking.cloudflare-operator.io
spec:
  group: networking.cloudflare-operator.io
  names:
    kind: CloudflareSyncState
    listKind: CloudflareSyncStateList
    plural: cloudflaresyncstates
    singular: cloudflaresyncstate
    shortNames:
    - cfss
    - syncstate
  scope: Cluster  # Cluster 级别，因为可能跨命名空间聚合
  versions:
  - name: v1alpha2
    served: true
    storage: true
    additionalPrinterColumns:
    - name: Type
      type: string
      jsonPath: .spec.resourceType
    - name: CloudflareID
      type: string
      jsonPath: .spec.cloudflareId
    - name: Status
      type: string
      jsonPath: .status.syncStatus
    - name: Version
      type: integer
      jsonPath: .status.configVersion
    - name: Age
      type: date
      jsonPath: .metadata.creationTimestamp
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            required:
            - resourceType
            - cloudflareId
            - accountId
            properties:
              resourceType:
                type: string
                description: "Cloudflare 资源类型"
                enum:
                - TunnelConfiguration
                - DNSRecord
                - AccessApplication
                - AccessGroup
                - AccessServiceToken
                - AccessIdentityProvider
                - VirtualNetwork
                - NetworkRoute
                - R2Bucket
                - R2BucketDomain
                - R2BucketNotification
                - ZoneRuleset
                - TransformRule
                - RedirectRule
                - GatewayRule
                - GatewayList
                - GatewayConfiguration
                - OriginCACertificate
                - CloudflareDomain
                - DomainRegistration
              cloudflareId:
                type: string
                description: "Cloudflare 资源 ID（如 tunnel ID, record ID）"
              accountId:
                type: string
                description: "Cloudflare Account ID"
              zoneId:
                type: string
                description: "Cloudflare Zone ID（如果适用）"
              credentialsRef:
                type: object
                description: "凭证引用"
                properties:
                  name:
                    type: string
                  namespace:
                    type: string
              sources:
                type: array
                description: "配置来源列表"
                items:
                  type: object
                  required:
                  - ref
                  - config
                  properties:
                    ref:
                      type: object
                      description: "来源资源引用"
                      required:
                      - kind
                      - name
                      properties:
                        kind:
                          type: string
                        namespace:
                          type: string
                        name:
                          type: string
                    config:
                      type: object
                      description: "配置内容（JSON 格式）"
                      x-kubernetes-preserve-unknown-fields: true
                    priority:
                      type: integer
                      description: "优先级（用于冲突解决）"
                      default: 100
                    lastUpdated:
                      type: string
                      format: date-time
                      description: "最后更新时间"
          status:
            type: object
            properties:
              syncStatus:
                type: string
                enum:
                - Pending
                - Syncing
                - Synced
                - Error
              lastSyncTime:
                type: string
                format: date-time
              configVersion:
                type: integer
                description: "Cloudflare 配置版本"
              configHash:
                type: string
                description: "配置内容 Hash，用于增量检测"
              aggregatedConfig:
                type: object
                description: "聚合后的配置（用于调试）"
                x-kubernetes-preserve-unknown-fields: true
              error:
                type: string
                description: "最后错误信息"
              conditions:
                type: array
                items:
                  type: object
                  properties:
                    type:
                      type: string
                    status:
                      type: string
                    reason:
                      type: string
                    message:
                      type: string
                    lastTransitionTime:
                      type: string
                      format: date-time
    subresources:
      status: {}
```

### 4.2 SyncState 示例

#### 4.2.1 TunnelConfiguration（聚合型）

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: CloudflareSyncState
metadata:
  name: tunnel-config-abc123
  labels:
    cloudflare-operator.io/resource-type: TunnelConfiguration
    cloudflare-operator.io/tunnel-id: abc123
spec:
  resourceType: TunnelConfiguration
  cloudflareId: "abc123"
  accountId: "account-xxx"
  credentialsRef:
    name: cloudflare-credentials
    namespace: cloudflare-operator-system
  sources:
  # 来自 ClusterTunnel
  - ref:
      kind: ClusterTunnel
      name: production-tunnel
    config:
      warpRouting:
        enabled: true
      fallbackTarget: "http_status:404"
      globalOriginRequest:
        connectTimeout: "30s"
        noTlsVerify: false
    priority: 10  # 最高优先级
    lastUpdated: "2026-01-16T10:00:00Z"
  # 来自 Ingress
  - ref:
      kind: Ingress
      namespace: default
      name: web-app
    config:
      rules:
      - hostname: app.example.com
        path: /
        service: http://web-app-svc.default.svc:80
        originRequest:
          httpHostHeader: app.example.com
    priority: 100
    lastUpdated: "2026-01-16T10:01:00Z"
  # 来自 TunnelBinding
  - ref:
      kind: TunnelBinding
      namespace: api
      name: api-binding
    config:
      rules:
      - hostname: api.example.com
        service: http://api-svc.api.svc:8080
      - hostname: api.example.com
        path: /v2/*
        service: http://api-v2-svc.api.svc:8080
    priority: 100
    lastUpdated: "2026-01-16T10:02:00Z"
status:
  syncStatus: Synced
  lastSyncTime: "2026-01-16T10:02:30Z"
  configVersion: 42
  configHash: "sha256:a1b2c3d4e5f6..."
  aggregatedConfig:
    warpRouting:
      enabled: true
    ingress:
    - hostname: app.example.com
      path: /
      service: http://web-app-svc.default.svc:80
      originRequest:
        httpHostHeader: app.example.com
    - hostname: api.example.com
      path: /v2/*
      service: http://api-v2-svc.api.svc:8080
    - hostname: api.example.com
      service: http://api-svc.api.svc:8080
    - service: http_status:404
  conditions:
  - type: Ready
    status: "True"
    reason: Synced
    message: "Configuration synced to Cloudflare"
    lastTransitionTime: "2026-01-16T10:02:30Z"
```

#### 4.2.2 DNSRecord（一对一型）

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: CloudflareSyncState
metadata:
  name: dns-record-xxx123
  labels:
    cloudflare-operator.io/resource-type: DNSRecord
spec:
  resourceType: DNSRecord
  cloudflareId: "xxx123"
  accountId: "account-xxx"
  zoneId: "zone-yyy"
  credentialsRef:
    name: cloudflare-credentials
    namespace: cloudflare-operator-system
  sources:
  - ref:
      kind: DNSRecord
      namespace: default
      name: app-cname
    config:
      type: CNAME
      name: app
      content: tunnel-abc123.cfargotunnel.com
      proxied: true
      ttl: 1
    lastUpdated: "2026-01-16T10:00:00Z"
status:
  syncStatus: Synced
  lastSyncTime: "2026-01-16T10:00:30Z"
  configVersion: 1
  conditions:
  - type: Ready
    status: "True"
    reason: Synced
    message: "DNS record synced to Cloudflare"
```

---

## 5. 代码结构

### 5.1 目录结构

```
cloudflare-operator/
├── api/
│   └── v1alpha2/
│       ├── tunnel_types.go
│       ├── clustertunnel_types.go
│       ├── tunnelbinding_types.go
│       ├── dnsrecord_types.go
│       ├── accessapplication_types.go
│       ├── accessgroup_types.go
│       ├── virtualnetwork_types.go
│       ├── networkroute_types.go
│       ├── r2bucket_types.go
│       ├── zoneruleset_types.go
│       ├── ...
│       ├── cloudflaresyncstate_types.go      # 新增
│       ├── groupversion_info.go
│       └── zz_generated.deepcopy.go
│
├── internal/
│   ├── controller/                            # Layer 2: Resource Controllers
│   │   ├── common/
│   │   │   ├── base.go                        # 基础 Controller 结构
│   │   │   ├── finalizer.go                   # Finalizer 处理
│   │   │   └── conditions.go                  # Condition 处理
│   │   │
│   │   ├── tunnel/
│   │   │   ├── reconciler.go                  # ~120 行
│   │   │   ├── validator.go                   # ~50 行
│   │   │   └── setup.go                       # ~30 行
│   │   │
│   │   ├── clustertunnel/
│   │   │   ├── reconciler.go
│   │   │   └── setup.go
│   │   │
│   │   ├── tunnelbinding/
│   │   │   ├── reconciler.go
│   │   │   ├── rule_builder.go
│   │   │   └── setup.go
│   │   │
│   │   ├── ingress/
│   │   │   ├── reconciler.go
│   │   │   ├── rule_builder.go
│   │   │   ├── class_resolver.go
│   │   │   └── setup.go
│   │   │
│   │   ├── gateway/
│   │   │   ├── reconciler.go
│   │   │   ├── route_handler.go
│   │   │   └── setup.go
│   │   │
│   │   ├── dnsrecord/
│   │   │   ├── reconciler.go
│   │   │   └── setup.go
│   │   │
│   │   ├── access/
│   │   │   ├── application_reconciler.go
│   │   │   ├── group_reconciler.go
│   │   │   ├── servicetoken_reconciler.go
│   │   │   ├── identityprovider_reconciler.go
│   │   │   └── setup.go
│   │   │
│   │   ├── network/
│   │   │   ├── virtualnetwork_reconciler.go
│   │   │   ├── networkroute_reconciler.go
│   │   │   └── setup.go
│   │   │
│   │   ├── r2/
│   │   │   ├── bucket_reconciler.go
│   │   │   ├── domain_reconciler.go
│   │   │   ├── notification_reconciler.go
│   │   │   └── setup.go
│   │   │
│   │   ├── ruleset/
│   │   │   ├── zoneruleset_reconciler.go
│   │   │   ├── transformrule_reconciler.go
│   │   │   ├── redirectrule_reconciler.go
│   │   │   └── setup.go
│   │   │
│   │   ├── domain/
│   │   │   ├── reconciler.go
│   │   │   └── setup.go
│   │   │
│   │   └── certificate/
│   │       ├── originca_reconciler.go
│   │       └── setup.go
│   │
│   ├── service/                               # Layer 3: Core Services
│   │   ├── interface.go                       # 通用接口
│   │   ├── base.go                            # 基础实现
│   │   │
│   │   ├── tunnel/
│   │   │   ├── service.go                     # TunnelConfigService
│   │   │   ├── aggregator.go                  # 规则聚合逻辑
│   │   │   └── types.go
│   │   │
│   │   ├── dns/
│   │   │   ├── service.go                     # DNSService
│   │   │   └── types.go
│   │   │
│   │   ├── access/
│   │   │   ├── service.go                     # AccessService
│   │   │   ├── dependency.go                  # 依赖管理
│   │   │   └── types.go
│   │   │
│   │   ├── network/
│   │   │   ├── service.go                     # NetworkService
│   │   │   └── types.go
│   │   │
│   │   ├── r2/
│   │   │   ├── service.go                     # R2Service
│   │   │   └── types.go
│   │   │
│   │   ├── ruleset/
│   │   │   ├── service.go                     # RulesetService
│   │   │   ├── aggregator.go                  # 规则聚合
│   │   │   └── types.go
│   │   │
│   │   └── domain/
│   │       ├── service.go                     # DomainService
│   │       └── types.go
│   │
│   ├── sync/                                  # Layer 5: Sync Controllers
│   │   ├── common/
│   │   │   ├── base.go                        # 基础 Sync Controller
│   │   │   ├── debouncer.go                   # 防抖器
│   │   │   ├── hash.go                        # Hash 计算
│   │   │   ├── retry.go                       # 重试逻辑
│   │   │   └── metrics.go                     # Metrics
│   │   │
│   │   ├── tunnel/
│   │   │   ├── controller.go                  # TunnelConfigSyncController
│   │   │   ├── aggregator.go                  # 配置聚合
│   │   │   └── setup.go
│   │   │
│   │   ├── dns/
│   │   │   ├── controller.go                  # DNSSyncController
│   │   │   └── setup.go
│   │   │
│   │   ├── access/
│   │   │   ├── controller.go                  # AccessSyncController
│   │   │   ├── dependency.go                  # 依赖排序
│   │   │   └── setup.go
│   │   │
│   │   ├── network/
│   │   │   ├── controller.go                  # NetworkSyncController
│   │   │   └── setup.go
│   │   │
│   │   ├── r2/
│   │   │   ├── controller.go                  # R2SyncController
│   │   │   └── setup.go
│   │   │
│   │   ├── ruleset/
│   │   │   ├── controller.go                  # RulesetSyncController
│   │   │   ├── aggregator.go
│   │   │   └── setup.go
│   │   │
│   │   └── domain/
│   │       ├── controller.go                  # DomainSyncController
│   │       └── setup.go
│   │
│   ├── clients/                               # Layer 6: Cloudflare API Client
│   │   └── cf/
│   │       ├── client.go                      # 统一客户端
│   │       ├── ratelimit.go                   # 速率限制
│   │       ├── retry.go                       # 重试策略
│   │       ├── errors.go                      # 错误处理
│   │       ├── metrics.go                     # API Metrics
│   │       │
│   │       ├── tunnel.go                      # Tunnel API
│   │       ├── tunnel_config.go               # Tunnel Configuration API
│   │       ├── dns.go                         # DNS API
│   │       ├── access.go                      # Access API
│   │       ├── network.go                     # Network API
│   │       ├── r2.go                          # R2 API
│   │       ├── ruleset.go                     # Ruleset API
│   │       └── zone.go                        # Zone API
│   │
│   ├── credentials/                           # 凭证管理
│   │   ├── resolver.go
│   │   └── cache.go
│   │
│   └── pkg/                                   # 通用工具
│       ├── conditions/
│       │   └── conditions.go
│       ├── finalizer/
│       │   └── finalizer.go
│       ├── events/
│       │   └── recorder.go
│       └── hash/
│           └── hash.go
│
├── cmd/
│   └── main.go                                # 入口，注册所有 Controller
│
├── config/
│   ├── crd/
│   │   └── bases/
│   │       ├── networking.cloudflare-operator.io_tunnels.yaml
│   │       ├── networking.cloudflare-operator.io_cloudflaresyncstates.yaml  # 新增
│   │       └── ...
│   ├── rbac/
│   └── manager/
│
└── docs/
    └── design/
        └── UNIFIED_SYNC_ARCHITECTURE.md       # 本文档
```

### 5.2 代码行数目标

| 组件类型 | 单文件行数 | 说明 |
|----------|-----------|------|
| Resource Controller | 100-150 行 | 验证 + 调用 Service |
| Core Service | 150-200 行 | 业务逻辑 + 写 SyncState |
| Sync Controller | 200-300 行 | 聚合 + 防抖 + 同步 |
| API Client | 100-200 行 | 单一 API 封装 |
| 工具类 | 50-100 行 | 单一职责 |

---

## 6. 接口设计

### 6.1 Core Service 接口

```go
// internal/service/interface.go

package service

import (
    "context"

    "github.com/your-org/cloudflare-operator/api/v1alpha2"
)

// Source 标识配置来源
type Source struct {
    Kind      string `json:"kind"`
    Namespace string `json:"namespace,omitempty"`
    Name      string `json:"name"`
}

func (s Source) String() string {
    if s.Namespace == "" {
        return s.Kind + "/" + s.Name
    }
    return s.Kind + "/" + s.Namespace + "/" + s.Name
}

// ConfigService 是所有 Service 的通用接口
type ConfigService interface {
    // RegisterConfig 注册配置
    RegisterConfig(ctx context.Context, opts RegisterOptions) error

    // UnregisterConfig 注销配置（资源删除时）
    UnregisterConfig(ctx context.Context, opts UnregisterOptions) error
}

// RegisterOptions 注册配置选项
type RegisterOptions struct {
    // Cloudflare 资源标识
    ResourceType string
    CloudflareID string
    AccountID    string
    ZoneID       string // 可选

    // 来源标识
    Source Source

    // 配置内容
    Config interface{}

    // 优先级（用于冲突解决）
    Priority int

    // 凭证引用
    CredentialsRef v1alpha2.CredentialsReference
}

// UnregisterOptions 注销配置选项
type UnregisterOptions struct {
    ResourceType string
    CloudflareID string
    Source       Source
}
```

### 6.2 TunnelConfigService 接口

```go
// internal/service/tunnel/service.go

package tunnel

import (
    "context"

    "github.com/your-org/cloudflare-operator/internal/service"
)

// IngressRule 表示一条路由规则
type IngressRule struct {
    Hostname      string                 `json:"hostname,omitempty"`
    Path          string                 `json:"path,omitempty"`
    Service       string                 `json:"service"`
    OriginRequest *OriginRequestConfig   `json:"originRequest,omitempty"`
}

// TunnelSettings 表示隧道设置
type TunnelSettings struct {
    WarpRouting         *bool                 `json:"warpRouting,omitempty"`
    FallbackTarget      string                `json:"fallbackTarget,omitempty"`
    GlobalOriginRequest *OriginRequestConfig  `json:"globalOriginRequest,omitempty"`
}

// TunnelConfigService 是 Tunnel 配置的专用服务
type TunnelConfigService interface {
    service.ConfigService

    // RegisterTunnelSettings 注册隧道设置（来自 Tunnel/ClusterTunnel Controller）
    RegisterTunnelSettings(ctx context.Context, opts TunnelSettingsOptions) error

    // RegisterIngressRules 注册 ingress 规则（来自 Ingress/TunnelBinding/Gateway Controller）
    RegisterIngressRules(ctx context.Context, opts IngressRulesOptions) error

    // UnregisterSource 注销来源的所有配置
    UnregisterSource(ctx context.Context, tunnelID string, source service.Source) error
}

// TunnelSettingsOptions 注册隧道设置选项
type TunnelSettingsOptions struct {
    TunnelID       string
    AccountID      string
    Source         service.Source
    Settings       TunnelSettings
    CredentialsRef v1alpha2.CredentialsReference
}

// IngressRulesOptions 注册 ingress 规则选项
type IngressRulesOptions struct {
    TunnelID       string
    AccountID      string
    Source         service.Source
    Rules          []IngressRule
    CredentialsRef v1alpha2.CredentialsReference
}
```

### 6.3 Sync Controller 基础接口

```go
// internal/sync/common/base.go

package common

import (
    "context"
    "time"

    "sigs.k8s.io/controller-runtime/pkg/client"

    "github.com/your-org/cloudflare-operator/api/v1alpha2"
)

// SyncController 是所有 Sync Controller 的基础接口
type SyncController interface {
    // Reconcile 处理 SyncState 变更
    Reconcile(ctx context.Context, syncState *v1alpha2.CloudflareSyncState) error

    // Aggregate 聚合多个 sources 的配置
    Aggregate(syncState *v1alpha2.CloudflareSyncState) (interface{}, error)

    // Sync 同步到 Cloudflare
    Sync(ctx context.Context, syncState *v1alpha2.CloudflareSyncState, config interface{}) error
}

// BaseSyncController 提供通用实现
type BaseSyncController struct {
    Client    client.Client
    Debouncer *Debouncer

    // 配置
    DebounceDelay time.Duration
}

// Debouncer 防抖器
type Debouncer struct {
    delay   time.Duration
    pending map[string]*time.Timer
    mu      sync.Mutex
}

func NewDebouncer(delay time.Duration) *Debouncer {
    return &Debouncer{
        delay:   delay,
        pending: make(map[string]*time.Timer),
    }
}

func (d *Debouncer) Debounce(key string, fn func()) {
    d.mu.Lock()
    defer d.mu.Unlock()

    if timer, ok := d.pending[key]; ok {
        timer.Stop()
    }

    d.pending[key] = time.AfterFunc(d.delay, func() {
        d.mu.Lock()
        delete(d.pending, key)
        d.mu.Unlock()
        fn()
    })
}
```

---

## 7. 实施计划

### 7.1 总体阶段

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           实施阶段总览                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Phase 1: 基础设施                                                          │
│  ├── 1.1 CloudflareSyncState CRD 定义                                      │
│  ├── 1.2 Core Service 接口和基础实现                                        │
│  ├── 1.3 Sync Controller 基础框架                                          │
│  └── 1.4 通用工具（防抖、Hash、重试）                                        │
│                                                                             │
│  Phase 2: Tunnel 配置迁移（最复杂，最高优先级）                               │
│  ├── 2.1 TunnelConfigService 实现                                          │
│  ├── 2.2 TunnelConfigSyncController 实现                                   │
│  ├── 2.3 重构 Tunnel/ClusterTunnel Controller                              │
│  ├── 2.4 重构 TunnelBinding Controller                                     │
│  ├── 2.5 重构 Ingress Controller                                           │
│  ├── 2.6 重构 Gateway Controller                                           │
│  └── 2.7 集成测试                                                           │
│                                                                             │
│  Phase 3: DNS 和 Network 迁移                                               │
│  ├── 3.1 DNSService 和 DNSSyncController                                   │
│  ├── 3.2 重构 DNSRecord Controller                                         │
│  ├── 3.3 NetworkService 和 NetworkSyncController                           │
│  ├── 3.4 重构 VirtualNetwork Controller                                    │
│  ├── 3.5 重构 NetworkRoute Controller                                      │
│  └── 3.6 集成测试                                                           │
│                                                                             │
│  Phase 4: Access 资源迁移                                                   │
│  ├── 4.1 AccessService（含依赖管理）                                        │
│  ├── 4.2 AccessSyncController                                              │
│  ├── 4.3 重构 AccessApplication Controller                                 │
│  ├── 4.4 重构 AccessGroup Controller                                       │
│  ├── 4.5 重构 AccessServiceToken Controller                                │
│  ├── 4.6 重构 AccessIdentityProvider Controller                            │
│  └── 4.7 集成测试                                                           │
│                                                                             │
│  Phase 5: R2 和 Ruleset 迁移                                                │
│  ├── 5.1 R2Service 和 R2SyncController                                     │
│  ├── 5.2 重构 R2Bucket/Domain/Notification Controllers                     │
│  ├── 5.3 RulesetService 和 RulesetSyncController                           │
│  ├── 5.4 重构 ZoneRuleset/TransformRule/RedirectRule Controllers           │
│  └── 5.5 集成测试                                                           │
│                                                                             │
│  Phase 6: 其他资源迁移                                                       │
│  ├── 6.1 DomainService 和 DomainSyncController                             │
│  ├── 6.2 重构 CloudflareDomain Controller                                  │
│  ├── 6.3 重构 OriginCACertificate Controller                               │
│  ├── 6.4 重构 DomainRegistration Controller                                │
│  ├── 6.5 Gateway 资源（GatewayRule, GatewayList, GatewayConfiguration）    │
│  └── 6.6 集成测试                                                           │
│                                                                             │
│  Phase 7: 清理和优化                                                         │
│  ├── 7.1 删除旧代码                                                         │
│  ├── 7.2 性能优化                                                           │
│  ├── 7.3 完善 Metrics                                                       │
│  ├── 7.4 E2E 测试                                                           │
│  └── 7.5 文档更新                                                           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 7.2 详细任务清单

#### Phase 1: 基础设施

| 任务 | 文件 | 描述 | 预计行数 |
|------|------|------|----------|
| 1.1.1 | `api/v1alpha2/cloudflaresyncstate_types.go` | SyncState CRD 类型定义 | ~200 |
| 1.1.2 | `config/crd/bases/...syncstates.yaml` | CRD YAML（自动生成） | - |
| 1.2.1 | `internal/service/interface.go` | 通用接口定义 | ~100 |
| 1.2.2 | `internal/service/base.go` | 基础 Service 实现 | ~150 |
| 1.3.1 | `internal/sync/common/base.go` | 基础 Sync Controller | ~150 |
| 1.3.2 | `internal/sync/common/debouncer.go` | 防抖器 | ~80 |
| 1.3.3 | `internal/sync/common/hash.go` | Hash 计算 | ~50 |
| 1.3.4 | `internal/sync/common/retry.go` | 重试逻辑 | ~80 |
| 1.4.1 | `internal/pkg/conditions/conditions.go` | Condition 工具 | ~100 |
| 1.4.2 | `internal/pkg/finalizer/finalizer.go` | Finalizer 工具 | ~80 |
| 1.4.3 | `internal/pkg/events/recorder.go` | 事件记录工具 | ~60 |

#### Phase 2: Tunnel 配置迁移

| 任务 | 文件 | 描述 | 预计行数 |
|------|------|------|----------|
| 2.1.1 | `internal/service/tunnel/types.go` | 类型定义 | ~100 |
| 2.1.2 | `internal/service/tunnel/service.go` | TunnelConfigService | ~200 |
| 2.1.3 | `internal/service/tunnel/aggregator.go` | 规则聚合逻辑 | ~150 |
| 2.2.1 | `internal/sync/tunnel/controller.go` | TunnelConfigSyncController | ~250 |
| 2.2.2 | `internal/sync/tunnel/aggregator.go` | 配置聚合 | ~150 |
| 2.2.3 | `internal/sync/tunnel/setup.go` | 注册 | ~30 |
| 2.3.1 | `internal/controller/tunnel/reconciler.go` | 重构 Tunnel Controller | ~120 |
| 2.3.2 | `internal/controller/tunnel/validator.go` | 验证逻辑 | ~50 |
| 2.3.3 | `internal/controller/tunnel/setup.go` | 注册 | ~30 |
| 2.4.1 | `internal/controller/clustertunnel/reconciler.go` | 重构 ClusterTunnel | ~120 |
| 2.5.1 | `internal/controller/tunnelbinding/reconciler.go` | 重构 TunnelBinding | ~150 |
| 2.5.2 | `internal/controller/tunnelbinding/rule_builder.go` | 规则构建 | ~100 |
| 2.6.1 | `internal/controller/ingress/reconciler.go` | 重构 Ingress | ~150 |
| 2.6.2 | `internal/controller/ingress/rule_builder.go` | 规则构建 | ~100 |
| 2.6.3 | `internal/controller/ingress/class_resolver.go` | IngressClass 解析 | ~80 |
| 2.7.1 | `internal/controller/gateway/reconciler.go` | 重构 Gateway | ~180 |
| 2.7.2 | `internal/controller/gateway/route_handler.go` | 路由处理 | ~120 |

#### Phase 3-6: 其他资源迁移

（类似结构，每个资源类型包含 Service + SyncController + 重构 Controller）

### 7.3 迁移策略

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           迁移策略                                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 渐进式迁移                                                               │
│     - 每个 Phase 独立可发布                                                  │
│     - 旧代码和新代码并存一段时间                                              │
│     - 通过 Feature Flag 控制切换                                             │
│                                                                             │
│  2. 向后兼容                                                                 │
│     - 用户 CRD 不变                                                         │
│     - SyncState 是内部资源，用户无需感知                                      │
│     - API 行为保持一致                                                       │
│                                                                             │
│  3. 测试策略                                                                 │
│     - 每个 Phase 完成后进行集成测试                                          │
│     - Phase 2（Tunnel）完成后进行 E2E 测试                                   │
│     - 全部完成后进行完整 E2E 测试                                            │
│                                                                             │
│  4. 回滚计划                                                                 │
│     - 保留旧 Controller 代码直到新架构稳定                                    │
│     - Feature Flag 可以快速回滚到旧实现                                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 7.4 风险和缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| SyncState CRD 设计不当 | 后续修改困难 | 充分设计评审，预留扩展字段 |
| 聚合逻辑复杂 | 规则丢失或冲突 | 详细单元测试，保留 aggregatedConfig 用于调试 |
| 迁移期间服务中断 | 用户影响 | 渐进式迁移，Feature Flag 控制 |
| 性能问题 | 同步延迟 | 防抖优化，增量检测，Metrics 监控 |
| 多实例竞态 | 配置不一致 | K8s 乐观锁，Leader Election |

---

## 8. 验收标准

### 8.1 功能验收

- [ ] 所有现有功能正常工作
- [ ] Tunnel 配置不再出现竞态问题
- [ ] 多实例部署安全
- [ ] kubectl get cloudflaresyncstates 可以查看状态

### 8.2 性能验收

- [ ] 配置同步延迟 < 2秒（防抖 500ms + API 调用）
- [ ] 批量创建 10 个 Ingress，只触发 1 次 API 调用
- [ ] 无变化时不触发 API 调用

### 8.3 代码质量验收

- [ ] 单个文件不超过 300 行
- [ ] 单元测试覆盖率 > 80%
- [ ] 通过 golangci-lint
- [ ] 所有 Controller 遵循统一模式

---

## 9. 附录

### 9.1 术语表

| 术语 | 定义 |
|------|------|
| Resource Controller | 监听用户 K8s 资源，调用 Core Service |
| Core Service | 业务逻辑层，写入 SyncState CRD |
| SyncState CRD | 共享状态存储，按 Cloudflare 资源组织 |
| Sync Controller | 监听 SyncState，同步到 Cloudflare |
| Source | 配置来源，标识哪个 K8s 资源贡献了配置 |
| Aggregation | 聚合多个 sources 的配置 |
| Debounce | 防抖，合并短时间内多次变更 |

### 9.2 参考资料

- [Kubernetes Controller Best Practices](https://kubernetes.io/docs/concepts/architecture/controller/)
- [Cloudflare API Reference](https://developers.cloudflare.com/api/)
- [controller-runtime Documentation](https://pkg.go.dev/sigs.k8s.io/controller-runtime)
