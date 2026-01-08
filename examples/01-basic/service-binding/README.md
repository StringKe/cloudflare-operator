# Service Binding Examples / 服务绑定示例

This directory contains examples for binding Kubernetes Services to Cloudflare Tunnels.

此目录包含将 Kubernetes Service 绑定到 Cloudflare Tunnel 的示例。

## Files / 文件

| File / 文件 | Description / 说明 |
|-------------|-------------------|
| `tunnel-binding.yaml` | Basic service binding / 基础服务绑定 |
| `multi-service-binding.yaml` | Multiple services on one tunnel / 一个隧道多个服务 |
| `tcp-service-binding.yaml` | TCP/SSH/RDP services / TCP/SSH/RDP 服务 |

## Concepts / 概念

### What TunnelBinding Does / TunnelBinding 的作用

1. **Configures cloudflared** - Adds ingress rules to the tunnel configuration
   **配置 cloudflared** - 向隧道配置添加入口规则

2. **Creates DNS records** - Automatically creates CNAME records pointing to the tunnel
   **创建 DNS 记录** - 自动创建指向隧道的 CNAME 记录

3. **Manages lifecycle** - Updates/deletes configuration when resources change
   **管理生命周期** - 资源变更时更新/删除配置

### Supported Protocols / 支持的协议

| Protocol / 协议 | Default Port / 默认端口 | Use Case / 使用场景 |
|-----------------|------------------------|---------------------|
| `http` | 80 | Web applications / Web 应用 |
| `https` | 443 | Secure web apps / 安全 Web 应用 |
| `tcp` | Any | Databases, custom apps / 数据库、自定义应用 |
| `ssh` | 22 | Remote shell access / 远程 shell 访问 |
| `rdp` | 3389 | Windows Remote Desktop / Windows 远程桌面 |
| `smb` | 445 | File sharing / 文件共享 |
| `udp` | Any | UDP applications / UDP 应用 |

## Usage / 使用方法

### Basic Binding / 基础绑定

```bash
# Create a tunnel first
# 首先创建隧道
kubectl apply -f ../tunnel/tunnel.yaml

# Wait for tunnel to be ready
# 等待隧道就绪
kubectl wait --for=condition=Ready tunnel/my-tunnel --timeout=60s

# Create the binding
# 创建绑定
kubectl apply -f tunnel-binding.yaml

# Check status
# 检查状态
kubectl get tunnelbinding web-app-binding -o wide
```

### Verify DNS Records / 验证 DNS 记录

```bash
# Check if DNS record was created
# 检查 DNS 记录是否已创建
dig app.example.com

# Should return CNAME to <tunnel-id>.cfargotunnel.com
# 应该返回指向 <tunnel-id>.cfargotunnel.com 的 CNAME
```

### Access Non-HTTP Services / 访问非 HTTP 服务

```bash
# SSH access
# SSH 访问
cloudflared access ssh --hostname ssh.example.com

# TCP access (generic)
# TCP 访问（通用）
cloudflared access tcp --hostname db.example.com --url localhost:5432

# Then connect to localhost:5432
# 然后连接到 localhost:5432
```

## Troubleshooting / 故障排除

### DNS record not created / DNS 记录未创建

```bash
# Check TunnelBinding events
# 检查 TunnelBinding 事件
kubectl describe tunnelbinding <name>

# Check operator logs
# 检查 operator 日志
kubectl logs -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager
```

### Service not accessible / 服务无法访问

1. Verify the Service exists and has endpoints
   验证 Service 存在且有 endpoints
   ```bash
   kubectl get svc <service-name>
   kubectl get endpoints <service-name>
   ```

2. Check cloudflared logs
   检查 cloudflared 日志
   ```bash
   kubectl logs -l app.kubernetes.io/name=cloudflared
   ```

3. Verify tunnel is healthy in Cloudflare Dashboard
   在 Cloudflare 控制台验证隧道健康状态
