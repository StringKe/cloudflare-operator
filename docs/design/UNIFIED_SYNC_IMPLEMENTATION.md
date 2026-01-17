# 统一同步架构实施计划

## 文档信息

| 项目 | 内容 |
|------|------|
| 关联文档 | [UNIFIED_SYNC_ARCHITECTURE.md](./UNIFIED_SYNC_ARCHITECTURE.md) |
| 版本 | v1.0 |
| 状态 | 待实施 |
| 日期 | 2026-01-16 |

---

## 1. 实施概览

### 1.1 实施阶段

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           实施阶段总览                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Phase 1: 基础设施 (1-2 周)                                                  │
│  ├── 1.1 CloudflareSyncState CRD                                           │
│  ├── 1.2 Core Service 接口和基础实现                                        │
│  ├── 1.3 Sync Controller 基础框架                                          │
│  └── 1.4 通用工具库                                                         │
│                                                                             │
│  Phase 2: Tunnel 配置迁移 (2-3 周)                                          │
│  ├── 2.1 TunnelConfigService                                               │
│  ├── 2.2 TunnelConfigSyncController                                        │
│  ├── 2.3 重构 Tunnel/ClusterTunnel Controller                              │
│  ├── 2.4 重构 TunnelBinding Controller                                     │
│  ├── 2.5 重构 Ingress Controller                                           │
│  └── 2.6 重构 Gateway Controller                                           │
│                                                                             │
│  Phase 3: DNS 和 Network 迁移 (1-2 周)                                      │
│  ├── 3.1 DNSService + DNSSyncController                                    │
│  ├── 3.2 重构 DNSRecord Controller                                         │
│  ├── 3.3 NetworkService + NetworkSyncController                            │
│  └── 3.4 重构 VirtualNetwork/NetworkRoute Controllers                      │
│                                                                             │
│  Phase 4: Access 资源迁移 (1-2 周)                                          │
│  ├── 4.1 AccessService (含依赖管理)                                         │
│  ├── 4.2 AccessSyncController                                              │
│  └── 4.3 重构所有 Access Controllers                                       │
│                                                                             │
│  Phase 5: R2 和 Ruleset 迁移 (1 周)                                         │
│  ├── 5.1 R2Service + R2SyncController                                      │
│  ├── 5.2 RulesetService + RulesetSyncController                            │
│  └── 5.3 重构相关 Controllers                                              │
│                                                                             │
│  Phase 6: 其他资源 + 清理 (1 周)                                            │
│  ├── 6.1 迁移剩余资源                                                       │
│  ├── 6.2 删除旧代码                                                         │
│  ├── 6.3 性能优化                                                           │
│  └── 6.4 E2E 测试                                                          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 1.2 代码行数目标

| 组件类型 | 目标行数 | 说明 |
|----------|----------|------|
| Resource Controller | 100-150 行 | 验证 + 调用 Service |
| Core Service | 150-200 行 | 业务逻辑 + 写 SyncState |
| Sync Controller | 200-300 行 | 聚合 + 防抖 + 同步 |
| 工具类 | 50-100 行 | 单一职责 |

---

## 2. Phase 1: 基础设施

### 2.1 CloudflareSyncState CRD

#### 2.1.1 类型定义

**文件**: `api/v1alpha2/cloudflaresyncstate_types.go`

```go
package v1alpha2

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
)

// +kubebuilder:validation:Enum=TunnelConfiguration;DNSRecord;AccessApplication;AccessGroup;AccessServiceToken;AccessIdentityProvider;VirtualNetwork;NetworkRoute;R2Bucket;R2BucketDomain;R2BucketNotification;ZoneRuleset;TransformRule;RedirectRule;GatewayRule;GatewayList;GatewayConfiguration;OriginCACertificate;CloudflareDomain;DomainRegistration
type SyncResourceType string

const (
    SyncResourceTunnelConfiguration SyncResourceType = "TunnelConfiguration"
    SyncResourceDNSRecord           SyncResourceType = "DNSRecord"
    SyncResourceAccessApplication   SyncResourceType = "AccessApplication"
    SyncResourceAccessGroup         SyncResourceType = "AccessGroup"
    SyncResourceVirtualNetwork      SyncResourceType = "VirtualNetwork"
    SyncResourceNetworkRoute        SyncResourceType = "NetworkRoute"
    // ... 其他类型
)

// CloudflareSyncStateSpec 定义 CloudflareSyncState 的期望状态
type CloudflareSyncStateSpec struct {
    // ResourceType 是 Cloudflare 资源类型
    ResourceType SyncResourceType `json:"resourceType"`

    // CloudflareID 是 Cloudflare 资源 ID
    CloudflareID string `json:"cloudflareId"`

    // AccountID 是 Cloudflare Account ID
    AccountID string `json:"accountId"`

    // ZoneID 是 Cloudflare Zone ID（可选）
    // +optional
    ZoneID string `json:"zoneId,omitempty"`

    // CredentialsRef 引用凭证
    CredentialsRef CredentialsReference `json:"credentialsRef"`

    // Sources 包含各个 K8s 资源贡献的配置
    // +optional
    Sources []ConfigSource `json:"sources,omitempty"`
}

// ConfigSource 表示一个配置来源
type ConfigSource struct {
    // Ref 标识来源 K8s 资源
    Ref SourceReference `json:"ref"`

    // Config 包含该来源贡献的配置
    // +kubebuilder:pruning:PreserveUnknownFields
    Config runtime.RawExtension `json:"config"`

    // Priority 决定配置冲突时的优先级（数字越小优先级越高）
    // +kubebuilder:default=100
    Priority int `json:"priority,omitempty"`

    // LastUpdated 是最后更新时间
    LastUpdated metav1.Time `json:"lastUpdated,omitempty"`
}

// SourceReference 标识一个 K8s 资源
type SourceReference struct {
    // Kind 是资源类型
    Kind string `json:"kind"`

    // Namespace 是资源命名空间（集群级资源为空）
    // +optional
    Namespace string `json:"namespace,omitempty"`

    // Name 是资源名称
    Name string `json:"name"`
}

// String 返回来源引用的字符串表示
func (r SourceReference) String() string {
    if r.Namespace == "" {
        return r.Kind + "/" + r.Name
    }
    return r.Kind + "/" + r.Namespace + "/" + r.Name
}

// SyncStatus 表示同步状态
// +kubebuilder:validation:Enum=Pending;Syncing;Synced;Error
type SyncStatus string

const (
    SyncStatusPending SyncStatus = "Pending"
    SyncStatusSyncing SyncStatus = "Syncing"
    SyncStatusSynced  SyncStatus = "Synced"
    SyncStatusError   SyncStatus = "Error"
)

// CloudflareSyncStateStatus 定义 CloudflareSyncState 的观察状态
type CloudflareSyncStateStatus struct {
    // SyncStatus 表示当前同步状态
    SyncStatus SyncStatus `json:"syncStatus,omitempty"`

    // LastSyncTime 是最后成功同步的时间
    LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

    // ConfigVersion 是最后同步后的 Cloudflare 配置版本
    ConfigVersion int `json:"configVersion,omitempty"`

    // ConfigHash 是聚合配置的 Hash，用于变更检测
    ConfigHash string `json:"configHash,omitempty"`

    // AggregatedConfig 是最终合并的配置（用于调试）
    // +kubebuilder:pruning:PreserveUnknownFields
    // +optional
    AggregatedConfig *runtime.RawExtension `json:"aggregatedConfig,omitempty"`

    // Error 包含最后的错误信息
    // +optional
    Error string `json:"error,omitempty"`

    // Conditions 表示最新的观察状态
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cfss;syncstate
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.resourceType`
// +kubebuilder:printcolumn:name="CloudflareID",type=string,JSONPath=`.spec.cloudflareId`
// +kubebuilder:printcolumn:name="Sources",type=integer,JSONPath=`.spec.sources`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.syncStatus`
// +kubebuilder:printcolumn:name="Version",type=integer,JSONPath=`.status.configVersion`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// CloudflareSyncState 存储 Cloudflare 同步状态
type CloudflareSyncState struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   CloudflareSyncStateSpec   `json:"spec,omitempty"`
    Status CloudflareSyncStateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudflareSyncStateList 包含 CloudflareSyncState 列表
