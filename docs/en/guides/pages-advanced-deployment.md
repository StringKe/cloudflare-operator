# Pages Advanced Deployment Guide

This guide covers advanced deployment features for Cloudflare Pages including Direct Upload, Smart Rollback, Project Adoption, and Web Analytics integration.

> **Version**: v0.27.13+

## Overview

The Cloudflare Operator provides powerful features for managing Pages deployments:

| Feature | Description | Use Case |
|---------|-------------|----------|
| **Direct Upload** | Deploy static files from external sources | CI/CD pipelines, build artifact storage |
| **Smart Rollback** | Intelligent rollback with multiple strategies | Quick recovery from failed deployments |
| **Project Adoption** | Import existing Cloudflare Pages projects | Migrate existing projects to GitOps |
| **Web Analytics** | Automatic Web Analytics integration | Monitor site performance |
| **Force Redeploy** | Trigger new deployment without config changes | Re-deploy same source |

## Architecture

```mermaid
---
config:
  layout: elk
---
flowchart TB
    subgraph Sources["External Sources"]
        HTTP["HTTP/HTTPS URL"]
        S3["S3/R2/MinIO"]
        OCI["OCI Registry"]
    end

    subgraph Operator["Cloudflare Operator"]
        PD["PagesDeployment Controller"]
        UP["Uploader Module"]
        CS["Checksum Verification"]
        AR["Archive Extraction"]
        MD5["MD5 Hash Generation"]
    end

    subgraph CloudflareAPI["Cloudflare Pages API (4-Step Flow)"]
        JWT["1. Get Upload Token (JWT)"]
        CHK["2. Check Missing Assets"]
        UPL["3. Upload Missing Files"]
        UPS["4. Upsert Hashes"]
        DEP["5. Create Deployment"]
    end

    subgraph Cloudflare["Cloudflare"]
        PP["Pages Project"]
        DEPLOY["Deployment"]
    end

    HTTP --> UP
    S3 --> UP
    OCI --> UP

    UP --> CS
    CS --> AR
    AR --> MD5
    MD5 --> PD
    PD --> JWT
    JWT --> CHK
    CHK --> UPL
    UPL --> UPS
    UPS --> DEP
    DEP --> PP
    PP --> DEPLOY
```

### Direct Upload API Flow

The operator implements Cloudflare's Direct Upload API using a 4-step flow:

1. **Get Upload Token**: Obtain a JWT token for authentication with the assets API
2. **Check Missing Assets**: Query which files (by MD5 hash) are not already uploaded
3. **Upload Missing Files**: Upload only the files that Cloudflare doesn't have (base64 encoded)
4. **Upsert Hashes**: Register all file hashes for the deployment
5. **Create Deployment**: Create the deployment with the manifest (path → MD5 hash mapping)

**Important Notes**:
- Files are identified by **MD5 hash** (not SHA256) as required by Cloudflare's API
- Special Pages config files (`_headers`, `_redirects`, `_worker.js`, `_routes.json`) are excluded from the manifest
- Files are uploaded in batches of 100 for efficiency
- Both API Token and Global API Key authentication are supported

---

## Direct Upload

Direct Upload allows deploying static files without a Git repository. This is ideal for:
- CI/CD pipelines that build artifacts separately
- Pre-built static sites stored in object storage
- Air-gapped environments without Git access

### Supported Sources

| Source | Description | Authentication |
|--------|-------------|----------------|
| **HTTP/HTTPS** | Any accessible URL | Headers, Bearer tokens |
| **S3** | AWS S3, MinIO, R2 | Access key/secret |
| **OCI** | Container registries | Docker config, basic auth |

### HTTP Source

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
        insecureSkipVerify: false  # Only for testing
    archive:
      type: tar.gz
      stripComponents: 1
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

#### HTTP Source Configuration

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `url` | string | Yes | - | HTTP/HTTPS URL to fetch files from |
| `headers` | map | No | - | Headers to include in the request |
| `headersSecretRef` | object | No | - | Reference to Secret containing headers |
| `timeout` | duration | No | `5m` | Request timeout |
| `insecureSkipVerify` | bool | No | `false` | Skip TLS verification |

