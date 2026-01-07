# Cloudflare Zero Trust Kubernetes Operator 设计文档

## 一、项目概述

### 1.1 项目目标

构建一个**完整的 Cloudflare Zero Trust Kubernetes Operator**，实现 Kubernetes 环境下对 Cloudflare Zero Trust 平台的全面管理，成为业界唯一同时支持 **私有网络访问 (ZTNA) + 公开访问 (Access) + 网关策略 (Gateway)** 的完整解决方案。

### 1.2 核心价值

- **零信任网络访问**：内部服务不直接暴露到公网，只能通过 WARP 客户端访问
- **声明式管理**：通过 Kubernetes CRD 声明式管理 Cloudflare Zero Trust 资源
- **自动化运维**：自动同步 K8s 资源与 Cloudflare 配置
- **多租户支持**：通过 Virtual Networks 实现环境隔离

### 1.3 命名规范

| 项目 | 规范 |
|------|------|
| API Group | `cloudflare.com` |
| Annotation 前缀 | `cloudflare.com/` |
| Finalizer 前缀 | `cloudflare.com/` |
| Label 前缀 | `cloudflare.com/` |

---

## 二、Zero Trust 架构理解

### 2.1 核心理念

**"Never Trust, Always Verify"** - 零信任意味着：

1. **网络层隔离**：服务不直接暴露到公网
2. **身份验证**：每次访问都需要验证身份
3. **设备态势**：验证设备安全状态
4. **最小权限**：只授予必要的访问权限
5. **持续验证**：访问过程中持续验证

### 2.2 访问模式对比

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           两种访问模式对比                                        │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│   公开访问 (Access Protected)              私有访问 (WARP Only)                  │
│   ┌─────────────────────────┐             ┌─────────────────────────┐          │
│   │                         │             │                         │          │
│   │   Browser ──────────────┼─────────────┼──→ WARP Client          │          │
│   │      │                  │             │        │                │          │
│   │      ▼                  │             │        ▼                │          │
│   │   公开域名               │             │   组织登录              │          │
│   │   app.example.com       │             │   设备注册              │          │
│   │      │                  │             │   设备态势检查           │          │
│   │      ▼                  │             │        │                │          │
│   │   Access 登录页          │             │        ▼                │          │
│   │   身份验证               │             │   私有 IP / 内部域名     │          │
│   │      │                  │             │   10.0.0.x              │          │
│   │      ▼                  │             │   internal.corp.com     │          │
│   │   Cloudflare Edge       │             │        │                │          │
│   │      │                  │             │        ▼                │          │
│   │      ▼                  │             │   Cloudflare Edge       │          │
│   │   Tunnel ───────────────┼─────────────┼──→ Tunnel (WARP Routing)│          │
│   │      │                  │             │        │                │          │
│   │      ▼                  │             │        ▼                │          │
│   │   K8s Service           │             │   K8s Service           │          │
│   │                         │             │   (私有网络)             │          │
│   └─────────────────────────┘             └─────────────────────────┘          │
│                                                                                 │
│   适用场景:                                适用场景:                             │
│   • SaaS 应用                              • 内部系统                           │
│   • 客户门户                               • 开发环境                           │
│   • 合作伙伴访问                            • 数据库                            │
│   • API 网关                               • 管理后台                           │
│                                            • 敏感服务                           │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### 2.3 私有网络访问架构

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                         Zero Trust 私有网络访问架构                               │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│   用户设备                    Cloudflare                      Kubernetes        │
│   ┌──────────────┐          ┌──────────────┐               ┌──────────────┐    │
│   │ WARP Client  │──────────│ Zero Trust   │───────────────│ cloudflared  │    │
│   │              │ WireGuard│ Edge Network │  Tunnel       │ (Connector)  │    │
│   └──────────────┘          └──────────────┘               └──────────────┘    │
│          │                         │                              │            │
│          │                         │                              │            │
│   ┌──────┴──────┐           ┌──────┴──────┐               ┌──────┴──────┐     │
│   │             │           │             │               │             │      │
│   │ • 设备注册   │           │ • Gateway   │               │ • Services  │      │
│   │ • 设备态势   │           │   DNS Rules │               │   10.96.x.x │      │
│   │ • Split     │           │   HTTP Rules│               │             │      │
│   │   Tunnels   │           │   L4 Rules  │               │ • Pods      │      │
│   │ • Local     │           │             │               │   10.244.x.x│      │
│   │   Fallback  │           │ • Network   │               │             │      │
│   │             │           │   Routes    │               │ • Endpoints │      │
│   │             │           │             │               │             │      │
│   └─────────────┘           └─────────────┘               └─────────────┘      │
│                                                                                 │
│   流量路径:                                                                      │
│   ┌─────────────────────────────────────────────────────────────────────────┐  │
│   │ 1. 用户打开 WARP → 登录组织 → 设备态势检查                                  │  │
│   │ 2. 用户访问 10.244.1.100:8080 (Pod IP)                                   │  │
│   │ 3. WARP 检查 Split Tunnels → 10.244.0.0/16 在 Include 列表中              │  │
│   │ 4. 流量通过 WireGuard 发送到 Cloudflare Edge                              │  │
│   │ 5. Edge 检查 Network Routes → 10.244.0.0/16 路由到 tunnel-xxx            │  │
│   │ 6. Gateway L4 Rules 检查 → 允许 developers 组访问                         │  │
│   │ 7. 流量通过 Tunnel 发送到 cloudflared                                     │  │
│   │ 8. cloudflared 将流量转发到 10.244.1.100:8080                            │  │
│   └─────────────────────────────────────────────────────────────────────────┘  │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## 三、功能模块

