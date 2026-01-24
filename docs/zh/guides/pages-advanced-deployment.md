# Pages 高级部署指南

本指南介绍 Cloudflare Pages 的高级部署功能，包括全新的持久化版本实体模式、直接上传、智能回滚、项目导入和 Web Analytics 集成。

> **版本**: v0.34.0+

## 概述

Cloudflare Operator 提供强大的 Pages 部署管理功能：

| 功能 | 说明 | 使用场景 |
|------|------|----------|
| **持久化版本实体** | 部署作为持久化 K8s 资源 | GitOps 版本管理 |
| **环境控制** | 显式声明 production/preview 环境 | 多环境工作流 |
| **Git 源** | 从 Git 分支/提交部署 | 标准 CI/CD 集成 |
| **Direct Upload** | 从外部源部署静态文件 | CI/CD 流水线、构建产物存储 |
| **Smart Rollback** | 支持多种策略的智能回滚 | 快速从失败部署中恢复 |
| **Project Adoption** | 导入已存在的 Cloudflare Pages 项目 | 将现有项目迁移到 GitOps |
| **Web Analytics** | 自动 Web Analytics 集成 | 监控站点性能 |
| **Force Redeploy** | 无需配置变更触发新部署 | 重新部署相同源 |

---

## 持久化版本实体模式

> **v0.28.0 新特性**

PagesDeployment 已从"一次性操作"模式重新设计为"持久化版本实体"模式。

### 核心概念

| 方面 | 旧设计 | 新设计 |
|------|--------|--------|
| **模式** | 一次性操作 (Action: create/retry/rollback) | 持久化版本实体 |
| **状态** | 执行后变成死状态 | 持续存在，代表一个部署版本 |
| **数量** | 每次操作一个 | 每个 Project 可有多个 |
| **环境** | 仅在 Status 中 | 在 Spec 中声明 (production/preview) |
| **源** | 混合的 Branch + DirectUpload | 统一的 source 结构 (git/directUpload) |

### Production 唯一性

**重要**：每个 PagesProject 同一时间只能有**一个** production 环境的 PagesDeployment。

```
my-app-project/
├── my-app-prod           # environment: production ✓
├── my-app-preview-v1     # environment: preview ✓
├── my-app-preview-v2     # environment: preview ✓
└── my-app-prod-v2        # environment: production ✗ (拒绝 - 已有 production)
```

### Production 删除保护

删除唯一的 production PagesDeployment 会被阻止，以防止意外的生产中断。你必须：
- 先创建新的 production 部署
- 或者删除整个 PagesProject

---

## 架构

```mermaid
---
config:
  layout: elk
---
flowchart TB
    subgraph Sources["部署源"]
        GIT["Git 分支/提交"]
        HTTP["HTTP/HTTPS URL"]
        S3["S3/R2/MinIO"]
        OCI["OCI 镜像仓库"]
    end

    subgraph Operator["Cloudflare Operator"]
        PD["PagesDeployment 控制器"]
        UP["Uploader 模块"]
        CS["校验和验证"]
        AR["归档解压"]
        MD5["MD5 哈希生成"]
    end

    subgraph CloudflareAPI["Cloudflare Pages API"]
        GITDEP["Git 部署 API"]
        JWT["1. 获取上传令牌 (JWT)"]
        CHK["2. 检查缺失资源"]
        UPL["3. 上传缺失文件"]
        UPS["4. 注册哈希"]
        DEP["5. 创建部署"]
    end

    subgraph Cloudflare["Cloudflare"]
        PP["Pages 项目"]
        DEPLOY["部署"]
    end

    GIT --> PD
    HTTP --> UP
    S3 --> UP
    OCI --> UP

    PD --> GITDEP
    UP --> CS
    CS --> AR
    AR --> MD5
    MD5 --> PD
    PD --> JWT
    JWT --> CHK
    CHK --> UPL
    UPL --> UPS
    UPS --> DEP
    GITDEP --> PP
    DEP --> PP
    PP --> DEPLOY
```

