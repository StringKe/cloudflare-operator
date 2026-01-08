# 配置

本指南涵盖 API token 配置和凭证管理。

## API Token 设置

### 创建 API Token

1. 访问 [Cloudflare 控制台](https://dash.cloudflare.com/profile/api-tokens)
2. 点击 **Create Token**
3. 选择 **Create Custom Token**
4. 根据需要配置权限

### 权限矩阵

| 功能 | 权限 | 范围 |
|------|------|------|
| **隧道管理** | `Account:Cloudflare Tunnel:Edit` | Account |
| **DNS 记录** | `Zone:DNS:Edit` | Zone（指定或全部）|
| **Access 应用** | `Account:Access: Apps and Policies:Edit` | Account |
| **Access 组** | `Account:Access: Apps and Policies:Edit` | Account |
| **Access 身份提供商** | `Account:Access: Apps and Policies:Edit` | Account |
| **Access 服务令牌** | `Account:Access: Service Tokens:Edit` | Account |
| **网关规则** | `Account:Zero Trust:Edit` | Account |
| **网关列表** | `Account:Zero Trust:Edit` | Account |
| **网关配置** | `Account:Zero Trust:Edit` | Account |
| **设备设置** | `Account:Zero Trust:Edit` | Account |
| **设备态势** | `Account:Zero Trust:Edit` | Account |
| **WARP Connector** | `Account:Cloudflare Tunnel:Edit` | Account |

### 推荐的 Token 配置

#### 最小权限（隧道 + DNS）

```
权限：
- Account > Cloudflare Tunnel > Edit
- Zone > DNS > Edit

账户资源：
- Include > 你的账户

区域资源：
- Include > Specific zone > example.com
```

#### 完整 Zero Trust

```
权限：
- Account > Cloudflare Tunnel > Edit
- Account > Access: Apps and Policies > Edit
- Account > Access: Service Tokens > Edit
- Account > Zero Trust > Edit
- Zone > DNS > Edit

账户资源：
- Include > 你的账户

区域资源：
- Include > All zones（或指定区域）
```

## Kubernetes Secret

### 使用 API Token（推荐）

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

### 使用 API Key（旧版）

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

### Secret 位置

- **命名空间级资源**（Tunnel、TunnelBinding 等）：Secret 在相同命名空间
- **集群级资源**（ClusterTunnel、VirtualNetwork 等）：Secret 在 operator 命名空间（`cloudflare-operator-system`）

## CloudflareSpec 参考

所有与 Cloudflare API 交互的 CRD 都包含 `cloudflare` 规格：

```yaml
spec:
  cloudflare:
    # Cloudflare Account ID（大多数资源必填）
    accountId: "your-account-id"

    # Cloudflare 管理的域名（DNS 相关操作必填）
    domain: example.com

    # 包含 API 凭证的 Kubernetes Secret 名称
    secret: cloudflare-credentials

    # 替代方式：使用账户名称代替 ID（可选）
    # accountName: "My Account"

    # Secret 中 API Token 的键名（默认：CLOUDFLARE_API_TOKEN）
    # CLOUDFLARE_API_TOKEN: "CUSTOM_TOKEN_KEY"

    # Secret 中 API Key 的键名（默认：CLOUDFLARE_API_KEY）
    # CLOUDFLARE_API_KEY: "CUSTOM_KEY"

    # API Key 认证的邮箱（可选）
    # email: admin@example.com
```

## 获取 Account ID

### 方法 1：域名概览

1. 登录 Cloudflare 控制台
2. 选择任意域名
3. 在右侧边栏"API"部分找到 **Account ID**

### 方法 2：账户 URL

1. 进入 Account Home
2. Account ID 在 URL 中：`dash.cloudflare.com/<account-id>/...`

### 方法 3：API

```bash
curl -X GET "https://api.cloudflare.com/client/v4/accounts" \
  -H "Authorization: Bearer <your-token>" \
  -H "Content-Type: application/json"
```

## 安全最佳实践

### Token 轮换

1. 在 Cloudflare 控制台创建新 token
2. 更新 Kubernetes Secret
3. 验证 operator 功能
4. 撤销旧 token

### RBAC

限制对凭证 secret 的访问：

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

### 外部 Secret 管理

考虑使用：
- [External Secrets Operator](https://external-secrets.io/)
- [Vault](https://www.vaultproject.io/)
- [AWS Secrets Manager](https://aws.amazon.com/secrets-manager/)

## 故障排除

### Token 不工作

```bash
# 使用 curl 测试 token
curl -X GET "https://api.cloudflare.com/client/v4/user/tokens/verify" \
  -H "Authorization: Bearer <your-token>"

# 检查 operator 日志
kubectl logs -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager
```

### 权限被拒绝

1. 验证 token 具有所需权限
2. 检查 account/zone 范围是否匹配你的资源
3. 确保 token 未过期
