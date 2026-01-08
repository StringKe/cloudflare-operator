# Scenario: Multi-Cluster Access / 场景：多集群访问

This scenario demonstrates how to connect multiple Kubernetes clusters through Cloudflare's network, enabling cross-cluster access for WARP clients.

此场景演示如何通过 Cloudflare 网络连接多个 Kubernetes 集群，为 WARP 客户端启用跨集群访问。

## Architecture / 架构

```
                    Cloudflare Edge
                         │
          ┌──────────────┼──────────────┐
          ▼              ▼              ▼
    ┌──────────┐  ┌──────────┐  ┌──────────┐
    │ Cluster A│  │ Cluster B│  │ Cluster C│
    │ (Prod)   │  │ (Staging)│  │ (Dev)    │
    └──────────┘  └──────────┘  └──────────┘
    10.1.0.0/16   10.2.0.0/16   10.3.0.0/16
```

## Key Concepts / 关键概念

### Virtual Networks / 虚拟网络

Use separate VirtualNetworks for each cluster to:
为每个集群使用单独的 VirtualNetwork 以：

- Prevent IP conflicts (overlapping CIDRs)
- 防止 IP 冲突（重叠的 CIDR）

- Isolate traffic between environments
- 隔离不同环境之间的流量

### Cluster Planning / 集群规划

| Cluster | Purpose | Pod CIDR | Service CIDR |
|---------|---------|----------|--------------|
| A | Production | 10.1.0.0/16 | 10.101.0.0/16 |
| B | Staging | 10.2.0.0/16 | 10.102.0.0/16 |
| C | Development | 10.3.0.0/16 | 10.103.0.0/16 |

## Files / 文件

| File / 文件 | Description / 说明 |
|-------------|-------------------|
| `cluster-a/` | Production cluster configuration |
| `cluster-b/` | Staging cluster configuration |
| `access-policy.yaml` | Access control between environments |

## Steps / 步骤

### 1. Deploy to each cluster / 部署到每个集群

```bash
# On Cluster A (Production)
# 在集群 A（生产）
kubectl config use-context cluster-a
kubectl apply -f cluster-a/

# On Cluster B (Staging)
# 在集群 B（预发布）
kubectl config use-context cluster-b
kubectl apply -f cluster-b/
```

### 2. Configure WARP client / 配置 WARP 客户端

WARP clients will now have access to:
WARP 客户端现在可以访问：

- Production: 10.1.x.x (via production-vnet)
- Staging: 10.2.x.x (via staging-vnet)

### 3. Switch between networks / 切换网络

Users can switch virtual networks in the WARP client settings, or you can configure access based on identity groups.

用户可以在 WARP 客户端设置中切换虚拟网络，或者你可以根据身份组配置访问。

## Best Practices / 最佳实践

1. **Non-overlapping CIDRs** - Plan IP ranges to avoid conflicts
   **非重叠的 CIDR** - 规划 IP 范围以避免冲突

2. **Separate VirtualNetworks** - Isolate environments
   **独立的 VirtualNetwork** - 隔离环境

3. **Access Control** - Use AccessGroups to restrict environment access
   **访问控制** - 使用 AccessGroup 限制环境访问

4. **Naming Convention** - Use consistent naming across clusters
   **命名约定** - 跨集群使用一致的命名

## Cleanup / 清理

```bash
# On each cluster
# 在每个集群上
kubectl delete -f cluster-a/
kubectl delete -f cluster-b/
```