### 3.1 网络层 (Networks)

管理 Kubernetes 集群与 Cloudflare 之间的网络连接。

| 功能 | 说明 | Cloudflare API |
|------|------|----------------|
| **Tunnel** | 建立 K8s 到 Cloudflare 的安全隧道 | `ZeroTrust.Tunnels` |
| **WARP Routing** | 启用隧道的私有网络路由能力 | `Tunnels.Cloudflared.Configurations` |
| **Network Routes** | 定义 CIDR → Tunnel 的路由映射 | `ZeroTrust.Networks.Routes` |
| **Virtual Networks** | 虚拟网络隔离（多环境/多租户） | `ZeroTrust.Networks.VirtualNetworks` |
| **WARP Connector** | 站点到站点双向连接 | `ZeroTrust.Tunnels.WARPConnector` |

### 3.2 设备层 (Devices)

管理用户设备的注册、态势检查和配置策略。

| 功能 | 说明 | Cloudflare API |
|------|------|----------------|
| **Device Posture Rules** | 设备安全态势检查规则 | `ZeroTrust.Devices.Posture` |
| **Device Posture Integrations** | 第三方安全集成 | `ZeroTrust.Devices.Posture.Integrations` |
| **Device Settings Policies** | 设备配置策略（含 Split Tunnels） | `ZeroTrust.Devices.Policies` |
| **Device Networks** | 设备网络配置 | `ZeroTrust.Devices.Networks` |
| **Local Domain Fallback** | 本地域名 DNS 回退 | `Devices.Policies.FallbackDomains` |

**设备态势检查类型**:

| 类型 | 说明 |
|------|------|
| `disk_encryption` | 磁盘加密 |
| `firewall` | 防火墙状态 |
| `os_version` | 操作系统版本 |
| `domain_joined` | 域加入状态 |
| `client_certificate` | 客户端证书 |
| `warp` | WARP 客户端状态 |
| `file` | 文件存在性检查 |
| `application` | 应用程序检查 |
| `crowdstrike_s2s` | CrowdStrike 集成 |
| `sentinelone` | SentinelOne 集成 |
| `intune` | Microsoft Intune 集成 |
| `kolide` | Kolide 集成 |
| `tanium` | Tanium 集成 |
| `workspace_one` | VMware Workspace ONE |

### 3.3 网关层 (Gateway)

管理网络流量的过滤、检查和控制策略。

| 功能 | 说明 | Cloudflare API |
|------|------|----------------|
| **Gateway Rules** | 网关规则 | `ZeroTrust.Gateway.Rules` |
| **Gateway Lists** | IP/域名/URL 列表 | `ZeroTrust.Gateway.Lists` |
| **Gateway Locations** | DNS 解析位置 | `ZeroTrust.Gateway.Locations` |
| **Gateway Certificates** | TLS 解密证书 | `ZeroTrust.Gateway.Certificates` |
| **Gateway Configuration** | 全局配置 | `ZeroTrust.Gateway.Configurations` |

**Gateway 规则类型**:

| Filter | 说明 | 适用场景 |
|--------|------|----------|
| `dns` | DNS 查询过滤 | 域名阻止、DNS 重定向 |
| `http` | HTTP 流量检查 | URL 过滤、内容检查 |
| `l4` | 网络层控制 | IP/端口访问控制 |
| `egress` | 出口流量控制 | 固定出口 IP |

**Gateway 规则动作**:

| Action | 说明 |
|--------|------|
| `allow` | 允许访问 |
| `block` | 阻止访问 |
| `isolate` | 浏览器隔离 |
| `scan` | 恶意软件扫描 |
| `override` | DNS 覆盖 |
| `safesearch` | 强制安全搜索 |
| `ytrestricted` | YouTube 限制模式 |
| `resolve` | DNS 解析 |
| `quarantine` | 隔离 |

### 3.4 身份层 (Access)

管理应用程序的身份验证和授权（主要用于公开访问场景）。

| 功能 | 说明 | Cloudflare API |
|------|------|----------------|
| **Access Applications** | 受保护的应用 | `ZeroTrust.Access.Applications` |
| **Access Groups** | 访问组定义 | `ZeroTrust.Access.Groups` |
| **Access Policies** | 访问策略 | `Access.Applications.Policies` |
| **Service Tokens** | 服务到服务认证 | `ZeroTrust.Access.ServiceTokens` |
| **Identity Providers** | 身份提供商配置 | `ZeroTrust.IdentityProviders` |
| **mTLS Certificates** | 客户端证书认证 | `ZeroTrust.Access.Certificates` |
| **Custom Pages** | 自定义页面 | `ZeroTrust.Access.CustomPages` |
| **Tags** | 应用标签 | `ZeroTrust.Access.Tags` |
| **Bookmarks** | 书签应用 | `ZeroTrust.Access.Bookmarks` |

**支持的身份提供商**:

