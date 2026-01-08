# Scenario: Kubernetes Private Network Access / 场景：Kubernetes 私有网络访问

This scenario demonstrates how to enable WARP clients to access Kubernetes Pod and Service networks through Cloudflare Tunnel.

此场景演示如何允许 WARP 客户端通过 Cloudflare Tunnel 访问 Kubernetes Pod 和 Service 网络。

## Architecture / 架构

```
WARP Client → Cloudflare Edge → Tunnel (WARP Routing) → K8s Network
WARP 客户端 → Cloudflare 边缘 → 隧道（WARP 路由）→ K8s 网络
```

## Prerequisites / 前置条件

- Cloudflare WARP client installed on user devices
- WARP 客户端已安装在用户设备上

- Users enrolled in your Zero Trust organization
- 用户已加入你的 Zero Trust 组织

## Files / 文件

| File / 文件 | Description / 说明 |
|-------------|-------------------|
| `secret.yaml` | Cloudflare API credentials |
| `cluster-tunnel.yaml` | Tunnel with WARP routing enabled |
| `virtual-network.yaml` | Virtual network for traffic isolation |
| `network-routes.yaml` | Routes for Pod and Service networks |
| `device-policy.yaml` | WARP client split tunnel configuration |

## Steps / 步骤

### 1. Create credentials / 创建凭证

```bash
kubectl apply -f secret.yaml
```

### 2. Create tunnel with WARP routing / 创建启用 WARP 路由的隧道

```bash
kubectl apply -f cluster-tunnel.yaml

# Wait for tunnel to be ready
# 等待隧道就绪
kubectl wait --for=condition=Ready clustertunnel/private-access-tunnel --timeout=120s
```

### 3. Create virtual network (optional) / 创建虚拟网络（可选）

```bash
kubectl apply -f virtual-network.yaml
```

### 4. Create network routes / 创建网络路由

```bash
kubectl apply -f network-routes.yaml
```

### 5. Configure WARP client settings / 配置 WARP 客户端设置

```bash
kubectl apply -f device-policy.yaml
```

### 6. Test connectivity / 测试连接

From a WARP-connected device:
从连接 WARP 的设备：

```bash
# Access a pod by IP
# 通过 IP 访问 Pod
ping 10.244.1.5

# Access a service by ClusterIP
# 通过 ClusterIP 访问 Service
curl http://10.96.0.1:443

# Access a service by DNS (requires fallback domain config)
# 通过 DNS 访问 Service（需要回退域配置）
curl http://my-service.default.svc.cluster.local
```

## Network Planning / 网络规划

### Common Kubernetes CIDR Ranges / 常见 Kubernetes CIDR 范围

| Network / 网络 | Default CIDR | Notes / 备注 |
|----------------|--------------|--------------|
| Pod Network | `10.244.0.0/16` | Flannel default |
| Pod Network | `10.0.0.0/16` | Calico default |
| Service Network | `10.96.0.0/12` | Kubernetes default |

### Check Your Cluster's CIDR / 检查你的集群 CIDR

```bash
# Pod CIDR
kubectl get nodes -o jsonpath='{.items[*].spec.podCIDR}'

# Service CIDR (from kube-apiserver)
kubectl cluster-info dump | grep -m1 service-cluster-ip-range
```

## Cleanup / 清理

```bash
kubectl delete -f .
```
