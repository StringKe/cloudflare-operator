# Cloudflare Operator Documentation

Welcome to the Cloudflare Zero Trust Operator documentation. This operator enables Kubernetes-native management of Cloudflare Zero Trust resources.

## Quick Navigation

| Topic | Description |
|-------|-------------|
| [Getting Started](getting-started.md) | Installation and first tunnel |
| [Configuration](configuration.md) | API tokens and credentials |
| [API Reference](api-reference/) | Complete CRD documentation |
| [Guides](guides/) | How-to guides for common tasks |
| [Troubleshooting](troubleshooting.md) | Common issues and solutions |
| [Migration](migration.md) | Upgrading from v1alpha1 |

## Overview

The Cloudflare Operator provides Kubernetes-native management of:

- **Tunnels** - Secure connections from your cluster to Cloudflare's edge
- **Private Network Access** - Enable WARP clients to access internal services
- **Access Control** - Zero Trust authentication for applications
- **Gateway** - DNS/HTTP/L4 security policies
- **Device Management** - WARP client configuration and posture rules

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Cloudflare Zero Trust                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│  │   Tunnels   │  │   Access    │  │   Gateway   │             │
│  │             │  │             │  │             │             │
│  │ • Tunnel    │  │ • Apps      │  │ • Rules     │             │
│  │ • Cluster   │  │ • Groups    │  │ • Lists     │             │
│  │   Tunnel    │  │ • IDPs      │  │ • Config    │             │
│  │ • Binding   │  │ • Tokens    │  │             │             │
│  └─────────────┘  └─────────────┘  └─────────────┘             │
│                                                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│  │   Network   │  │   Device    │  │    DNS      │             │
│  │             │  │             │  │             │             │
│  │ • VNet      │  │ • Policy    │  │ • Records   │             │
│  │ • Routes    │  │ • Posture   │  │ • WARP      │             │
│  │ • Private   │  │             │  │   Connector │             │
│  │   Service   │  │             │  │             │             │
│  └─────────────┘  └─────────────┘  └─────────────┘             │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## CRD Summary

### Tunnel Management

| CRD | Scope | Description |
|-----|-------|-------------|
| `Tunnel` | Namespaced | Cloudflare Tunnel with managed cloudflared |
| `ClusterTunnel` | Cluster | Cluster-wide Cloudflare Tunnel |
| `TunnelBinding` | Namespaced | Bind Services to Tunnels with DNS |

### Private Network

| CRD | Scope | Description |
|-----|-------|-------------|
| `VirtualNetwork` | Cluster | Traffic isolation network |
| `NetworkRoute` | Cluster | Route CIDR through tunnel |
| `PrivateService` | Namespaced | Expose Service via private IP |

### Access Control

| CRD | Scope | Description |
|-----|-------|-------------|
| `AccessApplication` | Namespaced | Zero Trust application |
| `AccessGroup` | Cluster | Reusable access policy group |
| `AccessIdentityProvider` | Cluster | Identity provider configuration |
| `AccessServiceToken` | Namespaced | M2M authentication token |

### Gateway & Security

| CRD | Scope | Description |
|-----|-------|-------------|
| `GatewayRule` | Cluster | DNS/HTTP/L4 policy rule |
| `GatewayList` | Cluster | List for gateway rules |
| `GatewayConfiguration` | Cluster | Global gateway settings |

### Device Management

| CRD | Scope | Description |
|-----|-------|-------------|
| `DeviceSettingsPolicy` | Cluster | WARP client configuration |
| `DevicePostureRule` | Cluster | Device health check rule |

### DNS & Connectivity

| CRD | Scope | Description |
|-----|-------|-------------|
| `DNSRecord` | Namespaced | DNS record management |
| `WARPConnector` | Cluster | WARP connector deployment |

## Getting Help

- **Examples**: See [/examples](../../examples/) for practical usage
- **Issues**: [GitHub Issues](https://github.com/StringKe/cloudflare-operator/issues)
- **Discussions**: [GitHub Discussions](https://github.com/StringKe/cloudflare-operator/discussions)

## Version Information

- Current Version: v0.17.x (Alpha)
- API Version: `networking.cloudflare-operator.io/v1alpha2`
- Kubernetes: v1.28+
- Go: 1.24+
