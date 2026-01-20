# DeviceSettingsPolicy

DeviceSettingsPolicy is a cluster-scoped resource that configures device settings for Cloudflare WARP clients.

## Overview

DeviceSettingsPolicy allows you to configure WARP client behavior and security settings centrally from Kubernetes.

### Key Features

- Device settings management
- Policy-based configuration
- Remote configuration
- Compliance enforcement

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | **Yes** | Policy name |
| `settings` | map[string]string | No | Device settings |
| `cloudflare` | CloudflareDetails | **Yes** | API credentials |

## Examples

### Example 1: Device Settings Policy

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: DeviceSettingsPolicy
metadata:
  name: enterprise-policy
spec:
  name: "Enterprise Policy"
  settings:
    splitTunneling: "enabled"
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## See Also

- [Cloudflare Device Settings](https://developers.cloudflare.com/warp-client/)
