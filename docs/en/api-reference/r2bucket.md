# R2Bucket

R2Bucket is a namespaced resource that creates and manages Cloudflare R2 object storage buckets with lifecycle rules and configuration.

## Overview

R2Bucket creates and manages Cloudflare R2 object storage buckets directly from Kubernetes. R2 is Cloudflare's S3-compatible object storage service. The operator handles bucket creation, configuration, and can apply lifecycle rules to automatically manage object retention and expiration.

### Key Features

| Feature | Description |
|---------|-------------|
| **S3 Compatible** | Full AWS S3 API compatibility |
| **Lifecycle Management** | Automatic retention and expiration rules |
| **Bucket Configuration** | Manage bucket settings from Kubernetes |
| **Access Control** | Configure bucket visibility and CORS |
| **Automatic Cleanup** | Delete bucket data during resource deletion |

### Use Cases

- **Object Storage**: Use Kubernetes-native R2 bucket management
- **Data Archival**: Implement lifecycle policies for data archival
- **Static Assets**: Store static content with CDN delivery
- **Backup Storage**: Centralized backup storage for applications
- **Multi-Tenant**: Create separate buckets for different applications

## Spec

### Main Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `bucketName` | string | No | Resource name | Name of the R2 bucket |
| `lifecycleRules` | []LifecycleRule | No | - | Bucket lifecycle rules |
| `cloudflare` | CloudflareDetails | **Yes** | - | Cloudflare API credentials |

### LifecycleRule

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | **Yes** | Rule ID (unique within bucket) |
| `enabled` | bool | **Yes** | Enable/disable rule |
| `prefix` | string | No | Object key prefix to match |
| `expiration` | *int | No | Days until object expiration |
| `noncurrentVersionExpiration` | *int | No | Days until old versions expire |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `bucketId` | string | Cloudflare R2 Bucket ID |
| `bucketName` | string | Bucket name in Cloudflare |
| `accountId` | string | Cloudflare Account ID |
| `state` | string | Current state |
| `endpoint` | string | R2 bucket endpoint URL |
| `conditions` | []metav1.Condition | Latest observations |

## Examples

### Example 1: Basic R2 Bucket

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: R2Bucket
metadata:
  name: app-storage
  namespace: production
spec:
  bucketName: "app-storage-prod"
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

### Example 2: Bucket with Lifecycle Rules

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: R2Bucket
metadata:
  name: logs-archive
  namespace: production
spec:
  bucketName: "logs-archive"
  lifecycleRules:
    - id: "delete-old-logs"
      enabled: true
      prefix: "logs/"
      expiration: 90
    - id: "archive-data"
      enabled: true
      prefix: "data/"
      expiration: 365
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

### Example 3: Multi-Tier Storage

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: R2Bucket
metadata:
  name: backups
  namespace: production
spec:
  bucketName: "database-backups"
  lifecycleRules:
    - id: "daily-cleanup"
      enabled: true
      prefix: "daily/"
      expiration: 30
    - id: "monthly-retain"
      enabled: true
      prefix: "monthly/"
      expiration: 365
    - id: "yearly-archive"
      enabled: true
      prefix: "yearly/"
      expiration: 2555
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## Prerequisites

- Cloudflare account with R2 enabled
- Valid API credentials with R2 permissions
- Unique bucket name (globally across all R2 users)

## Limitations

- Bucket names must be globally unique
- Bucket deletion removes all objects
- Lifecycle rules are applied asynchronously
- Cannot rename bucket after creation
- Object retention policies not yet supported

## Related Resources

- [R2BucketDomain](r2bucketdomain.md) - Custom domain for bucket
- [R2BucketNotification](r2bucketnotification.md) - Event notifications
- [CloudflareCredentials](cloudflarecredentials.md) - API credentials

## See Also

- [Cloudflare R2 Documentation](https://developers.cloudflare.com/r2/)
- [S3 API Compatibility](https://developers.cloudflare.com/r2/data-access/s3-api/)
