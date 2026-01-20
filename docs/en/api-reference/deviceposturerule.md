# DevicePostureRule

DevicePostureRule is a cluster-scoped resource that defines device security requirements for Zero Trust Access.

## Overview

DevicePostureRule defines conditions that devices must meet before accessing protected applications, such as antivirus installation, firewall status, or disk encryption.

### Key Features

- Device security requirements
- Multiple condition types
- Posture checks and validation
- Automatic enforcement

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | **Yes** | Rule name |
| `rules` | []PostureCheck | **Yes** | Posture checks |
| `cloudflare` | CloudflareDetails | **Yes** | API credentials |

## Examples

### Example 1: Require Antivirus

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: DevicePostureRule
metadata:
  name: require-antivirus
spec:
  name: "Require Antivirus"
  rules:
    - type: "firewall"
      enabled: true
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## See Also

- [Cloudflare Device Posture](https://developers.cloudflare.com/cloudflare-one/identity/devices/)
