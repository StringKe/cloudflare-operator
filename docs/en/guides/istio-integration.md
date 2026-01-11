# Istio Service Mesh Integration

This guide covers how to integrate Cloudflare Operator with Istio Service Mesh, including mTLS configuration, common issues, and best practices.

## Overview

When running Cloudflare Tunnel (cloudflared) in an Istio-enabled Kubernetes cluster, you may encounter TLS-related issues due to Istio's automatic mTLS (mutual TLS) feature. This guide explains the architecture and provides solutions for seamless integration.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           Kubernetes Cluster                            │
│                                                                         │
│  ┌─────────────┐     ┌──────────────────┐     ┌───────────────────┐   │
│  │ cloudflared │────▶│  Envoy Sidecar   │────▶│  Backend Service  │   │
│  │    Pod      │     │  (istio-proxy)   │     │     Sidecar       │   │
│  └─────────────┘     └──────────────────┘     └───────────────────┘   │
│         │                    │                         │               │
│         │                    │ mTLS (auto)             │               │
│         ▼                    ▼                         ▼               │
│    HTTP Request         TLS Upgrade              TLS Termination       │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### How Istio Auto mTLS Works

1. **Source Pod has sidecar**: Outbound traffic is intercepted by Envoy
2. **Target Pod detection**: Istio checks if target Pod has a sidecar
3. **Protocol decision**:
   - Target has sidecar → Use mTLS
   - Target has no sidecar → Use plaintext

## Common Issue: excludeInboundPorts Conflict

### Problem Description

When a port is configured in `excludeInboundPorts` (e.g., for Prometheus scraping), but the target Pod still has a sidecar:

```yaml
# Target Pod annotation
annotations:
  traffic.istio.io/excludeInboundPorts: "9091"
```

**What happens:**
1. Istio Gateway sees target Pod has sidecar → sends mTLS
2. Port 9091 bypasses sidecar → expects plaintext
3. Result: `TLS handshake failure` or `WRONG_VERSION_NUMBER`

### Traffic Flow Diagram

```
┌──────────────────────────────────────────────────────────────────────────┐
│                                                                          │
│  cloudflared ──▶ sidecar ──▶ mTLS ──▶ target:9091 ──▶ FAIL!             │
│       │              │                     │                             │
│       │              │                     └─ excludeInboundPorts        │
│       │              │                        (expects plaintext)        │
│       │              │                                                   │
│       │              └─ Sees target has sidecar                         │
│       │                 → Upgrades to mTLS                               │
│       │                                                                  │
│       └─ Sends HTTP request                                              │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

## Solutions

### Solution 1: DestinationRule (Recommended)

Create a DestinationRule to disable mTLS for specific ports:

```yaml
apiVersion: networking.istio.io/v1
kind: DestinationRule
metadata:
  name: backend-no-mtls-9091
  namespace: app
spec:
  host: my-backend.app.svc.cluster.local
  trafficPolicy:
    portLevelSettings:
    - port:
        number: 9091
      tls:
        mode: DISABLE  # Disable mTLS for this port
```

This tells Istio: "When connecting to port 9091, do NOT use mTLS."

### Solution 2: PeerAuthentication Exception

Add a port-level exception in PeerAuthentication:

```yaml
apiVersion: security.istio.io/v1
kind: PeerAuthentication
metadata:
  name: backend-9091-permissive
  namespace: app
spec:
  selector:
    matchLabels:
      app: my-backend
  mtls:
    mode: STRICT  # Default for other ports
  portLevelMtls:
    9091:
      mode: DISABLE  # Allow plaintext on 9091
```

### Solution 3: Remove Port from excludeInboundPorts

If mTLS is acceptable for the port, remove it from `excludeInboundPorts`:

```yaml
# Before: Port excluded
annotations:
  traffic.istio.io/excludeInboundPorts: "9091,15090"

# After: Only exclude metrics port
annotations:
  traffic.istio.io/excludeInboundPorts: "15090"
```

Now Istio will handle mTLS on port 9091 end-to-end.

### Solution 4: Inject Sidecar into cloudflared (Alternative)

If cloudflared doesn't have a sidecar, it sends plaintext. You can:

```yaml
# cloudflared Deployment
spec:
  template:
    metadata:
      annotations:
        sidecar.istio.io/inject: "true"
        # Ensure sidecar starts before cloudflared
        proxy.istio.io/config: '{ "holdApplicationUntilProxyStarts": true }'
```

With sidecar injection:
- cloudflared sends HTTP to local sidecar
- Sidecar automatically upgrades to mTLS
- Target sidecar terminates mTLS

**Note:** This doesn't help if the target port uses `excludeInboundPorts`.

## Complete Example

### Scenario: Spring Boot Admin on Port 9091

Spring Boot applications often expose management endpoints on a separate port (9091) which may be excluded from Istio for Prometheus scraping.

#### Step 1: Create DestinationRule

```yaml
apiVersion: networking.istio.io/v1
kind: DestinationRule
metadata:
  name: gateway-actuator-no-mtls
  namespace: app
