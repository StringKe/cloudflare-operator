# TunnelBinding Migration Guide

This guide explains how to migrate from the deprecated `TunnelBinding` resource to the recommended alternatives: Kubernetes Ingress with `TunnelIngressClassConfig` or Gateway API.

## Why Migrate?

`TunnelBinding` is deprecated for the following reasons:

1. **Non-standard API**: TunnelBinding uses a custom API that doesn't integrate with Kubernetes ecosystem tools
2. **Limited features**: Missing support for advanced routing, TLS configuration, and middleware
3. **Unified architecture**: The operator is moving to a unified sync architecture where Ingress and Gateway API provide the standard interface
4. **Better observability**: Ingress and Gateway API resources are better supported by monitoring tools

## Migration Options

| Feature | TunnelBinding | Ingress | Gateway API |
|---------|--------------|---------|-------------|
| HTTP/HTTPS routing | Yes | Yes | Yes |
| TCP/UDP services | Limited | No | Yes (TCPRoute/UDPRoute) |
| Path-based routing | Basic regex | Yes | Yes |
| Header-based routing | No | Limited | Yes |
| Multi-cluster | No | No | Yes |
| DNS management | Auto | Configurable | Configurable |
| Access integration | Manual | Annotation | Annotation |

### Recommended Migration Path

- **HTTP/HTTPS services**: Use **Ingress** with `TunnelIngressClassConfig`
- **TCP/UDP services**: Use **Gateway API** with `TunnelGatewayClassConfig`
- **Complex routing**: Use **Gateway API** with HTTPRoute/TCPRoute/UDPRoute

## Prerequisites

1. Operator version 0.20.0 or later
2. `TunnelIngressClassConfig` CRD installed
3. For Gateway API: Gateway API CRDs installed

## Automated Migration Tool

We provide a migration script to help convert TunnelBinding resources:

```bash
# Download the migration script
curl -O https://raw.githubusercontent.com/StringKe/cloudflare-operator/main/scripts/migrate-tunnelbinding.sh
chmod +x migrate-tunnelbinding.sh

# Run migration (dry-run by default)
./migrate-tunnelbinding.sh <namespace> <output-directory>

# Example
./migrate-tunnelbinding.sh default ./migration-output
```

The script generates:
- `TunnelIngressClassConfig` for tunnel configuration
- `IngressClass` for Kubernetes integration
- `Ingress` resources for each TunnelBinding subject

## Manual Migration Steps

### Step 1: Identify TunnelBinding Resources

```bash
# List all TunnelBindings
kubectl get tunnelbinding -A

# Export a specific TunnelBinding
kubectl get tunnelbinding <name> -n <namespace> -o yaml
```

### Step 2: Create TunnelIngressClassConfig

Create a `TunnelIngressClassConfig` that references your tunnel:

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: TunnelIngressClassConfig
metadata:
  name: my-tunnel-ingress
spec:
  tunnelRef:
    kind: ClusterTunnel  # or Tunnel
    name: my-tunnel
  dnsManagement: Automatic  # Automatic, DNSRecord, or Manual
  dnsProxied: true
```

### Step 3: Create IngressClass

```yaml
apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata:
  name: my-tunnel-ingress
spec:
  controller: cloudflare-operator.io/tunnel-ingress-controller
  parameters:
    apiGroup: networking.cloudflare-operator.io
    kind: TunnelIngressClassConfig
    name: my-tunnel-ingress
```

### Step 4: Convert TunnelBinding Subjects to Ingress

**Before (TunnelBinding):**

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha1
kind: TunnelBinding
metadata:
  name: my-binding
  namespace: default
subjects:
  - kind: Service
    name: my-service
    spec:
      fqdn: app.example.com
      protocol: https
      noTlsVerify: true
      path: /api/.*
tunnelRef:
  kind: ClusterTunnel
  name: my-tunnel
```

**After (Ingress):**

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-service-ingress
  namespace: default
  annotations:
    cloudflare-operator.io/origin-request-protocol: "https"
    cloudflare-operator.io/origin-request-no-tls-verify: "true"
spec:
  ingressClassName: my-tunnel-ingress
  rules:
  - host: app.example.com
    http:
      paths:
      - path: /api
        pathType: Prefix
        backend:
          service:
            name: my-service
            port:
              number: 443
