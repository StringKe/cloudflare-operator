# Cloudflare Operator Documentation

Welcome to the Cloudflare Zero Trust Operator documentation. This operator enables Kubernetes-native management of Cloudflare Zero Trust resources.

## Quick Navigation

| Topic | Description |
|-------|-------------|
| [Getting Started](getting-started.md) | Installation and first tunnel |
| [Configuration](configuration.md) | API tokens and credentials |
| [Namespace Restrictions](namespace-restrictions.md) | CRD scope and Secret management |
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
- **Kubernetes Integration** - Native Ingress and Gateway API support

## Architecture

```mermaid
flowchart TB
    subgraph Internet["Internet"]
        Users["Users / WARP Clients"]
    end

    subgraph Cloudflare["Cloudflare Edge"]
        Edge["Cloudflare Edge Network"]
        API["Cloudflare API"]
    end

    subgraph K8s["Kubernetes Cluster"]
        subgraph CRDs["Custom Resources"]
            Tunnel["Tunnel / ClusterTunnel"]
            TB["TunnelBinding"]
            VNet["VirtualNetwork"]
            Route["NetworkRoute"]
            Access["Access*"]
            Gateway["Gateway*"]
        end

        subgraph K8sNative["Kubernetes Native"]
            Ingress["Ingress"]
            GatewayAPI["Gateway API"]
        end

        subgraph Operator["Cloudflare Operator"]
            Controller["Controller Manager"]
        end

        subgraph Managed["Managed Resources"]
            ConfigMap["ConfigMap"]
            Secret["Secret"]
            Deployment["cloudflared"]
        end

        subgraph App["Applications"]
            Service["Services"]
            Pod["Pods"]
        end
    end

    CRDs -.->|watches| Controller
    K8sNative -.->|watches| Controller
    Controller -->|creates| Managed
    Controller -->|API calls| API
    Managed -->|proxy| Service
    Service --> Pod
    Users -->|HTTPS/WARP| Edge
    Edge <-->|tunnel| Deployment
```

## CRD Summary

### Core Credentials

| CRD | Scope | Description |
|-----|-------|-------------|
| `CloudflareCredentials` | Cluster | Shared API credential configuration |

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
| `WARPConnector` | Cluster | WARP connector for site-to-site |

### Access Control

| CRD | Scope | Description |
|-----|-------|-------------|
| `AccessApplication` | Namespaced | Zero Trust application |
| `AccessGroup` | Cluster | Reusable access policy group |
| `AccessIdentityProvider` | Cluster | Identity provider configuration |
| `AccessServiceToken` | Namespaced | M2M authentication token |
| `AccessTunnel` | Namespaced | Access-protected tunnel endpoint |

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

### Kubernetes Integration

| CRD | Scope | Description |
|-----|-------|-------------|
| `TunnelIngressClassConfig` | Namespaced | Configuration for Ingress integration |
| `TunnelGatewayClassConfig` | Cluster | Configuration for Gateway API integration |

> **Note**: The operator also supports native Kubernetes `Ingress` and Gateway API (`Gateway`, `HTTPRoute`, `TCPRoute`, `UDPRoute`) resources when configured with the appropriate IngressClass or GatewayClass.

## Namespace and Secret Rules

The operator uses different Secret lookup rules based on CRD scope:

| Resource Scope | Secret Location |
|----------------|-----------------|
| Namespaced | Same namespace as the resource |
| Cluster | Operator namespace (`cloudflare-operator-system`) |

See [Namespace Restrictions](namespace-restrictions.md) for detailed information.

## Getting Help

- **Examples**: See [/examples](../../examples/) for practical usage
- **Issues**: [GitHub Issues](https://github.com/StringKe/cloudflare-operator/issues)
- **Discussions**: [GitHub Discussions](https://github.com/StringKe/cloudflare-operator/discussions)

## Version Information

- Current Version: v0.18.x (Alpha)
- API Version: `networking.cloudflare-operator.io/v1alpha2`
- Kubernetes: v1.28+
- Go: 1.24+