---

## PagesDeployment Spec

### 新格式（推荐）

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-prod
spec:
  projectRef:
    name: my-pages-project

  # 新字段：部署环境（必需）
  environment: production  # 或 "preview"

  # 新字段：统一的源配置（必需）
  source:
    type: git  # 或 "directUpload"
    git:
      branch: main
      commitSha: "abc123def456"  # 可选：指定提交

  purgeBuildCache: false

  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

### Spec 字段

| 字段 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `projectRef` | object | 是 | 引用 PagesProject |
| `environment` | string | 是* | `production` 或 `preview` |
| `source` | object | 是* | 部署源配置 |
| `purgeBuildCache` | bool | 否 | 部署前清除构建缓存 |
| `cloudflare` | object | 是 | Cloudflare 账户和凭证 |

*新格式必需。旧字段仍然支持以保持向后兼容。

### 源类型

#### Git 源

```yaml
source:
  type: git
  git:
    branch: main           # 要部署的分支
    commitSha: "abc123"    # 可选：指定提交 SHA
```

#### Direct Upload 源

```yaml
source:
  type: directUpload
  directUpload:
    source:
      http:
        url: "https://example.com/dist.tar.gz"
    archive:
      type: tar.gz
    checksum:
      algorithm: sha256
      value: "e3b0c44..."
```

---

## Status 字段

PagesDeployment status 现在包含丰富的部署信息：

```yaml
status:
  # 核心字段
  deploymentId: "abc123def456"
  projectName: "my-pages-project"
  state: Succeeded        # Pending, Building, Succeeded, Failed

  # URL 字段
  url: "my-pages-project.pages.dev"
  hashUrl: "abc123def.my-pages-project.pages.dev"
  branchUrl: "main.my-pages-project.pages.dev"

  # 环境字段
  environment: production
  isCurrentProduction: true

  # 版本追踪
  version: 5
  versionName: "v1.2.3"   # 来自标签或部署名称
  sourceDescription: "git:main@abc123"

  # 阶段追踪
  stage: deploy          # queued, initialize, clone_repo, build, deploy

  conditions:
    - type: Ready
      status: "True"
      reason: DeploymentSucceeded
```

### Status 字段说明

| 字段 | 说明 |
|------|------|
| `deploymentId` | Cloudflare 部署 ID |
| `hashUrl` | **基于哈希的唯一 URL**（如 `<hash>.project.pages.dev`）- 不可变引用 |
| `branchUrl` | 基于分支的 URL（如 `<branch>.project.pages.dev`） |
| `environment` | 部署环境（production/preview） |
| `isCurrentProduction` | 是否为当前活跃的生产部署 |
| `version` | 项目内的顺序版本号 |
| `versionName` | **人类可读的版本标识符**（来自 `networking.cloudflare-operator.io/version` 标签或部署名称） |
| `sourceDescription` | 人类可读的源描述 |

### 版本追踪

要为外部应用程序追踪版本名称，添加 `networking.cloudflare-operator.io/version` 标签：

```yaml
metadata:
  name: my-app-deploy-v1-2-3
  labels:
    networking.cloudflare-operator.io/version: "v1.2.3"
```

此标签值会自动存储在 `status.versionName` 中。如果没有设置此标签，则使用部署名称作为版本名称。

**读取版本信息：**

```bash
# 获取版本名称
kubectl get pagesdeployment my-app-deploy -o jsonpath='{.status.versionName}'

# 获取 hash URL（此部署的不可变引用）
kubectl get pagesdeployment my-app-deploy -o jsonpath='{.status.hashUrl}'
```

---

## Git 源部署

直接从 Git 分支或提交部署：

### 基本 Git 部署

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-prod
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

### 部署特定提交

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-hotfix
spec:
  projectRef:
    name: my-app
  environment: preview
  source:
    type: git
    git:
      branch: main
      commitSha: "abc123def456789"  # 部署此特定提交
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

