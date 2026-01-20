# Cloudflare Pages Examples

This directory contains examples for managing Cloudflare Pages resources using the operator.

## Prerequisites

- Cloudflare account with Pages enabled
- API Token with `Account:Cloudflare Pages:Edit` permission

## CRDs

| CRD | Description / 说明 |
|-----|-------------------|
| `PagesProject` | Pages project with build config and resource bindings / Pages 项目，支持构建配置和资源绑定 |
| `PagesDomain` | Custom domain for Pages project / Pages 项目自定义域名 |
| `PagesDeployment` | Deployment operations (create, retry, rollback, direct upload) / 部署操作（创建、重试、回滚、直接上传） |

## New Features (v0.27.13+) / 新功能

| Feature / 功能 | Description / 说明 |
|---------------|-------------------|
| **Direct Upload** | Deploy static files from HTTP/S3/OCI sources without Git / 无需 Git，从 HTTP/S3/OCI 源部署静态文件 |
| **4-Step API Flow** | Correct MD5-based upload flow (v0.27.13) / 正确的基于 MD5 的 4 步上传流程 (v0.27.13) |
| **Smart Rollback** | Intelligent rollback with multiple strategies / 支持多种策略的智能回滚 |
| **Project Adoption** | Import existing Cloudflare Pages projects / 导入已存在的 Cloudflare Pages 项目 |
| **Web Analytics** | Automatic Web Analytics integration / 自动 Web Analytics 集成 |
| **Force Redeploy** | Trigger redeployment without config changes / 无需配置变更触发重新部署 |
| **DNS Auto-Config** | Automatic DNS configuration for custom domains / 自定义域名的自动 DNS 配置 |
| **FIFO History** | FIFO retention policy (max 200 entries) for deployment history / 部署历史的 FIFO 保留策略（最多 200 条） |

### Direct Upload Technical Details (v0.27.13) / 直接上传技术细节

The Direct Upload feature uses Cloudflare's 4-step API flow:
直接上传功能使用 Cloudflare 的 4 步 API 流程：

1. **Get Upload Token** / 获取上传令牌 - Obtain JWT for assets API authentication
2. **Check Missing** / 检查缺失 - Query which files (by MD5 hash) need uploading
3. **Upload Files** / 上传文件 - Upload missing files in batches (base64 encoded)
4. **Create Deployment** / 创建部署 - Create deployment with manifest (path → MD5 mapping)

**Note**: Special config files (`_headers`, `_redirects`, `_worker.js`, `_routes.json`) are automatically excluded from the manifest.
**注意**: 特殊配置文件（`_headers`、`_redirects`、`_worker.js`、`_routes.json`）会自动从清单中排除。

---

## Examples / 示例

### 1. Basic Pages Project / 基础 Pages 项目

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesProject
metadata:
  name: my-app
  namespace: default
spec:
  name: my-app
  productionBranch: main
  buildConfig:
    buildCommand: npm run build
    destinationDir: dist
    rootDir: /
  cloudflare:
    accountId: "<your-account-id>"     # 替换为实际值 / Replace with actual value
    credentialsRef:
      name: cloudflare-credentials
```

### 2. Pages Project with GitHub Source / 使用 GitHub 源

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesProject
metadata:
  name: my-github-app
  namespace: default
spec:
  name: my-github-app
  productionBranch: main
  source:
    type: github
    github:
      owner: my-org
      repo: my-repo
      productionDeploymentsEnabled: true
      previewDeploymentsEnabled: true
      prCommentsEnabled: true
  buildConfig:
    buildCommand: npm run build
    destinationDir: dist
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
```

### 3. Pages Project with Environment Variables / 配置环境变量

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesProject
metadata:
  name: my-app-with-env
  namespace: default