type CloudflareSyncStateList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []CloudflareSyncState `json:"items"`
}

func init() {
    SchemeBuilder.Register(&CloudflareSyncState{}, &CloudflareSyncStateList{})
}
```

#### 2.1.2 任务清单

- [ ] 创建 `api/v1alpha2/cloudflaresyncstate_types.go`
- [ ] 运行 `make manifests generate`
- [ ] 更新 `config/crd/kustomization.yaml` 添加新 CRD
- [ ] 创建单元测试 `api/v1alpha2/cloudflaresyncstate_types_test.go`

---

### 2.2 Core Service 接口

#### 2.2.1 通用接口

**文件**: `internal/service/interface.go`

```go
package service

import (
    "context"

    "github.com/your-org/cloudflare-operator/api/v1alpha2"
)

// Source 标识配置来源
type Source struct {
    Kind      string `json:"kind"`
    Namespace string `json:"namespace,omitempty"`
    Name      string `json:"name"`
}

// String 返回字符串表示
func (s Source) String() string {
    if s.Namespace == "" {
        return s.Kind + "/" + s.Name
    }
    return s.Kind + "/" + s.Namespace + "/" + s.Name
}

// ToReference 转换为 v1alpha2.SourceReference
func (s Source) ToReference() v1alpha2.SourceReference {
    return v1alpha2.SourceReference{
        Kind:      s.Kind,
        Namespace: s.Namespace,
        Name:      s.Name,
    }
}

// RegisterOptions 注册配置选项
type RegisterOptions struct {
    ResourceType   v1alpha2.SyncResourceType
    CloudflareID   string
    AccountID      string
    ZoneID         string
    Source         Source
    Config         interface{}
    Priority       int
    CredentialsRef v1alpha2.CredentialsReference
}

// UnregisterOptions 注销配置选项
type UnregisterOptions struct {
    ResourceType v1alpha2.SyncResourceType
    CloudflareID string
    Source       Source
}

// ConfigService 是所有配置服务的通用接口
type ConfigService interface {
    // Register 注册配置来源
    Register(ctx context.Context, opts RegisterOptions) error

    // Unregister 移除配置来源
    Unregister(ctx context.Context, opts UnregisterOptions) error
}
```

#### 2.2.2 基础 Service 实现

**文件**: `internal/service/base.go`

```go
package service

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"

    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/types"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"

    "github.com/your-org/cloudflare-operator/api/v1alpha2"
)

// BaseService 提供所有 Service 的通用功能
type BaseService struct {
    Client client.Client
}

// NewBaseService 创建新的 BaseService
func NewBaseService(c client.Client) *BaseService {
    return &BaseService{Client: c}
}

// GetOrCreateSyncState 获取或创建 CloudflareSyncState
func (s *BaseService) GetOrCreateSyncState(
    ctx context.Context,
    resourceType v1alpha2.SyncResourceType,
    cloudflareID, accountID, zoneID string,
    credRef v1alpha2.CredentialsReference,
) (*v1alpha2.CloudflareSyncState, error) {
    log := log.FromContext(ctx)
    name := SyncStateName(resourceType, cloudflareID)

    syncState := &v1alpha2.CloudflareSyncState{}
    err := s.Client.Get(ctx, types.NamespacedName{Name: name}, syncState)

    if err == nil {
        return syncState, nil
    }

    if !apierrors.IsNotFound(err) {
        return nil, fmt.Errorf("get syncstate: %w", err)
    }

    // 创建新的 SyncState
    log.Info("Creating new SyncState", "name", name, "resourceType", resourceType)

    syncState = &v1alpha2.CloudflareSyncState{
        ObjectMeta: metav1.ObjectMeta{
            Name: name,
            Labels: map[string]string{
                "cloudflare-operator.io/resource-type": string(resourceType),
                "cloudflare-operator.io/cloudflare-id": cloudflareID,
            },
        },
        Spec: v1alpha2.CloudflareSyncStateSpec{
            ResourceType:   resourceType,
            CloudflareID:   cloudflareID,
            AccountID:      accountID,
            ZoneID:         zoneID,
            CredentialsRef: credRef,
            Sources:        []v1alpha2.ConfigSource{},
        },
    }

    if err := s.Client.Create(ctx, syncState); err != nil {
        return nil, fmt.Errorf("create syncstate: %w", err)
    }

    return syncState, nil
}

