# Network Route Examples / 网络路由示例

This directory contains examples for routing private networks through Cloudflare Tunnels.

此目录包含通过 Cloudflare Tunnel 路由私有网络的示例。

## Prerequisites / 前置条件

1. A ClusterTunnel or Tunnel with `enableWarpRouting: true`
   启用 `enableWarpRouting: true` 的 ClusterTunnel 或 Tunnel

2. Optional: VirtualNetwork for traffic isolation
   可选：VirtualNetwork 用于流量隔离

## How It Works / 工作原理

```
WARP Client → Cloudflare Edge → Tunnel → Kubernetes Network
WARP 客户端 → Cloudflare 边缘 → 隧道 → Kubernetes 网络
```

When a WARP client tries to access an IP in the configured CIDR:
当 WARP 客户端尝试访问配置的 CIDR 中的 IP 时：

1. Traffic is routed to Cloudflare Edge
   流量被路由到 Cloudflare 边缘

2. Edge forwards to the appropriate tunnel based on route
   边缘根据路由转发到相应的隧道

3. cloudflared proxies the traffic to the target IP
   cloudflared 将流量代理到目标 IP

## Common CIDR Ranges / 常见 CIDR 范围

| CIDR | Description / 说明 |
|------|-------------------|
| `10.244.0.0/16` | Kubernetes Pod network (Flannel default) |
| `10.96.0.0/12` | Kubernetes Service network (default) |
| `192.168.0.0/16` | Internal networks / 内部网络 |
| `10.0.0.0/8` | Private network range / 私有网络范围 |
| `172.16.0.0/12` | Docker/container networks / Docker/容器网络 |

## Usage / 使用方法

```bash
# First, create a tunnel with WARP routing enabled
# 首先，创建启用 WARP 路由的隧道
kubectl apply -f ../../01-basic/tunnel/cluster-tunnel.yaml

# Create network routes
# 创建网络路由
kubectl apply -f network-route.yaml

# Verify routes
# 验证路由
kubectl get networkroute
kubectl describe networkroute k8s-pod-network
```

## Testing / 测试

From a WARP-connected device:
从连接 WARP 的设备：

```bash
# Test connectivity to a pod IP
# 测试与 Pod IP 的连接
ping 10.244.1.5

# Access a Kubernetes service by IP
# 通过 IP 访问 Kubernetes 服务
curl http://10.96.0.1:443

# SSH to a pod (if SSH is running)
# SSH 到 Pod（如果运行了 SSH）
ssh user@10.244.1.5
```

## Troubleshooting / 故障排除

### Route not working / 路由不工作

1. Verify tunnel has WARP routing enabled
   验证隧道已启用 WARP 路由
   ```bash
   kubectl get clustertunnel -o yaml | grep enableWarpRouting
   ```

2. Check NetworkRoute status
   检查 NetworkRoute 状态
   ```bash
   kubectl describe networkroute <name>
   ```

3. Verify WARP client is connected
   验证 WARP 客户端已连接

4. Check if route is in Cloudflare Dashboard
   在 Cloudflare 控制台检查路由是否存在