spec:
  name: my-app-with-env
  productionBranch: main
  buildConfig:
    buildCommand: npm run build
    destinationDir: dist
  deploymentConfigs:
    production:
      compatibilityDate: "2024-01-01"
      environmentVariables:
        API_URL:
          value: "https://api.example.com"
          type: plain_text
        API_KEY:
          value: "secret-key"
          type: secret_text
    preview:
      compatibilityDate: "2024-01-01"
      environmentVariables:
        API_URL:
          value: "https://staging-api.example.com"
          type: plain_text
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
```

### 4. Pages Project with Resource Bindings / 资源绑定

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesProject
metadata:
  name: full-stack-app
  namespace: default
spec:
  name: full-stack-app
  productionBranch: main
  buildConfig:
    buildCommand: npm run build
    destinationDir: dist
  deploymentConfigs:
    production:
      compatibilityDate: "2024-01-01"
      compatibilityFlags:
        - nodejs_compat
      kvBindings:
        - name: MY_KV
          namespaceId: "<kv-namespace-id>"
      r2Bindings:
        - name: MY_BUCKET
          bucketName: my-bucket
      d1Bindings:
        - name: MY_DB
          databaseId: "<d1-database-id>"
      serviceBindings:
        - name: MY_WORKER
          service: my-worker
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
```

### 5. Custom Domain for Pages / 自定义域名

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDomain
metadata:
  name: my-app-domain
  namespace: default
spec:
  domain: app.example.com
  projectRef:
    name: my-app  # Reference to PagesProject / 引用 PagesProject
  autoConfigureDNS: true  # Default: true - auto DNS config / 默认：true - 自动 DNS 配置
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
```

### 5a. Custom Domain with Manual DNS / 手动 DNS 的自定义域名

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDomain
metadata:
  name: external-domain
  namespace: default
spec:
  domain: app.external-dns.com
  projectRef:
    name: my-app
  autoConfigureDNS: false  # Manual DNS management / 手动 DNS 管理
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
# Note: Create CNAME record manually: app.external-dns.com → my-app.pages.dev
# 注意：手动创建 CNAME 记录：app.external-dns.com → my-app.pages.dev
```

### 5b. Pages Project with Web Analytics / 带 Web Analytics 的 Pages 项目

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesProject
metadata:
  name: my-app-with-analytics
  namespace: default
spec:
  name: my-app-with-analytics
  productionBranch: main
  enableWebAnalytics: true  # Default: true / 默认：true
  deploymentHistoryLimit: 100  # Max: 200 (FIFO retention) / 最大：200（FIFO 保留）
  buildConfig:
    buildCommand: npm run build
    destinationDir: dist
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
```

---

## Direct Upload (New) / 直接上传（新功能）

Direct Upload allows deploying static files without a Git repository. Supports fetching build artifacts from:
- **HTTP/HTTPS**: Public or presigned URLs
- **S3**: AWS S3, MinIO, Cloudflare R2
- **OCI**: Container registries (Docker Hub, GHCR, etc.)

直接上传允许无需 Git 仓库即可部署静态文件。支持从以下源获取构建产物：
- **HTTP/HTTPS**：公开或预签名 URL
- **S3**：AWS S3、MinIO、Cloudflare R2
- **OCI**：容器镜像仓库（Docker Hub、GHCR 等）

### 6. Direct Upload from HTTP URL / 从 HTTP 直接上传

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-http-deploy
  namespace: default
spec:
  projectRef:
    name: my-app
  action: create
  directUpload:
    source:
      http:
        url: "https://ci.example.com/builds/my-app/latest/dist.tar.gz"
        headers:
          Authorization: "Bearer <token>"   # Optional / 可选
        timeout: "10m"
    checksum:
      algorithm: sha256
      value: "e3b0c44298fc1c149afbf4c8996fb924..."  # Verify integrity / 校验完整性
    archive:
      type: tar.gz
      stripComponents: 1    # Remove top-level directory / 移除顶层目录
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
```

### 7. Direct Upload from S3 / 从 S3 直接上传

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-s3-deploy
  namespace: default
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
          name: aws-credentials    # Contains accessKeyId, secretAccessKey
    checksum:
      algorithm: sha256
      value: "abc123..."
    archive:
      type: tar.gz
      stripComponents: 1
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
---
# AWS Credentials Secret / AWS 凭证 Secret
apiVersion: v1
kind: Secret
metadata:
  name: aws-credentials
  namespace: default
