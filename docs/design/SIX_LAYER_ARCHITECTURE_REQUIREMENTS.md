# 六层架构完整需求规范

## 架构数据流

```
L1: K8s Resources → L2: Resource Controllers → L3: Core Services → L4: SyncState CRD → L5: Sync Controllers → L6: Cloudflare API
```

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        统一六层同步架构                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 1: K8s Resources (用户创建和管理)                                ║ │
│  ║  Tunnel, ClusterTunnel, TunnelBinding, Ingress, HTTPRoute, DNSRecord   ║ │
│  ║  VirtualNetwork, NetworkRoute, AccessApplication, R2Bucket, etc.       ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    ▼                                        │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 2: Resource Controllers (轻量级)                                 ║ │
│  ║  职责: ✓ 验证 Spec  ✓ 解析引用  ✓ 构建配置  ✓ 调用 Core Service       ║ │
│  ║  禁止: ✗ 直接调用 Cloudflare API  ✗ 持有 cfAPI 字段                   ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    ▼                                        │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 3: Core Services (业务逻辑层)                                    ║ │
│  ║  职责: ✓ 验证业务规则  ✓ GetOrCreateSyncState  ✓ UpdateSource (乐观锁)║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    ▼                                        │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 4: CloudflareSyncState CRD (共享状态存储)                        ║ │
│  ║  功能: ✓ K8s 原生存储 (etcd)  ✓ resourceVersion 乐观锁                 ║ │
│  ║        ✓ 多实例安全  ✓ kubectl 可观测  ✓ 状态持久化                    ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    ▼                                        │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 5: Sync Controllers (同步控制器)                                 ║ │
│  ║  职责: ✓ Watch SyncState  ✓ 防抖  ✓ 聚合配置  ✓ Hash 比较             ║ │
│  ║        ✓ 调用 Cloudflare API  ✓ 更新 SyncState Status                  ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    │ ✅ 唯一 API 调用点                     │
│                                    ▼                                        │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 6: Cloudflare API Client                                        ║ │
│  ║  功能: ✓ 连接池  ✓ 速率限制  ✓ 自动重试  ✓ 错误分类                   ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    ▼                                        │
│  ┌───────────────────────────────────────────────────────────────────────┐ │
│  │                         Cloudflare API                                │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 需求点 1: 创建/更新实现

### L2 Resource Controller 职责

