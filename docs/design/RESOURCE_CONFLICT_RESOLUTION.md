# 资源冲突解决方案 - 统一聚合模式

## 概述

本文档定义了 Cloudflare Operator 中多个 K8s 资源管理同一 Cloudflare 设置时的冲突检测、预防和解决机制。

**核心设计决策**: 所有资源类型统一采用 **Type A 聚合模式**，不使用多种不同的解决方案，避免维护复杂性和不一致问题。

## 统一聚合模式 (Unified Aggregation Pattern)

### 核心原则

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     统一聚合模式 (Type A)                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ 多个 K8s 资源 → CloudflareSyncState (多源聚合) → 一个 Cloudflare 资源│   │
│  │                                                                     │   │
│  │ 特点:                                                               │   │
│  │   1. 所有来源配置存储在 SyncState.spec.sources[]                     │   │
│  │   2. 按优先级排序 (priority 字段)                                    │   │
│  │   3. 所有权标记嵌入 Cloudflare 资源描述字段                           │   │
│  │   4. 删除时: 重新聚合剩余来源 + 保留外部配置                          │   │
│  │                                                                     │   │
│  │ 优势:                                                               │   │
│  │   ✅ 统一实现模式，易于维护                                          │   │
│  │   ✅ 支持多资源协作管理同一配置                                       │   │
│  │   ✅ 删除操作不会影响其他资源                                         │   │
│  │   ✅ 保留非 Operator 管理的外部配置                                   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 实现状态

| 资源类型 | 实现状态 | 备注 |
|----------|----------|------|
| Tunnel + Ingress + TunnelBinding + Gateway | ✅ 已完成 | 原始聚合实现 |
| DeviceSettingsPolicy | ✅ 已完成 | Split Tunnel 条目聚合 |
| CloudflareDomain | ✅ 已完成 | Zone 设置聚合 |
| GatewayConfiguration | ✅ 已完成 | 网关设置聚合 |
| ZoneRuleset | ✅ 已完成 | 规则聚合，支持多个同 Phase 资源 |
| TransformRule | ✅ 已完成 | Transform 规则聚合 |
| RedirectRule | ✅ 已完成 | Redirect 规则聚合 |
| AccessGroup | ✅ 已完成 | 标准六层架构 |
| AccessIdentityProvider | ✅ 已完成 | 标准六层架构 |

---

## 架构图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         资源冲突解决架构 (统一聚合模式)                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐     │
│  │ Resource A  │   │ Resource B  │   │ Resource C  │   │ Resource D  │     │
│  │ (K8s CRD)   │   │ (K8s CRD)   │   │ (K8s CRD)   │   │ (K8s CRD)   │     │
│  └──────┬──────┘   └──────┬──────┘   └──────┬──────┘   └──────┬──────┘     │
│         │                 │                 │                 │             │
│         ▼                 ▼                 ▼                 ▼             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    L2: Resource Controllers                          │   │
│  │  ┌─────────────────────────────────────────────────────────────┐    │   │
│  │  │ 1. 验证 Spec                                                 │    │   │
│  │  │ 2. 解析引用                                                  │    │   │
│  │  │ 3. 构建配置                                                  │    │   │
│  │  │ 4. 调用 Core Service.Register()                              │    │   │
│  │  └─────────────────────────────────────────────────────────────┘    │   │
│  └──────────────────────────────┬──────────────────────────────────────┘   │
│                                 │                                           │
│                                 ▼                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    L3: Core Services                                 │   │
│  │  ┌─────────────────────────────────────────────────────────────┐    │   │
│  │  │ BaseService.UpdateSource()                                   │    │   │
│  │  │ ├─ 获取或创建 SyncState                                       │    │   │
│  │  │ ├─ 添加/更新 sources[] 条目                                   │    │   │
│  │  │ └─ 乐观锁冲突重试                                             │    │   │
│  │  └─────────────────────────────────────────────────────────────┘    │   │
│  └──────────────────────────────┬──────────────────────────────────────┘   │
│                                 │                                           │
│                                 ▼                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    L4: CloudflareSyncState CRD                       │   │
│  │  ┌─────────────────────────────────────────────────────────────┐    │   │
│  │  │ spec:                                                        │    │   │
│  │  │   sources:                                                   │    │   │
│  │  │   - ref:                                                     │    │   │
│  │  │       kind: DeviceSettingsPolicy                             │    │   │
│  │  │       namespace: default                                     │    │   │
│  │  │       name: policy-a                                         │    │   │
│  │  │     priority: 100                                            │    │   │
│  │  │     config: {...}                                            │    │   │
│  │  │   - ref:                                                     │    │   │
│  │  │       kind: DeviceSettingsPolicy                             │    │   │
│  │  │       namespace: production                                  │    │   │
│  │  │       name: policy-b                                         │    │   │
│  │  │     priority: 200                                            │    │   │
│  │  │     config: {...}                                            │    │   │
│  │  └─────────────────────────────────────────────────────────────┘    │   │
│  └──────────────────────────────┬──────────────────────────────────────┘   │
│                                 │                                           │
│                                 ▼                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    L5: Sync Controllers                              │   │
│  │  ┌─────────────────────────────────────────────────────────────┐    │   │
│  │  │ 统一聚合流程:                                                 │    │   │
│  │  │ 1. Watch SyncState 变化                                      │    │   │
│  │  │ 2. aggregateAllSources() - 聚合所有来源配置                   │    │   │
│  │  │ 3. 添加所有权标记 [managed-by:Kind/Namespace/Name]           │    │   │
│  │  │ 4. 计算 Hash 检测变化                                        │    │   │
│  │  │ 5. 调用 Cloudflare API (唯一调用点)                          │    │   │
│  │  │ 6. 更新 SyncState Status                                     │    │   │
│  │  │                                                              │    │   │
│  │  │ 统一删除流程:                                                 │    │   │
│  │  │ 1. 重新聚合剩余 sources                                       │    │   │
│  │  │ 2. 获取 Cloudflare 现有配置                                   │    │   │
│  │  │ 3. filterExternalRules() - 过滤外部规则                       │    │   │
│  │  │ 4. 合并剩余配置 + 外部配置                                    │    │   │
│  │  │ 5. 同步到 Cloudflare                                         │    │   │
│  │  │ 6. 清理 SyncState                                            │    │   │
│  │  └─────────────────────────────────────────────────────────────┘    │   │
│  └──────────────────────────────┬──────────────────────────────────────┘   │
│                                 │                                           │
│                                 ▼                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    L6: Cloudflare API                                │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 核心实现

