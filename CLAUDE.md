# CLAUDE.md

Cloudflare Zero Trust Kubernetes Operator 开发指南。

## 项目概述

| 项目 | 值 |
|------|---|
| Fork | [adyanth/cloudflare-operator](https://github.com/adyanth/cloudflare-operator) |
| API Group | `networking.cloudflare-operator.io` |
| 版本 | v1alpha1 (deprecated), v1alpha2 (storage) |
| 技术栈 | Go 1.25, Kubebuilder v4, controller-runtime v0.22, cloudflare-go v0.116.0, gateway-api v1.4.1 |

### CRD 概览 (34个)

| 类别 | CRD | Scope | 备注 |
|------|-----|-------|------|
| 凭证 | CloudflareCredentials | Cluster | |
| 域名 | CloudflareDomain | Cluster | SSL/TLS, 缓存, WAF |
| 网络 | Tunnel, ClusterTunnel | NS/Cluster | |
| | VirtualNetwork, NetworkRoute | Cluster | 跨 VNet 采用 |
| | WARPConnector | NS | 站点间连接 |
| 服务 | ~~TunnelBinding~~ | NS | ⚠️废弃→DNSRecord/Ingress |
| | PrivateService, DNSRecord | NS | |
| 身份 | AccessApplication | NS | 内联策略, Watch AccessPolicy |
| | AccessGroup, AccessPolicy | Cluster | 可复用策略 |
| | AccessServiceToken | NS | |
| | AccessIdentityProvider | Cluster | |
| | ~~AccessTunnel~~ | NS | ⚠️废弃→WARPConnector |
| 设备 | DevicePostureRule, DeviceSettingsPolicy | Cluster | |
| 网关 | GatewayRule, GatewayList, GatewayConfiguration | Cluster | |
| SSL | OriginCACertificate | NS | 自动 K8s Secret |
| R2 | R2Bucket, R2BucketDomain, R2BucketNotification | NS | |
| 规则 | ZoneRuleset, TransformRule, RedirectRule | NS | |
| Pages | PagesProject, PagesDomain, PagesDeployment | NS | |
| 注册 | DomainRegistration | Cluster | Enterprise |
| K8s | TunnelIngressClassConfig, TunnelGatewayClassConfig | Cluster | 嵌入式 |

**Secret 位置**: Namespaced 资源在资源所在 NS，Cluster 资源在 `cloudflare-operator-system`

---

## 三层同步架构 (新架构)

```
L1: K8s CRD → L2: Controller → L3: Cloudflare API
```

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           极简三层架构                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 1: K8s CRD (用户资源)                                            ║ │
│  ║  ├─ 1:1 资源: DNSRecord, AccessApp, R2Bucket, PagesDeployment...      ║ │
│  ║  └─ 聚合资源: Tunnel, Ingress, TunnelBinding, HTTPRoute               ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    ▼                                        │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 2: Controllers (直接同步)                                        ║ │
│  ║                                                                       ║ │
│  ║  1:1 Controllers:                                                     ║ │
│  ║  ├─ 直接调用 CF API                                                   ║ │
│  ║  ├─ 直接写回 CRD Status                                               ║ │
│  ║  └─ 独立 Informer，互不干扰                                            ║ │
│  ║                                                                       ║ │
│  ║  TunnelConfig Controller (聚合专用):                                   ║ │
│  ║  ├─ 监听 ConfigMap 变化                                                ║ │
│  ║  ├─ 聚合规则，单次 API 调用                                            ║ │
│  ║  └─ OwnerReference 自动清理                                           ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                    │                                        │
│                                    ▼                                        │
│  ╔═══════════════════════════════════════════════════════════════════════╗ │
│  ║ Layer 3: Cloudflare API Client                                        ║ │
│  ║  功能: ✓ 连接池  ✓ 速率限制  ✓ 自动重试  ✓ 错误分类                   ║ │
│  ╚═══════════════════════════════════════════════════════════════════════╝ │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 资源分类处理

| 类型 | 示例资源 | 处理方式 |
|------|----------|----------|
| **1:1 直接同步** | DNSRecord, AccessApplication, R2Bucket, PagesDeployment... | Controller 直接调 API，状态直接写回 CRD |
| **聚合同步** | Tunnel, Ingress, TunnelBinding, HTTPRoute | 写入 ConfigMap → TunnelConfig Controller 聚合 |
| **异步生命周期** | Tunnel/ClusterTunnel 创建删除 | 使用 SyncState + Lifecycle Controller |

### ConfigMap 聚合方案

Tunnel 配置聚合使用 ConfigMap 替代 SyncState：

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tunnel-config-abc123
  namespace: cloudflare-operator-system
  labels:
    cloudflare-operator.io/tunnel-id: abc123
    cloudflare-operator.io/type: tunnel-config
  ownerReferences:
    - kind: ClusterTunnel
      name: production-tunnel
data:
  config.json: |
    {
      "tunnelId": "abc123",
      "warpRouting": {"enabled": true},
      "sources": {
        "ClusterTunnel/production-tunnel": {
          "settings": {"warpRouting": true}
        },
        "Ingress/default/web-app": {
          "rules": [{"hostname": "app.example.com", "service": "http://web:80"}]
        }
      }
    }
```

---

## 代码质量规范

### 必须使用

```go
// 1. 状态更新 - 冲突重试
controller.UpdateStatusWithConflictRetry(ctx, r.Client, obj, func() { ... })

// 2. Finalizer - 冲突重试
controller.UpdateWithConflictRetry(ctx, r.Client, obj, func() {
    controllerutil.RemoveFinalizer(obj, FinalizerName)
})

// 3. 条件管理
controller.SetSuccessCondition(&status.Conditions, "msg")
controller.SetErrorCondition(&status.Conditions, err)

// 4. 事件 - 清理敏感信息
r.Recorder.Event(obj, corev1.EventTypeWarning, "Failed", cf.SanitizeErrorMessage(err))

// 5. 删除 - 检查NotFound
if err := r.cfAPI.Delete(id); err != nil && !cf.IsNotFoundError(err) { return err }

// 6. 删除 - 聚合错误
var errs []error
for _, item := range items {
    if err := delete(item); err != nil { errs = append(errs, err) }
}
if len(errs) > 0 { return errors.Join(errs...) }

// 7. Watch依赖资源
ctrl.NewControllerManagedBy(mgr).For(&v1alpha2.MyResource{}).
    Watches(&v1alpha2.Tunnel{}, handler.EnqueueRequestsFromMapFunc(r.findForTunnel)).Complete(r)

// 8. 资源采用 - 检测冲突
mgmtInfo := controller.NewManagementInfo(obj, "Kind")
if conflict := controller.GetConflictingManager(existing.Comment, mgmtInfo); conflict != nil { return err }

// 9. 凭证解析
credInfo, _ := controller.ResolveCredentialsForService(ctx, r.Client, log, cloudflareDetails, ns, accountID)
// 或
credInfo, _ := controller.ResolveCredentialsFromRef(ctx, r.Client, log, credRef)

// 10. ConfigMap 写入 (聚合资源)
writer := tunnelconfig.NewWriter(r.Client, r.Namespace)
if err := writer.WriteSourceConfig(ctx, tunnelID, sourceKey, config); err != nil { ... }
```

---

## 控制器模板

### 1:1 资源 Controller (直接同步)

```go
// internal/controller/myresource/controller.go
type Reconciler struct {
    client.Client
    Scheme    *runtime.Scheme
    Recorder  record.EventRecorder
    APIClient *cf.Client  // 直接持有 API 客户端
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    obj := &v1alpha2.MyResource{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 删除处理
    if obj.DeletionTimestamp != nil {
        return r.handleDeletion(ctx, obj)
    }

    // 添加 Finalizer
    if !controllerutil.ContainsFinalizer(obj, FinalizerName) {
        controllerutil.AddFinalizer(obj, FinalizerName)
        if err := r.Update(ctx, obj); err != nil {
            return ctrl.Result{}, err
        }
    }

    // 直接调用 Cloudflare API
    result, err := r.syncToCloudflare(ctx, obj)
    if err != nil {
        return r.handleSyncError(ctx, obj, err)
    }

    // 直接写回 CRD Status
    return r.setSuccessStatus(ctx, obj, result)
}
```

### 聚合资源 Controller (ConfigMap)

```go
// internal/controller/ingress/controller.go
type Reconciler struct {
    client.Client
    Scheme    *runtime.Scheme
    Recorder  record.EventRecorder
    Namespace string  // Operator 命名空间
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    ingress := &networkingv1.Ingress{}
    if err := r.Get(ctx, req.NamespacedName, ingress); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 删除处理
    if ingress.DeletionTimestamp != nil {
        return r.handleDeletion(ctx, ingress)
    }

    // 解析 Tunnel ID
    tunnelID := r.resolveTunnelID(ingress)
    rules := r.buildIngressRules(ingress)

    // 写入 ConfigMap
    writer := tunnelconfig.NewWriter(r.Client, r.Namespace)
    sourceKey := fmt.Sprintf("Ingress/%s/%s", ingress.Namespace, ingress.Name)
    config := &tunnelconfig.SourceConfig{
        Rules:    rules,
        Priority: tunnelconfig.PriorityIngress,
    }

    if err := writer.WriteSourceConfig(ctx, tunnelID, sourceKey, config); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}
```

---

## 常用命令

```bash
make manifests generate  # 修改CRD后必须运行
make fmt vet test lint build  # 提交前验证
make docker-build docker-buildx  # 构建镜像
make install deploy undeploy  # 部署
make test-e2e  # E2E测试 ⚠️确认kubectl context!
```

---

## 添加新 CRD 检查清单

### 1:1 资源 (直接同步)

1. 创建 `api/v1alpha2/myresource_types.go`
2. `make manifests generate`
3. **🔴 添加到 `config/crd/kustomization.yaml`** (容易遗忘!)
4. 创建 `internal/controller/myresource/controller.go` (直接调用 CF API)
5. 注册到 `cmd/main.go`
6. 验证: `make build-installer VERSION=x.x.x && grep "myresources" dist/cloudflare-operator-crds.yaml`

### 聚合资源 (Tunnel 配置)

1. 确定资源属于哪个 Tunnel
2. 在 Controller 中使用 `tunnelconfig.Writer` 写入 ConfigMap
3. TunnelConfig Controller 自动聚合并同步

---

## 核心辅助函数

| 模块 | 函数 |
|------|------|
| status.go | `UpdateStatusWithConflictRetry`, `UpdateWithConflictRetry`, `SetCondition`, `SetSuccessCondition`, `SetErrorCondition` |
| finalizer.go | `EnsureFinalizer`, `RemoveFinalizerSafely`, `HasFinalizer`, `IsBeingDeleted`, `ShouldReconcileDeletion` |
| event.go | `RecordEventAndSetCondition`, `RecordSuccessEventAndCondition`, `RecordWarningEventAndCondition`, `RecordErrorEventAndCondition` |
| deletion.go | `NewDeletionHandler`, `HandleDeletion`, `QuickHandleDeletion` |
| cf/errors.go | `IsNotFoundError`, `IsConflictError`, `SanitizeErrorMessage` |
| management.go | `NewManagementInfo`, `BuildManagedComment`, `GetConflictingManager` |
| utils.go | `ResolveCredentialsForService`, `ResolveCredentialsFromRef`, `BuildCredentialsRef` |
| tunnelconfig/writer.go | `NewWriter`, `WriteSourceConfig`, `RemoveSourceConfig`, `GetTunnelConfig` |

---

## 代码结构

```
api/v1alpha2/                       # CRD 类型定义
internal/
├── controller/                     # Controllers
│   ├── {resource}/                 # 1:1 资源 Controller (直接调用 CF API)
│   ├── tunnelconfig/               # Tunnel 配置聚合 Controller
│   │   ├── controller.go           # 监听 ConfigMap，聚合同步
│   │   ├── writer.go               # ConfigMap 读写工具
│   │   └── types.go                # 配置类型定义
│   ├── ingress/                    # Ingress Controller (写入 ConfigMap)
│   └── gateway/                    # Gateway Controller (写入 ConfigMap)
├── clients/cf/                     # Cloudflare API Client
├── credentials/                    # 凭证加载
├── sync/tunnel/                    # Tunnel 生命周期 (异步创建/删除)
│   └── lifecycle_controller.go     # 使用 SyncState 处理异步操作
└── service/tunnel/                 # Tunnel 生命周期服务
    └── lifecycle_service.go        # Tunnel 创建/删除业务逻辑
```

---

## 架构说明

### 为什么从六层简化为三层？

旧六层架构问题：
1. **轮询不工作**：29 个 Sync Controller 共享 SyncState Informer，事件互相干扰
2. **状态回写缺失**：L5 写 SyncState.Status，L2 需要轮询读取再回写
3. **并发冲突**：L3 写 Spec.Sources + L5 写 Status，同时操作一个对象
4. **代码复杂**：六层架构导致数据流难以追踪

新三层架构收益：
1. **轮询稳定**：每个 CRD 独立 Controller + Informer，RequeueAfter 不被干扰
2. **状态直接回写**：无中间层，用户只看一个资源
3. **消除并发冲突**：单层写入，无竞争
4. **代码量减少**：删除了 Service 和 Sync 中间层

### Tunnel 配置特殊处理

Tunnel 配置需要聚合多个来源：
- Tunnel/ClusterTunnel: warpRouting, fallback 设置
- Ingress: hostname → service 规则
- TunnelBinding: 额外路由规则
- HTTPRoute: Gateway API 路由

使用 ConfigMap 聚合：
1. 各 Controller 写入自己的配置到 ConfigMap
2. TunnelConfig Controller 监听 ConfigMap 变化
3. 聚合所有 sources，单次 API 调用同步到 Cloudflare
4. OwnerReference 确保 Tunnel 删除时自动清理

---

## 文档规范

- 中英双语: `docs/{en,zh}/api-reference/{crd}.md`
- 必须包含: Spec/Status表格, 3+示例, Mermaid架构图, 前置条件/限制
- Mermaid布局: 复杂图表用`elk`，简单用`dagre`

---

## 参考

- [Cloudflare Zero Trust Docs](https://developers.cloudflare.com/cloudflare-one/)
- [cloudflare-go SDK](https://github.com/cloudflare/cloudflare-go)