- 验证 Spec 字段
- 解析引用（credentials, zone, tunnel 等）
- 构建配置对象
- 调用 `Service.Register()` 注册配置到 SyncState
- **禁止**直接调用 Cloudflare API

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. 获取资源
    obj := &v1alpha2.MyResource{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. 处理删除
    if obj.GetDeletionTimestamp() != nil {
        return r.handleDeletion(ctx, obj)
    }

    // 3. 添加 Finalizer
    if !controllerutil.ContainsFinalizer(obj, FinalizerName) {
        controllerutil.AddFinalizer(obj, FinalizerName)
        if err := r.Update(ctx, obj); err != nil {
            return ctrl.Result{}, err
        }
    }

    // 4. 解析引用
    credRef, accountID, zoneID, err := r.resolveReferences(ctx, obj)

    // 5. 构建配置
    config := r.buildConfig(obj)

    // 6. 通过 Service 注册 (禁止直接调用 Cloudflare API)
    if err := r.Service.Register(ctx, service.RegisterOptions{
        ResourceType:   v1alpha2.SyncResourceMyResource,
        CloudflareID:   obj.Status.CloudflareID,
        AccountID:      accountID,
        ZoneID:         zoneID,
        Source:         service.Source{Kind: "MyResource", Namespace: obj.Namespace, Name: obj.Name},
        Config:         config,
        Priority:       service.PriorityDefault,
        CredentialsRef: credRef,
    }); err != nil {
        return ctrl.Result{}, err
    }

    // 7. 更新状态
    return ctrl.Result{}, r.updateStatus(ctx, obj)
}
```

### L3 Core Service 职责

- 验证业务规则
- 调用 `GetOrCreateSyncState()` 获取或创建 SyncState
- 调用 `UpdateSource()` 更新配置（使用乐观锁重试）

```go
func (s *Service) Register(ctx context.Context, opts service.RegisterOptions) error {
    // 1. 验证业务规则
    if err := s.validate(opts); err != nil {
        return err
    }

    // 2. 获取或创建 SyncState
    syncState, err := s.GetOrCreateSyncState(ctx, opts)
    if err != nil {
        return err
    }

    // 3. 更新 Source (乐观锁重试)
    return s.UpdateSource(ctx, syncState, opts)
}
```

### L5 Sync Controller 职责

- Watch SyncState 变化
- 防抖（500ms 内多次变更合并）
- 聚合所有 sources 的配置
- 计算配置 Hash，无变化则跳过
- 调用 Cloudflare API 同步
- 更新 SyncState Status（syncStatus, configHash, lastSyncTime）

```go
func (r *SyncController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. 获取 SyncState
    syncState, err := r.GetSyncState(ctx, req.Name)
    if err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. 检查资源类型
    if syncState.Spec.ResourceType != v1alpha2.SyncResourceMyResource {
        return ctrl.Result{}, nil
    }

    // 3. 处理删除
    if !syncState.DeletionTimestamp.IsZero() {
        return r.handleDeletion(ctx, syncState)
    }

    // 4. 检查 sources 为空
    if len(syncState.Spec.Sources) == 0 {
        return r.handleDeletion(ctx, syncState)
    }

    // 5. 添加 Finalizer
    if !controllerutil.ContainsFinalizer(syncState, FinalizerName) {
        controllerutil.AddFinalizer(syncState, FinalizerName)
        if err := r.Client.Update(ctx, syncState); err != nil {
            return ctrl.Result{}, err
        }
        return ctrl.Result{Requeue: true}, nil
    }

    // 6. 防抖检查
    if r.Debouncer.IsPending(req.Name) {
        return ctrl.Result{}, nil
    }

    // 7. 提取配置
    config, err := r.extractConfig(syncState)
    if err != nil {
        return ctrl.Result{}, err
    }

    // 8. 计算 Hash，无变化则跳过
    newHash, _ := common.ComputeConfigHash(config)
    if !r.ShouldSync(syncState, newHash) {
        return ctrl.Result{}, nil
    }

    // 9. 设置 Syncing 状态
    r.SetSyncStatus(ctx, syncState, v1alpha2.SyncStatusSyncing)

    // 10. 调用 Cloudflare API (唯一调用点)
    result, err := r.syncToCloudflare(ctx, syncState, config)
    if err != nil {
        r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err)
        return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
    }

    // 11. 更新成功状态
    r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusSynced, &common.SyncResult{ConfigHash: newHash}, nil)

    return ctrl.Result{RequeueAfter: common.RequeueAfterSuccess()}, nil
}
```

---

## 需求点 2: 删除实现

### L2 Resource Controller 职责

- 检测 `DeletionTimestamp` 不为零
- 调用 `Service.Unregister()` 从 SyncState 移除 source
- 移除 Finalizer

```go
func (r *Reconciler) handleDeletion(ctx context.Context, obj *v1alpha2.MyResource) (ctrl.Result, error) {
    if !controllerutil.ContainsFinalizer(obj, FinalizerName) {
        return ctrl.Result{}, nil
    }

    // 1. 通过 Service 注销 (禁止直接调用 Cloudflare API)
    if err := r.Service.Unregister(ctx, service.UnregisterOptions{
        ResourceType: v1alpha2.SyncResourceMyResource,
        CloudflareID: obj.Status.CloudflareID,
        Source:       service.Source{Kind: "MyResource", Namespace: obj.Namespace, Name: obj.Name},
    }); err != nil {
        return ctrl.Result{}, err
    }

    // 2. 移除 Finalizer
    controllerutil.RemoveFinalizer(obj, FinalizerName)
    if err := r.Update(ctx, obj); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}
