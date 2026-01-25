# PagesProject 版本管理功能

## 概述

PagesProject 版本管理功能为 Cloudflare Pages 项目提供声明式的多版本管理能力，支持 8 种版本管理策略：

| 策略 | 描述 | 适用场景 |
|------|------|----------|
| `none` | 仅项目配置，无版本管理 | 项目元数据管理 |
| `targetVersion` | 单版本直接发布 | 简单场景 |
| `declarativeVersions` | 版本数组 + 模板 | 批量管理 |
| `fullVersions` | 完整版本配置 | 复杂场景 |
| `gitops` | Preview + Production 两阶段 | **GitOps 工作流** |
| `latestPreview` | 自动追踪最新 preview | 持续部署 |
| `autoPromote` | 成功后自动升级 | 自动化流水线 |
| `external` | 外部系统控制 | 集成第三方 |

## 功能特性

### 1. 版本管理策略配置

通过 `spec.versionManagement.policy` 选择版本管理策略：

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesProject
metadata:
  name: my-app
spec:
  name: "my-app"
  productionBranch: "main"

  versionManagement:
    policy: declarativeVersions  # 选择策略
    declarativeVersions:
      versions:
        - "v1.2.3"
        - "v1.2.2"
      sourceTemplate:
        type: http
        http:
          urlTemplate: "https://example.com/{{.Version}}/dist.tar.gz"
      productionTarget: "latest"
```

### 2. GitOps 两阶段部署

GitOps 模式支持 Preview + Production 两阶段部署：

```yaml
versionManagement:
  policy: gitops
  gitops:
    # CI 系统修改此字段触发 preview 部署
    previewVersion: "v1.3.0"

    # 运维手动修改此字段触发升级（必须已通过 preview）
    productionVersion: "v1.2.3"

    # 源配置模板
    sourceTemplate:
      type: s3
      s3:
        bucket: "my-bucket"
        keyTemplate: "builds/{{.Version}}/dist.tar.gz"
        region: "us-east-1"

    # 要求 preview 验证（默认 true）
    requirePreviewValidation: true

    # 可选：验证标签
    validationLabels:
      qa-approved: "true"
```

### 3. 自动升级模式

AutoPromote 模式支持 preview 成功后自动升级到 production：

```yaml
versionManagement:
  policy: autoPromote
  autoPromote:
    # 成功后等待时间
    promoteAfter: 5m

    # 健康检查
    requireHealthCheck: true
    healthCheckUrl: "https://preview.example.com/health"
    healthCheckTimeout: 30s
```

### 4. 托管部署标识

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
    networking.cloudflare-operator.io/version: v1.2.3
  ownerReferences:
    - apiVersion: networking.cloudflare-operator.io/v1alpha2
      kind: PagesProject
      name: my-app
      uid: xxx
      controller: true
```

### 5. 自动清理历史

基于 `revisionHistoryLimit` 自动清理旧版本：

```yaml
# 保留最近 10 个部署
revisionHistoryLimit: 10
```

**清理规则**:
- 生产部署永不删除
- 按创建时间排序（生产优先，其次按时间降序）
- 超出限制的非生产部署自动删除

### 6. 状态聚合

Status 中聚合所有托管版本的状态：

```yaml
status:
  # 当前活跃策略
  activePolicy: "gitops"

  # 版本映射 (versionName -> deploymentId)
  versionMapping:
    v1.2.3: "cf-abc123"
    v1.2.2: "cf-def456"

  # 当前生产部署信息
  currentProduction:
    version: "v1.2.3"
    deploymentId: "cf-abc123"
    deploymentName: "my-app-v1.2.3"
    url: "my-app.pages.dev"
    hashUrl: "abc123.my-app.pages.dev"
    deployedAt: "2025-01-20T10:00:00Z"

  # 当前预览部署信息
  previewDeployment:
    versionName: "v1.3.0"
    deploymentId: "cf-xyz789"
    deploymentName: "my-app-v1.3.0"
    url: "preview.my-app.pages.dev"
    state: "Succeeded"

  # 验证历史
  validationHistory:
    - versionName: "v1.2.3"
      deploymentId: "cf-abc123"
      validatedAt: "2025-01-20T09:00:00Z"
      validatedBy: "preview"
      validationResult: "passed"

  # 托管部署数量
  managedDeployments: 3

  # 所有托管版本状态
  managedVersions:
    - name: "v1.2.3"
      deploymentName: "my-app-v1.2.3"
      state: "Succeeded"
      isProduction: true
      deploymentId: "cf-abc123"
```

