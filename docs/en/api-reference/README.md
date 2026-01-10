# API Reference

This section contains detailed documentation for all Custom Resource Definitions (CRDs).

## CRD Categories

### Tunnel Management
- [Tunnel](tunnel.md) - Namespace-scoped Cloudflare Tunnel
- [ClusterTunnel](clustertunnel.md) - Cluster-wide Cloudflare Tunnel
- [TunnelBinding](tunnelbinding.md) - Bind Services to Tunnels

### Private Network
- [VirtualNetwork](virtualnetwork.md) - Traffic isolation network
- [NetworkRoute](networkroute.md) - CIDR routing through tunnel
- [PrivateService](privateservice.md) - Private IP service exposure
- [WARPConnector](warpconnector.md) - Site-to-site WARP connector

### Access Control
- [AccessApplication](accessapplication.md) - Zero Trust application
- [AccessGroup](accessgroup.md) - Reusable access policy group
- [AccessIdentityProvider](accessidentityprovider.md) - Identity provider config
- [AccessServiceToken](accessservicetoken.md) - M2M authentication token
- [AccessTunnel](accesstunnel.md) - Access-protected tunnel endpoint

### Gateway & Security
- [GatewayRule](gatewayrule.md) - DNS/HTTP/L4 policy rule
- [GatewayList](gatewaylist.md) - List for gateway rules
- [GatewayConfiguration](gatewayconfiguration.md) - Global gateway settings

### Device Management
- [DeviceSettingsPolicy](devicesettingspolicy.md) - WARP client configuration
- [DevicePostureRule](deviceposturerule.md) - Device health check rule

### DNS & Connectivity
- [DNSRecord](dnsrecord.md) - DNS record management

### Kubernetes Integration
- [TunnelIngressClassConfig](tunnelingressclassconfig.md) - Ingress integration
- [TunnelGatewayClassConfig](tunnelgatewayclassconfig.md) - Gateway API integration

## Common Types

### CloudflareSpec

All CRDs that interact with Cloudflare API include a `cloudflare` spec:

```yaml
spec:
  cloudflare:
    accountId: "your-account-id"
    domain: example.com
    secret: cloudflare-credentials
```

### Status Conditions

All CRDs report status through standard Kubernetes conditions:

| Condition | Description |
|-----------|-------------|
| `Ready` | Resource is fully reconciled and operational |
| `Progressing` | Resource is being created or updated |
| `Degraded` | Resource has errors but may be partially functional |

## API Version

Current API version: `networking.cloudflare-operator.io/v1alpha2`

Legacy version `v1alpha1` is deprecated but still supported for backwards compatibility.