// UpdateSource 更新或添加 SyncState 中的来源
func (s *BaseService) UpdateSource(
    ctx context.Context,
    syncState *v1alpha2.CloudflareSyncState,
    source Source,
    config interface{},
    priority int,
) error {
    log := log.FromContext(ctx)

    configJSON, err := json.Marshal(config)
    if err != nil {
        return fmt.Errorf("marshal config: %w", err)
    }

    sourceRef := source.ToReference()
    sourceStr := sourceRef.String()
    now := metav1.Now()

    // 查找并更新现有来源，或添加新来源
    found := false
    for i := range syncState.Spec.Sources {
        if syncState.Spec.Sources[i].Ref.String() == sourceStr {
            syncState.Spec.Sources[i].Config = runtime.RawExtension{Raw: configJSON}
            syncState.Spec.Sources[i].Priority = priority
            syncState.Spec.Sources[i].LastUpdated = now
            found = true
            log.V(1).Info("Updated existing source", "source", sourceStr)
            break
        }
    }

    if !found {
        syncState.Spec.Sources = append(syncState.Spec.Sources, v1alpha2.ConfigSource{
            Ref:         sourceRef,
            Config:      runtime.RawExtension{Raw: configJSON},
            Priority:    priority,
            LastUpdated: now,
        })
        log.V(1).Info("Added new source", "source", sourceStr)
    }

    return s.Client.Update(ctx, syncState)
}

// RemoveSource 从 SyncState 中移除来源
func (s *BaseService) RemoveSource(
    ctx context.Context,
    syncState *v1alpha2.CloudflareSyncState,
    source Source,
) error {
    log := log.FromContext(ctx)
    sourceStr := source.String()

    newSources := make([]v1alpha2.ConfigSource, 0, len(syncState.Spec.Sources))
    for _, src := range syncState.Spec.Sources {
        if src.Ref.String() != sourceStr {
            newSources = append(newSources, src)
        }
    }

    // 如果没有来源了，删除 SyncState
    if len(newSources) == 0 {
        log.Info("No sources left, deleting SyncState", "name", syncState.Name)
        return s.Client.Delete(ctx, syncState)
    }

    syncState.Spec.Sources = newSources
    log.V(1).Info("Removed source", "source", sourceStr, "remainingSources", len(newSources))

    return s.Client.Update(ctx, syncState)
}

// SyncStateName 生成 SyncState 的一致名称
func SyncStateName(resourceType v1alpha2.SyncResourceType, cloudflareID string) string {
    return fmt.Sprintf("%s-%s", toKebabCase(string(resourceType)), cloudflareID)
}

func toKebabCase(s string) string {
    var result strings.Builder
    for i, r := range s {
        if i > 0 && r >= 'A' && r <= 'Z' {
            result.WriteRune('-')
        }
        result.WriteRune(r)
    }
    return strings.ToLower(result.String())
}
```

#### 2.2.3 任务清单

- [ ] 创建 `internal/service/interface.go`
- [ ] 创建 `internal/service/base.go`
- [ ] 创建单元测试

---

### 2.3 Sync Controller 基础框架

#### 2.3.1 防抖器

**文件**: `internal/sync/common/debouncer.go`

```go
package common

import (
    "sync"
    "time"
)

// Debouncer 合并多个事件为单一操作
type Debouncer struct {
    delay   time.Duration
    pending map[string]*time.Timer
    mu      sync.Mutex
}

// NewDebouncer 创建新的 Debouncer
func NewDebouncer(delay time.Duration) *Debouncer {
    return &Debouncer{
        delay:   delay,
        pending: make(map[string]*time.Timer),
    }
}

// Debounce 安排函数在延迟后调用
// 如果在延迟前再次调用相同 key，定时器会重置
func (d *Debouncer) Debounce(key string, fn func()) {
    d.mu.Lock()
    defer d.mu.Unlock()

    // 取消现有定时器
    if timer, ok := d.pending[key]; ok {
        timer.Stop()
    }

    // 安排新定时器
    d.pending[key] = time.AfterFunc(d.delay, func() {
        d.mu.Lock()
        delete(d.pending, key)
        d.mu.Unlock()
        fn()
    })
}

// Cancel 取消待处理的防抖函数
func (d *Debouncer) Cancel(key string) {
    d.mu.Lock()
    defer d.mu.Unlock()

    if timer, ok := d.pending[key]; ok {
        timer.Stop()
        delete(d.pending, key)
    }
}

// Flush 立即执行所有待处理函数
func (d *Debouncer) Flush() {
    d.mu.Lock()
    pending := d.pending
    d.pending = make(map[string]*time.Timer)
    d.mu.Unlock()

    for _, timer := range pending {
        timer.Stop()
    }
}

// PendingCount 返回待处理的数量
func (d *Debouncer) PendingCount() int {
    d.mu.Lock()
    defer d.mu.Unlock()
    return len(d.pending)
}
```

#### 2.3.2 Hash 计算

**文件**: `internal/sync/common/hash.go`

```go
package common

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
)

// ComputeConfigHash 计算配置的 SHA256 Hash
func ComputeConfigHash(config interface{}) (string, error) {
    data, err := json.Marshal(config)
    if err != nil {
        return "", err
    }

    hash := sha256.Sum256(data)
    return hex.EncodeToString(hash[:]), nil
}

// ComputeSourcesHash 计算多个来源的 Hash
func ComputeSourcesHash(sources []interface{}) (string, error) {
    var all []byte
    for _, src := range sources {
        data, err := json.Marshal(src)
        if err != nil {
            return "", err
        }
        all = append(all, data...)
    }

    hash := sha256.Sum256(all)
    return hex.EncodeToString(hash[:]), nil
}
```

#### 2.3.3 基础 Sync Controller

**文件**: `internal/sync/common/base.go`

```go
package common

