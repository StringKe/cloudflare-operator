# 完整六层架构统一迁移计划

## 执行摘要

本文档详细描述了 Cloudflare Operator 完整六层架构统一迁移计划，目标是消除所有"例外"、"混合"、"待迁移"状态，使所有 30 个 CRD 100% 遵循六层架构。

## 六层架构回顾

```
K8s Resources → Resource Controllers → Core Services → SyncState CRD → Sync Controllers → Cloudflare API
     L1              L2                    L3              L4              L5               L6
```

**核心原则**：
- L2 Resource Controllers **禁止**直接调用 Cloudflare API
- L5 Sync Controllers 是**唯一**调用 Cloudflare API 的地方
- 所有配置通过 L4 SyncState 中转

---

## 问题清单与解决方案

### P0 - 阻塞业务 (立即修复)

#### 1. AccessApplication policies 不同步

**问题描述**：
- CRD 定义了 `policies` 字段
- Resource Controller 正确解析并传递 policies 到 Service
- **Sync Controller 的 `buildParams()` 完全忽略了 policies**
- Cloudflare Access Policy 是独立的 API 端点，需要单独调用

**影响**：
- 创建的 Application 没有访问策略
- 用户无法登录受保护的应用

**解决方案**：

```go
// internal/sync/access/application_controller.go

func (r *ApplicationController) syncToCloudflare(
    ctx context.Context,
    syncState *v1alpha2.CloudflareSyncState,
    config *accesssvc.AccessApplicationConfig,
) (*accesssvc.SyncResult, error) {
    // ... 现有代码：创建/更新 Application ...

    // ✅ 新增：同步 Policies
    if err := r.syncPolicies(ctx, apiClient, result.ID, config.Policies); err != nil {
        logger.Error(err, "Failed to sync policies")
        // 不阻塞 Application 同步，但记录错误
    }

    return result, nil
}

// syncPolicies 同步 Access Policies
func (r *ApplicationController) syncPolicies(
    ctx context.Context,
    apiClient *cf.API,
    applicationID string,
    desiredPolicies []accesssvc.AccessPolicyConfig,
) error {
    logger := log.FromContext(ctx)

    // 1. 获取现有 policies
    existingPolicies, err := apiClient.ListAccessPolicies(applicationID)
    if err != nil {
        return fmt.Errorf("list existing policies: %w", err)
    }

    // 2. 构建映射：precedence -> existing policy
    existingByPrecedence := make(map[int]*cf.AccessPolicyResult)
    for i := range existingPolicies {
        existingByPrecedence[existingPolicies[i].Precedence] = &existingPolicies[i]
    }

    // 3. 处理期望的 policies
    desiredPrecedences := make(map[int]bool)
    for _, desired := range desiredPolicies {
        desiredPrecedences[desired.Precedence] = true

        params := cf.AccessPolicyParams{
            ApplicationID:   applicationID,
            Name:            r.getPolicyName(desired),
            Decision:        desired.Decision,
            Precedence:      desired.Precedence,
            SessionDuration: desired.SessionDuration,
            Include: []interface{}{
                map[string]interface{}{"group": map[string]string{"id": desired.GroupID}},
            },
        }

        if existing, ok := existingByPrecedence[desired.Precedence]; ok {
            // 更新现有 policy
            if _, err := apiClient.UpdateAccessPolicy(existing.ID, params); err != nil {
                logger.Error(err, "Failed to update policy", "precedence", desired.Precedence)
            }
        } else {
            // 创建新 policy
            if _, err := apiClient.CreateAccessPolicy(params); err != nil {
                logger.Error(err, "Failed to create policy", "precedence", desired.Precedence)
            }
        }
    }

    // 4. 删除多余的 policies
    for precedence, existing := range existingByPrecedence {
        if !desiredPrecedences[precedence] {
            if err := apiClient.DeleteAccessPolicy(applicationID, existing.ID); err != nil {
                logger.Error(err, "Failed to delete policy", "policyId", existing.ID)
            }
        }
    }

    return nil
}
```

