# Cloudflare Pages Examples

This directory contains examples for managing Cloudflare Pages resources using the operator.

## Prerequisites

- Cloudflare account with Pages enabled
- API Token with `Account:Cloudflare Pages:Edit` permission

## CRDs

| CRD | Description |
|-----|-------------|
| `PagesProject` | Pages project with build config and resource bindings |
| `PagesDomain` | Custom domain for Pages project |
| `PagesDeployment` | Deployment operations (create, retry, rollback) |

## Examples

### 1. Basic Pages Project

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
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
```

### 2. Pages Project with GitHub Source

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

### 3. Pages Project with Environment Variables

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

### 4. Pages Project with Resource Bindings

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

### 5. Custom Domain for Pages

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDomain
metadata:
  name: my-app-domain
  namespace: default
spec:
  domain: app.example.com
  projectRef:
    name: my-app  # Reference to PagesProject
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
```

### 6. Trigger New Deployment

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-deploy-v1
  namespace: default
spec:
  projectRef:
    name: my-app
  branch: main
  action: create
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
```

### 7. Rollback Deployment

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PagesDeployment
metadata:
  name: my-app-rollback
  namespace: default
spec:
  projectRef:
    name: my-app
  action: rollback
  targetDeploymentId: "<previous-deployment-id>"
  cloudflare:
    accountId: "<your-account-id>"
    credentialsRef:
      name: cloudflare-credentials
```

## Verification

```bash
# Check PagesProject status
kubectl get pagesproject my-app -o yaml

# Check PagesDomain status
kubectl get pagesdomain my-app-domain -o yaml

# Check PagesDeployment status
kubectl get pagesdeployment my-app-deploy-v1 -o yaml

# View events
kubectl describe pagesproject my-app
```

## Cleanup

```bash
kubectl delete pagesdeployment --all
kubectl delete pagesdomain --all
kubectl delete pagesproject --all
```