import (
    "context"
    "time"

    "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"

    "github.com/your-org/cloudflare-operator/api/v1alpha2"
)

const (
    DefaultDebounceDelay = 500 * time.Millisecond
)

// SyncResult 包含同步操作的结果
type SyncResult struct {
    ConfigVersion int
    ConfigHash    string
}

// BaseSyncController 提供 Sync Controller 的通用功能
type BaseSyncController struct {
    Client    client.Client
    Debouncer *Debouncer
}

// NewBaseSyncController 创建新的 BaseSyncController
func NewBaseSyncController(c client.Client) *BaseSyncController {
    return &BaseSyncController{
        Client:    c,
        Debouncer: NewDebouncer(DefaultDebounceDelay),
    }
}

// UpdateSyncStatus 更新 SyncState 状态
func (c *BaseSyncController) UpdateSyncStatus(
    ctx context.Context,
    syncState *v1alpha2.CloudflareSyncState,
    status v1alpha2.SyncStatus,
    result *SyncResult,
    err error,
) error {
    log := log.FromContext(ctx)

    syncState.Status.SyncStatus = status

    if result != nil {
        syncState.Status.ConfigVersion = result.ConfigVersion
        syncState.Status.ConfigHash = result.ConfigHash
        now := metav1.Now()
        syncState.Status.LastSyncTime = &now
    }

    if err != nil {
        syncState.Status.Error = err.Error()
        meta.SetStatusCondition(&syncState.Status.Conditions, metav1.Condition{
            Type:               "Ready",
            Status:             metav1.ConditionFalse,
            Reason:             "SyncFailed",
            Message:            err.Error(),
            LastTransitionTime: metav1.Now(),
        })
    } else if status == v1alpha2.SyncStatusSynced {
        syncState.Status.Error = ""
        meta.SetStatusCondition(&syncState.Status.Conditions, metav1.Condition{
            Type:               "Ready",
            Status:             metav1.ConditionTrue,
            Reason:             "Synced",
            Message:            "Configuration synced to Cloudflare",
            LastTransitionTime: metav1.Now(),
        })
    }

    if updateErr := c.Client.Status().Update(ctx, syncState); updateErr != nil {
        log.Error(updateErr, "Failed to update SyncState status")
        return updateErr
    }

    return nil
}

// ShouldSync 基于 Hash 检查是否需要同步
func (c *BaseSyncController) ShouldSync(syncState *v1alpha2.CloudflareSyncState, newHash string) bool {
    return syncState.Status.ConfigHash != newHash
}

// GetSyncState 获取 SyncState
func (c *BaseSyncController) GetSyncState(ctx context.Context, name string) (*v1alpha2.CloudflareSyncState, error) {
    syncState := &v1alpha2.CloudflareSyncState{}
    err := c.Client.Get(ctx, client.ObjectKey{Name: name}, syncState)
    return syncState, err
}
```

#### 2.3.4 任务清单

- [ ] 创建 `internal/sync/common/debouncer.go`
- [ ] 创建 `internal/sync/common/hash.go`
- [ ] 创建 `internal/sync/common/base.go`
- [ ] 创建单元测试

---

### 2.4 通用工具库

#### 2.4.1 Condition 工具

**文件**: `internal/pkg/conditions/conditions.go`

```go
package conditions