**文件变更**：
| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/sync/access/application_controller.go` | 修改 | 添加 `syncPolicies()` 方法 |
| `internal/clients/cf/access.go` | 可能修改 | 确保 `AccessPolicyParams.Include` 支持 Group 规则 |

---

#### 2. NetworkRoute 幂等性问题

**问题描述**：
- 创建 route 时直接调用 `CreateTunnelRoute()`
- 没有检查 route 是否已存在
- 没有处理 "route already exists" 错误
- 没有采用 (adoption) 逻辑

**影响**：
- 手动创建的 route 无法被 Operator 管理
- 多实例并发创建会失败
- 报错 `error: You already have a route defined for this exact IP subnet (1014)`

**解决方案**：

```go
// internal/sync/networkroute/controller.go

func (r *NetworkRouteController) syncToCloudflare(
    ctx context.Context,
    syncState *v1alpha2.CloudflareSyncState,
    config *networkroutesvc.NetworkRouteConfig,
) (*cf.TunnelRouteResult, error) {
    logger := log.FromContext(ctx)

    isPending := common.IsPendingID(syncState.Spec.CloudflareID)

    if isPending {
        // ✅ 新增：先检查 route 是否已存在（采用逻辑）
        existing, err := apiClient.GetTunnelRoute(config.Network, config.VirtualNetworkID)
        if err == nil && existing != nil {
            logger.Info("Found existing tunnel route, adopting",
                "network", config.Network)
            // 更新 SyncState CloudflareID
            common.UpdateCloudflareID(ctx, r.Client, syncState, config.Network)
            return existing, nil
        }

        // 创建新 route
        result, err := apiClient.CreateTunnelRoute(params)
        if err != nil {
            // ✅ 新增：处理 "already exists" 错误
            if cf.IsConflictError(err) {
                logger.Info("Route creation conflict, attempting adoption",
                    "network", config.Network)
                existing, adoptErr := apiClient.GetTunnelRoute(config.Network, config.VirtualNetworkID)
                if adoptErr == nil && existing != nil {
                    common.UpdateCloudflareID(ctx, r.Client, syncState, config.Network)
                    return existing, nil
                }
                return nil, fmt.Errorf("create route conflict and adoption failed: %w (adoption: %v)", err, adoptErr)
            }
            return nil, fmt.Errorf("create NetworkRoute: %w", err)
        }

        return result, nil
    }

    // ... 现有更新逻辑 ...
}
```

**文件变更**：
| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/sync/networkroute/controller.go` | 修改 | 添加采用逻辑和冲突处理 |

---

### P1 - 架构统一 (核心迁移)

#### 3. Tunnel/ClusterTunnel 迁移

**当前问题**：
- Resource Controller 持有 `cfAPI` 字段
- 直接调用 `CreateTunnel()`, `DeleteTunnel()`, `GetTunnelToken()` 等
- `cleanupTunnel()` 删除逻辑直接调用 API

**迁移方案**：

**阶段 1：提取 Tunnel 生命周期到 Service**

```go
// internal/service/tunnel/lifecycle_service.go (新文件)

type LifecycleService struct {
    *service.BaseService
}

// RegisterTunnel 注册 Tunnel 到 SyncState
func (s *LifecycleService) RegisterTunnel(ctx context.Context, opts TunnelRegisterOptions) error {
    // 使用 "pending-{name}" ID 直到 Tunnel 创建
    syncStateID := fmt.Sprintf("pending-%s", opts.Source.Name)
    if opts.TunnelID != "" {
        syncStateID = opts.TunnelID
    }

    syncState, err := s.GetOrCreateSyncState(ctx, ResourceTypeTunnel, syncStateID, ...)
    // ...
}

// UnregisterTunnel 从 SyncState 注销
func (s *LifecycleService) UnregisterTunnel(ctx context.Context, tunnelID string, source service.Source) error {
    // ...
}
```

**阶段 2：Resource Controller 迁移**

