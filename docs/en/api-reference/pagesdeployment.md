# PagesDeployment

PagesDeployment is a namespaced resource that manages Cloudflare Pages project deployments with Git source, Direct Upload, and intelligent rollback capabilities.

## Overview

PagesDeployment enables you to deploy your application to Cloudflare Pages from Kubernetes. It supports Git-based deployments, direct upload of build artifacts from HTTP/S3/OCI sources, and automatic rollback to previous versions.

### Key Features

- Persistent version entity model (v0.28.0+)
- Git source deployment (branch/commit)
- Direct Upload from HTTP, S3, OCI sources
- Checksum verification and archive extraction
- Production/Preview environment separation
- Multiple rollback strategies
- Rich status tracking with hash URLs

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `projectRef` | PagesProjectRef | **Yes** | Reference to PagesProject |
| `environment` | string | No | Deployment environment: `production` or `preview` |
| `source` | PagesDeploymentSourceSpec | No | Deployment source (git or directUpload) |
| `purgeBuildCache` | bool | No | Purge build cache before deployment |
| `cloudflare` | CloudflareDetails | **Yes** | API credentials |
| `branch` | string | No | *Deprecated*: Use `source.git.branch` |
| `action` | string | No | *Deprecated*: Use `environment` and `source` |
| `directUpload` | PagesDirectUpload | No | *Deprecated*: Use `source.directUpload` |
| `rollback` | RollbackConfig | No | *Deprecated*: Rollback configuration |

### Source Types

#### Git Source

```yaml
source:
  type: git
  git:
    branch: main           # Branch to deploy
    commitSha: "abc123"    # Optional: specific commit SHA
```

#### Direct Upload Source

```yaml
source:
  type: directUpload
  directUpload:
    source:
      http:                # Or s3: or oci:
        url: "https://example.com/dist.tar.gz"
    checksum:
      algorithm: sha256
      value: "e3b0c44..."
    archive:
      type: tar.gz
      stripComponents: 1
```

## Status

| Field | Type | Description |
|-------|------|-------------|
| `deploymentId` | string | Cloudflare deployment ID |
| `projectName` | string | Cloudflare project name |
| `accountId` | string | Cloudflare account ID |
| `url` | string | Primary deployment URL |
| `hashUrl` | string | **Unique hash-based URL** (immutable, e.g., `<hash>.<project>.pages.dev`) |
| `branchUrl` | string | Branch-based URL (e.g., `<branch>.<project>.pages.dev`) |
| `environment` | string | Deployment environment (production/preview) |
| `isCurrentProduction` | bool | Whether this is the current production deployment |
| `version` | int | Sequential version number within the project |
| `versionName` | string | **Human-readable version identifier** (from label or deployment name) |
| `productionBranch` | string | Production branch used |
| `stage` | string | Current deployment stage |
| `stageHistory` | []PagesStageHistory | History of deployment stages |
| `buildConfig` | PagesBuildConfigStatus | Build configuration used |
| `source` | PagesDeploymentSource | Deployment source info |
| `sourceDescription` | string | Human-readable source description |
| `state` | PagesDeploymentState | Current state (Pending/Queued/Building/Deploying/Succeeded/Failed/Cancelled) |
| `conditions` | []Condition | Standard Kubernetes conditions |
| `observedGeneration` | int64 | Last observed generation |
| `message` | string | Additional state information |
| `startedAt` | Time | When deployment started |
| `finishedAt` | Time | When deployment finished |

### Status URL Fields

The status provides three types of URLs:

| Field | Example | Description |
|-------|---------|-------------|
| `url` | `my-project.pages.dev` | Main project URL (for production) |
| `hashUrl` | `abc123.my-project.pages.dev` | **Immutable URL for this specific deployment** |
| `branchUrl` | `main.my-project.pages.dev` | Branch-based URL (updates with new deployments) |

**Important**: `hashUrl` is the most reliable way to reference a specific deployment version, as it never changes once the deployment is created.

## Examples

### Example 1: Git Production Deployment

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

### Example 2: Direct Upload from S3

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

### Example 3: Preview Deployment

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

### Example 4: Force Redeploy

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-deploy
  annotations:
    cloudflare-operator.io/force-redeploy: "2026-01-24-v2"  # Change to trigger redeployment
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

## Version Tracking

### Using the Version Label

Add the `networking.cloudflare-operator.io/version` label to track version names:

```yaml
metadata:
  name: my-app-deploy-v1-2-3
  labels:
    networking.cloudflare-operator.io/version: "v1.2.3"
```

This label value is stored in `status.versionName` for external applications to read.

### Reading Version Information

```bash
# Get the version name
kubectl get pagesdeployment my-app-deploy -o jsonpath='{.status.versionName}'

# Get the hash URL (immutable reference)
kubectl get pagesdeployment my-app-deploy -o jsonpath='{.status.hashUrl}'

# Get all deployment info
kubectl get pagesdeployment my-app-deploy -o jsonpath='{.status}' | jq
```

## Prerequisites

- PagesProject resource created
- Valid Cloudflare API credentials
- For Direct Upload: Source accessible (HTTP URL, S3 bucket, or OCI registry)

## Related Resources

- [PagesProject](pagesproject.md) - Manage Pages projects
- [PagesDomain](pagesdomain.md) - Custom domains for Pages
- [Pages Advanced Deployment Guide](../guides/pages-advanced-deployment.md) - Comprehensive deployment guide

## See Also

- [Cloudflare Pages Deployments](https://developers.cloudflare.com/pages/deployments/)
- [Direct Upload API](https://developers.cloudflare.com/pages/platform/direct-upload/)