### 通用聚合框架

位置: `internal/sync/common/aggregator.go`

```go
// OwnershipMarker 定义资源所有权标记格式
// 格式: "managed-by:Kind/Namespace/Name"
type OwnershipMarker struct {
    Kind      string
    Namespace string
    Name      string
}

// 核心函数

// NewOwnershipMarker 从 SourceReference 创建标记
func NewOwnershipMarker(ref v1alpha2.SourceReference) OwnershipMarker

// AppendToDescription 在描述中附加所有权标记
func (m OwnershipMarker) AppendToDescription(description string) string

// IsManagedByOperator 检查是否由 Operator 管理
func IsManagedByOperator(description string) bool
```

### Sync Controller 标准实现

每个 Sync Controller 都实现以下统一模式:

```go
// 1. 聚合类型定义
type AggregatedConfig struct {
    // 聚合后的配置
    Rules       []RuleWithOwner
    SourceCount int
}

type RuleWithOwner struct {
    Rule  ConfigType
    Owner v1alpha2.SourceReference
}

// 2. 聚合函数
func (r *Controller) aggregateAllSources(syncState *v1alpha2.CloudflareSyncState) (*AggregatedConfig, error) {
    // 按优先级排序 sources
    sources := make([]v1alpha2.ConfigSource, len(syncState.Spec.Sources))
    copy(sources, syncState.Spec.Sources)
    sort.Slice(sources, func(i, j int) bool {
        return sources[i].Priority < sources[j].Priority
    })

    // 聚合所有来源的配置
    for _, source := range sources {
        // 解析配置并添加所有权信息
        for _, item := range config.Items {
            result.Rules = append(result.Rules, RuleWithOwner{
                Rule:  item,
                Owner: source.Ref,
            })
        }
    }
    return result, nil
}

// 3. 转换函数 (添加所有权标记)
func (r *Controller) convertAggregatedRules(aggregated *AggregatedConfig) []CloudflareRule {
    for _, ruleWithOwner := range aggregated.Rules {
        marker := common.NewOwnershipMarker(ruleWithOwner.Owner)
        description := marker.AppendToDescription(ruleWithOwner.Rule.Name)
        // 创建 Cloudflare 规则
    }
}

// 4. 删除处理 (统一模式)
func (r *Controller) handleDeletion(ctx context.Context, syncState *v1alpha2.CloudflareSyncState) (ctrl.Result, error) {
    // 重新聚合剩余来源
    aggregated, _ := r.aggregateAllSources(syncState)

    // 获取 Cloudflare 现有配置
    existing, _ := apiClient.GetRules(zoneID, phase)

    // 过滤外部规则 (非 Operator 管理)
    externalRules := r.filterExternalRules(existing.Rules)

    // 转换剩余配置
    remainingRules := r.convertAggregatedRules(aggregated)

    // 合并并同步
    finalRules := append(remainingRules, externalRules...)
    apiClient.UpdateRules(zoneID, phase, finalRules)

    // 清理 SyncState
    return r.cleanupSyncState(ctx, syncState)
}

// 5. 外部规则过滤
func (r *Controller) filterExternalRules(rules []CloudflareRule) []CloudflareRule {
    external := make([]CloudflareRule, 0)
    for _, rule := range rules {
        if !common.IsManagedByOperator(rule.Description) {
            external = append(external, rule)
        }
    }
    return external
}
```