```go
// internal/controller/tunnel_controller.go

type Reconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder
    // ❌ 移除: cfAPI *cf.API
    // ✅ 新增:
    tunnelService   *tunnelsvc.LifecycleService
    tunnelConfigSvc *tunnelsvc.Service
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ... 获取资源 ...

    if tunnel.DeletionTimestamp != nil {
        // ✅ 通过 Service 注销
        return r.handleDeletion(ctx, tunnel)
    }

    // ✅ 通过 Service 注册
    if err := r.tunnelService.RegisterTunnel(ctx, opts); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}
```

**阶段 3：Sync Controller 增强**

```go
// internal/sync/tunnel/lifecycle_controller.go (新文件)

type LifecycleSyncController struct {
    *common.BaseSyncController
}

func (r *LifecycleSyncController) syncToCloudflare(
    ctx context.Context,
    syncState *v1alpha2.CloudflareSyncState,
    config *tunnelsvc.TunnelConfig,
) (*cf.Tunnel, error) {
    if common.IsPendingID(syncState.Spec.CloudflareID) {
        // 创建 Tunnel
        tunnel, creds, err := apiClient.CreateTunnel()
        if err != nil {
            // 处理 "already exists" 错误
            if cf.IsConflictError(err) {
                existing, _ := apiClient.GetTunnelId()
                if existing != "" {
                    return r.adoptExistingTunnel(ctx, existing)
                }
            }
            return nil, err
        }

        // 存储 credentials 到 Secret
        if err := r.storeTunnelCredentials(ctx, config, creds); err != nil {
            // 回滚：删除刚创建的 Tunnel
            apiClient.DeleteTunnel()
            return nil, err
        }

        return tunnel, nil
    }

    // 更新现有 Tunnel（如果需要）
    return apiClient.GetTunnel(syncState.Spec.CloudflareID)
}

func (r *LifecycleSyncController) handleDeletion(
    ctx context.Context,
    syncState *v1alpha2.CloudflareSyncState,
) (ctrl.Result, error) {
    // 1. 删除关联的 routes
    apiClient.DeleteTunnelRoutesByTunnelID(tunnelID)

    // 2. 删除 Tunnel
    apiClient.DeleteTunnel()

    // 3. 移除 Finalizer
    return ctrl.Result{}, nil
}
```

**特殊处理：GetTunnelToken**

```go
// GetTunnelToken 需要在 Tunnel 创建后立即获取
// 保留在 Service 中作为辅助方法

// internal/service/tunnel/lifecycle_service.go
func (s *LifecycleService) GetTunnelToken(ctx context.Context, tunnelID string, credRef v1alpha2.CredentialsReference) (string, error) {
    apiClient, err := s.createTemporaryAPIClient(ctx, credRef)
    if err != nil {
        return "", err
    }
    return apiClient.GetTunnelToken(tunnelID)
}
```

**文件变更**：
| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/service/tunnel/lifecycle_service.go` | 新建 | Tunnel 生命周期服务 |
| `internal/sync/tunnel/lifecycle_controller.go` | 新建 | Tunnel 生命周期同步控制器 |
| `internal/controller/tunnel_controller.go` | 修改 | 移除 cfAPI，使用 Service |
| `internal/controller/clustertunnel_controller.go` | 修改 | 同上 |
| `internal/controller/generic_tunnel_reconciler.go` | 修改 | 大幅重构 |

---

#### 4. Ingress/Gateway 清理

**当前状态**：已大部分迁移，需验证和清理

**验证清单**：
- [ ] 确认 `syncTunnelConfigToAPI()` 仅通过 Service 操作
- [ ] 确认 DNS 处理部分不直接调用 API
- [ ] 移除所有临时 API 客户端创建

---

#### 5. CloudflareDomain 补充 L5 Sync Controller

**当前问题**：
- L3 Service 已存在但未使用
- Resource Controller 直接使用 `cloudflare.API`（非 operator 的 `cf.API`）
- 缺少 L5 Sync Controller

**解决方案**：

```go
// internal/sync/domain/cloudflaredomain_controller.go (新文件)

type CloudflareDomainSyncController struct {
    *common.BaseSyncController
}

