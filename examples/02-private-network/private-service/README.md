# Private Service Examples / 私有服务示例

This directory contains examples for exposing Kubernetes Services via private IPs.

此目录包含通过私有 IP 暴露 Kubernetes Service 的示例。

## Concepts / 概念

### PrivateService vs NetworkRoute / PrivateService 与 NetworkRoute 的区别

| Feature / 特性 | NetworkRoute | PrivateService |
|----------------|--------------|----------------|
| Scope / 范围 | CIDR range / CIDR 范围 | Single IP / 单个 IP |
| Use case / 使用场景 | Route entire networks / 路由整个网络 | Specific service access / 特定服务访问 |
| Control / 控制级别 | Network level / 网络级别 | Service level / 服务级别 |

### How It Works / 工作原理

```
WARP Client → Cloudflare → Tunnel → PrivateService → K8s Service → Pod
WARP 客户端 → Cloudflare → 隧道 → PrivateService → K8s Service → Pod
```

1. PrivateService creates a route for a specific IP
   PrivateService 为特定 IP 创建路由

2. Traffic to that IP is routed through the tunnel
   到该 IP 的流量通过隧道路由

3. cloudflared proxies to the target Kubernetes Service
   cloudflared 代理到目标 Kubernetes Service

## IP Address Planning / IP 地址规划

Choose a private IP range that:
选择私有 IP 范围时要确保：

- Does not overlap with existing Kubernetes networks
  不与现有 Kubernetes 网络重叠

- Is routable through your tunnel
  可通过你的隧道路由

- Example range: `10.200.0.0/24` for PrivateServices
  示例范围：`10.200.0.0/24` 用于 PrivateServices

## Usage / 使用方法

```bash
# Create private services
# 创建私有服务
kubectl apply -f private-service.yaml

# Verify status
# 验证状态
kubectl get privateservice -A

# Check details
# 检查详情
kubectl describe privateservice internal-api
```

## Accessing Services / 访问服务

From a WARP-connected device:
从连接 WARP 的设备：

```bash
# Access API
# 访问 API
curl http://10.200.0.10:8080/api/health

# Connect to PostgreSQL
# 连接 PostgreSQL
psql -h 10.200.0.20 -U postgres -d mydb

# Connect to Redis
# 连接 Redis
redis-cli -h 10.200.0.30
```

## Best Practices / 最佳实践

1. **Document IP assignments** - Maintain a record of which IPs are assigned
   **记录 IP 分配** - 维护已分配 IP 的记录

2. **Use consistent naming** - Match PrivateService name with Service name
   **使用一致的命名** - PrivateService 名称与 Service 名称匹配

3. **Virtual network isolation** - Use different VNets for different environments
   **虚拟网络隔离** - 不同环境使用不同的 VNet
