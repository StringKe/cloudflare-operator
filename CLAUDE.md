# CLAUDE.md

本文件为 Claude Code 提供项目开发指南和代码规范。

## 项目概述

**Cloudflare Zero Trust Kubernetes Operator** - 管理 Cloudflare Zero Trust 全套资源的 Kubernetes Operator。

**Fork 来源**: [adyanth/cloudflare-operator](https://github.com/adyanth/cloudflare-operator)

**技术栈**:
- Go 1.24
- Kubebuilder v4
- controller-runtime v0.20
- cloudflare-go SDK

## 当前实现状态

### API Group
- **Group**: `networking.cloudflare-operator.io`
- **版本**: v1alpha1 (deprecated), v1alpha2 (storage version)

### 已实现的 CRD (19个)

| 类别 | CRD | 作用域 | 状态 |
|------|-----|--------|------|
| **凭证** | CloudflareCredentials | Cluster | ✅ 完成 |
| **网络层** | Tunnel | Namespaced | ✅ 完成 |
| | ClusterTunnel | Cluster | ✅ 完成 |
| | VirtualNetwork | Cluster | ✅ 完成 |
| | NetworkRoute | Cluster | ✅ 完成 |
| | WARPConnector | Cluster | ⚠️ 框架完成 |
| **服务层** | TunnelBinding | Namespaced | ✅ 完成 |
| | PrivateService | Namespaced | ✅ 完成 |
| | DNSRecord | Namespaced | ✅ 完成 |
| **身份层** | AccessApplication | Namespaced | ✅ 完成 |
| | AccessGroup | Cluster | ✅ 完成 |
| | AccessServiceToken | Cluster | ✅ 完成 |
| | AccessIdentityProvider | Cluster | ✅ 完成 |
| | AccessTunnel | Namespaced | ⚠️ 框架完成 |
| **设备层** | DevicePostureRule | Cluster | ⚠️ 框架完成 |
| | DeviceSettingsPolicy | Cluster | ⚠️ 框架完成 |
| **网关层** | GatewayRule | Cluster | ⚠️ 框架完成 |
| | GatewayList | Cluster | ⚠️ 框架完成 |
| | GatewayConfiguration | Cluster | ✅ 完成 |

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

---

## 控制器标准模板

### Reconcile 流程

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. 获取资源
    obj := &v1alpha2.MyResource{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        if apierrors.IsNotFound(err) {
            return ctrl.Result{}, nil
        }
        return ctrl.Result{}, err
    }

    // 2. 初始化 API 客户端
    if err := r.initAPIClient(); err != nil {
        r.setCondition(metav1.ConditionFalse, "APIError", err.Error())
        return ctrl.Result{}, err
    }

    // 3. 处理删除
    if obj.GetDeletionTimestamp() != nil {
        return r.handleDeletion()
    }

    // 4. 添加 Finalizer
    if !controllerutil.ContainsFinalizer(obj, FinalizerName) {
        controllerutil.AddFinalizer(obj, FinalizerName)
        if err := r.Update(ctx, obj); err != nil {
            return ctrl.Result{}, err
        }
    }

    // 5. 业务逻辑
    if err := r.reconcile(); err != nil {
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    return ctrl.Result{}, nil
}
```

### 删除处理

```go
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
    if !controllerutil.ContainsFinalizer(obj, FinalizerName) {
        return ctrl.Result{}, nil
    }

    // 1. 从 Cloudflare 删除 (检查 NotFound)
    if err := r.cfAPI.Delete(id); err != nil {
        if !cf.IsNotFoundError(err) {
            r.Recorder.Event(obj, corev1.EventTypeWarning, "DeleteFailed",
                cf.SanitizeErrorMessage(err))
            return ctrl.Result{RequeueAfter: 30 * time.Second}, err
        }
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

---

## 代码结构

```
api/
├── v1alpha1/                 # 旧版 API (deprecated)
└── v1alpha2/                 # 当前存储版本
    ├── tunnel_types.go
    ├── clustertunnel_types.go
    ├── virtualnetwork_types.go
    ├── networkroute_types.go
    ├── privateservice_types.go
    ├── accessapplication_types.go
    └── ...

internal/
├── controller/
│   ├── status.go             # 状态更新辅助函数
│   ├── constants.go          # 常量定义
│   ├── management.go         # 资源管理标记
│   ├── adoption.go           # 资源采用逻辑
│   ├── generic_tunnel_reconciler.go  # Tunnel 共享逻辑
│   ├── tunnel_controller.go
│   ├── tunnelbinding_controller.go
│   ├── virtualnetwork/controller.go
│   ├── networkroute/controller.go
│   ├── privateservice/controller.go
│   └── ...
├── clients/
│   └── cf/                   # Cloudflare API 客户端
│       ├── api.go
│       ├── network.go        # VirtualNetwork, TunnelRoute
│       ├── device.go         # Device 设置
│       └── errors.go         # 错误处理辅助
└── credentials/              # 凭证加载逻辑
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

# E2E 测试
make test-e2e           # Kind 集群 E2E 测试
```

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

- [Cloudflare Zero Trust Docs](https://developers.cloudflare.com/cloudflare-one/)
- [Cloudflare API Reference](https://developers.cloudflare.com/api/)
- [cloudflare-go SDK](https://github.com/cloudflare/cloudflare-go)
- [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime)
