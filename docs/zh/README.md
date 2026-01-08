# Cloudflare Operator 文档

欢迎阅读 Cloudflare Zero Trust Operator 文档。此 operator 实现了 Kubernetes 原生的 Cloudflare Zero Trust 资源管理。

## 快速导航

| 主题 | 说明 |
|------|------|
| [快速开始](getting-started.md) | 安装和创建第一个隧道 |
| [配置](configuration.md) | API token 和凭证 |
| [API 参考](api-reference/) | 完整 CRD 文档 |
| [指南](guides/) | 常见任务操作指南 |
| [故障排除](troubleshooting.md) | 常见问题和解决方案 |
| [迁移](migration.md) | 从 v1alpha1 升级 |

## 概述

Cloudflare Operator 提供以下 Kubernetes 原生管理功能：

- **隧道** - 从集群到 Cloudflare 边缘的安全连接
- **私有网络访问** - 允许 WARP 客户端访问内部服务
- **访问控制** - 应用的 Zero Trust 认证
- **网关** - DNS/HTTP/L4 安全策略
- **设备管理** - WARP 客户端配置和态势规则

## 架构

```
┌─────────────────────────────────────────────────────────────────┐
│                    Cloudflare Zero Trust                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│  │    隧道     │  │    访问     │  │    网关     │             │
│  │             │  │             │  │             │             │
│  │ • Tunnel    │  │ • 应用      │  │ • 规则      │             │
│  │ • Cluster   │  │ • 组        │  │ • 列表      │             │
│  │   Tunnel    │  │ • IDP       │  │ • 配置      │             │
│  │ • Binding   │  │ • Token     │  │             │             │
│  └─────────────┘  └─────────────┘  └─────────────┘             │
│                                                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│  │    网络     │  │    设备     │  │    DNS      │             │
│  │             │  │             │  │             │             │
│  │ • VNet      │  │ • 策略      │  │ • 记录      │             │
│  │ • 路由      │  │ • 态势      │  │ • WARP      │             │
│  │ • 私有      │  │             │  │   Connector │             │
│  │   服务      │  │             │  │             │             │
│  └─────────────┘  └─────────────┘  └─────────────┘             │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## CRD 摘要

### 隧道管理

| CRD | 作用域 | 说明 |
|-----|--------|------|
| `Tunnel` | Namespaced | 带托管 cloudflared 的 Cloudflare Tunnel |
| `ClusterTunnel` | Cluster | 集群级 Cloudflare Tunnel |
| `TunnelBinding` | Namespaced | 将服务绑定到隧道并配置 DNS |

### 私有网络

| CRD | 作用域 | 说明 |
|-----|--------|------|
| `VirtualNetwork` | Cluster | 流量隔离网络 |
| `NetworkRoute` | Cluster | 通过隧道路由 CIDR |
| `PrivateService` | Namespaced | 通过私有 IP 暴露服务 |

### 访问控制

| CRD | 作用域 | 说明 |
|-----|--------|------|
| `AccessApplication` | Namespaced | Zero Trust 应用 |
| `AccessGroup` | Cluster | 可复用的访问策略组 |
| `AccessIdentityProvider` | Cluster | 身份提供商配置 |
| `AccessServiceToken` | Namespaced | M2M 认证令牌 |

### 网关与安全

| CRD | 作用域 | 说明 |
|-----|--------|------|
| `GatewayRule` | Cluster | DNS/HTTP/L4 策略规则 |
| `GatewayList` | Cluster | 网关规则使用的列表 |
| `GatewayConfiguration` | Cluster | 全局网关设置 |

### 设备管理

| CRD | 作用域 | 说明 |
|-----|--------|------|
| `DeviceSettingsPolicy` | Cluster | WARP 客户端配置 |
| `DevicePostureRule` | Cluster | 设备健康检查规则 |

### DNS 与连接

| CRD | 作用域 | 说明 |
|-----|--------|------|
| `DNSRecord` | Namespaced | DNS 记录管理 |
| `WARPConnector` | Cluster | WARP Connector 部署 |

## 获取帮助

- **示例**: 查看 [/examples](../../examples/) 获取实用示例
- **问题**: [GitHub Issues](https://github.com/StringKe/cloudflare-operator/issues)
- **讨论**: [GitHub Discussions](https://github.com/StringKe/cloudflare-operator/discussions)

## 版本信息

- 当前版本: v0.17.x (Alpha)
- API 版本: `networking.cloudflare-operator.io/v1alpha2`
- Kubernetes: v1.28+
- Go: 1.24+
