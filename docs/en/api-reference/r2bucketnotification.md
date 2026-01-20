# R2BucketNotification

R2BucketNotification is a namespaced resource that configures event notifications for Cloudflare R2 buckets.

## Overview

R2BucketNotification enables you to receive notifications when objects are uploaded, deleted, or modified in your R2 bucket. Notifications can be sent to event platforms like AWS SQS, Azure Queue Storage, or JSON endpoints.

### Key Features

- Object lifecycle event notifications
- Multiple destination types supported
- Event filtering by object prefix
- Automatic retry on delivery failure

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `bucketRef` | BucketRef | **Yes** | Reference to R2Bucket |
| `destination` | NotificationDestination | **Yes** | Event destination |
| `events` | []string | **Yes** | Events to notify on |
| `cloudflare` | CloudflareDetails | **Yes** | Cloudflare API credentials |

## Examples

### Example 1: SQS Notifications

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: R2BucketNotification
metadata:
  name: bucket-sqs-notifications
  namespace: production
spec:
  bucketRef:
    name: app-storage
  destination:
    queue:
      queueName: "r2-events"
      queueRegion: "us-east-1"
  events:
    - "object-created"
    - "object-deleted"
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## Prerequisites

- R2Bucket resource exists
- Destination service configured
- Valid credentials

## Related Resources

- [R2Bucket](r2bucket.md) - The storage bucket
- [R2BucketDomain](r2bucketdomain.md) - Custom domain

## See Also

- [Cloudflare R2 Notifications](https://developers.cloudflare.com/r2/notifications/)
