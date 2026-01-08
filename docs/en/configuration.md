# Configuration

This guide covers API token configuration and credential management.

## API Token Setup

### Creating an API Token

1. Go to [Cloudflare Dashboard](https://dash.cloudflare.com/profile/api-tokens)
2. Click **Create Token**
3. Select **Create Custom Token**
4. Configure permissions based on your needs

### Permission Matrix

| Feature | Permission | Scope |
|---------|------------|-------|
| **Tunnel Management** | `Account:Cloudflare Tunnel:Edit` | Account |
| **DNS Records** | `Zone:DNS:Edit` | Zone (specific or all) |
| **Access Applications** | `Account:Access: Apps and Policies:Edit` | Account |
| **Access Groups** | `Account:Access: Apps and Policies:Edit` | Account |
| **Access Identity Providers** | `Account:Access: Apps and Policies:Edit` | Account |
| **Access Service Tokens** | `Account:Access: Service Tokens:Edit` | Account |
| **Gateway Rules** | `Account:Zero Trust:Edit` | Account |
| **Gateway Lists** | `Account:Zero Trust:Edit` | Account |
| **Gateway Configuration** | `Account:Zero Trust:Edit` | Account |
| **Device Settings** | `Account:Zero Trust:Edit` | Account |
| **Device Posture** | `Account:Zero Trust:Edit` | Account |
| **WARP Connector** | `Account:Cloudflare Tunnel:Edit` | Account |

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

- **Namespaced Resources** (Tunnel, TunnelBinding, etc.): Secret in the same namespace
- **Cluster Resources** (ClusterTunnel, VirtualNetwork, etc.): Secret in the operator namespace (`cloudflare-operator-system`)

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