### S3 Source

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

#### S3 Source Configuration

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `bucket` | string | Yes | - | S3 bucket name |
| `key` | string | Yes | - | Object key (path) in the bucket |
| `region` | string | No | - | S3 region (required for AWS) |
| `endpoint` | string | No | - | Custom endpoint for S3-compatible services |
| `credentialsSecretRef` | object | No | - | Reference to Secret with credentials |
| `usePathStyle` | bool | No | `false` | Use path-style addressing |

#### S3 Credentials Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: aws-credentials
type: Opaque
stringData:
  accessKeyId: "AKIAIOSFODNN7EXAMPLE"
  secretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
  # sessionToken: "optional-session-token"
```

### S3-Compatible Services

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
      usePathStyle: true  # Required for R2
```

#### MinIO

```yaml
directUpload:
  source:
    s3:
      bucket: builds
      key: my-app/dist.tar.gz
      endpoint: "https://minio.internal.example.com"
      region: us-east-1  # Required even for MinIO
      credentialsSecretRef:
        name: minio-credentials
      usePathStyle: true
```

### OCI Source

OCI (Open Container Initiative) registries can store arbitrary artifacts, making them suitable for build artifacts.

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

#### OCI Credentials Secret

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

Or use basic auth:

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

### Checksum Verification

Verify file integrity before deployment:

```yaml
directUpload:
  source:
    http:
      url: "https://example.com/dist.tar.gz"
  checksum:
    algorithm: sha256  # sha256 (default), sha512, md5
    value: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
```

If the checksum doesn't match, the deployment will fail with an error.

### Archive Extraction

Configure how to extract the downloaded archive:

```yaml
directUpload:
  archive:
    type: tar.gz      # tar.gz, tar, zip, none
    stripComponents: 1  # Remove top-level directory
    subPath: "dist"     # Extract only this subdirectory
```

#### Archive Options

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | string | `tar.gz` | Archive format: `tar.gz`, `tar`, `zip`, `none` |
| `stripComponents` | int | `0` | Remove N leading path components |
| `subPath` | string | - | Extract only files under this path |

#### Example: Extract Subdirectory

If your archive contains:

```
my-app-v1.0.0/
├── dist/
│   ├── index.html
│   └── assets/
├── src/
└── README.md
```

To deploy only the `dist/` directory contents:

```yaml
archive:
  type: tar.gz
  stripComponents: 1   # Removes "my-app-v1.0.0/"
  subPath: "dist"      # Only files under dist/
```

Result: `index.html` and `assets/` are deployed to the root.

### Force Redeploy

To trigger a new deployment even when the source configuration hasn't changed, use the `cloudflare-operator.io/force-redeploy` annotation:

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-deploy
  annotations:
    cloudflare-operator.io/force-redeploy: "2025-01-20-v1"  # Change this value to trigger redeployment
spec:
  projectRef:
    name: my-app
  action: create
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

**How it works**:
- When the annotation value changes, the operator detects a configuration change
- A new deployment is created even if the S3 key or other settings remain the same
- Useful for re-deploying when the source file at the same URL has been updated

**Use cases**:
- CI/CD pipelines that always upload to the same S3 key (e.g., `latest/dist.tar.gz`)
- Force re-deployment after infrastructure changes
- Manual rollout triggers in GitOps workflows

---

## Smart Rollback

Smart Rollback provides intelligent deployment rollback with three strategies:

| Strategy | Description | Use Case |
|----------|-------------|----------|
| `LastSuccessful` | Roll back to the last successful deployment | Quick recovery |
| `ByVersion` | Roll back to a specific version number | Precise control |
| `ExactDeploymentID` | Roll back to a specific Cloudflare deployment | Maximum precision |

### Rollback Strategies

