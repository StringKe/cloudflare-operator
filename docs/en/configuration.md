# Configuration

This guide covers API token configuration and credential management.

## API Token Setup

### Quick Create (Recommended)

Use the script to create a pre-configured API token:

```bash
# Set your credentials
export CF_API_TOKEN="your-existing-token-with-token-create-permission"
export CF_ACCOUNT_ID="your-account-id"

# Create token with all required permissions
curl -X POST "https://api.cloudflare.com/client/v4/user/tokens" \
  -H "Authorization: Bearer $CF_API_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{
    "name": "cloudflare-operator",
    "policies": [
      {
        "effect": "allow",
        "resources": {"com.cloudflare.api.account.'$CF_ACCOUNT_ID'": "*"},
        "permission_groups": [
          {"id": "TUNNEL_EDIT_ID", "name": "Cloudflare Tunnel Edit"},
          {"id": "ACCESS_APPS_EDIT_ID", "name": "Access: Apps and Policies Edit"},
          {"id": "ACCESS_ORGS_EDIT_ID", "name": "Access: Organizations, Identity Providers, and Groups Edit"},
          {"id": "ACCESS_TOKENS_EDIT_ID", "name": "Access: Service Tokens Edit"},
          {"id": "ZERO_TRUST_EDIT_ID", "name": "Zero Trust Edit"}
        ]
      },
      {
        "effect": "allow",
        "resources": {"com.cloudflare.api.account.zone.*": "*"},
        "permission_groups": [
          {"id": "DNS_EDIT_ID", "name": "DNS Edit"}
        ]
      }
    ]
  }'
```

> **Note**: Replace `*_ID` placeholders with actual permission group IDs from your account. Get them via: `GET /accounts/{account_id}/iam/permission_groups`

### Manual Create (Dashboard)

