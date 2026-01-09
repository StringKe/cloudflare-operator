---
name: e2e-testing
description: End-to-end testing for cloudflare-operator. Guides testing CRDs against real Cloudflare API. Use when testing operator functionality, validating CRDs, or running integration tests.
allowed-tools: Read, Write, Bash, Glob, Grep
user-invocable: true
---

# E2E Testing Guide

## Overview

This skill guides end-to-end testing of cloudflare-operator against real Cloudflare API.

## Prerequisites

### Required Credentials
- Cloudflare API Token or Global API Key
- Cloudflare Account ID
- Cloudflare Domain (for DNS-related tests)

### Required Permissions

| Feature | Permission | Scope |
|---------|------------|-------|
| Tunnel | `Account:Cloudflare Tunnel:Edit` | Account |
| DNS | `Zone:DNS:Edit` | Zone |
| Access | `Account:Access: Apps and Policies:Edit` | Account |
| Zero Trust | `Account:Zero Trust:Edit` | Account |

## Setup

### 1. Deploy Operator

```bash
# Install CRDs
kubectl apply -f https://github.com/StringKe/cloudflare-operator/releases/latest/download/cloudflare-operator.crds.yaml

# Install Operator
kubectl apply -f https://github.com/StringKe/cloudflare-operator/releases/latest/download/cloudflare-operator.yaml

# Verify
kubectl get pods -n cloudflare-operator-system
```

### 2. Create Credentials Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cloudflare-credentials
  namespace: cloudflare-operator-system
type: Opaque
stringData:
  CLOUDFLARE_API_TOKEN: "${CLOUDFLARE_API_TOKEN}"
  # Or use Global API Key:
  # CLOUDFLARE_API_KEY: "${CLOUDFLARE_API_KEY}"
  # CLOUDFLARE_API_EMAIL: "${CLOUDFLARE_EMAIL}"
```

### 3. Create CloudflareCredentials Resource

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: CloudflareCredentials
metadata:
  name: default-credentials
spec:
  accountId: "${CLOUDFLARE_ACCOUNT_ID}"
  secretRef:
    name: cloudflare-credentials
    namespace: cloudflare-operator-system
```

## Test Order

Test CRDs in dependency order:

### Phase 1: Infrastructure
1. CloudflareCredentials
2. ClusterTunnel / Tunnel
3. VirtualNetwork

### Phase 2: Networking
4. NetworkRoute (depends on Tunnel + VirtualNetwork)
5. TunnelBinding (depends on Tunnel)
6. DNSRecord
7. PrivateService

### Phase 3: Access Control
8. AccessGroup
9. AccessIdentityProvider
10. AccessServiceToken
11. AccessApplication

### Phase 4: Zero Trust
12. GatewayConfiguration
13. GatewayList
14. GatewayRule
15. DevicePostureRule
16. DeviceSettingsPolicy

## Test Manifests

### ClusterTunnel

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: ClusterTunnel
metadata:
  name: test-tunnel
spec:
  newTunnel:
    name: e2e-test-tunnel
  cloudflare:
    accountId: "${CLOUDFLARE_ACCOUNT_ID}"
    domain: "${CLOUDFLARE_DOMAIN}"
    credentialsRef:
      name: default-credentials
  deployPatch: '{"spec":{"replicas":1}}'
```

### VirtualNetwork

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: VirtualNetwork
metadata:
  name: test-vnet
spec:
  name: e2e-test-vnet
  comment: "E2E test virtual network"
  cloudflare:
    accountId: "${CLOUDFLARE_ACCOUNT_ID}"
    credentialsRef:
      name: default-credentials
```

### NetworkRoute

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: NetworkRoute
metadata:
  name: test-route
spec:
  network: "10.200.0.0/24"
  comment: "E2E test route"
  tunnelRef:
    kind: ClusterTunnel
    name: test-tunnel
  virtualNetworkRef:
    name: test-vnet
  cloudflare:
    accountId: "${CLOUDFLARE_ACCOUNT_ID}"
    credentialsRef:
      name: default-credentials
```

## Validation Commands

```bash
# Check resource status
kubectl get <resource> -o wide

# Check conditions
kubectl get <resource> -o jsonpath='{.status.conditions}'

# Check operator logs
kubectl logs -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager -f

# Describe resource for events
kubectl describe <resource> <name>
```

## Common Issues

### "secret not found"
- Ensure secret is in correct namespace
- For cluster-scoped resources: `cloudflare-operator-system`
- For namespaced resources: same namespace as resource

### "authentication error"
- Verify API token permissions
- Check account ID is correct
- Ensure token hasn't expired

### "resource already exists"
- Check if resource exists in Cloudflare Dashboard
- Use `existingTunnel` instead of `newTunnel` for adoption

### Stuck in "Terminating"
```bash
# Check finalizers
kubectl get <resource> <name> -o jsonpath='{.metadata.finalizers}'

# Force delete (caution: may leave orphaned resources)
kubectl patch <resource> <name> -p '{"metadata":{"finalizers":null}}' --type=merge
```

## Cleanup

Delete in reverse order:

```bash
# Phase 4
kubectl delete gatewayrule,gatewaylist,gatewayconfiguration --all
kubectl delete deviceposturerule,devicesettingspolicy --all

# Phase 3
kubectl delete accessapplication,accessservicetoken --all
kubectl delete accessidentityprovider,accessgroup --all

# Phase 2
kubectl delete privateservice,dnsrecord,tunnelbinding --all
kubectl delete networkroute --all

# Phase 1
kubectl delete virtualnetwork --all
kubectl delete clustertunnel,tunnel --all
kubectl delete cloudflarecredentials --all

# Secrets
kubectl delete secret cloudflare-credentials -n cloudflare-operator-system
```

## Test Report Template

```markdown
## E2E Test Report

**Date:** YYYY-MM-DD
**Version:** v0.17.X
**Cluster:** cluster-name

### Results

| CRD | Create | Update | Delete | Status |
|-----|--------|--------|--------|--------|
| CloudflareCredentials | ✅ | ✅ | ✅ | PASS |
| ClusterTunnel | ✅ | ✅ | ✅ | PASS |
| VirtualNetwork | ✅ | ✅ | ✅ | PASS |
| NetworkRoute | ✅ | ✅ | ✅ | PASS |
| ... | | | | |

### Issues Found
- Issue description

### Notes
- Additional observations
```
