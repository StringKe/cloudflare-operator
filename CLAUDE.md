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

## 六层同步架构 ⚠️必须遵守

```
K8s Resources → L2 Resource Controllers → L3 Core Services → L4 SyncState CRD → L5 Sync Controllers → L6 Cloudflare API
```

| 层 | 位置 | 职责 | 禁止 |
|---|------|------|------|
| L2 | `internal/controller/` | 验证Spec, 解析引用, 调用Service | 直接调用cfAPI |
| L3 | `internal/service/` | 业务逻辑, 管理SyncState | |
| L4 | SyncState CRD | K8s原生存储, 乐观锁 | |
| L5 | `internal/sync/` | 聚合配置, 防抖500ms, Hash检测, 调用API | |
| L6 | `internal/clients/cf/` | 连接池, 速率限制, 重试 | |

**并发安全**: K8s乐观锁 + Leader Election + 防抖 + Hash检测

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

// 9. 凭证解析 - 禁止创建API客户端
credInfo, _ := controller.ResolveCredentialsForService(ctx, r.Client, log, cloudflareDetails, ns, accountID)
// 或
credInfo, _ := controller.ResolveCredentialsFromRef(ctx, r.Client, log, credRef)
```

---

## 控制器模板

```go
// internal/controller/myresource/controller.go
type Reconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder
    Service  *myresourcesvc.Service  // 注入Service，禁止cfAPI
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    obj := &v1alpha2.MyResource{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil { return ctrl.Result{}, client.IgnoreNotFound(err) }

    // 删除处理
    if obj.DeletionTimestamp != nil {
        if err := r.Service.Unregister(ctx, service.UnregisterOptions{...}); err != nil { return ctrl.Result{}, err }
        return controller.RemoveFinalizerSafely(ctx, r.Client, obj, FinalizerName)
    }

    // 添加Finalizer + 解析引用 + 注册配置
    if err := r.Service.Register(ctx, service.RegisterOptions{
        ResourceType: v1alpha2.SyncResourceMyResource, CloudflareID: id, AccountID: accountID,
        Source: service.Source{Kind: "MyResource", Namespace: obj.Namespace, Name: obj.Name},
        Config: config, Priority: service.PriorityDefault, CredentialsRef: credRef,
    }); err != nil { return ctrl.Result{}, err }
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

1. 创建 `api/v1alpha2/myresource_types.go`
2. `make manifests generate`
3. **🔴 添加到 `config/crd/kustomization.yaml`** (容易遗忘!)
4. 创建 `internal/controller/myresource/controller.go`
5. 创建 `internal/service/myresource/service.go`
6. 创建 `internal/sync/myresource/controller.go`
7. 注册到 `cmd/main.go`
8. 验证: `make build-installer VERSION=x.x.x && grep "myresources" dist/cloudflare-operator-crds.yaml`

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

---

## 代码结构

```
api/v1alpha2/                    # L1&L4: CRD类型
internal/
├── controller/{resource}/       # L2: Resource Controllers
├── service/{resource}/          # L3: Core Services
├── sync/{resource}/             # L5: Sync Controllers (唯一API调用点)
├── clients/cf/                  # L6: Cloudflare API Client
└── credentials/                 # 凭证加载
```

---

## 文档规范

- 中英双语: `docs/{en,zh}/api-reference/{crd}.md`
- 必须包含: Spec/Status表格, 3+示例, Mermaid架构图, 前置条件/限制
- Mermaid布局: 复杂图表用`elk`，简单用`dagre`

---

## 参考

- [统一同步架构设计](docs/design/UNIFIED_SYNC_ARCHITECTURE.md)
- [Cloudflare Zero Trust Docs](https://developers.cloudflare.com/cloudflare-one/)
- [cloudflare-go SDK](https://github.com/cloudflare/cloudflare-go)
