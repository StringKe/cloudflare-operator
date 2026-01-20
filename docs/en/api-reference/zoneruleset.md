# ZoneRuleset

ZoneRuleset is a namespaced resource that manages Cloudflare rulesets for WAF, rate limiting, and other security features.

## Overview

ZoneRuleset enables you to configure Cloudflare's managed rulesets including WAF rules, rate limiting, and other security features at the zone level.

### Key Features

- WAF rule management
- Rate limiting configuration
- Security rules
- Rule versioning

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | **Yes** | Ruleset name |
| `kind` | string | **Yes** | Ruleset type (waf, rateLimit, etc.) |
| `rules` | []Rule | No | Rules to apply |
| `cloudflare` | CloudflareDetails | **Yes** | API credentials |

## Examples

### Example 1: WAF Ruleset

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: ZoneRuleset
metadata:
  name: waf-rules
  namespace: production
spec:
  name: "WAF Rules"
  kind: "waf"
  cloudflare:
    accountId: "1234567890abcdef"
    domain: "example.com"
    credentialsRef:
      name: production
```

## See Also

- [Cloudflare Rulesets](https://developers.cloudflare.com/ruleset-engine/)