spec:
  host: sg-cloud-gateway.app.svc.cluster.local
  trafficPolicy:
    portLevelSettings:
    - port:
        number: 9091
      tls:
        mode: DISABLE
```

#### Step 2: Create Ingress with Protocol Annotation

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: spring-boot-admin
  namespace: app
  annotations:
    # Explicitly specify HTTP protocol for port 9091
    cloudflare.com/protocol-9091: http
spec:
  ingressClassName: cloudflare-tunnel
  rules:
  - host: admin.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: sg-cloud-gateway
            port:
              number: 9091
```

#### Step 3: Configure TunnelIngressClassConfig (Optional)

Set default protocol at the IngressClass level:

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: TunnelIngressClassConfig
metadata:
  name: cloudflare-tunnel-config
  namespace: cloudflare-operator-system
spec:
  tunnelRef:
    kind: ClusterTunnel
    name: main-tunnel
  defaultProtocol: http  # Default to HTTP for all backends
  defaultOriginRequest:
    connectTimeout: "30s"
    keepAliveTimeout: "90s"
```

## Protocol Detection Priority

The Cloudflare Operator uses a 7-level priority system for protocol detection:

| Priority | Source | Example |
|----------|--------|---------|
| 1 | Ingress annotation: `cloudflare.com/protocol` | `https` |
| 2 | Ingress annotation: `cloudflare.com/protocol-{port}` | `cloudflare.com/protocol-9091: http` |
| 3 | Service annotation: `cloudflare.com/protocol` | `http` |
| 4 | Service port `appProtocol` field | `kubernetes.io/h2c` |
| 5 | Service port name | `http`, `https`, `grpc` |
| 6 | TunnelIngressClassConfig `defaultProtocol` | `http` |
| 7 | Port number inference | `443` → `https`, others → `http` |

## Troubleshooting

### Check if cloudflared has Sidecar

```bash
kubectl get pod -l app=<tunnel-name> -o jsonpath='{.items[0].spec.containers[*].name}'
# If output includes "istio-proxy", sidecar is present
```

### Check Generated Tunnel Configuration

```bash
kubectl get cm <tunnel-name> -n <namespace> -o yaml | grep -A30 "ingress:"
# Verify service URLs use correct protocol (http:// vs https://)
```

### Check Istio mTLS Status

```bash
# Check PeerAuthentication
kubectl get peerauthentication -A

# Check DestinationRule
kubectl get destinationrule -A

# Check if mTLS is being used (from source pod)
istioctl x describe pod <source-pod>
```

### Common Error Messages

| Error | Cause | Solution |
|-------|-------|----------|
| `WRONG_VERSION_NUMBER` | mTLS to plaintext port | Add DestinationRule with `tls.mode: DISABLE` |
| `upstream_reset_before_response_started` | Connection terminated | Check excludeInboundPorts and DestinationRule |
| `TLS handshake failure` | Protocol mismatch | Use `cloudflare.com/protocol-{port}` annotation |
| `503 UC` | Upstream connection failure | Verify service exists and port is correct |

### Debug with istioctl

```bash
# Analyze configuration
istioctl analyze -n <namespace>

# Check proxy configuration
istioctl proxy-config cluster <pod-name> -n <namespace>

# Check listener configuration
istioctl proxy-config listener <pod-name> -n <namespace>
```

## Best Practices

### 1. Consistent Port Configuration

Ensure `excludeInboundPorts` and DestinationRule are aligned:

```yaml
# If you exclude a port from inbound...
traffic.istio.io/excludeInboundPorts: "9091"

# ...also disable mTLS for outbound to that port
trafficPolicy:
  portLevelSettings:
  - port:
      number: 9091
    tls:
      mode: DISABLE
```

### 2. Use Explicit Protocol Annotations

Don't rely on inference; specify protocols explicitly:

```yaml
annotations:
  cloudflare.com/protocol-8080: http    # Main service
  cloudflare.com/protocol-9091: http    # Actuator
  cloudflare.com/protocol-443: https    # External HTTPS
```

### 3. Separate Concerns

- **Istio**: Manages service mesh mTLS
- **Cloudflare Operator**: Manages tunnel ingress rules
- **Keep configurations independent but aligned**

### 4. Monitor and Alert

Set up monitoring for TLS-related errors:

```yaml
# Prometheus alert example
- alert: CloudflareTunnelTLSErrors
  expr: rate(cloudflared_tunnel_request_errors_total{error=~".*tls.*"}[5m]) > 0
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: TLS errors detected in Cloudflare Tunnel
```

## Reference

- [Istio mTLS Migration Guide](https://istio.io/latest/docs/tasks/security/authentication/mtls-migration/)
- [Istio DestinationRule Reference](https://istio.io/latest/docs/reference/config/networking/destination-rule/)
- [Istio PeerAuthentication Reference](https://istio.io/latest/docs/reference/config/security/peer_authentication/)
- [Cloudflare Tunnel Kubernetes Guide](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/deploy-tunnels/deployment-guides/kubernetes/)