| 类型 | 说明 |
|------|------|
| `azuread` | Microsoft Azure AD |
| `okta` | Okta |
| `google` | Google OAuth |
| `google-apps` | Google Workspace |
| `github` | GitHub |
| `oidc` | 通用 OpenID Connect |
| `saml` | 通用 SAML 2.0 |
| `onelogin` | OneLogin |
| `pingone` | PingOne |
| `centrify` | Centrify |
| `linkedin` | LinkedIn |
| `facebook` | Facebook |
| `onetimepin` | 一次性密码 |

### 3.5 DNS 管理

| 功能 | 说明 | 集成方式 |
|------|------|----------|
| **公开 DNS** | 公开域名 CNAME 记录 | 原生管理 / external-dns |
| **内部 DNS** | 私有域名解析 | Gateway DNS Rules + Local Fallback |

---

## 四、CRD 设计

### 4.1 CRD 总览

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              CRD 架构总览                                        │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│   网络层 (Networks)                    设备层 (Devices)                          │
│   ┌─────────────────────────┐         ┌─────────────────────────┐              │
│   │ Tunnel (ns)             │         │ DevicePostureRule (ns)  │              │
│   │ ClusterTunnel (cluster) │         │ DevicePosture           │              │
│   │ NetworkRoute (ns)       │         │   Integration (cluster) │              │
│   │ VirtualNetwork (cluster)│         │ DeviceSettings          │              │
│   │ WARPConnector (ns)      │         │   Policy (ns)           │              │
│   └─────────────────────────┘         └─────────────────────────┘              │
│              │                                   │                              │
│              └───────────────┬───────────────────┘                              │
│                              │                                                  │
│                              ▼                                                  │
│   ┌─────────────────────────────────────────────────────────────────────────┐  │
│   │                        服务暴露层                                         │  │
│   │  ┌─────────────────────────┐    ┌─────────────────────────┐             │  │
│   │  │ TunnelBinding (ns)      │    │ PrivateService (ns)     │             │  │
│   │  │ (公开访问)               │    │ (私有访问)               │             │  │
│   │  └─────────────────────────┘    └─────────────────────────┘             │  │
│   │                      │                        │                          │  │
│   │                      ▼                        ▼                          │  │
│   │              Ingress (标准 K8s)         NetworkRoute                     │  │
│   └─────────────────────────────────────────────────────────────────────────┘  │
│                              │                                                  │
│              ┌───────────────┴───────────────┐                                  │
│              ▼                               ▼                                  │
│   ┌─────────────────────────┐    ┌─────────────────────────┐                   │
│   │ 网关层 (Gateway)        │    │ 身份层 (Access)         │                   │
│   │ GatewayRule (ns)        │    │ AccessApplication (ns)  │                   │
│   │ GatewayList (ns)        │    │ AccessGroup (ns)        │                   │
│   │ GatewayLocation (ns)    │    │ AccessPolicy (ns)       │                   │
│   │ GatewayConfiguration    │    │ AccessServiceToken (ns) │                   │
│   │   (cluster)             │    │ AccessIdentity          │                   │
│   └─────────────────────────┘    │   Provider (cluster)    │                   │
│                                  │ AccessCertificate (ns)  │                   │
│                                  └─────────────────────────┘                   │
│                                                                                 │
│   DNS 层                                                                        │
│   ┌─────────────────────────┐                                                  │
│   │ DNSRecord (ns)          │ ←── 与 external-dns 互补                         │
│   └─────────────────────────┘                                                  │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘

