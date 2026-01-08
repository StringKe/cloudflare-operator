# Cloudflare API Credentials / Cloudflare API 凭证

This directory contains examples for setting up Cloudflare API credentials.

此目录包含设置 Cloudflare API 凭证的示例。

## Files / 文件

- `api-secret.yaml` - Kubernetes Secret for API credentials / API 凭证的 Kubernetes Secret

## Usage / 使用方法

### 1. Create API Token / 创建 API Token

1. Go to [Cloudflare Dashboard](https://dash.cloudflare.com/profile/api-tokens)
2. Click **Create Token**
3. Select **Create Custom Token**
4. Configure permissions based on your needs

---

1. 访问 [Cloudflare 控制台](https://dash.cloudflare.com/profile/api-tokens)
2. 点击 **Create Token**
3. 选择 **Create Custom Token**
4. 根据需要配置权限

### 2. Required Permissions / 所需权限

| Feature / 功能 | Permission / 权限 | Scope / 范围 |
|----------------|-------------------|--------------|
| Tunnel | `Account > Cloudflare Tunnel > Edit` | Account |
| DNS Records | `Zone > DNS > Edit` | Zone |
| Access Apps | `Account > Access: Apps and Policies > Edit` | Account |
| Service Tokens | `Account > Access: Service Tokens > Edit` | Account |
| Gateway | `Account > Zero Trust > Edit` | Account |

### 3. Apply Secret / 应用 Secret

```bash
# Edit the file with your token
# 编辑文件填入你的 token
vim api-secret.yaml

# Apply to cluster
# 应用到集群
kubectl apply -f api-secret.yaml

# Verify secret exists
# 验证 secret 存在
kubectl get secret cloudflare-api-credentials
```

## Security Notes / 安全注意事项

- Never commit secrets with real values to version control
- 永远不要将包含真实值的 secret 提交到版本控制

- Use RBAC to restrict access to the secret
- 使用 RBAC 限制对 secret 的访问

- Consider using external secret management (e.g., Vault, External Secrets Operator)
- 考虑使用外部 secret 管理（如 Vault、External Secrets Operator）

- Rotate API tokens regularly
- 定期轮换 API token