import (
    "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SetReady 设置 Ready=True 条件
func SetReady(conditions *[]metav1.Condition, reason, message string) {
    meta.SetStatusCondition(conditions, metav1.Condition{
        Type:               "Ready",
        Status:             metav1.ConditionTrue,
        Reason:             reason,
        Message:            message,
        LastTransitionTime: metav1.Now(),
    })
}

// SetNotReady 设置 Ready=False 条件
func SetNotReady(conditions *[]metav1.Condition, reason, message string) {
    meta.SetStatusCondition(conditions, metav1.Condition{
        Type:               "Ready",
        Status:             metav1.ConditionFalse,
        Reason:             reason,
        Message:            message,
        LastTransitionTime: metav1.Now(),
    })
}

// SetError 设置错误条件
func SetError(conditions *[]metav1.Condition, err error) {
    SetNotReady(conditions, "Error", err.Error())
}

// IsReady 检查 Ready 条件是否为 True
func IsReady(conditions []metav1.Condition) bool {
    cond := meta.FindStatusCondition(conditions, "Ready")
    return cond != nil && cond.Status == metav1.ConditionTrue
}
```

#### 2.4.2 Finalizer 工具

**文件**: `internal/pkg/finalizer/finalizer.go`

```go
package finalizer

import (
    "context"

    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Ensure 确保 finalizer 存在
func Ensure(ctx context.Context, c client.Client, obj client.Object, finalizer string) error {
    if controllerutil.ContainsFinalizer(obj, finalizer) {
        return nil
    }

    controllerutil.AddFinalizer(obj, finalizer)
    return c.Update(ctx, obj)
}

// Remove 移除 finalizer
func Remove(ctx context.Context, c client.Client, obj client.Object, finalizer string) error {
    if !controllerutil.ContainsFinalizer(obj, finalizer) {
        return nil
    }

    controllerutil.RemoveFinalizer(obj, finalizer)
    return c.Update(ctx, obj)
}

// IsBeingDeleted 检查对象是否正在被删除
func IsBeingDeleted(obj client.Object) bool {
    return !obj.GetDeletionTimestamp().IsZero()
}

// ShouldReconcileDeletion 检查是否应该处理删除
func ShouldReconcileDeletion(obj client.Object, finalizer string) bool {
    return IsBeingDeleted(obj) && controllerutil.ContainsFinalizer(obj, finalizer)
}
```

#### 2.4.3 任务清单

- [ ] 创建 `internal/pkg/conditions/conditions.go`
- [ ] 创建 `internal/pkg/finalizer/finalizer.go`
- [ ] 创建 `internal/pkg/events/recorder.go`
- [ ] 创建单元测试

---

## 3. Phase 2: Tunnel 配置迁移

### 3.1 TunnelConfigService

#### 3.1.1 类型定义

**文件**: `internal/service/tunnel/types.go`

```go
package tunnel

import "github.com/your-org/cloudflare-operator/internal/service"

// IngressRule 表示一条 Tunnel Ingress 规则
type IngressRule struct {
    Hostname      string               `json:"hostname,omitempty"`
    Path          string               `json:"path,omitempty"`
    Service       string               `json:"service"`
    OriginRequest *OriginRequestConfig `json:"originRequest,omitempty"`
}

// OriginRequestConfig 表示 Origin 请求配置
type OriginRequestConfig struct {
    ConnectTimeout         string `json:"connectTimeout,omitempty"`
    TLSTimeout             string `json:"tlsTimeout,omitempty"`
    TCPKeepAlive           string `json:"tcpKeepAlive,omitempty"`
    NoHappyEyeballs        bool   `json:"noHappyEyeballs,omitempty"`
    KeepAliveConnections   int    `json:"keepAliveConnections,omitempty"`
    KeepAliveTimeout       string `json:"keepAliveTimeout,omitempty"`
    HTTPHostHeader         string `json:"httpHostHeader,omitempty"`
    OriginServerName       string `json:"originServerName,omitempty"`
    CAPool                 string `json:"caPool,omitempty"`
    NoTLSVerify            bool   `json:"noTLSVerify,omitempty"`
    DisableChunkedEncoding bool   `json:"disableChunkedEncoding,omitempty"`
    BastionMode            bool   `json:"bastionMode,omitempty"`
    ProxyAddress           string `json:"proxyAddress,omitempty"`
    ProxyPort              int    `json:"proxyPort,omitempty"`
    ProxyType              string `json:"proxyType,omitempty"`
    HTTP2Origin            bool   `json:"http2Origin,omitempty"`
}

// TunnelSettings 表示 Tunnel 级别设置
type TunnelSettings struct {
    WarpRouting         *WarpRoutingConfig   `json:"warpRouting,omitempty"`
    FallbackTarget      string               `json:"fallbackTarget,omitempty"`
    GlobalOriginRequest *OriginRequestConfig `json:"globalOriginRequest,omitempty"`
}

// WarpRoutingConfig 表示 WARP 路由设置
type WarpRoutingConfig struct {
    Enabled bool `json:"enabled"`
}

// TunnelConfig 表示来自一个来源的完整配置
type TunnelConfig struct {
    Settings *TunnelSettings `json:"settings,omitempty"`
    Rules    []IngressRule   `json:"rules,omitempty"`
}

// RegisterSettingsOptions 注册 Tunnel 设置的选项
type RegisterSettingsOptions struct {
    TunnelID       string
    AccountID      string
    Source         service.Source
    Settings       TunnelSettings
    CredentialsRef interface{}
}

// RegisterRulesOptions 注册 Ingress 规则的选项
type RegisterRulesOptions struct {
    TunnelID       string
    AccountID      string
    Source         service.Source
    Rules          []IngressRule
    CredentialsRef interface{}
}
```

#### 3.1.2 Service 实现

**文件**: `internal/service/tunnel/service.go`

```go
package tunnel

import (
    "context"
    "fmt"

    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"

    "github.com/your-org/cloudflare-operator/api/v1alpha2"
    "github.com/your-org/cloudflare-operator/internal/service"
)

const (
    ResourceType    = v1alpha2.SyncResourceTunnelConfiguration
    PriorityTunnel  = 10  // Tunnel 设置最高优先级
    PriorityDefault = 100 // 规则默认优先级
)

// Service 处理 Tunnel 配置注册
type Service struct {
    *service.BaseService
}

// NewService 创建新的 TunnelConfigService
func NewService(c client.Client) *Service {
    return &Service{
        BaseService: service.NewBaseService(c),
    }
}

// RegisterSettings 注册 Tunnel 设置（来自 Tunnel/ClusterTunnel）
func (s *Service) RegisterSettings(ctx context.Context, opts RegisterSettingsOptions) error {
    log := log.FromContext(ctx).WithValues(
        "tunnelID", opts.TunnelID,
        "source", opts.Source.String(),
    )
    log.V(1).Info("Registering tunnel settings")

    credRef, ok := opts.CredentialsRef.(v1alpha2.CredentialsReference)
    if !ok {
        return fmt.Errorf("invalid credentials reference type")
    }

    syncState, err := s.GetOrCreateSyncState(
        ctx,
        ResourceType,
        opts.TunnelID,
        opts.AccountID,
        "",
        credRef,
    )
    if err != nil {
        return fmt.Errorf("get/create syncstate: %w", err)
    }

    config := TunnelConfig{
        Settings: &opts.Settings,
    }

    if err := s.UpdateSource(ctx, syncState, opts.Source, config, PriorityTunnel); err != nil {
        return fmt.Errorf("update source: %w", err)
    }

    log.Info("Tunnel settings registered")
    return nil
}

// RegisterRules 注册 Ingress 规则（来自 Ingress/TunnelBinding/Gateway）
func (s *Service) RegisterRules(ctx context.Context, opts RegisterRulesOptions) error {
    log := log.FromContext(ctx).WithValues(
        "tunnelID", opts.TunnelID,
        "source", opts.Source.String(),
        "ruleCount", len(opts.Rules),
    )
    log.V(1).Info("Registering ingress rules")

    credRef, ok := opts.CredentialsRef.(v1alpha2.CredentialsReference)
    if !ok {
        return fmt.Errorf("invalid credentials reference type")
    }

    syncState, err := s.GetOrCreateSyncState(
        ctx,
        ResourceType,
        opts.TunnelID,
        opts.AccountID,
        "",
        credRef,
    )
    if err != nil {
        return fmt.Errorf("get/create syncstate: %w", err)
    }

    config := TunnelConfig{
        Rules: opts.Rules,
    }

    if err := s.UpdateSource(ctx, syncState, opts.Source, config, PriorityDefault); err != nil {
        return fmt.Errorf("update source: %w", err)
    }

    log.Info("Ingress rules registered")
    return nil
}

// Unregister 从 Tunnel 配置中移除来源
func (s *Service) Unregister(ctx context.Context, tunnelID string, source service.Source) error {
    log := log.FromContext(ctx).WithValues(
        "tunnelID", tunnelID,
        "source", source.String(),
    )
    log.V(1).Info("Unregistering source")

    name := service.SyncStateName(ResourceType, tunnelID)
    syncState := &v1alpha2.CloudflareSyncState{}

    if err := s.Client.Get(ctx, client.ObjectKey{Name: name}, syncState); err != nil {
        if client.IgnoreNotFound(err) == nil {
            log.V(1).Info("SyncState not found, nothing to unregister")
            return nil
        }
        return fmt.Errorf("get syncstate: %w", err)
    }

    if err := s.RemoveSource(ctx, syncState, source); err != nil {
        return fmt.Errorf("remove source: %w", err)
    }

    log.Info("Source unregistered")
    return nil
}
```

#### 3.1.3 任务清单

- [ ] 创建 `internal/service/tunnel/types.go`
- [ ] 创建 `internal/service/tunnel/service.go`
- [ ] 创建单元测试

---

### 3.2 TunnelConfigSyncController

#### 3.2.1 聚合器

**文件**: `internal/sync/tunnel/aggregator.go`

```go
package tunnel

import (
    "encoding/json"
    "sort"

    "github.com/your-org/cloudflare-operator/api/v1alpha2"
    tunnelsvc "github.com/your-org/cloudflare-operator/internal/service/tunnel"
)

// AggregatedConfig 表示最终合并的 Tunnel 配置
type AggregatedConfig struct {
    WarpRouting   *tunnelsvc.WarpRoutingConfig   `json:"warp-routing,omitempty"`
    Ingress       []tunnelsvc.IngressRule        `json:"ingress"`
    OriginRequest *tunnelsvc.OriginRequestConfig `json:"originRequest,omitempty"`
}

// Aggregate 将所有来源合并为单一配置
func Aggregate(syncState *v1alpha2.CloudflareSyncState) (*AggregatedConfig, error) {
    result := &AggregatedConfig{
        Ingress: []tunnelsvc.IngressRule{},
    }

    // 按优先级排序（数字越小优先级越高）
    sources := make([]v1alpha2.ConfigSource, len(syncState.Spec.Sources))
    copy(sources, syncState.Spec.Sources)
    sort.Slice(sources, func(i, j int) bool {
        return sources[i].Priority < sources[j].Priority
    })

    var fallbackTarget string

    for _, source := range sources {
        var config tunnelsvc.TunnelConfig
        if err := json.Unmarshal(source.Config.Raw, &config); err != nil {
            continue // 跳过格式错误的配置
        }

        // 应用设置（优先级排序后，第一个生效）
        if config.Settings != nil {
            if result.WarpRouting == nil && config.Settings.WarpRouting != nil {
                result.WarpRouting = config.Settings.WarpRouting
            }
            if result.OriginRequest == nil && config.Settings.GlobalOriginRequest != nil {
                result.OriginRequest = config.Settings.GlobalOriginRequest
            }
            if fallbackTarget == "" && config.Settings.FallbackTarget != "" {
                fallbackTarget = config.Settings.FallbackTarget
            }
        }

        // 收集规则
        result.Ingress = append(result.Ingress, config.Rules...)
    }

    // 排序规则：更具体的路径优先
    sort.Slice(result.Ingress, func(i, j int) bool {
        // 有路径的规则优先于没有路径的
        if result.Ingress[i].Path != "" && result.Ingress[j].Path == "" {
            return true
        }
        if result.Ingress[i].Path == "" && result.Ingress[j].Path != "" {
            return false
        }
        // 更长的路径优先
        return len(result.Ingress[i].Path) > len(result.Ingress[j].Path)
    })

    // 添加 catch-all 规则到末尾
    if fallbackTarget == "" {
        fallbackTarget = "http_status:404"
    }
    result.Ingress = append(result.Ingress, tunnelsvc.IngressRule{
        Service: fallbackTarget,
    })

    return result, nil
}

// ExtractHostnames 从聚合配置中提取所有主机名
func ExtractHostnames(config *AggregatedConfig) []string {
    hostnames := make([]string, 0, len(config.Ingress))
    seen := make(map[string]bool)

    for _, rule := range config.Ingress {
        if rule.Hostname != "" && !seen[rule.Hostname] {
            hostnames = append(hostnames, rule.Hostname)
            seen[rule.Hostname] = true
        }
    }

    return hostnames
}
```

#### 3.2.2 Sync Controller

**文件**: `internal/sync/tunnel/controller.go`

```go
package tunnel

import (
    "context"
    "time"

    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"
    "sigs.k8s.io/controller-runtime/pkg/predicate"

    "github.com/your-org/cloudflare-operator/api/v1alpha2"
    "github.com/your-org/cloudflare-operator/internal/clients/cf"
    "github.com/your-org/cloudflare-operator/internal/sync/common"
)

// SyncController 处理 TunnelConfiguration 类型的 CloudflareSyncState
type SyncController struct {
    *common.BaseSyncController
    Scheme    *runtime.Scheme
    CFFactory cf.ClientFactory
}

// NewSyncController 创建新的 TunnelConfigSyncController
func NewSyncController(c client.Client, scheme *runtime.Scheme, factory cf.ClientFactory) *SyncController {
    return &SyncController{
        BaseSyncController: common.NewBaseSyncController(c),
        Scheme:             scheme,
        CFFactory:          factory,
    }
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=cloudflaresyncstates,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=cloudflaresyncstates/status,verbs=get;update;patch

func (r *SyncController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // 获取 SyncState
    syncState := &v1alpha2.CloudflareSyncState{}
    if err := r.Client.Get(ctx, req.NamespacedName, syncState); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 只处理 TunnelConfiguration 类型
    if syncState.Spec.ResourceType != v1alpha2.SyncResourceTunnelConfiguration {
        return ctrl.Result{}, nil
    }

    log.V(1).Info("Reconciling tunnel config sync",
        "tunnelID", syncState.Spec.CloudflareID,
        "sourceCount", len(syncState.Spec.Sources))

    // 聚合配置
    aggregatedConfig, err := Aggregate(syncState)
    if err != nil {
        log.Error(err, "Failed to aggregate config")
        _ = r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err)
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    // 计算 Hash
    hash, err := common.ComputeConfigHash(aggregatedConfig)
    if err != nil {
        log.Error(err, "Failed to compute config hash")
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    // 无变化则跳过
    if !r.ShouldSync(syncState, hash) {
        log.V(1).Info("Config unchanged, skipping sync")
        return ctrl.Result{}, nil
    }

    // 更新状态为 Syncing
    _ = r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusSyncing, nil, nil)

    // 获取 CF 客户端
    cfClient, err := r.CFFactory.GetClient(ctx, syncState.Spec.CredentialsRef)
    if err != nil {
        log.Error(err, "Failed to get Cloudflare client")
        _ = r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err)
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    // 同步到 Cloudflare
    version, err := cfClient.PutTunnelConfiguration(syncState.Spec.CloudflareID, aggregatedConfig)
    if err != nil {
        log.Error(err, "Failed to sync to Cloudflare")
        _ = r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusError, nil, err)
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    // 更新状态
    result := &common.SyncResult{
        ConfigVersion: version,
        ConfigHash:    hash,
    }
    if err := r.UpdateSyncStatus(ctx, syncState, v1alpha2.SyncStatusSynced, result, nil); err != nil {
        return ctrl.Result{}, err
    }

    log.Info("Tunnel config synced",
        "tunnelID", syncState.Spec.CloudflareID,
        "version", version,
        "hostnames", ExtractHostnames(aggregatedConfig))

    return ctrl.Result{}, nil
}

// SetupWithManager 设置 Controller 与 Manager
func (r *SyncController) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&v1alpha2.CloudflareSyncState{}).
        WithEventFilter(predicate.NewPredicateFuncs(func(obj client.Object) bool {
            syncState, ok := obj.(*v1alpha2.CloudflareSyncState)
            if !ok {
                return false
            }
            return syncState.Spec.ResourceType == v1alpha2.SyncResourceTunnelConfiguration
        })).
        Complete(r)
}
```

#### 3.2.3 任务清单

- [ ] 创建 `internal/sync/tunnel/aggregator.go`
- [ ] 创建 `internal/sync/tunnel/controller.go`
- [ ] 创建 `internal/sync/tunnel/setup.go`
- [ ] 创建单元测试
- [ ] 创建集成测试

---

### 3.3 重构 Ingress Controller

#### 3.3.1 Reconciler

**文件**: `internal/controller/ingress/reconciler.go`

```go
package ingress

import (
    "context"

    networkingv1 "k8s.io/api/networking/v1"
    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"

    "github.com/your-org/cloudflare-operator/api/v1alpha2"
    "github.com/your-org/cloudflare-operator/internal/pkg/finalizer"
    "github.com/your-org/cloudflare-operator/internal/service"
    tunnelsvc "github.com/your-org/cloudflare-operator/internal/service/tunnel"
)

const (
    FinalizerName = "ingress.cloudflare-operator.io/finalizer"
    IngressClass  = "cloudflare"
)

// Reconciler 处理 Ingress 资源
type Reconciler struct {
    client.Client
    Scheme        *runtime.Scheme
    TunnelService *tunnelsvc.Service
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // 获取 Ingress
    var ingress networkingv1.Ingress
    if err := r.Get(ctx, req.NamespacedName, &ingress); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 检查 IngressClass
    if !r.isCloudflareIngress(&ingress) {
        return ctrl.Result{}, nil
    }

    log.V(1).Info("Reconciling Ingress")

    // 处理删除
    if finalizer.IsBeingDeleted(&ingress) {
        return r.handleDeletion(ctx, &ingress)
    }

    // 确保 finalizer
    if err := finalizer.Ensure(ctx, r.Client, &ingress, FinalizerName); err != nil {
        return ctrl.Result{}, err
    }

    // 解析 Tunnel
    tunnelID, accountID, credRef, err := r.resolveTunnel(ctx, &ingress)
    if err != nil {
        log.Error(err, "Failed to resolve tunnel")
        return ctrl.Result{}, err
    }

    // 构建规则
    rules := BuildIngressRules(&ingress)

    // 注册到 Service
    source := service.Source{
        Kind:      "Ingress",
        Namespace: ingress.Namespace,
        Name:      ingress.Name,
    }

    opts := tunnelsvc.RegisterRulesOptions{
        TunnelID:       tunnelID,
        AccountID:      accountID,
        Source:         source,
        Rules:          rules,
        CredentialsRef: credRef,
    }

    if err := r.TunnelService.RegisterRules(ctx, opts); err != nil {
        log.Error(err, "Failed to register rules")
        return ctrl.Result{}, err
    }

    log.Info("Ingress reconciled", "ruleCount", len(rules))
    return ctrl.Result{}, nil
}

func (r *Reconciler) handleDeletion(ctx context.Context, ingress *networkingv1.Ingress) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    if !finalizer.ShouldReconcileDeletion(ingress, FinalizerName) {
        return ctrl.Result{}, nil
    }

    // 解析 Tunnel 用于注销
    tunnelID, _, _, err := r.resolveTunnel(ctx, ingress)
    if err == nil && tunnelID != "" {
        source := service.Source{
            Kind:      "Ingress",
            Namespace: ingress.Namespace,
            Name:      ingress.Name,
        }
        if err := r.TunnelService.Unregister(ctx, tunnelID, source); err != nil {
            log.Error(err, "Failed to unregister")
            return ctrl.Result{}, err
        }
    }

    // 移除 finalizer
    if err := finalizer.Remove(ctx, r.Client, ingress, FinalizerName); err != nil {
        return ctrl.Result{}, err
    }

    log.Info("Ingress deleted")
    return ctrl.Result{}, nil
}

