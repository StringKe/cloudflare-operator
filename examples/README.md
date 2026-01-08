# Examples / 示例

This directory contains practical examples for the Cloudflare Operator, organized by functionality and use cases.

此目录包含 Cloudflare Operator 的实用示例，按功能和使用场景组织。

## Directory Structure / 目录结构

```
examples/
├── 01-basic/                    # Basic examples / 基础示例
│   ├── credentials/             # API credentials setup
│   ├── tunnel/                  # Tunnel creation
│   ├── dns/                     # DNS record management
│   └── service-binding/         # Service binding to tunnels
│
├── 02-private-network/          # Private network access / 私有网络访问
│   ├── virtual-network/         # Virtual network configuration
│   ├── network-route/           # Network routing
│   └── private-service/         # Private service exposure
│
├── 03-zero-trust/               # Zero Trust Access / 零信任访问
│   ├── access-application/      # Access application configuration
│   ├── access-group/            # Access group management
│   ├── identity-provider/       # Identity provider setup
│   └── service-token/           # Service token for M2M auth
│
├── 04-gateway/                  # Gateway & Security / 网关与安全
│   ├── gateway-rule/            # Gateway rules
│   ├── gateway-list/            # Gateway lists
│   └── gateway-configuration/   # Gateway configuration
│
├── 05-device/                   # Device Management / 设备管理
│   ├── device-policy/           # Device settings policy
│   └── device-posture/          # Device posture rules
│
└── scenarios/                   # Complete Scenarios / 完整场景
    ├── web-app-exposure/        # Expose web application
    ├── kubernetes-private-access/ # K8s private network access
    └── multi-cluster-access/    # Multi-cluster setup
```

## Quick Start / 快速开始

### Prerequisites / 前置条件

1. A Kubernetes cluster (v1.28+)
2. Cloudflare account with Zero Trust enabled
3. Cloudflare API Token with appropriate permissions

---

1. Kubernetes 集群 (v1.28+)
2. 启用 Zero Trust 的 Cloudflare 账户
3. 具有适当权限的 Cloudflare API Token

### Step 1: Create Credentials / 步骤 1：创建凭证

```bash
# Edit the secret file with your API token
# 编辑 secret 文件，填入你的 API token
vim examples/01-basic/credentials/api-secret.yaml

# Apply the secret
# 应用 secret
kubectl apply -f examples/01-basic/credentials/api-secret.yaml
```

### Step 2: Create a Tunnel / 步骤 2：创建隧道

```bash
# Edit tunnel configuration
# 编辑隧道配置
vim examples/01-basic/tunnel/tunnel.yaml

# Apply the tunnel
# 应用隧道
kubectl apply -f examples/01-basic/tunnel/tunnel.yaml

# Check tunnel status
# 检查隧道状态
kubectl get tunnel -w
```

### Step 3: Expose a Service / 步骤 3：暴露服务

```bash
# Apply service binding
# 应用服务绑定
kubectl apply -f examples/01-basic/service-binding/

# Check binding status
# 检查绑定状态
kubectl get tunnelbinding
```

## Scenarios / 使用场景

### Web Application Exposure / Web 应用暴露

Expose a web application through Cloudflare Tunnel with automatic DNS and TLS.

通过 Cloudflare Tunnel 暴露 Web 应用，自动配置 DNS 和 TLS。

```bash
kubectl apply -f examples/scenarios/web-app-exposure/
```

### Kubernetes Private Access / Kubernetes 私有访问

Enable WARP clients to access Kubernetes services via private IPs.

允许 WARP 客户端通过私有 IP 访问 Kubernetes 服务。

```bash
kubectl apply -f examples/scenarios/kubernetes-private-access/
```

### Multi-Cluster Access / 多集群访问

Connect multiple Kubernetes clusters through Cloudflare network.

通过 Cloudflare 网络连接多个 Kubernetes 集群。

```bash
kubectl apply -f examples/scenarios/multi-cluster-access/
```

## API Version Reference / API 版本参考

| Resource | API Version | Scope |
|----------|-------------|-------|
| Tunnel | `networking.cloudflare-operator.io/v1alpha2` | Namespaced |
| ClusterTunnel | `networking.cloudflare-operator.io/v1alpha2` | Cluster |
| TunnelBinding | `networking.cfargotunnel.com/v1alpha1` | Namespaced |
| VirtualNetwork | `networking.cloudflare-operator.io/v1alpha2` | Cluster |
| NetworkRoute | `networking.cloudflare-operator.io/v1alpha2` | Cluster |
| PrivateService | `networking.cloudflare-operator.io/v1alpha2` | Namespaced |
| AccessApplication | `networking.cloudflare-operator.io/v1alpha2` | Namespaced |
| AccessGroup | `networking.cloudflare-operator.io/v1alpha2` | Cluster |
| AccessIdentityProvider | `networking.cloudflare-operator.io/v1alpha2` | Cluster |
| AccessServiceToken | `networking.cloudflare-operator.io/v1alpha2` | Namespaced |
| GatewayRule | `networking.cloudflare-operator.io/v1alpha2` | Cluster |
| GatewayList | `networking.cloudflare-operator.io/v1alpha2` | Cluster |
| GatewayConfiguration | `networking.cloudflare-operator.io/v1alpha2` | Cluster |
| DeviceSettingsPolicy | `networking.cloudflare-operator.io/v1alpha2` | Cluster |
| DevicePostureRule | `networking.cloudflare-operator.io/v1alpha2` | Cluster |
| DNSRecord | `networking.cloudflare-operator.io/v1alpha2` | Namespaced |
| WARPConnector | `networking.cloudflare-operator.io/v1alpha2` | Cluster |

## Notes / 注意事项

- Replace placeholder values (e.g., `<your-account-id>`, `<your-domain>`) with your actual values
- 将占位符值（如 `<your-account-id>`、`<your-domain>`）替换为实际值

- Ensure your API token has the required permissions for each resource type
- 确保你的 API token 具有每种资源类型所需的权限

- Check resource status with `kubectl describe <resource> <name>` for troubleshooting
- 使用 `kubectl describe <resource> <name>` 检查资源状态以进行故障排除
