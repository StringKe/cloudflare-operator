# Pages 高级部署指南

本指南介绍 Cloudflare Pages 的高级部署功能，包括直接上传、智能回滚和项目导入。

## 概述

Cloudflare Operator 提供三项强大的 Pages 部署管理功能：

| 功能 | 说明 | 使用场景 |
|------|------|----------|
| **Direct Upload** | 从外部源部署静态文件 | CI/CD 流水线、构建产物存储 |
| **Smart Rollback** | 支持多种策略的智能回滚 | 快速从失败部署中恢复 |
| **Project Adoption** | 导入已存在的 Cloudflare Pages 项目 | 将现有项目迁移到 GitOps |

## 架构

```mermaid
---
config:
  layout: elk
---
flowchart TB
    subgraph Sources["外部源"]
        HTTP["HTTP/HTTPS URL"]
        S3["S3/R2/MinIO"]
        OCI["OCI 镜像仓库"]
    end

    subgraph Operator["Cloudflare Operator"]
        PD["PagesDeployment 控制器"]
        UP["Uploader 模块"]
        CS["校验和验证"]
        AR["归档解压"]
    end

    subgraph Cloudflare["Cloudflare"]
        DU["Direct Upload API"]
        PP["Pages 项目"]
        DEPLOY["部署"]
    end

    HTTP --> UP
    S3 --> UP
    OCI --> UP

    UP --> CS
    CS --> AR
    AR --> PD
    PD --> DU
    DU --> PP
    PP --> DEPLOY
```

---

## 直接上传 (Direct Upload)

直接上传允许无需 Git 仓库即可部署静态文件。适用于：
- 单独构建产物的 CI/CD 流水线
- 存储在对象存储中的预构建静态站点
- 无法访问 Git 的隔离环境

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
  action: create
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
  action: create
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
  action: create
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

---

## 智能回滚 (Smart Rollback)

智能回滚提供三种策略的部署回滚：

| 策略 | 说明 | 使用场景 |
|------|------|----------|
| `LastSuccessful` | 回滚到最后一次成功的部署 | 快速恢复 |
| `ByVersion` | 回滚到特定版本号 | 精确控制 |
| `ExactDeploymentID` | 回滚到特定的 Cloudflare 部署 | 最大精度 |

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

PagesProject 跟踪用于回滚的部署历史：

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesProject
metadata:
  name: my-app
spec:
  name: my-app
  productionBranch: main
  deploymentHistoryLimit: 20  # 保留 20 个部署历史（默认：10）
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

查看部署历史：

```bash
kubectl get pagesproject my-app -o jsonpath='{.status.deploymentHistory}' | jq
```

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
            action: create
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