```

### Step 5: Verify and Clean Up

1. **Verify Ingress is working:**
   ```bash
   kubectl get ingress -n <namespace>
   kubectl describe ingress <name> -n <namespace>
   ```

2. **Check DNS records:**
   ```bash
   kubectl get dnsrecord -n <namespace>
   ```

3. **Delete TunnelBinding:**
   ```bash
   # WARNING: This removes DNS records managed by TunnelBinding
   kubectl delete tunnelbinding <name> -n <namespace>
   ```

## Origin Request Configuration Mapping

The following table maps TunnelBinding spec fields to Ingress annotations:

| TunnelBinding Field | Ingress Annotation |
|--------------------|-------------------|
| `spec.protocol` | `cloudflare-operator.io/origin-request-protocol` |
| `spec.noTlsVerify` | `cloudflare-operator.io/origin-request-no-tls-verify` |
| `spec.http2Origin` | `cloudflare-operator.io/origin-request-http2-origin` |
| `spec.caPool` | `cloudflare-operator.io/origin-request-ca-pool` |
| `spec.proxyAddress` | `cloudflare-operator.io/origin-request-proxy-address` |
| `spec.proxyPort` | `cloudflare-operator.io/origin-request-proxy-port` |
| `spec.proxyType` | `cloudflare-operator.io/origin-request-proxy-type` |

## Gateway API Migration

For TCP/UDP services or advanced routing, use Gateway API:

### Step 1: Create TunnelGatewayClassConfig

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: TunnelGatewayClassConfig
metadata:
  name: my-tunnel-gateway
spec:
  tunnelRef:
    kind: ClusterTunnel
    name: my-tunnel
  dnsManagement: Automatic
```

### Step 2: Create GatewayClass and Gateway

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: cloudflare-tunnel
spec:
  controllerName: cloudflare-operator.io/tunnel-gateway-controller
  parametersRef:
    group: networking.cloudflare-operator.io
    kind: TunnelGatewayClassConfig
    name: my-tunnel-gateway
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: my-gateway
  namespace: default
spec:
  gatewayClassName: cloudflare-tunnel
  listeners:
  - name: http
    protocol: HTTP
    port: 80
```

### Step 3: Create Routes

**HTTPRoute:**

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-http-route
  namespace: default
spec:
  parentRefs:
  - name: my-gateway
  hostnames:
  - "app.example.com"
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /api
    backendRefs:
    - name: my-service
      port: 80
```

**TCPRoute (for TCP services):**

```yaml
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: TCPRoute
metadata:
  name: my-tcp-route
  namespace: default
spec:
  parentRefs:
  - name: my-gateway
  rules:
  - backendRefs:
    - name: my-tcp-service
      port: 5432
```

## Troubleshooting

### DNS Records Not Created

1. Check `TunnelIngressClassConfig` DNS settings:
   ```bash
   kubectl describe tunnelingressclassconfig <name>
   ```

2. Verify Zone ID in tunnel/credentials:
   ```bash
   kubectl get secret <credentials> -n <namespace> -o yaml
   ```

### Service Not Accessible

1. Check tunnel configuration:
   ```bash
   kubectl get cloudflaresyncstate -A | grep tunnel
   ```

2. Verify Ingress status:
   ```bash
   kubectl describe ingress <name> -n <namespace>
   ```

### Origin Connection Errors

1. Verify origin request annotations are correct
2. Check if service is accessible within the cluster:
   ```bash
   kubectl run test --rm -it --image=curlimages/curl -- curl http://<service>.<namespace>.svc
   ```

## FAQ

**Q: Can I run TunnelBinding and Ingress side by side?**
A: Yes, but avoid configuring the same hostnames in both to prevent conflicts.

**Q: Will deleting TunnelBinding affect my DNS records?**
A: Yes, TunnelBinding creates DNS TXT records for ownership tracking. Deleting it will remove those records. Create Ingress resources first to ensure continuity.

**Q: How do I migrate without downtime?**
A:
1. Create Ingress resources for the same hostnames
2. Wait for DNS propagation and verify access
3. Delete TunnelBinding

**Q: What about Access Applications?**
A: Access Applications work with both TunnelBinding and Ingress. No changes needed for Access configuration.
