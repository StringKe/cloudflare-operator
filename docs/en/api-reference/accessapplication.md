# AccessApplication

AccessApplication is a cluster-scoped resource that represents a Cloudflare Access Application. It protects web applications, SSH endpoints, VNC, and other resources with Zero Trust authentication.

## Overview

AccessApplication supports two modes for defining access policies:

| Mode | Description | Use Case |
|------|-------------|----------|
| **Group Reference Mode** | Reference existing AccessGroup resources | Simple setups, reusable groups |
| **Inline Rules Mode** | Define include/exclude/require rules directly | Quick setup, application-specific rules |

## Spec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | No | K8s resource name | Application name in Cloudflare |
| `domain` | string | **Yes** | - | Primary domain/URL for the application |
| `type` | string | **Yes** | `self_hosted` | Application type (see below) |
| `sessionDuration` | string | No | `24h` | Session duration before re-authentication |
| `policies` | []AccessPolicyRef | No | - | Access policies (see Policy Modes) |
| `reusablePolicyRefs` | []ReusablePolicyRef | No | - | References to reusable Access Policies |
| `cloudflare` | CloudflareDetails | **Yes** | - | Cloudflare API credentials |

### Application Types

| Type | Description |
|------|-------------|
| `self_hosted` | Self-hosted web application |
| `saas` | SaaS application (SAML/OIDC) |
| `ssh` | SSH endpoint |
| `vnc` | VNC endpoint |
| `app_launcher` | App Launcher |
| `warp` | WARP client |
| `biso` | Browser Isolation |
| `bookmark` | Bookmark |
| `dash_sso` | Dashboard SSO |
| `infrastructure` | Infrastructure application |

### Additional Spec Fields

| Field | Type | Description |
|-------|------|-------------|
| `selfHostedDomains` | []string | Additional domains for the application |
| `destinations` | []AccessDestination | Public/private destination configurations |
| `allowedIdps` | []string | Allowed identity provider IDs |
| `allowedIdpRefs` | []AccessIdentityProviderRef | References to AccessIdentityProvider resources |
| `autoRedirectToIdentity` | bool | Auto-redirect to identity provider |
| `appLauncherVisible` | bool | Show in App Launcher |
| `skipInterstitial` | bool | Skip the interstitial page |
| `logoUrl` | string | Application logo URL |
| `customDenyMessage` | string | Custom access denied message |
| `customDenyUrl` | string | Custom access denied URL |
| `corsHeaders` | AccessApplicationCorsHeaders | CORS configuration |
| `saasApp` | SaasApplicationConfig | SaaS app config (for type=saas) |
| `tags` | []string | Custom tags |

## Policy Modes

### Mode 1: Group Reference Mode (Simple)

Reference existing AccessGroup resources or Cloudflare Access Groups:

```yaml
spec:
  policies:
    # Option 1: Reference K8s AccessGroup by name
    - name: accessgroup-employees
      decision: allow
      precedence: 1

    # Option 2: Reference Cloudflare Access Group by UUID
    - groupId: "12345678-1234-1234-1234-123456789abc"
      decision: allow
      precedence: 2

    # Option 3: Reference Cloudflare Access Group by display name
    - cloudflareGroupName: "Infrastructure Users"
      decision: allow
      precedence: 3
```

### Mode 2: Inline Rules Mode (Advanced)

Define include/exclude/require rules directly in the policy:

```yaml
spec:
  policies:
    - policyName: "Allow Company Employees"
      decision: allow
      precedence: 1
      # Include: Users matching ANY rule will be granted access (OR logic)
      include:
        - email:
            email: "admin@example.com"
        - emailDomain:
            domain: "example.com"
      # Exclude: Users matching ANY rule will be denied (NOT logic)
      exclude:
        - email:
            email: "contractor@example.com"
      # Require: Users must match ALL rules (AND logic)
      require:
        - geo:
            country: ["US", "CA"]
```

### AccessPolicyRef Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | K8s AccessGroup resource name (Group Reference Mode) |
| `groupId` | string | Cloudflare Access Group UUID (Group Reference Mode) |
| `cloudflareGroupName` | string | Cloudflare Access Group display name (Group Reference Mode) |
| `include` | []AccessGroupRule | Include rules (Inline Rules Mode) |
| `exclude` | []AccessGroupRule | Exclude rules (Inline Rules Mode) |
| `require` | []AccessGroupRule | Require rules (Inline Rules Mode) |
| `decision` | string | Policy decision: `allow`, `deny`, `bypass`, `non_identity` |
| `precedence` | int | Order of evaluation (lower = higher priority) |
| `policyName` | string | Policy name in Cloudflare |
| `sessionDuration` | string | Override session duration for this policy |

### Supported Rule Types

The following rule types are supported for `include`, `exclude`, and `require` arrays:

