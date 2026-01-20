# AccessPolicy

AccessPolicy 是一个集群作用域的资源，定义可重用的 Cloudflare Access 策略以根据身份、设备和上下文控制应用程序访问。

## 概述

AccessPolicy 使您能够定义可由多个 AccessApplication 资源引用的集中式、可重用的访问控制策略。您可以创建一次策略并在多个地方重用，而不是在每个应用程序中嵌入访问规则，从而简化策略管理并确保整个零信任基础设施的一致性。

### 主要特性

| 特性 | 描述 |
|------|------|
| **可重用策略** | 定义一次，从多个应用程序引用 |
| **灵活的规则** | 支持包含、排除和必需规则逻辑 |
| **决策控制** | 允许、拒绝、旁路或非身份决策 |
| **会话管理** | 每个策略覆盖会话时间 |
| **浏览器隔离** | 按策略启用隔离要求 |
| **审批工作流** | 需要管理员批准访问 |

### 使用场景

- **集中式规则**：定义组织范围的访问规则
- **策略重用**：将相同策略应用于多个应用程序
- **合规性**：实现一致的合规性控制
- **设备态势**：强制设备安全要求
- **审批工作流**：需要对敏感资源的审批

## 规范

### 主要字段

| 字段 | 类型 | 必需 | 默认值 | 描述 |
|------|------|------|--------|------|
| `precedence` | int | 否 | - | 评估顺序（较低的优先） |
| `decision` | string | **是** | `allow` | 策略决策：`allow`、`deny`、`bypass`、`non_identity` |
| `include` | []AccessGroupRule | **是** | - | 必须匹配的规则（OR 逻辑） |
| `exclude` | []AccessGroupRule | 否 | - | 必须不匹配的规则（NOT 逻辑） |
| `require` | []AccessGroupRule | 否 | - | 必须全部匹配的规则（AND 逻辑） |
| `sessionDuration` | string | 否 | - | 覆盖会话持续时间（例如"24h"、"30m"） |
| `isolationRequired` | *bool | 否 | - | 需要浏览器隔离 |
| `purposeJustificationRequired` | *bool | 否 | - | 需要访问理由 |
| `purposeJustificationPrompt` | string | 否 | - | 自定义理由提示 |
| `approvalRequired` | *bool | 否 | - | 需要管理员批准 |
| `approvalGroups` | []ApprovalGroup | 否 | - | 可以批准的组 |
| `cloudflare` | CloudflareDetails | **是** | - | Cloudflare API 凭证 |

## 状态

| 字段 | 类型 | 描述 |
|------|------|------|
| `policyId` | string | Cloudflare Access 策略 ID |
| `accountId` | string | Cloudflare 账户 ID |
| `state` | string | 当前状态 |
| `conditions` | []metav1.Condition | 最新观察 |
| `observedGeneration` | int64 | 控制器观察到的最后一代 |

## 示例

### 示例 1：基本允许策略

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessPolicy
metadata:
  name: allow-employees
spec:
  decision: allow
  precedence: 10
  include:
    - okta:
        identityProviderId: "okta-prod"
        groups:
          - "employees"
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

### 示例 2：具有多个条件的策略

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessPolicy
metadata:
  name: secure-app-access
spec:
  decision: allow
  precedence: 20
  include:
    - okta:
        identityProviderId: "okta-prod"
        groups:
          - "developers"
  require:
    - devicePosture:
        postures:
          - "malware_protection_enabled"
  exclude:
    - ip:
        ips:
          - "192.0.2.0/24"
  sessionDuration: "8h"
  isolationRequired: true
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

### 示例 3：需要审批的策略

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessPolicy
metadata:
  name: database-access
spec:
  decision: allow
  precedence: 100
  include:
    - okta:
        identityProviderId: "okta-prod"
        groups:
          - "dba"
  approvalRequired: true
  approvalGroups:
    - name: "database-admins"
  cloudflare:
    accountId: "1234567890abcdef"
    credentialsRef:
      name: production
```

## 前置条件

- Cloudflare Zero Trust 订阅
- 有效的 Cloudflare API 凭证
- 配置的身份提供商（Okta、Azure AD 等）
- 如果使用设备要求，需要设备态势规则

## 限制

- 策略是账户作用域的
- 优先级必须在账户内唯一
- 如果被活动应用程序引用，则无法删除
- 会话持续时间必须是有效的 Go 持续时间字符串
- 必须存在被引用的设备态势规则

## 相关资源

- [AccessApplication](accessapplication.md) - 在应用程序中引用这些策略
- [AccessGroup](accessgroup.md) - 创建可重用的访问组
- [AccessIdentityProvider](accessidentityprovider.md) - 配置身份提供商
- [DevicePostureRule](deviceposturerule.md) - 定义设备要求

## 另请参阅

- [Cloudflare Access 策略](https://developers.cloudflare.com/cloudflare-one/policies/access/)
- [零信任访问控制](https://developers.cloudflare.com/cloudflare-one/access/)