func (r *Reconciler) isCloudflareIngress(ingress *networkingv1.Ingress) bool {
    if ingress.Spec.IngressClassName != nil {
        return *ingress.Spec.IngressClassName == IngressClass
    }
    return ingress.Annotations["kubernetes.io/ingress.class"] == IngressClass
}

func (r *Reconciler) resolveTunnel(ctx context.Context, ingress *networkingv1.Ingress) (
    tunnelID, accountID string,
    credRef v1alpha2.CredentialsReference,
    err error,
) {
    // TODO: 实现 Tunnel 解析逻辑
    // 1. 获取 IngressClass
    // 2. 获取 TunnelIngressClassConfig
    // 3. 解析 TunnelRef 获取 Tunnel/ClusterTunnel
    // 4. 返回 tunnelID, accountID, credentialsRef
    return
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&networkingv1.Ingress{}).
        Complete(r)
}
```

#### 3.3.2 Rule Builder

**文件**: `internal/controller/ingress/rule_builder.go`

```go
package ingress

import (
    "fmt"

    networkingv1 "k8s.io/api/networking/v1"

    tunnelsvc "github.com/your-org/cloudflare-operator/internal/service/tunnel"
)

// BuildIngressRules 将 Ingress 规则转换为 Tunnel IngressRules
func BuildIngressRules(ingress *networkingv1.Ingress) []tunnelsvc.IngressRule {
    var rules []tunnelsvc.IngressRule

    for _, rule := range ingress.Spec.Rules {
        if rule.HTTP == nil {
            continue
        }

        for _, path := range rule.HTTP.Paths {
            serviceTarget := buildServiceTarget(ingress.Namespace, &path.Backend)

            ingressRule := tunnelsvc.IngressRule{
                Hostname: rule.Host,
                Service:  serviceTarget,
            }

            if path.Path != "" && path.Path != "/" {
                ingressRule.Path = path.Path
            }

            rules = append(rules, ingressRule)
        }
    }

    return rules
}