ns = Namespaced, cluster = Cluster-scoped
```

### 4.2 CRD 列表

#### 网络层 CRD

| CRD | 作用域 | API 版本 | 说明 |
|-----|--------|----------|------|
| `Tunnel` | Namespaced | v1alpha1 | 命名空间级隧道 |
| `ClusterTunnel` | Cluster | v1alpha1 | 集群级隧道 |
| `NetworkRoute` | Namespaced | v1alpha1 | 私有网络路由 (CIDR → Tunnel) |
| `VirtualNetwork` | Cluster | v1alpha1 | 虚拟网络定义 |
| `WARPConnector` | Namespaced | v1alpha1 | WARP 站点连接器 |

#### 服务暴露层 CRD

| CRD | 作用域 | API 版本 | 说明 |
|-----|--------|----------|------|
| `TunnelBinding` | Namespaced | v1alpha1 | 公开访问：Service → 公开域名 |
| `PrivateService` | Namespaced | v1alpha1 | 私有访问：Service → 私有路由 |

#### 设备层 CRD

| CRD | 作用域 | API 版本 | 说明 |
|-----|--------|----------|------|
| `DevicePostureRule` | Namespaced | v1alpha1 | 设备态势检查规则 |
| `DevicePostureIntegration` | Cluster | v1alpha1 | 第三方安全集成 |
| `DeviceSettingsPolicy` | Namespaced | v1alpha1 | 设备设置策略 |

#### 网关层 CRD

| CRD | 作用域 | API 版本 | 说明 |
|-----|--------|----------|------|
| `GatewayRule` | Namespaced | v1alpha1 | 网关规则 |
| `GatewayList` | Namespaced | v1alpha1 | IP/域名/URL 列表 |
| `GatewayLocation` | Namespaced | v1alpha1 | DNS 解析位置 |
| `GatewayConfiguration` | Cluster | v1alpha1 | 网关全局配置 |

#### 身份层 CRD

| CRD | 作用域 | API 版本 | 说明 |
|-----|--------|----------|------|
| `AccessApplication` | Namespaced | v1alpha1 | Access 应用 |
| `AccessGroup` | Namespaced | v1alpha1 | 访问组 |
| `AccessPolicy` | Namespaced | v1alpha1 | 可复用访问策略 |
| `AccessServiceToken` | Namespaced | v1alpha1 | 服务令牌 |
| `AccessIdentityProvider` | Cluster | v1alpha1 | 身份提供商 |
| `AccessCertificate` | Namespaced | v1alpha1 | mTLS 证书 |

#### DNS 层 CRD

| CRD | 作用域 | API 版本 | 说明 |
|-----|--------|----------|------|
| `DNSRecord` | Namespaced | v1alpha1 | DNS 记录 |

**总计: 21 个 CRD**

---

## 五、CRD 详细定义

### 5.1 Tunnel / ClusterTunnel

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: ClusterTunnel
metadata:
  name: main-tunnel
spec:
  # 隧道名称（创建新隧道时使用）
  newTunnel:
    name: "k8s-main-tunnel"

  # 或使用现有隧道
  # existingTunnel:
  #   id: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  #   name: "existing-tunnel"

  # Cloudflare 配置
  cloudflare:
    accountId: ""                         # 或从 Secret 读取
    domain: "example.com"                 # 主域名
    secret: "cloudflare-credentials"      # 包含 API Token 的 Secret

  # WARP 路由配置（私有网络访问）
  warpRouting:
    enabled: true                         # 启用私有网络路由

  # 虚拟网络
  virtualNetworkRef:
    name: "production-vnet"

  # cloudflared 配置
  cloudflared:
    image: "cloudflare/cloudflared:latest"
    replicas: 2
    protocol: "auto"                      # auto | quic | http2
    noTlsVerify: false
    originCaPool: ""                      # CA 证书 Secret

    # 资源配置
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
      limits:
        cpu: "500m"
        memory: "512Mi"

    # 节点选择
    nodeSelector: {}
    tolerations: []
    affinity: {}

  # 回退目标
  fallbackTarget: "http_status:404"

status:
  tunnelId: ""
  tunnelName: ""
  accountId: ""
  zoneId: ""
  ready: false
  conditions: []
```

### 5.2 NetworkRoute

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: NetworkRoute
metadata:
  name: k8s-pod-network
  namespace: cloudflare-system
spec:
  # 路由的 CIDR 网段
  network: "10.244.0.0/16"
  comment: "Kubernetes Pod Network"

  # 关联的 Tunnel
  tunnelRef:
    kind: ClusterTunnel                   # Tunnel | ClusterTunnel
    name: "main-tunnel"

  # 虚拟网络（可选，用于隔离）
  virtualNetworkRef:
    name: "production-vnet"

  # Cloudflare 凭证
  cloudflareRef:
    accountId: ""
    secretRef:
      name: "cloudflare-credentials"
      namespace: "cloudflare-system"

status:
  routeId: ""
  tunnelId: ""
  virtualNetworkId: ""
  network: ""
  ready: false
  conditions: []
```

### 5.3 VirtualNetwork

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: VirtualNetwork
metadata:
  name: production-vnet
spec:
  name: "Production Network"
  comment: "Production environment virtual network"
  isDefault: false

  cloudflareRef:
    accountId: ""
    secretRef:
      name: "cloudflare-credentials"
      namespace: "cloudflare-system"

status:
  virtualNetworkId: ""
  isDefault: false
  ready: false
  conditions: []
```

### 5.4 PrivateService

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: PrivateService
metadata:
  name: internal-api
  namespace: production
spec:
  # 要暴露的服务
  serviceRef:
    name: "api-server"
    port: 8080

  # 私有访问配置
  privateAccess:
    # 通过 Service ClusterIP 访问
    exposeClusterIP: true

    # 通过内部域名访问（可选）
    internalDNS:
      enabled: true
      hostname: "api.internal.corp.com"
      # 配置本地域名回退
      localFallback:
        enabled: true
        dnsServers:
          - "10.0.0.53"

  # 关联的 Tunnel
  tunnelRef:
    kind: ClusterTunnel
    name: "main-tunnel"

  # 自动创建 NetworkRoute（可选）
  autoCreateRoute:
    enabled: true
    virtualNetworkRef:
      name: "production-vnet"

  # Gateway 访问策略（可选）
  gatewayPolicy:
    # 身份匹配（Wirefilter 语法）
    identity: 'identity.email matches ".*@example.com"'
    # 设备态势匹配
    devicePosture: 'any(device_posture.checks.passed[*] == "disk_encryption")'

status:
  clusterIP: ""
  internalHostname: ""
  routeId: ""
  gatewayRuleId: ""
  ready: false
  conditions: []