1. Go to [Cloudflare Dashboard > API Tokens](https://dash.cloudflare.com/profile/api-tokens)
2. Click **Create Token**
3. Select **Create Custom Token**
4. Configure permissions as shown below

### Permission Matrix

#### Tunnel & Network

| Feature | Permission | Scope |
|---------|------------|-------|
| **Tunnel / ClusterTunnel** | `Account:Cloudflare Tunnel:Edit` | Account |
| **VirtualNetwork** | `Account:Cloudflare Tunnel:Edit` | Account |
| **NetworkRoute** | `Account:Cloudflare Tunnel:Edit` | Account |
| **WARPConnector** | `Account:Cloudflare Tunnel:Edit` | Account |
| **PrivateService** | `Account:Cloudflare Tunnel:Edit` | Account |
| **TunnelBinding** | `Zone:DNS:Edit` + (optional) `Account:Access: Apps and Policies:Edit` | Account + Zone |

#### DNS & Connectivity

| Feature | Permission | Scope |
|---------|------------|-------|
| **DNSRecord** | `Zone:DNS:Edit` | Zone (specific or all) |

#### Access Control (Zero Trust)

| Feature | Permission | Scope |
|---------|------------|-------|
| **AccessApplication** | `Account:Access: Apps and Policies:Edit` | Account |
| **AccessGroup** | `Account:Access: Organizations, Identity Providers, and Groups:Edit` | Account |
| **AccessPolicy** | `Account:Access: Apps and Policies:Edit` | Account |
| **AccessIdentityProvider** | `Account:Access: Organizations, Identity Providers, and Groups:Edit` | Account |
| **AccessServiceToken** | `Account:Access: Service Tokens:Edit` | Account |

#### Gateway & Device

| Feature | Permission | Scope |
|---------|------------|-------|
| **GatewayRule** | `Account:Zero Trust:Edit` | Account |
| **GatewayList** | `Account:Zero Trust:Edit` | Account |
| **GatewayConfiguration** | `Account:Zero Trust:Edit` | Account |
| **DevicePostureRule** | `Account:Access: Device Posture:Edit` | Account |
| **DeviceSettingsPolicy** | `Account:Zero Trust:Edit` | Account |

#### Zone Settings & SSL/TLS

| Feature | Permission | Scope |
|---------|------------|-------|
| **CloudflareDomain** | `Zone:Zone Settings:Edit` + `Zone:SSL and Certificates:Edit` | Zone |
| **OriginCACertificate** | `Zone:SSL and Certificates:Edit` | Zone |

#### R2 Storage

| Feature | Permission | Scope |
|---------|------------|-------|
| **R2Bucket** | `Account:Workers R2 Storage:Edit` | Account |
| **R2BucketDomain** | `Account:Workers R2 Storage:Edit` + `Zone:DNS:Edit` | Account + Zone |
| **R2BucketNotification** | `Account:Workers R2 Storage:Edit` | Account |

#### Rules Engine

| Feature | Permission | Scope |
|---------|------------|-------|
| **ZoneRuleset** | `Zone:Zone Rulesets:Edit` | Zone |
| **TransformRule** | `Zone:Zone Rulesets:Edit` | Zone |
| **RedirectRule** | `Zone:Zone Rulesets:Edit` | Zone |

#### Cloudflare Pages

| Feature | Permission | Scope |
|---------|------------|-------|
| **PagesProject** | `Account:Cloudflare Pages:Edit` | Account |
| **PagesDomain** | `Account:Cloudflare Pages:Edit` + `Zone:DNS:Edit` | Account + Zone |
| **PagesDeployment** | `Account:Cloudflare Pages:Edit` | Account |

#### Registrar (Enterprise)

| Feature | Permission | Scope |
|---------|------------|-------|
| **DomainRegistration** | `Account:Registrar:Edit` | Account |

### Recommended Token Configurations

#### Minimal (Tunnel + DNS)

```
Permissions:
- Account > Cloudflare Tunnel > Edit
- Zone > DNS > Edit

Account Resources:
- Include > Your Account

Zone Resources:
- Include > Specific zone > example.com
```

#### Full Zero Trust

```
Permissions:
- Account > Cloudflare Tunnel > Edit
- Account > Access: Apps and Policies > Edit
- Account > Access: Service Tokens > Edit
- Account > Zero Trust > Edit
- Zone > DNS > Edit

Account Resources:
- Include > Your Account

Zone Resources:
- Include > All zones (or specific zones)
```

## Kubernetes Secret

### Using API Token (Recommended)

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cloudflare-credentials
  namespace: default
type: Opaque
stringData:
  CLOUDFLARE_API_TOKEN: "your-api-token-here"
```

### Using API Key (Legacy)

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cloudflare-credentials
  namespace: default
type: Opaque
stringData:
  CLOUDFLARE_API_KEY: "your-global-api-key"
  CLOUDFLARE_API_EMAIL: "your-email@example.com"
```

### Secret Location

The operator uses different Secret lookup rules based on CRD scope:

| Resource Scope | Secret Location | Examples |
|----------------|-----------------|----------|
| **Namespaced** | Same namespace as the resource | Tunnel, TunnelBinding, DNSRecord, AccessApplication |
| **Cluster** | Operator namespace (`cloudflare-operator-system`) | ClusterTunnel, VirtualNetwork, NetworkRoute, AccessGroup |

> **Important**: For detailed information about namespace restrictions and Secret management, see [Namespace Restrictions](namespace-restrictions.md).

## CloudflareSpec Reference

All CRDs that interact with Cloudflare API include a `cloudflare` spec:

```yaml
spec:
  cloudflare:
    # Your Cloudflare Account ID (required for most resources)
    accountId: "your-account-id"

    # Domain managed by Cloudflare (required for DNS-related operations)
    domain: example.com

    # Name of the Kubernetes Secret containing API credentials
    secret: cloudflare-credentials

    # Alternative: Account name instead of ID (optional)
    # accountName: "My Account"

    # Key name in Secret for API Token (default: CLOUDFLARE_API_TOKEN)
    # CLOUDFLARE_API_TOKEN: "CUSTOM_TOKEN_KEY"

    # Key name in Secret for API Key (default: CLOUDFLARE_API_KEY)
    # CLOUDFLARE_API_KEY: "CUSTOM_KEY"

    # Email for API Key authentication (optional)
    # email: admin@example.com
```

## Finding Your Account ID

### Method 1: Domain Overview

1. Log in to Cloudflare Dashboard
2. Select any domain
3. Find **Account ID** in the right sidebar under "API"

### Method 2: Account URL

1. Go to Account Home
2. The Account ID is in the URL: `dash.cloudflare.com/<account-id>/...`

### Method 3: API

```bash
curl -X GET "https://api.cloudflare.com/client/v4/accounts" \
  -H "Authorization: Bearer <your-token>" \
  -H "Content-Type: application/json"
```

## Security Best Practices

### Token Rotation

1. Create new token in Cloudflare Dashboard
2. Update Kubernetes Secret
3. Verify operator functionality
4. Revoke old token

### RBAC

Restrict access to credential secrets:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: cloudflare-credentials-reader
  namespace: default
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    resourceNames: ["cloudflare-credentials"]
    verbs: ["get"]
```

### External Secret Management

Consider using:
- [External Secrets Operator](https://external-secrets.io/)
- [Vault](https://www.vaultproject.io/)
- [AWS Secrets Manager](https://aws.amazon.com/secrets-manager/)

## Troubleshooting

### Token Not Working

```bash
# Test token with curl
curl -X GET "https://api.cloudflare.com/client/v4/user/tokens/verify" \
  -H "Authorization: Bearer <your-token>"

# Check operator logs
kubectl logs -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager
```

### Permission Denied

1. Verify token has required permissions
2. Check account/zone scope matches your resources
3. Ensure token hasn't expired