---

## 所有权标记格式

### 标记格式

```
[managed-by:Kind/Namespace/Name]
```

### 示例

```
User redirect rule [managed-by:RedirectRule/default/my-redirect]
```

### 标记位置

| 资源类型 | 标记字段 |
|----------|----------|
| ZoneRuleset | rule.description |
| TransformRule | rule.description |
| RedirectRule | rule.description |
| DeviceSettingsPolicy | entry.description |
| DNSRecord | record.comment |
| VirtualNetwork | network.comment |

---

## 优先级机制

### 优先级定义

```go
const (
    PriorityHighest = 10   // 系统级配置
    PriorityHigh    = 50   // 管理员配置
    PriorityDefault = 100  // 默认优先级
    PriorityLow     = 200  // 低优先级
)
```

### 优先级规则

1. **数字越小优先级越高**
2. **高优先级配置先处理**
3. **冲突时高优先级获胜**
4. **同优先级按创建顺序**

### 示例

```yaml
# 高优先级 Tunnel 设置
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: Tunnel
metadata:
  name: primary-tunnel
spec:
  # ...
  priority: 10  # 高优先级

# 低优先级 Ingress 规则
# Ingress 默认使用 priority: 100
```

---

## 测试验证场景

### 场景 1: 多个 DeviceSettingsPolicy 互不干扰

```yaml
# 创建两个 DeviceSettingsPolicy
---
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: DeviceSettingsPolicy
metadata:
  name: aws-vpc-proxy
spec:
  splitTunnelInclude:
  - address: "10.0.0.0/8"
    description: "AWS VPC"
---
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: DeviceSettingsPolicy
metadata:
  name: office-network
spec:
  splitTunnelInclude:
  - address: "192.168.1.0/24"
    description: "Office Network"
```

**预期行为**:
1. 创建后 Cloudflare Split Tunnel Include 包含两个条目
2. 删除 `office-network` 后，`10.0.0.0/8` 仍然存在
3. 外部添加的条目不受影响

### 场景 2: 多个 ZoneRuleset 同一 Phase

```yaml
# 两个 ZoneRuleset 使用同一 phase
---
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: ZoneRuleset
metadata:
  name: waf-rules-team-a
spec:
  phase: http_request_firewall_custom
  rules:
  - expression: 'ip.src in {1.2.3.0/24}'
    action: block
---
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: ZoneRuleset
metadata:
  name: waf-rules-team-b
spec:
  phase: http_request_firewall_custom
  rules:
  - expression: 'ip.src in {5.6.7.0/24}'
    action: block
```

**预期行为**:
1. 两个 ZoneRuleset 的规则都被同步到 Cloudflare
2. 每条规则带有所有权标记
3. 删除 team-a 的规则不影响 team-b

### 场景 3: 保留外部配置

```bash
# 直接在 Cloudflare 控制台添加规则
# 规则描述: "Manual rule by admin"
```

**预期行为**:
1. Operator 管理的规则与外部规则共存
2. 删除 Operator 资源时保留外部规则
3. 外部规则不会被 Operator 修改

---

## 相关文件

### 核心实现

| 文件 | 说明 |
|------|------|
| `internal/sync/common/aggregator.go` | 通用聚合框架 |
| `internal/sync/device/settingspolicy_controller.go` | DeviceSettingsPolicy 实现 |
| `internal/sync/ruleset/zoneruleset_controller.go` | ZoneRuleset 实现 |
| `internal/sync/ruleset/transformrule_controller.go` | TransformRule 实现 |
| `internal/sync/ruleset/redirectrule_controller.go` | RedirectRule 实现 |

### 测试文件

| 文件 | 说明 |
|------|------|
| `internal/sync/common/aggregator_test.go` | 聚合框架单元测试 |
| `test/e2e/scenarios/state_consistency_test.go` | 状态一致性 E2E 测试 |

---

## 历史记录

### 设计演变

1. **v0.22.x**: 最初采用四种不同方案 (Type A-D)
2. **v0.23.x**: 统一为 Type A 聚合模式
   - 移除 Phase 所有权锁定 (Type C)
   - 移除引用计数方案 (Type D)
   - 所有资源使用相同的聚合+所有权标记模式

### 废弃的方案

以下方案已废弃，保留仅供历史参考:

- **Type B: 全局设置型** → 改用 Type A 聚合模式
- **Type C: Phase 独占型** → 改用 Type A 聚合模式，允许多资源共享 Phase
- **Type D: 引用计数** → 暂不实现，依赖 Kubernetes 本身的垃圾回收机制
