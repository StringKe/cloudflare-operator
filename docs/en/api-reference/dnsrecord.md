# DNSRecord

DNSRecord is a namespaced resource for managing Cloudflare DNS records declaratively. It supports all standard DNS record types and integrates with Cloudflare's proxy features.

## Overview

DNSRecord allows you to manage DNS records in your Cloudflare zone using Kubernetes manifests. Changes to DNSRecord resources are automatically synchronized to Cloudflare, and the operator manages the full lifecycle of DNS records.

### Key Features

| Feature | Description |
|---------|-------------|
| **Comprehensive Record Types** | Supports A, AAAA, CNAME, TXT, MX, SRV, CAA, and more |
| **Cloudflare Proxy** | Enable orange-cloud proxying for eligible record types |
| **Automatic TTL** | Use Cloudflare's automatic TTL management |
| **Tags and Comments** | Organize and document your DNS records |
| **Type-Specific Data** | Advanced configuration for SRV, CAA, LOC records |

### Use Cases

- **Application Endpoints**: Create A/AAAA records for service endpoints
- **CDN Integration**: Enable proxied records for Cloudflare CDN
- **Email Configuration**: Manage MX and TXT records for email
- **Service Discovery**: Use SRV records for service discovery
- **Domain Verification**: Create TXT records for domain ownership verification

## Spec

### Main Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | **Yes** | - | DNS record name (subdomain or FQDN, max 255 chars) |
| `type` | string | **Yes** | - | Record type: A, AAAA, CNAME, TXT, MX, NS, SRV, CAA, etc. |
| `content` | string | **Yes** | - | Record content (IP, hostname, or text value) |
| `ttl` | int | No | `1` | Time To Live in seconds (1 = automatic) |
| `proxied` | bool | No | `false` | Enable Cloudflare proxy (orange cloud) |
| `priority` | *int | No | - | Priority for MX/SRV records (0-65535) |
| `comment` | string | No | - | Optional comment (max 100 chars) |
| `tags` | []string | No | - | Tags for organization |
| `data` | *DNSRecordData | No | - | Type-specific data for SRV, CAA, LOC, etc. |
| `cloudflare` | CloudflareDetails | **Yes** | - | Cloudflare API credentials |

### Supported Record Types

A, AAAA, CNAME, TXT, MX, NS, SRV, CAA, CERT, DNSKEY, DS, HTTPS, LOC, NAPTR, SMIMEA, SSHFP, SVCB, TLSA, URI

### DNSRecordData (for advanced record types)

| Field | Type | Used For | Description |
|-------|------|----------|-------------|
| `service` | string | SRV | Service name |
| `proto` | string | SRV | Protocol (tcp/udp) |
| `weight` | int | SRV | Weight for load balancing |
| `port` | int | SRV | Service port |
| `target` | string | SRV | Target hostname |
| `flags` | int | CAA | CAA flags |
| `tag` | string | CAA | CAA tag (issue/issuewild/iodef) |
| `value` | string | CAA | CAA value |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `recordId` | string | Cloudflare DNS Record ID |
| `zoneId` | string | Cloudflare Zone ID |
| `fqdn` | string | Fully Qualified Domain Name |
| `state` | string | Current state (pending, ready, error) |
| `conditions` | []Condition | Standard Kubernetes conditions |
| `observedGeneration` | int64 | Last observed generation |

## Examples

### Basic A Record (Proxied)

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: DNSRecord
metadata:
  name: www-record
  namespace: default
spec:
  name: www
  type: A
  content: 203.0.113.50
  ttl: 1  # Automatic TTL
  proxied: true  # Enable Cloudflare proxy
  comment: "Web server endpoint"

  cloudflare:
    domain: example.com
    secret: cloudflare-api-credentials
```

### CNAME Record

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: DNSRecord
metadata:
  name: blog-cname
  namespace: default
spec:
  name: blog
  type: CNAME
  content: www.example.com
  proxied: true

  cloudflare:
    domain: example.com
    secret: cloudflare-api-credentials
```

### TXT Record for Verification

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: DNSRecord
metadata:
  name: txt-verification
  namespace: default
spec:
  name: _verify
  type: TXT
  content: "verification-token-12345"
  ttl: 3600
  tags:
    - verification
    - google

  cloudflare:
    domain: example.com
    secret: cloudflare-api-credentials
```

### MX Record for Email

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: DNSRecord
metadata:
  name: mx-primary
  namespace: default
spec:
  name: "@"  # Root domain
  type: MX
  content: mail.example.com
  priority: 10
  ttl: 3600
  comment: "Primary mail server"

  cloudflare:
    domain: example.com
    secret: cloudflare-api-credentials
```

### SRV Record for Service Discovery

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: DNSRecord
metadata:
  name: srv-ldap
  namespace: default
spec:
  name: _ldap._tcp
  type: SRV
  content: ldap.example.com
  priority: 10
  ttl: 3600

  data:
    service: ldap
    proto: tcp
    weight: 5
    port: 389
    target: ldap.example.com

  cloudflare:
    domain: example.com
    secret: cloudflare-api-credentials
```

### CAA Record for Certificate Authority

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: DNSRecord
metadata:
  name: caa-letsencrypt
  namespace: default
spec:
  name: "@"
  type: CAA
  content: letsencrypt.org

  data:
    flags: 0
    tag: issue
    value: letsencrypt.org

  cloudflare:
    domain: example.com
    secret: cloudflare-api-credentials
```

### Multiple Records for Load Balancing

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: DNSRecord
metadata:
  name: api-server-1
  namespace: default
spec:
  name: api
  type: A
  content: 203.0.113.10
  ttl: 300
  proxied: false

  cloudflare:
    domain: example.com
    secret: cloudflare-api-credentials
---
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: DNSRecord
metadata:
  name: api-server-2
  namespace: default
spec:
  name: api
  type: A
  content: 203.0.113.20
  ttl: 300
  proxied: false

  cloudflare:
    domain: example.com
    secret: cloudflare-api-credentials
```

## Prerequisites

1. **Cloudflare Zone**: Domain must be managed by Cloudflare
2. **API Credentials**: API Token with DNS Edit permission
3. **Zone ID**: Automatically resolved from domain name

### Required API Permissions

| Permission | Scope | Purpose |
|------------|-------|---------|
| `Zone:DNS:Edit` | Zone | Create/update/delete DNS records |

## Limitations

- **Proxied Records**: Only A, AAAA, and CNAME records can be proxied
- **Root Domain**: Use `"@"` for root domain records
- **TTL with Proxy**: TTL is automatically managed when `proxied: true`
- **Record Uniqueness**: Each DNSRecord resource should manage one DNS record

## Best Practices

1. **Use Descriptive Names**: Name DNSRecord resources clearly (e.g., `www-record`, `mx-primary`)
2. **Enable Proxy for Web**: Use `proxied: true` for web-facing A/AAAA/CNAME records
3. **Use Tags**: Organize records with tags for easier management
4. **Add Comments**: Document the purpose of each record
5. **Separate Namespaces**: Use namespaces to separate different applications' DNS records

## Related Resources

- [Tunnel](tunnel.md) - Automatically create DNS records for tunnel endpoints
- [Ingress Integration](../guides/ingress-integration.md) - Automatic DNS via Ingress annotations
- [Gateway API](../guides/gateway-api-integration.md) - DNS management with Gateway API

## See Also

- [Examples](../../../examples/01-basic/dns/)
- [Cloudflare DNS Documentation](https://developers.cloudflare.com/dns/)
- [DNS Record Types](https://www.cloudflare.com/learning/dns/dns-records/)