type: Opaque
stringData:
  accessKeyId: "<your-access-key-id>"
  secretAccessKey: "<your-secret-access-key>"
  # sessionToken: "<optional-session-token>"
```

### 8. Direct Upload from Cloudflare R2 / 从 R2 直接上传

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-r2-deploy
  namespace: default
spec:
  projectRef:
    name: my-app
  action: create
  directUpload:
    source:
      s3:
        bucket: my-build-artifacts
        key: dist.zip
        endpoint: "https://<account-id>.r2.cloudflarestorage.com"
        credentialsSecretRef:
          name: r2-credentials
        usePathStyle: true   # Required for R2 / R2 必需
    archive:
      type: zip
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
```

### 9. Direct Upload from OCI Registry / 从 OCI 镜像仓库直接上传

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-oci-deploy
  namespace: default
spec:
  projectRef:
    name: my-app
  action: create
  directUpload:
    source:
      oci:
        image: "ghcr.io/my-org/my-app-dist:v1.2.3"
        credentialsSecretRef:
          name: ghcr-credentials   # Docker config format / Docker 配置格式
    archive:
      type: tar.gz
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
---
# OCI Credentials Secret (Docker config format)
apiVersion: v1
kind: Secret
metadata:
  name: ghcr-credentials
  namespace: default
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: <base64-encoded-docker-config>
```

### 9a. Force Redeploy with Annotation / 使用注解强制重新部署

When the source URL/key doesn't change but you want to trigger a new deployment:
当源 URL/key 不变但需要触发新部署时：

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-s3-deploy
  namespace: default
  annotations:
    cloudflare-operator.io/force-redeploy: "2025-01-20-v1"  # Change this to trigger redeploy / 修改此值触发重新部署
spec:
  projectRef:
    name: my-app
  action: create
  directUpload:
    source:
      s3:
        bucket: my-ci-artifacts
        key: builds/latest/dist.tar.gz  # Always the same key / 始终相同的 key
        region: us-east-1
        credentialsSecretRef:
          name: aws-credentials
    archive:
      type: tar.gz
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
```

---

## Smart Rollback (New) / 智能回滚（新功能）

Smart Rollback provides intelligent deployment rollback with multiple strategies:
- **LastSuccessful**: Roll back to the last successful deployment
- **ByVersion**: Roll back to a specific version number from history
- **ExactDeploymentID**: Roll back to a specific Cloudflare deployment ID

智能回滚提供多种回滚策略：
- **LastSuccessful**：回滚到最后一次成功的部署
- **ByVersion**：回滚到历史记录中的特定版本号
- **ExactDeploymentID**：回滚到指定的 Cloudflare 部署 ID

### 10. Rollback to Last Successful Deployment / 回滚到上次成功部署

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-rollback-last
  namespace: default
spec:
  projectRef:
    name: my-app
  action: rollback
  rollback:
    strategy: LastSuccessful   # Automatically finds last successful deployment
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
```

### 11. Rollback to Specific Version / 回滚到特定版本

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-rollback-v5
  namespace: default
spec:
  projectRef:
    name: my-app
  action: rollback
  rollback:
    strategy: ByVersion
    version: 5    # Rollback to version 5 from deployment history
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
```

### 12. Rollback to Exact Deployment ID / 回滚到指定部署 ID

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-rollback-exact
  namespace: default
spec:
  projectRef:
    name: my-app
  action: rollback
  rollback:
    strategy: ExactDeploymentID
    deploymentId: "abc123def456"   # Cloudflare deployment ID
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
```

### 13. Legacy Rollback (Still Supported) / 传统回滚（仍支持）

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-rollback-legacy
  namespace: default
spec:
  projectRef:
    name: my-app
  action: rollback
  targetDeploymentId: "<previous-deployment-id>"   # Direct ID specification
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
```

---

## Project Adoption (New) / 项目导入（新功能）