```mermaid
---
config:
  layout: dagre
---
flowchart LR
    subgraph History["Deployment History"]
        V5["v5: Failed"]
        V4["v4: Success ✓"]
        V3["v3: Success ✓"]
        V2["v2: Failed"]
        V1["v1: Success ✓"]
    end

    subgraph Strategies["Rollback Strategies"]
        LS["LastSuccessful → v4"]
        BV["ByVersion(3) → v3"]
        EID["ExactDeploymentID → any"]
    end

    V4 --> LS
    V3 --> BV
```

### LastSuccessful Strategy

Automatically finds and rolls back to the most recent successful deployment:

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

### ByVersion Strategy

Roll back to a specific version from the deployment history:

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

### ExactDeploymentID Strategy

Roll back to a specific Cloudflare deployment ID:

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

### Deployment History

The PagesProject tracks deployment history for rollback with a **FIFO (First-In-First-Out) retention policy**:

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesProject
metadata:
  name: my-app
spec:
  name: my-app
  productionBranch: main
  deploymentHistoryLimit: 100  # Maximum: 200 (FIFO retention)
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

**History Configuration**:

| Field | Default | Maximum | Description |
|-------|---------|---------|-------------|
| `deploymentHistoryLimit` | 200 | 200 | Number of deployment records to keep |

**History Entry Fields**:

Each deployment history entry contains:
- `deploymentId`: Cloudflare deployment ID
- `version`: Sequential version number (starts at 1)
- `url`: Deployment URL
- `source`: Source description (e.g., `git:main`, `direct-upload`, `rollback:v5`)
- `sourceHash`: SHA-256 hash of the source package (for direct upload)
- `sourceUrl`: URL where source was fetched from (for direct upload)
- `k8sResource`: K8s resource that created this deployment (`namespace/name`)
- `createdAt`: Timestamp when deployment was created
- `status`: Deployment status (`active`, `failed`, `superseded`)
- `isProduction`: Whether this is the current production deployment

View deployment history:

```bash
# View full history
kubectl get pagesproject my-app -o jsonpath='{.status.deploymentHistory}' | jq

# View last successful deployment ID
kubectl get pagesproject my-app -o jsonpath='{.status.lastSuccessfulDeploymentId}'

# View latest deployment info
kubectl get pagesproject my-app -o jsonpath='{.status.latestDeployment}' | jq
```

---

## Project Adoption

Project Adoption allows importing existing Cloudflare Pages projects into Kubernetes management.

### Adoption Policies

| Policy | Description | Use Case |
|--------|-------------|----------|
| `MustNotExist` | Fail if project exists (default) | Create new projects |
| `IfExists` | Adopt if exists, create if not | Flexible migration |
| `MustExist` | Require project to exist | Import existing projects |

### Import Existing Project

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesProject
metadata:
  name: existing-project
spec:
  name: existing-project  # Must match Cloudflare project name
  productionBranch: main
  adoptionPolicy: MustExist  # Require project to exist
  deploymentHistoryLimit: 20
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

After adoption:
- The project is now managed by the operator
- Original configuration is stored in `status.originalConfig`
- `status.adopted: true` indicates the project was adopted
- `status.adoptedAt` shows when the adoption occurred

### Flexible Adoption

Use `IfExists` for GitOps workflows where the project may or may not exist:

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesProject
metadata:
  name: my-app
spec:
  name: my-app
  productionBranch: main
  adoptionPolicy: IfExists  # Adopt if exists, create if not
  buildConfig:
    buildCommand: npm run build
    destinationDir: dist
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

### Original Configuration

When adopting a project, the original Cloudflare configuration is preserved:

```bash
kubectl get pagesproject existing-project -o jsonpath='{.status.originalConfig}' | jq
```

Example output:

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

PagesProject supports automatic Cloudflare Web Analytics integration:

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesProject
metadata:
  name: my-app
spec:
  name: my-app
  productionBranch: main
  enableWebAnalytics: true  # Enabled by default
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

**Behavior**:
- When `enableWebAnalytics: true` (default), Web Analytics is automatically enabled for the `*.pages.dev` domain
- Auto-install is enabled, which injects the analytics script automatically
- For custom domains, configure Web Analytics separately through PagesDomain or Cloudflare dashboard

