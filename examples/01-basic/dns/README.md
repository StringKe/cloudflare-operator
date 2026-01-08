# DNS Record Examples / DNS 记录示例

This directory contains examples for managing Cloudflare DNS records.

此目录包含管理 Cloudflare DNS 记录的示例。

## Files / 文件

| File / 文件 | Description / 说明 |
|-------------|-------------------|
| `dns-record.yaml` | Various DNS record types / 各种 DNS 记录类型 |

## Supported Record Types / 支持的记录类型

| Type / 类型 | Proxiable / 可代理 | Description / 说明 |
|-------------|-------------------|-------------------|
| `A` | Yes | IPv4 address / IPv4 地址 |
| `AAAA` | Yes | IPv6 address / IPv6 地址 |
| `CNAME` | Yes | Canonical name / 规范名称 |
| `TXT` | No | Text record / 文本记录 |
| `MX` | No | Mail exchange / 邮件交换 |
| `NS` | No | Name server / 名称服务器 |
| `SRV` | No | Service record / 服务记录 |
| `CAA` | No | Certificate authority / 证书颁发机构 |

## Usage / 使用方法

```bash
# Apply DNS records
# 应用 DNS 记录
kubectl apply -f dns-record.yaml

# Check status
# 检查状态
kubectl get dnsrecord

# View details
# 查看详情
kubectl describe dnsrecord www-record
```

## Notes / 注意事项

- `proxied: true` enables Cloudflare's CDN and security features
  `proxied: true` 启用 Cloudflare 的 CDN 和安全功能

- `ttl: 1` means "automatic" - Cloudflare manages the TTL
  `ttl: 1` 表示"自动" - Cloudflare 管理 TTL

- For TunnelBinding, DNS records are created automatically
  对于 TunnelBinding，DNS 记录会自动创建

- DNSRecord does not require accountId (only zone/domain needed)
  DNSRecord 不需要 accountId（只需要 zone/domain）
