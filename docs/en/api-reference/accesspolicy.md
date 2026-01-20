# AccessPolicy

AccessPolicy is a cluster-scoped resource that defines reusable Cloudflare Access policies for controlling application access based on identity, device, and context.

## Overview

AccessPolicy enables you to define centralized, reusable access control policies that can be referenced by multiple AccessApplication resources. Instead of embedding access rules in each application, you can create policies once and reuse them, simplifying policy management and ensuring consistency across your zero-trust infrastructure.

### Key Features

| Feature | Description |
|---------|-------------|
| **Reusable Policies** | Define once, reference from multiple applications |
| **Flexible Rules** | Support include, exclude, and require rule logic |
| **Decision Control** | Allow, deny, bypass, or non-identity decisions |
| **Session Management** | Override session duration per policy |
| **Browser Isolation** | Enable isolation requirements per policy |
| **Approval Workflows** | Require admin approval for access |

### Use Cases

- **Centralized Rules**: Define organization-wide access rules
- **Policy Reuse**: Apply same policy to multiple applications
- **Compliance**: Implement consistent compliance controls
- **Device Posture**: Enforce device security requirements
- **Approval Workflows**: Require approval for sensitive resources

## Spec

### Main Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `precedence` | int | No | - | Evaluation order (lower first) |
| `decision` | string | **Yes** | `allow` | Policy decision: `allow`, `deny`, `bypass`, `non_identity` |
| `include` | []AccessGroupRule | **Yes** | - | Rules that must match (OR logic) |
| `exclude` | []AccessGroupRule | No | - | Rules that must NOT match (NOT logic) |
| `require` | []AccessGroupRule | No | - | Rules that must ALL match (AND logic) |
| `sessionDuration` | string | No | - | Override session duration (e.g., "24h", "30m") |
| `isolationRequired` | *bool | No | - | Require browser isolation |
| `purposeJustificationRequired` | *bool | No | - | Require access justification |
| `purposeJustificationPrompt` | string | No | - | Custom justification prompt |
| `approvalRequired` | *bool | No | - | Require admin approval |
| `approvalGroups` | []ApprovalGroup | No | - | Groups that can approve |
| `cloudflare` | CloudflareDetails | **Yes** | - | Cloudflare API credentials |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `policyId` | string | Cloudflare Access Policy ID |
| `accountId` | string | Cloudflare Account ID |
| `state` | string | Current state |
| `conditions` | []metav1.Condition | Latest observations |
| `observedGeneration` | int64 | Last generation observed |

## Examples

### Example 1: Basic Allow Policy

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessPolicy
metadata:
  name: allow-employees
spec:
  decision: allow
  precedence: 10
  include:
    - okta:
        identityProviderId: "okta-prod"
        groups:
          - "employees"
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

### Example 2: Policy with Multiple Conditions

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessPolicy
metadata:
  name: secure-app-access
spec:
  decision: allow
  precedence: 20
  include:
    - okta:
        identityProviderId: "okta-prod"
        groups:
          - "developers"
  require:
    - devicePosture:
        postures:
          - "malware_protection_enabled"
  exclude:
    - ip:
        ips:
          - "192.0.2.0/24"
  sessionDuration: "8h"
  isolationRequired: true
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

### Example 3: Policy with Approval

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessPolicy
metadata:
  name: database-access
spec:
  decision: allow
  precedence: 100
  include:
    - okta:
        identityProviderId: "okta-prod"
        groups:
          - "dba"
  approvalRequired: true
  approvalGroups:
    - name: "database-admins"
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## Prerequisites

- Cloudflare Zero Trust subscription
- Valid Cloudflare API credentials
- Configured Identity Providers (Okta, Azure AD, etc.)
- Device posture rules if using device requirements

## Limitations

- Policies are account-scoped
- Precedence must be unique within account
- Cannot be deleted if referenced by active applications
- Session duration must be valid Go duration string
- Device posture rules must exist if referenced

## Related Resources

- [AccessApplication](accessapplication.md) - Reference these policies in applications
- [AccessGroup](accessgroup.md) - Create reusable access groups
- [AccessIdentityProvider](accessidentityprovider.md) - Configure identity providers
- [DevicePostureRule](deviceposturerule.md) - Define device requirements

## See Also

- [Cloudflare Access Policies](https://developers.cloudflare.com/cloudflare-one/policies/access/)
- [Zero Trust Access Control](https://developers.cloudflare.com/cloudflare-one/access/)