```

### 5.5 TunnelBinding（增强版）

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: TunnelBinding
metadata:
  name: public-api
  namespace: production
spec:
  # 要绑定的服务
  subjects:
    - name: api-gateway
      kind: Service
      spec:
        fqdn: "api.example.com"
        protocol: https
        path: "/*"

        # Origin 请求配置
        originRequest:
          noTlsVerify: false
          http2Origin: false
          connectTimeout: "30s"
          httpHostHeader: ""

  # Tunnel 引用
  tunnelRef:
    kind: ClusterTunnel
    name: "main-tunnel"
    disableDNSUpdates: false

  # Access 配置（公开访问保护）
  accessConfig:
    enabled: true
    type: self_hosted

    # 认证设置
    authentication:
      allowedIdps: []
      sessionDuration: "24h"
      autoRedirectToIdentity: false

    # 外观设置
    appearance:
      appLauncherVisible: true
      logoUrl: ""

    # Cookie 设置
    cookies:
      sameSiteAttribute: lax
      httpOnlyAttribute: true
      enableBindingCookie: false

    # 策略
    policies:
      - name: "allow-employees"
        decision: allow
        include:
          - groupRef: "employees"
          - emailDomain: "example.com"
        require:
          - devicePostureRef: "disk-encryption"

  # 或引用独立 AccessApplication
  accessApplicationRef:
    name: "api-access-app"

status:
  hostnames: []
  services: []
  accessApplicationId: ""
  ready: false
  conditions: []
```

### 5.6 DeviceSettingsPolicy

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: DeviceSettingsPolicy
metadata:
  name: default-policy
  namespace: cloudflare-system
spec:
  name: "Default Device Policy"
  description: "Default policy for all devices"
  enabled: true
  precedence: 100
  default: false

  # 匹配条件（Wirefilter 语法）
  match: 'identity.email matches ".*@example.com"'

  # Split Tunnels 配置
  # Include 模式：只有这些流量走 WARP
  splitTunnelInclude:
    - address: "10.0.0.0/8"
      description: "Internal networks"
    - address: "192.168.0.0/16"
      description: "Private networks"
    - host: "*.internal.corp.com"
      description: "Internal domains"

  # 或 Exclude 模式：这些流量不走 WARP（二选一）
  # splitTunnelExclude:
  #   - address: "192.168.1.0/24"
  #     description: "Local network"

  # 本地域名回退
  localDomainFallback:
    - suffix: "internal.corp.com"
      description: "Internal DNS"
      dnsServers:
        - "10.0.0.53"
        - "10.0.0.54"
    - suffix: "corp.local"
      description: "Corp DNS"
      dnsServers:
        - "10.1.0.53"

  # WARP 客户端设置
  warpSettings:
    allowModeSwitch: false
    allowUpdates: true
    allowedToLeave: false
    autoConnect: 0                        # 秒，0=禁用
    captivePortal: 180
    switchLocked: true
    tunnelProtocol: "wireguard"           # wireguard | masque
    disableAutoFallback: false
    excludeOfficeIPs: true

  # 服务模式
  serviceMode:
    mode: "warp"                          # warp | proxy | tunnel_only | posture_only
    port: 0                               # proxy 模式端口

  cloudflareRef:
    accountId: ""
    secretRef:
      name: "cloudflare-credentials"

status:
  policyId: ""
  ready: false
  conditions: []
```

### 5.7 DevicePostureRule

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: DevicePostureRule
metadata:
  name: disk-encryption
  namespace: cloudflare-system
spec:
  name: "Disk Encryption Required"
  description: "Ensure disk encryption is enabled"
  type: disk_encryption

  # 平台匹配
  match:
    - platform: windows
    - platform: mac
    - platform: linux

  # 检查参数（根据 type 不同）
  input:
    # disk_encryption
    requireAll: true
    checkDisks: []

    # os_version 示例
    # version: "10.0.0"
    # operator: ">="                      # < | <= | == | >= | >
    # osDistroName: ""
    # osDistroRevision: ""

    # firewall 示例
    # enabled: true

    # file 示例
    # path: "/etc/security/config"
    # exists: true
    # sha256: ""
    # thumbprint: ""

    # warp 示例
    # versionOperator: ">="
    # version: "2024.1.0"

  # 调度
  schedule: "1h"
  expiration: ""

  cloudflareRef:
    accountId: ""
    secretRef:
      name: "cloudflare-credentials"

status:
  ruleId: ""
  ready: false
  conditions: []
```

### 5.8 GatewayRule

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: GatewayRule
metadata:
  name: allow-internal-access
  namespace: cloudflare-system
spec:
  name: "Allow Internal Network Access"
  description: "Allow developers to access internal services"
  enabled: true
  precedence: 100

  # 规则过滤器类型
  filters:
    - l4                                  # dns | http | l4 | egress

  # 流量匹配（Wirefilter 语法）
  traffic: 'net.dst.ip in {10.244.0.0/16 10.96.0.0/12}'

  # 身份匹配
  identity: 'identity.groups.name[*] in {"developers", "sre"}'

  # 设备态势匹配
  devicePosture: 'any(device_posture.checks.passed[*] in {"disk_encryption", "firewall"})'

  # 动作
  action: allow

  # 规则设置
  ruleSettings:
    # 阻止页面（action=block 时）
    blockPageEnabled: false
    blockReason: ""

    # 浏览器隔离（action=isolate 时）
    bisoAdminControls:
      disableClipboardRedirection: false
      disableCopyPaste: false
      disableDownload: false
      disableKeyboard: false
      disablePrinting: false
      disableUpload: false

    # DNS 覆盖（action=override 时）
    overrideIps: []
    overrideHost: ""

    # 出口配置（filter=egress 时）
    egress:
      ipv4: ""
      ipv6: ""
      ipv4Fallback: ""

    # 载荷日志
    payloadLog:
      enabled: false

    # 不安全设置
    insecureDisableDNSSECValidation: false
    untrustedCert:
      action: error                       # error | block | passthrough

  # 时间表
  schedule:
    timeZone: "Asia/Shanghai"
    monday: "08:00-20:00"
    tuesday: "08:00-20:00"
    wednesday: "08:00-20:00"
    thursday: "08:00-20:00"
    friday: "08:00-20:00"

  cloudflareRef:
    accountId: ""
    secretRef:
      name: "cloudflare-credentials"