func (r *CloudflareDomainSyncController) sync(
    ctx context.Context,
    syncState *v1alpha2.CloudflareSyncState,
) error {
    // 1. 如果是 "pending-*"，先验证 Zone
    if strings.HasPrefix(syncState.Spec.CloudflareID, "pending-") {
        return r.verifyZone(ctx, syncState)
    }

    // 2. 否则，同步 Settings
    return r.syncSettings(ctx, syncState)
}

func (r *CloudflareDomainSyncController) verifyZone(
    ctx context.Context,
    syncState *v1alpha2.CloudflareSyncState,
) error {
    config := r.extractConfig(syncState)

    // 查询 Zone ID
    zones, err := r.cfAPI.ListZones(config.Domain)
    if err != nil || len(zones) == 0 {
        return fmt.Errorf("zone verification failed: %w", err)
    }

    zoneID := zones[0].ID

    // 迁移到真实的 Zone ID SyncState
    return r.service.UpdateZoneID(ctx, syncState.Spec.Sources[0], zoneID)
}

func (r *CloudflareDomainSyncController) syncSettings(
    ctx context.Context,
    syncState *v1alpha2.CloudflareSyncState,
) error {
    config := r.extractConfig(syncState)
    zoneID := syncState.Spec.CloudflareID

    // 同步各种 Settings
    var errs []error

    if config.SSL != nil {
        if err := r.cfAPI.UpdateZoneSSLSettings(zoneID, config.SSL); err != nil {
            errs = append(errs, err)
        }
    }

    if config.Cache != nil {
        if err := r.cfAPI.UpdateZoneCacheSettings(zoneID, config.Cache); err != nil {
            errs = append(errs, err)
        }
    }

    // ... Security, Performance ...

    return errors.Join(errs...)
}
```

**文件变更**：
| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/sync/domain/cloudflaredomain_controller.go` | 新建 | CloudflareDomain Sync Controller |
| `internal/controller/cloudflaredomain/controller.go` | 修改 | 移除直接 API 调用，使用 Service |
| `internal/clients/cf/zone.go` | 可能新建 | Zone Settings API 封装 |

---

### P2 - 例外资源迁移

#### 6. OriginCACertificate 迁移

**特殊性**：
- 需要生成 CSR 和私钥
- 证书 ID 在签发后才能确定
- 需要同步证书到 K8s Secret

**迁移方案**：

```go
// internal/sync/domain/origincacertificate_controller.go (新文件)

type OriginCACertificateSyncController struct {
    *common.BaseSyncController
}

func (r *OriginCACertificateSyncController) sync(
    ctx context.Context,
    syncState *v1alpha2.CloudflareSyncState,
) error {
    config := r.extractConfig(syncState)

    if common.IsPendingID(syncState.Spec.CloudflareID) {
        // 签发新证书
        cert, err := r.cfAPI.CreateOriginCACertificate(cf.OriginCACertificateParams{
            Hostnames:       config.Hostnames,
            RequestType:     config.RequestType,
            RequestValidity: config.ValidityDays,
            CSR:             config.CSR,
        })
        if err != nil {
            return err
        }

        // 更新 SyncState ID
        common.UpdateCloudflareID(ctx, r.Client, syncState, cert.ID)

        // 通知 Resource Controller 更新 Secret
        // 通过 SyncState Status 传递证书内容
        syncState.Status.Metadata = map[string]string{
            "certificate": cert.Certificate,
        }

        return nil
    }

    // 检查续期
    // ...
}
```

**关键点**：
- CSR 生成保留在 Resource Controller（这是合理的业务逻辑）
- 证书签发移至 Sync Controller
- 通过 SyncState Status 传递证书内容给 Resource Controller 更新 Secret

**文件变更**：
| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/sync/domain/origincacertificate_controller.go` | 新建 | 证书 Sync Controller |
| `internal/controller/origincacertificate/controller.go` | 修改 | 移除 API 调用，使用 Service |

---

#### 7. DomainRegistration 迁移

**特殊性**：
- 读多写少（主要是查询状态）
- Enterprise 功能
- 删除时不删除 Cloudflare 域名

**迁移方案**：

```go
// internal/sync/domain/domainregistration_controller.go (新文件)