| Rule Type | Description | Example |
|-----------|-------------|---------|
| `email` | Match specific email | `email: { email: "user@example.com" }` |
| `emailDomain` | Match email domain | `emailDomain: { domain: "example.com" }` |
| `everyone` | Match all users | `everyone: {}` |
| `group` | Match IdP group | `group: { id: "group-id" }` |
| `ipRanges` | Match IP ranges | `ipRanges: { ranges: ["10.0.0.0/8"] }` |
| `geo` | Match country codes | `geo: { country: ["US", "CA"] }` |
| `anyValidServiceToken` | Match any valid service token | `anyValidServiceToken: {}` |
| `serviceToken` | Match specific service token | `serviceToken: { tokenId: "token-id" }` |
| `certificate` | Match client certificate | `certificate: {}` |
| `commonName` | Match certificate CN | `commonName: { commonName: "*.example.com" }` |
| `loginMethod` | Match login method | `loginMethod: { id: "method-id" }` |
| `devicePosture` | Match device posture | `devicePosture: { integrationUid: "uid" }` |
| `warp` | Match WARP clients | `warp: {}` |
| `gsuite` | Match Google Workspace | `gsuite: { identityProviderId: "id", email: "user@example.com" }` |
| `github` | Match GitHub organization | `github: { identityProviderId: "id", name: "org-name" }` |
| `okta` | Match Okta group | `okta: { identityProviderId: "id", name: "group-name" }` |
| `azure` | Match Azure AD group | `azure: { identityProviderId: "id", id: "group-id" }` |
| `saml` | Match SAML attribute | `saml: { attributeName: "role", attributeValue: "admin" }` |
| `authContext` | Match authentication context | `authContext: { id: "context-id", acId: "ac-id", identityProviderId: "id" }` |
| `externalEvaluation` | External evaluation | `externalEvaluation: { evaluateUrl: "https://..." }` |
| `groupId` | Match Access Group by ID | `groupId: { id: "group-id" }` |
| `emailList` | Match email list | `emailList: { id: "list-id" }` |
| `ipList` | Match IP list | `ipList: { id: "list-id" }` |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `applicationId` | string | Cloudflare Application ID |
| `aud` | string | Application Audience (AUD) Tag |
| `accountId` | string | Cloudflare Account ID |
| `domain` | string | Configured domain |
| `selfHostedDomains` | []string | All configured domains |
| `state` | string | Current state |
| `resolvedPolicies` | []ResolvedPolicyStatus | Resolved policy information |
| `conditions` | []Condition | Standard Kubernetes conditions |

## Examples

### Basic Application with Group Reference

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessApplication
metadata:
  name: internal-dashboard
spec:
  name: Internal Dashboard
  domain: dashboard.example.com
  type: self_hosted
  sessionDuration: 24h
  appLauncherVisible: true

  policies:
    - name: employees
      decision: allow
      precedence: 1

  cloudflare:
    accountId: "<account-id>"
    domain: example.com
    secret: cloudflare-credentials
```

### Application with Inline Email Rules

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessApplication
metadata:
  name: team-wiki
spec:
  name: Team Wiki
  domain: wiki.example.com
  type: self_hosted

  policies:
    - policyName: "Allow Team Members"
      decision: allow
      precedence: 1
      include:
        - emailDomain:
            domain: "example.com"
      exclude:
        - email:
            email: "contractor@example.com"

  cloudflare:
    accountId: "<account-id>"
    domain: example.com
    secret: cloudflare-credentials
```

### Application with Require Rules (Multiple Conditions)

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessApplication
metadata:
  name: admin-panel
spec:
  name: Admin Panel
  domain: admin.example.com
  type: self_hosted
  sessionDuration: 1h

  policies:
    - policyName: "Admin Access - US Only"
      decision: allow
      precedence: 1
      include:
        - emailDomain:
            domain: "example.com"
      require:
        - geo:
            country: ["US"]

  cloudflare:
    accountId: "<account-id>"
    domain: example.com
    secret: cloudflare-credentials
```

### Public API (Bypass Authentication)

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessApplication
metadata:
  name: public-api
spec:
  name: Public API
  domain: api.example.com
  type: self_hosted

  policies:
    - policyName: "Allow Everyone"
      decision: bypass
      precedence: 1
      include:
        - everyone: {}

  cloudflare:
    accountId: "<account-id>"
    domain: example.com
    secret: cloudflare-credentials
```

### Service Token Access (M2M)

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessApplication
metadata:
  name: api-service
spec:
  name: API Service
  domain: api-internal.example.com
  type: self_hosted

  policies:
    - policyName: "Service Token Access"
      decision: non_identity
      precedence: 1
      include:
        - anyValidServiceToken: {}
    - policyName: "Human Access"
      decision: allow
      precedence: 2
      include:
        - emailDomain:
            domain: "example.com"

  cloudflare:
    accountId: "<account-id>"
    domain: example.com
    secret: cloudflare-credentials
```

## Related Resources

- [AccessGroup](accessgroup.md) - Define reusable access groups
- [AccessServiceToken](accessservicetoken.md) - Create service tokens for M2M authentication
- [AccessIdentityProvider](accessidentityprovider.md) - Configure identity providers

## See Also

- [Examples](../../../examples/03-zero-trust/access-application/)
- [Cloudflare Access Documentation](https://developers.cloudflare.com/cloudflare-one/policies/access/)
