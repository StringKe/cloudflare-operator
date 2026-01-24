# PagesProject 版本化部署功能

## 概述

PagesProject 版本化部署功能为 Cloudflare Pages 项目提供声明式的多版本管理能力，支持：

- ✅ 声明式版本列表 (`spec.versions[]`)
- ✅ 自动创建 PagesDeployment 资源
- ✅ 生产目标控制 (`spec.productionTarget`)
- ✅ 版本历史限制 (`spec.revisionHistoryLimit`)
- ✅ 自动清理旧版本
- ✅ Webhook 验证

## 功能特性

### 1. 声明式版本管理

在 `spec.versions` 中声明所有需要部署的版本，Controller 自动为每个版本创建对应的 PagesDeployment：

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesProject
metadata:
  name: my-app
spec:
  name: "my-app"
  productionBranch: "main"

  # 声明版本列表
  versions:
    - name: "v1.2.3"
      source:
        source:
          http:
            url: "https://example.com/dist.tar.gz"
        archive:
          type: tar.gz
        checksum:
          algorithm: sha256
          value: "abc123..."
      metadata:
        gitCommit: "abc123"
        buildTime: "2025-01-20T10:00:00Z"

    - name: "v1.2.2"
      source:
        source:
          http:
            url: "https://example.com/v1.2.2/dist.tar.gz"
```

### 2. 生产目标控制

通过 `productionTarget` 控制哪个版本作为生产部署：

```yaml
# 自动部署最新版本
productionTarget: "latest"

# 锁定特定版本（用于回滚）
productionTarget: "v1.2.2"

# 不自动提升生产
productionTarget: ""
```

### 3. 托管部署标识

Controller 创建的 PagesDeployment 会自动添加标签和 ownerReference：

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-v1.2.3
  labels:
    networking.cloudflare-operator.io/managed-by: pagesproject
    networking.cloudflare-operator.io/managed-by-name: my-app
    networking.cloudflare-operator.io/managed-by-uid: "xxx"
    networking.cloudflare-operator.io/version: v1.2.3  # 版本名称，会自动写入 status.versionName
  ownerReferences:
    - apiVersion: networking.cloudflare-operator.io/v1alpha2
      kind: PagesProject
      name: my-app
      uid: xxx
      controller: true
```

**版本追踪**：`networking.cloudflare-operator.io/version` 标签的值会自动写入 `status.versionName`，供外部应用读取部署版本信息。

### 4. 自动清理历史

基于 `revisionHistoryLimit` 自动清理旧版本：

```yaml
# 保留最近 10 个部署
revisionHistoryLimit: 10
```

**清理规则**:
- 生产部署永不删除
- 按创建时间排序（生产优先，其次按时间降序）
- 超出限制的非生产部署自动删除

### 5. 状态聚合

Status 中聚合所有托管版本的状态：

```yaml
status:
  # 当前生产部署信息
  currentVersion: "v1.2.3"  # 当前生产版本名称
  currentProduction:
    version: "v1.2.3"
    deploymentId: "cf-abc123"
    deploymentName: "my-app-v1.2.3"
    url: "my-app.pages.dev"
    hashUrl: "abc123.my-app.pages.dev"  # 不可变的 hash URL
    deployedAt: "2025-01-20T10:00:00Z"

  # 托管部署数量
  managedDeployments: 3

  # 所有托管版本状态
  managedVersions:
    - name: "v1.2.3"
      deploymentName: "my-app-v1.2.3"
      state: "Succeeded"
      isProduction: true
      deploymentId: "cf-abc123"
    - name: "v1.2.2"
      deploymentName: "my-app-v1.2.2"
      state: "Succeeded"
      isProduction: false
      deploymentId: "cf-def456"
```

## 使用场景

### 场景 1: 持续部署最新版本

```yaml
spec:
  versions:
    - name: "v1.2.3"
      source: {...}
  productionTarget: "latest"
```

每次添加新版本到列表顶部，自动部署到生产环境。

### 场景 2: 回滚到稳定版本

```yaml
spec:
  versions:
    - name: "v1.2.3"
      source: {...}
    - name: "v1.2.2"
      source: {...}
  # 从 "latest" 改为锁定版本
  productionTarget: "v1.2.2"
```

只需修改 `productionTarget`，无需删除或重建资源。

### 场景 3: 金丝雀发布

```yaml
# Step 1: 部署新版本到 preview
spec:
  versions:
    - name: "v1.3.0"
      source: {...}
    - name: "v1.2.3"
      source: {...}
  productionTarget: "v1.2.3"  # 保持旧版本为生产

# Step 2: 验证 preview 无问题后切换
spec:
  productionTarget: "v1.3.0"  # 切换到新版本
```

## Webhook 验证

自动验证配置正确性：

### 验证规则

1. **生产目标验证**: `productionTarget` 必须引用存在的版本
2. **版本唯一性**: `spec.versions` 中所有版本名称必须唯一