func buildServiceTarget(namespace string, backend *networkingv1.IngressBackend) string {
    if backend.Service == nil {
        return ""
    }

    port := 80
    if backend.Service.Port.Number != 0 {
        port = int(backend.Service.Port.Number)
    }

    return fmt.Sprintf("http://%s.%s.svc:%d",
        backend.Service.Name,
        namespace,
        port,
    )
}
```

#### 3.3.3 任务清单

- [ ] 创建 `internal/controller/ingress/reconciler.go`
- [ ] 创建 `internal/controller/ingress/rule_builder.go`
- [ ] 创建 `internal/controller/ingress/class_resolver.go`
- [ ] 创建 `internal/controller/ingress/setup.go`
- [ ] 删除旧的 `internal/controller/ingress/controller.go`
- [ ] 创建单元测试

---

## 4. 检查清单

### Phase 1 完成标准

- [ ] CloudflareSyncState CRD 定义完成
- [ ] CRD YAML 生成并添加到 kustomization
- [ ] BaseService 实现完成
- [ ] Debouncer 实现完成
- [ ] Hash 工具实现完成
- [ ] BaseSyncController 实现完成
- [ ] 通用工具（conditions, finalizer）实现完成
- [ ] 单元测试覆盖率 > 80%
- [ ] `make manifests generate` 成功
- [ ] `make test` 通过
- [ ] `make lint` 通过

### Phase 2 完成标准

- [ ] TunnelConfigService 实现完成
- [ ] TunnelConfigSyncController 实现完成
- [ ] Aggregator 逻辑正确
- [ ] Tunnel Controller 重构完成
- [ ] ClusterTunnel Controller 重构完成
- [ ] TunnelBinding Controller 重构完成
- [ ] Ingress Controller 重构完成
- [ ] Gateway Controller 重构完成
- [ ] 集成测试通过
- [ ] E2E 测试通过（Tunnel 配置不再竞态）

---

## 5. 命令参考

```bash
# 生成 CRD 和 DeepCopy
make manifests generate

