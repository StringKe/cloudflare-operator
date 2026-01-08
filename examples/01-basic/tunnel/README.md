# Tunnel Examples / 隧道示例

This directory contains examples for creating and managing Cloudflare Tunnels.

此目录包含创建和管理 Cloudflare Tunnel 的示例。

## Files / 文件

| File / 文件 | Description / 说明 |
|-------------|-------------------|
| `tunnel.yaml` | Namespaced Tunnel / 命名空间级隧道 |
| `cluster-tunnel.yaml` | Cluster-scoped Tunnel / 集群级隧道 |
| `existing-tunnel.yaml` | Reference existing tunnel / 引用现有隧道 |

## Concepts / 概念

### Tunnel vs ClusterTunnel / Tunnel 与 ClusterTunnel

| Feature / 特性 | Tunnel | ClusterTunnel |
|----------------|--------|---------------|
| Scope / 范围 | Namespaced | Cluster |
| Accessibility / 可访问性 | Same namespace only / 仅同命名空间 | All namespaces / 所有命名空间 |
| Use case / 使用场景 | Per-namespace isolation / 命名空间隔离 | Shared infrastructure / 共享基础设施 |

### What the operator creates / Operator 创建的资源

When you create a Tunnel/ClusterTunnel, the operator automatically creates:

创建 Tunnel/ClusterTunnel 时，operator 自动创建：

1. **ConfigMap** - Contains cloudflared configuration / 包含 cloudflared 配置
2. **Secret** - Stores tunnel credentials / 存储隧道凭证
3. **Deployment** - Runs cloudflared pods / 运行 cloudflared pods

## Usage / 使用方法

### Create a New Tunnel / 创建新隧道

```bash
# Edit configuration
# 编辑配置
vim tunnel.yaml

# Apply
# 应用
kubectl apply -f tunnel.yaml

# Check status
# 检查状态
kubectl get tunnel my-tunnel -o wide
kubectl describe tunnel my-tunnel
```

### Check cloudflared Deployment / 检查 cloudflared 部署

```bash
# List deployments
# 列出部署
kubectl get deployments -l app.kubernetes.io/name=cloudflared

# Check logs
# 检查日志
kubectl logs -l app.kubernetes.io/name=cloudflared -f
```

### Delete Tunnel / 删除隧道

```bash
# Delete the tunnel resource
# 删除隧道资源
kubectl delete tunnel my-tunnel

# The operator will:
# operator 将会：
#   1. Remove DNS records / 删除 DNS 记录
#   2. Delete the tunnel from Cloudflare / 从 Cloudflare 删除隧道
#   3. Clean up ConfigMap, Secret, Deployment / 清理 ConfigMap、Secret、Deployment
```

## Status Fields / 状态字段

| Field / 字段 | Description / 说明 |
|--------------|-------------------|
| `tunnelId` | Cloudflare Tunnel UUID |
| `tunnelName` | Tunnel name in Cloudflare / Cloudflare 中的隧道名称 |
| `accountId` | Associated account / 关联账户 |
| `zoneId` | DNS zone ID / DNS 区域 ID |
| `conditions` | Current state and messages / 当前状态和消息 |