### 从特性分支部署预览

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-feature-x
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

---

## 直接上传 (Direct Upload)

直接上传允许无需 Git 仓库即可部署静态文件。适用于：
- 单独构建产物的 CI/CD 流水线
- 存储在对象存储中的预构建静态站点
- 无法访问 Git 的隔离环境

### Direct Upload API 流程

Operator 使用 Cloudflare 的 Direct Upload API，采用 4 步流程：

1. **获取上传令牌**: 获取用于资源 API 认证的 JWT 令牌
2. **检查缺失资源**: 查询哪些文件（按 MD5 哈希）尚未上传
3. **上传缺失文件**: 仅上传 Cloudflare 没有的文件（base64 编码）
4. **注册哈希**: 为部署注册所有文件哈希
5. **创建部署**: 使用清单（路径 → MD5 哈希映射）创建部署

**重要说明**:
- 文件使用 **MD5 哈希**（非 SHA256）标识，这是 Cloudflare API 的要求
- Pages 特殊配置文件（`_headers`、`_redirects`、`_worker.js`、`_routes.json`）不包含在清单中
- 文件以每批 100 个的方式上传以提高效率
- 同时支持 API Token 和 Global API Key 两种认证方式

### 支持的源类型

| 源类型 | 说明 | 认证方式 |
|--------|------|----------|
| **HTTP/HTTPS** | 任何可访问的 URL | Headers、Bearer token |
| **S3** | AWS S3、MinIO、R2 | Access key/secret |
| **OCI** | 容器镜像仓库 | Docker config、基本认证 |

### HTTP 源

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-deploy
spec:
  projectRef:
    name: my-app
  environment: production
  source:
    type: directUpload
    directUpload:
      source:
        http:
          url: "https://artifacts.example.com/builds/my-app/latest.tar.gz"
          headers:
            Authorization: "Bearer ${CI_TOKEN}"
          timeout: "10m"
          insecureSkipVerify: false  # 仅用于测试
      archive:
        type: tar.gz
        stripComponents: 1
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

#### HTTP 源配置

| 字段 | 类型 | 必需 | 默认值 | 说明 |
|------|------|------|--------|------|
| `url` | string | 是 | - | 获取文件的 HTTP/HTTPS URL |
| `headers` | map | 否 | - | 请求中包含的 Headers |
| `headersSecretRef` | object | 否 | - | 包含 Headers 的 Secret 引用 |
| `timeout` | duration | 否 | `5m` | 请求超时时间 |
| `insecureSkipVerify` | bool | 否 | `false` | 跳过 TLS 证书验证 |

### S3 源

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-s3-deploy
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
          key: builds/my-app/v1.2.3/dist.tar.gz
          region: us-east-1
          credentialsSecretRef:
            name: aws-credentials
      checksum:
        algorithm: sha256
        value: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
      archive:
        type: tar.gz
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

#### S3 源配置

| 字段 | 类型 | 必需 | 默认值 | 说明 |
|------|------|------|--------|------|
| `bucket` | string | 是 | - | S3 存储桶名称 |
| `key` | string | 是 | - | 存储桶中的对象键（路径） |
| `region` | string | 否 | - | S3 区域（AWS 必需） |
| `endpoint` | string | 否 | - | S3 兼容服务的自定义端点 |
| `credentialsSecretRef` | object | 否 | - | 凭证 Secret 引用 |
| `usePathStyle` | bool | 否 | `false` | 使用路径样式寻址 |

#### S3 凭证 Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: aws-credentials
type: Opaque
stringData:
  accessKeyId: "AKIAIOSFODNN7EXAMPLE"
  secretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
  # sessionToken: "可选的会话令牌"
```

### S3 兼容服务

#### Cloudflare R2

```yaml
source:
  type: directUpload
  directUpload:
    source:
      s3:
        bucket: my-build-artifacts
        key: dist.tar.gz
        endpoint: "https://YOUR_ACCOUNT_ID.r2.cloudflarestorage.com"
        credentialsSecretRef:
          name: r2-credentials
        usePathStyle: true  # R2 必需