# 格式化和检查
make fmt vet

# 运行测试
make test

# 运行 lint
make lint

# 构建
make build

# 本地运行（调试）
make run

# 部署到集群
make deploy

# 查看 SyncState
kubectl get cloudflaresyncstates -o wide
kubectl get cfss  # 短名

# 查看特定 SyncState 详情
kubectl describe cfss tunnel-configuration-abc123

# 查看 SyncState 的来源
kubectl get cfss tunnel-configuration-abc123 -o jsonpath='{.spec.sources[*].ref}'
```

---

## 6. 迁移策略

### 6.1 渐进式迁移

1. **Phase 1-2**: 新旧代码并存，通过 Feature Flag 控制
2. **Phase 3-5**: 逐步迁移其他资源
3. **Phase 6**: 删除旧代码，完成迁移

### 6.2 Feature Flag

```go
// 在 main.go 中
var useNewSyncArchitecture = os.Getenv("USE_NEW_SYNC_ARCHITECTURE") == "true"

if useNewSyncArchitecture {
    // 使用新架构
    if err := tunnelSync.NewSyncController(...).SetupWithManager(mgr); err != nil {
        // ...
    }
} else {
    // 使用旧架构
    if err := controller.NewTunnelReconciler(...).SetupWithManager(mgr); err != nil {
        // ...
    }
}
```

### 6.3 回滚计划

1. 保留旧 Controller 代码直到新架构稳定
2. Feature Flag 可以快速回滚到旧实现
3. SyncState CRD 不影响现有用户资源

---

## 7. 风险和缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| SyncState CRD 设计不当 | 后续修改困难 | 充分设计评审，预留扩展字段 |
| 聚合逻辑复杂 | 规则丢失或冲突 | 详细单元测试，保留 aggregatedConfig 用于调试 |
| 迁移期间服务中断 | 用户影响 | 渐进式迁移，Feature Flag 控制 |
| 性能问题 | 同步延迟 | 防抖优化，增量检测，Metrics 监控 |
| 多实例竞态 | 配置不一致 | K8s 乐观锁，Leader Election |
