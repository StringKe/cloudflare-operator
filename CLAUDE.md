# CLAUDE.md

本文件为 Claude Code 提供项目开发指南和代码规范。

## 项目概述

**Cloudflare Zero Trust Kubernetes Operator** - 管理 Cloudflare Zero Trust 全套资源的 Kubernetes Operator。

**Fork 来源**: [adyanth/cloudflare-operator](https://github.com/adyanth/cloudflare-operator)

**技术栈**:

- Go 1.25
- Kubebuilder v4
- controller-runtime v0.22
- cloudflare-go SDK v0.116.0
- gateway-api v1.4.1

## 当前实现状态

### API Group

- **Group**: `networking.cloudflare-operator.io`
- **版本**: v1alpha1 (deprecated), v1alpha2 (storage version)

### 已实现的 CRD (34个)

| 类别         | CRD                      | 作用域     | 状态                                        |
| ------------ | ------------------------ | ---------- | ------------------------------------------- |
| **凭证**     | CloudflareCredentials    | Cluster    | ✅ 完成                                      |
| **域名配置** | CloudflareDomain         | Cluster    | ✅ 完成 (SSL/TLS, 缓存, 安全, WAF)           |
| **网络层**   | Tunnel                   | Namespaced | ✅ 完成                                      |
|              | ClusterTunnel            | Cluster    | ✅ 完成                                      |
|              | VirtualNetwork           | Cluster    | ✅ 完成                                      |
|              | NetworkRoute             | Cluster    | ✅ 完成 (跨 VNet 采用)                       |
|              | WARPConnector            | Namespaced | ✅ 完成 (站点间连接)                         |
| **服务层**   | TunnelBinding            | Namespaced | ⚠️ 废弃 (请迁移到 DNSRecord/Ingress)        |
|              | PrivateService           | Namespaced | ✅ 完成                                      |
|              | DNSRecord                | Namespaced | ✅ 完成                                      |
| **身份层**   | AccessApplication        | Namespaced | ✅ 完成 (内联策略规则)                       |
|              | AccessGroup              | Cluster    | ✅ 完成                                      |
|              | AccessPolicy             | Cluster    | ⚠️ L5 缺失 (可复用策略)                     |
|              | AccessServiceToken       | Namespaced | ✅ 完成                                      |
|              | AccessIdentityProvider   | Cluster    | ✅ 完成                                      |
|              | AccessTunnel             | Namespaced | ⚠️ 废弃 (v1alpha1 遗留)                     |
| **设备层**   | DevicePostureRule        | Cluster    | ✅ 完成                                      |
|              | DeviceSettingsPolicy     | Cluster    | ✅ 完成                                      |
| **网关层**   | GatewayRule              | Cluster    | ✅ 完成                                      |
|              | GatewayList              | Cluster    | ✅ 完成                                      |
|              | GatewayConfiguration     | Cluster    | ✅ 完成                                      |
| **SSL/TLS**  | OriginCACertificate      | Namespaced | ✅ 完成 (自动 K8s Secret)                    |
| **R2 存储**  | R2Bucket                 | Namespaced | ✅ 完成 (生命周期规则)                       |
|              | R2BucketDomain           | Namespaced | ✅ 完成 (自定义域名)                         |
|              | R2BucketNotification     | Namespaced | ✅ 完成 (事件通知)                           |
| **规则引擎** | ZoneRuleset              | Namespaced | ✅ 完成 (WAF, 速率限制等)                    |
|              | TransformRule            | Namespaced | ✅ 完成 (URL/Header 转换)                    |
|              | RedirectRule             | Namespaced | ✅ 完成 (重定向规则)                         |
| **Pages**    | PagesProject             | Namespaced | ✅ 完成 (构建配置、资源绑定、项目采用)       |
|              | PagesDomain              | Namespaced | ✅ 完成 (自定义域名)                         |
|              | PagesDeployment          | Namespaced | ✅ 完成 (直接上传、智能回滚)                 |
| **域名注册** | DomainRegistration       | Cluster    | ✅ 完成 (Enterprise)                         |
| **K8s 集成** | TunnelIngressClassConfig | Cluster    | ✅ 嵌入式 (Ingress 控制器配置)               |
|              | TunnelGatewayClassConfig | Cluster    | ❌ 未实现 (仅类型定义)                       |

**图例**:
- ✅ 完成: 完整六层架构实现
- ⚠️ L5 缺失: 缺少 L5 Sync Controller
- ⚠️ 废弃: 资源已废弃，建议迁移
- ✅ 嵌入式: 嵌入其他控制器中，非独立实现
- ❌ 未实现: 仅有类型定义

### Secret 位置说明

- **Namespaced 资源** (Tunnel, TunnelBinding, DNSRecord 等)：Secret 在资源所在的命名空间
- **Cluster 资源** (ClusterTunnel, VirtualNetwork, NetworkRoute, AccessGroup 等)：Secret 必须在 `cloudflare-operator-system` 命名空间

---

## 代码质量规范 (必须遵守)

### 1. 状态更新必须使用冲突重试

```go
// ✅ 正确: 使用 UpdateStatusWithConflictRetry
err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, obj, func() {
    obj.Status.State = "active"
    controller.SetSuccessCondition(&obj.Status.Conditions, "Reconciled successfully")
})

// ❌ 错误: 直接更新状态
err := r.Status().Update(ctx, obj)
```

### 2. Finalizer 操作必须使用重试

```go
// ✅ 正确: 使用 UpdateWithConflictRetry
err := controller.UpdateWithConflictRetry(ctx, r.Client, obj, func() {
    controllerutil.RemoveFinalizer(obj, FinalizerName)
})

// ❌ 错误: 直接更新
controllerutil.RemoveFinalizer(obj, FinalizerName)
err := r.Update(ctx, obj)
```

### 3. 条件管理必须使用 meta.SetStatusCondition

```go
// ✅ 正确: 使用辅助函数
controller.SetSuccessCondition(&status.Conditions, "Resource reconciled")
controller.SetErrorCondition(&status.Conditions, err)

// 或直接使用 meta.SetStatusCondition
meta.SetStatusCondition(&status.Conditions, metav1.Condition{
    Type:               "Ready",
    Status:             metav1.ConditionTrue,
    Reason:             "Reconciled",
    Message:            "Resource is ready",
    ObservedGeneration: obj.Generation,
})
```

### 4. 事件消息禁止包含敏感信息

```go
// ✅ 正确: 使用 SanitizeErrorMessage
r.Recorder.Event(obj, corev1.EventTypeWarning, "Failed",
    fmt.Sprintf("Error: %s", cf.SanitizeErrorMessage(err)))

// ❌ 错误: 直接使用 err.Error()
r.Recorder.Event(obj, corev1.EventTypeWarning, "Failed", err.Error())
```

### 5. 删除操作必须检查 NotFound

```go
// ✅ 正确: 检查资源是否已删除
if err := r.cfAPI.DeleteResource(id); err != nil {
    if !cf.IsNotFoundError(err) {
        return err  // 真正的错误
    }
    // 已删除，继续处理
    log.Info("Resource already deleted")
}
```

### 6. 删除时必须聚合所有错误

```go
// ✅ 正确: 聚合错误，全部成功后才移除 Finalizer
var errs []error
for _, item := range items {
    if err := deleteItem(item); err != nil {
        errs = append(errs, fmt.Errorf("delete %s: %w", item.Name, err))
    }
}
if len(errs) > 0 {
    return errors.Join(errs...)  // 不移除 Finalizer，下次重试
}
// 全部成功，移除 Finalizer
```

### 7. 必须添加依赖资源的 Watch

```go
// ✅ 正确: Watch 所有依赖资源
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&v1alpha2.MyResource{}).
        Watches(&v1alpha2.Tunnel{},
            handler.EnqueueRequestsFromMapFunc(r.findResourcesForTunnel)).
        Watches(&v1alpha2.VirtualNetwork{},
            handler.EnqueueRequestsFromMapFunc(r.findResourcesForVNet)).
        Watches(&corev1.Service{},
            handler.EnqueueRequestsFromMapFunc(r.findResourcesForService)).
        Complete(r)
}
```

### 8. 资源采用必须检测冲突

```go
// ✅ 正确: 检查管理标记防止冲突
mgmtInfo := controller.NewManagementInfo(obj, "MyResource")
if conflict := controller.GetConflictingManager(existing.Comment, mgmtInfo); conflict != nil {
    return fmt.Errorf("resource managed by %s/%s", conflict.Kind, conflict.Name)
}
// 在 Comment 中添加管理标记
comment := controller.BuildManagedComment(mgmtInfo, userComment)
```

### 9. 凭证解析禁止创建 API 客户端 ⚠️

Resource Controller 只需要凭证元数据 (accountID, credentialsRef)，**禁止**直接创建 Cloudflare API 客户端。

```go
// ✅ 正确: 使用 ResolveCredentialsForService 获取凭证元数据
func (r *Reconciler) resolveCredentials() (*controller.CredentialsInfo, error) {
    return controller.ResolveCredentialsForService(
        r.ctx,
        r.Client,
        r.log,
        r.obj.Spec.Cloudflare,          // CloudflareDetails
        controller.OperatorNamespace,   // Cluster 资源使用 OperatorNamespace
        r.obj.Status.AccountID,         // 已验证的 AccountID (可选)
    )
}

// ✅ 正确: 对于简单 CredentialsReference 使用 ResolveCredentialsFromRef
credInfo, err := controller.ResolveCredentialsFromRef(
    ctx, r.Client, r.log,
    obj.Spec.CredentialsRef,  // *CredentialsReference, 可为 nil
)

// ❌ 错误: 直接创建 API 客户端
apiClient, err := cf.NewAPIClientFromDetails(ctx, r.Client, namespace, cloudflareDetails)
```

**例外情况** (允许直接创建 API 客户端):

- **删除操作**: `deleteFromCloudflare()` 方法中临时创建客户端执行删除（过渡期）
- **证书颁发**: OriginCACertificate 需要直接 API 调用获取证书
- **域名注册**: DomainRegistration 需要直接 API 调用
- **DNS 管理**: Ingress/DNS 控制器中的临时客户端

---

## 控制器标准模板

### 新架构 Resource Controller (推荐)

Resource Controller 应该是轻量级的，只负责验证和转发配置到 Core Service：

```go
// internal/controller/myresource/controller.go
type Reconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder
    Service  *myresourcesvc.Service  // 注入 Core Service，不直接持有 cfAPI
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // 1. 获取资源
    obj := &v1alpha2.MyResource{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. 处理删除 - 通过 Service 注销配置
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

    // 4. 解析引用 (credentials, zone, tunnel 等)
    credRef, accountID, zoneID, err := r.resolveReferences(ctx, obj)
    if err != nil {
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    // 5. 构建配置对象
    config := r.buildConfig(obj)

    // 6. 通过 Service 注册配置 (不直接调用 Cloudflare API)
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
        r.Recorder.Event(obj, corev1.EventTypeWarning, "RegisterFailed", err.Error())
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    // 7. 更新状态
    return ctrl.Result{}, r.updateStatus(ctx, obj, v1alpha2.StatePending, "Registered to sync queue")
}
```

### 删除处理 (新架构)

```go
func (r *Reconciler) handleDeletion(ctx context.Context, obj *v1alpha2.MyResource) (ctrl.Result, error) {
    if !controllerutil.ContainsFinalizer(obj, FinalizerName) {
        return ctrl.Result{}, nil
    }

    // 1. 通过 Service 注销配置 (不直接调用 Cloudflare API)
    if err := r.Service.Unregister(ctx, service.UnregisterOptions{
        ResourceType: v1alpha2.SyncResourceMyResource,
        CloudflareID: obj.Status.CloudflareID,
        Source:       service.Source{Kind: "MyResource", Namespace: obj.Namespace, Name: obj.Name},
    }); err != nil {
        r.Recorder.Event(obj, corev1.EventTypeWarning, "UnregisterFailed", err.Error())
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    // 2. 移除 Finalizer (使用重试)
    if err := controller.UpdateWithConflictRetry(ctx, r.Client, obj, func() {
        controllerutil.RemoveFinalizer(obj, FinalizerName)
    }); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}
```

### 遗留模式 (待迁移)

以下是旧的直接 API 调用模式，**新代码禁止使用**：

```go
// ❌ 禁止: 直接调用 Cloudflare API
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ... 获取资源 ...

    // ❌ 禁止: 控制器直接持有和调用 cfAPI
    if err := r.cfAPI.CreateResource(config); err != nil {
        return ctrl.Result{}, err
    }
}
```

---

## 统一同步架构 (必须遵守) ⚠️

### 核心数据流 (六层架构)

```
K8s Resources → Resource Controllers → Core Services → SyncState CRD → Sync Controllers → Cloudflare API
```

**所有代码必须遵循此数据流**。禁止 Resource Controller 直接调用 Cloudflare API。

### 架构图

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
│  ║ Layer 2: Resource Controllers (轻量级，100-150 行)                     ║ │
│  ║  职责: ✓ 验证 Spec  ✓ 解析引用  ✓ 构建配置  ✓ 调用 Core Service       ║ │
│  ║  禁止: ✗ 直接调用 Cloudflare API  ✗ 持有 cfAPI 字段                   ║ │
│  ║  位置: internal/controller/{resource}/controller.go                    ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    ▼                                        │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 3: Core Services (业务逻辑层，150-200 行)                        ║ │
│  ║  职责: ✓ 验证业务规则  ✓ GetOrCreateSyncState  ✓ UpdateSource (乐观锁)║ │
│  ║  位置: internal/service/{resource}/service.go                          ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    ▼                                        │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 4: CloudflareSyncState CRD (共享状态存储)                        ║ │
│  ║  功能: ✓ K8s 原生存储 (etcd)  ✓ resourceVersion 乐观锁                 ║ │
│  ║        ✓ 多实例安全  ✓ kubectl 可观测  ✓ 状态持久化                    ║ │
│  ║  字段: spec.sources[], status.configHash, status.syncStatus            ║ │
│  ║  位置: api/v1alpha2/cloudflaresyncstate_types.go                       ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    ▼                                        │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 5: Sync Controllers (同步控制器，200-300 行)                     ║ │
│  ║  职责: ✓ Watch SyncState  ✓ 防抖 (500ms)  ✓ 聚合配置  ✓ Hash 比较     ║ │
│  ║        ✓ 调用 Cloudflare API  ✓ 更新 SyncState Status                  ║ │
│  ║  位置: internal/sync/{resource}/controller.go                          ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    │ ✅ 唯一 API 调用点                     │
│                                    ▼                                        │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 6: Cloudflare API Client                                        ║ │
│  ║  功能: ✓ 连接池  ✓ 速率限制  ✓ 自动重试  ✓ 错误分类  ✓ Metrics        ║ │
│  ║  位置: internal/clients/cf/                                            ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    ▼                                        │
│  ┌───────────────────────────────────────────────────────────────────────┐ │
│  │                         Cloudflare API                                │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 资源分类

| 类型         | 说明                         | 示例                                                       |
| ------------ | ---------------------------- | ---------------------------------------------------------- |
| **聚合型**   | 多个 K8s 资源 → 一个 CF 资源 | Tunnel Config (Tunnel + Ingress + TunnelBinding + Gateway) |
| **一对一型** | 一个 K8s 资源 → 一个 CF 资源 | DNSRecord, VirtualNetwork, NetworkRoute                    |
| **依赖型**   | 资源间有顺序依赖             | AccessApplication → AccessGroup                            |

### 来源优先级

```go
const (
    PriorityTunnel  = 10   // Tunnel/ClusterTunnel 设置 (最高)
    PriorityBinding = 50   // TunnelBinding
    PriorityDefault = 100  // Ingress, Gateway, 其他
)
```

### 核心接口

```go
// Core Service 通用接口 (internal/service/interface.go)
type ConfigService interface {
    Register(ctx context.Context, opts RegisterOptions) error
    Unregister(ctx context.Context, opts UnregisterOptions) error
}

// Resource Controller 调用示例
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ... 获取资源、解析引用 ...

    // ✅ 正确: 调用 Core Service 注册配置
    opts := service.RegisterOptions{
        ResourceType:   v1alpha2.SyncResourceDNSRecord,
        CloudflareID:   recordID,
        AccountID:      accountID,
        ZoneID:         zoneID,
        Source:         service.Source{Kind: "DNSRecord", Namespace: ns, Name: name},
        Config:         config,
        Priority:       service.PriorityDefault,
        CredentialsRef: credRef,
    }
    if err := r.dnsService.Register(ctx, opts); err != nil {
        return ctrl.Result{}, err
    }

    // ❌ 错误: 直接调用 Cloudflare API
    // r.cfAPI.CreateDNSRecord(...)
}
```

### 并发安全机制

1. **K8s 乐观锁**: `UpdateSource()` 使用 resourceVersion 冲突重试
2. **Leader Election**: 仅 Leader 实例调用 Cloudflare API
3. **防抖 (Debouncing)**: 500ms 内多次变更合并为一次 API 调用
4. **Hash 检测**: 配置无变化时跳过同步

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

## 代码结构 (按六层架构组织)

```
cloudflare-operator/
├── api/                                     # Layer 1 & 4: CRD 类型定义
│   ├── v1alpha1/                            # 旧版 API (deprecated)
│   └── v1alpha2/                            # 当前存储版本
│       ├── cloudflaresyncstate_types.go     # [Layer 4] 共享同步状态 CRD ⭐
│       ├── tunnel_types.go                  # [Layer 1] 用户资源
│       ├── clustertunnel_types.go
│       ├── tunnelbinding_types.go
│       ├── dnsrecord_types.go
│       ├── virtualnetwork_types.go
│       ├── networkroute_types.go
│       ├── accessapplication_types.go
│       ├── r2bucket_types.go
│       └── ...
│
├── internal/
│   │
│   ├── controller/                          # [Layer 2] Resource Controllers ⭐
│   │   │                                    # 职责: 验证、解析引用、调用 Service
│   │   │                                    # 禁止: 直接调用 Cloudflare API
│   │   ├── status.go                        # 状态更新辅助函数
│   │   ├── constants.go                     # 常量定义
│   │   ├── utils.go                         # 凭证解析辅助 (CredentialsInfo)
│   │   ├── finalizer.go                     # Finalizer 管理辅助
│   │   ├── event.go                         # 事件记录辅助
│   │   ├── generic_tunnel_reconciler.go     # Tunnel/ClusterTunnel 共享逻辑
│   │   ├── dnsrecord/controller.go          # → 调用 dnsService
│   │   ├── virtualnetwork/controller.go     # → 调用 virtualnetworkService
│   │   ├── networkroute/controller.go       # → 调用 networkrouteService
│   │   ├── accessapplication/controller.go  # → 调用 accessService
│   │   ├── accessgroup/controller.go
│   │   ├── accessservicetoken/controller.go
│   │   ├── r2bucket/controller.go           # → 调用 r2Service
│   │   ├── zoneruleset/controller.go        # → 调用 rulesetService
│   │   ├── ingress/controller.go            # → 调用 tunnelService
│   │   ├── gateway/gateway_controller.go    # Gateway API 控制器
│   │   └── ...
│   │
│   ├── service/                             # [Layer 3] Core Services ⭐
│   │   │                                    # 职责: 业务逻辑、管理 SyncState
│   │   ├── interface.go                     # Source, RegisterOptions 定义
│   │   ├── base.go                          # BaseService (乐观锁、重试)
│   │   ├── tunnel/
│   │   │   ├── types.go                     # IngressRule, TunnelSettings
│   │   │   └── service.go                   # RegisterRules, Unregister
│   │   ├── dns/
│   │   │   ├── types.go                     # DNSRecordConfig
│   │   │   └── service.go                   # Register, Unregister
│   │   ├── virtualnetwork/
│   │   │   ├── types.go
│   │   │   └── service.go
│   │   ├── networkroute/
│   │   │   ├── types.go
│   │   │   └── service.go
│   │   ├── access/
│   │   │   ├── types.go
│   │   │   ├── application_service.go
│   │   │   ├── group_service.go
│   │   │   ├── servicetoken_service.go
│   │   │   └── identityprovider_service.go
│   │   ├── r2/
│   │   │   ├── types.go
│   │   │   ├── bucket_service.go
│   │   │   ├── domain_service.go
│   │   │   └── notification_service.go
│   │   ├── ruleset/
│   │   │   ├── types.go
│   │   │   ├── zoneruleset_service.go
│   │   │   ├── transformrule_service.go
│   │   │   └── redirectrule_service.go
│   │   ├── device/
│   │   ├── gateway/
│   │   └── domain/
│   │
│   ├── sync/                                # [Layer 5] Sync Controllers ⭐
│   │   │                                    # 职责: 聚合配置、调用 Cloudflare API
│   │   │                                    # 这是唯一调用 Cloudflare API 的地方！
│   │   ├── common/
│   │   │   ├── base.go                      # BaseSyncController
│   │   │   ├── debouncer.go                 # 防抖器 (500ms)
│   │   │   ├── hash.go                      # 配置 Hash 计算
│   │   │   ├── predicate.go                 # SyncResourceType 过滤
│   │   │   └── helpers.go                   # 通用辅助函数
│   │   ├── tunnel/
│   │   │   ├── aggregator.go                # 聚合多个 sources 的规则
│   │   │   └── controller.go                # → 调用 cfAPI.PutTunnelConfiguration
│   │   ├── dns/controller.go                # → 调用 cfAPI.CreateDNSRecord
│   │   ├── virtualnetwork/controller.go     # → 调用 cfAPI.CreateVirtualNetwork
│   │   ├── networkroute/controller.go       # → 调用 cfAPI.CreateTunnelRoute
│   │   ├── access/
│   │   │   ├── application_controller.go
│   │   │   ├── group_controller.go
│   │   │   ├── servicetoken_controller.go
│   │   │   └── identityprovider_controller.go
│   │   ├── r2/
│   │   │   ├── bucket_controller.go
│   │   │   ├── domain_controller.go
│   │   │   └── notification_controller.go
│   │   ├── ruleset/
│   │   │   ├── zoneruleset_controller.go
│   │   │   ├── transformrule_controller.go
│   │   │   └── redirectrule_controller.go
│   │   ├── device/
│   │   ├── gateway/
│   │   └── privateservice/
│   │
│   ├── clients/                             # [Layer 6] Cloudflare API Client
│   │   └── cf/
│   │       ├── api.go                       # 统一客户端入口
│   │       ├── tunnel_config.go             # Tunnel Configuration API
│   │       ├── dns.go                       # DNS API
│   │       ├── network.go                   # VirtualNetwork, TunnelRoute API
│   │       ├── access.go                    # Access API
│   │       ├── r2.go                        # R2 API
│   │       ├── errors.go                    # 错误处理、敏感信息清理
│   │       └── ...
│   │
│   └── credentials/                         # 凭证加载逻辑
│       └── loader.go
│
├── cmd/
│   └── main.go                              # 注册所有 Controllers 和 Services
│
└── config/
    ├── crd/bases/                           # CRD YAML
    │   ├── networking...cloudflaresyncstates.yaml  # SyncState CRD
    │   └── ...
    └── ...
```

---

## 常用命令

```bash
# 开发
make manifests          # 生成 CRD
make generate           # 生成 DeepCopy
make fmt vet            # 格式化和检查
make test               # 单元测试
make lint               # 运行 golangci-lint
make lint-fix           # 自动修复 lint

# 构建
make build              # 构建二进制
make docker-build       # 构建 Docker 镜像
make docker-buildx      # 多平台构建

# 部署
make install            # 安装 CRD
make deploy             # 部署 Operator
make undeploy           # 移除 Operator

# E2E 测试 ⚠️
# 重要: 运行 E2E 测试前必须确认当前 kubectl context 指向正确的测试集群！
# E2E 测试会与真实 Cloudflare API 交互，错误的集群可能影响生产环境！

kubectl config current-context    # 检查当前集群上下文
kubectl config use-context <test-cluster>  # 切换到测试集群
make test-e2e                     # 运行 E2E 测试
```

---

## 添加新 CRD 检查清单 ⚠️

**重要**: 添加新 CRD 时必须完成以下所有步骤，否则 Release 构建会遗漏 CRD！

### 必须步骤

1. **创建类型定义**

   ```bash
   # 创建 api/v1alpha2/myresource_types.go
   ```

2. **生成代码**

   ```bash
   make manifests generate
   ```

3. **🔴 添加到 kustomization.yaml** (容易遗忘！)

   ```bash
   # 编辑 config/crd/kustomization.yaml，在 resources 中添加:
   - bases/networking.cloudflare-operator.io_myresources.yaml
   ```

4. **创建控制器**

   ```bash
   # 创建 internal/controller/myresource/controller.go
   ```

5. **注册控制器到 main.go**

   ```go
   if err = (&myresource.Reconciler{...}).SetupWithManager(mgr); err != nil {
       // ...
   }
   ```

6. **验证构建输出**
   ```bash
   make build-installer VERSION=x.x.x
   grep "myresources" dist/cloudflare-operator-crds.yaml  # 必须有输出
   ```

### 验证脚本

```bash
# 检查所有 CRD 是否都在 kustomization 中
for crd in config/crd/bases/*.yaml; do
  name=$(basename "$crd")
  if ! grep -q "$name" config/crd/kustomization.yaml; then
    echo "⚠️  Missing: $name"
  fi
done
```

---

## 新资源架构遵循指南 ⚠️

添加新资源时**必须**遵循统一同步架构，禁止在 Resource Controller 中直接调用 Cloudflare API。

### 完整实现步骤

#### 1. 创建 Core Service (Layer 3)

```go
// internal/service/myresource/service.go
package myresource

import (
    "context"
    "github.com/your-org/cloudflare-operator/internal/service"
)

type Service struct {
    *service.BaseService
}

func NewService(client client.Client) *Service {
    return &Service{
        BaseService: service.NewBaseService(client),
    }
}

func (s *Service) Register(ctx context.Context, opts service.RegisterOptions) error {
    return s.BaseService.UpdateSource(ctx, opts)
}

func (s *Service) Unregister(ctx context.Context, opts service.UnregisterOptions) error {
    return s.BaseService.RemoveSource(ctx, opts)
}
```

#### 2. 创建 Sync Controller (Layer 5)

```go
// internal/sync/myresource/controller.go
package myresource

import (
    "context"
    networkingv1alpha2 "github.com/your-org/cloudflare-operator/api/v1alpha2"
    "github.com/your-org/cloudflare-operator/internal/sync/common"
)

type SyncController struct {
    *common.BaseSyncController
}

func (c *SyncController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    syncState := &networkingv1alpha2.CloudflareSyncState{}
    if err := c.Client.Get(ctx, req.NamespacedName, syncState); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 防抖
    c.Debouncer.Debounce(req.Name, func() {
        c.sync(ctx, syncState)
    })

    return ctrl.Result{}, nil
}

func (c *SyncController) sync(ctx context.Context, syncState *networkingv1alpha2.CloudflareSyncState) error {
    // 1. 聚合配置
    config := c.aggregate(syncState)

    // 2. 计算 Hash
    hash := common.ComputeConfigHash(config)
    if syncState.Status.ConfigHash == hash {
        return nil // 无变化，跳过
    }

    // 3. 调用 Cloudflare API (唯一调用点)
    if err := c.cfAPI.SyncResource(config); err != nil {
        return err
    }

    // 4. 更新状态
    return common.UpdateSyncStatus(ctx, c.Client, syncState, networkingv1alpha2.SyncStatusSynced, hash)
}
```

#### 3. 修改 Resource Controller (Layer 2)

```go
// internal/controller/myresource/controller.go

type Reconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder
    Service  *myresourcesvc.Service  // ✅ 注入 Core Service
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    obj := &networkingv1alpha2.MyResource{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 处理删除
    if obj.DeletionTimestamp != nil {
        if err := r.Service.Unregister(ctx, service.UnregisterOptions{
            ResourceType: networkingv1alpha2.SyncResourceMyResource,
            CloudflareID: obj.Status.CloudflareID,
            Source:       service.Source{Kind: "MyResource", Namespace: obj.Namespace, Name: obj.Name},
        }); err != nil {
            return ctrl.Result{}, err
        }
        return controller.RemoveFinalizerSafely(ctx, r.Client, obj, FinalizerName)
    }

    // ✅ 正确: 通过 Service 注册配置
    if err := r.Service.Register(ctx, service.RegisterOptions{
        ResourceType:   networkingv1alpha2.SyncResourceMyResource,
        CloudflareID:   cloudflareID,
        AccountID:      accountID,
        Source:         service.Source{Kind: "MyResource", Namespace: obj.Namespace, Name: obj.Name},
        Config:         buildConfig(obj),
        Priority:       service.PriorityDefault,
        CredentialsRef: credRef,
    }); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}
```

#### 4. 注册到 main.go

```go
// cmd/main.go

// 注册 Core Service
myresourceSvc := myresourcesvc.NewService(mgr.GetClient())

// 注册 Resource Controller
if err = (&myresource.Reconciler{
    Client:   mgr.GetClient(),
    Scheme:   mgr.GetScheme(),
    Recorder: mgr.GetEventRecorderFor("myresource-controller"),
    Service:  myresourceSvc,
}).SetupWithManager(mgr); err != nil {
    // ...
}

// 注册 Sync Controller
if err = (&myresourcesync.SyncController{
    BaseSyncController: common.NewBaseSyncController(mgr.GetClient(), cfAPI),
}).SetupWithManager(mgr); err != nil {
    // ...
}
```

### 架构合规检查清单

- [ ] Resource Controller 不直接调用 `cfAPI.*`
- [ ] Resource Controller 通过 `Service.Register/Unregister` 操作
- [ ] Core Service 继承 `BaseService`
- [ ] Sync Controller 继承 `BaseSyncController`
- [ ] Sync Controller 使用防抖器
- [ ] Sync Controller 使用 Hash 检测变化
- [ ] 资源类型已添加到 `SyncResourceType` 枚举

### 当前架构实施状态

六层架构实施状态一览 (按数据流顺序):

| 资源                         | L2 Controller | L3 Service | L4 SyncState | L5 Sync Ctrl | 完成度 | 备注                              |
| ---------------------------- | :-----------: | :--------: | :----------: | :----------: | :----: | --------------------------------- |
| **Tunnel**                   |      ✅       |     ✅     |      ✅      |      ✅      |  100%  | cfAPI 仅用于元数据存储            |
| **ClusterTunnel**            |      ✅       |     ✅     |      ✅      |      ✅      |  100%  | cfAPI 仅用于元数据存储            |
| **TunnelBinding**            |   ⚠️ 废弃    |     ❌     |      ❌      |      ❌      |  33%   | 废弃，请迁移到 DNSRecord/Ingress  |
| **DNSRecord**                |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **VirtualNetwork**           |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **NetworkRoute**             |      ✅       |     ✅     |      ✅      |      ✅      |  100%  | 跨 VNet 采用                      |
| **PrivateService**           |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **WARPConnector**            |      ✅       |     ✅     |      ✅      |      ✅      |  100%  | 站点间连接                        |
| **AccessApplication**        |      ✅       |     ✅     |      ✅      |      ✅      |  100%  | 内联策略规则                      |
| **AccessGroup**              |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **AccessPolicy**             |      ✅       |     ✅     |      ✅      |      ❌      |  80%   | 缺少 L5 Sync Controller           |
| **AccessServiceToken**       |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **AccessIdentityProvider**   |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **AccessTunnel**             |   ⚠️ 废弃    |     ❌     |      ❌      |      ❌      |  33%   | v1alpha1 遗留，直接创建 Deployment|
| **DevicePostureRule**        |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **DeviceSettingsPolicy**     |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **GatewayRule**              |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **GatewayList**              |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **GatewayConfiguration**     |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **R2Bucket**                 |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **R2BucketDomain**           |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **R2BucketNotification**     |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **ZoneRuleset**              |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **TransformRule**            |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **RedirectRule**             |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **CloudflareDomain**         |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **OriginCACertificate**      |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **DomainRegistration**       |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **PagesProject**             |      ✅       |     ✅     |      ✅      |      ✅      |  100%  | 构建配置、资源绑定                |
| **PagesDomain**              |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **PagesDeployment**          |      ✅       |     ✅     |      ✅      |      ✅      |  100%  | 直接上传、智能回滚                |
| **Ingress**                  |      ✅       |     ✅     |      ✅      |      ✅      |  100%  | DNS Automatic 已迁移              |
| **Gateway**                  |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |                                   |
| **TunnelIngressClassConfig** |      -        |     -      |      -       |      -       |   -    | 嵌入式配置 (Ingress 控制器)       |
| **TunnelGatewayClassConfig** |      ❌       |     ❌     |      ❌      |      ❌      |   0%   | 仅类型定义，未实现                |

**图例**:

- ✅ 已按六层架构实现
- ❌ 未实现或缺失
- ⚠️ 废弃: 资源已标记为废弃，将在未来版本移除
- `-`: 不适用 (嵌入式配置模式)

**废弃资源说明**:

**TunnelBinding** (v1alpha1): 已废弃，请迁移到以下替代方案：
- **Ingress** with `TunnelIngressClassConfig` - 标准 K8s Ingress 集成
- **Gateway API** (HTTPRoute, TCPRoute, UDPRoute) - 现代云原生网关
- **DNSRecord** CRD - 用于需要手动管理 DNS 的场景

TunnelBinding 的 DNS TXT 管理模式 (`createDNSLogic`/`deleteDNSLogic`) 是该资源特有的所有权追踪机制，
与标准 DNSRecord CRD 模式不同。由于资源已废弃，保留现有实现直到移除。

**AccessTunnel** (v1alpha1): 已废弃，请迁移到 **WARPConnector**。
AccessTunnel 直接创建 K8s Deployment 而非通过六层架构，违反统一同步模式。
WARPConnector 提供完整的六层架构实现和更好的站点间连接功能。

**待实现资源**:

**TunnelGatewayClassConfig**: 计划用于 Gateway API 集成配置，目前仅有类型定义。
如需 Gateway API 支持，请使用现有的 TunnelIngressClassConfig 配合 Gateway API 的 HTTPRoute/TCPRoute。

**已完成迁移**:

- ✅ GatewayRule: 删除操作从 L2 移至 L5 Sync Controller (含 Finalizer)
- ✅ GatewayList: 删除操作从 L2 移至 L5 Sync Controller (含 Finalizer)
- ✅ DevicePostureRule: 删除操作从 L2 移至 L5 Sync Controller (含 Finalizer)
- ✅ DeviceSettingsPolicy: L5 Sync Controller 添加 Finalizer 和完整删除处理
- ✅ GatewayConfiguration: L5 Sync Controller 添加 Finalizer 和完整删除处理
- ✅ CloudflareDomain: L5 Sync Controller 添加 Finalizer 和完整删除处理 (Zone 设置保留)
- ✅ TunnelConfiguration: L5 Sync Controller 添加 Finalizer 和完整删除处理 (清空配置)
- ✅ ZoneRuleset: L5 Sync Controller 添加 Finalizer 和完整删除处理 (清空规则)
- ✅ TransformRule: L5 Sync Controller 添加 Finalizer 和完整删除处理 (清空规则)
- ✅ RedirectRule: L5 Sync Controller 添加 Finalizer 和完整删除处理 (清空规则)
- ✅ AccessApplication: Group 引用解析从 L2 移至 L5 Sync Controller
- ✅ Ingress: DNS Automatic 模式从直接 API 改为 dnsService.Register()
- ✅ DomainRegistration: 完整六层架构
- ✅ OriginCACertificate: 完整六层架构
- ✅ Tunnel/ClusterTunnel: cfAPI 仅用于元数据，核心操作通过 LifecycleService
- ✅ DNSRecord: 完整六层架构
- ✅ Gateway: 完整六层架构

---

## 核心辅助函数

### status.go

```go
// 状态更新辅助
controller.UpdateStatusWithConflictRetry(ctx, client, obj, updateFn)
controller.UpdateWithConflictRetry(ctx, client, obj, updateFn)

// 条件设置辅助
controller.SetCondition(conditions, type, status, reason, message)
controller.SetSuccessCondition(conditions, message)
controller.SetErrorCondition(conditions, err)
```

### finalizer.go

```go
// Finalizer 管理辅助
controller.EnsureFinalizer(ctx, client, obj, finalizerName)      // 确保 finalizer 存在
controller.RemoveFinalizerSafely(ctx, client, obj, finalizerName) // 安全移除 finalizer
controller.HasFinalizer(obj, finalizerName)                       // 检查 finalizer 是否存在
controller.IsBeingDeleted(obj)                                    // 检查是否正在删除
controller.ShouldReconcileDeletion(obj, finalizerName)            // 判断是否需要处理删除
```

### event.go

```go
// 事件记录组合辅助
controller.RecordEventAndSetCondition(recorder, obj, conditions, eventType, reason, message, conditionStatus)
controller.RecordSuccessEventAndCondition(recorder, obj, conditions, reason, message)
controller.RecordWarningEventAndCondition(recorder, obj, conditions, reason, message)
controller.RecordErrorEventAndCondition(recorder, obj, conditions, reason, err)  // 自动清理敏感信息
controller.RecordError(recorder, obj, reason, err)   // 仅记录事件
controller.RecordSuccess(recorder, obj, reason, msg) // 仅记录事件
```

### deletion.go

```go
// 删除处理模板
handler := controller.NewDeletionHandler(client, log, recorder, finalizerName)
result, requeue, err := handler.HandleDeletion(ctx, obj, deleteFn)
result, requeue, err := handler.HandleDeletionWithMultipleResources(ctx, obj, deleteFns)

// 快捷函数
result, requeue, err := controller.QuickHandleDeletion(ctx, client, log, recorder, obj, finalizerName, deleteFn)
```

### cf/errors.go

```go
// 错误检查
cf.IsNotFoundError(err)     // 资源不存在
cf.IsConflictError(err)     // 资源已存在
cf.SanitizeErrorMessage(err) // 清理敏感信息
```

### management.go

```go
// 资源管理标记
controller.NewManagementInfo(obj, kind)
controller.BuildManagedComment(mgmtInfo, userComment)
controller.GetConflictingManager(comment, mgmtInfo)
```

### utils.go (凭证解析)

```go
// CredentialsInfo 包含 SyncState 注册所需的凭证信息
type CredentialsInfo struct {
    AccountID      string                           // Cloudflare 账户 ID
    Domain         string                           // Cloudflare 域名
    ZoneID         string                           // Cloudflare Zone ID
    CredentialsRef networkingv1alpha2.CredentialsReference // 凭证引用
}

// ResolveCredentialsForService - 从 CloudflareDetails 解析凭证元数据
// 用于使用 CloudflareDetails 的资源 (Tunnel, VirtualNetwork, NetworkRoute, Access* 等)
controller.ResolveCredentialsForService(ctx, client, log, cloudflareDetails, namespace, statusAccountID)

// ResolveCredentialsFromRef - 从简单 CredentialsReference 解析凭证元数据
// 用于使用 CredentialsReference 的资源 (R2Bucket, ZoneRuleset, TransformRule, RedirectRule 等)
controller.ResolveCredentialsFromRef(ctx, client, log, credRef)

// BuildCredentialsRef - 从 CloudflareDetails 构建 CredentialsReference
controller.BuildCredentialsRef(cloudflareDetails, namespace)
```

---

## 测试要求

1. **所有控制器必须通过**:
   - `make fmt` - 代码格式化
   - `make vet` - 静态检查
   - `make lint` - golangci-lint
   - `make test` - 单元测试

2. **修改 CRD 后必须运行**:

   ```bash
   make manifests generate
   ```

3. **提交前验证**:
   ```bash
   make fmt vet test lint build
   ```

---

## Git 提交规范

遵循 [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: 新功能
fix: Bug 修复
docs: 文档更新
refactor: 重构
test: 测试
chore: 构建/工具
```

示例:

```
feat(networkroute): add VirtualNetwork watch handler

- Add findNetworkRoutesForVirtualNetwork function
- Update SetupWithManager to watch VirtualNetwork changes
- Fixes P0 issue where NetworkRoute not reconciled on VNet update
```

---

## 参考资源

### 内部设计文档

- [统一同步架构设计](docs/design/UNIFIED_SYNC_ARCHITECTURE.md) - 六层架构详细设计
- [统一同步实现指南](docs/design/UNIFIED_SYNC_IMPLEMENTATION.md) - Phase 1-3 实现代码

### 外部参考

- [Cloudflare Zero Trust Docs](https://developers.cloudflare.com/cloudflare-one/)
- [Cloudflare API Reference](https://developers.cloudflare.com/api/)
- [cloudflare-go SDK](https://github.com/cloudflare/cloudflare-go)
- [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime)
