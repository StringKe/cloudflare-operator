# ClusterTunnel

ClusterTunnel is a cluster-scoped resource that creates and manages Cloudflare Tunnels accessible from any namespace. It shares the same functionality as Tunnel but operates at the cluster level, enabling tunnel sharing across multiple namespaces.

## Overview

ClusterTunnel extends the capabilities of Tunnel to the cluster scope, allowing multiple namespaces to use the same Cloudflare Tunnel. This is ideal for shared infrastructure where a single tunnel serves applications across different namespaces.

### Key Features

| Feature | Description |
|---------|-------------|
| **Cluster-Wide Access** | Accessible from Ingress/Gateway resources in any namespace |
| **Resource Sharing** | Single tunnel serves multiple namespaces |
| **Centralized Management** | Manage tunnel configuration in one place |
| **High Availability** | Same HA capabilities as namespaced Tunnel |
| **WARP Routing** | Support for private network access via WARP clients |

### Use Cases

- **Multi-Tenant Clusters**: Share one tunnel across multiple tenant namespaces
- **Centralized Infrastructure**: Manage all external access through a single cluster tunnel
- **Cost Optimization**: Reduce the number of tunnels needed in large clusters
- **Shared Private Networks**: Enable WARP routing for the entire cluster

## ClusterTunnel vs Tunnel

| Aspect | Tunnel | ClusterTunnel |
|--------|--------|---------------|
| **Scope** | Namespaced | Cluster-wide |
| **Secret Location** | Same namespace as Tunnel | `cloudflare-operator-system` namespace |
| **Ingress Binding** | Same namespace only | Any namespace in the cluster |
| **Gateway Binding** | Same namespace only | Any namespace in the cluster |
| **Use Case** | Namespace isolation | Shared infrastructure |

## Spec

