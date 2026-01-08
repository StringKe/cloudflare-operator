# Scenario: Expose Web Application / 场景：暴露 Web 应用

This scenario demonstrates how to expose a web application through Cloudflare Tunnel with automatic DNS and TLS.

此场景演示如何通过 Cloudflare Tunnel 暴露 Web 应用，自动配置 DNS 和 TLS。

## Architecture / 架构

```
Internet → Cloudflare Edge → Tunnel → Service → Pod
互联网 → Cloudflare 边缘 → 隧道 → Service → Pod
```

## Files / 文件

| File / 文件 | Description / 说明 |
|-------------|-------------------|
| `namespace.yaml` | Namespace for the application |
| `secret.yaml` | Cloudflare API credentials |
| `tunnel.yaml` | Cloudflare Tunnel |
| `app-deployment.yaml` | Sample web application |
| `tunnel-binding.yaml` | Bind service to tunnel |

## Steps / 步骤

### 1. Create namespace and credentials / 创建命名空间和凭证

```bash
kubectl apply -f namespace.yaml
kubectl apply -f secret.yaml
```

### 2. Create tunnel / 创建隧道

```bash
kubectl apply -f tunnel.yaml

# Wait for tunnel to be ready
# 等待隧道就绪
kubectl wait --for=condition=Ready tunnel/web-app-tunnel -n web-app --timeout=120s
```

### 3. Deploy application / 部署应用

```bash
kubectl apply -f app-deployment.yaml
```

### 4. Create tunnel binding / 创建隧道绑定

```bash
kubectl apply -f tunnel-binding.yaml
```

### 5. Verify / 验证

```bash
# Check all resources
# 检查所有资源
kubectl get all -n web-app

# Check tunnel status
# 检查隧道状态
kubectl get tunnel -n web-app

# Check DNS record (may take a few minutes)
# 检查 DNS 记录（可能需要几分钟）
dig app.example.com

# Access the application
# 访问应用
curl https://app.example.com
```

## Customization / 自定义

### Add Zero Trust Protection / 添加零信任保护

To require authentication:
要求认证：

```bash
# Apply Access resources
# 应用 Access 资源
kubectl apply -f ../../03-zero-trust/access-group/access-group.yaml
kubectl apply -f ../../03-zero-trust/access-application/access-application.yaml
```

### Enable High Availability / 启用高可用

Modify `tunnel.yaml` to add replicas:
修改 `tunnel.yaml` 添加副本：

```yaml
spec:
  deployPatch: |
    spec:
      replicas: 3
```

## Cleanup / 清理

```bash
kubectl delete -f .
```
