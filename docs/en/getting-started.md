# Getting Started

This guide will help you install the Cloudflare Operator and create your first tunnel.

## Prerequisites

- Kubernetes cluster v1.28+
- `kubectl` configured with cluster access
- Cloudflare account with Zero Trust enabled
- Cloudflare API Token

## Installation

### Step 1: Install CRDs

```bash
kubectl apply -f https://github.com/StringKe/cloudflare-operator/releases/latest/download/cloudflare-operator.crds.yaml
```

### Step 2: Install Operator

```bash
kubectl apply -f https://github.com/StringKe/cloudflare-operator/releases/latest/download/cloudflare-operator.yaml
```

### Step 3: Verify Installation

```bash
# Check operator pod
kubectl get pods -n cloudflare-operator-system

# Check CRDs
kubectl get crds | grep cloudflare
```

Expected output:
```
NAME                                                              CREATED AT
accessapplications.networking.cloudflare-operator.io              2024-01-01T00:00:00Z
accessgroups.networking.cloudflare-operator.io                    2024-01-01T00:00:00Z
...
tunnels.networking.cloudflare-operator.io                         2024-01-01T00:00:00Z
```

## Create Your First Tunnel

### Step 1: Create API Credentials

1. Go to [Cloudflare Dashboard > API Tokens](https://dash.cloudflare.com/profile/api-tokens)
2. Create a Custom Token with these permissions:
   - `Account:Cloudflare Tunnel:Edit`
   - `Zone:DNS:Edit` (for your domain)

3. Create the Kubernetes Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cloudflare-credentials
  namespace: default
type: Opaque
stringData:
  CLOUDFLARE_API_TOKEN: "<your-api-token>"
```

```bash
kubectl apply -f secret.yaml
```

### Step 2: Find Your Account ID

1. Log in to [Cloudflare Dashboard](https://dash.cloudflare.com)
2. Select any domain
3. Find **Account ID** in the right sidebar under "API"

### Step 3: Create a Tunnel

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: Tunnel
metadata:
  name: my-first-tunnel
  namespace: default
spec:
  newTunnel:
    name: my-k8s-tunnel
  cloudflare:
    accountId: "<your-account-id>"
    domain: example.com
    secret: cloudflare-credentials
```

```bash
kubectl apply -f tunnel.yaml
```

### Step 4: Verify Tunnel

```bash
# Check tunnel status
kubectl get tunnel my-first-tunnel

# Check cloudflared deployment
kubectl get deployment -l app.kubernetes.io/name=cloudflared

# Check cloudflared logs
kubectl logs -l app.kubernetes.io/name=cloudflared
```

### Step 5: Expose a Service

Deploy a sample application:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hello-world
spec:
  replicas: 1
  selector:
    matchLabels:
      app: hello-world
  template:
    metadata:
      labels:
        app: hello-world
    spec:
      containers:
        - name: nginx
          image: nginx:alpine
          ports:
            - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: hello-world
spec:
  selector:
    app: hello-world
  ports:
    - port: 80
```

Create a TunnelBinding:

```yaml
apiVersion: networking.cfargotunnel.com/v1alpha1
kind: TunnelBinding
metadata:
  name: hello-world-binding
  namespace: default
spec:
  subjects:
    - kind: Service
      name: hello-world
      spec:
        fqdn: hello.example.com
        protocol: http
  tunnelRef:
    kind: Tunnel
    name: my-first-tunnel
```

```bash
kubectl apply -f binding.yaml
```

### Step 6: Access Your Application

After a few moments, your application will be accessible at `https://hello.example.com`.

```bash
# Verify DNS record
dig hello.example.com

# Access the application
curl https://hello.example.com
```

## Advanced Configuration

### Scaling Tunnel Replicas

Use the `deployPatch` field to customize the cloudflared deployment. This is a JSON patch applied to the deployment spec.

**Set replica count:**

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: Tunnel
metadata:
  name: my-tunnel
  namespace: default
spec:
  newTunnel:
    name: my-k8s-tunnel
  cloudflare:
    accountId: "<your-account-id>"
    domain: example.com
    secret: cloudflare-credentials
  deployPatch: '{"spec":{"replicas":3}}'
```

**Set resources and node selector:**

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: Tunnel
metadata:
  name: my-tunnel
  namespace: default
spec:
  newTunnel:
    name: my-k8s-tunnel
  cloudflare:
    accountId: "<your-account-id>"
    domain: example.com
    secret: cloudflare-credentials
  deployPatch: |
    {
      "spec": {
        "replicas": 2,
        "template": {
          "spec": {
            "nodeSelector": {
              "node-role.kubernetes.io/edge": "true"
            },
            "containers": [{
              "name": "cloudflared",
              "resources": {
                "requests": {"cpu": "100m", "memory": "128Mi"},
                "limits": {"cpu": "500m", "memory": "512Mi"}
              }
            }]
          }
        }
      }
    }
```

### Using ClusterTunnel

For cluster-wide tunnels (accessible from any namespace), use ClusterTunnel:

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: ClusterTunnel
metadata:
  name: shared-tunnel
spec:
  newTunnel:
    name: shared-k8s-tunnel
  cloudflare:
    accountId: "<your-account-id>"
    domain: example.com
    secret: cloudflare-credentials  # Must be in cloudflare-operator-system namespace
  deployPatch: '{"spec":{"replicas":2}}'
```

> **Note:** For ClusterTunnel and other cluster-scoped resources, the secret must be in the `cloudflare-operator-system` namespace.

## What's Next?

- [Configure API Token Permissions](configuration.md)
- [Enable Private Network Access](guides/private-network.md)
- [Add Zero Trust Authentication](guides/zero-trust.md)
- [View All Examples](../../examples/)
