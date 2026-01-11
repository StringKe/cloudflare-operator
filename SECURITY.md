# Security Policy / 安全策略

## Supported Versions / 支持的版本

| Version | Supported          |
| ------- | ------------------ |
| 0.18.x  | :white_check_mark: |
| 0.17.x  | :white_check_mark: |
| < 0.17  | :x:                |

## Reporting a Vulnerability / 报告漏洞

If you discover a security vulnerability in this project, please report it responsibly.

如果你在此项目中发现安全漏洞，请负责任地报告。

### How to Report / 如何报告

**DO NOT** open a public GitHub issue for security vulnerabilities.

**不要**为安全漏洞开公开的 GitHub issue。

Instead, please:
请采用以下方式：

1. **Email**: Send details to the maintainers (check repository for contact info)
   **邮件**：将详情发送给维护者（查看仓库获取联系信息）

2. **GitHub Security Advisory**: Use GitHub's private vulnerability reporting feature
   **GitHub 安全公告**：使用 GitHub 的私密漏洞报告功能

### What to Include / 需包含的信息

- Description of the vulnerability / 漏洞描述
- Steps to reproduce / 复现步骤
- Potential impact / 潜在影响
- Suggested fix (if any) / 建议的修复方案（如有）

### Response Timeline / 响应时间

- **Initial response**: Within 48 hours / 初始响应：48 小时内
- **Status update**: Within 7 days / 状态更新：7 天内
- **Fix timeline**: Depends on severity / 修复时间：取决于严重程度

## Security Best Practices / 安全最佳实践

When using this operator, follow these security practices:

使用此 operator 时，请遵循以下安全实践：

### API Token Security / API Token 安全

```yaml
# ✅ Use Kubernetes Secrets for API tokens
apiVersion: v1
kind: Secret
metadata:
  name: cloudflare-credentials
type: Opaque
stringData:
  CLOUDFLARE_API_TOKEN: "<token>"

# ❌ Never hardcode tokens in CRDs
# spec:
#   cloudflare:
#     apiToken: "hardcoded-token"  # WRONG!
```

### RBAC Configuration / RBAC 配置

- Use least-privilege principles
- 使用最小权限原则

- Restrict access to Cloudflare credential secrets
- 限制对 Cloudflare 凭证 secret 的访问

- Use separate namespaces for different environments
- 为不同环境使用独立的命名空间

```yaml
# Restrict secret access to specific service accounts
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: cloudflare-secret-reader
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    resourceNames: ["cloudflare-credentials"]
    verbs: ["get"]
```

### Network Policies / 网络策略

Consider implementing network policies to restrict operator communication:

考虑实施网络策略以限制 operator 通信：

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: cloudflare-operator-egress
  namespace: cloudflare-operator-system
spec:
  podSelector:
    matchLabels:
      control-plane: controller-manager
  policyTypes:
    - Egress
  egress:
    # Allow Cloudflare API
    - to:
        - ipBlock:
            cidr: 0.0.0.0/0
      ports:
        - port: 443
          protocol: TCP
```

### Audit Logging / 审计日志

Enable Kubernetes audit logging to track:
启用 Kubernetes 审计日志以跟踪：

- Secret access / Secret 访问
- CRD modifications / CRD 修改
- Operator actions / Operator 操作

### Token Rotation / Token 轮换

Regularly rotate Cloudflare API tokens:
定期轮换 Cloudflare API token：

1. Create new token in Cloudflare Dashboard
   在 Cloudflare 控制台创建新 token

2. Update Kubernetes Secret
   更新 Kubernetes Secret

3. Verify operator functionality
   验证 operator 功能

4. Revoke old token
   撤销旧 token

## Known Security Considerations / 已知安全注意事项

### Event Messages / 事件消息

The operator sanitizes error messages in Kubernetes events to prevent credential leakage. However:

Operator 会清理 Kubernetes 事件中的错误消息以防止凭证泄露。但是：

- Review operator logs for sensitive information
- 检查 operator 日志中是否有敏感信息

- Configure log levels appropriately in production
- 在生产环境中适当配置日志级别

### Tunnel Credentials / 隧道凭证

Tunnel credentials are stored in Kubernetes Secrets. Ensure:
隧道凭证存储在 Kubernetes Secrets 中。请确保：

- Encryption at rest is enabled
- 启用静态加密

- Access is restricted via RBAC
- 通过 RBAC 限制访问

- Secrets are not exposed in ConfigMaps or logs
- Secrets 不会在 ConfigMaps 或日志中暴露

## Security Updates / 安全更新

Security updates will be released as patch versions when possible. Monitor:

安全更新将尽可能作为补丁版本发布。请关注：

- GitHub Releases for security patches
- GitHub Security Advisories
- This SECURITY.md for policy updates