status:
  ruleId: ""
  ready: false
  conditions: []
```

### 5.9 GatewayList

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: GatewayList
metadata:
  name: internal-services
  namespace: cloudflare-system
spec:
  name: "Internal Services"
  description: "List of internal service IPs"
  type: IP                                # SERIAL | URL | DOMAIN | EMAIL | IP | HOSTNAME

  # 静态项目
  items:
    - value: "10.244.0.0/16"
      description: "Pod network"
    - value: "10.96.0.0/12"
      description: "Service network"

  # 从 ConfigMap 加载（可选）
  itemsFrom:
    - configMapRef:
        name: "internal-ips"
        key: "ips.txt"

  # 从 Service 自动同步（可选）
  serviceSelector:
    matchLabels:
      expose: "private"
    namespaces:
      - production
      - staging

  cloudflareRef:
    accountId: ""
    secretRef:
      name: "cloudflare-credentials"

status:
  listId: ""
  itemCount: 0
  ready: false
  conditions: []
```

### 5.10 AccessApplication

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: AccessApplication
metadata:
  name: admin-portal
  namespace: production
spec:
  name: "Admin Portal"
  domain: "admin.example.com"
  type: self_hosted                       # self_hosted | saas | bookmark | app_launcher

  # 身份认证
  allowedIdps: []                         # 空=允许所有
  autoRedirectToIdentity: false
  sessionDuration: "24h"

  # Cookie 设置
  enableBindingCookie: false
  httpOnlyCookieAttribute: true
  sameSiteCookieAttribute: "lax"
  pathCookieAttribute: true

  # 外观
  appLauncherVisible: true
  logoUrl: ""

  # 高级配置
  customDenyMessage: ""
  customDenyUrl: ""
  skipInterstitial: false
  serviceAuth401Redirect: false

  # 额外域名
  selfHostedDomains:
    - "admin-v2.example.com"

  # CORS 配置
  corsHeaders:
    allowAllOrigins: false
    allowedOrigins:
      - "https://example.com"
    allowedMethods:
      - "GET"
      - "POST"
    allowedHeaders:
      - "Authorization"
    maxAge: 86400
    allowCredentials: true

  # 内嵌策略
  policies:
    - name: "allow-admins"
      decision: allow
      precedence: 1
      include:
        - group:
            id: ""                        # AccessGroup ID
        - email:
            email: "admin@example.com"
      require:
        - devicePosture:
            integrationId: ""
      sessionDuration: "12h"
      purposeJustificationRequired: true
      purposeJustificationPrompt: "请说明访问原因"

  # 或引用独立策略
  policyRefs:
    - name: "admin-policy"

  # 标签
  tags:
    - "admin"
    - "production"

  cloudflareRef:
    accountId: ""
    secretRef:
      name: "cloudflare-credentials"

status:
  applicationId: ""
  aud: ""
  createdAt: ""
  updatedAt: ""
  ready: false
  conditions: []
```

### 5.11 AccessGroup

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: AccessGroup
metadata:
  name: developers
  namespace: cloudflare-system
spec:
  name: "Developers"

  # Include 规则（OR 逻辑）
  include:
    - emailDomain:
        domain: "example.com"
    - email:
        email: "contractor@external.com"
    - gsuite:
        email: "engineering@example.com"
        identityProviderId: ""
    - okta:
        name: "developers"
        identityProviderId: ""
    - azureAD:
        id: "group-uuid"
        identityProviderId: ""
    - github:
        name: "my-org"
        team: "platform"
        identityProviderId: ""
    - saml:
        attributeName: "department"
        attributeValue: "engineering"
        identityProviderId: ""

  # Require 规则（AND 逻辑）
  require:
    - warp: true
    - certificate: true

  # Exclude 规则（NOT 逻辑）
  exclude:
    - email:
        email: "blocked@example.com"

  cloudflareRef:
    accountId: ""
    secretRef:
      name: "cloudflare-credentials"

status:
  groupId: ""
  createdAt: ""
  updatedAt: ""
  ready: false
  conditions: []
```

### 5.12 AccessIdentityProvider

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: AccessIdentityProvider
metadata:
  name: okta-sso
spec:
  name: "Okta SSO"
  type: okta

  config:
    # Okta 配置
    oktaAccount: "example.okta.com"
    clientId: "xxx"
    clientSecretRef:
      name: "okta-credentials"
      key: "client-secret"
    apiTokenRef:
      name: "okta-credentials"
      key: "api-token"

    # OIDC 通用配置
    # authUrl: ""
    # tokenUrl: ""
    # certsUrl: ""
    # scopes: ["openid", "email", "profile"]
    # emailClaimName: "email"
    # pkceEnabled: false

    # SAML 配置
    # ssoTargetUrl: ""
    # issuerUrl: ""
    # idpPublicCertRef:
    #   name: "saml-cert"
    #   key: "cert.pem"

  # SCIM 配置
  scimConfig:
    enabled: true
    secretRef:
      name: "scim-token"
      key: "token"
    userDeprovision: false
    seatDeprovision: false

  cloudflareRef:
    accountId: ""
    secretRef:
      name: "cloudflare-credentials"