```

#### MinIO

```yaml
source:
  type: directUpload
  directUpload:
    source:
      s3:
        bucket: builds
        key: my-app/dist.tar.gz
        endpoint: "https://minio.internal.example.com"
        region: us-east-1  # MinIO 也需要
        credentialsSecretRef:
          name: minio-credentials
        usePathStyle: true
```

### OCI 源

OCI（Open Container Initiative）镜像仓库可以存储任意产物，适合存储构建产物。

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-oci-deploy
spec:
  projectRef:
    name: my-app
  environment: production
  source:
    type: directUpload
    directUpload:
      source:
        oci:
          image: "ghcr.io/my-org/my-app-dist:v1.2.3"
          credentialsSecretRef:
            name: ghcr-credentials
      archive:
        type: tar.gz
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

#### OCI 凭证 Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: ghcr-credentials
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: |
    eyJhdXRocyI6eyJnaGNyLmlvIjp7InVzZXJuYW1lIjoiPHVzZXI+IiwicGFzc3dvcmQiOiI8dG9rZW4+In19fQ==
```

或使用基本认证：

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: registry-credentials
type: Opaque
stringData:
  username: "my-username"
  password: "my-token"
```

### 校验和验证

在部署前验证文件完整性：

```yaml
source:
  type: directUpload
  directUpload:
    source:
      http:
        url: "https://example.com/dist.tar.gz"
    checksum:
      algorithm: sha256  # sha256（默认）、sha512、md5
      value: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
```

如果校验和不匹配，部署将失败并报错。

### 归档解压

配置如何解压下载的归档：

```yaml
source:
  type: directUpload
  directUpload:
    archive:
      type: tar.gz        # tar.gz、tar、zip、none
      stripComponents: 1  # 移除顶层目录
      subPath: "dist"     # 仅解压此子目录
```

#### 归档选项

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `type` | string | `tar.gz` | 归档格式：`tar.gz`、`tar`、`zip`、`none` |
| `stripComponents` | int | `0` | 移除前 N 层路径组件 |
| `subPath` | string | - | 仅解压此路径下的文件 |

#### 示例：解压子目录

如果你的归档包含：

```
my-app-v1.0.0/
├── dist/
│   ├── index.html
│   └── assets/
├── src/
└── README.md
```

要仅部署 `dist/` 目录的内容：

```yaml
archive:
  type: tar.gz
  stripComponents: 1   # 移除 "my-app-v1.0.0/"
  subPath: "dist"      # 仅 dist/ 下的文件
```

结果：`index.html` 和 `assets/` 被部署到根目录。

### 强制重新部署

当源配置未更改时，可使用 `cloudflare-operator.io/force-redeploy` 注解触发新部署：

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-deploy
  annotations:
    cloudflare-operator.io/force-redeploy: "2025-01-20-v1"  # 修改此值触发重新部署
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

**工作原理**：
- 当注解值变更时，Operator 检测到配置变化
- 即使 S3 key 或其他设置保持不变，也会创建新部署
- 适用于源文件在相同 URL 已更新的场景

**使用场景**：
- CI/CD 流水线始终上传到相同 S3 key（如 `latest/dist.tar.gz`）
- 基础设施变更后强制重新部署
- GitOps 工作流中的手动发布触发

---

## 智能回滚 (Smart Rollback)

智能回滚提供三种策略的部署回滚：

| 策略 | 说明 | 使用场景 |
|------|------|----------|
| `LastSuccessful` | 回滚到最后一次成功的部署 | 快速恢复 |
| `ByVersion` | 回滚到特定版本号 | 精确控制 |
| `ExactDeploymentID` | 回滚到特定的 Cloudflare 部署 | 最大精度 |

> **注意**：回滚功能使用旧字段（`action: rollback`）。对于新部署，建议创建一个指向目标源的新 PagesDeployment。

### 回滚策略

```mermaid
---
config:
  layout: dagre