## 使用场景

### 场景 1: GitOps 工作流

```yaml
# Step 1: CI 系统部署新版本到 preview
versionManagement:
  policy: gitops
  gitops:
    previewVersion: "v1.3.0"
    productionVersion: "v1.2.3"

# Step 2: 验证 preview 无问题后，运维升级到 production
versionManagement:
  policy: gitops
  gitops:
    previewVersion: "v1.3.0"
    productionVersion: "v1.3.0"  # 修改此字段
```

### 场景 2: 声明式版本回滚

```yaml
# 从 "latest" 改为锁定版本
versionManagement:
  policy: declarativeVersions
  declarativeVersions:
    versions:
      - "v1.2.3"
      - "v1.2.2"
    productionTarget: "v1.2.2"  # 回滚到 v1.2.2
```

### 场景 3: 自动升级流水线

```yaml
versionManagement:
  policy: autoPromote
  autoPromote:
    promoteAfter: 10m  # preview 成功 10 分钟后自动升级
    requireHealthCheck: true
    healthCheckUrl: "https://preview.example.com/health"
```

### 场景 4: 追踪最新预览

```yaml
versionManagement:
  policy: latestPreview
  latestPreview:
    labelSelector:
      matchLabels:
        team: frontend  # 只追踪特定团队的部署
    autoPromote: true   # 自动升级最新成功的 preview
```

### 场景 5: 外部系统控制

```yaml
versionManagement:
  policy: external
  external:
    currentVersion: "v1.2.3"      # 外部系统更新此字段
    productionVersion: "v1.2.3"   # 外部系统更新此字段
    syncInterval: 5m
    webhookUrl: "https://ci.example.com/webhook"
```

## Webhook 验证

自动验证配置正确性：

### 验证规则

1. **策略验证**: `policy` 必须是有效的策略名称
2. **配置一致性**: 策略对应的配置字段必须存在
3. **模板验证**: `sourceTemplate` 配置必须有效

### 示例

```yaml
# ✅ 正确
versionManagement:
  policy: declarativeVersions
  declarativeVersions:
    versions: ["v1.0.0"]
    sourceTemplate: {...}
    productionTarget: "latest"

# ❌ 错误: 策略和配置不匹配
versionManagement:
  policy: gitops
  declarativeVersions: {...}  # 应该使用 gitops 配置
```

## 架构组件

### 版本管理 Reconciler

| 文件 | 职责 |
|------|------|
| `version_manager.go` | 版本解析和协调 |
| `gitops_reconciler.go` | GitOps 两阶段部署 |
| `latest_preview_reconciler.go` | 追踪最新预览 |
| `auto_promote_reconciler.go` | 自动升级 |
| `external_reconciler.go` | 外部系统控制 |
| `production_reconciler.go` | 生产目标协调 |
| `pruner.go` | 历史清理 |
| `status_aggregator.go` | 状态聚合 |
| `version_index.go` | 版本索引 |

## 示例配置

完整示例请参考：
- `config/samples/networking_v1alpha2_pagesproject_versions.yaml`

## 向后兼容

- ✅ 不配置 `versionManagement` 时，行为与现有完全一致
- ✅ 现有 PagesProject 无需修改
- ✅ 用户可渐进式迁移到版本管理模式

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

# 4. 验证 GitOps 升级
kubectl patch pagesproject my-app-gitops --type=merge \
  -p '{"spec":{"versionManagement":{"gitops":{"productionVersion":"v1.3.0"}}}}'

# 5. 检查状态
kubectl get pagesproject my-app-gitops -o yaml | grep -A 20 "status:"
```

## 故障排查

### 问题：版本未自动创建 PagesDeployment

**检查**:
```bash
kubectl describe pagesproject <name>
kubectl logs -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager
```

### 问题：GitOps 升级失败

**检查**:
```bash
# 确认版本已通过 preview 验证
kubectl get pagesproject <name> -o jsonpath='{.status.validationHistory}'

# 检查 requirePreviewValidation 设置
kubectl get pagesproject <name> -o jsonpath='{.spec.versionManagement.gitops.requirePreviewValidation}'
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
- ✅ 预览验证机制防止未测试代码上线
