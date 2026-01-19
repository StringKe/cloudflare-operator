<h1 align=center>Cloudflare Zero Trust Operator</h1>

<div align="center">
  <a href="https://github.com/StringKe/cloudflare-operator">
    <img src="docs/images/CloudflareOperatorLogo.png" alt="Logo" height="250">
  </a>
  <br />

  <p align="center">
    Cloudflare Zero Trust 的 Kubernetes Operator：隧道、访问控制、网关、设备、DNS、R2 和规则管理
    <br />
    <br />
    <a href="https://github.com/StringKe/cloudflare-operator/blob/main/docs/en/README.md"><strong>Documentation (English) »</strong></a>
    |
    <a href="https://github.com/StringKe/cloudflare-operator/blob/main/docs/zh/README.md"><strong>文档 (中文) »</strong></a>
    <br />
    <br />
    <a href="https://github.com/StringKe/cloudflare-operator/tree/main/examples">示例</a>
    ·
    <a href="https://github.com/StringKe/cloudflare-operator/issues">报告 Bug</a>
    ·
    <a href="https://github.com/StringKe/cloudflare-operator/issues">功能请求</a>
  </p>
</div>

<div align="center">

[![GitHub license](https://img.shields.io/github/license/StringKe/cloudflare-operator?color=brightgreen)](https://github.com/StringKe/cloudflare-operator/blob/main/LICENSE)
[![GitHub release](https://img.shields.io/github/v/release/StringKe/cloudflare-operator)](https://github.com/StringKe/cloudflare-operator/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/StringKe/cloudflare-operator)](https://goreportcard.com/report/github.com/StringKe/cloudflare-operator)
[![CI](https://github.com/StringKe/cloudflare-operator/actions/workflows/release.yml/badge.svg)](https://github.com/StringKe/cloudflare-operator/actions/workflows/release.yml)
[![Test](https://github.com/StringKe/cloudflare-operator/actions/workflows/test.yml/badge.svg)](https://github.com/StringKe/cloudflare-operator/actions/workflows/test.yml)
[![Lint](https://github.com/StringKe/cloudflare-operator/actions/workflows/lint.yml/badge.svg)](https://github.com/StringKe/cloudflare-operator/actions/workflows/lint.yml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/StringKe/cloudflare-operator/badge)](https://securityscorecards.dev/viewer/?uri=github.com/StringKe/cloudflare-operator)

</div>

> **注意**: 此项目目前处于 Alpha 阶段 (v0.26.x)。这**不是** Cloudflare 官方产品，它使用 [Cloudflare API](https://api.cloudflare.com/) 和 [cloudflared](https://github.com/cloudflare/cloudflared) 在 Kubernetes 上自动化 Zero Trust 配置。
>
> 本项目 Fork 自 [adyanth/cloudflare-operator](https://github.com/adyanth/cloudflare-operator)，在原项目基础上扩展了完整的 Zero Trust 功能。

## 概述

Cloudflare Zero Trust Operator 提供 Kubernetes 原生的 Cloudflare Zero Trust 资源管理。基于 `kubebuilder` 和 `controller-runtime` 构建，通过自定义资源定义 (CRD) 实现隧道、访问策略、网关规则、设备设置、R2 存储和区域规则的声明式配置。

## 功能特性

| 类别 | 功能 |
|------|------|
| **隧道管理** | 创建/管理 Cloudflare Tunnel，自动部署 cloudflared，服务绑定与 DNS |
| **私有网络** | 虚拟网络、网络路由、通过 WARP 暴露私有服务 |
| **访问控制** | Zero Trust 应用、访问组、访问策略、身份提供商、服务令牌 |
| **网关与安全** | 网关规则 (DNS/HTTP/L4)、网关列表、浏览器隔离 |
| **设备管理** | Split Tunnel 配置、回退域、设备态势规则 |
| **DNS 与连接** | DNS 记录管理、WARP Connector 站点连接 |
| **域名管理** | Zone 设置 (SSL/TLS、缓存、安全)、Origin CA 证书 |
| **R2 存储** | R2 存储桶、自定义域名、事件通知 |
| **规则引擎** | Zone 规则集、转换规则 (URL/Header)、重定向规则 |
| **Cloudflare Pages** | Pages 项目、自定义域名、部署管理 |
| **域名注册** | 域名注册管理 (Enterprise) |
| **Kubernetes 集成** | 原生 Ingress 支持、Gateway API 支持 (Gateway, HTTPRoute, TCPRoute, UDPRoute) |

## 架构

本 Operator 采用**统一同步架构**，包含六层设计以确保并发安全并消除竞态条件：

```mermaid
flowchart TB
    subgraph Internet["互联网"]
        Users["用户 / WARP 客户端"]
    end

    subgraph Cloudflare["Cloudflare 边缘"]
        Edge["Cloudflare 边缘网络"]
        API["Cloudflare API"]
    end

    subgraph K8s["Kubernetes 集群"]
        subgraph Layer1["Layer 1: K8s 资源"]
            CRDs["自定义资源<br/>(Tunnel, DNSRecord, AccessApp 等)"]
            K8sNative["Kubernetes 原生<br/>(Ingress, Gateway API)"]
        end

        subgraph Layer2["Layer 2: 资源控制器"]
            RC["资源控制器<br/>(轻量级，每个 100-150 行)"]
        end

        subgraph Layer3["Layer 3: 核心服务"]
            SVC["核心服务<br/>(TunnelConfigService, DNSService 等)"]
        end

        subgraph Layer4["Layer 4: SyncState CRD"]
            SyncState["CloudflareSyncState<br/>(乐观锁共享状态)"]
        end

        subgraph Layer5["Layer 5: 同步控制器"]
            SC["同步控制器<br/>(防抖、聚合、Hash 检测)"]
        end

        subgraph Managed["托管资源"]
            Deployment["cloudflared 部署"]
        end

        subgraph App["应用"]
            Service["Services"]
            Pod["Pods"]
        end
    end

    CRDs -.->|监听| RC
    K8sNative -.->|监听| RC
    RC -->|注册配置| SVC
    SVC -->|更新| SyncState
    SyncState -.->|监听| SC
    SC -->|"API 调用<br/>(唯一同步点)"| API
    SC -->|创建| Managed
    Managed -->|代理| Service
    Service --> Pod
    Users -->|HTTPS/WARP| Edge
    Edge <-->|隧道| Deployment

    style Layer4 fill:#f9f,stroke:#333,stroke-width:2px
    style SC fill:#9f9,stroke:#333,stroke-width:2px
```

### 架构优势

| 特性 | 优势 |
|------|------|
| **单一同步点** | 只有同步控制器调用 Cloudflare API，消除竞态条件 |
| **乐观锁** | SyncState CRD 使用 K8s resourceVersion 实现多实例安全 |
| **防抖** | 500ms 延迟将多次变更聚合为单次 API 调用 |
| **Hash 检测** | 配置无变化时跳过同步，减少 API 调用 |
| **关注点分离** | 每层有明确的单一职责 |

> **说明**: 详细架构设计请参阅 [统一同步架构设计](docs/design/UNIFIED_SYNC_ARCHITECTURE.md)。

## 快速开始

### 前置条件

- Kubernetes 集群 v1.28+
- 启用 Zero Trust 的 Cloudflare 账户
- Cloudflare API Token ([创建 Token](https://dash.cloudflare.com/profile/api-tokens))

### 安装

**方式 1：完整安装（推荐新用户使用）**

```bash
# 一键安装：CRDs + Namespace + RBAC + Operator
kubectl apply -f https://github.com/StringKe/cloudflare-operator/releases/latest/download/cloudflare-operator-full-no-webhook.yaml

# 验证安装
kubectl get pods -n cloudflare-operator-system
```

**方式 2：模块化安装（推荐生产环境使用）**

```bash
# 步骤 1：安装 CRD（需要 cluster-admin 权限）
kubectl apply -f https://github.com/StringKe/cloudflare-operator/releases/latest/download/cloudflare-operator-crds.yaml

# 步骤 2：创建命名空间
kubectl apply -f https://github.com/StringKe/cloudflare-operator/releases/latest/download/cloudflare-operator-namespace.yaml

# 步骤 3：安装 Operator（RBAC + Deployment）
kubectl apply -f https://github.com/StringKe/cloudflare-operator/releases/latest/download/cloudflare-operator-no-webhook.yaml

# 验证安装
kubectl get pods -n cloudflare-operator-system
```

**可用安装文件**

| 文件 | 内容 | 使用场景 |
|------|------|----------|
| `cloudflare-operator-full.yaml` | CRDs + Namespace + RBAC + Operator + Webhook | 完整安装（需要 cert-manager）|
| `cloudflare-operator-full-no-webhook.yaml` | CRDs + Namespace + RBAC + Operator | 完整安装（无 webhook）|
| `cloudflare-operator-crds.yaml` | 仅 CRDs | 模块化安装 |
| `cloudflare-operator-namespace.yaml` | 仅 Namespace | 模块化安装 |
| `cloudflare-operator.yaml` | RBAC + Operator + Webhook | 升级现有安装 |
| `cloudflare-operator-no-webhook.yaml` | RBAC + Operator | 升级（无 webhook）|

### 创建隧道

```yaml
# 1. 创建 API 凭证 secret
apiVersion: v1
kind: Secret
metadata:
  name: cloudflare-credentials
type: Opaque
stringData:
  CLOUDFLARE_API_TOKEN: "<your-api-token>"
---
# 2. 创建隧道
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: Tunnel
metadata:
  name: my-tunnel
spec:
  newTunnel:
    name: k8s-tunnel
  cloudflare:
    accountId: "<your-account-id>"
    domain: example.com
    secret: cloudflare-credentials
```

### 暴露服务

```yaml
apiVersion: networking.cfargotunnel.com/v1alpha1
kind: TunnelBinding
metadata:
  name: web-binding
subjects:
  - kind: Service
    name: web-app
    spec:
      fqdn: app.example.com
      protocol: http
tunnelRef:
  kind: Tunnel
  name: my-tunnel
```

## CRD 参考

### 凭证与配置

| CRD | API 版本 | 作用域 | 说明 |
|-----|---------|--------|------|
| CloudflareCredentials | `networking.cloudflare-operator.io/v1alpha2` | Cluster | Cloudflare API 凭证管理 |
| CloudflareDomain | `networking.cloudflare-operator.io/v1alpha2` | Cluster | Zone 设置 (SSL/TLS、缓存、安全、WAF) |

### 隧道管理

| CRD | API 版本 | 作用域 | 说明 |
|-----|---------|--------|------|
| Tunnel | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | 带托管 cloudflared 的 Cloudflare Tunnel |
| ClusterTunnel | `networking.cloudflare-operator.io/v1alpha2` | Cluster | 集群级 Cloudflare Tunnel |
| TunnelBinding | `networking.cfargotunnel.com/v1alpha1` | Namespaced | 将服务绑定到隧道并配置 DNS |

### 私有网络访问

| CRD | API 版本 | 作用域 | 说明 |
|-----|---------|--------|------|
| VirtualNetwork | `networking.cloudflare-operator.io/v1alpha2` | Cluster | 用于隔离的 Cloudflare 虚拟网络 |
| NetworkRoute | `networking.cloudflare-operator.io/v1alpha2` | Cluster | 通过隧道路由 CIDR |
| PrivateService | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | 通过私有 IP 暴露服务 |

### 访问控制

| CRD | API 版本 | 作用域 | 说明 |
|-----|---------|--------|------|
| AccessApplication | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | Zero Trust 应用 |
| AccessGroup | `networking.cloudflare-operator.io/v1alpha2` | Cluster | 访问策略组 |
| AccessPolicy | `networking.cloudflare-operator.io/v1alpha2` | Cluster | 可复用访问策略 |
| AccessIdentityProvider | `networking.cloudflare-operator.io/v1alpha2` | Cluster | 身份提供商配置 |
| AccessServiceToken | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | M2M 服务令牌 |

### 网关与安全

| CRD | API 版本 | 作用域 | 说明 |
|-----|---------|--------|------|
| GatewayRule | `networking.cloudflare-operator.io/v1alpha2` | Cluster | 网关策略规则 |
| GatewayList | `networking.cloudflare-operator.io/v1alpha2` | Cluster | 网关规则使用的列表 |
| GatewayConfiguration | `networking.cloudflare-operator.io/v1alpha2` | Cluster | 全局网关设置 |

### 设备管理

| CRD | API 版本 | 作用域 | 说明 |
|-----|---------|--------|------|
| DeviceSettingsPolicy | `networking.cloudflare-operator.io/v1alpha2` | Cluster | WARP 客户端设置 |
| DevicePostureRule | `networking.cloudflare-operator.io/v1alpha2` | Cluster | 设备态势检查 |

### DNS 与连接

| CRD | API 版本 | 作用域 | 说明 |
|-----|---------|--------|------|
| DNSRecord | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | DNS 记录管理 |
| WARPConnector | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | WARP Connector 部署 |
| AccessTunnel | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | Access 隧道配置 |

### SSL/TLS 与证书

| CRD | API 版本 | 作用域 | 说明 |
|-----|---------|--------|------|
| OriginCACertificate | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | Cloudflare Origin CA 证书，自动创建 K8s Secret |

### R2 存储

| CRD | API 版本 | 作用域 | 说明 |
|-----|---------|--------|------|
| R2Bucket | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | R2 存储桶，支持生命周期规则 |
| R2BucketDomain | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | R2 存储桶自定义域名 |
| R2BucketNotification | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | R2 存储桶事件通知 |

### 规则引擎

| CRD | API 版本 | 作用域 | 说明 |
|-----|---------|--------|------|
| ZoneRuleset | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | Zone 规则集 (WAF、速率限制等) |
| TransformRule | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | URL 重写和 Header 修改 |
| RedirectRule | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | URL 重定向规则 |

### Cloudflare Pages

| CRD | API 版本 | 作用域 | 说明 |
|-----|---------|--------|------|
| PagesProject | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | Pages 项目（构建配置、资源绑定）|
| PagesDomain | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | Pages 项目自定义域名 |
| PagesDeployment | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | Pages 部署（创建、重试、回滚）|

### 域名注册 (Enterprise)

| CRD | API 版本 | 作用域 | 说明 |
|-----|---------|--------|------|
| DomainRegistration | `networking.cloudflare-operator.io/v1alpha2` | Cluster | 域名注册设置 |

### Kubernetes 集成

| CRD | API 版本 | 作用域 | 说明 |
|-----|---------|--------|------|
| TunnelIngressClassConfig | `networking.cloudflare-operator.io/v1alpha2` | Cluster | Ingress 集成配置 |
| TunnelGatewayClassConfig | `networking.cloudflare-operator.io/v1alpha2` | Cluster | Gateway API 集成配置 |

> **说明**: Operator 还支持原生 Kubernetes `Ingress` 和 Gateway API (`Gateway`, `HTTPRoute`, `TCPRoute`, `UDPRoute`) 资源，需配置相应的 IngressClass 或 GatewayClass。

## 示例

查看 [examples](examples/) 目录获取完整的使用示例：

- **[基础](examples/01-basic/)** - 凭证、隧道、DNS、服务绑定
- **[私有网络](examples/02-private-network/)** - 虚拟网络、路由、私有服务
- **[零信任](examples/03-zero-trust/)** - Access 应用、组、策略、身份提供商
- **[网关](examples/04-gateway/)** - 网关规则、列表
- **[设备](examples/05-device/)** - 设备策略、态势规则
- **[Pages](examples/06-pages/)** - Pages 项目、域名、部署
- **[场景](examples/scenarios/)** - 完整的实际场景

## 文档

| 语言 | 链接 |
|------|------|
| English | [docs/en/README.md](docs/en/README.md) |
| 中文 | [docs/zh/README.md](docs/zh/README.md) |

文档包含：
- 安装指南
- API Token 权限
- 完整 CRD 参考
- 故障排除指南
- 迁移指南 (v1alpha1 → v1alpha2)

## API Token 权限

| 功能 | 权限 | 范围 |
|------|------|------|
| 隧道 | `Account:Cloudflare Tunnel:Edit` | Account |
| DNS | `Zone:DNS:Edit` | Zone |
| Access | `Account:Access: Apps and Policies:Edit` | Account |
| Gateway | `Account:Zero Trust:Edit` | Account |
| Zone 设置 | `Zone:Zone Settings:Edit` | Zone |
| SSL/TLS | `Zone:SSL and Certificates:Edit` | Zone |
| R2 | `Account:Workers R2 Storage:Edit` | Account |
| Pages | `Account:Cloudflare Pages:Edit` | Account |
| 规则 | `Zone:Zone Rulesets:Edit` | Zone |
| 域名注册 | `Account:Registrar:Edit` | Account |

## 贡献

欢迎贡献！请查看 [CONTRIBUTING.md](CONTRIBUTING.md) 了解指南。

## 致谢

本项目 Fork 自 [adyanth/cloudflare-operator](https://github.com/adyanth/cloudflare-operator)。感谢 [@adyanth](https://github.com/adyanth) 及所有原始贡献者在初始实现上的出色工作。

### 主要差异

本 Fork 在原项目基础上扩展了：
- 完整的 Zero Trust 资源支持（Access、Gateway、Device 管理）
- v1alpha2 API 及改进的资源管理
- 原生 Kubernetes Ingress 和 Gateway API 集成
- R2 存储管理（存储桶、自定义域名、通知）
- Cloudflare Pages 支持（项目、自定义域名、部署）
- Zone 设置和规则引擎（SSL/TLS、缓存、WAF、转换/重定向规则）
- Origin CA 证书集成
- 域名注册管理（Enterprise）
- 六层统一同步架构消除竞态条件
- 增强的错误处理和状态报告
- 完善的文档和示例

## 许可证

Apache License 2.0 - 详见 [LICENSE](LICENSE)。