---
flowchart LR
    subgraph History["部署历史"]
        V5["v5: 失败"]
        V4["v4: 成功 ✓"]
        V3["v3: 成功 ✓"]
        V2["v2: 失败"]
        V1["v1: 成功 ✓"]
    end

    subgraph Strategies["回滚策略"]
        LS["LastSuccessful → v4"]
        BV["ByVersion(3) → v3"]
        EID["ExactDeploymentID → 任意"]
    end

    V4 --> LS
    V3 --> BV
```

### LastSuccessful 策略

自动查找并回滚到最近一次成功的部署：

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: rollback-to-last
spec:
  projectRef:
    name: my-app
  action: rollback
  rollback:
    strategy: LastSuccessful
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

### ByVersion 策略

回滚到部署历史中的特定版本：

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: rollback-to-v5
spec:
  projectRef:
    name: my-app
  action: rollback
  rollback:
    strategy: ByVersion
    version: 5
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

### ExactDeploymentID 策略

回滚到特定的 Cloudflare 部署 ID：

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: rollback-to-exact
spec:
  projectRef:
    name: my-app
  action: rollback
  rollback:
    strategy: ExactDeploymentID
    deploymentId: "abc123def456"
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

### 部署历史

PagesProject 跟踪用于回滚的部署历史，采用 **FIFO（先进先出）保留策略**：

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesProject
metadata:
  name: my-app
spec:
  name: my-app
  productionBranch: main
  deploymentHistoryLimit: 100  # 最大值：200（FIFO 保留）
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

**历史配置**：

| 字段 | 默认值 | 最大值 | 说明 |
|------|--------|--------|------|
| `deploymentHistoryLimit` | 200 | 200 | 保留的部署记录数量 |

**历史记录字段**：

每个部署历史条目包含：
- `deploymentId`：Cloudflare 部署 ID
- `version`：顺序版本号（从 1 开始）
- `url`：部署 URL
- `source`：源描述（如 `git:main`、`direct-upload`、`rollback:v5`）
- `sourceHash`：源包的 SHA-256 哈希（用于直接上传）
- `sourceUrl`：获取源的 URL（用于直接上传）
- `k8sResource`：创建此部署的 K8s 资源（`namespace/name`）
- `createdAt`：部署创建时间戳
- `status`：部署状态（`active`、`failed`、`superseded`）
- `isProduction`：是否为当前生产部署

查看部署历史：

```bash
# 查看完整历史
kubectl get pagesproject my-app -o jsonpath='{.status.deploymentHistory}' | jq

# 查看最后成功部署 ID
kubectl get pagesproject my-app -o jsonpath='{.status.lastSuccessfulDeploymentId}'

# 查看最新部署信息
kubectl get pagesproject my-app -o jsonpath='{.status.latestDeployment}' | jq
```

---

## 从旧格式迁移

### 旧格式（已废弃）

```yaml
# 旧格式 - 已废弃
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-deploy
spec:
  projectRef:
    name: my-app
  action: create           # 已废弃
  branch: main             # 已废弃
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

### 新格式（推荐）

