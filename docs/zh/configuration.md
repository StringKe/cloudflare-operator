# 配置

本指南涵盖 API token 配置和凭证管理。

## API Token 设置

### 快速创建（推荐）

使用脚本创建预配置的 API Token：

```bash
# 设置凭证
export CF_API_TOKEN="你的已有token（需要有创建token的权限）"
export CF_ACCOUNT_ID="你的账户ID"

# 创建包含所有必需权限的 token
curl -X POST "https://api.cloudflare.com/client/v4/user/tokens" \
  -H "Authorization: Bearer $CF_API_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{
    "name": "cloudflare-operator",
    "policies": [
      {
        "effect": "allow",
        "resources": {"com.cloudflare.api.account.'$CF_ACCOUNT_ID'": "*"},
        "permission_groups": [
          {"id": "TUNNEL_EDIT_ID", "name": "Cloudflare Tunnel Edit"},
          {"id": "ACCESS_APPS_EDIT_ID", "name": "Access: Apps and Policies Edit"},
          {"id": "ACCESS_ORGS_EDIT_ID", "name": "Access: Organizations, Identity Providers, and Groups Edit"},
          {"id": "ACCESS_TOKENS_EDIT_ID", "name": "Access: Service Tokens Edit"},
          {"id": "ZERO_TRUST_EDIT_ID", "name": "Zero Trust Edit"}
        ]
      },
      {
        "effect": "allow",
        "resources": {"com.cloudflare.api.account.zone.*": "*"},
        "permission_groups": [
          {"id": "DNS_EDIT_ID", "name": "DNS Edit"}
        ]
      }
    ]
  }'
```

> **注意**：将 `*_ID` 占位符替换为你账户的实际权限组 ID。通过以下 API 获取：`GET /accounts/{account_id}/iam/permission_groups`

### 手动创建（控制台）

1. 访问 [Cloudflare 控制台 > API Tokens](https://dash.cloudflare.com/profile/api-tokens)
2. 点击 **Create Token**
3. 选择 **Create Custom Token**
4. 按下表配置权限

### 权限矩阵

| 功能 | 权限 | 范围 |
|------|------|------|
| **Tunnel / ClusterTunnel** | `Account:Cloudflare Tunnel:Edit` | Account |
| **VirtualNetwork** | `Account:Cloudflare Tunnel:Edit` | Account |
| **NetworkRoute** | `Account:Cloudflare Tunnel:Edit` | Account |
| **WARPConnector** | `Account:Cloudflare Tunnel:Edit` | Account |
| **DNS 记录** | `Zone:DNS:Edit` | Zone（指定或全部）|
| **TunnelBinding** | `Zone:DNS:Edit` + (可选) `Account:Access: Apps and Policies:Edit` | Account + Zone |
| **PrivateService** | `Account:Cloudflare Tunnel:Edit` | Account |
| **Access 应用** | `Account:Access: Apps and Policies:Edit` | Account |
| **Access 组** | `Account:Access: Organizations, Identity Providers, and Groups:Edit` | Account |
| **Access 身份提供商** | `Account:Access: Organizations, Identity Providers, and Groups:Edit` | Account |
| **Access 服务令牌** | `Account:Access: Service Tokens:Edit` | Account |
| **设备态势规则** | `Account:Access: Device Posture:Edit` | Account |
| **设备设置策略** | `Account:Zero Trust:Edit` | Account |
| **网关规则** | `Account:Zero Trust:Edit` | Account |
| **网关列表** | `Account:Zero Trust:Edit` | Account |
| **网关配置** | `Account:Zero Trust:Edit` | Account |

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

Operator 根据 CRD 作用域使用不同的 Secret 查找规则：

| 资源作用域 | Secret 位置 | 示例 |
|-----------|-------------|------|
| **Namespaced** | 与资源相同的命名空间 | Tunnel, TunnelBinding, DNSRecord, AccessApplication |
| **Cluster** | Operator 命名空间（`cloudflare-operator-system`）| ClusterTunnel, VirtualNetwork, NetworkRoute, AccessGroup |

> **重要**：有关命名空间限制和 Secret 管理的详细信息，请参阅 [命名空间限制](namespace-restrictions.md)。

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
