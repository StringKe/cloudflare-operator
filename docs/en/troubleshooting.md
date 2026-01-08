# Troubleshooting

This guide helps diagnose and resolve common issues with the Cloudflare Operator.

## Diagnostic Commands

### Check Operator Status

```bash
# Operator pod status
kubectl get pods -n cloudflare-operator-system

# Operator logs
kubectl logs -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager

# Enable debug logging
kubectl patch deployment cloudflare-operator-controller-manager \
  -n cloudflare-operator-system \
  --type='json' \
  -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--zap-log-level=debug"}]'
```

### Check Resource Status

```bash
# List all operator resources
kubectl get tunnels,clustertunnels,tunnelbindings,virtualnetworks,networkroutes -A

# Detailed status with conditions
kubectl get tunnel <name> -o jsonpath='{.status.conditions}' | jq

# Events for a resource
kubectl describe tunnel <name>
```

## Common Issues

### Tunnel Not Connecting

**Symptoms:**
- Tunnel status shows error
- cloudflared pods not running or CrashLooping

**Diagnostic Steps:**

```bash
# Check tunnel status
kubectl get tunnel <name> -o wide

# Check cloudflared deployment
kubectl get deployment -l app.kubernetes.io/name=cloudflared

# Check cloudflared logs
kubectl logs -l app.kubernetes.io/name=cloudflared
```

**Common Causes:**

1. **Invalid API Token**
   ```bash
   # Verify token works
   curl -X GET "https://api.cloudflare.com/client/v4/user/tokens/verify" \
     -H "Authorization: Bearer $(kubectl get secret cloudflare-credentials -o jsonpath='{.data.CLOUDFLARE_API_TOKEN}' | base64 -d)"
   ```

2. **Wrong Account ID**
   - Verify account ID in Cloudflare Dashboard
   - Check spelling in tunnel spec

3. **Network Connectivity**
   - Ensure pods can reach `api.cloudflare.com` and `*.cloudflareaccess.com`
   - Check network policies

4. **Secret Not Found**
   ```bash
   kubectl get secret <secret-name> -n <namespace>
   ```

### DNS Records Not Created

**Symptoms:**
- TunnelBinding shows success but DNS not resolving
- No CNAME record in Cloudflare Dashboard

**Diagnostic Steps:**

```bash
# Check TunnelBinding status
kubectl describe tunnelbinding <name>

# Check operator logs for DNS errors
kubectl logs -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager | grep -i dns
```

**Common Causes:**

1. **Missing DNS:Edit Permission**
   - Check token has `Zone:DNS:Edit` permission for the zone

2. **Wrong Domain**
   - Verify `cloudflare.domain` matches your Cloudflare zone

3. **Zone Not Found**
   - Domain must be active in your Cloudflare account

### Network Route Not Working

**Symptoms:**
- WARP clients can't reach routed IPs
- Traffic not flowing through tunnel

**Diagnostic Steps:**

```bash
# Check NetworkRoute status
kubectl get networkroute <name> -o wide

# Verify tunnel has WARP routing enabled
kubectl get clustertunnel <name> -o jsonpath='{.spec.enableWarpRouting}'
```

**Common Causes:**

1. **WARP Routing Not Enabled**
   ```yaml
   spec:
     enableWarpRouting: true  # Must be true
   ```

2. **Route Conflicts**
   - Check for overlapping routes in Cloudflare Dashboard
   - Verify CIDR doesn't conflict with existing routes

3. **Virtual Network Mismatch**
   - WARP client must be connected to the correct virtual network

### Access Application Not Protecting

**Symptoms:**
- Application accessible without authentication
- Login page not showing

**Diagnostic Steps:**

```bash
# Check AccessApplication status
kubectl get accessapplication <name> -o wide
kubectl describe accessapplication <name>
```

**Common Causes:**

1. **DNS Not Pointing to Tunnel**
   - Application domain must be served through Cloudflare Tunnel

2. **Policy Misconfiguration**
   - Check AccessGroup rules are correct
   - Verify policy decision is `allow` not `bypass`

3. **Missing IdP Configuration**
   - AccessIdentityProvider must be configured and referenced

### Resource Stuck in Deleting

**Symptoms:**
- Resource has `DeletionTimestamp` set
- Finalizers preventing deletion

**Diagnostic Steps:**

```bash
# Check finalizers
kubectl get <resource> <name> -o jsonpath='{.metadata.finalizers}'

# Check for deletion errors
kubectl describe <resource> <name>
```

**Resolution:**

1. **Check Cloudflare API Errors**
   - Resource may have already been deleted in Cloudflare
   - Operator retries deletion

2. **Manual Finalizer Removal** (use with caution)
   ```bash
   kubectl patch <resource> <name> -p '{"metadata":{"finalizers":null}}' --type=merge
   ```

## Error Messages

### "API Token validation failed"

- Token is invalid or expired
- Recreate token in Cloudflare Dashboard

### "Zone not found"

- Domain not in your Cloudflare account
- Domain not active (pending nameserver change)

### "Conflict: resource already exists"

- Resource with same name exists in Cloudflare
- Use `existingTunnel` to adopt existing resources

### "Permission denied"

- Token missing required permissions
- Check [Permission Matrix](configuration.md#permission-matrix)

## Getting Help

If issues persist:

1. **Collect Logs**
   ```bash
   kubectl logs -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager > operator.log
   ```

2. **Check GitHub Issues**
   - Search [existing issues](https://github.com/StringKe/cloudflare-operator/issues)

3. **Open New Issue**
   - Include operator version
   - Include relevant CRD manifests (sanitized)
   - Include error logs