```yaml
# 新格式 - 推荐
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-prod
spec:
  projectRef:
    name: my-app
  environment: production  # 新字段：显式环境
  source:                  # 新字段：统一源配置
    type: git
    git:
      branch: main
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

### 自动转换

控制器会自动将旧格式转换为新格式：

| 旧字段 | 转换为 |
|--------|--------|
| `action: create` + `branch` | `environment: preview` + `source.type: git` |
| `action: create` + `directUpload` | `environment: preview` + `source.type: directUpload` |
| `action: retry` | （作为旧格式处理） |
| `action: rollback` | （作为旧格式处理） |

使用旧字段时会记录废弃警告事件。

### 迁移检查清单

1. 将 `action: create` 替换为显式 `environment` 字段
2. 将 `branch` 移动到 `source.git.branch`
3. 将 `directUpload` 移动到 `source.directUpload`
4. 设置适当的 `environment`（production 或 preview）
5. 确保每个项目只有一个 production 部署

---

## 项目导入 (Project Adoption)

项目导入允许将已存在的 Cloudflare Pages 项目纳入 Kubernetes 管理。

### 导入策略

| 策略 | 说明 | 使用场景 |
|------|------|----------|
| `MustNotExist` | 如果项目存在则失败（默认） | 创建新项目 |
| `IfExists` | 存在则采纳，不存在则创建 | 灵活迁移 |
| `MustExist` | 要求项目必须存在 | 导入现有项目 |

### 导入现有项目

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesProject
metadata:
  name: existing-project
spec:
  name: existing-project  # 必须与 Cloudflare 项目名称匹配
  productionBranch: main
  adoptionPolicy: MustExist  # 要求项目必须存在
  deploymentHistoryLimit: 20
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

导入后：
- 项目现在由 Operator 管理
- 原始配置存储在 `status.originalConfig` 中
- `status.adopted: true` 表示项目已被采纳
- `status.adoptedAt` 显示采纳时间

### 灵活导入

对于项目可能存在也可能不存在的 GitOps 工作流，使用 `IfExists`：

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesProject
metadata:
  name: my-app
spec:
  name: my-app
  productionBranch: main
  adoptionPolicy: IfExists  # 存在则采纳，不存在则创建
  buildConfig:
    buildCommand: npm run build
    destinationDir: dist
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

### 原始配置

采纳项目时，原始 Cloudflare 配置会被保留：

```bash
kubectl get pagesproject existing-project -o jsonpath='{.status.originalConfig}' | jq
```

示例输出：

```json
{
  "productionBranch": "main",
  "subdomain": "existing-project",
  "source": {
    "type": "github",
    "github": {
      "owner": "my-org",
      "repo": "my-repo"
    }
  },
  "capturedAt": "2025-01-19T12:00:00Z"
}
```

---

## Web Analytics

PagesProject 支持自动 Cloudflare Web Analytics 集成：

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesProject
metadata:
  name: my-app
spec:
  name: my-app
  productionBranch: main
  enableWebAnalytics: true  # 默认启用
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

**行为**：
- 当 `enableWebAnalytics: true`（默认）时，Web Analytics 自动为 `*.pages.dev` 域名启用
- 启用自动安装，自动注入分析脚本
- 对于自定义域名，需通过 PagesDomain 或 Cloudflare 控制台单独配置 Web Analytics

**禁用 Web Analytics**：

```yaml
spec:
  enableWebAnalytics: false
```

---

## 自定义域名 (PagesDomain)

PagesDomain 管理 Pages 项目的自定义域名，支持自动 DNS 配置。

### DNS 自动配置

当域名的 zone 在同一 Cloudflare 账户时，DNS 记录可自动配置：

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDomain
metadata:
  name: my-app-domain
spec:
  domain: app.example.com
  projectRef:
    name: my-app
  autoConfigureDNS: true  # 默认：true - Cloudflare 自动配置 DNS
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

**DNS 配置模式**：

| 模式 | `autoConfigureDNS` | 行为 |
|------|-------------------|------|
| **自动** | `true`（默认） | Cloudflare 创建/管理 CNAME 记录 |
| **手动** | `false` | 您手动管理 DNS 记录 |

**何时使用手动 DNS**：
- 域名由外部 DNS 提供商管理
- 需要自定义 DNS 配置（如代理记录、TTL 设置）
- 域名 zone 在不同的 Cloudflare 账户
- 使用高级 DNS 功能（如负载均衡）

**手动 DNS 示例**：

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDomain
metadata:
  name: external-domain
spec:
  domain: app.external-dns.com
  projectRef:
    name: my-app
  autoConfigureDNS: false  # DNS 由外部管理
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

对于手动 DNS，创建指向 Pages 子域的 CNAME 记录：
```
app.external-dns.com  CNAME  my-app.pages.dev
```

### 域名验证状态

检查域名验证状态：

```bash
kubectl get pagesdomain my-app-domain -o wide