```

### L3 Core Service 职责

- 调用 `RemoveSource()` 从 SyncState 移除该资源的配置

```go
func (s *Service) Unregister(ctx context.Context, opts service.UnregisterOptions) error {
    return s.RemoveSource(ctx, opts)
}
```

### L5 Sync Controller 职责

- 检测 `DeletionTimestamp` 不为零 → 调用 `handleDeletion()`
- 检测 `len(sources) == 0` → 调用 `handleDeletion()`
- `handleDeletion()` 实现：
  - 从 Cloudflare 删除/清空资源
  - 移除 Finalizer
  - 删除孤立的 SyncState（如果是 sources 为空触发的）

```go
func (r *SyncController) handleDeletion(ctx context.Context, syncState *v1alpha2.CloudflareSyncState) (ctrl.Result, error) {
    // 1. 检查 Finalizer
    if !controllerutil.ContainsFinalizer(syncState, FinalizerName) {
        return ctrl.Result{}, nil
    }

    // 2. 从 Cloudflare 删除资源 (根据资源类型选择删除行为)
    if err := r.deleteFromCloudflare(ctx, syncState); err != nil {
        if !cf.IsNotFoundError(err) {
            r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err)
            return ctrl.Result{RequeueAfter: common.RequeueAfterError(err)}, nil
        }
    }

    // 3. 移除 Finalizer
    controllerutil.RemoveFinalizer(syncState, FinalizerName)
    if err := r.Client.Update(ctx, syncState); err != nil {
        return ctrl.Result{}, err
    }

    // 4. 如果是 sources 为空触发的删除，删除孤立的 SyncState
    if syncState.DeletionTimestamp.IsZero() && len(syncState.Spec.Sources) == 0 {
        if err := r.Client.Delete(ctx, syncState); err != nil {
            if client.IgnoreNotFound(err) != nil {
                return ctrl.Result{}, err
            }
        }
    }

    return ctrl.Result{}, nil
}
```

### 删除行为分类

| 类型 | 行为 | 示例资源 |
|------|------|----------|
| **物理删除** | 调用 Delete API | DNSRecord, VirtualNetwork, NetworkRoute, R2Bucket, AccessApplication, AccessGroup, AccessServiceToken, AccessIdentityProvider, DevicePostureRule, GatewayRule, GatewayList, R2BucketDomain, R2BucketNotification, PrivateService |
| **清空配置** | 更新为空数组 | TunnelConfiguration, ZoneRuleset, TransformRule, RedirectRule, DeviceSettingsPolicy |
| **保留配置** | 不调用 API | CloudflareDomain, GatewayConfiguration |

---

## 需求点 3: Finalizer 管理

### L2 Resource Controller

- 创建时添加 Finalizer
- 删除完成后移除 Finalizer

### L5 Sync Controller

- 创建时添加 Finalizer（确保 Cloudflare 资源创建后才能删除）
- `handleDeletion()` 成功后移除 Finalizer

### Finalizer 命名规范

```go
// L2 Resource Controller Finalizer
const FinalizerName = "{resource}.cloudflare-operator.io/finalizer"

// L5 Sync Controller Finalizer
const FinalizerName = "{resource}.sync.cloudflare-operator.io/finalizer"
```

---

## 需求点 4: 状态同步

### SyncState 状态字段

```yaml
status:
  syncStatus: Pending | Syncing | Synced | Error
  configHash: "sha256:..."      # 配置哈希，用于变更检测
  lastSyncTime: "2025-01-18T..."
  lastError: "..."              # 最后错误信息
  cloudflareID: "..."           # 实际 Cloudflare 资源 ID
```

### 状态流转

```
┌─────────┐     ┌─────────┐     ┌────────┐
│ Pending │────▶│ Syncing │────▶│ Synced │
└─────────┘     └────┬────┘     └────────┘
                     │
                     ▼
                ┌─────────┐
                │  Error  │────▶ Syncing (重试)
                └─────────┘