**Disable Web Analytics**:

```yaml
spec:
  enableWebAnalytics: false
```

---

## Custom Domains (PagesDomain)

PagesDomain manages custom domains for Pages projects with automatic DNS configuration support.

### DNS Auto-Configuration

When your domain's zone is in the same Cloudflare account, DNS records can be configured automatically:

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDomain
metadata:
  name: my-app-domain
spec:
  domain: app.example.com
  projectRef:
    name: my-app
  autoConfigureDNS: true  # Default: true - Cloudflare configures DNS automatically
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

**DNS Configuration Modes**:

| Mode | `autoConfigureDNS` | Behavior |
|------|-------------------|----------|
| **Automatic** | `true` (default) | Cloudflare creates/manages CNAME records |
| **Manual** | `false` | You manage DNS records manually |

**When to use Manual DNS**:
- Domain is managed by external DNS provider
- Need custom DNS configuration (e.g., proxied records, TTL settings)
- Domain zone is in a different Cloudflare account
- Using advanced DNS features (e.g., load balancing)

**Manual DNS Example**:

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDomain
metadata:
  name: external-domain
spec:
  domain: app.external-dns.com
  projectRef:
    name: my-app
  autoConfigureDNS: false  # DNS managed externally
  cloudflare:
    accountId: "your-account-id"
    credentialsRef:
      name: cloudflare-credentials
```

For manual DNS, create a CNAME record pointing to your Pages subdomain:
```
app.external-dns.com  CNAME  my-app.pages.dev
```

### Domain Verification Status

Check domain verification status:

```bash
kubectl get pagesdomain my-app-domain -o wide

# View verification data if manual verification needed
kubectl get pagesdomain my-app-domain -o jsonpath='{.status.verificationData}' | jq
```

---

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Deploy to Cloudflare Pages

on:
  push:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Build
        run: npm run build

      - name: Upload to S3
        run: |
          tar -czf dist.tar.gz -C dist .
          aws s3 cp dist.tar.gz s3://my-artifacts/builds/${{ github.sha }}/dist.tar.gz

      - name: Calculate checksum
        id: checksum
        run: echo "sha256=$(sha256sum dist.tar.gz | cut -d' ' -f1)" >> $GITHUB_OUTPUT

  deploy:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - name: Create deployment
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

## Troubleshooting

### Common Issues

#### Checksum Mismatch

```
Error: checksum mismatch: expected abc123..., got def456...
```

**Cause**: The downloaded file doesn't match the expected checksum.

**Solution**: Verify the checksum value is correct for the file you're downloading.

#### Archive Extraction Failed

```
Error: archive has no files after extraction
```

**Cause**: The archive is empty or `subPath` doesn't match any files.

**Solution**: Check `stripComponents` and `subPath` configuration.

#### S3 Access Denied

```
Error: get S3 object: AccessDenied
```

**Cause**: Insufficient permissions or incorrect credentials.

**Solution**: Verify the credentials secret and S3 bucket policy.

#### Rollback Target Not Found

```
Error: deployment version 10 not found in history
```

**Cause**: The requested version is not in the deployment history.

**Solution**: Check available versions with `kubectl get pagesproject my-app -o jsonpath='{.status.deploymentHistory}'`

### Debug Commands

```bash
# Check deployment status
kubectl get pagesdeployment my-app-deploy -o wide

# View detailed events
kubectl describe pagesdeployment my-app-deploy

# Check operator logs
kubectl logs -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager | grep -i pages

# View project status
kubectl get pagesproject my-app -o yaml
```

---

## Related Documentation

| Topic | Link |
|-------|------|
| Pages Examples | [examples/06-pages/](../../../examples/06-pages/) |
| Configuration | [configuration.md](../configuration.md) |
| Troubleshooting | [troubleshooting.md](../troubleshooting.md) |
| API Reference | [api-reference/](../api-reference/) |

---

**Previous**: [Getting Started](../getting-started.md)

**Next**: [Istio Integration](istio-integration.md)
