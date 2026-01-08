<h1 align=center>Cloudflare Zero Trust Operator</h1>

<div align="center">
  <a href="https://github.com/StringKe/cloudflare-operator">
    <img src="docs/images/CloudflareOperatorLogo.png" alt="Logo" height="250">
  </a>
  <br />

  <p align="center">
    A Kubernetes Operator for Cloudflare Zero Trust: Tunnels, Access, Gateway, and Device Management
    <br />
    <br />
    <a href="https://github.com/StringKe/cloudflare-operator/blob/main/docs/en/README.md"><strong>Documentation (English) »</strong></a>
    |
    <a href="https://github.com/StringKe/cloudflare-operator/blob/main/docs/zh/README.md"><strong>文档 (中文) »</strong></a>
    <br />
    <br />
    <a href="https://github.com/StringKe/cloudflare-operator/tree/main/examples">Examples</a>
    ·
    <a href="https://github.com/StringKe/cloudflare-operator/issues">Report Bug</a>
    ·
    <a href="https://github.com/StringKe/cloudflare-operator/issues">Request Feature</a>
  </p>
</div>

<div align="center">

[![GitHub license](https://img.shields.io/github/license/StringKe/cloudflare-operator?color=brightgreen)](https://github.com/StringKe/cloudflare-operator/blob/main/LICENSE)
[![GitHub release](https://img.shields.io/github/v/release/StringKe/cloudflare-operator)](https://github.com/StringKe/cloudflare-operator/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/StringKe/cloudflare-operator)](https://goreportcard.com/report/github.com/StringKe/cloudflare-operator)
[![CI](https://github.com/StringKe/cloudflare-operator/actions/workflows/release.yml/badge.svg)](https://github.com/StringKe/cloudflare-operator/actions/workflows/release.yml)
[![Test](https://github.com/StringKe/cloudflare-operator/actions/workflows/test.yml/badge.svg)](https://github.com/StringKe/cloudflare-operator/actions/workflows/test.yml)
[![Lint](https://github.com/StringKe/cloudflare-operator/actions/workflows/lint.yml/badge.svg)](https://github.com/StringKe/cloudflare-operator/actions/workflows/lint.yml)
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2FStringKe%2Fcloudflare-operator.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2FStringKe%2Fcloudflare-operator)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/StringKe/cloudflare-operator/badge)](https://securityscorecards.dev/viewer/?uri=github.com/StringKe/cloudflare-operator)

</div>

> **Note**: This project is currently in Alpha (v0.17.x)
>
> This project is a fork of [adyanth/cloudflare-operator](https://github.com/adyanth/cloudflare-operator) with extended Zero Trust features and improvements.

## Overview

The Cloudflare Zero Trust Operator provides Kubernetes-native management of Cloudflare Zero Trust resources. Built with `kubebuilder` and `controller-runtime`, it enables declarative configuration of tunnels, access policies, gateway rules, and device settings through Custom Resource Definitions (CRDs).

## Features

| Category | Features |
|----------|----------|
| **Tunnel Management** | Create/manage Cloudflare Tunnels, automatic cloudflared deployments, Service binding with DNS |
| **Private Network** | Virtual Networks, Network Routes, Private Service exposure via WARP |
| **Access Control** | Zero Trust Applications, Access Groups, Identity Providers, Service Tokens |
| **Gateway & Security** | Gateway Rules (DNS/HTTP/L4), Gateway Lists, Browser Isolation |
| **Device Management** | Split Tunnel configuration, Fallback Domains, Device Posture Rules |
| **DNS & Connectivity** | DNS Record management, WARP Connectors for site-to-site |

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
    Controller -->|creates| Managed
    Controller -->|API calls| API
    Managed -->|proxy| Service
    Service --> Pod
    Users -->|HTTPS/WARP| Edge
    Edge <-->|tunnel| Deployment
```

## Quick Start

### Prerequisites

- Kubernetes cluster v1.28+
- Cloudflare account with Zero Trust enabled
- Cloudflare API Token ([Create Token](https://dash.cloudflare.com/profile/api-tokens))

### Installation

```bash
# Install CRDs and operator
kubectl apply -f https://github.com/StringKe/cloudflare-operator/releases/latest/download/cloudflare-operator.crds.yaml
kubectl apply -f https://github.com/StringKe/cloudflare-operator/releases/latest/download/cloudflare-operator.yaml

# Verify installation
kubectl get pods -n cloudflare-operator-system
```

### Create a Tunnel

```yaml
# 1. Create API credentials secret
apiVersion: v1
kind: Secret
metadata:
  name: cloudflare-credentials
type: Opaque
stringData:
  CLOUDFLARE_API_TOKEN: "<your-api-token>"
---
# 2. Create tunnel
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

### Expose a Service

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

## CRD Reference

### Tunnel Management

| CRD | API Version | Scope | Description |
|-----|-------------|-------|-------------|
| Tunnel | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | Cloudflare Tunnel with managed cloudflared |
| ClusterTunnel | `networking.cloudflare-operator.io/v1alpha2` | Cluster | Cluster-wide Cloudflare Tunnel |
| TunnelBinding | `networking.cfargotunnel.com/v1alpha1` | Namespaced | Bind Services to Tunnels with DNS |

### Private Network Access

| CRD | API Version | Scope | Description |
|-----|-------------|-------|-------------|
| VirtualNetwork | `networking.cloudflare-operator.io/v1alpha2` | Cluster | Cloudflare virtual network for isolation |
| NetworkRoute | `networking.cloudflare-operator.io/v1alpha2` | Cluster | Route CIDR through tunnel |
| PrivateService | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | Expose Service via private IP |

### Access Control

| CRD | API Version | Scope | Description |
|-----|-------------|-------|-------------|
| AccessApplication | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | Zero Trust application |
| AccessGroup | `networking.cloudflare-operator.io/v1alpha2` | **Cluster** | Access policy group |
| AccessIdentityProvider | `networking.cloudflare-operator.io/v1alpha2` | **Cluster** | Identity provider config |
| AccessServiceToken | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | Service token for M2M |

### Gateway & Security

| CRD | API Version | Scope | Description |
|-----|-------------|-------|-------------|
| GatewayRule | `networking.cloudflare-operator.io/v1alpha2` | **Cluster** | Gateway policy rule |
| GatewayList | `networking.cloudflare-operator.io/v1alpha2` | **Cluster** | List for gateway rules |
| GatewayConfiguration | `networking.cloudflare-operator.io/v1alpha2` | Cluster | Global gateway settings |

### Device Management

| CRD | API Version | Scope | Description |
|-----|-------------|-------|-------------|
| DeviceSettingsPolicy | `networking.cloudflare-operator.io/v1alpha2` | Cluster | WARP client settings |
| DevicePostureRule | `networking.cloudflare-operator.io/v1alpha2` | **Cluster** | Device posture check |

### DNS & Connectivity

| CRD | API Version | Scope | Description |
|-----|-------------|-------|-------------|
| DNSRecord | `networking.cloudflare-operator.io/v1alpha2` | Namespaced | DNS record management |
| WARPConnector | `networking.cloudflare-operator.io/v1alpha2` | **Cluster** | WARP connector deployment |

## Examples

See the [examples](examples/) directory for comprehensive usage examples:

- **[Basic](examples/01-basic/)** - Credentials, Tunnels, DNS, Service Binding
- **[Private Network](examples/02-private-network/)** - Virtual Networks, Routes, Private Services
- **[Zero Trust](examples/03-zero-trust/)** - Access Apps, Groups, Identity Providers
- **[Gateway](examples/04-gateway/)** - Gateway Rules, Lists
- **[Device](examples/05-device/)** - Device Policies, Posture Rules
- **[Scenarios](examples/scenarios/)** - Complete real-world scenarios

## Documentation

| Language | Link |
|----------|------|
| English | [docs/en/README.md](docs/en/README.md) |
| 中文 | [docs/zh/README.md](docs/zh/README.md) |

Documentation includes:
- Installation Guide
- API Token Permissions
- Complete CRD Reference
- Troubleshooting Guide
- Migration Guide (v1alpha1 → v1alpha2)

## API Token Permissions

| Feature | Permission | Scope |
|---------|------------|-------|
| Tunnels | `Account:Cloudflare Tunnel:Edit` | Account |
| DNS | `Zone:DNS:Edit` | Zone |
| Access | `Account:Access: Apps and Policies:Edit` | Account |
| Gateway | `Account:Zero Trust:Edit` | Account |

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Acknowledgements

This project is forked from [adyanth/cloudflare-operator](https://github.com/adyanth/cloudflare-operator). We extend our gratitude to [@adyanth](https://github.com/adyanth) and all original contributors for their excellent work on the initial implementation.

### What's Different

This fork extends the original project with:
- Complete Zero Trust resource support (Access, Gateway, Device management)
- v1alpha2 API with improved resource management
- Enhanced error handling and status reporting
- Comprehensive documentation and examples

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details.

[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2FStringKe%2Fcloudflare-operator.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2FStringKe%2Fcloudflare-operator)

---

> **Disclaimer**: This is **NOT** an official Cloudflare product. It uses the [Cloudflare API](https://api.cloudflare.com/) and [cloudflared](https://github.com/cloudflare/cloudflared) to automate Zero Trust configuration on Kubernetes.