type DomainRegistrationSyncController struct {
    *common.BaseSyncController
}

func (r *DomainRegistrationSyncController) sync(
    ctx context.Context,
    syncState *v1alpha2.CloudflareSyncState,
) error {
    // 1. 获取域名信息
    domainInfo, err := r.cfAPI.GetRegistrarDomain(syncState.Spec.CloudflareID)
    if err != nil {
        return err
    }

    // 2. 如果配置有变更，更新
    config := r.extractConfig(syncState)
    if r.hasConfigChanges(config, domainInfo) {
        _, err = r.cfAPI.UpdateRegistrarDomain(syncState.Spec.CloudflareID, cf.RegistrarDomainParams{
            AutoRenew:    config.AutoRenew,
            Locked:       config.Locked,
            Privacy:      config.Privacy,
            NameServers:  config.NameServers,
        })
        if err != nil {
            return err
        }
    }

    // 3. 更新 Status（元数据同步）
    return r.updateStatusWithDomainInfo(ctx, syncState, domainInfo)
}
```

**文件变更**：
| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/service/domain/domainregistration_service.go` | 新建 | 域名注册服务 |
| `internal/sync/domain/domainregistration_controller.go` | 新建 | 域名注册 Sync Controller |
| `internal/controller/domainregistration/controller.go` | 修改 | 移除 API 调用，使用 Service |

---

#### 8. AccessTunnel/WARPConnector 补全

**当前状态**：框架完成，缺少 L3 Service 和 L5 Sync Controller

**迁移方案**：
- 创建 `internal/service/access/tunnel_service.go`
- 创建 `internal/sync/access/tunnel_controller.go`
- 创建 `internal/service/warp/connector_service.go`
- 创建 `internal/sync/warp/connector_controller.go`

---

## 实施时间表

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           实施时间表                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Week 1: P0 修复                                                            │
│  ├── Day 1-2: AccessApplication policies 同步                               │
│  │   ├── 修改 application_controller.go                                     │
│  │   ├── 添加 syncPolicies() 方法                                           │
│  │   └── 测试 policies CRUD                                                 │
│  └── Day 3-4: NetworkRoute 幂等性                                           │
│      ├── 添加采用逻辑                                                        │
│      ├── 添加冲突处理                                                        │
│      └── 测试并发创建                                                        │
│                                                                             │
│  Week 2-3: P1 架构统一                                                      │
│  ├── Day 5-8: Tunnel/ClusterTunnel 迁移                                     │
│  │   ├── 创建 lifecycle_service.go                                          │
│  │   ├── 创建 lifecycle_controller.go                                       │
│  │   ├── 重构 generic_tunnel_reconciler.go                                  │
│  │   └── 全面测试                                                            │
│  ├── Day 9-10: Ingress/Gateway 清理                                         │
│  │   ├── 验证现有实现                                                        │
│  │   └── 移除临时 API 客户端                                                 │
│  └── Day 11-14: CloudflareDomain Sync Controller                            │
│      ├── 创建 cloudflaredomain_controller.go                                │
│      ├── 实现 Zone 验证逻辑                                                  │
│      ├── 实现 Settings 同步                                                  │
│      └── 统一 API 客户端                                                     │
│                                                                             │
│  Week 4: P2 例外迁移                                                        │
│  ├── Day 15-17: OriginCACertificate                                         │
│  │   ├── 创建 Sync Controller                                               │
│  │   ├── 处理证书内容传递                                                    │
│  │   └── 测试续期逻辑                                                        │
│  ├── Day 18-19: DomainRegistration                                          │
│  │   ├── 创建 Service + Sync Controller                                     │
│  │   └── 测试读写操作                                                        │
│  └── Day 20: AccessTunnel/WARPConnector                                     │
│      └── 创建基本框架                                                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 迁移后架构状态

