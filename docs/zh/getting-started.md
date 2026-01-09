# 快速开始

本指南帮助你安装 Cloudflare Operator 并创建第一个隧道。

## 前置条件

- Kubernetes 集群 v1.28+
- 已配置集群访问权限的 `kubectl`
- 启用 Zero Trust 的 Cloudflare 账户
- Cloudflare API Token

## 安装

### 步骤 1：安装 CRD

```bash
kubectl apply -f https://github.com/StringKe/cloudflare-operator/releases/latest/download/cloudflare-operator.crds.yaml
```

### 步骤 2：安装 Operator

```bash
kubectl apply -f https://github.com/StringKe/cloudflare-operator/releases/latest/download/cloudflare-operator.yaml
```

### 步骤 3：验证安装

```bash
# 检查 operator pod
kubectl get pods -n cloudflare-operator-system

# 检查 CRD
kubectl get crds | grep cloudflare
```

预期输出：
```
NAME                                                              CREATED AT
accessapplications.networking.cloudflare-operator.io              2024-01-01T00:00:00Z
accessgroups.networking.cloudflare-operator.io                    2024-01-01T00:00:00Z
...
tunnels.networking.cloudflare-operator.io                         2024-01-01T00:00:00Z
```

## 创建第一个隧道

### 步骤 1：创建 API 凭证

1. 访问 [Cloudflare 控制台 > API Tokens](https://dash.cloudflare.com/profile/api-tokens)
2. 创建自定义 Token，包含以下权限：
   - `Account:Cloudflare Tunnel:Edit`
   - `Zone:DNS:Edit`（针对你的域名）

3. 创建 Kubernetes Secret：

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

### 步骤 2：获取 Account ID

1. 登录 [Cloudflare 控制台](https://dash.cloudflare.com)
2. 选择任意域名
3. 在右侧边栏"API"部分找到 **Account ID**

### 步骤 3：创建隧道

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

### 步骤 4：验证隧道

```bash
# 检查隧道状态
kubectl get tunnel my-first-tunnel

# 检查 cloudflared 部署
kubectl get deployment -l app.kubernetes.io/name=cloudflared

# 检查 cloudflared 日志
kubectl logs -l app.kubernetes.io/name=cloudflared
```

### 步骤 5：暴露服务

部署示例应用：

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

创建 TunnelBinding：

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

### 步骤 6：访问应用

片刻后，你的应用将可通过 `https://hello.example.com` 访问。

```bash
# 验证 DNS 记录
dig hello.example.com

# 访问应用
curl https://hello.example.com
```

## 高级配置

### 扩展隧道副本数

使用 `deployPatch` 字段来自定义 cloudflared deployment。这是一个应用到 deployment spec 的 JSON 补丁。

**设置副本数：**

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

**设置资源和节点选择器：**

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

### 使用 ClusterTunnel

对于集群范围的隧道（可从任何命名空间访问），使用 ClusterTunnel：

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
    secret: cloudflare-credentials  # 必须在 cloudflare-operator-system 命名空间
  deployPatch: '{"spec":{"replicas":2}}'
```

> **注意：** 对于 ClusterTunnel 和其他集群范围的资源，Secret 必须位于 `cloudflare-operator-system` 命名空间。

## 下一步

- [配置 API Token 权限](configuration.md)
- [启用私有网络访问](guides/private-network.md)
- [添加 Zero Trust 认证](guides/zero-trust.md)
- [查看所有示例](../../examples/)