ClusterTunnel uses the same spec as [Tunnel](tunnel.md#spec). See Tunnel documentation for detailed field descriptions.

### Main Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `newTunnel` | *NewTunnel | No | - | Create a new tunnel (mutually exclusive with `existingTunnel`) |
| `existingTunnel` | *ExistingTunnel | No | - | Use an existing tunnel (mutually exclusive with `newTunnel`) |
| `cloudflare` | CloudflareDetails | **Yes** | - | Cloudflare API credentials and configuration |
| `enableWarpRouting` | bool | No | `false` | Enable WARP routing for private network access |
| `protocol` | string | No | `"auto"` | Tunnel protocol: `"auto"`, `"quic"`, or `"http2"` |
| `fallbackTarget` | string | No | `"http_status:404"` | Default response when no ingress rule matches |
| `deployPatch` | string | No | `"{}"` | JSON patch for customizing cloudflared Deployment |

### Important: Secret Location

For ClusterTunnel, the Cloudflare API credentials secret **must** be in the `cloudflare-operator-system` namespace (or the namespace where the operator is installed).

## Status

Same as [Tunnel Status](tunnel.md#status).

| Field | Type | Description |
|-------|------|-------------|
| `tunnelId` | string | Cloudflare tunnel UUID |
| `tunnelName` | string | Cloudflare tunnel name |
| `accountId` | string | Cloudflare account ID |
| `state` | string | Current state: `pending`, `creating`, `active`, `error`, `deleting` |
| `configVersion` | int | Current tunnel configuration version |
| `conditions` | []Condition | Standard Kubernetes conditions |

## Examples

### Basic Cluster Tunnel

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: ClusterTunnel
metadata:
  name: shared-cluster-tunnel
spec:
  newTunnel:
    name: shared-k8s-tunnel

  cloudflare:
    accountId: "<your-account-id>"
    domain: example.com
    # Secret MUST be in cloudflare-operator-system namespace
    secret: cloudflare-api-credentials
```

### High Availability Cluster Tunnel

Multiple replicas with pod anti-affinity:

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: ClusterTunnel
metadata:
  name: ha-cluster-tunnel
spec:
  newTunnel:
    name: ha-k8s-tunnel

  # 3 replicas with pod anti-affinity
  deployPatch: |
    {
      "spec": {
        "replicas": 3,
        "template": {
          "spec": {
            "affinity": {
              "podAntiAffinity": {
                "preferredDuringSchedulingIgnoredDuringExecution": [
                  {
                    "weight": 100,
                    "podAffinityTerm": {
                      "labelSelector": {
                        "matchLabels": {
                          "app.kubernetes.io/name": "cloudflared"
                        }
                      },
                      "topologyKey": "kubernetes.io/hostname"
                    }
                  }
                ]
              }
            }
          }
        }
      }
    }

  cloudflare:
    accountId: "<your-account-id>"
    domain: example.com
    secret: cloudflare-api-credentials
```

### Cluster Tunnel with WARP Routing

For cluster-wide private network access:

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: ClusterTunnel
metadata:
  name: private-network-tunnel
spec:
  newTunnel:
    name: cluster-private-tunnel

  # Enable WARP routing for all namespaces
  enableWarpRouting: true

  cloudflare:
    accountId: "<your-account-id>"
    domain: example.com
    secret: cloudflare-api-credentials
```

### Using ClusterTunnel from Ingress (Different Namespace)

```yaml
# ClusterTunnel in cluster scope
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: ClusterTunnel
metadata:
  name: shared-tunnel
spec:
  newTunnel:
    name: shared-k8s-tunnel
  cloudflare:
    accountId: "<your-account-id>"
    domain: example.com
    secret: cloudflare-api-credentials
---
# Ingress in app-namespace-1
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app1-ingress
  namespace: app-namespace-1
  annotations:
    # Reference the ClusterTunnel
    cf-operator.io/tunnel: "shared-tunnel"
spec:
  ingressClassName: cloudflare-tunnel
  rules:
    - host: app1.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: app1-service
                port:
                  number: 80
---
# Ingress in app-namespace-2
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app2-ingress
  namespace: app-namespace-2
  annotations:
    # Reference the same ClusterTunnel
    cf-operator.io/tunnel: "shared-tunnel"
spec:
  ingressClassName: cloudflare-tunnel
  rules:
    - host: app2.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: app2-service
                port:
                  number: 80
```

### Using ClusterTunnel with Gateway API

```yaml
# ClusterTunnel
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: ClusterTunnel
metadata:
  name: gateway-tunnel
spec:
  newTunnel:
    name: gateway-k8s-tunnel
  cloudflare:
    accountId: "<your-account-id>"
    domain: example.com
    secret: cloudflare-api-credentials
---
# HTTPRoute in any namespace
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: api-route
  namespace: api-namespace
  annotations:
    cf-operator.io/tunnel: "gateway-tunnel"
spec:
  parentRefs:
    - name: cloudflare-gateway
      kind: Gateway
  hostnames:
    - "api.example.com"
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: api-service
          port: 8080
```

## Prerequisites

1. **Cluster Admin Access**: ClusterTunnel is cluster-scoped and requires admin permissions
2. **Cloudflare Account**: Active Cloudflare account with Zero Trust enabled
3. **API Credentials in Operator Namespace**: Secret must exist in `cloudflare-operator-system`

### Creating Credentials Secret in Operator Namespace

```bash
kubectl create secret generic cloudflare-api-credentials \
  --from-literal=CLOUDFLARE_API_TOKEN="your-token-here" \
  -n cloudflare-operator-system
```

Or via YAML:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cloudflare-api-credentials
  namespace: cloudflare-operator-system
type: Opaque
stringData:
  CLOUDFLARE_API_TOKEN: "your-api-token-here"
```

## Limitations

- **Secret Location**: Credentials secret must be in the operator's namespace (`cloudflare-operator-system`)
- **Requires Cluster Admin**: Creating ClusterTunnel requires cluster-admin permissions
- **Single Point of Configuration**: Changes to ClusterTunnel affect all using namespaces
- **Name Uniqueness**: ClusterTunnel names must be unique across the cluster

## Security Considerations

- **Namespace Isolation**: While ClusterTunnel is accessible from all namespaces, consider RBAC to control which namespaces can reference it
- **Centralized Credentials**: API credentials in the operator namespace are more sensitive - ensure proper access controls
- **Configuration Changes**: Updates to ClusterTunnel configuration affect all dependent Ingress/Gateway resources

## Related Resources

- [Tunnel](tunnel.md) - Namespaced tunnel for single-namespace use
- [DNSRecord](dnsrecord.md) - Manage DNS records for tunnel endpoints
- [Ingress Integration](../guides/ingress-integration.md) - Use Ingress with ClusterTunnel
- [Gateway API Integration](../guides/gateway-api-integration.md) - Use Gateway API with ClusterTunnel
- [NetworkRoute](networkroute.md) - Route private IP ranges through tunnel

## See Also

- [Examples](../../../examples/01-basic/tunnel/)
- [Cloudflare Tunnel Documentation](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/)
