# Virtual Network Examples / 虚拟网络示例

This directory contains examples for Cloudflare Virtual Networks.

此目录包含 Cloudflare 虚拟网络的示例。

## Concepts / 概念

### What is a Virtual Network? / 什么是虚拟网络？

Virtual Networks enable you to have multiple, segregated private networks in your Cloudflare account. This allows:

虚拟网络允许你在 Cloudflare 账户中拥有多个隔离的私有网络。这允许：

- **Overlapping IP ranges** - Same CIDR in different environments
  **重叠的 IP 范围** - 不同环境中使用相同的 CIDR

- **Traffic isolation** - Production vs Development traffic separation
  **流量隔离** - 生产与开发流量分离

- **Multi-tenant** - Different customers on different networks
  **多租户** - 不同客户在不同网络上

### Default Network / 默认网络

- Only ONE network can be the default
- 只能有一个网络设为默认

- Routes without explicit virtualNetworkRef use the default
- 没有明确 virtualNetworkRef 的路由使用默认网络

- WARP clients connect to the default network by default
- WARP 客户端默认连接到默认网络

## Usage / 使用方法

```bash
# Create virtual networks
# 创建虚拟网络
kubectl apply -f virtual-network.yaml

# Check status
# 检查状态
kubectl get virtualnetwork

# View details
# 查看详情
kubectl describe virtualnetwork production-vnet
```

## Status Fields / 状态字段

| Field / 字段 | Description / 说明 |
|--------------|-------------------|
| `virtualNetworkId` | Cloudflare Virtual Network UUID |
| `accountId` | Associated Cloudflare account |
| `state` | Current state (active, deleted) |
| `isDefault` | Whether this is the default network |