```

---

## 需求点 5: 并发安全

| 机制 | 说明 |
|------|------|
| **乐观锁** | `UpdateSource()` 使用 resourceVersion 冲突重试 |
| **Leader Election** | 仅 Leader 实例调用 Cloudflare API |
| **防抖** | 500ms 内多次变更合并为一次 API 调用 |
| **Hash 检测** | 配置无变化时跳过同步 |

### 竞态条件解决示例

**之前 (直接 API 调用)**:
```
T0: Tunnel Controller  → PUT config (ingress: [])
T1: Ingress Controller → PUT config (ingress: [app.com])
T2: TunnelBinding      → PUT config (ingress: [api.com])  ← 覆盖了 T1!
结果: app.com 规则丢失！
```

**现在 (通过 SyncState)**:
```
T0: Tunnel Controller  → Register settings to SyncState
T1: Ingress Controller → UpdateSource (乐观锁重试)
T2: TunnelBinding      → UpdateSource (乐观锁重试)
T3: Sync Controller    → Aggregate all sources → PUT (all rules)
结果: 所有规则都保留！
```

---

## 需求点 6: 聚合型资源

### 多个 K8s 资源 → 一个 Cloudflare 资源

- Tunnel Config = Tunnel + Ingress + TunnelBinding + Gateway
- 使用 `sources[]` 数组存储多个来源配置
- 使用 `priority` 字段处理冲突（数值小优先级高）

### 来源优先级

```go
const (
    PriorityTunnel  = 10   // Tunnel/ClusterTunnel 设置 (最高)
    PriorityBinding = 50   // TunnelBinding
    PriorityDefault = 100  // Ingress, Gateway, 其他
)
```

### 聚合逻辑示例

```go
func (r *SyncController) aggregateConfig(syncState *v1alpha2.CloudflareSyncState) *Config {
    // 按优先级排序 sources
    sources := sortByPriority(syncState.Spec.Sources)

    config := &Config{}
    for _, source := range sources {
        // 合并配置，优先级高的覆盖低的
        config.Merge(source.Config)
    }
    return config
}
```

---

## 需求点 7: 错误处理

| 函数 | 用途 |
|------|------|
| `cf.IsNotFoundError(err)` | 检查资源是否已删除 |
| `cf.IsConflictError(err)` | 检查资源冲突 (error 1014) |
| `cf.SanitizeErrorMessage(err)` | 清理敏感信息 |
| `common.RequeueAfterError(err)` | 计算错误重试延迟 |

### 删除时错误聚合

```go
func (r *SyncController) deleteMultipleResources(ctx context.Context, resources []Resource) error {
    var errs []error
    for _, res := range resources {
        if err := r.deleteResource(ctx, res); err != nil {
            if !cf.IsNotFoundError(err) {
                errs = append(errs, fmt.Errorf("delete %s: %w", res.Name, err))
            }
        }
    }
    if len(errs) > 0 {
        return errors.Join(errs...)  // 不移除 Finalizer，下次重试
    }
    return nil  // 全部成功，可以移除 Finalizer
}
```

---

## 需求点 8: 资源采用

### 管理标记

- 检查 Cloudflare 资源的 Comment 字段中的管理标记
- 防止不同 K8s 资源管理同一个 Cloudflare 资源
- 使用 `controller.BuildManagedComment()` 添加管理标记

### 实现示例

```go
// 检查是否有冲突的管理者
mgmtInfo := controller.NewManagementInfo(obj, "MyResource")
if conflict := controller.GetConflictingManager(existing.Comment, mgmtInfo); conflict != nil {
    return fmt.Errorf("resource managed by %s/%s", conflict.Kind, conflict.Name)
}