# 如需手动验证，查看验证数据
kubectl get pagesdomain my-app-domain -o jsonpath='{.status.verificationData}' | jq
```

---

## CI/CD 集成

### GitHub Actions 示例

```yaml
name: 部署到 Cloudflare Pages

on:
  push:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: 构建
        run: npm run build

      - name: 上传到 S3
        run: |
          tar -czf dist.tar.gz -C dist .
          aws s3 cp dist.tar.gz s3://my-artifacts/builds/${{ github.sha }}/dist.tar.gz

      - name: 计算校验和
        id: checksum
        run: echo "sha256=$(sha256sum dist.tar.gz | cut -d' ' -f1)" >> $GITHUB_OUTPUT

  deploy:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - name: 创建部署
        run: |
          cat <<EOF | kubectl apply -f -
          apiVersion: networking.cloudflare-operator.io/v1alpha2
          kind: PagesDeployment
          metadata:
            name: deploy-${{ github.sha }}
            namespace: default
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
                    key: builds/${{ github.sha }}/dist.tar.gz
                    region: us-east-1
                    credentialsSecretRef:
                      name: aws-credentials
                checksum:
                  algorithm: sha256
                  value: "${{ needs.build.outputs.sha256 }}"
                archive:
                  type: tar.gz
            cloudflare:
              accountId: "${{ secrets.CF_ACCOUNT_ID }}"
              credentialsRef:
                name: cloudflare-credentials
          EOF
```

---

## 故障排除

### 常见问题

#### Production 唯一性错误

```
Error: production environment already exists for project "my-app": default/my-app-prod
```

**原因**：尝试创建第二个 production PagesDeployment。

**解决方案**：每个项目只能有一个 production 部署。可以：
- 先删除现有 production 部署
- 新部署使用 `environment: preview`
- 更新现有 production 部署

#### Production 删除被阻止

```
Error: cannot delete the only production deployment, create a new production deployment first
```

**原因**：尝试删除唯一的 production PagesDeployment。

**解决方案**：先创建新的 production 部署再删除现有的。

#### 校验和不匹配

```
Error: checksum mismatch: expected abc123..., got def456...
```

**原因**：下载的文件与预期的校验和不匹配。

**解决方案**：验证校验和值对于你下载的文件是否正确。

#### 归档解压失败

```
Error: archive has no files after extraction
```

**原因**：归档为空或 `subPath` 不匹配任何文件。

**解决方案**：检查 `stripComponents` 和 `subPath` 配置。

#### S3 访问被拒绝

```
Error: get S3 object: AccessDenied
```

**原因**：权限不足或凭证不正确。

**解决方案**：验证凭证 Secret 和 S3 存储桶策略。

#### 回滚目标未找到

```
Error: deployment version 10 not found in history
```

**原因**：请求的版本不在部署历史中。

**解决方案**：使用 `kubectl get pagesproject my-app -o jsonpath='{.status.deploymentHistory}'` 检查可用版本

### 调试命令

```bash
# 检查部署状态
kubectl get pagesdeployment my-app-deploy -o wide

# 查看详细事件
kubectl describe pagesdeployment my-app-deploy

# 检查 Operator 日志
kubectl logs -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager | grep -i pages

# 查看项目状态
kubectl get pagesproject my-app -o yaml

# 列出项目的所有部署
kubectl get pagesdeployment -l cloudflare-operator.io/project=my-app
```

---

## 相关文档

| 主题 | 链接 |
|------|------|
| Pages 示例 | [examples/06-pages/](../../../examples/06-pages/) |
| 配置 | [configuration.md](../configuration.md) |
| 故障排除 | [troubleshooting.md](../troubleshooting.md) |
| API 参考 | [api-reference/](../api-reference/) |

---

**上一步**：[快速开始](../getting-started.md)

**下一步**：[Istio 集成](istio-integration.md)