| 资源                     | L2 Controller | L3 Service | L4 SyncState | L5 Sync Ctrl | 完成度 |
| ------------------------ | :-----------: | :--------: | :----------: | :----------: | :----: |
| **Tunnel**               |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **ClusterTunnel**        |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **TunnelBinding**        |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **DNSRecord**            |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **VirtualNetwork**       |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **NetworkRoute**         |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **PrivateService**       |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **AccessApplication**    |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **AccessGroup**          |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **AccessServiceToken**   |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **AccessIdentityProvider** |    ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **AccessTunnel**         |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **DevicePostureRule**    |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **DeviceSettingsPolicy** |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **GatewayRule**          |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **GatewayList**          |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **GatewayConfiguration** |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **R2Bucket**             |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **R2BucketDomain**       |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **R2BucketNotification** |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **ZoneRuleset**          |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **TransformRule**        |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **RedirectRule**         |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **CloudflareDomain**     |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **OriginCACertificate**  |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **DomainRegistration**   |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **Ingress**              |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **Gateway**              |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |
| **WARPConnector**        |      ✅       |     ✅     |      ✅      |      ✅      |  100%  |

**目标**：所有 30 个 CRD 100% 遵循六层架构，无例外。

---

## 验证清单

### 每个资源的验证点

- [ ] Resource Controller 不持有 `cfAPI` 字段
- [ ] Resource Controller 不直接调用 `cf.*` API 方法
- [ ] Resource Controller 通过 Service 的 `Register()/Unregister()` 操作
- [ ] Service 继承 `BaseService`
- [ ] Service 使用 `GetOrCreateSyncState()` 和 `UpdateSource()`
- [ ] Sync Controller 继承 `BaseSyncController`
- [ ] Sync Controller 使用防抖器
- [ ] Sync Controller 使用 Hash 检测变化
- [ ] 删除逻辑在 Sync Controller 中处理
- [ ] 错误处理使用 `cf.IsNotFoundError()`, `cf.IsConflictError()`

### 集成测试

- [ ] 创建资源 → 验证 SyncState 创建
- [ ] 更新资源 → 验证 SyncState 更新
- [ ] 删除资源 → 验证 Cloudflare 资源删除
- [ ] 并发创建 → 验证无冲突
- [ ] 外部删除 → 验证重新创建
- [ ] Operator 重启 → 验证状态恢复

---

## 风险和缓解措施

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| Tunnel 迁移复杂度高 | 可能引入回归 | 分阶段迁移，每阶段完整测试 |
| OriginCACertificate 私钥处理 | 安全风险 | 保持现有 Secret 管理模式不变 |
| 并发迁移冲突 | 数据不一致 | 使用乐观锁，添加重试逻辑 |
| 向后兼容性 | 现有部署受影响 | 保持 CRD API 不变，仅重构内部实现 |

---

## 附录：文件变更清单

### 新建文件

```
internal/
├── service/
│   ├── tunnel/lifecycle_service.go
│   ├── domain/
│   │   ├── cloudflaredomain_service.go (修改)
│   │   ├── domainregistration_service.go
│   │   └── origincacertificate_service.go (修改)
│   ├── access/tunnel_service.go
│   └── warp/connector_service.go
├── sync/
│   ├── tunnel/lifecycle_controller.go
│   ├── domain/
│   │   ├── cloudflaredomain_controller.go
│   │   ├── domainregistration_controller.go
│   │   └── origincacertificate_controller.go
│   ├── access/tunnel_controller.go
│   └── warp/connector_controller.go
└── clients/cf/
    └── zone.go (如需要)
```

### 修改文件

```
internal/
├── controller/
│   ├── tunnel_controller.go
│   ├── clustertunnel_controller.go
│   ├── generic_tunnel_reconciler.go
│   ├── cloudflaredomain/controller.go
│   ├── origincacertificate/controller.go
│   ├── domainregistration/controller.go
│   └── ingress/controller.go (验证)
├── sync/
│   ├── access/application_controller.go (policies)
│   └── networkroute/controller.go (幂等性)
└── cmd/main.go (注册新控制器)
```