Project Adoption allows importing existing Cloudflare Pages projects into Kubernetes management:
- **IfExists**: Adopt if exists, create if not
- **MustExist**: Require project to exist (for importing)
- **MustNotExist**: Require project to NOT exist (default, creates new)

项目导入允许将已存在的 Cloudflare Pages 项目纳入 Kubernetes 管理：
- **IfExists**：如果存在则采纳，不存在则创建
- **MustExist**：必须已存在（用于导入）
- **MustNotExist**：必须不存在（默认，创建新项目）

### 14. Adopt Existing Project / 导入已存在的项目

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesProject
metadata:
  name: existing-project
  namespace: default
spec:
  name: existing-project        # Must match existing Cloudflare project name
  productionBranch: main
  adoptionPolicy: MustExist     # Require project to exist in Cloudflare
  deploymentHistoryLimit: 20    # Keep more history for rollback
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
```

### 15. Adopt or Create Project / 采纳或创建项目

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesProject
metadata:
  name: maybe-existing-project
  namespace: default
spec:
  name: maybe-existing-project
  productionBranch: main
  adoptionPolicy: IfExists      # Adopt if exists, create if not
  buildConfig:
    buildCommand: npm run build
    destinationDir: dist
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
```

### 16. Create New Project Only / 仅创建新项目

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesProject
metadata:
  name: new-project
  namespace: default
spec:
  name: new-project
  productionBranch: main
  adoptionPolicy: MustNotExist  # Default - fail if project exists
  buildConfig:
    buildCommand: npm run build
    destinationDir: dist
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
```

---

## Archive Extraction Options / 归档解压选项

| Field / 字段 | Description / 说明 |
|--------------|-------------------|
| `type` | Archive format: `tar.gz`, `tar`, `zip`, `none` / 归档格式 |
| `stripComponents` | Remove N leading path components (like `tar --strip-components`) / 移除前 N 层目录 |
| `subPath` | Extract only files under this subdirectory / 仅解压指定子目录 |

### Example with stripComponents / stripComponents 示例

If your archive contains:
```
my-app-v1.0.0/
├── dist/
│   ├── index.html
│   └── assets/
└── README.md
```

Use `stripComponents: 1` and `subPath: "dist"` to extract only the `dist/` contents:

```yaml
archive:
  type: tar.gz
  stripComponents: 1   # Removes "my-app-v1.0.0/"
  subPath: "dist"      # Only extract files under dist/
```

---

## Checksum Verification / 校验和验证

Supported algorithms / 支持的算法:
- `sha256` (default / 默认)
- `sha512`
- `md5` (legacy / 兼容旧系统)

```yaml
checksum:
  algorithm: sha256
  value: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
```

---

## Verification / 验证

```bash
# Check PagesProject status / 检查 PagesProject 状态
kubectl get pagesproject my-app -o wide

# Check if project was adopted / 检查项目是否被采纳
kubectl get pagesproject existing-project -o jsonpath='{.status.adopted}'

# View deployment history / 查看部署历史
kubectl get pagesproject my-app -o jsonpath='{.status.deploymentHistory}'

# Check PagesDeployment status / 检查 PagesDeployment 状态
kubectl get pagesdeployment my-app-deploy -o wide

# View detailed status / 查看详细状态
kubectl describe pagesdeployment my-app-deploy

# View operator logs / 查看 Operator 日志
kubectl logs -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager
```

---

## Cleanup / 清理

```bash
kubectl delete pagesdeployment --all
kubectl delete pagesdomain --all
kubectl delete pagesproject --all
```

---

## Related Documentation / 相关文档

| Topic / 主题 | Link / 链接 |
|--------------|-------------|
| API Reference / API 参考 | [docs/en/api-reference/](../../docs/en/api-reference/) |
| Configuration / 配置 | [docs/en/configuration.md](../../docs/en/configuration.md) |
| Troubleshooting / 故障排除 | [docs/en/troubleshooting.md](../../docs/en/troubleshooting.md) |