// 在 Comment 中添加管理标记
comment := controller.BuildManagedComment(mgmtInfo, userComment)
```

---

## 资源分类

| 类型 | 说明 | 示例 |
|------|------|------|
| **聚合型** | 多个 K8s 资源 → 一个 CF 资源 | Tunnel Config (Tunnel + Ingress + TunnelBinding + Gateway) |
| **一对一型** | 一个 K8s 资源 → 一个 CF 资源 | DNSRecord, VirtualNetwork, NetworkRoute, R2Bucket |
| **依赖型** | 资源间有顺序依赖 | AccessApplication → AccessGroup |

---

## 当前合规性状态

### L5 Sync Controllers 合规性

| 控制器 | Finalizer | DeletionTimestamp检查 | sources为空检查 | handleDeletion | 删除行为 |
|--------|:---------:|:--------------------:|:--------------:|:--------------:|----------|
| `dns/controller.go` | ✅ | ✅ | ✅ | ✅ | 物理删除 |
| `virtualnetwork/controller.go` | ✅ | ✅ | ✅ | ✅ | 物理删除 |
| `networkroute/controller.go` | ✅ | ✅ | ✅ | ✅ | 物理删除 |
| `privateservice/controller.go` | ✅ | ✅ | ✅ | ✅ | 物理删除 |
| `tunnel/controller.go` | ✅ | ✅ | ✅ | ✅ | 清空配置 |
| `tunnel/lifecycle_controller.go` | ✅ | ✅ | ✅ | ✅ | 保留配置 |
| `domain/cloudflaredomain_controller.go` | ✅ | ✅ | ✅ | ✅ | 保留配置 |
| `domain/origincacertificate_controller.go` | ✅ | ✅ | ✅ | ✅ | 吊销证书 |
| `domain/domainregistration_controller.go` | ✅ | ✅ | ✅ | ✅ | 保留配置 |
| `access/application_controller.go` | ✅ | ✅ | ✅ | ✅ | 物理删除 |
| `access/group_controller.go` | ✅ | ✅ | ✅ | ✅ | 物理删除 |
| `access/servicetoken_controller.go` | ✅ | ✅ | ✅ | ✅ | 物理删除 |
| `access/identityprovider_controller.go` | ✅ | ✅ | ✅ | ✅ | 物理删除 |
| `r2/bucket_controller.go` | ✅ | ✅ | ✅ | ✅ | 物理删除 |
| `r2/domain_controller.go` | ✅ | ✅ | ✅ | ✅ | 物理删除 |
| `r2/notification_controller.go` | ✅ | ✅ | ✅ | ✅ | 物理删除 |
| `ruleset/zoneruleset_controller.go` | ✅ | ✅ | ✅ | ✅ | 清空配置 |
| `ruleset/transformrule_controller.go` | ✅ | ✅ | ✅ | ✅ | 清空配置 |
| `ruleset/redirectrule_controller.go` | ✅ | ✅ | ✅ | ✅ | 清空配置 |
| `device/posturerule_controller.go` | ✅ | ✅ | ✅ | ✅ | 物理删除 |
| `device/settingspolicy_controller.go` | ✅ | ✅ | ✅ | ✅ | 清空配置 |
| `gateway/rule_controller.go` | ✅ | ✅ | ✅ | ✅ | 物理删除 |
| `gateway/list_controller.go` | ✅ | ✅ | ✅ | ✅ | 物理删除 |
| `gateway/configuration_controller.go` | ✅ | ✅ | ✅ | ✅ | 保留配置 |
| `warp/connector_controller.go` | ✅ | ✅ | ✅ | ✅ | 保留配置 |

### L2 Resource Controllers 合规性

| 控制器 | 使用Service | 无cfAPI字段 | 状态 |
|--------|:-----------:|:-----------:|------|
| `accessapplication/controller.go` | ✅ | ✅ | 合规 |
| `accessgroup/controller.go` | ✅ | ✅ | 合规 |
| `accessservicetoken/controller.go` | ✅ | ✅ | 合规 |
| `accessidentityprovider/controller.go` | ✅ | ✅ | 合规 |
| `dnsrecord/controller.go` | ✅ | ✅ | 合规 |
| `virtualnetwork/controller.go` | ✅ | ✅ | 合规 |
| `networkroute/controller.go` | ✅ | ✅ | 合规 |
| `privateservice/controller.go` | ✅ | ✅ | 合规 |
| `r2bucket/controller.go` | ✅ | ✅ | 合规 |
| `r2bucketdomain/controller.go` | ✅ | ✅ | 合规 |
| `r2bucketnotification/controller.go` | ✅ | ✅ | 合规 |
| `zoneruleset/controller.go` | ✅ | ✅ | 合规 |
| `transformrule/controller.go` | ✅ | ✅ | 合规 |
| `redirectrule/controller.go` | ✅ | ✅ | 合规 |
| `deviceposturerule/controller.go` | ✅ | ✅ | 合规 |
| `devicesettingspolicy/controller.go` | ✅ | ✅ | 合规 |
| `gatewayrule/controller.go` | ✅ | ✅ | 合规 |
| `gatewaylist/controller.go` | ✅ | ✅ | 合规 |
| `gatewayconfiguration/controller.go` | ✅ | ✅ | 合规 |
| `tunnel_controller.go` | ⚠️ | ❌ | 待迁移 |
| `clustertunnel_controller.go` | ⚠️ | ❌ | 待迁移 |
| `tunnelbinding_controller.go` | ✅ | ✅ | 合规 (已废弃) |
| `ingress/controller.go` | ✅ | ✅ | 合规 |
| `gateway/gateway_controller.go` | ✅ | ✅ | 合规 |

**说明**:
- ✅ = 完全合规
- ⚠️ = 部分合规或待迁移
- ❌ = 不合规

**待迁移项**:
- `tunnel_controller.go` 和 `clustertunnel_controller.go` 持有 `cfAPI` 字段，需要迁移到使用 Service 层

---

## 文件组织结构

```
internal/
├── controller/                    # [Layer 2] Resource Controllers
│   └── {resource}/controller.go   # 调用 Service，禁止调用 cfAPI
│
├── service/                       # [Layer 3] Core Services
│   └── {resource}/
│       ├── types.go               # 配置类型定义
│       └── service.go             # Register/Unregister 实现
│
├── sync/                          # [Layer 5] Sync Controllers
│   ├── common/
│   │   ├── base.go                # BaseSyncController
│   │   ├── debouncer.go           # 防抖器
│   │   ├── hash.go                # Hash 计算
│   │   └── helpers.go             # 辅助函数
│   └── {resource}/controller.go   # 唯一调用 Cloudflare API 的地方
│
└── clients/cf/                    # [Layer 6] Cloudflare API Client
    └── *.go                       # API 封装
```
