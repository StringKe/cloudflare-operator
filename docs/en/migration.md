# Migration Guide

This guide covers migrating from v1alpha1 to v1alpha2 API version.

## Overview

The `v1alpha2` API version introduces several improvements:
- Enhanced status reporting with standard Kubernetes conditions
- Improved resource management and adoption
- New CRDs for Kubernetes integration (TunnelIngressClassConfig, TunnelGatewayClassConfig)
- Better error handling and validation

## Automatic Conversion

The operator includes a conversion webhook that automatically converts resources between v1alpha1 and v1alpha2. This means:

- **Existing v1alpha1 resources** continue to work without modification
- **New resources** should use v1alpha2
- **Storage version** is v1alpha2 (resources are stored in this format)

## API Changes

### Tunnel / ClusterTunnel

No breaking changes. The following fields are the same:
- `spec.newTunnel`
- `spec.existingTunnel`
- `spec.cloudflare`
- `spec.size`
- `spec.image`

### TunnelBinding

The `v1alpha1` TunnelBinding uses a different API group (`networking.cfargotunnel.com`) and is maintained for backwards compatibility.

**v1alpha1** (legacy):
```yaml
apiVersion: networking.cfargotunnel.com/v1alpha1
kind: TunnelBinding
```

**v1alpha2** (recommended):
```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: TunnelBinding
```

### Status Conditions

v1alpha2 uses standard Kubernetes condition types:

| Condition | Meaning |
|-----------|---------|
| `Ready` | Resource is fully operational |
| `Progressing` | Resource is being reconciled |
| `Degraded` | Resource has errors |

## Migration Steps

### Step 1: Update Operator

Ensure you're running the latest operator version:

```bash
# Update CRDs first
kubectl apply -f https://github.com/StringKe/cloudflare-operator/releases/latest/download/cloudflare-operator-crds.yaml

# Then update operator
kubectl apply -f https://github.com/StringKe/cloudflare-operator/releases/latest/download/cloudflare-operator-no-webhook.yaml
```

### Step 2: Verify Conversion Webhook

Check that the conversion webhook is running:

```bash
kubectl get pods -n cloudflare-operator-system
kubectl logs -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager
```

### Step 3: Test Existing Resources

Your existing v1alpha1 resources should continue to work:

```bash
kubectl get tunnels.networking.cloudflare-operator.io -A
kubectl get clustertunnels.networking.cloudflare-operator.io
```

### Step 4: Migrate Manifests (Optional)

Update your manifests to use v1alpha2 for new deployments:

```yaml
# Before (v1alpha1)
apiVersion: networking.cloudflare-operator.io/v1alpha1
kind: Tunnel

# After (v1alpha2)
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: Tunnel
```

### Step 5: Update TunnelBinding (Optional)

If using the legacy TunnelBinding API group, consider migrating:

```yaml
# Before (legacy)
apiVersion: networking.cfargotunnel.com/v1alpha1
kind: TunnelBinding

# After (recommended)
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: TunnelBinding
```

## Rollback

If you encounter issues:

1. The conversion webhook allows bidirectional conversion
2. You can continue using v1alpha1 resources
3. Check operator logs for conversion errors

## Troubleshooting

### Conversion Errors

If resources fail to convert:

```bash
# Check webhook logs
kubectl logs -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager | grep conversion

# Describe the resource
kubectl describe tunnel <name> -n <namespace>
```

### Version Mismatch

If you see version mismatch errors:

1. Ensure CRDs are updated: `kubectl apply -f cloudflare-operator-crds.yaml`
2. Restart the operator: `kubectl rollout restart deployment -n cloudflare-operator-system`

## FAQ

**Q: Do I need to recreate my resources?**
A: No, existing resources are automatically converted.

**Q: Can I use both v1alpha1 and v1alpha2?**
A: Yes, the conversion webhook handles this automatically.

**Q: When will v1alpha1 be removed?**
A: No timeline yet. We'll provide advance notice before deprecation.
