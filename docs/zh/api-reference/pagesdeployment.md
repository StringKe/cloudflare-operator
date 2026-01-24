# PagesDeployment

PagesDeployment 是一个命名空间作用域的资源，用于管理 Cloudflare Pages 项目部署，支持 Git 源、直接上传和智能回滚功能。

## 概述

PagesDeployment 使您能够从 Kubernetes 将应用程序部署到 Cloudflare Pages。它支持基于 Git 的部署、从 HTTP/S3/OCI 源直接上传构建工件，以及自动回滚到先前版本。

### 主要特性

- 持久化版本实体模型 (v0.28.0+)
- Git 源部署（分支/提交）
- 从 HTTP、S3、OCI 源直接上传
- 校验和验证和归档提取
- 生产/预览环境分离
- 多种回滚策略
- 丰富的状态跟踪，包含 hash URL

## 规范 (Spec)

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `projectRef` | PagesProjectRef | **是** | PagesProject 的引用 |
| `environment` | string | 否 | 部署环境：`production` 或 `preview` |
| `source` | PagesDeploymentSourceSpec | 否 | 部署源（git 或 directUpload） |
| `purgeBuildCache` | bool | 否 | 部署前清除构建缓存 |
| `cloudflare` | CloudflareDetails | **是** | API 凭证 |
| `branch` | string | 否 | *已弃用*：使用 `source.git.branch` |
| `action` | string | 否 | *已弃用*：使用 `environment` 和 `source` |
| `directUpload` | PagesDirectUpload | 否 | *已弃用*：使用 `source.directUpload` |
| `rollback` | RollbackConfig | 否 | *已弃用*：回滚配置 |

### 源类型

#### Git 源

```yaml
source:
  type: git
  git:
    branch: main           # 要部署的分支
    commitSha: "abc123"    # 可选：特定提交 SHA
```

#### 直接上传源

```yaml
source:
  type: directUpload
  directUpload:
    source:
      http:                # 或 s3: 或 oci:
        url: "https://example.com/dist.tar.gz"
    checksum:
      algorithm: sha256
      value: "e3b0c44..."
    archive:
      type: tar.gz
      stripComponents: 1
```

## 状态 (Status)

| 字段 | 类型 | 描述 |
|------|------|------|
| `deploymentId` | string | Cloudflare 部署 ID |
| `projectName` | string | Cloudflare 项目名称 |
| `accountId` | string | Cloudflare 账户 ID |
| `url` | string | 主部署 URL |
| `hashUrl` | string | **唯一的 hash URL**（不可变，如 `<hash>.<project>.pages.dev`） |
| `branchUrl` | string | 分支 URL（如 `<branch>.<project>.pages.dev`） |
| `environment` | string | 部署环境（production/preview） |
| `isCurrentProduction` | bool | 是否为当前生产部署 |
| `version` | int | 项目内的顺序版本号 |
| `versionName` | string | **可读的版本标识符**（来自标签或部署名称） |
| `productionBranch` | string | 使用的生产分支 |
| `stage` | string | 当前部署阶段 |
| `stageHistory` | []PagesStageHistory | 部署阶段历史 |
| `buildConfig` | PagesBuildConfigStatus | 使用的构建配置 |
| `source` | PagesDeploymentSource | 部署源信息 |
| `sourceDescription` | string | 可读的源描述 |
| `state` | PagesDeploymentState | 当前状态（Pending/Queued/Building/Deploying/Succeeded/Failed/Cancelled） |
| `conditions` | []Condition | 标准 Kubernetes 条件 |
| `observedGeneration` | int64 | 最后观察到的 generation |
| `message` | string | 额外的状态信息 |
| `startedAt` | Time | 部署开始时间 |
| `finishedAt` | Time | 部署完成时间 |

### Status URL 字段

状态提供三种类型的 URL：

| 字段 | 示例 | 描述 |
|------|------|------|
| `url` | `my-project.pages.dev` | 主项目 URL（用于生产） |
| `hashUrl` | `abc123.my-project.pages.dev` | **此特定部署的不可变 URL** |
| `branchUrl` | `main.my-project.pages.dev` | 分支 URL（随新部署更新） |

**重要提示**：`hashUrl` 是引用特定部署版本最可靠的方式，因为它在部署创建后永远不会改变。

## 示例

### 示例 1：Git 生产部署

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-prod
  namespace: production
  labels:
    networking.cloudflare-operator.io/version: "v1.2.3"
spec:
  projectRef:
    name: my-app
  environment: production
  source:
    type: git
    git:
      branch: main
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

### 示例 2：从 S3 直接上传

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-deploy-sha-abc123
  namespace: production
  labels:
    networking.cloudflare-operator.io/version: "sha-abc123"
spec:
  projectRef:
    name: my-app
  environment: production
  source:
    type: directUpload
    directUpload:
      source:
        s3:
          bucket: my-ci-artifacts
          key: builds/my-app/sha-abc123/dist.tar.gz
          region: us-east-1
          credentialsSecretRef:
            name: aws-credentials
      checksum:
        algorithm: sha256
        value: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
      archive:
        type: tar.gz
        stripComponents: 1
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

### 示例 3：预览部署

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-preview-feature-x
  namespace: staging
spec:
  projectRef:
    name: my-app
  environment: preview
  source:
    type: git
    git:
      branch: feature/new-feature
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

### 示例 4：强制重新部署

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-deploy
  annotations:
    cloudflare-operator.io/force-redeploy: "2026-01-24-v2"  # 更改此值以触发重新部署
spec:
  projectRef:
    name: my-app
  environment: production
  source:
    type: directUpload
    directUpload:
      source:
        s3:
          bucket: my-artifacts
          key: builds/latest/dist.tar.gz
          region: us-east-1
          credentialsSecretRef:
            name: aws-credentials
      archive:
        type: tar.gz
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

## 版本跟踪

### 使用版本标签

添加 `networking.cloudflare-operator.io/version` 标签来跟踪版本名称：

```yaml
metadata:
  name: my-app-deploy-v1-2-3
  labels:
    networking.cloudflare-operator.io/version: "v1.2.3"
```

此标签值存储在 `status.versionName` 中，供外部应用程序读取。

### 读取版本信息

```bash
# 获取版本名称
kubectl get pagesdeployment my-app-deploy -o jsonpath='{.status.versionName}'

# 获取 hash URL（不可变引用）
kubectl get pagesdeployment my-app-deploy -o jsonpath='{.status.hashUrl}'

# 获取所有部署信息
kubectl get pagesdeployment my-app-deploy -o jsonpath='{.status}' | jq
```

## 前置条件

- 已创建 PagesProject 资源
- 有效的 Cloudflare API 凭证
- 对于直接上传：源可访问（HTTP URL、S3 存储桶或 OCI 注册表）

## 相关资源

- [PagesProject](pagesproject.md) - 管理 Pages 项目
- [PagesDomain](pagesdomain.md) - Pages 的自定义域名
- [Pages 高级部署指南](../guides/pages-advanced-deployment.md) - 全面的部署指南

## 另请参阅

- [Cloudflare Pages 部署](https://developers.cloudflare.com/pages/deployments/)
- [直接上传 API](https://developers.cloudflare.com/pages/platform/direct-upload/)