status:
  providerId: ""
  ready: false
  conditions: []
```

### 5.13 DNSRecord

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: DNSRecord
metadata:
  name: api-record
  namespace: production
  annotations:
    # 与 external-dns 互操作
    external-dns.alpha.kubernetes.io/hostname: "api.example.com"
spec:
  type: CNAME                             # A | AAAA | CNAME | TXT | MX | SRV | NS | CAA
  name: "api.example.com"

  # 静态内容
  content: "xxx.cfargotunnel.com"

  # 或从 Tunnel 自动获取
  tunnelRef:
    kind: ClusterTunnel
    name: "main-tunnel"

  # 设置
  ttl: 1                                  # 1 = auto
  proxied: true
  priority: 0                             # MX, SRV 专用
  comment: "Managed by cloudflare-operator"
  tags:
    - "kubernetes"
    - "production"

  # Zone 选择
  zoneSelector:
    zoneName: "example.com"
    # 或
    # zoneId: ""
    # matchLabels: {}

  cloudflareRef:
    secretRef:
      name: "cloudflare-credentials"

status:
  recordId: ""
  zoneId: ""
  zoneName: ""
  proxied: true
  ready: false
  conditions: []
```

---

## 六、Annotations 规范

### 6.1 通用 Annotations

| Annotation | 说明 | 示例 |
|------------|------|------|
| `cloudflare.com/managed` | 标记资源由 Operator 管理 | `"true"` |
| `cloudflare.com/account-id` | Cloudflare 账户 ID | `"xxx"` |
| `cloudflare.com/zone-id` | DNS Zone ID | `"xxx"` |

### 6.2 Tunnel Annotations

| Annotation | 说明 | 示例 |
|------------|------|------|
| `cloudflare.com/tunnel-ref` | 引用 Tunnel | `"ClusterTunnel/main"` |
| `cloudflare.com/tunnel-id` | Tunnel ID | `"xxx"` |

### 6.3 Access Annotations (用于 Ingress)

| Annotation | 说明 | 示例 |
|------------|------|------|
| `cloudflare.com/access-enabled` | 启用 Access 保护 | `"true"` |
| `cloudflare.com/access-application` | 引用 AccessApplication | `"my-app"` |
| `cloudflare.com/access-allowed-idps` | 允许的 IdP | `"okta,azure"` |
| `cloudflare.com/access-session-duration` | 会话时长 | `"24h"` |

### 6.4 Backend Annotations (用于 Ingress)

| Annotation | 说明 | 示例 |
|------------|------|------|
| `cloudflare.com/backend-protocol` | 后端协议 | `"https"` |
| `cloudflare.com/proxy-ssl-verify` | SSL 验证 | `"on"` / `"off"` |
| `cloudflare.com/http-host-header` | Host 头 | `"internal.example.com"` |
| `cloudflare.com/origin-server-name` | 源服务器名 | `"internal.example.com"` |
| `cloudflare.com/connect-timeout` | 连接超时 | `"30s"` |

### 6.5 DNS Annotations

| Annotation | 说明 | 示例 |
|------------|------|------|
| `cloudflare.com/dns-proxied` | 启用 Cloudflare 代理 | `"true"` |
| `cloudflare.com/dns-ttl` | TTL 值 | `"120"` |

### 6.6 external-dns 兼容 Annotations

| Annotation | 说明 |
|------------|------|
| `external-dns.alpha.kubernetes.io/hostname` | DNS 主机名 |
| `external-dns.alpha.kubernetes.io/ttl` | TTL |
| `external-dns.alpha.kubernetes.io/cloudflare-proxied` | Cloudflare 代理 |

---

## 七、Cloudflare API Token 权限

### 7.1 最小权限集

```yaml
Account Permissions:
  - Cloudflare Tunnel: Edit
  - Access: Apps and Policies: Edit
  - Access: Organizations, Identity Providers, and Groups: Edit
  - Access: Service Tokens: Edit
  - Access: Device Posture: Edit
  - Zero Trust: Edit
  - Zero Trust Gateway: Edit

Zone Permissions:
  - DNS: Edit
  - Zone: Read
```

### 7.2 权限与功能对应

| 功能 | 所需权限 |
|------|----------|
| Tunnel 管理 | `Cloudflare Tunnel: Edit` |
| Network Routes | `Cloudflare Tunnel: Edit` |
| Virtual Networks | `Cloudflare Tunnel: Edit` |
| Access Applications | `Access: Apps and Policies: Edit` |
| Access Groups | `Access: Organizations, Identity Providers, and Groups: Edit` |
| Access Policies | `Access: Apps and Policies: Edit` |
| Identity Providers | `Access: Organizations, Identity Providers, and Groups: Edit` |
| Service Tokens | `Access: Service Tokens: Edit` |
| Device Posture | `Access: Device Posture: Edit` |
| Gateway Rules | `Zero Trust Gateway: Edit` |
| Gateway Lists | `Zero Trust Gateway: Edit` |
| Device Settings | `Zero Trust: Edit` |
| DNS Records | `DNS: Edit`, `Zone: Read` |