### 示例

```yaml
# ✅ 正确
spec:
  versions:
    - name: "v1.0.0"
    - name: "v0.9.0"
  productionTarget: "v1.0.0"

# ❌ 错误: 版本不存在
spec:
  versions:
    - name: "v1.0.0"
  productionTarget: "v2.0.0"  # 验证失败

# ❌ 错误: 重复版本名
spec:
  versions:
    - name: "v1.0.0"
    - name: "v1.0.0"  # 验证失败
```

## 托管 vs 用户部署

### 托管部署（Managed）

- 由 PagesProject 自动创建
- 带有 `managed-by: pagesproject` 标签
- 有 ownerReference（级联删除）
- 命名规则：`<project-name>-<version-name>`
- 受 `revisionHistoryLimit` 管理

### 用户部署（User-created）

- 用户手动创建
- 无特殊标签
- 无 ownerReference
- 任意命名
- 完全独立管理

**两者可以共存**，互不干扰。

## 生产部署冲突处理

### 规则

1. **托管部署之间**: 允许临时共存，PagesProject controller 会自动协调
2. **托管 vs 用户部署**: 如果用户手动创建了生产部署，新的托管部署允许共存（PagesProject 负责协调）
3. **用户部署之间**: 仍然保持原有的唯一性检查

### 实现

PagesDeployment Controller 已修改 `validateProductionUniqueness`，识别托管部署并允许共存。

## 架构组件

### 新增文件

| 文件 | 职责 |
|------|------|
| `api/v1alpha2/pagesproject_types.go` | 新增字段定义 |
| `api/v1alpha2/pagesproject_webhook.go` | Webhook 验证 |
| `internal/controller/pagesproject/constants.go` | 标签常量 |
| `internal/controller/pagesproject/version_manager.go` | 版本管理器 |
| `internal/controller/pagesproject/production_reconciler.go` | 生产目标协调 |
| `internal/controller/pagesproject/pruner.go` | 历史清理 |
| `internal/controller/pagesproject/status_aggregator.go` | 状态聚合 |
| `internal/webhook/v1alpha2/pagesproject_webhook.go` | Webhook 注册 |

### 修改文件

| 文件 | 修改内容 |
|------|----------|
| `internal/controller/pagesproject/controller.go` | 集成版本管理逻辑 |
| `internal/controller/pagesdeployment/controller.go` | 允许托管部署共存 |
| `cmd/main.go` | 注册 PagesProject webhook |

## 示例配置

完整示例请参考：
- `config/samples/networking_v1alpha2_pagesproject_versions.yaml`

## 向后兼容

- ✅ 不使用 `versions` 字段时，行为与现有完全一致
- ✅ 现有 PagesProject 无需修改
- ✅ 用户可渐进式迁移到托管模式

## 测试

### 单元测试

```bash
# 运行 webhook 验证测试
go test ./api/v1alpha2 -v -run TestPagesProjectValidator
```

### E2E 测试

```bash
# 1. 部署 Operator
make deploy

# 2. 创建测试资源
kubectl apply -f config/samples/networking_v1alpha2_pagesproject_versions.yaml

# 3. 验证托管部署
kubectl get pagesdeployment -l networking.cloudflare-operator.io/managed-by=pagesproject

# 4. 验证生产切换
kubectl patch pagesproject my-app --type=merge -p '{"spec":{"productionTarget":"v1.2.2"}}'
kubectl get pagesdeployment my-app-v1.2.2 -o jsonpath='{.spec.environment}'

# 5. 检查状态
kubectl get pagesproject my-app -o yaml | grep -A 10 "currentProduction"
```

## 故障排查

### 问题：版本未自动创建 PagesDeployment

**检查**:
```bash
kubectl describe pagesproject <name>
kubectl logs -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager
```

### 问题：生产目标未切换

**检查**:
```bash
# 查看 Events
kubectl describe pagesproject <name>

# 确认目标版本存在
kubectl get pagesdeployment -l networking.cloudflare-operator.io/version=<target-version>
```

### 问题：旧版本未清理

**检查**:
```bash
# 查看托管部署数量
kubectl get pagesdeployment -l networking.cloudflare-operator.io/managed-by=pagesproject

# 检查 revisionHistoryLimit
kubectl get pagesproject <name> -o jsonpath='{.spec.revisionHistoryLimit}'
```

## 性能考虑

- **Reconcile 频率**: 每次 Spec 变更触发一次
- **状态聚合**: 每次 Reconcile 执行，开销较小
- **历史清理**: 每次 Reconcile 执行，仅在超出限制时删除
- **Watch 负载**: Owns() 机制自动过滤，不会产生额外 reconcile

## 安全考虑

- ✅ Webhook 验证确保配置合法
- ✅ ownerReference 确保级联删除安全
- ✅ 标签隔离托管和用户部署
- ✅ 生产部署有安全保护（永不被清理）
