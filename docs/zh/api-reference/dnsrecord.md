# DNSRecord

DNSRecord 是命名空间级资源，用于声明式管理 Cloudflare DNS 记录。它支持所有标准 DNS 记录类型，并集成 Cloudflare 的代理功能。

## 概述

DNSRecord 允许您使用 Kubernetes 清单管理 Cloudflare 区域中的 DNS 记录。对 DNSRecord 资源的更改会自动同步到 Cloudflare，Operator 管理 DNS 记录的完整生命周期。

### 主要特性

| 特性 | 描述 |
|------|------|
| **全面的记录类型** | 支持 A、AAAA、CNAME、TXT、MX、SRV、CAA 等 |
| **Cloudflare 代理** | 为符合条件的记录类型启用橙色云代理 |
| **自动 TTL** | 使用 Cloudflare 的自动 TTL 管理 |
| **标签和注释** | 组织和记录您的 DNS 记录 |
| **类型特定数据** | SRV、CAA、LOC 记录的高级配置 |

### 使用场景

- **应用程序端点**: 为服务端点创建 A/AAAA 记录
- **CDN 集成**: 启用代理记录以使用 Cloudflare CDN
- **邮件配置**: 管理邮件的 MX 和 TXT 记录
- **服务发现**: 使用 SRV 记录进行服务发现
- **域名验证**: 创建 TXT 记录以验证域名所有权

## Spec

### 主要字段

| 字段 | 类型 | 必需 | 默认值 | 描述 |
|------|------|------|--------|------|
| `name` | string | **是** | - | DNS 记录名称（子域名或 FQDN，最大 255 字符） |
| `type` | string | **是** | - | 记录类型：A、AAAA、CNAME、TXT、MX、NS、SRV、CAA 等 |
| `content` | string | **是** | - | 记录内容（IP、主机名或文本值） |
| `ttl` | int | 否 | `1` | 生存时间（秒）（1 = 自动） |
| `proxied` | bool | 否 | `false` | 启用 Cloudflare 代理（橙色云） |
| `priority` | *int | 否 | - | MX/SRV 记录的优先级（0-65535） |
| `comment` | string | 否 | - | 可选注释（最多 100 字符） |
| `tags` | []string | 否 | - | 用于组织的标签 |
| `data` | *DNSRecordData | 否 | - | SRV、CAA、LOC 等的类型特定数据 |
| `cloudflare` | CloudflareDetails | **是** | - | Cloudflare API 凭证 |

### 支持的记录类型

A、AAAA、CNAME、TXT、MX、NS、SRV、CAA、CERT、DNSKEY、DS、HTTPS、LOC、NAPTR、SMIMEA、SSHFP、SVCB、TLSA、URI

### DNSRecordData（用于高级记录类型）

| 字段 | 类型 | 用于 | 描述 |
|------|------|------|------|
| `service` | string | SRV | 服务名称 |
| `proto` | string | SRV | 协议（tcp/udp） |
| `weight` | int | SRV | 负载均衡权重 |
| `port` | int | SRV | 服务端口 |
| `target` | string | SRV | 目标主机名 |
| `flags` | int | CAA | CAA 标志 |
| `tag` | string | CAA | CAA 标签（issue/issuewild/iodef） |
| `value` | string | CAA | CAA 值 |

## Status

| 字段 | 类型 | 描述 |
|------|------|------|
| `recordId` | string | Cloudflare DNS 记录 ID |
| `zoneId` | string | Cloudflare Zone ID |
| `fqdn` | string | 完全限定域名 |
| `state` | string | 当前状态（pending、ready、error） |
| `conditions` | []Condition | 标准 Kubernetes 条件 |
| `observedGeneration` | int64 | 最后观察到的 generation |

## 示例

### 基础 A 记录（已代理）

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
  ttl: 1  # 自动 TTL
  proxied: true  # 启用 Cloudflare 代理
  comment: "Web 服务器端点"

  cloudflare:
    domain: example.com
    secret: cloudflare-api-credentials
```

### CNAME 记录

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

### 用于验证的 TXT 记录

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

### 用于邮件的 MX 记录

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: DNSRecord
metadata:
  name: mx-primary
  namespace: default
spec:
  name: "@"  # 根域名
  type: MX
  content: mail.example.com
  priority: 10
  ttl: 3600
  comment: "主邮件服务器"

  cloudflare:
    domain: example.com
    secret: cloudflare-api-credentials
```

### 用于服务发现的 SRV 记录

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

### 用于证书颁发机构的 CAA 记录

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

### 用于负载均衡的多条记录

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

## 前置条件

1. **Cloudflare Zone**: 域名必须由 Cloudflare 管理
2. **API 凭证**: 具有 DNS 编辑权限的 API Token
3. **Zone ID**: 从域名自动解析

### 所需 API 权限

| 权限 | 范围 | 用途 |
|------|------|------|
| `Zone:DNS:Edit` | Zone | 创建/更新/删除 DNS 记录 |

## 限制

- **代理记录**: 只有 A、AAAA 和 CNAME 记录可以被代理
- **根域名**: 对根域名记录使用 `"@"`
- **代理时的 TTL**: 当 `proxied: true` 时，TTL 自动管理
- **记录唯一性**: 每个 DNSRecord 资源应管理一条 DNS 记录

## 最佳实践

1. **使用描述性名称**: 清晰命名 DNSRecord 资源（例如，`www-record`、`mx-primary`）
2. **为 Web 启用代理**: 对面向 Web 的 A/AAAA/CNAME 记录使用 `proxied: true`
3. **使用标签**: 使用标签组织记录以便管理
4. **添加注释**: 记录每条记录的用途
5. **分离命名空间**: 使用命名空间分离不同应用程序的 DNS 记录

## 相关资源

- [Tunnel](tunnel.md) - 自动为隧道端点创建 DNS 记录
- [Ingress 集成](../guides/ingress-integration.md) - 通过 Ingress 注解自动 DNS
- [Gateway API](../guides/gateway-api-integration.md) - 使用 Gateway API 管理 DNS

## 另请参阅

- [示例](../../../examples/01-basic/dns/)
- [Cloudflare DNS 文档](https://developers.cloudflare.com/dns/)
- [DNS 记录类型](https://www.cloudflare.com/learning/dns/dns-records/)