---

## 八、实现优先级

### Phase 0: 基础迁移
- [ ] API Group 迁移到 `cloudflare.com`
- [ ] 合并 Bug 修复 PR (#178, #166, #158, #140)
- [ ] Tunnel/ClusterTunnel 向后兼容
- [ ] 更新 CLAUDE.md 文档

### Phase 1: 私有访问核心
- [ ] Tunnel 增强（WARP Routing 配置）
- [ ] NetworkRoute CRD
- [ ] VirtualNetwork CRD
- [ ] PrivateService CRD
- [ ] DevicePostureRule CRD
- [ ] DeviceSettingsPolicy CRD（含 Split Tunnels）
- [ ] GatewayRule CRD（L4 规则）

### Phase 2: 公开访问 + 网关
- [ ] TunnelBinding 增强（Access 集成）
- [ ] AccessApplication CRD
- [ ] AccessGroup CRD
- [ ] AccessPolicy CRD
- [ ] GatewayRule CRD（DNS/HTTP 规则）
- [ ] GatewayList CRD
- [ ] DNSRecord CRD

### Phase 3: 身份集成
- [ ] AccessIdentityProvider CRD
- [ ] AccessServiceToken CRD
- [ ] AccessCertificate CRD
- [ ] DevicePostureIntegration CRD

### Phase 4: Ingress + 高级功能
- [ ] Ingress Controller
- [ ] WARPConnector CRD
- [ ] GatewayLocation CRD
- [ ] GatewayConfiguration CRD

### Phase 5: 文档与测试
- [ ] 完整文档
- [ ] E2E 测试
- [ ] 示例配置
- [ ] Helm Chart

---

## 九、目录结构

```
cloudflare-operator/
├── api/
│   └── v1alpha1/
│       ├── tunnel_types.go
│       ├── clustertunnel_types.go
│       ├── tunnelbinding_types.go
│       ├── networkroute_types.go
│       ├── virtualnetwork_types.go
│       ├── privateservice_types.go
│       ├── warpconnector_types.go
│       ├── deviceposturerule_types.go
│       ├── devicepostureintegration_types.go
│       ├── devicesettingspolicy_types.go
│       ├── gatewayrule_types.go
│       ├── gatewaylist_types.go
│       ├── gatewaylocation_types.go
│       ├── gatewayconfiguration_types.go
│       ├── accessapplication_types.go
│       ├── accessgroup_types.go
│       ├── accesspolicy_types.go
│       ├── accessservicetoken_types.go
│       ├── accessidentityprovider_types.go
│       ├── accesscertificate_types.go
│       ├── dnsrecord_types.go
│       ├── groupversion_info.go
│       ├── common_types.go              # 共享类型定义
│       └── zz_generated.deepcopy.go
├── internal/
│   ├── controller/
│   │   ├── tunnel/
│   │   ├── networkroute/
│   │   ├── virtualnetwork/
│   │   ├── privateservice/
│   │   ├── tunnelbinding/
│   │   ├── device/
│   │   ├── gateway/
│   │   ├── access/
│   │   ├── dns/
│   │   └── ingress/
│   ├── clients/
│   │   └── cloudflare/
│   │       ├── client.go
│   │       ├── tunnel.go
│   │       ├── network.go
│   │       ├── device.go
│   │       ├── gateway.go
│   │       ├── access.go
│   │       └── dns.go
│   └── webhook/
├── config/
│   ├── crd/
│   ├── rbac/
│   ├── manager/
│   ├── webhook/
│   └── samples/
├── docs/
│   ├── design/
│   ├── configuration/
│   ├── examples/
│   └── migrations/
├── test/
│   └── e2e/
├── hack/
├── cmd/
│   └── main.go
├── Dockerfile
├── Makefile
├── go.mod
├── go.sum
├── PROJECT
├── CLAUDE.md
└── README.md
```

---

## 十、参考资源

### 10.1 Cloudflare 文档

- [Zero Trust Documentation](https://developers.cloudflare.com/cloudflare-one/)
- [Cloudflare API Reference](https://developers.cloudflare.com/api/)
- [cloudflare-go SDK](https://github.com/cloudflare/cloudflare-go)

### 10.2 社区项目参考

- [adyanth/cloudflare-operator](https://github.com/adyanth/cloudflare-operator) - Tunnel 实现参考
- [BojanZelic/cloudflare-zero-trust-operator](https://github.com/BojanZelic/cloudflare-zero-trust-operator) - Access 实现参考
- [STRRL/cloudflare-tunnel-ingress-controller](https://github.com/STRRL/cloudflare-tunnel-ingress-controller) - Ingress 实现参考

### 10.3 相关 PR

- [PR #115](https://github.com/adyanth/cloudflare-operator/pull/115) - Access Config 支持
- [PR #166](https://github.com/adyanth/cloudflare-operator/pull/166) - DNS FQDN 变更清理
- [PR #158](https://github.com/adyanth/cloudflare-operator/pull/158) - Tunnel Secret Finalizer
- [PR #140](https://github.com/adyanth/cloudflare-operator/pull/140) - Dummy TunnelBinding
- [PR #178](https://github.com/adyanth/cloudflare-operator/pull/178) - Leader Election 修复
